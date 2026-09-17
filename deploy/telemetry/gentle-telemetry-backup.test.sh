#!/usr/bin/env bash
# Shell-level test for deploy/telemetry/gentle-telemetry-backup: runs the
# real script against a temp SQLite file with a stubbed `rclone` on PATH,
# and asserts it took a `VACUUM INTO` snapshot, "uploaded" it, and cleaned
# up. Cases 2-4 stub `systemctl` as reporting victoria-metrics.service
# active and run a fake VictoriaMetrics HTTP server on an ephemeral port
# (passed to the script via GENTLE_TELEMETRY_VM_URL, so no case ever talks
# to a real VictoriaMetrics): case 2 asserts the script archives and
# uploads a VM snapshot, then deletes it upstream, leaving nothing behind;
# case 3 asserts a failed VM archive upload still deletes the upstream
# snapshot; case 4 asserts a malicious snapshot name from VictoriaMetrics
# is rejected before it is ever used in a tar or delete call. Requires
# `sqlite3`; cases 2-4 additionally require `python3` and are skipped
# without it. Run manually (not part of `go test`):
#   ./deploy/telemetry/gentle-telemetry-backup.test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="${SCRIPT_DIR}/gentle-telemetry-backup"

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

# --- Cases 2-4: VictoriaMetrics also installed and running ---

if ! command -v python3 >/dev/null 2>&1; then
	printf 'skip: python3 not found on PATH, cannot fake the VictoriaMetrics HTTP API\n'
	exit 0
fi

tmp2="" tmp3="" tmp4=""
vm_pids=()
cleanup_vm_cases() {
	local pid
	for pid in "${vm_pids[@]}"; do
		kill "${pid}" 2>/dev/null || true
	done
	rm -rf "${tmp}" "${tmp2}" "${tmp3}" "${tmp4}" "${vm_cases_tmp}"
}
vm_cases_tmp="$(mktemp -d)"
trap cleanup_vm_cases EXIT

fake_vm_server_py="${vm_cases_tmp}/fake_vm_server.py"
cat >"${fake_vm_server_py}" <<'PYEOF'
import http.server
import sys

log_path, snapshot, port_file = sys.argv[1], sys.argv[2], sys.argv[3]


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


server = http.server.HTTPServer(("127.0.0.1", 0), Handler)
with open(port_file, "w") as f:
	f.write(str(server.server_address[1]))
server.serve_forever()
PYEOF

# write_fake_systemctl <dir>: a stub reporting victoria-metrics.service
# active, the only call the script makes (`systemctl is-active --quiet
# <unit>`).
write_fake_systemctl() {
	cat >"$1/systemctl" <<'EOF'
#!/usr/bin/env bash
if [[ "$1" == "is-active" && "$3" == "victoria-metrics.service" ]]; then
	exit 0
fi
exit 1
EOF
	chmod +x "$1/systemctl"
}

# start_fake_vm_server <log_path> <snapshot_name> <port_file>: launches the
# fake VictoriaMetrics HTTP API on an ephemeral port, waits for it to
# report that port and become reachable, and sets the `vm_url` global to
# its base URL.
start_fake_vm_server() {
	local log_path="$1" snapshot_name="$2" port_file="$3" port
	python3 "${fake_vm_server_py}" "${log_path}" "${snapshot_name}" "${port_file}" &
	vm_pids+=("$!")
	for _ in $(seq 1 50); do
		[[ -s "${port_file}" ]] && break
		sleep 0.1
	done
	[[ -s "${port_file}" ]] || fail "fake VictoriaMetrics server never reported its port"
	port="$(cat "${port_file}")"
	vm_url="http://127.0.0.1:${port}"
	for _ in $(seq 1 50); do
		curl -s -o /dev/null "${vm_url}/" && break
		sleep 0.1
	done
}

snapshot_name="20260101120000-0000000000000001"

# --- Case 2: VictoriaMetrics snapshot archived, uploaded, and deleted upstream ---

tmp2="$(mktemp -d)"
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
write_fake_systemctl "${fakebin2}"

vm_dir2="${tmp2}/victoria-metrics"
mkdir -p "${vm_dir2}/snapshots/${snapshot_name}"
echo "fake series data" >"${vm_dir2}/snapshots/${snapshot_name}/data.bin"

vm_requests_log2="${tmp2}/vm-requests.log"
start_fake_vm_server "${vm_requests_log2}" "${snapshot_name}" "${tmp2}/vm-port"
vm_url2="${vm_url}"

GENTLE_TELEMETRY_DB="${db2}" \
	GENTLE_TELEMETRY_BACKUP_REMOTE="fake-remote:bucket/path" \
	GENTLE_TELEMETRY_VM_DIR="${vm_dir2}" \
	GENTLE_TELEMETRY_VM_URL="${vm_url2}" \
	PATH="${fakebin2}:${PATH}" \
	"${TARGET}"

[[ -f "${rclone_log2}" ]] || fail "rclone was never invoked (case 2)"
grep -q 'vm-.*\.tar\.gz' "${rclone_log2}" || fail "vm archive was never uploaded: $(cat "${rclone_log2}")"
grep -q 'backup-.*\.sqlite' "${rclone_log2}" || fail "sqlite snapshot was never uploaded: $(cat "${rclone_log2}")"

[[ -f "${vm_requests_log2}" ]] || fail "VictoriaMetrics HTTP API was never called"
grep -qx "/snapshot/create" "${vm_requests_log2}" || fail "snapshot/create was never called"
grep -qx "/snapshot/delete?snapshot=${snapshot_name}" "${vm_requests_log2}" || fail "snapshot/delete was never called with the right name"

compgen -G "${tmp2}/vm-*.tar.gz" >/dev/null && fail "vm archive was not cleaned up"
compgen -G "${tmp2}/backup-*.sqlite" >/dev/null && fail "sqlite snapshot was not cleaned up"

printf 'PASS: VictoriaMetrics snapshot archived, uploaded, and deleted upstream\n'

# --- Case 3: a failed VM archive upload still deletes the upstream snapshot ---

tmp3="$(mktemp -d)"
db3="${tmp3}/events.sqlite"
sqlite3 "${db3}" "CREATE TABLE events (id INTEGER PRIMARY KEY); INSERT INTO events DEFAULT VALUES;"

fakebin3="${tmp3}/fakebin"
mkdir -p "${fakebin3}"
rclone_log3="${tmp3}/rclone.log"
# Succeeds for the sqlite upload, fails for the vm archive upload, mirroring
# an rclone error partway through the run.
cat >"${fakebin3}/rclone" <<EOF
#!/usr/bin/env bash
echo "\$@" >>"${rclone_log3}"
case "\$2" in
	*vm-*.tar.gz) exit 1 ;;
esac
EOF
chmod +x "${fakebin3}/rclone"
write_fake_systemctl "${fakebin3}"

vm_dir3="${tmp3}/victoria-metrics"
mkdir -p "${vm_dir3}/snapshots/${snapshot_name}"
echo "fake series data" >"${vm_dir3}/snapshots/${snapshot_name}/data.bin"

vm_requests_log3="${tmp3}/vm-requests.log"
start_fake_vm_server "${vm_requests_log3}" "${snapshot_name}" "${tmp3}/vm-port"
vm_url3="${vm_url}"

if GENTLE_TELEMETRY_DB="${db3}" \
	GENTLE_TELEMETRY_BACKUP_REMOTE="fake-remote:bucket/path" \
	GENTLE_TELEMETRY_VM_DIR="${vm_dir3}" \
	GENTLE_TELEMETRY_VM_URL="${vm_url3}" \
	PATH="${fakebin3}:${PATH}" \
	"${TARGET}"; then
	fail "script did not fail when the vm archive upload failed"
fi

grep -qx "/snapshot/delete?snapshot=${snapshot_name}" "${vm_requests_log3}" ||
	fail "snapshot/delete was not called after a failed upload: $(cat "${vm_requests_log3}")"

printf 'PASS: a failed VictoriaMetrics archive upload still deletes the upstream snapshot\n'

# --- Case 4: a malicious snapshot name is rejected before tar or delete ---

tmp4="$(mktemp -d)"
db4="${tmp4}/events.sqlite"
sqlite3 "${db4}" "CREATE TABLE events (id INTEGER PRIMARY KEY); INSERT INTO events DEFAULT VALUES;"

fakebin4="${tmp4}/fakebin"
mkdir -p "${fakebin4}"
rclone_log4="${tmp4}/rclone.log"
cat >"${fakebin4}/rclone" <<EOF
#!/usr/bin/env bash
echo "\$@" >>"${rclone_log4}"
EOF
chmod +x "${fakebin4}/rclone"
write_fake_systemctl "${fakebin4}"

vm_dir4="${tmp4}/victoria-metrics"
mkdir -p "${vm_dir4}/snapshots"
malicious_name="../../etc"
vm_requests_log4="${tmp4}/vm-requests.log"
start_fake_vm_server "${vm_requests_log4}" "${malicious_name}" "${tmp4}/vm-port"
vm_url4="${vm_url}"

output4="$(GENTLE_TELEMETRY_DB="${db4}" \
	GENTLE_TELEMETRY_BACKUP_REMOTE="fake-remote:bucket/path" \
	GENTLE_TELEMETRY_VM_DIR="${vm_dir4}" \
	GENTLE_TELEMETRY_VM_URL="${vm_url4}" \
	PATH="${fakebin4}:${PATH}" \
	"${TARGET}" 2>&1)" && fail "script did not fail on an unexpected snapshot name"

printf '%s' "${output4}" | grep -q 'unexpected snapshot name' ||
	fail "script did not report the unexpected snapshot name: ${output4}"

grep -qx "/snapshot/delete?snapshot=${malicious_name}" "${vm_requests_log4}" &&
	fail "snapshot/delete was called for a rejected snapshot name"
compgen -G "${tmp4}/vm-*.tar.gz" >/dev/null && fail "a vm archive was created for a rejected snapshot name"

printf 'PASS: a malicious snapshot name from VictoriaMetrics is rejected\n'
