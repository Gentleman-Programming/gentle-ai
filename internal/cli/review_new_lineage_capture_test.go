package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// Wave 5 fix cycle 2, CRITICAL-A (verify-report #10186, coordinator's
// "minimal capture primitive" decision, NOT a rebuilt v2-shaped admission
// pipeline): the exact verify repro is the RED shape below --
// `GENTLE_AI_RDD_NEW_LINEAGE=1 gentle-ai review start --lineage med1`
// (medium tier, consent granted) then `gentle-ai review finalize --lineage
// med1` fails with "new-lineage finalize requires captured results for
// every frozen selected lens before approving" because no production code
// ever set FinalizeAdvanceRequest.CapturedLensResults and `review
// capture-result` bound only the v2 compact store.

// startMediumTierNewLineage starts a v3 lineage whose content classifies as
// RiskMedium: an executable (non-documentation) file change is enough
// (risk_test.go's own "remaining executable change is medium" fixture:
// Stats{Additions: 1} on a .go path). Consent is granted through the plain
// (non-negotiated) one-time console path: authorizeReviewStart's own
// non-interactive branch (review_mode.go, "!console.Interactive") shows the
// "reviewed without asking" notice and proceeds without blocking -- the
// exact shape a non-interactive test process reaches with no --consent flag
// at all (--consent itself requires the negotiated --contract form, a
// separate surface this fixture deliberately does not use, matching the
// verify repro's own plain `review start` shape).
func startMediumTierNewLineage(t *testing.T, repo, lineage string) ReviewFacadeStartNewLineageResult {
	t.Helper()
	t.Setenv("GENTLE_AI_RDD_NEW_LINEAGE", "1")
	writeReviewStartCandidate(t, repo, "lib/"+lineage+".go", "package lib\n\nfunc Example() int { return 1 }\n", 0o644)
	var out bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", lineage}, &out); err != nil {
		t.Fatalf("medium-tier new-lineage start: %v\n%s", err, out.String())
	}
	var started ReviewFacadeStartNewLineageResult
	decodeStrictReviewJSON(t, out.Bytes(), &started)
	if started.LineageID != lineage || started.State != reviewtransaction.NewLineageStateReviewing {
		t.Fatalf("medium-tier new-lineage start = %#v", started)
	}
	if started.RiskLevel != reviewtransaction.RiskMedium {
		t.Fatalf("fixture sanity check failed: tier = %q, want medium (the exact verify repro tier)", started.RiskLevel)
	}
	if len(started.SelectedLenses) == 0 {
		t.Fatalf("fixture sanity check failed: medium tier selected zero lenses, nothing to capture")
	}
	t.Setenv("GENTLE_AI_RDD_NEW_LINEAGE", "")
	return started
}

// captureNewLineageLensResults submits one strict reviewer result for every
// one of the lineage's frozen selected lenses through the real CLI
// `review capture-result`, deriving each lens's provider-owned subject hash
// via --preflight first (the exact discovery path an operator or reviewing
// agent uses) rather than reimplementing the derivation in the test.
func captureNewLineageLensResults(t *testing.T, repo, lineage string, lenses []string) {
	t.Helper()
	for order, lens := range lenses {
		var preflightOut bytes.Buffer
		if err := RunReviewCaptureResult([]string{
			"--cwd", repo, "--lineage", lineage, "--target", "unused-for-new-lineage",
			"--lens", lens, "--order", strconv.Itoa(order), "--preflight",
		}, &preflightOut); err != nil {
			t.Fatalf("capture-result preflight for lens %q: %v\n%s", lens, err, preflightOut.String())
		}
		var preflight ReviewFacadeCaptureResultNewLineageResult
		decodeStrictReviewJSON(t, preflightOut.Bytes(), &preflight)
		if preflight.SubjectHash == "" {
			t.Fatalf("capture-result preflight for lens %q returned no subject hash: %#v", lens, preflight)
		}

		inputPath := filepath.Join(t.TempDir(), lens+".json")
		payload := `{"subject_hash":"` + preflight.SubjectHash + `","findings":[],"evidence":["reviewed"]}`
		if err := os.WriteFile(inputPath, []byte(payload), 0o644); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := RunReviewCaptureResult([]string{
			"--cwd", repo, "--lineage", lineage, "--target", "unused-for-new-lineage",
			"--lens", lens, "--order", strconv.Itoa(order), "--input", inputPath,
		}, &out); err != nil {
			t.Fatalf("capture-result for lens %q: %v\n%s", lens, err, out.String())
		}
		var result ReviewFacadeCaptureResultNewLineageResult
		decodeStrictReviewJSON(t, out.Bytes(), &result)
		if result.SubjectHash != preflight.SubjectHash {
			t.Fatalf("capture-result for lens %q did not echo the preflight subject hash: got %q want %q", lens, result.SubjectHash, preflight.SubjectHash)
		}
	}
}

// TestReviewFacadeCaptureResultNewLineage_MediumTierFinalizeAllowsAllFiveGates
// is C-A's END STATE: the coordinator's exact repro (medium-tier v3, consent
// granted, capture via the real CLI, finalize -> approved) then all five
// gates allow. Before this fix, finalize refused with
// ErrFinalizeRequiresLensResults and there was no way to satisfy it.
func TestReviewFacadeCaptureResultNewLineage_MediumTierFinalizeAllowsAllFiveGates(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	const lineage = "medium-tier-capture-lineage"
	started := startMediumTierNewLineage(t, repo, lineage)

	captureNewLineageLensResults(t, repo, lineage, started.SelectedLenses)

	var finalizeOut bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", lineage, "--captured-results=true"}, &finalizeOut); err != nil {
		t.Fatalf("finalize after capturing every selected lens: %v\n%s", err, finalizeOut.String())
	}
	var finalized ReviewFacadeFinalizeNewLineageResult
	decodeStrictReviewJSON(t, finalizeOut.Bytes(), &finalized)
	if finalized.State != reviewtransaction.NewLineageStateApproved {
		t.Fatalf("finalize(captured) state = %q, want approved", finalized.State)
	}

	dir := t.TempDir()
	releaseArtifact := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	releaseArgs := []string{
		"--release-configuration", releaseArtifact("configuration.txt", "configuration\n"),
		"--release-generated", releaseArtifact("generated.txt", "generated\n"),
		"--release-provenance", releaseArtifact("provenance.txt", "provenance\n"),
		"--release-publication-boundary", releaseArtifact("publication-boundary.txt", "publication boundary\n"),
		"--release-evidence-freshness", releaseArtifact("evidence-freshness.txt", "evidence freshness\n"),
	}
	runReviewCLIGit(t, repo, "add", "-A")

	for _, gate := range []struct {
		kind  reviewtransaction.GateKind
		extra []string
	}{
		{kind: reviewtransaction.GatePostApply},
		{kind: reviewtransaction.GatePreCommit},
		{kind: reviewtransaction.GatePrePush},
		{kind: reviewtransaction.GatePrePR},
		{kind: reviewtransaction.GateRelease, extra: releaseArgs},
	} {
		t.Run(string(gate.kind), func(t *testing.T) {
			args := append([]string{"--cwd", repo, "--lineage", lineage, "--gate", string(gate.kind)}, gate.extra...)
			var out bytes.Buffer
			if err := RunReviewFacadeValidate(args, &out); err != nil {
				t.Fatalf("gate %q must allow the approved medium-tier v3 candidate: %v\n%s", gate.kind, err, out.String())
			}
			assertReviewGateResult(t, out.Bytes(), reviewtransaction.GateAllow)
		})
	}
}

// TestReviewFacadeCaptureResultNewLineage_CandidateCausalBlockerEscalates is
// CRITICAL-E's exact A/B repro, inverted (Wave 5 fix cycle 3, verify-report
// #10186 cycle 2): `review capture-result` on a v3 lineage validated a
// reviewer result's findings/evidence and then discarded the findings --
// CaptureLensResult persisted only {Lens, Order, SubjectHash}, so a
// reviewer-reported, deterministic, candidate-introduced BLOCKER never
// reached AdmitCandidateCausalFindings, and finalize --captured-results
// issued `approved` where the identical input escalates on the
// --admission-findings channel. This test submits the identical BLOCKER
// shape the verify report used (severity BLOCKER, evidence_class
// deterministic, causal_disposition introduced) through the real capture
// CLI and asserts finalize now escalates with the finding admitted -- the
// SAME outcome (blocks) the v2/compact path reaches as correction_required
// and the v3 --admission-findings channel already reached as escalated.
func TestReviewFacadeCaptureResultNewLineage_CandidateCausalBlockerEscalates(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	const lineage = "candidate-causal-blocker-lineage"
	started := startMediumTierNewLineage(t, repo, lineage)
	lens := started.SelectedLenses[0]

	var preflightOut bytes.Buffer
	if err := RunReviewCaptureResult([]string{
		"--cwd", repo, "--lineage", lineage, "--target", "unused-for-new-lineage",
		"--lens", lens, "--order", "0", "--preflight",
	}, &preflightOut); err != nil {
		t.Fatalf("capture-result preflight: %v\n%s", err, preflightOut.String())
	}
	var preflight ReviewFacadeCaptureResultNewLineageResult
	decodeStrictReviewJSON(t, preflightOut.Bytes(), &preflight)

	inputPath := filepath.Join(t.TempDir(), "blocker.json")
	payload := `{
		"subject_hash": "` + preflight.SubjectHash + `",
		"findings": [{
			"id": "R3-boom-div-zero",
			"lens": "` + lens + `",
			"location": "lib/boom.go:1",
			"severity": "BLOCKER",
			"claim": "candidate introduces a division by zero",
			"proof_refs": ["lib/boom.go:1"],
			"evidence_class": "deterministic",
			"causal_disposition": "introduced"
		}],
		"evidence": ["reviewed lib/boom.go and confirmed the divide-by-zero"]
	}`
	if err := os.WriteFile(inputPath, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	var captureOut bytes.Buffer
	if err := RunReviewCaptureResult([]string{
		"--cwd", repo, "--lineage", lineage, "--target", "unused-for-new-lineage",
		"--lens", lens, "--order", "0", "--input", inputPath,
	}, &captureOut); err != nil {
		t.Fatalf("capture-result with a candidate-causal BLOCKER: %v\n%s", err, captureOut.String())
	}

	var finalizeOut bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", lineage, "--captured-results=true"}, &finalizeOut); err != nil {
		t.Fatalf("finalize after capturing a candidate-causal BLOCKER: %v\n%s", err, finalizeOut.String())
	}
	var finalized ReviewFacadeFinalizeNewLineageResult
	decodeStrictReviewJSON(t, finalizeOut.Bytes(), &finalized)
	if finalized.State != reviewtransaction.NewLineageStateEscalated {
		t.Fatalf("finalize after a captured candidate-causal BLOCKER = %q, want escalated (CRITICAL-E: a captured BLOCKER must not silently approve)", finalized.State)
	}
	if len(finalized.AdmittedFindingIDs) != 1 || finalized.AdmittedFindingIDs[0] != "R3-boom-div-zero" {
		t.Fatalf("admitted_finding_ids = %v, want exactly [R3-boom-div-zero]", finalized.AdmittedFindingIDs)
	}
}

// TestReviewFacadeCaptureResultNewLineage_NonCausalFindingDoesNotBlock is the
// companion scenario: a captured finding whose causal_disposition is NOT
// introduced/behavior-activated/worsened (matching --admission-findings'
// own AdmitCandidateCausalFindings rules exactly, reused rather than
// duplicated) is a follow-up, never a blocker -- captured findings must not
// become MORE aggressive than the existing admission machinery already is.
func TestReviewFacadeCaptureResultNewLineage_NonCausalFindingDoesNotBlock(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	const lineage = "non-causal-finding-lineage"
	started := startMediumTierNewLineage(t, repo, lineage)
	lens := started.SelectedLenses[0]

	var preflightOut bytes.Buffer
	if err := RunReviewCaptureResult([]string{
		"--cwd", repo, "--lineage", lineage, "--target", "unused-for-new-lineage",
		"--lens", lens, "--order", "0", "--preflight",
	}, &preflightOut); err != nil {
		t.Fatalf("capture-result preflight: %v\n%s", err, preflightOut.String())
	}
	var preflight ReviewFacadeCaptureResultNewLineageResult
	decodeStrictReviewJSON(t, preflightOut.Bytes(), &preflight)

	inputPath := filepath.Join(t.TempDir(), "pre-existing.json")
	payload := `{
		"subject_hash": "` + preflight.SubjectHash + `",
		"findings": [{
			"id": "pre-existing-finding",
			"lens": "` + lens + `",
			"location": "lib/legacy.go:1",
			"severity": "WARNING",
			"claim": "pre-existing issue, not introduced by this candidate",
			"proof_refs": ["lib/legacy.go:1"],
			"evidence_class": "deterministic",
			"causal_disposition": "pre_existing"
		}],
		"evidence": ["reviewed lib/legacy.go"]
	}`
	if err := os.WriteFile(inputPath, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	var captureOut bytes.Buffer
	if err := RunReviewCaptureResult([]string{
		"--cwd", repo, "--lineage", lineage, "--target", "unused-for-new-lineage",
		"--lens", lens, "--order", "0", "--input", inputPath,
	}, &captureOut); err != nil {
		t.Fatalf("capture-result with a pre-existing finding: %v\n%s", err, captureOut.String())
	}

	var finalizeOut bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", lineage, "--captured-results=true"}, &finalizeOut); err != nil {
		t.Fatalf("finalize after capturing a pre-existing finding: %v\n%s", err, finalizeOut.String())
	}
	var finalized ReviewFacadeFinalizeNewLineageResult
	decodeStrictReviewJSON(t, finalizeOut.Bytes(), &finalized)
	if finalized.State != reviewtransaction.NewLineageStateApproved {
		t.Fatalf("finalize after a captured pre-existing (non-causal) finding = %q, want approved (a follow-up must never block)", finalized.State)
	}
	if len(finalized.AdmittedFindingIDs) != 0 {
		t.Fatalf("admitted_finding_ids = %v, want empty", finalized.AdmittedFindingIDs)
	}
}

// TestReviewFacadeFinalizeNewLineage_PartialCaptureIsOrdinaryNotAFault is
// W-8 (Wave 5 fix cycle 3, verify-report #10186 cycle 2): a partial capture
// (some, not all, frozen selected lenses captured) at finalize used to
// surface as an UNEXPECTED FAULT -- `review core finalize: %w` wrapping
// ErrFinalizeRequiresLensResults, unclassified, which the outer dispatcher
// treated as a tool-internal fault and wrote a defect report for. This is an
// ORDINARY, entirely expected incomplete state (the same category
// reviewIntegrationPreflightError names throughout this package), and the
// message must name exactly which lens is still missing plus the runnable
// capture-result continuation -- never a bare "capture more" or a fault
// report.
func TestReviewFacadeFinalizeNewLineage_PartialCaptureIsOrdinaryNotAFault(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	const lineage = "partial-capture-lineage"
	started := startMediumTierNewLineage(t, repo, lineage)
	if len(started.SelectedLenses) == 0 {
		t.Fatal("fixture sanity check failed: need at least one selected lens to leave uncaptured")
	}
	// Deliberately capture NOTHING, leaving every selected lens missing --
	// the simplest genuine partial-capture shape (0 of N captured is still
	// "some lenses missing", the same code path a partial N-1-of-N capture
	// reaches).
	var out bytes.Buffer
	err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", lineage, "--captured-results=true"}, &out)
	if err == nil {
		t.Fatal("finalize with zero captured lenses against a medium-tier lineage unexpectedly succeeded")
	}

	var progress *reviewFacadeOperationProgressError
	if errors.As(err, &progress) {
		err = progress.Cause
	}
	var preflight *reviewIntegrationPreflightError
	if !errors.As(err, &preflight) {
		t.Fatalf("finalize with an incomplete capture must classify as an ordinary preflight refusal, not fall through to the unclassified/fault path: %v (%T)", err, err)
	}
	for _, lens := range started.SelectedLenses {
		if !strings.Contains(err.Error(), lens) {
			t.Fatalf("refusal = %q, must name the missing lens %q", err.Error(), lens)
		}
	}
	if !strings.Contains(err.Error(), "gentle-ai review capture-result") {
		t.Fatalf("refusal = %q, must name the runnable capture-result continuation", err.Error())
	}
}

// TestReviewFacadeCaptureResultNewLineage_TierLowZeroResultsStillWorks pins
// the non-regression: a tier-low v3 lineage (SelectedLenses is empty) still
// finalizes with zero captured results -- the C-A fix must not require a
// capture that a tier-low lineage never asked for.
func TestReviewFacadeCaptureResultNewLineage_TierLowZeroResultsStillWorks(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	const lineage = "tier-low-zero-results-lineage"
	startNewLineageForFinalizeTest(t, repo, lineage)

	var out bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", lineage, "--captured-results=true"}, &out); err != nil {
		t.Fatalf("tier-low finalize with --captured-results=true and zero results: %v\n%s", err, out.String())
	}
	var result ReviewFacadeFinalizeNewLineageResult
	decodeStrictReviewJSON(t, out.Bytes(), &result)
	if result.State != reviewtransaction.NewLineageStateApproved {
		t.Fatalf("tier-low finalize(captured-results, zero lenses) state = %q, want approved", result.State)
	}
}
