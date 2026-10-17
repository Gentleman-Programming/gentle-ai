package agentbuilder_test

import (
	"os"
	"path/filepath"
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

func TestUninstall_RejectDirectoryPayload(t *testing.T) {
	tempDir := t.TempDir()
	regPath := filepath.Join(tempDir, "custom-agents.json")
	skillsDir := filepath.Join(tempDir, "skills")

	badDir := filepath.Join(skillsDir, "bad-agent", "SKILL.md")
	if err := os.MkdirAll(badDir, 0755); err != nil {
		t.Fatal(err)
	}

	reg := &agentbuilder.Registry{
		Version: 1,
		Agents:  []agentbuilder.RegistryEntry{{Name: "bad-agent"}},
	}
	_ = agentbuilder.SaveRegistry(regPath, reg)

	adapters := []agentbuilder.AdapterInfo{
		{AgentID: model.AgentClaudeCode, SkillsDir: skillsDir},
	}

	err := agentbuilder.Uninstall(regPath, []string{"bad-agent"}, adapters)
	if err == nil {
		t.Fatal("expected error rejecting directory payload, got nil")
	}
}

func TestUninstall_RollbackOnSaveFailure(t *testing.T) {
	tempDir := t.TempDir()
	skillsDir := filepath.Join(tempDir, "skills")
	agentDir := filepath.Join(skillsDir, "my-agent")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatal(err)
	}
	skillPath := filepath.Join(agentDir, "SKILL.md")
	content := []byte("original content")
	if err := os.WriteFile(skillPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	// Point registry to a directory path so SaveRegistry fails trying to write to it.
	regDirPath := filepath.Join(tempDir, "reg-dir")
	if err := os.MkdirAll(regDirPath, 0755); err != nil {
		t.Fatal(err)
	}

	adapters := []agentbuilder.AdapterInfo{
		{AgentID: model.AgentClaudeCode, SkillsDir: skillsDir},
	}

	err := agentbuilder.Uninstall(regDirPath, []string{"my-agent"}, adapters)
	if err == nil {
		t.Fatal("expected error on save failure, got nil")
	}

	// Verify rollback restored the file
	restored, readErr := os.ReadFile(skillPath)
	if readErr != nil {
		t.Fatalf("file not restored: %v", readErr)
	}
	if string(restored) != string(content) {
		t.Errorf("restored content = %q, want %q", string(restored), string(content))
	}
}
