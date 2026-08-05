# Deploy the compose stack (Postgres + Caddy) — OPTIONAL

> **This is an optional deploy path.** The single binary in
> [`deploy.md`](deploy.md) (SQLite, built-in TLS) is the **default** and needs
> no Docker. Use this path only if you want Postgres and/or run several
> services behind a shared reverse proxy. See ADR-0003 for the rationale.

This path runs five containers via `docker-compose.yaml`: **caddy**
(TLS + reverse proxy), **web** (Next.js standalone), **api** (the Go server,
`APP_BEHIND_PROXY=1`, `APP_DB_DRIVER=postgres`), **postgres**, and **backup**
(a `pg_dump` sidecar). Only `caddy` publishes ports; the other services talk
over the compose network.

## 0. Before you start

- Docker Engine and the Docker Compose plugin (`docker compose version`) on
  the host.
- A domain name whose DNS **A record points at the host public IP**.
- Ports **80 and 443** open to the internet (Caddy's ACME validates over 80;
  the stack serves over 443). Open them in both the cloud firewall and any
  host firewall (`ufw allow 80,443/tcp`).
- A long, unique admin password and a long, unique Postgres password ready.

## 1. Clone and set up `.env`

```bash
git clone https://github.com/willywotz/phum-panya.git
cd phum-panya
cp .env.example .env
```

Edit `.env` and set the three required vars (see the "Prod stack" block in
`.env.example`):

```
APP_DOMAIN=your-domain.org
APP_ADMIN_PASSWORD=<LONG-RANDOM-PASSWORD>
POSTGRES_PASSWORD=<LONG-RANDOM-PASSWORD>
```

`APP_ADMIN_EMAIL`, `POSTGRES_DB`, and `POSTGRES_USER` have defaults
(`admin@example.com`, `phumpanya`, `phumpanya`) but can be overridden in the
same file.

## 2. Bring the stack up

```bash
docker compose up -d --build
```

What this does on first boot:

- `postgres` starts and reports healthy (`pg_isready`).
- `api` waits for postgres, then runs GORM `AutoMigrate` and seeds the first
  central admin from `APP_ADMIN_EMAIL` / `APP_ADMIN_PASSWORD`.
- `caddy` gets a Let's Encrypt certificate for `APP_DOMAIN` and serves HTTPS
  on **:443** (with :80 for the ACME challenge and HTTP→HTTPS), routing
  `/api/*` and media uploads to `api`, `GET`/`HEAD /media/*` reads directly
  from the shared volume, and everything else to `web`.
- `backup` waits for postgres, then loops a daily `pg_dump` into the
  `pg-backups` volume.

## 3. Verify it is up

```bash
docker compose ps
docker compose logs -f caddy   # watch ACME issuance
curl -fsS https://your-domain.org/api/health                # expect {"status":"ok"}
```

Then open `https://your-domain.org/` in a browser, sign in at `/login` with
the seeded admin, and confirm the staff nav shows the admin screens
(`/staff/review`, `/staff/year-locks`, `/staff/imports`, `/staff/herbs`).

## 4. Run the live UAT (SRS §6.1)

On the live host, walk the editor→pending→admin-approve→public flow end to
end:

1. As admin, create a district and a district-editor user.
2. Sign in as the editor; create a doctor/recipe/case — confirm it enters the
   **pending** queue, not the public site.
3. As admin, approve it in `/staff/review`; confirm it now appears in the
   public pages (and only after consent + approval).

## 5. Backups (verify, then take off-site)

The `backup` service writes a dated, gzip-compressed `pg_dump` into the
`pg-backups` named volume every 24h and keeps the newest 14:

```bash
docker compose exec backup ls -la /backups
```

**Off-site copies are the owner's job** — schedule a pull of the
`pg-backups` volume to another host or object storage. To restore, copy the
desired dump out of the volume and pipe it into `psql` against a fresh
`postgres` container:

```bash
docker compose exec backup \
  sh -c 'gunzip -c /backups/backup-<date>.sql.gz' | \
  docker compose exec -T postgres \
  psql -U phumpanya -d phumpanya
```

## Upgrading later

```bash
git pull
docker compose pull   # refresh third-party images
docker compose up -d --build
```

`AutoMigrate` applies any new columns/tables on boot. Take a Postgres backup
(see §5) before upgrading a production host.

## Rollback

Check out the previous release tag and rebuild:

```bash
git checkout <previous-tag>
docker compose up -d --build
```

If a bad migration corrupted data, restore the latest good `pg_dump` per §5.
Note: migrations here are additive (new columns/tables), so a straight
rollback is normally safe.
