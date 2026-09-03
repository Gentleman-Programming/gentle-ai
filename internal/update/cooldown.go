package update

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
	"github.com/gentleman-programming/gentle-ai/v2/internal/statecoord"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

// UpdateCheckTTL is the minimum time between remote update checks. Launches
// within this window reuse the cached state and skip the GitHub API call.
const UpdateCheckTTL = 6 * time.Hour

// checkAllFn is the function type that matches CheckAll's signature, used to
// allow injection of a stub in tests.
type checkAllFn func(ctx context.Context, currentVersion string, profile system.PlatformProfile) []UpdateResult

type installStateWriter func(string, state.InstallState) error

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
	return checkAllWithCooldown(ctx, currentVersion, profile, homeDir, ttl, nowFn, checkFn, persistLastUpdateCheck)
}

func checkAllWithCooldown(
	ctx context.Context,
	currentVersion string,
	profile system.PlatformProfile,
	homeDir string,
	ttl time.Duration,
	nowFn func() time.Time,
	checkFn checkAllFn,
	persistTimestamp func(string, time.Time) error,
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
		// Ignore persistence errors — non-fatal; next launch will retry.
		_ = persistTimestamp(homeDir, now)
	}

	return results
}

func persistLastUpdateCheck(homeDir string, timestamp time.Time) error {
	return persistLastUpdateCheckWithWriter(homeDir, timestamp, state.WriteReconciled)
}

func persistLastUpdateCheckWithWriter(homeDir string, timestamp time.Time, write installStateWriter) error {
	return statecoord.WithLock(homeDir, func() error {
		current, err := state.Read(homeDir)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			current = state.InstallState{}
		}
		current.LastUpdateCheck = &timestamp
		return write(homeDir, current)
	})
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
