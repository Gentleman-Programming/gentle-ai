package qastage

import (
	"context"
	"errors"
	"testing"
)

const revisionA = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const revisionB = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func TestApprove_RejectsMismatchedArtifactRevision(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	if _, err := machine.Begin(ctx, "change-x", "explore", "b1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Finish(ctx, "change-x", OutcomePassed, anyRevision, "f1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Begin(ctx, "change-x", "spec", "b2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Finish(ctx, "change-x", OutcomePassed, revisionA, "f2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// spec's actual completed revision is revisionA; approving revisionB
	// (a revision that was never produced) must be rejected outright.
	_, err := machine.Approve(ctx, "change-x", "spec", revisionB, "qa-lead", "wrong revision", "a1")
	if !errors.Is(err, ErrApprovalRevisionMismatch) {
		t.Fatalf("expected ErrApprovalRevisionMismatch, got %v", err)
	}
}

func TestBegin_NewSpecRevisionInvalidatesPriorApproval(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	if _, err := machine.Begin(ctx, "change-y", "explore", "b1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Finish(ctx, "change-y", OutcomePassed, anyRevision, "f1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Begin(ctx, "change-y", "spec", "b2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Finish(ctx, "change-y", OutcomePassed, revisionA, "f2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Approve(ctx, "change-y", "spec", revisionA, "qa-lead", "approved A", "a1"); err != nil {
		t.Fatalf("unexpected error approving revision A: %v", err)
	}

	// Spec is amended before apply ever started — this reopens spec (allowed
	// because apply has no attempt yet) and produces a new revision.
	if _, err := machine.Begin(ctx, "change-y", "spec", "b3"); err != nil {
		t.Fatalf("expected spec to be reopenable before apply starts, got %v", err)
	}
	if _, err := machine.Finish(ctx, "change-y", OutcomePassed, revisionB, "f3"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The approval on file is for revisionA, but spec's current revision is
	// now revisionB — the old approval must no longer unlock apply (design
	// decision 1.1 / contrato pendiente #5).
	_, err := machine.Begin(ctx, "change-y", "apply", "b4")
	if !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("expected the stale approval (revisionA) to no longer authorize apply, got %v", err)
	}

	if _, err := machine.Approve(ctx, "change-y", "spec", revisionB, "qa-lead", "approved B", "a2"); err != nil {
		t.Fatalf("unexpected error approving revision B: %v", err)
	}
	if _, err := machine.Begin(ctx, "change-y", "apply", "b5"); err != nil {
		t.Fatalf("expected apply to begin once the current revision (B) is approved, got %v", err)
	}
}

func TestBegin_CannotReopenSpecOnceApplyHasStarted(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	if _, err := machine.Begin(ctx, "change-z", "explore", "b1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Finish(ctx, "change-z", OutcomePassed, anyRevision, "f1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Begin(ctx, "change-z", "spec", "b2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Finish(ctx, "change-z", OutcomePassed, revisionA, "f2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Approve(ctx, "change-z", "spec", revisionA, "qa-lead", "ok", "a1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Begin(ctx, "change-z", "apply", "b3"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Finish(ctx, "change-z", OutcomePassed, anyRevision, "f3"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Once apply has completed, spec is no longer the "last completed stage"
	// nor reopenable — verify is the only valid next stage.
	_, err := machine.Begin(ctx, "change-z", "spec", "b4")
	if !errors.Is(err, ErrStageOutOfOrder) {
		t.Fatalf("expected ErrStageOutOfOrder, spec must not be reopenable after apply completed, got %v", err)
	}
}
