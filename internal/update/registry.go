package update

import (
	"path/filepath"
)

// Tools is the static registry of managed tools that can be checked for updates.
//
// InstallMethod controls which upgrade strategy the executor uses:
//   - InstallBrew: managed via homebrew (macOS/Linux with brew)
//   - InstallGoInstall: installed via `go install <GoImportPath>@version`
//   - InstallBinary: downloaded binary from GitHub Releases (atomic replace)
//
// For brew-managed platforms the executor picks brew regardless of the
// field here; InstallMethod represents the non-brew fallback strategy.
var Tools = []ToolInfo{
	{
		Name:          "gentle-ai",
		Owner:         "Gentleman-Programming",
		Repo:          "gentle-ai",
		DetectCmd:     nil, // version comes from build-time ldflags (app.Version)
		VersionPrefix: "v",
		// gentle-ai: brew on macOS, binary release download on Linux.
		// Windows self-upgrade uses the PowerShell installer so the running binary can exit before replacement.
		InstallMethod: InstallBinary,
	},
	{
		Name:              "engram",
		Owner:             "Gentleman-Programming",
		Repo:              "engram",
		DetectCmd:         []string{"engram", "version"},
		VersionPrefix:     "v",
		ReleaseTagPattern: `^v[0-9]+\.[0-9]+\.[0-9]+$`,
		// engram: brew on macOS/Linux-brew, binary download elsewhere.
		InstallMethod: InstallBinary,
		// FallbackPaths covers the Windows stale-PATH scenario (and Linux ~/.local/bin
		// when not yet in PATH): AddToUserPath updates the registry/profile but the
		// current process does not see the change until a new shell session starts.
		FallbackPaths: func(homeDir, localAppData string) []string {
			var paths []string
			// Windows: %LOCALAPPDATA%\engram\bin\engram.exe
			if localAppData != "" {
				paths = append(paths, filepath.Join(localAppData, "engram", "bin", "engram.exe"))
			} else if homeDir != "" {
				// LOCALAPPDATA is not set (e.g. restricted environment or CI on Windows).
				// Derive the standard path from homeDir for parity with the installer.
				paths = append(paths, filepath.Join(homeDir, "AppData", "Local", "engram", "bin", "engram.exe"))
			}
			// Linux/macOS: ~/.local/bin/engram (when /usr/local/bin is not writable,
			// the binary installer places it here, which may not be in PATH yet).
			if homeDir != "" {
				paths = append(paths, filepath.Join(homeDir, ".local", "bin", "engram"))
			}
			return paths
		},
	},
	{
		Name:          "gga",
		Owner:         "Gentleman-Programming",
		Repo:          "gentleman-guardian-angel",
		DetectCmd:     []string{"gga", "--version"},
		VersionPrefix: "v",
		// gga: brew on macOS, install.sh script on Linux/Windows.
		// GGA does not publish pre-built release binary assets — only source archives.
		// Using InstallScript runs curl | bash via the project's install.sh.
		InstallMethod: InstallScript,
		// FallbackPaths covers the Windows stale-PATH scenario: gga installs a
		// PowerShell shim to ~/bin/gga.ps1, and the bash script to ~/.local/bin/gga.
		// Both locations may not be in PATH immediately after install.
		FallbackPaths: func(homeDir, localAppData string) []string {
			var paths []string
			if homeDir != "" {
				// Windows: ~/bin/gga.ps1 (PowerShell shim, callable as "gga" in PS)
				paths = append(paths, filepath.Join(homeDir, "bin", "gga.ps1"))
				// Linux/macOS: ~/.local/bin/gga
				paths = append(paths, filepath.Join(homeDir, ".local", "bin", "gga"))
				// Linux/macOS: ~/bin/gga
				paths = append(paths, filepath.Join(homeDir, "bin", "gga"))
			}
			return paths
		},
	},
	{
		Name:          "opencode-subagent-statusline",
		Owner:         "Joaquinvesapa",
		Repo:          "sub-agent-statusline",
		VersionPrefix: "v",
		InstallMethod: InstallOpenCodePlugin,
		NpmPackage:    "opencode-subagent-statusline",
	},
	{
		Name:          "opencode-sdd-engram-manage",
		Owner:         "j0k3r-dev-rgl",
		Repo:          "sdd-engram-plugin",
		VersionPrefix: "v",
		InstallMethod: InstallOpenCodePlugin,
		NpmPackage:    "opencode-sdd-engram-manage",
	},
}
