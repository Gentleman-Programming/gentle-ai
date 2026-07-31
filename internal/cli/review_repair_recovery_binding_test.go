package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// driftedRecoveryBindingCLIFixture persists an escalated docs-only predecessor
// with its receipt and a changed-target recovery successor whose recorded
// gentle-ai.review-recovery-authorization/v1 binding names a target identity
// the successor never persisted, then restores the clean workspace.
func driftedRecoveryBindingCLIFixture(t *testing.T, repo, lineage, authorizedTarget string) (predecessorRevision, successorRevision string) {
	t.Helper()
	incident := filepath.Join(repo, "docs", lineage+".md")
	if err := os.MkdirAll(filepath.Dir(incident), 0o755); err != nil {
		t.Fatal(err)
	}
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	intended := []string{filepath.ToSlash(filepath.Join("docs", lineage+".md"))}
	build := func(content string) (reviewtransaction.Snapshot, int) {
		if err := os.WriteFile(incident, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		snapshot, err := builder.Build(context.Background(), reviewtransaction.Target{
			Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: intended,
		})
		if err != nil {
			t.Fatal(err)
		}
		risk, lines, err := builder.ClassifySnapshotRisk(context.Background(), snapshot)
		if err != nil {
			t.Fatal(err)
		}
		if risk != reviewtransaction.RiskLow {
			t.Fatalf("drifted recovery binding fixture risk = %q", risk)
		}
		return snapshot, lines
	}
	policy := sha256.Sum256([]byte("drifted-recovery-binding-policy"))
	policyHash := "sha256:" + hex.EncodeToString(policy[:])
	predecessorSnapshot, predecessorLines := build("incident report\n")
	predecessorState, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: lineage, Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 1,
		Snapshot: predecessorSnapshot, PolicyHash: policyHash, RiskLevel: reviewtransaction.RiskLow,
		SelectedLenses: []string{}, OriginalChangedLines: &predecessorLines,
	})
	if err != nil {
		t.Fatal(err)
	}
	predecessorState.State = reviewtransaction.StateEscalated
	predecessorRevision = writeReconcileCLIRecord(t, repo, predecessorState)
	receipt, err := predecessorState.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", lineage, "review-receipt.json")
	if err := reviewtransaction.WriteCompactReceiptAtomic(receiptPath, receipt); err != nil {
		t.Fatal(err)
	}
	successorSnapshot, successorLines := build("incident report\nrefreshed verify-report evidence\n")
	successorState, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: lineage + "-g2", Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 2,
		Snapshot: successorSnapshot, PolicyHash: policyHash, RiskLevel: reviewtransaction.RiskLow,
		SelectedLenses: []string{}, OriginalChangedLines: &successorLines,
	})
	if err != nil {
		t.Fatal(err)
	}
	successorState.Recovery = &reviewtransaction.CompactRecoveryProvenance{
		PredecessorLineageID: lineage, PredecessorRevision: predecessorRevision,
		Disposition: reviewtransaction.RecoveryEscalated, Actor: "maintainer@example.com",
		Reason:      "refresh current verify-report evidence after approved review",
		RecoveredAt: time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC),
		MaintainerAuthorization: "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + lineage +
			"\npredecessor_revision=" + predecessorRevision + "\ntarget_identity=" + authorizedTarget +
			"\nactor=maintainer@example.com\nreason=refresh current verify-report evidence after approved review",
	}
	successorRevision = writeReconcileCLIRecord(t, repo, successorState)
	if err := os.Remove(incident); err != nil {
		t.Fatal(err)
	}
	return predecessorRevision, successorRevision
}

// recoveryBindingRepairCLIAuthorization renders the fourteen-line binding by
// hand from the published candidate, so the test pins the exact document the
// executor re-derives rather than reusing the production renderer.
func recoveryBindingRepairCLIAuthorization(candidate reviewtransaction.CompactRecoveryBindingRepairCandidate, actor, reason string) string {
	return "gentle-ai.review-repair-authorization/v1" +
		"\nrepository=" + candidate.RepositoryBinding +
		"\nclass=" + string(candidate.Class) +
		"\ncause=" + string(candidate.Cause) +
		"\ndisposition=" + string(candidate.Disposition) +
		"\npredecessor_lineage=" + candidate.PredecessorLineageID +
		"\npredecessor_revision=" + candidate.ExpectedPredecessorRevision +
		"\nsuccessor_lineage=" + candidate.SuccessorLineageID +
		"\nsuccessor_revision=" + candidate.ExpectedSuccessorRevision +
		"\npersisted_target_identity=" + candidate.PersistedTargetIdentity +
		"\nauthorized_target_identity=" + candidate.AuthorizedTargetIdentity +
		"\nrecorded_authorization_sha256=" + candidate.RecordedAuthorizationSHA256 +
		"\nactor=" + actor + "\nreason=" + reason
}

func recoveryBindingRepairCLIArgs(repo string, candidate reviewtransaction.CompactRecoveryBindingRepairCandidate, actor, reason string) []string {
	return []string{
		"repair-recovery-binding", "--cwd", repo,
		"--predecessor-lineage", candidate.PredecessorLineageID,
		"--expected-predecessor-revision", candidate.ExpectedPredecessorRevision,
		"--successor-lineage", candidate.SuccessorLineageID,
		"--expected-successor-revision", candidate.ExpectedSuccessorRevision,
		"--actor", actor, "--reason", reason,
		"--maintainer-authorization", recoveryBindingRepairCLIAuthorization(candidate, actor, reason),
	}
}

func recoveryBindingRepairPreflight(t *testing.T, args []string) (ReviewRepairRecoveryBindingResult, string) {
	t.Helper()
	var output bytes.Buffer
	if err := RunReview(args, &output); err != nil {
		t.Fatalf("review repair-recovery-binding --preflight: %v\n%s", err, output.String())
	}
	var result ReviewRepairRecoveryBindingResult
	decodeStrictReviewJSON(t, output.Bytes(), &result)
	if err := result.Validate(); err != nil {
		t.Fatalf("review repair-recovery-binding preflight validate: %v", err)
	}
	return result, output.String()
}

// TestReviewRepairRecoveryBindingPreflightIsReadOnlyAndPublishesCompleteInputs
// replaces the unsupported dead end the issue reports: preflight is
// deterministic, read-only, path-free, and publishes every authorization input
// except the maintainer's own actor and reason.
func TestReviewRepairRecoveryBindingPreflightIsReadOnlyAndPublishesCompleteInputs(t *testing.T) {
	repo := initReviewCLIRepo(t)
	predecessorRevision, successorRevision := driftedRecoveryBindingCLIFixture(t, repo, "drift-incident", "sha256:"+strings.Repeat("9", 64))
	statePath := filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", "drift-incident-g2", "review-state.json")
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}

	args := []string{"repair-recovery-binding", "--preflight", "--cwd", repo}
	first, rendered := recoveryBindingRepairPreflight(t, args)
	var second bytes.Buffer
	if err := RunReview(args, &second); err != nil {
		t.Fatal(err)
	}
	if rendered != second.String() {
		t.Fatalf("repair-recovery-binding preflight changed between reads:\n%s\n%s", rendered, second.String())
	}
	if first.Mode != ReviewRepairModePreflight || first.Counts == nil || first.Counts.AdmissibleCandidates != 1 ||
		first.Counts.BlockedCandidates != 0 || first.Counts.RecoveryEdges != 1 || first.Counts.InvalidEdges != 1 ||
		len(first.Candidates) != 1 {
		t.Fatalf("repair-recovery-binding preflight = %#v", first)
	}
	candidate := first.Candidates[0]
	if candidate.PredecessorLineageID != "drift-incident" || candidate.ExpectedPredecessorRevision != predecessorRevision ||
		candidate.SuccessorLineageID != "drift-incident-g2" || candidate.ExpectedSuccessorRevision != successorRevision ||
		candidate.AuthorizedTargetIdentity != "sha256:"+strings.Repeat("9", 64) || candidate.Blocked != "" {
		t.Fatalf("repair-recovery-binding candidate = %#v", candidate)
	}
	if !reflect.DeepEqual(first.RequiredInputs, []string{"actor", "reason", "maintainer_authorization"}) ||
		len(first.AuthorizationLines) != 14 || first.AuthorizationLines[0] != reviewtransaction.AuthorityRepairAuthorizationSchema ||
		first.AuthorizationLines[13] != "reason" {
		t.Fatalf("repair-recovery-binding required inputs = %#v / %#v", first.RequiredInputs, first.AuthorizationLines)
	}
	for _, leak := range []string{repo, "\nactor=", "\nreason=", "gentle-ai.review-repair-authorization/v1\nrepository="} {
		if strings.Contains(rendered, leak) {
			t.Fatalf("repair-recovery-binding preflight leaked %q:\n%s", leak, rendered)
		}
	}
	after, err := os.ReadFile(statePath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("repair-recovery-binding preflight changed authority state: %v", err)
	}
	if err := RunReview(append(args, "--actor", "maintainer@example.com"), &bytes.Buffer{}); err == nil ||
		!strings.Contains(err.Error(), "does not accept execution inputs") {
		t.Fatalf("repair-recovery-binding preflight accepted execution inputs: %v", err)
	}
}

// TestReviewRepairRecoveryBindingPreflightListsEachEdgeAndNarrowsBySelector
// proves the repository never has to contain a single unsupported edge: every
// admissible edge is its own candidate, and a maintainer can select exactly one
// by both lineage IDs and both expected revisions.
func TestReviewRepairRecoveryBindingPreflightListsEachEdgeAndNarrowsBySelector(t *testing.T) {
	repo := initReviewCLIRepo(t)
	driftedRecoveryBindingCLIFixture(t, repo, "drift-alpha", "sha256:"+strings.Repeat("9", 64))
	_, bravoSuccessorRevision := driftedRecoveryBindingCLIFixture(t, repo, "drift-bravo", "sha256:"+strings.Repeat("8", 64))

	listed, _ := recoveryBindingRepairPreflight(t, []string{"repair-recovery-binding", "--preflight", "--cwd", repo})
	if len(listed.Candidates) != 2 || listed.Counts.AdmissibleCandidates != 2 || listed.Counts.InvalidEdges != 2 {
		t.Fatalf("two-edge preflight = %#v", listed)
	}
	selected, _ := recoveryBindingRepairPreflight(t, []string{
		"repair-recovery-binding", "--preflight", "--cwd", repo,
		"--predecessor-lineage", "drift-bravo", "--successor-lineage", "drift-bravo-g2",
		"--expected-successor-revision", bravoSuccessorRevision,
	})
	if len(selected.Candidates) != 1 || selected.Candidates[0].SuccessorLineageID != "drift-bravo-g2" ||
		selected.Counts.AdmissibleCandidates != 1 {
		t.Fatalf("selected preflight = %#v", selected)
	}
	if err := RunReview([]string{
		"repair-recovery-binding", "--preflight", "--cwd", repo,
		"--successor-lineage", "drift-bravo-g2", "--expected-successor-revision", "sha256:" + strings.Repeat("f", 64),
	}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "must select exactly one") {
		t.Fatalf("mismatched selector = %v", err)
	}
}

// TestReviewRepairRecoveryBindingExecutionRestoresLifecycle is the end-to-end
// proof: status, the post-apply gate and the quarantined residue all move
// together, an unrelated healthy candidate re-enters the normal lifecycle, and
// the exact same invocation converges instead of moving anything twice.
func TestReviewRepairRecoveryBindingExecutionRestoresLifecycle(t *testing.T) {
	repo := initReviewCLIRepo(t)
	approveDiscoveryMarkdown(t, repo, "review-recovery-binding-valid", "docs/valid.md", "valid\n")
	driftedRecoveryBindingCLIFixture(t, repo, "drift-incident", "sha256:"+strings.Repeat("9", 64))

	var statusOutput bytes.Buffer
	if err := RunReview([]string{"status", "--cwd", repo}, &statusOutput); err != nil {
		t.Fatal(err)
	}
	var before reviewtransaction.AuthorityStatusReport
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &before)
	if before.Complete || before.Authoritative {
		t.Fatalf("drifted status = complete %v authoritative %v", before.Complete, before.Authoritative)
	}
	var blockedOutput bytes.Buffer
	if err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &blockedOutput); err == nil {
		t.Fatal("a drifted recovery binding did not deny lineage-less gate validation")
	}
	if failure := decodeReviewIntegrationFailure(t, blockedOutput.Bytes()); failure.Code != "authority_corrupted" {
		t.Fatalf("drifted gate failure = %#v", failure)
	}

	preflight, _ := recoveryBindingRepairPreflight(t, []string{"repair-recovery-binding", "--preflight", "--cwd", repo})
	args := recoveryBindingRepairCLIArgs(repo, preflight.Candidates[0], "maintainer@example.com", "quarantine the drifted recovery binding")
	var repairOutput bytes.Buffer
	if err := RunReview(args, &repairOutput); err != nil {
		t.Fatalf("review repair-recovery-binding: %v\n%s", err, repairOutput.String())
	}
	var executed ReviewRepairRecoveryBindingResult
	decodeStrictReviewJSON(t, repairOutput.Bytes(), &executed)
	if executed.Mode != ReviewRepairModeExecute || executed.Record == nil ||
		executed.Record.Status != reviewtransaction.CompactReclaimCommitted ||
		executed.Record.LineageID != "drift-incident-g2" || executed.Record.RecoveryBindingRepair == nil {
		t.Fatalf("review repair-recovery-binding result = %#v", executed)
	}
	if strings.Contains(repairOutput.String(), "\nactor=") || strings.Contains(repairOutput.String(), "gentle-ai.review-repair-authorization/v1\nrepository=") {
		t.Fatalf("review repair-recovery-binding leaked the authorization:\n%s", repairOutput.String())
	}
	if _, err := os.Stat(filepath.Join(executed.Record.QuarantinePath, "residue", "review-state.json")); err != nil {
		t.Fatalf("quarantined successor state missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", "drift-incident-g2")); !os.IsNotExist(err) {
		t.Fatalf("repaired successor entry still present: %v", err)
	}

	statusOutput.Reset()
	if err := RunReview([]string{"status", "--cwd", repo}, &statusOutput); err != nil {
		t.Fatal(err)
	}
	var after reviewtransaction.AuthorityStatusReport
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &after)
	if !after.Complete || !after.Authoritative {
		t.Fatalf("post-repair status = complete %v authoritative %v", after.Complete, after.Authoritative)
	}
	var validateOutput bytes.Buffer
	if err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &validateOutput); err != nil {
		t.Fatalf("post-repair lineage-less validation: %v\n%s", err, validateOutput.String())
	}
	var validated ReviewValidateResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, validateOutput.Bytes()).Result, &validated)
	if !validated.Allowed || validated.Context.LineageID != "review-recovery-binding-valid" {
		t.Fatalf("post-repair validation = %#v", validated)
	}

	var replayOutput bytes.Buffer
	if err := RunReview(args, &replayOutput); err != nil {
		t.Fatalf("exact repair replay: %v\n%s", err, replayOutput.String())
	}
	var replayed ReviewRepairRecoveryBindingResult
	decodeStrictReviewJSON(t, replayOutput.Bytes(), &replayed)
	if replayed.Record == nil || replayed.Record.QuarantinePath != executed.Record.QuarantinePath ||
		replayed.Record.Status != reviewtransaction.CompactReclaimCommitted {
		t.Fatalf("exact repair replay = %#v", replayed)
	}
	stale := append([]string(nil), args...)
	stale[10] = "sha256:" + strings.Repeat("f", 64)
	if err := RunReview(stale, &bytes.Buffer{}); err == nil {
		t.Fatal("a stale expected revision replayed the committed repair")
	}
}

// TestReviewRepairRecoveryBindingFailureNeverPublishesAuthorizationOrPaths
// pins the failure envelope of the execute path: a refused repair publishes no
// record, echoes neither the maintainer authorization nor the actor and reason
// it was signed with, never prints the absolute repository path, and leaves the
// successor exactly as persisted.
func TestReviewRepairRecoveryBindingFailureNeverPublishesAuthorizationOrPaths(t *testing.T) {
	repo := initReviewCLIRepo(t)
	driftedRecoveryBindingCLIFixture(t, repo, "drift-incident", "sha256:"+strings.Repeat("9", 64))
	preflight, _ := recoveryBindingRepairPreflight(t, []string{"repair-recovery-binding", "--preflight", "--cwd", repo})
	actor, reason := "maintainer@example.com", "quarantine the drifted recovery binding"
	args := recoveryBindingRepairCLIArgs(repo, preflight.Candidates[0], actor, reason)
	// A well-formed, fully authorized request whose expected successor revision
	// is stale: refused under the compare-and-swap, after the authorization was
	// accepted as an input and before anything durable moved.
	stale := append([]string(nil), args...)
	stale[10] = "sha256:" + strings.Repeat("f", 64)

	var output bytes.Buffer
	err := RunReview(stale, &output)
	if err == nil {
		t.Fatalf("a stale expected revision executed the repair:\n%s", output.String())
	}
	if output.Len() != 0 {
		t.Fatalf("a refused repair published a result:\n%s", output.String())
	}
	published := output.String() + "\n" + err.Error()
	for _, leak := range []string{
		recoveryBindingRepairCLIAuthorization(preflight.Candidates[0], actor, reason),
		"gentle-ai.review-repair-authorization/v1\nrepository=", "\nactor=", "\nreason=", actor, reason, repo,
	} {
		if strings.Contains(published, leak) {
			t.Fatalf("a refused repair leaked %q:\n%s", leak, published)
		}
	}
	if _, statErr := os.Stat(filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", "drift-incident-g2")); statErr != nil {
		t.Fatalf("a refused repair moved the successor: %v", statErr)
	}
}

// TestReviewRepairRecoveryBindingRespectsReceiptDrivenKillSwitch keeps
// diagnosis available while reviews are off and refuses only the mutation.
func TestReviewRepairRecoveryBindingRespectsReceiptDrivenKillSwitch(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	driftedRecoveryBindingCLIFixture(t, repo, "drift-incident", "sha256:"+strings.Repeat("9", 64))
	preflight, _ := recoveryBindingRepairPreflight(t, []string{"repair-recovery-binding", "--preflight", "--cwd", repo})
	if err := RunReviewMode([]string{"disable", "--cwd", repo, "--json"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	_, _ = recoveryBindingRepairPreflight(t, []string{"repair-recovery-binding", "--preflight", "--cwd", repo})
	args := recoveryBindingRepairCLIArgs(repo, preflight.Candidates[0], "maintainer@example.com", "quarantine the drifted recovery binding")
	if err := RunReview(args, &bytes.Buffer{}); err == nil {
		t.Fatal("review repair-recovery-binding mutated authority while reviews are disabled")
	}
	if _, err := os.Stat(filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", "drift-incident-g2")); err != nil {
		t.Fatalf("a refused repair moved the successor: %v", err)
	}
}

// TestReviewRepairRecoveryBindingHelpNamesTheClassAndBindingKeys keeps the verb
// discoverable from the facade usage listing and from review repair's help.
func TestReviewRepairRecoveryBindingHelpNamesTheClassAndBindingKeys(t *testing.T) {
	var usage bytes.Buffer
	if err := RunReview([]string{}, &usage); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(usage.String(), "repair-recovery-binding") {
		t.Fatalf("review usage does not list the verb:\n%s", usage.String())
	}
	var help bytes.Buffer
	if err := RunReview([]string{"repair-recovery-binding", "--help"}, &help); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--preflight", "leaf-first", "--expected-predecessor-revision", "fourteen-line"} {
		if !strings.Contains(help.String(), want) {
			t.Fatalf("review repair-recovery-binding help does not name %q:\n%s", want, help.String())
		}
	}
	var repairHelp bytes.Buffer
	if err := RunReview([]string{"repair", "--help"}, &repairHelp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(repairHelp.String(), "repair-recovery-binding") ||
		!strings.Contains(repairHelp.String(), string(reviewtransaction.AuthorityRepairClassCompactV2RecoveryTargetDrift)) {
		t.Fatalf("review repair help does not route the drift class:\n%s", repairHelp.String())
	}
}
