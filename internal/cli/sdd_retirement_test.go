package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

// releasedSDDRender reads a real v3.7.0 render of a retired SDD agent.
func releasedSDDRender(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "components", "legacyassets", "testdata", "v3.7.0", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func backupManifestEntries(t *testing.T, home string) map[string]backup.ManifestEntry {
	t.Helper()
	entries := map[string]backup.ManifestEntry{}
	root := filepath.Join(home, ".gentle-ai", "backups")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != backup.ManifestFilename {
			return err
		}
		manifest, err := backup.ReadManifest(path)
		if err != nil {
			return err
		}
		for _, entry := range manifest.Entries {
			if entry.Existed {
				entries[entry.OriginalPath] = entry
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

// TestRunSyncRetiresOwnedSDDAgentsForEveryNativeFamily covers #5157 and #5253:
// a sync after SDD retirement removes the native SDD agents a release rendered
// (Kiro, Claude Code, Cursor, Kimi), snapshots them for rollback, and preserves
// and reports any file whose bytes no release wrote.
func TestRunSyncRetiresOwnedSDDAgentsForEveryNativeFamily(t *testing.T) {
	home := t.TempDir()
	setSyncTestHome(t, home)
	agents := []model.AgentID{model.AgentClaudeCode, model.AgentKiroIDE, model.AgentCursor, model.AgentKimi}
	installed := make([]string, 0, len(agents))
	for _, agent := range agents {
		installed = append(installed, string(agent))
	}
	if err := state.Write(home, state.InstallState{
		InstalledAgents: installed, SelectionConfigured: true,
		Components: []model.ComponentID{model.ComponentSDD, model.ComponentSkills},
	}); err != nil {
		t.Fatal(err)
	}
	owned := map[string][]byte{
		filepath.Join(home, ".claude", "agents", "sdd-apply.md"): releasedSDDRender(t, "claude-sdd-apply.md"),
		filepath.Join(home, ".kiro", "agents", "sdd-apply.md"):   releasedSDDRender(t, "kiro-sdd-apply.md"),
		filepath.Join(home, ".cursor", "agents", "sdd-apply.md"): releasedSDDRender(t, "cursor-sdd-apply.md"),
		filepath.Join(home, ".kimi", "agents", "sdd-apply.md"):   releasedSDDRender(t, "kimi-sdd-apply.md"),
		filepath.Join(home, ".kimi", "agents", "sdd-apply.yaml"): releasedSDDRender(t, "kimi-sdd-apply.yaml"),
	}
	userEdited := filepath.Join(home, ".kiro", "agents", "sdd-verify.md")
	userBytes := []byte("---\nname: sdd-verify\nmodel: auto\n---\nMy own verifier.\n")
	for path, data := range owned {
		mustWriteFile(t, path, data)
	}
	mustWriteFile(t, userEdited, userBytes)

	result, err := RunSync([]string{"--agents", strings.Join(installed, ",")})
	if err != nil {
		t.Fatalf("RunSync() error = %v", err)
	}
	snapshot := backupManifestEntries(t, home)
	for path := range owned {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Errorf("owned retired SDD agent survived sync: %s (%v)", path, err)
		}
		if _, ok := snapshot[path]; !ok {
			t.Errorf("rollback snapshot omitted removed file %s", path)
		}
		if !slices.Contains(result.ChangedFiles, path) {
			t.Errorf("sync did not report removed file %s among %d changed files", path, len(result.ChangedFiles))
		}
	}
	if got, err := os.ReadFile(userEdited); err != nil || string(got) != string(userBytes) {
		t.Fatalf("user-authored SDD agent changed: %q, %v", got, err)
	}
	reported := false
	for _, action := range result.ManualActions {
		if strings.Contains(action, userEdited) && strings.Contains(action, "move or delete it") {
			reported = true
		}
	}
	if !reported {
		t.Errorf("preserved SDD agent not reported with an action: %v", result.ManualActions)
	}

	again, err := RunSync([]string{"--agents", strings.Join(installed, ",")})
	if err != nil {
		t.Fatalf("second RunSync() error = %v", err)
	}
	for _, path := range again.ChangedFiles {
		if strings.Contains(filepath.Base(path), "sdd-") {
			t.Errorf("second sync touched SDD path %s", path)
		}
	}
	if got, err := os.ReadFile(userEdited); err != nil || string(got) != string(userBytes) {
		t.Fatalf("second sync changed the user-authored SDD agent: %q, %v", got, err)
	}
}

type sddRetirementFailingStep struct{ observe func() }

func (sddRetirementFailingStep) ID() string { return "test:fail-after-sdd-retirement" }
func (s sddRetirementFailingStep) Run() error {
	s.observe()
	return errors.New("injected failure")
}

// A sync that fails after retirement restores every removed agent from the
// pipeline snapshot, byte for byte.
func TestSyncRollbackRestoresRetiredSDDAgents(t *testing.T) {
	home := t.TempDir()
	setSyncTestHome(t, home)
	agentPath := filepath.Join(home, ".kiro", "agents", "sdd-apply.md")
	released := releasedSDDRender(t, "kiro-sdd-apply.md")
	mustWriteFile(t, agentPath, released)
	selection := BuildSyncSelection(SyncFlags{}, []model.AgentID{model.AgentKiroIDE})
	rt, err := newSyncRuntimeWithScope(home, selection, ScopeGlobal)
	if err != nil {
		t.Fatal(err)
	}
	plan := rt.stagePlan()
	if !slices.Contains(rt.managedPaths, agentPath) {
		t.Fatalf("sync snapshot does not declare %s", agentPath)
	}
	retireIndex := slices.IndexFunc(plan.Apply, func(step pipeline.Step) bool { return step.ID() == "sync:agent:retire-sdd:kiro-ide" })
	if retireIndex < 0 {
		t.Fatal("sync plan has no SDD retirement step for Kiro")
	}
	removedBeforeFailure := false
	plan.Apply = append(plan.Apply[:retireIndex+1:retireIndex+1], sddRetirementFailingStep{observe: func() {
		_, err := os.Lstat(agentPath)
		removedBeforeFailure = os.IsNotExist(err)
	}})
	execution := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy()).Execute(plan)
	if execution.Err == nil {
		t.Fatal("injected failure did not fail the sync")
	}
	if !removedBeforeFailure {
		t.Fatal("retirement did not remove the agent before the failure")
	}
	if got, err := os.ReadFile(agentPath); err != nil || string(got) != string(released) {
		t.Fatalf("rollback did not restore the retired agent: %v", err)
	}
}

// Install retires the same inventory as sync; a workspace-scoped install
// never touches the home-level agents.
func TestRunInstallRetiresOwnedSDDAgentsOnlyInGlobalScope(t *testing.T) {
	home := installTestHome(t)
	restoreBackupHome := backup.UserHomeDirFn
	backup.UserHomeDirFn = func() (string, error) { return home, nil }
	t.Cleanup(func() { backup.UserHomeDirFn = restoreBackupHome })
	owned := filepath.Join(home, ".claude", "agents", "sdd-apply.md")
	userEdited := filepath.Join(home, ".claude", "agents", "sdd-verify.md")
	mustWriteFile(t, owned, releasedSDDRender(t, "claude-sdd-apply.md"))
	mustWriteFile(t, userEdited, []byte("my verifier\n"))

	t.Chdir(t.TempDir())
	if _, err := RunInstall([]string{"--agent", "claude-code", "--component", "skills", "--scope", "workspace"}, system.DetectionResult{}); err != nil {
		t.Fatalf("workspace RunInstall() error = %v", err)
	}
	if _, err := os.Stat(owned); err != nil {
		t.Fatalf("workspace-scoped install touched a home-level SDD agent: %v", err)
	}

	result, err := RunInstall([]string{"--agent", "claude-code", "--component", "skills"}, system.DetectionResult{})
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if _, err := os.Lstat(owned); !os.IsNotExist(err) {
		t.Errorf("install kept the owned retired SDD agent: %v", err)
	}
	if _, ok := backupManifestEntries(t, home)[owned]; !ok {
		t.Error("install snapshot omitted the removed SDD agent")
	}
	if got, err := os.ReadFile(userEdited); err != nil || string(got) != "my verifier\n" {
		t.Fatalf("install changed the user-authored SDD agent: %q, %v", got, err)
	}
	if !slices.ContainsFunc(result.ManualActions, func(action string) bool { return strings.Contains(action, userEdited) }) {
		t.Errorf("install did not report the preserved SDD agent: %v", result.ManualActions)
	}
}
