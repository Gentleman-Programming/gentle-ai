#!/usr/bin/env bash
# Verify that mise.toml's go and node pins stay in lockstep with the
# authoritative sources every contributor and CI already trust: go.mod's
# `go` directive and ci.yml's `node-version`.
#
# mise.toml is a *second*, independent pin -- nothing else reads it. CI still
# provisions Go via `go-version-file: go.mod` and Node via setup-node's
# `node-version: "24"`. This guard is the only thing keeping the second pin
# honest as go.mod and ci.yml evolve.
#
# Pin equality only: this does NOT enforce a Node version-floor check. That
# is a separate, already-satisfied concern (gentle-ai's own runtime minimum),
# deliberately out of scope here.
#
# Fails closed: every extraction below must find EXACTLY one match. A
# duplicated `node-version:` line anywhere in ci.yml fails the script rather
# than silently validating whichever occurrence grep happens to see first.
set -euo pipefail

# die prints its arguments as a single `mise pins: <message>` line to stderr
# and exits non-zero.
die() {
  printf 'mise pins: %s\n' "$*" >&2
  exit 1
}

[[ $# -eq 0 ]] || die "takes no arguments"

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "${repo_root}"

go_mod="go.mod"
ci_yml=".github/workflows/ci.yml"
mise_toml="mise.toml"

for f in "${go_mod}" "${ci_yml}" "${mise_toml}"; do
  [[ -f "${f}" ]] || die "missing ${f}"
done

# extract_one prints the single line matching an anchored pattern in a file,
# or dies -- on zero matches (nothing to compare against) or on more than one
# (an ambiguous source; guessing which one is authoritative is worse than no
# guard at all).
extract_one() {
  local description="$1" file="$2" pattern="$3"
  local -a matches
  mapfile -t matches < <(grep -E "${pattern}" "${file}" || true)
  case "${#matches[@]}" in
    1) ;;
    0) die "no ${description} found in ${file}" ;;
    *) die "expected exactly one ${description} in ${file}, found ${#matches[@]} -- update it so only one remains authoritative" ;;
  esac
  printf '%s\n' "${matches[0]}"
}

go_mod_line="$(extract_one "go directive" "${go_mod}" '^go [0-9]')"
go_mod_pin="$(awk '{print $2}' <<<"${go_mod_line}")"

ci_node_line="$(extract_one "node-version key" "${ci_yml}" '^[[:space:]]*node-version:')"
ci_node_pin="$(sed -nE 's/^[[:space:]]*node-version: "([0-9][^"]*)".*$/\1/p' <<<"${ci_node_line}")"
[[ -n "${ci_node_pin}" ]] || die "unsupported node-version format in ${ci_yml}: ${ci_node_line}"

mise_go_line="$(extract_one "go pin" "${mise_toml}" '^go = "')"
mise_go_pin="$(sed -E 's/^go = "([^"]*)".*$/\1/' <<<"${mise_go_line}")"

mise_node_line="$(extract_one "node pin" "${mise_toml}" '^node = "')"
mise_node_pin="$(sed -E 's/^node = "([^"]*)".*$/\1/' <<<"${mise_node_line}")"

status=0

if [[ "${mise_go_pin}" != "${go_mod_pin}" ]]; then
  printf 'mise pins: go drift -- mise.toml pins "%s", go.mod pins "%s". Update mise.toml'\''s go value to match go.mod'\''s go directive.\n' "${mise_go_pin}" "${go_mod_pin}" >&2
  status=1
fi

if [[ "${mise_node_pin}" != "${ci_node_pin}" ]]; then
  printf 'mise pins: node drift -- mise.toml pins "%s", ci.yml pins "%s". Update mise.toml'\''s node value to match ci.yml'\''s node-version.\n' "${mise_node_pin}" "${ci_node_pin}" >&2
  status=1
fi

[[ "${status}" -eq 0 ]] || exit 1

printf 'mise pins: go=%s node=%s match go.mod and ci.yml\n' "${mise_go_pin}" "${mise_node_pin}"
