# Off-host Versioned Media Backup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Repoint the `media-backup` rclone sidecar from an on-host Docker volume to a single env-configured off-host rclone remote, with dated archiving of overwritten/deleted objects and retention pruning.

**Architecture:** Split the shell into a pure, unit-tested `media-backup-lib.sh` (archive-path, prune-plan, require-env) and a thin `media-backup.sh` runner that does the rclone I/O. The runner syncs `garage:$BUCKET → dest:$PATH/current` with `--backup-dir dest:$PATH/archive/<stamp>`, then prunes archives older than the retention window. Compose passes the destination's rclone config through as `RCLONE_CONFIG_DEST_*`, so the operator picks S3 or SFTP at deploy time.

**Tech Stack:** POSIX shell, rclone, Docker Compose, Go (test harness only).

## Global Constraints

- ASD-STE100 Simplified Technical English in prose/docs.
- 15-Factor App and Hexagonal architecture compliance — no app/api change; media stays behind the existing `media.Store` port.
- Uber Go style for the Go test; American English names; organized (path-sorted) imports; minimal comments.
- TDD mandatory (pure logic): failing Go test → confirm fail → minimal shell → confirm pass. rclone itself is trusted and not exercised in tests.
- Backend-agnostic: the destination is one rclone remote named **`dest`**, configured entirely by `RCLONE_CONFIG_DEST_*` env vars.
- Retention default `MEDIA_BACKUP_KEEP_DAYS=30`; interval default `MEDIA_BACKUP_INTERVAL=86400`.
- Commit prefix commands with `rtk`.
- Spec: `docs/superpowers/specs/2026-08-06-media-offhost-backup-design.md`.

**Spec refinement (intentional):** the spec named a configurable `MEDIA_BACKUP_REMOTE`. Docker Compose cannot template env-var *names* (`RCLONE_CONFIG_${REMOTE}_TYPE`), so the rclone remote label is fixed to `dest`. The backend remains fully operator-configurable via `RCLONE_CONFIG_DEST_*`; only the label is constant. This drops `MEDIA_BACKUP_REMOTE` and keeps `MEDIA_BACKUP_PATH`.

---

## File Structure

- Create: `deploy/backup/media-backup-lib.sh` — pure functions, no rclone, no `date` inside.
- Create: `deploy/backup/doc.go` — `package backup` doc so the dir is a Go package.
- Create: `deploy/backup/backup_test.go` — Go test that execs the lib via `sh`.
- Modify: `deploy/backup/media-backup.sh` — runner; sources the lib.
- Modify: `docker-compose.yaml:154-170` (media-backup service) and `:172-179` (volumes) — new env, mount the lib, drop the `media-backups` volume.
- Modify: `.env.example` — document the new vars with s3 default + commented sftp.

Reference — current runner (`deploy/backup/media-backup.sh`) syncs to `/backups/media` and will be rewritten. Current service mounts `media-backups:/backups` (compose line 170) and declares `media-backups:` (line 179); both are removed.

---

### Task 1: Pure backup library + Go unit tests

Create the pure shell functions and the Go test that exercises them. This is the TDD core.

**Files:**
- Create: `deploy/backup/media-backup-lib.sh`
- Create: `deploy/backup/doc.go`
- Create: `deploy/backup/backup_test.go`

**Interfaces:**
- Produces (shell functions, sourced by the runner in Task 2):
  - `archive_path NOW_EPOCH ISO` → prints `archive/<NOW_EPOCH>-<ISO>`
  - `prune_plan KEEP_SECONDS NOW_EPOCH NAME...` → prints each NAME whose leading epoch is strictly less than `NOW_EPOCH - KEEP_SECONDS` (name at exactly the cutoff is kept)
  - `require_env NAME...` → returns 0 if all set and non-empty; else prints `media-backup: missing required env <NAME>` to stderr and returns 1

- [ ] **Step 1: Write the failing Go test**

Create `deploy/backup/backup_test.go`:

```go
package backup

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func runLib(t *testing.T, env []string, snippet string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command("sh", "-c", ". ./media-backup-lib.sh; "+snippet)
	if env != nil {
		cmd.Env = env
	}
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run: %v", err)
		}
		code = ee.ExitCode()
	}
	return out.String(), errb.String(), code
}

func TestArchivePath(t *testing.T) {
	out, _, code := runLib(t, nil, "archive_path 1000 2026-08-06T13-41-00Z")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got, want := strings.TrimSpace(out), "archive/1000-2026-08-06T13-41-00Z"; got != want {
		t.Fatalf("archive_path = %q, want %q", got, want)
	}
}

func TestPrunePlanSelectsOnlyOlderThanCutoff(t *testing.T) {
	// keep=100, now=1000 -> cutoff=900. Only epochs < 900 are pruned; 900 (==cutoff) is kept.
	out, _, code := runLib(t, nil, "prune_plan 100 1000 899-a 900-b 901-c")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got, want := strings.TrimSpace(out), "899-a"; got != want {
		t.Fatalf("prune_plan = %q, want %q", got, want)
	}
}

func TestRequireEnvPassesWhenSet(t *testing.T) {
	_, _, code := runLib(t, append(os.Environ(), "FOO=bar"), "require_env FOO")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
}

func TestRequireEnvFailsAndNamesMissing(t *testing.T) {
	_, errOut, code := runLib(t, []string{"PATH=" + os.Getenv("PATH")}, "require_env FOO")
	if code == 0 {
		t.Fatal("exit = 0, want non-zero for unset FOO")
	}
	if !strings.Contains(errOut, "FOO") {
		t.Fatalf("stderr = %q, want it to name FOO", errOut)
	}
}
```

Also create `deploy/backup/doc.go`:

```go
// Package backup holds tests for the media-backup shell script's pure logic.
package backup
```

- [ ] **Step 2: Run the test to verify it FAILS**

Run: `rtk go test ./deploy/backup/`
Expected: FAIL — `media-backup-lib.sh` does not exist, so `sh` sourcing fails and functions are undefined (non-zero exit / empty output).

- [ ] **Step 3: Write the minimal library**

Create `deploy/backup/media-backup-lib.sh`:

```sh
# Pure helpers for media-backup.sh. No rclone, no date calls: time is injected.

# archive_path NOW_EPOCH ISO -> prints "archive/<NOW_EPOCH>-<ISO>".
archive_path() {
	printf 'archive/%s-%s\n' "$1" "$2"
}

# prune_plan KEEP_SECONDS NOW_EPOCH NAME... -> prints each NAME whose leading
# epoch (text before the first '-') is strictly older than NOW_EPOCH-KEEP_SECONDS.
prune_plan() {
	keep_seconds=$1
	now_epoch=$2
	shift 2
	cutoff=$((now_epoch - keep_seconds))
	for name in "$@"; do
		epoch=${name%%-*}
		case $epoch in
			'' | *[!0-9]*) continue ;;
		esac
		if [ "$epoch" -lt "$cutoff" ]; then
			printf '%s\n' "$name"
		fi
	done
	return 0
}

# require_env NAME... -> returns 1 and names the first unset/empty variable.
require_env() {
	for name in "$@"; do
		eval "val=\${$name:-}"
		if [ -z "$val" ]; then
			printf 'media-backup: missing required env %s\n' "$name" >&2
			return 1
		fi
	done
	return 0
}
```

- [ ] **Step 4: Run the test to verify it PASSES**

Run: `rtk go test ./deploy/backup/`
Expected: PASS — all four tests.

- [ ] **Step 5: Confirm the whole suite and build stay green**

Run: `rtk go build ./... && rtk go vet ./...`
Expected: build succeeds (dir with a package clause is fine); vet clean.

- [ ] **Step 6: Commit**

```bash
rtk git add deploy/backup/media-backup-lib.sh deploy/backup/doc.go deploy/backup/backup_test.go
rtk git commit -m "test(backup): pure media-backup lib (archive_path, prune_plan, require_env)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: Off-host runner + compose/env wiring

Rewrite the runner to source the lib and push to the `dest` remote with dated archiving and pruning; wire the compose service and `.env.example`. rclone I/O is trusted (per the design); this task's gates are shell syntax, `docker compose config`, and the still-green Go suite.

**Files:**
- Modify: `deploy/backup/media-backup.sh`
- Modify: `docker-compose.yaml` (media-backup service env/volumes + volumes list)
- Modify: `.env.example`

**Interfaces:**
- Consumes: `archive_path`, `prune_plan`, `require_env` from `media-backup-lib.sh` (Task 1).
- Produces: a runner that, per iteration, syncs `garage:$APP_S3_BUCKET → dest:$MEDIA_BACKUP_PATH/current` with `--backup-dir dest:$MEDIA_BACKUP_PATH/archive/<stamp>` and prunes old archives. `MEDIA_BACKUP_ONCE=1` runs one iteration and exits.

- [ ] **Step 1: Rewrite the runner**

Replace `deploy/backup/media-backup.sh` with:

```sh
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
```

- [ ] **Step 2: Verify runner syntax**

Run: `sh -n deploy/backup/media-backup.sh && echo OK`
Expected: `OK` (no syntax errors). If `shellcheck` is installed, also run `shellcheck deploy/backup/media-backup.sh deploy/backup/media-backup-lib.sh` and confirm no errors (the SC2086 splitting in `prune_archives` is intentional and suppressed inline).

- [ ] **Step 3: Wire the compose service**

In `docker-compose.yaml`, replace the `media-backup` service `environment`, `entrypoint`, and `volumes` (lines ~163-170) with:

```yaml
    environment:
      APP_S3_ENDPOINT: http://garage:3900
      APP_S3_BUCKET: ${APP_S3_BUCKET:-media}
      APP_S3_ACCESS_KEY: ${APP_S3_ACCESS_KEY:?set APP_S3_ACCESS_KEY}
      APP_S3_SECRET_KEY: ${APP_S3_SECRET_KEY:?set APP_S3_SECRET_KEY}
      MEDIA_BACKUP_PATH: ${MEDIA_BACKUP_PATH:-media-backup}
      MEDIA_BACKUP_KEEP_DAYS: ${MEDIA_BACKUP_KEEP_DAYS:-30}
      MEDIA_BACKUP_INTERVAL: ${MEDIA_BACKUP_INTERVAL:-86400}
      RCLONE_CONFIG_DEST_TYPE: ${RCLONE_CONFIG_DEST_TYPE:-}
      RCLONE_CONFIG_DEST_PROVIDER: ${RCLONE_CONFIG_DEST_PROVIDER:-}
      RCLONE_CONFIG_DEST_ENDPOINT: ${RCLONE_CONFIG_DEST_ENDPOINT:-}
      RCLONE_CONFIG_DEST_ACCESS_KEY_ID: ${RCLONE_CONFIG_DEST_ACCESS_KEY_ID:-}
      RCLONE_CONFIG_DEST_SECRET_ACCESS_KEY: ${RCLONE_CONFIG_DEST_SECRET_ACCESS_KEY:-}
      RCLONE_CONFIG_DEST_HOST: ${RCLONE_CONFIG_DEST_HOST:-}
      RCLONE_CONFIG_DEST_USER: ${RCLONE_CONFIG_DEST_USER:-}
      RCLONE_CONFIG_DEST_KEY_FILE: ${RCLONE_CONFIG_DEST_KEY_FILE:-}
    entrypoint: ["/bin/sh", "/media-backup.sh"]
    volumes:
      - ./deploy/backup/media-backup.sh:/media-backup.sh:ro
      - ./deploy/backup/media-backup-lib.sh:/media-backup-lib.sh:ro
```

Then remove the `media-backups:` line from the `volumes:` block at the bottom of the file (line ~179). Leave `pg-data`, `pg-backups`, `caddy-data`, `caddy-config`, `garage-meta`, `garage-data` untouched.

- [ ] **Step 4: Validate the compose config**

Run:
```bash
APP_DOMAIN=example.org APP_ADMIN_PASSWORD=ci POSTGRES_PASSWORD=ci GARAGE_RPC_SECRET=ci \
  APP_S3_ACCESS_KEY=ci APP_S3_SECRET_KEY=ci docker compose -f docker-compose.yaml config >/dev/null && echo OK
```
Expected: `OK`. The `RCLONE_CONFIG_DEST_*` empty defaults mean CI's existing `stack-validate` env block needs no new vars — confirm by also running the dev file: `... docker compose -f docker-compose.dev.yaml config >/dev/null && echo DEVOK`.

- [ ] **Step 5: Document the new vars in `.env.example`**

Add a section to `.env.example` (place it after the existing `APP_S3_*` block):

```sh
# --- Off-host media backup (prod only) ---
# The media-backup sidecar mirrors the Garage bucket to an off-host rclone
# remote named "dest", keeping dated archives of changed/deleted objects.
MEDIA_BACKUP_PATH=media-backup
MEDIA_BACKUP_KEEP_DAYS=30
MEDIA_BACKUP_INTERVAL=86400
# Destination backend (S3 example). The remote name is always "dest".
RCLONE_CONFIG_DEST_TYPE=s3
RCLONE_CONFIG_DEST_PROVIDER=Other
RCLONE_CONFIG_DEST_ENDPOINT=https://s3.example.com
RCLONE_CONFIG_DEST_ACCESS_KEY_ID=change-me
RCLONE_CONFIG_DEST_SECRET_ACCESS_KEY=change-me
# SFTP alternative (comment out the S3 block above and mount an SSH key):
# RCLONE_CONFIG_DEST_TYPE=sftp
# RCLONE_CONFIG_DEST_HOST=backup.example.com
# RCLONE_CONFIG_DEST_USER=backup
# RCLONE_CONFIG_DEST_KEY_FILE=/root/.ssh/id_backup
```

- [ ] **Step 6: Confirm the Go suite still passes**

Run: `rtk go test ./...`
Expected: PASS (255+ tests; the new `deploy/backup` package included).

- [ ] **Step 7: Commit**

```bash
rtk git add deploy/backup/media-backup.sh docker-compose.yaml .env.example
rtk git commit -m "feat(backup): off-host versioned media backup via env-driven rclone remote

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Post-plan (orchestrator-owned, not a task)

After both tasks pass verification: write ADR-0009 (off-host versioned backup, extends ADR-0004's single-node caveat) with the restore runbook (`rclone copy dest:$PATH/current garage:$BUCKET`, or `.../archive/<stamp>` for point-in-time), update `CONTEXT.md`, commit the docs, then run the full suite and integrate the branch.

---

## Self-Review

**Spec coverage:**
- Off-host env-driven remote (backend-agnostic) → Task 2 runner + compose `RCLONE_CONFIG_DEST_*`. ✓ (remote label fixed to `dest`, flagged above.)
- Versioned via `--backup-dir archive/<stamp>` → Task 2 `run_once`. ✓
- Retention prune per `MEDIA_BACKUP_KEEP_DAYS` (default 30) → Task 1 `prune_plan` + Task 2 `prune_archives`. ✓
- Replaces local backup (drop `media-backups` volume) → Task 2 Step 3. ✓
- Pure-logic TDD, no rclone in CI → Task 1 Go test execs `sh`. ✓
- `MEDIA_BACKUP_ONCE` one-shot mode → Task 2 runner. ✓
- Dev stack unchanged → no edit to `docker-compose.dev.yaml`; not in any task. ✓
- CI: no new vars needed (empty defaults) → Task 2 Step 4 verifies. ✓
- `.env.example` s3 default + commented sftp → Task 2 Step 5. ✓
- Restore doc → post-plan ADR-0009. ✓

**Placeholder scan:** No TBD/TODO; all shell and Go steps carry full code. ✓

**Type/name consistency:** `archive_path`, `prune_plan`, `require_env` defined in Task 1 with the exact arg orders used by Task 2's runner and the Go tests. Archive dir name shape `<epoch>-<iso>` is consistent between `archive_path` output, `prune_plan`'s `%%-*` parse, and the Go test assertions. Remote label `dest` and `$MEDIA_BACKUP_PATH` used identically in runner, compose, and `.env.example`. ✓
