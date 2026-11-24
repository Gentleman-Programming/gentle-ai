package reviewtransaction

import (
	"context"
	"strings"
	"testing"
)

// Issue #2498: rendering a repository path through fmt's %q escapes every
// backslash, so a Windows path prints with doubled separators in the exact
// invocation the user is told to copy-paste. These tests pin the correct
// behavior: the printed invocation contains the path as the filesystem knows
// it, quoted, with single separators, so strings.Contains(message, repo)
// holds on every platform.

func TestCompactAbandonCommandTextRendersWindowsPathVerbatim(t *testing.T) {
	repo := `C:\Users\dev\repo`
	text := compactAbandonCommandText(repo, "lineage-1", CompactAbandonEligibility{
		Eligible:         true,
		Revision:         "rev-1",
		SnapshotIdentity: "snap-1",
	})
	want := `--cwd "C:\Users\dev\repo"`
	if !strings.Contains(text, want) {
		t.Fatalf("abandon invocation does not contain the path as the filesystem knows it:\nwant substring: %s\ngot: %s", want, text)
	}
}

func TestCompactReclaimAuthorityRefusalRendersWindowsPathVerbatim(t *testing.T) {
	repo := `C:\Users\dev\repo`
	want := `--cwd "C:\Users\dev\repo"`
	err := compactReclaimAuthorityRefusal(context.Background(), repo, "reclaim-audit", "review-receipt.json")
	if err == nil {
		t.Fatal("reclaim accepted a store entry holding an authoritative artifact")
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("reclaim refusal does not contain the path as the filesystem knows it:\nwant substring: %s\ngot: %s", want, err)
	}
}
