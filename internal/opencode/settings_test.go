package opencode

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManagedSettingsPath(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		wantFile string
	}{
		{name: "existing JSONC is preferred", files: []string{settingsJSONC}, wantFile: settingsJSONC},
		{name: "existing JSON remains supported", files: []string{settingsJSON}, wantFile: settingsJSON},
		{name: "JSONC wins when both exist", files: []string{settingsJSON, settingsJSONC}, wantFile: settingsJSONC},
		{name: "new install keeps JSON compatibility", wantFile: settingsJSON},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configDir := t.TempDir()
			for _, name := range tt.files {
				if err := os.WriteFile(filepath.Join(configDir, name), []byte("{}"), 0o600); err != nil {
					t.Fatalf("write %s: %v", name, err)
				}
			}
			if got := ManagedSettingsPath(configDir); got != filepath.Join(configDir, tt.wantFile) {
				t.Fatalf("ManagedSettingsPath() = %q, want %q", got, filepath.Join(configDir, tt.wantFile))
			}
		})
	}
}
