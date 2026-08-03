# Single Go binary with embedded Next.js, SQLite first

Status: accepted

## Decision

We build the app as **one Go binary**. The Go binary serves a JSON API (with
**Gin**) and also serves the frontend, which is a **Next.js static export
embedded with `embed.FS`**. Node is a build-time tool only; the runtime is one
file. The database is **SQLite through GORM**, with the **pure-Go
`github.com/glebarez/sqlite` driver** so the binary stays cgo-free; GORM's
dialect layer lets us move to **PostgreSQL** later by swapping the driver. The
Go binary terminates **TLS itself** with built-in ACME (Let's Encrypt); it does
not need a reverse proxy.

Auth is a **server-side session** (bcrypt + a session row + Gin middleware), not
JWT, so a login is revoked instantly. CSRF is defended by a `SameSite=Strict`
session cookie plus an Origin check — there is no CSRF token.

## Why

The owner is not an IT person and self-hosts on one small VPS (~350 THB/month,
~30 GB). The easiest thing to run and back up is a single file plus a data
folder. A single Go binary meets that goal: copy one file, run it as a service.
Go-native TLS keeps it to one process, so there is no second service (nginx,
Caddy) to install or renew. SQLite makes the backup one file to copy; write load
is tiny (one editor per district), so SQLite is enough for a long time.

## Considered options

- **Django + PostgreSQL** — its admin gives staff CRUD almost for free, the
  biggest effort saver for a CRUD app. Rejected: it is not a single binary, and
  needs a Python runtime plus a separate DB server to run and back up.
- **Laravel + MySQL** — common on cheap Thai hosting. Rejected for the same
  single-file/self-host reason.
- **Go with `html/template` server-rendered UI** — truly one process, no Node
  at all. Rejected because we chose Next.js for the frontend developer
  experience; we keep the single-binary benefit by embedding the static export.

## Consequences

- **No free admin.** Unlike Django, the staff CRUD screens are hand-built. This
  is accepted effort.
- **Weak SEO for detail pages.** A static export is client-rendered for dynamic
  pages, so search engines index individual healer/recipe pages poorly. Access
  is by QR to a direct link, not by Google, so this is acceptable.
- **Portability via GORM.** All data access goes through GORM, whose dialect
  layer runs on both SQLite and PostgreSQL. No hand-written dialect-specific SQL
  except two documented raw statements (the backup `VACUUM INTO` and the DSN
  pragmas). The switch to PostgreSQL is triggered by a measured performance
  need, not a fixed date.
- **cgo-free is a hard rule.** GORM's default SQLite driver (`gorm.io/driver/
  sqlite`) pulls cgo `mattn/go-sqlite3`, which would break the static single
  binary. We use `github.com/glebarez/sqlite` (pure Go) and require
  `CGO_ENABLED=0 go build` to succeed.

## 15-Factor App compliance

The project rule is to follow the 15-Factor App methodology. We apply it **in
the spirit that fits a single self-hosted binary**, not by re-architecting into
stateless processes with attached network services (which the non-IT owner and
one small VPS cannot support).

We adopt: config from the environment (`config.Load`), logs to stdout, port
binding (the binary owns its listener via autocert), graceful shutdown /
disposability, dev-prod parity through the same binary and env vars, an
API-first JSON layer, and explicit authn/authz.

Documented deviations (deliberate, for cost and simplicity — not oversights):

- **Embedded SQLite** instead of an attached database service. Portable through
  GORM; the move to a PostgreSQL service stays the P5 trigger.
- **Local media directory** instead of object storage. Backup captures it.
- **In-memory login throttle** — process-local state; it resets on restart and
  does not share across instances. Acceptable because the app runs as one
  instance by design.
- **Local nightly backup zips** on the same host; off-site copy is the owner's
  job.

If the app ever needs horizontal scaling, these four are the items to move to
backing services first.
