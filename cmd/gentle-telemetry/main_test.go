package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSummaryToken(t *testing.T) {
	dir := t.TempDir()
	explicit := filepath.Join(dir, "explicit")
	os.WriteFile(filepath.Join(dir, "summary-token"), []byte(" from-credential \n"), 0o600)
	os.WriteFile(explicit, []byte("from-flag"), 0o600)

	for _, tt := range []struct{ name, path, credDir, want string }{
		{"falls back to CREDENTIALS_DIRECTORY", "", dir, "from-credential"},
		{"explicit path wins", explicit, dir, "from-flag"},
		{"neither configured returns empty", "", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CREDENTIALS_DIRECTORY", tt.credDir)
			if got, err := loadSummaryToken(tt.path); err != nil || got != tt.want {
				t.Errorf("loadSummaryToken(%q) = (%q, %v), want (%q, nil)", tt.path, got, err, tt.want)
			}
		})
	}
}
