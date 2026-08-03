# Dev reverse proxy image (docker-compose.dev.yaml + `docker compose watch`).
# The conf is baked in; watch re-syncs and reloads it when dev.conf changes.
FROM nginx:alpine
COPY deploy/nginx/dev.conf /etc/nginx/conf.d/default.conf
