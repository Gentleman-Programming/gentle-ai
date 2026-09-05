package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// escalatedSelectionReplayFixture is the #1972 predecessor: an escalated
// current-changes authority whose frozen snapshot declared ONLY notes-a.txt,
// while notes-b.txt stayed an eligible-but-unselected untracked path. The
// recovery-time selection can then legitimately diverge from the predecessor's
// frozen declaration.
func escalatedSelectionReplayFixture(t *testing.T, lineage string) (string, reviewtransaction.CompactRecord) {
	t.Helper()
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"notes-a.txt", "notes-b.txt"} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte("untracked but explicitly eligible\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, digest, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).IntendedUntrackedInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	startedBytes, err := runLegacyFacadeStartForTestBytes(t, []string{"--cwd", repo, "--lineage", lineage,
		"--untracked-scope=select", "--expected-untracked-inventory=" + digest,
		"--intended-untracked", "notes-a.txt"})
	if err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, startedBytes, &started)
	escalateReviewForRecovery(t, repo, started)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	predecessor, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if predecessor.State.State != reviewtransaction.StateEscalated ||
		len(predecessor.State.InitialSnapshot.IntendedUntracked) != 1 {
		t.Fatalf("fixture did not reach an escalated authority declaring only notes-a.txt: %#v", predecessor.State.InitialSnapshot)
	}
	return repo, predecessor
}

// TestRecoveryTransitionReplaysAuthorizedUntrackedSelection is #1972 end to
// end. STATUS accepts a recovery-time intended-untracked selection that
// diverges from the predecessor's frozen declaration, authorizes recovery for
// the selection-derived target, and renders the RECOVER transition. Executing
// that exact provider-returned command must create the successor for the SAME
// target STATUS authorized; before the fix the transition carried
// selector_arguments: [] and `review recover` re-derived the target from
// predecessor inheritance alone, refusing its own authorization.
func TestRecoveryTransitionReplaysAuthorizedUntrackedSelection(t *testing.T) {
	reviewEnabledHome(t)
	repo, predecessor := escalatedSelectionReplayFixture(t, "recover-1972")

	// The bounded correction the escalation asked for.
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\ncorrected\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, digest, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).IntendedUntrackedInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	selection := []string{
		"--untracked-scope=select",
		"--intended-untracked", "notes-a.txt", "--intended-untracked", "notes-b.txt",
		"--expected-untracked-inventory", digest,
	}
	probe := selectorTransitionStatus(t, repo, append([]string{"--lineage", predecessor.State.LineageID}, selection...)...)
	if probe.Action != reviewtransaction.TargetStatusActionRecover ||
		probe.ActionDisposition != reviewtransaction.RecoveryEscalated || probe.Authority == nil {
		t.Fatalf("selection-bearing recovery probe = %#v", probe)
	}
	const successor, actor, reason = "successor-1972", "maintainer", "recover the complete authorized candidate"
	authorization := "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + predecessor.State.LineageID +
		"\npredecessor_revision=" + probe.Authority.Revision + "\ntarget_identity=" + probe.TargetIdentity +
		"\nsuccessor_lineage=" + successor + "\nactor=" + actor + "\nreason=" + reason
	status := selectorTransitionStatus(t, repo, append(append([]string{"--lineage", predecessor.State.LineageID}, selection...),
		"--recovery-successor-lineage", successor, "--recovery-reason", reason,
		"--recovery-actor", actor, "--recovery-authorization", authorization)...)
	if status.NextTransition == nil || status.NextTransition.Execute == nil ||
		status.NextTransition.Execute.Operation != "review.recover" ||
		status.NextTransition.ReasonCode != "recovery_authorized" {
		t.Fatalf("selection-bearing recovery transition = %#v", status.NextTransition)
	}

	// Execute the exact provider-returned RECOVER command, verbatim.
	executeSelectorTransition(t, repo, status)

	// The rendered transition must replay the authorized selection.
	selectors := status.NextTransition.Execute.SelectorArguments
	if selectors == nil || len(*selectors) == 0 {
		t.Fatalf("selection-bearing recovery rendered selector_arguments = %#v, want the authorized untracked selection", selectors)
	}
	want := []ReviewTransitionArgument{
		{Name: "untracked-scope", Value: "select"},
		{Name: "expected-untracked-inventory", Value: digest},
		{Name: "intended-untracked", Value: "notes-a.txt"},
		{Name: "intended-untracked", Value: "notes-b.txt"},
	}
	if !reflect.DeepEqual(*selectors, want) {
		t.Fatalf("selection-bearing recovery selectors = %#v, want %#v", *selectors, want)
	}

	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, successor)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if recovered.State.InitialSnapshot.Identity != probe.TargetIdentity {
		t.Fatalf("successor identity %s does not bind the authorized target %s",
			recovered.State.InitialSnapshot.Identity, probe.TargetIdentity)
	}
	if got := recovered.State.InitialSnapshot.IntendedUntracked; strings.Join(got, ",") != "notes-a.txt,notes-b.txt" {
		t.Fatalf("successor intended-untracked = %v, want the authorized selection", got)
	}
}

// TestRecoveryArgumentsReplayUntrackedSelection pins the rendering seam: a
// workspace current-changes recovery with a declared selection replays the
// exact untracked-scope flags, and the inheritance-only case (no declaration)
// still renders no selector arguments at all.
func TestRecoveryArgumentsReplayUntrackedSelection(t *testing.T) {
	t.Parallel()

	selector := reviewTransitionSelector{Recovery: &reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace,
		IntendedUntracked: []string{"notes-a.txt", "notes-b.txt"},
	}}
	scope := reviewIntendedUntrackedScope{
		Intended: []string{"notes-a.txt", "notes-b.txt"}, Digest: "sha256:0123", Declared: true,
	}
	arguments, representable := selector.recoveryArguments(scope)
	if !representable {
		t.Fatalf("declared workspace selection is unrepresentable")
	}
	want := []ReviewTransitionArgument{
		{Name: "untracked-scope", Value: "select"},
		{Name: "expected-untracked-inventory", Value: "sha256:0123"},
		{Name: "intended-untracked", Value: "notes-a.txt"},
		{Name: "intended-untracked", Value: "notes-b.txt"},
	}
	if !reflect.DeepEqual(arguments, want) {
		t.Fatalf("declared selection arguments = %#v, want %#v", arguments, want)
	}

	exclude := reviewTransitionSelector{Recovery: &reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace,
	}}
	arguments, representable = exclude.recoveryArguments(reviewIntendedUntrackedScope{Digest: "sha256:0123", Declared: true})
	if !representable || !reflect.DeepEqual(arguments, []ReviewTransitionArgument{
		{Name: "untracked-scope", Value: "exclude"},
		{Name: "expected-untracked-inventory", Value: "sha256:0123"},
	}) {
		t.Fatalf("declared exclude arguments = %#v representable=%v", arguments, representable)
	}

	inherited := reviewTransitionSelector{Recovery: &reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace,
	}}
	arguments, representable = inherited.recoveryArguments(reviewIntendedUntrackedScope{})
	if !representable || len(arguments) != 0 {
		t.Fatalf("inheritance-only recovery arguments = %#v representable=%v, want none", arguments, representable)
	}

	// A declared selection without its validated inventory digest cannot be
	// replayed; the transition must fail closed rather than render a partial
	// selector.
	if _, representable = selector.recoveryArguments(reviewIntendedUntrackedScope{
		Intended: []string{"notes-a.txt"}, Declared: true,
	}); representable {
		t.Fatal("declared selection without an inventory digest was rendered")
	}
}

// TestReviewRecoverAcceptsDeclaredUntrackedSelection drives `review recover`
// directly with the mirrored START/STATUS untracked selection flags: the
// successor target must derive from the declared selection, not from
// predecessor inheritance.
func TestReviewRecoverAcceptsDeclaredUntrackedSelection(t *testing.T) {
	reviewEnabledHome(t)
	repo, predecessor := escalatedSelectionReplayFixture(t, "recover-1972-flags")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\ncorrected\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	_, digest, err := builder.IntendedUntrackedInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	complete, err := builder.Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace,
		IntendedUntracked: []string{"notes-a.txt", "notes-b.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	authorization := reviewRecoveryAuthorization(predecessor.State.LineageID, predecessor.Revision,
		complete.Identity, "maintainer", "recover the complete authorized candidate")

	var output bytes.Buffer
	if err := RunReviewRecover([]string{
		"--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision,
		"--successor-lineage", "recover-1972-flags-successor", "--disposition", "escalated",
		"--reason", "recover the complete authorized candidate", "--actor", "maintainer",
		"--maintainer-authorization", authorization,
		"--untracked-scope=select", "--expected-untracked-inventory=" + digest,
		"--intended-untracked", "notes-a.txt", "--intended-untracked", "notes-b.txt",
	}, &output); err != nil {
		t.Fatalf("recovery with the declared selection was refused: %v\n%s", err, output.String())
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "recover-1972-flags-successor")
	if err != nil {
		t.Fatal(err)
	}
	successor, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if successor.State.InitialSnapshot.Identity != complete.Identity ||
		strings.Join(successor.State.InitialSnapshot.IntendedUntracked, ",") != "notes-a.txt,notes-b.txt" {
		t.Fatalf("successor snapshot = %#v, want the declared selection target %s", successor.State.InitialSnapshot, complete.Identity)
	}
}

// TestReviewRecoverExcludeSelectionOverridesInheritance: an explicit exclude
// declaration is a real recovery-time decision, so it must override the
// predecessor's frozen declaration exactly the way a fresh START would.
func TestReviewRecoverExcludeSelectionOverridesInheritance(t *testing.T) {
	reviewEnabledHome(t)
	repo, predecessor := escalatedSelectionReplayFixture(t, "recover-1972-exclude")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\ncorrected\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	_, digest, err := builder.IntendedUntrackedInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	trackedOnly, err := builder.Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace,
		IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	authorization := reviewRecoveryAuthorization(predecessor.State.LineageID, predecessor.Revision,
		trackedOnly.Identity, "maintainer", "recover the tracked-only candidate")

	if err := RunReviewRecover([]string{
		"--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision,
		"--successor-lineage", "recover-1972-exclude-successor", "--disposition", "escalated",
		"--reason", "recover the tracked-only candidate", "--actor", "maintainer",
		"--maintainer-authorization", authorization,
		"--untracked-scope=exclude", "--expected-untracked-inventory=" + digest,
	}, io.Discard); err != nil {
		t.Fatalf("recovery with an explicit exclude declaration was refused: %v", err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "recover-1972-exclude-successor")
	if err != nil {
		t.Fatal(err)
	}
	successor, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if successor.State.InitialSnapshot.Identity != trackedOnly.Identity ||
		len(successor.State.InitialSnapshot.IntendedUntracked) != 0 {
		t.Fatalf("successor snapshot = %#v, want the tracked-only target %s", successor.State.InitialSnapshot, trackedOnly.Identity)
	}
}

// TestReviewRecoverDivergentSelectionStillRefused: the exact maintainer
// authorization keeps binding the selection-derived target, so an invocation
// whose declared selection does not match the authorized identity refuses
// exactly as before.
func TestReviewRecoverDivergentSelectionStillRefused(t *testing.T) {
	reviewEnabledHome(t)
	repo, predecessor := escalatedSelectionReplayFixture(t, "recover-1972-divergent")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\ncorrected\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	_, digest, err := builder.IntendedUntrackedInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	complete, err := builder.Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace,
		IntendedUntracked: []string{"notes-a.txt", "notes-b.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	authorization := reviewRecoveryAuthorization(predecessor.State.LineageID, predecessor.Revision,
		complete.Identity, "maintainer", "recover the complete authorized candidate")

	if err := RunReviewRecover([]string{
		"--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision,
		"--successor-lineage", "recover-1972-divergent-successor", "--disposition", "escalated",
		"--reason", "recover the complete authorized candidate", "--actor", "maintainer",
		"--maintainer-authorization", authorization,
		"--untracked-scope=select", "--expected-untracked-inventory=" + digest,
		"--intended-untracked", "notes-a.txt",
	}, io.Discard); err == nil {
		t.Fatal("recovery whose declared selection diverges from the authorized target was admitted")
	}
	if _, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "recover-1972-divergent-successor"); err == nil {
		if _, loadErr := os.Stat(filepath.Join(repo, ".git")); loadErr != nil {
			t.Fatal(loadErr)
		}
	}
}
