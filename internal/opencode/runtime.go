package opencode

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
)

// RuntimeMajor is executable evidence, never inferred from user config keys.
type RuntimeMajor int

const (
	RuntimeUnknown RuntimeMajor = iota
	RuntimeV1
	RuntimeV2
)

var stableRuntimeVersion = regexp.MustCompile(`^(?:opencode |OpenCode )?v?([12])\.[0-9]+\.[0-9]+$`)

func ParseRuntimeMajor(version string) RuntimeMajor {
	match := stableRuntimeVersion.FindStringSubmatch(strings.TrimSpace(version))
	if len(match) != 2 {
		return RuntimeUnknown
	}
	if match[1] == "2" {
		return RuntimeV2
	}
	return RuntimeV1
}

// VersionRunnerOverride is a process seam for isolated tests, like adapter
// LookPathOverride. Production probes only --version with bounded output/time.
var VersionRunnerOverride CommandRunner = runCatalogCommand

func DetectRuntimeMajor(ctx context.Context) (RuntimeMajor, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	output, err := VersionRunnerOverride(ctx, Command{Path: "opencode", Args: []string{"--version"}, OutputLimit: 4096})
	if err == nil && len(output.Stdout) <= 4096 && len(output.Stderr) <= 4096 {
		if major := ParseRuntimeMajor(string(output.Stdout)); major != RuntimeUnknown {
			return major, nil
		}
	}
	return RuntimeUnknown, errors.New("OpenCode runtime version unavailable or unsupported; managed runtime assets were not selected")
}
func (major RuntimeMajor) PluginAssetDirectory() (string, error) {
	switch major {
	case RuntimeV1:
		return "opencode/plugins/", nil
	case RuntimeV2:
		return "opencode/plugins-v2/", nil
	}
	return "", errors.New("OpenCode runtime major is unknown")
}
func (major RuntimeMajor) PluginDependency() (string, error) {
	switch major {
	case RuntimeV1:
		return "@opencode-ai/plugin@latest", nil
	case RuntimeV2:
		return "@opencode/plugin@2.0.4", nil
	}
	return "", errors.New("OpenCode runtime major is unknown")
}
