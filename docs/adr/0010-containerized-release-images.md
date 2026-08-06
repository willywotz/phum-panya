# Containerized release images published to ghcr.io

Status: accepted

Context: the compose stack (ADR-0003, ADR-0007) ran its `api`, `migrate`, and
`web` services from `build:` directives — each service compiled from source on
the deploy host at `docker compose` time — and `release.yml` published only the
Linux/Windows binaries and the MSI, never a container image. This has two
problems. It **violates 15-Factor build/release/run separation** (Factor V),
which ADR-0001 commits the project to: the build stage should produce one
immutable artifact that the run stage executes unchanged, not rebuild per
deploy. And it leaves continuous deployment (issue #19) with nothing immutable
to roll forward to or back to — every deploy would be a fresh, non-reproducible
host build. This ADR covers **F1** of the CD work: produce and publish the
release artifact. **F2** (getting a published release onto the host) is a
separate later decision.

## Decision

Build the stack's two images in CI and publish them to GitHub Container
Registry, and run those images instead of building on the host:

- `ghcr.io/willywotz/phum-panya-api` — the Go API image (`Dockerfile.api`); the
  `api` and `migrate` services share it (migrate runs the `server migrate`
  subcommand).
- `ghcr.io/willywotz/phum-panya-web` — the Next.js UI image (`web/Dockerfile`).
- Tags: the release version with a leading `v` stripped (e.g. `1.4.0`) and
  `latest`; linux/amd64.

Building is DRY across triggers. A reusable workflow
(`.github/workflows/build-images.yml`, `workflow_call` with a `push` input) runs
one script (`deploy/images/build-and-smoke.sh`) that builds both images and
smoke-tests them: the api image is booted in dev mode and polled until its
built-in `HEALTHCHECK` reports `healthy`; the web image is polled with `curl` on
`:3000`. `ci.yml` calls the workflow with `push:false` (build + smoke on every
push/PR, so a broken Dockerfile is caught before release); `release.yml` calls
it with `push:true` on a `v*` tag, logging into ghcr with the built-in
`GITHUB_TOKEN` and pushing the version and `latest` tags.

The compose stack switches `api`/`migrate`/`web` from `build:` to
`image: ghcr.io/willywotz/phum-panya-{api,web}:${APP_IMAGE_TAG:-latest}`. A new
`docker-compose.build.yaml` override restores the `build:` directives for local
image builds; `docker-compose.dev.yaml` is unchanged (keeps `build:` + hot
reload). `Dockerfile.api`'s Go base is pinned to `golang:1.26.5-alpine`, matching
the `toolchain go1.26.5` in `go.mod`.

## Why

Building once in CI and running the exact image is the 15-Factor build/release/run
discipline the project already committed to (ADR-0001), and it is the
precondition for any real CD: a versioned, immutable artifact that a deploy can
pin and a rollback can re-select. ghcr.io needs no extra account — the built-in
`GITHUB_TOKEN` already grants `packages: write` — and keeps the images beside the
code and the existing binary/MSI release. Building and smoke-testing on every
push/PR (not only on tag) surfaces Dockerfile breakage early. A reusable workflow
keeps the single build definition DRY across the CI and release triggers.

## Considered options

- **Build on the host per deploy** (`git pull` + `docker compose build`).
  Rejected: violates build/release/run separation and makes every deploy a
  fresh, non-reproducible build with a toolchain on the production host.
- **A managed/third-party registry.** Rejected: ghcr.io needs no extra account
  and keeps artifacts with the repo; the single-VPS model avoids external
  dependencies where it can.
- **Build + push only on tag (no per-PR build).** Rejected: a broken Dockerfile
  would not surface until release time.
- **Pin the compose default to an exact version instead of `latest`.** Rejected
  for the default: `${APP_IMAGE_TAG:-latest}` keeps `docker compose config`
  (the CI `stack-validate` job) valid with no new required env; prod and F2 pin
  `APP_IMAGE_TAG` to an exact released version.

## Consequences

- **An immutable, versioned release artifact exists** for the compose stack —
  the foundation F2 (deploy automation, issue #19) builds on.
- **CI does more work**: every push/PR builds and smoke-tests both images. The
  smoke test also gives early proof the images boot, migrate, and serve.
- **The deploy host no longer needs the source or a build toolchain** for the
  compose stack; `docker compose up` pulls the published images. Local image
  builds remain available via the `docker-compose.build.yaml` override.
- **Operators pin `APP_IMAGE_TAG`** to a released version in production; the
  default `latest` keeps config validation and quick starts working.
- **The single-binary systemd deploy path (`docs/ops/deploy.md`) is unchanged**;
  this ADR concerns only the compose stack.
