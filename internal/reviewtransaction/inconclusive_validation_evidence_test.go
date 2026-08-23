package reviewtransaction

import "testing"

// TestInconclusiveValidationEvidenceRecognisesPassiveInspectionFailure is the
// RED-first proof for issue #3378's narrower defect: the detector matched only
// the active voice ("could not inspect"), so the passive construction real
// validators write ("could not be inspected") was admitted as a genuine failed
// verdict and consumed the lineage's single correction attempt on a
// non-observation. The first string is verbatim field evidence captured on a
// maintainer machine; the rest are the neighbouring passive constructions the
// same phrasing family produces.
func TestInconclusiveValidationEvidenceRecognisesPassiveInspectionFailure(t *testing.T) {
	t.Parallel()

	for _, evidence := range []string{
		"Immutable correction candidate tree sha256:9f2c could not be inspected with read-only Git access; the supplied diff appears to remove the last-component omission, but that is insufficient for a verified verdict.",
		"The frozen candidate tree cannot be inspected from this sandbox, so no verdict was produced.",
		"Correction candidate could not be inspected.",
	} {
		if !InconclusiveValidationEvidence([]string{evidence}) {
			t.Errorf("InconclusiveValidationEvidence(%q) = false, want true: passive inspection failure is not a verdict", evidence)
		}
	}
}

// TestInconclusiveValidationEvidenceKeepsGenuineFailedVerdict is the paired
// false-positive guard. Broadening the detector is only safe while a real
// inspection that reports the fix did NOT land stays a verdict: misreading it
// as inconclusive would let an unfixed correction walk past the single
// correction attempt and reach a receipt, which is strictly worse than the
// bug being fixed.
func TestInconclusiveValidationEvidenceKeepsGenuineFailedVerdict(t *testing.T) {
	t.Parallel()

	for _, evidence := range []string{
		"Inspected the correction candidate tree: the loop still stops one entry short, so the finding is not fixed",
		"Inspected tracked.txt in the frozen candidate tree; the last component is still omitted and the regression check is not satisfied",
		"Read the immutable candidate with read-only Git access: the corrected hunk is present and both criteria pass",
	} {
		if InconclusiveValidationEvidence([]string{evidence}) {
			t.Errorf("InconclusiveValidationEvidence(%q) = true, want false: a completed inspection is a verdict", evidence)
		}
	}
}
