package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Issue #2523: RunArgs is the exact dispatch the shipped binary runs, so the
// argvs below take the same path a user's shell invocation takes. The two
// unambiguous SHA-256 spellings (64-hex containing uppercase, prefixed or
// bare, as PowerShell's Get-FileHash prints, and bare lowercase 64-hex)
// canonicalize at the argument boundary; every other value still refuses
// with the ledger's own message (#2395).

const canonSDDChange = "revision-canon-change"
const canonEvidenceHexLower = "27a44ad82d6e4f1d9f2c3b4a5d6e7f8091a2b3c4d5e6f70818293a4b5c6d7e8f"

func initCanonRepo(t *testing.T) string {
	t.Helper()
	repo, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q"}, {"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"}, {"config", "core.autocrlf", "false"},
		{"add", "tracked.txt"}, {"commit", "-qm", "base"},
	} {
		command := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	return repo
}

func canonRunJSON(t *testing.T, target any, args ...string) {
	t.Helper()
	var output bytes.Buffer
	if err := RunArgs(args, &output); err != nil {
		t.Fatalf("RunArgs(%v): %v", args, err)
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output.String())), target); err != nil {
		t.Fatalf("parse %v output: %v\n%s", args, err, output.String())
	}
}

func canonAcquireToken(t *testing.T, repo, requestID string) string {
	t.Helper()
	var result struct {
		State string `json:"state"`
		Token string `json:"token"`
	}
	canonRunJSON(t, &result, "sdd-attempt", "acquire", "--cwd", repo, "--change", canonSDDChange,
		"--request-id", requestID, "--work-unit", "canon-unit",
		"--evidence-goal", "prove canonical revision input",
		"--max-attempts", "4", "--max-changed-lines", "20")
	if result.State != "proceed" || result.Token == "" {
		t.Fatalf("acquire = %+v, want proceed with a token", result)
	}
	return result.Token
}

func canonSettle(repo, token, requestID, evidenceRevision string) error {
	return RunArgs([]string{
		"sdd-attempt", "settle", "--cwd", repo, "--change", canonSDDChange,
		"--token", token, "--request-id", requestID, "--outcome", "failed",
		"--evidence-revision", evidenceRevision,
		"--diagnosis", "attempt produced conclusive evidence", "--harness-disposition", "reused",
		"--cleanup-evidence", "process group exited", "--process-evidence", "no descendants remained",
	}, &bytes.Buffer{})
}

// TestSDDAttemptSettleRevisionInputCanonicalization pins both sides of the
// boundary: the unambiguous non-canonical spellings settle identically to the
// canonical one with the ledger recording the canonical form, and everything
// else reaches the ledger byte-for-byte and refuses exactly as before.
func TestSDDAttemptSettleRevisionInputCanonicalization(t *testing.T) {
	canonical := "sha256:" + canonEvidenceHexLower
	cases := []struct {
		name    string
		input   string
		refused bool
	}{
		{"uppercase prefixed (sha256: plus Get-FileHash case)", "sha256:" + strings.ToUpper(canonEvidenceHexLower), false},
		{"uppercase bare (PowerShell Get-FileHash default)", strings.ToUpper(canonEvidenceHexLower), false},
		{"lowercase bare (shasum default)", canonEvidenceHexLower, false},
		{"canonical passthrough", canonical, false},
		{"63 hex chars", canonEvidenceHexLower[:63], true},
		{"non-hex", "g" + canonEvidenceHexLower[1:], true},
		{"wrong prefix", "sha1:" + canonEvidenceHexLower, true},
	}
	for index, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			repo := initCanonRepo(t)
			token := canonAcquireToken(t, repo, fmt.Sprintf("canon-acquire-%d", index))
			err := canonSettle(repo, token, fmt.Sprintf("canon-settle-%d", index), tt.input)
			if tt.refused {
				if err == nil || !strings.Contains(err.Error(), "evidence_revision must be sha256:<64-lowercase-hex>") {
					t.Fatalf("settle with %s = %v, want the unchanged canonical-format refusal", tt.name, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("settle with %s = %v, want success", tt.name, err)
			}
			var status struct {
				EvidenceRevision string `json:"evidence_revision"`
			}
			canonRunJSON(t, &status, "sdd-attempt", "status", "--cwd", repo, "--change", canonSDDChange)
			if status.EvidenceRevision != canonical {
				t.Fatalf("recorded evidence revision = %q, want %q", status.EvidenceRevision, canonical)
			}
		})
	}
}
