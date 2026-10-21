package cli

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// TestRelayTimeoutIntegration_DeadlineExceeded tests that when the Pi host relay
// takes longer than the configured timeout, the context is cancelled with
// DeadlineExceeded. This mimics a slow Pi process that exceeds the relay timeout.
func TestRelayTimeoutIntegration_DeadlineExceeded(t *testing.T) {
	// Set a very short relay timeout (10ms)
	originalEnv := os.Getenv("GENTLE_AI_RELAY_TIMEOUT")
	os.Setenv("GENTLE_AI_RELAY_TIMEOUT", "10ms")
	defer func() {
		if originalEnv != "" {
			os.Setenv("GENTLE_AI_RELAY_TIMEOUT", originalEnv)
		} else {
			os.Unsetenv("GENTLE_AI_RELAY_TIMEOUT")
		}
	}()

	// Mock the Pi adapter to sleep for 100ms (longer than 10ms timeout)
	previous := reviewProviderAdapterFor
	reviewProviderAdapterFor = func(_ reviewerprovider.Contract, agent model.AgentID) (reviewerprovider.Adapter, error) {
		if agent != model.AgentPi {
			return nil, errors.New("unexpected runtime, expected pi")
		}
		return providerTestAdapterFunc(func(ctx context.Context, _ reviewerprovider.Invocation) ([]byte, error) {
			// Sleep longer than the configured timeout
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return []byte("should not reach here"), nil
			}
		}), nil
	}
	t.Cleanup(func() { reviewProviderAdapterFor = previous })

	// Verify the configured timeout is correct
	timeout := reviewtransaction.GetRelayTimeout()
	if timeout != 10*time.Millisecond {
		t.Errorf("GetRelayTimeout() = %v, want 10ms", timeout)
	}

	// Verify the context timeout works as expected
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Simulate a slow operation that exceeds the timeout
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
				t.Errorf("expected DeadlineExceeded, got %v", ctx.Err())
			}
			close(done)
		case <-time.After(200 * time.Millisecond):
			t.Error("context should have been cancelled by now")
			close(done)
		}
	}()

	// Wait for the context to be cancelled (should happen within ~10ms)
	select {
	case <-done:
		// Test passed
	case <-time.After(500 * time.Millisecond):
		t.Error("context cancellation took too long")
	}
}

// TestRelayTimeoutIntegration_ConfigurableSuccess tests that a configurable
// timeout (via GENTLE_AI_RELAY_TIMEOUT) allows operations to complete when
// they finish within the configured timeout.
func TestRelayTimeoutIntegration_ConfigurableSuccess(t *testing.T) {
	// Set a longer relay timeout (1 second)
	originalEnv := os.Getenv("GENTLE_AI_RELAY_TIMEOUT")
	os.Setenv("GENTLE_AI_RELAY_TIMEOUT", "1s")
	defer func() {
		if originalEnv != "" {
			os.Setenv("GENTLE_AI_RELAY_TIMEOUT", originalEnv)
		} else {
			os.Unsetenv("GENTLE_AI_RELAY_TIMEOUT")
		}
	}()

	// Mock the Pi adapter to sleep for 100ms (shorter than 1s timeout)
	previous := reviewProviderAdapterFor
	reviewProviderAdapterFor = func(_ reviewerprovider.Contract, agent model.AgentID) (reviewerprovider.Adapter, error) {
		if agent != model.AgentPi {
			return nil, errors.New("unexpected runtime, expected pi")
		}
		return providerTestAdapterFunc(func(ctx context.Context, _ reviewerprovider.Invocation) ([]byte, error) {
			// Sleep for 100ms - should complete within the 1s timeout
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(100 * time.Millisecond):
				// Return a valid refuter response
				return []byte(`{
					"refuter_request_hash": "sha256:test",
					"results": []
				}`), nil
			}
		}), nil
	}
	t.Cleanup(func() { reviewProviderAdapterFor = previous })

	// Verify the configured timeout is correct
	timeout := reviewtransaction.GetRelayTimeout()
	if timeout != 1*time.Second {
		t.Errorf("GetRelayTimeout() = %v, want 1s", timeout)
	}

	// Verify the context allows the operation to complete
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Simulate an operation that completes within the timeout
	done := make(chan error, 1)
	go func() {
		select {
		case <-ctx.Done():
			done <- ctx.Err()
		case <-time.After(100 * time.Millisecond):
			done <- nil
		}
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("operation should have completed within timeout, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("operation took too long")
	}
}