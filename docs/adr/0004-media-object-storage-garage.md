# Media object storage: Garage behind the media.Store port

Status: accepted

Context: the container stack (ADR-0003) ran the api and Caddy against a
shared Docker volume for uploaded images — fine for a single node, but it
pins media to one host's disk and blocks running multiple api replicas
(15-Factor: processes must be stateless, backing services attached).
Sub-project B (`docs/superpowers/plans/2026-08-05-media-garage-object-store.md`)
moves media into an S3-compatible object store while keeping the existing
`media.Store` port and image-processing rules unchanged.

## Decision

We add **Garage** (`dxflrs/garage`), a single-node S3-compatible object
store, as an internal-only compose service with no published port. The api
gains an `S3Store` adapter (`aws-sdk-go-v2`, path-style) alongside the
existing `LocalStore`, both satisfying the `media.Store` port. `APP_MEDIA_DRIVER`
(`local` | `s3`) selects the adapter at startup; only the api holds S3
credentials. The api serves `GET /media/*key` by streaming from
`Store.Open`, so Caddy now reverse-proxies `/media/*` to the api instead of
serving a shared volume with `file_server`. A one-shot `garage-init` service
runs Garage's layout/bucket setup and imports a fixed access key (from
`APP_S3_ACCESS_KEY`/`APP_S3_SECRET_KEY`) so the api's credentials always
match. An `rclone` sidecar (`media-backup`) mirrors the bucket to a Docker
volume on an interval, for durability outside Garage's own storage.

Dev (`docker-compose.dev.yaml`) mirrors prod exactly: the same `garage` +
`garage-init` services and the same `APP_MEDIA_DRIVER=s3` env on the dev
api, so what is developed against matches what ships. The dev stack does
not run a `media-backup` sidecar (not needed for disposable dev data).

## Why

The `media.Store` port (hexagonal architecture) already isolated image
storage behind `SaveReader`/`SaveMultipart`/`UsageBytes`; adding `Open` and
a second adapter is a driver swap, not a rewrite — the same discipline
ADR-0003 relied on for the Postgres driver swap. Garage is chosen over a
managed S3 provider because the deploy target is a single self-hosted VPS:
Garage runs in the same compose stack, needs no external account or egress
cost, and speaks the standard S3 API so the adapter has no Garage-specific
code path. Caddy proxying `/media/*` to the api (rather than serving files
directly) is the necessary consequence of media no longer living on a
locally-mounted disk.

## Considered options

- **Keep the shared Docker volume, add a periodic sync to S3 for backup
  only.** Rejected: still ties media to one host's disk, so it does not
  solve the statelessness/multi-replica problem — only the backup story
  improves.
- **Use a managed S3 provider (e.g. a cloud object store) instead of
  self-hosting Garage.** Rejected: adds an external account, egress
  billing, and network dependency that the single-VPS deploy model
  (ADR-0001, ADR-0003) is designed to avoid; Garage keeps everything in
  the same compose stack.
- **Serve media directly from Garage via Caddy (skip the api).** Rejected:
  the api holds the only S3 credentials — publishing Garage or its
  credentials to Caddy would leak write access and complicate the PDPA
  gate that already lives at the api layer.

## Consequences

- **Single-node caveat.** Garage is configured with `replication_factor = 1`;
  it is not yet a multi-node durable cluster. The `media-backup` rclone
  sidecar is the interim durability measure until multi-node Garage (or an
  off-host backup target) is set up.
- **New dependency.** `aws-sdk-go-v2` (`config`, `credentials`,
  `service/s3`) is added to `go.mod`; it is pure Go, so `CGO_ENABLED=0`
  still builds.
- **Dev/prod parity preserved.** Both compose files run the same Garage
  node and the same `APP_MEDIA_DRIVER=s3` api config, so image upload/serve
  behaviour does not diverge between dev and prod.
- **Garage's key-format requirement.** `garage key import` (v1.0.1) only
  accepts an access key of the form `GK` + 24 hex chars and a 64-hex-char
  secret key; `.env.example` documents this so operators do not hit an
  init failure from a freely-chosen credential string.
- **Media is still public-by-URL, unchanged.** The api's `/media/*key`
  handler is unauthenticated, matching the prior `file_server`/`Static`
  behavior; the PDPA gate stays at the API (URLs for unapproved/unconsented
  records are never emitted).
