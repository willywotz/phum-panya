#!/bin/sh
# deploy/backup/backup.sh — daily pg_dump into /backups, keep newest 14.
# Env: PGHOST, PGUSER, PGPASSWORD, PGDATABASE (standard libpq vars).
set -eu

OUT_DIR="${BACKUP_DIR:-/backups}"
KEEP="${BACKUP_KEEP:-14}"
mkdir -p "$OUT_DIR"

while true; do
	stamp="$(date +%Y-%m-%d)"
	file="$OUT_DIR/backup-$stamp.sql.gz"
	if pg_dump | gzip -c > "$file.tmp"; then
		mv "$file.tmp" "$file"
		echo "backup: wrote $file"
	else
		rm -f "$file.tmp"
		echo "backup: pg_dump failed" >&2
	fi
	# Prune: keep the newest $KEEP dumps.
	ls -1t "$OUT_DIR"/backup-*.sql.gz 2>/dev/null | tail -n +"$((KEEP + 1))" | while read -r old; do
		rm -f "$old"
		echo "backup: pruned $old"
	done
	sleep 86400
done
