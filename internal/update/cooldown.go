package update

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/state"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

// UpdateCheckTTL is the minimum time between remote update checks. Launches
// within this window reuse the cached state and skip the GitHub API call.
const UpdateCheckTTL = 6 * time.Hour

// checkAllFn is the function type that matches CheckAll's signature, used to
// allow injection of a stub in tests.
type checkAllFn func(ctx context.Context, currentVersion string, profile system.PlatformProfile) []UpdateResult

// CheckAllWithCooldown gates CheckAll behind a TTL-based cooldown. It reads
// LastUpdateCheck from state.json in homeDir; if the elapsed time since the
// last successful check is less than ttl, it returns nil (no network call).
// When a check is performed and ALL results are non-failed, it writes the
// current time back to LastUpdateCheck. On any read/write error the function
// falls back to running the check (fail-open).
//
// When homeDir is "" (home directory resolution failed), both the state read
// and write are skipped entirely: the check always runs and no state file is
// touched, preventing writes to arbitrary CWD-relative paths.
//
// nowFn is injected for test determinism; pass func() time.Time { return time.Now() }
// in production code. checkFn is injected for testing; pass CheckAll in production.
func CheckAllWithCooldown(
	ctx context.Context,
	currentVersion string,
	profile system.PlatformProfile,
	homeDir string,
	ttl time.Duration,
	nowFn func() time.Time,
	checkFn checkAllFn,
) []UpdateResult {
	now := nowFn()

	// When homeDir is empty (home resolution failed), skip both state read and
	// write: always run the check and never touch any state.json.
	if homeDir != "" {
		// Read existing state to inspect LastUpdateCheck.
		s, err := state.Read(homeDir)
		if err == nil && s.LastUpdateCheck != nil {
			elapsed := now.Sub(*s.LastUpdateCheck)
			// Only skip when elapsed is non-negative AND within the TTL window.
			// A future LastUpdateCheck (negative elapsed from clock skew) must
			// not suppress the check.
			if elapsed >= 0 && elapsed < ttl {
				// Cache is fresh — skip network call.
				return nil
			}
		}
		// err != nil means no state file (first run) → always check.
	}

	// Perform the remote check.
	results := checkFn(ctx, currentVersion, profile)

	// Update LastUpdateCheck only when the check succeeded (at least one
	// non-failed result, or empty-but-no-error; never if all are CheckFailed).
	// Also skip write when homeDir is empty.
	if homeDir != "" && checkSucceeded(results) {
		// Re-read state to avoid clobbering unrelated fields written concurrently.
		current, readErr := state.Read(homeDir)
		if readErr != nil {
			if !errors.Is(readErr, os.ErrNotExist) {
				// File exists but is unreadable/corrupt — do not overwrite; skip
				// persisting the timestamp this round to avoid data loss.
				return results
			}
			// File genuinely missing (first run) — start fresh.
			current = state.InstallState{}
		}
		current.LastUpdateCheck = &now
		// Ignore write errors — non-fatal; next launch will retry.
		_ = state.Write(homeDir, current)
	}

	return results
}

// checkSucceeded returns true when the result set should be considered a
// successful check. A check is successful when every result is non-failed, or
// there are no results at all (empty slice = no tools to check = trivially ok).
func checkSucceeded(results []UpdateResult) bool {
	for _, r := range results {
		if r.Status == CheckFailed {
			return false
		}
	}
	return true
}
