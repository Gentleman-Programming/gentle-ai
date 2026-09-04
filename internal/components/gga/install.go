package gga

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/installcmd"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

func InstallCommand(profile system.PlatformProfile) ([][]string, error) {
	return installcmd.NewResolver().ResolveComponentInstall(profile, model.ComponentGGA)
}

// CommandOutputRunner queries a package manager without changing its state.
type CommandOutputRunner interface {
	Output(name string, args ...string) ([]byte, error)
}

// RoutePathQueryError preserves the exact path whose state could not be observed.
type RoutePathQueryError struct {
	Path string
	Err  error
}

// RouteQueryResult reports exact script paths separately from Homebrew formula state.
type RouteQueryResult struct {
	PresentPaths   []string
	UnknownPaths   []RoutePathQueryError
	FormulaPresent bool
}

// ExternalInstallPathPresence reports a pre-install observation for one exact path.
type ExternalInstallPathPresence struct {
	Path    string
	Present bool
}

// ExternalInstallPaths returns only the exact destinations created by the upstream
// GGA script. It never searches PATH and excludes Gentle-AI-managed shims.
func ExternalInstallPaths(goos, homeDir string) []string {
	switch goos {
	case "linux":
		return []string{
			"/usr/local/bin/gga",
			filepath.Join(homeDir, ".local", "bin", "gga"),
		}
	case "windows":
		return []string{
			filepath.Join(homeDir, "bin", "gga"),
			filepath.Join(homeDir, "bin", "gga.bat"),
		}
	default:
		return nil
	}
}

// ObserveExternalInstallPathPresence records each possible script destination
// before installation. An Lstat failure other than NotExist aborts observation.
func ObserveExternalInstallPathPresence(goos, homeDir string) ([]ExternalInstallPathPresence, error) {
	return observeExternalInstallPathPresence(goos, homeDir, os.Lstat)
}

func observeExternalInstallPathPresence(goos, homeDir string, lstat func(string) (os.FileInfo, error)) ([]ExternalInstallPathPresence, error) {
	paths := ExternalInstallPaths(goos, homeDir)
	presence := make([]ExternalInstallPathPresence, 0, len(paths))
	for _, path := range paths {
		_, err := lstat(path)
		switch {
		case err == nil:
			presence = append(presence, ExternalInstallPathPresence{Path: path, Present: true})
		case os.IsNotExist(err):
			presence = append(presence, ExternalInstallPathPresence{Path: path})
		default:
			return nil, fmt.Errorf("lstat GGA script install candidate %q: %w", path, err)
		}
	}
	return presence, nil
}

// QueryExternalRoute queries only the exact recorded external route. Script paths
// use Lstat so symlinks are reported as paths rather than followed.
func QueryExternalRoute(goos, route string, paths []string, runner CommandOutputRunner) (RouteQueryResult, error) {
	return queryExternalRoute(goos, route, paths, runner, os.Lstat)
}

func queryExternalRoute(goos, route string, paths []string, runner CommandOutputRunner, lstat func(string) (os.FileInfo, error)) (RouteQueryResult, error) {
	result := RouteQueryResult{}
	switch route {
	case "script":
		if goos != "linux" && goos != "windows" {
			// refusal:by-design unsupported-route: unsupported journal provenance cannot be queried safely.
			return result, fmt.Errorf("recorded GGA script route is unsupported on %s; preserve the record and use a supported script route", goos)
		}
		for _, path := range paths {
			_, err := lstat(path)
			switch {
			case err == nil:
				result.PresentPaths = append(result.PresentPaths, path)
			case os.IsNotExist(err):
				// Absent paths remain explicitly represented by omission from PresentPaths.
			default:
				result.UnknownPaths = append(result.UnknownPaths, RoutePathQueryError{Path: path, Err: err})
			}
		}
		return result, nil
	case "brew":
		if goos != "darwin" {
			// refusal:by-design unsupported-route: unsupported journal provenance cannot be queried safely.
			return result, fmt.Errorf("recorded Homebrew GGA route is unsupported on %s; preserve the record and use a darwin brew route", goos)
		}
		if runner == nil {
			return result, fmt.Errorf("query recorded Homebrew GGA formula requires an output-capable command runner; retry `brew list --formula`")
		}
		installed, err := runner.Output("brew", "list", "--formula")
		if err != nil {
			return result, fmt.Errorf("query recorded GGA formula with brew list --formula: %w; retry `brew list --formula`", err)
		}
		result.FormulaPresent = brewFormulaInstalled(installed, "gga")
		return result, nil
	default:
		// refusal:by-design unknown-route: unknown journal provenance cannot be queried safely.
		return result, fmt.Errorf("unknown recorded GGA route %q; preserve the record and use script or brew provenance", route)
	}
}

func brewFormulaInstalled(output []byte, formula string) bool {
	for _, line := range strings.Split(string(output), "\n") {
		if strings.TrimSpace(line) == formula {
			return true
		}
	}
	return false
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
