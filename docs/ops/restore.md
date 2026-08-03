# Restore from Backup

This procedure restores the database and media files from a nightly backup
zip (`backup-<date>.zip`, produced by `internal/backup`).

## Steps

1. Stop the `phum-panya` service.
2. Choose the backup zip to restore, for example
   `backup-2026-08-01.zip`.
3. Unzip it to a working directory:
   ```
   unzip backup-2026-08-01.zip -d /tmp/restore
   ```
4. Copy `app.db` from the unzipped folder to the path set in `APP_DB_PATH`,
   replacing the existing file.
5. Copy the contents of the unzipped `media/` folder to the path set in
   `APP_MEDIA_DIR`, replacing existing files.
6. Start the `phum-panya` service.
7. Confirm the app loads and recent data looks correct.

## Off-site copies

This backup only writes zips to local disk (`outDir`, pruned to the newest
`keep` files). Copying backup zips off-site (for example to cloud storage or
another host) is the system owner's responsibility. Do this on a regular
schedule to protect against disk or host loss.
