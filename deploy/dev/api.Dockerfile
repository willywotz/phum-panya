# Dev image for the Go API (docker-compose.dev.yaml + `docker compose watch`).
# air + modules are baked in; watch syncs *.go changes and air rebuilds inside
# the container, and rebuilds this image when go.mod changes.
FROM golang:1.26-alpine
WORKDIR /app
RUN go install github.com/air-verse/air@v1.67.4
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# .dockerignore drops the embedded UI placeholder; recreate a stub so the
# //go:embed all:dist in internal/webui compiles. nginx serves pages in dev,
# not the Go server, so the content is irrelevant.
RUN mkdir -p internal/webui/dist \
 && printf '<!doctype html><title>phum-panya dev</title><div id="app"></div>' \
      > internal/webui/dist/index.html
EXPOSE 8080
CMD ["air", "-c", ".air.toml"]
