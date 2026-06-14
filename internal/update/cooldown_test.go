package update

import (
	"context"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/state"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

// TestCheckAllWithCooldown_FreshCacheSkipsNetwork verifies that when
// LastUpdateCheck is recent (elapsed < TTL) no GitHub call is made and the
// cached/empty result is returned immediately.
func TestCheckAllWithCooldown_FreshCacheSkipsNetwork(t *testing.T) {
	home := t.TempDir()
	profile := system.PlatformProfile{OS: "darwin", PackageManager: "brew"}

	// Write state with a LastUpdateCheck 1 minute ago (well within 6h TTL).
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-1 * time.Minute)
	s := state.InstallState{
		InstalledAgents: []string{"claude-code"},
		LastUpdateCheck: &recent,
	}
	if err := state.Write(home, s); err != nil {
		t.Fatalf("state.Write() error = %v", err)
	}

	checkCalled := 0
	stubCheckAll := func(_ context.Context, _ string, _ system.PlatformProfile) []UpdateResult {
		checkCalled++
		return []UpdateResult{{Status: UpdateAvailable}}
	}

	results := CheckAllWithCooldown(context.Background(), "1.0.0", profile, home, 6*time.Hour,
		func() time.Time { return now },
		stubCheckAll,
	)

	if checkCalled != 0 {
		t.Errorf("network check called %d times, want 0 (cache is fresh)", checkCalled)
	}
	if len(results) != 0 {
		t.Errorf("results = %v, want empty (fresh cache returns nil/empty)", results)
	}
}

// TestCheckAllWithCooldown_StaleCacheRefreshes verifies that when
// LastUpdateCheck is older than the TTL the network check fires and on success
// LastUpdateCheck is updated in state.
func TestCheckAllWithCooldown_StaleCacheRefreshes(t *testing.T) {
	home := t.TempDir()
	profile := system.PlatformProfile{OS: "darwin", PackageManager: "brew"}

	// Write state with LastUpdateCheck 7 hours ago (> 6h TTL).
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	stale := now.Add(-7 * time.Hour)
	s := state.InstallState{
		InstalledAgents: []string{"claude-code"},
		LastUpdateCheck: &stale,
	}
	if err := state.Write(home, s); err != nil {
		t.Fatalf("state.Write() error = %v", err)
	}

	stubResults := []UpdateResult{{Tool: ToolInfo{Name: "gentle-ai"}, Status: UpToDate}}
	checkCalled := 0
	stubCheckAll := func(_ context.Context, _ string, _ system.PlatformProfile) []UpdateResult {
		checkCalled++
		return stubResults
	}

	results := CheckAllWithCooldown(context.Background(), "1.0.0", profile, home, 6*time.Hour,
		func() time.Time { return now },
		stubCheckAll,
	)

	if checkCalled != 1 {
		t.Errorf("network check called %d times, want 1 (cache is stale)", checkCalled)
	}
	if len(results) != len(stubResults) {
		t.Errorf("results len = %d, want %d", len(results), len(stubResults))
	}

	// Verify LastUpdateCheck was updated to now on success.
	updated, err := state.Read(home)
	if err != nil {
		t.Fatalf("state.Read() error = %v", err)
	}
	if updated.LastUpdateCheck == nil || !updated.LastUpdateCheck.Equal(now) {
		t.Errorf("LastUpdateCheck after stale refresh = %v, want %v", updated.LastUpdateCheck, now)
	}
}

// TestCheckAllWithCooldown_MissingCache first-run always checks.
func TestCheckAllWithCooldown_MissingCache(t *testing.T) {
	home := t.TempDir()
	profile := system.PlatformProfile{OS: "darwin", PackageManager: "brew"}

	// No state file — first-run scenario.
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)

	checkCalled := 0
	stubCheckAll := func(_ context.Context, _ string, _ system.PlatformProfile) []UpdateResult {
		checkCalled++
		return nil
	}

	CheckAllWithCooldown(context.Background(), "1.0.0", profile, home, 6*time.Hour,
		func() time.Time { return now },
		stubCheckAll,
	)

	if checkCalled != 1 {
		t.Errorf("network check called %d times on first run, want 1", checkCalled)
	}
}

// TestCheckAllWithCooldown_FailedCheckDoesNotAdvanceTimestamp verifies that
// when the underlying check returns an error-flagged result (CheckFailed), the
// LastUpdateCheck timestamp is NOT updated, so the next launch retries.
func TestCheckAllWithCooldown_FailedCheckDoesNotAdvanceTimestamp(t *testing.T) {
	home := t.TempDir()
	profile := system.PlatformProfile{OS: "darwin", PackageManager: "brew"}

	// Stale cache — will attempt a refresh.
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	stale := now.Add(-7 * time.Hour)
	s := state.InstallState{
		InstalledAgents: []string{"claude-code"},
		LastUpdateCheck: &stale,
	}
	if err := state.Write(home, s); err != nil {
		t.Fatalf("state.Write() error = %v", err)
	}

	// Return a failed-check result (all tools failed).
	failedResults := []UpdateResult{
		{Tool: ToolInfo{Name: "gentle-ai"}, Status: CheckFailed},
	}
	stubCheckAll := func(_ context.Context, _ string, _ system.PlatformProfile) []UpdateResult {
		return failedResults
	}

	CheckAllWithCooldown(context.Background(), "1.0.0", profile, home, 6*time.Hour,
		func() time.Time { return now },
		stubCheckAll,
	)

	// LastUpdateCheck must NOT have advanced.
	updated, err := state.Read(home)
	if err != nil {
		t.Fatalf("state.Read() error = %v", err)
	}
	if updated.LastUpdateCheck != nil && updated.LastUpdateCheck.Equal(now) {
		t.Error("LastUpdateCheck was advanced after a failed check — must only advance on success")
	}
	// It should remain the original stale value.
	if updated.LastUpdateCheck == nil || !updated.LastUpdateCheck.Equal(stale) {
		t.Errorf("LastUpdateCheck = %v, want original stale %v", updated.LastUpdateCheck, stale)
	}
}
