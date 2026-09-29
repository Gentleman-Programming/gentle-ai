package system

import (
	"os"
	"path/filepath"
	"testing"
)

// TestScanConfigs_ReturnsAllKnownAgentsWithExistsFlag verifies the canonical
// ScanConfigs contract: ALL known registry agents are returned, with Exists=true
// for those whose config dir is present on disk and Exists=false for those absent.
//
// This is the TUI contract: the detection screen shows "present"/"missing" for
// every known agent. The shim must enumerate all adapters from the default
// registry, not just the ones that are installed.
func TestScanConfigs_ReturnsAllKnownAgentsWithExistsFlag(t *testing.T) {
	home := t.TempDir()

	// Create only claude-code config dir — others intentionally absent.
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	configs := ScanConfigs(home)

	// Must return at least as many entries as the registry has adapters with
	// a non-empty GlobalConfigDir. Currently 18 agents are supported.
	if len(configs) < 18 {
		t.Fatalf("ScanConfigs() returned %d entries, want >= 18; got %v", len(configs), configs)
	}

	// Find claude — must be Exists=true.
	var claudeState *ConfigState
	for i := range configs {
		if configs[i].Agent == "claude-code" {
			claudeState = &configs[i]
			break
		}
	}
	if claudeState == nil {
		t.Fatalf("ScanConfigs() missing claude-code entry; got %v", configs)
	}
	if !claudeState.Exists {
		t.Errorf("ScanConfigs() claude-code Exists = false, want true (dir was created)")
	}
	if !claudeState.IsDirectory {
		t.Errorf("ScanConfigs() claude-code IsDirectory = false, want true")
	}

	// All other agents must appear with Exists=false (their dirs weren't created).
	existsTrueCount := 0
	for _, c := range configs {
		if c.Exists {
			existsTrueCount++
		}
	}
	if existsTrueCount != 1 {
		t.Errorf("ScanConfigs() Exists=true count = %d, want 1 (only claude-code created); states: %v", existsTrueCount, configs)
	}
}

// TestScanConfigs_AgentFieldMatchesModelAgentID verifies that the Agent field
// in each ConfigState matches the canonical model.AgentID string values used
// by the TUI and validate.go switch statements.
func TestScanConfigs_AgentFieldMatchesModelAgentID(t *testing.T) {
	home := t.TempDir()
	configs := ScanConfigs(home)

	// These are the AgentID string values the TUI switch statements check.
	knownAgents := map[string]bool{
		"claude-code":    false,
		"opencode":       false,
		"kilocode":       false,
		"gemini-cli":     false,
		"cursor":         false,
		"vscode-copilot": false,
		"codex":          false,
		"antigravity":    false,
		"windsurf":       false,
		"kimi":           false,
		"qwen-code":      false,
		"kiro-ide":       false,
		"openclaw":       false,
		"pi":             false,
		"trae-ide":       false,
		"hermes":         false,
		"conductor":      false,
		"command-code":   false,
	}

	for _, c := range configs {
		if _, known := knownAgents[c.Agent]; known {
			knownAgents[c.Agent] = true
		}
	}

	// All known agents must appear.
	for agent, seen := range knownAgents {
		if !seen {
			t.Errorf("ScanConfigs() missing agent %q — TUI switch statements require it; got agents: %v", agent, agentNames(configs))
		}
	}
}

// TestScanConfigs_PathFieldIsNonEmpty verifies that every ConfigState has a
// non-empty Path — the TUI and validate.go use the path for display purposes.
func TestScanConfigs_PathFieldIsNonEmpty(t *testing.T) {
	home := t.TempDir()
	configs := ScanConfigs(home)

	for _, c := range configs {
		if c.Path == "" {
			t.Errorf("ScanConfigs() agent %q has empty Path — all entries must have non-empty paths", c.Agent)
		}
	}
}

// TestScanConfigs_ExistsFalseWhenDirAbsent verifies that agents whose
// GlobalConfigDir does not exist on disk have Exists=false.
func TestScanConfigs_ExistsFalseWhenDirAbsent(t *testing.T) {
	home := t.TempDir()
	// No dirs created — all agents should have Exists=false.

	configs := ScanConfigs(home)

	for _, c := range configs {
		if c.Exists {
			t.Errorf("ScanConfigs() agent %q Exists = true, want false (no dirs created)", c.Agent)
		}
	}
}

// TestScanConfigs_IsDirectorySetForExistingDirs verifies that IsDirectory is
// set correctly for existing directories.
func TestScanConfigs_IsDirectorySetForExistingDirs(t *testing.T) {
	home := t.TempDir()

	// Create two agent dirs.
	for _, rel := range []string{".claude", ".config/opencode"} {
		if err := os.MkdirAll(filepath.Join(home, rel), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", rel, err)
		}
	}

	configs := ScanConfigs(home)

	claudeFound, opencodeFound := false, false
	for _, c := range configs {
		switch c.Agent {
		case "claude-code":
			claudeFound = true
			if !c.Exists || !c.IsDirectory {
				t.Errorf("claude-code: Exists=%v IsDirectory=%v, want both true", c.Exists, c.IsDirectory)
			}
		case "opencode":
			opencodeFound = true
			if !c.Exists || !c.IsDirectory {
				t.Errorf("opencode: Exists=%v IsDirectory=%v, want both true", c.Exists, c.IsDirectory)
			}
		}
	}

	if !claudeFound {
		t.Error("ScanConfigs() missing claude-code entry")
	}
	if !opencodeFound {
		t.Error("ScanConfigs() missing opencode entry")
	}
}

// agentNames extracts agent name strings for error messages.
func agentNames(configs []ConfigState) []string {
	names := make([]string, len(configs))
	for i, c := range configs {
		names[i] = c.Agent
	}
	return names
}

// TestScanConfigs_KimiPrefersCurrentLayout verifies that the kimi entry
// resolves to the current kimi-code v0.11+ root ~/.kimi-code when it exists
// as a directory (issue #782).
func TestScanConfigs_KimiPrefersCurrentLayout(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".kimi-code"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	var kimi *ConfigState
	for i := range ScanConfigs(home) {
		if configs := ScanConfigs(home); configs[i].Agent == "kimi" {
			kimi = &configs[i]
			break
		}
	}
	if kimi == nil {
		t.Fatal("ScanConfigs() missing kimi entry")
	}
	if kimi.Path != filepath.Join(home, ".kimi-code") {
		t.Errorf("kimi Path = %q, want the .kimi-code root", kimi.Path)
	}
	if !kimi.Exists || !kimi.IsDirectory {
		t.Errorf("kimi Exists=%v IsDirectory=%v, want both true", kimi.Exists, kimi.IsDirectory)
	}
}

// TestScanConfigs_KimiUnexpectedStatErrorDoesNotSelectLegacy verifies that a
// stat failure other than absence (ENOENT/ENOTDIR) on the current config root
// does not silently select the legacy layout: the kimi entry keeps the
// preferred current root path and is reported as absent because the layout
// is undeterminable.
func TestScanConfigs_KimiUnexpectedStatErrorDoesNotSelectLegacy(t *testing.T) {
	home := t.TempDir()
	statErr := os.ErrPermission

	original := kimiConfigStat
	kimiConfigStat = func(string) (os.FileInfo, error) { return nil, statErr }
	defer func() { kimiConfigStat = original }()

	configs := ScanConfigs(home)
	var kimi *ConfigState
	for i, c := range configs {
		if c.Agent == "kimi" {
			kimi = &configs[i]
			break
		}
	}
	if kimi == nil {
		t.Fatal("ScanConfigs() missing kimi entry")
	}
	if kimi.Path != filepath.Join(home, ".kimi-code") {
		t.Errorf("kimi Path = %q, want the preferred .kimi-code root (not the legacy fallback)", kimi.Path)
	}
	if kimi.Exists {
		t.Error("kimi Exists = true, want false when the layout is undeterminable")
	}
}

// TestScanConfigs_KimiCodeFileIsNotCurrentLayout verifies that a plain file
// named .kimi-code is not treated as the v0.11+ layout: the scan falls back
// to the legacy ~/.kimi root.
func TestScanConfigs_KimiCodeFileIsNotCurrentLayout(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".kimi-code"), []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var kimi *ConfigState
	for i := range ScanConfigs(home) {
		if ScanConfigs(home)[i].Agent == "kimi" {
			kimi = &ScanConfigs(home)[i]
			break
		}
	}
	if kimi == nil {
		t.Fatal("ScanConfigs() missing kimi entry")
	}
	if kimi.Path != filepath.Join(home, ".kimi") {
		t.Errorf("kimi Path = %q, want the legacy .kimi root", kimi.Path)
	}
	if kimi.Exists {
		t.Errorf("kimi Exists = true, want false (only a .kimi-code file exists)")
	}
}

// TestScanConfigs_KimiLegacyFallback verifies that the kimi entry keeps
// reporting the legacy ~/.kimi root when no .kimi-code directory exists.
func TestScanConfigs_KimiLegacyFallback(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".kimi"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	var kimi *ConfigState
	for i := range ScanConfigs(home) {
		if ScanConfigs(home)[i].Agent == "kimi" {
			kimi = &ScanConfigs(home)[i]
			break
		}
	}
	if kimi == nil {
		t.Fatal("ScanConfigs() missing kimi entry")
	}
	if kimi.Path != filepath.Join(home, ".kimi") {
		t.Errorf("kimi Path = %q, want the legacy .kimi root", kimi.Path)
	}
	if !kimi.Exists || !kimi.IsDirectory {
		t.Errorf("kimi Exists=%v IsDirectory=%v, want both true", kimi.Exists, kimi.IsDirectory)
	}
}
