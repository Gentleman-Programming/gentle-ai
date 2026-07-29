package agentguidance

import (
	"errors"
	"os"
	"sort"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// TestRoutingPathsMatchesEveryPathInjectRoutingWrites is the anti-drift guard.
//
// RoutingPaths exists so callers such as the install/sync backup planner can
// learn where guidance lands without re-implementing the three-strategy
// dispatch. The moment the two answers disagree, a file is written that was
// never snapshotted and a rollback silently loses it. The expectation is
// therefore never restated here: it is read back from what InjectRouting
// actually touched.
func TestRoutingPathsMatchesEveryPathInjectRoutingWrites(t *testing.T) {
	t.Parallel()

	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()

			targetDir := t.TempDir()

			declared, err := RoutingPaths(targetDir, agent.ID)
			if err != nil {
				t.Fatalf("RoutingPaths(%q) error = %v", agent.ID, err)
			}

			result, err := InjectRouting(targetDir, agent.ID)
			if err != nil {
				t.Fatalf("InjectRouting(%q) error = %v", agent.ID, err)
			}

			if !samePathSet(declared, result.Files) {
				t.Fatalf("RoutingPaths(%q) = %v, but InjectRouting wrote %v", agent.ID, declared, result.Files)
			}
		})
	}
}

func TestRoutingPathsWritesNothingToDisk(t *testing.T) {
	t.Parallel()

	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()

			targetDir := t.TempDir()

			if _, err := RoutingPaths(targetDir, agent.ID); err != nil {
				t.Fatalf("RoutingPaths(%q) error = %v", agent.ID, err)
			}

			entries, err := os.ReadDir(targetDir)
			if err != nil {
				t.Fatalf("ReadDir error = %v", err)
			}
			if len(entries) != 0 {
				t.Fatalf("RoutingPaths(%q) created %d entries on disk", agent.ID, len(entries))
			}
		})
	}
}

func TestRoutingPathsRejectsBlankTargetDir(t *testing.T) {
	t.Parallel()

	paths, err := RoutingPaths("   ", model.AgentClaudeCode)
	if err == nil {
		t.Fatalf("RoutingPaths accepted a blank target dir: %v", paths)
	}
	if !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("RoutingPaths error = %v, want it to wrap ErrInvalidTarget", err)
	}
	if len(paths) != 0 {
		t.Fatalf("RoutingPaths returned %v for a blank target dir", paths)
	}
}

func TestRoutingPathsRejectsUnregisteredAgent(t *testing.T) {
	t.Parallel()

	targetDir := t.TempDir()

	paths, err := RoutingPaths(targetDir, model.AgentID("totally-unregistered-agent"))
	if err == nil {
		t.Fatalf("RoutingPaths accepted an unregistered agent: %v", paths)
	}
	if len(paths) != 0 {
		t.Fatalf("RoutingPaths returned %v for an unregistered agent", paths)
	}
}

func samePathSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	gotSorted := append([]string(nil), got...)
	wantSorted := append([]string(nil), want...)
	sort.Strings(gotSorted)
	sort.Strings(wantSorted)
	for i := range gotSorted {
		if gotSorted[i] != wantSorted[i] {
			return false
		}
	}
	return true
}
