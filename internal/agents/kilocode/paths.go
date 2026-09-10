package kilocode

import (
	"os"
	"path/filepath"
)

var candidateConfigFiles = []string{
	"kilo.jsonc",
	"kilo.json",
	"opencode.jsonc",
	"opencode.json",
	"config.json",
}

const defaultConfigFile = "kilo.jsonc"

func ConfigPath(homeDir string) string {
	return filepath.Join(homeDir, ".config", "kilo")
}

// ResolveConfigPath locates the effective global configuration file for Kilocode.
// It selects the first existing regular file in candidateConfigFiles precedence order,
// defaulting to kilo.jsonc if none exists.
func ResolveConfigPath(homeDir string) string {
	configDir := ConfigPath(homeDir)
	for _, candidate := range candidateConfigFiles {
		candidatePath := filepath.Join(configDir, candidate)
		if regularFileExists(candidatePath) {
			return candidatePath
		}
	}
	return filepath.Join(configDir, defaultConfigFile)
}

func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
