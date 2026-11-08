package cli

import (
	"bytes"
	"fmt"
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
	if want := []string{home, filepath.Join(appData, "gga")}; !slices.Equal(roots, want) {
		t.Fatalf("StandaloneRestoreRoots() = %v, want %v", roots, want)
	}
	if got := readCLIRestoreFile(t, homeFile); got != "home original" {
		t.Fatalf("home file = %q, want %q", got, "home original")
	}
	if got := readCLIRestoreFile(t, ggaAgents); got != "gga original" {
		t.Fatalf("gga AGENTS.md = %q, want %q", got, "gga original")
	}
}

// TestRestoreManagedBackupRefusesEntryOutsideEveryRoot: a manifest entry in the
// redirected %APPDATA% but outside %APPDATA%\gga is still refused, and nothing
// is written.
func TestRestoreManagedBackupRefusesEntryOutsideEveryRoot(t *testing.T) {
	_, appData := redirectedAppDataCLIHome(t, "windows")
	sibling := filepath.Join(appData, "Other", "file.txt")
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

// TestStandaloneRestoreRefusesTempEntryUnderLocalAppData reproduces the
// real-Windows runner: the temp directory lives under %LOCALAPPDATA%, outside a
// temp HOME, and `gentle-ai restore` refuses an entry there with the home-only
// message, exactly like TestStandaloneRestoreRefusesEntryOutsideWorkspaceAndHome.
func TestStandaloneRestoreRefusesTempEntryUnderLocalAppData(t *testing.T) {
	home := standaloneRestoreHome(t)
	original := backup.ConfigRootsGOOS
	backup.ConfigRootsGOOS = "windows"
	t.Cleanup(func() { backup.ConfigRootsGOOS = original })
	profile := canonicalDir(t)
	t.Setenv("APPDATA", filepath.Join(profile, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(profile, "AppData", "Local"))
	t.Chdir(canonicalDir(t))

	path := filepath.Join(profile, "AppData", "Local", "Temp", "003", "outside.json")
	writeCLIRestoreFile(t, path, "before")
	if _, err := backup.NewSnapshotter().Create(filepath.Join(home, ".gentle-ai", "backups", "local-temp"), []string{path}); err != nil {
		t.Fatal(err)
	}
	writeCLIRestoreFile(t, path, "after")

	var output bytes.Buffer
	wantError := fmt.Sprintf("restore failed: manifest entry has invalid OriginalPath %q: must be an absolute path under an allowed root (%s)", path, home)
	if err := RunRestore([]string{"local-temp", "--yes"}, &output); err == nil || err.Error() != wantError {
		t.Fatalf("restore error = %v, want %q", err, wantError)
	}
	if output.Len() != 0 {
		t.Fatalf("rejected restore output = %q, want empty", output.String())
	}
	if got := readCLIRestoreFile(t, path); got != "after" {
		t.Fatalf("refused entry changed = %q", got)
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

// TestRestoreManagedBackupRestoresAdapterUserDirs: `gentle-ai restore` of a
// manifest whose kiro, Windsurf, VS Code and Trae entries live in their User
// folders under a redirected %APPDATA% restores them, adding exactly those
// folders (issue #6029).
func TestRestoreManagedBackupRestoresAdapterUserDirs(t *testing.T) {
	home, appData := redirectedAppDataCLIHome(t, "windows")
	userDir := func(app string) string { return filepath.Join(appData, app, "User") }
	targets := []string{
		filepath.Join(userDir("kiro"), "settings.json"),
		filepath.Join(userDir("Windsurf"), "settings.json"),
		filepath.Join(userDir("Code"), "prompts", "gentle-ai.instructions.md"),
		filepath.Join(userDir("Trae"), "mcp.json"),
	}
	for _, target := range targets {
		writeCLIRestoreFile(t, target, "original")
	}
	manifest, err := backup.NewSnapshotter().Create(filepath.Join(t.TempDir(), "snapshot"), targets)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	for _, target := range targets {
		writeCLIRestoreFile(t, target, "changed")
	}

	if err := RestoreManagedBackup(manifest); err != nil {
		t.Fatalf("RestoreManagedBackup() error = %v", err)
	}
	roots, err := StandaloneRestoreRoots(manifest)
	if err != nil {
		t.Fatalf("StandaloneRestoreRoots() error = %v", err)
	}
	if want := []string{home, userDir("kiro"), userDir("Windsurf"), userDir("Code"), userDir("Trae")}; !slices.Equal(roots, want) {
		t.Fatalf("StandaloneRestoreRoots() = %v, want %v", roots, want)
	}
	for _, target := range targets {
		if got := readCLIRestoreFile(t, target); got != "original" {
			t.Fatalf("%s = %q, want %q", target, got, "original")
		}
	}
}

// TestRestoreManagedBackupRefusesAdapterUserDirSiblings: an entry beside an
// admitted User folder, elsewhere in %APPDATA% or under %LOCALAPPDATA% is
// refused, and nothing is written.
func TestRestoreManagedBackupRefusesAdapterUserDirSiblings(t *testing.T) {
	for name, refused := range map[string]func(appData, localAppData string) string{
		"Code UserX":     func(appData, _ string) string { return filepath.Join(appData, "Code", "UserX", "settings.json") },
		"Code other":     func(appData, _ string) string { return filepath.Join(appData, "Code", "other", "settings.json") },
		"Kiro Other":     func(appData, _ string) string { return filepath.Join(appData, "Kiro", "Other", "settings.json") },
		"Windsurf Cache": func(appData, _ string) string { return filepath.Join(appData, "Windsurf", "Cache", "data") },
		"temp dir under LOCALAPPDATA": func(_, localAppData string) string {
			return filepath.Join(localAppData, "Temp", "Code", "User", "settings.json")
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, appData := redirectedAppDataCLIHome(t, "windows")
			legit := filepath.Join(appData, "Code", "User", "settings.json")
			outside := refused(appData, os.Getenv("LOCALAPPDATA"))
			writeCLIRestoreFile(t, legit, "legit original")
			writeCLIRestoreFile(t, outside, "outside original")
			manifest, err := backup.NewSnapshotter().Create(filepath.Join(t.TempDir(), "snapshot"), []string{outside, legit})
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			writeCLIRestoreFile(t, legit, "legit changed")
			writeCLIRestoreFile(t, outside, "outside changed")

			err = RestoreManagedBackup(manifest)
			if err == nil || !strings.Contains(err.Error(), "invalid OriginalPath") {
				t.Fatalf("RestoreManagedBackup() error = %v, want an invalid OriginalPath refusal for %q", err, outside)
			}
			if got := readCLIRestoreFile(t, outside); got != "outside changed" {
				t.Fatalf("refused entry = %q, want it untouched", got)
			}
			if got := readCLIRestoreFile(t, legit); got != "legit changed" {
				t.Fatalf("VS Code entry = %q, want it untouched by the refused restore", got)
			}
		})
	}
}

// TestStandaloneRestoreRootsNonWindowsIgnoresAdapterUserDirs: off Windows a
// stray %APPDATA% holding the User folders never extends the standalone roots.
func TestStandaloneRestoreRootsNonWindowsIgnoresAdapterUserDirs(t *testing.T) {
	home, appData := redirectedAppDataCLIHome(t, "darwin")
	var entries []backup.ManifestEntry
	for _, app := range []string{"kiro", "Windsurf", "Code", "Trae"} {
		entries = append(entries, backup.ManifestEntry{OriginalPath: filepath.Join(appData, app, "User", "settings.json")})
	}
	roots, err := StandaloneRestoreRoots(backup.Manifest{Entries: entries})
	if err != nil {
		t.Fatalf("StandaloneRestoreRoots() error = %v", err)
	}
	if want := []string{home}; !slices.Equal(roots, want) {
		t.Fatalf("StandaloneRestoreRoots() = %v, want %v", roots, want)
	}
}
