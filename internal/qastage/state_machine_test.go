package qastage

import (
	"context"
	"errors"
	"testing"
)

const anyRevision = "sha256:1111111111111111111111111111111111111111111111111111111111111111"

func newTestMachine(t *testing.T) *QAStateMachine {
	t.Helper()
	return NewQAStateMachine(NewPersistentQAStateStore(t.TempDir()))
}

func TestBegin_FirstAttemptOfNewChangeRejectsNonExploreStage(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	// This is the corrected behavior for the gap the baseline found in v2
	// (Escenario 3): a fresh change's first qa-begin used to accept ANY
	// stage label unchecked. v3 rejects it.
	_, err := machine.Begin(ctx, "change-a", "apply", "req-begin-1")
	if !errors.Is(err, ErrStageOutOfOrder) {
		t.Fatalf("expected ErrStageOutOfOrder for a first begin on a non-explore stage, got %v", err)
	}
}

func TestBegin_FirstAttemptOfNewChangeAcceptsExplore(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	attempt, err := machine.Begin(ctx, "change-b", "explore", "req-begin-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempt.Stage != "explore" || attempt.Outcome != OutcomeRunning {
		t.Fatalf("unexpected attempt: %+v", attempt)
	}
}

func TestBegin_RejectsWhileAnAttemptIsAlreadyActive(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	if _, err := machine.Begin(ctx, "change-c", "explore", "req-begin-3"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err := machine.Begin(ctx, "change-c", "explore", "req-begin-4")
	if !errors.Is(err, ErrAttemptAlreadyActive) {
		t.Fatalf("expected ErrAttemptAlreadyActive on a retry while running, got %v", err)
	}
}

func TestBeginFinish_AdvancingBeginAcceptsImmediateSuccessor(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	if _, err := machine.Begin(ctx, "change-d", "explore", "req-begin-5"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Finish(ctx, "change-d", OutcomePassed, anyRevision, "req-finish-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	attempt, err := machine.Begin(ctx, "change-d", "spec", "req-begin-6")
	if err != nil {
		t.Fatalf("expected spec to be accepted as explore's immediate successor, got %v", err)
	}
	if attempt.Stage != "spec" {
		t.Fatalf("expected stage=spec, got %q", attempt.Stage)
	}
}

func TestBeginFinish_AdvancingBeginRejectsSkippedStage(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	if _, err := machine.Begin(ctx, "change-e", "explore", "req-begin-7"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Finish(ctx, "change-e", OutcomePassed, anyRevision, "req-finish-2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Skips spec entirely, jumping straight to apply — this is exactly the
	// real skip the baseline confirmed v2 rejects on an advancing begin
	// (Escenario 3, Prueba B). v3 preserves this rejection.
	_, err := machine.Begin(ctx, "change-e", "apply", "req-begin-8")
	if !errors.Is(err, ErrStageOutOfOrder) {
		t.Fatalf("expected ErrStageOutOfOrder for a skipped stage, got %v", err)
	}
}

func TestFinish_RejectsWithoutAnActiveAttempt(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	_, err := machine.Finish(ctx, "change-f", OutcomePassed, anyRevision, "req-finish-3")
	if !errors.Is(err, ErrNoActiveAttempt) {
		t.Fatalf("expected ErrNoActiveAttempt, got %v", err)
	}
}

func TestBeginFinish_FullVocabularyInOrderSucceeds(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	for _, stage := range QAStageVocabulary() {
		if stage == "apply" {
			if _, err := machine.Approve(ctx, "change-g", "spec", anyRevision, "qa-lead", "approved for this test", "req-approve-1"); err != nil {
				t.Fatalf("approve spec: unexpected error: %v", err)
			}
		}
		if _, err := machine.Begin(ctx, "change-g", stage, "req-begin-full-"+stage); err != nil {
			t.Fatalf("begin %q: unexpected error: %v", stage, err)
		}
		if _, err := machine.Finish(ctx, "change-g", OutcomePassed, anyRevision, "req-finish-full-"+stage); err != nil {
			t.Fatalf("finish %q: unexpected error: %v", stage, err)
		}
	}

	// Nothing comes after "docs" — the vocabulary is exhausted.
	_, err := machine.Begin(ctx, "change-g", "docs", "req-begin-9")
	if !errors.Is(err, ErrStageOutOfOrder) {
		t.Fatalf("expected ErrStageOutOfOrder once the vocabulary is exhausted, got %v", err)
	}
}

func TestFinish_RejectsMalformedArtifactRevision(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	if _, err := machine.Begin(ctx, "change-badhash", "explore", "b1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err := machine.Finish(ctx, "change-badhash", OutcomePassed, "not-a-real-hash", "f1")
	if !errors.Is(err, ErrMalformedArtifactRevision) {
		t.Fatalf("expected ErrMalformedArtifactRevision, got %v", err)
	}
}

func TestFinish_AcceptsWellFormedArtifactRevision(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	if _, err := machine.Begin(ctx, "change-goodhash", "explore", "b1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	attempt, err := machine.Finish(ctx, "change-goodhash", OutcomePassed, anyRevision, "f1")
	if err != nil {
		t.Fatalf("expected a well-formed sha256:<64 lowercase hex> revision to be accepted, got %v", err)
	}
	if attempt.ArtifactRevision != anyRevision {
		t.Fatalf("expected the revision to round-trip, got %q", attempt.ArtifactRevision)
	}
}
