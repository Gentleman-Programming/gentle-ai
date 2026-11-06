package telemetrycollector

import (
	"testing"
	"time"
)

func TestRateLimiter_AllowsBurstThenRejects(t *testing.T) {
	limiter := NewRateLimiter(3)
	fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return fixed }

	for i := 0; i < 3; i++ {
		if !limiter.Allow("203.0.113.5") {
			t.Fatalf("Allow: request %d within burst was rejected", i+1)
		}
	}
	if limiter.Allow("203.0.113.5") {
		t.Fatal("Allow: request beyond burst should have been rejected")
	}
}

func TestRateLimiter_RefillsOverTime(t *testing.T) {
	limiter := NewRateLimiter(60) // 1 token/second
	current := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return current }

	for i := 0; i < 60; i++ {
		if !limiter.Allow("198.51.100.1") {
			t.Fatalf("Allow: request %d within initial burst was rejected", i+1)
		}
	}
	if limiter.Allow("198.51.100.1") {
		t.Fatal("Allow: bucket should be empty after consuming the full burst")
	}

	current = current.Add(2 * time.Second)
	allowed := 0
	for i := 0; i < 3; i++ {
		if limiter.Allow("198.51.100.1") {
			allowed++
		}
	}
	if allowed != 2 {
		t.Errorf("allowed = %d after a 2s refill, want 2", allowed)
	}
}

func TestRateLimiter_KeysAreIndependent(t *testing.T) {
	limiter := NewRateLimiter(1)
	fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return fixed }

	if !limiter.Allow("10.0.0.1") {
		t.Fatal("Allow: first request for 10.0.0.1 should succeed")
	}
	if limiter.Allow("10.0.0.1") {
		t.Fatal("Allow: second request for 10.0.0.1 should be rejected")
	}
	if !limiter.Allow("10.0.0.2") {
		t.Fatal("Allow: first request for a different key should succeed")
	}
}

func TestRateLimiter_SweepEvictsIdleBuckets(t *testing.T) {
	limiter := NewRateLimiter(1)
	current := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return current }

	limiter.Allow("10.0.0.1")
	if _, ok := limiter.buckets["10.0.0.1"]; !ok {
		t.Fatal("expected a bucket to exist after Allow")
	}

	current = current.Add(time.Hour)
	limiter.Sweep(30 * time.Minute)
	if _, ok := limiter.buckets["10.0.0.1"]; ok {
		t.Fatal("Sweep: idle bucket should have been evicted")
	}
}

func TestRateLimiter_NonPositiveLimitDisablesLimiting(t *testing.T) {
	limiter := NewRateLimiter(0)
	for i := 0; i < 1000; i++ {
		if !limiter.Allow("any") {
			t.Fatal("Allow: a non-positive limit should never reject")
		}
	}
}
