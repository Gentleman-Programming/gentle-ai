package update

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

// TestCheckAllWithCooldown_EmptyHomeDirAlwaysChecks verifies that when homeDir
// is "" (home resolution failed), the check always runs and no state file is
// written anywhere.
func TestCheckAllWithCooldown_EmptyHomeDirAlwaysChecks(t *testing.T) {
	profile := system.PlatformProfile{OS: "darwin", PackageManager: "brew"}
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)

	// Ensure the CWD-relative state directory does not pre-exist from a
	// previous (buggy) run, so we can detect a fresh write unambiguously.
	cwd, _ := os.Getwd()
	stateInCWD := filepath.Join(cwd, ".gentle-ai", "state.json")
	_ = os.Remove(stateInCWD)
	_ = os.Remove(filepath.Join(cwd, ".gentle-ai"))

	checkCalled := 0
	stubCheckAll := func(_ context.Context, _ string, _ system.PlatformProfile) []UpdateResult {
		checkCalled++
		return []UpdateResult{{Tool: ToolInfo{Name: "gentle-ai"}, Status: UpToDate}}
	}

	results := CheckAllWithCooldown(context.Background(), "1.0.0", profile, "", 6*time.Hour,
		func() time.Time { return now },
		stubCheckAll,
	)

	if checkCalled != 1 {
		t.Errorf("network check called %d times with empty homeDir, want 1 (always-check)", checkCalled)
	}
	if len(results) == 0 {
		t.Error("expected non-empty results from the check, got empty")
	}

	// Verify no state file was written to CWD or any relative path.
	if _, err := os.Stat(stateInCWD); err == nil {
		t.Errorf("state file was written to CWD (%s) — must not write when homeDir is empty", stateInCWD)
	}
}

// TestCheckAllWithCooldown_EmptyHomeDirNoStateRead verifies that when homeDir
// is "", the cooldown skip does not engage (no state.Read), so the check runs
// even when a real state file might exist somewhere.
func TestCheckAllWithCooldown_EmptyHomeDirNoStateRead(t *testing.T) {
	profile := system.PlatformProfile{OS: "darwin", PackageManager: "brew"}
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-1 * time.Minute)

	checkCalled := 0
	stubCheckAll := func(_ context.Context, _ string, _ system.PlatformProfile) []UpdateResult {
		checkCalled++
		return nil
	}

	// Even if we pretend there's a "recent" timestamp, with empty homeDir we
	// never read state so the cooldown cannot engage — check must run.
	_ = recent // intentionally unused: we cannot write to homeDir="" safely

	CheckAllWithCooldown(context.Background(), "1.0.0", profile, "", 6*time.Hour,
		func() time.Time { return now },
		stubCheckAll,
	)

	if checkCalled != 1 {
		t.Errorf("network check called %d times, want 1 — empty homeDir disables cooldown read", checkCalled)
	}
}

// TestCheckAllWithCooldown_FutureTimestampAlwaysChecks verifies that a future
// LastUpdateCheck (clock skew) does not cause the cooldown to skip the check.
// Negative elapsed must be treated as needing a check.
func TestCheckAllWithCooldown_FutureTimestampAlwaysChecks(t *testing.T) {
	home := t.TempDir()
	profile := system.PlatformProfile{OS: "darwin", PackageManager: "brew"}

	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	// LastUpdateCheck is 1 hour IN THE FUTURE relative to now.
	future := now.Add(1 * time.Hour)
	s := state.InstallState{
		InstalledAgents: []string{"claude-code"},
		LastUpdateCheck: &future,
	}
	if err := state.Write(home, s); err != nil {
		t.Fatalf("state.Write() error = %v", err)
	}

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
		t.Errorf("network check called %d times with future timestamp, want 1 (must not skip on negative elapsed)", checkCalled)
	}
}

// TestCheckAllWithCooldown_CorruptStateResetsAndPersists verifies that when the
// write-side re-read fails with state.ErrCorruptState (a decode failure — the
// persisted data is already unrecoverable), the cooldown still records by
// resetting to a fresh InstallState and writing the timestamp. Without this the
// remote update check would fire on EVERY launch until the file is repaired.
//
// Both corrupt shapes route through state.Read's ErrCorruptState wrapping:
//   - structurally-broken JSON ("{ not valid")
//   - a malformed RFC3339 timestamp in the *time.Time field
func TestCheckAllWithCooldown_CorruptStateResetsAndPersists(t *testing.T) {
	profile := system.PlatformProfile{OS: "darwin", PackageManager: "brew"}
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		blob string
	}{
		{name: "structurally broken json", blob: "{ not valid"},
		{name: "malformed timestamp field", blob: `{"last_update_check":"not-a-timestamp"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()

			// Seed a corrupt state.json that state.Read classifies as
			// ErrCorruptState (verified below to guard against false positives).
			stateDir := filepath.Join(home, ".gentle-ai")
			if err := os.MkdirAll(stateDir, 0o755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			if err := os.WriteFile(state.Path(home), []byte(tt.blob), 0o644); err != nil {
				t.Fatalf("WriteFile corrupt: %v", err)
			}
			if _, err := state.Read(home); !errors.Is(err, state.ErrCorruptState) {
				t.Fatalf("precondition: state.Read(%q) = %v, want errors.Is(..., ErrCorruptState)", tt.blob, err)
			}

			stubCheckAll := func(_ context.Context, _ string, _ system.PlatformProfile) []UpdateResult {
				// Successful result → checkSucceeded is true → write attempted.
				return []UpdateResult{{Tool: ToolInfo{Name: "gentle-ai"}, Status: UpToDate}}
			}

			CheckAllWithCooldown(context.Background(), "1.0.0", profile, home, 6*time.Hour,
				func() time.Time { return now },
				stubCheckAll,
			)

			// After the run the corrupt file must have been reset to a valid,
			// decodable state.json whose LastUpdateCheck equals the injected now.
			updated, err := state.Read(home)
			if err != nil {
				t.Fatalf("state.Read() after cooldown error = %v, want nil (corrupt file should be reset)", err)
			}
			if updated.LastUpdateCheck == nil || !updated.LastUpdateCheck.Equal(now) {
				t.Errorf("LastUpdateCheck after corrupt reset = %v, want %v (timestamp must persist)", updated.LastUpdateCheck, now)
			}
		})
	}
}

// TestCheckAllWithCooldown_UnreadableStateSkipsWrite verifies the conservative
// branch: when the write-side re-read fails with a genuine IO/permission error
// (NEITHER os.ErrNotExist NOR state.ErrCorruptState), the file's bytes may still
// be a valid state.json that is merely unreadable this once, so the timestamp
// write is skipped to avoid clobbering recoverable data.
//
// The unreadable file is simulated with mode 0o000. This is not portable as
// root (root bypasses permission bits), so the test skips when running as root
// rather than asserting flakily.
func TestCheckAllWithCooldown_UnreadableStateSkipsWrite(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses file permission bits; cannot simulate an unreadable state.json")
	}

	home := t.TempDir()
	profile := system.PlatformProfile{OS: "darwin", PackageManager: "brew"}
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)

	// Seed a VALID state.json, then strip read permission so state.Read returns
	// an unwrapped os.ErrPermission (the bytes are possibly-valid-but-unreadable).
	stateDir := filepath.Join(home, ".gentle-ai")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	original := []byte(`{"installed_agents":["claude-code"]}` + "\n")
	statePath := state.Path(home)
	if err := os.WriteFile(statePath, original, 0o644); err != nil {
		t.Fatalf("WriteFile valid: %v", err)
	}
	if err := os.Chmod(statePath, 0o000); err != nil {
		t.Fatalf("Chmod 0o000: %v", err)
	}
	// Restore perms so t.TempDir cleanup can remove the file.
	t.Cleanup(func() { _ = os.Chmod(statePath, 0o644) })

	// Guard the precondition: the read error must be neither NotExist nor Corrupt.
	if _, err := state.Read(home); errors.Is(err, os.ErrNotExist) || errors.Is(err, state.ErrCorruptState) {
		t.Skipf("could not simulate an unreadable-only state.json (read err = %v)", err)
	}

	stubCheckAll := func(_ context.Context, _ string, _ system.PlatformProfile) []UpdateResult {
		return []UpdateResult{{Tool: ToolInfo{Name: "gentle-ai"}, Status: UpToDate}}
	}

	CheckAllWithCooldown(context.Background(), "1.0.0", profile, home, 6*time.Hour,
		func() time.Time { return now },
		stubCheckAll,
	)

	// The unreadable file must NOT have been overwritten: restore read access
	// and confirm the original bytes survive untouched.
	if err := os.Chmod(statePath, 0o644); err != nil {
		t.Fatalf("Chmod restore: %v", err)
	}
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("ReadFile after cooldown: %v", err)
	}
	if string(data) != string(original) {
		t.Errorf("unreadable state file was overwritten; got %q, want original %q", string(data), string(original))
	}
}
