package system

import "fmt"

// installHintGit returns the platform-specific install hint for git.
func installHintGit(profile PlatformProfile) string {
	switch {
	case profile.OS == "darwin":
		return "brew install git"
	case profile.OS == "windows":
		return "winget install Git.Git"
	case profile.PackageManager == "apt":
		return "sudo apt-get install -y git"
	case profile.PackageManager == "pacman":
		return "sudo pacman -S --noconfirm git"
	case profile.PackageManager == "dnf":
		return "sudo dnf install -y git"
	default:
		return "install git from https://git-scm.com/"
	}
}

// installHintCurl returns the platform-specific install hint for curl.
func installHintCurl(profile PlatformProfile) string {
	switch {
	case profile.OS == "darwin":
		return "brew install curl"
	case profile.OS == "windows":
		return "curl is pre-installed on Windows 10+"
	case profile.PackageManager == "apt":
		return "sudo apt-get install -y curl"
	case profile.PackageManager == "pacman":
		return "sudo pacman -S --noconfirm curl"
	case profile.PackageManager == "dnf":
		return "sudo dnf install -y curl"
	default:
		return "install curl from https://curl.se/"
	}
}

// installHintNode returns the platform-specific install hint for Node.js.
func installHintNode(profile PlatformProfile) string {
	switch {
	case profile.OS == "darwin":
		return "brew install node"
	case profile.OS == "windows":
		return "winget install OpenJS.NodeJS.LTS"
	case profile.PackageManager == "apt":
		return "curl -fsSL https://deb.nodesource.com/setup_lts.x | sudo -E bash - && sudo apt-get install -y nodejs"
	case profile.PackageManager == "pacman":
		return "sudo pacman -S --noconfirm nodejs npm"
	case profile.PackageManager == "dnf":
		return "curl -fsSL https://rpm.nodesource.com/setup_lts.x | sudo bash - && sudo dnf install -y nodejs"
	default:
		return "install node from https://nodejs.org/"
	}
}

// installHintNpm returns the platform-specific install hint for npm.
func installHintNpm(_ PlatformProfile) string {
	// npm comes with node on all platforms.
	return "npm is included with node — install node first"
}

// installHintBrew returns the install hint for Homebrew (macOS only).
func installHintBrew() string {
	return `/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"`
}

// installHintGo returns the platform-specific install hint for Go.
func installHintGo(profile PlatformProfile) string {
	switch {
	case profile.OS == "darwin":
		return "brew install go"
	case profile.OS == "windows":
		return "winget install GoLang.Go"
	case profile.PackageManager == "apt":
		return "sudo apt-get install -y golang"
	case profile.PackageManager == "pacman":
		return "sudo pacman -S --noconfirm go"
	case profile.PackageManager == "dnf":
		return "sudo dnf install -y golang"
	default:
		return "install go from https://go.dev/dl/"
	}
}

// InstallCommandsForDep returns the command sequence to install a missing dependency.
// Returns nil if no automatic install is available.
func InstallCommandsForDep(name string, profile PlatformProfile) [][]string {
	switch name {
	case "git":
		return installCommandsGit(profile)
	case "curl":
		return installCommandsCurl(profile)
	case "node":
		return installCommandsNode(profile)
	case "npm":
		// npm comes with node; installing node installs npm.
		return nil
	case "brew":
		return installCommandsBrew(profile)
	case "go":
		return installCommandsGo(profile)
	default:
		return nil
	}
}

func installCommandsGit(profile PlatformProfile) [][]string {
	switch {
	case profile.OS == "darwin":
		return [][]string{{"brew", "install", "git"}}
	case profile.OS == "windows":
		return [][]string{{"winget", "install", "--id", "Git.Git", "-e", "--accept-source-agreements", "--accept-package-agreements"}}
	case profile.PackageManager == "apt":
		return [][]string{{"sudo", "apt-get", "install", "-y", "git"}}
	case profile.PackageManager == "pacman":
		return [][]string{{"sudo", "pacman", "-S", "--noconfirm", "git"}}
	case profile.PackageManager == "dnf":
		return [][]string{{"sudo", "dnf", "install", "-y", "git"}}
	default:
		return nil
	}
}

func installCommandsCurl(profile PlatformProfile) [][]string {
	switch {
	case profile.OS == "darwin":
		return [][]string{{"brew", "install", "curl"}}
	case profile.OS == "windows":
		// curl is pre-installed on Windows 10+, no install command needed.
		return nil
	case profile.PackageManager == "apt":
		return [][]string{{"sudo", "apt-get", "install", "-y", "curl"}}
	case profile.PackageManager == "pacman":
		return [][]string{{"sudo", "pacman", "-S", "--noconfirm", "curl"}}
	case profile.PackageManager == "dnf":
		return [][]string{{"sudo", "dnf", "install", "-y", "curl"}}
	default:
		return nil
	}
}

func installCommandsNode(profile PlatformProfile) [][]string {
	switch {
	case profile.OS == "darwin":
		return [][]string{{"brew", "install", "node"}}
	case profile.OS == "windows":
		return [][]string{{"winget", "install", "--id", "OpenJS.NodeJS.LTS", "-e", "--accept-source-agreements", "--accept-package-agreements"}}
	case profile.PackageManager == "apt":
		// NodeSource LTS setup + install.
		return [][]string{
			{"bash", "-c", "curl -fsSL https://deb.nodesource.com/setup_lts.x | sudo -E bash -"},
			{"sudo", "apt-get", "install", "-y", "nodejs"},
		}
	case profile.PackageManager == "pacman":
		return [][]string{{"sudo", "pacman", "-S", "--noconfirm", "nodejs", "npm"}}
	case profile.PackageManager == "dnf":
		// Use NodeSource LTS on Fedora/RHEL family for parity with apt-based LTS behavior.
		return [][]string{
			{"bash", "-c", "curl -fsSL https://rpm.nodesource.com/setup_lts.x | sudo bash -"},
			{"sudo", "dnf", "install", "-y", "nodejs"},
		}
	default:
		return nil
	}
}

func installCommandsBrew(profile PlatformProfile) [][]string {
	if profile.OS != "darwin" {
		return nil
	}
	return [][]string{
		{"bash", "-c", `$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)`},
	}
}

func installCommandsGo(profile PlatformProfile) [][]string {
	switch {
	case profile.OS == "darwin":
		return [][]string{{"brew", "install", "go"}}
	case profile.OS == "windows":
		return [][]string{{"winget", "install", "--id", "GoLang.Go", "-e", "--accept-source-agreements", "--accept-package-agreements"}}
	case profile.PackageManager == "apt":
		return [][]string{{"sudo", "apt-get", "install", "-y", "golang"}}
	case profile.PackageManager == "pacman":
		return [][]string{{"sudo", "pacman", "-S", "--noconfirm", "go"}}
	case profile.PackageManager == "dnf":
		return [][]string{{"sudo", "dnf", "install", "-y", "golang"}}
	default:
		return nil
	}
}

// FormatMissingDepsMessage creates a human-readable message about missing dependencies.
func FormatMissingDepsMessage(report DependencyReport) string {
	if report.AllPresent {
		return "All required dependencies are present."
	}

	msg := fmt.Sprintf("Missing %d required dependency(ies): %s\n",
		len(report.MissingRequired),
		joinStrings(report.MissingRequired))

	msg += "\nInstall hints:\n"
	for _, dep := range report.Dependencies {
		if !dep.Installed && dep.Required {
			msg += fmt.Sprintf("  %s: %s\n", dep.Name, dep.InstallHint)
		}
	}

	return msg
}

func joinStrings(values []string) string {
	if len(values) == 0 {
		return "none"
	}

	result := values[0]
	for i := 1; i < len(values); i++ {
		result += ", " + values[i]
	}

	return result
}
