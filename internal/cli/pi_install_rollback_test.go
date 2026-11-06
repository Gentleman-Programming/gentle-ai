package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	piagent "github.com/gentleman-programming/gentle-ai/v4/internal/agents/pi"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v4/internal/planner"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

func TestPiInstallSnapshotsSettingsWithoutEngramComponent(t *testing.T) {
	for _, selected := range []bool{true, false} {
		t.Run(fmt.Sprint(selected), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("PI_CODING_AGENT_DIR", "")
			resolved := planner.ResolvedPlan{}
			if selected {
				resolved.Agents = []model.AgentID{model.AgentPi}
			}
			paths, err := installBackupTargets(home, t.TempDir(), ScopeWorkspace, model.Selection{}, resolved, false)
			if err != nil {
				t.Fatal(err)
			}
			want := filepath.Join(home, ".pi", "agent", "settings.json")
			found := false
			for _, path := range paths {
				if path == want {
					found = true
				}
			}
			if found != selected {
				t.Fatalf("Pi settings snapshot selected=%v, found=%v: %v", selected, found, paths)
			}
		})
	}
}

type piRollbackFailingStep struct{ run func() error }

func (s piRollbackFailingStep) ID() string { return "test:failure-after-provisioning" }
func (s piRollbackFailingStep) Run() error { return s.run() }

func TestPiExternalDirectoryRollback(t *testing.T) {
	for _, attack := range []bool{false, true} {
		t.Run(fmt.Sprint(attack), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			if runtime.GOOS == "windows" {
				t.Setenv("USERPROFILE", home)
			}
			parent := t.TempDir()
			agentDir := filepath.Join(parent, "agent")
			t.Setenv("PI_CODING_AGENT_DIR", agentDir)
			path := piagent.NewAdapter().SettingsPath(home)
			if path != filepath.Join(agentDir, "settings.json") {
				t.Fatalf("override not honored: %s", path)
			}
			if err := os.MkdirAll(agentDir, 0o755); err != nil {
				t.Fatal(err)
			}
			initial := []byte(`{"theme":"kanagawa","packages":["npm:gentle-engram",{"source":"npm:gentle-engram@0.1.16","extensions":[]}]}`)
			if err := os.WriteFile(path, initial, 0o644); err != nil {
				t.Fatal(err)
			}
			sibling := filepath.Join(parent, "sibling.json")
			if err := os.WriteFile(sibling, []byte("original sibling"), 0o644); err != nil {
				t.Fatal(err)
			}
			resolved := planner.ResolvedPlan{Agents: []model.AgentID{model.AgentPi}, OrderedComponents: []model.ComponentID{model.ComponentEngram}}
			rt, err := newInstallRuntime(home, ScopeWorkspace, ChannelStable, model.Selection{Agents: resolved.Agents, Components: resolved.OrderedComponents}, resolved, system.PlatformProfile{})
			if err != nil {
				t.Fatal(err)
			}
			defer rt.state.cleanupCompatibilityTransaction()
			defer rt.state.cleanupRollbackSnapshot()
			plan := pipeline.StagePlan{}
			// Exercise actual planned snapshot/restore, without installer commands.
			built := rt.stagePlan()
			for _, step := range built.Prepare {
				if snapshot, ok := step.(prepareBackupStep); ok {
					if attack {
						snapshot.targets = append(snapshot.targets, sibling)
					}
					plan.Prepare = append(plan.Prepare, snapshot)
				}
			}
			for _, step := range built.Apply {
				if step.ID() == "apply:rollback-restore" {
					plan.Apply = append(plan.Apply, step)
				}
			}
			if len(plan.Prepare) != 1 || len(plan.Apply) != 1 {
				t.Fatal("planned snapshot/restore missing")
			}
			failure := errors.New("intentional failure after normalization")
			plan.Apply = append(plan.Apply, piRollbackFailingStep{run: func() error {
				if _, err := piagent.NewAdapter().PrepareInstall(system.PlatformProfile{}, home); err != nil {
					return err
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				if string(data) == string(initial) {
					t.Fatal("normalization did not change bytes")
				}
				if attack {
					if err := os.WriteFile(sibling, []byte("changed sibling"), 0o644); err != nil {
						return err
					}
				}
				return failure
			}})
			result := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy()).Execute(plan)
			if !errors.Is(result.Err, failure) {
				t.Fatalf("failure = %v", result.Err)
			}
			if attack {
				if !strings.Contains(result.Err.Error(), fmt.Sprintf("invalid OriginalPath %q", sibling)) {
					t.Fatalf("sibling not refused: %v", result.Err)
				}
				data, err := os.ReadFile(sibling)
				if err != nil || string(data) != "changed sibling" {
					t.Fatalf("sibling restored: %q %v", data, err)
				}
			} else {
				data, err := os.ReadFile(path)
				if err != nil || string(data) != string(initial) {
					t.Fatalf("external bytes not restored: %s %v; %v", data, err, result.Err)
				}
			}
		})
	}
}

func TestPiRollbackExistingRootsAndUnselectedDirectory(t *testing.T) {
	for _, location := range []string{"home", "workspace", "unselected-pi"} {
		t.Run(location, func(t *testing.T) {
			home, workspace, agentDir := t.TempDir(), t.TempDir(), t.TempDir()
			t.Setenv("PI_CODING_AGENT_DIR", agentDir)
			rt, err := newInstallRuntime(home, ScopeWorkspace, ChannelStable, model.Selection{}, planner.ResolvedPlan{}, system.PlatformProfile{})
			if err != nil {
				t.Fatal(err)
			}
			rt.workspaceDir = workspace
			defer rt.state.cleanupCompatibilityTransaction()
			defer rt.state.cleanupRollbackSnapshot()
			root := map[string]string{"home": home, "workspace": workspace, "unselected-pi": agentDir}[location]
			path := filepath.Join(root, "settings.json")
			if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
				t.Fatal(err)
			}
			plan := pipeline.StagePlan{}
			built := rt.stagePlan()
			for _, step := range built.Prepare {
				if snapshot, ok := step.(prepareBackupStep); ok {
					snapshot.targets = []string{path}
					plan.Prepare = append(plan.Prepare, snapshot)
				}
			}
			for _, step := range built.Apply {
				if step.ID() == "apply:rollback-restore" {
					plan.Apply = append(plan.Apply, step)
				}
			}
			if len(plan.Prepare) != 1 || len(plan.Apply) != 1 {
				t.Fatal("planned snapshot/restore missing")
			}
			failure := errors.New("intentional failure")
			plan.Apply = append(plan.Apply, piRollbackFailingStep{run: func() error {
				if err := os.WriteFile(path, []byte("changed"), 0o644); err != nil {
					return err
				}
				return failure
			}})
			result := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy()).Execute(plan)
			if !errors.Is(result.Err, failure) {
				t.Fatalf("failure = %v", result.Err)
			}
			want := "original"
			if location == "unselected-pi" {
				want = "changed"
				if !strings.Contains(result.Err.Error(), "invalid OriginalPath") {
					t.Fatalf("unselected root accepted: %v", result.Err)
				}
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != want {
				t.Fatalf("restore = %q %v; want %q; %v", data, err, want, result.Err)
			}
		})
	}
}
