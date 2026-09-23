package update

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
)

// TestSelfHintDerivedFromToolInfo scans the real output of updateHint and
// gentleAIHint for the fork registry entry (REQ-22.2 "La pista de actualizacion
// nombra el fork, no upstream") and pins the upstream entry to its own ToolInfo.
func TestSelfHintDerivedFromToolInfo(t *testing.T) {
	origHomebrewPackageInstalled := homebrewPackageInstalled
	t.Cleanup(func() { homebrewPackageInstalled = origHomebrewPackageInstalled })

	forkCloneBuild := "git clone https://github.com/IGutierrezZ/axiom && cd axiom && go build -o axiom ./cmd/axiom"
	upstreamGoInstall := "go install github.com/gentleman-programming/gentle-ai/v3/cmd/gentle-ai@latest"

	tests := []struct {
		name          string
		tool          ToolInfo
		profile       system.PlatformProfile
		brewInstalled bool
		want          string
	}{
		{
			name:          "fork macOS brew-owned derives brew upgrade from Name",
			tool:          forkSourceTool,
			profile:       system.PlatformProfile{OS: "darwin", PackageManager: "brew"},
			brewInstalled: true,
			want:          "brew upgrade axiom",
		},
		{
			name:    "fork macOS non-brew names the fork binary",
			tool:    forkSourceTool,
			profile: system.PlatformProfile{OS: "darwin", PackageManager: "brew"},
			want:    "axiom upgrade (downloads pre-built binary)",
		},
		{
			name:    "fork linux derives install.sh URL from Owner/Repo",
			tool:    forkSourceTool,
			profile: system.PlatformProfile{OS: "linux", PackageManager: "apt"},
			want:    "curl -fsSL https://raw.githubusercontent.com/IGutierrezZ/axiom/main/scripts/install.sh | bash",
		},
		{
			name:    "fork windows degrades to clone and build without go install",
			tool:    forkSourceTool,
			profile: system.PlatformProfile{OS: "windows", PackageManager: "winget"},
			want:    WindowsDistributionHoldMessage + " Install/update from source with Go 1.25.10+: " + forkCloneBuild,
		},
		{
			name:          "upstream macOS brew-owned derives brew upgrade from Name",
			tool:          upstreamSourceTool,
			profile:       system.PlatformProfile{OS: "darwin", PackageManager: "brew"},
			brewInstalled: true,
			want:          "brew upgrade gentle-ai",
		},
		{
			name:    "upstream macOS non-brew names the upstream binary",
			tool:    upstreamSourceTool,
			profile: system.PlatformProfile{OS: "darwin", PackageManager: "brew"},
			want:    "gentle-ai upgrade (downloads pre-built binary)",
		},
		{
			name:    "upstream linux derives install.sh URL from Owner/Repo",
			tool:    upstreamSourceTool,
			profile: system.PlatformProfile{OS: "linux", PackageManager: "apt"},
			want:    "curl -fsSL https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/main/scripts/install.sh | bash",
		},
		{
			name:    "upstream windows keeps resolvable go install from ToolInfo",
			tool:    upstreamSourceTool,
			profile: system.PlatformProfile{OS: "windows", PackageManager: "winget"},
			want:    WindowsDistributionHoldMessage + " Install/update from source with Go 1.25.10+: " + upstreamGoInstall,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			homebrewPackageInstalled = func(toolName string) bool {
				return toolName == tc.tool.Name && tc.brewInstalled
			}

			hint := updateHint(tc.tool, tc.profile)
			if hint != tc.want {
				t.Fatalf("updateHint(%q, %q) = %q, want %q", tc.tool.Name, tc.profile.OS, hint, tc.want)
			}
			self := gentleAIHint(tc.tool, tc.profile)
			if self != tc.want {
				t.Fatalf("gentleAIHint(%q, %q) = %q, want %q", tc.tool.Name, tc.profile.OS, self, tc.want)
			}

			// The upstream prohibition is scoped to the fork entry: upstream's
			// own module path is legal in its own instruction (REQ-22.2).
			if tc.tool.Name == "axiom" {
				if hint == "" {
					t.Fatalf("updateHint(%q, %q) must not be empty for the fork entry", tc.tool.Name, tc.profile.OS)
				}
				for _, forbidden := range []string{"gentleman-programming/gentle-ai", "cmd/gentle-ai"} {
					if strings.Contains(hint, forbidden) {
						t.Fatalf("updateHint(%q, %q) = %q, must not contain %q", tc.tool.Name, tc.profile.OS, hint, forbidden)
					}
				}
				if !strings.Contains(hint, "axiom") && !strings.Contains(hint, "IGutierrezZ") {
					t.Fatalf("updateHint(%q, %q) = %q, must reference the fork identity", tc.tool.Name, tc.profile.OS, hint)
				}
			}
		})
	}
}

// TestSelfHintScanAcrossPlatforms is the REQ-22.2 output scan: every fork
// instruction across platforms must name the tool's own identity and never
// upstream coordinates. Both fixtures run so the upstream entry keeps producing
// a non-empty instruction from its own ToolInfo.
func TestSelfHintScanAcrossPlatforms(t *testing.T) {
	origHomebrewPackageInstalled := homebrewPackageInstalled
	t.Cleanup(func() { homebrewPackageInstalled = origHomebrewPackageInstalled })
	homebrewPackageInstalled = func(string) bool { return false }

	profiles := []system.PlatformProfile{
		{OS: "linux", PackageManager: "apt"},
		{OS: "darwin", PackageManager: "brew"},
		{OS: "windows", PackageManager: "winget"},
	}

	for _, tool := range []ToolInfo{forkSourceTool, upstreamSourceTool} {
		for _, profile := range profiles {
			for name, produce := range map[string]func() string{
				"updateHint":   func() string { return updateHint(tool, profile) },
				"gentleAIHint": func() string { return gentleAIHint(tool, profile) },
			} {
				got := produce()
				if got == "" {
					t.Fatalf("%s(%q, %q) must not be empty", name, tool.Name, profile.OS)
				}
				if tool.Name != "axiom" {
					continue
				}
				for _, forbidden := range []string{"gentleman-programming/gentle-ai", "cmd/gentle-ai"} {
					if strings.Contains(got, forbidden) {
						t.Fatalf("%s(%q, %q) = %q, must not contain %q", name, tool.Name, profile.OS, got, forbidden)
					}
				}
				if !strings.Contains(got, "axiom") && !strings.Contains(got, "IGutierrezZ") {
					t.Fatalf("%s(%q, %q) = %q, must reference the fork identity", name, tool.Name, profile.OS, got)
				}
			}
		}
	}
}
