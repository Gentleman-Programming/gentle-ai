package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/legacyassets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/opencodedefault"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// TestOpenCodeFamilySettingsWritersPreservePrivateMode proves that the
// OpenCode-family settings writers owned by the routing step keep a user's
// private settings file private. opencode.json may hold provider credentials,
// so a rewrite must never widen 0600 to 0644.
func TestOpenCodeFamilySettingsWritersPreservePrivateMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningful on Windows")
	}

	for _, agent := range []model.AgentID{model.AgentOpenCode, model.AgentKilocode} {
		t.Run(string(agent), func(t *testing.T) {
			settingsPath := filepath.Join(t.TempDir(), "opencode.json")
			prompt := filemerge.InjectMarkdownSection("", legacyTriggerRulesSection, "Retired WorkRun ceremony\n")
			seed, err := json.Marshal(map[string]any{
				"provider": map[string]any{"example": map[string]any{"options": map[string]any{"apiKey": "secret"}}},
				"agent": map[string]any{
					opencodedefault.ManagedAgent: map[string]any{"prompt": prompt},
					"sdd-apply":                  map[string]any{"__managed_by": "gentle-ai/sdd"},
					"general":                    map[string]any{"__managed_by": "gentle-ai/sdd"},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(settingsPath, seed, 0o600); err != nil {
				t.Fatal(err)
			}
			// os.WriteFile honors the umask; pin the seed mode explicitly.
			if err := os.Chmod(settingsPath, 0o600); err != nil {
				t.Fatal(err)
			}

			steps := []struct {
				name string
				run  func() (bool, error)
			}{
				{"strip legacy trigger rules", func() (bool, error) {
					result, err := stripLegacyTriggerRulesFromOrchestrator(settingsPath)
					return result.Changed, err
				}},
				{"retire SDD settings", func() (bool, error) {
					result, err := legacyassets.RetireOpenCodeSDDSettings(settingsPath)
					return result.Changed, err
				}},
				{"migrate legacy agents", func() (bool, error) {
					changed, _, err := migrateLegacyOpenCodeAgents(settingsPath, agent)
					return changed, err
				}},
				{"install review provider roles", func() (bool, error) { return installOpenCodeReviewProviderRoles(settingsPath, agent) }},
				{"install parity agents", func() (bool, error) { return installOpenCodeFamilyParityAgents(settingsPath, agent) }},
				{"share default", func() (bool, error) { return opencodedefault.ApplyShareDefault(settingsPath) }},
			}
			for _, step := range steps {
				changed, err := step.run()
				if err != nil {
					t.Fatalf("%s: %v", step.name, err)
				}
				if !changed {
					t.Fatalf("%s did not rewrite the settings; the mode assertion would prove nothing", step.name)
				}
				info, err := os.Stat(settingsPath)
				if err != nil {
					t.Fatal(err)
				}
				if got := info.Mode().Perm(); got != 0o600 {
					t.Fatalf("%s changed settings mode to %v, want 0600", step.name, got)
				}
			}
		})
	}
}
