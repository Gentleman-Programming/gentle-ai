#!/usr/bin/env bash
set -euo pipefail

die() {
  printf 'remote release verification: %s\n' "$*" >&2
  exit 1
}

: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
: "${GITHUB_REF_NAME:?GITHUB_REF_NAME is required}"
: "${MINISIGN_PUBLIC_KEYS:?MINISIGN_PUBLIC_KEYS is required}"
[[ "$GITHUB_REPOSITORY" == "Gentleman-Programming/gentle-ai" ]] || die "unexpected repository"

if ! canonical_public_keys=$(./scripts/canonicalize-release-public-keys.sh); then
  die "MINISIGN_PUBLIC_KEYS is not canonical"
fi
[[ "$canonical_public_keys" == "$MINISIGN_PUBLIC_KEYS" ]] || die "public-key canonicalization changed the configured value"

tag=$GITHUB_REF_NAME
[[ "$tag" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || die "tag is not exact stable semver"
version=${tag#v}

archives=(
  "gentle-ai_${version}_darwin_amd64.tar.gz"
  "gentle-ai_${version}_darwin_arm64.tar.gz"
  "gentle-ai_${version}_linux_amd64.tar.gz"
  "gentle-ai_${version}_linux_arm64.tar.gz"
)
expected_assets=("${archives[@]}" checksums.txt checksums.txt.minisig)

work=$(mktemp -d)
cleanup() {
  rm -rf "$work"
}
trap cleanup EXIT

release_json=$work/release.json
gh api "repos/$GITHUB_REPOSITORY/releases/tags/$tag" >"$release_json"
[[ "$(jq -r '.tag_name' "$release_json")" == "$tag" ]] || die "remote release tag mismatch"
[[ "$(jq -r '.draft' "$release_json")" == "false" ]] || die "remote release is still a draft"
[[ "$(jq -r '.prerelease' "$release_json")" == "false" ]] || die "stable release is marked prerelease"

mapfile -t actual_assets < <(jq -r '.assets[].name' "$release_json" | LC_ALL=C sort)
mapfile -t sorted_expected_assets < <(printf '%s\n' "${expected_assets[@]}" | LC_ALL=C sort)
if ! diff -u <(printf '%s\n' "${sorted_expected_assets[@]}") <(printf '%s\n' "${actual_assets[@]}"); then
  die "remote asset set is incomplete or unexpected"
fi

download_dir=$work/assets
mkdir -p "$download_dir"
gh release download "$tag" --repo "$GITHUB_REPOSITORY" --dir "$download_dir"
mapfile -t downloaded_assets < <(find "$download_dir" -maxdepth 1 -type f -printf '%f\n' | LC_ALL=C sort)
if ! diff -u <(printf '%s\n' "${sorted_expected_assets[@]}") <(printf '%s\n' "${downloaded_assets[@]}"); then
  die "downloaded asset set differs from the API"
fi

verified=false
trusted=""
IFS=',' read -r -a configured_keys <<<"$canonical_public_keys"
for signing_public_key in "${configured_keys[@]}"; do
  if trusted=$(cd "$download_dir" && minisign -VQ -m checksums.txt -x checksums.txt.minisig -P "$signing_public_key" 2>/dev/null); then
    verified=true
    break
  fi
done
[[ "$verified" == true ]] || die "remote checksum signature verification failed"
[[ "$trusted" == "repo=$GITHUB_REPOSITORY;tag=$tag" ]] || die "remote trusted comment identity mismatch"

mapfile -t manifest_assets < <(awk 'NF == 2 { print $2 }' "$download_dir/checksums.txt" | LC_ALL=C sort)
mapfile -t sorted_archives < <(printf '%s\n' "${archives[@]}" | LC_ALL=C sort)
if ! diff -u <(printf '%s\n' "${sorted_archives[@]}") <(printf '%s\n' "${manifest_assets[@]}"); then
  die "signed manifest has duplicate, missing, or unexpected archive entries"
fi
(cd "$download_dir" && sha256sum --check --strict checksums.txt)

printf 'remote release verification: authenticated %d archives for %s\n' "${#archives[@]}" "$tag"
