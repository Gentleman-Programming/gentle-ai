package sddstatus

import (
	"strings"
	"testing"
)

// #3814 artifact-locator collapse: phase instructions must name the locators
// the native surface already resolved for the active artifact store, never a
// store-specific filename. Before this change renderPhaseInstructions
// hardcoded "tasks.md" and "verify-report.md" regardless of ArtifactStore,
// which is one half of the disagreement that leaves an Engram-mode or
// hybrid-mode phase actor with no resolvable input.

func engramLocatorStatus() Status {
	change := "locator-collapse"
	return Status{
		ChangeName:    &change,
		ArtifactStore: ArtifactStoreEngram,
		ArtifactPaths: ArtifactPaths{
			Proposal:      []string{"sdd/locator-collapse/proposal"},
			Specs:         []string{"sdd/locator-collapse/spec"},
			Design:        []string{"sdd/locator-collapse/design"},
			Tasks:         []string{"sdd/locator-collapse/tasks"},
			ApplyProgress: []string{"sdd/locator-collapse/apply-progress"},
			VerifyReport:  []string{"sdd/locator-collapse/verify-report"},
		},
	}
}

// TestPhaseInstructionsNameResolvedLocatorsForEngramStore pins that apply and
// archive instructions carry the resolved Engram topic keys and never the
// OpenSpec filenames.
func TestPhaseInstructionsNameResolvedLocatorsForEngramStore(t *testing.T) {
	instructions := renderPhaseInstructions(engramLocatorStatus())

	apply := strings.Join(instructions.Apply, "\n")
	if !strings.Contains(apply, "sdd/locator-collapse/tasks") {
		t.Errorf("apply instructions omit the resolved tasks locator:\n%s", apply)
	}
	if strings.Contains(apply, "tasks.md") {
		t.Errorf("apply instructions name the OpenSpec filename under the Engram store:\n%s", apply)
	}

	archive := strings.Join(instructions.Archive, "\n")
	if !strings.Contains(archive, "sdd/locator-collapse/verify-report") {
		t.Errorf("archive instructions omit the resolved verify-report locator:\n%s", archive)
	}
	if strings.Contains(archive, "verify-report.md") {
		t.Errorf("archive instructions name the OpenSpec filename under the Engram store:\n%s", archive)
	}
}

// TestPhaseInstructionsNameResolvedLocatorsForOpenSpecStore pins the same
// contract from the other side: under OpenSpec the instructions carry the
// resolved paths the native surface produced, not a restated constant.
func TestPhaseInstructionsNameResolvedLocatorsForOpenSpecStore(t *testing.T) {
	change := "locator-collapse"
	status := Status{
		ChangeName:    &change,
		ArtifactStore: ArtifactStoreOpenSpec,
		ArtifactPaths: ArtifactPaths{
			Tasks:        []string{"openspec/changes/locator-collapse/tasks.md"},
			VerifyReport: []string{"openspec/changes/locator-collapse/verify-report.md"},
		},
	}

	instructions := renderPhaseInstructions(status)

	apply := strings.Join(instructions.Apply, "\n")
	if !strings.Contains(apply, "openspec/changes/locator-collapse/tasks.md") {
		t.Errorf("apply instructions omit the resolved tasks locator:\n%s", apply)
	}

	archive := strings.Join(instructions.Archive, "\n")
	if !strings.Contains(archive, "openspec/changes/locator-collapse/verify-report.md") {
		t.Errorf("archive instructions omit the resolved verify-report locator:\n%s", archive)
	}
}
