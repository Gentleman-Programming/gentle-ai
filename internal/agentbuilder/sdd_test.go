package agentbuilder

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeSDDAgent creates a GeneratedAgent with a given SDDConfig for testing.
func makeSDDAgent(name, trigger string, cfg *SDDIntegration) *GeneratedAgent {
	return &GeneratedAgent{
		Name:      name,
		Title:     "Test SDD Agent",
		Trigger:   trigger,
		Content:   "# Test SDD Agent\n",
		SDDConfig: cfg,
	}
}

func TestInjectSDDReference_InjectIntoEmptyFile(t *testing.T) {
	dir := t.TempDir()
	promptFile := filepath.Join(dir, "system-prompt.md")

	// Start with an empty file.
	if err := os.WriteFile(promptFile, []byte(""), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	agent := makeSDDAgent("my-skill", "When the user asks to do X", &SDDIntegration{
		Mode:        SDDPhaseSupport,
		TargetPhase: "apply",
	})

	if err := InjectSDDReference(agent, promptFile); err != nil {
		t.Fatalf("InjectSDDReference: %v", err)
	}

	data, err := os.ReadFile(promptFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	content := string(data)
	marker := "<!-- gentle-ai:custom-agent:my-skill -->"
	if !strings.Contains(content, marker) {
		t.Errorf("marker not found in file;\ngot:\n%s", content)
	}
}

func TestInjectSDDReference_ExistingContentPreserved(t *testing.T) {
	dir := t.TempDir()
	promptFile := filepath.Join(dir, "system-prompt.md")

	existing := "# My System Prompt\n\nSome existing instructions here.\n"
	if err := os.WriteFile(promptFile, []byte(existing), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	agent := makeSDDAgent("my-skill", "When X happens", &SDDIntegration{
		Mode:        SDDPhaseSupport,
		TargetPhase: "spec",
	})

	if err := InjectSDDReference(agent, promptFile); err != nil {
		t.Fatalf("InjectSDDReference: %v", err)
	}

	data, _ := os.ReadFile(promptFile)
	content := string(data)

	if !strings.Contains(content, "My System Prompt") {
		t.Errorf("existing content was not preserved;\ngot:\n%s", content)
	}
	if !strings.Contains(content, "<!-- gentle-ai:custom-agent:my-skill -->") {
		t.Errorf("marker not found in file;\ngot:\n%s", content)
	}
}

func TestInjectSDDReference_DuplicateInjection_MarkerReplacedNotDuplicated(t *testing.T) {
	dir := t.TempDir()
	promptFile := filepath.Join(dir, "system-prompt.md")

	if err := os.WriteFile(promptFile, []byte("# Prompt\n"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	agent := makeSDDAgent("dedup-skill", "When dedup needed", &SDDIntegration{
		Mode:        SDDPhaseSupport,
		TargetPhase: "verify",
	})

	// Inject twice.
	if err := InjectSDDReference(agent, promptFile); err != nil {
		t.Fatalf("first inject: %v", err)
	}
	if err := InjectSDDReference(agent, promptFile); err != nil {
		t.Fatalf("second inject: %v", err)
	}

	data, _ := os.ReadFile(promptFile)
	content := string(data)

	// Count marker occurrences — should appear exactly once.
	marker := "<!-- gentle-ai:custom-agent:dedup-skill -->"
	count := strings.Count(content, marker)
	if count != 1 {
		t.Errorf("marker appears %d times, want exactly 1;\ngot:\n%s", count, content)
	}
}

func TestInjectSDDReference_NewPhaseMode_DependencyGraphReferenced(t *testing.T) {
	dir := t.TempDir()
	promptFile := filepath.Join(dir, "system-prompt.md")

	if err := os.WriteFile(promptFile, []byte("# Prompt\n"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	agent := makeSDDAgent("new-phase-skill", "When new phase starts", &SDDIntegration{
		Mode:      SDDNewPhase,
		PhaseName: "my-phase",
	})

	if err := InjectSDDReference(agent, promptFile); err != nil {
		t.Fatalf("InjectSDDReference: %v", err)
	}

	data, _ := os.ReadFile(promptFile)
	content := string(data)

	// New phase references the dependency graph.
	if !strings.Contains(content, "dependency graph") {
		t.Errorf("new-phase block should reference dependency graph;\ngot:\n%s", content)
	}
	if !strings.Contains(content, "my-phase") {
		t.Errorf("new-phase block should reference phase name 'my-phase';\ngot:\n%s", content)
	}
}

func TestInjectSDDReference_PhaseSupportMode_TargetPhaseReferenced(t *testing.T) {
	dir := t.TempDir()
	promptFile := filepath.Join(dir, "system-prompt.md")

	if err := os.WriteFile(promptFile, []byte("# Prompt\n"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	agent := makeSDDAgent("support-skill", "When supporting design phase", &SDDIntegration{
		Mode:        SDDPhaseSupport,
		TargetPhase: "design",
	})

	if err := InjectSDDReference(agent, promptFile); err != nil {
		t.Fatalf("InjectSDDReference: %v", err)
	}

	data, _ := os.ReadFile(promptFile)
	content := string(data)

	if !strings.Contains(content, "design") {
		t.Errorf("phase-support block should reference target phase 'design';\ngot:\n%s", content)
	}
	if !strings.Contains(content, "sdd-design") {
		t.Errorf("phase-support block should reference sdd-design trigger;\ngot:\n%s", content)
	}
}

func TestInjectSDDReference_StandaloneMode_IsNoop(t *testing.T) {
	dir := t.TempDir()
	promptFile := filepath.Join(dir, "system-prompt.md")

	original := "# My Prompt\n\nNo changes expected.\n"
	if err := os.WriteFile(promptFile, []byte(original), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	agent := makeSDDAgent("standalone-skill", "Never triggered", &SDDIntegration{
		Mode: SDDStandalone,
	})

	if err := InjectSDDReference(agent, promptFile); err != nil {
		t.Fatalf("InjectSDDReference: %v", err)
	}

	data, _ := os.ReadFile(promptFile)
	if string(data) != original {
		t.Errorf("standalone mode should be a no-op;\ngot:\n%s\nwant:\n%s", string(data), original)
	}
}

func TestInjectSDDReference_NilAgent_IsNoop(t *testing.T) {
	dir := t.TempDir()
	promptFile := filepath.Join(dir, "system-prompt.md")

	if err := os.WriteFile(promptFile, []byte("# Prompt\n"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := InjectSDDReference(nil, promptFile); err != nil {
		t.Fatalf("expected no error for nil agent, got: %v", err)
	}
}
