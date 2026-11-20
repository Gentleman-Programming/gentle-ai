//go:build linux

package shellinstaller

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// userHostPlatform refuses a host architecture the installer cannot serve
// before any download: only Linux amd64 is supported.
func userHostPlatform(goarch string) error {
	if goarch == "amd64" {
		return nil
	}
	return privateError("unavailable", userUnsupportedHost("Linux "+goarch))
}

func userUnsupportedHost(host string) error {
	return fmt.Errorf("unsupported host: %s; supported: Linux amd64 with a systemd user manager >=254, as a non-root user", host)
}

// userExecutableMount refuses a target parent on a noexec mount before any
// download: the published Node runtime and launchers would otherwise fail at
// the first probe with no explanation. The publish renames into the parent,
// so the parent's mount is the one the installation runs from.
func userExecutableMount(parent string, statfs func(string, *unix.Statfs_t) error) error {
	var st unix.Statfs_t
	if err := statfs(parent, &st); err != nil {
		return privateError("filesystem", fmt.Errorf("inspect the mount of target parent %s: %w", parent, err))
	}
	if st.Flags&unix.ST_NOEXEC != 0 {
		return privateError("refused", fmt.Errorf("target parent %s is on a noexec mount, so Node and the launchers cannot run from it; choose a target on an exec-capable filesystem (for example under your home directory) or remount it without noexec", parent))
	}
	return nil
}
