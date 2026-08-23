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
	Flags: []string{"--cwd", "--contract", "--next-transition", "--untracked-scope", "--intended-untracked", "--expected-untracked-inventory"},
}

var unbornIntendedUntrackedStatusCapability = &Capability{
	Verb:  []string{"review", "status"},
	Flags: []string{"--cwd", "--contract", "--agent", "--next-transition"},
}

const unbornIntendedDeliveryPath = "docs/unborn-candidate.md"

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
	status, err := readStatusForContract(r, reviewContractV2)
	if err != nil {
		return err
	}
	if status.NextTransition.Kind != "collect" || status.NextTransition.ReasonCode != "intended_untracked_selection_required" ||
		len(status.NextTransition.Collect.Inputs) != 1 {
		return fmt.Errorf("initial STATUS did not collect intended untracked selection: %+v", status.NextTransition)
	}
	digest := status.argument("expected_untracked_inventory")
	selectedPaths := []string{"docs/chosen, file.md", "docs/second file,with comma.md"}
	selectors := []string{"--untracked-scope=select", "--expected-untracked-inventory=" + digest}
	for _, path := range selectedPaths {
		selectors = append(selectors, "--intended-untracked="+path)
	}
	selected, err := readStatusForContract(r, reviewContractV2, selectors...)
	if err != nil {
		return err
	}
	if selected.NextTransition.Kind != "execute" || selected.NextTransition.Execute.Operation != "review.start" ||
		slices.Contains(selected.Projection.Paths, "unrelated-credentials.env") ||
		!slices.Contains(selected.Projection.Paths, selectedPaths[0]) || !slices.Contains(selected.Projection.Paths, selectedPaths[1]) {
		return fmt.Errorf("selected STATUS = %+v", selected)
	}
	started, err := runPrintedTransition(r, selected)
	if err != nil {
		return err
	}
	if started.ExitCode != 0 {
		return fmt.Errorf("printed selected START exited %d: %s", started.ExitCode, firstLine(started.Stderr))
	}
	return nil
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
			Title:  "#3417: an untracked intended candidate terminal-burns and staged validation stays unmanaged",
			Source: "#3417 preserves explicit intended-untracked START selection while removing staged receipt authorization",
			Steps: []Step{
				{Name: "fixture: unborn repository with one all-untracked candidate", Fixture: unbornIntendedDeliveryCandidate},
				{Name: "select every untracked path and execute printed START", Requires: unbornIntendedUntrackedStatusCapability, Composite: selectAndStartUnbornIntendedDelivery},
				{Name: "terminal finalization burns the unborn transaction", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: func(sandbox *Sandbox, observation Observation) error {
					return requireBurnedApproval(sandbox.Lineage)(sandbox, observation)
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
