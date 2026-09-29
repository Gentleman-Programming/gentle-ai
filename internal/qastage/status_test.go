package qastage

import (
	"context"
	"testing"
)

func TestStatus_FreshChangeExpectsBegin(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	status, err := machine.Status(ctx, "change-status-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.NextAction != "begin" || status.Complete {
		t.Fatalf("unexpected status: %+v", status)
	}
	if status.Stage != "explore" {
		t.Fatalf("expected Stage=explore (the stage to begin next), got %q", status.Stage)
	}
	if status.Revision != "" {
		t.Fatalf("expected an empty revision for a never-used change, got %q", status.Revision)
	}
}

func TestStatus_ActiveAttemptExpectsFinish(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	if _, err := machine.Begin(ctx, "change-status-b", "explore", "b1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	status, err := machine.Status(ctx, "change-status-b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.NextAction != "finish" || status.Complete {
		t.Fatalf("unexpected status: %+v", status)
	}
	if status.Stage != "explore" {
		t.Fatalf("expected Stage=explore (the running attempt's stage), got %q", status.Stage)
	}
	if status.Revision == "" {
		t.Fatal("expected a non-empty revision once a record exists")
	}
}

func TestStatus_ExhaustedVocabularyIsComplete(t *testing.T) {
	machine := newTestMachine(t)
	ctx := context.Background()

	for _, stage := range QAStageVocabulary() {
		if stage == "apply" {
			if _, err := machine.Approve(ctx, "change-status-c", "spec", anyRevision, "qa-lead", "ok", "a1"); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		}
		if _, err := machine.Begin(ctx, "change-status-c", stage, "b-"+stage); err != nil {
			t.Fatalf("begin %q: unexpected error: %v", stage, err)
		}
		if _, err := machine.Finish(ctx, "change-status-c", OutcomePassed, anyRevision, "f-"+stage); err != nil {
			t.Fatalf("finish %q: unexpected error: %v", stage, err)
		}
	}

	status, err := machine.Status(ctx, "change-status-c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.NextAction != "complete" || !status.Complete {
		t.Fatalf("expected an exhausted vocabulary to report complete, got %+v", status)
	}
	if status.Stage != "" {
		t.Fatalf("expected an empty Stage once complete, got %q", status.Stage)
	}
}
