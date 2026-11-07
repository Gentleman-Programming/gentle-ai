package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/backup"
)

// redirectedAppDataCLIHome isolates the restore home and redirects %APPDATA%
// and %LOCALAPPDATA% outside it, as Windows folder redirection does (#6024).
func redirectedAppDataCLIHome(t *testing.T, goos string) (home, appData string) {
	t.Helper()
	home = codexCLIHome(t)
	originalHome := backup.UserHomeDirFn
	backup.UserHomeDirFn = func() (string, error) { return home, nil }
	originalGOOS := backup.ConfigRootsGOOS
	backup.ConfigRootsGOOS = goos
	t.Cleanup(func() {
		backup.UserHomeDirFn = originalHome
		backup.ConfigRootsGOOS = originalGOOS
	})
	redirected := t.TempDir()
	appData = filepath.Join(redirected, "Roaming")
	t.Setenv("APPDATA", appData)
	t.Setenv("LOCALAPPDATA", filepath.Join(redirected, "Local"))
	return home, appData
}

func writeCLIRestoreFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readCLIRestoreFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestRestoreManagedBackupRestoresRedirectedAppData: `gentle-ai restore` of a
// manifest whose gga entries live under a redirected %APPDATA% restores them.
func TestRestoreManagedBackupRestoresRedirectedAppData(t *testing.T) {
	home, appData := redirectedAppDataCLIHome(t, "windows")
	homeFile := filepath.Join(home, ".claude", "CLAUDE.md")
	ggaAgents := filepath.Join(appData, "gga", "AGENTS.md")
	writeCLIRestoreFile(t, homeFile, "home original")
	writeCLIRestoreFile(t, ggaAgents, "gga original")
	manifest, err := backup.NewSnapshotter().Create(filepath.Join(t.TempDir(), "snapshot"), []string{homeFile, ggaAgents})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	writeCLIRestoreFile(t, homeFile, "home changed")
	writeCLIRestoreFile(t, ggaAgents, "gga changed")

	if err := RestoreManagedBackup(manifest); err != nil {
		t.Fatalf("RestoreManagedBackup() error = %v", err)
	}
	roots, err := StandaloneRestoreRoots(manifest)
	if err != nil {
		t.Fatalf("StandaloneRestoreRoots() error = %v", err)
	}
	if want := []string{home, appData}; !slices.Equal(roots, want) {
		t.Fatalf("StandaloneRestoreRoots() = %v, want %v", roots, want)
	}
	if got := readCLIRestoreFile(t, homeFile); got != "home original" {
		t.Fatalf("home file = %q, want %q", got, "home original")
	}
	if got := readCLIRestoreFile(t, ggaAgents); got != "gga original" {
		t.Fatalf("gga AGENTS.md = %q, want %q", got, "gga original")
	}
}

// TestRestoreManagedBackupRefusesEntryOutsideEveryRoot: a manifest entry beside
// the redirected %APPDATA% is still refused, and nothing is written.
func TestRestoreManagedBackupRefusesEntryOutsideEveryRoot(t *testing.T) {
	_, appData := redirectedAppDataCLIHome(t, "windows")
	sibling := filepath.Join(filepath.Dir(appData), "Other", "file.txt")
	ggaAgents := filepath.Join(appData, "gga", "AGENTS.md")
	writeCLIRestoreFile(t, sibling, "sibling original")
	writeCLIRestoreFile(t, ggaAgents, "gga original")
	manifest, err := backup.NewSnapshotter().Create(filepath.Join(t.TempDir(), "snapshot"), []string{sibling, ggaAgents})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	writeCLIRestoreFile(t, sibling, "sibling changed")
	writeCLIRestoreFile(t, ggaAgents, "gga changed")

	err = RestoreManagedBackup(manifest)
	if err == nil || !strings.Contains(err.Error(), "invalid OriginalPath") || !strings.Contains(err.Error(), "Other") {
		t.Fatalf("RestoreManagedBackup() error = %v, want an invalid OriginalPath refusal for the sibling", err)
	}
	if got := readCLIRestoreFile(t, sibling); got != "sibling changed" {
		t.Fatalf("refused sibling = %q, want it untouched", got)
	}
	if got := readCLIRestoreFile(t, ggaAgents); got != "gga changed" {
		t.Fatalf("gga AGENTS.md = %q, want it untouched by the refused restore", got)
	}
}

// TestStandaloneRestoreRootsNonWindowsMatchesBase: off Windows a stray
// %APPDATA% outside the home never extends the standalone roots.
func TestStandaloneRestoreRootsNonWindowsMatchesBase(t *testing.T) {
	home, appData := redirectedAppDataCLIHome(t, "linux")
	manifest := backup.Manifest{Entries: []backup.ManifestEntry{
		{OriginalPath: filepath.Join(home, ".config", "gga", "AGENTS.md")},
		{OriginalPath: filepath.Join(appData, "gga", "AGENTS.md")},
	}}
	roots, err := StandaloneRestoreRoots(manifest)
	if err != nil {
		t.Fatalf("StandaloneRestoreRoots() error = %v", err)
	}
	if want := []string{home}; !slices.Equal(roots, want) {
		t.Fatalf("StandaloneRestoreRoots() = %v, want %v", roots, want)
	}
}

// TestRestoreManagedBackupRefusesConfigSymlinkedOutsideHome: off Windows a
// ~/.config symlinked outside the home stays refused, as before #6024, and
// nothing is written through the link.
func TestRestoreManagedBackupRefusesConfigSymlinkedOutsideHome(t *testing.T) {
	_, _ = redirectedAppDataCLIHome(t, "darwin")
	base := t.TempDir()
	home := filepath.Join(base, "home")
	outside := filepath.Join(base, "outside", "config")
	for _, dir := range []string{home, outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(home, ".config")); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	backup.UserHomeDirFn = func() (string, error) { return home, nil }
	t.Chdir(home)
	target := filepath.Join(home, ".config", "opencode", "opencode.json")
	writeCLIRestoreFile(t, target, "original")
	manifest, err := backup.NewSnapshotter().Create(filepath.Join(t.TempDir(), "snapshot"), []string{target})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	writeCLIRestoreFile(t, target, "changed")

	roots, err := StandaloneRestoreRoots(manifest)
	if err != nil {
		t.Fatalf("StandaloneRestoreRoots() error = %v", err)
	}
	if want := []string{home}; !slices.Equal(roots, want) {
		t.Fatalf("StandaloneRestoreRoots() = %v, want %v", roots, want)
	}
	err = RestoreManagedBackup(manifest)
	if err == nil || !strings.Contains(err.Error(), "invalid OriginalPath") {
		t.Fatalf("RestoreManagedBackup() error = %v, want an invalid OriginalPath refusal", err)
	}
	if got := readCLIRestoreFile(t, filepath.Join(outside, "opencode", "opencode.json")); got != "changed" {
		t.Fatalf("file behind the link = %q, want it untouched", got)
	}
}
