package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirDefaultsToDotGentleAIUnderHome(t *testing.T) {
	t.Setenv(HomeEnv, "")
	home := t.TempDir()

	if got, want := Dir(home), filepath.Join(home, ".gentle-ai"); got != want {
		t.Fatalf("Dir() = %q, want %q", got, want)
	}
	if got, want := Path(home), filepath.Join(home, ".gentle-ai", "state.json"); got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
	if got, want := ManifestPath(home), filepath.Join(home, ".gentle-ai", "managed-assets.manifest.json"); got != want {
		t.Fatalf("ManifestPath() = %q, want %q", got, want)
	}
	if err := ValidateHomeEnv(); err != nil {
		t.Fatalf("ValidateHomeEnv() with no override = %v, want nil", err)
	}
}

func TestHomeEnvRoutesStateAndManifestFiles(t *testing.T) {
	home := t.TempDir()
	override := filepath.Join(t.TempDir(), "gentle-ai")
	t.Setenv(HomeEnv, override)

	if got := Dir(home); got != override {
		t.Fatalf("Dir() = %q, want %q", got, override)
	}
	if err := Write(home, InstallState{InstalledAgents: []string{"pi"}}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := WriteManifestAtomic(home, Manifest{Schema: ManifestSchema}); err != nil {
		t.Fatalf("WriteManifestAtomic() error = %v", err)
	}
	if err := AppendJournal(home, OpIntent, "run", "resource"); err != nil {
		t.Fatalf("AppendJournal() error = %v", err)
	}

	for _, name := range []string{"state.json", "managed-assets.manifest.json", "managed-assets.journal"} {
		if _, err := os.Stat(filepath.Join(override, name)); err != nil {
			t.Fatalf("%s not written under %s: %v", name, HomeEnv, err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".gentle-ai")); !os.IsNotExist(err) {
		t.Fatalf("~/.gentle-ai must not be created when %s is set (stat err = %v)", HomeEnv, err)
	}
}

func TestValidateHomeEnvRejectsRelativePath(t *testing.T) {
	t.Setenv(HomeEnv, "relative/gentle-ai")

	err := ValidateHomeEnv()
	if err == nil {
		t.Fatal("ValidateHomeEnv() = nil, want error for a relative path")
	}
	if !strings.Contains(err.Error(), HomeEnv) {
		t.Fatalf("ValidateHomeEnv() error %q does not name %s", err, HomeEnv)
	}
}
