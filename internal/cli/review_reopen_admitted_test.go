package cli

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestLastRecapturedLensDrivesTheCurrentCorrectionPlan ensures the final lens
// capture, rather than a retired follow-up operation, is what opens the one
// bounded correction route.
func TestLastRecapturedLensDrivesTheCurrentCorrectionPlan(t *testing.T) {
	reviewEnabledHome(t)
	repo, started, store, record := newArtifactReview(t, true)
	for order, lens := range record.State.SelectedLenses {
		findings := []facadeFinding{}
		if order == len(record.State.SelectedLenses)-1 {
			findings = []facadeFinding{{
				ID: reviewtransaction.FindingIDPrefixForLens(lens) + "001", Lens: lens, Location: record.State.InitialSnapshot.Paths[0] + ":1",
				Severity: "CRITICAL", Claim: "the final reviewed line is incorrect",
				ProofRefs: []string{"the changed line was inspected"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
				CausalDisposition: reviewtransaction.CausalIntroduced,
			}}
		}
		var output bytes.Buffer
		captureCLIReviewerResultWithFindings(t, repo, started, order, findings, &output)
	}

	current, err := store.Load()
	if err != nil || current.State.State != reviewtransaction.StateCorrectionRequired {
		t.Fatalf("last capture state = %#v, %v", current, err)
	}
	request, err := reviewtransaction.BuildCorrectionPlanRequest(current.State, current.State.CapturePhaseRevision)
	if err != nil {
		t.Fatal(err)
	}
	if err := RunReviewCaptureCorrectionPlan([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", request.TargetIdentity,
		"--expected-revision", current.State.CapturePhaseRevision, "--request-hash", request.RequestHash,
		"--correction-lines", fmt.Sprint(1),
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("capture current correction plan: %v", err)
	}
	after, err := store.Load()
	if err != nil || after.State.ProposedCorrectionLines == nil || *after.State.ProposedCorrectionLines != 1 {
		t.Fatalf("captured correction plan = %#v, %v", after, err)
	}
}
