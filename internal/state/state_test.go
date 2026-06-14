package state

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// TestMergeAgents verifies that MergeAgents appends new agents to existing
// installed_agents with deduplication and preserves all other fields.
func TestMergeAgents(t *testing.T) {
	existingAssignments := map[string]ModelAssignmentState{
		"sdd-init": {ProviderID: "anthropic", ModelID: "claude-sonnet-4"},
	}
	existingClaude := map[string]string{"sdd-explore": "sonnet", "sdd-archive": "haiku"}
	existingKiro := map[string]string{"sdd-design": "opus"}

	tests := []struct {
		name      string
		existing  InstallState
		newAgents []string
		wantIDs   []string
	}{
		{
			name:      "empty existing state gets new agent",
			existing:  InstallState{},
			newAgents: []string{"pi"},
			wantIDs:   []string{"pi"},
		},
		{
			name:      "existing single agent plus new agent appended",
			existing:  InstallState{InstalledAgents: []string{"claude-code"}},
			newAgents: []string{"opencode"},
			wantIDs:   []string{"claude-code", "opencode"},
		},
		{
			name:      "duplicate agent is deduped",
			existing:  InstallState{InstalledAgents: []string{"opencode", "vscode-copilot", "codex"}},
			newAgents: []string{"opencode"},
			wantIDs:   []string{"opencode", "vscode-copilot", "codex"},
		},
		{
			name:      "existing multiple agents plus new agent appended",
			existing:  InstallState{InstalledAgents: []string{"opencode", "vscode-copilot", "codex"}},
			newAgents: []string{"pi"},
			wantIDs:   []string{"opencode", "vscode-copilot", "codex", "pi"},
		},
		{
			name: "model_assignments preserved across merge",
			existing: InstallState{
				InstalledAgents:        []string{"opencode"},
				ModelAssignments:       existingAssignments,
				ClaudeModelAssignments: existingClaude,
				KiroModelAssignments:   existingKiro,
				Persona:                "gentleman",
			},
			newAgents: []string{"pi"},
			wantIDs:   []string{"opencode", "pi"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeAgents(tt.existing, tt.newAgents)

			if !reflect.DeepEqual(got.InstalledAgents, tt.wantIDs) {
				t.Errorf("InstalledAgents = %v, want %v", got.InstalledAgents, tt.wantIDs)
			}

			// Verify all preserved fields are unchanged.
			if !reflect.DeepEqual(got.ModelAssignments, tt.existing.ModelAssignments) {
				t.Errorf("ModelAssignments not preserved: got %v, want %v", got.ModelAssignments, tt.existing.ModelAssignments)
			}
			if !reflect.DeepEqual(got.ClaudeModelAssignments, tt.existing.ClaudeModelAssignments) {
				t.Errorf("ClaudeModelAssignments not preserved: got %v, want %v", got.ClaudeModelAssignments, tt.existing.ClaudeModelAssignments)
			}
			if !reflect.DeepEqual(got.KiroModelAssignments, tt.existing.KiroModelAssignments) {
				t.Errorf("KiroModelAssignments not preserved: got %v, want %v", got.KiroModelAssignments, tt.existing.KiroModelAssignments)
			}
			if got.Persona != tt.existing.Persona {
				t.Errorf("Persona not preserved: got %q, want %q", got.Persona, tt.existing.Persona)
			}
		})
	}
}

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

// TestPersonaRoundTrip verifies the Persona field round-trips through
// Write/Read. Both `gentle-ai install` (CLI in run.go) and the TUI app
// (internal/app/app.go) write this field after a successful install so that
// `gentle-ai sync` regenerates the persona the user actually selected — not a
// hard-coded default.
func TestPersonaRoundTrip(t *testing.T) {
	for _, persona := range []string{"gentleman", "neutral", "custom"} {
		t.Run(persona, func(t *testing.T) {
			home := t.TempDir()
			if err := Write(home, InstallState{
				InstalledAgents: []string{"claude-code"},
				Persona:         persona,
			}); err != nil {
				t.Fatalf("Write() error = %v", err)
			}
			s, err := Read(home)
			if err != nil {
				t.Fatalf("Read() error = %v", err)
			}
			if s.Persona != persona {
				t.Errorf("Persona = %q, want %q", s.Persona, persona)
			}
		})
	}
}

// TestPersonaBackwardCompat verifies that state files written before persona
// persistence (no `persona` JSON field) still read cleanly with an empty
// Persona, allowing the sync fallback to take over.
func TestPersonaBackwardCompat(t *testing.T) {
	home := t.TempDir()
	if err := Write(home, InstallState{InstalledAgents: []string{"claude-code"}}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	s, err := Read(home)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if s.Persona != "" {
		t.Errorf("Persona = %q, want empty for pre-feature state", s.Persona)
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
			"sdd-propose":  "fable",
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

func TestClaudePhaseAssignmentsRoundTrip(t *testing.T) {
	home := t.TempDir()

	want := InstallState{
		InstalledAgents: []string{"claude-code"},
		ClaudePhaseAssignments: map[string]ClaudePhaseAssignmentState{
			"sdd-apply":   {Model: "sonnet", Effort: "max"},
			"sdd-archive": {Model: "haiku"},
		},
	}

	if err := Write(home, want); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got, err := Read(home)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if !reflect.DeepEqual(got.ClaudePhaseAssignments, want.ClaudePhaseAssignments) {
		t.Errorf("ClaudePhaseAssignments = %v, want %v", got.ClaudePhaseAssignments, want.ClaudePhaseAssignments)
	}
}

// TestModelAssignmentStateEffortRoundTrip verifies that Effort field survives
// a JSON serialization round-trip.
func TestModelAssignmentStateEffortRoundTrip(t *testing.T) {
	home := t.TempDir()

	want := InstallState{
		ModelAssignments: map[string]ModelAssignmentState{
			"sdd-apply": {ProviderID: "anthropic", ModelID: "claude-opus-4", Effort: "high"},
		},
	}

	if err := Write(home, want); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got, err := Read(home)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	a := got.ModelAssignments["sdd-apply"]
	if a.Effort != "high" {
		t.Errorf("Effort after round-trip = %q, want %q", a.Effort, "high")
	}
}

// TestModelAssignmentStateEffortLegacyMissing verifies that a state.json file
// with no "effort" key in a phase assignment deserializes to Effort="" with no error.
func TestModelAssignmentStateEffortLegacyMissing(t *testing.T) {
	home := t.TempDir()

	if err := os.MkdirAll(filepath.Join(home, stateDir), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	// Legacy format: no effort field
	legacy := `{"installed_agents":["opencode"],"model_assignments":{"sdd-apply":{"provider_id":"anthropic","model_id":"claude-opus-4"}}}` + "\n"
	if err := os.WriteFile(Path(home), []byte(legacy), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s, err := Read(home)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	a := s.ModelAssignments["sdd-apply"]
	if a.Effort != "" {
		t.Errorf("legacy missing effort field: got %q, want empty string", a.Effort)
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

// TestInstallStateCodexRoundTrip verifies that CodexModelAssignments persists
// with the "codexModelAssignments" JSON key and is omitted when empty.
func TestInstallStateCodexRoundTrip(t *testing.T) {
	home := t.TempDir()

	assignments := map[string]string{
		"sdd-apply":   "high",
		"sdd-explore": "low",
		"default":     "medium",
	}

	s := InstallState{
		InstalledAgents:       []string{"codex"},
		CodexModelAssignments: assignments,
	}

	if err := Write(home, s); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got, err := Read(home)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if !reflect.DeepEqual(got.CodexModelAssignments, assignments) {
		t.Errorf("CodexModelAssignments = %v, want %v", got.CodexModelAssignments, assignments)
	}
}

// TestInstallStateCodexOmitEmpty verifies that CodexModelAssignments is omitted
// from the JSON when empty.
func TestInstallStateCodexOmitEmpty(t *testing.T) {
	home := t.TempDir()

	s := InstallState{InstalledAgents: []string{"codex"}}
	if err := Write(home, s); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	data, err := os.ReadFile(Path(home))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if contains(string(data), "codexModelAssignments") {
		t.Error("JSON must not contain codexModelAssignments when empty")
	}
}

// TestInstallStateCodexMissingKeyReadback verifies that a state file without
// codexModelAssignments reads back as nil (forward-compat / absent key).
func TestInstallStateCodexMissingKeyReadback(t *testing.T) {
	home := t.TempDir()

	if err := os.MkdirAll(filepath.Join(home, stateDir), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	legacy := []byte(`{"installed_agents":["codex"]}` + "\n")
	if err := os.WriteFile(Path(home), legacy, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s, err := Read(home)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if s.CodexModelAssignments != nil {
		t.Errorf("CodexModelAssignments = %v, want nil (forward-compat)", s.CodexModelAssignments)
	}
}

// ─── WU-1 RED: CodexCarrilModelAssignments round-trip and backward-compat ────

// TestCodexCarrilModelAssignments_RoundTrip verifies the new carril→model key
// serialises with the JSON key "codexCarrilModelAssignments" and reads back.
func TestCodexCarrilModelAssignments_RoundTrip(t *testing.T) {
	home := t.TempDir()

	carrilMap := map[string]string{
		"sdd-strong": "gpt-5.5",
		"sdd-mid":    "gpt-5.5",
		"sdd-cheap":  "gpt-5.4-mini",
	}
	s := InstallState{
		InstalledAgents:             []string{"codex"},
		CodexCarrilModelAssignments: carrilMap,
	}

	if err := Write(home, s); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got, err := Read(home)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if !reflect.DeepEqual(got.CodexCarrilModelAssignments, carrilMap) {
		t.Errorf("CodexCarrilModelAssignments = %v, want %v", got.CodexCarrilModelAssignments, carrilMap)
	}

	// JSON key must be "codexCarrilModelAssignments"
	data, err := os.ReadFile(Path(home))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !contains(string(data), "codexCarrilModelAssignments") {
		t.Errorf("JSON must contain key codexCarrilModelAssignments; got:\n%s", data)
	}
}

// TestCodexCarrilModelAssignments_BackwardCompat verifies that a state blob
// without the new key still reads cleanly (field is nil or empty).
func TestCodexCarrilModelAssignments_BackwardCompat(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, stateDir), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	// Legacy blob has codexModelAssignments but no codexCarrilModelAssignments.
	legacy := `{"installed_agents":["codex"],"codexModelAssignments":{"sdd-apply":"high"}}` + "\n"
	if err := os.WriteFile(Path(home), []byte(legacy), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s, err := Read(home)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if s.CodexCarrilModelAssignments != nil {
		t.Errorf("CodexCarrilModelAssignments = %v, want nil for legacy state", s.CodexCarrilModelAssignments)
	}
	// Original key must still be there.
	if s.CodexModelAssignments["sdd-apply"] != "high" {
		t.Errorf("CodexModelAssignments[sdd-apply] = %q, want high", s.CodexModelAssignments["sdd-apply"])
	}
}

// TestCodexCarrilModelAssignments_OmitWhenEmpty verifies that the new key is
// absent from the JSON when the map is nil/empty (omitempty).
func TestCodexCarrilModelAssignments_OmitWhenEmpty(t *testing.T) {
	home := t.TempDir()
	s := InstallState{InstalledAgents: []string{"codex"}}
	if err := Write(home, s); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	data, err := os.ReadFile(Path(home))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if contains(string(data), "codexCarrilModelAssignments") {
		t.Error("JSON must not contain codexCarrilModelAssignments when empty")
	}
}

// TestMergeAgents_PreservesCodexCarrilAssignments verifies that MergeAgents
// preserves CodexCarrilModelAssignments from the existing state.
func TestMergeAgents_PreservesCodexCarrilAssignments(t *testing.T) {
	existing := InstallState{
		InstalledAgents: []string{"codex"},
		CodexCarrilModelAssignments: map[string]string{
			"sdd-strong": "gpt-5.5",
			"sdd-cheap":  "gpt-5.4-mini",
		},
	}
	merged := MergeAgents(existing, []string{"opencode"})
	if !reflect.DeepEqual(merged.CodexCarrilModelAssignments, existing.CodexCarrilModelAssignments) {
		t.Errorf("MergeAgents did not preserve CodexCarrilModelAssignments: got %v, want %v",
			merged.CodexCarrilModelAssignments, existing.CodexCarrilModelAssignments)
	}
}

// ─── WU-2 RED: CodexPhaseModelAssignments round-trip, omitempty, MergeAgents ──

// TestCodexPhaseModelAssignments_RoundTrip verifies the new field round-trips
// through Write/Read with JSON key "codexPhaseModelAssignments".
func TestCodexPhaseModelAssignments_RoundTrip(t *testing.T) {
	home := t.TempDir()

	assignments := map[string]string{
		"sdd-propose": "gpt-5.5",
		"sdd-apply":   "gpt-5.4",
		"sdd-explore": "gpt-5.4-mini",
	}
	s := InstallState{
		InstalledAgents:            []string{"codex"},
		CodexPhaseModelAssignments: assignments,
	}

	if err := Write(home, s); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got, err := Read(home)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if !reflect.DeepEqual(got.CodexPhaseModelAssignments, assignments) {
		t.Errorf("CodexPhaseModelAssignments = %v, want %v", got.CodexPhaseModelAssignments, assignments)
	}

	// JSON key must be "codexPhaseModelAssignments"
	data, err := os.ReadFile(Path(home))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !contains(string(data), "codexPhaseModelAssignments") {
		t.Errorf("JSON must contain key codexPhaseModelAssignments; got:\n%s", data)
	}
}

// TestCodexPhaseModelAssignments_OmitEmpty verifies the field is absent from JSON
// when nil/empty (omitempty).
func TestCodexPhaseModelAssignments_OmitEmpty(t *testing.T) {
	home := t.TempDir()
	s := InstallState{InstalledAgents: []string{"codex"}}
	if err := Write(home, s); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	data, err := os.ReadFile(Path(home))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if contains(string(data), "codexPhaseModelAssignments") {
		t.Error("JSON must not contain codexPhaseModelAssignments when empty")
	}
}

// TestCodexPhaseModelAssignments_LegacyAbsent verifies that old state files
// without the key read back with nil CodexPhaseModelAssignments.
func TestCodexPhaseModelAssignments_LegacyAbsent(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, stateDir), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	legacy := `{"installed_agents":["codex"],"codexModelAssignments":{"sdd-apply":"high"}}` + "\n"
	if err := os.WriteFile(Path(home), []byte(legacy), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	s, err := Read(home)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if s.CodexPhaseModelAssignments != nil {
		t.Errorf("CodexPhaseModelAssignments = %v, want nil for legacy state", s.CodexPhaseModelAssignments)
	}
}

// TestMergeAgents_PreservesCodexPhaseModelAssignments verifies that MergeAgents
// carries the field from the existing state into the merged result.
func TestMergeAgents_PreservesCodexPhaseModelAssignments(t *testing.T) {
	existing := InstallState{
		InstalledAgents: []string{"codex"},
		CodexPhaseModelAssignments: map[string]string{
			"sdd-propose": "gpt-5.5",
			"sdd-explore": "gpt-5.4-mini",
		},
	}
	merged := MergeAgents(existing, []string{"opencode"})
	if !reflect.DeepEqual(merged.CodexPhaseModelAssignments, existing.CodexPhaseModelAssignments) {
		t.Errorf("MergeAgents did not preserve CodexPhaseModelAssignments: got %v, want %v",
			merged.CodexPhaseModelAssignments, existing.CodexPhaseModelAssignments)
	}
}

// ─── Slice 2 RED: LastUpdateCheck round-trip and backward-compat ─────────────

// TestLastUpdateCheck_RoundTrip verifies that LastUpdateCheck survives a
// Write/Read cycle with the expected JSON key "last_update_check".
func TestLastUpdateCheck_RoundTrip(t *testing.T) {
	home := t.TempDir()

	ts := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	s := InstallState{
		InstalledAgents: []string{"claude-code"},
		LastUpdateCheck: &ts,
	}

	if err := Write(home, s); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got, err := Read(home)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if got.LastUpdateCheck == nil || !got.LastUpdateCheck.Equal(ts) {
		t.Errorf("LastUpdateCheck = %v, want %v", got.LastUpdateCheck, ts)
	}

	// JSON must contain the expected key.
	data, err := os.ReadFile(Path(home))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !contains(string(data), "last_update_check") {
		t.Errorf("JSON must contain key last_update_check; got:\n%s", data)
	}
}

// TestLastUpdateCheck_OmitWhenZero verifies that a zero-value LastUpdateCheck
// is omitted from JSON (omitempty), so old binaries that wrote state without
// the field still produce clean JSON.
func TestLastUpdateCheck_OmitWhenZero(t *testing.T) {
	home := t.TempDir()
	s := InstallState{InstalledAgents: []string{"claude-code"}}
	if err := Write(home, s); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	data, err := os.ReadFile(Path(home))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if contains(string(data), "last_update_check") {
		t.Error("JSON must not contain last_update_check when zero")
	}
}

// TestLastUpdateCheck_BackwardCompat verifies that state.json files written
// before the LastUpdateCheck field was added still read without error, with a
// nil LastUpdateCheck (never checked = always-check behavior).
func TestLastUpdateCheck_BackwardCompat(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, stateDir), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	legacy := `{"installed_agents":["claude-code"]}` + "\n"
	if err := os.WriteFile(Path(home), []byte(legacy), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s, err := Read(home)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if s.LastUpdateCheck != nil {
		t.Errorf("LastUpdateCheck = %v, want nil for legacy state", s.LastUpdateCheck)
	}
}

// TestMergeAgents_PreservesLastUpdateCheck verifies MergeAgents carries
// LastUpdateCheck from the existing state into the merged result.
func TestMergeAgents_PreservesLastUpdateCheck(t *testing.T) {
	ts := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	existing := InstallState{
		InstalledAgents: []string{"claude-code"},
		LastUpdateCheck: &ts,
	}
	merged := MergeAgents(existing, []string{"opencode"})
	if merged.LastUpdateCheck == nil || !merged.LastUpdateCheck.Equal(ts) {
		t.Errorf("MergeAgents did not preserve LastUpdateCheck: got %v, want %v",
			merged.LastUpdateCheck, ts)
	}
}

// ─── Slice 4 RED: PendingSync round-trip and backward-compat ────────────────

// TestPendingSync_RoundTrip verifies that PendingSync bool survives a
// Write/Read cycle with the expected JSON key "pending_sync".
func TestPendingSync_RoundTrip(t *testing.T) {
	home := t.TempDir()

	s := InstallState{
		InstalledAgents: []string{"claude-code"},
		PendingSync:     true,
	}

	if err := Write(home, s); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got, err := Read(home)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if !got.PendingSync {
		t.Errorf("PendingSync = false after round-trip, want true")
	}

	// JSON must contain the expected key.
	data, err := os.ReadFile(Path(home))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !contains(string(data), "pending_sync") {
		t.Errorf("JSON must contain key pending_sync; got:\n%s", data)
	}
}

// TestPendingSync_OmitWhenFalse verifies that PendingSync=false (zero value)
// is omitted from JSON (omitempty), preserving backward-compatibility with
// existing state files that lack the field.
func TestPendingSync_OmitWhenFalse(t *testing.T) {
	home := t.TempDir()
	s := InstallState{InstalledAgents: []string{"claude-code"}}
	if err := Write(home, s); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	data, err := os.ReadFile(Path(home))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if contains(string(data), "pending_sync") {
		t.Error("JSON must not contain pending_sync when false")
	}
}

// TestPendingSync_BackwardCompat verifies that state.json files written before
// PendingSync was added still read cleanly with PendingSync=false (safe default:
// no deferred sync pending).
func TestPendingSync_BackwardCompat(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, stateDir), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	legacy := `{"installed_agents":["claude-code"]}` + "\n"
	if err := os.WriteFile(Path(home), []byte(legacy), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s, err := Read(home)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if s.PendingSync {
		t.Errorf("PendingSync = true for legacy state, want false (no deferred sync)")
	}
}

// TestMergeAgents_PreservesPendingSync verifies MergeAgents carries
// PendingSync from the existing state into the merged result.
func TestMergeAgents_PreservesPendingSync(t *testing.T) {
	existing := InstallState{
		InstalledAgents: []string{"claude-code"},
		PendingSync:     true,
	}
	merged := MergeAgents(existing, []string{"opencode"})
	if !merged.PendingSync {
		t.Errorf("MergeAgents did not preserve PendingSync: got false, want true")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSub(s, substr)
}

func findSub(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
