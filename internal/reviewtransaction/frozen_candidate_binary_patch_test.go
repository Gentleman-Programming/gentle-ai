package reviewtransaction

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPatchInspectionNeverEmitsBinaryContentBytes is the regression guard for
// issue #3193. A reviewer holds no tools and cannot skip past what a prompt
// contains, so every byte the patch operation emits is a byte the reviewer must
// carry. Emitting the raw content of a path Git classifies as binary buys
// nothing a text reviewer can act on and costs the whole payload: one candidate
// carrying a PDF filled a lens prompt to roughly 114K tokens of blob.
//
// It is not only volume. Arbitrary content bytes are also arbitrary control
// material for whatever assembles the prompt downstream: an `@\` sequence
// inside a blob made one host's file-mention resolver staple a drive-root
// attachment onto every lens launch. The manifest entry, the mode, and Git's
// own "Binary files ... differ" line carry everything a lens can legitimately
// triage from, so the bytes themselves are pure liability.
func TestPatchInspectionNeverEmitsBinaryContentBytes(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)

	// A PNG header plus a NUL is what Git's own heuristic classifies as binary,
	// and the marker is content no legitimate patch summary would ever repeat.
	const marker = "GENTLE-AI-3193-BLOB-MARKER"
	base := append([]byte("\x89PNG\r\n\x1a\n\x00"), []byte("base "+marker+"\n")...)
	candidate := append([]byte("\x89PNG\r\n\x1a\n\x00"), []byte("candidate "+marker+"\x01\n")...)

	if err := os.WriteFile(filepath.Join(repo, "docs", "poster.png"), base, 0o644); err != nil {
		if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, "docs", "poster.png"), base, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitSnapshot(t, repo, "add", "--", "docs/poster.png")
	gitSnapshot(t, repo, "commit", "-m", "add binary fixture")

	if err := os.WriteFile(filepath.Join(repo, "docs", "poster.png"), candidate, 0o644); err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, repo, "add", "-A", "--")
	gitSnapshot(t, repo, "commit", "-m", "change binary fixture")
	candidateCommit := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))

	builder := SnapshotBuilder{Repo: repo}
	snapshot, err := builder.Build(context.Background(), Target{Kind: TargetExactRevision, Revision: candidateCommit})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	frozen, err := builder.FrozenCandidateContext(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("FrozenCandidateContext() error = %v", err)
	}

	index := -1
	for position, entry := range frozen.ChangedPathManifest {
		if entry.Path == "docs/poster.png" {
			index = position
		}
	}
	if index < 0 {
		t.Fatalf("binary fixture missing from manifest: %+v", frozen.ChangedPathManifest)
	}

	payload, err := builder.InspectCandidate(context.Background(), snapshot, "patch", index, "")
	if err != nil {
		t.Fatalf("InspectCandidate(patch) error = %v", err)
	}

	if bytes.Contains(payload, []byte(marker)) {
		t.Errorf("patch for a binary path emitted its content bytes (%d bytes):\n%q", len(payload), payload)
	}
	// The evidence must still exist: a silently empty patch is the one shape
	// that would let a reviewer report a clean result over an uninspected path,
	// and lens-context refuses it outright.
	if len(bytes.TrimSpace(payload)) == 0 {
		t.Fatal("patch for a binary path produced no bytes at all; the path must still be reported as changed")
	}
	if !bytes.Contains(payload, []byte("docs/poster.png")) {
		t.Errorf("patch for a binary path does not name the path:\n%q", payload)
	}
}
