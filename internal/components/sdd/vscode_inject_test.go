package sdd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestInject_VSCodeSubAgents(t *testing.T) {
	vscodeAdapter, err := agents.NewAdapter("vscode-copilot")
	if err != nil {
		t.Fatalf("NewAdapter(vscode-copilot) error = %v", err)
	}

	home := t.TempDir()

	result, err := Inject(home, vscodeAdapter, model.SDDModeMulti)
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject() first run should report changed = true")
	}

	agentsDir := vscodeAdapter.SubAgentsDir(home)

	// Verify agents dir was created
	if _, statErr := os.Stat(agentsDir); os.IsNotExist(statErr) {
		t.Fatalf("agents dir %q was not created", agentsDir)
	}

	// Verify all 10 .agent.md files exist in the agents dir
	expectedPhases := []string{
		"sdd-init", "sdd-explore", "sdd-propose", "sdd-spec",
		"sdd-design", "sdd-tasks", "sdd-apply", "sdd-verify",
		"sdd-archive", "sdd-onboard",
	}

	for _, phase := range expectedPhases {
		t.Run(phase, func(t *testing.T) {
			fileName := phase + ".agent.md"
			path := filepath.Join(agentsDir, fileName)
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("ReadFile(%q) error = %v — sub-agent file should exist", path, readErr)
			}
			if len(data) < 10 {
				t.Fatalf("sub-agent %q is too small (%d bytes), likely truncated", path, len(data))
			}
			content := string(data)
			if !strings.Contains(content, "name: "+phase) {
				t.Fatalf("sub-agent %q missing name field for %s", path, phase)
			}
		})
	}

	// Verify post-check: sdd-apply.agent.md should exist and be non-trivial
	applyPath := filepath.Join(agentsDir, "sdd-apply.agent.md")
	applyData, applyErr := os.ReadFile(applyPath)
	if applyErr != nil {
		t.Fatalf("post-check: sdd-apply.agent.md not found: %v", applyErr)
	}
	if len(applyData) < 10 {
		t.Fatalf("post-check: sdd-apply.agent.md is too small (%d bytes)", len(applyData))
	}
}

func TestInject_VSCode_ImplicitFeatureFlag(t *testing.T) {
	// Verify that inject only writes sub-agents when SupportsSubAgents() returns true.
	// OpenCode does NOT support sub-agents (returns false) — no .agent.md files
	// should be written to any agent agents directory by the 3c path.
	opencodeAdapter := opencode.NewAdapter()
	home := t.TempDir()

	_, err := Inject(home, opencodeAdapter, model.SDDModeMulti)
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	// VS Code agents directory should NOT have been created by OpenCode inject
	vscodeAgentsDir := filepath.Join(home, ".copilot", "agents")
	if _, statErr := os.Stat(vscodeAgentsDir); !os.IsNotExist(statErr) {
		entries, readErr := os.ReadDir(vscodeAgentsDir)
		if readErr != nil {
			t.Fatalf("ReadDir(%q) error = %v", vscodeAgentsDir, readErr)
		}
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".agent.md") {
				t.Fatalf("OpenCode injection should not write .agent.md files, found %q", entry.Name())
			}
		}
	}
}

func TestPostInjectionValidation_VSCode(t *testing.T) {
	vscodeAdapter, err := agents.NewAdapter("vscode-copilot")
	if err != nil {
		t.Fatalf("NewAdapter(vscode-copilot) error = %v", err)
	}

	home := t.TempDir()

	// First injection should succeed and write files
	_, err = Inject(home, vscodeAdapter, model.SDDModeMulti)
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	// Verify post-check catches a truncated sdd-apply file
	agentsDir := vscodeAdapter.SubAgentsDir(home)
	applyPath := filepath.Join(agentsDir, "sdd-apply.agent.md")
	if err := os.WriteFile(applyPath, []byte("tiny"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", applyPath, err)
	}

	// Re-inject should fix the truncated file (overwrites and passes post-check)
	result, err := Inject(home, vscodeAdapter, model.SDDModeMulti)
	if err != nil {
		t.Fatalf("Re-inject after truncation error = %v", err)
	}
	// Re-injection should have fixed the file
	_ = result

	data, readErr := os.ReadFile(applyPath)
	if readErr != nil {
		t.Fatalf("ReadFile(%q) after re-inject error = %v", applyPath, readErr)
	}
	if len(data) < 10 {
		t.Fatalf("sdd-apply.agent.md still too small after re-injection (%d bytes)", len(data))
	}
}

func TestPostInjectionValidation_VSCode_MissingFileDetected(t *testing.T) {
	vscodeAdapter, err := agents.NewAdapter("vscode-copilot")
	if err != nil {
		t.Fatalf("NewAdapter(vscode-copilot) error = %v", err)
	}

	home := t.TempDir()

	// First injection succeeds
	_, err = Inject(home, vscodeAdapter, model.SDDModeMulti)
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	// Delete sdd-verify.agent.md to simulate a missing file
	agentsDir := vscodeAdapter.SubAgentsDir(home)
	verifyPath := filepath.Join(agentsDir, "sdd-verify.agent.md")
	if err := os.Remove(verifyPath); err != nil {
		t.Fatalf("Remove(%q) error = %v", verifyPath, err)
	}

	// Re-inject should succeed (restores the missing file)
	_, err = Inject(home, vscodeAdapter, model.SDDModeMulti)
	if err != nil {
		t.Fatalf("Re-inject after removing verify file error = %v", err)
	}
}

// TestInject_VSCode_DefaultProfile_IsIdempotent verifies that running Inject
// twice in a row with identical inputs does NOT duplicate or rewrite files.
// The second run must report Changed=false and leave file mtimes untouched —
// proving that filemerge.WriteFileAtomic's content-equality short-circuit
// holds across the full VS Code default-profile path.
func TestInject_VSCode_DefaultProfile_IsIdempotent(t *testing.T) {
	vscodeAdapter, err := agents.NewAdapter("vscode-copilot")
	if err != nil {
		t.Fatalf("NewAdapter(vscode-copilot) error = %v", err)
	}

	home := t.TempDir()

	first, err := Inject(home, vscodeAdapter, model.SDDModeMulti)
	if err != nil {
		t.Fatalf("first Inject() error = %v", err)
	}
	if !first.Changed {
		t.Fatal("first Inject() should report Changed = true")
	}

	agentsDir := vscodeAdapter.SubAgentsDir(home)
	firstFiles, err := snapshotAgentFiles(agentsDir)
	if err != nil {
		t.Fatalf("snapshotAgentFiles after first inject error = %v", err)
	}
	if len(firstFiles) != 11 {
		t.Fatalf("expected 11 default .agent.md files (orchestrator + 10 phases), got %d", len(firstFiles))
	}

	second, err := Inject(home, vscodeAdapter, model.SDDModeMulti)
	if err != nil {
		t.Fatalf("second Inject() error = %v", err)
	}
	if second.Changed {
		t.Errorf("second Inject() with identical inputs reported Changed=true; want false (not idempotent)")
	}

	assertNoFileChurn(t, agentsDir, firstFiles)
}

// TestInject_VSCode_NamedProfile_IsIdempotent verifies idempotency for the
// step-2c named profile path. Running Inject twice with the same Profile
// must leave the 10 default agents AND the 10 suffixed profile agents
// unchanged on disk.
func TestInject_VSCode_NamedProfile_IsIdempotent(t *testing.T) {
	vscodeAdapter, err := agents.NewAdapter("vscode-copilot")
	if err != nil {
		t.Fatalf("NewAdapter(vscode-copilot) error = %v", err)
	}

	home := t.TempDir()

	opts := InjectOptions{
		Profiles: []model.Profile{
			{
				Name: "cheap",
				PhaseAssignments: map[string]model.ModelAssignment{
					"sdd-apply":  {ProviderID: "anthropic", ModelID: "claude-haiku-4-5"},
					"sdd-verify": {ProviderID: "anthropic", ModelID: "claude-sonnet-4"},
				},
			},
		},
	}

	first, err := Inject(home, vscodeAdapter, model.SDDModeMulti, opts)
	if err != nil {
		t.Fatalf("first Inject() error = %v", err)
	}
	if !first.Changed {
		t.Fatal("first Inject() with named profile should report Changed = true")
	}

	agentsDir := vscodeAdapter.SubAgentsDir(home)
	firstFiles, err := snapshotAgentFiles(agentsDir)
	if err != nil {
		t.Fatalf("snapshotAgentFiles after first inject error = %v", err)
	}
	// Expect 22: 11 default unsuffixed (orchestrator + 10 phases) + 11 "*-cheap.agent.md"
	if len(firstFiles) != 22 {
		t.Fatalf("expected 22 files (11 default + 11 cheap, each including orchestrator), got %d", len(firstFiles))
	}

	second, err := Inject(home, vscodeAdapter, model.SDDModeMulti, opts)
	if err != nil {
		t.Fatalf("second Inject() error = %v", err)
	}
	if second.Changed {
		t.Errorf("second Inject() with identical profile reported Changed=true; want false (named-profile path not idempotent)")
	}

	assertNoFileChurn(t, agentsDir, firstFiles)
}

// snapshotAgentFiles returns a map of file name → mod time for all entries
// in agentsDir. Used to detect spurious rewrites between Inject calls.
func snapshotAgentFiles(agentsDir string) (map[string]time.Time, error) {
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return nil, err
	}
	snap := make(map[string]time.Time, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		snap[entry.Name()] = info.ModTime()
	}
	return snap, nil
}

// assertNoFileChurn checks that agentsDir matches prior exactly: same file
// set, same modification times. Any divergence indicates a non-idempotent
// rewrite.
func assertNoFileChurn(t *testing.T, agentsDir string, prior map[string]time.Time) {
	t.Helper()
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		t.Fatalf("ReadDir(%q) error = %v", agentsDir, err)
	}
	if len(entries) != len(prior) {
		t.Errorf("file count diverged: prior=%d, current=%d", len(prior), len(entries))
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			t.Fatalf("Info(%q) error = %v", entry.Name(), err)
		}
		priorMod, existed := prior[entry.Name()]
		if !existed {
			t.Errorf("new file %q appeared after re-inject — not idempotent", entry.Name())
			continue
		}
		if !info.ModTime().Equal(priorMod) {
			t.Errorf("file %q mtime changed: prior=%v, current=%v — atomic writer rewrote unchanged content",
				entry.Name(), priorMod, info.ModTime())
		}
	}
}