package system

import (
	"runtime"
	"sort"
	"strings"
)

var hazardousEnvKeys = map[string]bool{
	"NODE_OPTIONS":   true,
	"NODE_PATH":      true,
	"GIT_DIR":        true,
	"GIT_WORK_TREE":  true,
	"GIT_INDEX_FILE": true,
}

var windowsEssentialEnvKeys = map[string]bool{
	"COMSPEC":          true,
	"PATH":             true,
	"PATHEXT":          true,
	"SYSTEMDRIVE":      true,
	"SYSTEMROOT":       true,
	"TEMP":             true,
	"TMP":              true,
	"TMPDIR":           true,
	"WINDIR":           true,
	"USERPROFILE":      true,
	"APPDATA":          true,
	"LOCALAPPDATA":     true,
	"ALLUSERSPROFILE":  true,
	"PROGRAMDATA":      true,
	"PROGRAMFILES":     true,
	"PROGRAMFILES(X86)": true,
}

// SanitizeCommandEnvironment sanitizes an environment slice for command execution.
// It deduplicates keys case-insensitively, strips hazardous environment variables
// (NODE_OPTIONS, NODE_PATH, GIT_DIR, GIT_WORK_TREE, GIT_INDEX_FILE), retains Windows
// essential variables and caller-requested pass-through keys, bidirectionally synchronizes
// HOME and USERPROFILE on Windows, and merges overrides with case-insensitive replacement.
func SanitizeCommandEnvironment(base []string, passThroughKeys []string, overrides map[string]string) []string {
	return SanitizeCommandEnvironmentForOS(base, passThroughKeys, overrides, runtime.GOOS)
}

// SanitizeCommandEnvironmentForOS implements SanitizeCommandEnvironment parameterized by target OS.
func SanitizeCommandEnvironmentForOS(base []string, passThroughKeys []string, overrides map[string]string, goos string) []string {
	isWindows := goos == "windows"

	passThroughSet := make(map[string]bool, len(passThroughKeys))
	for _, k := range passThroughKeys {
		passThroughSet[strings.ToUpper(strings.TrimSpace(k))] = true
	}
	hasPassThroughFilter := passThroughKeys != nil

	order := make([]string, 0, len(base)+len(overrides))
	entries := make(map[string]string)

	for _, entry := range base {
		idx := strings.Index(entry, "=")
		if idx <= 0 {
			continue
		}
		rawKey := entry[:idx]
		val := entry[idx+1:]
		upperKey := strings.ToUpper(rawKey)

		if hazardousEnvKeys[upperKey] {
			continue
		}

		if hasPassThroughFilter {
			allowed := passThroughSet[upperKey] || (isWindows && windowsEssentialEnvKeys[upperKey])
			if !allowed {
				continue
			}
		}

		if _, exists := entries[upperKey]; !exists {
			order = append(order, upperKey)
		}
		entries[upperKey] = rawKey + "=" + val
	}

	if len(overrides) > 0 {
		sortedKeys := make([]string, 0, len(overrides))
		for k := range overrides {
			sortedKeys = append(sortedKeys, k)
		}
		sort.Strings(sortedKeys)

		for _, k := range sortedKeys {
			upperKey := strings.ToUpper(k)
			if hazardousEnvKeys[upperKey] {
				continue
			}
			if _, exists := entries[upperKey]; !exists {
				order = append(order, upperKey)
			}
			entries[upperKey] = k + "=" + overrides[k]
		}
	}

	if isWindows {
		homeVal, hasHome := getEntryValue(entries, "HOME")
		userProfileVal, hasUserProfile := getEntryValue(entries, "USERPROFILE")

		if hasUserProfile && !hasHome {
			order = append(order, "HOME")
			entries["HOME"] = "HOME=" + userProfileVal
		} else if hasHome && !hasUserProfile {
			order = append(order, "USERPROFILE")
			entries["USERPROFILE"] = "USERPROFILE=" + homeVal
		}
	}

	result := make([]string, 0, len(order))
	for _, upperKey := range order {
		if val, ok := entries[upperKey]; ok {
			result = append(result, val)
		}
	}
	return result
}

func getEntryValue(entries map[string]string, upperKey string) (string, bool) {
	entry, ok := entries[upperKey]
	if !ok {
		return "", false
	}
	idx := strings.Index(entry, "=")
	if idx < 0 {
		return "", false
	}
	return entry[idx+1:], true
}
