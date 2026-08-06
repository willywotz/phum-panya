# Pure helpers for media-backup.sh. No rclone, no date calls: time is injected.

# archive_path NOW_EPOCH ISO -> prints "archive/<NOW_EPOCH>-<ISO>".
archive_path() {
	printf 'archive/%s-%s\n' "$1" "$2"
}

# prune_plan KEEP_SECONDS NOW_EPOCH NAME... -> prints each NAME whose leading
# epoch (text before the first '-') is strictly older than NOW_EPOCH-KEEP_SECONDS.
prune_plan() {
	keep_seconds=$1
	now_epoch=$2
	shift 2
	cutoff=$((now_epoch - keep_seconds))
	for name in "$@"; do
		epoch=${name%%-*}
		case $epoch in
			'' | *[!0-9]*) continue ;;
		esac
		if [ "$epoch" -lt "$cutoff" ]; then
			printf '%s\n' "$name"
		fi
	done
	return 0
}

# require_env NAME... -> returns 1 and names the first unset/empty variable.
require_env() {
	for name in "$@"; do
		eval "val=\${$name:-}"
		if [ -z "$val" ]; then
			printf 'media-backup: missing required env %s\n' "$name" >&2
			return 1
		fi
	done
	return 0
}
