package reviewerprovider

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
)

// ClaudeAdapter invokes Claude Code with an opaque provider invocation and
// returns its stdout bytes without interpreting them.
type ClaudeAdapter struct {
	LookPath       func(string) (string, error)
	commandContext func(context.Context, string, ...string) *exec.Cmd
}

// NewClaudeAdapter returns an adapter using the Claude Code binary from PATH.
func NewClaudeAdapter() *ClaudeAdapter {
	return &ClaudeAdapter{LookPath: exec.LookPath, commandContext: exec.CommandContext}
}

// Review runs Claude Code in an empty temporary directory. The provider
// material is delivered through stdin so command arguments stay opaque.
func (adapter *ClaudeAdapter) Review(ctx context.Context, invocation Invocation) ([]byte, error) {
	lookPath := adapter.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	binary, err := lookPath("claude")
	if err != nil {
		return nil, fmt.Errorf("claude reviewer transport unavailable: %w", err)
	}

	scratch, err := os.MkdirTemp("", "gentle-ai-claude-reviewer-*")
	if err != nil {
		return nil, fmt.Errorf("claude reviewer transport unavailable: create scratch directory: %w", err)
	}
	defer os.RemoveAll(scratch)

	commandContext := adapter.commandContext
	if commandContext == nil {
		commandContext = exec.CommandContext
	}
	command := commandContext(ctx, binary,
		"--bare", "--print", "--output-format", "text", "--tools", "",
		"--permission-mode", "dontAsk", "--no-session-persistence",
	)
	command.Dir = scratch
	command.Stdin = bytes.NewReader(invocation.Prompt())
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("claude reviewer transport failed: %w: %s", err, stderr.String())
	}
	return stdout.Bytes(), nil
}
