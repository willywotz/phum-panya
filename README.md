# phum-panya

A folk-medicine records web app (ตำรายาหมอพื้นบ้าน) for one province, grouped by
district (อำเภอ). The public can browse consented healer (หมอพื้นบ้าน) profiles, their
recipes, and treatment cases; a central admin and one editor per district maintain
the data. It ships as a single self-hosted Go binary with an embedded Next.js
static UI and a SQLite database — no separate frontend server, no external
database to run.

## Build

```
make build
```

This runs the Next.js static export, embeds it into the Go binary's asset
directory, then builds `./server`. It needs Go and Node installed; nothing
else. The build is `CGO_ENABLED=0` (pure-Go SQLite driver), so `./server` is
a single, statically linked, cgo-free binary — copy it to a host and run it.

## Run

Configure with environment variables (all optional, with sane defaults):

| Variable | Purpose | Default |
|---|---|---|
| `APP_DOMAIN` | Public domain; when set (and `APP_DEV` is unset), the server terminates TLS itself via Let's Encrypt and needs ports 80/443 | unset |
| `APP_DB_PATH` | SQLite database file path | `data/app.db` |
| `APP_MEDIA_DIR` | Uploaded photo storage directory | `data/media` |
| `APP_BACKUP_DIR` | Nightly backup zip output directory | `data/backup` |
| `APP_ADMIN_EMAIL` | First central admin's email, seeded on first run | unset |
| `APP_ADMIN_PASSWORD` | First central admin's password, seeded on first run | unset |
| `APP_DEV` | Set to `1` to run plain HTTP on `:8080` instead of terminating TLS | unset |

```
APP_DEV=1 APP_ADMIN_EMAIL=admin@example.com APP_ADMIN_PASSWORD=changeme ./server
```

On first run, the server seeds the first central admin from
`APP_ADMIN_EMAIL`/`APP_ADMIN_PASSWORD` if no admin exists yet. To seed (or
confirm) that admin without starting the server, run:

```
./server create-admin
```

In production, set `APP_DOMAIN` to a real domain and drop `APP_DEV`: the
server then terminates TLS itself and needs ports 80 and 443 open.

## Restore

See [`docs/ops/restore.md`](docs/ops/restore.md) to restore the database and
media from a nightly backup zip.

## Test

```
go test ./...
cd web && npx playwright test
```
