package codex

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
)

// codexProfile maps a profile filename to its model_reasoning_effort value.
// These profiles use the separate-file mechanism introduced in Codex >= 0.134.0:
// each file lives at ~/.codex/<name>.config.toml and is selected at runtime via
// `codex --profile <name>`. The model_reasoning_effort key is model-agnostic —
// Codex maps the effort tier to the appropriate model at runtime, so we do NOT
// hardcode any OpenAI model ID.
//
// Tier mapping to SDD Model Assignments:
//   - sdd-strong  xhigh  propose, design, verify, judge phases
//   - sdd-mid     high   spec, tasks, apply phases
//   - sdd-cheap   low    explore, archive, onboard phases
type codexProfile struct {
	filename        string
	reasoningEffort string
}

var gentleAIProfiles = []codexProfile{
	{filename: "sdd-strong.config.toml", reasoningEffort: "xhigh"},
	{filename: "sdd-mid.config.toml", reasoningEffort: "high"},
	{filename: "sdd-cheap.config.toml", reasoningEffort: "low"},
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
	paths := make([]string, 0, len(gentleAIProfiles))
	for _, p := range gentleAIProfiles {
		paths = append(paths, filepath.Join(codexHomeDir, p.filename))
	}
	return paths
}

// WriteCodexProfiles writes the three gentle-ai SDD profile files into the
// given Codex home directory (~/.codex). Each profile file contains a
// model_reasoning_effort key set to the appropriate SDD tier.
//
// Profile files are written idempotently using UpsertTopLevelTOMLString +
// WriteFileAtomic — re-running this function when files already contain the
// canonical values produces changed=false and leaves the files unchanged.
//
// Returns (changed, files, err) where changed is true if at least one file
// was written or modified, and files is the list of profile paths written.
func WriteCodexProfiles(codexHomeDir string) (changed bool, files []string, err error) {
	for _, p := range gentleAIProfiles {
		path := filepath.Join(codexHomeDir, p.filename)

		existing, readErr := readProfileFileOrEmpty(path)
		if readErr != nil {
			return false, nil, readErr
		}

		content := filemerge.UpsertTopLevelTOMLString(existing, "model_reasoning_effort", p.reasoningEffort)

		writeResult, writeErr := filemerge.WriteFileAtomic(path, []byte(content), 0o644)
		if writeErr != nil {
			return false, nil, writeErr
		}

		changed = changed || writeResult.Changed
		files = append(files, path)
	}

	return changed, files, nil
}
