package system

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type SystemInfo struct {
	OS        string
	Arch      string
	Shell     string
	Supported bool
	Profile   PlatformProfile
}

type PlatformProfile struct {
	OS             string
	LinuxDistro    string
	PackageManager string
	NpmWritable    bool // true when npm global prefix is user-writable (nvm/fnm/volta)
	GoAvailable    bool // true when `go` is found on PATH (used for auto-detect: brew → go-install → binary)
	Supported      bool
}

const (
	LinuxDistroUnknown = "unknown"
	LinuxDistroUbuntu  = "ubuntu"
	LinuxDistroDebian  = "debian"
	LinuxDistroArch    = "arch"
	LinuxDistroFedora  = "fedora"
)

// linuxPackageManagers is the ordered list of package managers gentle-ai
// probes on PATH to decide whether a Linux machine is usable. What makes a
// machine usable is a manager that answers, not membership in a list of
// distributions somebody has to keep editing: every distribution that is
// missing from such a list is a dead end for its users, and there is always
// another distribution.
//
// The order is preference, not correctness. The first manager found wins.
// brew stays first because it was already the short-circuit that made any
// Linux distribution supported, and it is the manager with the widest
// install coverage here. The rest are ordered by how many machines run them.
//
// Adding a distribution never needs a change here. Adding a manager does,
// and the refusal in EnsureSupportedPlatform names this exact list so a user
// on a machine with none of them knows what to install.
var linuxPackageManagers = []string{
	"brew",   // any distribution, via Homebrew on Linux
	"apt",    // Debian, Ubuntu, Mint, Pop!_OS, Deepin, UOS
	"dnf",    // Fedora, RHEL, CentOS Stream, Rocky, AlmaLinux, Nobara
	"pacman", // Arch, Manjaro, EndeavourOS
	"apk",    // Alpine
	"zypper", // openSUSE, SUSE Linux Enterprise
	"nix",    // NixOS
	"emerge", // Gentoo, Calculate
}

// detectedToolNames is the fixed probe list passed to DetectTools: the tools
// other detection reads, plus every Linux package manager.
func detectedToolNames() []string {
	names := []string{"git", "curl", "node", "go"}
	return append(names, linuxPackageManagers...)
}

type DetectionResult struct {
	System       SystemInfo
	Tools        map[string]ToolStatus
	Configs      []ConfigState
	Dependencies DependencyReport
}

func IsSupportedOS(goos string) bool {
	return goos == "darwin" || goos == "linux" || goos == "windows"
}

func Detect(ctx context.Context) (DetectionResult, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return DetectionResult{}, err
	}

	tools := DetectTools(ctx, detectedToolNames())
	configs := ScanConfigs(homeDir)
	osReleaseContent, _ := osReleaseContent(runtime.GOOS)

	result := detectFromInputs(runtime.GOOS, runtime.GOARCH, os.Getenv("SHELL"), osReleaseContent, tools, configs)
	// On Windows, npm global prefix is user-writable by default (no sudo needed).
	if runtime.GOOS == "windows" {
		result.System.Profile.NpmWritable = true
	} else {
		result.System.Profile.NpmWritable = detectNpmWritable(homeDir)
	}
	result.Dependencies = DetectDependencies(ctx, result.System.Profile)

	return result, nil
}

// detectNpmWritable checks if npm's global prefix is under the user's home
// directory (nvm, fnm, volta, etc.), meaning sudo is not needed for global installs.
func detectNpmWritable(homeDir string) bool {
	out, err := exec.Command("npm", "config", "get", "prefix").Output()
	if err != nil {
		return false
	}
	prefix := strings.TrimSpace(string(out))
	return strings.HasPrefix(prefix, homeDir)
}

func detectFromInputs(goos, arch, shell, linuxOSRelease string, tools map[string]ToolStatus, configs []ConfigState) DetectionResult {
	if shell == "" {
		if goos == "windows" {
			shell = "powershell"
		} else {
			shell = "unknown"
		}
	}

	profile := resolvePlatformProfile(goos, linuxOSRelease, tools)

	return DetectionResult{
		System: SystemInfo{
			OS:        goos,
			Arch:      arch,
			Shell:     shell,
			Supported: profile.Supported,
			Profile:   profile,
		},
		Tools:   tools,
		Configs: configs,
	}
}

func osReleaseContent(goos string) (string, error) {
	if goos != "linux" {
		return "", nil
	}

	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func resolvePlatformProfile(goos, linuxOSRelease string, tools map[string]ToolStatus) PlatformProfile {
	profile := PlatformProfile{OS: goos}

	// Detect Go availability for the brew → go-install → binary auto-detect order.
	if go_, ok := tools["go"]; ok && go_.Installed {
		profile.GoAvailable = true
	}

	switch goos {
	case "darwin":
		profile.PackageManager = "brew"
		profile.Supported = true
		return profile
	case "linux":
		// The distro is reported, never gated on. It is what the machine
		// calls itself, for humans reading logs and error messages.
		profile.LinuxDistro = osReleaseID(linuxOSRelease)

		for _, manager := range linuxPackageManagers {
			if tool, ok := tools[manager]; ok && tool.Installed {
				profile.PackageManager = manager
				profile.Supported = true
				return profile
			}
		}

		profile.PackageManager = ""
		profile.Supported = false

		return profile
	case "windows":
		profile.PackageManager = "winget"
		profile.Supported = true
		return profile
	default:
		profile.Supported = false
		return profile
	}
}

// osReleaseID returns the lower-cased ID field of an /etc/os-release file, or
// LinuxDistroUnknown when the file is absent, empty, or declares no ID.
//
// Values may be quoted with either double or single quotes per the os-release
// spec (https://www.freedesktop.org/software/systemd/man/latest/os-release.html).
// Stripping only double quotes left Gentoo's `ID='gentoo'` as the literal
// value `'gentoo'`, which matched nothing.
func osReleaseID(linuxOSRelease string) string {
	for _, line := range strings.Split(linuxOSRelease, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found || strings.ToUpper(strings.TrimSpace(key)) != "ID" {
			continue
		}

		id := strings.ToLower(strings.Trim(strings.TrimSpace(value), `"'`))
		if id == "" {
			continue
		}

		return id
	}

	return LinuxDistroUnknown
}
