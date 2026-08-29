package main

import (
	"fmt"
	"path/filepath"
)

// sddSharedScaffoldingJourneys protects the narrow shared OpenSpec allowlist:
// fresh project scaffolding is not another change payload and retained review
// authority is not selected by SDD archive routing.
func sddSharedScaffoldingJourneys() []Journey {
	return []Journey{{
		ID:     "j107-sdd-approved-active-change-allows-shared-openspec-scaffolding",
		Review: reviewOptedIn,
		Title:  "#3867: retained review authority leaves shared OpenSpec scaffolding under unmanaged archive policy",
		Source: "shared OpenSpec policy under #3867: config.yaml and empty archive/spec roots are infrastructure, not foreign payload; SDD archive does not select pre-commit authority",
		Steps: append(sddAcknowledgedAuthoritySteps(sddSharedScaffoldingAuthorityFixture),
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
