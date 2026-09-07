#!/usr/bin/env bash
# Installs and starts the gentle-telemetry collector on an AlmaLinux/RHEL 9
# VPS (cPanel/WHM + Apache, not Caddy): the binary, systemd units (service,
# backup service, backup timer), and — with --domain — a rendered Apache
# vhost template for the operator to append by hand.
#
# Subdomains on the reference server are explicit <VirtualHost> blocks
# appended to /etc/apache2/conf.d/includes/post_virtualhost_global.conf,
# not cPanel accounts — an EasyApache "userdata" include is never loaded
# for them. This script never edits that file itself; see the printed
# steps at the end and docs/telemetry-collector.md.
#
# Usage:
#   sudo ./install.sh --release-tag v0.1.0
#   sudo ./install.sh --local-source /path/to/gentle-ai/checkout
#   sudo ./install.sh --local-source /path/to/gentle-ai/checkout \
#       --domain telemetry.example.com --with-grafana
#
# gentle-telemetry (cmd/gentle-telemetry) is not yet wired into
# .goreleaser.yaml — see docs/telemetry-collector.md for why — so there is
# no published release asset today. Use --local-source to build from a
# checkout until that changes; --release-tag is kept for when it does.
#
# This script never touches the live Apache configuration beyond rendering
# the vhost template to a file under /root: it does not append to
# post_virtualhost_global.conf, run configtest, reload httpd, or issue or
# renew a TLS certificate. Those stay explicit operator steps, printed at
# the end in the order they must run.
#
# --with-grafana additionally installs Grafana OSS (free, self-hosted) on
# this same VPS with a read-only dashboard over the collector's database;
# see docs/telemetry-collector.md#grafana-dashboards.
set -euo pipefail

REPO="Gentleman-Programming/gentle-ai"
RELEASE_TAG=""
LOCAL_SOURCE=""
WITH_GRAFANA="false"
DOMAIN=""
BIN_DEST="/usr/local/bin/gentle-telemetry"
UNIT_DIR="/etc/systemd/system"
CONFIG_DIR="/etc/gentle-telemetry"
STATE_DIR="/var/lib/gentle-telemetry"
APACHE_INCLUDE_FILE="/etc/apache2/conf.d/includes/post_virtualhost_global.conf"
RENDERED_VHOST="/root/telemetry-vhost.conf.rendered"
GRAFANA_INI="/etc/grafana/grafana.ini"
GRAFANA_PROVISIONING_DIR="/etc/grafana/provisioning"
GRAFANA_DASHBOARD_DIR="${GRAFANA_PROVISIONING_DIR}/dashboards/gentle-ai"
GRAFANA_ADMIN_PASSWORD_FILE="/etc/grafana/admin-password"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

usage() {
	cat >&2 <<EOF
Usage: $0 (--release-tag <tag> | --local-source <path>)
          [--domain <fqdn>] [--with-grafana]
EOF
	exit 1
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	--release-tag)
		RELEASE_TAG="$2"
		shift 2
		;;
	--local-source)
		LOCAL_SOURCE="$2"
		shift 2
		;;
	--domain)
		DOMAIN="$2"
		shift 2
		;;
	--with-grafana)
		WITH_GRAFANA="true"
		shift
		;;
	-h | --help)
		usage
		;;
	*)
		printf 'unknown argument: %s\n' "$1" >&2
		usage
		;;
	esac
done

if [[ -z "${RELEASE_TAG}" && -z "${LOCAL_SOURCE}" ]]; then
	usage
fi

if [[ "$(id -u)" -ne 0 ]]; then
	printf 'this script must run as root (use sudo)\n' >&2
	exit 1
fi

arch="$(uname -m)"
case "${arch}" in
amd64 | x86_64) goarch="amd64" ;;
arm64 | aarch64) goarch="arm64" ;;
*)
	printf 'unsupported architecture: %s\n' "${arch}" >&2
	exit 1
	;;
esac

install_from_release() {
	local tag="$1"
	local asset="gentle-telemetry_${tag#v}_linux_${goarch}.tar.gz"
	local url="https://github.com/${REPO}/releases/download/${tag}/${asset}"
	local tmp
	tmp="$(mktemp -d)"
	trap 'rm -rf "${tmp}"' RETURN

	printf 'downloading %s\n' "${url}"
	curl -fsSL "${url}" -o "${tmp}/${asset}"
	tar -xzf "${tmp}/${asset}" -C "${tmp}"
	install -m 0755 "${tmp}/gentle-telemetry" "${BIN_DEST}"
}

install_from_source() {
	local src="$1"
	if ! command -v go >/dev/null 2>&1; then
		printf 'go toolchain not found; install Go or use --release-tag instead\n' >&2
		exit 1
	fi
	printf 'building gentle-telemetry from %s\n' "${src}"
	(cd "${src}" && CGO_ENABLED=0 GOARCH="${goarch}" GOOS=linux go build -o "${BIN_DEST}.new" ./cmd/gentle-telemetry)
	mv "${BIN_DEST}.new" "${BIN_DEST}"
	chmod 0755 "${BIN_DEST}"
}

if [[ -n "${RELEASE_TAG}" ]]; then
	install_from_release "${RELEASE_TAG}"
else
	install_from_source "${LOCAL_SOURCE}"
fi

# sqlite3 (the CLI, not a Go dependency: the binary itself is cgo-free and
# self-contained) is required by gentle-telemetry-backup.service's
# `sqlite3 .backup` snapshot step.
if ! command -v sqlite3 >/dev/null 2>&1; then
	dnf install -y sqlite
fi

if ! command -v rclone >/dev/null 2>&1; then
	dnf install -y rclone 2>/dev/null || curl -fsS https://rclone.org/install.sh | bash
fi

mkdir -p "${CONFIG_DIR}" "${STATE_DIR}"
chmod 0755 "${CONFIG_DIR}"

if [[ ! -f "${CONFIG_DIR}/summary.token" ]]; then
	printf 'generating a new /v1/summary bearer token at %s/summary.token\n' "${CONFIG_DIR}"
	umask 0177
	if command -v openssl >/dev/null 2>&1; then
		openssl rand -hex 32 >"${CONFIG_DIR}/summary.token"
	else
		head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n' >"${CONFIG_DIR}/summary.token"
	fi
	umask 0022
else
	printf '%s/summary.token already exists; leaving it in place (see the rotation steps in docs/telemetry-collector.md)\n' "${CONFIG_DIR}"
fi

if [[ ! -f "${CONFIG_DIR}/backup.env" ]]; then
	cat >"${CONFIG_DIR}/backup.env" <<'EOF'
# Configure the rclone remote:path the nightly backup uploads to, e.g.:
# GENTLE_TELEMETRY_BACKUP_REMOTE=b2:my-bucket/gentle-telemetry
GENTLE_TELEMETRY_BACKUP_REMOTE=
EOF
	printf 'wrote a placeholder %s/backup.env - set GENTLE_TELEMETRY_BACKUP_REMOTE before enabling the backup timer\n' "${CONFIG_DIR}"
fi

install -m 0644 "${SCRIPT_DIR}/gentle-telemetry.service" "${UNIT_DIR}/gentle-telemetry.service"
install -m 0755 "${SCRIPT_DIR}/gentle-telemetry-backup" /usr/local/bin/gentle-telemetry-backup
install -m 0644 "${SCRIPT_DIR}/gentle-telemetry-backup.service" "${UNIT_DIR}/gentle-telemetry-backup.service"
install -m 0644 "${SCRIPT_DIR}/gentle-telemetry-backup.timer" "${UNIT_DIR}/gentle-telemetry-backup.timer"

systemctl daemon-reload
systemctl enable --now gentle-telemetry.service

if grep -q '^GENTLE_TELEMETRY_BACKUP_REMOTE=.\+' "${CONFIG_DIR}/backup.env"; then
	systemctl enable --now gentle-telemetry-backup.timer
else
	printf 'skipping gentle-telemetry-backup.timer: set GENTLE_TELEMETRY_BACKUP_REMOTE in %s/backup.env, then run:\n' "${CONFIG_DIR}"
	printf '  systemctl enable --now gentle-telemetry-backup.timer\n'
fi

# set_ini_kv upserts key = value under [section] in an ini file, creating
# the section if it does not exist yet. Used for grafana.ini below, since
# it ships with these settings commented out rather than absent.
set_ini_kv() {
	local file="$1" section="$2" key="$3" value="$4"
	if ! grep -q "^\[${section}\]" "${file}" 2>/dev/null; then
		printf '\n[%s]\n%s = %s\n' "${section}" "${key}" "${value}" >>"${file}"
		return
	fi
	awk -v section="${section}" -v key="${key}" -v value="${value}" '
		BEGIN { in_section = 0; done = 0 }
		/^\[/ {
			if (in_section && !done) { print key " = " value; done = 1 }
			in_section = ($0 == "[" section "]")
			print
			next
		}
		{
			if (in_section && $0 ~ "^" key " *=") {
				print key " = " value
				done = 1
				next
			}
			print
		}
		END { if (in_section && !done) print key " = " value }
	' "${file}" >"${file}.tmp"
	# The redirection above creates ${file}.tmp fresh under the script's
	# own umask, and mv would otherwise replace ${file}'s inode with that
	# looser mode/ownership — silently widening access to a file that can
	# hold the Grafana admin password. Carry over the original file's mode
	# and owner first, falling back to a restrictive default if it somehow
	# does not exist yet.
	if [[ -e "${file}" ]]; then
		chmod --reference="${file}" "${file}.tmp"
		chown --reference="${file}" "${file}.tmp"
	else
		chmod 0640 "${file}.tmp"
		chown root:grafana "${file}.tmp" 2>/dev/null || true
	fi
	mv "${file}.tmp" "${file}"
}

grafana_cli() {
	if command -v grafana >/dev/null 2>&1; then
		grafana cli --homepath /usr/share/grafana "$@"
	else
		grafana-cli --homepath /usr/share/grafana "$@"
	fi
}

install_grafana() {
	if ! command -v grafana-server >/dev/null 2>&1; then
		printf 'installing Grafana OSS from the official rpm.grafana.com repository\n'
		cat >/etc/yum.repos.d/grafana.repo <<'EOF'
[grafana]
name=grafana
baseurl=https://rpm.grafana.com
repo_gpgcheck=1
enabled=1
gpgcheck=1
gpgkey=https://rpm.grafana.com/gpg.key
sslverify=1
sslcacert=/etc/pki/tls/certs/ca-bundle.crt
EOF
		dnf install -y grafana
	fi

	# The packaged CLI needs the homepath to find its config defaults when
	# run outside /usr/share/grafana; without it, plugin commands abort with
	# "Could not find config defaults". Newer packages ship `grafana cli`,
	# older ones only `grafana-cli`.
	if ! grafana_cli plugins ls 2>/dev/null | grep -q frser-sqlite-datasource; then
		grafana_cli plugins install frser-sqlite-datasource
	fi

	mkdir -p "${GRAFANA_PROVISIONING_DIR}/datasources" "${GRAFANA_PROVISIONING_DIR}/dashboards" "${GRAFANA_DASHBOARD_DIR}"
	install -m 0644 "${SCRIPT_DIR}/grafana/provisioning/datasources/telemetry.yaml" "${GRAFANA_PROVISIONING_DIR}/datasources/telemetry.yaml"
	install -m 0644 "${SCRIPT_DIR}/grafana/provisioning/dashboards/telemetry.yaml" "${GRAFANA_PROVISIONING_DIR}/dashboards/telemetry.yaml"
	install -m 0644 "${SCRIPT_DIR}/grafana/dashboards/gentle-ai-usage.json" "${GRAFANA_DASHBOARD_DIR}/gentle-ai-usage.json"

	# Served at /grafana/ behind Apache (deploy/telemetry/apache/telemetry-vhost.conf.tmpl),
	# on the same domain, so no separate port is exposed publicly.
	# Apache proxies /grafana/ to loopback; never expose port 3000 itself.
	set_ini_kv "${GRAFANA_INI}" server http_addr 127.0.0.1
	set_ini_kv "${GRAFANA_INI}" server root_url "%(protocol)s://%(domain)s/grafana/"
	set_ini_kv "${GRAFANA_INI}" server serve_from_sub_path true
	# frser-sqlite-datasource v3+ requires this to open a local filesystem
	# path instead of only a bundled/uploaded file.
	set_ini_kv "${GRAFANA_INI}" plugin.frser-sqlite-datasource allow_local_mode true
	# This box already runs cPanel and Docker on 3.6 GiB of RAM; Grafana's
	# background reporting/update checks are pure overhead here.
	set_ini_kv "${GRAFANA_INI}" analytics reporting_enabled false
	set_ini_kv "${GRAFANA_INI}" analytics check_for_updates false

	# Grafana ships with a default admin/admin login: generate a random
	# password and lock the account down in grafana.ini before Grafana
	# ever starts, so admin/admin is never reachable, not even for one
	# request. Grafana only seeds [security] admin_user/admin_password
	# into its own database on that very first startup — on a re-run
	# against an already-initialized Grafana, this does not rotate the
	# live password; use `grafana cli --homepath /usr/share/grafana admin reset-admin-password` for that.
	if [[ ! -f "${GRAFANA_ADMIN_PASSWORD_FILE}" ]]; then
		umask 0177
		openssl rand -base64 24 >"${GRAFANA_ADMIN_PASSWORD_FILE}"
		umask 0022
	fi
	chmod 0600 "${GRAFANA_ADMIN_PASSWORD_FILE}"
	set_ini_kv "${GRAFANA_INI}" security admin_user gentle
	set_ini_kv "${GRAFANA_INI}" security admin_password "$(cat "${GRAFANA_ADMIN_PASSWORD_FILE}")"
	set_ini_kv "${GRAFANA_INI}" security disable_gravatar true
	set_ini_kv "${GRAFANA_INI}" auth.anonymous enabled false
	set_ini_kv "${GRAFANA_INI}" users allow_sign_up false

	# The collector runs SQLite in journal_mode=DELETE (see storage.go),
	# not WAL: gentle-telemetry.service already serializes all of its own
	# reads and writes through a single connection, so WAL's concurrent-
	# reader benefit is moot for the collector itself, and dropping it
	# avoids ever creating -wal/-shm sidecar files. That leaves only one
	# file for a second, read-only process to deal with: grant Grafana's
	# system user read access to it via a POSIX ACL rather than group
	# membership, because gentle-telemetry.service runs under systemd's
	# DynamicUser, whose group is allocated per-unit with no stable name
	# to add "grafana" to.
	dnf install -y acl >/dev/null 2>&1 || true
	# With DynamicUser, systemd materialises StateDirectory under
	# /var/lib/private (mode 0700) and leaves a symlink at STATE_DIR, so
	# grafana also needs search permission on every ancestor that is not
	# world-searchable; without it the datasource fails with
	# "permission denied" even though the file ACL below is in place.
	local ancestor
	ancestor="$(dirname "$(readlink -f "${STATE_DIR}" 2>/dev/null || printf '/')")"
	# Walk only absolute paths below "/": an unresolvable STATE_DIR (unit
	# never started, dangling symlink) yields "." and must not loop forever.
	while [[ "${ancestor}" == /?* ]]; do
		if [[ ! -x "${ancestor}" ]] || [[ "$(stat -c '%A' "${ancestor}")" != *x ]]; then
			setfacl -m u:grafana:--x "${ancestor}"
		fi
		ancestor="$(dirname "${ancestor}")"
	done
	setfacl -m u:grafana:rx "${STATE_DIR}"
	if [[ -f "${STATE_DIR}/events.sqlite" ]]; then
		setfacl -m u:grafana:r "${STATE_DIR}/events.sqlite"
	fi

	systemctl daemon-reload
	systemctl enable --now grafana-server
	printf 'Grafana admin password: %s (root-only 0600); admin_user is "gentle". Change it via Administration -> Users if this is a first install.\n' "${GRAFANA_ADMIN_PASSWORD_FILE}"
	printf 'Grafana installed; it will be reachable at /grafana/ once the vhost blocks below are applied. Panel guide: docs/telemetry-collector.md#grafana-dashboards.\n'
}

if [[ "${WITH_GRAFANA}" == "true" ]]; then
	install_grafana
fi

mkdir -p /var/log/gentle-telemetry

if [[ -n "${DOMAIN}" ]]; then
	sed "s/__DOMAIN__/${DOMAIN}/g" "${SCRIPT_DIR}/apache/telemetry-vhost.conf.tmpl" >"${RENDERED_VHOST}"
	chmod 0600 "${RENDERED_VHOST}"
	# The :80 block's DocumentRoot and Certbot's --webroot both need this
	# directory to exist before httpd is reloaded with the block appended.
	install -d -m 0755 /var/www/gentle-telemetry-acme
	printf 'rendered the vhost template for %s to %s\n' "${DOMAIN}" "${RENDERED_VHOST}"
	print_domain="${DOMAIN}"
else
	printf 'no --domain given: render %s/apache/telemetry-vhost.conf.tmpl yourself (substitute __DOMAIN__ for the real hostname) before following the steps below.\n' "${SCRIPT_DIR}"
	print_domain="<domain>"
	RENDERED_VHOST="<rendered-vhost-file>"
fi

# This script never edits ${APACHE_INCLUDE_FILE} itself: only the operator
# appends to it, backing it up first, and only the :80 block before the
# certificate exists (Certbot's webroot check needs that block's ACME
# exception reachable) — the :443 block references certificate files that
# do not exist yet, so appending it first would fail apachectl configtest.
cat <<EOF

Next steps (not run by this script), in order:

1. DNS: create an A record for ${print_domain} at your registrar, pointing
   at this VPS's IP. Confirm with: dig +short ${print_domain}

2. Issue the certificate — the :80 block must exist and be live before
   Certbot's webroot check can pass, so append it, reload, THEN certbot,
   THEN append the :443 block:

     cp -a ${APACHE_INCLUDE_FILE} ${APACHE_INCLUDE_FILE}.bak-\$(date +%Y%m%dT%H%M%SZ)
     sed -n '/# --- BEGIN :80 VHOST ---/,/# --- END :80 VHOST ---/p' ${RENDERED_VHOST} >> ${APACHE_INCLUDE_FILE}
     apachectl configtest
     systemctl reload httpd
     certbot certonly --webroot -w /var/www/gentle-telemetry-acme -d ${print_domain}

     cp -a ${APACHE_INCLUDE_FILE} ${APACHE_INCLUDE_FILE}.bak-\$(date +%Y%m%dT%H%M%SZ)
     sed -n '/# --- BEGIN :443 VHOST ---/,/# --- END :443 VHOST ---/p' ${RENDERED_VHOST} >> ${APACHE_INCLUDE_FILE}

3. Apply and verify the full config:

     apachectl configtest
     systemctl reload httpd

4. Confirm it's live:

     curl -I https://${print_domain}/healthz

done. gentle-telemetry is listening on 127.0.0.1:18181 (see ${UNIT_DIR}/gentle-telemetry.service).
EOF
