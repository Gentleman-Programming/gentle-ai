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

assert document["schema"] == "gentle-ai.review-integration.capabilities/v1"
assert document["contract"] == "gentle-ai.review-integration/v1"
assert document["gates"] == ["post-apply", "pre-commit", "pre-push", "pre-pr", "release"]
assert document["projections"] == ["staged", "workspace"]
assert document["executable"]["evidence"] == "self-reported"
assert document["executable"]["verification"] == "compare-with-published-manifest"
assert re.fullmatch(r"sha256:[0-9a-f]{64}", document["executable"]["sha256"])
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
