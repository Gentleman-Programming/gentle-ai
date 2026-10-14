package uninstall

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/codex"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// TestComponentOperationsEngram_CodexHonorsCodexHomeOverride pins issue #5031
// end to end for the engram component: when CODEX_HOME points at an existing
// directory and the supplied home is the real user home, injection writes the
// Codex instruction files under the override, and the uninstaller targets and
// removes exactly those files — no orphan under <home>/.codex.
func TestComponentOperationsEngram_CodexHonorsCodexHomeOverride(t *testing.T) {
	restore := codex.SetRuntimeVersionCommandForTest("codex-cli 0.144.0", nil)
	t.Cleanup(restore)

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	override := t.TempDir()
	t.Setenv("CODEX_HOME", override)

	canonical, err := filepath.EvalSymlinks(override)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	workspaceDir := t.TempDir()

	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	adapter, ok := svc.registry.Get(model.AgentCodex)
	if !ok {
		t.Fatal("codex adapter not found in registry")
	}

	// Write via the real injector, like the consolidated-assets test: any
	// drift between renderer and uninstaller must fail here.
	if _, err := engram.InjectWithOptions(homeDir, adapter, engram.InjectOptions{}); err != nil {
		t.Fatalf("engram.InjectWithOptions(codex) error = %v", err)
	}

	instructionsPath := filepath.Join(canonical, "engram-instructions.md")
	compactPath := filepath.Join(canonical, "engram-compact-prompt.md")
	for _, path := range []string{instructionsPath, compactPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected engram injection to create %q under CODEX_HOME: %v", path, err)
		}
	}

	// Nothing may be written into the stale <home>/.codex root.
	for _, name := range []string{"engram-instructions.md", "engram-compact-prompt.md"} {
		if _, err := os.Stat(filepath.Join(homeDir, ".codex", name)); !os.IsNotExist(err) {
			t.Fatalf("expected no %q under <home>/.codex when CODEX_HOME is set, err = %v", name, err)
		}
	}

	svc.SetEngramUninstallScope(model.EngramUninstallScopeGlobal)

	ops, targets, err := svc.componentOperations(adapter, model.ComponentEngram)
	if err != nil {
		t.Fatalf("componentOperations() error = %v", err)
	}

	for _, want := range []string{instructionsPath, compactPath} {
		if !slices.Contains(targets, want) {
			t.Fatalf("componentOperations() targets missing %q (CODEX_HOME root); got: %v", want, targets)
		}
	}

	for _, op := range ops {
		if _, _, err := op.apply(op.path); err != nil {
			t.Fatalf("op.apply(%q) error = %v", op.path, err)
		}
	}

	for _, path := range []string{instructionsPath, compactPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected %q to be removed by uninstall, err = %v", path, err)
		}
	}

	// No orphaned engram asset left behind in the override root either.
	if entries, err := os.ReadDir(canonical); err == nil {
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "engram-") {
				t.Fatalf("orphaned engram asset left behind after uninstall: %s", entry.Name())
			}
		}
	}
}
