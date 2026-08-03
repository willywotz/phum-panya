.PHONY: web build test
web:
	cd web && npm ci && npm run build
	find internal/webui/dist -mindepth 1 ! -name index.html -delete 2>/dev/null || true
	cp -r web/out/. internal/webui/dist/
build: web
	CGO_ENABLED=0 go build -o server ./cmd/server
test:
	go test ./...
