package copilotconfig

import (
	"os"
	"path/filepath"
)

func Root(homeDir string) string {
	if root := os.Getenv("COPILOT_HOME"); root != "" {
		return root
	}
	return filepath.Join(homeDir, ".copilot")
}
