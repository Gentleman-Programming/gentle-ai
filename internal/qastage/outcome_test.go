package qastage

import (
	"context"
	"errors"
	"testing"
)

func TestFinish_FailedDoesNotAdvance_AllowsRetryOfSameStage(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	if _, err := machine.Begin(ctx, "change-l", "explore", "req-begin-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Finish(ctx, "change-l", OutcomeFailed, anyRevision, "req-finish-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Retry: same stage must still be the expected one, since failed never
	// advances the machine.
	retry, err := machine.Begin(ctx, "change-l", "explore", "req-begin-2")
	if err != nil {
		t.Fatalf("expected explore to remain retryable after a failed outcome, got %v", err)
	}
	if retry.Ordinal != 2 {
		t.Fatalf("expected the retry to be a new attempt (ordinal 2), got %d", retry.Ordinal)
	}
}

func TestFinish_InterruptedDoesNotAdvance_AllowsRetryOfSameStage(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	if _, err := machine.Begin(ctx, "change-m", "explore", "req-begin-3"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Finish(ctx, "change-m", OutcomeInterrupted, anyRevision, "req-finish-2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := machine.Begin(ctx, "change-m", "explore", "req-begin-4"); err != nil {
		t.Fatalf("expected explore to remain retryable after an interrupted outcome, got %v", err)
	}
}

func TestFinish_RejectsInvalidOutcome(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	if _, err := machine.Begin(ctx, "change-n", "explore", "req-begin-5"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err := machine.Finish(ctx, "change-n", AttemptOutcome("bogus"), anyRevision, "req-finish-bogus")
	if !errors.Is(err, ErrInvalidOutcome) {
		t.Fatalf("expected ErrInvalidOutcome, got %v", err)
	}
}

func TestRetryHistory_PreservesEveryPastAttempt(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	if _, err := machine.Begin(ctx, "change-o", "explore", "req-begin-6"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Finish(ctx, "change-o", OutcomeFailed, anyRevision, "req-finish-3"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Begin(ctx, "change-o", "explore", "req-begin-7"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Finish(ctx, "change-o", OutcomePassed, anyRevision, "req-finish-4"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	attempts, err := machine.Attempts(ctx, "change-o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(attempts) != 2 {
		t.Fatalf("expected both the failed and the passed attempt preserved in history, got %d: %+v", len(attempts), attempts)
	}
	if attempts[0].Outcome != OutcomeFailed || attempts[1].Outcome != OutcomePassed {
		t.Fatalf("expected [failed, passed] in order, got %+v", attempts)
	}
}

func TestReset_RequiresAnActiveAttempt(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	_, err := machine.Reset(ctx, "change-p", "qa-lead", "stuck process")
	if !errors.Is(err, ErrNoActiveAttemptToReset) {
		t.Fatalf("expected ErrNoActiveAttemptToReset, got %v", err)
	}
}

func TestReset_ClosesActiveAttemptAsInterruptedWithAudit(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	if _, err := machine.Begin(ctx, "change-q", "explore", "req-begin-8"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reset, err := machine.Reset(ctx, "change-q", "qa-lead", "agent process died mid-explore")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reset.Outcome != OutcomeInterrupted {
		t.Fatalf("expected reset to leave the attempt interrupted, got %q", reset.Outcome)
	}
	if reset.ResetBy != "qa-lead" || reset.ResetReason != "agent process died mid-explore" {
		t.Fatalf("expected the reset to be audited with actor/reason, got %+v", reset)
	}

	// History is preserved, not erased, and a retry becomes possible again.
	attempts, err := machine.Attempts(ctx, "change-q")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("expected the reset attempt to remain in history, got %d", len(attempts))
	}

	if _, err := machine.Begin(ctx, "change-q", "explore", "req-begin-9"); err != nil {
		t.Fatalf("expected explore to be retryable after reset, got %v", err)
	}
}
