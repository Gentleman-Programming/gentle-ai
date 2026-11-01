package agentguidance_test

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/codex"
)

// TestMainPinsCodexRuntimeVersionProbe locks the TestMain default: rendered
// guidance must not depend on whether the host has a working codex binary.
func TestMainPinsCodexRuntimeVersionProbe(t *testing.T) {
	if err := codex.ValidateGPT56Runtime(); err != nil {
		t.Fatalf("TestMain must pin a supported Codex runtime version; ValidateGPT56Runtime() = %v", err)
	}
}
