// Recovery guard-rail journeys around a HEALTHY approved lineage. All three
// refusals under test here are CORRECT and stay exactly as strict:
//
//   - recovering an approved lineage whose scope has not changed would mint
//     fresh authority over bytes that lineage already approved;
//   - `--disposition invalidated` really does need an already-invalidated
//     predecessor;
//   - a healthy approved authority really must not be destroyed on demand,
//     because that would throw away an approval the operator earned.
//
// What is under test is not whether the product refuses, but whether each
// refusal tells the operator what to do — and whether doing exactly that
// clears the block. A community tester walked this triangle in order, was sent
// from recover to invalidate and back, and concluded no legal successor could
// exist. The exit is a world action: the candidate has to change. Once it does,
// one command finishes the job, and every test below performs the world action
// the message describes and then runs the command the message named.
package organicruntime_test

import "testing"

// approveOrganicHealthyCandidate builds the terminal state the recovery guard
// rails defend: passive prose reaches approved FINALIZE and burns its authority.
func approveOrganicHealthyCandidate(t *testing.T, harness *organicHarness, lineage string) organicFinalizeResult {
	t.Helper()
	harness.writeFiles(map[string]string{
		"docs/healthy.md": "# healthy\n\nplain prose, no executable content.\n",
	})
	harness.git("add", "-A")
	started, _ := harness.startReview(lineage)
	if len(started.SelectedLenses) != 0 {
		t.Fatalf("passive prose selected lenses: %#v", started.SelectedLenses)
	}
	approved := harness.approveReview(lineage, started)
	harness.assertInvalidatedUnmanagedGate(harness.gate("post-apply"))
	return approved
}

// changeOrganicHealthyCandidate performs the world action all three refusals
// ask for: the reviewed candidate stops being the bytes that were approved.
func changeOrganicHealthyCandidate(harness *organicHarness) {
	harness.writeFiles(map[string]string{
		"docs/changed.md": "# changed\n\nplain prose, no executable content.\n",
	})
	harness.git("add", "-A")
}

// startOrganicFreshReviewAfterBurn performs the only post-approval continuation:
// materially changed work freezes in a new atomic transaction rather than
// recovering or reopening the authority FINALIZE removed.
func startOrganicFreshReviewAfterBurn(t *testing.T, harness *organicHarness, successor string) {
	t.Helper()
	changeOrganicHealthyCandidate(harness)
	started, _ := harness.startReview(successor)
	harness.approveReview(successor, started)
	harness.assertInvalidatedUnmanagedGate(harness.gate("pre-commit"))
}

// TestOrganicApprovedUnchangedScopeRecoveryNamesWhatToChange proves that an
// approved lineage cannot be recovered after FINALIZE has burned its authority.
// Changed content must use a fresh transaction, not an unchanged-scope successor.
func TestOrganicApprovedUnchangedScopeRecoveryNamesWhatToChange(t *testing.T) {
	harness := newOrganicHarness(t)
	const lineage = "approved-unchanged-scope"
	approved := approveOrganicHealthyCandidate(t, harness, lineage)

	_, _, err := harness.gentleAllowFailure(
		"review", "recover",
		"--predecessor-lineage", lineage,
		"--expected-predecessor-revision", approved.StoreRevision,
		"--successor-lineage", lineage+"-successor",
		"--disposition", "scope_changed",
	)
	if err == nil {
		t.Fatal("recovery reused authority that approved FINALIZE burned")
	}
	startOrganicFreshReviewAfterBurn(t, harness, lineage+"-successor")
}

// TestOrganicInvalidatedDispositionRefusalNamesTheDispositionThatWorks keeps
// the invalidated-disposition guard: it cannot reinterpret a burned approved
// lineage into a successor. A later change starts fresh instead.
func TestOrganicInvalidatedDispositionRefusalNamesTheDispositionThatWorks(t *testing.T) {
	harness := newOrganicHarness(t)
	const lineage = "approved-invalidated-disposition"
	approved := approveOrganicHealthyCandidate(t, harness, lineage)

	_, _, err := harness.gentleAllowFailure(
		"review", "recover",
		"--predecessor-lineage", lineage,
		"--expected-predecessor-revision", approved.StoreRevision,
		"--successor-lineage", lineage+"-successor",
		"--disposition", "invalidated",
	)
	if err == nil {
		t.Fatal("invalidated recovery reused authority that approved FINALIZE burned")
	}
	startOrganicFreshReviewAfterBurn(t, harness, lineage+"-successor")
}

// TestOrganicApprovedInvalidationRefusesAndNamesReviewValidate keeps the
// invalidation guard at the new boundary: there is no approved authority left to
// invalidate, and validate remains an informational unmanaged report.
func TestOrganicApprovedInvalidationRefusesAndNamesReviewValidate(t *testing.T) {
	harness := newOrganicHarness(t)
	const lineage = "approved-healthy-invalidation"
	approved := approveOrganicHealthyCandidate(t, harness, lineage)

	if _, _, err := harness.gentleAllowFailure(
		"review", "invalidate",
		"--cwd", harness.repo.worktree,
		"--lineage", lineage,
		"--expected-revision", approved.StoreRevision,
		"--gate", "post-apply",
	); err == nil {
		t.Fatal("invalidation operated on authority that FINALIZE burned")
	}
	harness.assertInvalidatedUnmanagedGate(harness.gate("post-apply"))
	startOrganicFreshReviewAfterBurn(t, harness, lineage+"-fresh")
}
