//go:build windows

package shellinstaller

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func userWindowsLauncherFixture(t *testing.T) (destination, apps string) {
	t.Helper()
	t.Setenv("PATH", "") // Foreign PATH commands must never leak into the outcomes.
	home := t.TempDir()
	destination = filepath.Join(home, "shell")
	if err := os.MkdirAll(filepath.Join(destination, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(destination, "bin", "gentle-shell.cmd")
	if err := os.WriteFile(target, userWindowsBinding(destination, "gentle-shell"), 0700); err != nil {
		t.Fatal(err)
	}
	local := t.TempDir()
	apps = filepath.Join(local, "Microsoft", "WindowsApps")
	if err := os.MkdirAll(apps, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCALAPPDATA", local)
	return destination, apps
}

func TestPublishUserShellLauncherWindowsPublishesBothCommands(t *testing.T) {
	destination, apps := userWindowsLauncherFixture(t)
	report, err := userWindowsPublishLauncher(destination)
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	if report.LauncherDir != apps || len(report.Outcomes) != 2 {
		t.Fatalf("unexpected report: %+v", report)
	}
	for _, outcome := range report.Outcomes {
		if outcome.State != userWindowsLauncherPublished {
			t.Fatalf("command %q state = %q, want published", outcome.Name, outcome.State)
		}
	}
	shim := userWindowsLauncherShim(filepath.Join(destination, "bin", "gentle-shell.cmd"))
	for _, name := range userWindowsLauncherNames {
		data, err := os.ReadFile(filepath.Join(apps, name+".cmd"))
		if err != nil || !bytes.Equal(data, shim) {
			t.Fatalf("command %s was not published with the owned shim: %v", name, err)
		}
	}
	note := userWindowsLauncherNote(report)
	if !strings.Contains(note, "Commands: ") || !strings.Contains(note, filepath.Join(apps, "gentle-shell.cmd")) || !strings.Contains(note, filepath.Join(apps, "gs.cmd")) {
		t.Fatalf("note does not report both published commands: %q", note)
	}
}

func TestPublishUserShellLauncherWindowsIsIdempotent(t *testing.T) {
	destination, _ := userWindowsLauncherFixture(t)
	if _, err := userWindowsPublishLauncher(destination); err != nil {
		t.Fatalf("first publish failed: %v", err)
	}
	report, err := userWindowsPublishLauncher(destination)
	if err != nil {
		t.Fatalf("republish failed: %v", err)
	}
	for _, outcome := range report.Outcomes {
		if outcome.State != userWindowsLauncherExisting {
			t.Fatalf("command %q state = %q, want existing", outcome.Name, outcome.State)
		}
	}
}

func TestPublishUserShellLauncherWindowsReportsOccupiedShim(t *testing.T) {
	destination, apps := userWindowsLauncherFixture(t)
	foreign := []byte("@echo off\r\necho ghostscript\r\n")
	if err := os.WriteFile(filepath.Join(apps, "gs.cmd"), foreign, 0700); err != nil {
		t.Fatal(err)
	}
	report, err := userWindowsPublishLauncher(destination)
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	var gs *userWindowsLauncherOutcome
	for i := range report.Outcomes {
		if report.Outcomes[i].Name == "gs" {
			gs = &report.Outcomes[i]
		}
	}
	if gs == nil || gs.State != userWindowsLauncherOccupied || !strings.Contains(gs.OccupiedBy, filepath.Join(apps, "gs.cmd")) {
		t.Fatalf("foreign gs command was not reported occupied: %+v", report.Outcomes)
	}
	if data, err := os.ReadFile(filepath.Join(apps, "gs.cmd")); err != nil || !bytes.Equal(data, foreign) {
		t.Fatalf("foreign gs command was overwritten: %v", err)
	}
	for _, outcome := range report.Outcomes {
		if outcome.Name == "gentle-shell" && outcome.State != userWindowsLauncherPublished {
			t.Fatalf("gentle-shell state = %q, want published", outcome.State)
		}
	}
	if note := userWindowsLauncherNote(report); !strings.Contains(note, `Command "gs" is occupied by`) {
		t.Fatalf("note does not report the occupied command: %q", note)
	}
}

func TestPublishUserShellLauncherWindowsReportsForeignPATHCommand(t *testing.T) {
	destination, _ := userWindowsLauncherFixture(t)
	other := filepath.Join(t.TempDir(), "tools")
	if err := os.MkdirAll(other, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "gs.cmd"), []byte("@echo off\r\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", other)
	report, err := userWindowsPublishLauncher(destination)
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	for _, outcome := range report.Outcomes {
		if outcome.Name != "gs" {
			continue
		}
		if outcome.State != userWindowsLauncherOccupied || outcome.OccupiedBy != filepath.Join(other, "gs.cmd") {
			t.Fatalf("gs outcome = %+v, want occupied by %s", outcome, filepath.Join(other, "gs.cmd"))
		}
	}
	if _, err := os.Lstat(filepath.Join(report.LauncherDir, "gs.cmd")); !os.IsNotExist(err) {
		t.Fatalf("gs.cmd was published over a foreign PATH command: %v", err)
	}
}

func TestRemoveUserShellLauncherWindowsRemovesOnlyOwnedShims(t *testing.T) {
	destination, apps := userWindowsLauncherFixture(t)
	if _, err := userWindowsPublishLauncher(destination); err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	foreign := []byte("@echo off\r\ncall \"C:\\elsewhere\\bin\\gentle-shell.cmd\" %*\r\n")
	if err := os.WriteFile(filepath.Join(apps, "gs.cmd"), foreign, 0700); err != nil {
		t.Fatal(err)
	}
	if err := userWindowsRemoveLauncher(destination); err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(apps, "gentle-shell.cmd")); !os.IsNotExist(err) {
		t.Fatalf("owned gentle-shell command survived removal: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(apps, "gs.cmd")); err != nil || !bytes.Equal(data, foreign) {
		t.Fatalf("foreign gs command was removed: %v", err)
	}
	if err := userWindowsRemoveLauncher(destination); err != nil {
		t.Fatalf("remove is not idempotent: %v", err)
	}
}

func TestPublishUserShellLauncherWindowsRequiresInstalledLauncher(t *testing.T) {
	destination, _ := userWindowsLauncherFixture(t)
	if err := os.Remove(filepath.Join(destination, "bin", "gentle-shell.cmd")); err != nil {
		t.Fatal(err)
	}
	if _, err := userWindowsPublishLauncher(destination); err == nil {
		t.Fatal("publish succeeded without the installed bin/gentle-shell.cmd binding")
	}
}
