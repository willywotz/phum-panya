# CD F1 — Containerize & Publish Release Images Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the compose stack's `api` and `web` images in CI, smoke-test them on every push/PR, and publish them to ghcr.io on a `v*` tag; switch the prod compose stack from building on the host to running those published images.

**Architecture:** One reusable workflow (`build-images.yml`) runs a single `deploy/images/build-and-smoke.sh` script (build + smoke both images); `ci.yml` calls it with `push:false`, `release.yml` calls it with `push:true` (adds ghcr login + tag + push). Compose `api`/`migrate`/`web` switch `build:` → `image:` with an `APP_IMAGE_TAG` selector and a `docker-compose.build.yaml` override for local builds.

**Tech Stack:** Docker/buildx, GitHub Actions (reusable workflow), POSIX shell, Docker Compose.

## Global Constraints

- 15-Factor App compliance — build/release/run separation is the whole point: build the image once, run it immutably.
- Hexagonal architecture — no application code change (infra only).
- ASD-STE100 Simplified Technical English in prose/docs.
- POSIX sh for the smoke script (ubuntu runner `/bin/sh` is dash; no bashisms).
- Registry: `ghcr.io/willywotz/phum-panya-api` and `ghcr.io/willywotz/phum-panya-web`; tags `<version>` (from `github.ref_name`, leading `v` stripped) and `latest`; linux/amd64 only.
- Commit prefix commands with `rtk`.
- Spec: `docs/superpowers/specs/2026-08-06-cd-f1-publish-images-design.md`.

**Environment note:** Tasks 1 and 3 need a running Docker daemon (present here — Docker 29.6.x). Task 2 is workflow YAML; its real proof runs in CI on the PR, locally gated by YAML-parse + `actionlint` if available.

---

## File Structure

- Create: `deploy/images/build-and-smoke.sh` — build + smoke both images (no push). Locally runnable, called by CI.
- Modify: `Dockerfile.api` — base `golang:1.26-alpine` → `golang:1.26.5-alpine`.
- Create: `.github/workflows/build-images.yml` — reusable (`workflow_call`, input `push`).
- Modify: `.github/workflows/ci.yml` — add `images` job calling the reusable workflow (`push:false`).
- Modify: `.github/workflows/release.yml` — add `images` job calling it (`push:true`).
- Modify: `docker-compose.yaml` — `api`/`migrate`/`web` `build:` → `image:`.
- Create: `docker-compose.build.yaml` — build override for local prod-image builds.
- Modify: `.env.example` — document `APP_IMAGE_TAG`.

Reference — exact current compose build blocks: `api` lines 22-24, `migrate` 57-59, `web` 74-76 (`build:` / `context:` / `dockerfile:`).

---

### Task 1: Build-and-smoke script + Dockerfile.api toolchain bump

Create the locally-runnable build+smoke script and bump the Go base image. Verify by
running the script against a live Docker daemon.

**Files:**
- Create: `deploy/images/build-and-smoke.sh`
- Modify: `Dockerfile.api`

**Interfaces:**
- Produces: `deploy/images/build-and-smoke.sh` — builds `phum-panya-api:ci` (from `Dockerfile.api`, context `.`) and `phum-panya-web:ci` (from `web/Dockerfile`, context `web`), smoke-tests both, exits non-zero on any failure. Honors `API_IMAGE`/`WEB_IMAGE` env overrides (default the `:ci` tags). The reusable workflow (Task 2) invokes this script.

- [ ] **Step 1: Write the build-and-smoke script**

Create `deploy/images/build-and-smoke.sh`:

```sh
#!/bin/sh
# Build the compose stack's api + web images and smoke-test them. No push.
set -eu

API_IMAGE="${API_IMAGE:-phum-panya-api:ci}"
WEB_IMAGE="${WEB_IMAGE:-phum-panya-web:ci}"

echo "build: $API_IMAGE"
docker build -f Dockerfile.api -t "$API_IMAGE" .
echo "build: $WEB_IMAGE"
docker build -f web/Dockerfile -t "$WEB_IMAGE" web

cleanup() { docker rm -f api-smoke web-smoke >/dev/null 2>&1 || true; }
trap cleanup EXIT

# api: boot in dev mode (SQLite, HTTP :8080, AutoMigrate), wait for the image's
# built-in HEALTHCHECK to report healthy.
echo "smoke: api"
docker run -d --name api-smoke -e APP_DEV=1 "$API_IMAGE" >/dev/null
status=starting
i=0
while [ "$i" -lt 30 ]; do
	status=$(docker inspect -f '{{.State.Health.Status}}' api-smoke 2>/dev/null || echo starting)
	[ "$status" = healthy ] && break
	if [ "$status" = unhealthy ]; then
		echo "api unhealthy"; docker logs api-smoke; exit 1
	fi
	i=$((i + 1))
	sleep 2
done
[ "$status" = healthy ] || { echo "api never healthy"; docker logs api-smoke; exit 1; }
echo "api healthy"

# web: standalone Next.js server on :3000; wait for an HTTP response.
echo "smoke: web"
docker run -d --name web-smoke -p 3000:3000 "$WEB_IMAGE" >/dev/null
ok=
i=0
while [ "$i" -lt 30 ]; do
	if curl -fsSL -o /dev/null http://localhost:3000/; then ok=1; break; fi
	i=$((i + 1))
	sleep 2
done
[ "$ok" = 1 ] || { echo "web never responded"; docker logs web-smoke; exit 1; }
echo "web responding"

echo "smoke OK"
```

Make it executable: `chmod +x deploy/images/build-and-smoke.sh`.

- [ ] **Step 2: Bump the Dockerfile.api Go base**

In `Dockerfile.api`, change the build stage base:

```
FROM golang:1.26.5-alpine AS build
```

(from `golang:1.26-alpine`) — matches the `toolchain go1.26.5` pinned in `go.mod`.

- [ ] **Step 3: Run the script to verify both images build and smoke-pass**

Run: `sh -n deploy/images/build-and-smoke.sh && deploy/images/build-and-smoke.sh`
Expected: `sh -n` clean; script prints `build: ...` for both, `api healthy`, `web responding`, `smoke OK`, and exits 0. The script exits non-zero (with container logs) if either image fails to build or never becomes healthy/responsive — the genuine assertion.

If `golang:1.26.5-alpine` is not a published tag, fall back to `golang:1.26-alpine` (which already tracks the latest 1.26 patch) and note it in the report.

- [ ] **Step 4: Commit**

```bash
rtk git add deploy/images/build-and-smoke.sh Dockerfile.api
rtk git commit -m "build(images): build+smoke script for api/web images; pin go1.26.5 base

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: Reusable build-images workflow + CI/release callers

Wire the script into a reusable workflow and call it from `ci.yml` (build+smoke) and
`release.yml` (build+smoke+push to ghcr).

**Files:**
- Create: `.github/workflows/build-images.yml`
- Modify: `.github/workflows/ci.yml`
- Modify: `.github/workflows/release.yml`

**Interfaces:**
- Consumes: `deploy/images/build-and-smoke.sh` (Task 1), which produces local images `phum-panya-api:ci` / `phum-panya-web:ci`.
- Produces: reusable workflow `.github/workflows/build-images.yml` (`workflow_call`, input `push: boolean`). On `push:true` it publishes `ghcr.io/willywotz/phum-panya-{api,web}:<version>` and `:latest`.

- [ ] **Step 1: Create the reusable workflow**

Create `.github/workflows/build-images.yml`:

```yaml
name: build-images

on:
  workflow_call:
    inputs:
      push:
        description: Push images to ghcr.io
        type: boolean
        default: false

jobs:
  images:
    name: Build + smoke images
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
    steps:
      - uses: actions/checkout@v7
      - name: Build and smoke-test images
        run: deploy/images/build-and-smoke.sh
      - name: Push to ghcr.io
        if: ${{ inputs.push }}
        env:
          GHCR_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          REF_NAME: ${{ github.ref_name }}
          OWNER: ${{ github.repository_owner }}
          ACTOR: ${{ github.actor }}
        run: |
          version="${REF_NAME#v}"
          echo "$GHCR_TOKEN" | docker login ghcr.io -u "$ACTOR" --password-stdin
          for name in api web; do
            src="phum-panya-$name:ci"
            repo="ghcr.io/$OWNER/phum-panya-$name"
            docker tag "$src" "$repo:$version"
            docker tag "$src" "$repo:latest"
            docker push "$repo:$version"
            docker push "$repo:latest"
          done
```

- [ ] **Step 2: Add the `images` caller to `ci.yml`**

In `.github/workflows/ci.yml`, add a job (top-level, alongside `go`, `web`, etc.):

```yaml
  images:
    name: Build + smoke images
    uses: ./.github/workflows/build-images.yml
    with:
      push: false
    permissions:
      contents: read
```

- [ ] **Step 3: Add the `images` caller to `release.yml`**

In `.github/workflows/release.yml`, add a job:

```yaml
  images:
    name: Publish images to ghcr.io
    uses: ./.github/workflows/build-images.yml
    with:
      push: true
    permissions:
      contents: read
      packages: write
```

- [ ] **Step 4: Validate the workflow YAML**

Run:
```bash
python3 -c "import yaml; [yaml.safe_load(open(f)) for f in ['.github/workflows/build-images.yml','.github/workflows/ci.yml','.github/workflows/release.yml']]; print('yaml ok')"
```
Expected: `yaml ok`. If `actionlint` is installed, also run `actionlint` and confirm no errors; if not installed, note it (CI on the PR is the real proof).

- [ ] **Step 5: Commit**

```bash
rtk git add .github/workflows/build-images.yml .github/workflows/ci.yml .github/workflows/release.yml
rtk git commit -m "ci(images): reusable build-images workflow; build+smoke on CI, push on release

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: Switch compose to published images + build override + env docs

Point the prod stack at the ghcr images, keep local builds possible via an override, and
document the tag selector.

**Files:**
- Modify: `docker-compose.yaml`
- Create: `docker-compose.build.yaml`
- Modify: `.env.example`

**Interfaces:**
- Consumes: images published by Task 2 (`ghcr.io/willywotz/phum-panya-{api,web}`).
- Produces: prod compose that runs `image: ...:${APP_IMAGE_TAG:-latest}`; a build override at `docker-compose.build.yaml`.

- [ ] **Step 1: Switch `api` to the published image**

In `docker-compose.yaml`, replace the `api` service build block (lines ~22-24):

```yaml
    build:
      context: .
      dockerfile: Dockerfile.api
```

with:

```yaml
    image: ghcr.io/willywotz/phum-panya-api:${APP_IMAGE_TAG:-latest}
```

- [ ] **Step 2: Switch `migrate` and `web` the same way**

`migrate` (lines ~57-59) → `image: ghcr.io/willywotz/phum-panya-api:${APP_IMAGE_TAG:-latest}`.
`web` (lines ~74-76) → `image: ghcr.io/willywotz/phum-panya-web:${APP_IMAGE_TAG:-latest}`.

Leave `garage-init` (which also builds, lines ~130-132) unchanged — it is not part of F1.

- [ ] **Step 3: Create the build override**

Create `docker-compose.build.yaml`:

```yaml
# Build the prod images locally instead of pulling them:
#   docker compose -f docker-compose.yaml -f docker-compose.build.yaml build
services:
  api:
    build:
      context: .
      dockerfile: Dockerfile.api
  migrate:
    build:
      context: .
      dockerfile: Dockerfile.api
  web:
    build:
      context: web
      dockerfile: Dockerfile
```

- [ ] **Step 4: Document `APP_IMAGE_TAG` in `.env.example`**

Add to `.env.example` (after the existing app/S3 block):

```sh
# --- Prod image tag (compose stack) ---
# The api/migrate/web services run ghcr.io/willywotz/phum-panya-{api,web} at this
# tag. Defaults to "latest"; pin it to a released version (e.g. 1.4.0) in prod.
APP_IMAGE_TAG=latest
```

- [ ] **Step 5: Validate both compose configs**

Run:
```bash
env APP_DOMAIN=example.org APP_ADMIN_PASSWORD=ci POSTGRES_PASSWORD=ci GARAGE_RPC_SECRET=ci \
  APP_S3_ACCESS_KEY=ci APP_S3_SECRET_KEY=ci sh -c '
  docker compose -f docker-compose.yaml config >/dev/null && echo PROD_OK &&
  docker compose -f docker-compose.yaml -f docker-compose.build.yaml config >/dev/null && echo BUILD_OK &&
  docker compose -f docker-compose.dev.yaml config >/dev/null && echo DEV_OK'
```
Expected: `PROD_OK`, `BUILD_OK`, `DEV_OK`. Confirms the image switch, the build override, and that dev is untouched — all with the existing CI env (no new required var).

- [ ] **Step 6: Commit**

```bash
rtk git add docker-compose.yaml docker-compose.build.yaml .env.example
rtk git commit -m "deploy(compose): run published ghcr images; add local build override

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Post-plan (orchestrator-owned, not a task)

After all tasks pass verification: write ADR-0010 (containerized release images to ghcr;
extends ADR-0001's 15-Factor build/release/run), update `CONTEXT.md`, commit docs, run
`rtk go test ./...` (unchanged app code — sanity), integrate the branch. F2 (deploy
automation, issue #19 core) is a separate later sub-project.

---

## Self-Review

**Spec coverage:**
- Two ghcr images (api serves api+migrate, web) → Task 3 image refs + Task 2 push loop. ✓
- Tags `<version>` + `latest`, `v` stripped, amd64 → Task 2 push step. ✓
- Reusable workflow, DRY across triggers → Task 2 `build-images.yml` + two callers. ✓
- Build+smoke every push/PR, push on tag → ci caller `push:false`, release caller `push:true`. ✓
- api smoke via built-in HEALTHCHECK (dev boots without admin env — verified) → Task 1 script. ✓
- web smoke via curl :3000 → Task 1 script. ✓
- compose `build:`→`image:` with `${APP_IMAGE_TAG:-latest}` default (config stays valid) → Task 3. ✓
- `docker-compose.build.yaml` local build override; dev unchanged → Task 3. ✓
- Dockerfile.api base → golang:1.26.5-alpine → Task 1. ✓
- `.env.example` documents `APP_IMAGE_TAG` → Task 3. ✓
- ghcr login via built-in GITHUB_TOKEN, packages:write → Task 2 workflow + release caller permissions. ✓

**Placeholder scan:** No TBD/TODO; all shell/YAML steps carry full content. ✓

**Type/name consistency:** Local image tags `phum-panya-api:ci` / `phum-panya-web:ci` are produced by the Task 1 script and consumed verbatim by the Task 2 push loop (`src="phum-panya-$name:ci"`). Registry repos `ghcr.io/willywotz/phum-panya-{api,web}` identical in Task 2 push and Task 3 compose. `APP_IMAGE_TAG` default `latest` identical in compose (Task 3) and `.env.example` (Task 3). ✓
