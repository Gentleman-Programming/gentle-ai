package qastage

import (
	"context"
	"errors"
	"testing"
)

func TestBegin_ReplayWithSameRequestIDAndStageReturnsSameAttempt(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	first, err := machine.Begin(ctx, "change-r", "explore", "req-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	replay, err := machine.Begin(ctx, "change-r", "explore", "req-1")
	if err != nil {
		t.Fatalf("expected a replay with the same request-id and stage to succeed, got %v", err)
	}
	if replay.Ordinal != first.Ordinal {
		t.Fatalf("expected the replay to return the same attempt (ordinal %d), got ordinal %d", first.Ordinal, replay.Ordinal)
	}

	attempts, err := machine.Attempts(ctx, "change-r")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("expected the replay to NOT create a second attempt, got %d attempts", len(attempts))
	}
}

func TestBegin_SameRequestIDDifferentStageIsConflict(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	if _, err := machine.Begin(ctx, "change-s", "explore", "req-2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Same request-id, different payload (a different stage) — must conflict,
	// not silently accept a mismatched replay and not report the unrelated
	// ErrAttemptAlreadyActive.
	_, err := machine.Begin(ctx, "change-s", "apply", "req-2")
	if !errors.Is(err, ErrRequestConflict) {
		t.Fatalf("expected ErrRequestConflict, got %v", err)
	}
}

func TestFinish_ReplayWithSameRequestIDAndOutcomeReturnsSameAttempt(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	if _, err := machine.Begin(ctx, "change-t", "explore", "req-3"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	first, err := machine.Finish(ctx, "change-t", OutcomePassed, anyRevision, "fin-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	replay, err := machine.Finish(ctx, "change-t", OutcomePassed, anyRevision, "fin-1")
	if err != nil {
		t.Fatalf("expected a replay with the same request-id and outcome to succeed, got %v", err)
	}
	if replay.Ordinal != first.Ordinal || replay.Outcome != first.Outcome {
		t.Fatalf("expected the replay to return the same result, got %+v vs %+v", first, replay)
	}
}

func TestFinish_SameRequestIDDifferentOutcomeIsConflict(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	if _, err := machine.Begin(ctx, "change-u", "explore", "req-4"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Finish(ctx, "change-u", OutcomePassed, anyRevision, "fin-2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err := machine.Finish(ctx, "change-u", OutcomeFailed, anyRevision, "fin-2")
	if !errors.Is(err, ErrRequestConflict) {
		t.Fatalf("expected ErrRequestConflict, got %v", err)
	}
}

func TestApprove_ReplayWithSameRequestIDReturnsSameApproval(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	if _, err := machine.Begin(ctx, "change-v", "explore", "req-5"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Finish(ctx, "change-v", OutcomePassed, anyRevision, "fin-3"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Begin(ctx, "change-v", "spec", "req-6"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Finish(ctx, "change-v", OutcomePassed, anyRevision, "fin-4"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	first, err := machine.Approve(ctx, "change-v", "spec", anyRevision, "qa-lead", "ok", "app-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	replay, err := machine.Approve(ctx, "change-v", "spec", anyRevision, "qa-lead", "ok", "app-1")
	if err != nil {
		t.Fatalf("expected a replay with the same request-id to succeed, got %v", err)
	}
	if replay != first {
		t.Fatalf("expected the replay to return the identical approval, got %+v vs %+v", first, replay)
	}

	attempts, err := machine.Attempts(ctx, "change-v")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = attempts // sanity: no panic reading state after a replayed approve
}

func TestApprove_SameRequestIDDifferentPayloadIsConflict(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	if _, err := machine.Begin(ctx, "change-w", "explore", "req-7"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Finish(ctx, "change-w", OutcomePassed, anyRevision, "fin-5"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Begin(ctx, "change-w", "spec", "req-8"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := machine.Finish(ctx, "change-w", OutcomePassed, anyRevision, "fin-6"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := machine.Approve(ctx, "change-w", "spec", anyRevision, "qa-lead", "ok", "app-2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err := machine.Approve(ctx, "change-w", "spec", anyRevision, "someone-else", "different reason", "app-2")
	if !errors.Is(err, ErrRequestConflict) {
		t.Fatalf("expected ErrRequestConflict, got %v", err)
	}
}
