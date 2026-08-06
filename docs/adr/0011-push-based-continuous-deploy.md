# Push-based continuous deployment of the compose stack

Status: accepted

Context: F1 (ADR-0010) publishes immutable `ghcr.io/willywotz/phum-panya-{api,web}`
images, but getting a released image onto the client VPS was still manual, and
`docs/ops/deploy-compose.md` described the pre-A-E flow (build on host,
AutoMigrate on boot). Issue #19 asked for continuous deployment. This ADR
covers **F2**: automate the deploy while keeping a human in control of a
single client production host.

## Decision

Deploys are **push-based**, **manually triggered**, and **self-healing**:

- `.github/workflows/deploy.yml` runs on `workflow_dispatch` with a `version`
  input. An operator runs it to deploy a version; running it with an older
  version is the rollback. There is no auto-deploy on release.
- The workflow validates `version` against `^[0-9]+\.[0-9]+\.[0-9]+$` on the
  runner, then SSHes to the host (key from a GitHub Actions secret,
  `StrictHostKeyChecking=accept-new`) and runs `deploy/deploy.sh <version>`.
- `deploy/deploy.sh` captures the currently-deployed `APP_IMAGE_TAG`, checks
  out the `v<version>` tag (so the compose topology matches the images), sets
  `APP_IMAGE_TAG`, `docker compose pull`, `docker compose up -d --wait`, then
  polls `https://$APP_DOMAIN/api/health`. If the deploy fails or never becomes
  healthy, it **automatically rolls back** to the previous version and exits
  non-zero so the run goes red and the operator is notified.
- Pure, testable helpers (`validate_version`, `read_tag`, `write_tag`) live in
  `deploy/deploy-lib.sh` and are unit-tested (`deploy/deploy_test.go`);
  docker/git/ssh are trusted.

Brief downtime during the api-replica recreate is accepted: plain Docker
Compose has no native rolling update (that is Swarm/k8s), and the app is
low-traffic and deployed by manual trigger.

## Why

Push from CI gives one control point and a trivial rollback (dispatch an older
version), which suits a single client VPS. Manual dispatch avoids surprise
deploys to a production host and doubles as the rollback path. Health-gated
auto-rollback makes a bad release self-correct while still failing the run so a
human is notified. Validating the version on the runner before it reaches the
SSH command string closes command-injection: a `workflow_dispatch` actor (who
already has repo write access) still cannot smuggle shell metacharacters
through, because only `N.N.N` ever reaches the remote shell.

## Considered options

- **Pull model (host self-updates via a timer/agent).** Rejected: push gives
  one control point and simpler rollback; the operator accepted storing an SSH
  key as a CI secret and inbound SSH.
- **Auto-deploy on release (gated or ungated).** Rejected: manual dispatch
  avoids surprise production deploys and is itself the rollback mechanism.
- **Fire-and-forget / fail-loud only.** Rejected: health-gated auto-rollback is
  self-healing and still notifies via a failed run.
- **Zero-downtime rolling replica restart.** Deferred: not worth the scripting
  for a low-traffic, manually-deployed single-client app.

## Consequences

- **Issue #19 is closed**: a released version is deployed (and rolled back) by
  running one workflow.
- **New operational surface**: the host must accept inbound SSH, and four
  `DEPLOY_SSH_*` / `DEPLOY_PATH` secrets must be set in the repo. The one-time
  host + repo setup is documented in `docs/ops/deploy-compose.md`.
- **The host holds the stack checkout and `.env`**; the deploy checks out the
  release tag so the compose file matches the images being run.
- **End-to-end validation is the owner's step**: it needs the live VPS
  reachable over SSH with the secrets configured.
- **The single-binary systemd deploy path (`docs/ops/deploy.md`) is
  unchanged**; this ADR concerns only the compose stack.
