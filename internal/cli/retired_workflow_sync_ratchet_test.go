package cli

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	opencodeagent "github.com/gentleman-programming/gentle-ai/v4/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
)

// TestFreshSyncWritesNoRetiredWorkflowReferences extends the embedded-asset
// and rendered-guidance ratchets to every byte a sync writes for each agent:
// skills, MCP and runtime configs, and agent YAML at any nesting level (Kimi
// agent specs included). The home starts empty, so any match was written by
// Gentle AI itself. Backup snapshots are excluded because they copy prior
// bytes rather than produce new ones. OpenCode runs against both runtime
// majors because each writes a different settings shape.
func TestFreshSyncWritesNoRetiredWorkflowReferences(t *testing.T) {
	components := []model.ComponentID{}
	for _, component := range catalog.MVPComponents() {
		components = append(components, component.ID)
	}
	type syncCase struct {
		agent   model.AgentID
		version string
	}
	var cases []syncCase
	for _, agent := range catalog.AllAgents() {
		switch agent.ID {
		case model.AgentConductor:
			continue // Catalog-only: sync has no target for it.
		case model.AgentOpenCode:
			cases = append(cases, syncCase{agent.ID, "1.18.30"}, syncCase{agent.ID, "2.0.23"})
		default:
			cases = append(cases, syncCase{agent.ID, "1.18.30"})
		}
	}
	for _, tc := range cases {
		t.Run(string(tc.agent)+"@"+tc.version, func(t *testing.T) {
			home := t.TempDir()
			setSyncTestHome(t, home)
			previous := opencode.VersionRunnerOverride
			t.Cleanup(func() { opencode.VersionRunnerOverride = previous })
			opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
				return opencode.CommandOutput{Stdout: []byte(tc.version)}, nil
			}
			selected := components
			if tc.agent == model.AgentOpenCode && strings.HasPrefix(tc.version, "2.") {
				// V2 refuses managed plugins until its SDK is installed.
				config := opencodeagent.NewAdapter().GlobalConfigDir(home)
				writeSDKRangeFile(t, filepath.Join(config, "package.json"), `{"packageManager":"npm@10.8.0"}`)
				writeSDKRangeFile(t, filepath.Join(config, "node_modules", "@opencode", "plugin", "package.json"), `{"version":"`+tc.version+`"}`)
				// V2 skips the logo, yet post-sync verification still expects
				// its files; that separate defect is out of this ratchet's scope.
				selected = slices.DeleteFunc(slices.Clone(components), func(id model.ComponentID) bool { return id == model.ComponentOpenCodeGentleLogo })
			}
			if err := state.Write(home, state.InstallState{
				InstalledAgents: []string{string(tc.agent)}, SelectionConfigured: true,
				Components: selected, Persona: string(model.PersonaGentleman),
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := RunSync([]string{"--agents", string(tc.agent)}); err != nil {
				t.Fatalf("RunSync() error = %v", err)
			}
			backups := filepath.Join(home, ".gentle-ai", "backups")
			scanned := 0
			err := filepath.WalkDir(home, func(path string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if path == backups {
					return filepath.SkipDir
				}
				if retiredWorkflowText.MatchString(entry.Name()) {
					t.Errorf("sync wrote a path naming a retired workflow: %s", path)
				}
				if entry.IsDir() {
					return nil
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				scanned++
				for index, line := range strings.Split(string(data), "\n") {
					if retiredWorkflowText.MatchString(line) {
						t.Errorf("%s:%d: sync wrote a retired workflow reference: %s", path, index+1, strings.TrimSpace(line))
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if scanned == 0 {
				t.Fatal("sync wrote no files to scan")
			}
		})
	}
}
