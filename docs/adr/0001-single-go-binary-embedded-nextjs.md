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
