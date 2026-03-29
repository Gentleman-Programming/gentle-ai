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
	Supported      bool
}

const (
	LinuxDistroUnknown = "unknown"
	LinuxDistroUbuntu  = "ubuntu"
	LinuxDistroDebian  = "debian"
	LinuxDistroArch    = "arch"
	LinuxDistroFedora  = "fedora"
)

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

	tools := DetectTools(ctx, []string{"git", "curl", "brew", "node"})
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

	switch goos {
	case "darwin":
		profile.PackageManager = "brew"
		profile.Supported = true
		return profile
	case "linux":
		distro := detectLinuxDistro(linuxOSRelease)
		profile.LinuxDistro = distro

		// Check if brew is available on Linux
		if brew, ok := tools["brew"]; ok && brew.Installed {
			profile.PackageManager = "brew"
			profile.Supported = true
			return profile
		}

		switch distro {
		case LinuxDistroUbuntu, LinuxDistroDebian:
			profile.PackageManager = "apt"
			profile.Supported = true
		case LinuxDistroArch:
			profile.PackageManager = "pacman"
			profile.Supported = true
		case LinuxDistroFedora:
			profile.PackageManager = "dnf"
			profile.Supported = true
		default:
			profile.PackageManager = ""
			profile.Supported = false
		}

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

func detectLinuxDistro(linuxOSRelease string) string {
	if strings.TrimSpace(linuxOSRelease) == "" {
		return LinuxDistroUnknown
	}

	fields := map[string]string{}
	for _, line := range strings.Split(linuxOSRelease, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.ToUpper(strings.TrimSpace(parts[0]))
		value := strings.Trim(strings.TrimSpace(parts[1]), `"`)
		fields[key] = strings.ToLower(value)
	}

	id := fields["ID"]
	idLike := fields["ID_LIKE"]

	if isUbuntuLike(id, idLike) {
		if id == LinuxDistroDebian {
			return LinuxDistroDebian
		}
		return LinuxDistroUbuntu
	}

	if isArchLike(id, idLike) {
		return LinuxDistroArch
	}

	if isFedoraLike(id, idLike) {
		return LinuxDistroFedora
	}

	return LinuxDistroUnknown
}

func isUbuntuLike(id, idLike string) bool {
	if id == LinuxDistroUbuntu || id == LinuxDistroDebian {
		return true
	}

	for _, token := range strings.Fields(idLike) {
		if token == LinuxDistroUbuntu || token == LinuxDistroDebian {
			return true
		}
	}

	return false
}

func isArchLike(id, idLike string) bool {
	if id == LinuxDistroArch {
		return true
	}

	for _, token := range strings.Fields(idLike) {
		if token == LinuxDistroArch {
			return true
		}
	}

	return false
}

func isFedoraLike(id, idLike string) bool {
	if id == LinuxDistroFedora || id == "rhel" || id == "centos" || id == "rocky" || id == "almalinux" || id == "nobara" {
		return true
	}

	for _, token := range strings.Fields(idLike) {
		if token == LinuxDistroFedora || token == "rhel" || token == "centos" || token == "rocky" || token == "almalinux" || token == "nobara" {
			return true
		}
	}

	return false
}
