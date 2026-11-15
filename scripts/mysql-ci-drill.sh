#!/usr/bin/env bash
# Run only against a disposable MySQL container with synthetic ledger data.
set -euo pipefail

work=$(mktemp -d)
container="tinynest-mysql-drill-${work##*/}"
cleanup() {
  docker rm -f "$container" >/dev/null 2>&1 || true
  rm -rf -- "$work"
}
trap cleanup EXIT
mkdir "$work/socket"
printf '%s' 'tinynest-ci-fixture-password' > "$work/password"

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
     mysql_root mysqladmin --protocol=socket --user=root ping >/dev/null 2>&1; then
    ready=true
    break
  fi
  sleep 1
done
if [[ "$ready" != true ]]; then
  docker logs "$container"
  exit 1
fi

mysql_root mysql --protocol=socket --user=root <<'SQL'
CREATE DATABASE tinynest_drill;
CREATE DATABASE tinynest_restore;
CREATE USER 'tinynest_drill'@'localhost' IDENTIFIED BY 'tinynest-ci-fixture-password';
GRANT ALL ON tinynest_drill.* TO 'tinynest_drill'@'localhost';
SQL

TINYNEST_TEST_MYSQL_SOCKET="$work/socket/mysqld.sock" \
TINYNEST_TEST_MYSQL_DATABASE=tinynest_drill \
TINYNEST_TEST_MYSQL_USER=tinynest_drill \
TINYNEST_TEST_MYSQL_PASSWORD_FILE="$work/password" \
go test ./internal/api -run '^TestV01EndToEnd$' -count=1

dump=(mysqldump --protocol=socket --user=root --single-transaction --no-tablespaces
      --set-gtid-purged=OFF --skip-comments --order-by-primary)
mysql_root "${dump[@]}" tinynest_drill > "$work/backup.sql"
mysql_root mysql --protocol=socket --user=root tinynest_restore < "$work/backup.sql"
mysql_root "${dump[@]}" --no-create-info tinynest_drill > "$work/source-data.sql"
mysql_root "${dump[@]}" --no-create-info tinynest_restore > "$work/restored-data.sql"
cmp "$work/source-data.sql" "$work/restored-data.sql"

balanced=$(mysql_root mysql --protocol=socket --user=root --batch --skip-column-names \
  tinynest_restore -e 'SELECT COUNT(*) > 0 AND SUM(debit_minor) = SUM(credit_minor) FROM journal_lines')
test "$balanced" = 1
printf '%s\n' 'MySQL Unix socket, ledger smoke test, restore and journal reconciliation passed.'
