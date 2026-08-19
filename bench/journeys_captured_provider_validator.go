package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const capturedProviderValidatorLineage = "captured-provider-validator"

var capturedProviderValidatorStatusCapability = &Capability{Verb: []string{"review", "status"}, Flags: []string{
	"--cwd", "--contract", "--agent", "--lineage", "--next-transition",
}}

// capturedProviderValidatorJourneys proves the generic STATUS-to-FINALIZE
// continuation with the native relay protocol. It deliberately drives relay
// frames directly; this is not evidence about OpenCode Task-hook affinity.
func capturedProviderValidatorJourneys() []Journey {
	return []Journey{{
		ID:     "j106-captured-provider-validator-slot-finalizes-generically",
		Review: reviewOptedIn,
		Title:  "Captured provider validator slot finalizes through generic STATUS without a runtime token",
		Source: "provider-slot continuation: occupied Go-admitted validator slots are provider facts, not OpenCode finalizer behavior",
		Steps: []Step{
			{Name: "fixture: repo", Fixture: baseRepo},
			{Name: "fixture: stage correction candidate", Fixture: stageCaptureEvidenceDescriptorCorrection},
			{Name: "start correction review", Requires: startNamedCapability, Args: productArgs("review", "start", "--lineage", capturedProviderValidatorLineage)},
			{Name: "capture correction finding and complete lenses", Requires: captureResultCapability, Composite: captureCorrectableFinding},
			{Name: "finalize reviewer results into correction-required", Requires: finalizeResultsCapability, Args: productArgs("review", "finalize", "--lineage", capturedProviderValidatorLineage, "--captured-results=true")},
			{Name: "forecast the bounded correction", Requires: finalizeCorrectionCapability, Args: productArgs("review", "finalize", "--lineage", capturedProviderValidatorLineage, "--correction-lines", "2")},
			{Name: "fixture: correct the reviewed candidate", Fixture: writeCorrectedCandidate},
			{Name: "capture passed correction evidence", Requires: captureEvidenceDescriptorCapability, Composite: func(r *journeyRun) error {
				return captureV5CorrectionEvidenceDescriptorFor(r, capturedProviderValidatorLineage)
			}},
			{Name: "capture the Go-issued validator Task through the native relay protocol", Requires: capturedProviderValidatorStatusCapability, Composite: captureProviderValidatorSlot},
			{Name: "finalize the generic captured-provider transition", Requires: finalizeEvidenceCapability, Composite: finalizeCapturedProviderValidatorSlot},
		},
	}}
}

func captureProviderValidatorSlot(r *journeyRun) error {
	status, err := readCapturedProviderValidatorStatus(r, true)
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
	payload, err := json.Marshal(map[string]any{
		"targeted_validation_request_hash": status.ValidationRequest.RequestHash,
		"correction_target_identity":       status.ValidationRequest.CorrectionTargetIdentity,
		"original_criteria":                map[string]any{"passed": true, "evidence": []string{"original acceptance check passed"}},
		"correction_regression":            map[string]any{"passed": true, "evidence": []string{"targeted regression check passed"}},
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
	if !capturedProviderSlotReported(observation.Stdout) {
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
	return nil
}

func readCapturedProviderValidatorStatus(r *journeyRun, withOpenCodeTask bool) (waveCorrectionStatus, error) {
	arguments := []string{"review", "status", "--contract", reviewContractV2, "--next-transition", "--lineage", capturedProviderValidatorLineage}
	if withOpenCodeTask {
		arguments = append(arguments, "--agent", "opencode")
	}
	observation := r.run(productArgsFor(r, arguments...), false)
	var status waveCorrectionStatus
	return status, decodeWaveObservation(observation, &status, "captured provider validator status")
}

func executeArgument(arguments []waveTransitionArgument, name string) string {
	for _, argument := range arguments {
		if argument.Name == name {
			return argument.Value
		}
	}
	return ""
}

func capturedProviderSlotReported(stdout string) bool {
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		var frame struct {
			Operation string `json:"operation"`
			Output    string `json:"output"`
		}
		if json.Unmarshal([]byte(line), &frame) == nil && frame.Operation == "result" &&
			strings.Contains(frame.Output, `"role":"targeted-validator"`) && strings.Contains(frame.Output, `"captured":true`) {
			return true
		}
	}
	return false
}
