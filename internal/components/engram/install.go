package engram

import (
	"github.com/gentleman-programming/gentle-ai/v2/internal/installcmd"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

func InstallCommand(profile system.PlatformProfile) ([][]string, error) {
	return installcmd.NewResolver().ResolveComponentInstall(profile, model.ComponentEngram)
}
