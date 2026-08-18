package executor

import (
	"context"
	"fmt"
	"sync"

	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/batch"
)

// AgentRunner is an interface for invoking an external agent (e.g. LLM CLI).
type AgentRunner interface {
	// Run executes an agent with the given context, batch context, and prompt.
	Run(ctx context.Context, b batch.ExecutionBatch, prompt string) error
}

// ConcurrentEngine orchestrates the concurrent execution of multiple batches.
type ConcurrentEngine struct {
	Runner     AgentRunner
	MaxWorkers int
}

// New creates a new ConcurrentEngine with the specified runner and concurrency limit.
func New(runner AgentRunner, maxWorkers int) *ConcurrentEngine {
	if maxWorkers <= 0 {
		maxWorkers = 1
	}
	return &ConcurrentEngine{
		Runner:     runner,
		MaxWorkers: maxWorkers,
	}
}

// ExecuteBatches runs the provided batches concurrently up to the MaxWorkers limit.
// It returns a map of RepoName (or a unique batch identifier) to an error if the batch failed.
func (e *ConcurrentEngine) ExecuteBatches(ctx context.Context, batches []batch.ExecutionBatch, prompts map[string]string) map[string]error {
	errorsMap := make(map[string]error)
	if len(batches) == 0 {
		return errorsMap
	}

	var mu sync.Mutex
	var wg sync.WaitGroup

	// Use a semaphore channel to limit concurrency.
	sem := make(chan struct{}, e.MaxWorkers)

	for _, b := range batches {
		if !b.Ready {
			continue
		}

		wg.Add(1)
		go func(currentBatch batch.ExecutionBatch) {
			defer wg.Done()

			// Acquire a token.
			sem <- struct{}{}
			defer func() { <-sem }() // Release the token.

			// Get the corresponding prompt for this batch.
			// The key in the map is assumed to be the RepoName.
			// If RepoName is empty, it means there's a single flat batch,
			// which should be handled smoothly if the map also uses an empty key.
			prompt := prompts[currentBatch.RepoName]

			err := e.Runner.Run(ctx, currentBatch, prompt)
			if err != nil {
				mu.Lock()
				errorsMap[currentBatch.RepoName] = fmt.Errorf("agent %s failed on repo %q: %w", currentBatch.AgentName, currentBatch.RepoName, err)
				mu.Unlock()
			}
		}(b)
	}

	wg.Wait()
	return errorsMap
}
