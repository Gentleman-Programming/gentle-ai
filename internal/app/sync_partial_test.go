package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/cli"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v3/internal/state"
)

// undetectableOpenCodeHome isolates HOME and the workspace for a sync of
// Claude Code plus OpenCode whose runtime version cannot be detected.
func undetectableOpenCodeHome(t *testing.T) string {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Chdir(workspace)
	if err := state.Write(home, state.InstallState{
		InstalledAgents:     []string{"claude-code", "opencode"},
		SelectionConfigured: true,
		Persona:             "neutral",
	}); err != nil {
		t.Fatal(err)
	}
	restore := opencode.VersionRunnerOverride
	t.Cleanup(func() { opencode.VersionRunnerOverride = restore })
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		return opencode.CommandOutput{}, os.ErrNotExist
	}
	return home
}

func TestRunArgsSyncPrintsPartialReportAndFails(t *testing.T) {
	home := undetectableOpenCodeHome(t)
	var out bytes.Buffer
	err := RunArgs([]string{"sync", "--agents", "claude-code,opencode"}, &out)
	var partial *cli.PartialSyncError
	if !errors.As(err, &partial) {
		t.Fatalf("RunArgs(sync) error = %v, want *cli.PartialSyncError so the exit code is non-zero", err)
	}
	for _, want := range []string{"Agents synced: claude-code", "Agents skipped: opencode", "opencode --version", "gentle-ai sync"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("sync output missing %q:\n%s", want, out.String())
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "CLAUDE.md")); err != nil {
		t.Errorf("Claude Code was not synced: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".config", "opencode")); !os.IsNotExist(err) {
		t.Errorf("OpenCode config was written although its runtime is unknown: %v", err)
	}
}

func TestTUISyncReportsSkippedOpenCode(t *testing.T) {
	overrides := &model.SyncOverrides{TargetAgents: []model.AgentID{model.AgentClaudeCode, model.AgentOpenCode}}

	t.Run("detailed sync shows the skip as a manual action", func(t *testing.T) {
		home := undetectableOpenCodeHome(t)
		files, actions, err := tuiSyncDetailed(home)(overrides)
		if err != nil {
			t.Fatalf("tuiSyncDetailed() error = %v, want the applied part reported without failing", err)
		}
		if len(files) == 0 {
			t.Error("tuiSyncDetailed() changed no files, want Claude Code synced")
		}
		if !strings.Contains(strings.Join(actions, "\n"), "opencode was skipped") {
			t.Errorf("manual actions = %q, want the OpenCode skip", actions)
		}
	})

	t.Run("plain sync surfaces the skip as its error", func(t *testing.T) {
		home := undetectableOpenCodeHome(t)
		_, err := tuiSync(home)(overrides)
		var partial *cli.PartialSyncError
		if !errors.As(err, &partial) {
			t.Fatalf("tuiSync() error = %v, want *cli.PartialSyncError", err)
		}
	})
}
