package sdd

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/catalog"
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
