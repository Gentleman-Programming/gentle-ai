package gga

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v2/internal/installcmd"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

func InstallCommand(profile system.PlatformProfile) ([][]string, error) {
	return installcmd.NewResolver().ResolveComponentInstall(profile, model.ComponentGGA)
}

func CleanupInstallDir() error {
	return cleanupInstallDir(filepath.Join(os.TempDir(), "gentleman-guardian-angel"))
}

func cleanupInstallDir(path string) error {
	return cleanupInstallDirWith(system.NewPowerShellRunner(), path)
}

func cleanupInstallDirWith(runner system.PowerShellRunner, path string) error {
	safePath := system.PowerShellSingleQuoted(path)
	script := fmt.Sprintf("$ErrorActionPreference = 'Stop'; if (Test-Path -LiteralPath '%s') { Remove-Item -Recurse -Force -LiteralPath '%s' }", safePath, safePath)
	if _, err := runner.Run(context.Background(), "-NoProfile", "-NonInteractive", "-Command", script); err != nil {
		return fmt.Errorf("clean GGA install directory %q: %w", path, err)
	}
	return nil
}

func ShouldInstall(enabled bool) bool {
	return enabled
}
