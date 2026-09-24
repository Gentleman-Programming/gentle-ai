package reviewtransaction

import (
	"strings"
	"testing"
)

// TestTargetStatusProjectsDecisionQuestionIFFDecisionRequired mirrors the
// escalation coupling at the native projection: the decision question is
// present exactly when a compact authority is paused at decision_required,
// and disappears the moment the human decision resolves it.
func TestTargetStatusProjectsDecisionQuestionIFFDecisionRequired(t *testing.T) {
	repo := initSnapshotRepo(t)
	paused, store, started := decisionFindingFixture(t, repo, "decision-projection")
	if _, err := store.Replace(started.Revision, "review/complete-review", paused); err != nil {
		t.Fatalf("persist the pause: %v", err)
	}
	persisted, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	target := Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}
	pausedStatus, err := AssessTargetStatus(t.Context(), repo, TargetStatusRequest{Target: target, LineageID: paused.LineageID})
	if err != nil {
		t.Fatal(err)
	}
	question := pausedStatus.DecisionQuestion
	if question == nil {
		t.Fatal("decision_required authority must project its decision question")
	}
	if question.Schema != "gentle-ai.review-decision/v1" {
		t.Fatalf("decision question schema = %q, want gentle-ai.review-decision/v1", question.Schema)
	}
	if question.LineageID != paused.LineageID || question.ExpectedRevision != persisted.Revision {
		t.Fatalf("decision question binding = %s@%s, want %s@%s", question.LineageID, question.ExpectedRevision, paused.LineageID, persisted.Revision)
	}
	if question.State != StateDecisionRequired {
		t.Fatalf("decision question state = %q, want %q", question.State, StateDecisionRequired)
	}
	if question.Evidence == nil || question.Evidence.Cause != "unknown_causality" {
		t.Fatalf("decision question evidence = %#v, want the frozen pause cause", question.Evidence)
	}
	if len(question.Choices) != 2 {
		t.Fatalf("decision question choices = %d, want the continue/stop twins", len(question.Choices))
	}
	if question.Choices[0].Answer != CompactDecisionContinue || question.Choices[1].Answer != CompactDecisionStop {
		t.Fatalf("decision choices = %q,%q, want continue then stop", question.Choices[0].Answer, question.Choices[1].Answer)
	}
	for _, choice := range question.Choices {
		if strings.TrimSpace(choice.Label) == "" || strings.TrimSpace(choice.Effect) == "" {
			t.Fatalf("decision choice %q is missing its label or effect", choice.Answer)
		}
		if !strings.Contains(choice.Invocation, "review decide") ||
			!strings.Contains(choice.Invocation, "--decision "+choice.Answer) ||
			!strings.Contains(choice.Invocation, persisted.Revision) {
			t.Fatalf("decision choice %q invocation %q is not a runnable decide command bound to the current revision", choice.Answer, choice.Invocation)
		}
	}

	// A decide-stop resolves the question: the escalated authority projects
	// its escalation evidence, never a decision question.
	resolvedStop, err := DecideCompactStore(t.Context(), repo, CompactDecisionRequest{
		LineageID: paused.LineageID, ExpectedRevision: persisted.Revision,
		Decision: CompactDecisionStop, Actor: "maintainer@example.com", Reason: "stop the review here",
	})
	if err != nil {
		t.Fatal(err)
	}
	stopStatus, err := AssessTargetStatus(t.Context(), repo, TargetStatusRequest{Target: target, LineageID: paused.LineageID})
	if err != nil {
		t.Fatal(err)
	}
	if stopStatus.DecisionQuestion != nil {
		t.Fatal("escalated authority must not project a decision question")
	}
	if stopStatus.Escalation == nil {
		t.Fatal("escalated authority must keep projecting its escalation evidence")
	}
	if resolvedStop.State.State != StateEscalated {
		t.Fatalf("decide stop state = %q", resolvedStop.State.State)
	}

	// A decide-continue returns to reviewing: no decision question either.
	pausedAgain, continueStore, startedAgain := decisionFindingFixture(t, repo, "decision-projection-continue")
	if _, err := continueStore.Replace(startedAgain.Revision, "review/complete-review", pausedAgain); err != nil {
		t.Fatalf("persist the second pause: %v", err)
	}
	persistedAgain, err := continueStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecideCompactStore(t.Context(), repo, CompactDecisionRequest{
		LineageID: pausedAgain.LineageID, ExpectedRevision: persistedAgain.Revision,
		Decision: CompactDecisionContinue, Actor: "maintainer@example.com", Reason: "answer the model's own refutation",
	}); err != nil {
		t.Fatal(err)
	}
	continueStatus, err := AssessTargetStatus(t.Context(), repo, TargetStatusRequest{Target: target, LineageID: pausedAgain.LineageID})
	if err != nil {
		t.Fatal(err)
	}
	if continueStatus.State != StateReviewing {
		t.Fatalf("post-continue state = %q, want reviewing", continueStatus.State)
	}
	if continueStatus.DecisionQuestion != nil {
		t.Fatal("reviewing authority must not project a decision question")
	}
}
