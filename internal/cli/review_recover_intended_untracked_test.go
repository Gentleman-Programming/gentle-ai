package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// escalatedIntendedUntrackedRecoveryFixture is the #3159 predecessor: an
// escalated current-changes authority whose frozen snapshot carries two
// explicitly declared intended-untracked paths alongside a tracked change.
func escalatedIntendedUntrackedRecoveryFixture(t *testing.T, lineage string) (string, reviewtransaction.CompactRecord) {
	t.Helper()
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"notes-a.txt", "notes-b.txt"} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte("untracked but explicitly selected\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, digest, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).IntendedUntrackedInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	startedBytes, err := runLegacyFacadeStartForTestBytes(t, []string{"--cwd", repo, "--lineage", lineage,
		"--untracked-scope=select", "--expected-untracked-inventory=" + digest,
		"--intended-untracked", "notes-a.txt", "--intended-untracked", "notes-b.txt"})
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
		len(predecessor.State.InitialSnapshot.IntendedUntracked) != 2 {
		t.Fatalf("fixture did not reach an escalated authority carrying the declared selection: %#v", predecessor.State.InitialSnapshot)
	}
	return repo, predecessor
}

// escalateReviewForRecovery reaches the surviving recovery predecessor through
// reviewer capture, a status-bound correction plan, and a captured validator.
func escalateReviewForRecovery(t *testing.T, repo string, started ReviewFacadeStartResult) {
	t.Helper()
	for order := range started.SelectedLenses {
		findings := []facadeFinding{}
		if order == 0 {
			findings = []facadeFinding{{
				Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "candidate regression",
				ProofRefs:     []string{"differential test fails only on candidate"},
				EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
			}}
		}
		captureCLIReviewerResultWithFindings(t, repo, started, order, findings, &bytes.Buffer{})
	}
	captureCorrectionPlanFromCurrentStatus(t, repo, started.LineageID, 1)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfixed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	request, err := reviewtransaction.BuildTargetedValidationRequest(context.Background(), repo, record.State, record.State.CapturePhaseRevision)
	if err != nil {
		t.Fatal(err)
	}
	var validation facadeValidationResult
	if err := json.Unmarshal(providerTargetedValidationPayload(t, request), &validation); err != nil {
		t.Fatal(err)
	}
	validation.OriginalCriteria.Passed = false
	payload, err := json.Marshal(validation)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	previous := reviewProviderRoleHostAdapter
	reviewProviderRoleHostAdapter = func() reviewerprovider.Adapter {
		return providerTestAdapterFunc(func(context.Context, reviewerprovider.Invocation) ([]byte, error) {
			return payload, nil
		})
	}
	t.Cleanup(func() { reviewProviderRoleHostAdapter = previous })
	if err := RunReviewCaptureValidation([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", request.CorrectionTargetIdentity,
		"--expected-revision", record.State.CapturePhaseRevision, "--request-hash", request.RequestHash,
		"--agent", string(model.AgentPi), "--execute=true",
	}, io.Discard); err != nil {
		t.Fatalf("capture rejected targeted validator: %v", err)
	}
}

// TestReviewRecoverInheritsDeclaredIntendedUntracked is #3159.
//
// `review recover` hardcoded the successor's intended-untracked declaration
// to empty, citing #2394's rule that a recovery successor must not re-sweep
// the worktree. The rule is right and stays; the hardcoding overshot it.
// Inheriting the predecessor's frozen, explicitly authorized declaration is
// not a sweep — those exact paths were selected by a human and frozen into
// the authority being recovered. Dropping them silently rebound the successor
// to a tracked-only target, so the lifecycle routed reviewer collection
// toward a partial candidate while the user believed the complete one was
// under review. The rc.7 reporter's client refused to treat that partial
// review as approval, which is the only reason this was caught.
func TestReviewRecoverInheritsDeclaredIntendedUntracked(t *testing.T) {
	reviewEnabledHome(t)
	repo, predecessor := escalatedIntendedUntrackedRecoveryFixture(t, "recover-3159")

	// Apply the fix that verification asked for: the successor candidate must
	// differ from the escalated predecessor.
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\ncorrected\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The maintainer authorizes the COMPLETE candidate: the successor identity
	// is derived with the predecessor's declared selection, exactly as the
	// reporter expected recovery to bind.
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	complete, err := builder.Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace,
		IntendedUntracked: append([]string{}, predecessor.State.InitialSnapshot.IntendedUntracked...),
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
		"--successor-lineage", "recover-3159-successor", "--disposition", "escalated",
		"--reason", "recover the complete authorized candidate", "--actor", "maintainer",
		"--maintainer-authorization", authorization,
	}, &output); err != nil {
		t.Fatalf("recovery authorized against the complete candidate identity was refused: %v\n%s", err, output.String())
	}
	var recovered ReviewRecoverResult
	if err := json.Unmarshal(output.Bytes(), &recovered); err != nil {
		t.Fatalf("decode recover result: %v\n%s", err, output.String())
	}

	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "recover-3159-successor")
	if err != nil {
		t.Fatal(err)
	}
	successor, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := successor.State.InitialSnapshot.IntendedUntracked, predecessor.State.InitialSnapshot.IntendedUntracked; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("successor dropped the declared intended-untracked selection: got %v, want %v; the successor target represents a partial candidate", got, want)
	}
	if successor.State.InitialSnapshot.Identity != complete.Identity {
		t.Fatalf("successor identity %s does not bind the complete authorized candidate %s", successor.State.InitialSnapshot.Identity, complete.Identity)
	}
}

// TestReviewRecoverStillNeverSweepsUndeclaredUntracked is #2394's guard,
// restated against the fix: inheritance carries ONLY the frozen declaration.
// An untracked file that appeared after the predecessor froze must stay out
// of the successor, no matter that it would be eligible for a fresh START.
func TestReviewRecoverStillNeverSweepsUndeclaredUntracked(t *testing.T) {
	reviewEnabledHome(t)
	repo, predecessor := escalatedIntendedUntrackedRecoveryFixture(t, "recover-3159-nosweep")

	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\ncorrected\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Appears AFTER the predecessor froze: never declared, never authorized.
	if err := os.WriteFile(filepath.Join(repo, "appeared-later.txt"), []byte("credential-shaped surprise\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	complete, err := builder.Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace,
		IntendedUntracked: append([]string{}, predecessor.State.InitialSnapshot.IntendedUntracked...),
	})
	if err != nil {
		t.Fatal(err)
	}
	authorization := reviewRecoveryAuthorization(predecessor.State.LineageID, predecessor.Revision,
		complete.Identity, "maintainer", "recover without sweeping")

	if err := RunReviewRecover([]string{
		"--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision,
		"--successor-lineage", "recover-3159-nosweep-successor", "--disposition", "escalated",
		"--reason", "recover without sweeping", "--actor", "maintainer",
		"--maintainer-authorization", authorization,
	}, io.Discard); err != nil {
		t.Fatalf("recovery refused: %v", err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "recover-3159-nosweep-successor")
	if err != nil {
		t.Fatal(err)
	}
	successor, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range successor.State.InitialSnapshot.IntendedUntracked {
		if path == "appeared-later.txt" {
			t.Fatalf("recovery swept an undeclared untracked file into the successor: %v; this is exactly what #2394 forbids", successor.State.InitialSnapshot.IntendedUntracked)
		}
	}
	for _, path := range successor.State.InitialSnapshot.Paths {
		if path == "appeared-later.txt" {
			t.Fatalf("successor manifest contains the undeclared untracked file: %v", successor.State.InitialSnapshot.Paths)
		}
	}
}

// TestReviewRecoverInheritsDeclarationAfterDeclaredPathWasCommitted is #3759.
//
// Inheriting the predecessor's frozen declaration (#3159) replayed it
// verbatim, so once ordinary development committed one of those paths the
// successor build refused with "already tracked" and named one path per
// attempt. A committed path is no longer untracked: the index carries it and
// the current-changes target already covers it, so there is nothing left to
// declare. Inheritance keeps only the entries that are still untracked.
func TestReviewRecoverInheritsDeclarationAfterDeclaredPathWasCommitted(t *testing.T) {
	reviewEnabledHome(t)
	repo, predecessor := escalatedIntendedUntrackedRecoveryFixture(t, "recover-3759")

	// Unrelated development lands one declared path between escalation and
	// recovery; the other declared path stays untracked.
	runReviewCLIGit(t, repo, "add", "notes-a.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "land notes-a")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\ncorrected\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	complete, err := builder.Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace,
		IntendedUntracked: []string{"notes-b.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	authorization := reviewRecoveryAuthorization(predecessor.State.LineageID, predecessor.Revision,
		complete.Identity, "maintainer", "recover after landing a declared path")

	var output bytes.Buffer
	if err := RunReviewRecover([]string{
		"--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision,
		"--successor-lineage", "recover-3759-successor", "--disposition", "escalated",
		"--reason", "recover after landing a declared path", "--actor", "maintainer",
		"--maintainer-authorization", authorization,
	}, &output); err != nil {
		t.Fatalf("recovery inheriting a declaration with one committed path was refused: %v\n%s", err, output.String())
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "recover-3759-successor")
	if err != nil {
		t.Fatal(err)
	}
	successor, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(successor.State.InitialSnapshot.IntendedUntracked, ","); got != "notes-b.txt" {
		t.Fatalf("successor declaration = %q, want only the still-untracked notes-b.txt", got)
	}
	if successor.State.InitialSnapshot.Identity != complete.Identity {
		t.Fatalf("successor identity %s does not bind the authorized candidate %s", successor.State.InitialSnapshot.Identity, complete.Identity)
	}
}
