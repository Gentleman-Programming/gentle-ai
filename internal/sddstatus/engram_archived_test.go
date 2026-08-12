package sddstatus

import (
	"slices"
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
