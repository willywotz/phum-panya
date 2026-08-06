# CD F1 — Containerize and publish release images

Date: 2026-08-06
Status: Approved (design)
Scope: `.github/workflows/`, `docker-compose.yaml`, `Dockerfile.api`, new `docker-compose.build.yaml`, `.env.example`, docs
Relates: issue #19 (No CD); ADR-0001 (15-Factor), ADR-0003/0007 (compose stack, multi-replica)
Sequences: this is F1 of two. F2 (deploy automation) is a separate later sub-project.

## Problem

The compose stack's `api`, `migrate`, and `web` services build from source on the
deploy host (`build: Dockerfile.api` and `build: web/Dockerfile`). `release.yml`
publishes the Linux/Windows binaries and the MSI, but **no container image**. So:

- There is no immutable, versioned artifact for the compose stack to run — the host must
  hold the source and a build toolchain and rebuild per deploy.
- Building on the host per deploy **violates 15-Factor's build/release/run separation**
  (Factor V), which the project rule mandates strictly.
- CD (issue #19) has nothing to roll forward to or roll back to.

F1 fixes the foundation: build the images once in CI, publish them to a registry, and
run those exact images. F2 (later) automates getting a published release onto the host.

## Decision

Publish two images to GitHub Container Registry on a `v*` release tag, and switch the
prod compose stack to run them:

- `ghcr.io/willywotz/phum-panya-api:<tag>` — the Go API image (`Dockerfile.api`); used by
  both the `api` and `migrate` services.
- `ghcr.io/willywotz/phum-panya-web:<tag>` — the Next.js UI image (`web/Dockerfile`).
- Tags: the release version with the leading `v` stripped (e.g. `1.4.0`) **and** `latest`.
  linux/amd64 only (matches the existing released Linux binary and the single-VPS target).

Building is DRY across triggers via one reusable workflow; images build and smoke-test on
every push/PR, and additionally push to ghcr on a `v*` tag.

### Alternatives considered

- **Build on the host per deploy** (SSH `git pull` + `docker compose build`). Rejected:
  violates 15-Factor build/release/run separation and makes every deploy a fresh,
  non-reproducible build.
- **A managed/third-party registry.** Rejected: ghcr.io needs no extra account — the
  built-in `GITHUB_TOKEN` already grants `packages: write` — and keeps release artifacts
  beside the code.
- **Only build+push on tag** (no per-PR build). Rejected: a broken Dockerfile would not
  surface until release time; per-PR build+smoke catches it early.

## Design

### Reusable build workflow

`.github/workflows/build-images.yml` — `on: workflow_call` with a boolean input
`push` (default `false`):

1. `docker build` both images (context `.` / `Dockerfile.api`; context `web` /
   `web/Dockerfile`), tagged locally (e.g. `phum-panya-api:ci`, `phum-panya-web:ci`).
2. Smoke-test both:
   - **api**: `docker run -d -e APP_DEV=1` (SQLite, HTTP on `:8080`, AutoMigrate on boot),
     then poll the image's own `HEALTHCHECK` result
     (`docker inspect --format '{{.State.Health.Status}}'` until `healthy`, with a
     timeout). This proves the binary boots, migrates, and serves `/api/health`.
   - **web**: `docker run -d -p 3000:3000`, then poll `curl -fsS http://localhost:3000/`
     until HTTP 200, with a timeout.
3. If `push` is `true`: `docker login ghcr.io` using `GITHUB_TOKEN`, tag both images with
   the version (from `github.ref_name`, `v` stripped) and `latest`, and `docker push`
   all four tags. The job declares `permissions: packages: write`.

### Callers

- `.github/workflows/ci.yml` — add an `images` job:
  `uses: ./.github/workflows/build-images.yml` with `push: false`. Runs on the existing
  push/PR triggers, so every change builds and smoke-tests both images.
- `.github/workflows/release.yml` — add an `images` job:
  `uses: ./.github/workflows/build-images.yml` with `push: true`. Runs on the `v*` tag,
  building, smoke-testing, and pushing to ghcr. Passes `permissions: packages: write`.

### Compose changes

`docker-compose.yaml`:

- `api`, `migrate`: replace `build: { context: ., dockerfile: Dockerfile.api }` with
  `image: ghcr.io/willywotz/phum-panya-api:${APP_IMAGE_TAG:-latest}`.
- `web`: replace `build: { context: web, dockerfile: Dockerfile }` with
  `image: ghcr.io/willywotz/phum-panya-web:${APP_IMAGE_TAG:-latest}`.

The `${APP_IMAGE_TAG:-latest}` default keeps `docker compose config` (the CI
`stack-validate` job) valid with no new required env. Prod and F2 pin
`APP_IMAGE_TAG` to an exact version; `.env.example` documents it.

New `docker-compose.build.yaml` override so the prod images can still be built locally:

```yaml
services:
  api:
    build: { context: ., dockerfile: Dockerfile.api }
  migrate:
    build: { context: ., dockerfile: Dockerfile.api }
  web:
    build: { context: web, dockerfile: Dockerfile }
```

Usage: `docker compose -f docker-compose.yaml -f docker-compose.build.yaml build`.

`docker-compose.dev.yaml` is unchanged (keeps `build:` + hot reload).

### Toolchain consistency

Bump `Dockerfile.api` base image `golang:1.26-alpine` → `golang:1.26.5-alpine`, matching
the `toolchain go1.26.5` pinned in `go.mod`, so the image builds on the same
govulncheck-clean stdlib.

## Testing (mandatory TDD, infra)

Red→green: today there is no image build/smoke in CI. F1 adds the reusable workflow and
both callers; the gate is:

1. Both images build.
2. api smoke: the container reaches `HEALTHCHECK` state `healthy` within the timeout.
3. web smoke: `curl` to `:3000` returns 200 within the timeout.
4. `docker compose -f docker-compose.yaml config` still validates (image-based, no new
   required env), and `docker compose -f docker-compose.yaml -f docker-compose.build.yaml
   config` validates the build override.

Little pure logic exists to unit-test; verification is build + smoke + compose-config,
consistent with prior infra sub-projects. rclone/docker themselves are trusted.

## Out of scope (F2 and later)

- Getting a published image onto the deploy host (SSH push vs host pull) — F2.
- Running the migrate job and rolling-restarting replicas on deploy — F2.
- Rollback automation — F2 (rollback is "redeploy the previous pinned tag").
- Multi-arch (arm64) images — amd64 only for now.
- Changing the single-binary systemd deploy path (`docs/ops/deploy.md`).
