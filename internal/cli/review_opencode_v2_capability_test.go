package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/model"
	"github.com/gentleman-programming/gentle-ai/v5/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v5/internal/reviewtransaction"
)

// OpenCode immutable review is admitted only for a matching pair: a V1 runtime
// with no relay declaration (the V1 plugin declares nothing), or a V2 runtime
// whose managed plugin declares exactly the V2 relay contract. Every other
// combination refuses before the relay reads any input.
func TestOpenCodeTransportCapabilityRequiresMatchingRuntimeAndDeclaration(t *testing.T) {
	old := opencode.VersionRunnerOverride
	t.Cleanup(func() { opencode.VersionRunnerOverride = old })
	for _, test := range []struct {
		name, declaration, version string
		admitted, probes           bool
	}{
		{"V1 runtime without declaration", "", "1.18.30", true, true},
		{"V2 runtime with the exact V2 declaration", openCodeRelayContractV2, "opencode v2.0.19", true, true},
		{"V2 runtime just below the excluded release", openCodeRelayContractV2, "2.4.0", true, true},
		{"V2 runtime at the excluded release", openCodeRelayContractV2, "2.4.1", false, true},
		{"V2 runtime just above the excluded release", openCodeRelayContractV2, "2.4.2", true, true},
		{"V2 runtime without declaration", "", "2.0.4", false, true},
		{"unknown runtime without declaration", "", "unknown", false, true},
		{"V1 runtime with the V2 declaration", openCodeRelayContractV2, "1.18.30", false, true},
		{"unknown runtime with the V2 declaration", openCodeRelayContractV2, "unknown", false, true},
		{"V2 runtime with another declaration", "gentle-ai.opencode-relay/v3", "2.0.19", false, false},
		{"V2 runtime with a padded declaration", openCodeRelayContractV2 + " ", "2.0.19", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(openCodeRelayContractEnvironment, test.declaration)
			freshOpenCodeRuntimeProbe(t)
			probed := false
			opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
				probed = true
				if !test.probes {
					t.Error("an unrecognized declaration must refuse before the PATH probe")
				}
				return opencode.CommandOutput{Stdout: []byte(test.version)}, nil
			}
			if got := reviewImmutableRuntimeCapability(model.AgentOpenCode).supportsImmutableReceiptReview(); got != test.admitted {
				t.Fatalf("capability admitted = %v, want %v", got, test.admitted)
			}
			if probed != test.probes {
				t.Fatalf("runtime probed = %v, want %v", probed, test.probes)
			}
			if !test.admitted {
				if err := runReviewOpenCodeTransport(nil, v2UnreadableInput{}, io.Discard); err == nil || !strings.Contains(err.Error(), reviewImmutableTransportUnsupportedCode) {
					t.Fatalf("direct relay did not refuse before input: %v", err)
				}
			}
		})
	}
}

func TestOpenCode241RefusesBeforeReviewDispatch(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
	probes := stubOpenCodeRuntimeProbeSequence(t, "  OpenCode v2.4.1\n", "2.4.2")
	t.Setenv(openCodeRelayContractEnvironment, openCodeRelayContractV2)
	t.Setenv(reviewPiHostRelayContractEnvironment, "")
	const wantCause = "OpenCode 2.4.1 does not return completed foreground reviewer Tasks for immutable receipt review. Switch to a compatible OpenCode release (for example, 2.0.19 with the managed V2 relay plugin) and rerun."
	const wantMessage = "The active runtime cannot provide immutable receipt-review transport; to keep working without receipt review here, run `gentle-ai review mode disable --scope clone --cwd <repo>`."
	const wantCode = "immutable_review_transport_unsupported"

	statusArgs := []string{"status", "--contract", ReviewIntegrationContractV2, "--cwd", repo}
	for _, invocation := range []struct {
		name, operation string
		args            []string
	}{
		{"status", "review.status", append(statusArgs, "--agent", "opencode")},
		{"status with transition", "review.status", append(statusArgs, "--agent", "opencode", "--next-transition")},
		{"start", "review.start", boundNegotiatedStartArgs(t, []string{"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--agent", "opencode"})},
	} {
		t.Run(invocation.name, func(t *testing.T) {
			var output bytes.Buffer
			err := RunReview(invocation.args, &output)
			if err == nil || err.Error() != wantMessage+" ["+wantCode+"]" {
				t.Fatalf("command error = %v\nstdout: %s", err, output.String())
			}
			failure := decodeReviewIntegrationFailure(t, output.Bytes())
			if failure.Operation != invocation.operation || failure.Code != wantCode || failure.Message != wantMessage || failure.Cause != wantCause ||
				failure.MutationOutcome != ReviewMutationNotStarted || failure.AuthorityApplicability != "not_evaluated" ||
				failure.Replayability != reviewtransaction.ReplayabilityNotReplayable || failure.RetrySafe || failure.NextAction != "stop" {
				t.Fatalf("unexpected public refusal: %#v", failure)
			}
		})
	}
	t.Run("direct relay", func(t *testing.T) {
		var output bytes.Buffer
		err := runReviewOpenCodeTransport(nil, strings.NewReader(""), &output)
		if err == nil || err.Error() != wantCode+": "+wantCause {
			t.Fatalf("relay error = %v", err)
		}
		const wantFrame = `{"schema":"gentle-ai.provider-transport/v1","operation":"refused","error":"capability_unavailable"}` + "\n"
		if output.String() != wantFrame {
			t.Fatalf("relay stdout = %q, want %q", output.String(), wantFrame)
		}
	})
	if *probes != 1 {
		t.Fatalf("dispatch gates ran %d probes, want 1", *probes)
	}

	var output bytes.Buffer
	if err := RunReview(append(statusArgs, "--next-transition"), &output); err != nil {
		t.Fatalf("manual STATUS: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if status.Authority != nil || status.Repair.Counts != (reviewtransaction.AuthorityRepairCounts{}) ||
		status.NextTransition == nil || status.NextTransition.ReasonCode != "fresh_target_ready" {
		t.Fatalf("refused operations changed the fresh target: %s", output.String())
	}
}

func TestOpenCode241KeepsBoundManualRecovery(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, "")
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
	started := startFacadeReviewForRuntime(t, repo, model.AgentOpenCode)
	stubOpenCodeRuntimeProbeSequence(t, "2.4.1")
	t.Setenv(openCodeRelayContractEnvironment, openCodeRelayContractV2)

	transition, encoded := frozenRuntimeStatusTransition(t, repo, started.LineageID)
	if transition.Collect == nil || len(transition.Collect.Inputs) == 0 || transition.Collect.Inputs[0].ProviderTask != nil {
		t.Fatalf("bound manual STATUS must not offer an incompatible provider Task: %s", encoded)
	}
	captureCLIReviewerResult(t, repo, started, 0)
}

func TestOpenCode241RefusesPendingBoundRoleDispatch(t *testing.T) {
	for _, test := range []struct {
		name, reason, agent, operation string
		state                          reviewtransaction.State
	}{
		{"refuter", "provider_refuter_required", "review-refuter", "review.capture-refuter", reviewtransaction.StateCorrectionRequired},
		{"targeted-validator", "targeted_validation_required", "review-validator", "review/capture-validation", reviewtransaction.StateApproved},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(reviewPiHostRelayContractEnvironment, "")
			stubOpenCodeRuntimeProbeSequence(t, "2.0.19")
			t.Setenv(openCodeRelayContractEnvironment, openCodeRelayContractV2)
			var repo, lineage string
			var request reviewtransaction.TargetedValidationRequest
			if test.name == "refuter" {
				repo, lineage = frozenRuntimeReview(t, model.AgentOpenCode, true)
			} else {
				reviewEnabledHome(t)
				repo, lineage, request = providerCorrectionReadyWithoutVerificationEvidence(t, "--agent", "opencode")
			}

			statusArgs := []string{"status", "--cwd", repo, "--lineage", lineage, "--contract", ReviewIntegrationContractV2}
			plainStatus := func() []byte {
				var output bytes.Buffer
				if err := RunReview(statusArgs, &output); err != nil {
					t.Fatalf("plain STATUS: %v\n%s", err, output.String())
				}
				return output.Bytes()
			}
			boundTask := func() (ReviewProviderTask, map[string]string) {
				transition, encoded := frozenRuntimeStatusTransition(t, repo, lineage)
				if transition.ReasonCode != test.reason || transition.Collect == nil || len(transition.Collect.Inputs) != 1 {
					t.Fatalf("compatible STATUS = %s, want %s collection", encoded, test.reason)
				}
				input := transition.Collect.Inputs[0]
				if input.ProviderTask == nil || input.ProviderTask.Role != test.name || input.ProviderTask.Agent != test.agent {
					t.Fatalf("compatible STATUS = %s, want its bound %s Task", encoded, test.name)
				}
				args, err := reviewTransitionArgumentMap(input.Arguments)
				if err != nil || args["lineage"] != lineage || args["target"] == "" || args["expected-revision"] == "" {
					t.Fatalf("Task binding = %v, %v", args, err)
				}
				return *input.ProviderTask, args
			}
			before := plainStatus()
			_, binding := boundTask()

			stubOpenCodeRuntimeProbeSequence(t, "2.4.1")
			t.Setenv(openCodeRelayContractEnvironment, openCodeRelayContractV2)
			var refused bytes.Buffer
			err := RunReview(append(statusArgs, "--next-transition"), &refused)
			const wantCode = "immutable_review_transport_unsupported"
			const wantMessage = "The active runtime cannot provide immutable receipt-review transport; to keep working without receipt review here, run `gentle-ai review mode disable --scope clone --cwd <repo>`."
			const wantCause = "OpenCode 2.4.1 does not return completed foreground reviewer Tasks for immutable receipt review. Switch to a compatible OpenCode release (for example, 2.0.19 with the managed V2 relay plugin) and rerun."
			if err == nil || err.Error() != wantMessage+" ["+wantCode+"]" {
				t.Fatalf("bound STATUS error = %v\nstdout: %s", err, refused.String())
			}
			if !json.Valid(refused.Bytes()) {
				t.Fatalf("refusal must emit exactly one JSON document: %s", refused.String())
			}
			var failure ReviewIntegrationFailure
			decodeStrictReviewJSON(t, refused.Bytes(), &failure)
			if err := failure.Validate(); err != nil || failure.Operation != "review.status" || failure.Code != wantCode ||
				failure.Message != wantMessage || failure.Cause != wantCause || failure.MutationOutcome != ReviewMutationNotStarted ||
				failure.AuthorityApplicability != "not_evaluated" || failure.Replayability != reviewtransaction.ReplayabilityNotReplayable ||
				failure.RetrySafe || failure.NextAction != "stop" {
				t.Fatalf("unexpected public refusal: %#v, %v", failure, err)
			}
			if after := plainStatus(); !bytes.Equal(after, before) {
				t.Fatalf("refusal changed public STATUS\nbefore: %s\nafter:  %s", before, after)
			}

			stubOpenCodeRuntimeProbeSequence(t, "2.0.19")
			t.Setenv(openCodeRelayContractEnvironment, openCodeRelayContractV2)
			task, resumed := boundTask()
			if resumed["target"] != binding["target"] || resumed["expected-revision"] != binding["expected-revision"] {
				t.Fatalf("resumed Task did not retain its binding: %v, want %v", resumed, binding)
			}
			relay := startOpenCodeTransportRelay(t, repo, openCodeTransportEnvelope{
				Schema: openCodeReviewTransportSchema, Operation: "start", Prompt: task.Prompt, Agent: task.Agent,
			})
			var raw string
			if test.name == "refuter" {
				hash := reviewProviderRequestHashForTest(t, []byte(relay.prompt.Prompt))
				raw = `{"refuter_request_hash":"` + hash + `","results":[{"finding_id":"R3-001","outcome":"corroborated","proof_refs":["independent reproduction"]}]}`
			} else {
				raw = string(providerTargetedValidationPayload(t, request))
			}
			completed, err := relay.complete(openCodeTransportEnvelope{
				Schema: openCodeReviewTransportSchema, Operation: "complete", Nonce: relay.prompt.Nonce, Output: &raw,
			})
			if err != nil || completed.Output == nil {
				t.Fatalf("resumed relay completion = %#v, %v", completed, err)
			}
			var closure reviewLastEventClosureResult
			decodeStrictReviewJSON(t, []byte(*completed.Output), &closure)
			if closure.Operation != test.operation || closure.LineageID != lineage || closure.State != test.state {
				t.Fatalf("resumed Task closure = %#v", closure)
			}

			if test.state == reviewtransaction.StateApproved {
				stubOpenCodeRuntimeProbeSequence(t, "2.4.1")
				t.Setenv(openCodeRelayContractEnvironment, openCodeRelayContractV2)
				pending, encoded := frozenRuntimeStatusTransition(t, repo, lineage)
				if closure.Acknowledgement == nil || pending.ReasonCode != "approved_acknowledgement_required" ||
					pending.Execute == nil || pending.Execute.Command != closure.Acknowledgement.Command {
					t.Fatalf("completed lineage lost its exact acknowledgement: %s", encoded)
				}
				args := []string{"acknowledge-approved"}
				for _, argument := range pending.Execute.Arguments {
					args = append(args, argument.Token)
				}
				var acknowledged bytes.Buffer
				if err := RunReview(args, &acknowledged); err != nil {
					t.Fatalf("acknowledge on 2.4.1: %v\n%s", err, acknowledged.String())
				}
				assertAcknowledgedEnvelope(t, acknowledged.Bytes(), lineage, pending.Execute.Binding.TargetIdentity, closure.StoreRevision)
				closed, encoded := frozenRuntimeStatusTransition(t, repo, lineage)
				if closed.Kind != reviewNextTransitionStop || closed.ReasonCode != "target_already_acknowledged" {
					t.Fatalf("acknowledged lineage did not stay closed: %s", encoded)
				}
			}
		})
	}
}

type v2UnreadableInput struct{}

func (v2UnreadableInput) Read([]byte) (int, error) {
	panic("refused runtime read transport input before capability refusal")
}

// A V2 host whose PATH resolves a coexisting V1 binary must not inherit V1
// capability through the V2 plugin's relay: the declaration and the detected
// runtime disagree, so the relay refuses.
func TestOpenCodeV2TransportDeclarationCannotInheritV1(t *testing.T) {
	t.Setenv(openCodeRelayContractEnvironment, openCodeRelayContractV2)
	old := opencode.VersionRunnerOverride
	t.Cleanup(func() { opencode.VersionRunnerOverride = old })
	freshOpenCodeRuntimeProbe(t)
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		return opencode.CommandOutput{Stdout: []byte("1.18.30")}, nil
	}
	if reviewImmutableRuntimeCapability(model.AgentOpenCode).supportsImmutableReceiptReview() {
		t.Fatal("active V2 declaration inherited V1 capability")
	}
}
