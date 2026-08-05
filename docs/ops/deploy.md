# Deploy to the VPS (Linux, systemd)

Deploy of **`v1.1.0`** to a Linux VPS. There is no CD; this is manual.
The server is one static, cgo-free binary that embeds the UI and stores all
state in a `data/` directory. It terminates TLS itself (built-in Let's Encrypt),
so no nginx/Caddy is needed.

## 0. Before you start

- A VPS you can reach over SSH, with `root`/`sudo`.
- A domain name whose DNS **A record points at the VPS public IP**.
- Ports **80 and 443** open to the internet (Let's Encrypt validates over 80;
  the app serves over 443). Open them in both the cloud firewall and any host
  firewall (`ufw allow 80,443/tcp`).
- A long, unique admin password ready (seeds the first central-admin login).

## 1. Put the binary on the host

Download the released Linux binary and install it to `/usr/local/bin`:

```bash
curl -fL -o /usr/local/bin/phum-panya \
  https://github.com/willywotz/phum-panya/releases/download/v1.1.0/server-v1.1.0-linux-amd64
chmod +x /usr/local/bin/phum-panya
/usr/local/bin/phum-panya --help 2>&1 | head -1   # sanity check it runs
```

> The release also ships `phum-panya-v1.1.0-windows-amd64.msi` and
> `server-v1.1.0-windows-amd64.exe` for Windows hosts — not used here.

## 2. Install and start the service

The binary registers itself with systemd. `service install` bakes the seed
config into the unit, so the service comes up already configured:

```bash
phum-panya service install \
  --admin-email=admin@your-domain.org \
  --admin-password='<LONG-RANDOM-PASSWORD>' \
  --domain=your-domain.org
phum-panya service start
```

What this creates:

- systemd unit **`phum-panya`** (data under working dir **`/var/lib/phum-panya`**).
- On first start the DB is migrated and the first central admin is seeded from
  the flags above. No manual migration step — GORM `AutoMigrate` runs on boot.
- Because `--domain` is set (and `APP_DEV` is not), the server gets a
  Let's Encrypt certificate for that domain and serves HTTPS on **:443**
  (with :80 for the ACME challenge and HTTP→HTTPS).

Data layout (relative to `/var/lib/phum-panya`):

| Path | What |
|---|---|
| `data/app.db` | SQLite database (`APP_DB_PATH`) |
| `data/media/` | uploaded photos (`APP_MEDIA_DIR`) |
| `data/backup/` | nightly backup zips, newest 14 kept (`APP_BACKUP_DIR`) |

### Config reference (env vars the binary reads)

| Variable | Purpose | Default |
|---|---|---|
| `APP_DOMAIN` | public domain → built-in TLS on 80/443 | unset |
| `APP_DEV` | `1` = plain HTTP on `:8080` (do **not** set in prod) | unset |
| `APP_HTTP_ADDR` | listen address | `:8080` |
| `APP_DB_PATH` | SQLite file | `data/app.db` |
| `APP_MEDIA_DIR` | photo storage | `data/media` |
| `APP_BACKUP_DIR` | nightly backup zips | `data/backup` |
| `APP_ADMIN_EMAIL` / `APP_ADMIN_PASSWORD` | first admin, seeded once | unset |

The `service install` flags map to `APP_ADMIN_EMAIL`, `APP_ADMIN_PASSWORD`,
`APP_DOMAIN`, `APP_HTTP_ADDR`. To seed/confirm the admin without starting the
server: `phum-panya create-admin`.

## 3. Verify it is up

```bash
systemctl status phum-panya --no-pager
journalctl -u phum-panya -n 50 --no-pager      # watch first-boot + ACME logs
curl -fsS https://your-domain.org/api/health   # expect {"status":"ok"}
```

Then open `https://your-domain.org/` in a browser, sign in at `/login` with the
seeded admin, and confirm the staff nav shows the admin screens
(`/staff/review`, `/staff/year-locks`, `/staff/imports`, `/staff/herbs`).

## 4. Run the live UAT (SRS §6.1)

On the live host, walk the editor→pending→admin-approve→public flow end to end:

1. As admin, create a district and a district-editor user.
2. Sign in as the editor; create a doctor/recipe/case — confirm it enters the
   **pending** queue, not the public site.
3. As admin, approve it in `/staff/review`; confirm it now appears in the public
   pages (and only after consent + approval).

## 5. Backups (verify, then take off-site)

The server writes a dated backup zip to `data/backup/` every 24h and keeps the
newest 14. **Off-site copies are the owner's job** — schedule a pull to another
host or object storage. Restore procedure: [`restore.md`](restore.md).

## Upgrading later

```bash
phum-panya service stop
curl -fL -o /usr/local/bin/phum-panya \
  https://github.com/willywotz/phum-panya/releases/download/<new-tag>/server-<new-tag>-linux-amd64
chmod +x /usr/local/bin/phum-panya
phum-panya service start      # AutoMigrate applies any new columns/tables on boot
```

Data in `/var/lib/phum-panya` is untouched by a binary swap. Take a backup
(or copy `data/`) before upgrading a production host.

## Rollback

Re-download the previous tag's binary over `/usr/local/bin/phum-panya` and
`service restart`. If a bad migration corrupted data, restore the latest good
backup zip per [`restore.md`](restore.md). Note: SQLite migrations here are
additive (new columns/tables), so a straight binary rollback is normally safe.
