package state

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestWriteAndRead writes state and reads it back, verifying agents match.
func TestWriteAndRead(t *testing.T) {
	home := t.TempDir()
	agents := []string{"claude-code", "opencode"}

	if err := Write(home, InstallState{InstalledAgents: agents}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	s, err := Read(home)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if !reflect.DeepEqual(s.InstalledAgents, agents) {
		t.Errorf("InstalledAgents = %v, want %v", s.InstalledAgents, agents)
	}
}

// TestWriteCreatesStateDir verifies that Write creates the .gentle-ai directory
// when it does not exist yet.
func TestWriteCreatesStateDir(t *testing.T) {
	home := t.TempDir()

	if err := Write(home, InstallState{InstalledAgents: []string{"opencode"}}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(home, stateDir)); err != nil {
		t.Errorf("Write() did not create %q: %v", stateDir, err)
	}
}

// TestWriteStateFilePath verifies Path() returns the expected location.
func TestWriteStateFilePath(t *testing.T) {
	home := t.TempDir()
	got := Path(home)
	want := filepath.Join(home, ".gentle-ai", "state.json")
	if got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

// TestReadMissing verifies that reading a non-existent file returns an error (not a panic).
func TestReadMissing(t *testing.T) {
	home := t.TempDir()
	// No Write — state.json does not exist.

	_, err := Read(home)
	if err == nil {
		t.Fatalf("Read() expected error for missing file, got nil")
	}

	if !os.IsNotExist(err) {
		t.Logf("Read() error = %v (non-nil, as expected — OS-level may differ)", err)
	}
}

// TestReadCorrupt verifies that writing garbage produces an error on read.
func TestReadCorrupt(t *testing.T) {
	home := t.TempDir()

	// Create the directory and write garbage JSON.
	if err := os.MkdirAll(filepath.Join(home, stateDir), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(Path(home), []byte("not valid json {{{{"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Read(home)
	if err == nil {
		t.Fatalf("Read() expected error for corrupt JSON, got nil")
	}
}

// TestWriteOverwrite verifies that a second Write call replaces the previous state.
func TestWriteOverwrite(t *testing.T) {
	home := t.TempDir()

	if err := Write(home, InstallState{InstalledAgents: []string{"claude-code"}}); err != nil {
		t.Fatalf("Write() first error = %v", err)
	}

	if err := Write(home, InstallState{InstalledAgents: []string{"opencode", "gemini-cli"}}); err != nil {
		t.Fatalf("Write() second error = %v", err)
	}

	s, err := Read(home)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	want := []string{"opencode", "gemini-cli"}
	if !reflect.DeepEqual(s.InstalledAgents, want) {
		t.Errorf("InstalledAgents after overwrite = %v, want %v", s.InstalledAgents, want)
	}
}

// TestWriteEmptyAgents verifies that an empty agent list round-trips correctly.
func TestWriteEmptyAgents(t *testing.T) {
	home := t.TempDir()

	if err := Write(home, InstallState{InstalledAgents: []string{}}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	s, err := Read(home)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	// An empty slice round-trips as an empty slice (not nil).
	if len(s.InstalledAgents) != 0 {
		t.Errorf("InstalledAgents = %v, want empty", s.InstalledAgents)
	}
}

// TestModelAssignmentsRoundTrip verifies that model assignments survive a write/read cycle.
func TestModelAssignmentsRoundTrip(t *testing.T) {
	home := t.TempDir()

	want := InstallState{
		InstalledAgents: []string{"claude-code"},
		ClaudeModelAssignments: map[string]string{
			"orchestrator": "opus",
			"sdd-explore":  "sonnet",
			"sdd-archive":  "haiku",
		},
		KiroModelAssignments: map[string]string{
			"sdd-design":  "opus",
			"sdd-archive": "haiku",
			"default":     "sonnet",
		},
		ModelAssignments: map[string]ModelAssignmentState{
			"sdd-init": {ProviderID: "anthropic", ModelID: "claude-sonnet-4"},
		},
	}

	if err := Write(home, want); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got, err := Read(home)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if !reflect.DeepEqual(got.ClaudeModelAssignments, want.ClaudeModelAssignments) {
		t.Errorf("ClaudeModelAssignments = %v, want %v", got.ClaudeModelAssignments, want.ClaudeModelAssignments)
	}
	if !reflect.DeepEqual(got.KiroModelAssignments, want.KiroModelAssignments) {
		t.Errorf("KiroModelAssignments = %v, want %v", got.KiroModelAssignments, want.KiroModelAssignments)
	}
	if !reflect.DeepEqual(got.ModelAssignments, want.ModelAssignments) {
		t.Errorf("ModelAssignments = %v, want %v", got.ModelAssignments, want.ModelAssignments)
	}
}

// TestBackwardCompatNoAssignments verifies that a state.json written before
// model assignment support was added still reads correctly (fields are nil).
func TestBackwardCompatNoAssignments(t *testing.T) {
	home := t.TempDir()

	// Simulate a legacy state file with only installed_agents.
	if err := os.MkdirAll(filepath.Join(home, stateDir), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	legacy := []byte(`{"installed_agents":["claude-code"]}` + "\n")
	if err := os.WriteFile(Path(home), legacy, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s, err := Read(home)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if !reflect.DeepEqual(s.InstalledAgents, []string{"claude-code"}) {
		t.Errorf("InstalledAgents = %v, want [claude-code]", s.InstalledAgents)
	}
	if s.ClaudeModelAssignments != nil {
		t.Errorf("ClaudeModelAssignments = %v, want nil", s.ClaudeModelAssignments)
	}
	if s.ModelAssignments != nil {
		t.Errorf("ModelAssignments = %v, want nil", s.ModelAssignments)
	}
}
