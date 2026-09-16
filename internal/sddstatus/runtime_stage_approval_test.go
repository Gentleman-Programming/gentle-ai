package sddstatus

import (
	"context"
	"testing"
)

func TestRuntimeLedgerStageApproval(t *testing.T) {
	ctx := context.Background()
	vocab := StageVocabulary{
		ID: "test-vocab-approve",
		Stages: []Stage{
			{Label: "explore"},
			{Label: "spec"},
			{Label: "apply"},
		},
	}
	repo := t.TempDir()
	initTestRepository(t, repo)

	store, err := OpenRuntimeStore(ctx, repo, "test-change-approve")
	if err != nil {
		t.Fatalf("OpenRuntimeStore failed: %v", err)
	}
	store = store.WithStageVocabulary(vocab)

	// Step 1: Begin and Finish 'explore'
	status, err := store.Begin(ctx, BeginAttemptRequest{
		RequestID:    "req-begin-explore",
		WorkUnit:     "explore",
		EvidenceGoal: "goal",
		MaxAttempts:  2,
	})
	if err != nil {
		t.Fatalf("Begin explore failed: %v", err)
	}

	status, err = store.Finish(ctx, FinishAttemptRequest{
		ExpectedRevision:   status.Revision,
		RequestID:          "req-finish-explore",
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
	firstExploreEvidence := status.EvidenceRevision

	// Step 2: Attempt to advance to 'spec' without approval (absent -> ErrRuntimeStageApprovalRequired)
	_, err = store.Begin(ctx, BeginAttemptRequest{
		ExpectedRevision: status.Revision,
		RequestID:        "req-begin-spec-no-approve",
		WorkUnit:         "spec",
		EvidenceGoal:     "goal spec",
		MaxAttempts:      2,
	})
	if err != ErrRuntimeStageApprovalRequired {
		t.Fatalf("expected ErrRuntimeStageApprovalRequired, got: %v", err)
	}

	// Step 3: Approve 'explore'
	status, err = store.ApproveStage(ctx, ApproveStageRequest{
		ExpectedRevision: status.Revision,
		RequestID:        "req-approve-explore",
		Stage:            "explore",
	})
	if err != nil {
		t.Fatalf("ApproveStage failed: %v", err)
	}

	// Verify projection in RuntimeStatus
	foundApproval := false
	for _, app := range status.StageApprovals {
		if app.Stage == "explore" && app.ApprovalRevision == firstExploreEvidence {
			foundApproval = true
			break
		}
	}
	if !foundApproval {
		t.Fatalf("expected StageApprovals to contain approval for explore with revision %s", firstExploreEvidence)
	}

	// Step 4: Advance to 'spec' WITH approval (succeeds)
	status, err = store.Begin(ctx, BeginAttemptRequest{
		ExpectedRevision: status.Revision,
		RequestID:        "req-begin-spec-with-approve",
		WorkUnit:         "spec",
		EvidenceGoal:     "goal spec",
		MaxAttempts:      2,
	})
	if err != nil {
		t.Fatalf("Begin spec (with approval) failed: %v", err)
	}
	if status.Objective == nil || status.Objective.ApprovalRevision != firstExploreEvidence {
		t.Fatalf("Expected BeginEvent to record ApprovalRevision = %s", firstExploreEvidence)
	}

	// Step 5: Invalidate by new predecessor EvidenceRevision (reset to explore, finish with new evidence)
	// Finish spec first
	status, err = store.Finish(ctx, FinishAttemptRequest{
		ExpectedRevision:   status.Revision,
		RequestID:          "req-finish-spec",
		Outcome:            AttemptPassed,
		EvidenceRevision:   "sha256:2222222222222222222222222222222222222222222222222222222222222222",
		Diagnosis:          "passed spec",
		CleanupEvidence:    "none",
		ProcessEvidence:    "none",
		HarnessDisposition: HarnessReused,
	})
	if err != nil {
		t.Fatalf("Finish spec failed: %v", err)
	}

	// Reset to 'explore'
	status, err = store.Reset(ctx, ResetObjectiveRequest{
		ExpectedRevision: status.Revision,
		RequestID:        "req-reset-explore",
		Reason:           "resetting to test invalidation",
	})
	if err != nil {
		t.Fatalf("Reset to explore failed: %v", err)
	}

	// Begin explore again
	status, err = store.Begin(ctx, BeginAttemptRequest{
		ExpectedRevision: status.Revision,
		RequestID:        "req-begin-explore-2",
		WorkUnit:         "explore",
		EvidenceGoal:     "explore again",
		MaxAttempts:      2,
	})
	if err != nil {
		t.Fatalf("Second Begin explore failed: %v", err)
	}

	// Finish explore with new evidence revision
	newExploreEvidence := "sha256:3333333333333333333333333333333333333333333333333333333333333333"
	status, err = store.Finish(ctx, FinishAttemptRequest{
		ExpectedRevision:   status.Revision,
		RequestID:          "req-finish-explore-2",
		Outcome:            AttemptPassed,
		EvidenceRevision:   newExploreEvidence,
		Diagnosis:          "passed explore again",
		CleanupEvidence:    "none",
		ProcessEvidence:    "none",
		HarnessDisposition: HarnessReused,
	})
	if err != nil {
		t.Fatalf("Second Finish explore failed: %v", err)
	}

	// Attempt to advance to 'spec' using old approval (fails)
	_, err = store.Begin(ctx, BeginAttemptRequest{
		ExpectedRevision: status.Revision,
		RequestID:        "req-begin-spec-stale-approve",
		WorkUnit:         "spec",
		EvidenceGoal:     "goal spec",
		MaxAttempts:      2,
	})
	if err != ErrRuntimeStageApprovalRequired {
		t.Fatalf("expected ErrRuntimeStageApprovalRequired for invalidated approval, got: %v", err)
	}

	// Approve 'explore' again
	status, err = store.ApproveStage(ctx, ApproveStageRequest{
		ExpectedRevision: status.Revision,
		RequestID:        "req-approve-explore-2",
		Stage:            "explore",
	})
	if err != nil {
		t.Fatalf("Second ApproveStage failed: %v", err)
	}

	// Advance to 'spec' (succeeds)
	status, err = store.Begin(ctx, BeginAttemptRequest{
		ExpectedRevision: status.Revision,
		RequestID:        "req-begin-spec-with-new-approve",
		WorkUnit:         "spec",
		EvidenceGoal:     "goal spec",
		MaxAttempts:      2,
	})
	if err != nil {
		t.Fatalf("Begin spec (with new approval) failed: %v", err)
	}
}
