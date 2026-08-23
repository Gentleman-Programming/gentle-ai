package main

import (
	"fmt"
	"path/filepath"
)

const stagedDeliveryLineage = "staged-validation-terminal-burn"

var stagedDeliveryPaths = []string{"docs/alpha.md", "docs/bravo.md"}

func prepareWorkspaceDeliveryBaseline(sandbox *Sandbox) error {
	if err := sandbox.write(filepath.Join(sandbox.Repo, stagedDeliveryPaths[0]), "baseline alpha\n"); err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(sandbox.Repo, stagedDeliveryPaths[1]), "baseline bravo\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "--", stagedDeliveryPaths[0], stagedDeliveryPaths[1]); err != nil {
		return err
	}
	return sandbox.git(sandbox.Repo, "commit", "-qm", "fixture: add tracked delivery baseline")
}

func stageWorkspaceDeliveryCandidate(sandbox *Sandbox) error {
	if err := sandbox.write(filepath.Join(sandbox.Repo, stagedDeliveryPaths[0]), "# Alpha\n"); err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(sandbox.Repo, stagedDeliveryPaths[1]), "# Bravo\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "--", stagedDeliveryPaths[0], stagedDeliveryPaths[1]); err != nil {
		return err
	}
	paths, err := gitOut(sandbox, sandbox.Repo, "diff", "--cached", "--name-only")
	if err != nil || paths != stagedDeliveryPaths[0]+"\n"+stagedDeliveryPaths[1] {
		return fmt.Errorf("staged workspace candidate paths = %q, %v", paths, err)
	}
	return nil
}

func unstageWorkspaceDeliveryCandidate(sandbox *Sandbox) error {
	return sandbox.git(sandbox.Repo, "reset", "--", stagedDeliveryPaths[0], stagedDeliveryPaths[1])
}

func stageWorkspaceDeliverySubset(sandbox *Sandbox) error {
	return sandbox.git(sandbox.Repo, "add", "--", stagedDeliveryPaths[0])
}

func stageDifferentWorkspaceDeliveryCandidate(sandbox *Sandbox) error {
	if err := sandbox.git(sandbox.Repo, "add", "--", stagedDeliveryPaths[1]); err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(sandbox.Repo, stagedDeliveryPaths[0]), "# different index\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "--", stagedDeliveryPaths[0]); err != nil {
		return err
	}
	return sandbox.write(filepath.Join(sandbox.Repo, stagedDeliveryPaths[0]), "# Alpha\n")
}

func restoreExactWorkspaceDeliveryCandidate(sandbox *Sandbox) error {
	return sandbox.git(sandbox.Repo, "add", "--", stagedDeliveryPaths[0], stagedDeliveryPaths[1])
}

func recordApprovedWorkspaceDeliveryAuthority(r *journeyRun) error {
	status, err := readStatusForContract(r, reviewContractV2, "--lineage", stagedDeliveryLineage, "--gate", "pre-commit")
	if err != nil {
		return err
	}
	if status.Authority.State != "approved" || status.Authority.Revision == "" || len(status.Projection.Paths) != len(stagedDeliveryPaths) {
		return fmt.Errorf("approved workspace receipt status = %#v", status)
	}
	r.sandbox.Scratch["staged-delivery-authority-revision"] = status.Authority.Revision
	return nil
}

func requireStagedDeliveryCandidateStop(r *journeyRun) error {
	status, err := readStatusForContract(r, reviewContractV2, "--lineage", stagedDeliveryLineage, "--gate", "pre-commit")
	if err != nil {
		return err
	}
	if status.Authority.Revision != r.sandbox.Scratch["staged-delivery-authority-revision"] ||
		status.NextTransition.Kind != "stop" || status.NextTransition.ReasonCode != "staged_delivery_candidate_required" ||
		status.NextTransition.Execute.Operation != "" || status.NextTransition.Execute.Command != "" {
		return fmt.Errorf("staged delivery stop = %#v", status)
	}
	return nil
}

func validateExactStagedDeliveryCandidate(r *journeyRun) error {
	status, err := readStatusForContract(r, reviewContractV2,
		"--lineage", stagedDeliveryLineage, "--gate", "pre-commit", "--projection", "staged")
	if err != nil {
		return err
	}
	if status.NextTransition.Kind != "execute" || status.NextTransition.ReasonCode != "approved_receipt_ready" ||
		status.NextTransition.Execute.Operation != "review.validate" || status.NextTransition.Execute.Command == "" {
		return fmt.Errorf("exact staged delivery transition = %#v", status.NextTransition)
	}
	observation, err := runPrintedTransition(r, status)
	if err != nil {
		return err
	}
	return requireGateForLineage(observation, stagedDeliveryLineage, false)
}

func stagedDeliveryJourneys() []Journey {
	return []Journey{{
		ID:     "j89-staged-validation-is-informational-and-unmanaged",
		Review: reviewOptedIn,
		Title:  "#3587: staged validation is informational and unmanaged after the terminal burn",
		Source: "#3587 replaces staged receipt reuse with a terminal event; no staged validation authorizes delivery",
		Steps: []Step{
			{Name: "fixture: repository", Fixture: baseRepo},
			{Name: "fixture: add a second tracked delivery path", Fixture: prepareWorkspaceDeliveryBaseline},
			{Name: "fixture: two-path workspace candidate staged", Fixture: stageWorkspaceDeliveryCandidate},
			{Name: "review the staged workspace candidate; zero lenses close and burn at START", Requires: startNamedCapability, Args: productArgs("review", "start", "--lineage", stagedDeliveryLineage)},
			{Name: "the zero-lens terminal event leaves no staged authority", Requires: statusCapability, Composite: func(r *journeyRun) error {
				return requireAtomicLineageBurned(r, stagedDeliveryLineage)
			}},
			{Name: "fixture: empty index after the terminal burn", Fixture: unstageWorkspaceDeliveryCandidate},
			{Name: "empty index pre-commit validation remains informational and unmanaged", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit"), After: func(_ *Sandbox, observation Observation) error {
				return requireUnmanagedShippedGate(observation, "pre-commit")
			}},
			{Name: "fixture: only one formerly reviewed path staged", Fixture: stageWorkspaceDeliverySubset},
			{Name: "partial index pre-commit validation remains informational and unmanaged", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit"), After: func(_ *Sandbox, observation Observation) error {
				return requireUnmanagedShippedGate(observation, "pre-commit")
			}},
			{Name: "fixture: a different two-path index", Fixture: stageDifferentWorkspaceDeliveryCandidate},
			{Name: "different index pre-commit validation remains informational and unmanaged", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit"), After: func(_ *Sandbox, observation Observation) error {
				return requireUnmanagedShippedGate(observation, "pre-commit")
			}},
		},
	}}
}
