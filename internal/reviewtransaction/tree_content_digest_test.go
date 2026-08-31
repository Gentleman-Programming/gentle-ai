package reviewtransaction

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeDigestFile(t *testing.T, repo, logicalPath, content string) {
	t.Helper()
	full := filepath.Join(repo, filepath.FromSlash(logicalPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func digestTree(t *testing.T, repo string) string {
	t.Helper()
	if err := runSnapshotGit(repo, "add", "-A"); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("git", "-C", repo, "write-tree").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(output))
}

// TestTreeContentDigestIsStableAcrossWritingTheExcludedArtifact is the property
// the digest exists for: an artifact stored inside a tree cannot name that
// tree's OID, because writing it changes the OID. Excluding the artifact's own
// path keeps the identity of everything else fixed.
func TestTreeContentDigestIsStableAcrossWritingTheExcludedArtifact(t *testing.T) {
	repo := initSnapshotRepo(t)
	const report = "openspec/changes/c/verify-report.md"
	writeDigestFile(t, repo, "docs/guide.md", "# Guide\n")
	before := digestTree(t, repo)

	ctx := context.Background()
	classified, err := TreeContentDigest(ctx, repo, before, []string{report})
	if err != nil {
		t.Fatal(err)
	}

	writeDigestFile(t, repo, report, "the report naming "+classified+"\n")
	after := digestTree(t, repo)
	if before == after {
		t.Fatal("writing the report did not change the tree; the test proves nothing")
	}

	settled, err := TreeContentDigest(ctx, repo, after, []string{report})
	if err != nil {
		t.Fatal(err)
	}
	if settled != classified {
		t.Fatalf("digest moved when the excluded artifact was written: %q then %q", classified, settled)
	}
}

// TestTreeContentDigestMovesWithEveryContentChange keeps the exclusion narrow:
// it hides one named path, never a real change elsewhere.
func TestTreeContentDigestMovesWithEveryContentChange(t *testing.T) {
	const report = "openspec/changes/c/verify-report.md"
	ctx := context.Background()
	baseline := func(t *testing.T) (string, string) {
		t.Helper()
		repo := initSnapshotRepo(t)
		writeDigestFile(t, repo, "docs/guide.md", "# Guide\n")
		writeDigestFile(t, repo, report, "report\n")
		digest, err := TreeContentDigest(ctx, repo, digestTree(t, repo), []string{report})
		if err != nil {
			t.Fatal(err)
		}
		return repo, digest
	}
	tests := []struct {
		name   string
		mutate func(t *testing.T, repo string)
	}{
		{name: "an edit to a covered path", mutate: func(t *testing.T, repo string) {
			writeDigestFile(t, repo, "docs/guide.md", "# Guide\n\nchanged\n")
		}},
		{name: "an added path", mutate: func(t *testing.T, repo string) {
			writeDigestFile(t, repo, "internal/run.go", "package internal\n")
		}},
		{name: "a removed path", mutate: func(t *testing.T, repo string) {
			if err := os.Remove(filepath.Join(repo, "docs", "guide.md")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "a mode change", mutate: func(t *testing.T, repo string) {
			if err := os.Chmod(filepath.Join(repo, "docs", "guide.md"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, before := baseline(t)
			tt.mutate(t, repo)
			after, err := TreeContentDigest(ctx, repo, digestTree(t, repo), []string{report})
			if err != nil {
				t.Fatal(err)
			}
			if after == before {
				t.Fatal("the digest did not move for a real content change")
			}
		})
	}
}

// TestTreeContentDigestRejectsAMalformedTree keeps it fail-closed.
func TestTreeContentDigestRejectsAMalformedTree(t *testing.T) {
	repo := initSnapshotRepo(t)
	for _, tree := range []string{"", "not-a-tree", "0000000000000000000000000000000000000000"} {
		if _, err := TreeContentDigest(context.Background(), repo, tree, nil); err == nil {
			t.Fatalf("accepted malformed tree %q", tree)
		}
	}
}

// TestTreeContentDigestMovesWithASubmodulePointer covers content the tree owns
// but stores as a gitlink rather than a blob. An external review pass found the
// digest skipping these: a candidate whose submodule pointer moved kept the
// identity it had before, so a report describing the old candidate went on
// attesting the new one.
func TestTreeContentDigestMovesWithASubmodulePointer(t *testing.T) {
	repo := initSnapshotRepo(t)
	sub := filepath.Join(repo, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "sub@example.com"}, {"config", "user.name", "Sub"}} {
		if err := runSnapshotGit(sub, args...); err != nil {
			t.Fatal(err)
		}
	}
	writeDigestFile(t, sub, "f.txt", "v1\n")
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-qm", "v1"}} {
		if err := runSnapshotGit(sub, args...); err != nil {
			t.Fatal(err)
		}
	}
	writeDigestFile(t, repo, "docs/guide.md", "# Guide\n")

	ctx := context.Background()
	before, err := TreeContentDigest(ctx, repo, digestTree(t, repo), nil)
	if err != nil {
		t.Fatal(err)
	}

	writeDigestFile(t, sub, "f.txt", "v2\n")
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-qm", "v2"}} {
		if err := runSnapshotGit(sub, args...); err != nil {
			t.Fatal(err)
		}
	}
	after, err := TreeContentDigest(ctx, repo, digestTree(t, repo), nil)
	if err != nil {
		t.Fatal(err)
	}
	if after == before {
		t.Fatal("the digest ignored a moved submodule pointer, so a stale report would keep attesting")
	}
}

// TestTreeContentDigestSeparatesPathsUnambiguously proves two different
// candidates cannot render the same record stream. Git permits a newline in a
// path, so a plain separator would let one candidate's path spill into the next
// record's position.
func TestTreeContentDigestSeparatesPathsUnambiguously(t *testing.T) {
	ctx := context.Background()
	digestOf := func(t *testing.T, paths map[string]string) string {
		t.Helper()
		repo := initSnapshotRepo(t)
		for logicalPath, content := range paths {
			writeDigestFile(t, repo, logicalPath, content)
		}
		digest, err := TreeContentDigest(ctx, repo, digestTree(t, repo), nil)
		if err != nil {
			t.Fatal(err)
		}
		return digest
	}
	// Same bytes, different path spellings that a newline-delimited record
	// stream could render identically.
	first := digestOf(t, map[string]string{"a\nb.md": "x\n"})
	second := digestOf(t, map[string]string{"a": "x\n", "b.md": "x\n"})
	if first == second {
		t.Fatal("two different candidates produced the same digest")
	}
}
