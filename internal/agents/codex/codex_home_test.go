package codex

import (
	"context"
	"path/filepath"
	"testing"
)

// canonicalPath mirrors system.CodexConfigDir's EvalSymlinks step so
// expectations hold on hosts whose TMPDIR contains symlinked components.
func canonicalPath(t *testing.T, path string) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", path, err)
	}
	return real
}

// TestPathsHonorCodexHomeOverride pins the codex-rs rule (issue #5031): with
// CODEX_HOME set to an existing directory and the supplied home equal to the
// real user home, every derived Codex path must resolve under the override
// instead of <home>/.codex.
func TestPathsHonorCodexHomeOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	override := t.TempDir()
	t.Setenv("CODEX_HOME", override)
	wantRoot := canonicalPath(t, override)

	a := NewAdapter()
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"GlobalConfigDir", a.GlobalConfigDir(home), wantRoot},
		{"SystemPromptDir", a.SystemPromptDir(home), wantRoot},
		{"SystemPromptFile", a.SystemPromptFile(home), filepath.Join(wantRoot, "AGENTS.md")},
		{"SkillsDir", a.SkillsDir(home), filepath.Join(wantRoot, "skills")},
		{"MCPConfigPath", a.MCPConfigPath(home, "engram"), filepath.Join(wantRoot, "config.toml")},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: got %q, want %q (CODEX_HOME=%q, home=%q)", tc.name, tc.got, tc.want, override, home)
		}
	}
}

// TestDetectReportsConfigUnderCodexHomeOverride: Detect must stat and report
// the config directory under the override, so presence is not reported from
// the wrong root (issue #5031, Impact).
func TestDetectReportsConfigUnderCodexHomeOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	override := t.TempDir()
	t.Setenv("CODEX_HOME", override)
	wantRoot := canonicalPath(t, override)

	a := NewAdapter()
	_, _, configPath, configFound, err := a.Detect(context.Background(), home)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !configFound {
		t.Fatal("Detect: config directory under CODEX_HOME not reported as found")
	}
	if configPath != wantRoot {
		t.Errorf("Detect configPath: got %q, want %q", configPath, wantRoot)
	}
}

// TestPathsPreserveIsolatedHomeUnderOverride pins the guard from the issue's
// suggested shape: an ambient CODEX_HOME must never redirect callers that
// supply an isolated home (sandbox or test HOME); they keep <isolated>/.codex.
func TestPathsPreserveIsolatedHomeUnderOverride(t *testing.T) {
	realHome := t.TempDir()
	t.Setenv("HOME", realHome)

	override := t.TempDir()
	t.Setenv("CODEX_HOME", override)

	isolated := t.TempDir()
	a := NewAdapter()
	if got, want := a.GlobalConfigDir(isolated), filepath.Join(isolated, ".codex"); got != want {
		t.Errorf("GlobalConfigDir(isolated home): got %q, want %q", got, want)
	}
	if got, want := a.SystemPromptFile(isolated), filepath.Join(isolated, ".codex", "AGENTS.md"); got != want {
		t.Errorf("SystemPromptFile(isolated home): got %q, want %q", got, want)
	}
	if got, want := a.SkillsDir(isolated), filepath.Join(isolated, ".codex", "skills"); got != want {
		t.Errorf("SkillsDir(isolated home): got %q, want %q", got, want)
	}
	if got, want := a.MCPConfigPath(isolated, "engram"), filepath.Join(isolated, ".codex", "config.toml"); got != want {
		t.Errorf("MCPConfigPath(isolated home): got %q, want %q", got, want)
	}
}
