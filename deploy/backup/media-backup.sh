#!/bin/sh
set -eu

DIR=$(dirname "$0")
. "$DIR/media-backup-lib.sh"

BUCKET="$APP_S3_BUCKET"
DEST_PATH="$MEDIA_BACKUP_PATH"
KEEP_DAYS="${MEDIA_BACKUP_KEEP_DAYS:-30}"
INTERVAL="${MEDIA_BACKUP_INTERVAL:-86400}"

# Garage source (unchanged).
export RCLONE_CONFIG_GARAGE_TYPE=s3
export RCLONE_CONFIG_GARAGE_PROVIDER=Other
export RCLONE_CONFIG_GARAGE_ENDPOINT="$APP_S3_ENDPOINT"
export RCLONE_CONFIG_GARAGE_ACCESS_KEY_ID="$APP_S3_ACCESS_KEY"
export RCLONE_CONFIG_GARAGE_SECRET_ACCESS_KEY="$APP_S3_SECRET_KEY"
export RCLONE_CONFIG_GARAGE_FORCE_PATH_STYLE=true

require_env APP_S3_BUCKET MEDIA_BACKUP_PATH RCLONE_CONFIG_DEST_TYPE

prune_archives() {
	now_epoch=$1
	keep_seconds=$((KEEP_DAYS * 86400))
	names=$(rclone lsf --dirs-only "dest:$DEST_PATH/archive" 2>/dev/null | sed 's:/*$::') || return 0
	# shellcheck disable=SC2086
	for name in $(prune_plan "$keep_seconds" "$now_epoch" $names); do
		echo "media-backup: pruning archive/$name"
		rclone purge "dest:$DEST_PATH/archive/$name" || echo "media-backup: prune failed for $name" >&2
	done
}

run_once() {
	now_epoch=$(date -u +%s)
	iso=$(date -u +%Y-%m-%dT%H-%M-%SZ)
	archive=$(archive_path "$now_epoch" "$iso")
	echo "media-backup: sync garage:$BUCKET -> dest:$DEST_PATH/current (archive $archive)"
	if ! rclone sync "garage:$BUCKET" "dest:$DEST_PATH/current" \
		--backup-dir "dest:$DEST_PATH/$archive"; then
		echo "media-backup: sync failed" >&2
		return 1
	fi
	prune_archives "$now_epoch"
}

if [ "${MEDIA_BACKUP_ONCE:-0}" = "1" ]; then
	run_once
else
	while true; do
		run_once || true
		sleep "$INTERVAL"
	done
fi
