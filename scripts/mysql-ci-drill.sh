#!/usr/bin/env bash
# Production-dialect drill for the V2 bookkeeping baseline.
#
# Runs only against a disposable MySQL container with synthetic data. It covers
# four things a local SQLite run cannot prove:
#   1. the V2 baseline and the generic migrator apply on MySQL 8;
#   2. the bookkeeping API behaves identically on the production dialect;
#   3. the destructive purge keeps identity rows and empties every business table
#      (the exact rehearsal for the production runbook);
#   4. identity export/import into a freshly created V2 schema (the cutover).
#
# No real credentials, database files or backups ever leave the runner.
set -euo pipefail

work=$(mktemp -d)
container="tinynest-mysql-drill-${work##*/}"
stage='prepare fixture'
cleanup() {
  local status=$?
  if ((status != 0)); then
    printf 'MySQL drill failed during %s (exit %s).\n' "$stage" "$status" >&2
    if [[ "$stage" = 'start server' ]]; then
      docker logs --tail 80 "$container" >&2 || true
    fi
  fi
  docker stop --time 10 "$container" >/dev/null 2>&1 || true
  docker rm -f "$container" >/dev/null 2>&1 || true
  rm -rf -- "$work"
}
trap cleanup EXIT
# The image can chown the bind mount to its mysql user. Keep it writable for
# runner cleanup even after a forced stop leaves socket files behind.
mkdir -m 0777 "$work/socket"
printf '%s' 'tinynest-ci-fixture-password' > "$work/password"

stage='start server'
docker run --detach --name "$container" --network none \
  --volume "$work/socket:/var/run/mysqld" \
  --env MYSQL_ROOT_PASSWORD=tinynest-ci-fixture-root \
  mysql:8.0 >/dev/null

mysql_root() {
  docker exec -i "$container" sh -c 'export MYSQL_PWD="$MYSQL_ROOT_PASSWORD"; exec "$@"' sh "$@"
}
# The entrypoint also starts a temporary server. Wait for the final server
# executable to take over PID 1 before using the socket.
ready=false
for ((attempt=0; attempt<90; attempt++)); do
  if docker exec "$container" sh -c 'test "$(cat /proc/1/comm)" = mysqld' &&
     mysql_root mysql --protocol=socket --user=root -e 'SELECT 1' >/dev/null 2>&1; then
    ready=true
    break
  fi
  sleep 1
done
if [[ "$ready" != true ]]; then
  mysql_root mysql --protocol=socket --user=root -e 'SELECT 1' >&2 || true
  exit 1
fi

# One fresh schema per test: every API test initializes its own household and
# POST /api/v1/setup is single-use by design.
databases=(
  lifecycle correction reversal idempotency isolation stats budget
  setup members accounts account_delete category_delete categories household
  profile mailquota smtp migrations purge cutover cutover_alt restore
)

stage='provision test databases'
mysql_root mysql --protocol=socket --user=root \
  -e "CREATE USER 'tinynest_drill'@'localhost' IDENTIFIED BY 'tinynest-ci-fixture-password';"
for name in "${databases[@]}"; do
  mysql_root mysql --protocol=socket --user=root \
    -e "CREATE DATABASE tinynest_${name}; GRANT ALL ON tinynest_${name}.* TO 'tinynest_drill'@'localhost';"
done

stage='check the host socket'
test -S "$work/socket/mysqld.sock"

mysql_env() {
  local database=$1
  shift
  TINYNEST_TEST_MYSQL_SOCKET="$work/socket/mysqld.sock" \
  TINYNEST_TEST_MYSQL_DATABASE="$database" \
  TINYNEST_TEST_MYSQL_USER=tinynest_drill \
  TINYNEST_TEST_MYSQL_PASSWORD_FILE="$work/password" \
  "$@"
}

run_api_test() {
  local database=$1 test_name=$2
  shift 2
  stage="run $test_name over the host socket"
  mysql_env "$database" go test ./internal/api -run "^$test_name$" -count=1 -parallel=16 "$@"
}

run_dbadmin_test() {
  local database=$1 test_name=$2
  shift 2
  stage="run $test_name over the host socket"
  mysql_env "$database" go test ./internal/dbadmin -run "^$test_name$" -count=1 -parallel=4 "$@"
}

# --- bookkeeping behaviour on the production dialect -------------------------
run_api_test tinynest_lifecycle      TestBookkeepingLifecycle
run_api_test tinynest_correction     TestTransactionCorrectionIsReversalPlusReplacement
run_api_test tinynest_reversal       TestTransactionDeleteIsReversalOnly
run_api_test tinynest_idempotency    TestIdempotentTransactionReplay
run_api_test tinynest_isolation      TestHouseholdIsolationTransactionsAndStats
run_api_test tinynest_stats          TestStatsConsistency
run_api_test tinynest_budget         TestBudgetUpsertAndSpent
run_api_test tinynest_setup          TestSetupIsSingleUseAndSessionAuthEnforced
run_api_test tinynest_members        TestCurrentHouseholdMember
run_api_test tinynest_accounts       TestAccountMaintenance
run_api_test tinynest_account_delete TestDeleteAccountRetainsLedger
run_api_test tinynest_category_delete TestDeleteCategoryRetainsHistory
run_api_test tinynest_categories     TestCategoriesAreSingleLevel
run_api_test tinynest_household      TestHouseholdIsolation
run_api_test tinynest_profile        TestUserSelfServiceProfileAndPassword
run_api_test tinynest_mailquota      TestRecipientDailyLimitSharedAndReset
run_api_test tinynest_smtp           TestGenericSMTPProviderConfiguration
run_api_test tinynest_migrations     TestMigrationsAreIdempotent

# --- destructive purge rehearsal --------------------------------------------
run_dbadmin_test tinynest_purge TestMySQLPurgeDrill

# --- identity cutover rehearsal into a fresh V2 baseline --------------------
stage='run the identity cutover rehearsal'
mysql_env tinynest_cutover env \
  TINYNEST_TEST_MYSQL_DATABASE_ALT=tinynest_cutover_alt \
  go test ./internal/dbadmin -run '^TestMySQLIdentityCutoverDrill$' -count=1

# --- backup / restore / journal reconciliation -------------------------------
dump=(mysqldump --protocol=socket --user=root --single-transaction --no-tablespaces
      --set-gtid-purged=OFF --skip-comments --order-by-primary)
stage='dump source database'
mysql_root "${dump[@]}" tinynest_lifecycle > "$work/backup.sql"
stage='restore into empty database'
mysql_root mysql --protocol=socket --user=root tinynest_restore < "$work/backup.sql"
stage='compare source and restored records'
mysql_root "${dump[@]}" --no-create-info tinynest_lifecycle > "$work/source-data.sql"
mysql_root "${dump[@]}" --no-create-info tinynest_restore > "$work/restored-data.sql"
cmp "$work/source-data.sql" "$work/restored-data.sql"

stage='reconcile restored journal'
balanced=$(mysql_root mysql --protocol=socket --user=root --batch --skip-column-names \
  tinynest_restore -e 'SELECT COUNT(*) > 0 AND SUM(debit_minor) = SUM(credit_minor) FROM journal_lines')
test "$balanced" = 1

stage='reconcile baseline metadata'
baseline=$(mysql_root mysql --protocol=socket --user=root --batch --skip-column-names \
  tinynest_restore -e "SELECT value FROM schema_meta WHERE meta_key='baseline'")
test "$baseline" = v2

printf '%s\n' 'MySQL V2 drill passed: migrations, bookkeeping API, purge, identity cutover and restore.'
