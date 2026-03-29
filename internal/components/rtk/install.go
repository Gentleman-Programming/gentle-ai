package rtk

import (
	"github.com/gentleman-programming/gentle-ai/internal/installcmd"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

// InstallCommand returns the command sequence to install RTK on the given platform.
func InstallCommand(profile system.PlatformProfile) ([][]string, error) {
	return installcmd.NewResolver().ResolveComponentInstall(profile, model.ComponentRTK)
}

// ShouldInstall returns whether RTK should be installed based on user selection.
func ShouldInstall(enabled bool) bool {
	return enabled
}
