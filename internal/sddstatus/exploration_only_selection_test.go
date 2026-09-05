package sddstatus

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Issue #3278 claim C: a directory holding only an exploration artifact is not
// an active change, yet listActiveOpenSpecChanges counted it as one. With one
// genuinely active change plus one stale sdd-explore directory, auto-selection
// reported "Change selection is ambiguous" and the SDD task-failure
// continuation (which carries no selector) could never resolve it.
//
// The contract these tests pin: a directory whose only SDD artifacts are
// exploration artifacts (exploration.md and nothing among proposal.md,
// design.md, tasks.md, specs/) is not a candidate for auto-selection or
// ambiguity, but remains addressable when explicitly named. Directories with
// no SDD artifacts at all (empty scaffolds) keep their historical behavior and
// stay candidates.

func TestExplorationOnlyDirectoryDoesNotMakeSelectionAmbiguous(t *testing.T) {
	root := t.TempDir()
	seedReadyChange(t, root, "add-widget", "- [ ] 1.1 Wire widget\n")
	write(t, filepath.Join(root, "openspec", "changes", "explore-widget-idea", "exploration.md"), "# Exploration\n")

	status, err := Resolve(ResolveOptions{CWD: root})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.ChangeName == nil || *status.ChangeName != "add-widget" {
		t.Fatalf("ChangeName = %v, want add-widget auto-selected beside an exploration-only directory", ptrValue(status.ChangeName))
	}
	if status.NextRecommended == "select-change" {
		t.Fatalf("NextRecommended = select-change; an exploration-only directory must not make selection ambiguous.\nBlockedReasons: %v", status.BlockedReasons)
	}
	if status.NextRecommended != "apply" {
		t.Fatalf("NextRecommended = %q, want apply for the one genuinely active change", status.NextRecommended)
	}
}

func TestTwoFullChangesStayAmbiguousBesideExplorationOnlyDirectory(t *testing.T) {
	root := t.TempDir()
	seedReadyChange(t, root, "first", "- [ ] 1.1 Work\n")
	seedReadyChange(t, root, "second", "- [ ] 1.1 Work\n")
	write(t, filepath.Join(root, "openspec", "changes", "explore-widget-idea", "exploration.md"), "# Exploration\n")

	status, err := Resolve(ResolveOptions{CWD: root})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.NextRecommended != "select-change" {
		t.Fatalf("NextRecommended = %q, want select-change with two full changes", status.NextRecommended)
	}
	reasons := strings.Join(status.BlockedReasons, "\n")
	if !strings.Contains(reasons, "ambiguous: first, second.") {
		t.Fatalf("ambiguity must list exactly the two full changes.\ngot:\n%s", reasons)
	}
	if strings.Contains(reasons, "explore-widget-idea") {
		t.Fatalf("exploration-only directory leaked into the ambiguity candidates.\ngot:\n%s", reasons)
	}
}

func TestExplorationOnlyDirectoryStillResolvesWhenExplicitlyNamed(t *testing.T) {
	root := t.TempDir()
	seedReadyChange(t, root, "add-widget", "- [ ] 1.1 Wire widget\n")
	write(t, filepath.Join(root, "openspec", "changes", "explore-widget-idea", "exploration.md"), "# Exploration\n")

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "explore-widget-idea"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.ChangeName == nil || *status.ChangeName != "explore-widget-idea" {
		t.Fatalf("ChangeName = %v, want explore-widget-idea when explicitly named", ptrValue(status.ChangeName))
	}
	reasons := strings.Join(status.BlockedReasons, "\n")
	if strings.Contains(reasons, "not found") {
		t.Fatalf("explicitly naming an exploration-only directory must keep resolving it.\ngot:\n%s", reasons)
	}
	if status.NextRecommended != "propose" {
		t.Fatalf("NextRecommended = %q, want propose for an exploration-only change", status.NextRecommended)
	}
}

func TestAllExplorationOnlyDirectoriesBlockAsNoActiveChanges(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "openspec", "changes", "explore-widget-idea", "exploration.md"), "# Exploration\n")

	status, err := Resolve(ResolveOptions{CWD: root})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.NextRecommended != "sdd-new" {
		t.Fatalf("NextRecommended = %q, want sdd-new when every directory is exploration-only", status.NextRecommended)
	}
	reasons := strings.Join(status.BlockedReasons, "\n")
	if !strings.Contains(reasons, "No active OpenSpec changes") {
		t.Fatalf("BlockedReasons = %v, want the no-active-changes refusal", status.BlockedReasons)
	}
	if !strings.Contains(reasons, "explore-widget-idea") {
		t.Fatalf("the refusal must name the exploration-only directory so the operator can address it explicitly.\ngot:\n%s", reasons)
	}
}

func TestUnstatableMarkerKeepsChangeAnActiveCandidate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("relies on POSIX symlink semantics to force a non-ErrNotExist stat error")
	}
	root := t.TempDir()
	seedReadyChange(t, root, "add-widget", "- [ ] 1.1 Wire widget\n")
	changeRoot := filepath.Join(root, "openspec", "changes", "explore-broken")
	write(t, filepath.Join(changeRoot, "exploration.md"), "# Exploration\n")
	// A self-referential symlink makes os.Stat fail with ELOOP, not
	// ErrNotExist. The classification is unprovable, so the directory must
	// stay an active-change candidate (surfacing as ambiguity), never drop
	// out of selection silently.
	if err := os.Symlink("proposal.md", filepath.Join(changeRoot, "proposal.md")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	status, err := Resolve(ResolveOptions{CWD: root})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.NextRecommended != "select-change" {
		t.Fatalf("NextRecommended = %q, want select-change: an unprovable exploration-only classification must fail toward ambiguity, not silent exclusion", status.NextRecommended)
	}
	if !strings.Contains(strings.Join(status.BlockedReasons, "\n"), "explore-broken") {
		t.Fatalf("the unprovable directory must stay visible as a candidate.\ngot: %v", status.BlockedReasons)
	}
}

func TestExplorationBesideRealArtifactsStaysASelectionCandidate(t *testing.T) {
	root := t.TempDir()
	// exploration.md beside a proposal is a change that finished exploring;
	// it must keep counting as active.
	changeRoot := filepath.Join(root, "openspec", "changes", "add-widget")
	write(t, filepath.Join(changeRoot, "exploration.md"), "# Exploration\n")
	write(t, filepath.Join(changeRoot, "proposal.md"), "# Proposal\n")

	status, err := Resolve(ResolveOptions{CWD: root})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.ChangeName == nil || *status.ChangeName != "add-widget" {
		t.Fatalf("ChangeName = %v, want add-widget: exploration.md beside real artifacts stays selectable", ptrValue(status.ChangeName))
	}
}
