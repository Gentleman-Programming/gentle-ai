package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

var intendedUntrackedStatusCapability = &Capability{
	Verb:  []string{"review", "status"},
	Flags: []string{"--cwd", "--contract", "--agent", "--next-transition", "--untracked-scope", "--intended-untracked", "--expected-untracked-inventory"},
}

var unbornIntendedUntrackedStatusCapability = &Capability{
	Verb:  []string{"review", "status"},
	Flags: []string{"--cwd", "--contract", "--agent", "--next-transition"},
}

const unbornIntendedDeliveryPath = "docs/unborn-candidate.md"

type intendedUntrackedSelectionStatus struct {
	Schema     string    `json:"schema"`
	Authority  *struct{} `json:"authority"`
	Projection struct {
		Paths []string `json:"paths"`
	} `json:"projection"`
	NextTransition struct {
		Kind       string `json:"kind"`
		ReasonCode string `json:"reason_code"`
		Collect    struct {
			Inputs []struct {
				Name             string                         `json:"name"`
				Schema           string                         `json:"schema"`
				CaptureOperation string                         `json:"capture_operation"`
				Arguments        []struct{ Name, Value string } `json:"arguments"`
				Submission       *waveSubmissionDescriptor      `json:"submission"`
			} `json:"inputs"`
		} `json:"collect"`
		Execute struct {
			Operation string `json:"operation"`
			Command   string `json:"command"`
		} `json:"execute"`
	} `json:"next_transition"`
}

type closedIntendedUntrackedSelection struct {
	Schema                     string   `json:"schema"`
	UntrackedScope             string   `json:"untracked_scope"`
	ExpectedUntrackedInventory string   `json:"expected_untracked_inventory"`
	IntendedUntracked          []string `json:"intended_untracked"`
}

func mixedIntendedUntrackedCandidate(sandbox *Sandbox) error {
	sandbox.PiReviewRelayContract = "gentle-pi.review-relay/v1"
	if err := sandbox.write(filepath.Join(sandbox.Repo, "README.md"), "# demo\n\ntracked review candidate\n"); err != nil {
		return err
	}
	for path, contents := range map[string]string{
		"docs/chosen, file.md":           "# Chosen\n",
		"docs/second file,with comma.md": "# Second\n",
		"unrelated-credentials.env":      "EXAMPLE_API_TOKEN=synthetic-placeholder\n",
		"ignored.txt":                    "ignored\n",
	} {
		if err := sandbox.write(filepath.Join(sandbox.Repo, path), contents); err != nil {
			return err
		}
	}
	return sandbox.write(filepath.Join(sandbox.Repo, ".git", "info", "exclude"), "ignored.txt\n")
}

func selectIntendedUntrackedAndRunPrintedStart(r *journeyRun) error {
	initialObservation := r.run(productArgsFor(r,
		"review", "status", "--cwd", r.sandbox.Repo, "--contract", reviewContractV2,
		"--agent", "pi", "--next-transition"), false)
	var initial intendedUntrackedSelectionStatus
	if err := decodeWaveObservation(initialObservation, &initial, "initial Pi STATUS"); err != nil {
		return err
	}
	if initial.Schema != statusSchemaV7 || initial.Authority != nil ||
		initial.NextTransition.Kind != "collect" || initial.NextTransition.ReasonCode != "intended_untracked_selection_required" ||
		len(initial.NextTransition.Collect.Inputs) != 1 {
		return fmt.Errorf("initial Pi STATUS = %+v", initial)
	}
	input := initial.NextTransition.Collect.Inputs[0]
	if input.Name != "intended_untracked_selection" || input.Schema != "gentle-ai.review-intended-untracked-selection/v1" ||
		input.CaptureOperation != "external.select_intended_untracked" || input.Submission == nil {
		return fmt.Errorf("initial intended-untracked submission = %+v", input)
	}
	inventory := ""
	for _, argument := range input.Arguments {
		if argument.Name == "expected_untracked_inventory" {
			inventory = argument.Value
			break
		}
	}
	if inventory == "" {
		return fmt.Errorf("initial intended-untracked selection omitted its inventory: %+v", input.Arguments)
	}
	selectedPaths := []string{"docs/chosen, file.md", "docs/second file,with comma.md"}
	answer, err := json.Marshal(closedIntendedUntrackedSelection{
		Schema:                     "gentle-ai.review-intended-untracked-selection/v1",
		UntrackedScope:             "select",
		ExpectedUntrackedInventory: inventory,
		IntendedUntracked:          selectedPaths,
	})
	if err != nil {
		return fmt.Errorf("encode intended-untracked selection: %w", err)
	}
	arguments, err := intendedUntrackedSelectionSubmissionArguments(input.Submission, string(answer))
	if err != nil {
		return err
	}
	selectedObservation := r.runAt(r.sandbox.Repo, arguments, false)
	var selected intendedUntrackedSelectionStatus
	if err := decodeWaveObservation(selectedObservation, &selected, "selected intended-untracked STATUS"); err != nil {
		return err
	}
	if selected.Schema != statusSchemaV7 || selected.NextTransition.Kind != "execute" ||
		selected.NextTransition.Execute.Operation != "review.start" ||
		!slices.Equal(selected.Projection.Paths, []string{"README.md", selectedPaths[0], selectedPaths[1]}) {
		return fmt.Errorf("selected Pi STATUS = %+v", selected)
	}
	var printed statusEnvelope
	if err := json.Unmarshal([]byte(strings.TrimSpace(selectedObservation.Stdout)), &printed); err != nil {
		return fmt.Errorf("parse selected printed START: %w", err)
	}
	started, err := runPrintedTransition(r, printed)
	if err != nil {
		return err
	}
	var terminal struct {
		State  string `json:"state"`
		Action string `json:"action"`
	}
	if err := decodeWaveObservation(started, &terminal, "printed selected START"); err != nil {
		return err
	}
	if terminal.State != "approved" || terminal.Action != "closed" {
		return fmt.Errorf("printed selected START terminal result = %+v", terminal)
	}
	return nil
}

func intendedUntrackedSelectionSubmissionArguments(descriptor *waveSubmissionDescriptor, value string) ([]string, error) {
	if descriptor == nil || descriptor.OperationToken != "status" || descriptor.Value == nil || len(descriptor.Values) != 0 ||
		descriptor.Value.Slot != "intended_untracked_selection" || descriptor.Value.Domain != "schema_bound_json" ||
		descriptor.Value.Schema != "gentle-ai.review-intended-untracked-selection/v1" ||
		descriptor.Value.SubstitutionLocation < 0 || descriptor.Value.SubstitutionLocation >= len(descriptor.ArgumentTokens) {
		return nil, fmt.Errorf("intended-untracked submission descriptor = %+v", descriptor)
	}
	placeholders := 0
	for index, token := range descriptor.ArgumentTokens {
		if !strings.HasPrefix(token, "--") || strings.ContainsAny(token, " \t\r\n") || strings.HasPrefix(token, "--cwd=") {
			return nil, fmt.Errorf("intended-untracked submission leaked a caller-owned token: %q", token)
		}
		if strings.Contains(token, "{{value}}") {
			placeholders++
			if index != descriptor.Value.SubstitutionLocation || strings.Count(token, "{{value}}") != 1 {
				return nil, fmt.Errorf("intended-untracked submission slot = %q at %d", token, index)
			}
		}
	}
	if placeholders != 1 {
		return nil, fmt.Errorf("intended-untracked submission descriptor has %d value slots: %+v", placeholders, descriptor)
	}
	arguments := append([]string{"review", descriptor.OperationToken}, descriptor.ArgumentTokens...)
	index := descriptor.Value.SubstitutionLocation + 2
	arguments[index] = strings.Replace(arguments[index], "{{value}}", value, 1)
	if strings.Contains(strings.Join(arguments, "\x00"), "{{value}}") {
		return nil, fmt.Errorf("intended-untracked submission did not replace its value slot: %v", arguments)
	}
	return arguments, nil
}

func unbornUntrackedExecutableCandidate(sandbox *Sandbox) error {
	if err := os.MkdirAll(sandbox.Repo, 0o755); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "init", "-b", "main", "-q"); err != nil {
		return err
	}
	path := filepath.Join(sandbox.Repo, "scripts", "unborn-candidate.sh")
	if err := sandbox.write(path, "#!/bin/sh\necho candidate\n"); err != nil {
		return err
	}
	return os.Chmod(path, 0o755)
}

func unbornUntrackedStatusCollectsSelection(r *journeyRun) error {
	observation := r.run(productArgsFor(r,
		"review", "status", "--cwd", r.sandbox.Repo, "--contract", reviewContractV2,
		"--agent", "opencode", "--next-transition"), false)
	if observation.ExitCode != 0 {
		return fmt.Errorf("unborn STATUS exited %d: %s", observation.ExitCode, firstLine(observation.Stderr))
	}
	var status struct {
		Authority      *json.RawMessage `json:"authority"`
		NextTransition struct {
			Kind       string `json:"kind"`
			ReasonCode string `json:"reason_code"`
			Collect    struct {
				Inputs []struct {
					Arguments []struct {
						Name  string `json:"name"`
						Value string `json:"value"`
					} `json:"arguments"`
				} `json:"inputs"`
			} `json:"collect"`
		} `json:"next_transition"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &status); err != nil {
		return fmt.Errorf("parse unborn STATUS: %w", err)
	}
	if status.Authority != nil || status.NextTransition.Kind != "collect" ||
		status.NextTransition.ReasonCode != "intended_untracked_selection_required" || len(status.NextTransition.Collect.Inputs) != 1 {
		return fmt.Errorf("unborn STATUS = %+v", status)
	}
	values := map[string]string{}
	for _, argument := range status.NextTransition.Collect.Inputs[0].Arguments {
		values[argument.Name] = argument.Value
	}
	var paths []string
	if err := json.Unmarshal([]byte(values["eligible_paths_json"]), &paths); err != nil || !slices.Equal(paths, []string{"scripts/unborn-candidate.sh"}) ||
		!strings.HasPrefix(values["expected_untracked_inventory"], "sha256:") {
		return fmt.Errorf("unborn selection inventory = paths %v digest %q: %v", paths, values["expected_untracked_inventory"], err)
	}
	return nil
}

func unbornIntendedDeliveryCandidate(sandbox *Sandbox) error {
	if err := os.MkdirAll(sandbox.Repo, 0o755); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "init", "-b", "main", "-q"); err != nil {
		return err
	}
	return sandbox.write(filepath.Join(sandbox.Repo, unbornIntendedDeliveryPath), "# Reviewed candidate\n")
}

func selectAndStartUnbornIntendedDelivery(r *journeyRun) error {
	status, err := readStatusForContract(r, reviewContractV2, "--agent", "opencode")
	if err != nil {
		return err
	}
	if status.NextTransition.Kind != "collect" || status.NextTransition.ReasonCode != "intended_untracked_selection_required" {
		return fmt.Errorf("unborn delivery STATUS did not collect intended untracked selection: %+v", status.NextTransition)
	}
	digest := status.argument("expected_untracked_inventory")
	selected, err := readStatusForContract(r, reviewContractV2,
		"--agent", "opencode", "--untracked-scope=select", "--expected-untracked-inventory="+digest,
		"--intended-untracked="+unbornIntendedDeliveryPath)
	if err != nil {
		return err
	}
	if selected.NextTransition.Kind != "execute" || selected.NextTransition.Execute.Operation != "review.start" ||
		!slices.Equal(selected.Projection.Paths, []string{unbornIntendedDeliveryPath}) {
		return fmt.Errorf("selected unborn delivery STATUS = %+v", selected)
	}
	started, err := runPrintedTransition(r, selected)
	if err != nil {
		return err
	}
	if started.ExitCode != 0 {
		return fmt.Errorf("printed unborn delivery START exited %d: %s", started.ExitCode, firstLine(started.Stderr))
	}
	if err := rememberLineage(r.sandbox, started); err != nil || r.sandbox.Lineage == "" {
		return fmt.Errorf("printed unborn delivery START did not publish a lineage: %v", err)
	}
	r.sandbox.Scratch["unborn-intended-inventory"] = digest
	return nil
}

func requireUnbornIntendedStagedDeliveryStop(r *journeyRun) error {
	if _, err := gitOut(r.sandbox, r.sandbox.Repo, "rev-parse", "--verify", "HEAD"); err == nil {
		return fmt.Errorf("unborn delivery fixture unexpectedly has a HEAD")
	}
	if cached, err := gitOut(r.sandbox, r.sandbox.Repo, "ls-files", "--cached"); err != nil || cached != "" {
		return fmt.Errorf("unborn delivery real index = %q, %v", cached, err)
	}
	selectors := []string{
		"--agent", "opencode", "--lineage", r.sandbox.Lineage, "--gate", "pre-commit",
		"--untracked-scope=select", "--expected-untracked-inventory=" + r.sandbox.Scratch["unborn-intended-inventory"],
		"--intended-untracked=" + unbornIntendedDeliveryPath,
	}
	status, err := readStatusForContract(r, reviewContractV2, selectors...)
	if err != nil {
		return err
	}
	replayed, replayErr := readStatusForContract(r, reviewContractV2, selectors...)
	if replayErr != nil || status.Authority.State != "approved" || status.Authority.Revision == "" ||
		status.Authority.Revision != replayed.Authority.Revision || status.NextTransition.Kind != "stop" ||
		status.NextTransition.ReasonCode != "staged_delivery_candidate_required" || status.NextTransition.Execute.Operation != "" {
		return fmt.Errorf("unborn intended staged delivery stop = %+v, replay = %+v, %v", status, replayed, replayErr)
	}
	if cached, err := gitOut(r.sandbox, r.sandbox.Repo, "ls-files", "--cached"); err != nil || cached != "" {
		return fmt.Errorf("unborn delivery STATUS mutated the real index: %q, %v", cached, err)
	}
	return nil
}

func stageUnbornIntendedDeliveryCandidate(sandbox *Sandbox) error {
	return sandbox.git(sandbox.Repo, "add", "--", unbornIntendedDeliveryPath)
}

func validateUnbornIntendedStagedDelivery(r *journeyRun) error {
	status, err := readStatusForContract(r, reviewContractV2,
		"--agent", "opencode", "--lineage", r.sandbox.Lineage, "--gate", "pre-commit", "--projection", "staged")
	if err != nil {
		return err
	}
	if status.NextTransition.Kind != "execute" || status.NextTransition.ReasonCode != "approved_receipt_ready" ||
		status.NextTransition.Execute.Operation != "review.validate" || status.NextTransition.Execute.Command == "" {
		return fmt.Errorf("exact unborn intended staged transition = %+v", status.NextTransition)
	}
	observation, err := runPrintedTransition(r, status)
	if err != nil {
		return err
	}
	return requireGateForLineage(observation, r.sandbox.Lineage, false)
}

const selectedUntrackedTerminalPath = "internal/selected.go"

func selectedUntrackedTerminalCandidate(sandbox *Sandbox) error {
	return sandbox.write(filepath.Join(sandbox.Repo, selectedUntrackedTerminalPath), "package selected\n\nfunc Value() int { return 1 }\n")
}

func selectUntrackedCaptureAndResumeTerminal(r *journeyRun) error {
	start, _, err := frozenLineageSelectedStatus(r, "", selectedUntrackedTerminalPath)
	if err != nil || start.NextTransition.Kind != "execute" || start.NextTransition.Execute.Operation != "review.start" {
		return fmt.Errorf("selected untracked START = %+v, %v", start.NextTransition, err)
	}
	if err := startFrozenLineageWithConsent(r, start); err != nil {
		return err
	}
	active, _, err := frozenLineageStatus(r, r.sandbox.Lineage)
	if err != nil || active.Authority.LineageID != r.sandbox.Lineage || active.Authority.State != "reviewing" ||
		active.NextTransition.Kind != "collect" || active.NextTransition.ReasonCode != "reviewer_results_required" ||
		len(active.NextTransition.Collect.Inputs) != 1 {
		return fmt.Errorf("selected untracked reviewer STATUS = authority=%+v transition=%+v err=%v", active.Authority, active.NextTransition, err)
	}
	if err := captureFrozenReviewerResult(r, active); err != nil {
		return err
	}
	resumed, _, err := frozenLineageStatus(r, r.sandbox.Lineage)
	if err != nil || resumed.TargetIdentity != active.TargetIdentity || resumed.Authority.LineageID != r.sandbox.Lineage ||
		resumed.Authority.State != "approved" || resumed.NextTransition.Kind != "execute" ||
		resumed.NextTransition.ReasonCode != "approved_acknowledgement_required" ||
		resumed.NextTransition.Execute.Operation != "review.acknowledge-approved" ||
		resumed.executeArgument("lineage") != r.sandbox.Lineage || resumed.executeArgument("target") != active.TargetIdentity ||
		resumed.executeArgument("expected-revision") != resumed.Authority.Revision {
		return fmt.Errorf("selected untracked terminal resume = authority=%+v target=%q transition=%+v err=%v", resumed.Authority, resumed.TargetIdentity, resumed.NextTransition, err)
	}
	return nil
}

func intendedUntrackedJourneys() []Journey {
	return []Journey{
		{
			ID:     "j75-intended-untracked-selection-executes-printed-start",
			Review: reviewOptedIn,
			Title:  "Mixed workspace candidate: STATUS collects explicit untracked intent and its printed START executes exactly",
			Source: "issue #2652: intended untracked files require explicit inventory-bound admission; #2394 keeps unrelated local bytes excluded",
			Steps: []Step{
				{Name: "fixture: repository", Fixture: baseRepo},
				{Name: "fixture: mixed tracked and intended/unrelated untracked files", Fixture: mixedIntendedUntrackedCandidate},
				{Name: "STATUS collects selection and printed START freezes only chosen paths", Requires: intendedUntrackedStatusCapability, Composite: selectIntendedUntrackedAndRunPrintedStart},
			},
		},
		{
			ID:     "j126-selected-untracked-terminal-status-resumes-without-flags",
			Review: reviewOptedIn,
			Title:  "#4018: selected untracked reviewer closure resumes its terminal acknowledgement without selection flags",
			Source: "#4018: an explicit occupied lineage owns its immutable intended-untracked selection through terminal continuation",
			Steps: []Step{
				{Name: "fixture: repository", Fixture: baseRepo},
				{Name: "fixture: selected untracked Go review candidate", Fixture: selectedUntrackedTerminalCandidate},
				{Name: "select untracked candidate, close its actual reviewer slot, and resume the acknowledgement through lineage STATUS without flags", Requires: frozenLineageStatusCapability, Composite: selectUntrackedCaptureAndResumeTerminal},
			},
		},
		{
			ID:     "j88-unborn-status-collects-untracked-selection",
			Review: reviewOptedIn,
			Title:  "Unborn repository: STATUS collects explicit untracked selection before snapshot assessment",
			Source: "issue #2843: an unselected executable candidate must collect inventory-bound intent, never return generic retry or create authority",
			Steps: []Step{
				{Name: "fixture: unborn repository with one untracked executable candidate", Fixture: unbornUntrackedExecutableCandidate},
				{Name: "mode enable", Requires: modeCapability, Args: productArgs("review", "mode", "enable", "--json")},
				{Name: "v2 OpenCode STATUS collects the inventory-bound untracked selection", Requires: unbornIntendedUntrackedStatusCapability, Composite: unbornUntrackedStatusCollectsSelection},
			},
		},
		{
			ID:     "j110-untracked-terminal-burn-and-unmanaged-staged-validation",
			Review: reviewOptedIn,
			Title:  "#3797: an untracked intended candidate awaits acknowledgement before burn and staged validation stays unmanaged",
			Source: "#3797 preserves explicit intended-untracked START selection while requiring exact acknowledgement before terminal burn",
			Steps: []Step{
				{Name: "fixture: unborn repository with one all-untracked candidate", Fixture: unbornIntendedDeliveryCandidate},
				{Name: "select every untracked path and execute printed zero-lens START", Requires: unbornIntendedUntrackedStatusCapability, Composite: selectAndStartUnbornIntendedDelivery},
				{Name: "the zero-lens terminal event emits acknowledgement before burning the unborn transaction", Requires: statusCapability, Composite: func(r *journeyRun) error {
					return requireAtomicLineageAcknowledged(r, r.sandbox.Lineage,
						"--agent", "opencode", "--untracked-scope=select",
						"--expected-untracked-inventory="+r.sandbox.Scratch["unborn-intended-inventory"],
						"--intended-untracked="+unbornIntendedDeliveryPath,
					)
				}},
				{Name: "unstaged unborn pre-commit validation is informational and unmanaged", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit"), After: func(_ *Sandbox, observation Observation) error {
					return requireUnmanagedShippedGate(observation, "pre-commit")
				}},
				{Name: "fixture: stage the formerly reviewed candidate", Fixture: stageUnbornIntendedDeliveryCandidate},
				{Name: "staged unborn pre-commit validation remains informational and unmanaged", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit"), After: func(_ *Sandbox, observation Observation) error {
					return requireUnmanagedShippedGate(observation, "pre-commit")
				}},
			},
		},
	}
}
