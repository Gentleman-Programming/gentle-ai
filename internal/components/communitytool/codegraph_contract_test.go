package communitytool

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func TestCodeGraphCompatibilityTableIsExhaustive(t *testing.T) {
	reg, err := agents.NewDefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	want := reg.SupportedAgents()
	got := make([]model.AgentID, 0, len(codeGraphCompatibilityTable))
	for id, compatibility := range codeGraphCompatibilityTable {
		got = append(got, id)
		if compatibility.Agent != id {
			t.Errorf("entry %q owns agent %q", id, compatibility.Agent)
		}
		if compatibility.Strategy == codeGraphExcluded {
			if compatibility.Target != "" || compatibility.OwnedPaths != nil || compatibility.Postcondition != nil {
				t.Errorf("excluded agent %q owns CodeGraph behavior", id)
			}
		} else if compatibility.OwnedPaths == nil || compatibility.Postcondition == nil {
			t.Errorf("compatible agent %q lacks owned path/postcondition semantics", id)
		}
	}
	slices.Sort(want)
	slices.Sort(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("compatibility agents = %v, want registry agents %v", got, want)
	}
	if codeGraphUpstreamVersion != "1.4.1" {
		t.Fatalf("upstream contract = %q", codeGraphUpstreamVersion)
	}
}

func TestCodeGraphCompatibilityStrategies(t *testing.T) {
	tests := []struct {
		strategy codeGraphStrategy
		agents   []model.AgentID
	}{
		{codeGraphNative, []model.AgentID{model.AgentClaudeCode, model.AgentCursor, model.AgentCodex, model.AgentGeminiCLI, model.AgentHermes, model.AgentAntigravity, model.AgentKiroIDE}},
		{codeGraphReconciled, []model.AgentID{model.AgentOpenCode, model.AgentPi}},
		{codeGraphExcluded, []model.AgentID{model.AgentKilocode, model.AgentVSCodeCopilot, model.AgentWindsurf, model.AgentKimi, model.AgentQwenCode, model.AgentOpenClaw, model.AgentTrae}},
	}
	for _, tt := range tests {
		for _, id := range tt.agents {
			if got := codeGraphCompatibilityTable[id].Strategy; got != tt.strategy {
				t.Errorf("%s strategy = %s, want %s", id, got, tt.strategy)
			}
		}
	}
}

func TestExcludedAgentsNeverEnterCodeGraphSurfaces(t *testing.T) {
	reg, err := agents.NewDefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []model.AgentID{model.AgentKilocode, model.AgentVSCodeCopilot, model.AgentWindsurf, model.AgentKimi, model.AgentQwenCode, model.AgentOpenClaw, model.AgentTrae} {
		t.Run(string(id), func(t *testing.T) {
			home := t.TempDir()
			adapter, ok := reg.Get(id)
			if !ok {
				t.Fatal("adapter missing")
			}
			root := adapter.GlobalConfigDir(home)
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatal(err)
			}
			before, _ := filesBelow(root)
			status := DetectStatus(model.CommunityToolCodeGraph, home, DetectorFunc(func(string) (string, error) { return "/bin/codegraph", nil }))
			if slices.ContainsFunc(status.Agents, func(agent AgentStatus) bool { return agent.Agent == id }) {
				t.Fatalf("excluded agent entered status: %#v", status.Agents)
			}
			if slices.Contains(detectedCodeGraphTargets(home), codeGraphCompatibilityTable[id].Target) && codeGraphCompatibilityTable[id].Target != "" {
				t.Fatal("excluded agent entered targets")
			}
			if _, err := InjectCodeGraphGuidance(home); err != nil {
				t.Fatal(err)
			}
			after, _ := filesBelow(root)
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("excluded root changed: before=%v after=%v", before, after)
			}
			for _, path := range CodeGraphManagedPaths(home) {
				if path == root || strings.HasPrefix(path, root+string(os.PathSeparator)) {
					t.Fatalf("excluded path became candidate-owned: %s", path)
				}
			}
		})
	}
}

func TestClaudeGlobalConfigRequiresValidMCPServersEntry(t *testing.T) {
	home := t.TempDir()
	reg, _ := agents.NewDefaultRegistry()
	adapter, _ := reg.Get(model.AgentClaudeCode)
	path := filepath.Join(home, ".claude.json")
	for _, tt := range []struct {
		name, data string
		want       bool
	}{
		{"valid", `{"theme":"dark","mcpServers":{"other":{"command":"x"},"codegraph":{"command":"codegraph"}}}`, true},
		{"missing key", `{"mcpServers":{"other":{"command":"codegraph"}}}`, false},
		{"null entry", `{"mcpServers":{"codegraph":null}}`, false},
		{"malformed entry", `{"mcpServers":{"codegraph":"codegraph"}}`, false},
		{"wrong command", `{"mcpServers":{"codegraph":{"command":"other"}}}`, false},
		{"invalid JSON", `{`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mustWrite(t, path, tt.data)
			_, got := hasCodeGraphToolWiring(home, adapter)
			if got != tt.want {
				t.Fatalf("configured = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAntigravityRequiresCanonicalCodeGraphEntry(t *testing.T) {
	home := t.TempDir()
	reg, _ := agents.NewDefaultRegistry()
	adapter, _ := reg.Get(model.AgentAntigravity)
	path := filepath.Join(home, ".gemini", "antigravity", "mcp_config.json")
	for _, tt := range []struct {
		name, data string
		want       bool
	}{
		{"valid", `{"mcpServers":{"codegraph":{"command":"codegraph"}}}`, true},
		{"unrelated key", `{"mcpServers":{"not-codegraph":{"command":"codegraph"}}}`, false},
		{"wrong command", `{"mcpServers":{"codegraph":{"command":"other-codegraph"}}}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mustWrite(t, path, tt.data)
			_, got := hasCodeGraphToolWiring(home, adapter)
			if got != tt.want {
				t.Fatalf("configured = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCodeGraphNativeOwnedPaths(t *testing.T) {
	home := t.TempDir()
	reg, _ := agents.NewDefaultRegistry()
	tests := map[model.AgentID][]string{
		model.AgentClaudeCode:  {filepath.Join(home, ".claude.json")},
		model.AgentAntigravity: {filepath.Join(home, ".gemini", "config", "mcp_config.json"), filepath.Join(home, ".gemini", "antigravity", "mcp_config.json")},
		model.AgentKiroIDE:     {filepath.Join(home, ".kiro", "settings", "mcp.json")},
	}
	for id, want := range tests {
		adapter, _ := reg.Get(id)
		got := codeGraphToolWiringPaths(home, adapter)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s paths = %v, want %v", id, got, want)
		}
	}
}

func TestCodeGraphCommandsUseExplicitMultipleTargets(t *testing.T) {
	commands, err := CodeGraphCommandsForDetectorAndTargets(DetectorFunc(func(name string) (string, error) {
		if name == "npm" {
			return "/bin/npm", nil
		}
		return "", errors.New("missing")
	}), []string{"claude", "cursor", "kiro"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"codegraph", "install", "--target", "claude,cursor,kiro", "--location", "global", "--yes"}
	if !reflect.DeepEqual(commands[1], want) {
		t.Fatalf("install command = %v, want %v", commands[1], want)
	}
}

func TestCodeGraphRollbackRestoresBytesModesAndRemovesCreatedFiles(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	global := filepath.Join(home, ".claude.json")
	prompt := filepath.Join(home, ".claude", "CLAUDE.md")
	mustWrite(t, global, `{"theme":"dark"}`)
	if err := os.Chmod(global, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := InstallWithHome(model.CommunityToolCodeGraph, "", home, RunnerFunc(func(string, ...string) error {
		mustWrite(t, global, `{"mcpServers":{"codegraph":{"command":"codegraph"}}}`)
		mustWrite(t, prompt, "created")
		return errors.New("upstream failed")
	}), DetectorFunc(func(string) (string, error) { return "/bin/codegraph", nil }))
	if err == nil {
		t.Fatal("InstallWithHome() error = nil")
	}
	data, _ := os.ReadFile(global)
	info, _ := os.Stat(global)
	if string(data) != `{"theme":"dark"}` || runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("rollback global = %q mode %o", data, info.Mode().Perm())
	}
	if _, err := os.Stat(prompt); !os.IsNotExist(err) {
		t.Fatalf("candidate-created prompt remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); err != nil {
		t.Fatalf("rollback deleted parent directory: %v", err)
	}
}

func TestCodeGraphRollbackAfterGuidanceAndPostconditionFailures(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(*testing.T, string)
		run   func(*testing.T, string, string, ...string) error
	}{
		{
			name: "guidance",
			setup: func(t *testing.T, home string) {
				if err := os.MkdirAll(filepath.Join(home, ".claude", "CLAUDE.md"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			run: func(t *testing.T, home, _ string, _ ...string) error {
				mustWrite(t, filepath.Join(home, ".claude.json"), `{"mcpServers":{"codegraph":{"command":"codegraph"}}}`)
				return nil
			},
		},
		{
			name:  "postcondition",
			setup: func(*testing.T, string) {},
			run:   func(*testing.T, string, string, ...string) error { return nil },
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
				t.Fatal(err)
			}
			global := filepath.Join(home, ".claude.json")
			mustWrite(t, global, `{"sibling":true}`)
			tt.setup(t, home)
			_, err := InstallWithHome(model.CommunityToolCodeGraph, "", home, RunnerFunc(func(name string, args ...string) error {
				return tt.run(t, home, name, args...)
			}), DetectorFunc(func(string) (string, error) { return "/bin/codegraph", nil }))
			if err == nil {
				t.Fatal("InstallWithHome() error = nil")
			}
			data, _ := os.ReadFile(global)
			if string(data) != `{"sibling":true}` {
				t.Fatalf("rollback bytes = %q", data)
			}
		})
	}
}

func TestCodeGraphSnapshotRoundTripIsIdempotent(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".kiro", "settings", "mcp.json")
	mustWrite(t, path, `{"mcpServers":{"sibling":{"command":"x"}}}`)
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	snapshots, err := snapshotCodeGraphPaths([]string{path, filepath.Join(home, "new.json")})
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, path, `{}`)
	mustWrite(t, filepath.Join(home, "new.json"), `{}`)
	if err := restoreCodeGraphPaths(snapshots); err != nil {
		t.Fatal(err)
	}
	if err := restoreCodeGraphPaths(snapshots); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	info, _ := os.Stat(path)
	if string(data) != `{"mcpServers":{"sibling":{"command":"x"}}}` || runtime.GOOS != "windows" && info.Mode().Perm() != 0o640 {
		t.Fatalf("round trip = %q mode %o", data, info.Mode().Perm())
	}
}

func filesBelow(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}
