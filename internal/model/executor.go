package model

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// FallbackReason categorizes why a model triggered fallback.
type FallbackReason string

const (
	// ReasonRateLimit indicates HTTP 429 - Too Many Requests.
	ReasonRateLimit FallbackReason = "rate_limit"
	// ReasonServiceDown indicates HTTP 5xx - Server Error.
	ReasonServiceDown FallbackReason = "service_down"
	// ReasonTimeout indicates the request exceeded the 30s timeout.
	ReasonTimeout FallbackReason = "timeout"
	// ReasonGatewayError indicates HTTP 502/504 - Gateway errors.
	ReasonGatewayError FallbackReason = "gateway_error"
	// ReasonUnknown indicates an unexpected error type.
	ReasonUnknown FallbackReason = "unknown"
)

// FallbackDecision indicates whether a fallback should occur.
type FallbackDecision int

const (
	// FallbackYes means error is retryable, should try next model.
	FallbackYes FallbackDecision = iota
	// FallbackNo means error is fatal, should stop immediately.
	FallbackNo
)

// ModelAttempt records a single attempt to use a model.
type ModelAttempt struct {
	// Model identifies which model was tried.
	Model ModelReference `json:"model"`
	// Reason explains why the attempt failed (empty if success).
	Reason FallbackReason `json:"reason,omitempty"`
	// Error contains the original error for debugging.
	Error string `json:"error,omitempty"`
}

// AggregatedError is returned when all models in a pool have been exhausted.
// It contains the history of all attempted models and why they failed.
type AggregatedError struct {
	// Phase is the SDD phase that triggered the execution.
	Phase string `json:"phase"`
	// Attempts records all model attempts in order.
	Attempts []ModelAttempt `json:"attempts"`
}

// Error implements the error interface.
func (e *AggregatedError) Error() string {
	if len(e.Attempts) == 0 {
		return fmt.Sprintf("all models failed for phase %q (no attempts recorded)", e.Phase)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("all models failed for phase %q: ", e.Phase))

	for i, attempt := range e.Attempts {
		if i > 0 {
			sb.WriteString("; ")
		}
		sb.WriteString(fmt.Sprintf("%s (%s", attempt.Model, attempt.Reason))
		if attempt.Error != "" {
			sb.WriteString(fmt.Sprintf(": %s", attempt.Error))
		}
		sb.WriteString(")")
	}

	return sb.String()
}

// classifyError analyzes an error and determines:
// 1. Whether fallback should occur (FallbackDecision)
// 2. The reason for failure (FallbackReason)
//
// Retryable errors (trigger fallback):
//   - HTTP 429: Rate limit
//   - HTTP 500, 502, 503, 504: Server/gateway errors
//   - Timeout (context.DeadlineExceeded, net.OpError)
//
// Non-retryable errors (fail immediately):
//   - HTTP 400: Bad request (client error)
//   - HTTP 401: Unauthorized (auth failure)
//   - HTTP 403: Forbidden (permission denied)
//   - HTTP 404: Not found
func classifyError(err error) (FallbackDecision, FallbackReason) {
	if err == nil {
		return FallbackNo, ReasonUnknown
	}

	errStr := strings.ToLower(err.Error())

	// Check for timeout errors via context
	if errors.Is(err, context.DeadlineExceeded) {
		return FallbackYes, ReasonTimeout
	}

	// Check for URL errors (often wrapped timeouts)
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Timeout() {
			return FallbackYes, ReasonTimeout
		}
		// Check inner error for timeout
		if errors.Is(urlErr.Err, context.DeadlineExceeded) {
			return FallbackYes, ReasonTimeout
		}
	}

	// Check HTTP status codes and error strings in message
	// This supports common HTTP client libraries that embed status codes in error strings

	// Rate limit - retryable
	if strings.Contains(errStr, "429") || strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "too many requests") {
		return FallbackYes, ReasonRateLimit
	}

	// Server errors - retryable
	if strings.Contains(errStr, "500") || strings.Contains(errStr, "internal server error") {
		return FallbackYes, ReasonServiceDown
	}

	// Gateway errors - retryable
	if strings.Contains(errStr, "502") || strings.Contains(errStr, "bad gateway") {
		return FallbackYes, ReasonGatewayError
	}
	if strings.Contains(errStr, "503") || strings.Contains(errStr, "service unavailable") {
		return FallbackYes, ReasonServiceDown
	}
	if strings.Contains(errStr, "504") || strings.Contains(errStr, "gateway timeout") {
		return FallbackYes, ReasonGatewayError
	}

	// Generic timeout in error string
	if strings.Contains(errStr, "timeout") {
		return FallbackYes, ReasonTimeout
	}

	// Authentication errors - NOT retryable
	if strings.Contains(errStr, "401") || strings.Contains(errStr, "unauthorized") {
		return FallbackNo, ReasonUnknown
	}

	// Permission errors - NOT retryable
	if strings.Contains(errStr, "403") || strings.Contains(errStr, "forbidden") {
		return FallbackNo, ReasonUnknown
	}

	// Bad request errors - NOT retryable
	if strings.Contains(errStr, "400") || strings.Contains(errStr, "bad request") {
		return FallbackNo, ReasonUnknown
	}

	// Not found errors - NOT retryable
	if strings.Contains(errStr, "404") || strings.Contains(errStr, "not found") {
		return FallbackNo, ReasonUnknown
	}

	// Default: other errors are retryable (conservative approach)
	return FallbackYes, ReasonUnknown
}

// NotifyFunc is called when a fallback occurs.
// The message describes which model failed and why.
type NotifyFunc func(message string)

// ExecuteWithFallback iterates through models in a pool until success or all fail.
// It handles:
// - 30 second timeout per model attempt
// - Health tracking with cooldown (skips unhealthy models)
// - Error classification (retryable vs fatal)
// - Fallback notifications via notifyFn
//
// The generic type T allows this to work with any return type.
// Use a notification function to log/observe fallback events (can be nil).
//
// Example:
//
//	result, err := ExecuteWithFallback(ctx, pool, tracker, nil, func(ctx context.Context, model ModelReference) (string, error) {
//	    return callLLM(ctx, model, prompt)
//	})
func ExecuteWithFallback[T any](
	ctx context.Context,
	pool ModelPool,
	tracker *HealthTracker,
	notifyFn NotifyFunc,
	fn func(ctx context.Context, model ModelReference) (T, error),
) (T, error) {
	var zero T

	if pool.IsZero() {
		return zero, &AggregatedError{
			Phase:    "",
			Attempts: []ModelAttempt{{Model: "", Reason: ReasonUnknown, Error: "empty model pool"}},
		}
	}

	// Apply total cycle timeout of 90 seconds (as per spec)
	ctx, cancelAll := context.WithTimeout(ctx, 90*time.Second)
	defer cancelAll()

	attempts := make([]ModelAttempt, 0, 1+len(pool.Fallbacks))
	models := pool.All()

	for _, model := range models {
		// Skip models in cooldown
		if tracker != nil && tracker.IsUnhealthy(model) {
			attempt := ModelAttempt{
				Model:  model,
				Reason: ReasonUnknown,
				Error:  "skipped: model in cooldown",
			}
			attempts = append(attempts, attempt)

			if notifyFn != nil {
				notifyFn(fmt.Sprintf("Skipping %s (in cooldown)", model))
			}
			continue
		}

		// Create child context with 30s timeout
		attemptCtx, cancel := context.WithTimeout(ctx, 30*time.Second)

		// Execute the function
		result, err := fn(attemptCtx, model)
		cancel()

		// Success: reset health and return
		if err == nil {
			if tracker != nil {
				tracker.Reset(model)
			}
			return result, nil
		}

		// Classify the error
		decision, reason := classifyError(err)

		attempt := ModelAttempt{
			Model:  model,
			Reason: reason,
			Error:  err.Error(),
		}
		attempts = append(attempts, attempt)

		// Mark the model as unhealthy
		if tracker != nil {
			tracker.Mark(model, reason)
		}

		// Notify about the failure
		if notifyFn != nil {
			notifyFn(fmt.Sprintf("Model %s failed (%s), trying fallback", model, reason))
		}

		// Non-retryable error: stop immediately
		if decision == FallbackNo {
			return zero, &AggregatedError{
				Phase:    "",
				Attempts: attempts,
			}
		}

		// Retryable error: continue to next model
	}

	// All models exhausted
	return zero, &AggregatedError{
		Phase:    "",
		Attempts: attempts,
	}
}

// ExecuteWithFallbackForPhase is a convenience wrapper that includes the phase name
// in the AggregatedError for better error messages.
func ExecuteWithFallbackForPhase[T any](
	ctx context.Context,
	pool ModelPool,
	tracker *HealthTracker,
	notifyFn NotifyFunc,
	phase string,
	fn func(ctx context.Context, model ModelReference) (T, error),
) (T, error) {
	var zero T

	if pool.IsZero() {
		return zero, &AggregatedError{
			Phase:    phase,
			Attempts: []ModelAttempt{{Model: "", Reason: ReasonUnknown, Error: "empty model pool"}},
		}
	}

	result, err := ExecuteWithFallback(ctx, pool, tracker, notifyFn, fn)

	// Inject phase name into error if it's an AggregatedError
	if err != nil {
		var aggErr *AggregatedError
		if errors.As(err, &aggErr) {
			aggErr.Phase = phase
		}
	}

	return result, err
}