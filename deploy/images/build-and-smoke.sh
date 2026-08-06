#!/bin/sh
# Build the compose stack's api + web images and smoke-test them. No push.
set -eu

API_IMAGE="${API_IMAGE:-phum-panya-api:ci}"
WEB_IMAGE="${WEB_IMAGE:-phum-panya-web:ci}"

echo "build: $API_IMAGE"
docker build -f Dockerfile.api -t "$API_IMAGE" .
echo "build: $WEB_IMAGE"
docker build -f web/Dockerfile -t "$WEB_IMAGE" web

cleanup() { docker rm -f api-smoke web-smoke >/dev/null 2>&1 || true; }
trap cleanup EXIT

# api: boot in dev mode (SQLite, HTTP :8080, AutoMigrate), wait for the image's
# built-in HEALTHCHECK to report healthy.
echo "smoke: api"
docker run -d --name api-smoke -e APP_DEV=1 "$API_IMAGE" >/dev/null
status=starting
i=0
while [ "$i" -lt 30 ]; do
	status=$(docker inspect -f '{{.State.Health.Status}}' api-smoke 2>/dev/null || echo starting)
	[ "$status" = healthy ] && break
	if [ "$status" = unhealthy ]; then
		echo "api unhealthy"; docker logs api-smoke; exit 1
	fi
	i=$((i + 1))
	sleep 2
done
[ "$status" = healthy ] || { echo "api never healthy"; docker logs api-smoke; exit 1; }
echo "api healthy"

# web: standalone Next.js server on :3000; wait for an HTTP response.
echo "smoke: web"
docker run -d --name web-smoke -p 3000:3000 "$WEB_IMAGE" >/dev/null
ok=
i=0
while [ "$i" -lt 30 ]; do
	if curl -fsSL -o /dev/null http://localhost:3000/; then ok=1; break; fi
	i=$((i + 1))
	sleep 2
done
[ "$ok" = 1 ] || { echo "web never responded"; docker logs web-smoke; exit 1; }
echo "web responding"

echo "smoke OK"
