package system

import (
	"strings"
	"testing"
)

// The fixtures below are shaped like the real /etc/os-release each
// distribution ships, including the details that broke detection before:
// Deepin's capitalised ID, Gentoo's single-quoted ID, and openSUSE's
// hyphenated ID. They exist so a distribution nobody anticipated is a test
// case, not a support ticket.

const (
	osReleaseOpenSUSE = `NAME="openSUSE Tumbleweed"
# VERSION="20240115"
ID="opensuse-tumbleweed"
ID_LIKE="opensuse suse"
VERSION_ID="20240115"
PRETTY_NAME="openSUSE Tumbleweed"
ANSI_COLOR="0;32"
CPE_NAME="cpe:/o:opensuse:tumbleweed:20240115"
BUG_REPORT_URL="https://bugzilla.opensuse.org"
HOME_URL="https://www.opensuse.org/"
LOGO="distributor-logo-Tumbleweed"
`

	osReleaseAlpine = `NAME="Alpine Linux"
ID=alpine
VERSION_ID=3.19.1
PRETTY_NAME="Alpine Linux v3.19"
HOME_URL="https://alpinelinux.org/"
BUG_REPORT_URL="https://gitlab.alpinelinux.org/alpine/aports/-/issues"
`

	osReleaseNixOS = `ANSI_COLOR="1;34"
BUG_REPORT_URL="https://github.com/NixOS/nixpkgs/issues"
BUILD_ID="24.05.20240115.abcdef0"
CPE_NAME="cpe:/o:nixos:nixos:24.05"
DEFAULT_HOSTNAME=nixos
DOCUMENTATION_URL="https://nixos.org/learn.html"
HOME_URL="https://nixos.org/"
ID=nixos
IMAGE_ID=""
IMAGE_VERSION=""
LOGO="nix-snowflake"
NAME=NixOS
PRETTY_NAME="NixOS 24.05 (Uakari)"
SUPPORT_URL="https://nixos.org/community.html"
VERSION="24.05 (Uakari)"
VERSION_CODENAME=uakari
VERSION_ID="24.05"
`

	// Deepin ships ID with a capital D on 20.x. Lower-casing is what keeps
	// the reported distro stable across their releases.
	osReleaseDeepin = `PRETTY_NAME="Deepin 23"
NAME="Deepin"
VERSION_ID="23"
VERSION="23"
ID=Deepin
HOME_URL="https://www.deepin.org/"
BUG_REPORT_URL="https://bbs.deepin.org/"
`

	osReleaseUOS = `PRETTY_NAME="UnionTech OS Desktop 20 Pro"
NAME="UOS"
VERSION_ID="20"
VERSION="20"
ID=uos
HOME_URL="https://www.chinauos.com/"
`

	// The os-release spec allows single quotes. Gentoo uses them, and the
	// double-quote-only parser left the value as `'gentoo'`, which matched
	// nothing.
	osReleaseGentoo = `NAME=Gentoo
ID='gentoo'
PRETTY_NAME="Gentoo Linux"
ANSI_COLOR="1;32"
HOME_URL="https://www.gentoo.org/"
SUPPORT_URL="https://www.gentoo.org/support/"
BUG_REPORT_URL="https://bugs.gentoo.org/"
VERSION_ID="2.17"
`

	osReleaseUbuntu = `NAME="Ubuntu"
VERSION="22.04.3 LTS (Jammy Jellyfish)"
ID=ubuntu
ID_LIKE=debian
PRETTY_NAME="Ubuntu 22.04.3 LTS"
VERSION_ID="22.04"
`
)

// toolsOnPath builds the tool map DetectTools would produce when exactly the
// named binaries resolve on PATH.
func toolsOnPath(names ...string) map[string]ToolStatus {
	tools := make(map[string]ToolStatus, len(names))
	for _, name := range names {
		tools[name] = ToolStatus{Name: name, Installed: true, Path: "/usr/bin/" + name}
	}
	return tools
}

// TestLinuxIsSupportedWheneverAPackageManagerIsOnPath is the whole point: what
// makes a Linux machine usable is a package manager that answers, not
// membership in a list somebody has to keep editing.
func TestLinuxIsSupportedWheneverAPackageManagerIsOnPath(t *testing.T) {
	tests := []struct {
		name      string
		osRelease string
		onPath    []string
		wantPM    string
	}{
		{name: "opensuse tumbleweed with zypper (#140)", osRelease: osReleaseOpenSUSE, onPath: []string{"zypper"}, wantPM: "zypper"},
		{name: "alpine with apk (#334)", osRelease: osReleaseAlpine, onPath: []string{"apk"}, wantPM: "apk"},
		{name: "nixos with nix (#110)", osRelease: osReleaseNixOS, onPath: []string{"nix"}, wantPM: "nix"},
		{name: "deepin with apt (#926)", osRelease: osReleaseDeepin, onPath: []string{"apt"}, wantPM: "apt"},
		{name: "uos with apt (#926)", osRelease: osReleaseUOS, onPath: []string{"apt"}, wantPM: "apt"},
		{name: "gentoo with emerge (#1669)", osRelease: osReleaseGentoo, onPath: []string{"emerge"}, wantPM: "emerge"},
		// Distributions the deleted enum recognised, plus the derivatives it
		// recognised only through ID_LIKE. None of them needs to be named now.
		{name: "ubuntu with apt stays supported", osRelease: osReleaseUbuntu, onPath: []string{"apt"}, wantPM: "apt"},
		{name: "debian with apt", osRelease: "ID=debian\nVERSION_ID=\"12\"\n", onPath: []string{"apt"}, wantPM: "apt"},
		{name: "linux mint with apt", osRelease: "ID=linuxmint\nID_LIKE=\"ubuntu debian\"\n", onPath: []string{"apt"}, wantPM: "apt"},
		{name: "pop os with apt", osRelease: "ID=pop\nID_LIKE=\"ubuntu debian\"\n", onPath: []string{"apt"}, wantPM: "apt"},
		{name: "arch with pacman", osRelease: "ID=arch\n", onPath: []string{"pacman"}, wantPM: "pacman"},
		{name: "manjaro with pacman", osRelease: "ID=manjaro\nID_LIKE=arch\n", onPath: []string{"pacman"}, wantPM: "pacman"},
		{name: "endeavouros with pacman", osRelease: "ID=endeavouros\nID_LIKE=arch\n", onPath: []string{"pacman"}, wantPM: "pacman"},
		{name: "fedora with dnf", osRelease: "ID=fedora\nID_LIKE=\"rhel fedora\"\n", onPath: []string{"dnf"}, wantPM: "dnf"},
		{name: "rocky with dnf", osRelease: "ID=rocky\nID_LIKE=\"rhel fedora\"\n", onPath: []string{"dnf"}, wantPM: "dnf"},
		{name: "almalinux with dnf", osRelease: "ID=almalinux\nID_LIKE=\"rhel fedora\"\n", onPath: []string{"dnf"}, wantPM: "dnf"},
		{name: "nobara with dnf", osRelease: "ID=nobara\nID_LIKE=\"fedora\"\n", onPath: []string{"dnf"}, wantPM: "dnf"},
		{name: "distro nobody has heard of, with dnf", osRelease: "ID=some-future-distro\n", onPath: []string{"dnf"}, wantPM: "dnf"},
		{name: "no os-release at all, with pacman", osRelease: "", onPath: []string{"pacman"}, wantPM: "pacman"},
		{name: "brew keeps winning over the native manager", osRelease: osReleaseAlpine, onPath: []string{"apk", "brew"}, wantPM: "brew"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			profile := resolvePlatformProfile("linux", tc.osRelease, toolsOnPath(tc.onPath...))

			if !profile.Supported {
				t.Fatalf("Supported = false, want true (package manager %q is on PATH)", tc.wantPM)
			}
			if profile.PackageManager != tc.wantPM {
				t.Fatalf("PackageManager = %q, want %q", profile.PackageManager, tc.wantPM)
			}
			if err := EnsureSupportedPlatform(profile); err != nil {
				t.Fatalf("EnsureSupportedPlatform() = %v, want nil", err)
			}
		})
	}
}

// TestLinuxDistroReportsTheOSReleaseIDVerbatim covers the second half of
// #1669: single-quoted values were left quoted, so `ID='gentoo'` matched
// nothing. The distro is reported for humans, so it must be the ID the
// machine actually declares.
func TestLinuxDistroReportsTheOSReleaseIDVerbatim(t *testing.T) {
	tests := []struct {
		name       string
		osRelease  string
		wantDistro string
	}{
		{name: "unquoted id", osRelease: osReleaseAlpine, wantDistro: "alpine"},
		{name: "double-quoted id", osRelease: osReleaseOpenSUSE, wantDistro: "opensuse-tumbleweed"},
		{name: "single-quoted id (#1669)", osRelease: osReleaseGentoo, wantDistro: "gentoo"},
		{name: "capitalised id is lower-cased", osRelease: osReleaseDeepin, wantDistro: "deepin"},
		{name: "uos", osRelease: osReleaseUOS, wantDistro: "uos"},
		{name: "nixos", osRelease: osReleaseNixOS, wantDistro: "nixos"},
		{name: "ubuntu", osRelease: osReleaseUbuntu, wantDistro: "ubuntu"},
		{name: "missing os-release", osRelease: "", wantDistro: LinuxDistroUnknown},
		{name: "comments only", osRelease: "# nothing here\n", wantDistro: LinuxDistroUnknown},
		{name: "malformed lines are skipped", osRelease: "no-equals-sign\nID='gentoo'\n", wantDistro: "gentoo"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			profile := resolvePlatformProfile("linux", tc.osRelease, toolsOnPath("apt"))
			if profile.LinuxDistro != tc.wantDistro {
				t.Fatalf("LinuxDistro = %q, want %q", profile.LinuxDistro, tc.wantDistro)
			}
		})
	}
}

// TestLinuxRefusalNamesTheProbedManagersAndARunnableCheck is the dead-end
// guard. A machine with no package manager at all is still a refusal, but it
// must be a refusal the reader can act on without editing /etc/os-release to
// lie about the distribution (which is what #334's reporter had to do).
func TestLinuxRefusalNamesTheProbedManagersAndARunnableCheck(t *testing.T) {
	profile := resolvePlatformProfile("linux", osReleaseGentoo, nil)
	if profile.Supported {
		t.Fatal("Supported = true with no package manager on PATH, want false")
	}

	err := EnsureSupportedPlatform(profile)
	if err == nil {
		t.Fatal("EnsureSupportedPlatform() = nil, want a refusal")
	}
	message := err.Error()

	// Every manager the probe looks for must be named, so the reader knows
	// exactly which one to install.
	for _, manager := range []string{"brew", "apt", "dnf", "pacman", "apk", "zypper", "nix", "emerge"} {
		if !strings.Contains(message, manager) {
			t.Errorf("refusal does not name probed package manager %q:\n%s", manager, message)
		}
	}

	// The exit has to be runnable, not a description of a policy.
	if !strings.Contains(message, `for m in brew apt dnf pacman apk zypper nix emerge; do command -v "$m"; done`) {
		t.Errorf("refusal does not print the runnable probe check:\n%s", message)
	}
	if !strings.Contains(message, "PATH") {
		t.Errorf("refusal does not say that PATH is what it searched:\n%s", message)
	}
	if !strings.Contains(message, "gentoo") {
		t.Errorf("refusal does not report the detected distro:\n%s", message)
	}

	// The old message named a closed list and no exit at all. It must be gone.
	if strings.Contains(message, "Linux support is limited to") {
		t.Errorf("refusal still names a closed distro list:\n%s", message)
	}
}
