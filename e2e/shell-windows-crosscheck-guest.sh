#!/bin/sh
# Compilation/syntax only. Never Windows 11 runtime or user-delivery evidence.
# stdout is exclusively inert artifact DATA; diagnostics stay on stderr.
exec 3>&1 1>&2
set -eu
stop() { printf 'STOP: %s\n' "$1" >&2; exit 1; }
test "$(id -u)" = 65532 && test "$(id -g)" = 65532 || stop 'unprivileged Guest identity'
awk '/^CapEff:/ {cap=($2=="0000000000000000")} /^NoNewPrivs:/ {lock=($2==1)} END {exit !(cap&&lock)}' /proc/self/status || stop 'privilege limits'
test "$(cat /sys/fs/cgroup/memory.max)" = 3221225472 || stop 'memory limit'
test "$(cat /sys/fs/cgroup/memory.swap.max)" = 0 || stop 'swap limit'
test "$(cat /sys/fs/cgroup/pids.max)" = 64 || stop 'PID limit'
test "$(cat /sys/fs/cgroup/cpu.max)" = '100000 100000' || stop 'CPU limit'
awk '$5=="/" {seen=1;if(index(","$6",",",ro,")==0)bad=1} END{exit(!seen||bad)}' /proc/self/mountinfo || stop 'read-only OS root'
env | awk -F= '$1 !~ /^(PATH|HOME|PWD|HOST_NETNS|HOST_MNTNS)$/ {bad=1} END {exit bad}' || stop 'unexpected environment (credentials forbidden)'
for identity in "$HOST_NETNS" "$HOST_MNTNS"; do
 case "$identity" in ""|*[!0-9]*) stop 'missing host namespace witness' ;; esac
done
test "$(stat -Lc %i /proc/self/ns/net)" != "$HOST_NETNS" || stop 'shared host network namespace'
test "$(stat -Lc %i /proc/self/ns/mnt)" != "$HOST_MNTNS" || stop 'shared host mount namespace'
test "$HOME" = /tmp && test "$(pwd)" = / || stop 'private HOME/CWD'
for path in /root /home /run/docker.sock /var/run/docker.sock; do
 case "$path" in
 /root) test ! -r "$path" || stop 'root HOME accessible' ;;
 /home) test -z "$(find /home -mindepth 1 -maxdepth 1 -print -quit)" || stop 'personal HOME present' ;;
 *) test ! -e "$path" || stop 'host socket' ;;
 esac
done
# Mounts other than Docker plumbing, cgroups and the single private tmpfs fail.
awk '{
 m=$5
 if(m=="/"||m=="/tmp"||m=="/sys"||m=="/sys/fs/cgroup"||m=="/dev"||m=="/dev/pts"||m=="/dev/shm"||m=="/dev/mqueue"||m=="/sys/firmware"||m=="/sys/devices/virtual/powercap"||m~/^\/proc(\/|$)/||m~/^\/etc\/(hosts|hostname|resolv.conf)$/)next
 bad=1
} END{exit bad}' /proc/self/mountinfo || stop 'unexpected host/candidate mount'
umask 077
mkdir /tmp/source /tmp/cache /tmp/gopath
tar -xf /source.tar -C /tmp/source
cd /tmp/source
export GOMAXPROCS=1 GOTOOLCHAIN=local GOENV=off GOFLAGS= GOWORK=off CGO_ENABLED=0
export GOPATH=/tmp/gopath GOMODCACHE=/tmp/gopath/pkg/mod GOCACHE=/tmp/cache
export GOSUMDB=sum.golang.org GOPROXY=https://proxy.golang.org GONOSUMDB= GOPRIVATE= GONOPROXY= GOINSECURE=
gofmt -l internal/shellinstaller/*.go internal/cli/shell_install*.go scripts/user_windows_helpers.go > /tmp/format-names
# Retain originals for a complete bounded correction patch, not an excerpt.
for name in internal/shellinstaller/*.go internal/cli/shell_install*.go scripts/user_windows_helpers.go; do
 mkdir -p "/tmp/original/$(dirname "$name")"
 cp "$name" "/tmp/original/$name"
 gofmt -w "$name"
done
: > /tmp/format-patch
for name in internal/shellinstaller/*.go internal/cli/shell_install*.go scripts/user_windows_helpers.go; do
 diff -u --label "a/$name" --label "b/$name" "/tmp/original/$name" "$name" >> /tmp/format-patch || status=$?
 test "${status:-0}" -le 1 || stop 'format diff error'
 unset status
done
format_bytes=$(wc -c < /tmp/format-patch)
printf 'FORMAT-PATCH-BYTES: %s\n' "$format_bytes"
printf 'FORMAT-PATCH-SHA256: %s\n' "$(sha256sum /tmp/format-patch | cut -d' ' -f1)"
if test "$format_bytes" -le 20000; then
 printf 'FORMAT-DATA-BASE64: '; base64 -w0 /tmp/format-patch; printf '\n'
else
 printf 'FORMAT-DATA-WHOLLY-WITHHELD: exceeds20000\n'
fi
diagnostic() {
 bytes=$(wc -c < /tmp/check.log)
 printf 'BUILD-BYTES: %s\nBUILD-SHA256: %s\n' "$bytes" "$(sha256sum /tmp/check.log | cut -d' ' -f1)"
 if test "$bytes" -le 1024; then printf 'BUILD-RAW-BASE64: '; base64 -w0 /tmp/check.log; printf '\n'; else printf 'BUILD-RAW-WHOLLY-WITHHELD: exceeds1024\n'; fi
}
go mod download > /tmp/check.log 2>&1 || { diagnostic; stop 'authenticated module acquisition'; }
go test -timeout=90s -run '^TestWindowsShellInstall' ./internal/cli > /tmp/check.log 2>&1 || { diagnostic; stop 'portable TUI tests'; }
GOOS=windows GOARCH=amd64 go test -c -o /tmp/windows-shellinstaller.test.exe ./internal/shellinstaller > /tmp/check.log 2>&1 || { diagnostic; stop 'Windows test compile'; }
GOOS=windows GOARCH=amd64 go test -c -o /tmp/windows-cli.test.exe ./internal/cli > /tmp/check.log 2>&1 || { diagnostic; stop 'Windows CLI test compile'; }
GOOS=windows GOARCH=amd64 go build -o /tmp/windows-gentle-ai.exe ./cmd/gentle-ai > /tmp/check.log 2>&1 || { diagnostic; stop 'Windows product compile'; }
node --check scripts/provision-gentle-shell-windows.mjs > /tmp/check.log 2>&1 || { diagnostic; stop 'JavaScript syntax'; }
printf 'PASS: bounded Linux Guest syntax/Windows crosscompile only; normalized formatted source, NOT Win11 runtime qualification or deliverability.\n'
test ! -s /tmp/format-names || { printf 'STOP: original candidate needs Guest-generated formatting; original format check NOT PASS.\n'; exit 1; }
cd /tmp
cp /source-commit.txt source-commit.txt
sha256sum windows-gentle-ai.exe windows-shellinstaller.test.exe windows-cli.test.exe > SHA256SUMS
bytes=$(wc -c windows-gentle-ai.exe windows-shellinstaller.test.exe windows-cli.test.exe | awk 'END {print $1}')
test "$bytes" -le 268435456 || stop 'whole Windows artifact payload exceeds 256 MiB'
# Export while tmpfs is still mounted; never parse/extract this archive on host.
tar -cf - windows-gentle-ai.exe windows-shellinstaller.test.exe windows-cli.test.exe SHA256SUMS source-commit.txt >&3
