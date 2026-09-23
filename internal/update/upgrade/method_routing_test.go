package upgrade

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
	"github.com/gentleman-programming/gentle-ai/v3/internal/update"
)

// TestEffectiveMethodSelfToolRouting pins the D-02 routing table for the
// self-tool across platforms (F5.1).
func TestEffectiveMethodSelfToolRouting(t *testing.T) {
	origHomebrew := homebrewPackageInstalled
	t.Cleanup(func() { homebrewPackageInstalled = origHomebrew })
	homebrewPackageInstalled = func(string) bool { return false }

	forkTool := update.ToolInfo{
		Name:         "gentle-ai",
		Owner:        "IGutierrezZ",
		Repo:         "axiom",
		GoImportPath: "github.com/IGutierrezZ/axiom/cmd/axiom",
		GoModulePath: "github.com/gentleman-programming/gentle-ai/v3",
	}

	upstreamTool := update.ToolInfo{
		Name:         "gentle-ai",
		Owner:        "Gentleman-Programming",
		Repo:         "gentle-ai",
		GoImportPath: "github.com/gentleman-programming/gentle-ai/v3/cmd/gentle-ai",
		GoModulePath: "github.com/gentleman-programming/gentle-ai/v3",
	}

	tests := []struct {
		name    string
		tool    update.ToolInfo
		profile system.PlatformProfile
		want    update.InstallMethod
	}{
		{
			name:    "self-tool Windows !GoInstallResolvable → InstallSourceBuild",
			tool:    forkTool,
			profile: system.PlatformProfile{OS: "windows", GoAvailable: true},
			want:    update.InstallSourceBuild,
		},
		{
			name:    "self-tool Windows GoInstallResolvable → InstallGoInstall",
			tool:    upstreamTool,
			profile: system.PlatformProfile{OS: "windows", GoAvailable: true},
			want:    update.InstallGoInstall,
		},
		{
			name:    "self-tool Linux → InstallBinary",
			tool:    forkTool,
			profile: system.PlatformProfile{OS: "linux", GoAvailable: true},
			want:    update.InstallBinary,
		},
		{
			name:    "self-tool macOS → InstallBinary",
			tool:    upstreamTool,
			profile: system.PlatformProfile{OS: "darwin", GoAvailable: true},
			want:    update.InstallBinary,
		},
		{
			name:    "self-tool Windows without Go → InstallSourceBuild",
			tool:    upstreamTool,
			profile: system.PlatformProfile{OS: "windows", GoAvailable: false},
			want:    update.InstallSourceBuild,
		},
		{
			name:    "foreign tool Windows → declared method unchanged",
			tool:    update.ToolInfo{Name: "engram", InstallMethod: update.InstallBinary, GoImportPath: "github.com/x/y/cmd/y"},
			profile: system.PlatformProfile{OS: "windows", GoAvailable: true},
			want:    update.InstallGoInstall,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := effectiveMethod(tc.tool, tc.profile)
			if got != tc.want {
				t.Errorf("effectiveMethod(%q, %s) = %q, want %q", tc.tool.Name, tc.profile.OS, got, tc.want)
			}
		})
	}
}
