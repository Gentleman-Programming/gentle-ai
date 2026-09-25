package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v3/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/opencodedefault"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v3/internal/planner"
	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
)

// Exercise the public post-apply boundaries with a real owned agent and ledger.
// The backup is deliberately deduplicated, so rollback depends on the temporary snapshot.
func TestNativeReviewPostApplyErrorsRestoreDeduplicatedSnapshot(t *testing.T) {
	for _, branch := range []string{"install digest", "sync comparison", "sync digest"} {
		t.Run(branch, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			originalHome, originalCommand, originalLookPath := osUserHomeDir, runCommand, cmdLookPath
			osUserHomeDir = func() (string, error) { return home, nil }
			runCommand = func(string, ...string) error { return nil }
			cmdLookPath = func(string) (string, error) { return "", exec.ErrNotFound }
			t.Cleanup(func() { osUserHomeDir, runCommand, cmdLookPath = originalHome, originalCommand, originalLookPath })
			if _, err := RunInstall([]string{"--agent", "cursor"}, system.DetectionResult{}); err != nil {
				t.Fatalf("seed install: %v", err)
			}
			adapter, err := agents.NewAdapter(model.AgentCursor)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{CodeGraphGuidanceMarkdown: "previous guidance"}); err != nil {
				t.Fatal(err)
			}
			targets := []string{filepath.Join(adapter.SubAgentsDir(home), reviewassets.OwnershipLedgerFilename), filepath.Join(adapter.SubAgentsDir(home), "review-risk.md")}
			before := make(map[string][]byte)
			for _, path := range targets {
				before[path], err = os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
			}
			failure := errors.New("injected post-apply failure")
			var transaction *runtimeState
			assertApplied := func() {
				for _, path := range targets {
					got, err := os.ReadFile(path)
					if err != nil || bytes.Equal(got, before[path]) {
						t.Fatalf("post-apply failure did not follow a change to %s: %v", path, err)
					}
				}
				if transaction == nil || transaction.rollbackSnapshotDir == "" {
					t.Fatal("deduplicated temporary snapshot missing before failure")
				}
				if _, err := os.Stat(transaction.rollbackSnapshotDir); err != nil {
					t.Fatalf("deduplicated temporary snapshot missing before failure: %v", err)
				}
			}
			originalDigest, originalCompare := deriveManagedAssetWriter, compareChangedSyncFiles
			originalInstall, originalSync := installStagePlan, syncStagePlan
			t.Cleanup(func() {
				deriveManagedAssetWriter, compareChangedSyncFiles = originalDigest, originalCompare
				installStagePlan, syncStagePlan = originalInstall, originalSync
			})
			if branch != "sync comparison" {
				deriveManagedAssetWriter = func() (string, error) { assertApplied(); return "", failure }
			} else {
				compareChangedSyncFiles = func([]string, map[string]syncFileSnapshot) ([]string, error) { assertApplied(); return nil, failure }
			}
			planFor := func(backupRoot, workspace string, state *runtimeState, selection model.Selection, changed *[]string) pipeline.StagePlan {
				transaction = state
				if _, err := backup.NewSnapshotter().Create(filepath.Join(backupRoot, "existing"), targets); err != nil {
					t.Fatal(err)
				}
				snapshotDir := filepath.Join(backupRoot, "next")
				return pipeline.StagePlan{
					Prepare: []pipeline.Step{prepareBackupStep{id: "backup", snapshotter: backup.NewSnapshotter(), snapshotDir: snapshotDir, targets: targets, state: state, backupRoot: backupRoot}},
					Apply:   []pipeline.Step{rollbackRestoreStep{id: "restore", state: state, homeDir: home, workspaceDir: workspace}, nativeReviewAgentStep{id: "native", agent: model.AgentCursor, homeDir: home, workspaceDir: workspace, scope: ScopeGlobal, selection: selection, changedFiles: changed, state: state}},
				}
			}
			selection := model.Selection{Agents: []model.AgentID{model.AgentCursor}}
			var runErr error
			if branch == "install digest" {
				installStagePlan = func(rt *installRuntime) pipeline.StagePlan {
					return planFor(rt.backupRoot, rt.workspaceDir, rt.state, selection, nil)
				}
				_, runErr = RunInstall([]string{"--agent", "cursor"}, system.DetectionResult{})
			} else {
				syncStagePlan = func(rt *syncRuntime) pipeline.StagePlan {
					plan := planFor(rt.backupRoot, rt.workspaceDir, rt.state, selection, &rt.changedFiles)
					rt.managedPaths = targets
					return plan
				}
				_, runErr = RunSyncWithSelection(home, selection)
			}
			if !errors.Is(runErr, failure) {
				t.Fatalf("%s returned %v, want injected failure", branch, runErr)
			}
			for _, path := range targets {
				got, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(got, before[path]) {
					t.Fatalf("%s left changed %s: %v", branch, path, err)
				}
			}
			if transaction.rollbackSnapshotDir != "" {
				t.Fatalf("%s retained transaction snapshot: %s", branch, transaction.rollbackSnapshotDir)
			}
		})
	}
}

type failingPostApplyRollbackStep struct{ err error }

func (s failingPostApplyRollbackStep) ID() string      { return "failing-post-apply-rollback" }
func (s failingPostApplyRollbackStep) Run() error      { return nil }
func (s failingPostApplyRollbackStep) Rollback() error { return s.err }

func TestNativeReviewPostApplyErrorJoinsRollbackFailure(t *testing.T) {
	cause, rollbackFailure := errors.New("post-apply error"), errors.New("rollback error")
	orchestrator := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy())
	execution := orchestrator.Execute(pipeline.StagePlan{Apply: []pipeline.Step{failingPostApplyRollbackStep{err: rollbackFailure}}})
	if execution.Err != nil {
		t.Fatal(execution.Err)
	}
	err := rollbackPostApplyError(orchestrator, execution, cause)
	if !errors.Is(err, cause) || !errors.Is(err, rollbackFailure) {
		t.Fatalf("joined error = %v", err)
	}
}

// A deduplicated backup still needs a live transaction snapshot until the TUI
// caller has persisted state (or rolled back a failed persistence).
func TestTUIDeduplicatedNativeReviewSnapshotSurvivesPostApplyRollback(t *testing.T) {
	for _, outcome := range []string{"post-apply rollback", "persisted success"} {
		t.Run(outcome, func(t *testing.T) {
			home := t.TempDir()
			adapter, err := agents.NewAdapter(model.AgentCursor)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{CodeGraphGuidanceMarkdown: "previous guidance"}); err != nil {
				t.Fatal(err)
			}
			ledger := filepath.Join(adapter.SubAgentsDir(home), reviewassets.OwnershipLedgerFilename)
			agent := filepath.Join(adapter.SubAgentsDir(home), "review-risk.md")
			targets := []string{ledger, agent}
			before := make(map[string][]byte)
			for _, path := range targets {
				before[path], err = os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
			}
			selection := model.Selection{Agents: []model.AgentID{model.AgentCursor}}
			resolved := planner.ResolvedPlan{Agents: selection.Agents}
			var transaction *runtimeState
			original := tuiInstallStagePlan
			tuiInstallStagePlan = func(rt *installRuntime) pipeline.StagePlan {
				transaction = rt.state
				if _, err := backup.NewSnapshotter().Create(filepath.Join(rt.backupRoot, "existing"), targets); err != nil {
					t.Fatal(err)
				}
				return pipeline.StagePlan{
					Prepare: []pipeline.Step{prepareBackupStep{id: "backup", snapshotter: backup.NewSnapshotter(), snapshotDir: filepath.Join(rt.backupRoot, "next"), targets: targets, state: rt.state, backupRoot: rt.backupRoot}},
					Apply: []pipeline.Step{
						rollbackRestoreStep{id: "restore", state: rt.state, homeDir: home, workspaceDir: rt.workspaceDir},
						nativeReviewAgentStep{id: "native", agent: model.AgentCursor, homeDir: home, workspaceDir: rt.workspaceDir, scope: ScopeGlobal, selection: selection, state: rt.state},
					},
				}
			}
			t.Cleanup(func() { tuiInstallStagePlan = original })
			result, orchestrator := ExecuteTUIInstallWithBackgroundAndOrchestrator(home, selection, resolved, system.PlatformProfile{OS: "darwin", Supported: true}, model.OpenCodeBackgroundAuto, "", nil)
			if result.Err != nil {
				t.Fatalf("apply: %v", result.Err)
			}
			if transaction.rollbackSnapshotDir == "" {
				t.Fatal("expected deduplicated transaction snapshot")
			}
			for _, path := range targets {
				content, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if bytes.Equal(content, before[path]) {
					t.Fatalf("%s was not updated before simulated persistence failure", path)
				}
			}
			dir := transaction.rollbackSnapshotDir
			if _, err := os.Stat(dir); err != nil {
				t.Fatalf("snapshot must survive apply until persistence settles: %v", err)
			}
			if outcome == "post-apply rollback" {
				rollback := orchestrator.Rollback(result)
				if rollback.Err != nil {
					t.Fatalf("post-apply rollback: %v", rollback.Err)
				}
				for _, path := range targets {
					content, err := os.ReadFile(path)
					if err != nil || !bytes.Equal(content, before[path]) {
						t.Fatalf("rollback did not restore %s: %v", path, err)
					}
				}
			} else {
				orchestrator.Finish()
				orchestrator.Finish() // settling twice must not re-run cleanup
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatalf("transaction snapshot not cleaned after %s: %v", outcome, err)
			}
		})
	}
}

func TestNativeReviewLedgerInstallBackupAndManualActions(t *testing.T) {
	home, workspace := t.TempDir(), t.TempDir()
	adapter, err := agents.NewAdapter(model.AgentCursor)
	if err != nil {
		t.Fatal(err)
	}
	ledger := filepath.Join(adapter.SubAgentsDir(home), reviewassets.OwnershipLedgerFilename)
	selection := model.Selection{Agents: []model.AgentID{model.AgentCursor}}
	targets, err := backupTargets(home, workspace, ScopeGlobal, selection, planner.ResolvedPlan{Agents: selection.Agents})
	if err != nil {
		t.Fatal(err)
	}
	if !containsPath(targets, ledger) {
		t.Fatalf("install backup missing ledger %s", ledger)
	}
	path := filepath.Join(adapter.SubAgentsDir(home), "review-risk.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("my review agent"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := &runtimeState{}
	step := nativeReviewAgentStep{id: "test", agent: model.AgentCursor, homeDir: home, workspaceDir: workspace, scope: ScopeGlobal, selection: selection, state: state}
	if err := step.Run(); err != nil {
		t.Fatal(err)
	}
	if len(state.nativeReviewActions) != 1 || !strings.Contains(state.nativeReviewActions[0], path) || !strings.Contains(state.nativeReviewActions[0], "preserved") {
		t.Fatalf("preserved-file actions = %v", state.nativeReviewActions)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "my review agent" {
		t.Fatalf("user bytes = %q, error = %v", data, err)
	}
}

func TestComponentPathsGlobalOpenClawAndPiPersonaMatchBackup(t *testing.T) {
	home, workspace := t.TempDir(), t.TempDir()
	selection := model.Selection{
		Agents:     []model.AgentID{model.AgentOpenClaw, model.AgentPi},
		Components: []model.ComponentID{model.ComponentPersona, model.ComponentEngram, model.ComponentSDD, model.ComponentSkills},
		Persona:    model.PersonaGentleman, Skills: []model.SkillID{model.SkillGoTesting},
	}
	adapters := resolveAdapters(selection.Agents)
	targets, err := backupTargets(home, workspace, ScopeGlobal, selection, planner.ResolvedPlan{Agents: selection.Agents, OrderedComponents: selection.Components})
	if err != nil {
		t.Fatal(err)
	}
	for _, component := range selection.Components {
		paths := componentPathsWithWorkspaceScoped(home, workspace, ScopeGlobal, selection, adapters, component)
		for _, path := range paths {
			if !strings.HasPrefix(path, home+string(filepath.Separator)) || !containsPath(targets, path) {
				t.Errorf("%s verification path %q must be at home and backed up", component, path)
			}
		}
	}
	for _, path := range targets {
		if strings.HasPrefix(path, workspace+string(filepath.Separator)) {
			t.Errorf("backup escaped global scope: %s", path)
		}
	}
	for _, relative := range []string{"AGENTS.md", "SOUL.md", ".openclaw/openclaw.json", ".openclaw/skills/go-testing/SKILL.md", ".pi/gentle-ai/persona.json"} {
		if !containsPath(targets, filepath.Join(home, relative)) {
			t.Errorf("backup missing %s", relative)
		}
	}
}

func TestComponentPathsSDDIncludesSystemPromptForPromptFileAdapters(t *testing.T) {
	home := t.TempDir()
	adapters := resolveAdapters([]model.AgentID{
		model.AgentClaudeCode,
		model.AgentGeminiCLI,
		model.AgentCursor,
		model.AgentVSCodeCopilot,
	})

	paths := componentPaths(home, model.Selection{}, adapters, model.ComponentSDD)

	for _, adapter := range adapters {
		p := adapter.SystemPromptFile(home)
		if !containsPath(paths, p) {
			t.Fatalf("componentPaths(sdd) missing system prompt path %q\npaths=%v", p, paths)
		}
	}
}

// TestComponentPathsSDDExcludesSystemPromptForManagedOpenCodeAgents pins issue
// #3975: the SDD injector scopes the orchestrator to the gentle-orchestrator
// agent in the settings file for OpenCode and Kilocode in every mode, so SDD
// must not require, back up, or verify the AGENTS.md it never writes.
func TestComponentPathsSDDExcludesSystemPromptForManagedOpenCodeAgents(t *testing.T) {
	home := t.TempDir()
	adapters := resolveAdapters([]model.AgentID{model.AgentOpenCode, model.AgentKilocode})

	for _, mode := range []model.SDDModeID{"", model.SDDModeSingle, model.SDDModeMulti} {
		paths := componentPaths(home, model.Selection{SDDMode: mode}, adapters, model.ComponentSDD)
		for _, adapter := range adapters {
			p := adapter.SystemPromptFile(home)
			if containsPath(paths, p) {
				t.Fatalf("componentPaths(sdd, mode=%q) lists %q, which the SDD injector never writes for %s\npaths=%v", mode, p, adapter.Agent(), paths)
			}
		}
	}
}

func TestComponentPathsLegacyCommandsExactAndAbsentFromODD(t *testing.T) {
	home := t.TempDir()
	for _, tc := range []struct {
		agent  model.AgentID
		dir    string
		prefix string
	}{
		{model.AgentClaudeCode, ".claude/commands", "gentle-"},
		{model.AgentOpenCode, ".config/opencode/commands", ""},
	} {
		adapter := resolveAdapters([]model.AgentID{tc.agent})[0]
		paths := componentPaths(home, model.Selection{}, []agents.Adapter{adapter}, model.ComponentSDD)
		names := []string{"sdd-init", "sdd-new", "sdd-continue", "sdd-status", "sdd-explore", "sdd-research", "sdd-ff", "sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard"}
		for _, name := range names {
			for _, prefix := range []string{tc.prefix, ""} {
				path := filepath.Join(home, tc.dir, prefix+name+".md")
				if !containsPath(paths, path) {
					t.Errorf("legacy inventory missing %s", path)
				}
			}
		}
		active := componentPaths(home, model.Selection{}, []agents.Adapter{adapter}, model.ComponentPersona)
		for _, path := range active {
			if strings.Contains(path, "/commands/sdd-") || strings.Contains(path, "/commands/gentle-sdd-") {
				t.Errorf("active ODD inventory includes legacy command %s", path)
			}
		}
	}
}

func TestComponentPathsSDDIncludesOpenCodeSettingsAndCommands(t *testing.T) {
	home := t.TempDir()
	adapters := resolveAdapters([]model.AgentID{model.AgentOpenCode})

	paths := componentPaths(home, model.Selection{}, adapters, model.ComponentSDD)

	settings := filepath.Join(home, ".config", "opencode", "opencode.json")
	if !containsPath(paths, settings) {
		t.Fatalf("componentPaths(sdd) missing OpenCode settings path %q\npaths=%v", settings, paths)
	}

	command := filepath.Join(home, ".config", "opencode", "commands", "sdd-init.md")
	if !containsPath(paths, command) {
		t.Fatalf("componentPaths(sdd) missing OpenCode command path %q\npaths=%v", command, paths)
	}
}

func TestComponentPathsClaudeRetainsReviewAndRouting(t *testing.T) {
	home := t.TempDir()
	selection := model.Selection{Agents: []model.AgentID{model.AgentClaudeCode}}
	targets, err := backupTargets(home, "", ScopeGlobal, selection, planner.ResolvedPlan{Agents: selection.Agents})
	if err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{".claude/CLAUDE.md", ".claude/agents/review-risk.md", ".claude/agents/jd-judge-a.md"} {
		if !containsPath(targets, filepath.Join(home, relative)) {
			t.Errorf("retained Claude path missing: %s", relative)
		}
	}
}

func TestComponentPathsSDDMultiIncludesOpenCodePlugins(t *testing.T) {
	assertRetainedOpenCodePluginPaths(t)
}

func TestComponentPathsSDDSingleIncludesOpenCodePlugins(t *testing.T) {
	assertRetainedOpenCodePluginPaths(t)
}

func assertRetainedOpenCodePluginPaths(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	selection := model.Selection{Agents: []model.AgentID{model.AgentOpenCode}}
	targets, err := backupTargets(home, "", ScopeGlobal, selection, planner.ResolvedPlan{Agents: selection.Agents})
	if err != nil {
		t.Fatal(err)
	}
	for _, plugin := range []string{"model-variants.ts", "opencode-review-transport.ts", "skill-registry.ts", "telemetry-runtime.ts"} {
		want := filepath.Join(home, ".config", "opencode", "plugins", plugin)
		if !containsPath(targets, want) {
			t.Errorf("ODD install snapshot missing retained plugin %s", want)
		}
	}
	if containsPath(targets, filepath.Join(home, ".config", "opencode", "plugins", "sdd-task-result-artifacts.ts")) {
		t.Error("retired plugin in snapshot")
	}
}

func TestComponentPathsWorkspaceScopedOpenCodeSDDUsesWorkspaceManagedPaths(t *testing.T) {
	home, workspace := t.TempDir(), t.TempDir()
	selection := model.Selection{Agents: []model.AgentID{model.AgentOpenCode}, Skills: []model.SkillID{model.SkillGoTesting}}
	selection.Components = []model.ComponentID{model.ComponentSkills}
	paths := componentPathsWithWorkspaceScoped(home, workspace, ScopeWorkspace, selection, resolveAdapters(selection.Agents), model.ComponentSkills)
	want := filepath.Join(workspace, ".config", "opencode", "skills", "go-testing", "SKILL.md")
	if !containsPath(paths, want) {
		t.Fatalf("workspace skill missing: %v", paths)
	}
	if containsPath(paths, filepath.Join(home, ".config", "opencode", "skills", "go-testing", "SKILL.md")) {
		t.Fatalf("home skill leaked: %v", paths)
	}
}

func TestComponentPathsPiPersonaUsesResolvedScopePath(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	adapters := resolveAdapters([]model.AgentID{model.AgentPi})
	selection := model.Selection{Persona: model.PersonaNeutral}

	global := componentPathsWithWorkspaceScoped(home, workspace, ScopeGlobal, selection, adapters, model.ComponentPersona)
	if !containsPath(global, filepath.Join(home, ".pi", "gentle-ai", "persona.json")) {
		t.Fatalf("global Pi persona paths = %v, missing home-scoped config", global)
	}
	if containsPath(global, filepath.Join(workspace, ".pi", "gentle-ai", "persona.json")) {
		t.Fatalf("global Pi persona paths = %v, includes active workspace config", global)
	}

	workspacePaths := componentPathsWithWorkspaceScoped(home, workspace, ScopeWorkspace, selection, adapters, model.ComponentPersona)
	if !containsPath(workspacePaths, filepath.Join(workspace, ".pi", "gentle-ai", "persona.json")) {
		t.Fatalf("workspace Pi persona paths = %v, missing workspace-scoped config", workspacePaths)
	}
	if containsPath(workspacePaths, filepath.Join(home, ".pi", "gentle-ai", "persona.json")) {
		t.Fatalf("workspace Pi persona paths = %v, unexpectedly contains home config", workspacePaths)
	}

	custom := componentPathsWithWorkspaceScoped(home, workspace, ScopeWorkspace, model.Selection{Persona: model.PersonaCustom}, adapters, model.ComponentPersona)
	if len(custom) != 0 {
		t.Fatalf("custom Pi persona paths = %v, want none", custom)
	}
}

func TestInstallPiPersonaWritesManagedScopePaths(t *testing.T) {
	for _, tt := range []struct {
		name  string
		scope InstallScope
	}{
		{name: "global", scope: ScopeGlobal},
		{name: "workspace", scope: ScopeWorkspace},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			workspace := t.TempDir()
			root, other := home, workspace
			if tt.scope == ScopeWorkspace {
				root, other = workspace, home
			}

			step := componentApplyStep{
				component:    model.ComponentPersona,
				homeDir:      home,
				workspaceDir: workspace,
				scope:        tt.scope,
				agents:       []model.AgentID{model.AgentPi},
				selection:    model.Selection{Persona: model.PersonaNeutral},
			}
			if err := step.Run(); err != nil {
				t.Fatalf("componentApplyStep.Run() error = %v", err)
			}

			want := filepath.Join(root, ".pi", "gentle-ai", "persona.json")
			if _, err := os.Stat(want); err != nil {
				t.Fatalf("Pi persona config %q was not written: %v", want, err)
			}
			unwanted := filepath.Join(other, ".pi", "gentle-ai", "persona.json")
			if _, err := os.Stat(unwanted); !os.IsNotExist(err) {
				t.Fatalf("workspace-scoped Pi persona config %q was written outside scope; stat err = %v", unwanted, err)
			}
		})
	}
}

// Issue #3219: a home installed with XDG_CONFIG_HOME keeps the legacy plugin
// under $XDG_CONFIG_HOME/opencode/plugins, and the ~/.config form stays legacy
// while XDG is set; the classification depends on the path and XDG alone.
func TestLegacyOpenCodeBackgroundAgentsPluginUnderXDGConfigHome(t *testing.T) {
	home := t.TempDir()
	xdg := filepath.Join(t.TempDir(), "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	for _, tt := range []struct {
		name string
		path string
		want bool
	}{
		{name: "legacy plugin under XDG opencode config", path: filepath.Join(xdg, "opencode", "plugins", "background-agents.ts"), want: true},
		{name: "legacy plugin under ~/.config while XDG is set", path: filepath.Join(home, ".config", "opencode", "plugins", "background-agents.ts"), want: true},
		{name: "same file under unrelated opencode directory", path: filepath.Join(home, "opencode", "plugins", "background-agents.ts"), want: false},
		{name: "managed replacement plugin under XDG is not legacy", path: filepath.Join(xdg, "opencode", "plugins", "model-variants.ts"), want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := isLegacyOpenCodeBackgroundAgentsPlugin(tt.path); got != tt.want {
				t.Fatalf("isLegacyOpenCodeBackgroundAgentsPlugin(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestLegacyOpenCodeBackgroundAgentsPluginRequiresConfigOpenCodePluginsPath(t *testing.T) {
	home := t.TempDir()

	for _, tt := range []struct {
		name string
		path string
		want bool
	}{
		{
			name: "legacy plugin under opencode config",
			path: filepath.Join(home, ".config", "opencode", "plugins", "background-agents.ts"),
			want: true,
		},
		{
			name: "same file under unrelated opencode directory",
			path: filepath.Join(home, "opencode", "plugins", "background-agents.ts"),
			want: false,
		},
		{
			name: "managed replacement plugin is not legacy",
			path: filepath.Join(home, ".config", "opencode", "plugins", "model-variants.ts"),
			want: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := isLegacyOpenCodeBackgroundAgentsPlugin(tt.path); got != tt.want {
				t.Fatalf("isLegacyOpenCodeBackgroundAgentsPlugin(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestComponentPathsSDDIncludesSkillsAndSharedConventions(t *testing.T) {
	home := t.TempDir()
	selection := model.Selection{Skills: []model.SkillID{model.SkillGoTesting, model.SkillJudgmentDay}}
	paths := componentPaths(home, selection, resolveAdapters([]model.AgentID{model.AgentGeminiCLI}), model.ComponentSkills)
	for _, skill := range []string{"go-testing", "judgment-day"} {
		want := filepath.Join(home, ".gemini", "skills", skill, "SKILL.md")
		if !containsPath(paths, want) {
			t.Errorf("retained skill missing: %s", want)
		}
	}
}

func TestComponentPathsWithWorkspaceOpenClawSDDUsesWorkspaceScopedSkills(t *testing.T) {
	home, workspace := t.TempDir(), t.TempDir()
	selection := model.Selection{Skills: []model.SkillID{model.SkillGoTesting}}
	adapters := resolveAdapters([]model.AgentID{model.AgentOpenClaw})
	paths := componentPathsWithWorkspaceScoped(home, workspace, ScopeWorkspace, selection, adapters, model.ComponentSkills)
	want := filepath.Join(workspace, ".openclaw", "skills", "go-testing", "SKILL.md")
	if !containsPath(paths, want) {
		t.Fatalf("workspace skill missing: %v", paths)
	}
	if containsPath(paths, filepath.Join(home, ".openclaw", "skills", "go-testing", "SKILL.md")) {
		t.Fatalf("home skill leaked: %v", paths)
	}
}

func TestComponentPathsOpenClawSkillsSkipsSDDPhaseSkills(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	adapters := resolveAdapters([]model.AgentID{model.AgentOpenClaw})
	selection := model.Selection{
		Skills: []model.SkillID{
			model.SkillSDDInit,
			model.SkillGoTesting,
			model.SkillSDDOnboard,
		},
	}

	paths := componentPathsWithWorkspaceScoped(home, workspace, ScopeWorkspace, selection, adapters, model.ComponentSkills)

	want := filepath.Join(workspace, ".openclaw", "skills", "go-testing", "SKILL.md")
	if !containsPath(paths, want) {
		t.Fatalf("componentPaths(skills,openclaw) missing portable skill path %q\npaths=%v", want, paths)
	}

	for _, unwanted := range []string{
		filepath.Join(workspace, ".openclaw", "skills", "sdd-init", "SKILL.md"),
		filepath.Join(workspace, ".openclaw", "skills", "sdd-onboard", "SKILL.md"),
	} {
		if containsPath(paths, unwanted) {
			t.Fatalf("componentPaths(skills,openclaw) must not verify SDD phase skill path %q\npaths=%v", unwanted, paths)
		}
	}
}

func TestComponentPathsWorkspaceScopedSkillsUsesWorkspaceDir(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	adapters := resolveAdapters([]model.AgentID{model.AgentClaudeCode})
	selection := model.Selection{
		Skills: []model.SkillID{
			model.SkillGoTesting,
			model.SkillBranchPR,
		},
	}

	paths := componentPathsWithWorkspaceScoped(home, workspace, ScopeWorkspace, selection, adapters, model.ComponentSkills)

	for _, want := range []string{
		filepath.Join(workspace, ".claude", "skills", "go-testing", "SKILL.md"),
		filepath.Join(workspace, ".claude", "skills", "branch-pr", "SKILL.md"),
	} {
		if !containsPath(paths, want) {
			t.Fatalf("componentPathsWithWorkspaceScoped(skills,claude-code,workspace) missing workspace-scoped path %q\npaths=%v", want, paths)
		}
	}

	for _, unwanted := range []string{
		filepath.Join(home, ".claude", "skills", "go-testing", "SKILL.md"),
		filepath.Join(home, ".claude", "skills", "branch-pr", "SKILL.md"),
	} {
		if containsPath(paths, unwanted) {
			t.Fatalf("componentPathsWithWorkspaceScoped(skills,claude-code,workspace) must not include home-scoped path %q\npaths=%v", unwanted, paths)
		}
	}
}

// TestInstallWorkspaceScopeVerificationWithNoGlobalSkills verifies that
// post-apply verification succeeds when --scope=workspace is used and no
// global skill files exist. This is a regression test for issue #785:
// the verifier used to check home-scoped paths even when workspace scope
// was active, causing false failures when only workspace skills existed.
func TestInstallWorkspaceScopeVerificationWithNoGlobalSkills(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	adapters := resolveAdapters([]model.AgentID{model.AgentClaudeCode})
	selection := model.Selection{
		Skills: []model.SkillID{
			model.SkillGoTesting,
			model.SkillBranchPR,
		},
	}

	// Simulate workspace-scoped install: skills are written to workspace only.
	// The verification should check workspace paths, not home paths.
	paths := componentPathsWithWorkspaceScoped(home, workspace, ScopeWorkspace, selection, adapters, model.ComponentSkills)

	// Verify that workspace paths are included (these should exist after install).
	for _, want := range []string{
		filepath.Join(workspace, ".claude", "skills", "go-testing", "SKILL.md"),
		filepath.Join(workspace, ".claude", "skills", "branch-pr", "SKILL.md"),
	} {
		if !containsPath(paths, want) {
			t.Fatalf("workspace-scoped verification missing workspace path %q\npaths=%v", want, paths)
		}
	}

	// Verify that home paths are NOT included (these would cause false failures
	// if checked when only workspace skills exist).
	for _, unwanted := range []string{
		filepath.Join(home, ".claude", "skills", "go-testing", "SKILL.md"),
		filepath.Join(home, ".claude", "skills", "branch-pr", "SKILL.md"),
	} {
		if containsPath(paths, unwanted) {
			t.Fatalf("workspace-scoped verification must not check home path %q when scope=workspace\npaths=%v", unwanted, paths)
		}
	}
}

func TestComponentPathsSDDKimiIncludesAgentFilesAndGlobalSkills(t *testing.T) {
	home := t.TempDir()
	selection := model.Selection{Agents: []model.AgentID{model.AgentKimi}, Components: []model.ComponentID{model.ComponentSkills}, Skills: []model.SkillID{model.SkillGoTesting}}
	targets, err := backupTargets(home, "", ScopeGlobal, selection, planner.ResolvedPlan{Agents: selection.Agents, OrderedComponents: selection.Components})
	if err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{".kimi/KIMI.md", ".kimi/agents/gentleman.yaml", ".kimi/agents/review-risk.yaml", ".config/agents/skills/go-testing/SKILL.md"} {
		if !containsPath(targets, filepath.Join(home, relative)) {
			t.Errorf("retained Kimi path missing: %s", relative)
		}
	}
}

func TestComponentPathsContext7KimiIncludesMCPConfig(t *testing.T) {
	home := t.TempDir()
	adapters := resolveAdapters([]model.AgentID{model.AgentKimi})

	paths := componentPaths(home, model.Selection{}, adapters, model.ComponentContext7)

	want := filepath.Join(home, ".kimi", "mcp.json")
	if !containsPath(paths, want) {
		t.Fatalf("componentPaths(context7,kimi) missing %q\npaths=%v", want, paths)
	}
}

// TestComponentPathsContext7ClaudeUsesUserRegistry pins Claude Context7 to
// the file injection actually writes: ~/.claude.json (issue #1868).
// settings.json is only mutated best-effort and may not exist, and the legacy
// managed ~/.claude/mcp/context7.json is removed by injection, so verifying
// either would fail on a healthy install.
func TestComponentPathsContext7ClaudeUsesUserRegistry(t *testing.T) {
	home := t.TempDir()
	adapters := resolveAdapters([]model.AgentID{model.AgentClaudeCode})

	paths := componentPaths(home, model.Selection{}, adapters, model.ComponentContext7)

	registry := filepath.Join(home, ".claude.json")
	if !containsPath(paths, registry) {
		t.Fatalf("componentPaths(context7,claude) missing %q\npaths=%v", registry, paths)
	}
	for _, absent := range []string{
		filepath.Join(home, ".claude", "mcp", "context7.json"),
		filepath.Join(home, ".claude", "settings.json"),
	} {
		if containsPath(paths, absent) {
			t.Fatalf("componentPaths(context7,claude) must not require %q\npaths=%v", absent, paths)
		}
	}
}

func TestComponentPathsContext7ClaudeRespectsWorkspaceScope(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	adapters := resolveAdapters([]model.AgentID{model.AgentClaudeCode})

	paths := componentPathsWithWorkspaceScoped(home, workspace, ScopeWorkspace, model.Selection{}, adapters, model.ComponentContext7)

	// Workspace scope writes <project-root>/.mcp.json, the file Claude Code
	// loads project-scoped MCP servers from (issue #2213). The legacy
	// .claude/settings.json key is inert for MCP discovery and is not declared.
	want := filepath.Join(workspace, ".mcp.json")
	if !containsPath(paths, want) {
		t.Fatalf("componentPathsWithWorkspaceScoped(context7,claude) with ScopeWorkspace missing %q\npaths=%v", want, paths)
	}
	for _, absent := range []string{
		filepath.Join(workspace, ".claude", "settings.json"),
		filepath.Join(home, ".claude.json"),
	} {
		if containsPath(paths, absent) {
			t.Fatalf("componentPathsWithWorkspaceScoped(context7,claude) with ScopeWorkspace must not require %q\npaths=%v", absent, paths)
		}
	}
}

func TestComponentPathsEngramClaudeUsesUserRegistryAndPreservesWorkspaceScope(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	adapters := resolveAdapters([]model.AgentID{model.AgentClaudeCode})

	global := componentPaths(home, model.Selection{}, adapters, model.ComponentEngram)
	registry := filepath.Join(home, ".claude.json")
	legacy := filepath.Join(home, ".claude", "mcp", "engram.json")
	if !containsPath(global, registry) || containsPath(global, legacy) {
		t.Fatalf("global Engram paths must use only Claude's user registry; paths=%v", global)
	}

	workspacePaths := componentPathsWithWorkspaceScoped(home, workspace, ScopeWorkspace, model.Selection{}, adapters, model.ComponentEngram)
	workspaceLegacy := filepath.Join(workspace, ".claude", "mcp", "engram.json")
	if !containsPath(workspacePaths, workspaceLegacy) || containsPath(workspacePaths, filepath.Join(workspace, ".claude.json")) {
		t.Fatalf("workspace Engram paths must remain unchanged; paths=%v", workspacePaths)
	}
}

// TestComponentPathsEngramCodexIncludesConfigTOML verifies that componentPaths
// for ComponentEngram + Codex reports ~/.codex/config.toml as a backup target.
func TestComponentPathsEngramCodexIncludesConfigTOML(t *testing.T) {
	home := t.TempDir()
	adapters := resolveAdapters([]model.AgentID{model.AgentCodex})

	paths := componentPaths(home, model.Selection{}, adapters, model.ComponentEngram)

	want := filepath.Join(home, ".codex", "config.toml")
	if !containsPath(paths, want) {
		t.Fatalf("componentPaths(engram,codex) missing %q\npaths=%v", want, paths)
	}
}

func TestComponentPathsSDDCodexIncludesHooksJSONOnlyForCodex(t *testing.T) {
	home := t.TempDir()
	adapters := resolveAdapters([]model.AgentID{model.AgentCodex, model.AgentClaudeCode})

	paths := componentPaths(home, model.Selection{}, adapters, model.ComponentSDD)
	codexHooks := filepath.Join(home, ".codex", "hooks.json")
	if !containsPath(paths, codexHooks) {
		t.Fatalf("componentPaths(sdd,codex) missing skill-registry hook %q\npaths=%v", codexHooks, paths)
	}
	claudeHooks := filepath.Join(home, ".claude", "hooks.json")
	if containsPath(paths, claudeHooks) {
		t.Fatalf("componentPaths(sdd,claude) declared unsupported hooks path %q\npaths=%v", claudeHooks, paths)
	}
}

// TestComponentPathsPermissionsCodexContributesNoPaths pins that the
// Permission component claims nothing under ~/.codex. gentle-ai does not write
// Codex's permissions config — not a profile, and not the legacy cleanup that
// used to strip one — so there is no injection target to verify and nothing to
// snapshot for rollback. A path reappearing here would mean something started
// writing that file again (#1794).
func TestComponentPathsPermissionsCodexContributesNoPaths(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(configPath, []byte("model = \"gpt-5.5\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	adapters := resolveAdapters([]model.AgentID{model.AgentCodex})

	paths := componentPaths(home, model.Selection{}, adapters, model.ComponentPermission)

	if len(paths) != 0 {
		t.Fatalf("componentPaths(permissions,codex) = %v, want none", paths)
	}
}

func TestComponentPathsPermissionsSkipsAgentsWithoutInjectionTarget(t *testing.T) {
	home := t.TempDir()
	adapters := resolveAdapters([]model.AgentID{
		model.AgentCursor,
		model.AgentAntigravity,
		model.AgentHermes,
	})

	paths := componentPaths(home, model.Selection{}, adapters, model.ComponentPermission)

	for _, adapter := range adapters {
		unwanted := adapter.SettingsPath(home)
		if unwanted == "" {
			continue
		}
		if containsPath(paths, unwanted) {
			t.Fatalf("componentPaths(permissions) must not include unsupported injection path %q\npaths=%v", unwanted, paths)
		}
	}
}

func TestComponentPathsPermissionsIncludesAgentsWithInjectionTarget(t *testing.T) {
	home := t.TempDir()
	adapters := resolveAdapters([]model.AgentID{
		model.AgentClaudeCode,
		model.AgentOpenCode,
		model.AgentKilocode,
		model.AgentGeminiCLI,
		model.AgentQwenCode,
		model.AgentVSCodeCopilot,
	})

	paths := componentPaths(home, model.Selection{}, adapters, model.ComponentPermission)

	for _, adapter := range adapters {
		want := adapter.SettingsPath(home)
		if !containsPath(paths, want) {
			t.Fatalf("componentPaths(permissions) missing supported injection path %q\npaths=%v", want, paths)
		}
	}
}

// TestComponentPathsEngramOpenClawUsesCanonicalSettingsPath asserts that the
// engram component path for OpenClaw always resolves to the canonical
// ~/.openclaw/openclaw.json and never to a workspace-scoped copy.
//
// This is a regression test for issue #522: the verifier used to call
// SettingsPath(workspaceDir) which produced
// <workspace>/.openclaw/openclaw.json, causing post-sync verification to
// fail even when the file at the canonical path existed.
func TestComponentPathsEngramOpenClawUsesCanonicalSettingsPath(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	adapters := resolveAdapters([]model.AgentID{model.AgentOpenClaw})

	paths := componentPathsWithWorkspace(home, workspace, model.Selection{}, adapters, model.ComponentEngram)

	canonical := filepath.Join(home, ".openclaw", "openclaw.json")
	if !containsPath(paths, canonical) {
		t.Fatalf("componentPathsWithWorkspace(engram,openclaw) missing canonical path %q\npaths=%v", canonical, paths)
	}

	wrongPath := filepath.Join(workspace, ".openclaw", "openclaw.json")
	if containsPath(paths, wrongPath) {
		t.Fatalf("componentPathsWithWorkspace(engram,openclaw) must not include workspace-scoped path %q\npaths=%v", wrongPath, paths)
	}
}

func containsPath(paths []string, want string) bool {
	for _, p := range paths {
		if p == want {
			return true
		}
	}
	return false
}

// ─── Organic routing guidance is installed for every configured agent ──────
//
// Routing guidance decides how an agent picks between direct, delegated, and
// proposed work. It is therefore unconditional: an install that did not select
// the optional SDD component must still receive it (issue #1794).

const (
	routingOpenMarker  = "<!-- gentle-ai:" + agentguidance.RoutingSectionID + " -->"
	routingCloseMarker = "<!-- /gentle-ai:" + agentguidance.RoutingSectionID + " -->"

	legacyTriggerRulesOpenMarker = "<!-- gentle-ai:trigger-rules -->"
)

// newTestInstallRuntime builds an install runtime whose resolved plan mirrors
// the selection, which is what the real planner produces for these inputs.
func newTestInstallRuntime(t *testing.T, home string, selection model.Selection) *installRuntime {
	t.Helper()

	resolved := planner.ResolvedPlan{Agents: selection.Agents, OrderedComponents: selection.Components}
	rt, err := newInstallRuntime(home, ScopeGlobal, ChannelStable, selection, resolved, system.PlatformProfile{PackageManager: "brew"})
	if err != nil {
		t.Fatalf("newInstallRuntime() error = %v", err)
	}
	return rt
}

// runInstallInjectionSteps executes the staged apply steps that write managed
// assets. Agent installation steps are skipped because they shell out to real
// package managers, which is unrelated to what these tests assert.
func runInstallInjectionSteps(t *testing.T, rt *installRuntime) {
	t.Helper()

	for _, step := range rt.stagePlan().Apply {
		if _, isAgentInstall := step.(agentInstallStep); isAgentInstall {
			continue
		}
		if err := step.Run(); err != nil {
			t.Fatalf("Run(%s) error = %v", step.ID(), err)
		}
	}
}

// runInstallComponentSteps executes only the component steps, which is how a
// later run reaches an already-guided installation.
func runInstallComponentSteps(t *testing.T, rt *installRuntime) {
	t.Helper()

	for _, step := range rt.stagePlan().Apply {
		if _, isComponent := step.(componentApplyStep); !isComponent {
			continue
		}
		if err := step.Run(); err != nil {
			t.Fatalf("Run(%s) error = %v", step.ID(), err)
		}
	}
}

func systemPromptFileFor(t *testing.T, home string, agent model.AgentID) string {
	t.Helper()

	adapter, err := agents.NewAdapter(agent)
	if err != nil {
		t.Fatalf("NewAdapter(%q) error = %v", agent, err)
	}
	return adapter.SystemPromptFile(home)
}

func readTextFile(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return string(content)
}

func openCodeSettingsPath(home string) string {
	return filepath.Join(home, ".config", "opencode", "opencode.json")
}

// openCodeOrchestratorPrompt returns the decoded managed orchestrator prompt.
// Reading the raw settings bytes would not do: Go's JSON encoder escapes "<" to
// "<", so the managed markers only exist as such in the decoded string the
// agent actually loads.
func openCodeOrchestratorPrompt(t *testing.T, home string) string {
	t.Helper()

	var settings struct {
		Agent map[string]struct {
			Prompt string `json:"prompt"`
		} `json:"agent"`
	}
	if err := json.Unmarshal([]byte(readTextFile(t, openCodeSettingsPath(home))), &settings); err != nil {
		t.Fatalf("decode OpenCode settings error = %v", err)
	}
	return settings.Agent[opencodedefault.ManagedAgent].Prompt
}

func TestRemoteAuthorizationLeavesPiPackagePromptUntouched(t *testing.T) {
	home := t.TempDir()
	path := systemPromptFileFor(t, home, model.AgentPi)
	const personal = "Package-owned Pi instructions\n"
	mustWriteFile(t, path, []byte(personal))
	runInstallInjectionSteps(t, newTestInstallRuntime(t, home, model.Selection{Agents: []model.AgentID{model.AgentPi}}))
	runSyncInjectionSteps(t, home, model.Selection{Agents: []model.AgentID{model.AgentPi}})
	if readTextFile(t, path) != personal {
		t.Fatal("install/sync modified Pi's package-owned primary carrier")
	}
}

func TestInstallRoutingCleanupRetiresPiPrompt(t *testing.T) {
	home := t.TempDir()
	path := systemPromptFileFor(t, home, model.AgentPi)
	mustWriteFile(t, path, []byte("user\n<!-- gentle-ai:agent-routing -->\nstale\n<!-- /gentle-ai:agent-routing -->\n"))
	runInstallInjectionSteps(t, newTestInstallRuntime(t, home, model.Selection{Agents: []model.AgentID{model.AgentPi}}))
	if got := readTextFile(t, path); strings.Contains(got, "gentle-ai:agent-routing") || !strings.Contains(got, "user") {
		t.Fatalf("install Pi cleanup = %q", got)
	}
}

func TestInstallRemoteAuthorizationIndependentOfComponents(t *testing.T) {
	for _, persona := range []model.PersonaID{"", model.PersonaCustom} {
		t.Run(string(persona), func(t *testing.T) {
			home := t.TempDir()
			selection := model.Selection{Agents: []model.AgentID{model.AgentClaudeCode}, Persona: persona}
			if persona != "" {
				selection.Components = []model.ComponentID{model.ComponentPersona}
			}
			runInstallInjectionSteps(t, newTestInstallRuntime(t, home, selection))
			prompt := readTextFile(t, systemPromptFileFor(t, home, model.AgentClaudeCode))
			if !strings.Contains(prompt, "<!-- gentle-ai:remote-authorization -->") {
				t.Fatal("install omitted remote authorization without SDD/default persona")
			}
		})
	}
}

func TestInstallDeliversRoutingGuidanceWithoutSDDComponent(t *testing.T) {
	home := t.TempDir()

	rt := newTestInstallRuntime(t, home, model.Selection{
		Agents:     []model.AgentID{model.AgentClaudeCode},
		Components: []model.ComponentID{model.ComponentPersona},
		Persona:    model.PersonaGentleman,
	})
	runInstallInjectionSteps(t, rt)

	prompt := readTextFile(t, systemPromptFileFor(t, home, model.AgentClaudeCode))
	if !strings.Contains(prompt, routingOpenMarker) || !strings.Contains(prompt, routingCloseMarker) {
		t.Fatalf("install without the SDD component left the agent unrouted:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Implementation Routing") {
		t.Fatalf("routing section is present but carries no routing guidance:\n%s", prompt)
	}
}

func TestInstallRetainedAgentsWithoutSDD(t *testing.T) {
	for _, agent := range []model.AgentID{model.AgentClaudeCode, model.AgentCursor, model.AgentKimi, model.AgentKiroIDE} {
		t.Run(string(agent), func(t *testing.T) {
			home := t.TempDir()
			selection := model.Selection{Agents: []model.AgentID{agent}}
			run := func() { runInstallInjectionSteps(t, newTestInstallRuntime(t, home, selection)) }
			run()
			adapter := resolveAdapters([]model.AgentID{agent})[0]
			first := map[string]string{}
			for _, name := range reviewassets.NativeAgentManifest[agent] {
				path := filepath.Join(adapter.SubAgentsDir(home), name)
				body := readTextFile(t, path)
				if strings.HasPrefix(name, "review-") && name != "review-refuter.md" && strings.HasSuffix(name, ".md") && !strings.Contains(body, "GENTLE_AI_") {
					t.Fatalf("%s missing lens envelope markers", path)
				}
				first[path] = body
			}
			run()
			for path, body := range first {
				if got := readTextFile(t, path); got != body {
					t.Fatalf("second install changed %s", path)
				}
			}
		})
	}
}

func TestInstallCodexTelemetryWithoutSDD(t *testing.T) {
	home := t.TempDir()
	selection := model.Selection{Agents: []model.AgentID{model.AgentCodex}}
	runInstallInjectionSteps(t, newTestInstallRuntime(t, home, selection))
	path := filepath.Join(home, ".codex", "hooks.json")
	first := readTextFile(t, path)
	for _, want := range []string{`"SubagentStop"`, `"Stop"`, `gentle-ai telemetry runtime codex --json`, `gentle-ai skill-registry refresh`} {
		if !strings.Contains(first, want) {
			t.Fatalf("missing %q in %s", want, path)
		}
	}
	runInstallInjectionSteps(t, newTestInstallRuntime(t, home, selection))
	if second := readTextFile(t, path); second != first {
		t.Fatal("second install changed hooks bytes")
	}
}

func TestInstallRoutingGuidanceIsIndependentOfSDDSelection(t *testing.T) {
	for _, tc := range []struct {
		name      string
		selection model.Selection
	}{
		{"no components", model.Selection{Agents: []model.AgentID{model.AgentClaudeCode}}},
		{"persona selected", model.Selection{Agents: []model.AgentID{model.AgentClaudeCode}, Components: []model.ComponentID{model.ComponentPersona}, Persona: model.PersonaGentleman}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			runInstallInjectionSteps(t, newTestInstallRuntime(t, home, tc.selection))
			prompt := readTextFile(t, systemPromptFileFor(t, home, model.AgentClaudeCode))
			if !strings.Contains(prompt, routingOpenMarker) || !strings.Contains(prompt, routingCloseMarker) || !strings.Contains(prompt, "Implementation Routing") {
				t.Fatalf("routing guidance missing from %s:\n%s", tc.name, prompt)
			}
			if strings.Contains(prompt, "<!-- gentle-ai:sdd-orchestrator -->") {
				t.Fatalf("retired SDD orchestration appeared in %s:\n%s", tc.name, prompt)
			}
		})
	}
}

// The managed plugins and routing guidance are independent of retired SDD assets.
func TestInstallRoutingGuidanceSurvivesOpenCodePluginInstall(t *testing.T) {
	home := t.TempDir()
	selection := model.Selection{Agents: []model.AgentID{model.AgentOpenCode}}
	runInstallInjectionSteps(t, newTestInstallRuntime(t, home, selection))
	prompt := openCodeOrchestratorPrompt(t, home)
	if !strings.Contains(prompt, routingOpenMarker) || !strings.Contains(prompt, routingCloseMarker) || !strings.Contains(prompt, "<!-- gentle-ai:remote-authorization -->") {
		t.Fatalf("OpenCode prompt lost routing or remote authorization:\n%s", prompt)
	}
	plugin := filepath.Join(home, ".config", "opencode", "plugins", "opencode-review-transport.ts")
	first := readTextFile(t, plugin)
	if first == "" {
		t.Fatal("empty review transport plugin")
	}
	runInstallInjectionSteps(t, newTestInstallRuntime(t, home, selection))
	if got := openCodeOrchestratorPrompt(t, home); got != prompt {
		t.Fatal("plugin reinstall changed OpenCode routing prompt")
	}
	if got := readTextFile(t, plugin); got != first {
		t.Fatal("plugin reinstall changed review transport bytes")
	}
}

func TestInstallStripsLegacyTriggerRulesSection(t *testing.T) {
	home := t.TempDir()

	promptPath := systemPromptFileFor(t, home, model.AgentClaudeCode)
	if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(promptPath), err)
	}
	seeded := filemerge.InjectMarkdownSection("# My own notes\n", "trigger-rules", "Retired WorkRun ceremony\n")
	if err := os.WriteFile(promptPath, []byte(seeded), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", promptPath, err)
	}

	runInstallInjectionSteps(t, newTestInstallRuntime(t, home, model.Selection{
		Agents: []model.AgentID{model.AgentClaudeCode},
	}))

	prompt := readTextFile(t, promptPath)
	if strings.Contains(prompt, legacyTriggerRulesOpenMarker) {
		t.Fatalf("legacy trigger-rules section survived the install:\n%s", prompt)
	}
	if strings.Contains(prompt, "Retired WorkRun ceremony") {
		t.Fatalf("legacy trigger-rules content survived the install:\n%s", prompt)
	}
	if !strings.Contains(prompt, "# My own notes") {
		t.Fatalf("stripping the legacy section destroyed unmanaged user content:\n%s", prompt)
	}
	if !strings.Contains(prompt, routingOpenMarker) {
		t.Fatalf("routing guidance missing after the legacy strip:\n%s", prompt)
	}
}

func TestInstallRoutingGuidanceSecondRunIsByteIdentical(t *testing.T) {
	home := t.TempDir()
	selection := model.Selection{Agents: []model.AgentID{model.AgentOpenCode, model.AgentClaudeCode}}

	runInstallInjectionSteps(t, newTestInstallRuntime(t, home, selection))
	first := map[string]string{
		"opencode.json": readTextFile(t, openCodeSettingsPath(home)),
		"claude prompt": readTextFile(t, systemPromptFileFor(t, home, model.AgentClaudeCode)),
		"review plugin": readTextFile(t, filepath.Join(home, ".config", "opencode", "plugins", "opencode-review-transport.ts")),
	}

	runInstallInjectionSteps(t, newTestInstallRuntime(t, home, selection))
	second := map[string]string{
		"opencode.json": readTextFile(t, openCodeSettingsPath(home)),
		"claude prompt": readTextFile(t, systemPromptFileFor(t, home, model.AgentClaudeCode)),
		"review plugin": readTextFile(t, filepath.Join(home, ".config", "opencode", "plugins", "opencode-review-transport.ts")),
	}

	for label, before := range first {
		if second[label] != before {
			t.Fatalf("second install rewrote %s; routing delivery is not idempotent", label)
		}
	}
}

// ─── Workspace scope must not strand orchestrator-prompt guidance ──────────
//
// OpenCode and Kilocode only ever load the home-level settings document, so a
// workspace-scoped install that resolves their guidance against the workspace
// root writes a file the agent never reads (issue #1825). Guidance for these
// agents therefore resolves against the home directory in every scope, while
// every other agent keeps its workspace-scoped delivery.

func TestInstallRoutingGuidanceWorkspaceScopeDeliversOpenCodeToHome(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()

	// Seed the home settings with the retired section so the strip is proven to
	// resolve against the same home scope the injector writes.
	seeded := filemerge.InjectMarkdownSection("", "trigger-rules", "Retired WorkRun ceremony\n")
	seedOpenCodeOrchestratorPrompt(t, home, seeded)

	step := agentRoutingGuidanceStep{
		id:           "agent-guidance:" + string(model.AgentOpenCode),
		agent:        model.AgentOpenCode,
		homeDir:      home,
		workspaceDir: workspace,
		scope:        ScopeWorkspace,
	}
	if err := step.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	prompt := openCodeOrchestratorPrompt(t, home)
	if !strings.Contains(prompt, routingOpenMarker) || !strings.Contains(prompt, routingCloseMarker) {
		t.Fatalf("workspace-scoped install left the home OpenCode orchestrator prompt unrouted:\n%s", prompt)
	}
	if strings.Contains(prompt, "Retired WorkRun ceremony") {
		t.Fatalf("legacy trigger-rules content survived the workspace-scoped install:\n%s", prompt)
	}

	stranded := filepath.Join(workspace, ".config", "opencode")
	if _, err := os.Stat(stranded); !os.IsNotExist(err) {
		t.Fatalf("workspace-scoped install created %q, a directory OpenCode never loads (stat err = %v)", stranded, err)
	}

	first := readTextFile(t, openCodeSettingsPath(home))
	if err := step.Run(); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	if second := readTextFile(t, openCodeSettingsPath(home)); second != first {
		t.Fatalf("second workspace-scoped run rewrote the home settings; delivery is not idempotent")
	}
}

// TestAgentRoutingGuidanceStepRetiresPiManagedBlocks covers issue #3508: Pi
// owns APPEND_SYSTEM.md, so routing never injects a new block but does remove
// the paired stale block an older gentle-ai release wrote.
func TestAgentRoutingGuidanceStepRetiresPiManagedBlocks(t *testing.T) {
	home := t.TempDir()
	promptPath := systemPromptFileFor(t, home, model.AgentPi)
	existing := "user text before\n" +
		"\n" +
		"<!-- gentle-ai:agent-routing -->\n" +
		"stale routing body\n" +
		"<!-- /gentle-ai:agent-routing -->\n" +
		"\n" +
		"user text after\n"
	mustWriteFile(t, promptPath, []byte(existing))

	step := agentRoutingGuidanceStep{
		id:      "agent-guidance:" + string(model.AgentPi),
		agent:   model.AgentPi,
		homeDir: home,
		scope:   ScopeGlobal,
	}
	if err := step.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := readTextFile(t, promptPath)
	if strings.Contains(got, "gentle-ai:agent-routing") || !strings.Contains(got, "user text before") || !strings.Contains(got, "user text after") {
		t.Fatalf("Pi routing cleanup = %q, want only user content", got)
	}
}

func TestRoutingGuidancePathsWorkspaceScopeReportOrchestratorPromptAgentsAtHome(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	adapters := resolveAdapters([]model.AgentID{model.AgentOpenCode, model.AgentKilocode, model.AgentClaudeCode})

	paths := routingGuidancePaths(home, workspace, ScopeWorkspace, adapters)

	for _, want := range []string{
		filepath.Join(home, ".config", "opencode", "opencode.json"),
		filepath.Join(home, ".config", "kilo", "opencode.json"),
	} {
		if !containsPath(paths, want) {
			t.Fatalf("routingGuidancePaths(workspace) missing home settings path %q\npaths=%v", want, paths)
		}
	}
	for _, unwanted := range []string{
		filepath.Join(workspace, ".config", "opencode", "opencode.json"),
		filepath.Join(workspace, ".config", "kilo", "opencode.json"),
	} {
		if containsPath(paths, unwanted) {
			t.Fatalf("routingGuidancePaths(workspace) reported %q, a path the agent never loads\npaths=%v", unwanted, paths)
		}
	}

	// Every other agent keeps its workspace-scoped delivery.
	claudePrompt := systemPromptFileFor(t, workspace, model.AgentClaudeCode)
	if !containsPath(paths, claudePrompt) {
		t.Fatalf("routingGuidancePaths(workspace) lost the workspace-scoped path %q for prompt-file agents\npaths=%v", claudePrompt, paths)
	}
}

// TestRoutingGuidancePathsExcludesAgentsWithoutSystemPrompt covers issue
// #4063: routing injection does not own Pi's APPEND_SYSTEM.md; cleanup backup
// coverage is asserted separately.
func TestRoutingGuidancePathsExcludesAgentsWithoutSystemPrompt(t *testing.T) {
	home := t.TempDir()
	adapters := resolveAdapters([]model.AgentID{model.AgentPi, model.AgentClaudeCode})

	paths := routingGuidancePaths(home, "", ScopeGlobal, adapters)

	piPrompt := systemPromptFileFor(t, home, model.AgentPi)
	if containsPath(paths, piPrompt) {
		t.Fatalf("routingGuidancePaths() listed Pi's system prompt %q, a file the step no longer writes\npaths=%v", piPrompt, paths)
	}
	claudePrompt := systemPromptFileFor(t, home, model.AgentClaudeCode)
	if !containsPath(paths, claudePrompt) {
		t.Fatalf("routingGuidancePaths() lost Claude Code's prompt path %q\npaths=%v", claudePrompt, paths)
	}
}

// seedOpenCodeOrchestratorPrompt writes a minimal home settings document whose
// managed orchestrator agent already carries the given prompt.
func seedOpenCodeOrchestratorPrompt(t *testing.T, home, prompt string) {
	t.Helper()

	settingsPath := openCodeSettingsPath(home)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(settingsPath), err)
	}
	payload, err := json.Marshal(map[string]any{
		"agent": map[string]any{
			opencodedefault.ManagedAgent: map[string]any{"prompt": prompt},
		},
	})
	if err != nil {
		t.Fatalf("marshal seeded settings error = %v", err)
	}
	if err := os.WriteFile(settingsPath, payload, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", settingsPath, err)
	}
}

// ─── Routing guidance is part of the rollback contract ─────────────────────
//
// Routing guidance is installed for every configured agent, independently of
// the selected components. A selection whose components happen to cover the
// same file hid the gap; a selection with no components at all exposes it:
// without the routing path in the snapshot, install rewrites a file that was
// never backed up and a rollback cannot restore it.

func TestBackupTargetsIncludeRoutingGuidancePathsWithoutAnyComponent(t *testing.T) {
	home := t.TempDir()
	agent := model.AgentClaudeCode
	selection := model.Selection{Agents: []model.AgentID{agent}}
	resolved := planner.ResolvedPlan{Agents: selection.Agents}

	targets, err := backupTargets(home, "", ScopeGlobal, selection, resolved)
	if err != nil {
		t.Fatalf("backupTargets() error = %v", err)
	}

	routing, err := agentguidance.RoutingPaths(home, agent)
	if err != nil {
		t.Fatalf("RoutingPaths(%q) error = %v", agent, err)
	}
	if len(routing) == 0 {
		t.Fatalf("RoutingPaths(%q) returned no path; the test proves nothing", agent)
	}
	for _, path := range routing {
		if !containsPath(targets, path) {
			t.Fatalf("backupTargets missing routing guidance path %q\ntargets = %v", path, targets)
		}
	}
}

func TestBackupTargetsIncludePiPromptForRoutingCleanup(t *testing.T) {
	home := t.TempDir()
	selection := model.Selection{Agents: []model.AgentID{model.AgentPi}}
	targets, err := backupTargets(home, "", ScopeGlobal, selection, planner.ResolvedPlan{Agents: selection.Agents})
	if err != nil {
		t.Fatal(err)
	}
	if want := systemPromptFileFor(t, home, model.AgentPi); !containsPath(targets, want) {
		t.Fatalf("backup targets = %v, missing Pi cleanup path %q", targets, want)
	}
}

func TestBackupTargetsEngramClaudeIncludeRegistryAndLegacyMigrationSource(t *testing.T) {
	home := t.TempDir()
	selection := model.Selection{Agents: []model.AgentID{model.AgentClaudeCode}, Components: []model.ComponentID{model.ComponentEngram}}
	resolved := planner.ResolvedPlan{Agents: selection.Agents, OrderedComponents: selection.Components}

	targets, err := backupTargets(home, "", ScopeGlobal, selection, resolved)
	if err != nil {
		t.Fatalf("backupTargets() error = %v", err)
	}
	for _, want := range []string{
		filepath.Join(home, ".claude.json"),
		filepath.Join(home, ".claude", "mcp", "engram.json"),
	} {
		if !containsPath(targets, want) {
			t.Fatalf("backupTargets missing Claude Engram path %q; targets=%v", want, targets)
		}
	}
}

func TestBackupTargetsClaudeContext7IncludeCleanupWithoutVerificationRequirement(t *testing.T) {
	for _, tc := range []struct {
		name          string
		scope         InstallScope
		sameWorkspace bool
		wantRoot      string
	}{
		{name: "user scope", scope: ScopeGlobal, wantRoot: "home"},
		{name: "workspace scope", scope: ScopeWorkspace, wantRoot: "workspace"},
		{name: "workspace is home", scope: ScopeWorkspace, sameWorkspace: true, wantRoot: "home"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			workspace := t.TempDir()
			if tc.sameWorkspace {
				workspace = home
			}
			selection := model.Selection{
				Agents:     []model.AgentID{model.AgentClaudeCode},
				Components: []model.ComponentID{model.ComponentContext7},
			}
			resolved := planner.ResolvedPlan{Agents: selection.Agents, OrderedComponents: selection.Components}
			adapters := resolveAdapters(selection.Agents)

			targets, err := backupTargets(home, workspace, tc.scope, selection, resolved)
			if err != nil {
				t.Fatalf("backupTargets() error = %v", err)
			}
			root := home
			if tc.wantRoot == "workspace" {
				root = workspace
			}
			wantSettings := adapters[0].SettingsPath(root)
			if !containsPath(targets, wantSettings) {
				t.Fatalf("backupTargets missing cleanup path %q; targets=%v", wantSettings, targets)
			}

			verificationPaths := componentPathsWithWorkspaceScoped(home, workspace, tc.scope, selection, adapters, model.ComponentContext7)
			if containsPath(verificationPaths, wantSettings) {
				t.Fatalf("component verification must not require best-effort cleanup path %q; paths=%v", wantSettings, verificationPaths)
			}

			otherRoot := workspace
			if root == workspace {
				otherRoot = home
			}
			if !tc.sameWorkspace && containsPath(targets, adapters[0].SettingsPath(otherRoot)) {
				t.Fatalf("backupTargets selected the wrong scope's cleanup path; targets=%v", targets)
			}
		})
	}
}

func TestComponentPathsVisualThemesMatchSelectedAdapter(t *testing.T) {
	home := t.TempDir()
	for _, tt := range []struct {
		agent model.AgentID
		want  []string
	}{
		{model.AgentClaudeCode, []string{filepath.Join(home, ".claude", "themes", "gentleman.json"), filepath.Join(home, ".claude", "themes", "gentleman-cute.json")}},
		{model.AgentOpenCode, []string{filepath.Join(home, ".config", "opencode", "themes", "gentleman.json"), filepath.Join(home, ".config", "opencode", "themes", "gentleman-cute.json")}},
	} {
		paths := componentPaths(home, model.Selection{}, resolveAdapters([]model.AgentID{tt.agent}), model.ComponentClaudeTheme)
		if len(paths) != len(tt.want) {
			t.Fatalf("%q paths = %v, want %v", tt.agent, paths, tt.want)
		}
		for i := range tt.want {
			if paths[i] != tt.want[i] {
				t.Fatalf("%q paths = %v, want %v", tt.agent, paths, tt.want)
			}
		}
	}
}

func TestComponentPathsOpenCodeGentleLogoMatchesSelectedAdapter(t *testing.T) {
	home := t.TempDir()
	for _, tt := range []struct {
		agent model.AgentID
		want  []string
	}{
		{model.AgentClaudeCode, []string{}},
		{model.AgentOpenCode, []string{
			filepath.Join(home, ".config", "opencode", "tui-plugins", "gentle-logo.tsx"),
			filepath.Join(home, ".config", "opencode", "tui.json"),
		}},
	} {
		paths := componentPaths(home, model.Selection{}, resolveAdapters([]model.AgentID{tt.agent}), model.ComponentOpenCodeGentleLogo)
		if len(paths) != len(tt.want) {
			t.Fatalf("%q paths = %v, want %v", tt.agent, paths, tt.want)
		}
		for i := range tt.want {
			if paths[i] != tt.want[i] {
				t.Fatalf("%q paths = %v, want %v", tt.agent, paths, tt.want)
			}
		}
	}
}

func TestBackupTargetsContainNoDuplicatePaths(t *testing.T) {
	home := t.TempDir()
	agentIDs := []model.AgentID{model.AgentClaudeCode, model.AgentOpenCode, model.AgentKimi}
	selection := model.Selection{
		Agents:     agentIDs,
		Components: []model.ComponentID{model.ComponentSDD, model.ComponentEngram, model.ComponentPersona},
		SDDMode:    model.SDDModeSingle,
	}
	resolved := planner.ResolvedPlan{Agents: agentIDs, OrderedComponents: selection.Components}

	targets, err := backupTargets(home, "", ScopeGlobal, selection, resolved)
	if err != nil {
		t.Fatalf("backupTargets() error = %v", err)
	}

	assertNoDuplicatePaths(t, "backupTargets", targets)
}

func assertNoDuplicatePaths(t *testing.T, label string, paths []string) {
	t.Helper()

	seen := map[string]struct{}{}
	for _, path := range paths {
		if _, duplicate := seen[path]; duplicate {
			t.Fatalf("%s returned duplicate path %q\npaths = %v", label, path, paths)
		}
		seen[path] = struct{}{}
	}
}
