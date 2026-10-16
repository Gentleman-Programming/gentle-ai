package uninstall

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/agentguidance"
)

// #5256 U2c: a failed pilot retirement only claims partial changes when its
// rollback did not complete. This drives the report from a retirement Result;
// it does not reproduce the failed restore that would produce it.

func TestClaudeModuleRetirementFailureReport(t *testing.T) {
	_, _, home := claudeModuleUninstallFixture(t, true)
	paths := pilotPaths(t, home)
	corePath, moduleDir := paths[0], filepath.Dir(paths[len(paths)-1])

	// Changed reports whether the failed retirement may have left changes.
	tests := []struct {
		name    string
		changed bool
	}{
		{name: "rollback incomplete", changed: true},
		{name: "rolled back or never written", changed: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cause := errors.New("restore orchestrator core; modules not rolled back")
			var result Result
			retired := agentguidance.Result{Changed: tt.changed, Files: paths}
			if err := reportClaudeModuleRetirement(&result, paths, retired, cause); err != cause {
				t.Fatalf("error = %v, want the retirement error unchanged", err)
			}
			if len(result.RemovedFiles) != 0 || len(result.ChangedFiles) != 0 {
				t.Fatalf("failed retirement claimed files: removed %v, changed %v", result.RemovedFiles, result.ChangedFiles)
			}
			if !tt.changed {
				if len(result.ManualActions) != 0 {
					t.Fatalf("ManualActions = %v, want no partial-change claim", result.ManualActions)
				}
				return
			}
			if len(result.ManualActions) != 1 {
				t.Fatalf("ManualActions = %v, want one inspection notice", result.ManualActions)
			}
			action := result.ManualActions[0]
			for _, want := range []string{corePath, moduleDir, "may be partially changed"} {
				if !strings.Contains(action, want) {
					t.Fatalf("ManualActions[0] = %q, missing %q", action, want)
				}
			}
		})
	}
}
