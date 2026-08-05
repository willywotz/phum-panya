# Sub-project B — Media → Garage object store (design)

Date: 2026-08-05
Branch: `feat/media-garage-object-store`
Status: approved design, ready for implementation plan

## Program context

Sub-project **B** of the 15-Factor + Hexagonal compliance program (see
`docs/superpowers/specs/2026-08-05-hexagonal-core-refactor-design.md` for the
A→E decomposition). Sub-project A (done, merged) made `media.Store` a port with
a filesystem adapter (`LocalStore`). B adds the Garage (S3-compatible) adapter
behind that port and moves image serving through the app.

Order: A ✓ → **B** → C (Postgres-only + shared throttle) → D (telemetry) →
E (migrations-as-release + multi-replica).

## Locked decisions (from brainstorming)

- **Serving:** the app streams `GET /media/<key>` via a new read method on the
  `media.Store` port. Caddy proxies `/media/*` to the api (like `/api/*`).
  Garage stays fully internal (no published port); only the api holds S3
  credentials. One serving path in every mode (dev, single-binary, stack).
- **Backends:** keep `LocalStore`; add `S3Store` (Garage); select via
  `APP_MEDIA_DRIVER` (`local` default, `s3` for the stack). The single Go binary
  and dev-simple keep working on local without Garage.
- **Durability:** a scheduled mirror sidecar (`rclone sync`) copies the Garage
  `media` bucket to a backup volume, alongside the existing `pg_dump` sidecar.

## 1. Goal and non-goals

**Goal:** Store uploaded images in Garage instead of local disk, behind the
existing `media.Store` port, and stream them back through the app. The compose
stack gains Garage, a Garage init job, and a media-mirror sidecar.

**Non-goals:**
- No change to image processing: still decode → resize longest side to 1600px →
  strip EXIF → re-encode JPEG (q80) → content-hash key (`ab/<sha256>.jpg`).
- No frontend change: image URLs stay `/media/<key>`.
- No data migration: the app is pre-launch; no production media exists.
- No change to the SQLite/local backup path (it still zips the local media dir
  when the driver is `local`).

## 2. Port change (the core seam)

`media.Store` gains one read method so the app can serve bytes from any backend:

```go
// Object is a readable stored image plus the metadata the serving handler needs.
type Object struct {
	Body        io.ReadCloser
	ContentType string
	Size        int64
}

// Store (existing port) gains:
Open(key string) (Object, error) // returns media.ErrNotFound if the key is absent
```

- New sentinel `media.ErrNotFound`. `LocalStore.Open` maps `os.IsNotExist` to it;
  `S3Store.Open` maps the S3 `NoSuchKey`/404 to it.
- `LocalStore.Open` opens the file, returns `image/jpeg`, and the stat size.
- Existing `SaveReader`/`SaveMultipart`/`UsageBytes` stay on the port.
- Extract the shared decode/resize/encode/hash logic into one helper
  `processImage(r io.Reader) (key string, jpeg []byte, err error)`. `LocalStore`
  writes the bytes to disk; `S3Store` PutObjects them. This keeps the two
  adapters DRY and keeps the image rules in one place.

## 3. Serving handler

Replace `engine.Static("/media", cfg.MediaDir)` with `GET /media/*key`, handler
in a new `internal/router/media.go` (keeps the `media` package free of gin).

The handler:
1. Trims the leading slash from `*key`; rejects any key containing `..` or that
   does not equal its `path.Clean` result → 404 (path-traversal guard, matches
   what `engine.Static` did implicitly).
2. Calls `store.Open(key)`; maps `media.ErrNotFound` → 404, other errors → 500.
3. Streams `Object.Body` (deferred `Close`), setting `Content-Type`,
   `Content-Length`, and — because keys are content hashes, so bytes never
   change — `Cache-Control: public, max-age=31536000, immutable`.

Caddy's `/media/*` block changes from `file_server` (root `/srv/media`) to
`reverse_proxy api:8080`. The `media` volume mount is removed from the caddy
service.

## 4. S3 / Garage adapter

New file `internal/media/s3store.go`, using **aws-sdk-go-v2** (`config` + `s3`),
path-style addressing (Garage requires it).

- Testability: `S3Store` depends on a tiny internal interface so unit tests need
  no live server:
  ```go
  type s3API interface {
  	PutObject(ctx, *s3.PutObjectInput, ...) (*s3.PutObjectOutput, error)
  	GetObject(ctx, *s3.GetObjectInput, ...) (*s3.GetObjectOutput, error)
  	ListObjectsV2(ctx, *s3.ListObjectsV2Input, ...) (*s3.ListObjectsV2Output, error)
  }
  ```
  The real `*s3.Client` satisfies it; a fake implements it in tests.
- `NewS3Store(ctx, endpoint, region, accessKey, secretKey, bucket string, usePathStyle bool) (*S3Store, error)`
  builds the client with static credentials and `BaseEndpoint = endpoint`.
- `SaveReader` = `processImage` then `PutObject` (ContentType `image/jpeg`).
  `SaveMultipart` opens the upload and calls `SaveReader`.
- `UsageBytes` paginates `ListObjectsV2` and sums `Size`.
- `Open` = `GetObject`; returns body + `ContentType` (`image/jpeg`) + size;
  maps not-found to `media.ErrNotFound`.

Config additions (`internal/config`):

| Env | Meaning | Default |
|-----|---------|---------|
| `APP_MEDIA_DRIVER` | `local` or `s3` | `local` |
| `APP_S3_ENDPOINT` | Garage S3 API URL | (empty) |
| `APP_S3_REGION` | region label | `garage` |
| `APP_S3_BUCKET` | bucket name | `media` |
| `APP_S3_ACCESS_KEY` | access key id | (empty) |
| `APP_S3_SECRET_KEY` | secret key | (empty) |
| `APP_S3_USE_PATH_STYLE` | path-style addressing | `true` |

`main.go` builds `S3Store` when `APP_MEDIA_DRIVER=s3`, else `LocalStore`, and
injects it as `router.Deps.Media` (interface — unchanged from A).

## 5. Compose stack (prod + dev parity)

- **`garage`** (`dxflrs/garage`), internal only — **no published port** — with
  `garage-meta` + `garage-data` volumes and a mounted `deploy/garage/garage.toml`
  (single node, `replication_factor = 1`). Healthcheck on the S3 API port.
- **`garage-init`** one-shot (runs after garage is healthy):
  waits for Garage, assigns and applies the node layout, creates the `media`
  bucket, **imports a fixed key from `APP_S3_ACCESS_KEY`/`APP_S3_SECRET_KEY`**
  (so the api's env credentials match — avoids Garage generating a random key),
  and grants that key read+write on the bucket. Every step is idempotent so a
  restart is safe. Script: `deploy/garage/init.sh`.
- **`media-backup`** sidecar (`rclone/rclone`): loops on a schedule and
  `rclone sync`s the `media` bucket to a `media-backups` volume — mirrors the
  `pg_dump` sidecar pattern. Script: `deploy/backup/media-backup.sh`.
- **api**: gains `APP_MEDIA_DRIVER=s3` + `APP_S3_*` env, `depends_on` garage
  healthy. The shared `media` volume is dropped from api and caddy.
- **Dev** (`docker-compose.dev.yaml`): gains the same `garage` + `garage-init`
  and the api runs in `s3` mode, so dev keeps mirroring prod.
- **`.env.example`**: documents the new vars (with a "set these secrets" note
  like the existing Postgres block).
- **ADR-0004**: records "media object storage via Garage (S3 API)".

## 6. Testing (TDD, mandatory)

- `processImage` helper: unit test (a real small image in → deterministic key +
  JPEG bytes out). Existing `LocalStore` tests continue to cover the write path.
- Config predicates / driver selection: unit tests (red→green).
- `S3Store`: unit-tested against a **fake `s3API`** — Put round-trips through
  `processImage`, `UsageBytes` sums a paged listing, `Open` maps `NoSuchKey` to
  `media.ErrNotFound`. A real-Garage integration test is **skipped unless an
  endpoint env var is set** (same pattern as the existing skipped
  `TestOpenWithPostgres`).
- Serving handler: `httptest` with a fake `media.Store` — asserts the streamed
  bytes, `Content-Type`, `Cache-Control`, 404 on `ErrNotFound`, and 404/400 on a
  `..` traversal key.
- Full Go suite stays green; `docker compose -f docker-compose.yaml config` and
  `-f docker-compose.dev.yaml config` validate.

## 7. Risks / call-outs

- **Garage provisioning** (`garage-init`) is the riskiest, most fiddly piece:
  layout apply and key import must be idempotent and must not race the api. It
  gets its own plan task with explicit `docker compose up` verification.
- **Single-node Garage keeps one data copy.** The mirror sidecar is B's
  durability answer; true multi-node redundancy is out of scope (revisit in E).
- **aws-sdk-go-v2** adds several `github.com/aws/aws-sdk-go-v2/*` modules to
  `go.mod`. Acceptable — it is the standard, well-maintained S3 client; Garage
  speaks its dialect with path-style addressing.

## Success criteria

- With `APP_MEDIA_DRIVER=s3`, uploads land in Garage and `GET /media/<key>`
  streams them back with the immutable cache header; with `local`, behavior is
  unchanged.
- No handler or route string changes beyond the `/media` serving swap; existing
  photo-upload endpoints are untouched.
- `media.Store` has exactly one new method (`Open`) and one new sentinel
  (`ErrNotFound`); both adapters satisfy the port (compile-time assertions).
- Full test suite green; both compose files validate; Garage has no published
  port.
