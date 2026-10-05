package opencode

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
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

type VersionCommandRunner func(context.Context, Command) (CommandOutput, error)

// VersionRunnerOverride is a process seam for isolated tests, like adapter
// LookPathOverride. Production probes only --version with bounded output/time.
var VersionRunnerOverride VersionCommandRunner = runVersionCommand

func runVersionCommand(ctx context.Context, command Command) (CommandOutput, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, command.Path, command.Args...)
	cmd.Dir = command.Dir
	if command.Env != nil {
		cmd.Env = command.Env
	}
	stdout, stderr := &limitedBuffer{limit: command.OutputLimit, cancel: cancel}, &limitedBuffer{limit: command.OutputLimit, cancel: cancel}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	if stdout.overflow || stderr.overflow {
		return CommandOutput{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, &CatalogError{Kind: CatalogErrorOutputTooLarge}
	}
	return CommandOutput{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, err
}

type limitedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
	cancel   context.CancelFunc
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	remaining := b.limit - b.buffer.Len()
	if len(data) > remaining {
		if remaining > 0 {
			_, _ = b.buffer.Write(data[:remaining])
		}
		b.overflow = true
		b.cancel()
		return len(data), nil
	}
	return b.buffer.Write(data)
}

func (b *limitedBuffer) Bytes() []byte { return b.buffer.Bytes() }

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
