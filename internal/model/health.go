package model

import (
	"sync"
	"time"
)

// HealthEntry tracks the health state of a single model.
// It records consecutive failures and the timestamp of the last failure.
type HealthEntry struct {
	// FailureCount is the number of consecutive failures for this model.
	FailureCount int `json:"failure_count"`
	// LastFailure is the timestamp of the most recent failure.
	LastFailure time.Time `json:"last_failure"`
	// LastReason records why the model failed (for debugging/logging).
	LastReason FallbackReason `json:"last_reason"`
}

// IsUnhealthy returns true if the model is in cooldown.
// Cooldown duration depends on the number of consecutive failures:
// - 1-2 failures: 30 second cooldown (Degraded)
// - 3+ failures: 60 second cooldown (Unhealthy)
func (e HealthEntry) IsUnhealthy() bool {
	if e.FailureCount == 0 {
		return false
	}

	cooldown := e.cooldownDuration()
	return time.Since(e.LastFailure) < cooldown
}

// cooldownDuration returns the cooldown period based on failure count.
func (e HealthEntry) cooldownDuration() time.Duration {
	if e.FailureCount >= 3 {
		return 60 * time.Second // Unhealthy: 60s cooldown
	}
	return 30 * time.Second // Degraded: 30s cooldown
}

// HealthTracker tracks model health status within the current session.
// It uses sync.Map for goroutine-safe concurrent access.
// Health state is session-scoped and not persisted across restarts.
type HealthTracker struct {
	// entries maps ModelReference to *HealthEntry
	entries sync.Map
}

// NewHealthTracker creates a new HealthTracker instance.
func NewHealthTracker() *HealthTracker {
	return &HealthTracker{}
}

// IsUnhealthy checks if a model is currently in cooldown.
// Returns true if the model should be skipped due to recent failures.
func (h *HealthTracker) IsUnhealthy(model ModelReference) bool {
	value, ok := h.entries.Load(model)
	if !ok {
		return false
	}

	entry := value.(*HealthEntry)
	return entry.IsUnhealthy()
}

// Mark records a failure for a model and updates its health state.
// This increments the failure count and records the reason and timestamp.
func (h *HealthTracker) Mark(model ModelReference, reason FallbackReason) {
	// Load current entry or create new one
	value, _ := h.entries.LoadOrStore(model, &HealthEntry{
		FailureCount: 0,
		LastReason:   reason,
	})

	// Update the entry atomically
	for {
		entry := value.(*HealthEntry)
		newEntry := &HealthEntry{
			FailureCount: entry.FailureCount + 1,
			LastFailure:  time.Now(),
			LastReason:   reason,
		}

		// Try to swap atomically
		if h.entries.CompareAndSwap(model, value, newEntry) {
			break
		}

		// Racing update happened, reload and retry
		value, _ = h.entries.Load(model)
	}
}

// Reset clears the health state for a model (e.g., after successful request).
// This should be called when a model succeeds to reset its failure count.
func (h *HealthTracker) Reset(model ModelReference) {
	h.entries.Delete(model)
}

// GetEntry returns a copy of the health entry for a model.
// Returns nil if no entry exists for the model.
func (h *HealthTracker) GetEntry(model ModelReference) *HealthEntry {
	value, ok := h.entries.Load(model)
	if !ok {
		return nil
	}

	entry := value.(*HealthEntry)
	// Return a copy to prevent external modification
	return &HealthEntry{
		FailureCount: entry.FailureCount,
		LastFailure:  entry.LastFailure,
		LastReason:   entry.LastReason,
	}
}

// GetAllUnhealthy returns all models currently in cooldown.
// This can be used for logging or diagnostics.
func (h *HealthTracker) GetAllUnhealthy() []ModelReference {
	var unhealthy []ModelReference

	h.entries.Range(func(key, value interface{}) bool {
		model := key.(ModelReference)
		entry := value.(*HealthEntry)

		if entry.IsUnhealthy() {
			unhealthy = append(unhealthy, model)
		}
		return true
	})

	return unhealthy
}

// Clear removes all health entries.
// This can be used for testing or to reset state.
func (h *HealthTracker) Clear() {
	h.entries = sync.Map{}
}