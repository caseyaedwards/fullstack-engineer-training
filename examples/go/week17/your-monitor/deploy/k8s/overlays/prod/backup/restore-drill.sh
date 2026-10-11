#!/bin/sh
# region: drill-script
# restore-drill.sh: restores the newest dump in BACKUP_DIR into a new,
# empty Postgres server in DRILL_DIR, counts what comes back, and prints
# how long it took. It fails when there is no dump, when the newest one
# is MAX_AGE_HOURS old or older, or when any step of the restore fails.
set -eu
backups=${BACKUP_DIR:-/backups}
work=${DRILL_DIR:-/drill}
max_age=${MAX_AGE_HOURS:-26}
start=$(date +%s)

fail() { echo "restore drill: FAILED: $*"; exit 1; }

# The shell sorts the names, and the names sort by time: the last
# match is the newest dump.
dump=
for f in "$backups"/monitor-*.dump; do
  [ -e "$f" ] && dump=$f
done
[ -n "$dump" ] || fail "no dump in $backups"
age=$(((start - $(stat -c %Y "$dump")) / 3600))
[ "$age" -lt "$max_age" ] || fail "the newest dump, $(basename "$dump"), is $age hours old"

# A brand-new server: its own data directory, no TCP port, a socket in
# $work. Nothing the live database holds exists here before the restore.
initdb --pgdata="$work/data" --username=drill --auth=trust >/dev/null
pg_ctl --pgdata="$work/data" --options="-c listen_addresses='' -k $work" \
  --log="$work/server.log" --wait start >/dev/null
trap 'pg_ctl --pgdata="$work/data" --mode=fast --wait stop >/dev/null' EXIT
createdb --host="$work" --username=drill monitor

restore_start=$(date +%s)
pg_restore --host="$work" --username=drill --dbname=monitor \
  --no-owner --no-privileges --exit-on-error "$dump" || fail "pg_restore exited with $?"
restore_secs=$(($(date +%s) - restore_start))

counts="select (select count(*) from monitors), (select count(*) from check_results),
  (select count(*) from incidents), (select max(version) from schema_migrations)"
restored=$(psql --host="$work" --username=drill --dbname=monitor -XAt -F ' ' -c "$counts") ||
  fail "a table is missing from the restore"
live=$(psql -XAt -F ' ' -c "$counts") || fail "cannot read the live database"

echo "dump:     $(basename "$dump"), $(($(stat -c %s "$dump") / 1024)) KiB, $age hours old"
echo "          monitors results incidents schema"
echo "restored: $restored"
echo "live:     $live"
echo "restore drill: ok, restore $restore_secs s, whole drill $(($(date +%s) - start)) s"
# endregion: drill-script
