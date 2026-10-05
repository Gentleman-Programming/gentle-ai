package assets

import (
	"io/fs"
	"path"
	"strings"
	"testing"
)

// A gate that rejects unresolved paths must distinguish promised future paths
// from paths claimed to have already been created or read. ODD orchestrators
// without a path-existence gate have no such rejection to qualify.
const plannedPathCarveOut = "A path the artifact explicitly marks as planned (to be created by a later apply) is not required to exist yet; only paths claimed as already created or read must resolve."

func rejectsPlannedPaths(content string) bool {
	return strings.Contains(content, "FAILS the gate.") && !strings.Contains(content, plannedPathCarveOut)
}

func TestEveryODDOrchestratorDoesNotRejectPlannedPaths(t *testing.T) {
	seen := map[string]bool{}
	err := fs.WalkDir(FS, ".", func(assetPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || path.Base(assetPath) != "orchestrator.md" {
			return nil
		}
		seen[path.Dir(assetPath)] = true
		content := MustRead(assetPath)
		if !strings.Contains(content, "GENTLE_AI_ODD_SECTION:") {
			t.Errorf("%s no longer carries the ODD contract", assetPath)
		}
		if rejectsPlannedPaths(content) {
			t.Errorf("%s rejects unresolved paths without the planned-path carve-out", assetPath)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk ODD orchestrators: %v", err)
	}
	for _, runtime := range []string{"antigravity", "claude", "codex", "cursor", "gemini", "generic", "hermes", "kimi", "kiro", "opencode", "qwen", "windsurf"} {
		if !seen[runtime] {
			t.Errorf("missing ODD orchestrator for %s", runtime)
		}
	}
	if len(seen) != 12 {
		t.Errorf("ODD orchestrator inventory changed: got %d runtimes, want 12", len(seen))
	}
}

func TestPlannedPathGateDistinguishesUnqualifiedRejection(t *testing.T) {
	for _, tt := range []struct {
		name, content string
		rejects       bool
	}{
		{"unqualified gate", "A path that does not resolve FAILS the gate.", true},
		{"planned path exception", "A path that does not resolve FAILS the gate. " + plannedPathCarveOut, false},
		{"no path-existence gate", "Track planned implementation paths as tasks.", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := rejectsPlannedPaths(tt.content); got != tt.rejects {
				t.Errorf("rejectsPlannedPaths() = %v, want %v", got, tt.rejects)
			}
		})
	}
}
