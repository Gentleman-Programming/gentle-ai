package tui

import (
	"errors"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/screens"
)

// shellPlatformTargetNote is the Windows installer refusal for every target
// except Separate (shellinstaller.InspectUserInstall), shown on the target row
// so the choice is disabled before Continue instead of refused at inspection.
// Windows Pi detection reports the same text, so the first render names it.
const shellPlatformTargetNote = shellinstaller.UserWindowsSeparateOnly

// shellPlatformConfigure disables the existing-Pi targets on Windows: no agent
// is offered for reuse (a managed Separate install's private agent is not a
// personal Pi), the row carries the Windows reason, and choosing the row is
// refused with that reason while the selection stays Separate.
func shellPlatformConfigure(config shellExperienceConfig, msg tea.Msg) (shellExperienceConfig, bool) {
	if config.mode == "separate" {
		config.agent = ""
	}
	config.piFound, config.piNote = false, shellPlatformTargetNote
	key, ok := msg.(tea.KeyMsg)
	if !ok || key.String() != "enter" {
		return config, false
	}
	targets := screens.ShellInstallTargetEntries(config.mode, config.piFound, config.piNote, "", config.existing, config.reconfigure)
	uninstall := config.reconfigure && (config.mode == "separate" || config.mode == "agent")
	options := screens.ShellExperienceOptionsWithUninstall(config.profile, uninstall, targets...)
	entry := options[min(max(0, config.cursor), len(options)-1)]
	if entry.Control != screens.ShellExperienceInstallTarget || (entry.Choice != "agent" && entry.Choice != "shared") {
		return config, false
	}
	config.err = errors.New(shellPlatformTargetNote)
	return config, true
}
