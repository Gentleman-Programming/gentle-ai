package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/gga"
)

func TestInstallTestHomeIsolatesGGA(t *testing.T) {
	inherited := t.TempDir()
	t.Setenv("APPDATA", inherited)
	t.Run("isolated home", func(t *testing.T) {
		home := installTestHome(t)
		want := filepath.Join(home, "AppData", "Roaming")
		if got := os.Getenv("APPDATA"); got != want {
			t.Errorf("APPDATA = %q, want %q", got, want)
		}
		for _, path := range []string{gga.ConfigPath(home), gga.AgentsTemplatePath(home)} {
			rel, err := filepath.Rel(home, path)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				t.Errorf("GGA path %q escapes isolated home %q", path, home)
			}
		}
	})
	if got := os.Getenv("APPDATA"); got != inherited {
		t.Errorf("APPDATA after cleanup = %q, want %q", got, inherited)
	}
}
