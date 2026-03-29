package rtk

import (
	"fmt"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestConfigureAgentHook(t *testing.T) {
	// Save and restore originals
	origExecCommand := execCommand
	origRtkCommand := rtkCommand
	defer func() {
		execCommand = origExecCommand
		rtkCommand = origRtkCommand
	}()

	tests := []struct {
		name      string
		agentID   model.AgentID
		cmdFails  bool
		wantErr   bool
		skipCheck bool // true for antigravity (no-op)
	}{
		{
			name:    "claude-code succeeds",
			agentID: model.AgentClaudeCode,
			wantErr: false,
		},
		{
			name:    "opencode succeeds",
			agentID: model.AgentOpenCode,
			wantErr: false,
		},
		{
			name:      "antigravity is no-op",
			agentID:   model.AgentAntigravity,
			skipCheck: true,
			wantErr:   false,
		},
		{
			name:     "command failure returns error",
			agentID:  model.AgentClaudeCode,
			cmdFails: true,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipCheck {
				// For no-op cases, just verify it returns nil
				err := ConfigureAgentHook(tt.agentID)
				if err != nil {
					t.Errorf("ConfigureAgentHook(%q) error = %v, want nil", tt.agentID, err)
				}
				return
			}

			// Use a real command that will succeed (true) or fail (false)
			if tt.cmdFails {
				rtkCommand = "/nonexistent/rtk"
			} else {
				rtkCommand = "true" // Unix true command always succeeds
			}

			err := ConfigureAgentHook(tt.agentID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConfigureAgentHook(%q) error = %v, wantErr %v", tt.agentID, err, tt.wantErr)
			}
		})
	}
}

func TestConfigureAllHooks(t *testing.T) {
	origExecCommand := execCommand
	origRtkCommand := rtkCommand
	defer func() {
		execCommand = origExecCommand
		rtkCommand = origRtkCommand
	}()

	// Use "true" command so all hooks succeed
	rtkCommand = "true"

	agents := []model.AgentID{
		model.AgentClaudeCode,
		model.AgentOpenCode,
		model.AgentAntigravity,
		model.AgentCursor,
	}

	results := ConfigureAllHooks(agents)

	if len(results) != len(agents) {
		t.Fatalf("ConfigureAllHooks() returned %d results, want %d", len(results), len(agents))
	}

	// Claude Code, OpenCode, Cursor should succeed
	for i, r := range results[:2] {
		if !r.Success {
			t.Errorf("results[%d] (%s) Success = false, want true: %v", i, r.AgentID, r.Err)
		}
	}

	// Antigravity should fail (no support)
	if results[2].Success {
		t.Error("antigravity should not succeed (no hook support)")
	}
	if results[2].Err == nil {
		t.Error("antigravity should have an error")
	}

	// Cursor should succeed
	if !results[3].Success {
		t.Errorf("cursor should succeed: %v", results[3].Err)
	}
}

// Suppress unused import warning
var _ = fmt.Sprintf
