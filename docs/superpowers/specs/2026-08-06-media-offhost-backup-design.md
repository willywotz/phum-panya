# Off-host, versioned media backup

Date: 2026-08-06
Status: Approved (design)
Scope: `deploy/backup/`, `docker-compose.yaml`, `.env.example`, CI env block, docs
Extends: ADR-0004 (media in Garage), its single-node caveat

## Problem

Uploaded media lives in a single-node Garage store (`replication_factor = 1`,
ADR-0004). The only durability measure is the `media-backup` rclone sidecar, which does
`rclone sync garage:$BUCKET /backups/media` — a Docker volume **on the same host**. This
leaves two gaps:

1. **One host, one disk domain.** Losing the VPS disk loses Garage *and* its backup
   together. There is no off-host durability.
2. **`rclone sync` mirrors deletions.** An accidental or malicious delete (or corruption)
   in Garage propagates to the backup on the next run. There is no history and no
   point-in-time recovery.

Multi-node Garage on the single VPS would be durability theater — extra nodes share the
same physical disk, so `replication_factor > 1` there survives a container crash but not
a disk loss. Real durability needs a second failure domain.

## Decision

Repoint the `media-backup` sidecar from the on-host volume to a **single env-configured,
backend-agnostic rclone remote** (S3 or SFTP, chosen at deploy time), and make the backup
**versioned**:

- Each run: `rclone sync garage:$BUCKET <remote>:$PATH/current --backup-dir
  <remote>:$PATH/archive/<stamp>`. The `current` mirror stays live; any overwritten or
  deleted object is moved into a timestamped `archive/<stamp>` instead of being lost.
- A retention step prunes `archive/*` dirs older than `MEDIA_BACKUP_KEEP_DAYS`.

This **replaces** the local-volume backup rather than adding a second tier (YAGNI). It
closes both gaps: off-host survives host/disk loss; the dated archive gives point-in-time
recovery.

### Alternatives considered

- **Harden the local backup only** (sync→copy + archive on the same host). Rejected: does
  not survive host/disk loss — the primary risk.
- **True multi-node Garage cluster.** Rejected here: requires a second host and changes
  the single-VPS topology (ADR-0001, ADR-0003); larger scope than the durability gap
  needs.
- **Fixed S3 backend.** Rejected: an env-driven remote lets the operator pick S3 or SFTP
  with no code change and keeps the fully self-hosted path open.

## Design

### Script structure

Split the current single script into a pure library and a thin runner.

`deploy/backup/media-backup-lib.sh` — pure POSIX shell, no rclone calls, no `date` calls
inside the functions (time is injected):

- `archive_path(now_epoch, iso)` → `archive/<now_epoch>-<iso>`. The epoch prefix makes
  archive dirs numerically prunable; the ISO suffix keeps them human-readable.
- `prune_plan(names, keep_seconds, now_epoch)` → the subset of `names` whose leading
  epoch is older than `now_epoch - keep_seconds`. Names at exactly the cutoff are kept.
- `require_env(names...)` → exits non-zero and prints the first unset variable's name;
  exits 0 when all are set.

`deploy/backup/media-backup.sh` — the runner:

1. Sources the lib.
2. `require_env MEDIA_BACKUP_REMOTE MEDIA_BACKUP_PATH RCLONE_CONFIG_<REMOTE>_TYPE`
   (resolved from `MEDIA_BACKUP_REMOTE`) — fail loudly on a misconfigured sidecar
   instead of syncing nowhere.
3. Computes `now_epoch`/`iso` once via `date`, builds the archive path with
   `archive_path`.
4. Runs the rclone `sync` + `--backup-dir`.
5. Lists `archive/*` and prunes per `prune_plan` (via `rclone purge`).
6. `MEDIA_BACKUP_ONCE=1` runs one iteration and exits (manual verification); default is
   the existing interval loop.

The Garage source config (`RCLONE_CONFIG_GARAGE_*`) is unchanged.

### Environment and compose

`docker-compose.yaml` `media-backup` service: destination changes from the `/backups`
volume to the env-driven remote; the old `/backups/media` volume mount is removed (drop
the named volume if unused elsewhere). New env vars, documented in `.env.example`:

- `MEDIA_BACKUP_REMOTE` — rclone remote name (e.g. `dest`).
- `MEDIA_BACKUP_PATH` — base path/bucket on that remote (e.g. `media-backup`).
- `MEDIA_BACKUP_KEEP_DAYS` — archive retention in days, **default `30`**.
- `MEDIA_BACKUP_INTERVAL` — unchanged, default `86400`.
- `RCLONE_CONFIG_<REMOTE>_*` — the remote's rclone config (`TYPE`, `ENDPOINT`,
  `ACCESS_KEY_ID`, `SECRET_ACCESS_KEY`, ... for s3; `TYPE`, `HOST`, `USER`, key for
  sftp), passed straight through to the sidecar.

`.env.example` documents a concrete s3 default with placeholders plus a commented sftp
alternative.

### Dev stack

No change. `docker-compose.dev.yaml` still runs no `media-backup` sidecar (dev media is
disposable). The change is prod-only.

### CI

Add the new required env vars to the `stack-validate` job env block the same way the
S3/Garage vars were added, so `docker compose config` passes. No new CI job — the Go test
rides in the existing `go test ./...` suite.

## Restore

Documented in ADR-0009 and a short runbook line:

- Latest: `rclone copy <remote>:$PATH/current garage:$BUCKET`.
- Point-in-time: `rclone copy <remote>:$PATH/archive/<stamp> garage:$BUCKET`.

No application change — the api reads from Garage as normal once objects are restored.

## Testing (mandatory TDD, pure logic)

A Go test `deploy/backup/backup_test.go` execs the lib functions via
`sh -c '. media-backup-lib.sh; <fn> <args>'` (needs only `sh`; runs inside
`go test ./...`; no rclone, no CI change). rclone itself is trusted and not exercised.

1. `archive_path(now_epoch, iso)` with fixed inputs → exact expected `archive/<epoch>-<iso>`.
2. `prune_plan` → returns only names older than the cutoff, keeps the rest, keeps the
   boundary name at exactly `keep_seconds`.
3. `require_env` → exit 0 when all set; non-zero and names the missing variable when one
   is unset.

## Out of scope

- Multi-node / multi-host Garage clustering.
- Restoring media automatically (restore is an operator runbook step).
- Changing the api, the `media.Store` port, or dev media behavior.
