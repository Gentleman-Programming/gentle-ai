package assets

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestAllEmbeddedAssetsAreReadable verifies that every expected embedded file
// can be loaded via Read() without error. This catches missing/misnamed files
// at test time rather than at runtime.
func TestAllEmbeddedAssetsAreReadable(t *testing.T) {
	expectedFiles := []string{
		// Claude agent files
		"claude/engram-protocol.md",
		"claude/persona-gentleman.md",
		"claude/sdd-orchestrator.md",
		"claude/commands/sdd-apply.md",
		"claude/commands/sdd-archive.md",
		"claude/commands/sdd-continue.md",
		"claude/commands/sdd-explore.md",
		"claude/commands/sdd-ff.md",
		"claude/commands/sdd-init.md",
		"claude/commands/sdd-new.md",
		"claude/commands/sdd-onboard.md",
		"claude/commands/sdd-verify.md",

		// OpenCode agent files
		"opencode/persona-gentleman.md",
		"opencode/sdd-orchestrator.md",
		"opencode/sdd-overlay-single.json",
		"opencode/sdd-overlay-multi.json",
		"opencode/commands/sdd-apply.md",
		"opencode/commands/sdd-archive.md",
		"opencode/commands/sdd-continue.md",
		"opencode/commands/sdd-explore.md",
		"opencode/commands/sdd-ff.md",
		"opencode/commands/sdd-init.md",
		"opencode/commands/sdd-new.md",
		"opencode/commands/sdd-onboard.md",
		"opencode/commands/sdd-verify.md",
		"opencode/plugins/background-agents.ts",

		// Gemini agent files
		"gemini/sdd-orchestrator.md",

		// Codex agent files
		"codex/sdd-orchestrator.md",

		// Cursor agent files
		"cursor/sdd-orchestrator.md",
		"cursor/agents/sdd-init.md",
		"cursor/agents/sdd-explore.md",
		"cursor/agents/sdd-propose.md",
		"cursor/agents/sdd-spec.md",
		"cursor/agents/sdd-design.md",
		"cursor/agents/sdd-tasks.md",
		"cursor/agents/sdd-apply.md",
		"cursor/agents/sdd-verify.md",
		"cursor/agents/sdd-archive.md",

		// Kimi agent files
		"kimi/persona-gentleman.md",
		"kimi/output-style-gentleman.md",
		"kimi/sdd-orchestrator.md",
		"kimi/KIMI.md",
		"kimi/agents/gentleman.yaml",
		"kimi/agents/sdd-init.yaml",
		"kimi/agents/sdd-explore.yaml",
		"kimi/agents/sdd-propose.yaml",
		"kimi/agents/sdd-spec.yaml",
		"kimi/agents/sdd-design.yaml",
		"kimi/agents/sdd-tasks.yaml",
		"kimi/agents/sdd-apply.yaml",
		"kimi/agents/sdd-verify.yaml",
		"kimi/agents/sdd-archive.yaml",
		"kimi/agents/sdd-onboard.yaml",
		"kimi/agents/sdd-init.md",
		"kimi/agents/sdd-explore.md",
		"kimi/agents/sdd-propose.md",
		"kimi/agents/sdd-spec.md",
		"kimi/agents/sdd-design.md",
		"kimi/agents/sdd-tasks.md",
		"kimi/agents/sdd-apply.md",
		"kimi/agents/sdd-verify.md",
		"kimi/agents/sdd-archive.md",
		"kimi/agents/sdd-onboard.md",

		// SDD skills
		"skills/sdd-init/SKILL.md",
		"skills/sdd-init/references/init-details.md",
		"skills/sdd-apply/SKILL.md",
		"skills/sdd-archive/SKILL.md",
		"skills/sdd-design/SKILL.md",
		"skills/sdd-explore/SKILL.md",
		"skills/sdd-propose/SKILL.md",
		"skills/sdd-spec/SKILL.md",
		"skills/sdd-tasks/SKILL.md",
		"skills/sdd-verify/SKILL.md",
		"skills/sdd-verify/references/report-format.md",
		"skills/skill-registry/SKILL.md",
		"skills/judgment-day/references/prompts-and-formats.md",
		"skills/_shared/persistence-contract.md",
		"skills/_shared/engram-convention.md",
		"skills/_shared/openspec-convention.md",
		"skills/_shared/sdd-phase-common.md",

		// Foundation skills
		"skills/go-testing/SKILL.md",
		"skills/go-testing/references/examples.md",
		"skills/skill-creator/SKILL.md",
		"skills/skill-improver/SKILL.md",
		"skills/chained-pr/references/chaining-details.md",
	}

	for _, path := range expectedFiles {
		t.Run(path, func(t *testing.T) {
			content, err := Read(path)
			if err != nil {
				t.Fatalf("Read(%q) error = %v", path, err)
			}

			if len(strings.TrimSpace(content)) == 0 {
				t.Fatalf("Read(%q) returned empty content", path)
			}

			// Real content should be substantial, not a one-line stub.
			if len(content) < 50 {
				t.Fatalf("Read(%q) content is suspiciously short (%d bytes) — possible stub", path, len(content))
			}
		})
	}
}

func TestOpenCodeEmbeddedAssetLayout(t *testing.T) {
	entries, err := FS.ReadDir("opencode")
	if err != nil {
		t.Fatalf("ReadDir(opencode) error = %v", err)
	}

	seen := map[string]bool{}
	for _, entry := range entries {
		seen[entry.Name()] = true
	}

	for _, name := range []string{"commands", "plugins", "persona-gentleman.md", "sdd-orchestrator.md", "sdd-overlay-single.json", "sdd-overlay-multi.json"} {
		if !seen[name] {
			t.Fatalf("opencode embedded assets missing %q", name)
		}
	}

	commandEntries, err := FS.ReadDir("opencode/commands")
	if err != nil {
		t.Fatalf("ReadDir(opencode/commands) error = %v", err)
	}
	if len(commandEntries) != 9 {
		t.Fatalf("opencode commands count = %d, want 9", len(commandEntries))
	}

	pluginEntries, err := FS.ReadDir("opencode/plugins")
	if err != nil {
		t.Fatalf("ReadDir(opencode/plugins) error = %v", err)
	}
	if len(pluginEntries) != 2 {
		t.Fatalf("opencode plugins count = %d, want 2", len(pluginEntries))
	}
	wantPlugins := map[string]bool{"background-agents.ts": true, "model-variants.ts": true}
	for _, entry := range pluginEntries {
		if !wantPlugins[entry.Name()] {
			t.Fatalf("unexpected plugin entry = %q", entry.Name())
		}
	}
}

// TestModelVariantsPluginContract verifies the embedded model-variants.ts
// plugin keeps the contract enforced by PR #440 review: atomic write via
// tmp+rename, always-write semantics (no early return on empty variants),
// and visible error logging instead of silent failure.
func TestModelVariantsPluginContract(t *testing.T) {
	source, err := Read("opencode/plugins/model-variants.ts")
	if err != nil {
		t.Fatalf("Read(model-variants.ts) error = %v", err)
	}
	src := string(source)

	// Atomic write: must import rename and write to a .tmp file before renaming.
	if !strings.Contains(src, "rename") {
		t.Errorf("model-variants.ts must use rename() for atomic write")
	}
	if !strings.Contains(src, ".tmp") {
		t.Errorf("model-variants.ts must write to a .tmp file before rename()")
	}

	// Always-write semantics: the cache must be written unconditionally so an
	// empty variants object overwrites a stale cache from a previous run.
	// Reject any guard on `Object.keys(variants).length` that could short-circuit
	// the write path.
	if strings.Contains(src, "Object.keys(variants).length") {
		t.Errorf("model-variants.ts must not gate the write on variants length (allows stale cache to survive)")
	}
	if !strings.Contains(src, "JSON.stringify(variants") {
		t.Errorf("model-variants.ts must serialize the variants object — even when empty — to overwrite stale cache")
	}

	// Errors must be logged, not swallowed silently.
	if strings.Contains(src, "} catch {") {
		t.Errorf("model-variants.ts must not have a parameterless `catch {}` block (silences ENOSPC/EACCES)")
	}
	if !strings.Contains(src, "console.error") {
		t.Errorf("model-variants.ts must log errors via console.error so users see failures")
	}
}

func TestClaudeEmbeddedAssetLayout(t *testing.T) {
	entries, err := FS.ReadDir("claude")
	if err != nil {
		t.Fatalf("ReadDir(claude) error = %v", err)
	}

	seen := map[string]bool{}
	for _, entry := range entries {
		seen[entry.Name()] = true
	}

	for _, name := range []string{"commands", "engram-protocol.md", "persona-gentleman.md", "sdd-orchestrator.md"} {
		if !seen[name] {
			t.Fatalf("claude embedded assets missing %q", name)
		}
	}

	commandEntries, err := FS.ReadDir("claude/commands")
	if err != nil {
		t.Fatalf("ReadDir(claude/commands) error = %v", err)
	}
	if len(commandEntries) != 9 {
		t.Fatalf("claude commands count = %d, want 9", len(commandEntries))
	}
}

func TestOpenCodeSDDOrchestratorRequiresSessionPreflight(t *testing.T) {
	content := MustRead("opencode/sdd-orchestrator.md")

	for _, required := range []string{
		"### SDD Session Preflight (HARD GATE)",
		"Before executing ANY SDD command or natural-language SDD request",
		"Execution mode",
		"Artifact store",
		"Chained PR strategy",
		"Review budget",
		"`openspec/config.yaml`, existing SDD artifacts, previous `sdd-init` results, or installed SDD assets do NOT satisfy session preflight",
		"ask the localized user-facing preflight prompt above and STOP",
		"Ask the user directly with a compact, numbered preflight prompt",
		"Match the user's current language",
		"Do NOT mix languages inside one preflight prompt",
		"If the current language is Spanish, use the Spanish localized shape below verbatim",
		"translate user-facing prose to the user's current language",
		"¿Querés ajustar algo o continuamos?",
		"B. Artefactos",
		"D. Revisión",
		"la estimación supera el presupuesto",
		"Do NOT mention non-existent tools",
		"A1, B1, C1, D1",
		"### SDD Entry Routing (MANDATORY)",
		"Never launch `sdd-apply` just because the user asked to implement a feature",
		"In **Interactive** mode, between phases",
		"Ask before launching the next phase",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("opencode/sdd-orchestrator.md missing required preflight wording %q", required)
		}
	}
}

func TestOpenCodeSDDOrchestratorSpanishPreflightIsLocalized(t *testing.T) {
	content := MustRead("opencode/sdd-orchestrator.md")
	start := strings.Index(content, "Antes de continuar con SDD")
	if start < 0 {
		t.Fatal("opencode/sdd-orchestrator.md missing Spanish preflight block")
	}
	end := strings.Index(content[start:], "Map answers to canonical values")
	if end < 0 {
		t.Fatal("opencode/sdd-orchestrator.md missing end of Spanish preflight block")
	}
	spanishBlock := content[start : start+end]

	for _, forbidden := range []string{"B. Artifacts", "D. Review", "forecast", "budget"} {
		if strings.Contains(spanishBlock, forbidden) {
			t.Fatalf("Spanish preflight block should localize user-facing prose; found %q", forbidden)
		}
	}
}

func TestOpenCodeSDDFFHonorsInteractiveMode(t *testing.T) {
	content := MustRead("opencode/commands/sdd-ff.md")

	for _, forbidden := range []string{
		"Present a combined summary after ALL phases complete (not between each one).",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("opencode/commands/sdd-ff.md must not contain unqualified back-to-back planning instruction %q", forbidden)
		}
	}

	for _, required := range []string{
		"Honor the cached execution mode from SDD Session Preflight",
		"In `interactive` mode: run only the next planning phase",
		"Do not launch the following phase until the user confirms",
		"In `auto` mode: run all planning phases back-to-back",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("opencode/commands/sdd-ff.md missing interactive/auto guard wording %q", required)
		}
	}
}

func TestOpenCodeSDDCommandsAreOrchestratorGuarded(t *testing.T) {
	entries, err := FS.ReadDir("opencode/commands")
	if err != nil {
		t.Fatalf("ReadDir(opencode/commands) error = %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "sdd-") {
			continue
		}
		path := "opencode/commands/" + entry.Name()
		content := MustRead(path)

		for _, forbidden := range []string{
			"You are an SDD sub-agent",
			"Artifact store mode: engram",
		} {
			if strings.Contains(content, forbidden) {
				t.Fatalf("%s must not bypass orchestration with %q", path, forbidden)
			}
		}

		for _, required := range []string{
			"SDD Session Preflight must already be complete",
			"If missing, ask the exact orchestrator preflight prompt and STOP",
		} {
			if !strings.Contains(content, required) {
				t.Fatalf("%s missing orchestration guard wording %q", path, required)
			}
		}
	}

	applyContent := MustRead("opencode/commands/sdd-apply.md")
	for _, required := range []string{
		"You are the `gentle-orchestrator`, not an SDD executor",
		"If spec, design, or tasks are missing, do NOT implement",
		"do not hardcode Engram",
	} {
		if !strings.Contains(applyContent, required) {
			t.Fatalf("opencode/commands/sdd-apply.md missing apply guard wording %q", required)
		}
	}
}

func TestClaudeSDDOrchestratorChainStrategy(t *testing.T) {
	content := MustRead("claude/sdd-orchestrator.md")

	for _, required := range []string{
		"### Chain Strategy",
		"`stacked-to-main`",
		"`feature-branch-chain`",
		"Pass it as `chain_strategy` to `sdd-tasks` and `sdd-apply` prompts alongside `delivery_strategy`.",
		"When launching `sdd-apply`, always include the resolved `delivery_strategy`, `chain_strategy`, and any chosen PR boundary/exception in the prompt.",
		"Claude Code's native Agent/Task mechanism",
		"results are not persisted by OpenCode's background-agent plugin",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("claude/sdd-orchestrator.md missing required SDD chain/delegation wording %q", required)
		}
	}

	for _, forbidden := range []string{
		"plugin-backed persisted background delegation",
		"background task storage",
		"OpenCode plugin-backed persistence guarantees",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("claude/sdd-orchestrator.md must not imply OpenCode persisted delegation semantics via %q", forbidden)
		}
	}
}

func TestNonClaudeSDDOrchestratorChainStrategyParity(t *testing.T) {
	tests := []struct {
		path             string
		propagationScope string
	}{
		{path: "codex/sdd-orchestrator.md", propagationScope: "prompt"},
		{path: "gemini/sdd-orchestrator.md", propagationScope: "prompt"},
		{path: "qwen/sdd-orchestrator.md", propagationScope: "prompt"},
		{path: "generic/sdd-orchestrator.md", propagationScope: "prompt"},
		{path: "kimi/sdd-orchestrator.md", propagationScope: "Kimi custom-agent prompt"},
		{path: "kiro/sdd-orchestrator.md", propagationScope: "Kiro phase context"},
		{path: "windsurf/sdd-orchestrator.md", propagationScope: "inline phase context"},
		{path: "antigravity/sdd-orchestrator.md", propagationScope: "inline phase context"},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			content := MustRead(tc.path)

			for _, required := range []string{
				"### Chain Strategy",
				"`stacked-to-main`",
				"`feature-branch-chain`",
				"delivery_strategy",
				"chain_strategy",
				"sdd-tasks",
				"sdd-apply",
				tc.propagationScope,
			} {
				if !strings.Contains(content, required) {
					t.Fatalf("%s missing required chain strategy wording %q", tc.path, required)
				}
			}
		})
	}
}

func TestPlatformNativeSDDOrchestratorsAvoidOpenCodePersistenceClaims(t *testing.T) {
	tests := []struct {
		path     string
		required []string
	}{
		{path: "kimi/sdd-orchestrator.md", required: []string{"/skill:sdd-*", "multiagent:Task", "custom-agent prompt"}},
		{path: "kiro/sdd-orchestrator.md", required: []string{"Kiro phase context", "native Kiro subagent context", "approval"}},
		{path: "windsurf/sdd-orchestrator.md", required: []string{"solo-agent", "inline phase context", "There are no sub-agents"}},
		{path: "antigravity/sdd-orchestrator.md", required: []string{"inline phase context", "Phase Execution Protocol", "directly"}},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			content := MustRead(tc.path)

			for _, required := range tc.required {
				if !strings.Contains(content, required) {
					t.Fatalf("%s missing platform-native wording %q", tc.path, required)
				}
			}

			for _, forbidden := range []string{
				"OpenCode's background-agent plugin",
				"OpenCode plugin-backed persistence",
				"plugin-backed persisted background delegation",
				"background task storage",
				"delegate to `sdd-init` sub-agent",
			} {
				if strings.Contains(content, forbidden) {
					t.Fatalf("%s must not imply inaccurate OpenCode/subagent semantics via %q", tc.path, forbidden)
				}
			}
		})
	}
}

func TestGentlemanLanguageInstructionsDoNotBiasEnglishSessions(t *testing.T) {
	personaPaths := []string{
		"claude/persona-gentleman.md",
		"generic/persona-gentleman.md",
		"kiro/persona-gentleman.md",
		"kimi/persona-gentleman.md",
		"opencode/persona-gentleman.md",
	}

	for _, path := range personaPaths {
		t.Run(path, func(t *testing.T) {
			content := MustRead(path)

			for _, banned := range []string{
				`Say "déjame verificar"`,
				`Spanish input → Rioplatense Spanish (voseo):`,
				`English input → same warm energy:`,
			} {
				if strings.Contains(content, banned) {
					t.Fatalf("%s still contains language-biasing phrase %q", path, banned)
				}
			}

			for _, required := range []string{
				"Match the user's current language in your REPLY ONLY",
				"Do not switch languages unless the user does, asks you to, or you are quoting/translating content.",
				"When replying to the user in English, keep the full reply in natural English with the same warm energy.",
			} {
				if !strings.Contains(content, required) {
					t.Fatalf("%s missing language guardrail %q", path, required)
				}
			}
		})
	}

	for _, path := range []string{
		"claude/output-style-gentleman.md",
		"kimi/output-style-gentleman.md",
	} {
		t.Run(path, func(t *testing.T) {
			content := MustRead(path)

			for _, banned := range []string{
				"### Spanish Input → Rioplatense Spanish (voseo)",
				`Use naturally: "Bien"`,
				`Use naturally: "Here's the thing"`,
			} {
				if strings.Contains(content, banned) {
					t.Fatalf("%s still contains drift-prone style example %q", path, banned)
				}
			}

			for _, required := range []string{
				"Always match the user's current language",
				"Do not drift into another language because of persona wording, examples, or stylistic momentum.",
				"keep the full response in English unless the user explicitly asks for another language or you are translating/quoting",
			} {
				if !strings.Contains(content, required) {
					t.Fatalf("%s missing output-style guardrail %q", path, required)
				}
			}
		})
	}

	// engram-protocol assets must not ship Spanish trigger examples that bias
	// English sessions into Spanish replies (same mechanism as #341 / #350).
	// Covers all agent families that ship a dedicated engram instruction asset.
	for _, path := range []string{
		"claude/engram-protocol.md",
		"codex/engram-instructions.md",
	} {
		t.Run(path, func(t *testing.T) {
			content := MustRead(path)

			for _, banned := range []string{
				`"recordar"`,
				`"listo"`,
				`"acordate"`,
				`"qué hicimos"`,
			} {
				if strings.Contains(content, banned) {
					t.Fatalf("%s still contains Spanish trigger phrase %q that biases English sessions", path, banned)
				}
			}
		})
	}
}

// TestPersonasContainContextualSkillLoadingDirective verifies that every
// persona asset injected into a host's system prompt carries the mandatory
// "Contextual Skill Loading" directive (design Decisions 1 and 2 of the
// contextual-skill-loading change). The hardcoded "Skills (Auto-load based
// on context)" table MUST be removed at the same time.
//
// Claude variant references the native `Skill` tool by name. Non-Claude
// variants instruct the model to read the matching SKILL.md using their
// agent's read mechanism, since they have no Skill tool.
func TestPersonasContainContextualSkillLoadingDirective(t *testing.T) {
	tests := []struct {
		path      string
		isClaude  bool
		invokeMsg string // wording specific to the agent family
	}{
		{path: "claude/persona-gentleman.md", isClaude: true, invokeMsg: "invoke it via the built-in `Skill` tool"},
		{path: "opencode/persona-gentleman.md", isClaude: false, invokeMsg: "read the matching SKILL.md"},
		{path: "generic/persona-gentleman.md", isClaude: false, invokeMsg: "read the matching SKILL.md"},
		{path: "generic/persona-neutral.md", isClaude: false, invokeMsg: "read the matching SKILL.md"},
		{path: "kiro/persona-gentleman.md", isClaude: false, invokeMsg: "read the matching SKILL.md"},
		{path: "kimi/persona-gentleman.md", isClaude: false, invokeMsg: "read the matching SKILL.md"},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			content := MustRead(tc.path)

			// The competing hardcoded table MUST be gone.
			if strings.Contains(content, "## Skills (Auto-load based on context)") {
				t.Errorf("%s still contains the hardcoded `## Skills (Auto-load based on context)` table — must be replaced by the contextual directive", tc.path)
			}
			if strings.Contains(content, "| Context | Read this file |") {
				t.Errorf("%s still contains the hardcoded skill trigger table header — must be replaced by the contextual directive", tc.path)
			}

			// The new directive MUST be present.
			for _, required := range []string{
				"## Contextual Skill Loading (MANDATORY)",
				"<available_skills>",
				"Self-check BEFORE every response",
				"blocking requirement",
			} {
				if !strings.Contains(content, required) {
					t.Errorf("%s missing required directive substring %q", tc.path, required)
				}
			}

			// Claude variant references the Skill tool; non-Claude variants
			// instruct the model to read SKILL.md directly.
			if !strings.Contains(content, tc.invokeMsg) {
				t.Errorf("%s missing agent-specific invocation phrasing %q", tc.path, tc.invokeMsg)
			}
			if tc.isClaude {
				if !strings.Contains(content, "`Skill` tool") {
					t.Errorf("claude variant must name the `Skill` tool: %s", tc.path)
				}
			} else {
				// Non-Claude personas must NOT reference the Skill tool — that
				// would mislead users on agents that lack it.
				if strings.Contains(content, "`Skill` tool") {
					t.Errorf("non-Claude variant must not reference the `Skill` tool: %s", tc.path)
				}
			}
		})
	}
}

// TestMustReadPanicsOnMissingFile verifies that MustRead panics for a
// nonexistent file, confirming the safety mechanism works.
func TestMustReadPanicsOnMissingFile(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("MustRead() did not panic for missing file")
		}
	}()

	MustRead("nonexistent/file.md")
}

// TestEmbeddedAssetCount verifies we have the expected number of embedded files.
// This catches accidental deletions of asset files.
func TestEmbeddedAssetCount(t *testing.T) {
	// Count skill files.
	entries, err := FS.ReadDir("skills")
	if err != nil {
		t.Fatalf("ReadDir(skills) error = %v", err)
	}

	skillDirs := 0
	for _, entry := range entries {
		if entry.IsDir() {
			skillDirs++
		}
	}

	// We expect 22 skill directories (10 SDD + judgment-day + 6 foundation + 4 sustainable-review + _shared).
	if skillDirs != 22 {
		t.Fatalf("expected 22 skill directories, got %d", skillDirs)
	}

	// Verify each skill directory has a SKILL.md.
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if entry.Name() == "_shared" {
			for _, sharedFile := range []string{"persistence-contract.md", "engram-convention.md", "openspec-convention.md", "sdd-phase-common.md", "skill-resolver.md"} {
				sharedPath := "skills/_shared/" + sharedFile
				if _, err := Read(sharedPath); err != nil {
					t.Fatalf("shared directory missing %q: %v", sharedFile, err)
				}
			}
			continue
		}
		skillPath := "skills/" + entry.Name() + "/SKILL.md"
		if _, err := Read(skillPath); err != nil {
			t.Fatalf("skill directory %q missing SKILL.md: %v", entry.Name(), err)
		}
	}
}

func TestSDDPhaseCommonEnforcesExecutorBoundary(t *testing.T) {
	content := MustRead("skills/_shared/sdd-phase-common.md")

	// Must enforce executor boundary — no delegation allowed.
	for _, want := range []string{
		"EXECUTOR, not an orchestrator",
		"Do NOT launch sub-agents",
		"do NOT call `delegate`/`task`",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("sdd-phase-common missing executor boundary rule %q", want)
		}
	}

	// Must instruct phase agents to search the skill registry themselves
	// when no explicit skill path was provided — this is skill LOADING, not delegation.
	if !strings.Contains(content, `mem_search(query: "skill-registry"`) {
		t.Fatal("sdd-phase-common must instruct phase agents to search skill-registry themselves for skill loading")
	}

	// Must NOT tell agents to launch sub-agents or delegate tasks.
	for _, forbidden := range []string{
		"launch a sub-agent",
		"delegate this to",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("sdd-phase-common should not contain delegation instruction %q", forbidden)
		}
	}
}

func TestOpenCodeSDDOverlaySubagentsAreExplicitExecutors(t *testing.T) {
	for _, assetPath := range []string{"opencode/sdd-overlay-single.json", "opencode/sdd-overlay-multi.json"} {
		t.Run(assetPath, func(t *testing.T) {
			var root map[string]any
			if err := json.Unmarshal([]byte(MustRead(assetPath)), &root); err != nil {
				t.Fatalf("Unmarshal(%q) error = %v", assetPath, err)
			}

			agents, ok := root["agent"].(map[string]any)
			if !ok {
				t.Fatalf("%q missing agent map", assetPath)
			}

			// multi overlay uses __PROMPT_FILE_{phase}__ placeholders that are
			// replaced at runtime with absolute {file:...} references by
			// inlineOpenCodeSDDPrompts. Verify the placeholder format.
			// single overlay still uses inline prompt strings.
			isMulti := assetPath == "opencode/sdd-overlay-multi.json"

			for _, phase := range []string{"sdd-init", "sdd-explore", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive"} {
				agentDef, ok := agents[phase].(map[string]any)
				if !ok {
					t.Fatalf("%q missing %s agent", assetPath, phase)
				}
				prompt, _ := agentDef["prompt"].(string)
				if isMulti {
					// Multi overlay uses placeholders — verify the placeholder exists.
					expectedPlaceholder := "__PROMPT_FILE_" + phase + "__"
					if prompt != expectedPlaceholder {
						t.Fatalf("%q phase %s prompt = %q, want placeholder %q", assetPath, phase, prompt, expectedPlaceholder)
					}
				} else {
					// Single overlay has inline executor-scoped prompts.
					for _, want := range []string{"not the orchestrator", "Do NOT delegate", "Do NOT call task/delegate", "Do NOT launch sub-agents"} {
						if !strings.Contains(prompt, want) {
							t.Fatalf("%q phase %s prompt missing %q", assetPath, phase, want)
						}
					}
				}
			}
		})
	}
}

// TestCommandsDoNotUseEchoNPwd guards against the nested-subshell pattern
// `echo -n "$(pwd)"` (and the basename variant) that causes Claude Code v2.1.113+
// to reject slash commands with "Unhandled node type: string". Use the plain pwd
// or basename command forms instead — both are accepted by old and new parsers.
func TestCommandsDoNotUseEchoNPwd(t *testing.T) {
	forbidden := `echo -n "$(pwd)"`

	for _, dir := range []string{"claude/commands", "opencode/commands"} {
		entries, err := FS.ReadDir(dir)
		if err != nil {
			t.Fatalf("ReadDir(%s) error = %v", dir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			path := dir + "/" + entry.Name()
			content := MustRead(path)
			if strings.Contains(content, forbidden) {
				t.Errorf("%s contains banned pattern %q — use !`pwd` or !`basename \"$(pwd)\"` instead", path, forbidden)
			}
		}
	}
}

func TestSDDOrchestratorAssetsScopedToDedicatedAgent(t *testing.T) {
	for _, assetPath := range []string{
		"generic/sdd-orchestrator.md",
		"claude/sdd-orchestrator.md",
		"opencode/sdd-orchestrator.md",
		"gemini/sdd-orchestrator.md",
		"codex/sdd-orchestrator.md",
		"cursor/sdd-orchestrator.md",
		"kimi/sdd-orchestrator.md",
	} {
		t.Run(assetPath, func(t *testing.T) {
			content := MustRead(assetPath)
			dedicatedAgent := "sdd-orchestrator"
			if assetPath == "opencode/sdd-orchestrator.md" {
				dedicatedAgent = "gentle-orchestrator"
			}
			if assetPath == "claude/sdd-orchestrator.md" {
				if !strings.Contains(content, "Claude Code orchestrator rule") {
					t.Fatalf("%q missing Claude rule scoping note", assetPath)
				}
			} else if !strings.Contains(content, "dedicated `"+dedicatedAgent+"`") {
				t.Fatalf("%q missing dedicated-agent scoping note", assetPath)
			}
			if !strings.Contains(content, "Do NOT apply it to executor phase agents") {
				t.Fatalf("%q missing executor exclusion note", assetPath)
			}
		})
	}
}
