package model

import (
	"testing"
	"time"
)

// TestHealthEntry_IsUnhealthy tests the cooldown logic for health entries.
func TestHealthEntry_IsUnhealthy(t *testing.T) {
	tests := []struct {
		name         string
		failureCount int
		timeSince    time.Duration
		wantHealthy  bool
	}{
		{
			name:         "no failures - healthy",
			failureCount: 0,
			timeSince:    0,
			wantHealthy:  false, // not unhealthy
		},
		{
			name:         "1 failure within 30s - unhealthy",
			failureCount: 1,
			timeSince:    15 * time.Second,
			wantHealthy:  true, // unhealthy
		},
		{
			name:         "1 failure after 30s - healthy",
			failureCount: 1,
			timeSince:    31 * time.Second,
			wantHealthy:  false, // not unhealthy (cooldown expired)
		},
		{
			name:         "2 failures within 30s - unhealthy",
			failureCount: 2,
			timeSince:    20 * time.Second,
			wantHealthy:  true,
		},
		{
			name:         "3 failures within 60s - unhealthy",
			failureCount: 3,
			timeSince:    45 * time.Second,
			wantHealthy:  true,
		},
		{
			name:         "3 failures after cooldown - healthy",
			failureCount: 3,
			timeSince:    61 * time.Second,
			wantHealthy:  false,
		},
		{
			name:         "5 failures within 60s - unhealthy",
			failureCount: 5,
			timeSince:    59 * time.Second,
			wantHealthy:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := HealthEntry{
				FailureCount: tt.failureCount,
				LastFailure:  time.Now().Add(-tt.timeSince),
			}

			got := entry.IsUnhealthy()
			if got != tt.wantHealthy {
				t.Errorf("HealthEntry.IsUnhealthy() = %v, want %v", got, tt.wantHealthy)
			}
		})
	}
}

// TestHealthTracker_Mark tests marking models as unhealthy.
func TestHealthTracker_Mark(t *testing.T) {
	tracker := NewHealthTracker()
	model := ModelReference("anthropic/claude-opus")

	// Initially healthy
	if tracker.IsUnhealthy(model) {
		t.Error("model should be healthy initially")
	}

	// Mark as unhealthy
	tracker.Mark(model, ReasonRateLimit)

	// Should be unhealthy now
	if !tracker.IsUnhealthy(model) {
		t.Error("model should be unhealthy after Mark")
	}

	// Check entry
	entry := tracker.GetEntry(model)
	if entry == nil {
		t.Fatal("expected entry to exist")
	}
	if entry.FailureCount != 1 {
		t.Errorf("FailureCount = %d, want 1", entry.FailureCount)
	}
	if entry.LastReason != ReasonRateLimit {
		t.Errorf("LastReason = %s, want %s", entry.LastReason, ReasonRateLimit)
	}
}

// TestHealthTracker_Reset tests resetting model health.
func TestHealthTracker_Reset(t *testing.T) {
	tracker := NewHealthTracker()
	model := ModelReference("openai/gpt-4")

	// Mark as unhealthy
	tracker.Mark(model, ReasonServiceDown)
	if !tracker.IsUnhealthy(model) {
		t.Error("model should be unhealthy after Mark")
	}

	// Reset
	tracker.Reset(model)

	// Should be healthy now
	if tracker.IsUnhealthy(model) {
		t.Error("model should be healthy after Reset")
	}

	// Entry should be nil
	entry := tracker.GetEntry(model)
	if entry != nil {
		t.Errorf("expected nil entry after Reset, got %+v", entry)
	}
}

// TestHealthTracker_GetAllUnhealthy tests retrieving all unhealthy models.
func TestHealthTracker_GetAllUnhealthy(t *testing.T) {
	tracker := NewHealthTracker()

	model1 := ModelReference("anthropic/claude-opus")
	model2 := ModelReference("openai/gpt-4")
	model3 := ModelReference("google/gemini-pro")

	// Mark model1 and model2 as unhealthy
	tracker.Mark(model1, ReasonRateLimit)
	// No need to mark model2 - just model1 for this test

	// Get unhealthy models
	unhealthy := tracker.GetAllUnhealthy()
	if len(unhealthy) != 1 {
		t.Errorf("expected 1 unhealthy model, got %d", len(unhealthy))
	}

	// Mark model2 as well
	tracker.Mark(model2, ReasonTimeout)

	unhealthy = tracker.GetAllUnhealthy()
	if len(unhealthy) != 2 {
		t.Errorf("expected 2 unhealthy models, got %d", len(unhealthy))
	}

	// model3 should not be in the list (never marked)
	for _, m := range unhealthy {
		if m == model3 {
			t.Error("model3 should not be in unhealthy list")
		}
	}
}

// TestHealthTracker_ConcurrentAccess tests thread safety.
func TestHealthTracker_ConcurrentAccess(t *testing.T) {
	tracker := NewHealthTracker()
	model := ModelReference("test/model")

	// Run multiple goroutines marking and checking
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				tracker.Mark(model, ReasonRateLimit)
				tracker.IsUnhealthy(model)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Final state should be consistent
	entry := tracker.GetEntry(model)
	if entry == nil {
		t.Fatal("expected entry to exist after concurrent access")
	}
	if entry.FailureCount < 100 {
		t.Errorf("FailureCount = %d, expected at least 100 (concurrent increments)", entry.FailureCount)
	}
}

// TestCooldownDuration tests the cooldown duration calculation.
func TestCooldownDuration(t *testing.T) {
	tests := []struct {
		name            string
		failureCount    int
		expectedDiff    time.Duration
		tolerance       time.Duration
	}{
		{
			name:         "degraded (1-2 failures): 30s cooldown",
			failureCount: 1,
			expectedDiff: 30 * time.Second,
			tolerance:    time.Second,
		},
		{
			name:         "degraded (2 failures): 30s cooldown",
			failureCount: 2,
			expectedDiff: 30 * time.Second,
			tolerance:    time.Second,
		},
		{
			name:         "unhealthy (3 failures): 60s cooldown",
			failureCount: 3,
			expectedDiff: 60 * time.Second,
			tolerance:    time.Second,
		},
		{
			name:         "unhealthy (5 failures): 60s cooldown",
			failureCount: 5,
			expectedDiff: 60 * time.Second,
			tolerance:    time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := HealthEntry{FailureCount: tt.failureCount}
			got := entry.cooldownDuration()

			diff := tt.expectedDiff - got
			if diff < 0 {
				diff = -diff
			}
			if diff > tt.tolerance {
				t.Errorf("cooldownDuration() = %v, want %v (tolerance %v)", got, tt.expectedDiff, tt.tolerance)
			}
		})
	}
}