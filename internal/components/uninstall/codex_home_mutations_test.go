package uninstall

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/codex"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/agenthooks"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/mcp"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/persona"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/skills"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

// TestCodexMutationsValidateHome exercises the public component boundaries.
// Refusal must preserve files and counters, not merely return an error after a write.
func TestCodexMutationsValidateHome(t *testing.T) {
	restore := codex.SetRuntimeVersionCommandForTest("codex-cli 0.144.0", nil)
	t.Cleanup(restore)
	for _, operation := range []string{"engram", "mcp", "skills", "persona", "persona sync", "uninstall", "telemetry hooks", "registry hooks", "routing"} {
		for _, scenario := range []string{"missing", "relative missing", "file", "valid", "unset", "isolated"} {
			t.Run(operation+"/"+scenario, func(t *testing.T) {
				root := t.TempDir()
				home := filepath.Join(root, "home")
				workspace := filepath.Join(root, "workspace")
				for _, dir := range []string{home, workspace, filepath.Join(home, ".codex")} {
					if err := os.MkdirAll(dir, 0o755); err != nil {
						t.Fatal(err)
					}
				}
				t.Setenv("HOME", home)
				t.Setenv("USERPROFILE", home)
				override := filepath.Join(root, "override")
				switch scenario {
				case "relative missing":
					cwd, err := os.Getwd()
					if err != nil {
						t.Fatal(err)
					}
					override, err = filepath.Rel(cwd, override)
					if err != nil {
						t.Fatal(err)
					}
				case "file":
					if err := os.WriteFile(override, []byte("not a directory\n"), 0o600); err != nil {
						t.Fatal(err)
					}
				case "valid":
					if err := os.MkdirAll(override, 0o755); err != nil {
						t.Fatal(err)
					}
				case "unset":
					override = ""
				case "isolated":
					realHome := filepath.Join(root, "other-user-home")
					if err := os.MkdirAll(realHome, 0o755); err != nil {
						t.Fatal(err)
					}
					t.Setenv("HOME", realHome)
					t.Setenv("USERPROFILE", realHome)
				}
				t.Setenv("CODEX_HOME", override)
				// A stale default-root config must never be changed on refusal.
				if err := os.WriteFile(filepath.Join(home, ".codex", "config.toml"), []byte("# user config\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				adapter := codex.NewAdapter()
				var call func() (any, error)
				switch operation {
				case "engram":
					call = func() (any, error) { return engram.InjectWithOptions(home, adapter, engram.InjectOptions{}) }
				case "mcp":
					call = func() (any, error) { return mcp.Inject(home, home, adapter) }
				case "skills":
					call = func() (any, error) {
						return skills.InjectWithCapability(home, adapter, []model.SkillID{"issue-creation"}, "")
					}
				case "persona":
					call = func() (any, error) { return persona.Inject(home, adapter, model.PersonaGentleman) }
				case "persona sync":
					call = func() (any, error) { return persona.InjectForSync(home, adapter, model.PersonaGentleman) }
				case "telemetry hooks":
					call = func() (any, error) { return agenthooks.InstallCodexTelemetry(home, adapter) }
				case "registry hooks":
					call = func() (any, error) { return agenthooks.InstallSkillRegistry(home, adapter) }
				case "routing":
					call = func() (any, error) {
						return agentguidance.InjectRoutingWithOptions(home, model.AgentCodex, agentguidance.RoutingOptions{
							ReviewContract: func(model.AgentID) (string, error) { return "test review execution contract", nil },
						})
					}
				case "uninstall":
					svc, err := NewService(home, workspace, "dev")
					if err != nil {
						t.Fatal(err)
					}
					call = func() (any, error) {
						return svc.PartialUninstall([]model.AgentID{model.AgentCodex}, []model.ComponentID{model.ComponentEngram})
					}
				}
				before := codexMutationTree(t, root)
				validationErr := system.ValidateCodexHome(home)
				result, err := call()
				if validationErr == nil {
					if err != nil {
						t.Fatalf("valid or isolated home rejected: %v", err)
					}
					return
				}
				if err == nil || !strings.Contains(err.Error(), validationErr.Error()) {
					t.Errorf("error = %v, want CODEX_HOME validation error %q", err, validationErr)
				}
				if !reflect.DeepEqual(result, reflect.Zero(reflect.TypeOf(result)).Interface()) {
					t.Errorf("rejected mutation returned nonzero result: %#v", result)
				}
				if after := codexMutationTree(t, root); !reflect.DeepEqual(after, before) {
					t.Error("rejected mutation changed files, directories or permissions")
				}
			})
		}
	}
}

func codexMutationTree(t *testing.T, root string) map[string]string {
	t.Helper()
	entries := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		value := info.Mode().String()
		if !entry.IsDir() {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += ":" + string(content)
		}
		entries[rel] = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return entries
}
