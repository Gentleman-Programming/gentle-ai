package pi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

func (a *Adapter) DetectPiSubagents(homeDir string, workspaceDir string) (bool, error) {
	// Prefer authoritative PI settings package declarations first.
	// Legacy footprint checks remain as a fallback for older layouts.
	for _, settingsPath := range a.piSettingsPaths(homeDir, workspaceDir) {
		detected, err := detectPiSubagentsFromSettings(settingsPath)
		if err != nil {
			return false, err
		}

		if detected {
			return true, nil
		}
	}

	for _, footprint := range a.PiSubagentsFootprints(homeDir, workspaceDir) {
		stat := a.detectStat(footprint)
		if stat.err == nil {
			return true, nil
		}

		if isNotExist(stat.err) {
			continue
		}

		return false, stat.err
	}

	return false, nil
}

func (a *Adapter) PiSubagentsFootprints(homeDir string, workspaceDir string) []string {
	paths := ResolvePaths(homeDir, os.Getenv)
	footprints := []string{
		filepath.Join(paths.Root, "extensions", "pi-subagents"),
		filepath.Join(paths.Root, "extensions", "pi-subagents.json"),
		filepath.Join(paths.LegacyConfigDir, "extensions", "pi-subagents"),
		filepath.Join(paths.LegacyConfigDir, "extensions", "pi-subagents.json"),
	}
	for _, base := range workspaceCandidates(workspaceDir) {
		footprints = append(footprints,
			filepath.Join(base, ".pi", "extensions", "pi-subagents"),
			filepath.Join(base, ".pi", "extensions", "pi-subagents.json"),
		)
	}
	return footprints
}

func (a *Adapter) detectStat(path string) statResult {
	if a != nil && a.statPath != nil {
		return a.statPath(path)
	}

	return defaultStat(path)
}

func isNotExist(err error) bool {
	return os.IsNotExist(err)
}

func (a *Adapter) piSettingsPaths(homeDir string, workspaceDir string) []string {
	paths := ResolvePaths(homeDir, os.Getenv)
	settings := []string{
		paths.SettingsPath,
	}
	for _, base := range workspaceCandidates(workspaceDir) {
		settings = append(settings, filepath.Join(base, ".pi", "settings.json"))
	}
	return settings
}

func workspaceCandidates(workspaceDir string) []string {
	if strings.TrimSpace(workspaceDir) == "" {
		return nil
	}
	out := make([]string, 0, 8)
	seen := map[string]struct{}{}
	current := filepath.Clean(workspaceDir)
	for i := 0; i < 64; i++ {
		if _, ok := seen[current]; !ok {
			out = append(out, current)
			seen[current] = struct{}{}
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return out
}

type piSettings struct {
	Packages []string `json:"packages"`
}

func detectPiSubagentsFromSettings(path string) (bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if isNotExist(err) {
			return false, nil
		}

		return false, err
	}

	var settings piSettings
	if err := json.Unmarshal(content, &settings); err != nil {
		return false, err
	}

	for _, spec := range settings.Packages {
		if packageSpecName(spec) == "pi-subagents" {
			return true, nil
		}
	}

	return false, nil
}

func packageSpecName(spec string) string {
	normalized := strings.TrimSpace(spec)
	normalized = strings.TrimPrefix(normalized, "npm:")

	if strings.HasPrefix(normalized, "@") {
		lastAt := strings.LastIndex(normalized, "@")
		slash := strings.Index(normalized, "/")
		if lastAt > slash {
			normalized = normalized[:lastAt]
		}

		return normalized
	}

	if at := strings.Index(normalized, "@"); at > 0 {
		normalized = normalized[:at]
	}

	return normalized
}
