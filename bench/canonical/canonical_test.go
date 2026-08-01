package canonical

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// rawResultWithSandboxPaths is a synthetic bench result JSON with sandbox paths
// embedded in the binary path and journey commands. Paths use the canonical
// bench sandbox structure: <RUN_ROOT>/home/demo for the repo, <RUN_ROOT>/home
// for HOME, and <RUN_ROOT>/gentle-ai-test for the binary.
const rawResultWithSandboxPaths = `{
  "schema": "gentle-ai-bench.results/v2",
  "mode": "driven",
  "binary": "/tmp/gentle-ai-bench-j01-sandbox-abc123/gentle-ai-test",
  "identity": {
    "target_binary_sha256": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "embedded_vcs_revision": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    "classifier_version": "v0.1.0",
    "source_revision": "cccccccccccccccccccccccccccccccccccccccc",
    "vcs_modified": false,
    "runtime_goos": "linux",
    "runtime_goarch": "amd64"
  },
  "corpus": {
    "manifest_sha256": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
    "journey_ids": ["j01-docs-happy-path", "j05-gate-without-any-review"]
  },
  "invocation": {
    "mode": "driven",
    "only": ["j01-docs-happy-path"]
  },
  "journeys": [
    {
      "id": "j01-docs-happy-path",
      "status": "completed",
      "metrics": {
        "blocks": {"in_band": 1, "out_of_band": 0, "dead_end": 0, "self_recovered": 0, "by_design": 0}
      },
      "commands": [
        {
          "sequence": 1,
          "step": "fixture: repo with remote",
          "args": ["git", "-C", "/tmp/gentle-ai-bench-j01-sandbox-abc123/home/demo", "status"]
        }
      ]
    }
  ]
}`

// rawResultEquivalent is the same logical result but with a different sandbox
// temp-dir suffix to prove canonicalization produces identical digest.
const rawResultEquivalent = `{
  "schema": "gentle-ai-bench.results/v2",
  "mode": "driven",
  "binary": "/tmp/gentle-ai-bench-j01-sandbox-xyz789/gentle-ai-test",
  "identity": {
    "target_binary_sha256": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "embedded_vcs_revision": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    "classifier_version": "v0.1.0",
    "source_revision": "cccccccccccccccccccccccccccccccccccccccc",
    "vcs_modified": false,
    "runtime_goos": "linux",
    "runtime_goarch": "amd64"
  },
  "corpus": {
    "manifest_sha256": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
    "journey_ids": ["j01-docs-happy-path", "j05-gate-without-any-review"]
  },
  "invocation": {
    "mode": "driven",
    "only": ["j01-docs-happy-path"]
  },
  "journeys": [
    {
      "id": "j01-docs-happy-path",
      "status": "completed",
      "metrics": {
        "blocks": {"in_band": 1, "out_of_band": 0, "dead_end": 0, "self_recovered": 0, "by_design": 0}
      },
      "commands": [
        {
          "sequence": 1,
          "step": "fixture: repo with remote",
          "args": ["git", "-C", "/tmp/gentle-ai-bench-j01-sandbox-xyz789/home/demo", "status"]
        }
      ]
    }
  ]
}`

// rawResultDifferentInput differs in the target binary SHA-256, so digest must differ.
const rawResultDifferentInput = `{
  "schema": "gentle-ai-bench.results/v2",
  "mode": "driven",
  "binary": "/tmp/gentle-ai-bench-j01-sandbox-abc123/gentle-ai-test",
  "identity": {
    "target_binary_sha256": "sha256:0000000000000000000000000000000000000000000000000000000000000000",
    "embedded_vcs_revision": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    "classifier_version": "v0.1.0",
    "source_revision": "cccccccccccccccccccccccccccccccccccccccc",
    "vcs_modified": false,
    "runtime_goos": "linux",
    "runtime_goarch": "amd64"
  },
  "corpus": {
    "manifest_sha256": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
    "journey_ids": ["j01-docs-happy-path", "j05-gate-without-any-review"]
  },
  "invocation": {
    "mode": "driven",
    "only": ["j01-docs-happy-path"]
  },
  "journeys": [
    {
      "id": "j01-docs-happy-path",
      "status": "completed",
      "metrics": {
        "blocks": {"in_band": 1, "out_of_band": 0, "dead_end": 0, "self_recovered": 0, "by_design": 0}
      },
      "commands": [
        {
          "sequence": 1,
          "step": "fixture: repo with remote",
          "args": ["git", "-C", "/tmp/gentle-ai-bench-j01-sandbox-abc123/home/demo", "status"]
        }
      ]
    }
  ]
}`

// TestReplacesSandboxPaths asserts raw sandbox paths are replaced with typed tokens
// and no raw sandbox paths appear in the canonical output. The canonical form
// drops individual commands (diagnostic-only) and keeps only the path-free identity
// tuple and aggregated outcomes; the replaceSandboxPaths step ensures that any
// future addition of path-bearing fields is caught.
func TestReplacesSandboxPaths(t *testing.T) {
	canonical, err := Canonicalize([]byte(rawResultWithSandboxPaths))
	if err != nil {
		t.Fatalf("Canonicalize error = %v", err)
	}

	// Must be valid JSON.
	var result Result
	if err := json.Unmarshal(canonical, &result); err != nil {
		t.Fatalf("canonical output is not valid JSON: %v\n%s", err, string(canonical))
	}

	// Identity must be preserved.
	if result.Identity.TargetBinarySHA256 != "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("target_binary_sha256 not preserved in canonical: got %q",
			result.Identity.TargetBinarySHA256)
	}
	if result.Identity.RuntimeGOOS != "linux" {
		t.Fatalf("runtime_goos not preserved: got %q", result.Identity.RuntimeGOOS)
	}

	// The raw JSON has sandbox paths in commands (diagnostic-only). After
	// canonicalization those are stripped and only aggregated outcomes remain.
	// We verify the raw string representation has no /tmp/gentle-ai-bench- paths.
	norm := strings.ReplaceAll(rawResultWithSandboxPaths, "\\", "/")
	replaced := replaceSandboxPaths(norm)
	if bytes.Contains([]byte(replaced), []byte("/tmp/gentle-ai-bench-")) {
		t.Fatalf("replaceSandboxPaths did not eliminate all raw sandbox paths from raw JSON:\n%s",
			replaced)
	}
	if !bytes.Contains([]byte(replaced), []byte("<REPOSITORY>")) {
		t.Fatalf("replaceSandboxPaths did not produce <REPOSITORY> token")
	}
	if !bytes.Contains([]byte(replaced), []byte("<TARGET_BINARY>")) {
		t.Fatalf("replaceSandboxPaths did not produce <TARGET_BINARY> token")
	}
}

// TestDigestStableForEquivalentRuns asserts two semantically identical raw results
// (different sandbox temp paths) produce identical canonical digests.
func TestDigestStableForEquivalentRuns(t *testing.T) {
	canonA, err := Canonicalize([]byte(rawResultWithSandboxPaths))
	if err != nil {
		t.Fatalf("Canonicalize A error = %v", err)
	}
	canonB, err := Canonicalize([]byte(rawResultEquivalent))
	if err != nil {
		t.Fatalf("Canonicalize B error = %v", err)
	}

	digestA := Digest(canonA)
	digestB := Digest(canonB)

	if digestA != digestB {
		t.Fatalf("Digest differs for equivalent runs:\n  digestA = %s\n  digestB = %s", digestA, digestB)
	}
}

// TestDifferentInputProducesDifferentDigest asserts that a changed identity
// (target SHA-256) produces a distinct canonical digest.
func TestDifferentInputProducesDifferentDigest(t *testing.T) {
	canonA, err := Canonicalize([]byte(rawResultWithSandboxPaths))
	if err != nil {
		t.Fatalf("Canonicalize A error = %v", err)
	}
	canonB, err := Canonicalize([]byte(rawResultDifferentInput))
	if err != nil {
		t.Fatalf("Canonicalize B error = %v", err)
	}

	digestA := Digest(canonA)
	digestB := Digest(canonB)

	if digestA == digestB {
		t.Fatal("Digest is identical for different inputs; digest must change when identity changes")
	}
}

// TestCanonicalizeResultIsValidJSON asserts the canonical output is always valid JSON.
func TestCanonicalizeResultIsValidJSON(t *testing.T) {
	canon, err := Canonicalize([]byte(rawResultWithSandboxPaths))
	if err != nil {
		t.Fatalf("Canonicalize error = %v", err)
	}

	var result Result
	if err := json.Unmarshal(canon, &result); err != nil {
		t.Fatalf("canonical output is not valid JSON: %v\n%s", err, string(canon))
	}

	// Verify identity and corpus are populated.
	if result.Identity.TargetBinarySHA256 == "" {
		t.Fatal("identity.target_binary_sha256 is empty in canonical result")
	}
	if result.Identity.RuntimeGOOS == "" {
		t.Fatal("identity.runtime_goos is empty in canonical result")
	}
}

// TestDigestFormat asserts Digest returns the "sha256:<lowercase-hex>" format.
func TestDigestFormat(t *testing.T) {
	digest := Digest([]byte("hello"))
	if digest != "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
		t.Fatalf("Digest returned unexpected value: %s", digest)
	}
}
