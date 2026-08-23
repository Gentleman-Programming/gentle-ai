package sdd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func requireEngramProjectIdentityContract(t *testing.T, content, client string) {
	t.Helper()
	for _, want := range []string{
		"Engram Project Identity Contract",
		"mem_current_project",
		"cache its returned canonical `project`",
		"every filesystem read, write, and native command `--cwd`",
		"Never derive the logical Engram project from the active workspace basename",
		"When explicit project-scoped persistence is required and no canonical project is available, fail closed",
		"`ENGRAM_PROJECT`, then the active workspace's Git remote",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("%s missing Engram project identity contract %q", client, want)
		}
	}
}

func TestRenderedSDDOrchestratorsUseCanonicalEngramProjectIdentity(t *testing.T) {
	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			requireEngramProjectIdentityContract(t, renderSDDOrchestratorAsset(agent.ID), string(agent.ID)+" orchestrator")
		})
	}
}

func TestRenderedSDDCommandsDoNotDeriveEngramProjectFromWorkspaceBasename(t *testing.T) {
	clients := []struct {
		name  string
		agent model.AgentID
		dir   string
	}{
		{name: "Claude", agent: model.AgentClaudeCode, dir: "claude/commands"},
		{name: "OpenCode", agent: model.AgentOpenCode, dir: "opencode/commands"},
	}

	for _, client := range clients {
		t.Run(client.name, func(t *testing.T) {
			entries, err := assets.FS.ReadDir(client.dir)
			if err != nil {
				t.Fatalf("ReadDir(%s): %v", client.dir, err)
			}
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasPrefix(entry.Name(), "sdd-") || !strings.HasSuffix(entry.Name(), ".md") {
					continue
				}
				path := client.dir + "/" + entry.Name()
				source := assets.MustRead(path)
				if strings.Contains(source, "Current project: Derive agent-side from the detected working directory basename") ||
					strings.Contains(source, "Current project: the `basename` of the detected workspace above") {
					t.Fatalf("%s retains the workspace-basename project derivation", path)
				}
				for _, want := range []string{"git rev-parse --show-toplevel", "Logical Engram identity:"} {
					if !strings.Contains(source, want) {
						t.Fatalf("%s missing workspace/identity separation %q", path, want)
					}
				}
				requireEngramProjectIdentityContract(t, renderBoundedReviewAsset(client.agent, path), path)
			}
		})
	}
}

func TestRenderedNonSDDCommandDoesNotReceiveEngramProjectIdentity(t *testing.T) {
	content := renderBoundedReviewAsset(model.AgentOpenCode, "opencode/commands/skill-registry.md")
	if strings.Contains(content, "Engram Project Identity Contract") {
		t.Fatal("non-SDD OpenCode command received the Engram project identity contract")
	}
}

func TestRenderedSDDPhaseAndClaudeWorkflowReceiveEngramProjectIdentity(t *testing.T) {
	home := t.TempDir()
	if _, err := WriteSharedPromptFiles(home, nil); err != nil {
		t.Fatalf("WriteSharedPromptFiles(): %v", err)
	}
	for _, phase := range SharedPromptPhases() {
		content, err := os.ReadFile(filepath.Join(SharedPromptDir(home), phase+".md"))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", phase, err)
		}
		requireEngramProjectIdentityContract(t, string(content), phase+" shared prompt")
	}

	if _, err := Inject(home, claudeAdapter(), ""); err != nil {
		t.Fatalf("Inject(Claude): %v", err)
	}
	workflow, err := os.ReadFile(filepath.Join(home, ".claude", "skills", "_shared", "sdd-orchestrator-workflow.md"))
	if err != nil {
		t.Fatalf("ReadFile(Claude workflow): %v", err)
	}
	requireEngramProjectIdentityContract(t, string(workflow), "Claude lazy workflow")
}
