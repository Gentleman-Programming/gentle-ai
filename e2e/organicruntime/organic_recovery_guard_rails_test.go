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

import (
	"strings"
	"testing"
)

// approveOrganicHealthyCandidate builds the exact state the recovery guard
// rails defend: one passive tier-0 candidate reviewed and approved, whose
// lifecycle gate allows.
func approveOrganicHealthyCandidate(t *testing.T, harness *organicHarness, lineage string) string {
	t.Helper()
	harness.writeFiles(map[string]string{
		"docs/healthy.md": "# healthy\n\nplain prose, no executable content.\n",
	})
	harness.git("add", "-A")
	started, _ := harness.startReview(lineage)
	if len(started.SelectedLenses) != 0 {
		t.Fatalf("passive prose selected lenses: %#v", started.SelectedLenses)
	}
	if approved := harness.finalize(lineage); approved.State != organicStateApproved {
		t.Fatalf("passive prose did not approve: %#v", approved)
	}
	if allowed := harness.gate("post-apply"); allowed.Result != organicGateAllow || !allowed.Allowed {
		t.Fatalf("fixture is not a healthy approved authority: %#v", allowed)
	}
	return harnessStatus(t, harness, lineage).Authority.Revision
}

// changeOrganicHealthyCandidate performs the world action all three refusals
// ask for: the reviewed candidate stops being the bytes that were approved.
func changeOrganicHealthyCandidate(harness *organicHarness) {
	harness.writeFiles(map[string]string{
		"docs/changed.md": "# changed\n\nplain prose, no executable content.\n",
	})
	harness.git("add", "-A")
}

// followOrganicGuardRailContinuation runs the command a guard-rail refusal
// named, after the world action, and requires the lineage it creates to reach
// a real `allow`. Nothing here is knowledge the test has independently: the
// argv comes out of the product's own message.
func followOrganicGuardRailContinuation(t *testing.T, harness *organicHarness, arguments []string, successor string) {
	t.Helper()
	changeOrganicHealthyCandidate(harness)
	if stdout, stderr, err := harness.gentleAllowFailure(arguments...); err != nil {
		t.Fatalf("the continuation the refusal named failed: gentle-ai %v: %v\nstdout:\n%s\nstderr:\n%s",
			arguments, err, stdout, stderr)
	}
	harness.finalize(successor)
	if allowed := harness.gate("pre-commit"); allowed.Result != organicGateAllow || !allowed.Allowed {
		t.Fatalf("gate still denies after the continuation the refusal named: %#v", allowed)
	}
}

// TestOrganicApprovedUnchangedScopeRecoveryNamesWhatToChange is the first leg
// of the reported triangle. Recovering an approved lineage into a successor
// whose scope has not moved is refused, and correctly so: the successor would
// freeze the exact bytes the predecessor already approved.
//
// The refusal used to be the bare sentence "approved predecessor scope has not
// changed", which names neither why nor what to do. It must say the candidate
// has to change, and name the command that then works.
func TestOrganicApprovedUnchangedScopeRecoveryNamesWhatToChange(t *testing.T) {
	harness := newOrganicHarness(t)
	const lineage = "approved-unchanged-scope"
	revision := approveOrganicHealthyCandidate(t, harness, lineage)

	_, stderr, err := harness.gentleAllowFailure(
		"review", "recover",
		"--predecessor-lineage", lineage,
		"--expected-predecessor-revision", revision,
		"--successor-lineage", lineage+"-successor",
		"--disposition", "scope_changed",
	)
	if err == nil {
		t.Fatalf("recovery of an unchanged approved scope was allowed")
	}
	message := strings.TrimSpace(stderr)
	if !strings.Contains(message, "nothing to re-review") {
		t.Fatalf("unchanged approved scope refusal does not say why it refuses:\n%s", message)
	}
	if !strings.Contains(message, "change the candidate") {
		t.Fatalf("unchanged approved scope refusal does not say the candidate has to change:\n%s", message)
	}
	arguments, placeholders := organicNamedCommand(t, message)
	if len(placeholders) != 0 {
		t.Fatalf("unchanged approved scope refusal named placeholders it already knows the values for: %v\n%s",
			placeholders, message)
	}

	followOrganicGuardRailContinuation(t, harness, arguments, lineage+"-successor")
}

// TestOrganicInvalidatedDispositionRefusalNamesTheDispositionThatWorks is the
// second leg, and the one that closed the reported loop. Told that recovery
// needs an invalidated predecessor, the tester tried to invalidate the
// predecessor, was refused, and concluded no successor could ever exist.
//
// The refusal must therefore name the lineage's real state and route to the
// disposition an approved predecessor actually accepts — never back to
// `review invalidate`, which is the dead end this loop is made of.
func TestOrganicInvalidatedDispositionRefusalNamesTheDispositionThatWorks(t *testing.T) {
	harness := newOrganicHarness(t)
	const lineage = "approved-invalidated-disposition"
	revision := approveOrganicHealthyCandidate(t, harness, lineage)

	_, stderr, err := harness.gentleAllowFailure(
		"review", "recover",
		"--predecessor-lineage", lineage,
		"--expected-predecessor-revision", revision,
		"--successor-lineage", lineage+"-successor",
		"--disposition", "invalidated",
	)
	if err == nil {
		t.Fatalf("recovery with an invalidated disposition over an approved predecessor was allowed")
	}
	message := strings.TrimSpace(stderr)
	if !strings.Contains(message, organicStateApproved) {
		t.Fatalf("invalidated-disposition refusal does not name the predecessor's real state:\n%s", message)
	}
	if !strings.Contains(message, "change the candidate") {
		t.Fatalf("invalidated-disposition refusal does not say the candidate has to change:\n%s", message)
	}
	arguments, placeholders := organicNamedCommand(t, message)
	if len(placeholders) != 0 {
		t.Fatalf("invalidated-disposition refusal named placeholders it already knows the values for: %v\n%s",
			placeholders, message)
	}
	// The one thing the message must never do is send the operator to
	// `review invalidate`, which refuses a healthy approved authority and
	// completes the reported deadlock. The verb is checked, not the whole
	// argv, because a lineage identifier may legitimately carry the word.
	if len(arguments) < 2 || arguments[0] != "review" || arguments[1] == "invalidate" {
		t.Fatalf("invalidated-disposition refusal routed back into the reported loop: gentle-ai %v\n%s",
			arguments, message)
	}

	followOrganicGuardRailContinuation(t, harness, arguments, lineage+"-successor")
}

// TestOrganicHealthyInvalidationRefusalNamesTheWorldAction is the third leg.
// Invalidating a healthy approved authority is refused, and that refusal
// protects an approval the operator earned.
//
// It used to answer in the authority model's own vocabulary — `native gate
// result is "allow"` — which is a fact about an internal evaluation the
// operator never asked for. It must instead state what is true of their
// lineage, and name the one continuation that exists once the candidate moves.
// The successor's name is theirs to choose, so exactly one value stays a
// placeholder and every other value is already filled in.
func TestOrganicHealthyInvalidationRefusalNamesTheWorldAction(t *testing.T) {
	harness := newOrganicHarness(t)
	const lineage = "approved-healthy-invalidation"
	revision := approveOrganicHealthyCandidate(t, harness, lineage)

	_, stderr, err := harness.gentleAllowFailure(
		"review", "invalidate",
		"--cwd", harness.repo.worktree,
		"--lineage", lineage,
		"--expected-revision", revision,
		"--gate", "post-apply",
	)
	if err == nil {
		t.Fatalf("invalidation of a healthy approved authority was allowed")
	}
	message := strings.TrimSpace(stderr)
	if strings.Contains(message, "native gate result") {
		t.Fatalf("healthy invalidation refusal answers in internal vocabulary:\n%s", message)
	}
	if !strings.Contains(message, "change the candidate") {
		t.Fatalf("healthy invalidation refusal does not say the candidate has to change:\n%s", message)
	}
	arguments, placeholders := organicNamedCommand(t, message)
	if len(placeholders) != 1 {
		t.Fatalf("healthy invalidation refusal named %d operator-supplied placeholders, want exactly the successor name: %v\n%s",
			len(placeholders), placeholders, message)
	}
	arguments = organicFillPlaceholder(arguments, placeholders[0], lineage+"-successor")

	followOrganicGuardRailContinuation(t, harness, arguments, lineage+"-successor")
}
