package reviewtransaction

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestAssessAuthorityRepairAtRepositoryRootAddsNoGitSubprocess(t *testing.T) {
	repo := initSnapshotRepo(t)
	root, err := (SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	originalCommand := gitCommandContext
	t.Cleanup(func() { gitCommandContext = originalCommand })
	count := 0
	gitCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		count++
		return originalCommand(ctx, name, args...)
	}
	assessment, err := AssessAuthorityRepairAtRepositoryRoot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if err := assessment.Validate(); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("authority repair assessment added %d Git subprocesses after STATUS root resolution", count)
	}
}

func TestAssessAuthorityRepairStopsLockedAndTruncatedInventory(t *testing.T) {
	t.Run("store lock", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		approvedCompactFixture(t, repo, "repair-store-lock")
		root, _, err := reviewAuthorityRoot(context.Background(), repo)
		if err != nil {
			t.Fatal(err)
		}
		lock, err := acquireStoreLock(filepath.Join(root, "v2", "LOCK"))
		if err != nil {
			t.Fatal(err)
		}
		defer lock.release()
		assessment, err := AssessAuthorityRepair(context.Background(), repo)
		if err != nil || assessment.Status != AuthorityRepairConflicting || assessment.Candidate != nil {
			t.Fatalf("store-lock assessment = %#v, %v", assessment, err)
		}
	})

	t.Run("maintenance lock", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		approvedCompactFixture(t, repo, "repair-maintenance-lock")
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		lock, err := AcquireReviewMaintenanceExclusive(ctx, repo)
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Release()
		assessment, err := AssessAuthorityRepair(context.Background(), repo)
		if err != nil || assessment.Status != AuthorityRepairConflicting || assessment.Candidate != nil {
			t.Fatalf("maintenance-lock assessment = %#v, %v", assessment, err)
		}
	})

	t.Run("oversized state", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		_, store := approvedCompactFixture(t, repo, "repair-oversized")
		if err := os.WriteFile(store.StatePath(), bytes.Repeat([]byte("x"), authorityRepairMaxEventBytes+1), 0o644); err != nil {
			t.Fatal(err)
		}
		assessment, err := AssessAuthorityRepair(context.Background(), repo)
		if err != nil || assessment.Status != AuthorityRepairTruncated || assessment.Candidate != nil {
			t.Fatalf("truncated assessment = %#v, %v", assessment, err)
		}
		// issue #3371: even a truncation that trips deep inside per-lineage
		// reading (not the raw directory listing) must say so and how far it
		// got, not leave Counts looking like a healthy, nothing-to-repair
		// store.
		if !assessment.Truncated || assessment.TruncationCap != authorityRepairMaxLineages || assessment.TruncationScanned < 1 {
			t.Fatalf("truncated assessment scan bound = %#v", assessment)
		}
	})

	t.Run("too many unexpected entries", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		root, _, err := reviewAuthorityRoot(context.Background(), repo)
		if err != nil {
			t.Fatal(err)
		}
		versionRoot := filepath.Join(root, "v2")
		if err := os.MkdirAll(versionRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		// authorityRepairMaxLineages+1 = 257 entries: past the 256-lineage
		// cap from issue #3371 ("gentle-ai review repair --preflight is
		// blind past 256 lineages").
		for index := 0; index <= authorityRepairMaxLineages; index++ {
			path := filepath.Join(versionRoot, fmt.Sprintf("unexpected-%03d.json", index))
			if err := os.WriteFile(path, []byte("residue\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		assessment, err := AssessAuthorityRepair(context.Background(), repo)
		if err != nil || assessment.Status != AuthorityRepairTruncated || assessment.Candidate != nil {
			t.Fatalf("entry-bound assessment = %#v, %v", assessment, err)
		}
		// issue #3371's exact reported shape: hitting the cap at the raw
		// directory listing, before a single entry is classified, must never
		// report all-zero counts as if the store were healthy. It must say
		// truncated=true with the cap (256) and how many entries it actually
		// saw (257, the exact bound-plus-one ReadDir returned).
		if assessment.Counts != (AuthorityRepairCounts{}) {
			t.Fatalf("truncated assessment reported non-zero classified counts = %#v", assessment.Counts)
		}
		if !assessment.Truncated || assessment.TruncationCap != authorityRepairMaxLineages || assessment.TruncationScanned != authorityRepairMaxLineages+1 {
			t.Fatalf("entry-bound assessment scan bound = %#v, want truncated=true cap=%d scanned=%d",
				assessment, authorityRepairMaxLineages, authorityRepairMaxLineages+1)
		}
	})
}

// The assessment no longer reads or classifies legacy v1 authority: a v1 store
// alone yields the same stopped, candidate-free assessment as no store.
func TestAssessAuthorityRepairIgnoresLegacyV1Store(t *testing.T) {
	repo := initSnapshotRepo(t)
	approvedCompactFixture(t, repo, "repair-v1-bystander")
	v1Root := legacyV1Fixture(t, repo)
	before := legacyV1TreeDigest(t, v1Root)
	assessment, err := AssessAuthorityRepair(context.Background(), repo)
	if err != nil || assessment.Status != AuthorityRepairUnsupported || assessment.Candidate != nil ||
		assessment.Counts.LegacyLineages != 0 || assessment.Counts.Conflicts != 0 || assessment.Counts.UnsupportedLineages != 0 {
		t.Fatalf("assessment with a v1 store = %#v, %v", assessment, err)
	}
	if err := assessment.Validate(); err != nil {
		t.Fatal(err)
	}
	if after := legacyV1TreeDigest(t, v1Root); after != before {
		t.Fatal("assessment modified the v1 store")
	}
}
