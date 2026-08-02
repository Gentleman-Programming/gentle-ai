#!/usr/bin/env bash
set -euo pipefail

# The Release workflow and the CI workflow are independent. Both fire on the
# same commit and neither waits for the other, so a tag could publish signed
# assets while CI was red on the exact bytes being shipped. That is not
# hypothetical: v2.2.0 published green with CI failing on ee83e83d, because
# nothing in the release path had ever looked.
#
# This closes it for the whole of CI rather than for one job. Adding the E2E
# matrix to the release workflow would cover today's gap and miss the next job
# someone adds; requiring the CI run itself covers every job CI already has and
# every job it grows.

die() {
  printf 'release CI gate: %s\n' "$*" >&2
  exit 1
}

require_env() {
  local name=$1
  [[ -n "${!name:-}" ]] || die "$name is required"
}

require_env GITHUB_REPOSITORY
require_env GITHUB_SHA
require_env GH_TOKEN

workflow_ref=${RELEASE_REQUIRED_WORKFLOW:-ci.yml}
workflow_name=CI

runs=$(gh api --paginate \
  "repos/$GITHUB_REPOSITORY/actions/workflows/$workflow_ref/runs?head_sha=$GITHUB_SHA&per_page=100" \
  --jq '.workflow_runs[] | [.created_at, .id, .status, (.conclusion // "none"), .html_url] | @tsv') ||
  die "could not read workflow runs for $GITHUB_SHA"

if [[ -z "$runs" ]]; then
  die "no $workflow_name run exists for $GITHUB_SHA. Push the commit to a branch that runs $workflow_name, wait for it to finish, then re-run this release from the same tag"
fi

# A commit can carry several runs of the same workflow: a re-run, or a retry
# after an infrastructure failure. API order is not a contract, so select by
# creation time and use the immutable run ID as a deterministic tie-breaker.
newest=$(printf '%s\n' "$runs" | LC_ALL=C sort -t $'\t' -k1,1 -k2,2n | tail -n 1)
status=$(printf '%s' "$newest" | cut -f3)
conclusion=$(printf '%s' "$newest" | cut -f4)
url=$(printf '%s' "$newest" | cut -f5)

case "$status" in
  completed) ;;
  *) die "$workflow_name is still $status for $GITHUB_SHA ($url). Wait for it to finish, then re-run this release from the same tag" ;;
esac

[[ "$conclusion" == "success" ]] ||
  die "$workflow_name concluded $conclusion for $GITHUB_SHA ($url). Fix it on main, let $workflow_name go green on the exact commit you intend to ship, then move the tag and release again"

printf 'release CI gate: %s succeeded for %s\n' "$workflow_name" "$GITHUB_SHA"
