package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/reviewtransaction"
)

func openCodeV2ReservationTestShim(t *testing.T) string {
	t.Helper()
	if testing.Short() || runtime.GOOS == "windows" {
		t.Skip("requires git fixtures and the Unix relay subprocess shim")
	}
	reviewEnabledHome(t)
	stubOpenCodeReviewRuntimeVersion(t, "2.4.2", openCodeRelayContractV2)
	versionProbe := filepath.Join(t.TempDir(), "opencode-version")
	if err := os.WriteFile(versionProbe, []byte("#!/bin/sh\n[ \"$#\" -eq 1 ] && [ \"$1\" = --version ] || exit 1\nprintf '2.4.2\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return openCodeV2HostShim(t, openCodeV2HostInputs{host: versionProbe}, openCodeV2GateReal)
}

func assertOpenCodeV2ReservationRefusal(t *testing.T, relay *openCodeDuplicateRelayProcess, reason, stderr string) {
	t.Helper()
	// The relay must respond and exit with stdin still open. There is no
	// queued start or completion to send to this non-owner.
	got := relay.readFrame(t)
	want := openCodeTransportEnvelope{Schema: openCodeReviewTransportSchema, Operation: "refused", Error: reason}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("frame = %s/%s (prompt=%t nonce=%t output=%t), want exactly %s/%s", got.Operation, got.Error, got.Prompt != "", got.Nonce != "", got.Output != nil, want.Operation, want.Error)
	}
	if code := relay.wait(t); code != 1 || relay.stderr.String() != stderr+"\n" {
		t.Fatalf("refusal: exit=%d stderr=%q; want exit=1 stderr=%q", code, relay.stderr.String(), stderr+"\n")
	}
}

func TestOpenCodeV2DispatchReservationKeepsSiblingLensesIndependent(t *testing.T) {
	shim, host := openCodeV2ReservationTestShim(t), t.TempDir()
	repo, started, store, record := newArtifactReview(t, true)
	var output bytes.Buffer
	if err := RunReview([]string{"status", "--cwd", repo, "--lineage", started.LineageID, "--contract", ReviewIntegrationContractV2,
		"--agent", "opencode", "--next-transition"}, &output); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	inputs := status.NextTransition.Collect.Inputs
	if len(inputs) != 4 {
		t.Fatalf("selected inputs = %d, want four sibling lenses", len(inputs))
	}
	starts := make([]openCodeTransportEnvelope, len(inputs))
	for index, input := range inputs {
		starts[index] = openCodeTransportEnvelope{Schema: openCodeReviewTransportSchema, Operation: "start", Agent: input.ProviderTask.Agent, Prompt: input.ProviderTask.Prompt}
	}
	start := func(frame openCodeTransportEnvelope) (*openCodeDuplicateRelayProcess, openCodeTransportEnvelope) {
		t.Helper()
		relay := startOpenCodeDuplicateRelayProcess(t, shim, host, frame)
		prompt := relay.readFrame(t)
		if !openCodeDuplicateDispatchable(prompt) {
			t.Fatalf("expected a dispatchable prompt for %s, got %s/%s", frame.Agent, prompt.Operation, prompt.Error)
		}
		return relay, prompt
	}
	complete := func(relay *openCodeDuplicateRelayProcess, prompt openCodeTransportEnvelope, raw string) string {
		t.Helper()
		relay.send(t, openCodeTransportEnvelope{Schema: openCodeReviewTransportSchema, Operation: "complete", Nonce: prompt.Nonce, Output: &raw})
		relay.closeInput(t)
		result := relay.readFrame(t)
		if code := relay.wait(t); code != 0 || result.Operation != "result" || result.Output == nil || relay.stderr.Len() != 0 {
			t.Fatalf("completion: exit=%d operation=%q stderr=%q", code, result.Operation, relay.stderr.String())
		}
		return *result.Output
	}
	payload := func(order int) string {
		return string(admittedReviewerPayloadForTest(t, repo, record, starts[order].Agent, order))
	}
	publicStatus := func() []byte {
		return frozenCollectEnvelope(t, explicitFrozenReviewingStatus(t, repo, started.LineageID))
	}
	before := publicStatus()
	first, firstPrompt := start(starts[0])
	second, secondPrompt := start(starts[1])
	// An exact Go-issued prompt keeps the existing non-capturing pass-through;
	// a host-authored copy or padding still addresses the original slot.
	reintercepted := starts[0]
	reintercepted.Prompt = firstPrompt.Prompt
	pass, prompt := start(reintercepted)
	if prompt.Prompt != firstPrompt.Prompt || complete(pass, prompt, payload(0)) != payload(0) || !bytes.Equal(publicStatus(), before) {
		t.Fatal("re-interception did not pass through exact bytes without capture")
	}
	assertBusy := func(start openCodeTransportEnvelope) {
		t.Helper()
		assertOpenCodeV2ReservationRefusal(t, startOpenCodeDuplicateRelayProcess(t, shim, host, start),
			"lens_already_in_progress", "opencode_review_transport_lens_already_in_progress: this lens slot is reserved; the previous dispatch outcome may be unresolved")
	}
	changedHostPrompt := reintercepted
	changedHostPrompt.Prompt += "\nnot a byte-exact replay"
	assertBusy(changedHostPrompt)
	if !bytes.Equal(publicStatus(), before) {
		t.Fatal("dispatches or refusal changed public STATUS before a capture")
	}
	var artifact reviewResultArtifact
	decodeStrictReviewJSON(t, []byte(complete(second, secondPrompt, payload(1))), &artifact)
	afterSibling := publicStatus()
	if artifact.Lens != starts[1].Agent || bytes.Equal(afterSibling, before) {
		t.Fatal("sibling capture did not commit its own slot")
	}
	assertBusy(starts[0])
	if !bytes.Equal(publicStatus(), afterSibling) {
		t.Fatal("same slot became dispatchable after a sibling changed the record revision")
	}
	assertOpenCodeV2ReservationRefusal(t, startOpenCodeDuplicateRelayProcess(t, shim, host, starts[1]),
		"stale_authority", "opencode_review_transport_binding_invalid: Task does not match an uncollected lens slot")
	complete(first, firstPrompt, payload(0))
	third, thirdPrompt := start(starts[2])
	complete(third, thirdPrompt, payload(2))
	last, lastPrompt := start(starts[3])
	var closure reviewLastEventClosureResult
	decodeStrictReviewJSON(t, []byte(complete(last, lastPrompt, payload(3))), &closure)
	if closure.State != reviewtransaction.StateApproved || closure.Acknowledgement == nil {
		t.Fatalf("terminal closure = %#v", closure)
	}
	terminal := explicitFrozenReviewingStatus(t, repo, started.LineageID)
	if terminal.NextTransition == nil || terminal.NextTransition.Collect != nil {
		t.Fatal("committed terminal STATUS did not stop lens collection")
	}
	assertApprovedCompactAuthorityBurned(t, store, started.LineageID)
}

func TestOpenCodeV2DispatchReservationDoesNotInferHostStop(t *testing.T) {
	shim, host := openCodeV2ReservationTestShim(t), t.TempDir()
	for _, outcome := range []string{"abrupt_exit", "ambiguous_error", "unadmitted_output"} {
		t.Run(outcome, func(t *testing.T) {
			repo, started, store, record := newArtifactReview(t, false)
			task := openCodeV2StatusProviderTask(t, repo, started.LineageID)
			start := openCodeTransportEnvelope{Schema: openCodeReviewTransportSchema, Operation: "start", Agent: task.Agent, Prompt: task.Prompt}
			status := func() []byte {
				return frozenCollectEnvelope(t, explicitFrozenReviewingStatus(t, repo, started.LineageID))
			}
			before := status()
			relay := startOpenCodeDuplicateRelayProcess(t, shim, host, start)
			prompt := relay.readFrame(t)
			if !openCodeDuplicateDispatchable(prompt) {
				t.Fatal("fresh canonical slot did not dispatch")
			}
			if outcome == "abrupt_exit" {
				if err := relay.cmd.Process.Kill(); err != nil {
					t.Fatal(err)
				}
				if code := relay.wait(t); code != -1 || relay.stderr.Len() != 0 {
					t.Fatalf("killed relay: exit=%d stderr=%q", code, relay.stderr.String())
				}
			} else {
				completion := openCodeTransportEnvelope{Schema: openCodeReviewTransportSchema, Operation: "complete", Nonce: prompt.Nonce}
				reason := "provider_failed"
				stderr := "opencode_task_transport_failed: OpenCode Task transport did not produce a capturable reviewer result; run `gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --next-transition` before retrying"
				if outcome == "ambiguous_error" {
					completion.Error = "host-stopped" // Arbitrary completion.error text proves nothing.
				} else {
					bad := "<task id=\"child\" state=\"completed\">\n<task_result>\n\n</task_result>\n</task>"
					completion.Output = &bad
					reason, stderr = "output_refused", "opencode_task_output_empty: the reviewer Task completed without producing a result"
				}
				relay.send(t, completion)
				relay.closeInput(t)
				assertOpenCodeV2ReservationRefusal(t, relay, reason, stderr)
			}
			if !bytes.Equal(status(), before) {
				t.Fatal("uncertain outcome changed public STATUS")
			}
			assertOpenCodeV2ReservationRefusal(t, startOpenCodeDuplicateRelayProcess(t, shim, host, start),
				"lens_already_in_progress", "opencode_review_transport_lens_already_in_progress: this lens slot is reserved; the previous dispatch outcome may be unresolved")
			if !bytes.Equal(status(), before) {
				t.Fatal("duplicate changed public STATUS")
			}
			if outcome == "ambiguous_error" {
				// The reservation gates dispatch, not the existing native capture.
				captureFrozenReviewerResults(t, repo, record, 1)
				if bytes.Equal(status(), before) {
					t.Fatal("bound native capture did not advance public STATUS")
				}
				assertApprovedCompactAuthorityBurned(t, store, started.LineageID)
			}
		})
	}
}

func TestOpenCodeV2DispatchReservationRefusesLinkedStorage(t *testing.T) {
	shim, host := openCodeV2ReservationTestShim(t), t.TempDir()
	repo, started, store, _ := newArtifactReview(t, false)
	task := openCodeV2StatusProviderTask(t, repo, started.LineageID)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(store.Dir, "opencode-dispatch")); err != nil {
		t.Fatal(err)
	}
	before := frozenCollectEnvelope(t, explicitFrozenReviewingStatus(t, repo, started.LineageID))
	start := openCodeTransportEnvelope{Schema: openCodeReviewTransportSchema, Operation: "start", Agent: task.Agent, Prompt: task.Prompt}
	assertOpenCodeV2ReservationRefusal(t, startOpenCodeDuplicateRelayProcess(t, shim, host, start),
		"relay_unavailable", "opencode_review_transport_reservation_unavailable: could not safely reserve the lens slot")
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("external directory changed: %d entries, err=%v", len(entries), err)
	}
	if !bytes.Equal(frozenCollectEnvelope(t, explicitFrozenReviewingStatus(t, repo, started.LineageID)), before) {
		t.Fatal("linked storage refusal changed public STATUS")
	}
}
