package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// startNewLineageForFinalizeTest is the shared fixture for this file's
// tests: a tier-0 (RiskLow) new lineage, created through the real CLI
// `review start` with the activation switch on — the exact repro shape the
// verify report's CRITICAL C1 used (`GENTLE_AI_RDD_NEW_LINEAGE=1 gentle-ai
// review start --lineage X`).
func startNewLineageForFinalizeTest(t *testing.T, repo, lineage string) {
	t.Helper()
	t.Setenv("GENTLE_AI_RDD_NEW_LINEAGE", "1")
	lines := make([]string, 5)
	for index := range lines {
		lines[index] = "ordinary documentation line"
	}
	path := filepath.Join(repo, "docs", lineage+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(joinReviewCLILines(lines)), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", lineage}, &out); err != nil {
		t.Fatalf("new-lineage start: %v\n%s", err, out.String())
	}
	var started ReviewFacadeStartNewLineageResult
	decodeStrictReviewJSON(t, out.Bytes(), &started)
	if started.LineageID != lineage || started.State != reviewtransaction.NewLineageStateReviewing {
		t.Fatalf("new-lineage start = %#v", started)
	}
	t.Setenv("GENTLE_AI_RDD_NEW_LINEAGE", "")
}

// TestReviewFacadeFinalizeNewLineageReachesReceiptNoBlocker is task C1's
// primary RED/GREEN evidence: the verify report's exact repro
// (`review start` then `review finalize --lineage X`) currently errors with
// "load compact facade review lineage: open .../v2/<lineage>/review-state.json:
// no such file or directory" because runReviewFacadeFinalize only ever
// discovers the legacy v2 store. This proves finalize now routes on
// discovered lineage kind (DiscoverNewLineage) and reaches a real receipt
// for the tier-0 minimum-viable path: no admitted blocker, no --failed ⇒
// approved.
func TestReviewFacadeFinalizeNewLineageReachesReceiptNoBlocker(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	const lineage = "finalize-no-blocker-lineage"
	startNewLineageForFinalizeTest(t, repo, lineage)

	var out bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", lineage}, &out); err != nil {
		t.Fatalf("new-lineage finalize must reach a receipt, got error: %v\n%s", err, out.String())
	}
	var result ReviewFacadeFinalizeNewLineageResult
	decodeStrictReviewJSON(t, out.Bytes(), &result)
	if result.State != reviewtransaction.NewLineageStateApproved {
		t.Fatalf("finalize(no blocker) state = %q, want approved", result.State)
	}
	if result.Receipt == nil || result.Receipt.LineageID != lineage {
		t.Fatalf("finalize(no blocker) receipt = %#v", result.Receipt)
	}

	store, err := reviewtransaction.NewLineageAuthorityStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(store.ReceiptPath()); statErr != nil {
		t.Fatalf("terminal receipt not published on disk: %v", statErr)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if record.Authority.State != reviewtransaction.NewLineageStateApproved {
		t.Fatalf("persisted authority state = %q, want approved", record.Authority.State)
	}
}

// TestReviewFacadeFinalizeNewLineageFailedEvidenceEscalates proves the other
// half of the tier-0 minimum-viable path: --failed routes to an escalated
// receipt rather than approved.
func TestReviewFacadeFinalizeNewLineageFailedEvidenceEscalates(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	const lineage = "finalize-failed-evidence-lineage"
	startNewLineageForFinalizeTest(t, repo, lineage)

	var out bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", lineage, "--failed"}, &out); err != nil {
		t.Fatalf("new-lineage finalize(failed) must still reach a receipt, got error: %v\n%s", err, out.String())
	}
	var result ReviewFacadeFinalizeNewLineageResult
	decodeStrictReviewJSON(t, out.Bytes(), &result)
	if result.State != reviewtransaction.NewLineageStateEscalated {
		t.Fatalf("finalize(--failed) state = %q, want escalated", result.State)
	}
	if result.Receipt == nil {
		t.Fatal("finalize(--failed) issued no receipt")
	}
}

// TestReviewFacadeFinalizeNewLineageAdmitsOnlyCandidateCausalFindings is
// task C2's CLI-level RED/GREEN evidence: a candidate-caused finding blocks
// finalize (escalates rather than approves) and is the ONLY finding ID ever
// persisted into review-state.json's admitted_finding_ids; the sibling
// pre-existing finding never appears there (spec rdd-review-core-transitions,
// "Candidate-Causal Admission Only", both scenarios).
func TestReviewFacadeFinalizeNewLineageAdmitsOnlyCandidateCausalFindings(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	const lineage = "finalize-admission-blocks-lineage"
	startNewLineageForFinalizeTest(t, repo, lineage)

	findingsPath := filepath.Join(t.TempDir(), "findings.json")
	findings := []reviewtransaction.FindingEvidence{
		{FindingID: "candidate-caused-finding", Class: reviewtransaction.EvidenceDeterministic, Causality: reviewtransaction.CausalIntroduced, Proof: "diff shows the introduced defect"},
		{FindingID: "pre-existing-finding", Class: reviewtransaction.EvidenceDeterministic, Causality: reviewtransaction.CausalPreExisting, Proof: "present in the base tree"},
	}
	payload, err := json.Marshal(findings)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(findingsPath, payload, 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", lineage, "--admission-findings", findingsPath}, &out); err != nil {
		t.Fatalf("new-lineage finalize(admission findings) must still reach a receipt, got error: %v\n%s", err, out.String())
	}
	var result ReviewFacadeFinalizeNewLineageResult
	decodeStrictReviewJSON(t, out.Bytes(), &result)
	if result.State != reviewtransaction.NewLineageStateEscalated {
		t.Fatalf("finalize with an admitted candidate-causal finding = %q, want escalated (blocked)", result.State)
	}
	if !reflect.DeepEqual(result.AdmittedFindingIDs, []string{"candidate-caused-finding"}) {
		t.Fatalf("admitted_finding_ids = %v, want exactly [candidate-caused-finding]", result.AdmittedFindingIDs)
	}

	store, err := reviewtransaction.NewLineageAuthorityStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "pre-existing-finding") {
		t.Fatalf("pre-existing (follow-up) finding must never be persisted into review-state.json's admitted set: %s", raw)
	}
	if !strings.Contains(string(raw), "candidate-caused-finding") {
		t.Fatalf("candidate-causal finding must be persisted: %s", raw)
	}
}

// TestReviewFacadeFinalizeNewLineageFollowUpFindingsDoNotBlock is the
// companion scenario: a lineage whose only findings are pre-existing/
// base-only still reaches approved — a follow-up can never authorize a
// correction (block finalize).
func TestReviewFacadeFinalizeNewLineageFollowUpFindingsDoNotBlock(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	const lineage = "finalize-followups-dont-block-lineage"
	startNewLineageForFinalizeTest(t, repo, lineage)

	findingsPath := filepath.Join(t.TempDir(), "findings.json")
	findings := []reviewtransaction.FindingEvidence{
		{FindingID: "pre-existing-finding", Class: reviewtransaction.EvidenceDeterministic, Causality: reviewtransaction.CausalPreExisting, Proof: "present in the base tree"},
		{FindingID: "base-only-finding", Class: reviewtransaction.EvidenceDeterministic, Causality: reviewtransaction.CausalBaseOnly, Proof: "base-only path"},
	}
	payload, err := json.Marshal(findings)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(findingsPath, payload, 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", lineage, "--admission-findings", findingsPath}, &out); err != nil {
		t.Fatalf("new-lineage finalize(follow-ups only) must reach a receipt, got error: %v\n%s", err, out.String())
	}
	var result ReviewFacadeFinalizeNewLineageResult
	decodeStrictReviewJSON(t, out.Bytes(), &result)
	if result.State != reviewtransaction.NewLineageStateApproved {
		t.Fatalf("finalize with only follow-up findings = %q, want approved (never blocked)", result.State)
	}
	if len(result.AdmittedFindingIDs) != 0 {
		t.Fatalf("admitted_finding_ids = %v, want empty", result.AdmittedFindingIDs)
	}
}

// TestReviewFacadeFinalizeNewLineageCorrectingStateReachesReceipt is W14's
// RED/GREEN evidence: a lineage in `correcting` — previously refused
// outright by this command with no path forward — now finalizes through the
// identical AdvanceRequest path as `reviewing`, reaching a genuine terminal
// receipt. This is the product-surface proof for spec scenario "In-flight
// new lineage still finalizes after rollback", whose GIVEN is literally a
// `correcting` lineage.
func TestReviewFacadeFinalizeNewLineageCorrectingStateReachesReceipt(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	const lineage = "finalize-correcting-lineage"
	startNewLineageForFinalizeTest(t, repo, lineage)

	store, err := reviewtransaction.NewLineageAuthorityStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Mutate(context.Background(), record.Revision, func(next *reviewtransaction.NewLineageAuthority) error {
		next.State = reviewtransaction.NewLineageStateCorrecting
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", lineage}, &out); err != nil {
		t.Fatalf("new-lineage finalize on a correcting lineage must now reach a receipt, got error: %v\n%s", err, out.String())
	}
	var result ReviewFacadeFinalizeNewLineageResult
	decodeStrictReviewJSON(t, out.Bytes(), &result)
	if result.State != reviewtransaction.NewLineageStateApproved {
		t.Fatalf("finalize(correcting, no blocker) state = %q, want approved", result.State)
	}
	if result.Receipt == nil || result.Receipt.LineageID != lineage {
		t.Fatalf("finalize(correcting) receipt = %#v", result.Receipt)
	}
	if _, statErr := os.Stat(store.ReceiptPath()); statErr != nil {
		t.Fatalf("terminal receipt not published on disk: %v", statErr)
	}
}

// TestReviewFacadeFinalizeNewLineageMarkerCorruptedDeniesNeverLegacy proves
// finalize reuses S4/S5's own discovery-integrity denial (DiscoverNewLineage)
// rather than falling through to legacy discovery when a v3 marker exists
// but its record cannot be read.
func TestReviewFacadeFinalizeNewLineageMarkerCorruptedDeniesNeverLegacy(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	const lineage = "finalize-corrupted-marker-lineage"
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("corrupted marker fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")

	store, err := reviewtransaction.NewLineageAuthorityStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	head := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD^{tree}"))
	if _, err := store.Mutate(context.Background(), "", func(next *reviewtransaction.NewLineageAuthority) error {
		next.State = reviewtransaction.NewLineageStateReviewing
		next.CandidateIdentity = reviewtransaction.CandidateIdentity{
			RepositoryID: "corrupted-marker-repo", BaseTree: head, CandidateTree: head,
			ChangedPathsModesDigest: "sha256:" + head, PolicyHash: "unknown",
		}
		next.Tier = reviewtransaction.RiskLow
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(store.StatePath()); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err = RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", lineage}, &out)
	if err == nil {
		t.Fatal("finalize on a corrupted new-lineage marker must deny, got nil error")
	}
	if strings.Contains(err.Error(), "no such file or directory") && strings.Contains(err.Error(), "v2") {
		t.Fatalf("finalize must never fall through to the legacy v2 not-found error for a corrupted v3 marker: %v", err)
	}
}

// TestReviewFacadeFinalizeNewLineageEscalatedReceiptDeniesEveryGate is the
// pinning test from the accepted adjudication (b), now doubling as C5's own
// pinning regression: once a lineage's authority reaches `escalated`, it is
// a terminal non-approval — every gate must deny on it regardless of
// whether the live candidate still relates exactly to the frozen one.
// Before the adjudication (b) fix, resolveGoverningAuthority never
// consulted record.Authority.State at all, so an escalated-but-unchanged
// candidate would have reached CoreTransitionContinue → GateAllow.
//
// The frozen authority is built directly from governingAuthorityLiveEvidence
// (the same live-resolution function resolveGoverningAuthority itself calls
// at every gate), rather than through a full `review start`, so the fixture
// is provably an EXACT relation. C5 inversion (verify-report CRITICAL,
// "default-deny at gates"): the sanity check below used to assert ALLOW for
// the plain `reviewing` state before escalation — that assertion WAS the
// C5 bug pinned as expected behavior (a never-approved, receipt-less
// authority reaching GateAllow at an exact relation). It now asserts the
// opposite: `reviewing` must ALSO deny, proving default-deny closes that
// gap for exactly this fixture, before the escalated-specific short-circuit
// is exercised by the loop below.
func TestReviewFacadeFinalizeNewLineageEscalatedReceiptDeniesEveryGate(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	const lineage = "escalated-denies-every-gate-lineage"
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("escalated-denial fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")

	live, _, err := governingAuthorityLiveEvidence(context.Background(), repo, reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePreCommit})
	if err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.NewLineageAuthorityStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Mutate(context.Background(), "", func(next *reviewtransaction.NewLineageAuthority) error {
		next.State = reviewtransaction.NewLineageStateReviewing
		next.CandidateIdentity = live
		next.Tier = reviewtransaction.RiskLow
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// C5 inversion: a plain `reviewing` authority — never approved, no
	// receipt — must DENY here, not allow, even though its live candidate
	// relates exactly to the frozen one. This is the exact shape the
	// verify report's C5 repro used (`review start`, never finalized).
	if governs, evaluation, discoveryErr := resolveGoverningAuthority(context.Background(), repo, lineage, reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePreCommit}); !governs {
		t.Fatalf("fixture sanity check failed: an in-flight v3 record must still govern (deny), got governs=false")
	} else if discoveryErr == nil && evaluation.Result == reviewtransaction.GateAllow {
		t.Fatalf("fixture sanity check failed: a never-approved, receipt-less reviewing authority must never allow, got evaluation=%#v", evaluation)
	}

	if _, err = store.Mutate(context.Background(), revision, func(next *reviewtransaction.NewLineageAuthority) error {
		next.State = reviewtransaction.NewLineageStateEscalated
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	for _, gate := range []reviewtransaction.GateKind{
		reviewtransaction.GatePostApply, reviewtransaction.GatePreCommit, reviewtransaction.GatePrePush,
		reviewtransaction.GatePrePR, reviewtransaction.GateRelease,
	} {
		t.Run(string(gate), func(t *testing.T) {
			governs, evaluation, discoveryErr := resolveGoverningAuthority(context.Background(), repo, lineage, reviewtransaction.NativeGateRequestInput{Gate: gate})
			if !governs {
				t.Fatalf("gate %q: an escalated v3 record must still govern (deny), got governs=false", gate)
			}
			if discoveryErr != nil {
				// A typed discovery denial is an acceptable deny shape too,
				// as long as it never reports allow.
				return
			}
			if evaluation.Result == reviewtransaction.GateAllow {
				t.Fatalf("gate %q: escalated authority must never allow, got %#v", gate, evaluation)
			}
		})
	}
}
