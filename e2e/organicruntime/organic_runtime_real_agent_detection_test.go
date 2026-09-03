//go:build real_agent_e2e

// This file is gated behind the real_agent_e2e build tag, the same shape
// internal/reviewtransaction and internal/sddstatus use for bench_fixture:
// code that needs a real prerequisite the ordinary `go test ./...` sweep
// never provides does not compile into that sweep at all, rather than
// running and depending on ambient machine state, or skipping in a way that
// reads as passing.
//
// Claude Code and OpenCode's routing-guidance cases need gentle-ai's real
// Detect() to find a real installed binary on PATH: install now refuses
// instead of installing a missing runtime (agentInstallStep in
// internal/cli/run.go), so without the real binary these cases fail with a
// refusal, not a skip. `go test ./...` at the repo root (the Unit Tests CI
// job) never installs agent runtimes and must not silently pass this file's
// absence off as full coverage — Cursor's equivalent case, which needs no
// real binary, still runs there unconditionally
// (TestOrganicConfiguredAgentReceivesRoutingGuidanceCursor in
// organic_runtime_test.go).
//
// The organic-runtime-e2e job in .github/workflows/ci.yml installs both
// Claude Code and OpenCode before running `go test -tags real_agent_e2e
// ./e2e/organicruntime`, and its "Verify real-agent detection E2E is
// registered" step fails loudly if this test ever stops compiling into that
// job — the same shape as the sibling "Verify Windows OpenCode reviewer
// rejection E2E is registered" step already in that job.
package organicruntime_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestOrganicConfiguredAgentReceivesRoutingGuidanceRealAgents proves the
// markdown-section and orchestrator-prompt delivery strategies for Claude
// Code and OpenCode — the two rows of
// TestOrganicConfiguredAgentReceivesRoutingGuidanceCursor's original table
// that need a real installed agent binary. See this file's package comment
// for why they moved here.
func TestOrganicConfiguredAgentReceivesRoutingGuidanceRealAgents(t *testing.T) {
	t.Parallel()
	// The orchestrator prompt lives in the home settings document even under
	// workspace scope, because that is the only settings document the
	// OpenCode family ever loads (issue #1825).
	agents := []struct {
		name    string
		agentID string
		path    string
		inHome  bool
	}{
		{name: "markdown section", agentID: "claude-code", path: ".claude/CLAUDE.md"},
		{name: "orchestrator prompt", agentID: "opencode", path: ".config/opencode/opencode.json", inHome: true},
	}
	for _, agent := range agents {
		t.Run(agent.name, func(t *testing.T) {
			t.Parallel()
			workspace := t.TempDir()
			home := t.TempDir()
			if _, err := organicGitOutput(context.Background(), workspace, "init", "--quiet", "--initial-branch=main", "."); err != nil {
				t.Fatal(err)
			}
			output, stderr, err := runOrganicCommand(
				t, organicBinary, workspace, organicEnvironment(home),
				"install", "--agent", agent.agentID, "--scope", "workspace", "--components", "permissions",
			)
			if err != nil {
				t.Fatalf("install %s: %v\nstdout:\n%s\nstderr:\n%s", agent.agentID, err, output, stderr)
			}
			root := workspace
			if agent.inHome {
				root = home
			}
			rendered, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(agent.path)))
			if readErr != nil {
				t.Fatalf("configured agent %s received no routing guidance at %s: %v", agent.agentID, agent.path, readErr)
			}
			for _, fragment := range organicRoutingGuidanceRequiredFragments {
				if !bytes.Contains(rendered, []byte(fragment)) {
					t.Fatalf("routing guidance for %s omits %q:\n%s", agent.agentID, fragment, rendered)
				}
			}
			if agent.inHome {
				stranded := filepath.Join(workspace, filepath.FromSlash(agent.path))
				if _, statErr := os.Stat(stranded); !os.IsNotExist(statErr) {
					t.Fatalf("workspace-scoped install stranded a settings document the agent never loads at %s (stat err = %v)", stranded, statErr)
				}
			}
		})
	}
}

// TestOrganicCodexWorkspaceInstructionsAreDiscoverable verifies native Codex discovers workspace instructions.
func TestOrganicCodexWorkspaceInstructionsAreDiscoverable(t *testing.T) {
	if os.Getenv("GENTLE_AI_CODEX_DISCOVERY_E2E") != "1" {
		t.Skip("set GENTLE_AI_CODEX_DISCOVERY_E2E=1 to run the authenticated native Codex discovery smoke test")
	}
	workspace := t.TempDir()
	home := t.TempDir()
	codexHome := prepareOrganicCodexHome(t)
	codexEnvironment := append(organicEnvironment(home), "CODEX_HOME="+codexHome)
	if err := os.MkdirAll(filepath.Join(home, ".gentle-ai"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := organicGitOutput(context.Background(), workspace, "init", "--quiet", "--initial-branch=main", "."); err != nil {
		t.Fatal(err)
	}
	if _, stderr, err := runOrganicCommand(t, organicBinary, workspace, codexEnvironment, "install", "--agent", "codex", "--scope", "workspace", "--components", "permissions"); err != nil {
		t.Fatalf("Codex workspace install: %v\nstderr:\n%s", err, stderr)
	}
	rootAgents := filepath.Join(workspace, "AGENTS.md")
	if _, err := os.Stat(rootAgents); err != nil {
		t.Fatalf("workspace Codex instructions missing at repository root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".codex", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("workspace Codex instructions stranded in .codex/AGENTS.md (stat err = %v)", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "codex", "exec", "--cd", workspace, "--sandbox", "read-only", "--ephemeral", "--ignore-user-config", "--color", "never", "Without opening or searching the filesystem, report only the first Markdown heading from project instructions automatically loaded before this prompt. State whether this repository supplied project instructions.")
	cmd.Env = codexEnvironment
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("native Codex discovery smoke: %v\noutput:\n%s", err, output)
	}
	text := strings.ToLower(string(output))
	if !strings.Contains(text, "implementation routing") || !strings.Contains(text, "supplied project instructions") {
		t.Fatalf("native Codex did not report the repository AGENTS.md instructions:\n%s", output)
	}
}

// prepareOrganicCodexHome creates an isolated Codex home with a copy of an
// existing auth.json without exposing credential contents in test output.
func prepareOrganicCodexHome(t *testing.T) string {
	t.Helper()
	authPath := ""
	if configuredHome := os.Getenv("CODEX_HOME"); configuredHome != "" {
		authPath = filepath.Join(configuredHome, "auth.json")
	} else if userHome, err := os.UserHomeDir(); err == nil {
		authPath = filepath.Join(userHome, ".codex", "auth.json")
	}
	if authPath == "" {
		t.Skip("native Codex discovery skipped: no usable Codex authentication source")
	}
	info, err := os.Stat(authPath)
	if err != nil || !info.Mode().IsRegular() {
		t.Skipf("native Codex discovery skipped: Codex authentication source is unavailable at %s", authPath)
	}
	authContents, err := os.ReadFile(authPath)
	if err != nil {
		t.Skipf("native Codex discovery skipped: Codex authentication source cannot be read at %s", authPath)
	}
	codexHome := t.TempDir()
	destination := filepath.Join(codexHome, "auth.json")
	if err := os.WriteFile(destination, authContents, 0o600); err != nil {
		t.Fatalf("copy Codex authentication source: %v", err)
	}
	destinationInfo, err := os.Stat(destination)
	if err != nil {
		t.Fatalf("verify isolated Codex authentication source: %v", err)
	}
	if !destinationInfo.Mode().IsRegular() || destinationInfo.Mode().Perm() != 0o600 {
		t.Fatalf("isolated Codex authentication source has unexpected file mode %s", destinationInfo.Mode())
	}
	return codexHome
}
