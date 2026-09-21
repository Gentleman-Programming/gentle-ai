#!/usr/bin/env bash
set -euo pipefail

readonly ruleset_id=13932547
readonly expected_repository='Gentleman-Programming/gentle-ai'
readonly expected_schema='gentle-ai.merge-blocking-status-contexts/v1'
policy_drift() {
  printf 'policy_drift: %s\n' "$*" >&2
  exit 1
}
policy_unverifiable() {
  printf 'policy_unverifiable: %s\n' "$*" >&2
  exit 2
}
[[ -n "${GITHUB_REPOSITORY:-}" ]] || policy_unverifiable 'GITHUB_REPOSITORY is required'
[[ "$GITHUB_REPOSITORY" == "$expected_repository" ]] ||
  policy_unverifiable 'unexpected repository'
[[ -n "${GH_TOKEN:-}" ]] || policy_unverifiable 'GH_TOKEN is required'
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
manifest=${POLICY_MANIFEST:-"$script_dir/merge-blocking-status-contexts.json"}
[[ -f "$manifest" ]] || policy_unverifiable 'policy manifest is missing'
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
declared="$work/declared.json"
rulesets="$work/rulesets.json"
ruleset="$work/ruleset.json"

if ! jq -e -c --arg schema "$expected_schema" '
  def valid_entry:
    type == "object" and
    (keys | sort) == ["context", "integration_id"] and
    (.context | type == "string" and length > 0) and
    (.integration_id | type == "number" and floor == . and . >= 0);
  def valid_manifest:
    type == "object" and
    .schema == $schema and
    (.required_status_checks | type == "array" and length > 0) and
    all(.required_status_checks[]; valid_entry) and
    ((.required_status_checks | map([.context, .integration_id]) | unique | length)
      == (.required_status_checks | length));
  if valid_manifest then
    [.required_status_checks[]] | sort_by(.context, .integration_id)
  else
    error("unknown schema")
  end
' "$manifest" >"$declared" 2>/dev/null; then
  policy_unverifiable 'policy manifest has an unknown schema'
fi

if ! gh api --method GET "repos/$GITHUB_REPOSITORY/rulesets?includes_parents=false" \
  >"$rulesets" 2>/dev/null; then
  policy_unverifiable 'could not read repository rulesets'
fi

if ! jq -e '
  type == "array" and
  all(.[];
    type == "object" and
    (.id | type == "number" and floor == . and . >= 1) and
    (.enforcement | type == "string")
  )
' "$rulesets" >/dev/null 2>&1; then
  policy_unverifiable 'ruleset list response has an unknown schema'
fi

active_count=$(jq '[.[] | select(.enforcement == "active")] | length' "$rulesets" 2>/dev/null) ||
  policy_unverifiable 'ruleset list response could not be inspected'
if [[ "$active_count" == 0 ]]; then
  policy_unverifiable 'no active rulesets were returned'
fi

if ! gh api --method GET "repos/$GITHUB_REPOSITORY/rulesets/$ruleset_id" \
  >"$ruleset" 2>/dev/null; then
  policy_unverifiable "could not read ruleset $ruleset_id"
fi

if ! jq -e -c --argjson id "$ruleset_id" '
  def valid_entry:
    type == "object" and
    (keys | sort) == ["context", "integration_id"] and
    (.context | type == "string" and length > 0) and
    (.integration_id | type == "number" and floor == . and . >= 0);
  ([.rules[]? | select(type == "object" and .type == "required_status_checks")]) as $required |
  def valid_ruleset:
    type == "object" and
    .id == $id and
    .enforcement == "active" and
    (.rules | type == "array") and
    ($required | length == 1) and
    ($required[0].parameters | type == "object") and
    ($required[0].parameters.required_status_checks | type == "array") and
    all($required[0].parameters.required_status_checks[]; valid_entry) and
    (($required[0].parameters.required_status_checks
        | map([.context, .integration_id]) | unique | length)
      == ($required[0].parameters.required_status_checks | length));
  if valid_ruleset then
    $required[0].parameters.required_status_checks | sort_by(.context, .integration_id)
  else
    error("unknown schema")
  end
' "$ruleset" >"$work/observed.json" 2>/dev/null; then
  policy_unverifiable 'ruleset response has an unknown schema'
fi

if ! cmp -s "$declared" "$work/observed.json"; then
  printf 'policy_drift: required status contexts differ for ruleset %s\n' "$ruleset_id" >&2
  printf 'declared: ' >&2
  jq -c . "$declared" >&2
  printf 'observed: ' >&2
  jq -c . "$work/observed.json" >&2
  exit 1
fi

printf 'merge policy verified: %s required status contexts match ruleset %s\n' \
  "$(jq 'length' "$declared")" "$ruleset_id"
