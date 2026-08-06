# CD F2 — Push-based Deploy Automation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A manually-dispatched GitHub Actions workflow that SSHes to the VPS and runs a health-gated, auto-rollback deploy of a chosen released version of the compose stack.

**Architecture:** Pure POSIX-sh helpers (`deploy-lib.sh`, unit-tested) back an orchestration script (`deploy.sh`) that checks out the release tag, sets `APP_IMAGE_TAG`, pulls the ghcr images, brings the stack up, health-gates on `/api/health`, and auto-rolls-back on failure. A thin `deploy.yml` (`workflow_dispatch`, `version` input) SSHes in and runs the script.

**Tech Stack:** POSIX shell, Docker Compose, git, GitHub Actions, Go (test harness only).

## Global Constraints

- 15-Factor App compliance; Hexagonal architecture — infra only, no application code change.
- ASD-STE100 Simplified Technical English in prose/docs.
- POSIX sh (host + ubuntu runner target; no bashisms). `deploy-lib.sh` must have no docker/git/ssh side effects (pure).
- TDD mandatory for the pure logic: failing Go test → confirm fail → minimal shell → confirm pass. docker/git/ssh are trusted, not exercised.
- Deploy is push-based, `workflow_dispatch` only, `version` input; rollback = dispatch an older version.
- Health gate: poll `https://$APP_DOMAIN/api/health` for `"status":"ok"`. Auto-rollback to the previous `APP_IMAGE_TAG` on failure, then fail the job.
- Secrets (owner-configured): `DEPLOY_SSH_HOST`, `DEPLOY_SSH_USER`, `DEPLOY_SSH_KEY`, `DEPLOY_PATH`.
- Commit prefix commands with `rtk`.
- Spec: `docs/superpowers/specs/2026-08-06-cd-f2-deploy-automation-design.md`.

**Environment note:** Task 1 runs `go test` (needs only `sh`, present). Task 2 is shell + workflow YAML; gates are `sh -n` and `actionlint` if available (the real end-to-end proof is the owner running the dispatch against the live VPS).

---

## File Structure

- Create: `deploy/deploy-lib.sh` — pure helpers (`validate_version`, `read_tag`, `write_tag`).
- Create: `deploy/doc.go` — `package deploy` doc so the dir is a Go package.
- Create: `deploy/deploy_test.go` — Go test execing the helpers via `sh`.
- Create: `deploy/deploy.sh` — orchestration (sources the lib); host-side deploy + rollback.
- Create: `.github/workflows/deploy.yml` — `workflow_dispatch` → SSH → run `deploy/deploy.sh`.

---

### Task 1: Pure deploy library + Go unit tests

Create the pure helpers and the Go test that exercises them. TDD core.

**Files:**
- Create: `deploy/deploy-lib.sh`
- Create: `deploy/doc.go`
- Create: `deploy/deploy_test.go`

**Interfaces:**
- Produces (shell functions, sourced by `deploy.sh` in Task 2):
  - `validate_version V` → return 0 iff `V` matches `^[0-9]+\.[0-9]+\.[0-9]+$`; else print `deploy: invalid version '<V>' (want N.N.N)` to stderr and return 1.
  - `read_tag ENVFILE` → print the value of the last `APP_IMAGE_TAG=` line in `ENVFILE`, or nothing if absent.
  - `write_tag ENVFILE V` → replace the `APP_IMAGE_TAG=` line in `ENVFILE` with `APP_IMAGE_TAG=V` (other lines untouched), or append it if absent.

- [ ] **Step 1: Write the failing Go test**

Create `deploy/deploy_test.go`:

```go
package deploy

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runLib(t *testing.T, snippet string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command("sh", "-c", ". ./deploy-lib.sh; "+snippet)
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

func TestValidateVersionAccepts(t *testing.T) {
	_, _, code := runLib(t, "validate_version 1.5.0")
	if code != 0 {
		t.Fatalf("validate_version 1.5.0 exit = %d, want 0", code)
	}
}

func TestValidateVersionRejects(t *testing.T) {
	for _, bad := range []string{"v1.5.0", "1.5", "latest", "1.5.0-rc1", "1.2.3.4"} {
		_, errOut, code := runLib(t, "validate_version "+bad)
		if code == 0 {
			t.Fatalf("validate_version %q exit = 0, want non-zero", bad)
		}
		if !strings.Contains(errOut, bad) {
			t.Fatalf("validate_version %q stderr = %q, want it to name the bad value", bad, errOut)
		}
	}
}

func TestReadTag(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	if err := os.WriteFile(env, []byte("APP_DOMAIN=x.org\nAPP_IMAGE_TAG=1.4.0\nFOO=bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runLib(t, "read_tag "+env)
	if code != 0 {
		t.Fatalf("read_tag exit = %d", code)
	}
	if got := strings.TrimSpace(out); got != "1.4.0" {
		t.Fatalf("read_tag = %q, want 1.4.0", got)
	}
}

func TestReadTagAbsentIsEmpty(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	if err := os.WriteFile(env, []byte("APP_DOMAIN=x.org\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, _ := runLib(t, "read_tag "+env)
	if got := strings.TrimSpace(out); got != "" {
		t.Fatalf("read_tag absent = %q, want empty", got)
	}
}

func TestWriteTagReplaceInPlace(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	if err := os.WriteFile(env, []byte("APP_DOMAIN=x.org\nAPP_IMAGE_TAG=1.4.0\nFOO=bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, code := runLib(t, "write_tag "+env+" 1.5.0"); code != 0 {
		t.Fatalf("write_tag exit = %d", code)
	}
	b, _ := os.ReadFile(env)
	got := string(b)
	if !strings.Contains(got, "APP_IMAGE_TAG=1.5.0") {
		t.Fatalf("after write_tag, file = %q, want APP_IMAGE_TAG=1.5.0", got)
	}
	if strings.Contains(got, "1.4.0") {
		t.Fatalf("old tag still present: %q", got)
	}
	if !strings.Contains(got, "APP_DOMAIN=x.org") || !strings.Contains(got, "FOO=bar") {
		t.Fatalf("write_tag disturbed other lines: %q", got)
	}
}

func TestWriteTagAppendsWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	if err := os.WriteFile(env, []byte("APP_DOMAIN=x.org\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, code := runLib(t, "write_tag "+env+" 1.5.0"); code != 0 {
		t.Fatalf("write_tag exit = %d", code)
	}
	out, _, _ := runLib(t, "read_tag "+env)
	if got := strings.TrimSpace(out); got != "1.5.0" {
		t.Fatalf("round-trip read_tag = %q, want 1.5.0", got)
	}
}
```

Also create `deploy/doc.go`:

```go
// Package deploy holds tests for the compose-stack deploy script's pure logic.
package deploy
```

- [ ] **Step 2: Run the test to verify it FAILS**

Run: `rtk go test ./deploy/`
Expected: FAIL — `deploy-lib.sh` does not exist, so sourcing fails and the functions are undefined.

- [ ] **Step 3: Write the pure library**

Create `deploy/deploy-lib.sh`:

```sh
# Pure helpers for deploy.sh. No docker/git/ssh side effects.

# validate_version V -> 0 iff V is N.N.N; else print an error and return 1.
validate_version() {
	if printf '%s' "$1" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
		return 0
	fi
	echo "deploy: invalid version '$1' (want N.N.N)" >&2
	return 1
}

# read_tag ENVFILE -> print the value of the last APP_IMAGE_TAG= line, or nothing.
read_tag() {
	sed -n 's/^APP_IMAGE_TAG=//p' "$1" 2>/dev/null | tail -n1
}

# write_tag ENVFILE V -> set APP_IMAGE_TAG=V in ENVFILE (replace or append),
# leaving all other lines unchanged.
write_tag() {
	envfile=$1
	value=$2
	if grep -q '^APP_IMAGE_TAG=' "$envfile" 2>/dev/null; then
		sed "s|^APP_IMAGE_TAG=.*|APP_IMAGE_TAG=$value|" "$envfile" > "$envfile.tmp" &&
			mv "$envfile.tmp" "$envfile"
	else
		printf 'APP_IMAGE_TAG=%s\n' "$value" >> "$envfile"
	fi
}
```

- [ ] **Step 4: Run the test to verify it PASSES**

Run: `rtk go test ./deploy/`
Expected: PASS — all six tests.

- [ ] **Step 5: Confirm the whole suite and build stay green**

Run: `rtk go build ./... && rtk go vet ./...`
Expected: build succeeds; vet clean.

- [ ] **Step 6: Commit**

```bash
rtk git add deploy/deploy-lib.sh deploy/doc.go deploy/deploy_test.go
rtk git commit -m "test(deploy): pure deploy lib (validate_version, read_tag, write_tag)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: Deploy orchestration script + dispatch workflow

Create the host-side `deploy.sh` (health-gated, auto-rollback) and the `deploy.yml`
workflow that SSHes in and runs it. rclone/docker/git/ssh are trusted; gates are `sh -n`
and `actionlint`.

**Files:**
- Create: `deploy/deploy.sh`
- Create: `.github/workflows/deploy.yml`

**Interfaces:**
- Consumes: `validate_version`, `read_tag`, `write_tag` from `deploy/deploy-lib.sh` (Task 1).
- Produces: `deploy/deploy.sh <version>` (host-side deploy + rollback); `deploy.yml` (`workflow_dispatch`, `version` input) that runs it over SSH.

- [ ] **Step 1: Write the orchestration script**

Create `deploy/deploy.sh`:

```sh
#!/bin/sh
# Deploy (or roll back to) a released version of the compose stack. Run on the
# host, from the stack checkout dir. Health-gated with automatic rollback.
set -eu

VERSION=${1:-}
DIR=$(dirname "$0")
. "$DIR/deploy-lib.sh"

ENV_FILE=.env

validate_version "$VERSION" || exit 1

prev=$(read_tag "$ENV_FILE")
[ -n "$prev" ] || prev=latest
echo "deploy: current=$prev target=$VERSION"

# deploy_version V: check out V's compose topology (when V is a real version),
# point APP_IMAGE_TAG at V, pull the images, and bring the stack up.
deploy_version() {
	v=$1
	if validate_version "$v" 2>/dev/null; then
		git fetch --tags --quiet || return 1
		git checkout --quiet "v$v" || return 1
	fi
	write_tag "$ENV_FILE" "$v" || return 1
	docker compose pull || return 1
	docker compose up -d --wait || return 1
	return 0
}

# health_ok: poll the public health endpoint until it reports ok, or time out.
health_ok() {
	domain=$(sed -n 's/^APP_DOMAIN=//p' "$ENV_FILE" | tail -n1)
	i=0
	while [ "$i" -lt 30 ]; do
		if curl -fsS "https://$domain/api/health" 2>/dev/null | grep -q '"status":"ok"'; then
			return 0
		fi
		i=$((i + 1))
		sleep 5
	done
	return 1
}

if deploy_version "$VERSION" && health_ok; then
	echo "deploy: $VERSION healthy"
	exit 0
fi

echo "deploy: $VERSION FAILED — rolling back to $prev" >&2
deploy_version "$prev" || true
if health_ok; then
	echo "deploy: rolled back to $prev (healthy)" >&2
else
	echo "deploy: rollback to $prev also unhealthy — manual intervention needed" >&2
fi
exit 1
```

Make it executable: `chmod +x deploy/deploy.sh`.

- [ ] **Step 2: Verify the script syntax**

Run: `sh -n deploy/deploy.sh && echo OK`
Expected: `OK`. If `shellcheck` is installed, run `shellcheck deploy/deploy.sh deploy/deploy-lib.sh` and confirm no errors; if not installed, note it.

- [ ] **Step 3: Write the dispatch workflow**

Create `.github/workflows/deploy.yml`:

```yaml
name: deploy

on:
  workflow_dispatch:
    inputs:
      version:
        description: Released version to deploy (e.g. 1.5.0)
        required: true

jobs:
  deploy:
    name: Deploy to VPS
    runs-on: ubuntu-latest
    steps:
      - name: Deploy over SSH
        env:
          DEPLOY_SSH_KEY: ${{ secrets.DEPLOY_SSH_KEY }}
          DEPLOY_SSH_HOST: ${{ secrets.DEPLOY_SSH_HOST }}
          DEPLOY_SSH_USER: ${{ secrets.DEPLOY_SSH_USER }}
          DEPLOY_PATH: ${{ secrets.DEPLOY_PATH }}
          VERSION: ${{ inputs.version }}
        run: |
          mkdir -p "$HOME/.ssh"
          printf '%s\n' "$DEPLOY_SSH_KEY" > key
          chmod 600 key
          ssh -i key -o StrictHostKeyChecking=accept-new \
            "$DEPLOY_SSH_USER@$DEPLOY_SSH_HOST" \
            "cd '$DEPLOY_PATH' && ./deploy/deploy.sh '$VERSION'"
```

- [ ] **Step 4: Validate the workflow YAML**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/deploy.yml')); print('yaml ok')"`
Expected: `yaml ok`. If `actionlint` is installed, run it on the file and confirm no errors; if not, note that CI/dispatch is the real proof.

- [ ] **Step 5: Commit**

```bash
rtk git add deploy/deploy.sh .github/workflows/deploy.yml
rtk git commit -m "deploy(ci): workflow_dispatch SSH deploy with health-gated auto-rollback

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Post-plan (orchestrator-owned, not a task)

After both tasks pass verification:
- Rewrite `docs/ops/deploy-compose.md` to the current reality: image-based `api`/`web`
  services, the `migrate` job + `APP_AUTO_MIGRATE=false`, the 2 api replicas behind
  Caddy, `APP_IMAGE_TAG`, and the new deploy/rollback flow (dispatch `deploy.yml` with a
  `version`; rollback = dispatch an older `version`). Add the required GitHub secrets and
  the one-time host setup (clone at `DEPLOY_PATH`, populate `.env`, `docker login
  ghcr.io` if the packages are private).
- Write ADR-0011 (push-based CD: manual dispatch + health-gated auto-rollback).
- Update `CONTEXT.md`; commit docs; run `rtk go test ./...`; integrate the branch.
- Note in the handoff that end-to-end validation needs the owner's VPS + secrets (the
  live dispatch is the owner's step). Close issue #19 on merge.

---

## Self-Review

**Spec coverage:**
- `workflow_dispatch` + `version` input; rollback = older version → Task 2 `deploy.yml`. ✓
- SSH in, run host script → Task 2 workflow SSH step. ✓
- Capture prev → checkout tag → set APP_IMAGE_TAG → pull → up --wait → health gate → auto-rollback → fail job → Task 2 `deploy.sh`. ✓
- `prev=latest` edge (first deploy) handled: `validate_version` guards the `git checkout` so no `vlatest` checkout is attempted → Task 2 `deploy_version`. ✓
- Pure helpers `validate_version`/`read_tag`/`write_tag`, unit-tested → Task 1. ✓
- Health via `https://$APP_DOMAIN/api/health` for `"status":"ok"` → Task 2 `health_ok`. ✓
- Secrets list → Task 2 workflow env; documented in post-plan doc rewrite. ✓
- Doc rewrite + ADR-0011 + CONTEXT → post-plan (orchestrator). ✓

**Placeholder scan:** No TBD/TODO; all shell/YAML/Go steps carry full content. ✓

**Type/name consistency:** `validate_version`/`read_tag`/`write_tag` defined in Task 1 with the exact arg orders used by Task 2's `deploy.sh` and the Task 1 tests. `APP_IMAGE_TAG` / `APP_DOMAIN` env keys identical in `deploy-lib.sh`, `deploy.sh`, and the tests. `deploy.yml` runs `./deploy/deploy.sh '$VERSION'`, matching the script's `$1` version arg. ✓
