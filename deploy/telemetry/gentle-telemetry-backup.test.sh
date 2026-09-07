#!/usr/bin/env bash
# Shell-level test for deploy/telemetry/gentle-telemetry-backup: runs the
# real script against a temp SQLite file with a stubbed `rclone` on PATH,
# and asserts it took a snapshot, "uploaded" it, and cleaned up. Requires
# `sqlite3`; run manually (not part of `go test`):
#   ./deploy/telemetry/gentle-telemetry-backup.test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="${SCRIPT_DIR}/gentle-telemetry-backup"

if ! command -v sqlite3 >/dev/null 2>&1; then
	printf 'skip: sqlite3 not found on PATH\n'
	exit 0
fi

tmp="$(mktemp -d)"
trap 'rm -rf "${tmp}"' EXIT

db="${tmp}/events.sqlite"
sqlite3 "${db}" "CREATE TABLE events (id INTEGER PRIMARY KEY); INSERT INTO events DEFAULT VALUES;"

fakebin="${tmp}/fakebin"
mkdir -p "${fakebin}"
rclone_log="${tmp}/rclone.log"
cat >"${fakebin}/rclone" <<EOF
#!/usr/bin/env bash
echo "\$@" >>"${rclone_log}"
EOF
chmod +x "${fakebin}/rclone"

GENTLE_TELEMETRY_DB="${db}" \
	GENTLE_TELEMETRY_BACKUP_REMOTE="fake-remote:bucket/path" \
	PATH="${fakebin}:${PATH}" \
	"${TARGET}"

if [[ ! -f "${rclone_log}" ]]; then
	printf 'FAIL: rclone was never invoked\n' >&2
	exit 1
fi

logged="$(cat "${rclone_log}")"
if [[ "${logged}" != copy\ "${tmp}"/backup-*.sqlite\ fake-remote:bucket/path ]]; then
	printf 'FAIL: unexpected rclone invocation: %s\n' "${logged}" >&2
	exit 1
fi

if compgen -G "${tmp}/backup-*.sqlite" >/dev/null; then
	printf 'FAIL: snapshot file was not cleaned up\n' >&2
	exit 1
fi

printf 'PASS: gentle-telemetry-backup took a snapshot, "uploaded" it, and cleaned up\n'
