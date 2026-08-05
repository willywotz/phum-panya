#!/bin/sh
set -eu
export RCLONE_CONFIG_GARAGE_TYPE=s3
export RCLONE_CONFIG_GARAGE_PROVIDER=Other
export RCLONE_CONFIG_GARAGE_ENDPOINT="$APP_S3_ENDPOINT"
export RCLONE_CONFIG_GARAGE_ACCESS_KEY_ID="$APP_S3_ACCESS_KEY"
export RCLONE_CONFIG_GARAGE_SECRET_ACCESS_KEY="$APP_S3_SECRET_KEY"
export RCLONE_CONFIG_GARAGE_FORCE_PATH_STYLE=true

INTERVAL="${MEDIA_BACKUP_INTERVAL:-86400}"
while true; do
  echo "media-backup: syncing garage:$APP_S3_BUCKET -> /backups/media"
  rclone sync "garage:$APP_S3_BUCKET" /backups/media || echo "media-backup: sync failed"
  sleep "$INTERVAL"
done
