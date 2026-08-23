package opencode

import (
	opencodeconfig "github.com/gentleman-programming/gentle-ai/v2/internal/opencode"
)

func ConfigPath(homeDir string) string {
	return opencodeconfig.ConfigPath(homeDir)
}
