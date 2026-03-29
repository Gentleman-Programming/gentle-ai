package engram

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/system"
)

// TestResolveEngramInstallNonBrewReturnsError verifies that after the fix,
// resolver.go only handles brew. Non-brew cases handled by DownloadLatestBinary.
// This is tested indirectly via the engram package — after our change,
// InstallCommand for linux/windows returns an error (those cases removed).
func TestInstallCommandNonBrewReturnsError(t *testing.T) {
	tests := []struct {
		name    string
		profile system.PlatformProfile
	}{
		{
			name:    "ubuntu returns error (no longer go install)",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroUbuntu, PackageManager: "apt"},
		},
		{
			name:    "arch returns error (no longer go install)",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroArch, PackageManager: "pacman"},
		},
		{
			name:    "fedora returns error (no longer go install)",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroFedora, PackageManager: "dnf"},
		},
		{
			name:    "windows returns error (no longer go install)",
			profile: system.PlatformProfile{OS: "windows", PackageManager: "winget"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := InstallCommand(tt.profile)
			if err == nil {
				t.Errorf("InstallCommand(%s) should return error for non-brew: expected error, got nil", tt.profile.PackageManager)
			}
		})
	}
}

// TestInstallCommandBrewStillWorks verifies brew path is unchanged.
func TestInstallCommandBrewStillWorks(t *testing.T) {
	profile := system.PlatformProfile{OS: "darwin", PackageManager: "brew"}
	cmds, err := InstallCommand(profile)
	if err != nil {
		t.Fatalf("InstallCommand(brew) unexpected error = %v", err)
	}
	if len(cmds) == 0 {
		t.Fatal("InstallCommand(brew) returned empty CommandSequence")
	}
	// Must still use brew tap + brew install
	found := false
	for _, cmd := range cmds {
		for _, arg := range cmd {
			if strings.Contains(arg, "engram") {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("InstallCommand(brew) commands don't reference engram: %v", cmds)
	}
}
