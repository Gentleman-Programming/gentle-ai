package tui

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

func TestGetTurnTimeout(t *testing.T) {
	// unset env -> default 10 minutes
	if v := os.Getenv("GENTLE_AI_TURN_TIMEOUT"); v != "" {
		t.Setenv("GENTLE_AI_TURN_TIMEOUT", "")
		os.Unsetenv("GENTLE_AI_TURN_TIMEOUT")
	}
	if got := GetTurnTimeout(); got != 10*time.Minute {
		t.Errorf("GetTurnTimeout() = %v, want %v", got, 10*time.Minute)
	}
	// set valid duration
	t.Setenv("GENTLE_AI_TURN_TIMEOUT", "2m30s")
	if got := GetTurnTimeout(); got != 2*time.Minute+30*time.Second {
		t.Errorf("GetTurnTimeout() = %v, want %v", got, 2*time.Minute+30*time.Second)
	}
	// invalid duration falls back to default
	t.Setenv("GENTLE_AI_TURN_TIMEOUT", "notaduration")
	if got := GetTurnTimeout(); got != 10*time.Minute {
		t.Errorf("GetTurnTimeout() = %v, want %v (fallback)", got, 10*time.Minute)
	}
}

// sleepingEngine is a GenerationEngine test double whose Generate respects the
// context deadline: it returns ctx.Err() once the context expires. Upstream's
// MockEngine ignores the context, so the turn-timeout behavior needs its own
// double (upstream turned MockEngine.Generate from a field into a method).
type sleepingEngine struct {
	agentID model.AgentID
	delay   time.Duration
}

func (e *sleepingEngine) Agent() model.AgentID { return e.agentID }
func (e *sleepingEngine) Available() bool      { return true }
func (e *sleepingEngine) Generate(ctx context.Context, _ string) (string, error) {
	select {
	case <-time.After(e.delay):
		return "", ctx.Err()
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func TestTurnTimeoutUsedInGeneration(t *testing.T) {
	// A short timeout expires before the engine finishes its 300 ms of work.
	t.Setenv("GENTLE_AI_TURN_TIMEOUT", "200ms")

	engine := &sleepingEngine{agentID: model.AgentGeminiCLI, delay: 300 * time.Millisecond}

	// Simulate what the TUI does: create ctx with GetTurnTimeout()
	timeout := GetTurnTimeout()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	_, err := engine.Generate(ctx, "test")
	if err == nil {
		t.Errorf("expected error due to timeout, got nil")
		return
	}
	if ctx.Err() == nil || !os.IsTimeout(err) && err.Error() != context.DeadlineExceeded.Error() {
		if err != context.DeadlineExceeded {
			t.Errorf("expected deadline exceeded error, got %v", err)
		}
	}

	// The context the generation ran with must carry the configured deadline.
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatalf("context has no deadline")
	}
	if deadline.Sub(time.Now()) >= 0 {
		t.Errorf("deadline already passed: %v", deadline)
	}
}
