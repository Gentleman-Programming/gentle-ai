#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
tmp_root="$(mktemp -d)"
trap 'rm -rf "$tmp_root"' EXIT

binary="$tmp_root/gentle-ai"
outside="$tmp_root/outside-repository"
result="$tmp_root/capabilities.json"
mkdir -p "$outside"

(
  cd "$repo_root"
  go build -o "$binary" ./cmd/gentle-ai
)
(
  cd "$outside"
  "$binary" review capabilities --contract gentle-ai.review-integration/v1 >"$result"
)

python3 - "$result" "$outside" <<'PY'
import json
import pathlib
import re
import sys

result_path = pathlib.Path(sys.argv[1])
outside = pathlib.Path(sys.argv[2])
document = json.loads(result_path.read_text(encoding="utf-8"))

assert document["schema"] == "gentle-ai.review-integration.capabilities/v1.4"
assert document["contract"] == "gentle-ai.review-integration/v1"
assert document["protocol"] == {"major": 1, "minor": 4}
assert document["gates"] == ["post-apply", "pre-commit", "pre-push", "pre-pr", "release"]
assert document["projections"] == ["staged", "workspace"]
assert document["executable"]["evidence"] == "self-reported"
assert document["executable"]["verification"] == "compare-with-published-manifest"
assert re.fullmatch(r"sha256:[0-9a-f]{64}", document["executable"]["sha256"])
features = {feature["name"]: feature for feature in document["features"]["optional"]}
assert features["classified_authority_repair"] == {
    "name": "classified_authority_repair",
    "supported": True,
    "requires": ["native_next_transition", "uniform_failure_envelope"],
}
assert features["one_shot_final_verification_retry"] == {
    "name": "one_shot_final_verification_retry",
    "supported": True,
    "requires": ["compact_v2_authority", "exact_receipt_replay", "native_next_transition"],
}
assert features["native_frozen_candidate_context"] == {
    "name": "native_frozen_candidate_context",
    "supported": True,
    "requires": ["immutable_snapshot"],
}
assert features["opaque_repository_context"] == {
    "name": "opaque_repository_context",
    "supported": True,
    "requires": ["compact_v2_authority", "native_next_transition"],
}
assert features["provider_artifact_admission"] == {
    "name": "provider_artifact_admission",
    "supported": True,
    "requires": ["compact_v2_authority", "native_frozen_candidate_context", "opaque_repository_context"],
}
assert features["provider_targeted_validation_request"] == {
    "name": "provider_targeted_validation_request",
    "supported": True,
    "requires": ["compact_v2_authority", "native_next_transition"],
}
assert features["recovered_correction_evidence"] == {
    "name": "recovered_correction_evidence",
    "supported": True,
    "requires": ["compact_v2_authority", "provider_targeted_validation_request"],
}
assert features["validating_result_reopen"] == {
    "name": "validating_result_reopen",
    "supported": True,
    "requires": ["compact_v2_authority", "provider_artifact_admission"],
}
assert "gentle-ai.review-artifact-subject/v1" in document["schemas"]
assert "gentle-ai.review-admitted-result/v1" in document["schemas"]
assert "gentle-ai.review-targeted-validation-request/v1" in document["schemas"]
assert "gentle-ai.review-authority-repair-assessment/v1" in document["schemas"]
assert "gentle-ai.review-integration.repair/v1" in document["schemas"]
assert "gentle-ai.review-final-verification-incident/v1" in document["schemas"]
assert "review.repair" in document["operations"]
assert "review.retry_final_verification" in document["operations"]
assert list(outside.iterdir()) == []

def keys(value):
    if isinstance(value, dict):
        for key, child in value.items():
            yield key.lower()
            yield from keys(child)
    elif isinstance(value, list):
        for child in value:
            yield from keys(child)

present_keys = set(keys(document))
for forbidden in ("model", "provider", "profile", "cwd", "repository", "store_path", "executable_path"):
    assert forbidden not in present_keys
PY

printf 'review capabilities outside-repository harness: PASS\n'
