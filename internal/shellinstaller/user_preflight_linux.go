//go:build linux

package shellinstaller

import (
	"debug/elf"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// userHostShell is the executable whose ELF interpreter identifies the host
// libc. A glibc host may also carry a musl loader (Debian's musl package
// installs /lib/ld-musl-*.so.1), so the loader actually used by the system
// shell is the evidence, never the presence of a musl loader file.
const userHostShell = "/bin/sh"

// userHostPlatform refuses a host the installer cannot serve before any
// download: only Linux amd64 with glibc is supported. A shell that cannot be
// classified (unreadable, static or not ELF) is no evidence of musl, so this
// diagnosis never refuses an unclassified host; later checks still apply.
func userHostPlatform(goarch, shell string) error {
	libc := userHostLibc(shell)
	if goarch == "amd64" && libc != "musl" {
		return nil
	}
	host := "Linux " + goarch
	if libc == "musl" {
		host += "/musl"
	}
	return privateError("unavailable", userUnsupportedHost(host))
}

func userUnsupportedHost(host string) error {
	return fmt.Errorf("unsupported host: %s; supported: Linux amd64 with glibc and a systemd user manager >=254, as a non-root user", host)
}

// userHostLibc reports "musl" or "glibc" from the PT_INTERP of shell, or ""
// when the shell cannot be classified.
func userHostLibc(shell string) string {
	file, err := elf.Open(shell)
	if err != nil {
		return ""
	}
	defer file.Close()
	for _, program := range file.Progs {
		if program.Type != elf.PT_INTERP {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(program.Open(), 4096))
		if err != nil {
			return ""
		}
		loader := filepath.Base(strings.TrimRight(string(data), "\x00"))
		switch {
		case strings.HasPrefix(loader, "ld-musl-"):
			return "musl"
		case strings.HasPrefix(loader, "ld-linux"):
			return "glibc"
		}
		return ""
	}
	return ""
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
