#!/bin/sh
set -eu
# Wait for the Garage RPC to answer.
until garage status >/dev/null 2>&1; do sleep 1; done

# Assign + apply a layout once (idempotent: skip if a role is already set).
if ! garage layout show | grep -q "Zone"; then
  NODE_ID=$(garage node id -q | cut -d@ -f1)
  garage layout assign -z dc1 -c 1G "$NODE_ID"
  garage layout apply --version 1
fi

# Bucket (ignore "already exists").
garage bucket create "$APP_S3_BUCKET" 2>/dev/null || true

# Import the fixed key so the api's env credentials match (idempotent).
if ! garage key info "$APP_S3_ACCESS_KEY" >/dev/null 2>&1; then
  garage key import --yes "$APP_S3_ACCESS_KEY" "$APP_S3_SECRET_KEY" -n phumpanya
fi

garage bucket allow --read --write --key "$APP_S3_ACCESS_KEY" "$APP_S3_BUCKET"
echo "garage-init: done"
