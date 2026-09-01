package system

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
)

var ErrUnsupportedOS = errors.New("unsupported operating system")
var ErrUnsupportedLinuxDistro = errors.New("unsupported linux distro")

func EnsureCurrentOSSupported() error {
	return EnsureSupportedOS(runtime.GOOS)
}

func EnsureSupportedOS(goos string) error {
	if IsSupportedOS(goos) {
		return nil
	}

	return fmt.Errorf("%w: only macOS, Linux, and Windows are supported (detected %s)", ErrUnsupportedOS, goos)
}

func EnsureSupportedPlatform(profile PlatformProfile) error {
	if err := EnsureSupportedOS(profile.OS); err != nil {
		return err
	}

	if profile.OS == "linux" && !profile.Supported {
		distro := strings.TrimSpace(profile.LinuxDistro)
		if distro == "" {
			distro = LinuxDistroUnknown
		}

		// A refusal that names no exit is why users edited /etc/os-release to
		// lie about their distribution. This one names what was searched,
		// where, and the single thing that resolves it.
		return fmt.Errorf(
			"%w: no package manager found on PATH (detected distro %s).\n"+
				"gentle-ai looks for these, in order: %s\n"+
				"See which ones this machine already has:\n"+
				"  for m in %s; do command -v \"$m\"; done\n"+
				"Install one of them, or add the directory holding it to PATH, then run gentle-ai again.",
			ErrUnsupportedLinuxDistro,
			distro,
			strings.Join(linuxPackageManagers, ", "),
			strings.Join(linuxPackageManagers, " "),
		)
	}

	return nil
}
