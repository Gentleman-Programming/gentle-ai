package update

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
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
	if profile.PackageManager == "brew" && strings.TrimSpace(tool.NpmPackage) == "" {
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
	var pluginRegistered bool
	var release githubRelease
	var mainCommit githubCommit
	var fetchErr error

	wg.Add(2)

	go func() {
		defer wg.Done()
		if strings.TrimSpace(tool.NpmPackage) != "" {
			localVersion, pluginRegistered = detectOpenCodePluginPackage(tool.NpmPackage)
			return
		}
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
		return result
	}

	if betaMainHead {
		return applyBetaMainHeadStatus(result, localVersion, mainCommit)
	}

	result.LatestVersion = normalizeVersion(release.TagName)
	result.ReleaseURL = release.HTMLURL

	// Determine status based on local version.
	if localVersion == "" {
		if strings.TrimSpace(tool.NpmPackage) != "" {
			if pluginRegistered {
				result.Status = RegisteredNotMaterialized
				result.UpdateHint = openCodeRegisteredNotMaterializedHint(tool)
				return result
			}
			result.Status = NotInstalled
			return result
		}
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

func applyBetaMainHeadStatus(result UpdateResult, localVersion string, commit githubCommit) UpdateResult {
	remoteSHA := strings.TrimSpace(commit.SHA)
	shortRemote := shortCommit(remoteSHA)
	if shortRemote == "" {
		result.Status = VersionUnknown
		return result
	}

	result.LatestVersion = "main@" + shortRemote
	result.ReleaseURL = strings.TrimSpace(commit.HTMLURL)
	// Derive the instruction from the advertised target: the only installer
	// that delivers main@<sha> is `go install ...@main`. The per-OS stable
	// hint would silently replace this beta build with the latest stable
	// release (issue #2323).
	result.UpdateHint = GentleAISourceInstallCommand(result.LatestVersion)

	if strings.TrimSpace(localVersion) == "" {
		result.Status = VersionUnknown
		return result
	}
	if strings.TrimSpace(localVersion) == "dev" {
		result.Status = DevBuild
		return result
	}

	localSHA := localBuildCommit(localVersion)
	if localSHA == "" {
		result.Status = VersionUnknown
		return result
	}

	if sameCommitPrefix(localSHA, remoteSHA) {
		result.Status = UpToDate
		return result
	}

	// Ordering guard: a prefix mismatch alone does not mean the local build is
	// behind main. When the local pseudo-version timestamp is newer than the
	// fetched commit's date, offering an "update" advertises a downgrade
	// (issue #2319 offer half). Unknown dates fail open and keep the offer.
	if localTime, ok := pseudoVersionTime(localVersion); ok {
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
