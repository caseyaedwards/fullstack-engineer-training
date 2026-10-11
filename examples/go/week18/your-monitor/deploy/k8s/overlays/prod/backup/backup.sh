#!/bin/sh
# region: backup-script
# backup.sh: dumps the database that the PG* variables name (PGHOST,
# PGUSER, PGPASSWORD, PGDATABASE) into BACKUP_DIR, in the custom format,
# and deletes dumps that are KEEP_DAYS days old or older.
set -eu
dir=${BACKUP_DIR:-/backups}
keep=${KEEP_DAYS:-14}
file="$dir/monitor-$(date -u +%Y%m%d-%H%M%S).dump"
start=$(date +%s)

# Write to .partial, then rename: a dump cut off halfway never carries
# the name that the restore drill looks for.
pg_dump --format=custom --file="$file.partial"
mv "$file.partial" "$file"

# -mtime +13 matches files 14 or more whole days old.
find "$dir" -name 'monitor-*.dump' -mtime +"$((keep - 1))" -print -delete
find "$dir" -name 'monitor-*.dump.partial' -mmin +60 -print -delete

echo "backup: $(basename "$file"), $(($(stat -c %s "$file") / 1024)) KiB in $(($(date +%s) - start)) s"
# endregion: backup-script
