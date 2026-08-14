package advisoryreview

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ClaudeAdapter is the thin invocation wiring for the Claude Code runtime
// (rdd-advisory-transport SKILL.md: "Adapters only invoke the reviewer and
// return raw bytes plus a transport error"). It renders nothing, parses
// nothing, and validates nothing: Prompt and Validate own every one of those
// responsibilities. Its only job is handing a provider-rendered prompt to a
// real `claude --print` process and returning that process's raw final
// message unmodified.
//
// Invoke runs claude in a brand-new, empty directory it creates and deletes
// itself, never the caller's repository or any path the caller names, with
// every built-in tool disabled: the prompt text is the reviewer's only
// legitimate input, exactly as the shared advisory contract requires
// (organic proof: TestRealClaudeReviewerOrdinarySessionAdmitsRawOutput,
// e2e/organicruntime).
type ClaudeAdapter struct {
	// LookPath resolves the claude binary. Overridable so a test can prove
	// the typed-unavailable path without requiring a real installation.
	LookPath func(string) (string, error)
	// PromptFor renders the canonical provider prompt for a request. It is a
	// seam, not a re-implementation: the adapter never carries its own copy
	// of the reviewer input contract.
	PromptFor func(context.Context, Request) ([]byte, error)
}

// NewClaudeAdapter returns a ClaudeAdapter wired to the real claude binary
// on PATH and the canonical provider renderer.
func NewClaudeAdapter() *ClaudeAdapter {
	return &ClaudeAdapter{LookPath: exec.LookPath, PromptFor: NewProvider().PromptFor}
}

// Invoke renders the request through the injected PromptFor seam and invokes
// claude non-interactively with that prompt on stdin, returning its raw final
// message. Every returned error is a transport failure -- unavailable binary,
// render failure, scratch-directory setup, non-zero exit, or a
// canceled/expired context -- never a verdict about the reviewer's content.
// Only Validate may render that verdict, and only once this adapter has hung
// up (SEN-RPC-5).
func (adapter *ClaudeAdapter) Invoke(ctx context.Context, r Request) ([]byte, error) {
	lookPath := adapter.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	binary, err := lookPath("claude")
	if err != nil {
		return nil, fmt.Errorf("claude advisory transport unavailable: %w", err)
	}

	promptFor := adapter.PromptFor
	if promptFor == nil {
		promptFor = NewProvider().PromptFor
	}
	prompt, err := promptFor(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("claude advisory transport unavailable: render prompt: %w", err)
	}

	scratch, err := os.MkdirTemp("", "gentle-ai-claude-advisory-*")
	if err != nil {
		return nil, fmt.Errorf("claude advisory transport unavailable: create scratch directory: %w", err)
	}
	defer os.RemoveAll(scratch)

	command := exec.CommandContext(ctx, binary,
		"--bare",
		"--print",
		"--tools", "",
		"--permission-mode", "dontAsk",
		"--setting-sources=",
		"--strict-mcp-config",
		"--disable-slash-commands",
		"--no-chrome",
		"--no-session-persistence",
		"--prompt-suggestions=false",
	)
	// Dir is the OS-level process boundary this adapter independently
	// guarantees: the child runs in the same empty scratch directory the
	// adapter created, never the caller's repository.
	command.Dir = scratch
	// Env is an explicit allowlist, not a security boundary: authority over
	// what a runtime may do lives in Go admission, never in what environment
	// variables happen to reach a transport subprocess. claudeAdvisoryEnvironment
	// carries only PATH (so claude can start), HOME (so it can resolve its
	// own auth paths), and CLAUDE_CONFIG_DIR (so it can read its own ~/.claude
	// config), nothing else.
	command.Env = claudeAdvisoryEnvironment()
	command.Stdin = bytes.NewReader(prompt)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		return nil, fmt.Errorf("claude advisory transport failed: %w: %s", err, detail)
	}

	return stdout.Bytes(), nil
}

// claudeAdvisoryEnvironment returns the minimal explicit environment
// allowlist for the claude child process: PATH, HOME, and CLAUDE_CONFIG_DIR,
// nothing else. Everything the parent process happens to carry -- PWD in
// particular, which would otherwise still name the real worktree even though
// Dir points claude at an empty scratch directory -- is dropped by
// construction, since exec.Cmd.Env, once non-nil, replaces rather than
// extends the inherited environment.
func claudeAdvisoryEnvironment() []string {
	env := make([]string, 0, 3)
	if path, ok := os.LookupEnv("PATH"); ok {
		env = append(env, "PATH="+path)
	}
	// HOME carries the Claude credentials store (~/.claude/.credentials.json);
	// CLAUDE_CONFIG_DIR covers config but not the auth path the child still
	// resolves from HOME.
	if home, err := os.UserHomeDir(); err == nil {
		env = append(env, "HOME="+home)
	}
	if configDir, ok := os.LookupEnv("CLAUDE_CONFIG_DIR"); ok {
		env = append(env, "CLAUDE_CONFIG_DIR="+configDir)
	} else if home, err := os.UserHomeDir(); err == nil {
		env = append(env, "CLAUDE_CONFIG_DIR="+filepath.Join(home, ".claude"))
	}
	return env
}

var _ Invoker = (*ClaudeAdapter)(nil)
