package uninstall

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/backup"
)

// redirectedAppDataFixture models a Windows profile whose %APPDATA% and
// %LOCALAPPDATA% are redirected outside the user home (issue #6024).
func redirectedAppDataFixture(t *testing.T, goos string) (homeDir, appData string) {
	t.Helper()
	original := backup.ConfigRootsGOOS
	backup.ConfigRootsGOOS = goos
	t.Cleanup(func() { backup.ConfigRootsGOOS = original })
	homeDir = t.TempDir()
	redirected := t.TempDir()
	appData = filepath.Join(redirected, "Roaming")
	t.Setenv("APPDATA", appData)
	t.Setenv("LOCALAPPDATA", filepath.Join(redirected, "Local"))
	return homeDir, appData
}

func writeRestoreRootsFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readRestoreRootsFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestRestoreRootsForPlanRestoresRedirectedAppData: the gga targets live under
// a redirected %APPDATA%, and the uninstall's own roots restore them instead of
// refusing the whole manifest.
func TestRestoreRootsForPlanRestoresRedirectedAppData(t *testing.T) {
	homeDir, appData := redirectedAppDataFixture(t, "windows")
	homeFile := filepath.Join(homeDir, ".claude", "CLAUDE.md")
	ggaAgents := filepath.Join(appData, "gga", "AGENTS.md")
	// Recorded even when absent, like globalBackupTargets does.
	ggaConfig := filepath.Join(appData, "gga", "config")
	writeRestoreRootsFile(t, homeFile, "home original")
	writeRestoreRootsFile(t, ggaAgents, "gga original")
	targets := []string{homeFile, ggaAgents, ggaConfig}

	manifest, err := backup.NewSnapshotter().Create(filepath.Join(t.TempDir(), "snapshot"), targets)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	writeRestoreRootsFile(t, homeFile, "home uninstalled")
	writeRestoreRootsFile(t, ggaAgents, "gga uninstalled")
	writeRestoreRootsFile(t, ggaConfig, "created after the snapshot")

	roots := restoreRootsForPlan(homeDir, "", targets)
	if err := (backup.RestoreService{Roots: roots}).Restore(manifest); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if want := []string{homeDir, filepath.Join(appData, "gga")}; !slices.Equal(roots, want) {
		t.Fatalf("restoreRootsForPlan() = %v, want %v", roots, want)
	}
	if got := readRestoreRootsFile(t, homeFile); got != "home original" {
		t.Fatalf("home file = %q, want %q", got, "home original")
	}
	if got := readRestoreRootsFile(t, ggaAgents); got != "gga original" {
		t.Fatalf("gga AGENTS.md = %q, want %q", got, "gga original")
	}
	if _, err := os.Stat(ggaConfig); !os.IsNotExist(err) {
		t.Fatalf("gga config created after the snapshot still exists: %v", err)
	}
}

// TestRestoreRootsForPlanRefusesEntryOutsideEveryRoot: admitting %APPDATA%\gga
// does not widen the guard to the rest of %APPDATA%, its parent, or the temp
// directory under %LOCALAPPDATA%, the real-Windows runner layout where HOME is
// a temp directory and both variables sit outside it.
func TestRestoreRootsForPlanRefusesEntryOutsideEveryRoot(t *testing.T) {
	for name, refused := range map[string]func(appData, localAppData string) string{
		"sibling of APPDATA":   func(appData, _ string) string { return filepath.Join(filepath.Dir(appData), "Other", "file.txt") },
		"elsewhere in APPDATA": func(appData, _ string) string { return filepath.Join(appData, "Other", "file.txt") },
		"temp dir under LOCALAPPDATA": func(_, localAppData string) string {
			return filepath.Join(localAppData, "Temp", "Other", "outside.json")
		},
	} {
		t.Run(name, func(t *testing.T) {
			homeDir, appData := redirectedAppDataFixture(t, "windows")
			ggaAgents := filepath.Join(appData, "gga", "AGENTS.md")
			outside := refused(appData, os.Getenv("LOCALAPPDATA"))
			writeRestoreRootsFile(t, ggaAgents, "gga original")
			writeRestoreRootsFile(t, outside, "outside original")
			targets := []string{outside, ggaAgents}

			manifest, err := backup.NewSnapshotter().Create(filepath.Join(t.TempDir(), "snapshot"), targets)
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			writeRestoreRootsFile(t, ggaAgents, "gga uninstalled")
			writeRestoreRootsFile(t, outside, "outside changed")

			roots := restoreRootsForPlan(homeDir, "", targets)
			err = (backup.RestoreService{Roots: roots}).Restore(manifest)
			if err == nil || !strings.Contains(err.Error(), "invalid OriginalPath") || !strings.Contains(err.Error(), "Other") {
				t.Fatalf("Restore() error = %v, want an invalid OriginalPath refusal for %q", err, outside)
			}
			if want := []string{homeDir, filepath.Join(appData, "gga")}; !slices.Equal(roots, want) {
				t.Fatalf("restoreRootsForPlan() = %v, want %v", roots, want)
			}
			if got := readRestoreRootsFile(t, outside); got != "outside changed" {
				t.Fatalf("refused entry = %q, want it untouched", got)
			}
			if got := readRestoreRootsFile(t, ggaAgents); got != "gga uninstalled" {
				t.Fatalf("gga AGENTS.md = %q, want it untouched by the refused restore", got)
			}
		})
	}
}

// TestRestoreRootsForPlanTempUnderLocalAppDataKeepsHomeRoot reproduces the
// real-Windows runner: the temp directory lives under %LOCALAPPDATA%, outside
// a temp HOME, and an entry there neither adds a root nor restores.
func TestRestoreRootsForPlanTempUnderLocalAppDataKeepsHomeRoot(t *testing.T) {
	homeDir, _ := redirectedAppDataFixture(t, "windows")
	outside := filepath.Join(os.Getenv("LOCALAPPDATA"), "Temp", "003", "outside.json")
	writeRestoreRootsFile(t, outside, "before")
	targets := []string{outside}
	manifest, err := backup.NewSnapshotter().Create(filepath.Join(t.TempDir(), "snapshot"), targets)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	writeRestoreRootsFile(t, outside, "changed")

	roots := restoreRootsForPlan(homeDir, "", targets)
	if want := []string{homeDir}; !slices.Equal(roots, want) {
		t.Fatalf("restoreRootsForPlan() = %v, want %v", roots, want)
	}
	err = (backup.RestoreService{Roots: roots}).Restore(manifest)
	if err == nil || !strings.Contains(err.Error(), "invalid OriginalPath") {
		t.Fatalf("Restore() error = %v, want an invalid OriginalPath refusal", err)
	}
	if got := readRestoreRootsFile(t, outside); got != "changed" {
		t.Fatalf("refused entry = %q, want it untouched", got)
	}
}

// TestRestoreRootsForPlanNonWindowsMatchesBase: off Windows the config roots
// resolve under the home, so the roots stay the historical home/workspace set.
func TestRestoreRootsForPlanNonWindowsMatchesBase(t *testing.T) {
	homeDir, appData := redirectedAppDataFixture(t, "linux")
	workspaceDir := t.TempDir()
	targets := []string{
		filepath.Join(homeDir, ".config", "gga", "AGENTS.md"),
		filepath.Join(appData, "gga", "AGENTS.md"),
		filepath.Join(workspaceDir, ".mcp.json"),
	}
	if got, want := restoreRootsForPlan(homeDir, "", targets), []string{homeDir}; !slices.Equal(got, want) {
		t.Fatalf("restoreRootsForPlan(no workspace) = %v, want %v", got, want)
	}
	if got, want := restoreRootsForPlan(homeDir, workspaceDir, targets), []string{homeDir, workspaceDir}; !slices.Equal(got, want) {
		t.Fatalf("restoreRootsForPlan(workspace) = %v, want %v", got, want)
	}
}

// TestRestoreRootsForPlanNonWindowsConfigSymlinkedOutside: a ~/.config
// symlinked outside the home adds no root, matching the historical set.
func TestRestoreRootsForPlanNonWindowsConfigSymlinkedOutside(t *testing.T) {
	original := backup.ConfigRootsGOOS
	backup.ConfigRootsGOOS = "linux"
	t.Cleanup(func() { backup.ConfigRootsGOOS = original })
	base := t.TempDir()
	homeDir := filepath.Join(base, "home")
	outside := filepath.Join(base, "outside")
	for _, dir := range []string{homeDir, outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(homeDir, ".config")); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	got := restoreRootsForPlan(homeDir, "", []string{filepath.Join(homeDir, ".config", "opencode", "opencode.json")})
	if want := []string{homeDir}; !slices.Equal(got, want) {
		t.Fatalf("restoreRootsForPlan() = %v, want %v", got, want)
	}
}

// TestRestoreRootsForPlanRefusesAppDataSymlinkedOutsideHome: %APPDATA% spelled
// inside the profile but linked outside it (a junction on Windows) is not a new
// root, so the restore is refused exactly like a home-only restore.
func TestRestoreRootsForPlanRefusesAppDataSymlinkedOutsideHome(t *testing.T) {
	homeDir, _ := redirectedAppDataFixture(t, "windows")
	outside := t.TempDir()
	appData := filepath.Join(homeDir, "AppData", "Roaming")
	if err := os.MkdirAll(filepath.Dir(appData), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, appData); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	t.Setenv("APPDATA", appData)
	ggaAgents := filepath.Join(appData, "gga", "AGENTS.md")
	writeRestoreRootsFile(t, ggaAgents, "gga original")
	targets := []string{ggaAgents}
	manifest, err := backup.NewSnapshotter().Create(filepath.Join(t.TempDir(), "snapshot"), targets)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	writeRestoreRootsFile(t, ggaAgents, "gga uninstalled")

	roots := restoreRootsForPlan(homeDir, "", targets)
	if want := []string{homeDir}; !slices.Equal(roots, want) {
		t.Fatalf("restoreRootsForPlan() = %v, want %v", roots, want)
	}
	err = (backup.RestoreService{Roots: roots}).Restore(manifest)
	if err == nil || !strings.Contains(err.Error(), "invalid OriginalPath") {
		t.Fatalf("Restore() error = %v, want an invalid OriginalPath refusal", err)
	}
	if got := readRestoreRootsFile(t, filepath.Join(outside, "gga", "AGENTS.md")); got != "gga uninstalled" {
		t.Fatalf("file behind the link = %q, want it untouched", got)
	}
}

// adapterUserDirTargets lists, per adapter, its User folder under appData and
// a file the uninstall rewrites there (issue #6029).
func adapterUserDirTargets(appData string) map[string][2]string {
	userDir := func(app string) string { return filepath.Join(appData, app, "User") }
	return map[string][2]string{
		"kiro":     {userDir("kiro"), filepath.Join(userDir("kiro"), "settings.json")},
		"Windsurf": {userDir("Windsurf"), filepath.Join(userDir("Windsurf"), "settings.json")},
		"VS Code":  {userDir("Code"), filepath.Join(userDir("Code"), "prompts", "gentle-ai.instructions.md")},
		"Trae":     {userDir("Trae"), filepath.Join(userDir("Trae"), "mcp.json")},
	}
}

// TestRestoreRootsForPlanRestoresAdapterUserDirs: each adapter's User folder
// under a redirected %APPDATA% restores through the uninstall's own roots, and
// only that exact folder is added.
func TestRestoreRootsForPlanRestoresAdapterUserDirs(t *testing.T) {
	for _, name := range []string{"kiro", "Windsurf", "VS Code", "Trae"} {
		t.Run(name, func(t *testing.T) {
			homeDir, appData := redirectedAppDataFixture(t, "windows")
			t.Setenv("HOME", homeDir)
			t.Setenv("USERPROFILE", homeDir)
			paths := adapterUserDirTargets(appData)[name]
			userDir, target := paths[0], paths[1]
			writeRestoreRootsFile(t, target, "original")
			targets := []string{target}
			manifest, err := backup.NewSnapshotter().Create(filepath.Join(t.TempDir(), "snapshot"), targets)
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			writeRestoreRootsFile(t, target, "uninstalled")

			roots := restoreRootsForPlan(homeDir, "", targets)
			if want := []string{homeDir, userDir}; !slices.Equal(roots, want) {
				t.Fatalf("restoreRootsForPlan() = %v, want %v", roots, want)
			}
			if err := (backup.RestoreService{Roots: roots}).Restore(manifest); err != nil {
				t.Fatalf("Restore() error = %v", err)
			}
			if got := readRestoreRootsFile(t, target); got != "original" {
				t.Fatalf("%s = %q, want %q", target, got, "original")
			}
		})
	}
}

// TestRestoreRootsForPlanRefusesAdapterUserDirSiblings: admitting the User
// folders does not reach their siblings, the rest of %APPDATA%, or the temp
// directory under %LOCALAPPDATA%; the refused restore writes nothing.
func TestRestoreRootsForPlanRefusesAdapterUserDirSiblings(t *testing.T) {
	for name, refused := range map[string]func(appData, localAppData string) string{
		"Code UserX":       func(appData, _ string) string { return filepath.Join(appData, "Code", "UserX", "settings.json") },
		"Code other":       func(appData, _ string) string { return filepath.Join(appData, "Code", "other", "settings.json") },
		"Kiro Other":       func(appData, _ string) string { return filepath.Join(appData, "Kiro", "Other", "settings.json") },
		"Windsurf Cache":   func(appData, _ string) string { return filepath.Join(appData, "Windsurf", "Cache", "data") },
		"Trae folder root": func(appData, _ string) string { return filepath.Join(appData, "Trae", "settings.json") },
		"temp dir under LOCALAPPDATA": func(_, localAppData string) string {
			return filepath.Join(localAppData, "Temp", "Code", "User", "settings.json")
		},
	} {
		t.Run(name, func(t *testing.T) {
			homeDir, appData := redirectedAppDataFixture(t, "windows")
			t.Setenv("HOME", homeDir)
			t.Setenv("USERPROFILE", homeDir)
			legit := adapterUserDirTargets(appData)["VS Code"][1]
			outside := refused(appData, os.Getenv("LOCALAPPDATA"))
			writeRestoreRootsFile(t, legit, "legit original")
			writeRestoreRootsFile(t, outside, "outside original")
			targets := []string{outside, legit}
			manifest, err := backup.NewSnapshotter().Create(filepath.Join(t.TempDir(), "snapshot"), targets)
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			writeRestoreRootsFile(t, legit, "legit uninstalled")
			writeRestoreRootsFile(t, outside, "outside changed")

			roots := restoreRootsForPlan(homeDir, "", targets)
			if want := []string{homeDir, filepath.Join(appData, "Code", "User")}; !slices.Equal(roots, want) {
				t.Fatalf("restoreRootsForPlan() = %v, want %v", roots, want)
			}
			err = (backup.RestoreService{Roots: roots}).Restore(manifest)
			if err == nil || !strings.Contains(err.Error(), "invalid OriginalPath") {
				t.Fatalf("Restore() error = %v, want an invalid OriginalPath refusal for %q", err, outside)
			}
			if got := readRestoreRootsFile(t, outside); got != "outside changed" {
				t.Fatalf("refused entry = %q, want it untouched", got)
			}
			if got := readRestoreRootsFile(t, legit); got != "legit uninstalled" {
				t.Fatalf("VS Code entry = %q, want it untouched by the refused restore", got)
			}
		})
	}
}

// TestRestoreRootsForPlanNonWindowsIgnoresAdapterUserDirs: off Windows a stray
// %APPDATA% holding the User folders never extends the roots.
func TestRestoreRootsForPlanNonWindowsIgnoresAdapterUserDirs(t *testing.T) {
	for _, goos := range []string{"linux", "darwin"} {
		t.Run(goos, func(t *testing.T) {
			homeDir, appData := redirectedAppDataFixture(t, goos)
			t.Setenv("HOME", homeDir)
			t.Setenv("USERPROFILE", homeDir)
			var targets []string
			for _, target := range adapterUserDirTargets(appData) {
				targets = append(targets, target[1])
			}
			if got, want := restoreRootsForPlan(homeDir, "", targets), []string{homeDir}; !slices.Equal(got, want) {
				t.Fatalf("restoreRootsForPlan() = %v, want %v", got, want)
			}
		})
	}
}
