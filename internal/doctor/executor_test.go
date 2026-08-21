package doctor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPreflightRemedySyncIsReadOnlyAndFailClosed(t *testing.T) {
	homeDir := t.TempDir()
	stateDir := filepath.Join(homeDir, ".gentle-ai")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(stateDir, "state.json")
	if err := os.WriteFile(statePath, []byte(`{"installed_agents":["claude-code"]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if !PreflightRemedySync(context.Background(), homeDir) {
		t.Fatal("supported missing config did not pass preflight")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if PreflightRemedySync(ctx, homeDir) {
		t.Fatal("canceled preflight passed")
	}
	if _, err := os.Stat(filepath.Join(homeDir, ".claude")); !os.IsNotExist(err) {
		t.Fatalf("preflight created or changed the config directory: %v", err)
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("preflight changed state ownership: %v", err)
	}
}
