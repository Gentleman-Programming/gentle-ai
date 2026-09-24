package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction"
)

// decisionRequiredNextTransitionStatus crafts the negotiated STATUS shape the
// adapter receives for a decision_required compact authority: current, stopped
// action, manual replayability. Before #1380 this collapsed to
// native_stop_required; after it, the stop names the review decide exit.
func decisionRequiredNextTransitionStatus() ReviewTargetStatusResult {
	return ReviewTargetStatusResult{
		Applicability: reviewtransaction.TargetApplicabilityCurrent,
		Action:        reviewtransaction.TargetStatusActionStop, Replayability: reviewtransaction.ReplayabilityManualActionRequired,
		TargetIdentity: "sha256:" + strings.Repeat("b", 64),
		Authority:      &ReviewTargetStatusAuthority{LineageID: "decision-required", Revision: "sha256:" + strings.Repeat("a", 64), State: reviewtransaction.StateDecisionRequired},
		Projection:     ReviewTargetStatusProjection{Projection: reviewtransaction.ProjectionWorkspace, BaseTree: strings.Repeat("c", 40), CurrentCandidateTree: strings.Repeat("d", 40)},
		repositoryRoot: "/decision-required-repo",
	}
}

func TestReviewNextTransitionDecisionRequiredNamesDecideContinuation(t *testing.T) {
	got := newReviewNextTransition(decisionRequiredNextTransitionStatus(), nil, nil, nil, reviewNextTransitionInput{})
	if got.Kind != reviewNextTransitionStop {
		t.Fatalf("decision_required transition kind = %q, want a stop", got.Kind)
	}
	if got.ReasonCode != "review_decision_required" {
		t.Fatalf("reason code = %q, want review_decision_required", got.ReasonCode)
	}
	if got.Execute != nil || got.Collect != nil || got.Continuation != nil {
		t.Fatalf("decision_required stop must stay narration-only (the table row carries the command): %#v", got)
	}
	statement, registered := reviewStopReasonNarration["review_decision_required"]
	if !registered {
		t.Fatal("review_decision_required must register its Tier C narration statement")
	}
	for _, fragment := range []string{"review decide", "--decision continue", "--decision stop"} {
		if !strings.Contains(statement, fragment) {
			t.Fatalf("narration statement %q does not name %q", statement, fragment)
		}
	}
	if classification, classified := reviewStopInvariantClassification["review_decision_required"]; !classified {
		t.Fatal("review_decision_required must be classified in the stop invariant table")
	} else if classification.Terminal {
		t.Fatal("review_decision_required is caller-continuable via review decide; it must not be classified terminal")
	}
}

func pausedDecisionCLIFixture(t *testing.T, repo, lineage string) (revision string) {
	t.Helper()
	accidental := filepath.Join(repo, "docs", "accidental.md")
	if err := os.MkdirAll(filepath.Dir(accidental), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(accidental, []byte("accidental\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	snapshot, err := builder.Build(t.Context(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{"docs/accidental.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	risk, lines, err := builder.ClassifySnapshotRisk(t.Context(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	state, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: lineage, Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 1,
		Snapshot: snapshot, PolicyHash: "sha256:" + strings.Repeat("ab", 32), RiskLevel: risk,
		SelectedLenses: []string{}, OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteReview(reviewtransaction.CompactReviewInput{}); err != nil {
		t.Fatalf("clean empty-lens completion: %v", err)
	}
	if state.State != reviewtransaction.StateValidating {
		t.Fatalf("fixture pre-state = %q, want validating before the pause injection", state.State)
	}
	state.FixFindingIDs = []string{}
	state.State = reviewtransaction.StateDecisionRequired
	state.Decision = &reviewtransaction.CompactDecisionEvidence{
		Reason:            reviewtransaction.CompactDecisionReasonEvidenceInconclusive,
		Cause:             "unresolved_severe_findings",
		FindingIDs:        []string{"R3-001"},
		AttemptedEvidence: []string{"admitted review lens results"},
		MissingEvidence:   []string{"conclusive_outcome:R3-001"},
		Recommendation:    "A human decides whether this review continues or stops.",
		NextAction:        "review decide (continue|stop) with the current lineage and expected revision",
		BoundedCost:       "one decide invocation; no reviewer recapture and no additional reviewer work",
	}
	state.DecisionEpoch = 1
	state.DecisionHistory = []reviewtransaction.CompactDecisionEntry{{
		Decision: reviewtransaction.CompactDecisionPause, Actor: reviewtransaction.CompactDecisionSystemActor,
		Reason: reviewtransaction.CompactDecisionReasonEvidenceInconclusive,
	}}
	if err := state.Validate(); err != nil {
		t.Fatalf("paused fixture state refuses validation: %v", err)
	}
	return writeReconcileCLIRecord(t, repo, state)
}

func TestRunReviewDecideStopsPausedLineage(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	revision := pausedDecisionCLIFixture(t, repo, "decide-stop-cli")

	var stdout bytes.Buffer
	if err := RunReview([]string{
		"decide", "--cwd", repo, "--lineage", "decide-stop-cli", "--expected-revision", revision,
		"--decision", "stop", "--actor", "maintainer@example.com", "--reason", "stop the review here",
	}, &stdout); err != nil {
		t.Fatalf("review decide stop: %v", err)
	}
	var result struct {
		Operation string                                     `json:"operation"`
		Record    reviewtransaction.CompactRecord            `json:"record"`
		Question  *reviewtransaction.CompactDecisionQuestion `json:"-"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode decide result %s: %v", stdout.String(), err)
	}
	if result.Operation != "review/decide" {
		t.Fatalf("operation = %q, want review/decide", result.Operation)
	}
	if result.Record.State.State != reviewtransaction.StateEscalated {
		t.Fatalf("decide stop state = %q, want escalated", result.Record.State.State)
	}
	if result.Record.State.DecisionEpoch != 2 || len(result.Record.State.DecisionHistory) != 2 {
		t.Fatalf("decide stop epoch=%d history=%d, want the pause entry plus the stop entry", result.Record.State.DecisionEpoch, len(result.Record.State.DecisionHistory))
	}

	var replay bytes.Buffer
	if err := RunReview([]string{
		"decide", "--cwd", repo, "--lineage", "decide-stop-cli", "--expected-revision", revision,
		"--decision", "stop", "--actor", "maintainer@example.com", "--reason", "stop the review here",
	}, &replay); err != nil {
		t.Fatalf("idempotent decide replay: %v", err)
	}
	if !strings.Contains(replay.String(), result.Record.Revision) {
		t.Fatalf("decide replay = %s, want the committed revision %s", replay.String(), result.Record.Revision)
	}
}

func TestRunReviewDecideContinueResumesReviewing(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	revision := pausedDecisionCLIFixture(t, repo, "decide-continue-cli")

	var stdout bytes.Buffer
	if err := RunReview([]string{
		"decide", "--cwd", repo, "--lineage", "decide-continue-cli", "--expected-revision", revision,
		"--decision", "continue", "--actor", "maintainer@example.com", "--reason", "answer the model's own refutation",
	}, &stdout); err != nil {
		t.Fatalf("review decide continue: %v", err)
	}
	var result struct {
		Operation string                          `json:"operation"`
		Record    reviewtransaction.CompactRecord `json:"record"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode decide result %s: %v", stdout.String(), err)
	}
	if result.Record.State.State != reviewtransaction.StateReviewing {
		t.Fatalf("decide continue state = %q, want reviewing", result.Record.State.State)
	}
	if result.Record.State.Decision != nil {
		t.Fatal("decide continue must clear the answered question")
	}
}

func TestRunReviewDecideRefusals(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	if err := RunReview([]string{"decide", "--cwd", repo, "--lineage", "x"}, &bytes.Buffer{}); err == nil ||
		!strings.Contains(err.Error(), "review status") {
		t.Fatalf("incomplete decide flags error = %v, want the inputs refusal naming where to read values", err)
	}
	if err := RunReview([]string{
		"decide", "--cwd", repo, "--lineage", "x", "--expected-revision", "sha256:" + strings.Repeat("ab", 32),
		"--decision", "maybe", "--actor", "a", "--reason", "r",
	}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "continue") {
		t.Fatalf("decision outside the vocabulary = %v, want the closed vocabulary refusal", err)
	}
}

func TestReviewDecisionEnvelopeFixtureBytesUnchanged(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join("..", "..", "contracts", "review-integration", "v2", "fixtures", "decision-v1.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var question reviewtransaction.CompactDecisionQuestion
	if err := decoder.Decode(&question); err != nil {
		t.Fatalf("decision fixture no longer decodes into the live type: %v", err)
	}
	if err := question.Validate(); err != nil {
		t.Fatalf("decision fixture no longer validates: %v", err)
	}
	remarshaled, err := json.MarshalIndent(question, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	remarshaled = append(remarshaled, '\n')
	if !bytes.Equal(remarshaled, payload) {
		t.Fatalf("decision envelope no longer serializes to the shipped fixture bytes\n--- fixture ---\n%s\n--- remarshaled ---\n%s", payload, remarshaled)
	}
}
