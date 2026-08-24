package executor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/batch"
)

// DefaultMaxAttempts and DefaultBaseBackoff are the bounded-retry defaults
// (H-09a) a caller may opt a ConcurrentEngine into; New leaves both at their
// zero value, preserving the pre-H-09a single-attempt behavior.
const (
	DefaultMaxAttempts = 3
	DefaultBaseBackoff = 200 * time.Millisecond
)

// AgentRunner is an interface for invoking an external agent (e.g. LLM CLI).
type AgentRunner interface {
	// Run executes an agent with the given context, batch context, and prompt.
	Run(ctx context.Context, b batch.ExecutionBatch, prompt string) error
}

// ConcurrentEngine orchestrates the concurrent execution of multiple batches.
//
// MaxAttempts/BaseBackoff/Sleep implement H-09a's bounded retry with
// backoff: a failing dispatch retries up to MaxAttempts times, sleeping
// BaseBackoff*2^(attempt-1) between attempts, before being recorded failed.
// MaxAttempts <= 0 keeps the pre-H-09a single-attempt behavior. Sleep
// defaults to time.Sleep but is injectable so tests stay fast. H-09b
// (circuit breaker, cross-repo rollback) is explicitly out of scope: each
// repo retries independently and a succeeded repo is never rolled back.
type ConcurrentEngine struct {
	Runner      AgentRunner
	MaxWorkers  int
	MaxAttempts int
	BaseBackoff time.Duration
	Sleep       func(time.Duration)
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

// ExecuteBatches runs the provided batches concurrently up to MaxWorkers,
// retrying each ready batch up to MaxAttempts times with backoff (H-09a)
// before recording it as failed. It returns two maps keyed by RepoName:
// errs holds the final error (absent means success), and attempts holds
// the real attempt count made, which never exceeds MaxAttempts.
func (e *ConcurrentEngine) ExecuteBatches(ctx context.Context, batches []batch.ExecutionBatch, prompts map[string]string) (map[string]error, map[string]int) {
	errorsMap := make(map[string]error)
	attemptsMap := make(map[string]int)
	if len(batches) == 0 {
		return errorsMap, attemptsMap
	}

	maxAttempts := e.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	sleep := e.Sleep
	if sleep == nil {
		sleep = time.Sleep
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

			var lastErr error
			attempt := 0
			for i := 1; i <= maxAttempts; i++ {
				attempt = i
				lastErr = e.Runner.Run(ctx, currentBatch, prompt)
				if lastErr == nil {
					break
				}
				if i < maxAttempts {
					sleep(e.BaseBackoff << (i - 1))
				}
			}

			mu.Lock()
			attemptsMap[currentBatch.RepoName] = attempt
			if lastErr != nil {
				errorsMap[currentBatch.RepoName] = fmt.Errorf("agent %s failed on repo %q after %d attempt(s): %w", currentBatch.AgentName, currentBatch.RepoName, attempt, lastErr)
			}
			mu.Unlock()
		}(b)
	}

	wg.Wait()
	return errorsMap, attemptsMap
}
