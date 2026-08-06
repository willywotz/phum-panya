# CD F2 — Push-based deploy automation

Date: 2026-08-06
Status: Approved (design)
Scope: `.github/workflows/deploy.yml`, `deploy/deploy.sh`, `deploy/deploy-lib.sh`, `deploy/deploy_test.go`, `docs/ops/deploy-compose.md`, docs
Closes: issue #19 (No CD)
Builds on: F1 (ADR-0010, published ghcr images), ADR-0007 (migrate job + multi-replica)

## Problem

F1 publishes immutable `ghcr.io/willywotz/phum-panya-{api,web}` images, but getting a
released image onto the client VPS is still manual (`docs/ops/deploy-compose.md`), and
that doc is stale — it describes the pre-A-E flow (build on host, single api, AutoMigrate
on boot) rather than the current image-based, migrate-job, multi-replica stack. Issue #19
asks for continuous deployment.

## Decision

A **push-based**, **manually-triggered**, **self-healing** deploy:

- `.github/workflows/deploy.yml`, `workflow_dispatch` only, with a `version` input. The
  operator runs it to deploy a version; running it with an older version is the rollback.
- The job SSHes to the VPS (plain `ssh`, key from a GitHub secret — no marketplace
  action, matching the repo style) and runs `deploy/deploy.sh <version>` on the host.
- The script is health-gated with automatic rollback: a failed deploy restores the
  previous version and fails the workflow so the operator is notified.

Brief downtime during the api-replica recreate is accepted: plain Docker Compose has no
native rolling update (that is Swarm/k8s), and the app is low-traffic and deployed by
manual trigger. Zero-downtime rolling is out of scope for F2.

### Alternatives considered

- **Pull model (host self-updates via a timer/agent).** Rejected (operator choice):
  push gives one control point and simple rollback; the operator accepted storing an SSH
  key as a CI secret and inbound SSH.
- **Auto-deploy on release (gated or not).** Rejected (operator choice): manual dispatch
  avoids surprise deploys to a client production host and doubles as the rollback path.
- **Fire-and-forget / fail-loud only.** Rejected: the operator chose health-gate with
  automatic rollback (self-healing, still fails the run so a human is notified).
- **Zero-downtime rolling replica restart.** Deferred: not worth the scripting for a
  low-traffic, manually-deployed single-client app.

## Design

### Workflow — `.github/workflows/deploy.yml`

```yaml
on:
  workflow_dispatch:
    inputs:
      version:
        description: Released version to deploy (e.g. 1.5.0)
        required: true
```

One job on `ubuntu-latest`:
1. Write the SSH key from `secrets.DEPLOY_SSH_KEY` to a file (mode 600), add the host to
   `known_hosts` (`ssh-keyscan`).
2. `ssh DEPLOY_SSH_USER@DEPLOY_SSH_HOST "cd $DEPLOY_PATH && ./deploy/deploy.sh <version>"`.

Secrets (documented; the owner configures them): `DEPLOY_SSH_HOST`, `DEPLOY_SSH_USER`,
`DEPLOY_SSH_KEY`, `DEPLOY_PATH` (the stack checkout dir on the host). The `version` input
is validated host-side by the script.

### Host script — `deploy/deploy.sh <version>`

Sources `deploy/deploy-lib.sh`. Sequence:

1. `validate_version "$1"` — reject anything not `N.N.N`; exit non-zero with a clear
   message otherwise.
2. `prev=$(read_tag .env)` — the currently-deployed version (`APP_IMAGE_TAG`), for
   rollback. If unset, default `latest`.
3. `git fetch --tags --quiet` and `git checkout "v$version"` — align the compose topology
   with the images being deployed (the compose file evolves between releases).
4. `write_tag .env "$version"`.
5. `docker compose pull` then `docker compose up -d --wait` (migrate job runs the DDL
   once via its `service_completed_successfully` dependency, then the api replicas
   recreate).
6. Health gate: poll `https://$APP_DOMAIN/api/health` (read `APP_DOMAIN` from `.env`)
   until the body reports `ok`, up to a timeout.
7. On any failure in steps 5-6 (`up --wait` non-zero, or health never passes): roll back —
   `git checkout "v$prev"` (if `prev` is a version) or restore the prior state,
   `write_tag .env "$prev"`, `docker compose pull`, `docker compose up -d --wait`, then
   `exit 1` so the workflow run goes red and the operator is notified.

`docker compose up -d --wait` is best-effort (the one-shot `migrate` job complicates
`--wait` semantics); the authoritative gate is the explicit `/api/health` poll.

### Pure library — `deploy/deploy-lib.sh`

Pure POSIX-sh helpers, no docker/git/ssh, unit-tested:

- `validate_version v` → return 0 for `N.N.N`, else print an error to stderr and return 1.
- `read_tag envfile` → print the value of `APP_IMAGE_TAG=` from `envfile`, or empty if
  absent.
- `write_tag envfile v` → set `APP_IMAGE_TAG=v` in `envfile` (replace the existing line,
  or append if absent), leaving all other lines unchanged.

### Testing (mandatory TDD)

A Go test `deploy/deploy_test.go` execs the helpers via `sh -c '. ./deploy-lib.sh; ...'`
(no docker/git/ssh; runs in `go test ./...`):

1. `validate_version` — accepts `1.5.0`; rejects `v1.5.0`, `1.5`, `latest`, `1.5.0-rc1`
   (non-zero, names the bad value).
2. `read_tag` — returns the tag from a fixture `.env`; empty when the key is absent;
   returns only the value, not the whole line.
3. `write_tag` — replaces an existing `APP_IMAGE_TAG=` line in place (other lines
   untouched); appends the line when the key is absent; a subsequent `read_tag` returns
   the written value (round-trip).

docker/git/ssh side effects are trusted and not exercised; the end-to-end proof is the
owner running the dispatch against the live VPS.

### Documentation

- Rewrite `docs/ops/deploy-compose.md` to the current reality: image-based services
  (F1), the `migrate` job + `APP_AUTO_MIGRATE=false`, the 2 api replicas behind Caddy,
  `APP_IMAGE_TAG` selection, and the new deploy/rollback flow (dispatch `deploy.yml` with
  a `version`; rollback = dispatch an older `version`). List the required GitHub secrets
  and the one-time host setup (clone the repo at `DEPLOY_PATH`, populate `.env`, `docker
  login ghcr.io` for private-image pulls if the packages are private).
- ADR-0011 (push-based CD): the model, the manual-dispatch trigger, and health-gated
  auto-rollback.

## Scope boundary

F2 ships the workflow, the host script + pure lib + tests, the corrected docs, and the
secret list. End-to-end validation requires the owner's VPS reachable over SSH with the
secrets configured — that live dispatch is the owner's step.

## Out of scope

- Zero-downtime rolling replica restart.
- Auto-deploy on release / approval-gated triggers (manual dispatch only for now).
- Provisioning the host (Docker install, first clone, firewall) — a one-time manual
  setup documented in `deploy-compose.md`.
- The single-binary systemd deploy path (`docs/ops/deploy.md`) — unchanged.
