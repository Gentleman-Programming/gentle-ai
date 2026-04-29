package pi

import (
	"path/filepath"
	"strings"
)

const piCodingAgentDirEnv = "PI_CODING_AGENT_DIR"

type Paths struct {
	Root            string
	AgentsDir       string
	SettingsPath    string
	LegacyConfigDir string
}

func ResolvePaths(homeDir string, getenv func(string) string) Paths {
	root := strings.TrimSpace(getenv(piCodingAgentDirEnv))
	if root == "" {
		root = filepath.Join(homeDir, ".pi", "agent")
	}

	legacy := filepath.Join(homeDir, ".config", "pi-coding-agent")

	return Paths{
		Root:            root,
		AgentsDir:       filepath.Join(root, "agents"),
		SettingsPath:    filepath.Join(root, "settings.json"),
		LegacyConfigDir: legacy,
	}
}

func detectionConfigCandidates(paths Paths) []string {
	return []string{paths.Root, paths.LegacyConfigDir}
}
