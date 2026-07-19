package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestComponentPathsSDDIncludesSystemPromptForAllSupportedAgents(t *testing.T) {
	home := t.TempDir()
	adapters := resolveAdapters([]model.AgentID{
		model.AgentClaudeCode,
		model.AgentOpenCode,
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

func TestComponentPathsSDDIncludesClaudeLazyWorkflow(t *testing.T) {
	home := t.TempDir()
	adapters := resolveAdapters([]model.AgentID{model.AgentClaudeCode})

	paths := componentPaths(home, model.Selection{}, adapters, model.ComponentSDD)

	workflow := filepath.Join(home, ".claude", "skills", "_shared", "sdd-orchestrator-workflow.md")
	if !containsPath(paths, workflow) {
		t.Fatalf("componentPaths(sdd) missing Claude lazy workflow path %q\npaths=%v", workflow, paths)
	}
}

func TestComponentPathsSDDMultiIncludesOpenCodePlugins(t *testing.T) {
	home := t.TempDir()
	adapters := resolveAdapters([]model.AgentID{model.AgentOpenCode})

	paths := componentPaths(home, model.Selection{SDDMode: model.SDDModeMulti}, adapters, model.ComponentSDD)

	for _, plugin := range []string{"background-agents.ts", "model-variants.ts", "review-result-artifacts.ts", "skill-registry.ts"} {
		path := filepath.Join(home, ".config", "opencode", "plugins", plugin)
		if !containsPath(paths, path) {
			t.Fatalf("componentPaths(sdd multi) missing OpenCode plugin path %q\npaths=%v", path, paths)
		}
	}
}

func TestComponentPathsSDDSingleIncludesOpenCodePlugins(t *testing.T) {
	home := t.TempDir()
	adapters := resolveAdapters([]model.AgentID{model.AgentOpenCode})

	paths := componentPaths(home, model.Selection{SDDMode: model.SDDModeSingle}, adapters, model.ComponentSDD)

	for _, plugin := range []string{"background-agents.ts", "model-variants.ts", "review-result-artifacts.ts", "skill-registry.ts"} {
		path := filepath.Join(home, ".config", "opencode", "plugins", plugin)
		if !containsPath(paths, path) {
			t.Fatalf("componentPaths(sdd single) missing OpenCode plugin path %q\npaths=%v", path, paths)
		}
	}
}

func TestComponentPathsWorkspaceScopedOpenCodeSDDUsesWorkspaceManagedPaths(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	adapters := resolveAdapters([]model.AgentID{model.AgentOpenCode})
	selection := model.Selection{SDDMode: model.SDDModeMulti}

	paths := componentPathsWithWorkspaceScoped(home, workspace, ScopeWorkspace, selection, adapters, model.ComponentSDD)

	for _, want := range []string{
		filepath.Join(workspace, ".config", "opencode", "opencode.json"),
		filepath.Join(workspace, ".config", "opencode", "commands", "sdd-init.md"),
		filepath.Join(workspace, ".config", "opencode", "plugins", "background-agents.ts"),
		filepath.Join(workspace, ".config", "opencode", "plugins", "model-variants.ts"),
		filepath.Join(workspace, ".config", "opencode", "plugins", "skill-registry.ts"),
		filepath.Join(workspace, ".config", "opencode", "prompts", "sdd", "sdd-apply.md"),
		filepath.Join(workspace, ".config", "opencode", "skills", "sdd-apply", "SKILL.md"),
	} {
		if !containsPath(paths, want) {
			t.Fatalf("componentPathsWithWorkspaceScoped(sdd,opencode,workspace) missing workspace-scoped path %q\npaths=%v", want, paths)
		}
	}

	for _, unwanted := range []string{
		filepath.Join(home, ".config", "opencode", "opencode.json"),
		filepath.Join(home, ".config", "opencode", "commands", "sdd-init.md"),
		filepath.Join(home, ".config", "opencode", "plugins", "background-agents.ts"),
		filepath.Join(home, ".config", "opencode", "plugins", "model-variants.ts"),
		filepath.Join(home, ".config", "opencode", "plugins", "skill-registry.ts"),
		filepath.Join(home, ".config", "opencode", "prompts", "sdd", "sdd-apply.md"),
		filepath.Join(home, ".config", "opencode", "skills", "sdd-apply", "SKILL.md"),
	} {
		if containsPath(paths, unwanted) {
			t.Fatalf("componentPathsWithWorkspaceScoped(sdd,opencode,workspace) must not include home-scoped path %q\npaths=%v", unwanted, paths)
		}
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
	adapters := resolveAdapters([]model.AgentID{model.AgentGeminiCLI})

	paths := componentPaths(home, model.Selection{}, adapters, model.ComponentSDD)

	// Verify all four shared convention files are reported.
	for _, sharedFile := range []string{
		"persistence-contract.md",
		"engram-convention.md",
		"openspec-convention.md",
		"sdd-phase-common.md",
		"skill-resolver.md",
	} {
		shared := filepath.Join(home, ".gemini", "skills", "_shared", sharedFile)
		if !containsPath(paths, shared) {
			t.Fatalf("componentPaths(sdd) missing shared convention path %q\npaths=%v", shared, paths)
		}
	}

	skill := filepath.Join(home, ".gemini", "skills", "sdd-verify", "SKILL.md")
	if !containsPath(paths, skill) {
		t.Fatalf("componentPaths(sdd) missing SDD skill path %q\npaths=%v", skill, paths)
	}
}

func TestComponentPathsWithWorkspaceOpenClawSDDUsesWorkspaceScopedSkills(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	adapters := resolveAdapters([]model.AgentID{model.AgentOpenClaw})

	paths := componentPathsWithWorkspace(home, workspace, model.Selection{}, adapters, model.ComponentSDD)

	for _, want := range []string{
		filepath.Join(workspace, ".openclaw", "skills", "_shared", "sdd-phase-common.md"),
		filepath.Join(workspace, ".openclaw", "skills", "sdd-init", "SKILL.md"),
		filepath.Join(workspace, ".openclaw", "skills", "sdd-verify", "SKILL.md"),
	} {
		if !containsPath(paths, want) {
			t.Fatalf("componentPathsWithWorkspace(sdd,openclaw) missing workspace-scoped skill path %q\npaths=%v", want, paths)
		}
	}

	for _, unwanted := range []string{
		filepath.Join(home, ".openclaw", "skills", "_shared", "sdd-phase-common.md"),
		filepath.Join(home, ".openclaw", "skills", "sdd-init", "SKILL.md"),
		filepath.Join(home, ".openclaw", "skills", "sdd-verify", "SKILL.md"),
	} {
		if containsPath(paths, unwanted) {
			t.Fatalf("componentPathsWithWorkspace(sdd,openclaw) must not include home-scoped SDD skill path %q\npaths=%v", unwanted, paths)
		}
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

	// OpenClaw always uses workspaceDir when set, independent of scope.
	paths := componentPathsWithWorkspace(home, workspace, selection, adapters, model.ComponentSkills)

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
	adapters := resolveAdapters([]model.AgentID{model.AgentKimi})

	paths := componentPaths(home, model.Selection{}, adapters, model.ComponentSDD)

	for _, want := range []string{
		filepath.Join(home, ".kimi", "KIMI.md"),
		filepath.Join(home, ".kimi", "agents", "gentleman.yaml"),
		filepath.Join(home, ".kimi", "agents", "sdd-init.yaml"),
		filepath.Join(home, ".kimi", "agents", "sdd-propose.md"),
		filepath.Join(home, ".kimi", "agents", "sdd-apply.yaml"),
		filepath.Join(home, ".kimi", "agents", "sdd-verify.md"),
		filepath.Join(home, ".kimi", "agents", "sdd-archive.yaml"),
		filepath.Join(home, ".config", "agents", "skills", "sdd-init", "SKILL.md"),
		filepath.Join(home, ".config", "agents", "skills", "_shared", "engram-convention.md"),
	} {
		if !containsPath(paths, want) {
			t.Fatalf("componentPaths(sdd,kimi) missing %q\npaths=%v", want, paths)
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

func TestComponentPathsContext7ClaudeUsesSettingsFile(t *testing.T) {
	home := t.TempDir()
	adapters := resolveAdapters([]model.AgentID{model.AgentClaudeCode})

	paths := componentPaths(home, model.Selection{}, adapters, model.ComponentContext7)

	want := filepath.Join(home, ".claude", "settings.json")
	if !containsPath(paths, want) {
		t.Fatalf("componentPaths(context7,claude) missing %q\npaths=%v", want, paths)
	}
	legacy := filepath.Join(home, ".claude", "mcp", "context7.json")
	if containsPath(paths, legacy) {
		t.Fatalf("componentPaths(context7,claude) should not verify legacy path %q\npaths=%v", legacy, paths)
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

// TestComponentPathsPermissionsCodexExcludesMissingConfigTOML verifies that
// ComponentPermission + Codex does not report ~/.codex/config.toml when the
// file does not exist: cleanup never creates it, so post-apply verification
// must not require it.
func TestComponentPathsPermissionsCodexExcludesMissingConfigTOML(t *testing.T) {
	home := t.TempDir()
	adapters := resolveAdapters([]model.AgentID{model.AgentCodex})

	paths := componentPaths(home, model.Selection{}, adapters, model.ComponentPermission)

	unwanted := filepath.Join(home, ".codex", "config.toml")
	if containsPath(paths, unwanted) {
		t.Fatalf("componentPaths(permissions,codex) must not include missing %q\npaths=%v", unwanted, paths)
	}
}

// TestComponentPathsPermissionsCodexIncludesConfigTOMLOnUnconfirmedStat pins
// the fail-toward-backup direction: when Stat fails with an error that does
// not confirm absence (here ENOTDIR because ~/.codex is a file), the path must
// still be included so backup coverage is never silently dropped.
func TestComponentPathsPermissionsCodexIncludesConfigTOMLOnUnconfirmedStat(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Stat under a file path does not yield a non-IsNotExist error on Windows")
	}
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".codex"), []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	adapters := resolveAdapters([]model.AgentID{model.AgentCodex})

	paths := componentPaths(home, model.Selection{}, adapters, model.ComponentPermission)

	configPath := filepath.Join(home, ".codex", "config.toml")
	if !containsPath(paths, configPath) {
		t.Fatalf("componentPaths(permissions,codex) missing %q on unconfirmed stat\npaths=%v", configPath, paths)
	}
}

// TestComponentPathsPermissionsCodexIncludesExistingConfigTOML pins backup and
// rollback coverage: Codex permission cleanup mutates an existing
// ~/.codex/config.toml, so a Permission-selected run must snapshot it when it
// exists.
func TestComponentPathsPermissionsCodexIncludesExistingConfigTOML(t *testing.T) {
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

	if !containsPath(paths, configPath) {
		t.Fatalf("componentPaths(permissions,codex) missing existing %q\npaths=%v", configPath, paths)
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
