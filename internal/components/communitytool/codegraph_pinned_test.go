package communitytool

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

func writePinnedOpenCodeSettings(t *testing.T, settings string) (string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	path := filepath.Join(home, ".config", "opencode", "opencode.json")
	mustWrite(t, path, settings)
	return home, path
}

func TestInstallCodeGraphPinnedNativePathIsUserConfig(t *testing.T) {
	home, path := writePinnedOpenCodeSettings(t, `{"mcp":{"servers":{"codegraph":{"type":"local","command":["codegraph","serve","--mcp","--path","/other"]}}}}`)
	if _, err := InjectCodeGraphGuidance(home); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	detector := DetectorFunc(func(string) (string, error) { return "/bin/codegraph", nil })
	calls := 0
	runner := RunnerFunc(func(name string, args ...string) error {
		calls++
		t.Errorf("unexpected runner call %s %v", name, args)
		return nil
	})
	if _, err := InstallWithHome(model.CommunityToolCodeGraph, "/work/p", home, runner, detector); err != nil {
		t.Errorf("InstallWithHome() with pinned native path: %v", err)
	}
	if calls != 0 {
		t.Errorf("runner calls = %d, want zero", calls)
	}
	if after, err := os.ReadFile(path); err != nil || string(after) != string(before) {
		t.Errorf("pinned native config bytes changed: %q, error=%v", after, err)
	}
	found := false
	for _, agent := range DetectStatus(model.CommunityToolCodeGraph, home, detector).Agents {
		if agent.Agent != model.AgentOpenCode {
			continue
		}
		found = true
		if !agent.Detected || !agent.Configured {
			t.Errorf("OpenCode status = detected %v configured %v (%q), want configured by user pin", agent.Detected, agent.Configured, agent.Reason)
		}
	}
	if !found {
		t.Error("status did not report OpenCode")
	}
}

func TestReconcileOpenCodeCodeGraphLegacyPinnedPathIsUserConfig(t *testing.T) {
	for _, legacy := range []string{
		`{"mcp":{"codegraph":{"type":"local","command":["codegraph","serve","--mcp","--path","/other"],"enabled":true}}}`,
		`{"mcp":{"codegraph":{"type":"local","command":["codegraph","serve","-p","/other","--no-watch","--mcp"]}}}`,
	} {
		t.Run(legacy, func(t *testing.T) {
			home, path := writePinnedOpenCodeSettings(t, legacy)
			calls := 0
			// Mirrors the upstream installer, which replaces the pinned command.
			runner := RunnerFunc(func(string, ...string) error {
				calls++
				mustWrite(t, path, `{"mcp":{"codegraph":{"type":"local","command":["codegraph","serve","--mcp"],"enabled":true}}}`)
				return nil
			})
			if NeedsOpenCodeCodeGraphReconcile(home) {
				t.Error("legacy pinned path needs reconciliation")
			}
			result, err := ReconcileOpenCodeCodeGraph(home, runner)
			if err != nil || result.Changed || len(result.Files) != 0 {
				t.Errorf("reconcile = (%#v, %v), want skipped success", result, err)
			}
			if calls != 0 {
				t.Errorf("installer calls = %d, want zero", calls)
			}
			if after, err := os.ReadFile(path); err != nil || string(after) != legacy {
				t.Errorf("legacy pinned config rewritten: %q, error=%v", after, err)
			}
		})
	}
}

func TestReconcileOpenCodeCodeGraphMalformedPinKeepsDiagnostic(t *testing.T) {
	for _, tt := range []struct{ name, entry, reason string }{
		{"foreign remote pin", `{"type":"remote","command":["other","-p","x"]}`, "type must be local"},
		{"foreign local program pin", `{"type":"local","command":["other","serve","--mcp","-p","x"]}`, "command must be"},
		{"empty equals path", `{"type":"local","command":["codegraph","serve","--mcp","--path="]}`, "command must be"},
		{"missing short path value", `{"type":"local","command":["codegraph","serve","--mcp","-p"]}`, "command must be"},
		{"flag as path value", `{"type":"local","command":["codegraph","serve","--path","--mcp"]}`, "command must be"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home, path := writePinnedOpenCodeSettings(t, `{"mcp":{"servers":{"codegraph":`+tt.entry+`}}}`)
			calls := 0
			runner := RunnerFunc(func(string, ...string) error { calls++; return errors.New("installer must not run") })
			if !NeedsOpenCodeCodeGraphReconcile(home) {
				t.Error("malformed pin hidden as user configuration")
			}
			_, err := ReconcileOpenCodeCodeGraph(home, runner)
			if err == nil {
				t.Fatal("reconcile error = nil, want actionable native diagnostic")
			}
			for _, want := range []string{path, "mcp.servers.codegraph", tt.reason} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("diagnostic %q must name %q", err, want)
				}
			}
			if calls != 0 {
				t.Errorf("installer calls = %d, want zero", calls)
			}
		})
	}
}

func TestReconcileOpenCodeCodeGraphNativePinIgnoresLegacyEnabled(t *testing.T) {
	settings := `{"mcp":{"servers":{"codegraph":{"type":"local","command":["codegraph","serve","--mcp","--path","/x"],"enabled":false}}}}`
	home, path := writePinnedOpenCodeSettings(t, settings)
	calls := 0
	runner := RunnerFunc(func(string, ...string) error { calls++; return errors.New("installer must not run") })
	if NeedsOpenCodeCodeGraphReconcile(home) {
		t.Error("native pinned path needs reconciliation")
	}
	if result, err := ReconcileOpenCodeCodeGraph(home, runner); err != nil || result.Changed {
		t.Errorf("reconcile = (%#v, %v), want skipped success", result, err)
	}
	if calls != 0 {
		t.Errorf("installer calls = %d, want zero", calls)
	}
	if after, err := os.ReadFile(path); err != nil || string(after) != settings {
		t.Errorf("native pinned config bytes changed: %q, error=%v", after, err)
	}
}
