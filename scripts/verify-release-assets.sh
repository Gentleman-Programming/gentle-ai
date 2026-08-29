#!/usr/bin/env bash
set -euo pipefail
umask 077

die() {
  printf 'remote release verification: %s\n' "$*" >&2
  exit 1
}

: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
: "${GITHUB_WORKFLOW:?GITHUB_WORKFLOW is required}"
: "${GITHUB_RUN_ID:?GITHUB_RUN_ID is required}"
: "${GITHUB_RUN_ATTEMPT:?GITHUB_RUN_ATTEMPT is required}"
: "${MINISIGN_PUBLIC_KEYS:?MINISIGN_PUBLIC_KEYS is required}"
[[ "$GITHUB_RUN_ID" =~ ^[1-9][0-9]*$ ]] || die "GITHUB_RUN_ID is invalid"
[[ "$GITHUB_RUN_ATTEMPT" =~ ^[1-9][0-9]*$ ]] || die "GITHUB_RUN_ATTEMPT is invalid"
[[ "$GITHUB_REPOSITORY" == "Gentleman-Programming/gentle-ai" ]] || die "unexpected repository"
case "$GITHUB_WORKFLOW" in
  Release|"Promote stable RC") ;;
  *) die "unexpected workflow" ;;
esac

if ! canonical_public_keys=$(./scripts/canonicalize-release-public-keys.sh); then
  die "MINISIGN_PUBLIC_KEYS is not canonical"
fi
[[ "$canonical_public_keys" == "$MINISIGN_PUBLIC_KEYS" ]] || die "public-key canonicalization changed the configured value"

if [[ -v RELEASE_VERIFICATION_TAG ]]; then
  tag=$RELEASE_VERIFICATION_TAG
  [[ -n "$tag" ]] || die "RELEASE_VERIFICATION_TAG is empty"
else
  : "${GITHUB_REF_NAME:?GITHUB_REF_NAME is required when RELEASE_VERIFICATION_TAG is unset}"
  tag=$GITHUB_REF_NAME
fi
[[ "$tag" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || die "tag is not exact stable semver"
version=${tag#v}
if [[ -v PROVIDER_CONTRACT_SEMVER ]]; then
  contract_semver=$PROVIDER_CONTRACT_SEMVER
else
  contract_semver=$(tr -d '\n' < contracts/review-provider-contract/CONTRACT_SEMVER)
fi
[[ "$contract_semver" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || die "provider contract semver is invalid"

archives=(
  "gentle-ai_${version}_darwin_amd64.tar.gz"
  "gentle-ai_${version}_darwin_arm64.tar.gz"
  "gentle-ai_${version}_linux_amd64.tar.gz"
  "gentle-ai_${version}_linux_arm64.tar.gz"
  "gentle-ai-review-provider-contract-${contract_semver}.tar.gz"
)
provenance_archive=gentle-ai-release-provenance-v1.tar.gz
signed_archives=("${archives[@]}" "$provenance_archive")
expected_assets=("${signed_archives[@]}" checksums.txt checksums.txt.minisig)

work=$(mktemp -d)
chmod 700 "$work"
cleanup() {
  rm -rf "$work"
}
trap cleanup EXIT

tag_ref_json=$work/tag-ref.json
tag_object_json=$work/tag-object.json
if ! gh api "repos/$GITHUB_REPOSITORY/git/ref/tags/$tag" >"$tag_ref_json" 2>/dev/null; then
  die "could not resolve release tag"
fi
tag_object=$(jq -er '.object.sha' "$tag_ref_json" 2>/dev/null) || die "release tag reference is invalid"
[[ "$(jq -er '.object.type' "$tag_ref_json" 2>/dev/null)" == tag && "$tag_object" =~ ^[0-9a-f]{40}$ ]] || die "release tag is not annotated"
if ! gh api "repos/$GITHUB_REPOSITORY/git/tags/$tag_object" >"$tag_object_json" 2>/dev/null; then
  die "could not resolve annotated release tag"
fi
source_sha=$(jq -er '.object.sha' "$tag_object_json" 2>/dev/null) || die "annotated release tag is invalid"
[[ "$(jq -er '.object.type' "$tag_object_json" 2>/dev/null)" == commit && "$source_sha" =~ ^[0-9a-f]{40}$ ]] || die "annotated release tag does not resolve to a commit"

release_json=$work/release.json
if ! gh api "repos/$GITHUB_REPOSITORY/releases/tags/$tag" >"$release_json" 2>/dev/null; then
  die "could not read release"
fi
[[ "$(jq -r '.tag_name' "$release_json")" == "$tag" ]] || die "remote release tag mismatch"
[[ "$(jq -r '.draft' "$release_json")" == "false" ]] || die "remote release is still a draft"
[[ "$(jq -r '.prerelease' "$release_json")" == "false" ]] || die "stable release is marked prerelease"
[[ "$(jq -r '.immutable' "$release_json")" == "true" ]] || die "remote release is not immutable"

mapfile -t actual_assets < <(jq -r '.assets[].name' "$release_json" | LC_ALL=C sort)
mapfile -t sorted_expected_assets < <(printf '%s\n' "${expected_assets[@]}" | LC_ALL=C sort)
if ! diff -u <(printf '%s\n' "${sorted_expected_assets[@]}") <(printf '%s\n' "${actual_assets[@]}") >/dev/null; then
  die "remote asset set is incomplete or unexpected"
fi

download_dir=$work/assets
mkdir -p "$download_dir"
if ! gh release download "$tag" --repo "$GITHUB_REPOSITORY" --dir "$download_dir" 2>/dev/null; then
  die "could not download release assets"
fi
mapfile -t downloaded_assets < <(find "$download_dir" -maxdepth 1 -type f -printf '%f\n' | LC_ALL=C sort)
if ! diff -u <(printf '%s\n' "${sorted_expected_assets[@]}") <(printf '%s\n' "${downloaded_assets[@]}") >/dev/null; then
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
mapfile -t sorted_archives < <(printf '%s\n' "${signed_archives[@]}" | LC_ALL=C sort)
if ! diff -u <(printf '%s\n' "${sorted_archives[@]}") <(printf '%s\n' "${manifest_assets[@]}") >/dev/null; then
  die "signed manifest has duplicate, missing, or unexpected archive entries"
fi
(cd "$download_dir" && sha256sum --check --strict checksums.txt >/dev/null 2>&1) || die "signed archive checksums do not verify"

provenance_bundle=$download_dir/$provenance_archive
mapfile -t provenance_entries < <(tar -tzf "$provenance_bundle" 2>/dev/null) || die "provenance archive cannot be listed"
[[ ${#provenance_entries[@]} == 1 && ${provenance_entries[0]} == manifest.json ]] || die "provenance archive layout is invalid"
tar -tvzf "$provenance_bundle" 2>/dev/null | grep -Eq '^-.* manifest\.json$' || die "provenance manifest is not regular"
provenance_dir=$work/provenance
mkdir -m 700 "$provenance_dir"
provenance_manifest=$provenance_dir/manifest.json
tar -xOf "$provenance_bundle" manifest.json >"$provenance_manifest" 2>/dev/null || die "provenance manifest cannot be extracted"
chmod 600 "$provenance_manifest"
canonical_manifest=$provenance_dir/canonical.json
jq -c . "$provenance_manifest" >"$canonical_manifest" 2>/dev/null || die "provenance manifest is not JSON"
cmp -s "$provenance_manifest" "$canonical_manifest" || die "provenance manifest is not canonical"

config_json=$work/config.json
if ! gh api "repos/$GITHUB_REPOSITORY/contents/.goreleaser.yaml?ref=$source_sha" >"$config_json" 2>/dev/null; then
  die "could not read release configuration"
fi
config_file=$work/goreleaser.yaml
jq -er 'select(.encoding == "base64" and (.content | type == "string")) | .content' "$config_json" 2>/dev/null | base64 --decode >"$config_file" 2>/dev/null || die "release configuration is invalid"
configuration_sha256=sha256:$(sha256sum "$config_file" | awk '{print $1}')

if ! jq -e --arg repository "$GITHUB_REPOSITORY" --arg tag "$tag" --arg source_sha "$source_sha" \
  --arg workflow "$GITHUB_WORKFLOW" --arg run_id "$GITHUB_RUN_ID" --argjson run_attempt "$GITHUB_RUN_ATTEMPT" \
  --arg contract_semver "$contract_semver" --arg configuration_sha256 "$configuration_sha256" --arg version "$version" '
  def binary($name; $goos; $goarch):
    keys_unsorted == ["name", "kind", "goos", "goarch", "cgo_enabled", "trimpath"] and
    .name == $name and .kind == "binary" and .goos == $goos and .goarch == $goarch and
    .cgo_enabled == "0" and .trimpath == true;
  keys_unsorted == ["schema", "repository", "tag", "source_sha", "workflow", "toolchain", "provider_contract_semver", "configuration_sha256", "artifacts"] and
  (.schema | type == "string") and .schema == "gentle-ai.release-provenance/v1" and
  (.repository | type == "string") and .repository == $repository and (.tag | type == "string") and .tag == $tag and
  (.source_sha | type == "string") and .source_sha == $source_sha and
  (.workflow | type == "object" and keys_unsorted == ["name", "run_id", "run_attempt", "job"] and
    (.name | type == "string") and .name == $workflow and (.run_id | type == "string") and .run_id == $run_id and
    (.run_attempt | type == "number") and .run_attempt == $run_attempt and (.job | type == "string") and .job == "release") and
  (.toolchain | type == "object" and keys_unsorted == ["goreleaser", "go"] and
    (.goreleaser | type == "string") and .goreleaser == "v2.15.2" and (.go | type == "string") and test("^go1\\.[0-9]+(\\.[0-9]+)?$")) and
  (.provider_contract_semver | type == "string") and .provider_contract_semver == $contract_semver and
  (.configuration_sha256 | type == "string") and .configuration_sha256 == $configuration_sha256 and
  (.artifacts | type == "array" and length == 5) and
  (.artifacts[0] | binary("gentle-ai_" + $version + "_darwin_amd64.tar.gz"; "darwin"; "amd64")) and
  (.artifacts[1] | binary("gentle-ai_" + $version + "_darwin_arm64.tar.gz"; "darwin"; "arm64")) and
  (.artifacts[2] | binary("gentle-ai_" + $version + "_linux_amd64.tar.gz"; "linux"; "amd64")) and
  (.artifacts[3] | binary("gentle-ai_" + $version + "_linux_arm64.tar.gz"; "linux"; "arm64")) and
  (.artifacts[4] | keys_unsorted == ["name", "kind"] and .name == "gentle-ai-review-provider-contract-" + $contract_semver + ".tar.gz" and .kind == "provider-contract")
' "$provenance_manifest" >/dev/null 2>&1; then
  die "release provenance manifest does not match the release identity"
fi

printf 'remote release verification: authenticated %d archives for %s\n' "${#signed_archives[@]}" "$tag"
