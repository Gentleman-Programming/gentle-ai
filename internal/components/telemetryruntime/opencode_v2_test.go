package telemetryruntime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadOpenCodeNativeAssignment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	dir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "opencode.json"), []byte(`{"agent":{"worker":{"model":"old/model"}},"agents":{"worker":{"model":"native/coder#high"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	got := readOpenCodeAssignment(home, "worker")
	if got.ProviderID != "native" || got.ModelID != "coder" || got.Effort != "high" {
		t.Fatalf("assignment = %+v", got)
	}
}
