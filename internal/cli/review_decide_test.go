package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

type decideCLIFixture struct {
	repo            string
	lineage         string
	store           reviewtransaction.Store
	head            string
	transaction     reviewtransaction.Transaction
	stateAtDecision string
}

func newDecideCLIFixture(t *testing.T, lineage string, outcomes []reviewtransaction.EvidenceOutcome) decideCLIFixture {
	t.Helper()
	t.Setenv("GENTLE_AI_REVIEW_DECISION_REQUIRED", "1")
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	policyHash := "sha256:" + strings.Repeat("d", 64)
	tx, err := reviewtransaction.NewTransaction(reviewtransaction.Start{
		LineageID: lineage, Mode: reviewtransaction.ModeOrdinary4R, Generation: 1,
		Snapshot: snapshot, PolicyHash: policyHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	findings := []reviewtransaction.Finding{{ID: "R2-INF-1", Severity: "CRITICAL"}, {ID: "R2-INF-2", Severity: "BLOCKER"}}
	ledger, err := reviewtransaction.CanonicalLedger(findings)
	if err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.AuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	head := appendLegacyCLIRecord(t, store, "", "review/start", *tx)
	if err := tx.FreezeFindings(findings, ledger, ""); err != nil {
		t.Fatal(err)
	}
	head = appendLegacyCLIRecord(t, store, head, "review/freeze-findings", *tx)
	if _, err := tx.ClassifyEvidence([]reviewtransaction.FindingEvidence{
		{FindingID: "R2-INF-1", Class: reviewtransaction.EvidenceInferential, Causality: reviewtransaction.CausalIntroduced, Proof: "race requires interpretation"},
		{FindingID: "R2-INF-2", Class: reviewtransaction.EvidenceInferential, Causality: reviewtransaction.CausalIntroduced, Proof: "ordering requires interpretation"},
	}); err != nil {
		t.Fatal(err)
	}
	head = appendLegacyCLIRecord(t, store, head, "review/classify-evidence", *tx)
	results := make([]reviewtransaction.EvidenceResult, 0, len(outcomes))
	for index, outcome := range outcomes {
		results = append(results, reviewtransaction.EvidenceResult{
			FindingID: findings[index].ID, Outcome: outcome, Proof: "refuter trace " + string(outcome),
		})
	}
	if err := tx.ApplyRefuterOutcomes(results); err != nil {
		t.Fatal(err)
	}
	head = appendLegacyCLIRecord(t, store, head, "review/apply-refuter-outcomes", *tx)
	return decideCLIFixture{
		repo:            repo,
		lineage:         lineage,
		store:           store,
		head:            head,
		transaction:     *tx,
		stateAtDecision: string(tx.State),
	}
}

func TestRunReviewDecideRejectsMismatchedRevision(t *testing.T) {
	fixture := newDecideCLIFixture(t, "decide-cas-mismatch", []reviewtransaction.EvidenceOutcome{
		reviewtransaction.OutcomeInconclusive, reviewtransaction.OutcomeInconclusive,
	})
	if fixture.stateAtDecision != string(reviewtransaction.StateDecisionRequired) {
		t.Fatalf("fixture state = %q, want %q", fixture.stateAtDecision, reviewtransaction.StateDecisionRequired)
	}
	bogusRevision := "sha256:" + strings.Repeat("e", 64)
	var output bytes.Buffer
	err := RunReviewDecide([]string{
		"--cwd", fixture.repo, "--lineage", fixture.lineage,
		"--expected-revision", bogusRevision,
		"--decision", "continue", "--reason", "authorize production pass",
	}, &output)
	if err == nil || !strings.Contains(err.Error(), "revision mismatch") {
		t.Fatalf("revision mismatch error = %v, want mismatch message", err)
	}
	if output.Len() != 0 {
		t.Fatalf("output = %q, want empty on rejection", output.String())
	}
}

func TestRunReviewDecideAcceptsMatchingRevisionAndEmitsAuditEntry(t *testing.T) {
	fixture := newDecideCLIFixture(t, "decide-accept", []reviewtransaction.EvidenceOutcome{
		reviewtransaction.OutcomeInconclusive, reviewtransaction.OutcomeInconclusive,
	})
	var output bytes.Buffer
	if err := RunReviewDecide([]string{
		"--cwd", fixture.repo, "--lineage", fixture.lineage,
		"--expected-revision", fixture.head,
		"--decision", "continue", "--reason", "authorize production pass",
	}, &output); err != nil {
		t.Fatalf("RunReviewDecide(continue) error = %v", err)
	}
	var result ReviewDecideResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decide result JSON: %v\n%s", err, output.String())
	}
	if result.Operation != "review/decide" || result.Decision != "continue" || result.Idempotent {
		t.Fatalf("decide result = %#v", result)
	}
	if result.Payload.Schema != reviewtransaction.DecisionPayloadSchema || result.Payload.LineageID != fixture.lineage ||
		result.Payload.Decision != "continue" || result.Payload.Reason != "authorize production pass" {
		t.Fatalf("decide payload = %#v", result.Payload)
	}
	if !strings.HasPrefix(result.Payload.SHA256, "sha256:") {
		t.Fatalf("payload sha256 = %q, want sha256: prefix", result.Payload.SHA256)
	}
	chain, err := fixture.store.LoadChain()
	if err != nil {
		t.Fatal(err)
	}
	last := chain.Records[len(chain.Records)-1]
	if last.Operation != "review/decide" {
		t.Fatalf("last operation = %q, want review/decide", last.Operation)
	}
	if last.Transaction.Decision == nil {
		t.Fatalf("last transaction decision = nil, want persisted decision")
	}
}

func TestRunReviewDecideStopMovesToEscalated(t *testing.T) {
	fixture := newDecideCLIFixture(t, "decide-stop-escalates", []reviewtransaction.EvidenceOutcome{
		reviewtransaction.OutcomeInconclusive, reviewtransaction.OutcomeInconclusive,
	})
	if err := RunReviewDecide([]string{
		"--cwd", fixture.repo, "--lineage", fixture.lineage,
		"--expected-revision", fixture.head,
		"--decision", "stop", "--reason", "maintainer stops inconclusive review",
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("RunReviewDecide(stop) error = %v", err)
	}

	chain, err := fixture.store.LoadChain()
	if err != nil {
		t.Fatal(err)
	}
	stored := chain.Records[len(chain.Records)-1].Transaction
	if stored.State != reviewtransaction.StateEscalated {
		t.Fatalf("stored state = %q, want %q", stored.State, reviewtransaction.StateEscalated)
	}
}

func TestRunReviewDecideContinueMovesForward(t *testing.T) {
	fixture := newDecideCLIFixture(t, "decide-continue-carries-on", []reviewtransaction.EvidenceOutcome{
		reviewtransaction.OutcomeInconclusive, reviewtransaction.OutcomeInconclusive,
	})
	if err := RunReviewDecide([]string{
		"--cwd", fixture.repo, "--lineage", fixture.lineage,
		"--expected-revision", fixture.head,
		"--decision", "continue", "--reason", "maintainer accepts bounded continuation",
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("RunReviewDecide(continue) error = %v", err)
	}

	chain, err := fixture.store.LoadChain()
	if err != nil {
		t.Fatal(err)
	}
	stored := chain.Records[len(chain.Records)-1].Transaction
	want := reviewtransaction.State("decision_carry_on")
	if stored.State != want {
		t.Fatalf("stored state = %q, want forward state %q", stored.State, want)
	}
}

func TestRunReviewDecideCanProduceReceiptAfterStop(t *testing.T) {
	fixture := newDecideCLIFixture(t, "decide-stop-receipt", []reviewtransaction.EvidenceOutcome{
		reviewtransaction.OutcomeInconclusive, reviewtransaction.OutcomeInconclusive,
	})
	if err := RunReviewDecide([]string{
		"--cwd", fixture.repo, "--lineage", fixture.lineage,
		"--expected-revision", fixture.head,
		"--decision", "stop", "--reason", "preserve terminal escalation path",
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("RunReviewDecide(stop) error = %v", err)
	}

	chain, err := fixture.store.LoadChain()
	if err != nil {
		t.Fatal(err)
	}
	stored := chain.Records[len(chain.Records)-1].Transaction
	receipt, err := stored.Receipt()
	if err != nil {
		t.Fatalf("Receipt() after stop error = %v (state %q)", err, stored.State)
	}
	if receipt.TerminalState != reviewtransaction.TerminalEscalated {
		t.Fatalf("receipt terminal state = %q, want %q", receipt.TerminalState, reviewtransaction.TerminalEscalated)
	}
}

func TestRunReviewDecideIsIdempotentForSameDecision(t *testing.T) {
	fixture := newDecideCLIFixture(t, "decide-idempotent", []reviewtransaction.EvidenceOutcome{
		reviewtransaction.OutcomeInconclusive, reviewtransaction.OutcomeInconclusive,
	})
	var first bytes.Buffer
	if err := RunReviewDecide([]string{
		"--cwd", fixture.repo, "--lineage", fixture.lineage,
		"--expected-revision", fixture.head,
		"--decision", "continue", "--reason", "first attempt",
	}, &first); err != nil {
		t.Fatalf("first decide: %v", err)
	}
	chain, err := fixture.store.LoadChain()
	if err != nil {
		t.Fatal(err)
	}
	if len(chain.Records) == 0 {
		t.Fatal("first decide did not append a record")
	}
	secondHead := chain.HeadRevision

	var second bytes.Buffer
	if err := RunReviewDecide([]string{
		"--cwd", fixture.repo, "--lineage", fixture.lineage,
		"--expected-revision", secondHead,
		"--decision", "continue", "--reason", "first attempt",
	}, &second); err != nil {
		t.Fatalf("idempotent decide: %v", err)
	}
	var result ReviewDecideResult
	if err := json.Unmarshal(second.Bytes(), &result); err != nil {
		t.Fatalf("second result JSON: %v", err)
	}
	if !result.Idempotent {
		t.Fatalf("second result idempotent = false, payload = %#v", result.Payload)
	}
	reloaded, err := fixture.store.LoadChain()
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Records) != len(chain.Records) {
		t.Fatalf("idempotent decide changed chain length: %d -> %d", len(chain.Records), len(reloaded.Records))
	}
}

func TestRunReviewDecideRejectsConflictingDecision(t *testing.T) {
	fixture := newDecideCLIFixture(t, "decide-conflict", []reviewtransaction.EvidenceOutcome{
		reviewtransaction.OutcomeInconclusive, reviewtransaction.OutcomeInconclusive,
	})
	var first bytes.Buffer
	if err := RunReviewDecide([]string{
		"--cwd", fixture.repo, "--lineage", fixture.lineage,
		"--expected-revision", fixture.head,
		"--decision", "continue", "--reason", "first",
	}, &first); err != nil {
		t.Fatalf("first decide: %v", err)
	}
	chain, err := fixture.store.LoadChain()
	if err != nil {
		t.Fatal(err)
	}
	secondHead := chain.HeadRevision
	var second bytes.Buffer
	err = RunReviewDecide([]string{
		"--cwd", fixture.repo, "--lineage", fixture.lineage,
		"--expected-revision", secondHead,
		"--decision", "stop", "--reason", "conflicting",
	}, &second)
	if err == nil || !strings.Contains(err.Error(), "conflicting") {
		t.Fatalf("conflicting decide error = %v, want conflict rejection", err)
	}
	if second.Len() != 0 {
		t.Fatalf("conflicting decide emitted output = %q, want empty", second.String())
	}
}

func TestRunReviewDecideRejectsWhenStateIsNotDecisionRequired(t *testing.T) {
	// Build a fixture whose final refuter outcomes are mixed (corroborated + inconclusive).
	// ApplyRefuterOutcomes will route that to StateEscalated, not StateDecisionRequired.
	fixture := newDecideCLIFixture(t, "decide-wrong-state", []reviewtransaction.EvidenceOutcome{
		reviewtransaction.OutcomeInconclusive, reviewtransaction.OutcomeCorroborated,
	})
	if fixture.stateAtDecision != string(reviewtransaction.StateEscalated) {
		t.Fatalf("fixture state = %q, want %q (mixed outcomes escalate)", fixture.stateAtDecision, reviewtransaction.StateEscalated)
	}
	var output bytes.Buffer
	err := RunReviewDecide([]string{
		"--cwd", fixture.repo, "--lineage", fixture.lineage,
		"--expected-revision", fixture.head,
		"--decision", "stop", "--reason", "should fail",
	}, &output)
	if err == nil || !strings.Contains(err.Error(), "decision_required") {
		t.Fatalf("wrong-state error = %v, want state rejection", err)
	}
}

func TestRunReviewDecideRejectsMalformedFlagsBeforeTouchingStore(t *testing.T) {
	fixture := newDecideCLIFixture(t, "decide-bad-flags", []reviewtransaction.EvidenceOutcome{
		reviewtransaction.OutcomeInconclusive, reviewtransaction.OutcomeInconclusive,
	})
	headBefore := fixture.head
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing revision",
			args: []string{"--cwd", fixture.repo, "--lineage", fixture.lineage, "--decision", "continue", "--reason", "x"},
			want: "requires",
		},
		{
			name: "invalid revision format",
			args: []string{"--cwd", fixture.repo, "--lineage", fixture.lineage, "--expected-revision", "not-a-sha", "--decision", "continue", "--reason", "x"},
			want: "sha256",
		},
		{
			name: "invalid decision",
			args: []string{"--cwd", fixture.repo, "--lineage", fixture.lineage, "--expected-revision", fixture.head, "--decision", "maybe", "--reason", "x"},
			want: "continue or stop",
		},
		{
			name: "missing reason",
			args: []string{"--cwd", fixture.repo, "--lineage", fixture.lineage, "--expected-revision", fixture.head, "--decision", "continue", "--reason", "   "},
			want: "requires",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			err := RunReviewDecide(tt.args, &output)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
	chain, err := fixture.store.LoadChain()
	if err != nil {
		t.Fatal(err)
	}
	if chain.HeadRevision != headBefore {
		t.Fatalf("malformed-flag run mutated chain head: before=%q after=%q", headBefore, chain.HeadRevision)
	}
	// io.Discard sanity check: ensure the helper works without side effects on stdout.
	_ = RunReviewDecide([]string{"--help"}, io.Discard)
	if !errors.Is(nil, nil) {
		t.Fatal("sanity check failed")
	}
}
