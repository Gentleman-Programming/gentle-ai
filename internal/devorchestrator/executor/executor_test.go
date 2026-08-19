package executor

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/batch"
)

// MockRunner is a simple mock for testing the concurrent engine.
type MockRunner struct {
	mu            sync.Mutex
	CallCount     int
	Delay         time.Duration
	InjectedError error
}

func (m *MockRunner) Run(ctx context.Context, b batch.ExecutionBatch, prompt string) error {
	// Simulate work delay
	time.Sleep(m.Delay)

	m.mu.Lock()
	m.CallCount++
	m.mu.Unlock()

	// Return error for a specific repo to test error mapping,
	// or return the globally injected error.
	if b.RepoName == "fail-repo" {
		return errors.New("simulated failure")
	}

	return m.InjectedError
}

func TestConcurrentEngine_ExecuteBatches_Parallelism(t *testing.T) {
	runner := &MockRunner{
		Delay: 100 * time.Millisecond,
	}

	// 3 workers means 3 batches can run perfectly in parallel
	engine := New(runner, 3)

	batches := []batch.ExecutionBatch{
		{RepoName: "repo-1", AgentName: "agent-a", Ready: true},
		{RepoName: "repo-2", AgentName: "agent-a", Ready: true},
		{RepoName: "repo-3", AgentName: "agent-a", Ready: true},
	}
	prompts := map[string]string{
		"repo-1": "prompt-1",
		"repo-2": "prompt-2",
		"repo-3": "prompt-3",
	}

	start := time.Now()
	errs := engine.ExecuteBatches(context.Background(), batches, prompts)
	duration := time.Since(start)

	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}

	if runner.CallCount != 3 {
		t.Errorf("expected 3 calls, got %d", runner.CallCount)
	}

	// If it ran sequentially, it would take ~300ms.
	// Running concurrently, it should take ~100ms.
	// We allow some overhead margin.
	if duration >= 250*time.Millisecond {
		t.Errorf("execution was too slow (%v), expected it to run in parallel", duration)
	}
}

func TestConcurrentEngine_ExecuteBatches_LimitsConcurrency(t *testing.T) {
	runner := &MockRunner{
		Delay: 100 * time.Millisecond,
	}

	// 1 worker means it has to run sequentially
	engine := New(runner, 1)

	batches := []batch.ExecutionBatch{
		{RepoName: "repo-1", AgentName: "agent-a", Ready: true},
		{RepoName: "repo-2", AgentName: "agent-a", Ready: true},
	}
	prompts := map[string]string{
		"repo-1": "prompt-1",
		"repo-2": "prompt-2",
	}

	start := time.Now()
	engine.ExecuteBatches(context.Background(), batches, prompts)
	duration := time.Since(start)

	// Since maxWorkers is 1, it must take at least 200ms
	if duration < 200*time.Millisecond {
		t.Errorf("execution was too fast (%v), expected it to run sequentially", duration)
	}
}

func TestConcurrentEngine_ExecuteBatches_IgnoresNotReady(t *testing.T) {
	runner := &MockRunner{
		Delay: 10 * time.Millisecond,
	}

	engine := New(runner, 5)

	batches := []batch.ExecutionBatch{
		{RepoName: "repo-1", AgentName: "agent-a", Ready: true},
		{RepoName: "repo-2", AgentName: "agent-a", Ready: false}, // Should be ignored
		{RepoName: "repo-3", AgentName: "agent-a", Ready: true},
	}
	prompts := map[string]string{}

	errs := engine.ExecuteBatches(context.Background(), batches, prompts)

	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}

	if runner.CallCount != 2 {
		t.Errorf("expected 2 calls, got %d", runner.CallCount)
	}
}

func TestConcurrentEngine_ExecuteBatches_CollectsErrors(t *testing.T) {
	runner := &MockRunner{
		Delay: 10 * time.Millisecond,
	}

	engine := New(runner, 2)

	batches := []batch.ExecutionBatch{
		{RepoName: "good-repo", AgentName: "agent-a", Ready: true},
		{RepoName: "fail-repo", AgentName: "agent-a", Ready: true},
	}
	prompts := map[string]string{}

	errs := engine.ExecuteBatches(context.Background(), batches, prompts)

	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error, got %d", len(errs))
	}

	err, ok := errs["fail-repo"]
	if !ok {
		t.Fatalf("expected error for 'fail-repo', got none")
	}

	expectedPrefix := "agent agent-a failed on repo \"fail-repo\""
	if err.Error()[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("unexpected error message: %v", err)
	}
}
