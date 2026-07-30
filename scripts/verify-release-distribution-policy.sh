#!/usr/bin/env bash
set -euo pipefail

die() {
  printf 'release distribution policy: %s\n' "$*" >&2
  exit 1
}

(( $# == 0 )) || die "arguments are forbidden; validation is bound to the canonical release files"
command -v go >/dev/null 2>&1 || die "Go is required"

root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
[[ -n "${RELEASE_POLICY_SNAPSHOT_MARKER:-}" ]] || die "RELEASE_POLICY_SNAPSHOT_MARKER is required"
[[ -n "${RELEASE_POLICY_SNAPSHOT_RUN_ID:-}" ]] || die "RELEASE_POLICY_SNAPSHOT_RUN_ID is required"

cd "$root"
exec go run ./internal/releasepolicycmd
