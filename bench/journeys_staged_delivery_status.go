package main

import (
	"fmt"
	"path/filepath"
)

const approvedWorkspacePreCommitLineage = "j89-approved-workspace-precommit-staged-status"

var stagedProjectionStatusCapability = &Capability{
	Verb:  []string{"review", "status"},
	Flags: []string{"--cwd", "--contract", "--agent", "--next-transition", "--lineage", "--projection"},
}

func unstagedPassiveWorkspaceReceiptCandidate(sandbox *Sandbox) error {
	path := filepath.Join(sandbox.Repo, "README.md")
	return sandbox.write(path, "# demo\n\nreviewed workspace receipt candidate for #2758.\n")
}

func stageApprovedWorkspaceReceiptPaths(sandbox *Sandbox) error {
	return sandbox.git(sandbox.Repo, "add", "README.md")
}

func approvedWorkspaceStatusRequiresStagedDelivery(run *journeyRun) error {
	status, err := readStatusForContract(run, reviewContractV2,
		"--agent", "opencode", "--lineage", approvedWorkspacePreCommitLineage)
	if err != nil {
		return err
	}
	if status.NextTransition.Kind != "stop" || status.NextTransition.ReasonCode != "staged_delivery_candidate_required" ||
		status.NextTransition.Execute.Operation != "" || len(status.NextTransition.Collect.Inputs) != 0 {
		return fmt.Errorf("workspace STATUS transition = %+v, want stop/staged_delivery_candidate_required with no command", status.NextTransition)
	}
	return nil
}

func stagedStatusExecutesPreCommitValidation(run *journeyRun) error {
	status, err := readStatusForContract(run, reviewContractV2,
		"--agent", "opencode", "--lineage", approvedWorkspacePreCommitLineage, "--projection", "staged")
	if err != nil {
		return err
	}
	if status.NextTransition.Kind != "execute" || status.NextTransition.Execute.Operation != "review.validate" {
		return fmt.Errorf("staged STATUS transition = %+v, want executable review.validate", status.NextTransition)
	}
	observation, err := runPrintedTransition(run, status)
	if err != nil {
		return err
	}
	return requireGateForLineage(observation, approvedWorkspacePreCommitLineage, false)
}

func stagedDeliveryStatusJourneys() []Journey {
	return []Journey{
		{
			ID:     "j89-approved-workspace-receipt-requires-exact-staged-precommit-status",
			Title:  "Approved workspace receipt: pre-commit STATUS stops until the exact reviewed paths are staged",
			Source: "issue #2758: workspace receipt cannot authorize a different staged-index delivery candidate",
			Steps: []Step{
				{Name: "fixture: repository", Fixture: baseRepo},
				{Name: "fixture: unstaged passive workspace candidate", Fixture: unstagedPassiveWorkspaceReceiptCandidate},
				{Name: "review start freezes the workspace candidate", Requires: startNamedCapability,
					Args: productArgs("review", "start", "--lineage", approvedWorkspacePreCommitLineage), After: rememberLineage},
				{Name: "review finalize approves the workspace receipt", Requires: finalizeCapability,
					Args: productArgs("review", "finalize", "--lineage", approvedWorkspacePreCommitLineage), After: rememberLineage},
				{Name: "workspace STATUS refuses to hand out pre-commit validation", Requires: stagedProjectionStatusCapability,
					Composite: approvedWorkspaceStatusRequiresStagedDelivery},
				{Name: "fixture: stage the exact reviewed path", Fixture: stageApprovedWorkspaceReceiptPaths},
				{Name: "staged STATUS prints validation and that validation allows", Requires: stagedProjectionStatusCapability,
					Composite: stagedStatusExecutesPreCommitValidation},
			},
		},
	}
}
