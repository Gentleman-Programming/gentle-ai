package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const providerCaptureRetryLineage = "compiled-provider-capture-retry"

var providerCaptureRetryStatusCapability = &Capability{Verb: []string{"review", "status"}, Flags: []string{
	"--cwd", "--contract", "--agent", "--lineage", "--next-transition",
}}

var providerCaptureRetryCapability = &Capability{Verb: []string{"review", "capture-result"}, Flags: []string{
	"--agent", "--repository-context", "--expected-revision", "--lineage", "--target", "--lens", "--order",
}}

func providerCaptureRetryJourneys() []Journey {
	return []Journey{{
		ID:     "j105-compiled-provider-capture-retries-same-binding",
		Review: reviewOptedIn,
		Title:  "Compiled provider capture retries the same pending binding after a transport failure",
		Source: "issue #3138: Go owns provider capture binding and retry lifecycle",
		Steps: []Step{
			{Name: "fixture: isolated Claude provider fake", Fixture: providerCaptureRetryFixture},
			{Name: "fixture: stage candidate", Fixture: stageWaveCandidate},
			{Name: "negotiate and start the Claude provider review", Requires: providerCaptureRetryStatusCapability, Composite: startProviderCaptureRetry},
			{Name: "failed provider capture preserves the exact pending binding and its retry closes", Requires: providerCaptureRetryCapability, Composite: retryProviderCapture},
			{Name: "the final provider capture awaits exact acknowledgement before it burns the completed lineage", Requires: statusCapability, Composite: func(r *journeyRun) error {
				return requireAtomicLineageAcknowledged(r, providerCaptureRetryLineage, "--agent", "claude-code")
			}},
		},
	}}
}

func providerCaptureRetryFixture(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	bin := filepath.Join(sandbox.Root, "provider-bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		return err
	}
	launcher := filepath.Join(bin, "claude")
	const script = `#!/bin/sh
set -eu
prompt="$0.prompt"
attempts="$0.attempts"
cat > "$prompt"
attempt=0
if [ -f "$attempts" ]; then attempt=$(cat "$attempts"); fi
attempt=$((attempt + 1))
printf '%s\n' "$attempt" > "$attempts"
if [ "$attempt" -eq 1 ]; then exit 1; fi
subject=$(sed -n 's/.*"subject_hash":"\([^"]*\)".*/\1/p' "$prompt" | head -n 1)
test -n "$subject"
printf '{"subject_hash":"%s","inspection":{"status":"completed","paths":["candidate.go"]},"findings":[],"evidence":["inspected the complete frozen candidate"]}\n' "$subject"
`
	if err := os.WriteFile(launcher, []byte(script), 0o755); err != nil {
		return err
	}
	sandbox.PathOverride = bin
	sandbox.Scratch["provider-capture-attempts"] = launcher + ".attempts"
	return nil
}

func providerCaptureRetryStatus(r *journeyRun) (statusEnvelope, json.RawMessage, error) {
	observation := r.run([]string{"review", "status", "--cwd", r.sandbox.Repo, "--contract", reviewContractV2,
		"--agent", "claude-code", "--lineage", providerCaptureRetryLineage, "--next-transition"}, false)
	if observation.ExitCode != 0 {
		return statusEnvelope{}, nil, fmt.Errorf("provider STATUS exited %d: %s", observation.ExitCode, firstLine(observation.Stderr))
	}
	var status statusEnvelope
	var document struct {
		NextTransition struct {
			Collect struct {
				Inputs []json.RawMessage `json:"inputs"`
			} `json:"collect"`
		} `json:"next_transition"`
	}
	if err := json.Unmarshal([]byte(observation.Stdout), &status); err != nil {
		return statusEnvelope{}, nil, fmt.Errorf("decode provider STATUS: %w", err)
	}
	if err := json.Unmarshal([]byte(observation.Stdout), &document); err != nil {
		return statusEnvelope{}, nil, fmt.Errorf("decode exact provider STATUS binding: %w", err)
	}
	if len(document.NextTransition.Collect.Inputs) == 1 {
		return status, document.NextTransition.Collect.Inputs[0], nil
	}
	return status, nil, nil
}

func startProviderCaptureRetry(r *journeyRun) error {
	status, _, err := providerCaptureRetryStatus(r)
	if err != nil {
		return err
	}
	if status.NextTransition.Kind != "execute" || status.NextTransition.Execute.Operation != "review.start" {
		return fmt.Errorf("provider START transition = %+v", status.NextTransition)
	}
	args, err := printedCommandArguments(status.NextTransition.Execute.Command)
	if err != nil {
		return err
	}
	consent, agent := false, false
	for index, argument := range args {
		if argument == "--consent=relay" {
			args[index], consent = "--consent=granted", true
		}
		if argument == "--agent=claude-code" {
			agent = true
		}
	}
	if !consent || !agent {
		return fmt.Errorf("provider START did not carry the negotiated Claude binding: %v", args)
	}
	if observation := r.run(append(args, "--focus", "reliability"), false); observation.ExitCode != 0 {
		return fmt.Errorf("provider START failed: %s", firstLine(observation.Stderr))
	}
	return nil
}

func providerCaptureRetryArguments(status statusEnvelope) ([]string, error) {
	if status.NextTransition.Kind != "collect" || len(status.NextTransition.Collect.Inputs) != 1 ||
		status.NextTransition.Collect.Inputs[0].Name != "reviewer_result" || status.argument("repository-context") == "" ||
		status.argument("expected-revision") == "" || status.argument("lineage") != providerCaptureRetryLineage ||
		status.argument("target") == "" || status.argument("lens") == "" || status.argument("order") == "" {
		return nil, fmt.Errorf("provider capture binding = %+v", status.NextTransition)
	}
	return []string{"review", "capture-result", "--agent", "claude-code",
		"--repository-context", status.argument("repository-context"), "--expected-revision", status.argument("expected-revision"),
		"--lineage", status.argument("lineage"), "--target", status.argument("target"), "--lens", status.argument("lens"), "--order", status.argument("order")}, nil
}

func retryProviderCapture(r *journeyRun) error {
	before, pending, err := providerCaptureRetryStatus(r)
	if err != nil {
		return err
	}
	args, err := providerCaptureRetryArguments(before)
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		return fmt.Errorf("provider capture has no pending binding: %+v", before.NextTransition)
	}
	if observation := r.run(args, true); observation.ExitCode == 0 {
		return fmt.Errorf("first provider capture unexpectedly succeeded: %s", observation.Stdout)
	}
	if attempts, err := os.ReadFile(r.sandbox.Scratch["provider-capture-attempts"]); err != nil || string(attempts) != "1\n" {
		return fmt.Errorf("failed provider capture attempts = %q, %v; want first invocation", attempts, err)
	}
	after, retried, err := providerCaptureRetryStatus(r)
	if err != nil {
		return err
	}
	if len(retried) == 0 {
		return fmt.Errorf("provider retry has no pending binding: %+v", after.NextTransition)
	}
	if !bytes.Equal(pending, retried) {
		return fmt.Errorf("failed provider capture changed the pending binding:\nbefore=%s\nafter=%s", pending, retried)
	}
	if observation := r.run(args, true); observation.ExitCode != 0 {
		return fmt.Errorf("valid provider result bound to the exact pending subject failed: %s", firstLine(observation.Stderr))
	}
	if attempts, err := os.ReadFile(r.sandbox.Scratch["provider-capture-attempts"]); err != nil || string(attempts) != "2\n" {
		return fmt.Errorf("provider retry attempts = %q, %v; want two adapter invocations", attempts, err)
	}
	if advanced, _, err := providerCaptureRetryStatus(r); err != nil || advanced.Authority.LineageID != providerCaptureRetryLineage ||
		advanced.Authority.State != "approved" || advanced.NextTransition.Kind != "execute" ||
		advanced.NextTransition.ReasonCode != "approved_acknowledgement_required" ||
		advanced.NextTransition.Execute.Operation != "review.acknowledge-approved" {
		return fmt.Errorf("provider retry did not expose pending acknowledgement after its final capture: authority=%+v transition=%+v err=%v", advanced.Authority, advanced.NextTransition, err)
	}
	return nil
}
