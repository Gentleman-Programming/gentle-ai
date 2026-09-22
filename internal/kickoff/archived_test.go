package kickoff

import (
	"path/filepath"
	"testing"
)

// TestRefuseArchivedRootRejectsArchivedChange is D-14's core rule: no
// write path this package owns may target a change root already living
// under openspec/changes/archive/.
func TestRefuseArchivedRootRejectsArchivedChange(t *testing.T) {
	workspaceRoot := t.TempDir()
	changeRoot := filepath.Join(workspaceRoot, "openspec", "changes", "archive", "2026-01-01-x")
	if err := RefuseArchivedRoot(workspaceRoot, changeRoot); err == nil {
		t.Fatal("RefuseArchivedRoot() = nil, se esperaba rechazo para una raiz bajo archive/")
	}
}

// TestRefuseArchivedRootAcceptsActiveChange is the mirror case: an active
// (non-archived) change root must not be rejected.
func TestRefuseArchivedRootAcceptsActiveChange(t *testing.T) {
	workspaceRoot := t.TempDir()
	changeRoot := filepath.Join(workspaceRoot, "openspec", "changes", "x")
	if err := RefuseArchivedRoot(workspaceRoot, changeRoot); err != nil {
		t.Fatalf("RefuseArchivedRoot() error = %v, se esperaba nil para un cambio activo", err)
	}
}

// TestRefuseArchivedRootRejectsEscapeAboveWorkspace covers T-7's "raiz de
// cambio" vector: a changeRoot that resolves above workspaceRoot via ".."
// must never be accepted, regardless of whether it happens to land under
// an archive/-looking path or not.
func TestRefuseArchivedRootRejectsEscapeAboveWorkspace(t *testing.T) {
	workspaceRoot := t.TempDir()
	changeRoot := filepath.Join(workspaceRoot, "..", "otro")
	if err := RefuseArchivedRoot(workspaceRoot, changeRoot); err == nil {
		t.Fatal("RefuseArchivedRoot() = nil, se esperaba rechazo por contencion (.. escapa del workspace)")
	}
}

// TestRefuseArchivedRootRejectsAbsolutePathOutsideWorkspace covers the
// second T-7 containment vector: an absolute path naming a location that
// is not inside workspaceRoot at all.
func TestRefuseArchivedRootRejectsAbsolutePathOutsideWorkspace(t *testing.T) {
	workspaceRoot := t.TempDir()
	unrelatedRoot := t.TempDir()
	changeRoot := filepath.Join(unrelatedRoot, "openspec", "changes", "x")
	if err := RefuseArchivedRoot(workspaceRoot, changeRoot); err == nil {
		t.Fatal("RefuseArchivedRoot() = nil, se esperaba rechazo por contencion (ruta absoluta fuera del workspace)")
	}
}
