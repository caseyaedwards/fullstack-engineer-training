#!/bin/sh
# region: backup-script
# backup.sh: dumps the database that the PG* variables name, counts its
# rows in the same snapshot, encrypts the dump to the age public key in
# AGE_RECIPIENT, and uploads the dump and the counts to BACKUP_REMOTE
# with rclone. The dump leaves the pod only encrypted. It pings
# BACKUP_PING_URL, the pg-backup check of healthchecks.io, when it ends:
# the URL itself on success, the URL with /fail added on a failure.
set -eu
ping=${BACKUP_PING_URL:?set BACKUP_PING_URL in the offsite Secret}
work=${WORK_DIR:-/work}
name="monitor-$(date -u +%Y%m%d-%H%M%S)"
start=$(date +%s)

fail() {
  curl -fsS -m 10 --retry 3 -o /dev/null "$ping/fail" || true
  echo "backup: FAILED: $*"
  exit 1
}

# One read-only transaction exports its snapshot. pg_dump dumps exactly
# that snapshot, and the counts come from it too, however many results
# the worker writes in the meantime.
DUMP="$work/$name.dump" psql -X -q -At -F ' ' -v ON_ERROR_STOP=1 -o "$work/$name.counts" <<'SQL' ||
BEGIN ISOLATION LEVEL REPEATABLE READ, READ ONLY;
SELECT pg_export_snapshot() AS snapshot \gset
\setenv SNAPSHOT :snapshot
\! pg_dump --format=custom --snapshot="$SNAPSHOT" --file="$DUMP" || rm -f "$DUMP"
SELECT (SELECT count(*) FROM monitors), (SELECT count(*) FROM check_results),
  (SELECT count(*) FROM incidents), (SELECT max(version) FROM schema_migrations);
COMMIT;
SQL
  fail "psql exited with $?"
[ -s "$work/$name.dump" ] || fail "pg_dump wrote no dump"
pg_restore --list "$work/$name.dump" >/dev/null || fail "the dump does not read back"

age --encrypt --recipient="$AGE_RECIPIENT" --output="$work/$name.dump.age" "$work/$name.dump" ||
  fail "age exited with $?"
rm "$work/$name.dump"

# rclone does not list the bucket, check whether it exists, or read an
# object back: the upload key has no right to. No provider offers a key
# that can only add objects: this one can write, and on B2 hide files,
# and the versioning and lifecycle rule of the bucket cover that. The
# counts go last, so a .counts object always has its dump beside it.
for f in "$name.dump.age" "$name.counts"; do
  rclone copyto --no-check-dest --s3-no-check-bucket --s3-no-head \
    "$work/$f" "$BACKUP_REMOTE/$f" || fail "rclone could not upload $f"
done

echo "backup: $name.dump.age, $(($(stat -c %s "$work/$name.dump.age") / 1024)) KiB," \
  "counts $(cat "$work/$name.counts"), in $(($(date +%s) - start)) s"
# Last: the check goes green only after both uploads. A failed ping does
# not fail the Job: the backup is safe, a retry would take a second one,
# and the missing ping makes healthchecks.io alert anyway.
curl -fsS -m 10 --retry 3 -o /dev/null "$ping" ||
  echo "backup: the success ping failed; healthchecks.io will alert"
# endregion: backup-script
