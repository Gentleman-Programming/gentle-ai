package agentbuilder_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agentbuilder"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

func TestValidateAgentName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"valid-agent", false},
		{"agent_123", false},
		{"custom.agent", false},
		{"", true},
		{"   ", true},
		{".", true},
		{"..", true},
		{"../escape", true},
		{"path/traversal", true},
		{"path\\traversal", true},
	}

	for _, tt := range tests {
		err := agentbuilder.ValidateAgentName(tt.name)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateAgentName(%q) err = %v, wantErr = %v", tt.name, err, tt.wantErr)
		}
	}
}

func TestUninstall_Success(t *testing.T) {
	tempDir := t.TempDir()
	regPath := filepath.Join(tempDir, "custom-agents.json")
	claudeSkills := filepath.Join(tempDir, "claude-skills")
	opencodeSkills := filepath.Join(tempDir, "opencode-skills")

	if err := os.MkdirAll(claudeSkills, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(opencodeSkills, 0755); err != nil {
		t.Fatal(err)
	}

	adapters := []agentbuilder.AdapterInfo{
		{AgentID: model.AgentClaudeCode, SkillsDir: claudeSkills},
		{AgentID: model.AgentOpenCode, SkillsDir: opencodeSkills},
	}

	// Install 2 agents on disk
	for _, name := range []string{"agent-a", "agent-b"} {
		for _, ad := range adapters {
			dir := filepath.Join(ad.SkillsDir, name)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Populate registry
	reg := &agentbuilder.Registry{
		Version: 1,
		Agents: []agentbuilder.RegistryEntry{
			{Name: "agent-a", Title: "Agent A", CreatedAt: time.Now()},
			{Name: "agent-b", Title: "Agent B", CreatedAt: time.Now()},
		},
	}
	if err := agentbuilder.SaveRegistry(regPath, reg); err != nil {
		t.Fatal(err)
	}

	// Uninstall agent-a
	if err := agentbuilder.Uninstall(regPath, []string{"agent-a"}, adapters); err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}

	// Verify agent-a removed from registry and disk
	updatedReg, err := agentbuilder.LoadRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	if updatedReg.FindByName("agent-a") != nil {
		t.Errorf("agent-a still present in registry")
	}
	if updatedReg.FindByName("agent-b") == nil {
		t.Errorf("agent-b missing from registry")
	}

	for _, ad := range adapters {
		pathA := filepath.Join(ad.SkillsDir, "agent-a", "SKILL.md")
		if _, err := os.Stat(pathA); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed", pathA)
		}
		pathB := filepath.Join(ad.SkillsDir, "agent-b", "SKILL.md")
		if _, err := os.Stat(pathB); err != nil {
			t.Errorf("expected %s to still exist", pathB)
		}
	}
}

func TestUninstall_SymlinkSafety(t *testing.T) {
	tempDir := t.TempDir()
	regPath := filepath.Join(tempDir, "custom-agents.json")
	skillsDir := filepath.Join(tempDir, "skills")
	externalDir := filepath.Join(tempDir, "external-target")

	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(externalDir, 0755); err != nil {
		t.Fatal(err)
	}
	externalFile := filepath.Join(externalDir, "SKILL.md")
	if err := os.WriteFile(externalFile, []byte("external payload"), 0644); err != nil {
		t.Fatal(err)
	}

	// Symlink in skillsDir pointing to externalDir
	symlinkPath := filepath.Join(skillsDir, "symlinked-agent")
	if err := os.Symlink(externalDir, symlinkPath); err != nil {
		t.Skip("symlinks not supported on this environment")
	}

	reg := &agentbuilder.Registry{
		Version: 1,
		Agents:  []agentbuilder.RegistryEntry{{Name: "symlinked-agent"}},
	}
	if err := agentbuilder.SaveRegistry(regPath, reg); err != nil {
		t.Fatal(err)
	}

	adapters := []agentbuilder.AdapterInfo{
		{AgentID: model.AgentClaudeCode, SkillsDir: skillsDir},
	}

	if err := agentbuilder.Uninstall(regPath, []string{"symlinked-agent"}, adapters); err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}

	// Symlink itself should be removed
	if _, err := os.Lstat(symlinkPath); !os.IsNotExist(err) {
		t.Errorf("symlink %s should be removed", symlinkPath)
	}

	// External target file must remain untouched
	if _, err := os.Stat(externalFile); err != nil {
		t.Errorf("external target %s was modified or deleted", externalFile)
	}
}

// TestUninstall_RemovesEntirePayloadDirectory pins that uninstall removes the
// agent's whole payload directory, not just SKILL.md: partial removals would
// leave untracked leftovers with no registry entry to retry from.
func TestUninstall_RemovesEntirePayloadDirectory(t *testing.T) {
	tempDir := t.TempDir()
	regPath := filepath.Join(tempDir, "custom-agents.json")
	skillsDir := filepath.Join(tempDir, "skills")

	agentDir := filepath.Join(skillsDir, "my-agent")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "SKILL.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "extra-asset.txt"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(agentDir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "nested", "deep.bin"), []byte("z"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := &agentbuilder.Registry{
		Version: 1,
		Agents:  []agentbuilder.RegistryEntry{{Name: "my-agent"}},
	}
	_ = agentbuilder.SaveRegistry(regPath, reg)

	adapters := []agentbuilder.AdapterInfo{
		{AgentID: model.AgentClaudeCode, SkillsDir: skillsDir},
	}

	if err := agentbuilder.Uninstall(regPath, []string{"my-agent"}, adapters); err != nil {
		t.Fatalf("uninstall with extra payload files reported: %v", err)
	}
	if _, err := os.Lstat(agentDir); !os.IsNotExist(err) {
		t.Errorf("agent directory with extras should be fully removed, got: %v", err)
	}
	final, err := agentbuilder.LoadRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(final.Agents) != 0 {
		t.Errorf("expected empty registry after uninstall, got %d entries", len(final.Agents))
	}
}

func TestUninstall_RollbackOnSaveFailure(t *testing.T) {
	tempDir := t.TempDir()
	regPath := filepath.Join(tempDir, "custom-agents.json")
	skillsDir := filepath.Join(tempDir, "skills")
	agentDir := filepath.Join(skillsDir, "my-agent")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillPath := filepath.Join(agentDir, "SKILL.md")
	content := []byte("original content")
	if err := os.WriteFile(skillPath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	reg := &agentbuilder.Registry{
		Version: 1,
		Agents:  []agentbuilder.RegistryEntry{{Name: "my-agent"}},
	}
	if err := agentbuilder.SaveRegistry(regPath, reg); err != nil {
		t.Fatal(err)
	}
	// Make SaveRegistry fail only at persistence time: removals already
	// succeeded, so this exercises the real rollback path.
	if err := os.Chmod(regPath, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(regPath, 0o600) })

	adapters := []agentbuilder.AdapterInfo{
		{AgentID: model.AgentClaudeCode, SkillsDir: skillsDir},
	}

	err := agentbuilder.Uninstall(regPath, []string{"my-agent"}, adapters)
	if err == nil {
		t.Fatal("expected error on save failure, got nil")
	}
	if !strings.Contains(err.Error(), "save registry") {
		t.Errorf("expected save-registry failure, got: %v", err)
	}

	// Verify rollback restored the file
	restored, readErr := os.ReadFile(skillPath)
	if readErr != nil {
		t.Fatalf("file not restored: %v", readErr)
	}
	if string(restored) != string(content) {
		t.Errorf("restored content = %q, want %q", string(restored), string(content))
	}

	// Registry entry must remain so the uninstall is retryable.
	final, err := agentbuilder.LoadRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(final.Agents) != 1 || final.Agents[0].Name != "my-agent" {
		t.Errorf("registry should still list the agent for retry, got %+v", final.Agents)
	}
}

// TestUninstall_RestoreFailureSurfaces: covered in uninstall_internal_test.go
// (runRestorers is package-internal); this file keeps end-to-end guarantees.

func TestUninstall_RollbackMidwayMultiAgent(t *testing.T) {
	tempDir := t.TempDir()
	regPath := filepath.Join(tempDir, "custom-agents.json")
	skillsDir := filepath.Join(tempDir, "skills")

	adapters := []agentbuilder.AdapterInfo{
		{AgentID: model.AgentClaudeCode, SkillsDir: skillsDir},
	}

	// agent-1 is valid
	dir1 := filepath.Join(skillsDir, "agent-1")
	if err := os.MkdirAll(dir1, 0755); err != nil {
		t.Fatal(err)
	}
	content1 := []byte("# agent-1")
	if err := os.WriteFile(filepath.Join(dir1, "SKILL.md"), content1, 0644); err != nil {
		t.Fatal(err)
	}

	// agent-2 is an anomalous regular file (not a directory payload): the
	// removal must fail midway and agent-1 must be restored.
	fileAgent := filepath.Join(skillsDir, "agent-2")
	if err := os.WriteFile(fileAgent, []byte("not a payload"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := &agentbuilder.Registry{
		Version: 1,
		Agents: []agentbuilder.RegistryEntry{
			{Name: "agent-1", Title: "Agent 1", CreatedAt: time.Now()},
			{Name: "agent-2", Title: "Agent 2", CreatedAt: time.Now()},
		},
	}
	if err := agentbuilder.SaveRegistry(regPath, reg); err != nil {
		t.Fatal(err)
	}

	err := agentbuilder.Uninstall(regPath, []string{"agent-1", "agent-2"}, adapters)
	if err == nil {
		t.Fatal("expected error on midway failure, got nil")
	}

	// Verify agent-1 restored
	restored, err := os.ReadFile(filepath.Join(dir1, "SKILL.md"))
	if err != nil {
		t.Fatalf("expected agent-1 SKILL.md restored, got: %v", err)
	}
	if string(restored) != string(content1) {
		t.Fatalf("agent-1 content mismatch: got %q, want %q", string(restored), string(content1))
	}

	// Verify registry was not mutated
	savedReg, err := agentbuilder.LoadRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(savedReg.Agents) != 2 {
		t.Fatalf("expected 2 agents in registry after rollback, got %d", len(savedReg.Agents))
	}
}
