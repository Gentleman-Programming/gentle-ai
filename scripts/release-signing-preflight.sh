#!/usr/bin/env bash
set -euo pipefail

die() {
  printf 'release signing preflight: %s\n' "$*" >&2
  exit 1
}

: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
: "${GITHUB_REF_NAME:?GITHUB_REF_NAME is required}"
: "${MINISIGN_PUBLIC_KEYS:?MINISIGN_PUBLIC_KEYS is required}"
: "${MINISIGN_PUBLIC_KEYS_CANONICAL:?MINISIGN_PUBLIC_KEYS_CANONICAL is required}"
: "${MINISIGN_SECRET_KEY_FILE:?MINISIGN_SECRET_KEY_FILE is required}"
: "${MINISIGN_SIGNING_PUBLIC_KEY_FILE:?MINISIGN_SIGNING_PUBLIC_KEY_FILE is required}"

[[ -s "$MINISIGN_SECRET_KEY_FILE" ]] || die "signing key file is missing or empty"
[[ "$(stat -c '%a' "$MINISIGN_SECRET_KEY_FILE")" == "600" ]] || die "signing key file mode must be 600"
command -v minisign >/dev/null 2>&1 || die "minisign is unavailable"

work=$(mktemp -d)
cleanup() {
  rm -rf "$work"
}
trap cleanup EXIT

derived_file=$work/derived.pub
if ! timeout 15s minisign -R -s "$MINISIGN_SECRET_KEY_FILE" -p "$derived_file" </dev/null >/dev/null 2>&1; then
  die "cannot derive a public key non-interactively; provision an unencrypted (-W) Minisign key"
fi
derived=$(sed -n '2{s/\r$//;p;}' "$derived_file")
[[ -n "$derived" ]] || die "derived public key is empty"

if ! validated_keys=$(./scripts/canonicalize-release-public-keys.sh); then
  die "MINISIGN_PUBLIC_KEYS is not canonical"
fi
[[ "$validated_keys" == "$MINISIGN_PUBLIC_KEYS_CANONICAL" ]] ||
  die "canonical trust anchors do not match the validated repository variable"

matched=false
IFS=',' read -r -a configured_keys <<<"$MINISIGN_PUBLIC_KEYS_CANONICAL"
test_key=$(tr -d '\r\n' < internal/update/upgrade/testdata/minisign-test.pub)
for key in "${configured_keys[@]}"; do
  [[ "$key" != "$test_key" ]] || die "isolated test key cannot be used for production"
  if [[ "$key" == "$derived" ]]; then
    matched=true
  fi
done
[[ "$derived" != "$test_key" ]] || die "production private key derives the isolated test key"
[[ "$matched" == true ]] || die "signing private key does not match any injected public trust anchor"

printf '%s\n' "$derived" >"$MINISIGN_SIGNING_PUBLIC_KEY_FILE"
chmod 600 "$MINISIGN_SIGNING_PUBLIC_KEY_FILE"

canary=$work/canary.txt
signature=$work/canary.txt.minisig
trusted="repo=$GITHUB_REPOSITORY;tag=$GITHUB_REF_NAME"
printf 'gentle-ai release signing preflight\n' >"$canary"
if ! timeout 15s minisign -S -s "$MINISIGN_SECRET_KEY_FILE" -m "$canary" -x "$signature" \
  -c 'gentle-ai release preflight' -t "$trusted" </dev/null >/dev/null 2>&1; then
  die "signing canary failed"
fi
verified=$(minisign -VQ -m "$canary" -x "$signature" -P "$derived") || die "canary verification failed"
[[ "$verified" == "$trusted" ]] || die "canary trusted comment does not bind the exact repository and tag"

printf 'release signing preflight: key match and canary verified\n'
