package system

import (
	"errors"
	"fmt"
	"runtime"
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
		return fmt.Errorf("%w: Linux support is limited to Ubuntu/Debian, Arch, and Fedora/RHEL family (detected %s)", ErrUnsupportedLinuxDistro, profile.LinuxDistro)
	}

	return nil
}
