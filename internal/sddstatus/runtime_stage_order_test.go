package sddstatus

import (
	"context"
	"testing"
)

func TestRuntimeLedgerStageOrder(t *testing.T) {
	ctx := context.Background()
	vocab := StageVocabulary{
		ID: "test-vocab-1",
		Stages: []Stage{
			{Label: "explore"},
			{Label: "spec"},
			{Label: "apply"},
		},
	}
	repo := t.TempDir()
	initTestRepository(t, repo)

	store, err := OpenRuntimeStore(ctx, repo, "test-change-order")
	if err != nil {
		t.Fatalf("OpenRuntimeStore failed: %v", err)
	}
	store = store.WithStageVocabulary(vocab)

	// Step 1: Begin the first stage (explore)
	status, err := store.Begin(ctx, BeginAttemptRequest{
		RequestID:    "req-begin-1",
		WorkUnit:     "explore",
		EvidenceGoal: "goal-1",
		MaxAttempts:  2,
	})
	if err != nil {
		t.Fatalf("Begin explore failed: %v", err)
	}

	// Record a finish for the first stage
	status, err = store.Finish(ctx, FinishAttemptRequest{
		ExpectedRevision: status.Revision,
		RequestID:        "req-finish-1",
		Outcome:            AttemptPassed,
		EvidenceRevision:   "sha256:1111111111111111111111111111111111111111111111111111111111111111",
		Diagnosis:          "passed explore",
		CleanupEvidence:    "none",
		ProcessEvidence:    "none",
		HarnessDisposition: HarnessReused,
	})
	if err != nil {
		t.Fatalf("Finish explore failed: %v", err)
	}

	// Step 2: Attempt to skip 'spec' and go straight to 'apply'
	_, err = store.Begin(ctx, BeginAttemptRequest{
		ExpectedRevision: status.Revision,
		RequestID:        "req-begin-skip",
		WorkUnit:         "apply",
		EvidenceGoal:     "goal-2",
		MaxAttempts:      2,
	})
	if err != ErrRuntimeStageOutOfOrder {
		t.Fatalf("expected ErrRuntimeStageOutOfOrder for skipped stage, got: %v", err)
	}

	// Step 3: Attempt an unknown stage label
	_, err = store.Begin(ctx, BeginAttemptRequest{
		ExpectedRevision: status.Revision,
		RequestID:        "req-begin-unknown",
		WorkUnit:         "unknown-stage",
		EvidenceGoal:     "goal-unknown",
		MaxAttempts:      2,
	})
	if err != ErrRuntimeStageUnknown {
		t.Fatalf("expected ErrRuntimeStageUnknown for unknown label, got: %v", err)
	}

	// Step 4: Legal successor ('spec') should be accepted
	status, err = store.Begin(ctx, BeginAttemptRequest{
		ExpectedRevision: status.Revision,
		RequestID:        "req-begin-spec",
		WorkUnit:         "spec",
		EvidenceGoal:     "goal-spec",
		MaxAttempts:      2,
	})
	if err != nil {
		t.Fatalf("Begin spec (legal successor) failed: %v", err)
	}
	if status.Objective == nil || status.Objective.WorkUnit != "spec" {
		t.Fatalf("Expected objective to be 'spec'")
	}
	if status.Objective.StagePosition != 1 {
		t.Fatalf("Expected StagePosition to be 1, got %d", status.Objective.StagePosition)
	}

	// Step 5: Test empty-vocabulary predecessor unaffected
	// Create a new store without vocabulary
	storeNoVocab, err := OpenRuntimeStore(ctx, repo, "test-change-no-vocab")
	if err != nil {
		t.Fatalf("OpenRuntimeStore failed: %v", err)
	}
	status, err = storeNoVocab.Begin(ctx, BeginAttemptRequest{
		RequestID:    "req-begin-nv1",
		WorkUnit:     "any-stage",
		EvidenceGoal: "goal-1",
		MaxAttempts:  2,
	})
	if err != nil {
		t.Fatalf("Begin without vocab failed: %v", err)
	}
	status, err = storeNoVocab.Finish(ctx, FinishAttemptRequest{
		ExpectedRevision: status.Revision,
		RequestID:        "req-finish-nv1",
		Outcome:            AttemptPassed,
		EvidenceRevision:   "sha256:2222222222222222222222222222222222222222222222222222222222222222",
		Diagnosis:          "passed any",
		CleanupEvidence:    "none",
		ProcessEvidence:    "none",
		HarnessDisposition: HarnessReused,
	})
	if err != nil {
		t.Fatalf("Finish without vocab failed: %v", err)
	}
	// Can advance to anything
	status, err = storeNoVocab.Begin(ctx, BeginAttemptRequest{
		ExpectedRevision: status.Revision,
		RequestID:        "req-begin-nv2",
		WorkUnit:         "any-next-stage",
		EvidenceGoal:     "goal-2",
		MaxAttempts:      2,
	})
	if err != nil {
		t.Fatalf("Legal advance without vocab failed: %v", err)
	}
	if status.Objective == nil || status.Objective.WorkUnit != "any-next-stage" {
		t.Fatalf("Expected objective to advance without vocab")
	}
}

// initTestRepository initializes a bare git repo required for tests.
func initTestRepository(t *testing.T, dir string) {
	t.Helper()
	runRuntimeLedgerGit(t, dir, "init", "--initial-branch=main")
	runRuntimeLedgerGit(t, dir, "config", "user.name", "Test")
	runRuntimeLedgerGit(t, dir, "config", "user.email", "test@example.com")
	runRuntimeLedgerGit(t, dir, "commit", "--allow-empty", "-m", "Initial commit")
}
