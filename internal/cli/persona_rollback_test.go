package cli

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/persona"
	"github.com/gentleman-programming/gentle-ai/v2/internal/pipeline"
)

// captureStderr runs fn with os.Stderr redirected to a pipe and returns
// everything written to it. Tests in this package never use t.Parallel with
// the persona seam, so temporarily swapping the global stderr is safe.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() { os.Stderr = old }()
	fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestHandleRolledBackPersonaTransition pins the D2 classification contract:
// the CLI converts a rolled-back persona output-style removal into an exit-0
// warning ONLY when the failure is the typed removal error AND the pipeline
// rollback succeeded. Rollback failure, generic errors, and nil errors must
// fall through to the hard-failure path unchanged and print nothing.
func TestHandleRolledBackPersonaTransition(t *testing.T) {
	typedErr := &persona.OutputStyleRemovalError{
		Path:         "/home/.claude/output-styles/gentleman.md",
		SettingsPath: "/home/.claude/settings.json",
		Err:          errors.New("simulated removal failure: file locked"),
	}

	t.Run("typed error with successful rollback returns true and warns", func(t *testing.T) {
		exec := pipeline.ExecutionResult{
			Err:      typedErr,
			Rollback: pipeline.StageResult{Success: true},
		}
		var handled bool
		stderr := captureStderr(t, func() {
			handled = handleRolledBackPersonaTransition(exec)
		})
		if !handled {
			t.Fatal("handleRolledBackPersonaTransition(typed+rollback-ok) = false, want true")
		}
		if !strings.Contains(stderr, persona.MessageRolledBackOutputStyle) {
			t.Fatalf("stderr missing rollback warning %q; got:\n%s", persona.MessageRolledBackOutputStyle, stderr)
		}
		if !strings.HasPrefix(stderr, "WARNING: ") {
			t.Fatalf("stderr does not start with WARNING prefix; got:\n%s", stderr)
		}
		for _, target := range typedErr.RestoredTargets() {
			if !strings.Contains(stderr, target) {
				t.Fatalf("stderr missing restored target %q; got:\n%s", target, stderr)
			}
		}
	})

	t.Run("typed error without settings path still names the retired style", func(t *testing.T) {
		noSettingsErr := &persona.OutputStyleRemovalError{
			Path: "/home/.claude/output-styles/gentleman.md",
			Err:  errors.New("simulated removal failure: file locked"),
		}
		exec := pipeline.ExecutionResult{
			Err:      noSettingsErr,
			Rollback: pipeline.StageResult{Success: true},
		}
		var handled bool
		stderr := captureStderr(t, func() {
			handled = handleRolledBackPersonaTransition(exec)
		})
		if !handled {
			t.Fatal("handleRolledBackPersonaTransition(typed no-settings+rollback-ok) = false, want true")
		}
		if !strings.Contains(stderr, noSettingsErr.Path) {
			t.Fatalf("stderr missing restored style path %q; got:\n%s", noSettingsErr.Path, stderr)
		}
		if strings.Contains(stderr, "settings.json") {
			t.Fatalf("stderr unexpectedly names settings when SettingsPath is empty; got:\n%s", stderr)
		}
	})

	t.Run("typed error with failed rollback returns false and stays silent", func(t *testing.T) {
		exec := pipeline.ExecutionResult{
			Err:      typedErr,
			Rollback: pipeline.StageResult{Success: false, Err: errors.New("rollback failed")},
		}
		var handled bool
		stderr := captureStderr(t, func() {
			handled = handleRolledBackPersonaTransition(exec)
		})
		if handled {
			t.Fatal("handleRolledBackPersonaTransition(typed+rollback-failed) = true, want false")
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty when rollback failed", stderr)
		}
	})

	t.Run("generic error returns false and stays silent", func(t *testing.T) {
		exec := pipeline.ExecutionResult{
			Err:      errors.New("unrelated pipeline failure"),
			Rollback: pipeline.StageResult{Success: true},
		}
		var handled bool
		stderr := captureStderr(t, func() {
			handled = handleRolledBackPersonaTransition(exec)
		})
		if handled {
			t.Fatal("handleRolledBackPersonaTransition(generic) = true, want false")
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty for generic error", stderr)
		}
	})

	t.Run("nil error returns false and stays silent", func(t *testing.T) {
		var handled bool
		stderr := captureStderr(t, func() {
			handled = handleRolledBackPersonaTransition(pipeline.ExecutionResult{})
		})
		if handled {
			t.Fatal("handleRolledBackPersonaTransition(nil) = true, want false")
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty for nil error", stderr)
		}
	})
}
