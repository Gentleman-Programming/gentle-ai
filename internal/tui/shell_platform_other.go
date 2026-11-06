//go:build !windows

package tui

import tea "github.com/charmbracelet/bubbletea"

// shellPlatformConfigure keeps every detected target on Unix platforms.
func shellPlatformConfigure(config shellExperienceConfig, _ tea.Msg) (shellExperienceConfig, bool) {
	return config, false
}
