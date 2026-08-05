.PHONY: web build build-release test dev
web:
	cd web && npm ci && npm run build
	find internal/webui/dist -mindepth 1 ! -name index.html -delete 2>/dev/null || true
	cp -r web/out/. internal/webui/dist/
build: web
	CGO_ENABLED=0 go build -o server ./cmd/server
# Cross-compiled release binaries (single cgo-free file each; embeds the UI).
# The Windows .exe runs as a service via `server.exe service install`.
build-release: web
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -o bin/server-linux-amd64 ./cmd/server
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o bin/server.exe          ./cmd/server
test:
	go test ./...
# Dev stack: prod-parity Caddy + Postgres + behind-proxy api, web/api hot reload (needs APP_ADMIN_PASSWORD, e.g. from .env).
dev:
	docker compose -f docker-compose.dev.yaml up -w --build --force-recreate
