#!/bin/sh
# Deploy (or roll back to) a released version of the compose stack. Run on the
# host, from the stack checkout dir. Health-gated with automatic rollback.
set -eu

VERSION=${1:-}
DIR=$(dirname "$0")
. "$DIR/deploy-lib.sh"

ENV_FILE=.env

validate_version "$VERSION" || exit 1

prev=$(read_tag "$ENV_FILE")
[ -n "$prev" ] || prev=latest
echo "deploy: current=$prev target=$VERSION"

# deploy_version V: check out V's compose topology (when V is a real version),
# point APP_IMAGE_TAG at V, pull the images, and bring the stack up.
deploy_version() {
	v=$1
	if validate_version "$v" 2>/dev/null; then
		git fetch --tags --quiet || return 1
		git checkout --quiet "v$v" || return 1
	fi
	write_tag "$ENV_FILE" "$v" || return 1
	docker compose pull || return 1
	docker compose up -d --wait || return 1
	return 0
}

# health_ok: poll the public health endpoint until it reports ok, or time out.
health_ok() {
	domain=$(sed -n 's/^APP_DOMAIN=//p' "$ENV_FILE" | tail -n1)
	i=0
	while [ "$i" -lt 30 ]; do
		if curl -fsS "https://$domain/api/health" 2>/dev/null | grep -q '"status":"ok"'; then
			return 0
		fi
		i=$((i + 1))
		sleep 5
	done
	return 1
}

if deploy_version "$VERSION" && health_ok; then
	echo "deploy: $VERSION healthy"
	exit 0
fi

echo "deploy: $VERSION FAILED — rolling back to $prev" >&2
deploy_version "$prev" || true
if health_ok; then
	echo "deploy: rolled back to $prev (healthy)" >&2
else
	echo "deploy: rollback to $prev also unhealthy — manual intervention needed" >&2
fi
exit 1
