package backup

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/kiro"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/trae"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/vscode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/windsurf"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/gga"
)

// TestProductConfigDirsMatchAdapterResolvers pins that the restore candidates
// are exactly the directories the gga and adapter resolvers write to on
// Windows, for the real home and any other home, whatever %APPDATA% holds. The
// per-GOOS resolver results are pinned by each adapter's TestUserDirPerGOOS.
func TestProductConfigDirsMatchAdapterResolvers(t *testing.T) {
	useConfigRootsGOOS(t, "windows")
	base := t.TempDir()
	realHome := filepath.Join(base, "real")
	otherHome := filepath.Join(base, "other")
	useRealUserHome(t, realHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, "xdg"))
	for _, appData := range []string{filepath.Join(base, "redirected", "Roaming"), "", filepath.Join("relative", "Roaming")} {
		for _, home := range []string{realHome, otherHome} {
			t.Run(home+" APPDATA="+appData, func(t *testing.T) {
				t.Setenv("APPDATA", appData)
				want := []string{
					gga.ConfigDir(home, "windows"),
					kiro.UserDir(home, "windows"),
					windsurf.UserDir(home, "windows"),
					vscode.UserDir(home, "windows"),
					trae.UserDir(home, "windows"),
				}
				if got := productConfigDirs(home); !slices.Equal(got, want) {
					t.Fatalf("productConfigDirs() = %v, want %v", got, want)
				}
			})
		}
	}
	t.Run("off Windows", func(t *testing.T) {
		for _, goos := range []string{"darwin", "linux", ""} {
			useConfigRootsGOOS(t, goos)
			if got := productConfigDirs(realHome); got != nil {
				t.Fatalf("productConfigDirs(%q) = %v, want none", goos, got)
			}
		}
	})
}
