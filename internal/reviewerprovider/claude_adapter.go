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

// claudeReviewerArguments locks the fresh-reviewer boundary without `--bare`:
// bare mode reads auth strictly from ANTHROPIC_API_KEY or an apiKeyHelper and
// never reads OAuth or keychain credentials, so every capture on a
// subscription-authenticated machine failed with "Not logged in". The same
// boundary is rebuilt from flags that keep ordinary credential resolution:
// `--setting-sources ""` drops user/project settings (hooks, plugins,
// permissions), the explicit `--system-prompt` replaces default context
// assembly including CLAUDE.md memory, `--tools ""` leaves no live tools, and
// the empty scratch working directory carries no project state.
var claudeReviewerArguments = []string{
	"--print", "--output-format", "text", "--tools", "",
	"--permission-mode", "dontAsk", "--no-session-persistence",
	"--setting-sources", "",
	"--system-prompt", "You are an isolated code reviewer process. Your only instructions are those delivered in the user message.",
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
	command := commandContext(ctx, binary, claudeReviewerArguments...)
	command.Dir = scratch
	command.Stdin = bytes.NewReader(invocation.Prompt())
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("claude reviewer transport failed: %w: %s", err, reviewerTransportFailureDetail(stderr.String(), stdout.String()))
	}
	return stdout.Bytes(), nil
}
