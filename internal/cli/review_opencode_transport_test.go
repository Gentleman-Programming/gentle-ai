package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestOpenCodeReviewTransportRelaysOneLiveTaskAndCapturesHostOutput(t *testing.T) {
	repo, _, store, record := newArtifactReview(t, false)
	lens := record.State.SelectedLenses[0]
	relay := startOpenCodeTransportRelay(t, openCodeLensTransportStart(t, repo, record, lens))
	if !strings.HasPrefix(relay.prompt.Prompt, reviewLensContextBindingHeader+" ") {
		t.Fatalf("relay prompt = %q", relay.prompt.Prompt)
	}
	raw := admittedReviewerPayloadForTest(t, repo, record, lens, 0)
	hostOutput := `<task id="call-opaque" state="completed">
<task_result>
` + string(raw) + `
</task_result>
</task>`
	completed, err := relay.complete(openCodeTransportEnvelope{
		Schema: openCodeReviewTransportSchema, Operation: "complete", Nonce: relay.prompt.Nonce, Output: &hostOutput,
	})
	if err != nil {
		t.Fatal(err)
	}
	var artifact reviewResultArtifact
	decodeStrictReviewJSON(t, []byte(*completed.Output), &artifact)
	if artifact.AdmissionDecision != reviewtransaction.ArtifactAdmissionCompleted || artifact.Reference == "" || artifact.Path != "" {
		t.Fatalf("transport artifact = %#v", artifact)
	}
	if _, found := reviewtransaction.ReadLensContextEmission(store.Dir, record.State.LineageID, record.State.InitialSnapshot.Identity,
		record.Revision, lens, 0, artifact.SubjectHash); !found {
		t.Fatal("provider-contract context emission was not recorded after live Go relay capture")
	}
	if _, found, err := store.ResolveAdmittedReviewerResult(context.Background(), record.Revision, record.State.InitialSnapshot.Identity,
		mustFrozenContext(t, repo, record), mustArtifactSubject(t, repo, record, lens, 0)); err != nil || !found {
		t.Fatalf("captured provider result found=%v err=%v", found, err)
	}
}

func TestOpenCodeReviewTransportPublishesProvenanceOnlyAfterDurableCapture(t *testing.T) {
	for _, test := range []struct {
		name           string
		beforeComplete func(*testing.T, string, reviewtransaction.CompactStore, reviewtransaction.CompactRecord, string)
		wantSlot       bool
	}{
		{
			name: "capture conflict leaves neither provenance nor frame",
			beforeComplete: func(t *testing.T, repo string, store reviewtransaction.CompactStore, record reviewtransaction.CompactRecord, lens string) {
				t.Helper()
				raw := admittedReviewerPayloadForTest(t, repo, record, lens, 0)
				admitted, err := reviewProviderAdmitRaw(t.Context(), repo, record.State, record.Revision,
					mustFrozenContext(t, repo, record), mustArtifactSubject(t, repo, record, lens, 0), append([]byte("transport prose\n"), raw...))
				if err != nil {
					t.Fatal(err)
				}
				if _, err := store.CaptureAdmittedReviewerResult(t.Context(), reviewtransaction.CompactAdmittedReviewerResultRequest{
					ExpectedRevision: record.Revision, TargetIdentity: record.State.InitialSnapshot.Identity, FrozenContext: admitted.Frozen,
					ArtifactSubject: admitted.Subject, Inspection: admitted.Result.Inspection, Result: admitted.NativeResult,
					CandidateCausalFindingIDs: admitted.CandidateCausalFindingIDs, RawPayload: append([]byte("transport prose\n"), raw...),
				}); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "provenance failure preserves durable slot without frame",
			beforeComplete: func(t *testing.T, repo string, store reviewtransaction.CompactStore, record reviewtransaction.CompactRecord, lens string) {
				t.Helper()
				if err := reviewtransaction.PublishLensContextEmission(store.Dir, reviewtransaction.LensContextEmission{
					Schema: reviewtransaction.LensContextEmissionSchema, LineageID: record.State.LineageID,
					TargetIdentity: record.State.InitialSnapshot.Identity, AuthorityRevision: record.Revision,
					Lens: lens, SelectedOrder: 0, SubjectHash: mustArtifactSubject(t, repo, record, lens, 0).SubjectHash,
					Level: reviewtransaction.ReviewerContextLevelProviderCommand,
				}); err != nil {
					t.Fatal(err)
				}
			},
			wantSlot: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo, _, store, record := newArtifactReview(t, false)
			lens := record.State.SelectedLenses[0]
			relay := startOpenCodeTransportRelay(t, openCodeLensTransportStart(t, repo, record, lens))
			if test.beforeComplete != nil {
				test.beforeComplete(t, repo, store, record, lens)
			}
			raw := admittedReviewerPayloadForTest(t, repo, record, lens, 0)
			hostOutput := string(raw)
			if _, err := relay.complete(openCodeTransportEnvelope{
				Schema: openCodeReviewTransportSchema, Operation: "complete", Nonce: relay.prompt.Nonce, Output: &hostOutput,
			}); err == nil {
				t.Fatal("relay completion unexpectedly emitted a result frame")
			}

			subject := mustArtifactSubject(t, repo, record, lens, 0)
			emission, found := reviewtransaction.ReadLensContextEmission(store.Dir, record.State.LineageID,
				record.State.InitialSnapshot.Identity, record.Revision, lens, 0, subject.SubjectHash)
			if test.wantSlot {
				if !found || emission.Level != reviewtransaction.ReviewerContextLevelProviderCommand {
					t.Fatalf("provenance conflict was not preserved exactly: %#v found=%v", emission, found)
				}
				if _, resolved, err := store.ResolveAdmittedReviewerResult(context.Background(), record.Revision,
					record.State.InitialSnapshot.Identity, mustFrozenContext(t, repo, record), subject); err != nil || !resolved {
					t.Fatalf("durable capture did not survive provenance failure: resolved=%v err=%v", resolved, err)
				}
				return
			}
			if found {
				if emission.Level == reviewtransaction.ReviewerContextLevelProviderContract {
					t.Fatalf("failed capture recorded provider-contract provenance: %#v", emission)
				}
				return
			}
			if _, resolved, err := store.ResolveAdmittedReviewerResult(context.Background(), record.Revision,
				record.State.InitialSnapshot.Identity, mustFrozenContext(t, repo, record), subject); err != nil || !resolved {
				t.Fatalf("pre-existing conflicting slot was not preserved: resolved=%v err=%v", resolved, err)
			}
		})
	}
}

func TestOpenCodeReviewTransportRefusesStandaloneCompletionWithoutAuthorityMutation(t *testing.T) {
	repo, _, store, record := newArtifactReview(t, false)
	lens := record.State.SelectedLenses[0]
	raw := admittedReviewerPayloadForTest(t, repo, record, lens, 0)
	hostOutput := string(raw)
	request, err := json.Marshal(openCodeTransportEnvelope{
		Schema: openCodeReviewTransportSchema, Operation: "complete", Nonce: strings.Repeat("0", 32), Output: &hostOutput,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runReviewOpenCodeTransport(nil, bytes.NewReader(request), io.Discard); err == nil || !strings.Contains(err.Error(), "relay start") {
		t.Fatalf("standalone completion error = %v", err)
	}
	subject := mustArtifactSubject(t, repo, record, lens, 0)
	if _, found := reviewtransaction.ReadLensContextEmission(store.Dir, record.State.LineageID, record.State.InitialSnapshot.Identity,
		record.Revision, lens, 0, subject.SubjectHash); found {
		t.Fatal("standalone completion recorded provider-contract provenance")
	}
	if _, found, err := store.ResolveAdmittedReviewerResult(context.Background(), record.Revision, record.State.InitialSnapshot.Identity,
		mustFrozenContext(t, repo, record), subject); err != nil || found {
		t.Fatalf("standalone completion captured a result: found=%v err=%v", found, err)
	}
}

func TestOpenCodeReviewTransportSessionFailuresDoNotMutateAuthority(t *testing.T) {
	originalTimeout := openCodeTransportCompletionTimeout
	t.Cleanup(func() { openCodeTransportCompletionTimeout = originalTimeout })
	openCodeTransportCompletionTimeout = 20 * time.Millisecond
	for _, test := range []struct {
		name       string
		completion func(openCodeTransportPrompt) openCodeTransportEnvelope
		extra      *openCodeTransportEnvelope
		want       string
	}{
		{name: "missing completion", completion: func(openCodeTransportPrompt) openCodeTransportEnvelope { return openCodeTransportEnvelope{} }, want: "completion_missing"},
		{name: "completion timeout", completion: func(openCodeTransportPrompt) openCodeTransportEnvelope { return openCodeTransportEnvelope{} }, want: "completion_timeout"},
		{name: "wrong call correlation", completion: func(openCodeTransportPrompt) openCodeTransportEnvelope {
			output := "{}"
			return openCodeTransportEnvelope{Schema: openCodeReviewTransportSchema, Operation: "complete", Nonce: strings.Repeat("f", 32), Output: &output}
		}, want: "relay completion"},
		{name: "malformed host frame", completion: func(prompt openCodeTransportPrompt) openCodeTransportEnvelope {
			output := "<task"
			return openCodeTransportEnvelope{Schema: openCodeReviewTransportSchema, Operation: "complete", Nonce: prompt.Nonce, Output: &output}
		}, want: "opencode_task_output_malformed"},
		{name: "duplicate completion", completion: func(prompt openCodeTransportPrompt) openCodeTransportEnvelope {
			output := "{}"
			return openCodeTransportEnvelope{Schema: openCodeReviewTransportSchema, Operation: "complete", Nonce: prompt.Nonce, Output: &output}
		}, extra: &openCodeTransportEnvelope{Schema: openCodeReviewTransportSchema, Operation: "complete", Nonce: strings.Repeat("0", 32), Output: stringPointer("{}")}, want: "exactly one start frame and one completion frame"},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo, _, store, record := newArtifactReview(t, false)
			lens := record.State.SelectedLenses[0]
			relay := startOpenCodeTransportRelay(t, openCodeLensTransportStart(t, repo, record, lens))
			var err error
			if test.name == "missing completion" {
				err = relay.closeWithoutCompletion()
			} else if test.name == "completion timeout" {
				err = relay.timeoutWithoutCompletion()
			} else {
				_, err = relay.complete(test.completion(relay.prompt), test.extra)
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("relay error = %v, want %q", err, test.want)
			}
			assertOpenCodeRelayLensUncaptured(t, repo, store, record, lens)
		})
	}
}

func TestOpenCodeReviewTransportTimedOutReadDoesNotLeaveAGoroutine(t *testing.T) {
	reader, writer := io.Pipe()
	decoder := json.NewDecoder(reader)
	before := runtime.NumGoroutine()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, err := decodeOpenCodeTransportEnvelopeContext(ctx, decoder); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked transport read error = %v, want deadline exceeded", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := runtime.NumGoroutine(); got > before {
		t.Fatalf("blocked transport read leaked goroutine: before=%d after=%d", before, got)
	}
}

func TestOpenCodeReviewTransportCapturesProviderRefuter(t *testing.T) {
	repo, started, store, record := newArtifactReview(t, false)
	reviewer := admittedReviewerResultForTest(t, repo, record, record.State.SelectedLenses[0], 0)
	reviewer.Findings = []facadeFinding{{
		ID: "R3-001", Location: "tracked.txt:1", Severity: "CRITICAL", Claim: "candidate regression",
		ProofRefs: []string{"candidate trace"}, EvidenceClass: reviewtransaction.EvidenceInferential,
		CausalDisposition: reviewtransaction.CausalIntroduced,
	}}
	path := filepath.Join(t.TempDir(), "reviewer.json")
	writeReviewCLIJSON(t, path, reviewer)
	if err := RunReviewCaptureResult([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", path,
	}, io.Discard); err != nil {
		t.Fatal(err)
	}
	contextHandle, err := reviewtransaction.PublishReviewRepositoryContext(t.Context(), repo, reviewtransaction.ReviewRepositoryContextBinding{
		LineageID: record.State.LineageID, TargetIdentity: record.State.InitialSnapshot.Identity, Revision: record.Revision,
	})
	if err != nil {
		t.Fatal(err)
	}
	task, err := newReviewProviderTask(reviewerprovider.RoleRefuter, ReviewTransitionBinding{
		LineageID: record.State.LineageID, Revision: record.Revision, TargetIdentity: record.State.InitialSnapshot.Identity, RepositoryContext: contextHandle,
	})
	if err != nil {
		t.Fatal(err)
	}
	relay := startOpenCodeTransportRelay(t, openCodeTransportEnvelope{Schema: openCodeReviewTransportSchema, Operation: "start", Prompt: task.Prompt})
	request, err := reviewProviderNewRefuterRequest(t.Context(), repo, store.Dir, record.State, record.Revision)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(facadeRefuterResult{RequestHash: request.RequestHash, Results: []facadeRefuterOutcome{{
		FindingID: "R3-001", Outcome: reviewtransaction.OutcomeCorroborated, ProofRefs: []string{"independent reproduction"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	hostOutput := string(raw)
	completed, err := relay.complete(openCodeTransportEnvelope{
		Schema: openCodeReviewTransportSchema, Operation: "complete", Nonce: relay.prompt.Nonce, Output: &hostOutput,
	})
	if err != nil || completed.Output == nil || !strings.Contains(*completed.Output, `"captured":true`) {
		t.Fatalf("provider refuter completion = %#v, %v", completed, err)
	}
	slot, err := reviewtransaction.ReadCompactRefuterResultSlot(store.Dir)
	if err != nil || !slot.Occupied {
		t.Fatalf("provider refuter slot = %#v, %v", slot, err)
	}
}

func TestOpenCodeTaskHostOutputPreservesPayloadBytesAndFailsClosed(t *testing.T) {
	payload := "  {\n\t\"findings\": []\n}  "
	tests := []struct {
		name string
		raw  string
		want string
		code string
	}{
		{name: "bare host output", raw: payload, want: payload},
		{name: "completed task envelope", raw: "<task id=\"opaque\" state=\"completed\">\n<task_result>\n" + payload + "\n</task_result>\n</task>", want: payload},
		{name: "short task prefix", raw: "<task", code: "opencode_task_output_malformed"},
		{name: "unterminated task", raw: "<task id=\"opaque\" state=\"completed\">\n<task_result>\n{", code: "opencode_task_output_truncated"},
		{name: "nested task", raw: "<task id=\"opaque\" state=\"completed\">\n<task_result>\n<task id=\"nested\" state=\"completed\">\n</task>\n</task_result>\n</task>", code: "opencode_task_output_malformed"},
		{name: "host limit", raw: strings.Repeat("x", openCodeTaskHostOutputLimit+1), code: "opencode_task_output_truncated"},
		{name: "native provider limit", raw: strings.Repeat("x", reviewResultArtifactLimit+1), code: "opencode_task_output_truncated"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := decodeOpenCodeTaskHostOutput([]byte(test.raw))
			if test.code != "" {
				if err == nil || !strings.Contains(err.Error(), test.code) {
					t.Fatalf("decode error = %v, want %s", err, test.code)
				}
				return
			}
			if err != nil || string(got) != test.want {
				t.Fatalf("decoded = %q, %v; want %q", got, err, test.want)
			}
		})
	}
}

type openCodeTransportPrompt struct {
	Nonce  string
	Prompt string
}

type openCodeTransportRelay struct {
	input  *io.PipeWriter
	output *json.Decoder
	done   <-chan error
	prompt openCodeTransportPrompt
}

func startOpenCodeTransportRelay(t *testing.T, start openCodeTransportEnvelope) openCodeTransportRelay {
	t.Helper()
	inputReader, input := io.Pipe()
	outputReader, output := io.Pipe()
	done := make(chan error, 1)
	go func() {
		err := runReviewOpenCodeTransport(nil, inputReader, output)
		_ = output.CloseWithError(err)
		done <- err
	}()
	if err := json.NewEncoder(input).Encode(start); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bufio.NewReader(outputReader))
	var prompt openCodeTransportEnvelope
	if err := decoder.Decode(&prompt); err != nil {
		t.Fatal(err)
	}
	if prompt.Schema != openCodeReviewTransportSchema || prompt.Operation != "prompt" || prompt.Nonce == "" || prompt.Prompt == "" {
		t.Fatalf("relay prompt frame = %#v", prompt)
	}
	return openCodeTransportRelay{input: input, output: decoder, done: done, prompt: openCodeTransportPrompt{Nonce: prompt.Nonce, Prompt: prompt.Prompt}}
}

func (relay openCodeTransportRelay) complete(completion openCodeTransportEnvelope, extra ...*openCodeTransportEnvelope) (openCodeTransportEnvelope, error) {
	if err := json.NewEncoder(relay.input).Encode(completion); err != nil {
		return openCodeTransportEnvelope{}, err
	}
	for _, frame := range extra {
		if frame != nil {
			if err := json.NewEncoder(relay.input).Encode(*frame); err != nil {
				return openCodeTransportEnvelope{}, err
			}
		}
	}
	if err := relay.input.Close(); err != nil {
		return openCodeTransportEnvelope{}, err
	}
	var response openCodeTransportEnvelope
	decodeErr := relay.output.Decode(&response)
	err := <-relay.done
	if err != nil {
		return openCodeTransportEnvelope{}, err
	}
	if decodeErr != nil {
		return openCodeTransportEnvelope{}, decodeErr
	}
	return response, nil
}

func (relay openCodeTransportRelay) closeWithoutCompletion() error {
	if err := relay.input.Close(); err != nil {
		return err
	}
	return <-relay.done
}

func (relay openCodeTransportRelay) timeoutWithoutCompletion() error {
	err := <-relay.done
	_ = relay.input.Close()
	return err
}

func openCodeLensTransportStart(t *testing.T, repo string, record reviewtransaction.CompactRecord, lens string) openCodeTransportEnvelope {
	t.Helper()
	contextHandle, err := reviewtransaction.PublishReviewRepositoryContext(context.Background(), repo, reviewtransaction.ReviewRepositoryContextBinding{
		LineageID: record.State.LineageID, TargetIdentity: record.State.InitialSnapshot.Identity, Revision: record.Revision,
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := json.Marshal(reviewLensContextBinding{
		Lineage: record.State.LineageID, Target: record.State.InitialSnapshot.Identity, Lens: lens, Order: 0,
		Revision: record.Revision, RepositoryContext: contextHandle,
	})
	if err != nil {
		t.Fatal(err)
	}
	return openCodeTransportEnvelope{
		Schema: openCodeReviewTransportSchema, Operation: "start", Prompt: reviewLensContextBindingHeader + " " + string(binding),
	}
}

func assertOpenCodeRelayLensUncaptured(t *testing.T, repo string, store reviewtransaction.CompactStore, record reviewtransaction.CompactRecord, lens string) {
	t.Helper()
	subject := mustArtifactSubject(t, repo, record, lens, 0)
	if _, found := reviewtransaction.ReadLensContextEmission(store.Dir, record.State.LineageID, record.State.InitialSnapshot.Identity,
		record.Revision, lens, 0, subject.SubjectHash); found {
		t.Fatal("failed relay recorded provider-contract provenance")
	}
	if _, found, err := store.ResolveAdmittedReviewerResult(context.Background(), record.Revision, record.State.InitialSnapshot.Identity,
		mustFrozenContext(t, repo, record), subject); err != nil || found {
		t.Fatalf("failed relay captured a result: found=%v err=%v", found, err)
	}
}

func stringPointer(value string) *string { return &value }

func mustFrozenContext(t *testing.T, repo string, record reviewtransaction.CompactRecord) reviewtransaction.FrozenCandidateContext {
	t.Helper()
	frozen, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).FrozenCandidateContext(context.Background(), record.State.InitialSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	return frozen
}

func mustArtifactSubject(t *testing.T, repo string, record reviewtransaction.CompactRecord, lens string, order int) reviewtransaction.ArtifactSubject {
	t.Helper()
	subject, err := reviewtransaction.NewArtifactSubject(record.State, record.Revision, mustFrozenContext(t, repo, record), lens, order, "")
	if err != nil {
		t.Fatal(err)
	}
	return subject
}
