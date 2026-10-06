package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// Only OpenCode hosts receive the sealed rctx3 handle (#5136, #4516): their
// relay runs in the host session directory, which names nothing about the
// review. These tests pin where it appears, that every consumer of an OpenCode
// collect input resolves it from any cwd, and that every other runtime keeps
// the unchanged rctx2 digest.

func TestOpenCodeLineageStatusSealsOnlyItsCollectInputs(t *testing.T) {
	// Inheritance applies only without the Pi relay handshake; a Pi host
	// running this test must not turn the STATUS into a Pi-driven one.
	t.Setenv(reviewPiHostRelayContractEnvironment, "")
	if testing.Short() {
		t.Skip("requires relay subprocesses")
	}
	reviewEnabledHome(t)
	repo, started, store, record := openCodeSealedLineage(t, "opencode-sealed-status")
	if started.RepositoryContext == nil || !strings.HasPrefix(started.RepositoryContext.Handle, "rctx2_") {
		t.Fatalf("START envelope repository context = %#v, want the unchanged rctx2 digest", started.RepositoryContext)
	}

	raw, status := openCodeSealedStatus(t, repo, started.LineageID, "")
	validatePublishedReviewSchema(t, compileWholeNativeStatusSchema(t, "status-v9.schema.json"), raw)
	if status.RepositoryContext == nil || !strings.HasPrefix(status.RepositoryContext.Handle, "rctx2_") {
		t.Fatalf("STATUS envelope repository context = %#v, want the unchanged rctx2 digest", status.RepositoryContext)
	}
	input := openCodeSealedCollectInput(t, status)
	sealed := reviewTransitionArgumentValue(t, input, "repository-context")
	if !strings.HasPrefix(sealed, "rctx3_") {
		t.Fatalf("OpenCode collect input repository context = %q, want rctx3", sealed)
	}
	if input.ProviderTask == nil || !strings.Contains(input.ProviderTask.Prompt, `"repository_context":"`+sealed+`"`) {
		t.Fatalf("OpenCode provider task does not carry the sealed handle: %#v", input.ProviderTask)
	}
	if bytes.Contains(raw, []byte(repo)) {
		t.Fatalf("OpenCode STATUS leaked the repository path: %s", raw)
	}
	_, again := openCodeSealedStatus(t, repo, started.LineageID, "")
	if reviewTransitionArgumentValue(t, openCodeSealedCollectInput(t, again), "repository-context") != sealed {
		t.Fatal("OpenCode STATUS reissued a different sealed handle for the same lineage")
	}

	// The exact provider-issued Task, relayed from a git-less session root.
	lens := record.State.SelectedLenses[0]
	hostOutput := string(admittedReviewerPayloadForTest(t, repo, record, lens, 0))
	relay := startOpenCodeTransportRelay(t, filepath.Dir(repo), openCodeTransportEnvelope{
		Schema: openCodeReviewTransportSchema, Operation: "start", Prompt: input.ProviderTask.Prompt,
	})
	if _, err := relay.complete(openCodeTransportEnvelope{
		Schema: openCodeReviewTransportSchema, Operation: "complete", Nonce: relay.prompt.Nonce, Output: &hostOutput,
	}); err != nil {
		t.Fatal(err)
	}
	assertApprovedCompactAuthorityBurned(t, store, record.State.LineageID)
}

func TestOpenCodeSealedCollectArgumentsResolveFromAnUnrelatedCwd(t *testing.T) {
	// Inheritance applies only without the Pi relay handshake; a Pi host
	// running this test must not turn the STATUS into a Pi-driven one.
	t.Setenv(reviewPiHostRelayContractEnvironment, "")
	if testing.Short() {
		t.Skip("requires git fixtures")
	}
	reviewEnabledHome(t)
	for _, tt := range []struct {
		name string
		run  func(t *testing.T, args sealedCollectArguments) error
	}{
		{name: "lens-context", run: func(t *testing.T, args sealedCollectArguments) error {
			var output bytes.Buffer
			err := RunReview([]string{
				"lens-context", "--repository-context", args.context, "--lineage", args.lineage, "--target", args.target,
				"--expected-revision", args.revision, "--lens", args.lens,
			}, &output)
			if err == nil && !strings.Contains(output.String(), reviewLensContextBindingHeader) {
				t.Fatalf("lens-context output carries no binding: %s", output.String())
			}
			return err
		}},
		{name: "capture-result", run: func(t *testing.T, args sealedCollectArguments) error {
			path := filepath.Join(t.TempDir(), "reviewer.json")
			if err := os.WriteFile(path, admittedReviewerPayloadForTest(t, args.repo, args.record, args.lens, 0), 0o600); err != nil {
				t.Fatal(err)
			}
			return RunReviewCaptureResult([]string{
				"--repository-context", args.context, "--lineage", args.lineage, "--target", args.target,
				"--expected-revision", args.revision, "--lens", args.lens, "--order", "0", "--input", path,
			}, &bytes.Buffer{})
		}},
		{name: "capture-unachievable", run: func(t *testing.T, args sealedCollectArguments) error {
			return RunReview([]string{
				"capture-unachievable", "--repository-context", args.context, "--lineage", args.lineage, "--target", args.target,
				"--expected-revision", args.revision, "--request-hash", args.subjectHash,
				"--reason", "relay_transport_bound_exceeded", "--detail", "host relay bound exceeded",
			}, &bytes.Buffer{})
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo, started, store, record := openCodeSealedLineage(t, "opencode-sealed-"+tt.name)
			_, status := openCodeSealedStatus(t, repo, started.LineageID, "")
			args := sealedCollectArgumentsFrom(t, repo, record, openCodeSealedCollectInput(t, status))
			before, err := os.ReadFile(store.StatePath())
			if err != nil {
				t.Fatal(err)
			}
			t.Chdir(t.TempDir())
			if err := tt.run(t, args); err != nil {
				t.Fatalf("%s with the sealed handle from an unrelated cwd: %v", tt.name, err)
			}
			after, err := os.ReadFile(store.StatePath())
			if err != nil {
				t.Fatal(err)
			}
			if mutates := tt.name != "lens-context"; mutates == bytes.Equal(before, after) {
				t.Fatalf("%s authority mutation = %v, want %v", tt.name, !bytes.Equal(before, after), mutates)
			}
		})
	}
}

func TestSharedResolverRefusesATamperedSealedHandleWithoutAuthorityMutation(t *testing.T) {
	if testing.Short() {
		t.Skip("requires git fixtures")
	}
	reviewEnabledHome(t)
	repo, started, store, record := openCodeSealedLineage(t, "opencode-sealed-tamper")
	_, status := openCodeSealedStatus(t, repo, started.LineageID, "")
	args := sealedCollectArgumentsFrom(t, repo, record, openCodeSealedCollectInput(t, status))
	index := len(args.context) - 7
	replacement := byte('A')
	if args.context[index] == 'A' {
		replacement = 'B'
	}
	args.context = args.context[:index] + string(replacement) + args.context[index+1:]
	path := filepath.Join(t.TempDir(), "reviewer.json")
	if err := os.WriteFile(path, admittedReviewerPayloadForTest(t, repo, record, args.lens, 0), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	err = RunReviewCaptureResult([]string{
		"--repository-context", args.context, "--lineage", args.lineage, "--target", args.target,
		"--expected-revision", args.revision, "--lens", args.lens, "--order", "0", "--input", path,
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "repository_context_") || strings.Contains(err.Error(), repo) {
		t.Fatalf("tampered sealed capture error = %v", err)
	}
	after, err := os.ReadFile(store.StatePath())
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("tampered sealed capture mutated compact authority: %v", err)
	}
}

// A STATUS renders for the runtime that drives it: the declared --agent, else
// the lineage's frozen runtime. A lineage driven by another runtime is
// reissued in that runtime's format, so a Pi host never sees rctx3 and an
// OpenCode host never receives a digest its relay cannot resolve.
func TestCrossRuntimeStatusReissuesTheHandleInTheDrivingRuntimesFormat(t *testing.T) {
	// Pi is admitted only through its relay handshake; declare it so the test
	// does not depend on running inside a Pi host.
	t.Setenv("GENTLE_PI_REVIEW_RELAY_CONTRACT", "gentle-pi.review-relay/v1")
	if testing.Short() {
		t.Skip("requires relay subprocesses")
	}
	reviewEnabledHome(t)

	t.Run("OpenCode lineage driven by Pi", func(t *testing.T) {
		repo, started, _, _ := openCodeSealedLineage(t, "opencode-driven-by-pi")
		raw, status := openCodeSealedStatus(t, repo, started.LineageID, model.AgentPi)
		if bytes.Contains(raw, []byte("rctx3_")) {
			t.Fatalf("Pi-driven STATUS carried a sealed handle: %s", raw)
		}
		if status.RepositoryContext == nil || !strings.HasPrefix(status.RepositoryContext.Handle, "rctx2_") {
			t.Fatalf("Pi-driven STATUS repository context = %#v", status.RepositoryContext)
		}
	})

	t.Run("Pi lineage driven by OpenCode", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc driven() int { return 1 }\n", 0o644)
		started := runNegotiatedReviewStartWith(t, repo, "pi-driven-by-opencode", "--agent", string(model.AgentPi))
		store, record := loadOpenCodeRelayAuthority(t, repo, started.LineageID)
		if record.State.RuntimeAgent != string(model.AgentPi) {
			t.Fatalf("fixture froze runtime %q, want pi", record.State.RuntimeAgent)
		}
		raw, piStatus := openCodeSealedStatus(t, repo, started.LineageID, "")
		if bytes.Contains(raw, []byte("rctx3_")) || piStatus.RepositoryContext == nil {
			t.Fatalf("Pi lineage STATUS carried a sealed handle or no context: %s", raw)
		}
		_, status := openCodeSealedStatus(t, repo, started.LineageID, model.AgentOpenCode)
		input := openCodeSealedCollectInput(t, status)
		if !strings.HasPrefix(reviewTransitionArgumentValue(t, input, "repository-context"), "rctx3_") || input.ProviderTask == nil {
			t.Fatalf("OpenCode-driven collect input = %#v, want a sealed provider task", input)
		}
		lens := record.State.SelectedLenses[0]
		hostOutput := string(admittedReviewerPayloadForTest(t, repo, record, lens, 0))
		relay := startOpenCodeTransportRelay(t, initReviewCLIRepo(t), openCodeTransportEnvelope{
			Schema: openCodeReviewTransportSchema, Operation: "start", Prompt: input.ProviderTask.Prompt,
		})
		if _, err := relay.complete(openCodeTransportEnvelope{
			Schema: openCodeReviewTransportSchema, Operation: "complete", Nonce: relay.prompt.Nonce, Output: &hostOutput,
		}); err != nil {
			t.Fatal(err)
		}
		assertApprovedCompactAuthorityBurned(t, store, record.State.LineageID)
	})
}

// Every non-OpenCode runtime keeps the rctx2 digest everywhere STATUS renders
// a handle, and that digest is exactly the unchanged derivation.
func TestNonOpenCodeStatusKeepsTheUnchangedRctx2Digest(t *testing.T) {
	// Pi is admitted only through its relay handshake; declare it so the test
	// does not depend on running inside a Pi host.
	t.Setenv("GENTLE_PI_REVIEW_RELAY_CONTRACT", "gentle-pi.review-relay/v1")
	if testing.Short() {
		t.Skip("requires git fixtures")
	}
	reviewEnabledHome(t)
	for _, runtime := range []model.AgentID{model.AgentPi, model.AgentClaudeCode, model.AgentCodex} {
		t.Run(string(runtime), func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc kept() int { return 1 }\n", 0o644)
			started := runNegotiatedReviewStartWith(t, repo, "rctx2-kept-"+strings.ReplaceAll(string(runtime), "-", ""), "--agent", string(runtime))
			raw, status := openCodeSealedStatus(t, repo, started.LineageID, "")
			if bytes.Contains(raw, []byte("rctx3_")) {
				t.Fatalf("%s STATUS carried a sealed handle: %s", runtime, raw)
			}
			want, err := reviewtransaction.DeriveReviewRepositoryContextHandle(t.Context(), repo, reviewtransaction.ReviewRepositoryContextBinding{
				LineageID: started.LineageID, TargetIdentity: started.RepositoryContext.TargetIdentity, Revision: started.RepositoryContext.Revision,
			})
			if err != nil || status.RepositoryContext == nil || status.RepositoryContext.Handle != want || started.RepositoryContext.Handle != want {
				t.Fatalf("%s handles = START %q / STATUS %#v, want the rctx2 digest %q (%v)", runtime, started.RepositoryContext.Handle, status.RepositoryContext, want, err)
			}
			if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".gentle-ai", "review-context.key")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("%s lifecycle created the OpenCode sealing key: %v", runtime, err)
			}
		})
	}
}

type sealedCollectArguments struct {
	repo                                                  string
	record                                                reviewtransaction.CompactRecord
	context, lineage, target, revision, lens, subjectHash string
}

func sealedCollectArgumentsFrom(t *testing.T, repo string, record reviewtransaction.CompactRecord, input ReviewTransitionInput) sealedCollectArguments {
	t.Helper()
	if input.ArtifactSubject == nil {
		t.Fatalf("collect input carries no artifact subject: %#v", input)
	}
	return sealedCollectArguments{
		repo: repo, record: record,
		context: reviewTransitionArgumentValue(t, input, "repository-context"), lineage: reviewTransitionArgumentValue(t, input, "lineage"),
		target: reviewTransitionArgumentValue(t, input, "target"), revision: reviewTransitionArgumentValue(t, input, "expected-revision"),
		lens: reviewTransitionArgumentValue(t, input, "lens"), subjectHash: input.ArtifactSubject.SubjectHash,
	}
}

func openCodeSealedLineage(t *testing.T, lineage string) (string, ReviewIntegrationStartResult, reviewtransaction.CompactStore, reviewtransaction.CompactRecord) {
	t.Helper()
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc sealed() int { return 1 }\n", 0o644)
	started := runNegotiatedReviewStartWith(t, repo, lineage, "--agent", string(model.AgentOpenCode))
	store, record := loadOpenCodeRelayAuthority(t, repo, started.LineageID)
	if record.State.RuntimeAgent != string(model.AgentOpenCode) || len(record.State.SelectedLenses) == 0 {
		t.Fatalf("fixture froze runtime %q with lenses %v, want an OpenCode review", record.State.RuntimeAgent, record.State.SelectedLenses)
	}
	return repo, started, store, record
}

func openCodeSealedStatus(t *testing.T, repo, lineage string, runtime model.AgentID) ([]byte, ReviewTargetStatusResult) {
	t.Helper()
	args := []string{"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--lineage", lineage, "--next-transition"}
	if runtime != "" {
		args = append(args, "--agent", string(runtime))
	}
	var output bytes.Buffer
	if err := RunReview(args, &output); err != nil {
		t.Fatalf("STATUS %v: %v\n%s", args, err, output.String())
	}
	var status ReviewTargetStatusResult
	if err := json.Unmarshal(output.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if err := status.Validate(); err != nil {
		t.Fatalf("STATUS failed its own contract: %v\n%s", err, output.String())
	}
	return output.Bytes(), status
}

func openCodeSealedCollectInput(t *testing.T, status ReviewTargetStatusResult) ReviewTransitionInput {
	t.Helper()
	if status.NextTransition == nil || status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) == 0 {
		t.Fatalf("STATUS offered no collect input: %#v", status.NextTransition)
	}
	return status.NextTransition.Collect.Inputs[0]
}
