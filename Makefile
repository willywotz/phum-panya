.PHONY: web build test dev
web:
	cd web && npm ci && npm run build
	find internal/webui/dist -mindepth 1 ! -name index.html -delete 2>/dev/null || true
	cp -r web/out/. internal/webui/dist/
build: web
	CGO_ENABLED=0 go build -o server ./cmd/server
test:
	go test ./...
# Dev stack: web + api behind nginx with hot reload (needs APP_ADMIN_PASSWORD, e.g. from .env).
dev:
	docker compose -f docker-compose.dev.yaml up -w --build --force-recreate
