#!/usr/bin/env bash
set -euo pipefail

die() {
  printf 'release public keys: %s\n' "$*" >&2
  exit 1
}

raw=${MINISIGN_PUBLIC_KEYS:-}
[[ -n "$raw" && "$raw" != "UNSET" ]] || die "MINISIGN_PUBLIC_KEYS is unset"
[[ "$raw" != "0000000000000000000000000000000000000000000000000000000000000000" ]] || die "legacy placeholder public key is forbidden"

# A public-key payload is exactly 42 bytes and therefore exactly 56 canonical
# base64 characters without padding. Accept one payload or two distinct
# payloads separated by one comma, with no whitespace or trailing separator.
[[ "$raw" =~ ^[A-Za-z0-9+/]{56}(,[A-Za-z0-9+/]{56})?$ ]] ||
  die "configure one canonical key or a two-key rotation overlap"

IFS=',' read -r -a keys <<<"$raw"
declare -A seen=()
for key in "${keys[@]}"; do
  [[ -z "${seen[$key]:-}" ]] || die "duplicate public key"

  decoded_hex=$(printf '%s' "$key" | base64 --decode 2>/dev/null | od -An -v -tx1 | tr '\n' ' ') ||
    die "public key is not valid base64"
  read -r -a decoded <<<"$decoded_hex"
  (( ${#decoded[@]} == 42 )) || die "public key payload must decode to 42 bytes"
  [[ "${decoded[0]} ${decoded[1]}" == "45 64" ]] || die "public key algorithm must be Ed"

  canonical=$(printf '%s' "$key" | base64 --decode 2>/dev/null | base64 -w0) ||
    die "public key is not valid base64"
  [[ "$canonical" == "$key" ]] || die "public key must use canonical base64 encoding"
  seen[$key]=1
done

printf '%s\n' "$raw"
