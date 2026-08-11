#!/usr/bin/env bash
set -euo pipefail

die() {
  printf 'stable promotion preflight: %s\n' "$*" >&2
  exit 1
}

require_env() {
  local name=$1
  [[ -n "${!name:-}" ]] || die "$name is required"
}

for name in GITHUB_OUTPUT GITHUB_REF GITHUB_REPOSITORY GITHUB_SHA GH_TOKEN RELEASE_ENVIRONMENT_POLICY_ID SOURCE_PRERELEASE_TAG STABLE_TAG; do
  require_env "$name"
done

[[ "$GITHUB_REPOSITORY" == "Gentleman-Programming/gentle-ai" ]] || die "unexpected repository $GITHUB_REPOSITORY"
[[ "$GITHUB_REF" == "refs/heads/main" ]] || die "promotion must run from main"
[[ "$RELEASE_ENVIRONMENT_POLICY_ID" =~ ^[1-9][0-9]*$ ]] || die "release environment policy ID is invalid"

source_tag=${SOURCE_PRERELEASE_TAG:?}
stable_tag=${STABLE_TAG:?}
if [[ ! "$source_tag" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-rc\.(0|[1-9][0-9]*)$ ]]; then
  die "source prerelease tag must be canonical vMAJOR.MINOR.PATCH-rc.N"
fi
expected_stable="v${BASH_REMATCH[1]}.${BASH_REMATCH[2]}.${BASH_REMATCH[3]}"
[[ "$stable_tag" == "$expected_stable" ]] || die "stable tag must be $expected_stable for $source_tag"

git fetch --no-tags --quiet origin '+refs/heads/main:refs/remotes/origin/main'
[[ "$(git rev-parse "$GITHUB_SHA^{commit}")" == "$(git rev-parse 'refs/remotes/origin/main^{commit}')" ]] ||
  die "workflow revision is not exact current origin/main"

mapfile -t refs < <(git ls-remote origin "refs/tags/$source_tag" "refs/tags/$source_tag^{}")
(( ${#refs[@]} > 0 && ${#refs[@]} < 3 )) || die "source prerelease tag has an invalid remote identity"
direct_sha=
peeled_sha=
for ref in "${refs[@]}"; do
  sha=${ref%%$'\t'*}
  name=${ref#*$'\t'}
  case "$name" in
    "refs/tags/$source_tag") [[ -z "$direct_sha" ]] || die "source prerelease tag is duplicated"; direct_sha=$sha ;;
    "refs/tags/$source_tag^{}") [[ -z "$peeled_sha" ]] || die "source prerelease tag peel is duplicated"; peeled_sha=$sha ;;
    *) die "source prerelease tag has an unexpected remote ref" ;;
  esac
done
source_sha=${peeled_sha:-$direct_sha}
[[ "$source_sha" =~ ^[0-9a-f]{40}$ ]] || die "source prerelease tag does not resolve to a commit"

release=$(gh api "repos/$GITHUB_REPOSITORY/releases/tags/$source_tag") || die "source prerelease release is unavailable"
jq -e --arg tag "$source_tag" \
  '.tag_name == $tag and .draft == false and .prerelease == true and .immutable == true' <<<"$release" >/dev/null ||
  die "source prerelease release must be immutable, published, and prerelease"

# shellcheck disable=SC2016 # GraphQL receives its own $tag variable.
stable_release=$(gh api graphql -f 'query=query($tag: String!) { repository(owner: "Gentleman-Programming", name: "gentle-ai") { release(tagName: $tag) { databaseId } } }' -f "tag=$stable_tag" --jq '.data.repository.release.databaseId // empty')
stable_ref=$(git ls-remote origin "refs/tags/$stable_tag" | awk 'NR == 1 { print $1 }')
stable_peeled=$(git ls-remote origin "refs/tags/$stable_tag^{}" | awk 'NR == 1 { print $1 }')
recovery_state=fresh
if [[ -z "$stable_ref" ]]; then
  [[ -z "$stable_release" ]] || die "stable release exists without its tag"
else
  [[ -n "$stable_peeled" && "$stable_peeled" == "$source_sha" ]] || die "stable tag is incompatible with the admitted source"
  if [[ -z "$stable_release" ]]; then
    recovery_state=resume-tag
  else
    release=$(gh api "repos/$GITHUB_REPOSITORY/releases/$stable_release") || die "stable release is unavailable"
    version=${stable_tag#v}
    if jq -e '.draft == true and (.assets | length) == 0' <<<"$release" >/dev/null; then
      recovery_state=reset-empty-draft
    elif jq -e '.draft == false and .prerelease == false and .immutable == true' <<<"$release" >/dev/null &&
      diff -u <(printf '%s\n' "gentle-ai_${version}_darwin_amd64.tar.gz" "gentle-ai_${version}_darwin_arm64.tar.gz" "gentle-ai_${version}_linux_amd64.tar.gz" "gentle-ai_${version}_linux_arm64.tar.gz" checksums.txt checksums.txt.minisig | LC_ALL=C sort) <(jq -r '.assets[].name' <<<"$release" | LC_ALL=C sort); then
      recovery_state=verify-existing
    else
      die "stable release state is incompatible with safe recovery"
    fi
  fi
fi

GITHUB_SHA=$source_sha ./scripts/require-ci-success.sh
printf 'source_sha=%s\n' "$source_sha" >>"$GITHUB_OUTPUT"
printf 'recovery_state=%s\n' "$recovery_state" >>"$GITHUB_OUTPUT"
printf 'stable promotion preflight: %s promotes %s (%s)\n' "$stable_tag" "$source_sha" "$recovery_state"
