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
	if want := []string{homeDir, appData}; !slices.Equal(roots, want) {
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

// TestRestoreRootsForPlanRefusesEntryOutsideEveryRoot: admitting %APPDATA%
// does not widen the guard to its parent or to any other path.
func TestRestoreRootsForPlanRefusesEntryOutsideEveryRoot(t *testing.T) {
	homeDir, appData := redirectedAppDataFixture(t, "windows")
	ggaAgents := filepath.Join(appData, "gga", "AGENTS.md")
	// A sibling of %APPDATA%: same redirected parent, outside every root.
	sibling := filepath.Join(filepath.Dir(appData), "Other", "file.txt")
	writeRestoreRootsFile(t, ggaAgents, "gga original")
	writeRestoreRootsFile(t, sibling, "sibling original")
	targets := []string{sibling, ggaAgents}

	manifest, err := backup.NewSnapshotter().Create(filepath.Join(t.TempDir(), "snapshot"), targets)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	writeRestoreRootsFile(t, ggaAgents, "gga uninstalled")
	writeRestoreRootsFile(t, sibling, "sibling changed")

	roots := restoreRootsForPlan(homeDir, "", targets)
	if want := []string{homeDir, appData}; !slices.Equal(roots, want) {
		t.Fatalf("restoreRootsForPlan() = %v, want %v", roots, want)
	}
	err = (backup.RestoreService{Roots: roots}).Restore(manifest)
	if err == nil || !strings.Contains(err.Error(), "invalid OriginalPath") || !strings.Contains(err.Error(), "Other") {
		t.Fatalf("Restore() error = %v, want an invalid OriginalPath refusal for the sibling", err)
	}
	if got := readRestoreRootsFile(t, sibling); got != "sibling changed" {
		t.Fatalf("refused sibling = %q, want it untouched", got)
	}
	if got := readRestoreRootsFile(t, ggaAgents); got != "gga uninstalled" {
		t.Fatalf("gga AGENTS.md = %q, want it untouched by the refused restore", got)
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
