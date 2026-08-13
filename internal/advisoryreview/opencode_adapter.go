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

// OpenCodeAdapter is the thin invocation wiring for the OpenCode runtime
// (rdd-advisory-transport SKILL.md: "Adapters only invoke the reviewer and
// return raw bytes plus a transport error"). It renders nothing, parses
// nothing, and validates nothing: Prompt and Validate own every one of those
// responsibilities. Its only job is handing a provider-rendered prompt to a
// real `opencode run` process and returning that process's raw final message
// unmodified.
//
// Invoke runs opencode in a brand-new, empty directory it creates and
// deletes itself, never the caller's repository or any path the caller
// names: the prompt text is the reviewer's only legitimate input, exactly as
// the shared advisory contract requires (organic proof:
// TestRealOpenCodeReviewerOrdinarySessionInjectsFrozenContextAndAdmitsRawOutput,
// e2e/organicruntime).
type OpenCodeAdapter struct {
	// LookPath resolves the opencode binary. Overridable so a test can prove
	// the typed-unavailable path without requiring a real installation.
	LookPath func(string) (string, error)
	// PromptFor renders the canonical provider prompt for a request. It is a
	// seam, not a re-implementation: the adapter never carries its own copy
	// of the reviewer input contract.
	PromptFor func(context.Context, Request) ([]byte, error)
}

// NewOpenCodeAdapter returns an OpenCodeAdapter wired to the real opencode
// binary on PATH and the canonical provider renderer.
func NewOpenCodeAdapter() *OpenCodeAdapter {
	return &OpenCodeAdapter{LookPath: exec.LookPath, PromptFor: NewProvider().PromptFor}
}

// Invoke renders the request through the injected PromptFor seam and invokes
// `opencode run` with that prompt, returning its raw final message. Every
// returned error is a transport failure -- unavailable binary, render
// failure, scratch-directory setup, non-zero exit, or a canceled/expired
// context -- never a verdict about the reviewer's content. Only Validate may
// render that verdict, and only once this adapter has hung up (SEN-RPC-5).
func (adapter *OpenCodeAdapter) Invoke(ctx context.Context, r Request) ([]byte, error) {
	lookPath := adapter.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	binary, err := lookPath("opencode")
	if err != nil {
		return nil, fmt.Errorf("opencode advisory transport unavailable: %w", err)
	}

	promptFor := adapter.PromptFor
	if promptFor == nil {
		promptFor = NewProvider().PromptFor
	}
	prompt, err := promptFor(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("opencode advisory transport unavailable: render prompt: %w", err)
	}

	scratch, err := os.MkdirTemp("", "gentle-ai-opencode-advisory-*")
	if err != nil {
		return nil, fmt.Errorf("opencode advisory transport unavailable: create scratch directory: %w", err)
	}
	defer os.RemoveAll(scratch)

	command := exec.CommandContext(ctx, binary, "run", string(prompt))
	// Dir is the OS-level process boundary this adapter independently
	// guarantees: the child runs in the same empty scratch directory the
	// adapter created, never the caller's repository.
	command.Dir = scratch
	// Env is an explicit allowlist, not a security boundary: authority over
	// what a runtime may do lives in Go admission, never in what environment
	// variables happen to reach a transport subprocess. opencodeAdvisoryEnvironment
	// carries only PATH (so opencode can start) and OPENCODE_CONFIG_DIR (so
	// it can still read its own config without inheriting the caller's),
	// nothing else.
	command.Env = opencodeAdvisoryEnvironment()
	command.Stdin = bytes.NewReader(nil)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		return nil, fmt.Errorf("opencode advisory transport failed: %w: %s", err, detail)
	}

	return stdout.Bytes(), nil
}

// opencodeAdvisoryEnvironment returns the minimal explicit environment
// allowlist for the opencode child process: PATH and OPENCODE_CONFIG_DIR,
// nothing else. Everything the parent process happens to carry -- PWD in
// particular, which would otherwise still name the real worktree even though
// Dir points opencode at an empty scratch directory -- is dropped by
// construction, since exec.Cmd.Env, once non-nil, replaces rather than
// extends the inherited environment.
func opencodeAdvisoryEnvironment() []string {
	env := make([]string, 0, 2)
	if path, ok := os.LookupEnv("PATH"); ok {
		env = append(env, "PATH="+path)
	}
	if configDir, ok := os.LookupEnv("OPENCODE_CONFIG_DIR"); ok {
		env = append(env, "OPENCODE_CONFIG_DIR="+configDir)
	} else if home, err := os.UserHomeDir(); err == nil {
		env = append(env, "OPENCODE_CONFIG_DIR="+filepath.Join(home, ".config", "opencode"))
	}
	return env
}

var _ Invoker = (*OpenCodeAdapter)(nil)
