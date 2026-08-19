package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

var intendedUntrackedStatusCapability = &Capability{
	Verb:  []string{"review", "status"},
	Flags: []string{"--cwd", "--contract", "--next-transition", "--untracked-scope", "--intended-untracked", "--expected-untracked-inventory"},
}

func mixedIntendedUntrackedCandidate(sandbox *Sandbox) error {
	if err := sandbox.write(filepath.Join(sandbox.Repo, "README.md"), "# demo\n\ntracked review candidate\n"); err != nil {
		return err
	}
	for path, contents := range map[string]string{
		"docs/chosen, file.md":           "# Chosen\n",
		"docs/second file,with comma.md": "# Second\n",
		"unrelated-credentials.env":      "EXAMPLE_API_TOKEN=synthetic-placeholder\n",
		"ignored.txt":                    "ignored\n",
		".gitignore":                     "ignored.txt\n",
	} {
		if err := sandbox.write(filepath.Join(sandbox.Repo, path), contents); err != nil {
			return err
		}
	}
	return nil
}

func selectIntendedUntrackedAndRunPrintedStart(r *journeyRun) error {
	initialObservation := r.run(productArgsFor(r, "review", "status", "--contract", reviewContractV2, "--next-transition"), false)
	var initial waveCorrectionStatus
	if err := decodeWaveObservation(initialObservation, &initial, "initial intended-untracked STATUS"); err != nil {
		return err
	}
	if initial.NextTransition.Kind != "collect" || initial.NextTransition.ReasonCode != "intended_untracked_selection_required" ||
		len(initial.NextTransition.Collect.Inputs) != 1 || initial.NextTransition.Collect.Inputs[0].Submission == nil {
		return fmt.Errorf("initial STATUS did not own intended untracked submission: %+v", initial.NextTransition)
	}
	selectedPaths := []string{"docs/chosen, file.md", "docs/second file,with comma.md"}
	arguments, err := intendedUntrackedSubmissionArguments(r, *initial.NextTransition.Collect.Inputs[0].Submission, "select", selectedPaths)
	if err != nil {
		return err
	}
	selectedObservation := r.run(arguments, false)
	var selected waveCorrectionStatus
	if err := decodeWaveObservation(selectedObservation, &selected, "provider-owned selected STATUS"); err != nil {
		return err
	}
	if selected.NextTransition.Kind != "execute" || selected.NextTransition.Execute.Operation != "review.start" ||
		selected.TargetIdentity == initial.TargetIdentity ||
		slices.Contains(selected.Projection.Paths, "unrelated-credentials.env") ||
		!slices.Contains(selected.Projection.Paths, selectedPaths[0]) || !slices.Contains(selected.Projection.Paths, selectedPaths[1]) {
		return fmt.Errorf("selected STATUS = %+v", selected)
	}
	startArguments, err := printedCommandArguments(selected.NextTransition.Execute.Command)
	if err != nil {
		return err
	}
	started := r.run(startArguments, false)
	if started.ExitCode != 0 {
		return fmt.Errorf("printed selected START exited %d: %s", started.ExitCode, firstLine(started.Stderr))
	}
	if strings.Contains(started.Stdout+started.Stderr, "stale_target_identity") {
		return fmt.Errorf("selected START reported stale_target_identity: %s", started.Stdout+started.Stderr)
	}
	var authority waveOperationResult
	if err := decodeWaveObservation(started, &authority, "selected START"); err != nil {
		return err
	}
	if authority.Operation != "review.start" || authority.TargetIdentity != selected.TargetIdentity || authority.TargetIdentity == initial.TargetIdentity {
		return fmt.Errorf("selected authority = %+v, selected target = %s", authority, selected.TargetIdentity)
	}
	return nil
}

func selectOverlayIntendedUntrackedAndRunPrintedStart(r *journeyRun) error {
	baseRef, err := gitOut(r.sandbox, r.sandbox.Repo, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	const runtimeAgent = "opencode"
	initialObservation := r.run(productArgsFor(r, "review", "status", "--contract", reviewContractV2, "--next-transition", "--agent="+runtimeAgent, "--base-ref", baseRef, "--workspace-overlay"), false)
	var initial waveCorrectionStatus
	if err := decodeWaveObservation(initialObservation, &initial, "initial overlay intended-untracked STATUS"); err != nil {
		return err
	}
	if initial.Projection.Kind != "base-workspace-overlay" || initial.NextTransition.Kind != "collect" ||
		initial.NextTransition.ReasonCode != "intended_untracked_selection_required" || len(initial.NextTransition.Collect.Inputs) != 1 ||
		initial.NextTransition.Collect.Inputs[0].Submission == nil {
		return fmt.Errorf("initial overlay STATUS = %+v", initial)
	}
	if !slices.Contains(initial.NextTransition.Collect.Inputs[0].Submission.ArgumentTokens, "--agent="+runtimeAgent) {
		return fmt.Errorf("initial overlay STATUS submission = %+v, want --agent=%s", initial.NextTransition.Collect.Inputs[0].Submission.ArgumentTokens, runtimeAgent)
	}
	selectedPaths := []string{"docs/chosen, file.md", "docs/second file,with comma.md"}
	arguments, err := intendedUntrackedSubmissionArguments(r, *initial.NextTransition.Collect.Inputs[0].Submission, "select", selectedPaths)
	if err != nil {
		return err
	}
	selectedObservation := r.run(arguments, false)
	var selected waveCorrectionStatus
	if err := decodeWaveObservation(selectedObservation, &selected, "provider-owned overlay selected STATUS"); err != nil {
		return err
	}
	if selected.Projection.Kind != "base-workspace-overlay" || selected.TargetIdentity == initial.TargetIdentity ||
		!slices.Contains(selected.Projection.Paths, selectedPaths[0]) || !slices.Contains(selected.Projection.Paths, selectedPaths[1]) ||
		slices.Contains(selected.Projection.Paths, "unrelated-credentials.env") || selected.NextTransition.Kind != "execute" ||
		selected.NextTransition.Execute.Operation != "review.start" {
		return fmt.Errorf("selected overlay STATUS = %+v", selected)
	}
	command := selected.NextTransition.Execute.Command
	for _, token := range []string{
		"gentle-ai review start ",
		"--contract=" + reviewContractV2,
		"--target=" + selected.TargetIdentity,
		"--projection=workspace",
		"--agent=" + runtimeAgent,
		"--workspace-overlay=true",
		"--untracked-scope=select",
	} {
		if !strings.Contains(command, token) {
			return fmt.Errorf("selected overlay printed START = %q, want %q", command, token)
		}
	}
	startArguments, err := printedCommandArguments(command)
	if err != nil {
		return err
	}
	if !slices.Contains(startArguments, "--agent="+runtimeAgent) {
		return fmt.Errorf("selected overlay START arguments = %v, want --agent=%s", startArguments, runtimeAgent)
	}
	started := r.run(startArguments, false)
	if started.ExitCode != 0 {
		return fmt.Errorf("printed overlay START exited %d: %s", started.ExitCode, firstLine(started.Stderr))
	}
	if strings.Contains(started.Stdout+started.Stderr, "stale_target_identity") {
		return fmt.Errorf("selected overlay START reported stale_target_identity: %s", started.Stdout+started.Stderr)
	}
	var authority waveOperationResult
	if err := decodeWaveObservation(started, &authority, "selected overlay START"); err != nil {
		return err
	}
	if authority.Operation != "review.start" || authority.TargetIdentity != selected.TargetIdentity || authority.TargetIdentity == initial.TargetIdentity {
		return fmt.Errorf("selected overlay authority = %+v, selected target = %s, initial target = %s", authority, selected.TargetIdentity, initial.TargetIdentity)
	}
	return nil
}

func intendedUntrackedSubmissionArguments(r *journeyRun, descriptor waveSubmissionDescriptor, scope string, paths []string) ([]string, error) {
	if descriptor.OperationToken != "status" || len(descriptor.ArgumentTokens) == 0 || len(descriptor.Values) < 3 {
		return nil, fmt.Errorf("intended-untracked submission = %+v", descriptor)
	}
	arguments := append([]string{"review", descriptor.OperationToken}, descriptor.ArgumentTokens...)
	values := map[string][]string{"cwd": {r.sandbox.Repo}, "untracked_scope": {scope}, "intended_untracked": paths}
	for _, slot := range descriptor.Values {
		index := slot.SubstitutionLocation + 2
		placeholder := "{{" + slot.Slot + "}}"
		if slot.Repeated {
			replacements := make([]string, len(values[slot.Slot]))
			for valueIndex, value := range values[slot.Slot] {
				replacements[valueIndex] = strings.Replace(arguments[index], placeholder, value, 1)
			}
			arguments = append(arguments[:index], append(replacements, arguments[index+1:]...)...)
			continue
		}
		if len(values[slot.Slot]) != 1 {
			return nil, fmt.Errorf("submission slot %q requires one value", slot.Slot)
		}
		arguments[index] = strings.Replace(arguments[index], placeholder, values[slot.Slot][0], 1)
	}
	return arguments, nil
}

func intendedUntrackedJourneys() []Journey {
	return []Journey{
		{
			ID:     "j75-intended-untracked-selection-executes-printed-start",
			Title:  "Mixed workspace candidate: STATUS collects explicit untracked intent and its printed START executes exactly",
			Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/2872",
			Steps: []Step{
				{Name: "fixture: repository", Fixture: baseRepo},
				{Name: "fixture: mixed tracked and intended/unrelated untracked files", Fixture: mixedIntendedUntrackedCandidate},
				{Name: "STATUS collects selection and printed START freezes only chosen paths", Requires: intendedUntrackedStatusCapability, Composite: selectIntendedUntrackedAndRunPrintedStart},
			},
		},
		{
			ID:     "j115-intended-untracked-overlay-selection-preserves-target",
			Title:  "Workspace overlay STATUS preserves target kind and identity through provider-owned untracked selection and the printed START",
			Source: "https://github.com/Gentleman-Programming/gentle-ai/pull/2880",
			Steps: []Step{
				{Name: "fixture: repository", Fixture: baseRepo},
				{Name: "fixture: mixed tracked and intended/unrelated untracked files", Fixture: mixedIntendedUntrackedCandidate},
				{Name: "overlay STATUS owns selection and its exact printed START", Requires: intendedUntrackedStatusCapability, Composite: selectOverlayIntendedUntrackedAndRunPrintedStart},
			},
		},
	}
}
