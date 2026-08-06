# Pure helpers for deploy.sh. No docker/git/ssh side effects.

# validate_version V -> 0 iff V is N.N.N; else print an error and return 1.
validate_version() {
	if printf '%s' "$1" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
		return 0
	fi
	echo "deploy: invalid version '$1' (want N.N.N)" >&2
	return 1
}

# read_tag ENVFILE -> print the value of the last APP_IMAGE_TAG= line, or nothing.
read_tag() {
	sed -n 's/^APP_IMAGE_TAG=//p' "$1" 2>/dev/null | tail -n1
}

# write_tag ENVFILE V -> set APP_IMAGE_TAG=V in ENVFILE (replace or append),
# leaving all other lines unchanged.
write_tag() {
	envfile=$1
	value=$2
	if grep -q '^APP_IMAGE_TAG=' "$envfile" 2>/dev/null; then
		sed "s|^APP_IMAGE_TAG=.*|APP_IMAGE_TAG=$value|" "$envfile" > "$envfile.tmp" &&
			mv "$envfile.tmp" "$envfile"
	else
		printf 'APP_IMAGE_TAG=%s\n' "$value" >> "$envfile"
	fi
}
