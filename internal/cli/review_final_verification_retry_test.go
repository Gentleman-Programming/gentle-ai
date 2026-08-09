package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

func TestReviewRetryFinalVerificationOperationAndStatusCompleteNormally(t *testing.T) {
	fixture := failedFinalVerificationCLIFixture(t)
	statusArgs := []string{"status", "--contract", ReviewIntegrationContractV1, "--action-eligibility", "--next-transition", "--cwd", fixture.repo, "--lineage", fixture.predecessor.State.LineageID}
	var statusOutput bytes.Buffer
	if err := RunReview(statusArgs, &statusOutput); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &status)
	if status.Action != reviewtransaction.TargetStatusActionRetryFinalVerification || status.ActionDisposition != reviewtransaction.RecoveryFinalVerificationRetry ||
		status.FinalVerificationRetry == nil || status.FinalVerificationRetry.ValidatingRevision != fixture.incident.ValidatingRevision ||
		status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
		status.NextTransition.ReasonCode != "final_verification_retry_authorization_required" ||
		status.Eligibility == nil || status.Eligibility.AllowedActions[0].Action != ReviewIntegrationOperationRetryFinalVerification {
		t.Fatalf("eligible retry status = %#v\n%s", status, statusOutput.String())
	}
	if strings.Contains(statusOutput.String(), fixture.repo) || strings.Contains(statusOutput.String(), fixture.incidentPath) {
		t.Fatalf("eligible status leaked a local path:\n%s", statusOutput.String())
	}

	request := reviewtransaction.FinalVerificationRetryRequest{
		PredecessorLineageID: fixture.predecessor.State.LineageID, ExpectedPredecessorRevision: fixture.predecessor.Revision,
		SuccessorLineageID: "retry-final-cli-successor", Incident: fixture.incident,
		Actor: "maintainer", Reason: "retry final verification after provider tooling failure",
	}
	authorization, err := reviewtransaction.FinalVerificationRetryAuthorization(request)
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"retry-final-verification", "--contract", ReviewIntegrationContractV1, "--cwd", fixture.repo,
		"--predecessor-lineage", request.PredecessorLineageID, "--expected-predecessor-revision", request.ExpectedPredecessorRevision,
		"--successor-lineage", request.SuccessorLineageID, "--incident", fixture.incidentPath,
		"--actor", request.Actor, "--reason", request.Reason, "--maintainer-authorization", authorization}
	var output bytes.Buffer
	if err := RunReview(args, &output); err != nil {
		t.Fatal(err)
	}
	var result ReviewFinalVerificationRetryResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, output.Bytes()).Result, &result)
	if result.Operation != ReviewIntegrationOperationRetryFinalVerification || result.LineageID != request.SuccessorLineageID ||
		result.State != reviewtransaction.StateValidating || result.PredecessorLineageID != request.PredecessorLineageID ||
		result.TargetIdentity != fixture.incident.TargetIdentity || result.IncidentDigest != reviewtransaction.FinalVerificationIncidentDigest(fixture.incident) {
		t.Fatalf("retry result = %#v\n%s", result, output.String())
	}
	if strings.Contains(output.String(), fixture.repo) || strings.Contains(output.String(), fixture.incidentPath) {
		t.Fatalf("retry result leaked a local path:\n%s", output.String())
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), fixture.repo, request.SuccessorLineageID)
	if err != nil {
		t.Fatal(err)
	}
	successor, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if successor.State.Recovery == nil || successor.State.Recovery.Disposition != reviewtransaction.RecoveryFinalVerificationRetry ||
		successor.State.EvidenceHash != "" || successor.State.Generation != fixture.predecessor.State.Generation+1 {
		t.Fatalf("retry successor = %#v", successor.State)
	}

	replayOutput := bytes.Buffer{}
	if err := RunReview(args, &replayOutput); err != nil {
		t.Fatal(err)
	}
	var replay ReviewFinalVerificationRetryResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, replayOutput.Bytes()).Result, &replay)
	if replay.StoreRevision != result.StoreRevision {
		t.Fatalf("exact retry replay revision = %q, want %q", replay.StoreRevision, result.StoreRevision)
	}

	passed := filepath.Join(t.TempDir(), "retry-passed.txt")
	if err := os.WriteFile(passed, []byte("retry verification passed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunReview([]string{"capture-evidence", "--cwd", fixture.repo, "--lineage", successor.State.LineageID,
		"--target", successor.State.CurrentSnapshot.Identity, "--expected-revision", successor.Revision, "--outcome", string(reviewtransaction.VerificationOutcomePassed), "--input", passed}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var finalized bytes.Buffer
	if err := RunReview([]string{"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", fixture.repo,
		"--lineage", successor.State.LineageID, "--captured-evidence"}, &finalized); err != nil {
		t.Fatal(err)
	}
	var terminal ReviewIntegrationFinalizeResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, finalized.Bytes()).Result, &terminal)
	if terminal.State != reviewtransaction.StateApproved {
		t.Fatalf("retry final state = %#v", terminal)
	}
}

func TestReviewRetryFinalVerificationCorrectedRestartUsesFrozenAuthorityTarget(t *testing.T) {
	fixture := failedCorrectedFinalVerificationCLIFixture(t)
	if fixture.predecessor.State.InitialSnapshot.Identity == fixture.predecessor.State.CurrentSnapshot.Identity {
		t.Fatal("corrected fixture did not advance CurrentSnapshot")
	}
	statusArgs := []string{"status", "--contract", ReviewIntegrationContractV1, "--action-eligibility", "--next-transition", "--cwd", fixture.repo, "--lineage", fixture.predecessor.State.LineageID}
	var predecessorOutput bytes.Buffer
	if err := RunReview(statusArgs, &predecessorOutput); err != nil {
		t.Fatalf("corrected predecessor STATUS: %v\n%s", err, predecessorOutput.String())
	}
	var predecessorStatus ReviewTargetStatusResult
	decodeStrictReviewJSON(t, predecessorOutput.Bytes(), &predecessorStatus)
	if predecessorStatus.TargetIdentity == predecessorStatus.AuthorityTargetIdentity ||
		predecessorStatus.TargetIdentity != predecessorStatus.Projection.CurrentSnapshotIdentity ||
		predecessorStatus.AuthorityTargetIdentity != fixture.predecessor.State.CurrentSnapshot.Identity ||
		predecessorStatus.FinalVerificationRetry == nil || predecessorStatus.FinalVerificationRetry.TargetIdentity != predecessorStatus.AuthorityTargetIdentity ||
		transitionArgumentValue(t, predecessorStatus.NextTransition, "target") != predecessorStatus.AuthorityTargetIdentity ||
		predecessorStatus.Eligibility == nil || predecessorStatus.Eligibility.AllowedActions[0].Binding == nil ||
		predecessorStatus.Eligibility.AllowedActions[0].Binding.TargetIdentity != predecessorStatus.AuthorityTargetIdentity {
		t.Fatalf("corrected predecessor STATUS bindings = %#v\n%s", predecessorStatus, predecessorOutput.String())
	}

	retryArgs := finalVerificationRetryCLIArgs(t, fixture, "retry-corrected-cli-successor", fixture.incidentPath)
	if err := RunReview(retryArgs, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var successorOutput bytes.Buffer
	if err := RunReview([]string{"status", "--contract", ReviewIntegrationContractV1, "--next-transition", "--cwd", fixture.repo, "--lineage", "retry-corrected-cli-successor"}, &successorOutput); err != nil {
		t.Fatalf("corrected retry successor STATUS: %v\n%s", err, successorOutput.String())
	}
	var successorStatus ReviewTargetStatusResult
	decodeStrictReviewJSON(t, successorOutput.Bytes(), &successorStatus)
	if successorStatus.Authority == nil || successorStatus.Authority.State != reviewtransaction.StateValidating ||
		successorStatus.TargetIdentity == successorStatus.AuthorityTargetIdentity ||
		successorStatus.AuthorityTargetIdentity != fixture.predecessor.State.CurrentSnapshot.Identity ||
		transitionArgumentValue(t, successorStatus.NextTransition, "target") != successorStatus.AuthorityTargetIdentity {
		t.Fatalf("corrected successor STATUS bindings = %#v\n%s", successorStatus, successorOutput.String())
	}

	passed := filepath.Join(t.TempDir(), "corrected-retry-passed.txt")
	if err := os.WriteFile(passed, []byte("corrected retry verification passed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunReview([]string{"capture-evidence", "--cwd", fixture.repo, "--lineage", successorStatus.Authority.LineageID,
		"--target", successorStatus.AuthorityTargetIdentity, "--expected-revision", successorStatus.Authority.Revision, "--outcome", string(reviewtransaction.VerificationOutcomePassed), "--input", passed}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var terminal bytes.Buffer
	if err := RunReview([]string{"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", fixture.repo,
		"--lineage", successorStatus.Authority.LineageID, "--captured-evidence"}, &terminal); err != nil {
		t.Fatal(err)
	}
	var finalized ReviewIntegrationFinalizeResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, terminal.Bytes()).Result, &finalized)
	if finalized.State != reviewtransaction.StateApproved {
		t.Fatalf("corrected retry final state = %#v", finalized)
	}
}

func TestStatusFinalVerificationRetryProjectsOpaqueConsentEnvelope(t *testing.T) {
	fixture := failedFinalVerificationCLIFixture(t)
	var output bytes.Buffer
	if err := RunReview([]string{"status", "--contract", ReviewIntegrationContractV2, "--agent", "opencode", "--next-transition", "--cwd", fixture.repo, "--lineage", fixture.predecessor.State.LineageID}, &output); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	decodeStrictReviewJSON(t, output.Bytes(), &payload)
	transition := payload["next_transition"].(map[string]any)
	collection := transition["collect"].(map[string]any)
	consent, ok := collection["consent"].(map[string]any)
	if !ok || consent["schema"] != "gentle-ai.review-final-verification-retry-consent/v1" {
		t.Fatalf("retry STATUS did not project a typed consent envelope: %#v", collection)
	}
	transitionPayload, err := json.Marshal(transition)
	if err != nil {
		t.Fatal(err)
	}
	validateAgainstPublishedNextTransitionSchemaV5(t, transitionPayload)
	choices := consent["choices"].([]any)
	if len(choices) != 2 || choices[0].(map[string]any)["answer"] != "granted" || choices[1].(map[string]any)["answer"] != "declined" {
		t.Fatalf("retry consent choices = %#v", choices)
	}
	for _, choice := range choices {
		invocation := choice.(map[string]any)["invocation"].(string)
		if !strings.Contains(invocation, "gentle-ai review retry-final-verification --repository-context rctx1_") || !strings.Contains(invocation, " --consent "+choice.(map[string]any)["answer"].(string)) {
			t.Fatalf("retry consent invocation = %q", invocation)
		}
	}
	offPath := consent["off_path"].(map[string]any)["command"].(string)
	if !strings.Contains(offPath, "--contract gentle-ai.review-integration/v2 --agent opencode --next-transition --repository-context rctx1_") || strings.Contains(offPath, fixture.repo) {
		t.Fatalf("retry consent off-path = %q", offPath)
	}
	parts := strings.Fields(offPath)
	oldWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWorkingDirectory) })
	var rechecked bytes.Buffer
	if err := RunReview(parts[2:], &rechecked); err != nil {
		t.Fatalf("off-path STATUS outside repository: %v\n%s", err, rechecked.String())
	}
	if !strings.Contains(rechecked.String(), ReviewFinalVerificationRetryConsentSchema) || strings.Contains(rechecked.String(), fixture.repo) {
		t.Fatalf("off-path STATUS projection = %s", rechecked.String())
	}
}

func TestOpaqueStatusRejectsEveryExternalSelector(t *testing.T) {
	fixture := failedFinalVerificationCLIFixture(t)
	context := finalVerificationRetryContext(t, fixture)
	base := []string{"status", "--contract", ReviewIntegrationContractV2, "--agent", "opencode", "--next-transition", "--repository-context", context}
	tests := []struct {
		name string
		args []string
	}{
		{name: "cwd", args: []string{"--cwd", fixture.repo}},
		{name: "lineage", args: []string{"--lineage", fixture.predecessor.State.LineageID}},
		{name: "projection", args: []string{"--projection", "staged"}},
		{name: "base ref", args: []string{"--base-ref", "HEAD"}},
		{name: "base tree", args: []string{"--base-tree", strings.Repeat("a", 40)}},
		{name: "committed only", args: []string{"--committed-only"}},
		{name: "workspace overlay", args: []string{"--workspace-overlay"}},
		{name: "untracked scope", args: []string{"--untracked-scope", "select"}},
		{name: "intended untracked", args: []string{"--intended-untracked", "tracked.txt"}},
		{name: "untracked inventory", args: []string{"--expected-untracked-inventory", "sha256:" + strings.Repeat("a", 64)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := cliReviewAuthoritySnapshot(t, fixture.repo)
			var output bytes.Buffer
			err := RunReview(append(append([]string{}, base...), tt.args...), &output)
			if err == nil {
				t.Fatalf("opaque STATUS selector error = %v\n%s", err, output.String())
			}
			var failure ReviewIntegrationFailure
			decodeStrictReviewJSON(t, output.Bytes(), &failure)
			if failure.Schema != ReviewIntegrationFailureSchemaV2 || failure.Contract != ReviewIntegrationContractV2 ||
				failure.MutationOutcome != ReviewMutationNotStarted || !strings.Contains(failure.Cause, "cannot combine repository selectors") {
				t.Fatalf("opaque STATUS selector failure = %#v", failure)
			}
			if after := cliReviewAuthoritySnapshot(t, fixture.repo); !reflect.DeepEqual(after, before) {
				t.Fatalf("opaque STATUS selector mutated authority: %#v != %#v", after, before)
			}
		})
	}
}

func TestFinalVerificationRetryConsentGrantIsExactAndOneShot(t *testing.T) {
	fixture := failedFinalVerificationCLIFixture(t)
	context := finalVerificationRetryContext(t, fixture)
	args := []string{"retry-final-verification", "--repository-context", context, "--consent", "granted"}
	var output bytes.Buffer
	if err := RunReview(args, &output); err != nil {
		t.Fatalf("grant opaque retry consent: %v\n%s", err, output.String())
	}
	var first ReviewFinalVerificationRetryResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, output.Bytes()).Result, &first)
	if first.PredecessorLineageID != fixture.predecessor.State.LineageID || first.PredecessorRevision != fixture.predecessor.Revision || first.State != reviewtransaction.StateValidating {
		t.Fatalf("grant result = %#v", first)
	}
	var replay bytes.Buffer
	if err := RunReview(args, &replay); err != nil {
		t.Fatalf("exact consent replay: %v\n%s", err, replay.String())
	}
	var second ReviewFinalVerificationRetryResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, replay.Bytes()).Result, &second)
	if second.LineageID != first.LineageID || second.StoreRevision != first.StoreRevision {
		t.Fatalf("one-shot replay = %#v, want %#v", second, first)
	}
	beforeDecline := cliReviewAuthoritySnapshot(t, fixture.repo)
	var declinedOutput bytes.Buffer
	err := RunReview([]string{"retry-final-verification", "--repository-context", context, "--consent", "declined"}, &declinedOutput)
	if err == nil {
		t.Fatalf("decline after grant error = %v", err)
	}
	var failure ReviewIntegrationFailure
	decodeStrictReviewJSON(t, declinedOutput.Bytes(), &failure)
	if failure.Schema != ReviewIntegrationFailureSchemaV2 || failure.Contract != ReviewIntegrationContractV2 ||
		failure.MutationOutcome != ReviewMutationNotStarted || failure.Operation != ReviewIntegrationOperationRetryFinalVerification {
		t.Fatalf("decline after grant did not produce a typed v2 refusal: %#v", failure)
	}
	if !strings.Contains(failure.Cause, "already consumed by grant") {
		t.Fatalf("decline after grant cause = %#v", failure)
	}
	if afterDecline := cliReviewAuthoritySnapshot(t, fixture.repo); !reflect.DeepEqual(afterDecline, beforeDecline) {
		t.Fatalf("decline after grant mutated authority: %#v != %#v", afterDecline, beforeDecline)
	}
}

func TestFinalVerificationRetryConsentDeclineIsExactAndNonMutating(t *testing.T) {
	fixture := failedFinalVerificationCLIFixture(t)
	context := finalVerificationRetryContext(t, fixture)
	before := cliReviewAuthoritySnapshot(t, fixture.repo)
	var output bytes.Buffer
	if err := RunReview([]string{"retry-final-verification", "--repository-context", context, "--consent", "declined"}, &output); err != nil {
		t.Fatalf("decline opaque retry consent: %v\n%s", err, output.String())
	}
	var result map[string]any
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, output.Bytes()).Result, &result)
	if result["disposition"] != "declined" {
		t.Fatalf("decline result = %#v", result)
	}
	if after := cliReviewAuthoritySnapshot(t, fixture.repo); !reflect.DeepEqual(after, before) {
		t.Fatalf("decline mutated authority: %#v != %#v", after, before)
	}
	var repeated bytes.Buffer
	if err := RunReview([]string{"retry-final-verification", "--repository-context", context, "--consent", "declined"}, &repeated); err != nil {
		t.Fatalf("repeat eligible decline: %v\n%s", err, repeated.String())
	}
	if repeated.String() != output.String() {
		t.Fatalf("repeat decline result changed:\nfirst=%s\nsecond=%s", output.String(), repeated.String())
	}
}

func TestFinalVerificationRetryConsentNeverGrantsInvalidSource(t *testing.T) {
	t.Run("stale repository context", func(t *testing.T) {
		fixture := failedFinalVerificationCLIFixture(t)
		before := cliReviewAuthoritySnapshot(t, fixture.repo)
		context := finalVerificationRetryContext(t, fixture)
		stale := context[:len(context)-1] + "0"
		if stale == context {
			stale = context[:len(context)-1] + "1"
		}
		if err := RunReview([]string{"retry-final-verification", "--repository-context", stale, "--consent", "granted"}, io.Discard); err == nil {
			t.Fatal("stale repository context granted retry")
		}
		if after := cliReviewAuthoritySnapshot(t, fixture.repo); !reflect.DeepEqual(after, before) {
			t.Fatalf("stale context mutated authority: %#v != %#v", after, before)
		}
	})
	t.Run("wrong candidate", func(t *testing.T) {
		fixture := failedFinalVerificationCLIFixture(t)
		if err := os.WriteFile(filepath.Join(fixture.repo, "tracked.txt"), []byte("different candidate\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		other := startFacadeReview(t, fixture.repo)
		store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), fixture.repo, other.LineageID)
		if err != nil {
			t.Fatal(err)
		}
		record, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		handle, err := reviewtransaction.PublishReviewRepositoryContext(context.Background(), fixture.repo, reviewtransaction.ReviewRepositoryContextBinding{
			LineageID: record.State.LineageID, Revision: record.Revision, TargetIdentity: record.State.InitialSnapshot.Identity,
		})
		if err != nil {
			t.Fatal(err)
		}
		before := cliReviewAuthoritySnapshot(t, fixture.repo)
		if err := RunReview([]string{"retry-final-verification", "--repository-context", handle, "--consent", "granted"}, io.Discard); err == nil {
			t.Fatal("wrong candidate context granted retry")
		}
		if after := cliReviewAuthoritySnapshot(t, fixture.repo); !reflect.DeepEqual(after, before) {
			t.Fatalf("wrong candidate mutated authority: %#v != %#v", after, before)
		}
	})
	t.Run("non-procedural failure", func(t *testing.T) {
		fixture := prepareFacadeRawFailedEvidence(t)
		evidence := filepath.Join(t.TempDir(), "verification-failed.txt")
		if err := os.WriteFile(evidence, []byte("verification failed\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := RunReviewFacadeFinalize([]string{"--cwd", fixture.repo, "--lineage", fixture.started.LineageID, "--evidence", evidence, "--failed"}, io.Discard); err != nil {
			t.Fatal(err)
		}
		var status bytes.Buffer
		if err := RunReview([]string{"status", "--contract", ReviewIntegrationContractV2, "--agent", "opencode", "--next-transition", "--cwd", fixture.repo, "--lineage", fixture.started.LineageID}, &status); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(status.String(), ReviewFinalVerificationRetryConsentSchema) {
			t.Fatalf("non-procedural failure exposed retry consent:\n%s", status.String())
		}
	})
}

func finalVerificationRetryContext(t *testing.T, fixture failedFinalVerificationCLI) string {
	t.Helper()
	handle, err := reviewtransaction.PublishReviewRepositoryContext(context.Background(), fixture.repo, reviewtransaction.ReviewRepositoryContextBinding{
		LineageID: fixture.predecessor.State.LineageID, Revision: fixture.predecessor.Revision,
		TargetIdentity: fixture.predecessor.State.CurrentSnapshot.Identity,
	})
	if err != nil {
		t.Fatal(err)
	}
	return handle
}

func TestReviewRetryFinalVerificationNegotiatedDenialIsNoMutation(t *testing.T) {
	fixture := failedFinalVerificationCLIFixture(t)
	request := reviewtransaction.FinalVerificationRetryRequest{
		PredecessorLineageID: fixture.predecessor.State.LineageID, ExpectedPredecessorRevision: fixture.predecessor.Revision,
		SuccessorLineageID: "retry-final-cli-denied-successor", Incident: fixture.incident,
		Actor: "maintainer", Reason: "retry after tooling failure",
	}
	authorization, err := reviewtransaction.FinalVerificationRetryAuthorization(request)
	if err != nil {
		t.Fatal(err)
	}
	before := cliReviewAuthoritySnapshot(t, fixture.repo)
	var output bytes.Buffer
	err = RunReview([]string{"retry-final-verification", "--contract", ReviewIntegrationContractV1, "--cwd", fixture.repo,
		"--predecessor-lineage", request.PredecessorLineageID, "--expected-predecessor-revision", request.ExpectedPredecessorRevision,
		"--successor-lineage", request.SuccessorLineageID, "--incident", fixture.incidentPath,
		"--actor", request.Actor, "--reason", request.Reason, "--maintainer-authorization", authorization + "\n"}, &output)
	if err == nil {
		t.Fatal("inexact authorization succeeded")
	}
	var failure ReviewIntegrationFailure
	decodeStrictReviewJSON(t, output.Bytes(), &failure)
	// organic-dx Phase 3b task 3b.3: STATUS already re-derives retry
	// eligibility for this lineage, so the denial now names review.status
	// instead of a bare stop.
	if failure.Operation != ReviewIntegrationOperationRetryFinalVerification || failure.Code != "final_verification_retry_denied" ||
		failure.MutationOutcome != ReviewMutationNotStarted || failure.RetrySafe || failure.NextAction != "review.status" {
		t.Fatalf("retry denial failure = %#v", failure)
	}
	if after := cliReviewAuthoritySnapshot(t, fixture.repo); !reflect.DeepEqual(after, before) {
		t.Fatalf("retry denial mutated authority: %#v != %#v", after, before)
	}
}

func TestReviewRetryFinalVerificationIncidentInputIsBoundedAndCancellable(t *testing.T) {
	t.Run("oversize regular file", func(t *testing.T) {
		fixture := failedFinalVerificationCLIFixture(t)
		oversize := filepath.Join(t.TempDir(), "oversize-incident.json")
		if err := os.WriteFile(oversize, bytes.Repeat([]byte("x"), 32<<10), 0o600); err != nil {
			t.Fatal(err)
		}
		before := cliReviewAuthoritySnapshot(t, fixture.repo)
		args := finalVerificationRetryCLIArgs(t, fixture, "retry-oversize-successor", oversize)
		err := runReviewRetryFinalVerification(context.Background(), args[1:], &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), "size limit") {
			t.Fatalf("oversize incident error = %v", err)
		}
		if after := cliReviewAuthoritySnapshot(t, fixture.repo); !reflect.DeepEqual(after, before) {
			t.Fatalf("oversize incident mutated authority: %#v != %#v", after, before)
		}
	})

	t.Run("cancelled stdin", func(t *testing.T) {
		fixture := failedFinalVerificationCLIFixture(t)
		reader, writer, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		oldStdin := os.Stdin
		os.Stdin = reader
		t.Cleanup(func() {
			os.Stdin = oldStdin
			_ = writer.Close()
			_ = reader.Close()
		})
		before := cliReviewAuthoritySnapshot(t, fixture.repo)
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		args := finalVerificationRetryCLIArgs(t, fixture, "retry-cancelled-stdin-successor", "-")
		go func() {
			result <- runReviewRetryFinalVerification(ctx, args[1:], &bytes.Buffer{})
		}()
		time.Sleep(25 * time.Millisecond)
		cancel()
		select {
		case err := <-result:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("cancelled stdin error = %v", err)
			}
		case <-time.After(500 * time.Millisecond):
			_ = writer.Close()
			<-result
			t.Fatal("cancelled incident stdin remained blocked")
		}
		if after := cliReviewAuthoritySnapshot(t, fixture.repo); !reflect.DeepEqual(after, before) {
			t.Fatalf("cancelled stdin mutated authority: %#v != %#v", after, before)
		}
	})

	t.Run("aggregate timeout joins without blocking", func(t *testing.T) {
		fixture := failedFinalVerificationCLIFixture(t)
		reader, writer, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		oldStdin, oldTimeout := os.Stdin, reviewFacadeOperationTimeout
		os.Stdin, reviewFacadeOperationTimeout = reader, 25*time.Millisecond
		t.Cleanup(func() {
			os.Stdin, reviewFacadeOperationTimeout = oldStdin, oldTimeout
			_ = writer.Close()
			_ = reader.Close()
		})
		before := cliReviewAuthoritySnapshot(t, fixture.repo)
		var output bytes.Buffer
		result := make(chan error, 1)
		args := finalVerificationRetryCLIArgs(t, fixture, "retry-timeout-stdin-successor", "-")
		go func() {
			result <- RunReview(args, &output)
		}()
		select {
		case err := <-result:
			if err == nil {
				t.Fatal("blocked negotiated stdin unexpectedly succeeded")
			}
		case <-time.After(500 * time.Millisecond):
			_ = writer.Close()
			<-result
			t.Fatal("aggregate retry timeout remained blocked joining incident stdin")
		}
		if after := cliReviewAuthoritySnapshot(t, fixture.repo); !reflect.DeepEqual(after, before) {
			t.Fatalf("timed out stdin mutated authority: %#v != %#v", after, before)
		}
	})
}

func TestFinalVerificationRetryContractFixturesAreStrictAndPathFree(t *testing.T) {
	root := filepath.Join("..", "..", "contracts", "review-integration", "v1", "fixtures")
	incidentPayload, err := os.ReadFile(filepath.Join(root, "final-verification-incident.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	incident, err := reviewtransaction.ParseFinalVerificationIncident(incidentPayload)
	if err != nil || incident.Class != reviewtransaction.FinalVerificationIncidentProceduralToolingFailure {
		t.Fatalf("incident fixture = %#v, %v", incident, err)
	}
	validateFinalVerificationContractSchema(t, "final-verification-incident.schema.json", incidentPayload)
	statusPayload, err := os.ReadFile(filepath.Join(root, "status-v2-final-verification-retry.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusPayload, &status)
	if err := status.Validate(); err != nil {
		t.Fatal(err)
	}
	retryPayload, err := json.Marshal(status.FinalVerificationRetry)
	if err != nil {
		t.Fatal(err)
	}
	validateStatusV2FinalVerificationRetrySchema(t, retryPayload)
	if status.Action != reviewtransaction.TargetStatusActionRetryFinalVerification ||
		status.FinalVerificationRetry == nil || status.NextTransition == nil ||
		status.NextTransition.ReasonCode != "final_verification_retry_authorization_required" ||
		status.FinalVerificationRetry.FailedEvidenceRecordDigest == "" ||
		transitionArgumentValue(t, status.NextTransition, "failed-evidence-record-digest") != status.FinalVerificationRetry.FailedEvidenceRecordDigest {
		t.Fatalf("retry status fixture = %#v", status)
	}
	if strings.Contains(string(statusPayload), "/tmp/") || strings.Contains(string(statusPayload), `\\`) {
		t.Fatal("retry status fixture contains a local path")
	}
}

func TestStatusV2FinalVerificationRetryRecordDigestIsAdditiveAndProviderBound(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "contracts", "review-integration", "v1", "fixtures", "status-v2-final-verification-retry.fixture.json")
	payload, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	var historical map[string]any
	if err := json.Unmarshal(payload, &historical); err != nil {
		t.Fatal(err)
	}
	retry := historical["final_verification_retry"].(map[string]any)
	delete(retry, "failed_evidence_record_digest")
	transition := historical["next_transition"].(map[string]any)
	collection := transition["collect"].(map[string]any)
	inputs := collection["inputs"].([]any)
	input := inputs[0].(map[string]any)
	arguments := input["arguments"].([]any)
	legacyArguments := make([]any, 0, len(arguments)-1)
	for _, raw := range arguments {
		argument := raw.(map[string]any)
		if argument["name"] != "failed-evidence-record-digest" {
			legacyArguments = append(legacyArguments, argument)
		}
	}
	input["arguments"] = legacyArguments
	historicalPayload, err := json.Marshal(historical)
	if err != nil {
		t.Fatal(err)
	}
	historicalRetryPayload, err := json.Marshal(retry)
	if err != nil {
		t.Fatal(err)
	}
	validateStatusV2FinalVerificationRetrySchema(t, historicalRetryPayload)
	var historicalStatus ReviewTargetStatusResult
	decodeStrictReviewJSON(t, historicalPayload, &historicalStatus)
	if err := historicalStatus.Validate(); err != nil || historicalStatus.FinalVerificationRetry == nil || historicalStatus.FinalVerificationRetry.FailedEvidenceRecordDigest != "" {
		t.Fatalf("historical status-v2 retry artifact = %#v, %v", historicalStatus.FinalVerificationRetry, err)
	}

	var mismatched ReviewTargetStatusResult
	decodeStrictReviewJSON(t, payload, &mismatched)
	for index := range mismatched.NextTransition.Collect.Inputs[0].Arguments {
		argument := &mismatched.NextTransition.Collect.Inputs[0].Arguments[index]
		if argument.Name == "failed-evidence-record-digest" {
			argument.Value = "sha256:" + strings.Repeat("c", 64)
		}
	}
	if err := mismatched.Validate(); err == nil {
		t.Fatal("new status-v2 retry artifact accepted a transition bound to a different evidence record digest")
	}
}

func validateStatusV2FinalVerificationRetrySchema(t *testing.T, payload []byte) {
	t.Helper()
	path := filepath.Join("..", "..", "contracts", "review-integration", "v1", "schemas", "status-v2.schema.json")
	schemaPayload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(schemaPayload, &document); err != nil {
		t.Fatal(err)
	}
	definitions := document["$defs"].(map[string]any)
	properties := document["properties"].(map[string]any)
	retry := properties["final_verification_retry"].(map[string]any)
	const location = "https://gentle-ai.dev/contracts/review-integration/v1/schemas/_test-final-verification-retry.schema.json"
	synthetic := map[string]any{
		"$schema": document["$schema"],
		"$id":     location,
		"$defs":   map[string]any{"sha256": definitions["sha256"]},
	}
	for key, value := range retry {
		synthetic[key] = value
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(location, synthetic); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(location)
	if err != nil {
		t.Fatal(err)
	}
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(value); err != nil {
		t.Fatalf("published status-v2 final_verification_retry schema rejected artifact: %v", err)
	}
}

func validateFinalVerificationContractSchema(t *testing.T, name string, payload []byte) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "contracts", "review-integration", "v1", "schemas"))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		schemaPayload, readErr := os.ReadFile(filepath.Join(root, entry.Name()))
		if readErr != nil {
			t.Fatal(readErr)
		}
		var document any
		if err := json.Unmarshal(schemaPayload, &document); err != nil {
			t.Fatalf("decode %s: %v", entry.Name(), err)
		}
		location := "https://gentle-ai.dev/contracts/review-integration/v1/schemas/" + entry.Name()
		if err := compiler.AddResource(location, document); err != nil {
			t.Fatalf("add %s: %v", entry.Name(), err)
		}
	}
	schema, err := compiler.Compile("https://gentle-ai.dev/contracts/review-integration/v1/schemas/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatalf("%s rejected fixture: %v", name, err)
	}
}

func TestReviewHelpPublishesDedicatedFinalVerificationRetryBoundary(t *testing.T) {
	var output bytes.Buffer
	if err := RunReview([]string{"help"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "retry-final-verification") ||
		!strings.Contains(output.String(), "Generic review recover remains unchanged") {
		t.Fatalf("review help omits the dedicated boundary:\n%s", output.String())
	}
}

type failedFinalVerificationCLI struct {
	repo         string
	predecessor  reviewtransaction.CompactRecord
	incident     reviewtransaction.FinalVerificationIncident
	incidentPath string
}

func finalVerificationRetryCLIArgs(t *testing.T, fixture failedFinalVerificationCLI, successor, incidentPath string) []string {
	t.Helper()
	request := reviewtransaction.FinalVerificationRetryRequest{
		PredecessorLineageID: fixture.predecessor.State.LineageID, ExpectedPredecessorRevision: fixture.predecessor.Revision,
		SuccessorLineageID: successor, Incident: fixture.incident, Actor: "maintainer", Reason: "retry after provider tooling failure",
	}
	authorization, err := reviewtransaction.FinalVerificationRetryAuthorization(request)
	if err != nil {
		t.Fatal(err)
	}
	return []string{"retry-final-verification", "--contract", ReviewIntegrationContractV1, "--cwd", fixture.repo,
		"--predecessor-lineage", request.PredecessorLineageID, "--expected-predecessor-revision", request.ExpectedPredecessorRevision,
		"--successor-lineage", request.SuccessorLineageID, "--incident", incidentPath,
		"--actor", request.Actor, "--reason", request.Reason, "--maintainer-authorization", authorization}
}

func failedFinalVerificationCLIFixture(t *testing.T) failedFinalVerificationCLI {
	t.Helper()
	repo, started, _, _, _ := capturedArtifact(t)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--captured-results"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	validating, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	evidencePath := filepath.Join(t.TempDir(), "failed-evidence.txt")
	evidence := []byte("provider final verification tooling failed\n")
	if err := os.WriteFile(evidencePath, evidence, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewCaptureEvidence([]string{"--cwd", repo, "--lineage", started.LineageID,
		"--target", validating.State.CurrentSnapshot.Identity, "--expected-revision", validating.Revision, "--outcome", string(reviewtransaction.VerificationOutcomeProceduralFailure), "--input", evidencePath}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--captured-evidence", "--failed"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	predecessor, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if predecessor.State.State != reviewtransaction.StateEscalated {
		t.Fatalf("failed fixture state = %q", predecessor.State.State)
	}
	requestDigest := completedFinalizeRequestDigest(t, store.FinalizeAttemptJournalPath(), predecessor.Revision)
	incident := reviewtransaction.FinalVerificationIncident{
		Schema: reviewtransaction.FinalVerificationIncidentSchema, Class: reviewtransaction.FinalVerificationIncidentProceduralToolingFailure,
		LineageID: predecessor.State.LineageID, TerminalRevision: predecessor.Revision, ValidatingRevision: validating.Revision,
		TargetIdentity: predecessor.State.CurrentSnapshot.Identity, FailedEvidenceHash: predecessor.State.EvidenceHash, FinalizeRequestDigest: requestDigest,
	}
	payload, err := reviewtransaction.CanonicalFinalVerificationIncident(incident)
	if err != nil {
		t.Fatal(err)
	}
	incidentPath := filepath.Join(t.TempDir(), "incident.json")
	if err := os.WriteFile(incidentPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return failedFinalVerificationCLI{repo: repo, predecessor: predecessor, incident: incident, incidentPath: incidentPath}
}

func failedCorrectedFinalVerificationCLIFixture(t *testing.T) failedFinalVerificationCLI {
	t.Helper()
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := runNegotiatedReviewStart(t, repo, "retry-corrected-cli-source")
	resultPath := filepath.Join(t.TempDir(), "blocking-result.json")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{
		Lens: started.SelectedLenses[0], Findings: []facadeFinding{{
			Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "terminal value is incorrect",
			ProofRefs: []string{"tracked.txt:5 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
			CausalDisposition: reviewtransaction.CausalIntroduced,
		}}, Evidence: []string{"inspected corrected candidate"},
	})
	if err := finalizeReviewCLIArgs(t, repo, []string{"--cwd", repo, "--lineage", started.LineageID, "--result", resultPath}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--correction-lines", "2"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfixed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	validationPath := filepath.Join(t.TempDir(), "validation.json")
	validation := facadeValidationResult{
		OriginalCriteria:     facadeValidationCheck{Passed: true, Evidence: []string{"original criteria passed"}},
		CorrectionRegression: facadeValidationCheck{Passed: true, Evidence: []string{"correction regression passed"}},
		FollowUps:            []reviewtransaction.FollowUp{},
	}
	writeReviewCLIJSON(t, validationPath, validation)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	correction, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	fix, err := facadeVerificationEvidenceTarget(context.Background(), repo, correction.State, correction.Revision)
	if err != nil {
		t.Fatal(err)
	}
	request, err := reviewtransaction.BuildTargetedValidationRequestFromSnapshot(context.Background(), repo, correction.State, correction.Revision, fix)
	if err != nil {
		t.Fatal(err)
	}
	validation.TargetedValidationRequestHash = request.RequestHash
	validation.CorrectionTargetIdentity = request.CorrectionTargetIdentity
	native, err := validation.compact(reviewtransaction.FixDeltaHashForSnapshot(fix), correction.State.FixFindingIDs, request)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ChangedLines(context.Background(), fix)
	if err != nil {
		t.Fatal(err)
	}
	legacyValidating := correction.State
	if err := legacyValidating.CompleteCorrection(fix, actual, native); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(correction.Revision, "review/complete-fix", legacyValidating); err != nil {
		t.Fatal(err)
	}
	validating, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	evidencePath := filepath.Join(t.TempDir(), "corrected-failed-evidence.txt")
	evidence := []byte("corrected provider final verification tooling failed\n")
	if err := os.WriteFile(evidencePath, evidence, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewCaptureEvidence([]string{"--cwd", repo, "--lineage", started.LineageID,
		"--target", validating.State.CurrentSnapshot.Identity, "--expected-revision", validating.Revision, "--outcome", string(reviewtransaction.VerificationOutcomeProceduralFailure), "--input", evidencePath}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--captured-evidence", "--failed"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	predecessor, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	requestDigest := completedFinalizeRequestDigest(t, store.FinalizeAttemptJournalPath(), predecessor.Revision)
	incident := reviewtransaction.FinalVerificationIncident{
		Schema: reviewtransaction.FinalVerificationIncidentSchema, Class: reviewtransaction.FinalVerificationIncidentProceduralToolingFailure,
		LineageID: predecessor.State.LineageID, TerminalRevision: predecessor.Revision, ValidatingRevision: validating.Revision,
		TargetIdentity: predecessor.State.CurrentSnapshot.Identity, FailedEvidenceHash: predecessor.State.EvidenceHash, FinalizeRequestDigest: requestDigest,
	}
	payload, err := reviewtransaction.CanonicalFinalVerificationIncident(incident)
	if err != nil {
		t.Fatal(err)
	}
	incidentPath := filepath.Join(t.TempDir(), "corrected-incident.json")
	if err := os.WriteFile(incidentPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return failedFinalVerificationCLI{repo: repo, predecessor: predecessor, incident: incident, incidentPath: incidentPath}
}

func completedFinalizeRequestDigest(t *testing.T, path, terminalRevision string) string {
	t.Helper()
	var journal struct {
		Attempts []struct {
			Request struct {
				RequestDigest string `json:"request_digest"`
			} `json:"request"`
			Transitions []struct {
				Operation string `json:"operation"`
				Revision  string `json:"revision"`
			} `json:"transitions"`
			ReceiptPublished bool `json:"receipt_published"`
			Completed        bool `json:"completed"`
		} `json:"attempts"`
	}
	payload, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(payload, &journal) != nil {
		t.Fatalf("read finalize journal: %v", err)
	}
	for _, attempt := range journal.Attempts {
		if !attempt.Completed || !attempt.ReceiptPublished || len(attempt.Transitions) == 0 {
			continue
		}
		last := attempt.Transitions[len(attempt.Transitions)-1]
		if last.Operation == "review/complete-verification" && last.Revision == terminalRevision {
			return attempt.Request.RequestDigest
		}
	}
	t.Fatal("completed final-verification attempt not found")
	return ""
}

func cliReviewAuthoritySnapshot(t *testing.T, repo string) map[string]string {
	t.Helper()
	gitDir := strings.TrimSpace(runReviewCLIGitOutput(t, repo, "rev-parse", "--git-common-dir"))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(repo, gitDir)
	}
	root := filepath.Join(gitDir, "gentle-ai", "review-transactions")
	result := map[string]string{}
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || entry.Name() == "LOCK" || strings.HasPrefix(entry.Name(), ".atomic-") {
			return err
		}
		payload, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		relative, _ := filepath.Rel(root, path)
		result[filepath.ToSlash(relative)] = facadePayloadHash(payload)
		return nil
	})
	return result
}

func runReviewCLIGitOutput(t *testing.T, repo string, args ...string) string {
	t.Helper()
	command := append([]string{"-C", repo}, args...)
	output, err := exec.Command("git", command...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", command, err, output)
	}
	return string(output)
}
