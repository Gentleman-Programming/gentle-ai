package update

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gentleman-programming/gentle-ai/v5/internal/system"
)

var updateChannelEnv = os.Getenv

// CheckAll runs update checks for all registered tools concurrently.
// currentVersion is the build-time version of gentle-ai (from app.Version).
// profile determines platform-specific update instructions.
func CheckAll(ctx context.Context, currentVersion string, profile system.PlatformProfile) []UpdateResult {
	return CheckFiltered(ctx, currentVersion, profile, nil)
}

// CheckFiltered runs update checks for a subset of tools identified by name.
// If toolNames is nil or empty, it behaves identically to CheckAll (all tools).
// Unknown tool names in toolNames are silently ignored.
func CheckFiltered(ctx context.Context, currentVersion string, profile system.PlatformProfile, toolNames []string) []UpdateResult {
	// Build the target slice: all tools when filter is empty, otherwise only matching ones.
	var targets []ToolInfo
	if len(toolNames) == 0 {
		targets = Tools
	} else {
		nameSet := make(map[string]struct{}, len(toolNames))
		for _, n := range toolNames {
			nameSet[n] = struct{}{}
		}
		for _, t := range Tools {
			if _, ok := nameSet[t.Name]; ok {
				targets = append(targets, t)
			}
		}
	}

	results := make([]UpdateResult, len(targets))

	var wg sync.WaitGroup
	for i, tool := range targets {
		wg.Add(1)
		go func(idx int, t ToolInfo) {
			defer wg.Done()
			results[idx] = checkSingleTool(ctx, t, currentVersion, profile)
		}(i, tool)
	}

	wg.Wait()
	return results
}

// checkSingleTool checks a single tool: detects local version, fetches remote, compares.
func checkSingleTool(ctx context.Context, tool ToolInfo, currentBuildVersion string, profile system.PlatformProfile) UpdateResult {
	result := UpdateResult{Tool: tool}
	homebrewOwnership := HomebrewNone
	if profile.PackageManager == "brew" {
		var err error
		homebrewOwnership, err = homebrewOwnershipDetector(tool.Name)
		if err != nil {
			result.Status = CheckFailed
			result.Err = fmt.Errorf("detect Homebrew ownership for %s: %w; inspect `brew list --formula --full-name`, `brew list --cask --full-name`, and `command -v %s`", tool.Name, err, tool.Name)
			return result
		}
	}

	// The advertisement must name a target the effective installer can deliver.
	// A brew-owned install only ever receives the tap's stable formula, so a
	// main-head beta target is advertised only when Homebrew does not own the
	// tool (issue #2323: checker advertised main@sha while the instruction
	// installed stable).
	betaMainHead := usesBetaMainHeadCheck(tool, currentBuildVersion) && homebrewOwnership == HomebrewNone

	// Run local detection and remote fetch concurrently.
	var wg sync.WaitGroup
	var localVersion string
	var release githubRelease
	var mainCommit githubCommit
	var fetchErr error

	wg.Add(2)

	go func() {
		defer wg.Done()
		localVersion = detectInstalledVersion(ctx, tool, currentBuildVersion)
	}()

	go func() {
		defer wg.Done()
		if betaMainHead {
			mainCommit, fetchErr = fetchMainCommit(ctx, tool.Owner, tool.Repo)
			return
		}
		release, fetchErr = fetchLatestReleaseForTool(ctx, tool)
	}()

	wg.Wait()

	result.InstalledVersion = localVersion
	result.UpdateHint = updateHintForOwnership(tool, profile, homebrewOwnership)

	// Handle fetch failure.
	if fetchErr != nil {
		result.Err = fetchErr
		result.Status = CheckFailed
		if betaMainHead {
			result.UpdateHint = "Beta target metadata is unavailable; rerun the update check."
		}
		return result
	}

	if betaMainHead {
		if !validBetaCommit(mainCommit.SHA) {
			result.Status = CheckFailed
			result.UpdateHint = "Beta target metadata is unavailable; rerun the update check."
			result.Err = fmt.Errorf("invalid beta main commit SHA")
			return result
		}
		module, err := fetchBetaModulePath(ctx, tool.Owner, tool.Repo, mainCommit.SHA)
		if err != nil {
			result.Status = CheckFailed
			result.UpdateHint = "Beta target metadata is unavailable; rerun the update check."
			result.Err = err
			return result
		}
		result.BetaCommit = strings.ToLower(mainCommit.SHA)
		result.BetaModulePath = module
		return applyBetaMainHeadStatus(ctx, result, localVersion, mainCommit)
	}

	if profile.OS == "windows" && isGentleAIRepo(tool) && homebrewOwnership == HomebrewNone {
		// A newly migrated source binary may be ahead of the published module major.
		// Use the exact validated tag so the hint cannot resolve an unreleased @latest.
		tag := release.TagName
		if tag == "" || tag != plainReleaseTag(tag) || parseMajorVersion(tag) < 0 {
			result.Status = CheckFailed
			result.Err = fmt.Errorf("stable release tag is missing or invalid; rerun the update check")
			return result
		}
		result.UpdateHint = WindowsDistributionHoldMessage + " Install/update from source with Go 1.25.10+: " + GentleAISourceInstallCommand(tag)
	}
	result.LatestVersion = normalizeVersion(release.TagName)
	result.ReleaseURL = release.HTMLURL

	// Determine status based on local version.
	if localVersion == "" {
		if tool.DetectCmd == nil {
			// gentle-ai with no build version (shouldn't happen, but handle gracefully).
			result.Status = VersionUnknown
		} else {
			// Binary not found on PATH.
			if _, err := lookPath(tool.DetectCmd[0]); err != nil {
				result.Status = NotInstalled
			} else {
				result.Status = VersionUnknown
			}
		}
		return result
	}

	// Check for non-semver local versions.
	// "dev" is a well-known sentinel for source-built binaries — report as DevBuild
	// so the upgrade executor knows to skip this tool without treating it as an error.
	normalizedLocal := normalizeVersion(localVersion)
	if normalizedLocal == "dev" {
		result.Status = DevBuild
		return result
	}
	if !isSemver(normalizedLocal) {
		result.Status = VersionUnknown
		return result
	}

	// Compare versions.
	result.Status = compareVersions(normalizedLocal, result.LatestVersion)
	return result
}

func usesBetaMainHeadCheck(tool ToolInfo, currentVersion string) bool {
	return isGentleAIRepo(tool) && (isBetaUpdateChannel() || isGoPseudoVersionWithCommit(currentVersion))
}

func isGentleAIRepo(tool ToolInfo) bool {
	return tool.Name == "gentle-ai" && strings.EqualFold(tool.Owner, "Gentleman-Programming") && tool.Repo == "gentle-ai"
}

func isBetaUpdateChannel() bool {
	switch strings.ToLower(strings.TrimSpace(updateChannelEnv("GENTLE_AI_CHANNEL"))) {
	case "beta", "nightly":
		return true
	default:
		return false
	}
}

func applyBetaMainHeadStatus(ctx context.Context, result UpdateResult, localVersion string, commit githubCommit) UpdateResult {
	remoteSHA := strings.TrimSpace(commit.SHA)
	shortRemote := shortCommit(remoteSHA)
	if shortRemote == "" {
		result.Status = VersionUnknown
		return result
	}

	result.LatestVersion = "main@" + shortRemote
	result.ReleaseURL = strings.TrimSpace(commit.HTMLURL)
	// Compose the fallback from the verified module and full checked commit.
	// A stable-channel hint or mutable @main would not deliver this target.
	result.UpdateHint = BetaSourceInstallCommand(result.BetaModulePath, result.BetaCommit)

	if strings.TrimSpace(localVersion) == "" {
		result.Status = VersionUnknown
		return result
	}
	if strings.TrimSpace(localVersion) == "dev" {
		result.Status = DevBuild
		return result
	}

	localSHA := localBuildCommit(localVersion)
	tag := plainReleaseTag(localVersion)
	if tag != "" {
		// Resolve the tag to its commit, not to an annotated tag object or a
		// same-named branch. The validated main target remains usable as a
		// manual fallback when we cannot establish the installed commit.
		tagCommit, err := fetchCommit(ctx, result.Tool.Owner, result.Tool.Repo, "refs/tags/"+tag)
		if err == nil && validBetaCommit(tagCommit.SHA) {
			localSHA = tagCommit.SHA
		}
	}
	if localSHA == "" {
		result.Status = VersionUnknown
		return result
	}

	if sameCommitPrefix(localSHA, remoteSHA) {
		result.Status = UpToDate
		return result
	}

	if tag != "" {
		if reason := betaReleaseBlockReason(ctx, result.Tool, strings.ToLower(localSHA), result.BetaCommit); reason != "" {
			result.Status = VersionUnknown
			result.UpdateHint = reason
			return result
		}
	} else if localTime, ok := pseudoVersionTime(localVersion); ok {
		// Preserve the pseudo-version ordering guard (issue #2319); unknown
		// dates fail open here, not for resolved release tags.
		remoteTime := commit.Commit.Committer.Date
		if !remoteTime.IsZero() && localTime.After(remoteTime) {
			result.Status = UpToDate
			return result
		}
	}

	result.Status = UpdateAvailable
	result.ReleaseURL = fmt.Sprintf("https://github.com/%s/%s/compare/%s...%s", result.Tool.Owner, result.Tool.Repo, shortCommit(localSHA), shortRemote)
	return result
}

// betaReleaseBlockReason requires evidence of strict advancement before offering
// a release-to-main update. An unresolved tag is handled before this comparison.
func betaReleaseBlockReason(ctx context.Context, tool ToolInfo, localSHA, mainSHA string) string {
	comparison, err := compareCommits(ctx, tool.Owner, tool.Repo, localSHA, mainSHA)
	if err == nil {
		switch comparison.Status {
		case "ahead":
			if comparison.AheadBy > 0 && comparison.BehindBy != nil && *comparison.BehindBy == 0 {
				return ""
			}
		case "behind":
			return "Beta main is behind the installed release; keep the installed binary."
		case "diverged":
			return "Beta main has diverged from the installed release; keep the installed binary."
		}
	}
	return "Could not verify that beta main advances the installed release; keep the installed binary."
}

// pseudoVersionTime extracts the UTC timestamp a Go pseudo-version embeds
// (vX.Y.Z-0.yyyymmddhhmmss-abcdefabcdef). It reports false for any version
// that does not carry a well-formed 14-digit timestamp.
func pseudoVersionTime(version string) (time.Time, bool) {
	if !isGoPseudoVersionWithCommit(version) {
		return time.Time{}, false
	}
	parts := strings.Split(strings.TrimSpace(version), "-")
	timestampPart := parts[len(parts)-2]
	if idx := strings.LastIndex(timestampPart, "."); idx >= 0 {
		timestampPart = timestampPart[idx+1:]
	}
	ts, err := time.Parse("20060102150405", timestampPart)
	if err != nil {
		return time.Time{}, false
	}
	return ts.UTC(), true
}

var plainReleasePattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

func plainReleaseTag(version string) string {
	version = strings.TrimSpace(version)
	if !plainReleasePattern.MatchString(version) {
		return ""
	}
	return "v" + strings.TrimPrefix(version, "v")
}

func localBuildCommit(version string) string {
	parts := strings.Split(strings.TrimSpace(version), "-")
	if len(parts) == 0 {
		return ""
	}
	candidate := parts[len(parts)-1]
	if len(candidate) < 7 {
		return ""
	}
	for _, r := range candidate {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return ""
		}
	}
	return strings.ToLower(candidate)
}

func isGoPseudoVersionWithCommit(version string) bool {
	version = strings.TrimSpace(version)
	if localBuildCommit(version) == "" {
		return false
	}

	parts := strings.Split(version, "-")
	if len(parts) < 3 {
		return false
	}
	if !versionRegexp.MatchString(parts[0]) {
		return false
	}

	timestampPart := parts[len(parts)-2]
	if idx := strings.LastIndex(timestampPart, "."); idx >= 0 {
		timestampPart = timestampPart[idx+1:]
	}
	if len(timestampPart) != 14 {
		return false
	}
	for _, r := range timestampPart {
		if r < '0' || r > '9' {
			return false
		}
	}

	return true
}

func sameCommitPrefix(local, remote string) bool {
	local = strings.ToLower(strings.TrimSpace(local))
	remote = strings.ToLower(strings.TrimSpace(remote))
	if len(local) < 7 || len(remote) < 7 {
		return false
	}
	return strings.HasPrefix(remote, local) || strings.HasPrefix(local, remote)
}

func shortCommit(sha string) string {
	sha = strings.ToLower(strings.TrimSpace(sha))
	if len(sha) < 12 {
		return sha
	}
	return sha[:12]
}

func fetchLatestReleaseForTool(ctx context.Context, tool ToolInfo) (githubRelease, error) {
	if pattern := strings.TrimSpace(tool.ReleaseTagPattern); pattern != "" {
		return fetchLatestReleaseMatchingPattern(ctx, tool.Owner, tool.Repo, pattern)
	}
	return fetchLatestRelease(ctx, tool.Owner, tool.Repo)
}

// normalizeVersion strips a leading "v" and extracts a semver pattern.
func normalizeVersion(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "v")

	match := versionRegexp.FindStringSubmatch(raw)
	if len(match) >= 2 {
		return match[1]
	}

	return raw
}

// isSemver checks if a version string looks like a semver (N.N or N.N.N).
func isSemver(v string) bool {
	return versionRegexp.MatchString(v)
}

// compareVersions returns UpToDate if local >= remote, UpdateAvailable otherwise.
func compareVersions(local, remote string) UpdateStatus {
	localParts := parseVersionParts(local)
	remoteParts := parseVersionParts(remote)

	for i := 0; i < 3; i++ {
		if localParts[i] > remoteParts[i] {
			return UpToDate
		}
		if localParts[i] < remoteParts[i] {
			return UpdateAvailable
		}
	}

	return UpToDate // equal
}

// parseVersionParts splits "1.2.3" into [1, 2, 3], padding with zeros.
// Same logic as internal/system/deps.go:parseVersionParts.
func parseVersionParts(version string) [3]int {
	parts := strings.SplitN(version, ".", 3)
	var result [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		n, _ := strconv.Atoi(parts[i])
		result[i] = n
	}
	return result
}
