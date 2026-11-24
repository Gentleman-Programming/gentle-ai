package engram

import (
	"github.com/gentleman-programming/gentle-ai/v5/internal/installcmd"
	"github.com/gentleman-programming/gentle-ai/v5/internal/model"
	"github.com/gentleman-programming/gentle-ai/v5/internal/system"
)

func InstallCommand(profile system.PlatformProfile) ([][]string, error) {
	return installcmd.NewResolver().ResolveComponentInstall(profile, model.ComponentEngram)
}
