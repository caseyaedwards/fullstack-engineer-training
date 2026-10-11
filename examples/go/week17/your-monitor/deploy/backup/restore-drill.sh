#!/bin/sh
# region: drill-script
# restore-drill.sh: fetches the newest backup from BACKUP_REMOTE,
# decrypts it with the age identity file AGE_IDENTITY ("-" reads it from
# standard input), restores it into a new, empty Postgres server in
# DRILL_DIR, and checks the row counts against the counts that backup.sh
# wrote beside the dump. LIVE_MONITORS, the monitor count of the live
# database, catches a backup of the wrong or an empty database: it must
# be set, and a number.
set -eu
work=${DRILL_DIR:-/tmp/drill}
backups=${BACKUP_DIR:-$work}
max_age=${MAX_AGE_HOURS:-26}
start=$(date +%s)

fail() { echo "restore drill: FAILED: $*"; exit 1; }

# An empty value is what a failed kubectl in $(...) leaves behind, and
# the drill would then pass a backup of the wrong database.
live=${LIVE_MONITORS-}
case $live in
  '' | *[!0-9]*) fail "LIVE_MONITORS must be the monitor count of the live database, a number; got '$live'" ;;
esac

# The names sort by time: the last .counts is the newest complete backup.
list=$(rclone lsf --include 'monitor-*.counts' "$BACKUP_REMOTE") ||
  fail "rclone could not list $BACKUP_REMOTE"
name=$(printf '%s\n' "$list" | sort | tail -n 1)
[ -n "$name" ] || fail "no dump in $BACKUP_REMOTE"
name=${name%.counts}
taken=$(echo "$name" | sed -E 's/monitor-(....)(..)(..)-(..)(..)(..)/\1-\2-\3 \4:\5:\6/')
secs=$((start - $(date -u -d "$taken" +%s)))
# A name from the future is no backup of this database: only someone
# else's upload, or a clock that is wrong, makes one. Five minutes of
# skew between the clocks of the server and the laptop pass.
[ "$secs" -ge -300 ] || fail "the newest dump, $name, is dated more than 5 minutes in the future"
age=$((secs / 3600))
[ "$age" -lt "$max_age" ] || fail "the newest dump, $name, is $age hours old"

mkdir -p "$backups" "$work"
for f in "$name.dump.age" "$name.counts"; do
  rclone copyto "$BACKUP_REMOTE/$f" "$backups/$f" || fail "rclone could not fetch $f"
done
age --decrypt --identity="$AGE_IDENTITY" --output="$work/$name.dump" "$backups/$name.dump.age" ||
  fail "age could not decrypt $name.dump.age"

# A brand-new server: its own data directory, no TCP port, a socket in
# $work. Nothing the live database holds exists here before the restore.
initdb --pgdata="$work/data" --username=drill --auth=trust >/dev/null
pg_ctl --pgdata="$work/data" --options="-c listen_addresses='' -k $work" \
  --log="$work/server.log" --wait start >/dev/null
trap 'pg_ctl --pgdata="$work/data" --mode=fast --wait stop >/dev/null' EXIT
createdb --host="$work" --username=drill monitor

restore_start=$(date +%s)
pg_restore --host="$work" --username=drill --dbname=monitor \
  --no-owner --no-privileges --exit-on-error "$work/$name.dump" || fail "pg_restore exited with $?"
restore_secs=$(($(date +%s) - restore_start))

restored=$(psql --host="$work" --username=drill --dbname=monitor -XAt -F ' ' -c "
  SELECT (SELECT count(*) FROM monitors), (SELECT count(*) FROM check_results),
    (SELECT count(*) FROM incidents), (SELECT max(version) FROM schema_migrations)") ||
  fail "a table is missing from the restore"
want=$(cat "$backups/$name.counts")

echo "dump:     $name.dump.age, $(($(stat -c %s "$backups/$name.dump.age") / 1024)) KiB, $age hours old"
echo "          monitors results incidents schema"
echo "restored: $restored"
echo "backup:   $want"
[ "$restored" = "$want" ] || fail "the restored counts differ from the backup counts"
[ "${restored%% *}" != 0 ] || [ "$live" = 0 ] ||
  fail "the restored database has no monitors, the live one has $live"
echo "restore drill: ok, restore $restore_secs s, whole drill $(($(date +%s) - start)) s"
# endregion: drill-script
