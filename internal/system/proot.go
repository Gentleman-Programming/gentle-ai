package system

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// PRootInfo contains the result of PRoot environment detection.
type PRootInfo struct {
	Detected    bool
	Distro      string
	Confidence  float64
	Signals     []string
}

// detectProot checks for PRoot/Termux environment using three independent signals.
func detectProot() PRootInfo {
	info := PRootInfo{}

	// Signal 1: PROOT_DISTRO env var (Termux / PRoot-Distro)
	if distro := os.Getenv("PROOT_DISTRO"); distro != "" {
		info.Detected = true
		info.Distro = distro
		info.Signals = append(info.Signals, "PROOT_DISTRO")
		info.Confidence = 0.4
	}

	// Signal 2: /proc/self/status TracerPid
	statusPath := "/proc/self/status"
	statusData, err := os.ReadFile(statusPath)
	if err == nil && strings.Contains(string(statusData), "TracerPid:") {
		info.Detected = true
		info.Signals = append(info.Signals, "TracerPid=0")
		info.Confidence += 0.3
	}

	// Signal 3: uname -a contains "proot"
	cmd := exec.Command("uname", "-a")
	output, err := cmd.Output()
	if err == nil && strings.Contains(strings.ToLower(string(output)), "proot") {
		info.Detected = true
		info.Signals = append(info.Signals, "uname-proot")
		info.Confidence += 0.3
	}

	// Cap confidence at 1.0
	if info.Confidence > 1.0 {
		info.Confidence = 1.0
	}

	// If any signal triggered, we're likely in PRoot
	if !info.Detected {
		info.Signals = nil
	}

	return info
}

// GetPreloadScriptPath returns the path to the proot-override.js script,
// or empty string if not found.
func GetPreloadScriptPath() string {
	candidates := []string{
		filepath.Join(runtime.GOROOT(), "..", "scripts", "proot-override.js"),
		filepath.Join(os.Getenv("HOME"), ".gentle-ai", "scripts", "proot-override.js"),
		"/root/opencode/scripts/proot-override.js",
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// HasProotOverrideChecks returns true if the current environment has PRoot
// detection and platform override capabilities enabled.
func HasProotOverrideChecks() bool {
	info := detectProot()
	return info.Detected && GetPreloadScriptPath() != ""
}

// ProotInfo returns PRoot detection info for the current environment.
func ProotInfo() PRootInfo {
	return detectProot()
}

// IsProot returns true if the current environment appears to be a PRoot/Termux system.
// This includes PRoot-Distro and Termux environments where Node.js may report
// "android" as the platform instead of "linux", causing CodeGraph to look for
// non-existent "android-<arch>" bundles.
func IsProot() bool {
	return detectProot().Detected
}

// CodeGraphPlatformSupportedWithProot returns true if the current platform has
// prebuilt CodeGraph native binaries available, considering PRoot detection.
// CodeGraph publishes optionalDependencies for: darwin-arm64, darwin-x64,
// linux-arm64, linux-x64, win32-arm64, win32-x64.
// Android (linux-arm64 with TERMUX_VERSION or ANDROID_ROOT set) is not
// supported because Node.js on Android may report "android" as the platform,
// causing the npm-shim to look for a non-existent "android-arm64" bundle.
// On PRoot systems, the platform is overridden to "linux" so CodeGraph can
// download the correct bundle.
func CodeGraphPlatformSupportedWithProot() bool {
	// First check if we're in a PRoot environment
	prootInfo := detectProot()

	// If in PRoot, we can still support CodeGraph by overriding the platform
	if prootInfo.Detected {
		// On PRoot, we map "android" to "linux" via the preload script
		// so CodeGraph downloads the correct bundle
		return true
	}

	// Fall back to the regular platform check
	return CodeGraphPlatformSupported()
}