package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mattn/go-isatty"

	"github.com/gentleman-programming/gentle-ai/internal/state"
	"github.com/gentleman-programming/gentle-ai/internal/system"
	"github.com/gentleman-programming/gentle-ai/internal/update"
	"github.com/gentleman-programming/gentle-ai/internal/update/upgrade"
)

// selfUpdateNowFn returns the current time; injected for test determinism.
var selfUpdateNowFn = func() time.Time { return time.Now() }

// selfUpdateHomeDirFn resolves the user home directory; injected for tests.
var selfUpdateHomeDirFn = os.UserHomeDir

// Environment variable names for self-update control.
// NOTE: GENTLE_AI_CONFIRM_UPDATE removed in slice 5 — prompt is now unconditional.
const (
	envNoSelfUpdate   = "GENTLE_AI_NO_SELF_UPDATE"
	envSelfUpdateDone = "GENTLE_AI_SELF_UPDATE_DONE"
	envYesUpdate      = "GENTLE_AI_YES"
)

// isattyFn is a package-level var for TTY detection, injectable for tests.
var isattyFn = func(fd uintptr) bool { return isatty.IsTerminal(fd) }

// selfUpdateYesFn returns true when the caller wants the upgrade to proceed
// without an interactive prompt. Set GENTLE_AI_YES=1 for scripted upgrades.
// Injectable for tests.
var selfUpdateYesFn = func() bool {
	return os.Getenv(envYesUpdate) == "1"
}

// promptFn is swappable for tests — asks the user whether to apply the update.
// Returns true if the user confirms, false to skip.
var promptFn = defaultPromptForUpdate

// defaultPromptForUpdate prints the version delta and reads a Y/n answer from stdin.
// Default answer is Y (Enter = accept). If stdin is not a TTY, it auto-declines so
// scripts and CI are never blocked. Uses isattyFn for testability.
func defaultPromptForUpdate(stdout io.Writer, stdin io.Reader, currentVersion, latestVersion string) (bool, error) {
	// Require a TTY; non-interactive environments silently decline.
	if f, ok := stdin.(*os.File); !ok || !isattyFn(f.Fd()) {
		return false, nil
	}

	_, _ = fmt.Fprintf(stdout, "Update available: %s → %s. Apply now? [Y/n]: ", currentVersion, latestVersion)

	scanner := bufio.NewScanner(stdin)
	if !scanner.Scan() {
		return false, scanner.Err()
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	// Empty input (Enter) = accept default Y; "y"/"yes" explicit accept; anything else declines.
	return answer == "" || answer == "y" || answer == "yes", nil
}

// selfUpdateTimeout is the maximum time allowed for the update check + upgrade.
const selfUpdateTimeout = 7 * time.Second

// selfUpdate checks for and applies a gentle-ai update before normal dispatch.
// Returns nil on success or skip; errors are non-fatal (caller logs and continues).
//
// Guard evaluation order (per spec):
//  1. GENTLE_AI_SELF_UPDATE_DONE=1 → skip (loop guard)
//  2. GENTLE_AI_NO_SELF_UPDATE=1 → skip (opt-out)
//  3. version == "dev" → skip (dev build)
//  4. Proceed with update check
func selfUpdate(ctx context.Context, version string, profile system.PlatformProfile, stdout io.Writer) error {
	// Guard 1: loop prevention — already updated this invocation.
	if os.Getenv(envSelfUpdateDone) == "1" {
		return nil
	}

	// Guard 2: user opt-out.
	if os.Getenv(envNoSelfUpdate) == "1" {
		return nil
	}

	// Guard 3: dev build — no meaningful version to compare.
	if version == "dev" {
		return nil
	}

	// Apply timeout to the entire check+upgrade cycle.
	ctx, cancel := context.WithTimeout(ctx, selfUpdateTimeout)
	defer cancel()

	// Resolve home directory for cooldown state read/write.
	homeDir, err := selfUpdateHomeDirFn()
	if err != nil {
		homeDir = "" // fall back to always-check on home dir failure
	}

	// Check for updates (only gentle-ai), gated by the 6h cooldown.
	// When the cache is fresh (elapsed < UpdateCheckTTL), this returns nil
	// and no network request is made. The underlying check is always
	// updateCheckFiltered, kept as a package-level var for other tests.
	results := update.CheckAllWithCooldown(ctx, version, profile, homeDir, update.UpdateCheckTTL,
		selfUpdateNowFn,
		func(c context.Context, ver string, prof system.PlatformProfile) []update.UpdateResult {
			return updateCheckFiltered(c, ver, prof, []string{"gentle-ai"})
		},
	)

	// Find the gentle-ai result.
	var target *update.UpdateResult
	for i := range results {
		if results[i].Tool.Name == "gentle-ai" {
			target = &results[i]
			break
		}
	}

	// No result or not an available update — nothing to do.
	if target == nil || target.Status != update.UpdateAvailable {
		return nil
	}

	// Prompt the user before applying — unconditional (GENTLE_AI_CONFIRM_UPDATE removed).
	// When --yes / GENTLE_AI_YES=1, substitute an auto-accept stub so scripted
	// upgrades work without a TTY. When stdin is not a TTY, defaultPromptForUpdate
	// auto-declines, making non-interactive runs (CI, pipes) safe by default.
	activePrmptFn := promptFn
	if selfUpdateYesFn() {
		activePrmptFn = func(_ io.Writer, _ io.Reader, _, _ string) (bool, error) {
			return true, nil
		}
	}
	ok, err := activePrmptFn(stdout, os.Stdin, version, target.LatestVersion)
	if err != nil || !ok {
		return nil
	}

	// Run upgrade (backup + strategy execution).
	// homeDir was resolved above for the cooldown gate; re-check in case it failed.
	if homeDir == "" {
		_, _ = fmt.Fprintf(stdout, "self-update: cannot resolve home directory\n")
		return nil // non-fatal
	}

	report := upgradeExecute(ctx, results, profile, homeDir, false, stdout)

	// Check if upgrade succeeded.
	var succeeded bool
	for _, r := range report.Results {
		if r.ToolName == "gentle-ai" && r.Status == upgrade.UpgradeSucceeded {
			succeeded = true
			break
		}
	}

	if !succeeded {
		// Upgrade failed or was skipped — non-fatal, continue with current binary.
		return nil
	}

	// Deferred sync: set PendingSync=true in state before exiting so the new
	// binary runs sync automatically on its next launch. This replaces the
	// previous "restart and sync manually" skip path. Failure to write state is
	// non-fatal — the user can re-run sync explicitly.
	//
	// No-clobber guard: only fall back to a fresh InstallState{} when the file
	// is genuinely missing (ErrNotExist). Any other read error (e.g. corrupt
	// JSON, permission denied) means an existing file is present — do not
	// overwrite it and risk dropping unrelated persisted fields.
	if homeDir != "" {
		s, readErr := state.Read(homeDir)
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			// File exists but is unreadable/corrupt — skip this round to avoid
			// clobbering installed_agents, model assignments, etc.
		} else {
			s.PendingSync = true
			_ = state.Write(homeDir, s)
		}
	}

	return restartAfterGentleAIUpgrade(target.LatestVersion, stdout)
}

func gentleAIUpgradeSucceeded(report upgrade.UpgradeReport) (string, bool) {
	for _, r := range report.Results {
		if r.ToolName == "gentle-ai" && r.Status == upgrade.UpgradeSucceeded {
			return strings.TrimPrefix(r.NewVersion, "v"), true
		}
	}
	return "", false
}

func restartAfterGentleAIUpgrade(latestVersion string, stdout io.Writer) error {
	latestVersion = strings.TrimPrefix(latestVersion, "v")
	// Converged behavior (task 4.6): always print the restart message on every OS
	// and return. The new binary runs automatically on next launch, picking up
	// PendingSync=true and completing the deferred sync. This sidesteps the
	// Windows binary-lock issue and gives a consistent single path across all OSes.
	// Tradeoff: Unix loses seamless re-exec restart; mitigated by clear copy below.
	_, _ = fmt.Fprintf(stdout, "Updated to v%s — restart gentle-ai to continue.\n", latestVersion)
	return nil
}
