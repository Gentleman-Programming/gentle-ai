package fixer

import (
	"runtime"
	"strings"
)

// Fixer defines the interface for OS-specific remediation command suggestions.
// Fixers only SUGGEST commands, they never execute them.
type Fixer interface {
	Name() string        // Unique name like "darwin:brew-install"
	OS() string          // Target OS: "darwin", "linux", "windows"
	Fixes(checkName string) (FixResult, bool)
	AllFixes() map[string]FixResult
}

// FixResult contains the suggested remediation command and metadata.
type FixResult struct {
	Command       string   // The shell command to run
	Description   string   // Human-readable explanation of what the command does
	RequiresSudo  bool     // Whether the command requires elevated privileges
	Alternatives  []string // Fallback commands if primary fails
	DocsURL       string   // Documentation URL for more info
}

// BaseFixer provides common functionality.
type BaseFixer struct {
	name     string
	os       string
	fixMap   map[string]FixResult
}

// Name returns the fixer identifier.
func (b *BaseFixer) Name() string { return b.name }

// OS returns the target operating system for this fixer.
func (b *BaseFixer) OS() string { return b.os }

// Fixes returns the fix result for the given check name, if available.
func (b *BaseFixer) Fixes(checkName string) (FixResult, bool) {
	fix, ok := b.fixMap[checkName]
	return fix, ok
}

// AllFixes returns the complete map of check names to fix results.
func (b *BaseFixer) AllFixes() map[string]FixResult { return b.fixMap }

// NewBaseFixer creates a base fixer with the given fix map.
func NewBaseFixer(name, os string, fixMap map[string]FixResult) *BaseFixer {
	return &BaseFixer{name: name, os: os, fixMap: fixMap}
}

// FixerRegistry manages fixers by OS and provides lookup by check name.
type FixerRegistry struct {
	fixers map[string]Fixer // key: OS name
}

// NewFixerRegistry creates a new registry with all built-in fixers.
func NewFixerRegistry() *FixerRegistry {
	r := &FixerRegistry{
		fixers: make(map[string]Fixer),
	}
	// Register built-in fixers
	r.fixers["darwin"] = NewDarwinFixer()
	r.fixers["linux"] = NewLinuxFixer()
	r.fixers["windows"] = NewWindowsFixer()
	return r
}

// GetFixes returns the fix result for a specific check on the current OS.
func (r *FixerRegistry) GetFixes(goos, checkName string) (FixResult, bool) {
	fixer, ok := r.fixers[goos]
	if !ok {
		return FixResult{}, false
	}
	return fixer.Fixes(checkName)
}

// GetAllFixesForOS returns all available fixes for an OS.
func (r *FixerRegistry) GetAllFixesForOS(goos string) map[string]FixResult {
	fixer, ok := r.fixers[goos]
	if !ok {
		return nil
	}
	return fixer.AllFixes()
}

// ListOS returns all registered OS targets.
func (r *FixerRegistry) ListOS() []string {
	osList := make([]string, 0, len(r.fixers))
	for os := range r.fixers {
		osList = append(osList, os)
	}
	return osList
}

// CurrentOS returns the runtime GOOS for convenience.
func CurrentOS() string {
	return runtime.GOOS
}

// =============================================================================
// Darwin (macOS) Fixer
// =============================================================================

// DarwinFixer provides macOS-specific remediation commands.
type DarwinFixer struct {
	*BaseFixer
}

// NewDarwinFixer creates the macOS fixer with comprehensive command map.
func NewDarwinFixer() *DarwinFixer {
	return &DarwinFixer{
		BaseFixer: NewBaseFixer("darwin-fixer", "darwin", map[string]FixResult{
			// Package installation
			"brew:install": {
				Command:      "brew install {{package}}",
				Description:  "Install a package via Homebrew",
				RequiresSudo: false,
				Alternatives: []string{"brew upgrade {{package}}", "brew reinstall {{package}}"},
				DocsURL:      "https://docs.brew.sh/Manpage",
			},
			"brew:upgrade": {
				Command:      "brew upgrade {{package}}",
				Description:  "Upgrade a package to the latest version",
				RequiresSudo: false,
				Alternatives: []string{"brew upgrade", "brew reinstall {{package}}"},
				DocsURL:      "https://docs.brew.sh/Manpage",
			},
			"brew:missing": {
				Command:      "brew install {{package}}",
				Description:  "Install missing Homebrew package",
				RequiresSudo: false,
				Alternatives: []string{"brew upgrade {{package}}"},
				DocsURL:      "https://docs.brew.sh/Manpage",
			},

			// Xcode Command Line Tools
			"xcode:cli:missing": {
				Command:      "xcode-select --install",
				Description:  "Install Xcode Command Line Tools",
				RequiresSudo: false,
				Alternatives: []string{"sudo xcodebuild -license accept"},
				DocsURL:      "https://developer.apple.com/download/more/",
			},
			"xcode:license": {
				Command:      "sudo xcodebuild -license accept",
				Description:  "Accept Xcode license agreement",
				RequiresSudo: true,
				Alternatives: []string{"xcodebuild -license"},
				DocsURL:      "https://developer.apple.com/documentation/xcode/accepting-the-xcode-license",
			},

			// launchctl service management
			"launchctl:load": {
				Command:      "launchctl load ~/Library/LaunchAgents/{{service}}.plist",
				Description:  "Load a user launch agent",
				RequiresSudo: false,
				Alternatives: []string{"launchctl enable user/{{service}}", "brew services start {{service}}"},
				DocsURL:      "https://www.launchd.info/",
			},
			"launchctl:start": {
				Command:      "launchctl start {{service}}",
				Description:  "Start a launchd service",
				RequiresSudo: false,
				Alternatives: []string{"brew services start {{service}}"},
				DocsURL:      "https://www.launchd.info/",
			},
			"launchctl:stop": {
				Command:      "launchctl stop {{service}}",
				Description:  "Stop a launchd service",
				RequiresSudo: false,
				Alternatives: []string{"brew services stop {{service}}"},
				DocsURL:      "https://www.launchd.info/",
			},
			"launchctl:daemon:load": {
				Command:      "sudo launchctl load /Library/LaunchDaemons/{{service}}.plist",
				Description:  "Load a system daemon (requires sudo)",
				RequiresSudo: true,
				Alternatives: []string{"sudo launchctl enable system/{{service}}"},
				DocsURL:      "https://www.launchd.info/",
			},

			// macOS software updates
			"softwareupdate:install": {
				Command:      "softwareupdate -i -a",
				Description:  "Install all available macOS updates",
				RequiresSudo: true,
				Alternatives: []string{"softwareupdate -i -a --restart"},
				DocsURL:      "https://support.apple.com/guide/mac-help/mchlp1528/mac",
			},
			"softwareupdate:check": {
				Command:      "softwareupdate -l",
				Description:  "List available macOS updates",
				RequiresSudo: false,
				Alternatives: []string{},
				DocsURL:      "https://support.apple.com/guide/mac-help/mchlp1528/mac",
			},

			// Defaults / system preferences
			"defaults:write": {
				Command:      "defaults write {{domain}} {{key}} -{{type}} {{value}}",
				Description:  "Write a macOS defaults value",
				RequiresSudo: false,
				Alternatives: []string{"defaults write -g {{key}} -{{type}} {{value}}"},
				DocsURL:      "https://developer.apple.com/documentation/foundation/userdefaults",
			},
			"defaults:read": {
				Command:      "defaults read {{domain}} {{key}}",
				Description:  "Read a macOS defaults value",
				RequiresSudo: false,
				Alternatives: []string{},
				DocsURL:      "https://developer.apple.com/documentation/foundation/userdefaults",
			},

			// Rosetta 2
			"rosetta2:install": {
				Command:      "softwareupdate --install-rosetta --agree-to-license",
				Description:  "Install Rosetta 2 for x86_64 emulation on Apple Silicon",
				RequiresSudo: false,
				Alternatives: []string{"/usr/sbin/softwareupdate --install-rosetta --agree-to-license"},
				DocsURL:      "https://support.apple.com/en-us/HT211861",
			},

			// Disk utility
			"diskutil:repair": {
				Command:      "diskutil verifyVolume /",
				Description:  "Verify and repair disk permissions/volume",
				RequiresSudo: true,
				Alternatives: []string{"diskutil repairVolume /"},
				DocsURL:      "https://support.apple.com/guide/disk-utility/welcome/mac",
			},

			// SSH
			"ssh:config:perms": {
				Command:      "chmod 600 ~/.ssh/id_* && chmod 644 ~/.ssh/*.pub && chmod 700 ~/.ssh",
				Description:  "Fix SSH key and config permissions",
				RequiresSudo: false,
				Alternatives: []string{},
				DocsURL:      "https://www.ssh.com/academy/ssh/configuring-ssh",
			},
		}),
	}
}

// =============================================================================
// Linux Fixer
// =============================================================================

// LinuxFixer provides Linux-specific remediation commands.
type LinuxFixer struct {
	*BaseFixer
}

// NewLinuxFixer creates the Linux fixer with comprehensive command map.
func NewLinuxFixer() *LinuxFixer {
	return &LinuxFixer{
		BaseFixer: NewBaseFixer("linux-fixer", "linux", map[string]FixResult{
			// Package managers - apt (Debian/Ubuntu)
			"apt:install": {
				Command:      "sudo apt-get update && sudo apt-get install -y {{package}}",
				Description:  "Install a package via apt (Debian/Ubuntu)",
				RequiresSudo: true,
				Alternatives: []string{"sudo apt install -y {{package}}", "sudo apt-get install --only-upgrade {{package}}"},
				DocsURL:      "https://manpages.debian.org/testing/apt/apt-get.8.en.html",
			},
			"apt:upgrade": {
				Command:      "sudo apt-get update && sudo apt-get upgrade -y",
				Description:  "Upgrade all packages via apt",
				RequiresSudo: true,
				Alternatives: []string{"sudo apt upgrade -y"},
				DocsURL:      "https://manpages.debian.org/testing/apt/apt-get.8.en.html",
			},
			"apt:missing": {
				Command:      "sudo apt-get update && sudo apt-get install -y {{package}}",
				Description:  "Install missing package via apt",
				RequiresSudo: true,
				Alternatives: []string{"sudo apt install -y {{package}}"},
				DocsURL:      "https://packages.debian.org/",
			},
			"apt:key:add": {
				Command:      "curl -fsSL {{keyurl}} | sudo gpg --dearmor -o /usr/share/keyrings/{{name}}.gpg",
				Description:  "Add GPG key for apt repository",
				RequiresSudo: true,
				Alternatives: []string{"sudo apt-key add - < keyfile"},
				DocsURL:      "https://wiki.debian.org/SecureApt",
			},

			// Package managers - dnf (Fedora/RHEL)
			"dnf:install": {
				Command:      "sudo dnf install -y {{package}}",
				Description:  "Install a package via dnf (Fedora/RHEL)",
				RequiresSudo: true,
				Alternatives: []string{"sudo dnf upgrade {{package}}", "sudo yum install -y {{package}}"},
				DocsURL:      "https://dnf.readthedocs.io/en/latest/command_ref.html",
			},
			"dnf:upgrade": {
				Command:      "sudo dnf upgrade -y",
				Description:  "Upgrade all packages via dnf",
				RequiresSudo: true,
				Alternatives: []string{"sudo yum update -y"},
				DocsURL:      "https://dnf.readthedocs.io/en/latest/command_ref.html",
			},
			"dnf:missing": {
				Command:      "sudo dnf install -y {{package}}",
				Description:  "Install missing package via dnf",
				RequiresSudo: true,
				Alternatives: []string{"sudo yum install -y {{package}}"},
				DocsURL:      "https://dnf.readthedocs.io/en/latest/command_ref.html",
			},

			// Package managers - pacman (Arch)
			"pacman:install": {
				Command:      "sudo pacman -Sy {{package}}",
				Description:  "Install a package via pacman (Arch)",
				RequiresSudo: true,
				Alternatives: []string{"sudo pacman -S --noconfirm {{package}}", "yay -S {{package}}"},
				DocsURL:      "https://wiki.archlinux.org/title/Pacman",
			},
			"pacman:upgrade": {
				Command:      "sudo pacman -Syu",
				Description:  "Upgrade all packages via pacman",
				RequiresSudo: true,
				Alternatives: []string{"yay -Syu"},
				DocsURL:      "https://wiki.archlinux.org/title/Pacman",
			},
			"pacman:missing": {
				Command:      "sudo pacman -Sy {{package}}",
				Description:  "Install missing package via pacman",
				RequiresSudo: true,
				Alternatives: []string{"yay -S {{package}}"},
				DocsURL:      "https://wiki.archlinux.org/title/Pacman",
			},

			// Package managers - zypper (openSUSE)
			"zypper:install": {
				Command:      "sudo zypper install -y {{package}}",
				Description:  "Install a package via zypper (openSUSE)",
				RequiresSudo: true,
				Alternatives: []string{"sudo zypper in -y {{package}}"},
				DocsURL:      "https://en.opensuse.org/SDB:Zypper_usage",
			},
			"zypper:upgrade": {
				Command:      "sudo zypper update -y",
				Description:  "Upgrade all packages via zypper",
				RequiresSudo: true,
				Alternatives: []string{"sudo zypper up -y"},
				DocsURL:      "https://en.opensuse.org/SDB:Zypper_usage",
			},

			// systemd service management
			"systemd:enable": {
				Command:      "sudo systemctl enable {{service}}",
				Description:  "Enable a systemd service to start on boot",
				RequiresSudo: true,
				Alternatives: []string{"sudo systemctl enable --now {{service}}"},
				DocsURL:      "https://www.freedesktop.org/software/systemd/man/systemctl.html",
			},
			"systemd:start": {
				Command:      "sudo systemctl start {{service}}",
				Description:  "Start a systemd service immediately",
				RequiresSudo: true,
				Alternatives: []string{"sudo systemctl enable --now {{service}}"},
				DocsURL:      "https://www.freedesktop.org/software/systemd/man/systemctl.html",
			},
			"systemd:restart": {
				Command:      "sudo systemctl restart {{service}}",
				Description:  "Restart a systemd service",
				RequiresSudo: true,
				Alternatives: []string{"sudo systemctl reload {{service}}"},
				DocsURL:      "https://www.freedesktop.org/software/systemd/man/systemctl.html",
			},
			"systemd:stop": {
				Command:      "sudo systemctl stop {{service}}",
				Description:  "Stop a systemd service",
				RequiresSudo: true,
				Alternatives: []string{},
				DocsURL:      "https://www.freedesktop.org/software/systemd/man/systemctl.html",
			},
			"systemd:disable": {
				Command:      "sudo systemctl disable {{service}}",
				Description:  "Disable a systemd service from starting on boot",
				RequiresSudo: true,
				Alternatives: []string{"sudo systemctl disable --now {{service}}"},
				DocsURL:      "https://www.freedesktop.org/software/systemd/man/systemctl.html",
			},
			"systemd:status": {
				Command:      "systemctl status {{service}}",
				Description:  "Check status of a systemd service",
				RequiresSudo: false,
				Alternatives: []string{"journalctl -u {{service}} -f"},
				DocsURL:      "https://www.freedesktop.org/software/systemd/man/systemctl.html",
			},
			"systemd:daemon-reload": {
				Command:      "sudo systemctl daemon-reload",
				Description:  "Reload systemd daemon after unit file changes",
				RequiresSudo: true,
				Alternatives: []string{},
				DocsURL:      "https://www.freedesktop.org/software/systemd/man/systemctl.html",
			},

			// Flatpak
			"flatpak:install": {
				Command:      "flatpak install -y {{remote}} {{package}}",
				Description:  "Install a Flatpak application",
				RequiresSudo: false,
				Alternatives: []string{"flatpak install --user -y {{remote}} {{package}}"},
				DocsURL:      "https://docs.flatpak.org/en/latest/flatpak-command-reference.html",
			},
			"flatpak:update": {
				Command:      "flatpak update -y",
				Description:  "Update all Flatpak applications",
				RequiresSudo: false,
				Alternatives: []string{"flatpak update --user -y"},
				DocsURL:      "https://docs.flatpak.org/en/latest/flatpak-command-reference.html",
			},
			"flatpak:remote-add": {
				Command:      "flatpak remote-add --if-not-exists flathub https://flathub.org/repo/flathub.flatpakrepo",
				Description:  "Add Flathub remote repository",
				RequiresSudo: false,
				Alternatives: []string{"flatpak remote-add --user --if-not-exists flathub https://flathub.org/repo/flathub.flatpakrepo"},
				DocsURL:      "https://flatpak.org/setup/",
			},

			// Snap
			"snap:install": {
				Command:      "sudo snap install {{package}}",
				Description:  "Install a snap package",
				RequiresSudo: true,
				Alternatives: []string{"sudo snap install --classic {{package}}"},
				DocsURL:      "https://snapcraft.io/docs/installing-snaps",
			},
			"snap:refresh": {
				Command:      "sudo snap refresh",
				Description:  "Refresh all snap packages",
				RequiresSudo: true,
				Alternatives: []string{},
				DocsURL:      "https://snapcraft.io/docs/keeping-snaps-up-to-date",
			},

			// Kernel parameters
			"sysctl:set": {
				Command:      "echo '{{key}} = {{value}}' | sudo tee -a /etc/sysctl.d/99-custom.conf && sudo sysctl -p /etc/sysctl.d/99-custom.conf",
				Description:  "Set a kernel parameter persistently",
				RequiresSudo: true,
				Alternatives: []string{"sudo sysctl -w {{key}}={{value}}"},
				DocsURL:      "https://www.kernel.org/doc/html/latest/admin-guide/sysctl/",
			},
			"sysctl:view": {
				Command:      "sysctl {{key}}",
				Description:  "View current kernel parameter value",
				RequiresSudo: false,
				Alternatives: []string{"cat /proc/sys/{{key//./}}"},
				DocsURL:      "https://www.kernel.org/doc/html/latest/admin-guide/sysctl/",
			},

			// File permissions
			"ssh:config:perms": {
				Command:      "chmod 600 ~/.ssh/id_* && chmod 644 ~/.ssh/*.pub && chmod 700 ~/.ssh",
				Description:  "Fix SSH key and config permissions",
				RequiresSudo: false,
				Alternatives: []string{},
				DocsURL:      "https://www.ssh.com/academy/ssh/configuring-ssh",
			},

			// systemd-journald
			"journalctl:vacuum": {
				Command:      "sudo journalctl --vacuum-time=2weeks",
				Description:  "Clean up old journal entries",
				RequiresSudo: true,
				Alternatives: []string{"sudo journalctl --vacuum-size=500M"},
				DocsURL:      "https://www.freedesktop.org/software/systemd/man/journalctl.html",
			},

			// CPU frequency / power
			"cpufreq:governor": {
				Command:      "echo performance | sudo tee /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor",
				Description:  "Set CPU governor to performance mode",
				RequiresSudo: true,
				Alternatives: []string{"sudo cpupower frequency-set -g performance"},
				DocsURL:      "https://www.kernel.org/doc/html/latest/admin-guide/pm/cpufreq.html",
			},
		}),
	}
}

// =============================================================================
// Windows Fixer
// =============================================================================

// WindowsFixer provides Windows-specific remediation commands.
type WindowsFixer struct {
	*BaseFixer
}

// NewWindowsFixer creates the Windows fixer with comprehensive command map.
func NewWindowsFixer() *WindowsFixer {
	return &WindowsFixer{
		BaseFixer: NewBaseFixer("windows-fixer", "windows", map[string]FixResult{
			// winget (Windows Package Manager)
			"winget:install": {
				Command:      "winget install --silent --accept-source-agreements {{package}}",
				Description:  "Install a package via winget",
				RequiresSudo: false, // Runs in elevated PowerShell if needed
				Alternatives: []string{"winget install --id {{package}}", "winget install {{package}} --source winget"},
				DocsURL:      "https://learn.microsoft.com/en-us/windows/package-manager/winget/",
			},
			"winget:upgrade": {
				Command:      "winget upgrade --all --silent --accept-source-agreements",
				Description:  "Upgrade all packages via winget",
				RequiresSudo: false,
				Alternatives: []string{"winget upgrade --id {{package}}"},
				DocsURL:      "https://learn.microsoft.com/en-us/windows/package-manager/winget/upgrade",
			},
			"winget:missing": {
				Command:      "winget install --silent --accept-source-agreements {{package}}",
				Description:  "Install missing package via winget",
				RequiresSudo: false,
				Alternatives: []string{"winget install {{package}}"},
				DocsURL:      "https://learn.microsoft.com/en-us/windows/package-manager/winget/install",
			},
			"winget:source:add": {
				Command:      "winget source add --name {{name}} {{url}}",
				Description:  "Add a custom winget source",
				RequiresSudo: false,
				Alternatives: []string{"winget source add --name {{name}} {{url}} --trust-level trusted"},
				DocsURL:      "https://learn.microsoft.com/en-us/windows/package-manager/winget/source",
			},
			"winget:source:update": {
				Command:      "winget source update",
				Description:  "Update winget sources",
				RequiresSudo: false,
				Alternatives: []string{},
				DocsURL:      "https://learn.microsoft.com/en-us/windows/package-manager/winget/source",
			},
			"winget:list": {
				Command:      "winget list",
				Description:  "List installed packages via winget",
				RequiresSudo: false,
				Alternatives: []string{"winget list --name {{package}}"},
				DocsURL:      "https://learn.microsoft.com/en-us/windows/package-manager/winget/list",
			},

			// scoop
			"scoop:install": {
				Command:      "scoop install {{package}}",
				Description:  "Install a package via scoop",
				RequiresSudo: false,
				Alternatives: []string{"scoop install {{package}} --global"},
				DocsURL:      "https://scoop.sh/",
			},
			"scoop:update": {
				Command:      "scoop update *",
				Description:  "Update all scoop packages",
				RequiresSudo: false,
				Alternatives: []string{"scoop update {{package}}"},
				DocsURL:      "https://scoop.sh/",
			},
			"scoop:bucket:add": {
				Command:      "scoop bucket add {{bucket}} {{url}}",
				Description:  "Add a scoop bucket",
				RequiresSudo: false,
				Alternatives: []string{"scoop bucket add extras"},
				DocsURL:      "https://github.com/ScoopInstaller/Extras",
			},
			"scoop:missing": {
				Command:      "scoop install {{package}}",
				Description:  "Install missing package via scoop",
				RequiresSudo: false,
				Alternatives: []string{},
				DocsURL:      "https://scoop.sh/",
			},

			// chocolatey
			"choco:install": {
				Command:      "choco install -y {{package}}",
				Description:  "Install a package via Chocolatey",
				RequiresSudo: true, // Requires Admin PowerShell
				Alternatives: []string{"choco install {{package}} --force"},
				DocsURL:      "https://docs.chocolatey.org/en-us/choco/commands/install",
			},
			"choco:upgrade": {
				Command:      "choco upgrade all -y",
				Description:  "Upgrade all Chocolatey packages",
				RequiresSudo: true,
				Alternatives: []string{"choco upgrade {{package}} -y"},
				DocsURL:      "https://docs.chocolatey.org/en-us/choco/commands/upgrade",
			},
			"choco:missing": {
				Command:      "choco install -y {{package}}",
				Description:  "Install missing package via Chocolatey",
				RequiresSudo: true,
				Alternatives: []string{},
				DocsURL:      "https://docs.chocolatey.org/en-us/choco/commands/install",
			},
			"choco:source:add": {
				Command:      "choco source add --name={{name}} --source={{url}}",
				Description:  "Add a Chocolatey source",
				RequiresSudo: true,
				Alternatives: []string{},
				DocsURL:      "https://docs.chocolatey.org/en-us/choco/commands/source",
			},

			// PowerShell execution policy
			"powershell:policy:set": {
				Command:      "Set-ExecutionPolicy -Scope CurrentUser -ExecutionPolicy RemoteSigned -Force",
				Description:  "Set PowerShell execution policy to RemoteSigned for current user",
				RequiresSudo: false,
				Alternatives: []string{"Set-ExecutionPolicy -Scope CurrentUser -ExecutionPolicy Unrestricted -Force", "Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass -Force"},
				DocsURL:      "https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.security/set-executionpolicy",
			},
			"powershell:policy:get": {
				Command:      "Get-ExecutionPolicy -List",
				Description:  "Get current PowerShell execution policies",
				RequiresSudo: false,
				Alternatives: []string{},
				DocsURL:      "https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.security/get-executionpolicy",
			},

			// Scheduled tasks
			"task:create": {
				Command:      "schtasks /Create /TN \"{{name}}\" /TR \"{{command}}\" /SC ONCE /ST 00:00 /F",
				Description:  "Create a Windows scheduled task",
				RequiresSudo: true, // Requires Admin
				Alternatives: []string{"schtasks /Create /TN \"{{name}}\" /TR \"{{command}}\" /SC DAILY /ST 00:00 /F"},
				DocsURL:      "https://learn.microsoft.com/en-us/windows/win32/taskschd/schtasks",
			},
			"task:delete": {
				Command:      "schtasks /Delete /TN \"{{name}}\" /F",
				Description:  "Delete a Windows scheduled task",
				RequiresSudo: true,
				Alternatives: []string{},
				DocsURL:      "https://learn.microsoft.com/en-us/windows/win32/taskschd/schtasks",
			},
			"task:run": {
				Command:      "schtasks /Run /TN \"{{name}}\"",
				Description:  "Run a scheduled task immediately",
				RequiresSudo: false,
				Alternatives: []string{},
				DocsURL:      "https://learn.microsoft.com/en-us/windows/win32/taskschd/schtasks",
			},

			// Windows features / optional components
			"dism:enable-feature": {
				Command:      "dism /online /enable-feature /featurename:{{feature}} /all /norestart",
				Description:  "Enable a Windows optional feature",
				RequiresSudo: true,
				Alternatives: []string{"Enable-WindowsOptionalFeature -Online -FeatureName {{feature}} -All"},
				DocsURL:      "https://learn.microsoft.com/en-us/windows-hardware/manufacture/desktop/dism---enable-feature-command-line-options",
			},
			"dism:restore-health": {
				Command:      "dism /online /cleanup-image /restorehealth",
				Description:  "Repair Windows component store",
				RequiresSudo: true,
				Alternatives: []string{"sfc /scannow"},
				DocsURL:      "https://learn.microsoft.com/en-us/windows-hardware/manufacture/desktop/dism---restorehealth-command-line-options",
			},

			// System file checker
			"sfc:scan": {
				Command:      "sfc /scannow",
				Description:  "Scan and repair system files",
				RequiresSudo: true,
				Alternatives: []string{},
				DocsURL:      "https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/sfc",
			},

			// SSH
			"ssh:config:perms": {
				Command:      "icacls \"~/.ssh/id_*\" /inheritance:r /grant:r \"%USERNAME%:(R)\" && icacls \"~/.ssh/*.pub\" /inheritance:r /grant:r \"%USERNAME%:(R)\" && icacls \"~/.ssh\" /inheritance:r /grant:r \"%USERNAME%:(R,W)\"",
				Description:  "Fix SSH key permissions on Windows",
				RequiresSudo: false,
				Alternatives: []string{},
				DocsURL:      "https://learn.microsoft.com/en-us/windows-server/administration/openssh/openssh_keymanagement",
			},

			// Windows Update
			"wu:install": {
				Command:      "Install-Module -Name PSWindowsUpdate -Force; Get-WindowsUpdate -Install -AcceptAll -AutoReboot",
				Description:  "Install Windows updates via PowerShell",
				RequiresSudo: true,
				Alternatives: []string{"UsoClient StartInteractiveScan"},
				DocsURL:      "https://learn.microsoft.com/en-us/powershell/module/pswindowsupdate/",
			},

			// Environment variables
			"env:set": {
				Command:      "[Environment]::SetEnvironmentVariable('{{name}}', '{{value}}', 'User')",
				Description:  "Set a user environment variable",
				RequiresSudo: false,
				Alternatives: []string{"setx {{name}} \"{{value}}\""},
				DocsURL:      "https://learn.microsoft.com/en-us/dotnet/api/system.environment.setenvironmentvariable",
			},
			"env:path:add": {
				Command:      "$p = [Environment]::GetEnvironmentVariable('Path', 'User'); [Environment]::SetEnvironmentVariable('Path', $p + ';{{path}}', 'User')",
				Description:  "Add a directory to user PATH",
				RequiresSudo: false,
				Alternatives: []string{"setx Path \"%Path%;{{path}}\""},
				DocsURL:      "https://learn.microsoft.com/en-us/dotnet/api/system.environment.setenvironmentvariable",
			},

			// Developer mode
			"devmode:enable": {
				Command:      "reg add \"HKLM\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\AppModelUnlock\" /t REG_DWORD /v AllowDevelopmentWithoutDevLicense /d 1 /f",
				Description:  "Enable Developer Mode (requires Admin)",
				RequiresSudo: true,
				Alternatives: []string{"Settings > Privacy & Security > For Developers > Developer Mode"},
				DocsURL:      "https://learn.microsoft.com/en-us/windows/apps/get-started/enable-your-device-for-development",
			},
		}),
	}
}

// =============================================================================
// Helper functions
// =============================================================================

// RenderFixResult formats a FixResult for display.
func RenderFixResult(fix FixResult, checkName string) string {
	var b strings.Builder
	b.WriteString("  Fix for " + checkName + ":\n")
	b.WriteString("    Command: " + fix.Command + "\n")
	b.WriteString("    Description: " + fix.Description + "\n")
	if fix.RequiresSudo {
		b.WriteString("    ⚠ Requires sudo/Administrator\n")
	}
	if len(fix.Alternatives) > 0 {
		b.WriteString("    Alternatives:\n")
		for _, alt := range fix.Alternatives {
			b.WriteString("      - " + alt + "\n")
		}
	}
	if fix.DocsURL != "" {
		b.WriteString("    Docs: " + fix.DocsURL + "\n")
	}
	return b.String()
}

// FormatCommandForOS returns the command formatted for the current OS with template variables replaced.
func FormatCommandForOS(fix FixResult, vars map[string]string) string {
	cmd := fix.Command
	for k, v := range vars {
		cmd = strings.ReplaceAll(cmd, "{{"+k+"}}", v)
	}
	return cmd
}