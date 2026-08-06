# Off-host versioned media backup

Status: accepted

Context: ADR-0004 moved media into a single-node Garage store
(`replication_factor = 1`) and added the `media-backup` rclone sidecar as the
interim durability measure. That sidecar ran `rclone sync garage:$BUCKET
/backups/media` into a Docker volume **on the same host**, which leaves two
gaps. First, everything lives in one disk domain: losing the VPS disk loses
Garage *and* its backup together, so there is no off-host durability.
Second, `rclone sync` mirrors deletions — an accidental or malicious delete
(or corruption) in Garage propagates to the backup on the next run, with no
history and no point-in-time recovery. Multi-node Garage on the single VPS
would not help: extra nodes share the same physical disk, so
`replication_factor > 1` there survives a container crash but not a disk
loss. Real durability needs a second failure domain.

## Decision

The `media-backup` sidecar now pushes to a single **off-host**, operator-chosen
rclone remote and keeps **versioned** backups:

- The remote is a fixed rclone label `dest`, configured entirely by
  `RCLONE_CONFIG_DEST_*` env vars, so the operator picks the backend (S3 or
  SFTP) at deploy time with no code change.
- Each run: `rclone sync garage:$APP_S3_BUCKET dest:$MEDIA_BACKUP_PATH/current
  --backup-dir dest:$MEDIA_BACKUP_PATH/archive/<stamp>`. The `current` mirror
  stays live; any overwritten or deleted object is moved into a timestamped
  `archive/<stamp>` instead of being lost.
- Archives older than `MEDIA_BACKUP_KEEP_DAYS` (default 30) are pruned.

The pure, backend-independent logic — archive-path construction, retention
selection, and required-env validation — lives in
`deploy/backup/media-backup-lib.sh` and is unit-tested by a Go test that execs
`sh` (so it runs inside `go test ./...` with no rclone dependency in CI); the
rclone I/O in `media-backup.sh` is trusted and not exercised in tests. This
**replaces** the on-host volume backup rather than adding a second tier.

## Why

The primary risk is loss of the single VPS disk, which only an off-host copy
survives; the env-driven remote keeps the fully self-hosted path open (SFTP to
a second machine) without forcing a third-party account, matching the
single-VPS deploy model (ADR-0001, ADR-0003). `--backup-dir` gives
point-in-time recovery on any backend, so protection against accidental
deletion does not depend on server-side object versioning. Fixing the remote
label to `dest` (rather than the spec's configurable `MEDIA_BACKUP_REMOTE`) is
a consequence of Docker Compose being unable to template env-var *names*
(`RCLONE_CONFIG_${REMOTE}_TYPE`); the backend stays fully configurable, only
the label is constant.

## Considered options

- **Harden the on-host backup only** (sync→copy plus a local dated archive).
  Rejected: closes the deletion-propagation gap but not the primary one — it
  still dies with the host disk.
- **True multi-node / multi-host Garage cluster.** Rejected here: needs a
  second host and changes the single-VPS topology; larger scope than the
  durability gap requires. Left as a future option if a second host appears.
- **Pin the backup to a fixed S3 provider.** Rejected: an env-driven remote
  lets an operator choose S3 or a self-hosted SFTP target without code change.

## Consequences

- **Media survives loss of the VPS disk.** Backups land on an off-host remote;
  the dated archive gives point-in-time recovery instead of a
  deletion-mirroring copy.
- **The old `media-backups` Docker volume is removed** from
  `docker-compose.yaml`; the sidecar mounts both `media-backup.sh` and
  `media-backup-lib.sh`.
- **Loud misconfiguration.** `require_env` fails the sidecar with a named
  message when the destination is not configured, instead of silently syncing
  nowhere.
- **CI is unchanged.** The new `RCLONE_CONFIG_DEST_*` vars carry empty
  `:-` defaults, so `docker compose config` in the `stack-validate` job passes
  with no new env.
- **Dev stack unaffected.** `docker-compose.dev.yaml` still runs no
  `media-backup` sidecar (dev media is disposable).
- **Restore is an operator step.** `rclone copy dest:$MEDIA_BACKUP_PATH/current
  garage:$APP_S3_BUCKET` for the latest, or `.../archive/<stamp>` for a point
  in time; no application change is needed once objects are back.
