package qastage

import (
	"context"
	"errors"
	"testing"
)

func TestBegin_ApplyRequiresPriorApprovalOfSpec(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	if _, err := machine.Begin(ctx, "change-h", "explore", "req-begin-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Finish(ctx, "change-h", OutcomePassed, anyRevision, "req-finish-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Begin(ctx, "change-h", "spec", "req-begin-2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Finish(ctx, "change-h", OutcomePassed, anyRevision, "req-finish-2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// This is exactly the gap the baseline found in v2 (Escenario 2): the
	// approval gate must apply unconditionally, not only when some special
	// "already has prior history" condition happens to hold.
	_, err := machine.Begin(ctx, "change-h", "apply", "req-begin-3")
	if !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("expected ErrApprovalRequired without a recorded spec approval, got %v", err)
	}
}

func TestBegin_ApplySucceedsAfterSpecIsApproved(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	if _, err := machine.Begin(ctx, "change-i", "explore", "req-begin-4"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Finish(ctx, "change-i", OutcomePassed, anyRevision, "req-finish-3"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Begin(ctx, "change-i", "spec", "req-begin-5"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Finish(ctx, "change-i", OutcomePassed, anyRevision, "req-finish-4"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := machine.Approve(ctx, "change-i", "spec", anyRevision, "qa-lead", "looks correct", "req-approve-1"); err != nil {
		t.Fatalf("unexpected error approving spec: %v", err)
	}

	attempt, err := machine.Begin(ctx, "change-i", "apply", "req-begin-6")
	if err != nil {
		t.Fatalf("expected apply to begin after spec was approved, got %v", err)
	}
	if attempt.Stage != "apply" {
		t.Fatalf("expected stage=apply, got %q", attempt.Stage)
	}
}

func TestApprove_RejectsApprovingAStageThatNeverCompleted(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	_, err := machine.Approve(ctx, "change-j", "spec", anyRevision, "qa-lead", "premature", "req-approve-2")
	if !errors.Is(err, ErrNothingToApprove) {
		t.Fatalf("expected ErrNothingToApprove for an unfinished stage, got %v", err)
	}
}

func TestBegin_StagesOtherThanApplyNeverRequireApproval(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	// explore, spec, verify, docs must never be gated by an approval — only
	// apply is, per the vocabulary's requires_approval design (unchanged
	// from v2's intent, now actually enforced).
	if _, err := machine.Begin(ctx, "change-k", "explore", "req-begin-7"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Finish(ctx, "change-k", OutcomePassed, anyRevision, "req-finish-5"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Begin(ctx, "change-k", "spec", "req-begin-8"); err != nil {
		t.Fatalf("expected spec to begin without any approval requirement, got %v", err)
	}
}
