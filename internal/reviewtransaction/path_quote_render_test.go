package reviewtransaction

import (
	"context"
	"os"
	"path/filepath"
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

func TestCompactRepairCommandTextRendersWindowsPathVerbatim(t *testing.T) {
	repo := `C:\Users\dev\repo`
	text := compactRepairCommandText(repo, AuthorityDispositionPlan{
		AuthorityInventoryRevision: "inv-rev-1",
		PlanDigest:                 "digest-1",
	})
	want := `--cwd "C:\Users\dev\repo"`
	if got := strings.Count(text, want); got != 2 {
		t.Fatalf("repair invocation renders --cwd twice and both must carry the verbatim path:\nwant 2 occurrences of %s, got %d\ngot: %s", want, got, text)
	}
}

func TestCompactReclaimAuthorityRefusalRendersWindowsPathVerbatim(t *testing.T) {
	repo := `C:\Users\dev\repo`
	want := `--cwd "C:\Users\dev\repo"`
	tests := []struct {
		name     string
		populate func(t *testing.T, dir string)
	}{
		{
			// No review-state.json beside the artifact: nothing can prove the
			// artifact never carried authority, so the refusal names the
			// inspect-authority escalation.
			name:     "missing record",
			populate: func(*testing.T, string) {},
		},
		{
			// A record an interrupted write left unreadable also routes to the
			// inspect-authority escalation.
			name: "unreadable record",
			populate: func(t *testing.T, dir string) {
				writeReclaimFixtureFile(t, filepath.Join(dir, compactStateFileName), "{malformed")
			},
		},
		{
			// A readable record whose abandonment eligibility cannot be proven
			// falls through to the no-advertised-operation escalation. The
			// eligibility probe runs against the rendered repo path, which
			// does not exist here, so the fail-closed default renders.
			name: "no advertised operation",
			populate: func(t *testing.T, dir string) {
				state := newCompactTestState(t, initSnapshotRepo(t), "reclaim-audit")
				_, payload, err := makeCompactRecord(state)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, compactStateFileName), payload, 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.populate(t, dir)
			err := compactReclaimAuthorityRefusal(context.Background(), repo, dir, "reclaim-audit", "review-receipt.json")
			if err == nil {
				t.Fatal("reclaim accepted a store entry holding an authoritative artifact")
			}
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("reclaim refusal does not contain the path as the filesystem knows it:\nwant substring: %s\ngot: %s", want, err)
			}
		})
	}
}
