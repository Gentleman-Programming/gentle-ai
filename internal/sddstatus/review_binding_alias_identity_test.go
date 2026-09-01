package sddstatus

import (
	"context"
	"testing"
)

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
