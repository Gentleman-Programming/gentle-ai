package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v4/internal/pipeline"
)

// executeRollbackAfterOverwrite snapshots targets, overwrites every target in
// a later apply step that then fails, and returns the pipeline result, driving
// the same rollbackRestoreStep a failed install or sync runs.
func executeRollbackAfterOverwrite(t *testing.T, home, workspace string, targets []string) pipeline.ExecutionResult {
	t.Helper()
	state := &runtimeState{}
	cause := errors.New("injected failure after the managed writes")
	plan := pipeline.StagePlan{
		Prepare: []pipeline.Step{prepareBackupStep{id: "prepare:backup-snapshot", snapshotter: backup.NewSnapshotter(), snapshotDir: filepath.Join(t.TempDir(), "snapshot"), targets: targets, state: state}},
		Apply: []pipeline.Step{
			rollbackRestoreStep{id: "apply:rollback-restore", state: state, homeDir: home, workspaceDir: workspace},
			overwriteAllThenFailStep{paths: targets, cause: cause},
		},
	}
	result := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy()).Execute(plan)
	if !errors.Is(result.Err, cause) {
		t.Fatalf("pipeline error = %v, want the injected failure", result.Err)
	}
	return result
}

// TestRollbackRestoresGGAConfigUnderRedirectedAppData pins issue #6030: a
// failed install or sync restores the gga files it snapshotted under a
// %APPDATA% redirected outside the profile, like the standalone and uninstall
// restores already do (#6027).
func TestRollbackRestoresGGAConfigUnderRedirectedAppData(t *testing.T) {
	home, appData := redirectedAppDataCLIHome(t, "windows")
	workspace := t.TempDir()
	homeFile := filepath.Join(home, ".claude", "CLAUDE.md")
	ggaConfig := filepath.Join(appData, "gga", "config")
	ggaAgents := filepath.Join(appData, "gga", "AGENTS.md")
	originals := map[string]string{homeFile: "home original", ggaConfig: "gga config original", ggaAgents: "gga agents original"}
	for path, content := range originals {
		writeCLIRestoreFile(t, path, content)
	}

	result := executeRollbackAfterOverwrite(t, home, workspace, []string{homeFile, ggaConfig, ggaAgents})
	if !result.Rollback.Success {
		t.Fatalf("rollback = %#v, want success", result.Rollback)
	}
	for path, want := range originals {
		if got := readCLIRestoreFile(t, path); got != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
}

// TestRollbackRefusesAppDataEntriesOutsideGGA pins that the rollback admits
// only the gga product directory: an entry elsewhere in the redirected
// %APPDATA%, or in the temp directory under %LOCALAPPDATA%, is still refused
// and nothing in the manifest is written.
func TestRollbackRefusesAppDataEntriesOutsideGGA(t *testing.T) {
	for _, tc := range []struct {
		name    string
		outside func(appData string) string
	}{
		{name: "sibling in APPDATA", outside: func(appData string) string { return filepath.Join(appData, "Other", "file.txt") }},
		{name: "temp under LOCALAPPDATA", outside: func(string) string { return filepath.Join(os.Getenv("LOCALAPPDATA"), "Temp", "003", "outside.json") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, appData := redirectedAppDataCLIHome(t, "windows")
			outside := tc.outside(appData)
			ggaAgents := filepath.Join(appData, "gga", "AGENTS.md")
			writeCLIRestoreFile(t, outside, "outside original")
			writeCLIRestoreFile(t, ggaAgents, "gga original")

			result := executeRollbackAfterOverwrite(t, home, t.TempDir(), []string{outside, ggaAgents})
			if result.Rollback.Success || result.Rollback.Err == nil || !strings.Contains(result.Rollback.Err.Error(), "invalid OriginalPath") || !strings.Contains(result.Rollback.Err.Error(), outside) {
				t.Fatalf("rollback = %#v, want an invalid OriginalPath refusal for %q", result.Rollback, outside)
			}
			if got := readCLIRestoreFile(t, outside); got != overwrittenContent {
				t.Fatalf("refused entry = %q, want it untouched", got)
			}
			if got := readCLIRestoreFile(t, ggaAgents); got != overwrittenContent {
				t.Fatalf("gga AGENTS.md = %q, want it untouched by the refused restore", got)
			}
		})
	}
}

// TestRollbackRootsAddOnlyGGAOnWindows pins the derived roots: on Windows a gga
// target adds exactly %APPDATA%\gga, while elsewhere the roots stay the
// historical home and workspace pair whatever the targets.
func TestRollbackRootsAddOnlyGGAOnWindows(t *testing.T) {
	for _, tc := range []struct {
		goos    string
		wantGGA bool
	}{
		{goos: "windows", wantGGA: true},
		{goos: "linux"},
		{goos: "darwin"},
	} {
		t.Run(tc.goos, func(t *testing.T) {
			home, appData := redirectedAppDataCLIHome(t, tc.goos)
			workspace := t.TempDir()
			targets := []string{filepath.Join(home, ".claude", "CLAUDE.md"), filepath.Join(appData, "gga", "config"), filepath.Join(appData, "Other", "file.txt")}
			want := []string{home, workspace}
			if tc.wantGGA {
				want = append(want, filepath.Join(appData, "gga"))
			}
			if got := rollbackRoots(home, workspace, targets); strings.Join(got, "|") != strings.Join(want, "|") {
				t.Fatalf("rollbackRoots() = %v, want %v", got, want)
			}
		})
	}
}

const overwrittenContent = "managed content\n"

type overwriteAllThenFailStep struct {
	paths []string
	cause error
}

func (s overwriteAllThenFailStep) ID() string { return "test:overwrite-all-then-fail" }
func (s overwriteAllThenFailStep) Run() error {
	for _, path := range s.paths {
		if err := os.WriteFile(path, []byte(overwrittenContent), 0o644); err != nil {
			return err
		}
	}
	return s.cause
}
func (s overwriteAllThenFailStep) Rollback() error { return nil }
