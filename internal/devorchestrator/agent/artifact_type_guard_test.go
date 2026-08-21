package agent

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
)

// canonicalArtifactTypes is the closed set of artifact types
// DeriveArtifactType can ever produce, sourced independently from the
// artifactTypeAliases table's values so a coding mistake in that table
// (e.g. producing "tasks" instead of "task") is caught here rather than
// silently accepted by both sides of the same map. See design decision D5.
var canonicalArtifactTypes = map[string]bool{
	"proposal":    true,
	"spec":        true,
	"design":      true,
	"task":        true,
	"exploration": true,
	"blueprint":   true,
}

// TestAgentContractArtifactTypesAreDerivable guards H-08's artifact-type
// enforcement the same way H-07's fix guards skill lookups: a contract must
// never declare a canonical, filename-representable artifact type that
// DeriveArtifactType can never actually produce, or the enforcement gate
// would silently refuse every dispatch of that type forever (the same
// defect class as a dangling skill name).
//
// This intentionally does NOT require every AllowedArtifactTypes value to
// be derivable -- some declared inputs (e.g. "requirement", "bug",
// "feature", "diff", "test-results", "db-assessment") describe raw,
// pre-artifact concepts with no canonical filename anywhere in this
// workspace (verified: no requirement.md/bug.md/feature.md/diff.md/
// test-results.*/db-assessment.md convention exists), so they are never fed
// through DeriveArtifactType at all -- primaryArtifact is either empty
// (unclassified, gate skipped) or one of the six canonical filenames for
// those calls. The guard is scoped to exactly the subset this change's
// enforcement gate can ever observe: the six canonical, filename-derivable
// types. A contract declaring one of THOSE six with a value
// DeriveArtifactType cannot reproduce is the real dangling-reference risk.
func TestAgentContractArtifactTypesAreDerivable(t *testing.T) {
	registry, err := LoadRegistryFromFS(assets.FS, "claude/agents")
	if err != nil {
		t.Fatalf("LoadRegistryFromFS() error = %v", err)
	}
	if len(registry) == 0 {
		t.Fatal("LoadRegistryFromFS() returned an empty registry")
	}

	produced := map[string]bool{}
	for filename := range artifactTypeAliases {
		produced[DeriveArtifactType(filename)] = true
	}

	for agentName, contract := range registry {
		for _, declared := range contract.Inputs.AllowedArtifactTypes {
			if !canonicalArtifactTypes[declared] {
				continue // raw/pre-artifact concept, not filename-derivable by design
			}
			if !produced[declared] {
				t.Errorf("agent %q declares canonical AllowedArtifactTypes %q, but no filename in the alias table derives it -- dangling type reference", agentName, declared)
			}
		}
	}
}
