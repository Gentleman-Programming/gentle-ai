package main

import (
	"fmt"
	"path/filepath"
)

// sddSharedScaffoldingJourneys protects the narrow shared OpenSpec allowlist:
// fresh project scaffolding is not another change payload and must not shadow
// an approved authority bound to the active change.
func sddSharedScaffoldingJourneys() []Journey {
	return []Journey{{
		ID:     "j107-sdd-approved-active-change-allows-shared-openspec-scaffolding",
		Review: reviewOptedIn,
		Title:  "Approved active change remains eligible with exact fresh OpenSpec scaffolding",
		Source: "shared OpenSpec review-gate policy: config.yaml and empty archive/spec roots are infrastructure, not foreign change payload",
		Steps: append(sddApprovedAuthoritySteps(sddSharedScaffoldingAuthorityFixture),
			Step{Name: "sdd-status preserves the approved active-change review gate", Requires: sddStatusCapability,
				Args: productArgs("sdd-status", sddChange, "--json"), After: sddStatusAssertion("shared OpenSpec scaffolding approval", func(status sddStatusV1) error {
					if status.ReviewGate == nil || status.ReviewGate.Result != "allow" || status.Dependencies.Archive == "blocked" || status.NextRecommended == "resolve-review" {
						return fmt.Errorf("shared scaffolding status = %+v", status)
					}
					return nil
				})},
		),
	}}
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
