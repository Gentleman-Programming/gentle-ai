package upgrade

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
	"github.com/gentleman-programming/gentle-ai/v2/internal/update"
)

func mockHomebrewOwnership(t *testing.T, ownership update.HomebrewOwnership) {
	original := homebrewOwnershipDetector
	t.Cleanup(func() { homebrewOwnershipDetector = original })
	homebrewOwnershipDetector = func(string) (update.HomebrewOwnership, error) { return ownership, nil }
}

func TestRunStrategyFailsClosedOnHomebrewOwnershipUncertainty(t *testing.T) {
	original := homebrewOwnershipDetector
	t.Cleanup(func() { homebrewOwnershipDetector = original })
	homebrewOwnershipDetector = func(string) (update.HomebrewOwnership, error) {
		return update.HomebrewNone, errors.New("ownership uncertain")
	}
	_, err := runStrategy(context.Background(), update.UpdateResult{Tool: update.ToolInfo{Name: "engram", InstallMethod: update.InstallBinary}}, system.PlatformProfile{OS: "darwin", PackageManager: "brew"})
	if err == nil || !strings.Contains(err.Error(), "ownership uncertain") {
		t.Fatalf("error=%v, want ownership failure", err)
	}
}

func TestRunStrategyUsesCaskOwnershipAndMigrationGuidance(t *testing.T) {
	originalExec, originalOwnership := execCommand, homebrewOwnershipDetector
	t.Cleanup(func() { execCommand, homebrewOwnershipDetector = originalExec, originalOwnership })
	for _, tt := range []struct {
		name, version, target string
		upgradeFails          bool
	}{{"current", "engram 1.2.3", "1.2.3", false}, {"stale prerelease", "engram 1.2.3-beta.1", "1.2.3-beta.2", false}, {"upgrade failure", "", "1.2.3", true}} {
		homebrewOwnershipDetector = func(string) (update.HomebrewOwnership, error) { return update.HomebrewCask, nil }
		calls := ""
		execCommand = func(name string, args ...string) *exec.Cmd {
			if name == "brew" {
				call := strings.Join(args, " ")
				calls += call + "\n"
				if tt.upgradeFails && strings.HasPrefix(call, "upgrade ") {
					return mockCmd("false")
				}
				return mockCmd("echo", "ok")
			}
			if name == "engram" {
				return mockCmd("printf", tt.version)
			}
			return mockCmd("false")
		}
		r := update.UpdateResult{Tool: update.ToolInfo{Name: "engram", DetectCmd: []string{"engram", "version"}, InstallMethod: update.InstallBinary}, LatestVersion: tt.target}
		_, err := runStrategy(context.Background(), r, system.PlatformProfile{OS: "darwin", PackageManager: "brew"})
		wantErr := tt.name != "current"
		if (err != nil) != wantErr || !strings.Contains(calls, "trust --cask gentleman-programming/tap/engram") || !strings.Contains(calls, "upgrade --cask engram") {
			t.Fatalf("%s: error=%v calls=%s", tt.name, err, calls)
		}
		if wantErr && (!strings.Contains(err.Error(), "brew uninstall --cask engram") || !strings.Contains(err.Error(), "brew install --formula gentleman-programming/tap/engram")) {
			t.Fatalf("%s: migration guidance missing: %v", tt.name, err)
		}
	}
}
