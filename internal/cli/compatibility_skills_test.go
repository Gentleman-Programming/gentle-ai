package cli

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/backup"
	componentskills "github.com/gentleman-programming/gentle-ai/v2/internal/components/skills"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v2/internal/planner"
)

func temporaryUserHome(t *testing.T) string {
	t.Helper()
	root, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	home, err := os.MkdirTemp(root, ".gentle-ai-compatibility-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	return home
}

func writeStale(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCompatibilitySkillsRefreshStepRefreshesExistingOrdinaryAndSDDAssets(t *testing.T) {
	home := t.TempDir()
	skillsDir := filepath.Join(home, ".agents", "skills")
	staleFiles := []string{
		filepath.Join(skillsDir, "go-testing", "SKILL.md"),
		filepath.Join(skillsDir, "sdd-apply", "SKILL.md"),
		filepath.Join(skillsDir, "judgment-day", "SKILL.md"),
		filepath.Join(skillsDir, "_shared", "persistence-contract.md"),
	}
	for _, path := range staleFiles {
		writeStale(t, path)
	}
	step := compatibilitySkillsRefreshStep{homeDir: home, components: []model.ComponentID{model.ComponentSkills, model.ComponentSDD}, selection: model.Selection{Skills: []model.SkillID{model.SkillGoTesting}}}
	if err := step.Run(); err != nil {
		t.Fatal(err)
	}
	for _, path := range staleFiles {
		if content, err := os.ReadFile(path); err != nil || string(content) == "stale" {
			t.Errorf("%q was not refreshed: %v", path, err)
		}
	}
}

func TestRunSyncDryRunMatchesZeroAgentCompatibilityRefresh(t *testing.T) {
	home := t.TempDir()
	setSyncTestHome(t, home)
	if noRefresh, err := RunSync([]string{"--dry-run"}); err != nil || !noRefresh.NoOp {
		t.Fatalf("zero-agent dry-run without compatibility work: no-op=%t, err=%v", noRefresh.NoOp, err)
	}
	path := filepath.Join(home, ".agents", "skills", "go-testing", "SKILL.md")
	writeStale(t, path)
	dryRun, err := RunSync([]string{"--dry-run"})
	if err != nil || dryRun.NoOp || !slices.ContainsFunc(dryRun.Plan.Apply, func(step pipeline.Step) bool { return step.ID() == "sync:compatibility-skills-refresh" }) {
		t.Fatalf("compatibility refresh plan missing without agents: no-op=%t, err=%v", dryRun.NoOp, err)
	}
	backupRoot := filepath.Join(home, ".gentle-ai", "backups")
	if _, statErr := os.Stat(backupRoot); !os.IsNotExist(statErr) {
		t.Fatalf("agentless compatibility dry-run created backup root %q: %v", backupRoot, statErr)
	}
	result, err := RunSyncWithSelection(home, model.Selection{Components: []model.ComponentID{model.ComponentSkills}, Skills: []model.SkillID{model.SkillGoTesting}})
	content, readErr := os.ReadFile(path)
	if err != nil || readErr != nil || string(content) == "stale" || result.NoOp {
		t.Fatalf("compatibility skill was not refreshed without agents; no-op=%t, sync=%v, read=%v", result.NoOp, err, readErr)
	}
}

func TestRunSyncDryRunPropagatesCompatibilityManagedPathError(t *testing.T) {
	if usesAnchoredCompatibilityTransaction() {
		t.Skip("Windows compatibility discovery intentionally has no path-based lstat seam")
	}
	home := t.TempDir()
	setSyncTestHome(t, home)
	managedPath := filepath.Join(home, ".agents", "skills", "go-testing", "SKILL.md")
	if err := os.MkdirAll(filepath.Join(home, ".agents", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	pathErr := errors.New("injected managed destination failure")
	originalLstat := lstatCompatibilityDestination
	lstatCompatibilityDestination = func(path string) (os.FileInfo, error) {
		if path == managedPath {
			return nil, pathErr
		}
		return originalLstat(path)
	}
	t.Cleanup(func() { lstatCompatibilityDestination = originalLstat })

	result, err := RunSync([]string{"--dry-run", "--skill", "go-testing"})
	if !errors.Is(err, pathErr) || result.NoOp {
		t.Fatalf("RunSync(--dry-run) no-op = %t, error = %v; want propagated managed-path failure", result.NoOp, err)
	}
}

func TestCompatibilitySkillsRefreshRequiresPhysicalDirectory(t *testing.T) {
	selection := model.Selection{Skills: []model.SkillID{model.SkillGoTesting}}
	t.Run("absent", func(t *testing.T) {
		home := t.TempDir()
		step := compatibilitySkillsRefreshStep{homeDir: home, components: []model.ComponentID{model.ComponentSkills, model.ComponentSDD}, selection: selection}
		if err := step.Run(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(home, ".agents", "skills")); !os.IsNotExist(err) {
			t.Fatalf("compatibility directory was created: %v", err)
		}
	})
	for _, tc := range []struct {
		name       string
		linkParent bool
	}{
		{name: "root symlink"},
		{name: "parent symlink", linkParent: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("symlink creation may require elevated privileges")
			}
			home, target := t.TempDir(), t.TempDir()
			targetSkills := target
			if tc.linkParent {
				targetSkills = filepath.Join(target, "skills")
				if err := os.Symlink(target, filepath.Join(home, ".agents")); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.MkdirAll(filepath.Join(home, ".agents"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, filepath.Join(home, ".agents", "skills")); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(targetSkills, "go-testing", "SKILL.md")
			writeStale(t, path)
			step := compatibilitySkillsRefreshStep{homeDir: home, components: []model.ComponentID{model.ComponentSkills}, selection: selection}
			if err := step.Run(); err != nil {
				t.Fatal(err)
			}
			if content, err := os.ReadFile(path); err != nil || string(content) != "stale" {
				t.Fatalf("symlink target changed without backup: %v", err)
			}
		})
	}
	for _, tc := range []struct {
		name       string
		components []model.ComponentID
		link       string
	}{
		{name: "ordinary descendant symlink", components: []model.ComponentID{model.ComponentSkills}, link: "go-testing"},
		{name: "SDD descendant symlink", components: []model.ComponentID{model.ComponentSDD}, link: "_shared"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("symlink creation may require elevated privileges")
			}
			home, target := t.TempDir(), t.TempDir()
			skillsDir := filepath.Join(home, ".agents", "skills")
			if err := os.MkdirAll(skillsDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(skillsDir, tc.link)); err != nil {
				t.Fatal(err)
			}
			step := compatibilitySkillsRefreshStep{homeDir: home, components: tc.components, selection: selection}
			if err := step.Run(); err == nil {
				t.Fatal("refresh followed descendant symlink")
			}
			entries, err := os.ReadDir(target)
			if err != nil || len(entries) != 0 {
				t.Fatalf("external target was mutated: entries=%v, err=%v", entries, err)
			}
		})
	}
}

func TestCompatibilityRefreshDoesNotFollowParentSwappedAfterValidation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated privileges")
	}

	home, outside := t.TempDir(), t.TempDir()
	skillsDir := filepath.Join(home, ".agents", "skills")
	inside := filepath.Join(skillsDir, "go-testing")
	destination := filepath.Join(inside, "references", "examples.md")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}

	originalLstat := lstatCompatibilityDestination
	swapped := false
	lstatCompatibilityDestination = func(path string) (os.FileInfo, error) {
		info, err := originalLstat(path)
		if path == destination && !swapped {
			swapped = true
			if err := os.Rename(inside, inside+"-original"); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, inside); err != nil {
				t.Fatal(err)
			}
		}
		return info, err
	}
	t.Cleanup(func() { lstatCompatibilityDestination = originalLstat })

	step := compatibilitySkillsRefreshStep{
		homeDir:    home,
		components: []model.ComponentID{model.ComponentSkills},
		selection:  model.Selection{Skills: []model.SkillID{model.SkillGoTesting}},
	}
	err := step.Run()
	if !swapped {
		t.Fatal("test did not reach the validation-to-write swap window")
	}
	_, outsideErr := os.Stat(filepath.Join(outside, "SKILL.md"))
	if err == nil || outsideErr == nil {
		t.Fatalf("compatibility refresh must reject the swapped parent without external mutation: error=%v external=%v", err, outsideErr)
	}
}

func TestCompatibilityManagedPathErrorsReachBackupPreparation(t *testing.T) {
	if usesAnchoredCompatibilityTransaction() {
		t.Skip("Windows compatibility snapshots are anchored before generic backup")
	}
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".agents", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	managedPath := filepath.Join(home, ".agents", "skills", "go-testing", "SKILL.md")
	pathErr := errors.New("injected managed destination failure")
	originalLstat := lstatCompatibilityDestination
	lstatCompatibilityDestination = func(path string) (os.FileInfo, error) {
		if path == managedPath {
			return nil, pathErr
		}
		return originalLstat(path)
	}
	t.Cleanup(func() { lstatCompatibilityDestination = originalLstat })

	selection := model.Selection{
		Components: []model.ComponentID{model.ComponentSkills},
		Skills:     []model.SkillID{model.SkillGoTesting},
	}
	resolved := planner.ResolvedPlan{OrderedComponents: selection.Components}
	backupRoot := filepath.Join(home, "backups")
	tests := []struct {
		name string
		plan pipeline.StagePlan
	}{
		{
			name: "install",
			plan: (&installRuntime{homeDir: home, selection: selection, resolved: resolved, backupRoot: backupRoot, state: &runtimeState{}}).stagePlan(),
		},
		{
			name: "sync",
			plan: (&syncRuntime{homeDir: home, selection: selection, backupRoot: backupRoot, state: &runtimeState{}}).stagePlan(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var backupStep pipeline.Step
			for _, step := range tt.plan.Prepare {
				if step.ID() == "prepare:backup-snapshot" {
					backupStep = step
					break
				}
			}
			if backupStep == nil {
				t.Fatal("prepare backup step not found")
			}
			err := backupStep.Run()
			if !errors.Is(err, pathErr) || !strings.Contains(err.Error(), "resolve backup targets") {
				t.Fatalf("backup preparation error = %v, want propagated managed-path failure", err)
			}
			if _, statErr := os.Stat(backupRoot); !os.IsNotExist(statErr) {
				t.Fatalf("backup preparation mutated the filesystem after managed-path failure: %v", statErr)
			}
		})
	}
}

func TestCompatibilityDirectoryStatErrorsReachBackupPreparation(t *testing.T) {
	if usesAnchoredCompatibilityTransaction() {
		t.Skip("Windows compatibility discovery intentionally has no path-based lstat seam")
	}
	for _, statTarget := range []string{".agents", filepath.Join(".agents", "skills")} {
		t.Run(statTarget, func(t *testing.T) {
			home := t.TempDir()
			if err := os.MkdirAll(filepath.Join(home, ".agents"), 0o755); err != nil {
				t.Fatal(err)
			}
			failedPath := filepath.Join(home, statTarget)
			pathErr := errors.New("injected compatibility directory failure")
			originalLstat := lstatCompatibilityDestination
			lstatCompatibilityDestination = func(path string) (os.FileInfo, error) {
				if path == failedPath {
					return nil, pathErr
				}
				return originalLstat(path)
			}
			t.Cleanup(func() { lstatCompatibilityDestination = originalLstat })

			selection := model.Selection{Components: []model.ComponentID{model.ComponentSkills}, Skills: []model.SkillID{model.SkillGoTesting}}
			resolved := planner.ResolvedPlan{OrderedComponents: selection.Components}
			backupRoot := filepath.Join(home, "backups")
			plans := []struct {
				name string
				plan pipeline.StagePlan
			}{
				{name: "install", plan: (&installRuntime{homeDir: home, selection: selection, resolved: resolved, backupRoot: backupRoot, state: &runtimeState{}}).stagePlan()},
				{name: "sync", plan: (&syncRuntime{homeDir: home, selection: selection, backupRoot: backupRoot, state: &runtimeState{}}).stagePlan()},
			}
			for _, tt := range plans {
				t.Run(tt.name, func(t *testing.T) {
					var backupStep pipeline.Step
					for _, step := range tt.plan.Prepare {
						if step.ID() == "prepare:backup-snapshot" {
							backupStep = step
							break
						}
					}
					if backupStep == nil {
						t.Fatal("prepare backup step not found")
					}
					err := backupStep.Run()
					if !errors.Is(err, pathErr) || !strings.Contains(err.Error(), "resolve backup targets") {
						t.Fatalf("backup preparation error = %v, want propagated directory stat failure", err)
					}
					if _, statErr := os.Stat(backupRoot); !os.IsNotExist(statErr) {
						t.Fatalf("backup preparation mutated the filesystem after directory stat failure: %v", statErr)
					}
				})
			}
		})
	}
}

func TestCompatibilityDirectoryStatErrorsPreventZeroAgentSyncNoOp(t *testing.T) {
	if usesAnchoredCompatibilityTransaction() {
		t.Skip("Windows compatibility discovery intentionally has no path-based lstat seam")
	}
	for _, statTarget := range []string{".agents", filepath.Join(".agents", "skills")} {
		t.Run(statTarget, func(t *testing.T) {
			home := t.TempDir()
			if err := os.MkdirAll(filepath.Join(home, ".agents"), 0o755); err != nil {
				t.Fatal(err)
			}
			failedPath := filepath.Join(home, statTarget)
			pathErr := errors.New("injected compatibility directory failure")
			originalLstat := lstatCompatibilityDestination
			lstatCompatibilityDestination = func(path string) (os.FileInfo, error) {
				if path == failedPath {
					return nil, pathErr
				}
				return originalLstat(path)
			}
			t.Cleanup(func() { lstatCompatibilityDestination = originalLstat })

			selection := model.Selection{Components: []model.ComponentID{model.ComponentSkills}, Skills: []model.SkillID{model.SkillGoTesting}}
			result, err := RunSyncWithSelection(home, selection)
			if !errors.Is(err, pathErr) || result.NoOp {
				t.Fatalf("RunSyncWithSelection() no-op = %t, error = %v; want propagated directory stat failure", result.NoOp, err)
			}

			setSyncTestHome(t, home)
			result, err = RunSync([]string{"--dry-run"})
			if !errors.Is(err, pathErr) || result.NoOp {
				t.Fatalf("RunSync(--dry-run) no-op = %t, error = %v; want propagated directory stat failure", result.NoOp, err)
			}
		})
	}
}

func TestCompatibilityRefreshIgnoresUnrelatedCustomSkillErrors(t *testing.T) {
	home := t.TempDir()
	customPath := filepath.Join(home, ".agents", "skills", "custom-private", "SKILL.md")
	writeStale(t, customPath)
	originalLstat := lstatCompatibilityDestination
	lstatCompatibilityDestination = func(path string) (os.FileInfo, error) {
		if strings.Contains(path, "custom-private") {
			return nil, errors.New("injected unrelated custom-skill failure")
		}
		return originalLstat(path)
	}
	t.Cleanup(func() { lstatCompatibilityDestination = originalLstat })

	step := compatibilitySkillsRefreshStep{homeDir: home, components: []model.ComponentID{model.ComponentSkills}, selection: model.Selection{Skills: []model.SkillID{model.SkillGoTesting}}}
	if err := step.Run(); err != nil {
		t.Fatalf("Run() error = %v, want unrelated custom skill ignored", err)
	}
	content, err := os.ReadFile(customPath)
	if err != nil || string(content) != "stale" {
		t.Fatalf("unrelated custom skill mutated: content=%q error=%v", content, err)
	}
}

func TestCompatibilityRefreshRejectsNonRegularFinalDestinationWithoutMutation(t *testing.T) {
	home := t.TempDir()
	skillsDir := filepath.Join(home, ".agents", "skills")
	destination := filepath.Join(skillsDir, "go-testing", "SKILL.md")
	if err := os.MkdirAll(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(destination, "marker")
	staleReference := filepath.Join(skillsDir, "go-testing", "references", "examples.md")
	writeStale(t, marker)
	writeStale(t, staleReference)

	step := compatibilitySkillsRefreshStep{
		homeDir:    home,
		components: []model.ComponentID{model.ComponentSkills},
		selection:  model.Selection{Skills: []model.SkillID{model.SkillGoTesting}},
	}
	err := step.Run()
	if err == nil || !strings.Contains(err.Error(), "must be a regular file") {
		t.Fatalf("Run() error = %v, want non-regular destination refusal", err)
	}
	for _, path := range []string{marker, staleReference} {
		content, readErr := os.ReadFile(path)
		if readErr != nil || string(content) != "stale" {
			t.Fatalf("compatibility refusal mutated %q: content=%q error=%v", path, content, readErr)
		}
	}
}

func TestCompatibilitySkillFilesAreInstallAndSyncBackupTargets(t *testing.T) {
	home := t.TempDir()
	skillsDir := filepath.Join(home, ".agents", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := []string{
		filepath.Join(skillsDir, "go-testing", "SKILL.md"),
		filepath.Join(skillsDir, "go-testing", "references", "examples.md"),
		filepath.Join(skillsDir, "sdd-apply", "SKILL.md"),
		filepath.Join(skillsDir, "_shared", "persistence-contract.md"),
	}
	selection := model.Selection{Components: []model.ComponentID{model.ComponentSkills, model.ComponentSDD}, Skills: []model.SkillID{model.SkillGoTesting}}
	resolved := planner.ResolvedPlan{OrderedComponents: selection.Components}
	installTargets, installErr := backupTargets(home, "", ScopeGlobal, selection, resolved)
	syncTargets, syncErr := syncBackupTargets(home, "", selection, nil)
	if installErr != nil || syncErr != nil {
		t.Fatalf("resolve backup targets: install=%v sync=%v", installErr, syncErr)
	}
	for _, targets := range [][]string{installTargets, syncTargets} {
		for _, path := range files {
			if !usesAnchoredCompatibilityTransaction() && !slices.Contains(targets, path) {
				t.Errorf("backup targets missing prospective %q; targets=%v", path, targets)
			}
			if usesAnchoredCompatibilityTransaction() && slices.Contains(targets, path) {
				t.Errorf("generic backup target includes anchored compatibility path %q; targets=%v", path, targets)
			}
		}
	}
}

func TestAdapterSkillFilesAreBackupTargets(t *testing.T) {
	home := t.TempDir()
	selection := model.Selection{
		Agents:     []model.AgentID{model.AgentClaudeCode},
		Components: []model.ComponentID{model.ComponentSkills, model.ComponentSDD},
		Skills:     []model.SkillID{model.SkillGoTesting},
	}
	resolved := planner.ResolvedPlan{Agents: selection.Agents, OrderedComponents: selection.Components}
	installTargets, installErr := backupTargets(home, "", ScopeGlobal, selection, resolved)
	syncTargets, syncErr := syncBackupTargets(home, "", selection, resolveAdapters(selection.Agents))
	if installErr != nil || syncErr != nil {
		t.Fatalf("resolve adapter backup targets: install=%v sync=%v", installErr, syncErr)
	}
	for _, targets := range [][]string{installTargets, syncTargets} {
		for _, relative := range []string{
			"go-testing/references/examples.md",
			"_shared/SKILL.md",
			"sdd-onboard/SKILL.md",
			"judgment-day/SKILL.md",
		} {
			path := filepath.Join(home, ".claude", "skills", filepath.FromSlash(relative))
			if !slices.Contains(targets, path) {
				t.Errorf("adapter backup targets missing %q", path)
			}
		}
	}
}

func TestAdapterSkillBackupRollbackRemovesNewFiles(t *testing.T) {
	home := temporaryUserHome(t)
	skillDir := filepath.Join(home, ".claude", "skills")
	selection := model.Selection{Agents: []model.AgentID{model.AgentClaudeCode}, Components: []model.ComponentID{model.ComponentSkills}, Skills: []model.SkillID{model.SkillGoTesting}}
	targets, err := backupTargets(home, "", ScopeGlobal, selection, planner.ResolvedPlan{Agents: selection.Agents, OrderedComponents: selection.Components})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := backup.NewSnapshotter().Create(filepath.Join(home, "snapshot"), targets)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := componentskills.InjectDirectoryWithCapability(skillDir, selection.Skills, ""); err != nil {
		t.Fatal(err)
	}
	created := filepath.Join(skillDir, "go-testing", "references", "examples.md")
	if err := (backup.RestoreService{}).Restore(manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Fatalf("rollback left newly created adapter file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(skillDir, "go-testing", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("rollback left newly created adapter skill: %v", err)
	}
}

func TestPrepareBackupStep_CreatesSnapshotWhenDuplicateChecksumHasDifferentTargets(t *testing.T) {
	home := t.TempDir()
	backupRoot := filepath.Join(home, "backups")
	existing := filepath.Join(home, "existing")
	writeStale(t, existing)
	if _, err := backup.NewSnapshotter().Create(filepath.Join(backupRoot, "first"), []string{existing}); err != nil {
		t.Fatal(err)
	}

	freshDir := filepath.Join(backupRoot, "second")
	state := &runtimeState{}
	step := prepareBackupStep{snapshotter: backup.NewSnapshotter(), snapshotDir: freshDir,
		targets: []string{existing, filepath.Join(home, "absent")}, state: state, backupRoot: backupRoot}
	if err := step.Run(); err != nil {
		t.Fatal(err)
	}
	if state.manifest.RootDir != freshDir || len(state.manifest.Entries) != 2 {
		t.Fatalf("reused stale manifest: root=%q entries=%d", state.manifest.RootDir, len(state.manifest.Entries))
	}
}

func TestCompatibilityRefreshRollbackRemovesNewFilesAfterDuplicateBackup(t *testing.T) {
	home := temporaryUserHome(t)
	existing := filepath.Join(home, ".agents", "skills", "go-testing", "SKILL.md")
	created := filepath.Join(home, ".agents", "skills", "go-testing", "references", "examples.md")
	writeStale(t, existing)
	selection := model.Selection{Components: []model.ComponentID{model.ComponentSkills}, Skills: []model.SkillID{model.SkillGoTesting}}
	initialRuntime, err := newSyncRuntime(home, selection)
	if err != nil {
		t.Fatal(err)
	}
	if err := initialRuntime.stagePlan().Prepare[0].Run(); err != nil {
		t.Fatal(err)
	}
	runtime, err := newSyncRuntime(home, selection)
	if err != nil {
		t.Fatal(err)
	}
	plan := runtime.stagePlan()
	plan.Apply = append(plan.Apply, failingCompatibilityStep{})
	result := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy()).Execute(plan)
	content, readErr := os.ReadFile(existing)
	if result.Err == nil || readErr != nil || string(content) != "stale" {
		t.Fatalf("rollback did not restore existing file: result=%v content=%q err=%v", result.Err, content, readErr)
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Fatalf("rollback left newly created file: %v", err)
	}
}

type failingCompatibilityStep struct{}

func (failingCompatibilityStep) ID() string { return "test:fail-after-refresh" }
func (failingCompatibilityStep) Run() error { return errors.New("injected failure") }

func TestStagePlansRefreshCompatibilitySkillsOncePerOperation(t *testing.T) {
	selection := model.Selection{Agents: []model.AgentID{model.AgentClaudeCode, model.AgentCodex}, Components: []model.ComponentID{model.ComponentSkills, model.ComponentSDD}, Skills: []model.SkillID{model.SkillGoTesting}}
	resolved := planner.ResolvedPlan{Agents: selection.Agents, OrderedComponents: selection.Components}
	plans := []pipeline.StagePlan{
		(&installRuntime{selection: selection, resolved: resolved, state: &runtimeState{}}).stagePlan(),
		(&syncRuntime{selection: selection, agentIDs: selection.Agents, state: &runtimeState{}}).stagePlan(),
	}
	for _, plan := range plans {
		count := 0
		for _, step := range plan.Apply {
			if step.ID() == "component:compatibility-skills-refresh" || step.ID() == "sync:compatibility-skills-refresh" {
				count++
			}
		}
		if count != 1 {
			t.Errorf("compatibility refresh count=%d, want 1", count)
		}
	}
}
