# Deploy the compose stack — OPTIONAL

> **This is an optional deploy path.** The single binary in
> [`deploy.md`](deploy.md) (SQLite, built-in TLS) is the **default** and needs
> no Docker. Use this path only for the multi-service stack (Postgres, Garage
> object storage, multiple api replicas behind Caddy). See ADR-0003 and
> ADR-0007 for the rationale.

This path runs the stack in `docker-compose.yaml`. The services:

| Service | Role |
|---|---|
| `caddy` | TLS (Let's Encrypt) + reverse proxy; the only service that publishes ports |
| `web` | Next.js UI (published image) |
| `api` | the Go server, **2 replicas**, `APP_AUTO_MIGRATE=false` (published image) |
| `migrate` | one-shot DDL job (`server migrate`); the api replicas wait for it |
| `postgres` | database |
| `backup` | daily `pg_dump` sidecar into the `pg-backups` volume |
| `garage` + `garage-init` | S3-compatible object storage for media (ADR-0004) |
| `jaeger` | trace collector (ADR-0006) |
| `media-backup` | off-host media backup sidecar (ADR-0009) |

The `api` and `web` services run **published images** from
`ghcr.io/willywotz/phum-panya-{api,web}` (ADR-0010) selected by
`APP_IMAGE_TAG`; they are **not** built on the host. Migrations run once as the
`migrate` job before the replicas serve (ADR-0007).

## 0. Before you start

- Docker Engine and the Docker Compose plugin (`docker compose version`) on
  the host.
- A domain name whose DNS **A record points at the host public IP**.
- Ports **80 and 443** open (Caddy's ACME validates over 80; the stack serves
  over 443). Open them in both the cloud firewall and any host firewall
  (`ufw allow 80,443/tcp`).
- Long, unique values for the required secrets (see step 1).

## 1. Clone and set up `.env`

```bash
git clone https://github.com/willywotz/phum-panya.git
cd phum-panya
cp .env.example .env
```

Edit `.env` and set the required vars (see `.env.example`):

```
APP_DOMAIN=your-domain.org
APP_ADMIN_PASSWORD=<LONG-RANDOM>
POSTGRES_PASSWORD=<LONG-RANDOM>
GARAGE_RPC_SECRET=<64-HEX-CHARS>
APP_S3_ACCESS_KEY=GK<24-HEX>          # see ADR-0004 for the key format
APP_S3_SECRET_KEY=<64-HEX>
APP_IMAGE_TAG=1.4.0                   # the released version to run (not "latest" in prod)
```

`APP_ADMIN_EMAIL`, `POSTGRES_DB`, `POSTGRES_USER`, and the media-backup
destination have defaults (see `.env.example`).

## 2. Log in to the registry (only if the images are private)

If the ghcr packages are private, authenticate once so the host can pull:

```bash
echo "$GITHUB_TOKEN" | docker login ghcr.io -u <github-user> --password-stdin
```

Public packages need no login.

## 3. Bring the stack up

```bash
docker compose pull            # pull the api/web images at APP_IMAGE_TAG
docker compose up -d --wait
```

What this does on first boot:

- `postgres` and `garage` start and report healthy.
- `garage-init` creates the media bucket and imports the fixed access key.
- `migrate` runs `server migrate` once (GORM `AutoMigrate` + admin seed), then
  exits; the two `api` replicas wait for it to complete before starting.
- `caddy` gets a Let's Encrypt certificate for `APP_DOMAIN`, serves HTTPS on
  **:443** (with :80 for ACME + HTTP→HTTPS), load-balances `/api/*` and media
  uploads across the api replicas (active health check on `/api/health`), and
  routes everything else to `web`.
- `backup` loops a daily `pg_dump` into `pg-backups`; `media-backup` mirrors
  the Garage bucket off-host (ADR-0009).

## 4. Verify it is up

```bash
docker compose ps
curl -fsS https://your-domain.org/api/health          # expect {"status":"ok"}
```

Then open `https://your-domain.org/`, sign in at `/login` with the seeded
admin, and confirm the staff nav shows the admin screens.

## 5. Backups

- **Postgres**: `backup` writes a dated gzip `pg_dump` into `pg-backups` every
  24h (newest 14 kept). See [`restore.md`](restore.md) to restore.
- **Media**: `media-backup` pushes the Garage bucket to an off-host rclone
  remote with dated archives (ADR-0009).

## Continuous deployment (push model)

Deploys are driven by the **`deploy` GitHub Actions workflow**
(`.github/workflows/deploy.yml`), triggered manually:

1. In the repo's **Actions** tab, run the **deploy** workflow and enter the
   released `version` (e.g. `1.5.0`).
2. The workflow SSHes to the host and runs `deploy/deploy.sh <version>`, which:
   checks out the `v<version>` tag, sets `APP_IMAGE_TAG=<version>`, pulls the
   images, `docker compose up -d --wait`, then polls `/api/health`.
3. **Auto-rollback**: if the new version is unhealthy, the script restores the
   previously-deployed version and the workflow run fails (so you are
   notified). Brief downtime during the api-replica recreate is expected.

**Rollback** is the same workflow with an older `version`.

### One-time host + repo setup for CD

On the host: clone the repo at a fixed path, populate `.env` (steps 1-2), and
ensure the SSH user can run `docker compose` in that directory.

In the GitHub repo, set these **Actions secrets** (Settings → Secrets and
variables → Actions):

| Secret | Value |
|---|---|
| `DEPLOY_SSH_HOST` | the VPS host or IP |
| `DEPLOY_SSH_USER` | the SSH user |
| `DEPLOY_SSH_KEY` | the private SSH key (PEM) authorized on the host |
| `DEPLOY_PATH` | the stack checkout directory on the host |

The `version` input is validated (`N.N.N`) on the runner before use.

## Manual upgrade / rollback (without the workflow)

```bash
git fetch --tags && git checkout v<version>
# set APP_IMAGE_TAG=<version> in .env
docker compose pull && docker compose up -d --wait
```

To roll back, repeat with the previous version tag. Migrations are additive
(new columns/tables), so a straight rollback is normally safe; if a bad
migration corrupted data, restore the latest good `pg_dump` (see
[`restore.md`](restore.md)).
