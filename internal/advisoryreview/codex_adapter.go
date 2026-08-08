package advisoryreview

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// CodexAdapter is the thin invocation wiring for the Codex runtime
// (rdd-advisory-transport SKILL.md: "Adapters only invoke the reviewer and
// return raw bytes plus a transport error"). It renders nothing, parses
// nothing, and validates nothing: Prompt and Validate above own every one of
// those responsibilities. Its only job is handing a provider-rendered prompt
// to a real `codex exec` process and returning that process's raw final
// message unmodified.
//
// Review runs codex in a brand-new, empty directory it creates and deletes
// itself, never the caller's repository or any path the caller names.
// codex exec's own shell tool is still permitted to read files even under
// --sandbox read-only (that flag bounds writes and network, not reads), so an
// adapter that pointed codex at a live or "sandboxed" repository worktree
// could let the reviewer see live or poisoned content the frozen prompt never
// carried. An empty scratch directory has nothing to read: the prompt text is
// the reviewer's only legitimate input, exactly as the shared advisory
// contract requires (organic proof:
// TestRealCodexReviewerOrdinarySessionAdmitsRawOutput,
// e2e/organicruntime).
type CodexAdapter struct {
	// LookPath resolves the codex binary. Overridable so a test can prove the
	// typed-unavailable path without requiring a real installation.
	LookPath func(string) (string, error)
}

// NewCodexAdapter returns a CodexAdapter wired to the real codex binary on
// PATH.
func NewCodexAdapter() *CodexAdapter {
	return &CodexAdapter{LookPath: exec.LookPath}
}

// Review invokes codex non-interactively with prompt and returns its raw
// final message. Every returned error is a transport failure -- unavailable
// binary, scratch-directory setup, non-zero exit, or a canceled/expired
// context -- never a verdict about the reviewer's content. Only Validate may
// render that verdict, and only once this adapter has hung up.
func (adapter *CodexAdapter) Review(ctx context.Context, prompt string) ([]byte, error) {
	lookPath := adapter.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	binary, err := lookPath("codex")
	if err != nil {
		return nil, fmt.Errorf("codex advisory transport unavailable: %w", err)
	}

	scratch, err := os.MkdirTemp("", "gentle-ai-codex-advisory-*")
	if err != nil {
		return nil, fmt.Errorf("codex advisory transport unavailable: create scratch directory: %w", err)
	}
	defer os.RemoveAll(scratch)

	outputPath := scratch + string(os.PathSeparator) + "result.txt"
	command := exec.CommandContext(ctx, binary, "exec",
		"--skip-git-repo-check",
		"--ignore-user-config",
		"--sandbox", "read-only",
		"-C", scratch,
		"--output-last-message", outputPath,
		prompt,
	)
	// Dir is set in addition to -C: -C is codex's own working-root flag, but
	// the OS-level process working directory is the actual boundary this
	// adapter can independently guarantee. Both must agree on the same empty
	// scratch directory rather than trusting the child process alone to
	// honor -C.
	command.Dir = scratch
	// Env is an explicit allowlist, not a security boundary: authority over
	// what a runtime may do lives in Go admission (candidate-causality
	// checks, evidence bounds, the terminal receipt), never in what
	// environment variables happen to reach a transport subprocess. This is
	// quality hardening of the documented scratch-process boundary above --
	// command.Dir already stops codex from reading the caller's repository,
	// and command.Env stops it from inheriting unrelated ambient state (most
	// notably PWD, which without this would still point at the real
	// worktree despite Dir/-C both naming the empty scratch directory).
	// codexAdvisoryEnvironment carries only PATH (so codex can find the
	// bubblewrap sandbox helper the same way it would from a normal shell)
	// and CODEX_HOME (so it can still read its own ~/.codex auth/config),
	// nothing else.
	command.Env = codexAdvisoryEnvironment()
	command.Stdin = bytes.NewReader(nil)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("codex advisory transport failed: %w: %s", err, stderr.String())
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("codex advisory transport produced no final message: %w", err)
	}
	return raw, nil
}

// codexAdvisoryEnvironment returns the minimal explicit environment allowlist
// for the codex child process: PATH and CODEX_HOME, nothing else. Everything
// the parent process happens to carry -- PWD in particular, which would
// otherwise still name the real worktree even though Dir/-C both point codex
// at an empty scratch directory -- is dropped by construction, since
// exec.Cmd.Env, once non-nil, replaces rather than extends the inherited
// environment.
//
// Verified empirically against codex-cli 0.146.1 (env -i PATH=... CODEX_HOME=...
// codex exec --skip-git-repo-check --sandbox read-only ...): CODEX_HOME alone
// is sufficient for codex to locate and read its own auth.json, so HOME is
// not required and is deliberately left out; PATH is kept so codex finds the
// real bubblewrap sandbox helper instead of silently falling back to its own
// bundled copy.
func codexAdvisoryEnvironment() []string {
	env := make([]string, 0, 2)
	if path, ok := os.LookupEnv("PATH"); ok {
		env = append(env, "PATH="+path)
	}
	if codexHome, ok := os.LookupEnv("CODEX_HOME"); ok {
		env = append(env, "CODEX_HOME="+codexHome)
	} else if home, err := os.UserHomeDir(); err == nil {
		env = append(env, "CODEX_HOME="+filepath.Join(home, ".codex"))
	}
	return env
}
