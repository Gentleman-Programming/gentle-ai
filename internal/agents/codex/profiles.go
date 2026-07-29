package codex

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// ProfileAssignment holds the resolved model and reasoning_effort for a
// single Codex SDD profile (one of the three carriles: sdd-strong, sdd-mid,
// sdd-cheap). This is the contract between the inject call site (which knows
// about the user's selection) and WriteCodexProfiles (which knows about TOML).
type ProfileAssignment struct {
	Profile         string // e.g. "sdd-strong"
	Model           string // e.g. "gpt-5.6-sol"
	ReasoningEffort string // e.g. "high"
}

// defaultProfileAssignments derives the canonical default assignments from
// model.CodexTierGroups() so that WriteCodexProfiles(dir, nil) and
// resolveProfileAssignments(nil, nil) agree on the Recommended preset values:
//
//	sdd-strong: high
//	sdd-mid:    medium
//	sdd-cheap:  low
func defaultProfileAssignments() []ProfileAssignment {
	tiers := model.CodexTierGroups()
	out := make([]ProfileAssignment, 0, len(tiers))
	for _, t := range tiers {
		out = append(out, ProfileAssignment{
			Profile:         t.Profile,
			Model:           t.Model,
			ReasoningEffort: string(t.DefaultEffort),
		})
	}
	return out
}

// readProfileFileOrEmpty returns the file content as a string, or "" if the
// file does not exist. Any other error is returned as-is.
func readProfileFileOrEmpty(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// SddProfilePaths returns the absolute paths of all gentle-ai SDD profile
// files that WriteCodexProfiles would write into codexHomeDir. Useful for
// uninstall path tracking without re-running the write.
func SddProfilePaths(codexHomeDir string) []string {
	defaults := defaultProfileAssignments()
	paths := make([]string, 0, len(defaults))
	for _, p := range defaults {
		paths = append(paths, filepath.Join(codexHomeDir, p.Profile+".config.toml"))
	}
	return paths
}

// WriteCodexProfiles writes the three gentle-ai SDD profile files into the
// given Codex home directory (~/.codex). Each profile file contains both a
// model key and a model_reasoning_effort key set to the resolved tier values.
//
// When assignments is nil or empty, canonical Recommended defaults are used
// (derived from model.CodexTierGroups()):
//   - sdd-strong: model=gpt-5.6-sol, model_reasoning_effort=medium
//   - sdd-mid:    model=gpt-5.6-terra, model_reasoning_effort=high
//   - sdd-cheap:  model=gpt-5.6-luna, model_reasoning_effort=high
//
// Profile files are written idempotently using UpsertTopLevelTOMLString +
// WriteFileAtomic — re-running this function when files already contain the
// canonical values produces changed=false and leaves the files unchanged.
//
// Returns (changed, files, err) where changed is true if at least one file
// was written or modified, and files is the list of profile paths written.
func WriteCodexProfiles(codexHomeDir string, assignments []ProfileAssignment) (changed bool, files []string, err error) {
	if len(assignments) == 0 {
		assignments = defaultProfileAssignments()
	}

	for _, a := range assignments {
		path := filepath.Join(codexHomeDir, a.Profile+".config.toml")

		existing, readErr := readProfileFileOrEmpty(path)
		if readErr != nil {
			return false, nil, readErr
		}

		// Write model first, then model_reasoning_effort. Each UpsertTopLevelTOMLString
		// call is idempotent: existing value unchanged = no re-write.
		content := filemerge.UpsertTopLevelTOMLString(existing, "model", a.Model)
		content = filemerge.UpsertTopLevelTOMLString(content, "model_reasoning_effort", a.ReasoningEffort)

		writeResult, writeErr := filemerge.WriteFileAtomic(path, []byte(content), 0o644)
		if writeErr != nil {
			return false, nil, writeErr
		}

		changed = changed || writeResult.Changed
		files = append(files, path)
	}

	return changed, files, nil
}
