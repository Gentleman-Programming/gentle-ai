package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// contaminatedReviewerPayloadForTest builds a structurally perfect,
// provider-admissible reviewer result whose CRITICAL finding describes content
// the reviewer was never actually shown — the exact artifact a diff-transport
// error produces. Everything about it admits cleanly; only its truth is wrong.
func contaminatedReviewerPayloadForTest(t *testing.T, repo string, record reviewtransaction.CompactRecord, lens string, order int) []byte {
	t.Helper()
	prefix := map[string]string{
		"review-risk": "R1-", "review-readability": "R2-",
		"review-reliability": "R3-", "review-resilience": "R4-",
	}[lens]
	if prefix == "" {
		t.Fatalf("unsupported lens %q", lens)
	}
	result := admittedReviewerResultForTest(t, repo, record, lens, order)
	location := record.State.InitialSnapshot.Paths[0] + ":1"
	result.Findings = []facadeFinding{{
		ID:                prefix + "001",
		Severity:          "CRITICAL",
		Claim:             "the file is not valid source: pseudocode phrases appear where statements are required",
		Location:          location,
		ProofRefs:         []string{location + " in the supplied candidate content"},
		EvidenceClass:     reviewtransaction.EvidenceDeterministic,
		CausalDisposition: reviewtransaction.CausalIntroduced,
	}}
	result.Evidence = []string{"inspection: read the supplied candidate content for " + record.State.InitialSnapshot.Paths[0] + " end to end"}
	payload := mustReviewJSON(t, result)
	return []byte(payload)
}

// TestReviewReopenResultsAdmittedSlotQuarantineRequiresNamedAuthorization
// proves both refusal axes survive the maintainer-authorized extension:
//  1. an unauthorized attempt (no named lens) over fully admitted slots keeps
//     refusing with the existing native classification, and
//  2. an authorization that does not name an admitted slot can never sweep it,
//     even when the request names it.
func TestReviewReopenResultsAdmittedSlotQuarantineRequiresNamedAuthorization(t *testing.T) {
	reviewEnabledHome(t)
	repo, started, store, initial := newArtifactReview(t, true)
	for order, lens := range initial.State.SelectedLenses {
		input := filepath.Join(t.TempDir(), fmt.Sprintf("result-%d.json", order))
		if err := os.WriteFile(input, admittedReviewerPayloadForTest(t, repo, initial, lens, order), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := RunReviewCaptureResult([]string{
			"--cwd", repo, "--lineage", started.LineageID, "--target", initial.State.InitialSnapshot.Identity,
			"--lens", lens, "--order", fmt.Sprint(order), "--input", input,
		}, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	if err := RunReviewFacadeFinalize([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--captured-results",
	}, io.Discard); err != nil {
		t.Fatal(err)
	}
	validating, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	baseArgs := []string{
		"--cwd", repo, "--lineage", started.LineageID, "--expected-revision", validating.Revision,
		"--target", validating.State.InitialSnapshot.Identity,
		"--reason", "two lenses reviewed a truncated candidate transcript", "--actor", "maintainer@example.com",
	}

	// Axis 1: with every slot admitted and no lens named, the native
	// classification still refuses — admitted results never become
	// quarantinable by default.
	if err := RunReview(append([]string{"reopen-results"}, append(append([]string{}, baseArgs...), "--prepare")...), io.Discard); err == nil ||
		!strings.Contains(err.Error(), "found no unusable") {
		t.Fatalf("unauthorized reopen over admitted slots = %v, want native refusal", err)
	}

	// A legacy slot makes a native-only plan possible. Its authorization names
	// only that slot.
	legacyPayload := []byte("{\"lens\":\"" + initial.State.SelectedLenses[1] + "\",\"findings\":[],\"evidence\":[\"reviewed exact candidate tree\"]}\n")
	writeLegacyReviewerSlot(t, store, initial.State.SelectedLenses[1], 1, legacyPayload)
	var nativePrepared bytes.Buffer
	if err := RunReview(append([]string{"reopen-results"}, append(append([]string{}, baseArgs...), "--prepare")...), &nativePrepared); err != nil {
		t.Fatalf("native-only prepare: %v", err)
	}
	var nativePlan ReviewResultReopenResult
	decodeStrictReviewJSON(t, nativePrepared.Bytes(), &nativePlan)
	if nativePlan.Plan == nil || len(nativePlan.Plan.Quarantined) != 1 || nativePlan.Plan.Quarantined[0].SelectedOrder != 1 {
		t.Fatalf("native-only plan = %#v", nativePlan.Plan)
	}

	// Axis 2: the native authorization does not name the admitted
	// review-readability slot, so a request that names it must keep refusing,
	// without mutating the authority.
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := RunReview(append([]string{"reopen-results"}, append(append([]string{}, baseArgs...),
		"--quarantine-lens", "review-readability",
		"--maintainer-authorization", nativePlan.Plan.RequiredMaintainerAuthorization)...), io.Discard); err == nil {
		t.Fatal("authorization that does not name the admitted slot was accepted")
	}
	after, err := store.Load()
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("refused admitted-slot quarantine mutated authority: err=%v", err)
	}

	// The named prepare/apply pair works, and its authorization names the lens.
	var namedPrepared bytes.Buffer
	namedArgs := append(append([]string{}, baseArgs...), "--quarantine-lens", "review-readability", "--prepare")
	if err := RunReview(append([]string{"reopen-results"}, namedArgs...), &namedPrepared); err != nil {
		t.Fatalf("named prepare: %v", err)
	}
	var namedPlan ReviewResultReopenResult
	decodeStrictReviewJSON(t, namedPrepared.Bytes(), &namedPlan)
	if namedPlan.Plan == nil || len(namedPlan.Plan.Quarantined) != 2 || len(namedPlan.Plan.Retained) != 2 ||
		!strings.Contains(namedPlan.Plan.RequiredMaintainerAuthorization, "authorized_lenses=review-readability") {
		t.Fatalf("named plan = %#v", namedPlan.Plan)
	}

	// The named authorization conversely cannot apply without the named
	// request: the plan would differ, so the binding must not match.
	if err := RunReview(append([]string{"reopen-results"}, append(append([]string{}, baseArgs...),
		"--maintainer-authorization", namedPlan.Plan.RequiredMaintainerAuthorization)...), io.Discard); err == nil {
		t.Fatal("named authorization applied without the named request")
	}
}

// TestReviewReopenResultsRecoversContaminatedAdmittedReviewFromCorrectionRequired
// drives the maintainer's incident end to end and follows only what the
// product's messages name: four lenses, two clean, two admitted CRITICALs from
// contaminated reviewer input, correction_required — then the narrated reopen
// route, recapture, and finalize to an approved terminal state over the exact
// same candidate identity, with the overridden bytes preserved.
func TestReviewReopenResultsRecoversContaminatedAdmittedReviewFromCorrectionRequired(t *testing.T) {
	// Not parallel: opting in writes the user's global mode through t.Setenv,
	// which Go forbids in a test that also calls t.Parallel.
	reviewEnabledHome(t)

	repo, started, store, initial := newArtifactReview(t, true)
	if len(initial.State.SelectedLenses) != 4 {
		t.Fatalf("selected lenses = %v, want 4R", initial.State.SelectedLenses)
	}
	candidatePath := filepath.Join(repo, initial.State.InitialSnapshot.Paths[0])
	candidateBefore, err := os.ReadFile(candidatePath)
	if err != nil {
		t.Fatal(err)
	}
	contaminated := map[int]bool{2: true, 3: true}
	for order, lens := range initial.State.SelectedLenses {
		payload := admittedReviewerPayloadForTest(t, repo, initial, lens, order)
		if contaminated[order] {
			payload = contaminatedReviewerPayloadForTest(t, repo, initial, lens, order)
		}
		input := filepath.Join(t.TempDir(), fmt.Sprintf("result-%d.json", order))
		if err := os.WriteFile(input, payload, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := RunReviewCaptureResult([]string{
			"--cwd", repo, "--lineage", started.LineageID, "--target", initial.State.InitialSnapshot.Identity,
			"--lens", lens, "--order", fmt.Sprint(order), "--input", input,
		}, io.Discard); err != nil {
			t.Fatalf("capture %d: %v", order, err)
		}
	}
	if err := RunReviewFacadeFinalize([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--captured-results",
	}, io.Discard); err != nil {
		t.Fatal(err)
	}
	blocked, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if blocked.State.State != reviewtransaction.StateCorrectionRequired {
		t.Fatalf("contaminated CRITICALs landed in %q, want correction_required", blocked.State.State)
	}
	identityBefore := blocked.State.InitialSnapshot.Identity

	// Preserve the admitted contaminated bytes for the byte-preservation
	// assertion after quarantine.
	contaminatedBytes := map[int][]byte{}
	for order := range contaminated {
		slotPath := filepath.Join(store.Dir, reviewtransaction.CompactReviewerResultsDir,
			fmt.Sprintf("%02d-%s.json", order, blocked.State.SelectedLenses[order]))
		payload, readErr := os.ReadFile(slotPath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		contaminatedBytes[order] = payload
	}

	// Follow the product's messages. The collect transition asks for a
	// correction forecast; after the forecast, the stop reason must present
	// both truths, and the second one must name the reopen route runnably.
	for _, contract := range []string{ReviewIntegrationContractV1, ReviewIntegrationContractV2} {
		status := legacyCompatibleFrozenStatusForTest(t, repo, started.LineageID, contract)
		if status.NextTransition == nil {
			t.Fatalf("%s status omitted next transition", contract)
		}
	}
	transition := nextTransitionForTest(t, repo, started.LineageID)
	if transition == nil || transition.Kind != reviewNextTransitionCollect || transition.ReasonCode != "correction_plan_required" {
		t.Fatalf("first transition = %#v, want correction_plan_required collect", transition)
	}
	if err := RunReviewFacadeFinalize([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--correction-lines", "1",
	}, io.Discard); err != nil {
		t.Fatal(err)
	}
	transition = nextTransitionForTest(t, repo, started.LineageID)
	if transition == nil || transition.Kind != reviewNextTransitionStop || transition.ReasonCode != "corrected_candidate_unavailable" {
		t.Fatalf("post-forecast transition = %#v, want corrected_candidate_unavailable stop", transition)
	}
	emission, ok := reviewNarrationRegistry["stop:corrected_candidate_unavailable"]
	if !ok {
		t.Fatal("corrected_candidate_unavailable has no narration statement")
	}
	statement := emission.Statement
	if !strings.Contains(statement, "Change the candidate content") {
		t.Fatalf("narration lost the real-findings truth: %q", statement)
	}
	if !strings.Contains(statement, "review reopen-results") || !strings.Contains(statement, "--quarantine-lens") {
		t.Fatalf("narration does not name the wrong-input exit runnably: %q", statement)
	}

	forecasted, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	baseArgs := []string{
		"--cwd", repo, "--lineage", started.LineageID, "--expected-revision", forecasted.Revision,
		"--target", identityBefore,
		"--reason", "two lenses reviewed a truncated candidate transcript, not the frozen candidate",
		"--actor", "maintainer@example.com",
		"--quarantine-lens", "review-readability", "--quarantine-lens", "review-reliability",
	}
	var prepared bytes.Buffer
	if err := RunReview(append([]string{"reopen-results"}, append(append([]string{}, baseArgs...), "--prepare")...), &prepared); err != nil {
		t.Fatalf("prepare from correction_required: %v", err)
	}
	var preparation ReviewResultReopenResult
	decodeStrictReviewJSON(t, prepared.Bytes(), &preparation)
	if preparation.Plan == nil || len(preparation.Plan.Quarantined) != 2 || len(preparation.Plan.Retained) != 2 {
		t.Fatalf("prepared plan = %#v", preparation.Plan)
	}
	for _, slot := range preparation.Plan.Quarantined {
		if !contaminated[slot.SelectedOrder] || slot.SubjectHash == "" {
			t.Fatalf("quarantined slot is not the named admitted one: %#v", slot)
		}
	}
	if err := RunReview(append([]string{"reopen-results"}, append(append([]string{}, baseArgs...),
		"--maintainer-authorization", preparation.Plan.RequiredMaintainerAuthorization)...), io.Discard); err != nil {
		t.Fatalf("apply reopen from correction_required: %v", err)
	}
	reopened, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if reopened.State.State != reviewtransaction.StateReviewing ||
		reopened.State.InitialSnapshot.Identity != identityBefore ||
		!reflect.DeepEqual(reopened.State.SelectedLenses, blocked.State.SelectedLenses) ||
		reopened.State.CorrectionBudget != blocked.State.CorrectionBudget ||
		reopened.State.CorrectionBudgetPolicy != reviewtransaction.CorrectionBudgetPolicyFloorTwo ||
		len(reopened.State.ResultReopens) != 1 {
		t.Fatalf("reopen changed frozen inputs: %#v", reopened.State)
	}
	if !reflect.DeepEqual(reopened.State.ResultReopens[0].AuthorizedLenses, []string{"review-readability", "review-reliability"}) {
		t.Fatalf("audit does not record the maintainer-named lenses: %#v", reopened.State.ResultReopens[0])
	}
	for _, contract := range []string{ReviewIntegrationContractV1, ReviewIntegrationContractV2} {
		status := legacyCompatibleFrozenStatusForTest(t, repo, started.LineageID, contract)
		if status.NextTransition == nil {
			t.Fatalf("%s status omitted next transition after reopen", contract)
		}
	}

	// Recapture the two reopened lenses with results derived from the real
	// candidate; the retained two stay admitted.
	for order := range contaminated {
		lens := reopened.State.SelectedLenses[order]
		input := filepath.Join(t.TempDir(), fmt.Sprintf("replacement-%d.json", order))
		if err := os.WriteFile(input, admittedReviewerPayloadForTest(t, repo, reopened, lens, order), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := RunReviewCaptureResult([]string{
			"--cwd", repo, "--lineage", started.LineageID, "--target", identityBefore,
			"--lens", lens, "--order", fmt.Sprint(order), "--expected-revision", reopened.Revision, "--input", input,
		}, io.Discard); err != nil {
			t.Fatalf("recapture %d: %v", order, err)
		}
	}

	// Byte preservation: the overridden admitted results survive verbatim in
	// quarantine, exactly where the audit record points.
	for _, slot := range preparation.Plan.Quarantined {
		archivePath, ok := reviewtransaction.ReviewerResultQuarantinePath(store.Dir, reopened.State, slot.SelectedOrder, slot.ArtifactDigest)
		if !ok {
			t.Fatalf("no quarantine destination for slot %d", slot.SelectedOrder)
		}
		archived, readErr := os.ReadFile(archivePath)
		if readErr != nil || !bytes.Equal(archived, contaminatedBytes[slot.SelectedOrder]) {
			t.Fatalf("contaminated admitted bytes were not preserved for slot %d: err=%v", slot.SelectedOrder, readErr)
		}
	}

	if err := RunReviewFacadeFinalize([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--captured-results",
	}, io.Discard); err != nil {
		t.Fatalf("finalize recaptured results: %v", err)
	}
	evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
	if err := os.WriteFile(evidencePath, []byte("go test ./... passed on the unchanged candidate\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var finalized bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--evidence", evidencePath,
	}, &finalized); err != nil {
		t.Fatalf("finalize verification evidence: %v", err)
	}
	result := decodeFacadeFinalize(t, finalized.Bytes())
	if result.State != reviewtransaction.StateApproved || result.Action != reviewApprovedCompactBurnedFinalizeAction || result.ReceiptPath != "" {
		t.Fatalf("reopened review terminal = %#v", result)
	}
	assertApprovedCompactAuthorityBurned(t, store, started.LineageID)

	// The whole point: the candidate never moved.
	candidateAfter, err := os.ReadFile(candidatePath)
	if err != nil || !bytes.Equal(candidateBefore, candidateAfter) {
		t.Fatalf("candidate bytes moved: err=%v", err)
	}
}

func nextTransitionForTest(t *testing.T, repo, lineage string) *ReviewNextTransition {
	t.Helper()
	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1, "--next-transition", "--lineage", lineage,
	}, &output); err != nil {
		t.Fatalf("review status --next-transition: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	return status.NextTransition
}

func legacyCompatibleFrozenStatusForTest(t *testing.T, repo, lineage, contract string) ReviewTargetStatusResult {
	t.Helper()
	args := []string{"status", "--cwd", repo, "--contract", contract, "--lineage", lineage}
	if contract == ReviewIntegrationContractV2 {
		args = append(args, "--agent", "claude-code")
	}
	args = append(args, "--next-transition")
	var output bytes.Buffer
	if err := RunReview(args, &output); err != nil {
		t.Fatalf("review status %s --next-transition: %v\n%s", contract, err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if status.Frozen == nil || status.Frozen.CorrectionBudget != 1 {
		t.Fatalf("%s wire budget = %#v, want legacy budget 1", contract, status.Frozen)
	}
	if bytes.Contains(output.Bytes(), []byte("correction_budget_policy")) {
		t.Fatalf("%s status leaked internal correction budget policy: %s", contract, output.String())
	}
	return status
}
