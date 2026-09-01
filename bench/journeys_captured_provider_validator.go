package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

const capturedProviderValidatorLineage = "captured-provider-validator"
const capturedProviderValidatorRejectionLineage = "captured-provider-validator-rejection"

var capturedProviderValidatorStatusCapability = &Capability{Verb: []string{"review", "status"}, Flags: []string{
	"--cwd", "--contract", "--agent", "--lineage", "--next-transition",
}}

// capturedProviderValidatorJourneys proves the STATUS-to-terminal-validator
// continuation with the native relay protocol. It deliberately drives relay
// frames directly; this is not evidence about OpenCode Task-hook affinity.
func capturedProviderValidatorJourneys() []Journey {
	return []Journey{
		{
			ID:     "j106-captured-provider-validator-terminal-capture",
			Review: reviewOptedIn,
			Title:  "#3587: captured provider validator closes only through its exact active lineage",
			Source: "#3587 provider-slot continuation: an occupied Go-admitted validator slot is an exact active-lineage provider fact, and its capture is the terminal event",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage correction candidate", Fixture: stageCaptureEvidenceDescriptorCorrection},
				{Name: "start correction review with an exact active lineage", Requires: startNamedCapability, Args: productArgs("review", "start", "--lineage", capturedProviderValidatorLineage)},
				{Name: "capture correction finding and the full selected lens set for the exact active lineage", Requires: captureResultCapability, Composite: func(r *journeyRun) error {
					return captureExactSelectedReviewerSlots(r, capturedProviderValidatorLineage, true)
				}},
				{Name: "capture the Go-issued bounded correction plan", Requires: captureCorrectionPlanCapability, Composite: func(r *journeyRun) error {
					return captureCorrectionPlanFor(r, capturedProviderValidatorLineage, 2)
				}},
				{Name: "fixture: correct the reviewed candidate", Fixture: writeCorrectedCandidate},
				{Name: "capture the Go-issued validator Task through the native relay protocol", Requires: capturedProviderValidatorStatusCapability, Composite: captureProviderValidatorSlot},
				{Name: "the terminal validator capture exposes acknowledgement before the exact lineage burns", Requires: statusCapability, Composite: func(r *journeyRun) error {
					return requireAtomicLineageAcknowledged(r, capturedProviderValidatorLineage)
				}},
			},
		},
		{
			ID:     "j123-rejected-provider-validator-starts-fresh-high-risk-review",
			Review: reviewOptedIn,
			Title:  "#3799: a rejected validator leaves a changed normal candidate for a fresh high-risk review",
			Source: "#3799 Boundary B: inline actionable rejection evidence is terminal for its exact authority; a normal candidate edit receives only selectorless STATUS/START and all four new lenses",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage high-risk correction candidate", Fixture: stageAtomicHighRiskCorrectionCandidate},
				{Name: "start correction review with an exact active lineage", Requires: startNamedCapability, Args: productArgs("review", "start", "--lineage", capturedProviderValidatorRejectionLineage)},
				{Name: "capture correction finding and all four selected lens slots", Requires: captureResultCapability, Composite: func(r *journeyRun) error {
					return captureAtomicReviewerSlots(r, capturedProviderValidatorRejectionLineage, true)
				}},
				{Name: "capture the Go-issued bounded correction plan", Requires: captureCorrectionPlanCapability, Composite: func(r *journeyRun) error {
					return captureCorrectionPlanFor(r, capturedProviderValidatorRejectionLineage, 2)
				}},
				{Name: "fixture: correct the reviewed candidate", Fixture: writeCorrectedCandidate},
				{Name: "capture inline actionable rejection evidence from Boundary A", Requires: capturedProviderValidatorStatusCapability, Composite: func(r *journeyRun) error {
					return captureRejectedProviderValidatorSlotFor(r, capturedProviderValidatorRejectionLineage)
				}},
				{Name: "fixture: make a normal candidate edit", Fixture: writeNormalCandidateAfterRejectedValidator},
				{Name: "selectorless STATUS/START creates a different fresh lineage with all four required lenses", Requires: atomicReviewStatusCapability, Composite: startFreshRejectedValidatorReview},
			},
		},
	}
}

func captureProviderValidatorSlot(r *journeyRun) error {
	return captureProviderValidatorSlotFor(r, capturedProviderValidatorLineage)
}

// captureProviderValidatorSlotFor relays the provider-owned validator request
// that STATUS binds to one correction. The relay's successful completion is
// the final event: it captures validation, approves, and exposes acknowledgement before the lineage burns.
func captureProviderValidatorSlotFor(r *journeyRun, lineage string) error {
	return captureProviderValidatorSlotWithResult(r, lineage, true)
}

func captureRejectedProviderValidatorSlotFor(r *journeyRun, lineage string) error {
	return captureProviderValidatorSlotWithResult(r, lineage, false)
}

func captureProviderValidatorSlotWithResult(r *journeyRun, lineage string, passed bool) error {
	status, err := readProviderValidatorStatus(r, lineage, true)
	if err != nil {
		return err
	}
	if status.ValidationRequest == nil || status.NextTransition == nil || status.NextTransition.Kind != "collect" ||
		status.NextTransition.ReasonCode != "targeted_validation_required" || status.NextTransition.Collect == nil ||
		len(status.NextTransition.Collect.Inputs) != 1 {
		return fmt.Errorf("provider slot capture status = %+v", status)
	}
	input := status.NextTransition.Collect.Inputs[0]
	if input.Name != "provider_targeted_validator" || input.CaptureOperation != "external.run_provider_role" ||
		input.ProviderTask == nil || input.ProviderTask.Role != "targeted-validator" || input.ProviderTask.Prompt == "" {
		return fmt.Errorf("provider slot task = %+v", input)
	}
	originalEvidence := "original acceptance check passed"
	if !passed {
		originalEvidence = "the corrected candidate still fails the original criterion"
	}
	payload, err := json.Marshal(map[string]any{
		"targeted_validation_request_hash": status.ValidationRequest.RequestHash,
		"correction_target_identity":       status.ValidationRequest.CorrectionTargetIdentity,
		"original_criteria":                map[string]any{"passed": passed, "evidence": []string{originalEvidence}},
		"correction_regression":            map[string]any{"passed": true, "evidence": []string{"the correction introduced no unrelated regression"}},
		"follow_ups":                       []any{},
	})
	if err != nil {
		return err
	}
	start, err := json.Marshal(map[string]string{
		"schema": "gentle-ai.provider-transport/v1", "operation": "start", "prompt": input.ProviderTask.Prompt,
	})
	if err != nil {
		return err
	}
	observation, err := r.runInteractive([]string{"review", "opencode-transport"}, true, func(reader *bufio.Reader, writer io.WriteCloser) error {
		if _, err := writer.Write(append(start, '\n')); err != nil {
			return fmt.Errorf("write relay start: %w", err)
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read relay prompt: %w", err)
		}
		var prompt struct {
			Schema    string `json:"schema"`
			Operation string `json:"operation"`
			Nonce     string `json:"nonce"`
			Prompt    string `json:"prompt"`
		}
		if err := json.Unmarshal([]byte(line), &prompt); err != nil || prompt.Schema != "gentle-ai.provider-transport/v1" ||
			prompt.Operation != "prompt" || prompt.Nonce == "" || prompt.Prompt == "" {
			return fmt.Errorf("relay prompt = %q: %w", line, err)
		}
		completion, err := json.Marshal(map[string]string{
			"schema": "gentle-ai.provider-transport/v1", "operation": "complete", "nonce": prompt.Nonce, "output": string(payload),
		})
		if err != nil {
			return err
		}
		if _, err := writer.Write(append(completion, '\n')); err != nil {
			return fmt.Errorf("write relay completion: %w", err)
		}
		return nil
	})
	if err != nil || observation.ExitCode != 0 {
		return fmt.Errorf("native provider-slot relay = exit %d err=%v stderr=%s", observation.ExitCode, err, firstLine(observation.Stderr))
	}
	if !capturedProviderSlotReported(observation.Stdout, passed) {
		return fmt.Errorf("native provider-slot relay did not report capture: %s", observation.Stdout)
	}
	return nil
}

func finalizeCapturedProviderValidatorSlot(r *journeyRun) error {
	status, err := readCapturedProviderValidatorStatus(r, false)
	if err != nil {
		return err
	}
	if status.Authority == nil || status.ValidationRequest == nil || status.NextTransition == nil || status.NextTransition.Kind != "execute" ||
		status.NextTransition.ReasonCode != "captured_provider_targeted_validation_ready" || status.NextTransition.Execute == nil ||
		status.NextTransition.Execute.Operation != "review.finalize" {
		return fmt.Errorf("generic captured-provider transition = %+v", status.NextTransition)
	}
	context := executeArgument(status.NextTransition.Execute.Arguments, "repository-context")
	want := []string{
		"--contract=" + reviewContractV2,
		"--lineage=" + capturedProviderValidatorLineage,
		"--expected-revision=" + status.Authority.Revision,
		"--target=" + status.ValidationRequest.CorrectionTargetIdentity,
		"--request-hash=" + status.ValidationRequest.RequestHash,
		"--repository-context=" + context,
		"--captured-evidence=true",
	}
	got := make([]string, len(status.NextTransition.Execute.Arguments))
	for index, argument := range status.NextTransition.Execute.Arguments {
		got[index] = argument.Token
	}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") || strings.Contains(strings.Join(got, "\n"), "--agent=") ||
		strings.Contains(strings.Join(got, "\n"), "--validation=") || context == "" {
		return fmt.Errorf("generic captured-provider finalize argv = %v, want %v", got, want)
	}
	result, err := decodeWaveOperation(r.runAt(r.sandbox.Root, append([]string{"review", "finalize"}, got...), false), "generic captured-provider finalize")
	if err != nil || result.State != "approved" || result.LineageID != capturedProviderValidatorLineage {
		return fmt.Errorf("generic captured-provider finalize result = %+v, %v", result, err)
	}
	return requireAtomicLineageAcknowledged(r, capturedProviderValidatorLineage)
}

func readCapturedProviderValidatorStatus(r *journeyRun, withOpenCodeTask bool) (waveCorrectionStatus, error) {
	return readProviderValidatorStatus(r, capturedProviderValidatorLineage, withOpenCodeTask)
}

func readProviderValidatorStatus(r *journeyRun, lineage string, withOpenCodeTask bool, selectors ...string) (waveCorrectionStatus, error) {
	arguments := []string{"review", "status", "--contract", reviewContractV2, "--next-transition", "--lineage", lineage}
	arguments = append(arguments, selectors...)
	if withOpenCodeTask {
		arguments = append(arguments, "--agent", "opencode")
	}
	observation := r.run(productArgsFor(r, arguments...), false)
	var status waveCorrectionStatus
	return status, decodeWaveObservation(observation, &status, "provider validator status")
}

func executeArgument(arguments []waveTransitionArgument, name string) string {
	for _, argument := range arguments {
		if argument.Name == name {
			return argument.Value
		}
	}
	return ""
}

func writeNormalCandidateAfterRejectedValidator(sandbox *Sandbox) error {
	return sandbox.write(filepath.Join(sandbox.Repo, "candidate.go"), "package candidate\n\nfunc value() int { return 4 }\n")
}

func startFreshRejectedValidatorReview(r *journeyRun) error {
	lineage, err := startAtomicTransactionFromSelectorlessStatus(r, capturedProviderValidatorRejectionLineage)
	if err != nil {
		return fmt.Errorf("selectorless START after rejected validator: %w", err)
	}
	r.sandbox.Lineage = lineage
	return requireExplicitAtomicFourLensStatusFor(r, lineage)
}

func capturedProviderSlotReported(stdout string, passed bool) bool {
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		var frame struct {
			Operation string `json:"operation"`
			Output    string `json:"output"`
		}
		if json.Unmarshal([]byte(line), &frame) != nil || frame.Operation != "result" {
			continue
		}
		var closure struct {
			Schema          string `json:"schema"`
			Operation       string `json:"operation"`
			State           string `json:"state"`
			Acknowledgement struct {
				Operation string `json:"operation"`
			} `json:"acknowledgement"`
		}
		if json.Unmarshal([]byte(frame.Output), &closure) == nil &&
			closure.Schema == "gentle-ai.review-last-event-closure/v1" && closure.Operation == "review/capture-validation" &&
			(closure.State == "escalated" && !passed || closure.State == "approved" && passed &&
				closure.Acknowledgement.Operation == "review.acknowledge-approved") {
			return true
		}
	}
	return false
}
