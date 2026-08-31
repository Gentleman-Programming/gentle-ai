package sddstatus

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// An Engram-backed change is never closed, so every change that ever persisted
// an artifact is reported active forever. Measured on a real store: 30 for one
// project, including seven `rdd-root-simplification-wave*` archived weeks ago.
//
// The two stores archive differently and only one leaves a trace the resolver
// reads. OpenSpec moves the directory into `openspec/changes/archive/`, so
// "active" is what remains. Engram moves nothing — `sdd-archive/SKILL.md` says
// "there are no openspec/ directories to move. The archive report in Engram
// serves as the audit trail" — and `engramTitlePattern` does not recognize
// `archive-report`. The one artifact that proves a change is finished is the
// one the resolver ignores.
//
// `state` is not a substitute. Its documented schema carries phase, artifacts,
// tasks_progress and last_updated with no terminal value, and in the measured
// store 1 change out of 62 had one at all.

func engramArtifact(project, title string) engramObservation {
	return engramObservation{Project: project, Title: title}
}

func TestCollectEngramChangesExcludesArchivedChanges(t *testing.T) {
	observations := []engramObservation{
		engramArtifact("demo", "sdd/in-flight/proposal"),
		engramArtifact("demo", "sdd/in-flight/tasks"),
		engramArtifact("demo", "sdd/wave-one/proposal"),
		engramArtifact("demo", "sdd/wave-one/tasks"),
		engramArtifact("demo", "sdd/wave-one/verify-report"),
		engramArtifact("demo", "sdd/wave-one/archive-report"),
	}

	changes := collectEngramChanges(observations, "demo")
	if slices.Contains(changes, "wave-one") {
		t.Fatalf("collectEngramChanges = %v; a change with an archive report is finished and must not be offered as active (#3008)", changes)
	}
	if !slices.Contains(changes, "in-flight") {
		t.Fatalf("collectEngramChanges = %v, want the unarchived change still listed", changes)
	}
}

// TestArchivedEngramChangeStillResolvesWhenNamed keeps the exclusion scoped to
// discovery. Asking "which changes are in flight" and asking "show me this
// change" are different questions, and an archived change must still answer the
// second one — its artifacts are the audit trail.
func TestArchivedEngramChangeStillResolvesWhenNamed(t *testing.T) {
	observations := []engramObservation{
		engramArtifact("demo", "sdd/wave-one/proposal"),
		engramArtifact("demo", "sdd/wave-one/tasks"),
		engramArtifact("demo", "sdd/wave-one/archive-report"),
	}

	artifacts := engramArtifactsForChange(observations, "demo", "wave-one")
	if len(artifacts) == 0 {
		t.Fatal("an archived change resolved no artifacts when named explicitly; excluding it from discovery must not erase it")
	}
}

// TestArchiveReportAloneDoesNotCreateAChange guards the other direction: a
// change known only by its archive report is finished by definition and was
// never active, so it must not appear because the pattern learned a new title.
func TestArchiveReportAloneDoesNotCreateAChange(t *testing.T) {
	changes := collectEngramChanges([]engramObservation{
		engramArtifact("demo", "sdd/only-archived/archive-report"),
	}, "demo")
	if len(changes) != 0 {
		t.Fatalf("collectEngramChanges = %v, want none", changes)
	}
}

// TestNamedArchivedEngramChangeDoesNotRecommendArchive pins #3480's residue:
// naming an archived Engram change still answered `archive: ready` and
// `nextRecommended: archive`, so an orchestrator was told to archive a change
// whose archive report already exists.
func TestNamedArchivedEngramChangeDoesNotRecommendArchive(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "openspec", "config.yaml"), "sdd:\n  artifact_store: engram\n")
	t.Setenv("ENGRAM_PROJECT", "gentle-ai")
	t.Cleanup(stubEngramExport(t, []engramObservation{
		{Title: "sdd/wave-one/proposal", Content: "# Proposal\n", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/wave-one/spec", Content: "### Requirement: Wave\n#### Scenario: Done\n", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/wave-one/design", Content: "# Design\n", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/wave-one/tasks", Content: "- [x] 1.1 Work\n", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/wave-one/verify-report", Content: testVerifyEnvelope("pass", 0, 0, "1/1", "1/1", 0, 0), Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/wave-one/archive-report", Content: "# Archive\n", Project: "gentle-ai", Scope: "project"},
	}))

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "wave-one"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.NextRecommended == string(PhaseArchive) || status.Dependencies.Archive == DependencyReady {
		t.Fatalf("archived change routed to archive again: archive %q next %q", status.Dependencies.Archive, status.NextRecommended)
	}
	if !slices.ContainsFunc(status.BlockedReasons, func(reason string) bool { return strings.Contains(reason, "sdd/wave-one/archive-report") }) {
		t.Fatalf("blocked reasons do not name the archive report: %v", status.BlockedReasons)
	}
}
