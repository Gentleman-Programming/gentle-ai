package app

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	piagent "github.com/gentleman-programming/gentle-ai/v4/internal/agents/pi"
	"github.com/gentleman-programming/gentle-ai/v4/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

func TestRunArgsRejectsInvalidIsolationVariablesBeforeDetection(t *testing.T) {
	for _, tc := range []struct {
		name  string
		env   string
		value string
	}{
		{"relative GENTLE_AI_HOME", state.HomeEnv, "relative/state"},
		{"relative GENTLE_AI_PI_CONFIG_DIR", piagent.GentlePiConfigDirEnv, "relative/config"},
		{"missing GENTLE_AI_ENGRAM_BIN", system.EngramBinEnv, filepath.Join(t.TempDir(), "engram")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupMockHome(t, t.TempDir())
			t.Setenv(tc.env, tc.value)
			oldSelf, oldDetect, oldEnsure := selfUpdateFn, detectSystem, ensureCurrentOSSupported
			t.Cleanup(func() { selfUpdateFn, detectSystem, ensureCurrentOSSupported = oldSelf, oldDetect, oldEnsure })
			ensureCurrentOSSupported = func() error { return nil }
			detectSystem = func(context.Context) (system.DetectionResult, error) {
				t.Fatal("system detection ran despite an invalid isolation variable")
				return system.DetectionResult{}, nil
			}
			selfUpdateFn = func(context.Context, string, system.PlatformProfile, io.Writer) error {
				t.Fatal("self-update ran despite an invalid isolation variable")
				return nil
			}

			err := RunArgs([]string{"install", "--agent", "pi", "--dry-run"}, &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), tc.env) {
				t.Fatalf("RunArgs() error = %v, want an error naming %s", err, tc.env)
			}
		})
	}
}

func TestBackupRootFollowsGentleAIHome(t *testing.T) {
	home := t.TempDir()
	setupMockHome(t, home)
	restore := backup.BackupRootFn
	t.Cleanup(func() { backup.BackupRootFn = restore })

	t.Setenv(state.HomeEnv, "")
	applyStateHome()
	if got, err := backup.BackupRootFn(); err != nil || got != filepath.Join(home, ".gentle-ai", "backups") {
		t.Fatalf("default BackupRootFn() = %q, %v", got, err)
	}

	override := filepath.Join(t.TempDir(), "gentle-ai")
	t.Setenv(state.HomeEnv, override)
	applyStateHome()
	if got, err := backup.BackupRootFn(); err != nil || got != filepath.Join(override, "backups") {
		t.Fatalf("BackupRootFn() with %s = %q, %v", state.HomeEnv, got, err)
	}
}
