package main

import (
	"fmt"
	"path/filepath"
)

// sddSharedScaffoldingJourneys protects the narrow shared OpenSpec allowlist:
// fresh project scaffolding is not another change payload and, after #3417 burns
// the terminal review, SDD continues under ordinary policy without a gate.
func sddSharedScaffoldingJourneys() []Journey {
	return []Journey{{
		ID:     "j107-sdd-approved-active-change-allows-shared-openspec-scaffolding",
		Review: reviewOptedIn,
		Title:  "#3417: terminal burn leaves shared OpenSpec scaffolding eligible under unmanaged ordinary policy",
		Source: "shared OpenSpec policy under #3417: config.yaml and empty archive/spec roots are infrastructure, not foreign payload; burned review leaves no durable authority or gate",
		Steps: append(sddBurnedAuthoritySteps(sddSharedScaffoldingAuthorityFixture),
			Step{Name: "sdd-status keeps shared OpenSpec scaffolding archive-ready under unmanaged ordinary policy", Requires: sddStatusCapability,
				Args: productArgs("sdd-status", sddChange, "--json"), After: sddStatusAssertion("shared OpenSpec scaffolding", requireSDDUnmanagedOrdinaryArchive("shared OpenSpec scaffolding"))},
		),
	}}
}

func requireSDDUnmanagedOrdinaryArchive(label string) func(sddStatusV2) error {
	return func(status sddStatusV2) error {
		if status.Dependencies.Archive != "ready" || status.NextRecommended != "archive" {
			return fmt.Errorf("%s status = %+v, want ordinary archive readiness", label, status)
		}
		return nil
	}
}

func sddSharedScaffoldingAuthorityFixture(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	if err := sddStageAuthorityChange(sandbox, false); err != nil {
		return err
	}
	for path, content := range map[string]string{
		filepath.Join(sandbox.Repo, "openspec", "config.yaml"):                    "schema: specification\n",
		filepath.Join(sandbox.Repo, "openspec", "changes", "archive", ".gitkeep"): "",
		filepath.Join(sandbox.Repo, "openspec", "specs", ".gitkeep"):              "",
		filepath.Join(sddChangeRoot(sandbox), "verify-report.md"):                 sddVerifyReport,
	} {
		if err := sandbox.write(path, content); err != nil {
			return err
		}
	}
	if err := sandbox.git(sandbox.Repo, "add", "openspec"); err != nil {
		return err
	}
	return sddFixtureStart(sandbox, sddNewerAuthorityLineage)
}
