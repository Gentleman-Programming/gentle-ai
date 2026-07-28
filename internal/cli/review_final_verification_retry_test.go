package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
		"--target", successor.State.CurrentSnapshot.Identity, "--expected-revision", successor.Revision, "--input", passed}, &bytes.Buffer{}); err != nil {
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
		"--target", successorStatus.AuthorityTargetIdentity, "--expected-revision", successorStatus.Authority.Revision, "--input", passed}, &bytes.Buffer{}); err != nil {
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
	if status.Action != reviewtransaction.TargetStatusActionRetryFinalVerification ||
		status.FinalVerificationRetry == nil || status.NextTransition == nil ||
		status.NextTransition.ReasonCode != "final_verification_retry_authorization_required" {
		t.Fatalf("retry status fixture = %#v", status)
	}
	if strings.Contains(string(statusPayload), "/tmp/") || strings.Contains(string(statusPayload), `\\`) {
		t.Fatal("retry status fixture contains a local path")
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
		"--target", validating.State.CurrentSnapshot.Identity, "--expected-revision", validating.Revision, "--input", evidencePath}, &bytes.Buffer{}); err != nil {
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
	writeReviewCLIJSON(t, validationPath, facadeValidationResult{
		OriginalCriteria:     facadeValidationCheck{Passed: true, Evidence: []string{"original criteria passed"}},
		CorrectionRegression: facadeValidationCheck{Passed: true, Evidence: []string{"correction regression passed"}},
		FollowUps:            []reviewtransaction.FollowUp{},
	})
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--validation", validationPath}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
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
		"--target", validating.State.CurrentSnapshot.Identity, "--expected-revision", validating.Revision, "--input", evidencePath}, &bytes.Buffer{}); err != nil {
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
