package capabilities

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

type fakePiDetector struct {
	detected bool
	err      error
}

func TestResolverResolveFailsClosedWhenPiSettingsMalformed(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	settingsPath := filepath.Join(homeDir, ".pi", "agent", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(settingsPath), err)
	}

	if err := os.WriteFile(settingsPath, []byte(`{"packages":["npm:pi-subagents"`), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", settingsPath, err)
	}

	resolver := NewResolver(nil)

	got, err := resolver.Resolve(homeDir, workspaceDir, model.AgentPiCodingAgent)
	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}

	if got.SupportsSDDMultiMode {
		t.Fatal("Resolve().SupportsSDDMultiMode = true, want false")
	}

	if got.SupportsModelPicker {
		t.Fatal("Resolve().SupportsModelPicker = true, want false")
	}

	if got.SupportsGeneratedMulti {
		t.Fatal("Resolve().SupportsGeneratedMulti = true, want false")
	}

	if len(got.Requires) != 1 {
		t.Fatalf("Resolve().Requires length = %d, want 1", len(got.Requires))
	}

	if got.Requires[0].Message != PiMultiModelRequiresPiSubagentsMessage {
		t.Fatalf("Resolve().Requires[0].Message = %q, want %q", got.Requires[0].Message, PiMultiModelRequiresPiSubagentsMessage)
	}
}

func (f fakePiDetector) DetectPiSubagents(_ string, _ string) (bool, error) {
	return f.detected, f.err
}

func TestResolverResolve(t *testing.T) {
	tests := []struct {
		name                    string
		agent                   model.AgentID
		detector                fakePiDetector
		wantSupportsSDDMulti    bool
		wantSupportsModelPicker bool
		wantSupportsGenerated   bool
		wantRequirements        int
		wantErr                 bool
	}{
		{
			name:                    "pi with pi-subagents detected enables multi-model capabilities",
			agent:                   model.AgentPiCodingAgent,
			detector:                fakePiDetector{detected: true},
			wantSupportsSDDMulti:    true,
			wantSupportsModelPicker: true,
			wantSupportsGenerated:   true,
			wantRequirements:        0,
		},
		{
			name:                    "pi without pi-subagents stays single-mode with requirement",
			agent:                   model.AgentPiCodingAgent,
			detector:                fakePiDetector{detected: false},
			wantSupportsSDDMulti:    false,
			wantSupportsModelPicker: false,
			wantSupportsGenerated:   false,
			wantRequirements:        1,
		},
		{
			name:                    "pi detector error fails closed and returns requirement",
			agent:                   model.AgentPiCodingAgent,
			detector:                fakePiDetector{err: errors.New("permission denied")},
			wantSupportsSDDMulti:    false,
			wantSupportsModelPicker: false,
			wantSupportsGenerated:   false,
			wantRequirements:        1,
		},
		{
			name:                    "opencode remains multi-model capable",
			agent:                   model.AgentOpenCode,
			detector:                fakePiDetector{detected: false},
			wantSupportsSDDMulti:    true,
			wantSupportsModelPicker: true,
			wantSupportsGenerated:   true,
			wantRequirements:        0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := NewResolver(tt.detector)

			got, err := resolver.Resolve("/tmp/home", "/tmp/workspace", tt.agent)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Resolve() error = %v, wantErr %v", err, tt.wantErr)
			}

			if got.SupportsSDDMultiMode != tt.wantSupportsSDDMulti {
				t.Fatalf("Resolve().SupportsSDDMultiMode = %v, want %v", got.SupportsSDDMultiMode, tt.wantSupportsSDDMulti)
			}

			if got.SupportsModelPicker != tt.wantSupportsModelPicker {
				t.Fatalf("Resolve().SupportsModelPicker = %v, want %v", got.SupportsModelPicker, tt.wantSupportsModelPicker)
			}

			if got.SupportsGeneratedMulti != tt.wantSupportsGenerated {
				t.Fatalf("Resolve().SupportsGeneratedMulti = %v, want %v", got.SupportsGeneratedMulti, tt.wantSupportsGenerated)
			}

			if len(got.Requires) != tt.wantRequirements {
				t.Fatalf("Resolve().Requires length = %d, want %d", len(got.Requires), tt.wantRequirements)
			}

			if tt.wantRequirements > 0 {
				if got.Requires[0].Message != PiMultiModelRequiresPiSubagentsMessage {
					t.Fatalf("Resolve().Requires[0].Message = %q, want %q", got.Requires[0].Message, PiMultiModelRequiresPiSubagentsMessage)
				}
			}
		})
	}
}
