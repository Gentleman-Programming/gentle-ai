package sddstatus

import (
	"path/filepath"
	"strings"
	"testing"
)

// #4002: an archived change is a positive terminal fact, not a blocker.
// OpenSpec proves closure by the folder that moved into
// openspec/changes/archive/YYYY-MM-DD-<change>; naming that change used to
// answer "Active OpenSpec change not found" through the blocked channel, so an
// orchestrator could not tell a finished change from a typo. These tests pin
// the positive projection gentle-pi already ships (gentle-pi#538): terminal,
// unblocked, nextRecommended "archived", and the archive location on the wire.

func allDoneStatusDependencies() Dependencies {
	return Dependencies{
		Proposal: DependencyAllDone,
		Specs:    DependencyAllDone,
		Design:   DependencyAllDone,
		Tasks:    DependencyAllDone,
		Apply:    DependencyAllDone,
		Verify:   DependencyAllDone,
		Archive:  DependencyAllDone,
	}
}

func assertArchivedTerminal(t *testing.T, status Status, wantPath string) {
	t.Helper()
	if status.NextRecommended != "archived" {
		t.Fatalf("NextRecommended = %q, want %q", status.NextRecommended, "archived")
	}
	if len(status.BlockedReasons) != 0 {
		t.Fatalf("BlockedReasons = %v, want none: archived is a positive terminal state", status.BlockedReasons)
	}
	if status.Dependencies != allDoneStatusDependencies() {
		t.Fatalf("Dependencies = %#v, want every phase all_done", status.Dependencies)
	}
	if status.ApplyState != ApplyAllDone {
		t.Fatalf("ApplyState = %q, want %q", status.ApplyState, ApplyAllDone)
	}
	if status.Archived == nil {
		t.Fatal("Archived is nil, want the archive location projected")
	}
	if status.Archived.Path != wantPath {
		t.Fatalf("Archived.Path = %q, want %q", status.Archived.Path, wantPath)
	}
	// The change is closed; never route back to the archive phase (#3480).
	if status.NextRecommended == string(PhaseArchive) || status.Dependencies.Archive == DependencyReady {
		t.Fatalf("archived change routed to archive again: archive %q next %q", status.Dependencies.Archive, status.NextRecommended)
	}
}

func TestNamedArchivedOpenSpecChangeProjectsPositiveTerminal(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "openspec", "changes", "archive", "2026-08-30-wave-one", "proposal.md"), "# Proposal\n")

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "wave-one"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.ArtifactStore != ArtifactStoreOpenSpec {
		t.Fatalf("ArtifactStore = %q, want %q", status.ArtifactStore, ArtifactStoreOpenSpec)
	}
	if status.ChangeName == nil || *status.ChangeName != "wave-one" {
		t.Fatalf("ChangeName = %v, want wave-one", ptrValue(status.ChangeName))
	}
	assertArchivedTerminal(t, status, filepath.Join("openspec", "changes", "archive", "2026-08-30-wave-one"))
}

func TestArchivedOpenSpecChangeNewestDateWins(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "openspec", "changes", "archive", "2026-01-02-wave-one"))
	mkdir(t, filepath.Join(root, "openspec", "changes", "archive", "2026-03-04-wave-one"))
	mkdir(t, filepath.Join(root, "openspec", "changes", "archive", "2025-12-31-wave-one"))

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "wave-one"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	assertArchivedTerminal(t, status, filepath.Join("openspec", "changes", "archive", "2026-03-04-wave-one"))
}

// TestArchivedOpenSpecChangeRequiresExactSuffix guards the match discipline: a
// change name matches only the exact suffix after the 11-character date
// prefix. "foo-bar" must not claim "2026-01-01-foo-bar-baz", "bar" must not
// claim "2026-01-01-foo-bar", and an entry without a date prefix is not an
// archive entry.
func TestArchivedOpenSpecChangeRequiresExactSuffix(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "openspec", "changes", "archive", "2026-01-01-foo-bar-baz"))
	mkdir(t, filepath.Join(root, "openspec", "changes", "archive", "foo-bar"))

	for _, change := range []string{"foo-bar", "bar-baz", "baz"} {
		status, err := Resolve(ResolveOptions{CWD: root, ChangeName: change})
		if err != nil {
			t.Fatalf("Resolve(%q) error = %v", change, err)
		}
		if status.Archived != nil || status.NextRecommended == "archived" {
			t.Fatalf("Resolve(%q) projected archived %v next %q; only the exact date-prefixed suffix may match", change, status.Archived, status.NextRecommended)
		}
	}
}

// TestNeverExistedChangeKeepsNotFoundBlock pins the pre-#4002 contract for a
// change with no active folder and no archive entry: the exact not-found
// block, byte for byte.
func TestNeverExistedChangeKeepsNotFoundBlock(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "openspec", "changes", "archive", "2026-08-30-wave-one"))

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "ghost"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.NextRecommended != "sdd-new" {
		t.Fatalf("NextRecommended = %q, want %q", status.NextRecommended, "sdd-new")
	}
	if len(status.BlockedReasons) != 1 || status.BlockedReasons[0] != "Active OpenSpec change not found: ghost." {
		t.Fatalf("BlockedReasons = %v, want exactly the not-found reason", status.BlockedReasons)
	}
	if status.Archived != nil {
		t.Fatalf("Archived = %v, want nil for a change that never existed", status.Archived)
	}
}

func TestStatusV2ProjectionCarriesArchivedField(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "openspec", "changes", "archive", "2026-08-30-wave-one"))

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "wave-one"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	projected, err := ProjectStatusV2(status)
	if err != nil {
		t.Fatalf("ProjectStatusV2() error = %v", err)
	}
	if projected.NextRecommended != "archived" {
		t.Fatalf("projected NextRecommended = %q, want %q", projected.NextRecommended, "archived")
	}
	if projected.Archived == nil || projected.Archived.Path != filepath.Join("openspec", "changes", "archive", "2026-08-30-wave-one") {
		t.Fatalf("projected Archived = %v, want the archive path on the wire", projected.Archived)
	}

	serialized, err := marshalStatusV2Indent(status)
	if err != nil {
		t.Fatalf("marshalStatusV2Indent() error = %v", err)
	}
	if !strings.Contains(string(serialized), `"archived"`) {
		t.Fatalf("serialized v2 document carries no archived field:\n%s", serialized)
	}
}

// TestStatusV2ProjectionOmitsArchivedForActiveChange keeps the field
// structurally absent everywhere else — the optional-block discipline
// ReviewOffer established.
func TestStatusV2ProjectionOmitsArchivedForActiveChange(t *testing.T) {
	root := t.TempDir()
	seedReadyChange(t, root, "in-flight", "- [ ] 1.1 Work\n")

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "in-flight"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.Archived != nil {
		t.Fatalf("Archived = %v, want nil for an active change", status.Archived)
	}
	serialized, err := marshalStatusV2Indent(status)
	if err != nil {
		t.Fatalf("marshalStatusV2Indent() error = %v", err)
	}
	if strings.Contains(string(serialized), `"archived"`) {
		t.Fatalf("serialized v2 document carries archived for an active change:\n%s", serialized)
	}
}
