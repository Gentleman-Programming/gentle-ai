package agent

import "path/filepath"

// artifactTypeAliases maps a canonical artifact filename (design decision
// D5, filename-driven derivation, no `type:` frontmatter field) to the
// contract artifact type it derives. Two entries are NOT a literal
// filename-minus-extension match:
//   - "tasks.md" -> "task" (plural file, singular type; see
//     defaultInputsForAgent's "task" entries and dev_task_planner's Writes).
//   - "explore.md" -> "exploration" (intent.Router writes exactly
//     "explore.md" -- see intent/router.go's artifactName selection --
//     while the contract type is spelled out as "exploration").
//
// Any filename outside this table derives to "" (unclassified). That is an
// accepted risk, not a bug: an unclassified artifact skips the
// AllowedArtifactTypes enforcement gate entirely rather than being refused,
// and no migration of existing openspec/changes/ artifacts is performed to
// force them into this table.
var artifactTypeAliases = map[string]string{
	"proposal.md":  "proposal",
	"spec.md":      "spec",
	"design.md":    "design",
	"tasks.md":     "task",
	"explore.md":   "exploration",
	"blueprint.md": "blueprint",
}

// DeriveArtifactType derives an artifact's contract type from its filename
// (the base name only -- any leading directory path is ignored). A
// non-canonical filename, or an empty path, returns "" (unclassified).
func DeriveArtifactType(artifactPath string) string {
	if artifactPath == "" {
		return ""
	}
	return artifactTypeAliases[filepath.Base(artifactPath)]
}
