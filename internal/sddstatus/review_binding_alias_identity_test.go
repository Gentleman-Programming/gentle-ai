//go:build legacy_compact_receipt

package sddstatus

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/pathidentity"
)

// aliasedRepository builds a repository that is reachable under two different
// spellings: its real path, and the same path addressed through a symlinked
// ancestor. That is the shape /var and /private/var produce on macOS, where
// $TMPDIR lives under both.
func aliasedRepository(t *testing.T, change string) (realRepo string, aliasedRepo string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	realRepo = filepath.Join(base, "real", "repo")
	if err := os.MkdirAll(realRepo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "real"), filepath.Join(base, "alias")); err != nil {
		t.Fatal(err)
	}
	changeRoot := seedReadyChange(t, realRepo, change, "- [x] 1.1 Done\n")
	writeApprovedCompactAuthorityForChange(t, realRepo, changeRoot, change+"-lineage")
	return realRepo, filepath.Join(base, "alias", "repo")
}

// TestResolveBindingChangeRootAcceptsAnAliasedRepositoryRoot pins 1773
// boundary 1. resolveBindingChangeRoot canonicalized the workspace with
// filepath.EvalSymlinks and the root with filepath.Clean only, so two
// spellings of one repository compared unequal and the planning workspace was
// reported outside its own repository.
func TestResolveBindingChangeRootAcceptsAnAliasedRepositoryRoot(t *testing.T) {
	realRepo, aliasedRepo := aliasedRepository(t, "thin")

	fromReal, err := resolveBindingChangeRoot(context.Background(), realRepo, realRepo, "thin")
	if err != nil {
		t.Fatalf("resolveBindingChangeRoot(real root, real workspace) error = %v", err)
	}

	fromAliasedRoot, err := resolveBindingChangeRoot(context.Background(), aliasedRepo, realRepo, "thin")
	if err != nil {
		t.Fatalf("resolveBindingChangeRoot(aliased root, real workspace) error = %v", err)
	}
	if fromAliasedRoot != fromReal {
		t.Fatalf("aliased root resolved %q, real root resolved %q", fromAliasedRoot, fromReal)
	}

	fromAliasedWorkspace, err := resolveBindingChangeRoot(context.Background(), realRepo, aliasedRepo, "thin")
	if err != nil {
		t.Fatalf("resolveBindingChangeRoot(real root, aliased workspace) error = %v", err)
	}
	if fromAliasedWorkspace != fromReal {
		t.Fatalf("aliased workspace resolved %q, real root resolved %q", fromAliasedWorkspace, fromReal)
	}

	fromBothAliased, err := resolveBindingChangeRoot(context.Background(), aliasedRepo, aliasedRepo, "thin")
	if err != nil {
		t.Fatalf("resolveBindingChangeRoot(aliased root, aliased workspace) error = %v", err)
	}
	if fromBothAliased != fromReal {
		t.Fatalf("both-aliased resolved %q, real root resolved %q", fromBothAliased, fromReal)
	}
}

// TestResolveBindingChangeRootStillRejectsAForeignWorkspace keeps the
// containment decision a real decision: accepting a second spelling of the
// same repository must not accept a different repository.
func TestResolveBindingChangeRootStillRejectsAForeignWorkspace(t *testing.T) {
	realRepo, _ := aliasedRepository(t, "thin")
	foreign := t.TempDir()
	if _, err := resolveBindingChangeRoot(context.Background(), realRepo, foreign, "thin"); err == nil {
		t.Fatal("a workspace outside the selected repository was accepted")
	}
}

// TestBindApprovedReviewThroughAnAliasedSpelling is the end-to-end positive
// control the reporter already had passing: a full bind through the aliased
// spelling must keep working, and must produce the same binding as the real
// spelling.
func TestBindApprovedReviewThroughAnAliasedSpelling(t *testing.T) {
	realRepo, aliasedRepo := aliasedRepository(t, "thin")

	binding, err := BindApprovedReview(context.Background(), aliasedRepo, "thin", "thin-lineage", "")
	if err != nil {
		t.Fatalf("BindApprovedReview through the aliased spelling: %v", err)
	}
	if binding.Revision == "" {
		t.Fatalf("binding = %#v", binding)
	}

	retry, err := BindApprovedReview(context.Background(), realRepo, "thin", "thin-lineage", binding.Revision)
	if err != nil {
		t.Fatalf("BindApprovedReview retry through the real spelling: %v", err)
	}
	if retry.Revision != binding.Revision {
		t.Fatalf("real spelling produced revision %q, aliased spelling produced %q", retry.Revision, binding.Revision)
	}

	fromAliased, err := Resolve(ResolveOptions{CWD: aliasedRepo, ChangeName: "thin"})
	if err != nil {
		t.Fatalf("Resolve through the aliased spelling: %v", err)
	}
	fromReal, err := Resolve(ResolveOptions{CWD: realRepo, ChangeName: "thin"})
	if err != nil {
		t.Fatalf("Resolve through the real spelling: %v", err)
	}
	// PlanningHome.Path is a status display field and deliberately echoes the
	// caller's spelling, so the question here is identity, not string equality:
	// both spellings must report the one planning home.
	if !pathidentity.SameDirectory(fromAliased.PlanningHome.Path, fromReal.PlanningHome.Path) {
		t.Fatalf("aliased planning home %q and real planning home %q are not the same directory",
			fromAliased.PlanningHome.Path, fromReal.PlanningHome.Path)
	}
	if fromAliased.ReviewGate == nil || fromReal.ReviewGate == nil ||
		fromAliased.ReviewGate.Result != fromReal.ReviewGate.Result {
		t.Fatalf("aliased gate %#v, real gate %#v", fromAliased.ReviewGate, fromReal.ReviewGate)
	}
}
