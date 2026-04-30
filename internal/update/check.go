package update

import (
	"context"
	"strconv"
	"strings"
	"sync"

	"github.com/gentleman-programming/gentle-ai/internal/system"
)

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

	// Run local detection and remote fetch concurrently.
	var wg sync.WaitGroup
	var localVersion string
	var pluginRegistered bool
	var release githubRelease
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
		release, fetchErr = fetchLatestRelease(ctx, tool.Owner, tool.Repo)
	}()

	wg.Wait()

	result.InstalledVersion = localVersion
	result.UpdateHint = updateHint(tool, profile)

	// Handle fetch failure.
	if fetchErr != nil {
		result.Err = fetchErr
		result.Status = CheckFailed
		return result
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
