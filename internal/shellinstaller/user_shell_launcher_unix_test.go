//go:build linux || darwin

package shellinstaller

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func launcherTestTarget(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	emptyBin := filepath.Join(home, "emptybin")
	if err := os.MkdirAll(emptyBin, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", emptyBin)
	// Pin every input of the startup file choice and both forwarded intents.
	for _, key := range []string{"SHELL", "ZDOTDIR", "XDG_CONFIG_HOME", "GENTLE_AI_LAUNCHER_DIR_ON_PATH", "GENTLE_AI_LAUNCHER_PROFILE"} {
		t.Setenv(key, "")
	}
	target := filepath.Join(home, "install")
	bin := filepath.Join(target, "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "gentle-shell"), []byte("#!/bin/sh\n"), 0700); err != nil {
		t.Fatal(err)
	}
	return target
}

func launcherLink(t *testing.T, name string) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(home, ".local", "bin", name)
}

func TestPublishUserShellLauncherPublishesBothCommands(t *testing.T) {
	target := launcherTestTarget(t)
	report, err := PublishUserShellLauncher(target)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(target, "bin", "gentle-shell")
	for index, name := range userShellLauncherNames {
		outcome := report.Outcomes[index]
		if outcome.Name != name || outcome.State != userShellLauncherPublished || outcome.OccupiedBy != "" {
			t.Fatalf("outcome %d differs: %+v", index, outcome)
		}
		if got, err := os.Readlink(launcherLink(t, name)); err != nil || got != bin {
			t.Fatalf("%s launcher differs: %q %v", name, got, err)
		}
	}
	if report.DirOnPath {
		t.Fatal("launcher dir unexpectedly on PATH")
	}
	note := userShellLauncherNote(report)
	for _, want := range []string{launcherLink(t, "gentle-shell"), launcherLink(t, "gs"), "is not on this terminal's PATH", "export PATH="} {
		if !strings.Contains(note, want) {
			t.Fatalf("note misses %q: %s", want, note)
		}
	}
	// A republication reports the same launchers as already owned.
	repeat, err := PublishUserShellLauncher(target)
	if err != nil {
		t.Fatal(err)
	}
	for _, outcome := range repeat.Outcomes {
		if outcome.State != userShellLauncherExisting {
			t.Fatalf("republished %s: %+v", outcome.Name, outcome)
		}
	}
}

func TestPublishUserShellLauncherReportsOccupiedNames(t *testing.T) {
	target := launcherTestTarget(t)
	// A foreign regular file occupies gs at the launcher path.
	if err := os.MkdirAll(filepath.Dir(launcherLink(t, "gs")), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(launcherLink(t, "gs"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	report, err := PublishUserShellLauncher(target)
	if err != nil {
		t.Fatal(err)
	}
	var gentle, gs userShellLauncherOutcome
	for _, outcome := range report.Outcomes {
		switch outcome.Name {
		case "gentle-shell":
			gentle = outcome
		case "gs":
			gs = outcome
		}
	}
	if gentle.State != userShellLauncherPublished {
		t.Fatalf("gentle-shell differs: %+v", gentle)
	}
	if gs.State != userShellLauncherOccupied || !strings.Contains(gs.OccupiedBy, "existing file") {
		t.Fatalf("gs differs: %+v", gs)
	}
	if _, err := os.Lstat(launcherLink(t, "gs")); err != nil {
		t.Fatalf("foreign gs launcher was touched: %v", err)
	}
	note := userShellLauncherNote(report)
	if !strings.Contains(note, `"gs" is occupied by`) {
		t.Fatalf("note misses the gs conflict: %s", note)
	}

	// A foreign symlink pointing elsewhere occupies gentle-shell the same way.
	other := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.WriteFile(other, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(launcherLink(t, "gentle-shell")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, launcherLink(t, "gentle-shell")); err != nil {
		t.Fatal(err)
	}
	report, err = PublishUserShellLauncher(target)
	if err != nil {
		t.Fatal(err)
	}
	for _, outcome := range report.Outcomes {
		if outcome.Name != "gentle-shell" {
			continue
		}
		if outcome.State != userShellLauncherOccupied || !strings.Contains(outcome.OccupiedBy, "points to "+other) {
			t.Fatalf("gentle-shell differs: %+v", outcome)
		}
	}
}

func TestPublishUserShellLauncherReportsPATHCommandConflicts(t *testing.T) {
	target := launcherTestTarget(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	// Ghostscript-like: gs resolves on PATH to a foreign binary.
	tools := filepath.Join(home, "tools")
	if err := os.MkdirAll(tools, 0755); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(tools, "gs")
	if err := os.WriteFile(foreign, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tools)
	report, err := PublishUserShellLauncher(target)
	if err != nil {
		t.Fatal(err)
	}
	for _, outcome := range report.Outcomes {
		if outcome.Name != "gs" {
			continue
		}
		if outcome.State != userShellLauncherOccupied || outcome.OccupiedBy != foreign {
			t.Fatalf("gs differs: %+v", outcome)
		}
		if _, err := os.Lstat(launcherLink(t, "gs")); !os.IsNotExist(err) {
			t.Fatalf("gs launcher was created over the PATH command: %v", err)
		}
	}
}

func TestPublishUserShellLauncherOmitsHintWhenDirOnPath(t *testing.T) {
	target := launcherTestTarget(t)
	t.Setenv("PATH", filepath.Join(t.TempDir(), "nothing")+string(os.PathListSeparator)+filepath.Dir(launcherLink(t, "gentle-shell")))
	report, err := PublishUserShellLauncher(target)
	if err != nil {
		t.Fatal(err)
	}
	if !report.DirOnPath {
		t.Fatal("launcher dir not detected on PATH")
	}
	if strings.Contains(userShellLauncherNote(report), "is not on this terminal's PATH") {
		t.Fatal("remediation hint emitted while the launcher dir is on PATH")
	}
}

// The Linux delegated unit runs with PATH=/usr/bin:/bin, so the launcher dir
// decision must be the caller terminal's, forwarded as an intent (#5758).
func TestPublishUserShellLauncherUsesForwardedTerminalPathDecision(t *testing.T) {
	target := launcherTestTarget(t)
	launcherDir := filepath.Dir(launcherLink(t, "gentle-shell"))
	for _, test := range []struct {
		name       string
		terminal   string
		wantOnPath bool
	}{
		{"terminal PATH holds the launcher dir", "/usr/local/bin" + string(os.PathListSeparator) + launcherDir, true},
		{"terminal PATH lacks the launcher dir", "/usr/local/bin", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("GENTLE_AI_LAUNCHER_DIR_ON_PATH", "")
			t.Setenv("GENTLE_AI_LAUNCHER_PROFILE", "")
			t.Setenv("PATH", test.terminal)
			forwarded := userServiceIntentEnvironment(os.Getenv("HOME"))
			// Re-enter as the unit: its own PATH stays the fixed system one and
			// only the forwarded entries cross the env -i boundary.
			t.Setenv("PATH", "/usr/bin:/bin")
			for _, entry := range forwarded {
				key, value, _ := strings.Cut(entry, "=")
				t.Setenv(key, value)
			}
			for _, entry := range forwarded {
				if strings.HasPrefix(entry, "PATH=") || strings.Contains(entry, test.terminal) {
					t.Fatalf("caller PATH leaked into the unit environment: %q", forwarded)
				}
			}
			report, err := PublishUserShellLauncher(target)
			if err != nil {
				t.Fatal(err)
			}
			if report.DirOnPath != test.wantOnPath {
				t.Fatalf("DirOnPath = %v, want %v (forwarded %q)", report.DirOnPath, test.wantOnPath, forwarded)
			}
			if hint := strings.Contains(userShellLauncherNote(report), "is not on this terminal's PATH"); hint == test.wantOnPath {
				t.Fatalf("hint shown = %v with the launcher dir on the terminal PATH = %v", hint, test.wantOnPath)
			}
		})
	}
}

// Only the exact forwarded value counts; anything else falls back to PATH.
func TestUserLauncherDirOnPathIntentIsExact(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".local", "bin")
	t.Setenv("PATH", "/usr/bin:/bin")
	for value, want := range map[string]bool{"1": true, "": false, "0": false, "yes": false, "true": false} {
		t.Setenv("GENTLE_AI_LAUNCHER_DIR_ON_PATH", value)
		if got := userLauncherDirOnPath(dir); got != want {
			t.Fatalf("intent %q: DirOnPath = %v, want %v", value, got, want)
		}
	}
}

func TestRemoveUserShellLauncherRemovesOnlyOwnedLinks(t *testing.T) {
	target := launcherTestTarget(t)
	// Owned links are removed together; absent ones are skipped.
	if _, err := PublishUserShellLauncher(target); err != nil {
		t.Fatal(err)
	}
	if err := RemoveUserShellLauncher(target); err != nil {
		t.Fatal(err)
	}
	for _, name := range userShellLauncherNames {
		if _, err := os.Lstat(launcherLink(t, name)); !os.IsNotExist(err) {
			t.Fatalf("%s launcher was not removed: %v", name, err)
		}
	}

	// A foreign gentle-shell symlink pointing elsewhere is skipped silently;
	// only a non-symlink gentle-shell launcher aborts the removal.
	other := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.Symlink(other, launcherLink(t, "gentle-shell")); err != nil {
		t.Fatal(err)
	}
	if err := RemoveUserShellLauncher(target); err != nil {
		t.Fatal(err)
	}
	if got, err := os.Readlink(launcherLink(t, "gentle-shell")); err != nil || got != other {
		t.Fatalf("foreign gentle-shell launcher was touched: %q %v", got, err)
	}
	if err := os.Remove(launcherLink(t, "gentle-shell")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(launcherLink(t, "gentle-shell"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := RemoveUserShellLauncher(target); err == nil {
		t.Fatal("non-symlink gentle-shell launcher removal was accepted")
	}
	if err := os.Remove(launcherLink(t, "gentle-shell")); err != nil {
		t.Fatal(err)
	}

	// A foreign gs regular file is never removed by an uninstall.
	if err := os.WriteFile(launcherLink(t, "gs"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := RemoveUserShellLauncher(target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(launcherLink(t, "gs")); err != nil {
		t.Fatalf("foreign gs launcher was touched: %v", err)
	}
}
