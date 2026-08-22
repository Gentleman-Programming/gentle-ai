package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestCrossRepositoryLifecycleRootConformance(t *testing.T) {
	const lineage = "cross-repository-lifecycle-root"

	matrix := []struct {
		name      string
		runtime   model.AgentID
		supported bool
	}{
		{name: "Claude Code", runtime: model.AgentClaudeCode, supported: true},
		{name: "Codex", runtime: model.AgentCodex, supported: true},
		{name: "OpenCode", runtime: model.AgentOpenCode, supported: true},
		{name: "Pi", runtime: model.AgentPi, supported: true},
		{name: "unsupported Kilo", runtime: model.AgentKilocode},
	}

	supported := 0
	for _, testCase := range matrix {
		if testCase.supported {
			supported++
		}
		t.Run(testCase.name, func(t *testing.T) {
			reviewEnabledHome(t)
			if testCase.runtime == model.AgentPi {
				t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
			}

			sessionRoot := initReviewCLIRepo(t)
			targetRoot := initReviewCLIRepo(t)
			targetNested := filepath.Join(targetRoot, "nested", "requested")
			if err := os.MkdirAll(targetNested, 0o755); err != nil {
				t.Fatal(err)
			}
			t.Chdir(sessionRoot)

			if !testCase.supported {
				assertUnsupportedCrossRepositoryLifecycleRoot(t, targetNested, testCase.runtime)
				return
			}

			writeCrossRepositoryLifecycleCandidate(t, sessionRoot, "session")
			writeCrossRepositoryLifecycleCandidate(t, targetRoot, "target")

			sessionStatus := crossRepositoryLifecycleStatus(t, sessionRoot, lineage, testCase.runtime)
			sessionStarted := startCrossRepositoryLifecycleFromTransition(t, sessionStatus, testCase.runtime)
			sessionStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), sessionRoot, lineage)
			if err != nil {
				t.Fatal(err)
			}
			sessionAuthorityBefore, err := os.ReadFile(sessionStore.StatePath())
			if err != nil {
				t.Fatal(err)
			}
			sessionCandidatePath := filepath.Join(sessionRoot, "internal", "auth", "session.go")
			sessionCandidateBefore, err := os.ReadFile(sessionCandidatePath)
			if err != nil {
				t.Fatal(err)
			}

			t.Chdir(sessionRoot)
			targetStatus := crossRepositoryLifecycleStatus(t, targetNested, lineage, testCase.runtime)
			if targetStatus.Authority != nil {
				t.Fatalf("selectorless B STATUS selected A authority: %#v", targetStatus.Authority)
			}
			assertCrossRepositoryStartTransition(t, targetStatus, targetRoot, lineage, testCase.runtime)

			t.Chdir(targetRoot)
			targetStarted := startCrossRepositoryLifecycleFromTransition(t, targetStatus, testCase.runtime)
			if sessionStarted.RepositoryContext == nil || targetStarted.RepositoryContext == nil ||
				sessionStarted.RepositoryContext.Handle == targetStarted.RepositoryContext.Handle {
				t.Fatalf("same lineage text collided across repositories: A=%#v B=%#v", sessionStarted.RepositoryContext, targetStarted.RepositoryContext)
			}
			targetStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), targetRoot, lineage)
			if err != nil {
				t.Fatal(err)
			}
			if sessionStore.Dir == targetStore.Dir {
				t.Fatalf("same lineage text selected one compact store: %q", sessionStore.Dir)
			}

			targetContinuation := crossRepositoryLifecycleStatus(t, targetRoot, lineage, testCase.runtime)
			assertCrossRepositoryContinuation(t, targetContinuation, targetStarted, lineage, "B")
			t.Chdir(sessionRoot)
			sessionContinuation := crossRepositoryLifecycleStatus(t, sessionRoot, lineage, testCase.runtime)
			assertCrossRepositoryContinuation(t, sessionContinuation, sessionStarted, lineage, "A")
			if sessionContinuation.RepositoryContext.Handle == targetContinuation.RepositoryContext.Handle {
				t.Fatal("A and B active continuations shared one opaque repository context")
			}
			if testCase.runtime == model.AgentOpenCode {
				t.Chdir(sessionRoot)
				assertOpenCodeBContextAdmitsFromSessionA(t, targetRoot, targetStore, targetStarted)
			}

			t.Chdir(targetRoot)
			legacyTargetStart := ReviewFacadeStartResult{
				LineageID: targetStarted.LineageID, TargetIdentity: targetStarted.RepositoryContext.TargetIdentity,
				SelectedLenses: append([]string(nil), targetStarted.SelectedLenses...),
			}
			for order := range legacyTargetStart.SelectedLenses {
				captureCLIReviewerResult(t, targetRoot, legacyTargetStart, order)
			}
			if err := RunReviewFacadeFinalize([]string{
				"--cwd", targetRoot, "--lineage", lineage, "--captured-results=true",
			}, &bytes.Buffer{}); err != nil {
				t.Fatalf("advance B to validation: %v", err)
			}
			targetValidating := crossRepositoryLifecycleStatus(t, targetRoot, lineage, testCase.runtime)
			assertCrossRepositoryContinuation(t, targetValidating, targetStarted, lineage, "B validating")

			evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
			if err := os.WriteFile(evidencePath, []byte("focused cross-repository verification passed\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Chdir(sessionRoot)
			if err := RunReviewCaptureEvidence([]string{
				"--repository-context", targetValidating.RepositoryContext.Handle,
				"--lineage", lineage,
				"--target", targetValidating.RepositoryContext.TargetIdentity,
				"--expected-revision", targetValidating.RepositoryContext.Revision,
				"--outcome", string(reviewtransaction.VerificationOutcomePassed),
				"--input", evidencePath,
			}, &bytes.Buffer{}); err != nil {
				t.Fatalf("capture B evidence while process cwd is A: %v", err)
			}
			targetRecord, err := targetStore.Load()
			if err != nil {
				t.Fatal(err)
			}
			captured, err := reviewtransaction.ReadCapturedVerificationEvidence(
				targetStore.Dir, lineage, targetRecord.Revision, targetRecord.State.CurrentSnapshot,
			)
			if err != nil || captured.Record.Outcome != reviewtransaction.VerificationOutcomePassed {
				t.Fatalf("B captured evidence = %#v, err=%v", captured, err)
			}
			sessionAuthorityAfterCapture, err := os.ReadFile(sessionStore.StatePath())
			if err != nil {
				t.Fatal(err)
			}
			sessionCandidateAfterCapture, err := os.ReadFile(sessionCandidatePath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(sessionAuthorityAfterCapture, sessionAuthorityBefore) || !bytes.Equal(sessionCandidateAfterCapture, sessionCandidateBefore) {
				t.Fatal("B opaque evidence capture changed A authority or candidate bytes")
			}

			t.Chdir(targetRoot)
			var finalizedOutput bytes.Buffer
			if err := RunReviewFacadeFinalize([]string{
				"--cwd", targetRoot, "--lineage", lineage, "--captured-evidence=true",
			}, &finalizedOutput); err != nil {
				t.Fatalf("finalize B captured evidence: %v", err)
			}
			var finalized ReviewFacadeFinalizeResult
			if err := json.Unmarshal(finalizedOutput.Bytes(), &finalized); err != nil {
				t.Fatal(err)
			}
			if finalized.State != reviewtransaction.StateApproved {
				t.Fatalf("B finalize = %#v, want approved", finalized)
			}
			assertApprovedCompactAuthorityBurned(t, targetStore, lineage)
			sessionAuthorityAfterBurn, err := os.ReadFile(sessionStore.StatePath())
			if err != nil {
				t.Fatal(err)
			}
			sessionCandidateAfterBurn, err := os.ReadFile(sessionCandidatePath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(sessionAuthorityAfterBurn, sessionAuthorityBefore) || !bytes.Equal(sessionCandidateAfterBurn, sessionCandidateBefore) {
				t.Fatal("B approval burn changed A authority or candidate bytes")
			}
			if record, err := sessionStore.Load(); err != nil || record.State.State != reviewtransaction.StateReviewing {
				t.Fatalf("A authority after B burn = %#v, err=%v", record.State, err)
			}
		})
	}
	if supported != 4 {
		t.Fatalf("supported runtime rows = %d, want 4", supported)
	}
}

func writeCrossRepositoryLifecycleCandidate(t *testing.T, repo, marker string) {
	t.Helper()
	writeReviewStartCandidate(t, repo, "internal/auth/session.go", "package auth\n\nfunc CheckToken(token string) bool {\n\treturn token == \""+marker+"\"\n}\n", 0o644)
}

func crossRepositoryLifecycleStatus(t *testing.T, cwd, lineage string, runtime model.AgentID) ReviewTargetStatusResult {
	t.Helper()
	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", cwd, "--contract", ReviewIntegrationContractV2,
		"--agent", string(runtime), "--next-transition", "--lineage", lineage,
	}, &output); err != nil {
		t.Fatalf("status for %s at %s: %v\n%s", runtime, cwd, err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if status.NextTransition == nil {
		t.Fatalf("status for %s at %s omitted next transition: %#v", runtime, cwd, status)
	}
	return status
}

func assertCrossRepositoryStartTransition(t *testing.T, status ReviewTargetStatusResult, root, lineage string, runtime model.AgentID) {
	t.Helper()
	if status.NextTransition.Execute == nil || status.NextTransition.Execute.Operation != "review.start" || status.NextTransition.Execute.Binding.LineageID != lineage {
		t.Fatalf("fresh B transition = %#v", status.NextTransition)
	}
	rootFound, runtimeFound := false, false
	for _, argument := range status.NextTransition.Execute.Arguments {
		if argument.Token == "" {
			t.Fatalf("START emitted an untokenized argument: %#v", argument)
		}
		rootFound = rootFound || argument.Name == "cwd" && argument.Value == root
		runtimeFound = runtimeFound || argument.Name == "agent" && argument.Value == string(runtime)
	}
	if !rootFound || !runtimeFound {
		t.Fatalf("START did not preserve B root/runtime: %#v", status.NextTransition.Execute)
	}
}

func startCrossRepositoryLifecycleFromTransition(t *testing.T, status ReviewTargetStatusResult, runtime model.AgentID) ReviewIntegrationStartResult {
	t.Helper()
	if status.NextTransition == nil || status.NextTransition.Execute == nil || status.NextTransition.Execute.Operation != "review.start" {
		t.Fatalf("START transition unavailable: %#v", status.NextTransition)
	}
	args := []string{"start"}
	for _, argument := range status.NextTransition.Execute.Arguments {
		if argument.Token == "" {
			t.Fatalf("START argument has no exact token: %#v", argument)
		}
		args = append(args, argument.Token)
	}
	question := decodeConsentQuestion(t, runConsentRelayStart(t, args).Bytes())
	if question.Agent != string(runtime) {
		t.Fatalf("consent runtime = %q, want %q", question.Agent, runtime)
	}
	for _, choice := range question.Choices {
		if choice.Answer == "granted" {
			return decodeNegotiatedReviewStart(t, runConsentRelayStart(t, invocationArgs(t, choice.Invocation)).Bytes())
		}
	}
	t.Fatalf("consent envelope has no granted invocation: %#v", question.Choices)
	return ReviewIntegrationStartResult{}
}

func assertCrossRepositoryContinuation(t *testing.T, status ReviewTargetStatusResult, started ReviewIntegrationStartResult, lineage, label string) {
	t.Helper()
	if started.RepositoryContext == nil {
		t.Fatalf("%s START has no repository context", label)
	}
	if status.Authority == nil || status.Authority.LineageID != lineage || status.RepositoryContext == nil ||
		status.RepositoryContext.TargetIdentity != started.RepositoryContext.TargetIdentity {
		t.Fatalf("%s continuation = %#v, want %q bound to target %q", label, status, lineage, started.RepositoryContext.TargetIdentity)
	}
}

func assertOpenCodeBContextAdmitsFromSessionA(t *testing.T, targetRoot string, store reviewtransaction.CompactStore, started ReviewIntegrationStartResult) {
	t.Helper()
	if started.RepositoryContext == nil {
		t.Fatal("OpenCode B START has no repository context")
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	lens := record.State.SelectedLenses[0]
	subject := mustArtifactSubject(t, targetRoot, record, lens, 0)
	binding := hostLensBindingJSON(record.State.LineageID, started.RepositoryContext.TargetIdentity, lens, "0", record.Revision, started.RepositoryContext.Handle, subject.SubjectHash)
	relay := startOpenCodeTransportRelay(t, openCodeTransportEnvelope{
		Schema:    openCodeReviewTransportSchema,
		Operation: "start",
		Prompt:    reviewLensContextBindingHeader + " " + binding + "\nOpenCode host task.",
	})
	taskPrompt, reintercepted, err := decodeOpenCodeTransportMaterialization(relay.prompt.Prompt)
	if err != nil || !reintercepted || !bytes.Contains([]byte(taskPrompt), []byte(started.RepositoryContext.Handle)) {
		t.Fatalf("OpenCode B context materialization = %q, reintercepted=%t, err=%v", taskPrompt, reintercepted, err)
	}
	if err := relay.closeWithoutCompletion(); err == nil {
		t.Fatal("OpenCode relay without a model result unexpectedly succeeded")
	}
}

func assertUnsupportedCrossRepositoryLifecycleRoot(t *testing.T, targetNested string, runtime model.AgentID) {
	t.Helper()
	before, err := os.ReadFile(filepath.Join(filepath.Dir(filepath.Dir(targetNested)), "tracked.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err = RunReview([]string{
		"status", "--cwd", targetNested, "--contract", ReviewIntegrationContractV2,
		"--agent", string(runtime), "--next-transition",
	}, &output)
	if err == nil {
		t.Fatalf("unsupported runtime %q accepted foreign B status", runtime)
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Code != reviewImmutableTransportUnsupportedCode || failure.MutationOutcome != ReviewMutationNotStarted ||
		failure.AuthorityApplicability != "not_evaluated" {
		t.Fatalf("unsupported foreign-root failure = %#v", failure)
	}
	after, err := os.ReadFile(filepath.Join(filepath.Dir(filepath.Dir(targetNested)), "tracked.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("unsupported runtime changed B bytes before transport refusal")
	}
	stores, err := reviewtransaction.DiscoverCompactStores(context.Background(), filepath.Dir(filepath.Dir(targetNested)))
	if err != nil {
		t.Fatal(err)
	}
	if len(stores) != 0 {
		t.Fatalf("unsupported runtime created B authority: %#v", stores)
	}
}
