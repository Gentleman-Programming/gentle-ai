package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// invalidatedRecoverySelfDerivationPredecessor starts a pristine review and
// invalidates it, reaching a StateInvalidated predecessor eligible for the
// simplest self-derivation shape.
func invalidatedRecoverySelfDerivationPredecessor(t *testing.T, lineage string) (string, reviewtransaction.CompactRecord) {
	t.Helper()
	repo := initReviewCLIRepo(t)
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", lineage}, io.Discard); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	predecessor, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := RunReviewInvalidate([]string{
		"--cwd", repo, "--lineage", lineage, "--expected-revision", predecessor.Revision, "--reason", "operator abandoned",
	}, io.Discard); err != nil {
		t.Fatal(err)
	}
	predecessor, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if predecessor.State.State != reviewtransaction.StateInvalidated {
		t.Fatalf("predecessor fixture did not reach StateInvalidated: %#v", predecessor.State)
	}
	return repo, predecessor
}

// TestReviewRecoverSelfDerivesBindingForInvalidatedPredecessor is the
// flagship positive proof of Duty 2: `review recover` with only
// --predecessor-lineage, --expected-predecessor-revision, --successor-lineage
// and --disposition — no --actor, --reason, or --maintainer-authorization —
// succeeds, and the persisted CAS audit entry carries a Git-derived actor and
// a closed machine-generated reason, never an empty or operator-invented one.
func TestReviewRecoverSelfDerivesBindingForInvalidatedPredecessor(t *testing.T) {
	repo, predecessor := invalidatedRecoverySelfDerivationPredecessor(t, "self-derive-invalidated")
	runReviewCLIGit(t, repo, "config", "user.name", "Ada Lovelace")
	runReviewCLIGit(t, repo, "config", "user.email", "ada@example.com")

	var output bytes.Buffer
	err := RunReviewRecover([]string{
		"--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision,
		"--successor-lineage", "self-derive-invalidated-successor", "--disposition", "invalidated",
	}, &output)
	if err != nil {
		t.Fatalf("self-derived invalidated recovery rejected: %v", err)
	}
	var recovered ReviewRecoverResult
	if err := json.Unmarshal(output.Bytes(), &recovered); err != nil {
		t.Fatalf("decode recover result: %v\n%s", err, output.String())
	}
	successorStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "self-derive-invalidated-successor")
	if err != nil {
		t.Fatal(err)
	}
	successor, err := successorStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	if successor.State.Recovery == nil || successor.State.Recovery.Actor != "Ada Lovelace <ada@example.com>" {
		t.Fatalf("recovery actor = %#v, want the git-derived identity", successor.State.Recovery)
	}
	if successor.State.Recovery.Reason != "self-recovery: invalidated authority, fresh successor lineage required" {
		t.Fatalf("recovery reason = %q, want the closed machine-generated reason", successor.State.Recovery.Reason)
	}
}

// TestReviewRecoverSelfDerivedBindingRoundTripsAcrossActorSources proves the
// same self-derivation seam falls through to GIT_AUTHOR_EMAIL when no local
// Git identity is configured, matching reviewAuditActor's own fallback chain.
func TestReviewRecoverSelfDerivedBindingFallsBackToEnvActor(t *testing.T) {
	repo, predecessor := invalidatedRecoverySelfDerivationPredecessor(t, "self-derive-env-actor")
	// initReviewCLIRepo (via the fixture above) sets a local Git identity;
	// remove it and isolate global/system config so the env fallback below
	// is genuinely reached, matching reviewAuditActor's own isolation.
	runReviewCLIGit(t, repo, "config", "--unset", "user.name")
	runReviewCLIGit(t, repo, "config", "--unset", "user.email")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "unused-gitconfig"))
	t.Setenv("GIT_AUTHOR_EMAIL", "author@example.com")
	t.Setenv("GIT_COMMITTER_EMAIL", "")

	err := RunReviewRecover([]string{
		"--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision,
		"--successor-lineage", "self-derive-env-actor-successor", "--disposition", "invalidated",
	}, io.Discard)
	if err != nil {
		t.Fatalf("self-derived invalidated recovery rejected: %v", err)
	}
	successorStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "self-derive-env-actor-successor")
	if err != nil {
		t.Fatal(err)
	}
	successor, err := successorStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	if successor.State.Recovery == nil || successor.State.Recovery.Actor != "author@example.com" {
		t.Fatalf("recovery actor = %#v, want the GIT_AUTHOR_EMAIL fallback", successor.State.Recovery)
	}
}

// TestReviewRecoverSelfDerivationExplicitWrongBindingStillRefuses is
// fail-closed negative #1: an operator-supplied, explicitly wrong
// --maintainer-authorization is never silently corrected by self-derivation.
// Presence of the flag (not its value) is what disables derivation. Uses an
// escalated, target-changed predecessor: unlike invalidated recovery, this
// shape actually validates the authorization binding downstream.
func TestReviewRecoverSelfDerivationExplicitWrongBindingStillRefuses(t *testing.T) {
	repo, baseRef, predecessor := escalatedCurrentChangesRecoveryFixture(t, "self-derive-wrong-binding")
	err := RunReviewRecover([]string{
		"--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision,
		"--successor-lineage", "self-derive-wrong-binding-successor", "--disposition", "escalated",
		"--base-ref", baseRef, "--committed-only",
		"--actor", "maintainer", "--reason", "manual recovery",
		"--maintainer-authorization", "not the exact binding",
	}, io.Discard)
	if err == nil {
		t.Fatal("explicit wrong maintainer-authorization was silently accepted by self-derivation")
	}
	successorStore, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "self-derive-wrong-binding-successor")
	if _, statErr := os.Stat(successorStore.StatePath()); !os.IsNotExist(statErr) {
		t.Fatalf("rejected recovery created a successor: %v", statErr)
	}
}

// TestReviewRecoverSelfDerivationCorruptedAuthorityStillRefuses is
// fail-closed negative #2: a corrupted predecessor store still refuses
// before self-derivation ever runs (Load() fails first).
func TestReviewRecoverSelfDerivationCorruptedAuthorityStillRefuses(t *testing.T) {
	repo, predecessor := invalidatedRecoverySelfDerivationPredecessor(t, "self-derive-corrupted")
	predecessorStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, predecessor.State.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(predecessorStore.StatePath(), []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = RunReviewRecover([]string{
		"--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision,
		"--successor-lineage", "self-derive-corrupted-successor", "--disposition", "invalidated",
	}, io.Discard)
	if err == nil {
		t.Fatal("corrupted predecessor authority was accepted by self-derivation")
	}
	successorStore, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "self-derive-corrupted-successor")
	if _, statErr := os.Stat(successorStore.StatePath()); !os.IsNotExist(statErr) {
		t.Fatalf("rejected recovery created a successor: %v", statErr)
	}
}

// TestReviewRecoverSelfDerivationActiveAttemptNeverAutoResets is fail-closed
// negative #3: an ACTIVE (StateReviewing) predecessor is not a derivable
// shape, so self-derivation refuses to fabricate a binding for it, and the
// pre-existing manual-flow requirement (explicit --actor/--reason/
// --maintainer-authorization) — which itself refuses for this predecessor
// state — still governs.
func TestReviewRecoverSelfDerivationActiveAttemptNeverAutoResets(t *testing.T) {
	repo := initReviewCLIRepo(t)
	lineage := "self-derive-active-attempt"
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", lineage}, io.Discard); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	predecessor, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if predecessor.State.State != reviewtransaction.StateReviewing {
		t.Fatalf("predecessor fixture is not an ACTIVE attempt: %#v", predecessor.State)
	}
	err = RunReviewRecover([]string{
		"--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision,
		"--successor-lineage", "self-derive-active-attempt-successor", "--disposition", "invalidated",
	}, io.Discard)
	if err == nil {
		t.Fatal("self-derivation auto-reset an ACTIVE attempt")
	}
	successorStore, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "self-derive-active-attempt-successor")
	if _, statErr := os.Stat(successorStore.StatePath()); !os.IsNotExist(statErr) {
		t.Fatalf("rejected recovery created a successor: %v", statErr)
	}
}

// TestReviewRecoverSelfDerivationFailedCriteriaEscalationNeverAutoRecovers
// is fail-closed negative #4. The predecessor is genuinely StateEscalated
// with an unchanged target, so self-derivation confidently picks the
// accounting-only reason and computes a binding — but this predecessor is
// NOT accounting-only-eligible (its escalation came from an over-budget
// forecast, not a completed correction attempt whose criteria passed), so
// reviewtransaction.RecoverCompactAuthority still refuses exactly as it did
// before self-derivation existed. Self-derivation adds no authority.
func TestReviewRecoverSelfDerivationFailedCriteriaEscalationNeverAutoRecovers(t *testing.T) {
	repo, _, predecessor := escalatedCurrentChangesRecoveryFixture(t, "self-derive-failed-criteria")
	err := RunReviewRecover([]string{
		"--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision,
		"--successor-lineage", "self-derive-failed-criteria-successor", "--disposition", "escalated",
	}, io.Discard)
	if err == nil {
		t.Fatal("self-derivation auto-recovered a non-accounting-only escalation over its unchanged target")
	}
	successorStore, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "self-derive-failed-criteria-successor")
	if _, statErr := os.Stat(successorStore.StatePath()); !os.IsNotExist(statErr) {
		t.Fatalf("rejected recovery created a successor: %v", statErr)
	}
}

// TestReviewRecoverSelfDerivationNoDriftElectiveResetStillRefuses is
// fail-closed negative #5: a scope-changed recovery over a workspace-overlay
// approved predecessor whose live target has not actually drifted still
// refuses under the existing drift gate ("recovery scope has not changed"),
// which runs entirely before self-derivation.
func TestReviewRecoverSelfDerivationNoDriftElectiveResetStillRefuses(t *testing.T) {
	repo, predecessor := approvedWorkspaceOverlayRecoveryPredecessor(t, "self-derive-no-drift")
	err := RunReviewRecover([]string{
		"--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision,
		"--successor-lineage", "self-derive-no-drift-successor", "--disposition", "scope_changed",
	}, io.Discard)
	if err == nil || err.Error() != "recovery scope has not changed" {
		t.Fatalf("no-drift elective reset error = %v, want the unchanged drift-gate refusal", err)
	}
	successorStore, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "self-derive-no-drift-successor")
	if _, statErr := os.Stat(successorStore.StatePath()); !os.IsNotExist(statErr) {
		t.Fatalf("rejected recovery created a successor: %v", statErr)
	}
}
