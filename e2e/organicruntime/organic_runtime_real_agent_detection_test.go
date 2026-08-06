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
	"path/filepath"
	"testing"
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
