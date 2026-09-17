#!/usr/bin/env bash
# Shell-level test for deploy/telemetry/axiom-telemetry-backup: runs the
# real script against a temp SQLite file with a stubbed `rclone` on PATH,
# and asserts it took a `VACUUM INTO` snapshot, "uploaded" it, and cleaned
# up. A second case stubs `systemctl` as reporting victoria-metrics.service
# active and runs a fake VictoriaMetrics HTTP server, asserting the script
# also archives and uploads a VM snapshot, then deletes it upstream, and
# that neither run leaves anything behind. Requires `sqlite3`; the second
# case additionally requires `python3` and is skipped without it. Run
# manually (not part of `go test`):
#   ./deploy/telemetry/axiom-telemetry-backup.test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="${SCRIPT_DIR}/axiom-telemetry-backup"

if ! command -v sqlite3 >/dev/null 2>&1; then
	printf 'skip: sqlite3 not found on PATH\n'
	exit 0
fi

fail() {
	printf 'FAIL: %s\n' "$1" >&2
	exit 1
}

# --- Case 1: SQLite-only backup (VictoriaMetrics not installed) ---

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

[[ -f "${rclone_log}" ]] || fail "rclone was never invoked"

logged="$(cat "${rclone_log}")"
[[ "${logged}" == copy\ "${tmp}"/backup-*.sqlite\ fake-remote:bucket/path ]] ||
	fail "unexpected rclone invocation: ${logged}"

compgen -G "${tmp}/backup-*.sqlite" >/dev/null && fail "sqlite snapshot was not cleaned up"
compgen -G "${tmp}/vm-*.tar.gz" >/dev/null && fail "a vm archive was created with no VictoriaMetrics installed"

printf 'PASS: sqlite-only backup (VACUUM INTO) uploaded and cleaned up\n'

# --- Case 2: VictoriaMetrics also installed and running ---

if ! command -v python3 >/dev/null 2>&1; then
	printf 'skip: python3 not found on PATH, cannot fake the VictoriaMetrics HTTP API\n'
	exit 0
fi

tmp2="$(mktemp -d)"
trap 'rm -rf "${tmp}" "${tmp2}"' EXIT

db2="${tmp2}/events.sqlite"
sqlite3 "${db2}" "CREATE TABLE events (id INTEGER PRIMARY KEY); INSERT INTO events DEFAULT VALUES;"

fakebin2="${tmp2}/fakebin"
mkdir -p "${fakebin2}"
rclone_log2="${tmp2}/rclone.log"
cat >"${fakebin2}/rclone" <<EOF
#!/usr/bin/env bash
echo "\$@" >>"${rclone_log2}"
EOF
chmod +x "${fakebin2}/rclone"

# Stub systemctl to report victoria-metrics.service active. The script
# only ever calls it as `systemctl is-active --quiet <unit>`.
cat >"${fakebin2}/systemctl" <<'EOF'
#!/usr/bin/env bash
if [[ "$1" == "is-active" && "$3" == "victoria-metrics.service" ]]; then
	exit 0
fi
exit 1
EOF
chmod +x "${fakebin2}/systemctl"

vm_dir="${tmp2}/victoria-metrics"
snapshot_name="20260101120000-0000000000000001"
mkdir -p "${vm_dir}/snapshots/${snapshot_name}"
echo "fake series data" >"${vm_dir}/snapshots/${snapshot_name}/data.bin"

vm_requests_log="${tmp2}/vm-requests.log"

cat >"${tmp2}/fake_vm_server.py" <<'PYEOF'
import http.server
import sys

log_path, snapshot = sys.argv[1], sys.argv[2]


class Handler(http.server.BaseHTTPRequestHandler):
	def log_message(self, *args):
		pass

	def _json(self, body):
		encoded = body.encode()
		self.send_response(200)
		self.send_header("Content-Type", "application/json")
		self.send_header("Content-Length", str(len(encoded)))
		self.end_headers()
		self.wfile.write(encoded)

	def do_GET(self):
		self.send_response(200)
		self.end_headers()

	def do_POST(self):
		with open(log_path, "a") as f:
			f.write(self.path + "\n")
		if self.path == "/snapshot/create":
			self._json('{"status":"ok","snapshot":"' + snapshot + '"}')
		elif self.path.startswith("/snapshot/delete"):
			self._json('{"status":"ok"}')
		else:
			self.send_response(404)
			self.end_headers()


http.server.HTTPServer(("127.0.0.1", 8428), Handler).serve_forever()
PYEOF

python3 "${tmp2}/fake_vm_server.py" "${vm_requests_log}" "${snapshot_name}" &
vm_server_pid=$!
trap 'kill "${vm_server_pid}" 2>/dev/null; rm -rf "${tmp}" "${tmp2}"' EXIT

# Wait for the fake server to start accepting connections before running
# the script against it, without exercising /snapshot/create ourselves
# (that would leave a spurious line in vm_requests_log).
for _ in $(seq 1 50); do
	curl -s -o /dev/null "http://127.0.0.1:8428/" && break
	sleep 0.1
done

GENTLE_TELEMETRY_DB="${db2}" \
	GENTLE_TELEMETRY_BACKUP_REMOTE="fake-remote:bucket/path" \
	GENTLE_TELEMETRY_VM_DIR="${vm_dir}" \
	PATH="${fakebin2}:${PATH}" \
	"${TARGET}"

[[ -f "${rclone_log2}" ]] || fail "rclone was never invoked (case 2)"
grep -q 'vm-.*\.tar\.gz' "${rclone_log2}" || fail "vm archive was never uploaded: $(cat "${rclone_log2}")"
grep -q 'backup-.*\.sqlite' "${rclone_log2}" || fail "sqlite snapshot was never uploaded: $(cat "${rclone_log2}")"

[[ -f "${vm_requests_log}" ]] || fail "VictoriaMetrics HTTP API was never called"
grep -qx "/snapshot/create" "${vm_requests_log}" || fail "snapshot/create was never called"
grep -qx "/snapshot/delete?snapshot=${snapshot_name}" "${vm_requests_log}" || fail "snapshot/delete was never called with the right name"

compgen -G "${tmp2}/vm-*.tar.gz" >/dev/null && fail "vm archive was not cleaned up"
compgen -G "${tmp2}/backup-*.sqlite" >/dev/null && fail "sqlite snapshot was not cleaned up"

printf 'PASS: VictoriaMetrics snapshot archived, uploaded, and deleted upstream\n'
