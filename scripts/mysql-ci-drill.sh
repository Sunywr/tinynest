#!/usr/bin/env bash
# Run only against a disposable MySQL container with synthetic ledger data.
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
# The image can chown the bind mount to its mysql user. Keep it writable
# for runner cleanup even after a forced stop leaves socket files behind.
# mktemp's private parent still limits host access to the runner.
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

stage='provision test databases'
mysql_root mysql --protocol=socket --user=root <<'SQL'
CREATE DATABASE tinynest_drill;
CREATE DATABASE tinynest_credit;
CREATE DATABASE tinynest_concurrency;
CREATE DATABASE tinynest_mixed_load;
CREATE DATABASE tinynest_restore;
CREATE USER 'tinynest_drill'@'localhost' IDENTIFIED BY 'tinynest-ci-fixture-password';
GRANT ALL ON tinynest_drill.* TO 'tinynest_drill'@'localhost';
GRANT ALL ON tinynest_credit.* TO 'tinynest_drill'@'localhost';
GRANT ALL ON tinynest_concurrency.* TO 'tinynest_drill'@'localhost';
GRANT ALL ON tinynest_mixed_load.* TO 'tinynest_drill'@'localhost';
SQL

stage='check the host socket'
test -S "$work/socket/mysqld.sock"
run_ledger_test() {
  local database=$1 test_name=$2
  shift 2
  stage="run $test_name over the host socket"
  TINYNEST_TEST_MYSQL_SOCKET="$work/socket/mysqld.sock" \
  TINYNEST_TEST_MYSQL_DATABASE="$database" \
  TINYNEST_TEST_MYSQL_USER=tinynest_drill \
  TINYNEST_TEST_MYSQL_PASSWORD_FILE="$work/password" \
  go test ./internal/api -run "^$test_name$" -count=1 -parallel=16 "$@"
}

# Each test owns a fresh schema and initializes its own household.
run_ledger_test tinynest_drill TestV01EndToEnd
run_ledger_test tinynest_credit TestV011_T1_CreditStatementLifecycle
run_ledger_test tinynest_concurrency TestConcurrentTransactionReplays
run_ledger_test tinynest_mixed_load TestMixedLedgerLoad -v

dump=(mysqldump --protocol=socket --user=root --single-transaction --no-tablespaces
      --set-gtid-purged=OFF --skip-comments --order-by-primary)
stage='dump source database'
mysql_root "${dump[@]}" tinynest_drill > "$work/backup.sql"
stage='restore into empty database'
mysql_root mysql --protocol=socket --user=root tinynest_restore < "$work/backup.sql"
stage='compare source and restored records'
mysql_root "${dump[@]}" --no-create-info tinynest_drill > "$work/source-data.sql"
mysql_root "${dump[@]}" --no-create-info tinynest_restore > "$work/restored-data.sql"
cmp "$work/source-data.sql" "$work/restored-data.sql"

stage='reconcile restored journal'
balanced=$(mysql_root mysql --protocol=socket --user=root --batch --skip-column-names \
  tinynest_restore -e 'SELECT COUNT(*) > 0 AND SUM(debit_minor) = SUM(credit_minor) FROM journal_lines')
test "$balanced" = 1
printf '%s\n' 'MySQL Unix socket, ledger, credit, concurrent idempotency, restore and journal reconciliation passed.'
