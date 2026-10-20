package tui

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agentbuilder"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

func TestTurnTimeoutUsedInGeneration(t *testing.T) {
	// Set a short timeout
	os.Setenv("GENTLE_AI_TURN_TIMEOUT", "200ms")
	defer os.Unsetenv("GENTLE_AI_TURN_TIMEOUT")

	// Create a mock engine that sleeps longer than timeout
	mockEngine := &agentbuilder.MockEngine{
		AgentIDVal:  model.AgentGeminiCLI,
		IsAvailable: true,
		Output:      "ok",
		Err:         nil,
	}
	mockEngine.Generate = func(ctx context.Context, prompt string) (string, error) {
		// sleep 300ms > timeout
		time.Sleep(300 * time.Millisecond)
		return "ok", nil
	}

	// Simulate what the TUI does: create ctx with GetTurnTimeout()
	timeout := GetTurnTimeout()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Call the mock engine's Generate
	_, err := mockEngine.Generate(ctx, "test")
	// Expect context deadline exceeded
	if err == nil {
		t.Errorf("expected error due to timeout, got nil")
	} else if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected deadline exceeded error, got %v", err)
	}

	// Verify that the deadline in ctx is roughly the timeout we set
	if !ctx.Deadline().IsZero() {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatalf("deadline not ok")
		}
		// deadline should be in the future
		if deadline.Sub(time.Now()) < 0 {
			t.Errorf("deadline already passed: %v", deadline)
		}
	} else {
		t.Errorf("context has no deadline")
	}
}
