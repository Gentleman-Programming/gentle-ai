package main

import (
	"fmt"
	"path/filepath"
	"slices"
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

func intendedUntrackedJourneys() []Journey {
	return []Journey{
		{
			ID:     "j75-intended-untracked-selection-executes-printed-start",
			Title:  "Mixed workspace candidate: STATUS collects explicit untracked intent and its printed START executes exactly",
			Source: "issue #2652: intended untracked files require explicit inventory-bound admission; #2394 keeps unrelated local bytes excluded",
			Steps: []Step{
				{Name: "fixture: repository", Fixture: baseRepo},
				{Name: "fixture: mixed tracked and intended/unrelated untracked files", Fixture: mixedIntendedUntrackedCandidate},
				{Name: "STATUS collects selection and printed START freezes only chosen paths", Requires: intendedUntrackedStatusCapability, Composite: selectIntendedUntrackedAndRunPrintedStart},
			},
		},
	}
}
