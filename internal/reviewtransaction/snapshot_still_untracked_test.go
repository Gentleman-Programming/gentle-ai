package reviewtransaction

import (
	"context"
	"reflect"
	"testing"
)

// Issue #3842: StillUntrackedIntended reconciles a HISTORICAL intended-
// untracked selection against the real index — paths that became tracked are
// dropped, still-untracked paths survive in their recorded order, and a
// selection that landed completely returns a non-nil empty slice because
// snapshot targets demand an explicit selection, never an absent one.
func TestSnapshotBuilderStillUntrackedIntendedReconcilesTrackedPaths(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "keep-a.txt", "a\n")
	writeSnapshotFile(t, repo, "keep-b.txt", "b\n")
	builder := SnapshotBuilder{Repo: repo}

	got, err := builder.StillUntrackedIntended(context.Background(), []string{"keep-b.txt", "tracked.txt", "keep-a.txt"})
	if err != nil {
		t.Fatalf("StillUntrackedIntended over a mixed selection: %v", err)
	}
	// Recorded order is preserved; only the tracked path drops out.
	if want := []string{"keep-b.txt", "keep-a.txt"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("StillUntrackedIntended() = %v, want %v", got, want)
	}

	landed, err := builder.StillUntrackedIntended(context.Background(), []string{"tracked.txt", "deleted.txt"})
	if err != nil {
		t.Fatalf("StillUntrackedIntended over a fully-landed selection: %v", err)
	}
	if landed == nil || len(landed) != 0 {
		t.Fatalf("fully-landed selection = %#v, want a non-nil empty slice", landed)
	}

	// An empty historical selection short-circuits without touching the
	// repository at all — nil stays nil so absent provenance stays absent.
	empty, err := builder.StillUntrackedIntended(context.Background(), nil)
	if err != nil || empty != nil {
		t.Fatalf("empty selection = %#v err=%v, want nil unchanged", empty, err)
	}
}
