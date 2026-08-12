package reviewtransaction

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestLegacyCloneOverrideStillDecides is the migration guard for #2882.
//
// Moving the switch out of the review authority tree must never re-enable
// reviews for someone who already disabled them. An override written at the
// pre-#2882 location keeps deciding, because the read path falls back to it
// whenever the switch's own root holds no generation.
func TestLegacyCloneOverrideStillDecides(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)

	// Write through the current writer, then relocate the record to the
	// legacy path so the only thing under test is where reads look.
	if _, err := SetCloneLocalRDDMode(ctx, repo, RDDModeOff, "", RDDGlobalMode{}); err != nil {
		t.Fatalf("SetCloneLocalRDDMode() error = %v", err)
	}
	current, err := cloneLocalRDDModeRoot(ctx, repo, false)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := cloneLocalRDDModeLegacyRoot(ctx, repo)
	if err != nil {
		// The legacy tree is created lazily; build it with the same helpers.
		identity, idErr := cloneLocalRDDModeIdentity(ctx, repo)
		if idErr != nil {
			t.Fatal(idErr)
		}
		base := filepath.Join(identity.GitCommonDir, "gentle-ai", "review-transactions", rarAuthorityDirectory, rarAuthorityVersion)
		if err := ensureRARRepositoryRoot(identity.GitCommonDir, base, true); err != nil {
			t.Fatal(err)
		}
		legacy = filepath.Join(base, rddModeDirectory)
		if err := ensurePrivateRARDirectoryTree(base, legacy, true); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := os.ReadDir(current)
	if err != nil {
		t.Fatal(err)
	}
	moved := 0
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == rddModeLockName {
			continue
		}
		if renameErr := os.Rename(filepath.Join(current, entry.Name()), filepath.Join(legacy, entry.Name())); renameErr != nil {
			t.Fatal(renameErr)
		}
		moved++
	}
	if moved == 0 {
		t.Fatal("no override record to relocate; the writer stored nothing")
	}

	status, err := ResolveRDDMode(ctx, repo, RDDGlobalMode{})
	if err != nil {
		t.Fatalf("ResolveRDDMode() error = %v", err)
	}
	if status.Effective != RDDModeOff || status.Source != RDDModeSourceCloneLocal {
		t.Fatalf("a pre-#2882 clone override stopped deciding: %#v; the move silently re-enabled reviews", status)
	}
}
