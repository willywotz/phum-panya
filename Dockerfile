# syntax=docker/dockerfile:1
#
# Multi-stage build for phum-panya: one cgo-free Go binary with the Next.js
# static export embedded. Node is a build-time dependency only; the runtime
# image is a single static binary plus CA certificates.

# --- Stage 1: build the Next.js static export (web/out) ---
FROM node:24-alpine AS web
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# --- Stage 2: build the cgo-free Go binary with the UI embedded ---
FROM golang:1.26-alpine AS build
WORKDIR /app
# Download modules first so they cache across source changes.
COPY go.mod go.sum ./
RUN go mod download
# Copy the Go source (web/ and the tracked dist placeholder are excluded by
# .dockerignore; the embedded assets come from the web stage below).
COPY . .
# Place the freshly built static export where //go:embed all:dist expects it.
COPY --from=web /app/web/out/ ./internal/webui/dist/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /server ./cmd/server

# --- Stage 3: minimal runtime ---
FROM alpine:3.20 AS runtime
# ca-certificates: needed for the built-in ACME/Let's Encrypt TLS client.
# tzdata: correct local time in logs/timestamps. wget (in busybox) powers the healthcheck.
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -u 10001 app \
 && mkdir -p /data \
 && chown -R app:app /data

COPY --from=build /server /usr/local/bin/server

# All mutable state (SQLite DB, uploaded media, nightly backups, ACME certs)
# lives under /data, the single persisted volume. The ACME cache path
# "data/certs" is relative to the working directory, so keep WORKDIR at / so it
# resolves to /data/certs inside the volume.
WORKDIR /
ENV APP_DB_PATH=/data/app.db \
    APP_MEDIA_DIR=/data/media \
    APP_BACKUP_DIR=/data/backup
VOLUME ["/data"]
USER app

# 8080 = plain HTTP (APP_DEV=1). 80/443 = production TLS via built-in ACME.
EXPOSE 8080 80 443

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/api/health || exit 1

ENTRYPOINT ["server"]
