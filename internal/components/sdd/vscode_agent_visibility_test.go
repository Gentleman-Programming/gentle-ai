package sdd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHideAndRestoreManagedClaudeInternalAgentsForVSCode(t *testing.T) {
	homeDir := t.TempDir()

	agentsDir := filepath.Join(homeDir, ".claude", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Create a dummy Claude agent file that should be hidden
	agentFile := filepath.Join(agentsDir, "sdd-init.md")
	originalContent := "---\nname: sdd-init\nuser-invocable: true\n---\nbody content"
	if err := os.WriteFile(agentFile, []byte(originalContent), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Run hide
	res, err := hideManagedClaudeInternalAgentsForVSCode(homeDir)
	if err != nil {
		t.Fatalf("hideManagedClaudeInternalAgentsForVSCode: %v", err)
	}
	if !res.Changed {
		t.Error("expected hide to report changed = true")
	}

	// Verify hidden agent content
	content, err := os.ReadFile(agentFile)
	if err != nil {
		t.Fatalf("ReadFile agent: %v", err)
	}
	if !strings.Contains(string(content), "user-invocable: false") {
		t.Errorf("expected user-invocable: false in content, got:\n%s", string(content))
	}

	// Verify backup file exists
	backupFile := agentFile + ".backup"
	backupContent, err := os.ReadFile(backupFile)
	if err != nil {
		t.Fatalf("ReadFile backup: %v", err)
	}
	if string(backupContent) != originalContent {
		t.Errorf("backupContent = %q, want original %q", string(backupContent), originalContent)
	}

	// Run restore
	if err := RestoreManagedClaudeInternalAgentsForVSCode(homeDir); err != nil {
		t.Fatalf("RestoreManagedClaudeInternalAgentsForVSCode: %v", err)
	}

	// Verify restored agent content is back to original
	restoredContent, err := os.ReadFile(agentFile)
	if err != nil {
		t.Fatalf("ReadFile agent after restore: %v", err)
	}
	if string(restoredContent) != originalContent {
		t.Errorf("restoredContent = %q, want original %q", string(restoredContent), originalContent)
	}

	// Verify backup file is deleted
	if _, err := os.Stat(backupFile); err == nil {
		t.Error("expected backup file to be deleted after restore")
	}
}
