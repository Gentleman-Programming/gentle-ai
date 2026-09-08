package reviewerprovider

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"
)

// piReviewerWaitDelay forcibly releases Wait shortly after a context kill so
// a grandchild holding the inherited stdout pipe cannot outlive the deadline.
const piReviewerWaitDelay = 5 * time.Second

// PiAdapter invokes a brand-new print-mode pi process with an opaque provider
// invocation and returns its raw final bytes without interpreting them.
type PiAdapter struct {
	LookPath       func(string) (string, error)
	commandContext func(context.Context, string, ...string) *exec.Cmd
}

// NewPiAdapter returns an adapter using the pi binary resolved from PATH.
func NewPiAdapter() *PiAdapter {
	return &PiAdapter{LookPath: exec.LookPath, commandContext: exec.CommandContext}
}

// Review runs pi in an empty temporary directory with every discovery surface
// disabled. The prompt is delivered through stdin so command arguments never
// carry provider material.
func (adapter *PiAdapter) Review(ctx context.Context, invocation Invocation) ([]byte, error) {
	lookPath := adapter.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	binary, err := lookPath("pi")
	if err != nil {
		return nil, fmt.Errorf("pi reviewer transport unavailable: %w", err)
	}
	scratch, err := os.MkdirTemp("", "gentle-ai-pi-reviewer-*")
	if err != nil {
		return nil, fmt.Errorf("pi reviewer transport unavailable: create scratch directory: %w", err)
	}
	defer os.RemoveAll(scratch)
	commandContext := adapter.commandContext
	if commandContext == nil {
		commandContext = exec.CommandContext
	}
	command := commandContext(ctx, binary,
		"--print", "--mode", "text", "--no-session", "--no-tools", "--no-extensions",
		"--no-skills", "--no-prompt-templates", "--no-themes", "--no-context-files", "--no-approve")
	command.Dir = scratch
	command.Env = piRuntimeEnvironment()
	command.WaitDelay = piReviewerWaitDelay
	command.Stdin = bytes.NewReader(invocation.Prompt())
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		// A deadline kill reads as the typed context error, not "signal: killed".
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return nil, fmt.Errorf("pi reviewer transport failed: %w: %s", err, reviewerTransportFailureDetail(stderr.String(), stdout.String()))
	}
	if len(bytes.TrimSpace(stdout.Bytes())) == 0 {
		return nil, errors.New("pi reviewer transport produced no final message")
	}
	return stdout.Bytes(), nil
}

// piRuntimeEnvironment passes only the process locators Pi needs to launch and
// resolve its local configuration. Credentials remain in Pi's auth file.
func piRuntimeEnvironment() []string {
	environment := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
	}
	if runtime.GOOS == "windows" {
		environment = appendPiRuntimeEnvironmentValue(environment, "SYSTEMROOT")
		environment = appendPiRuntimeEnvironmentValue(environment, "USERPROFILE")
		environment = appendPiRuntimeEnvironmentValue(environment, "HOMEDRIVE")
		environment = appendPiRuntimeEnvironmentValue(environment, "HOMEPATH")
	}
	return appendPiRuntimeEnvironmentValue(environment, "PI_CODING_AGENT_DIR")
}

func appendPiRuntimeEnvironmentValue(environment []string, name string) []string {
	if value := os.Getenv(name); value != "" {
		return append(environment, name+"="+value)
	}
	return environment
}
