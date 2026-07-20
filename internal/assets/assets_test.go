package assets

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

func TestOrchestratorsRequireNonSkippableGeneralDelegationTriggers(t *testing.T) {
	paths := []string{
		"claude/sdd-orchestrator.md",
		"opencode/sdd-orchestrator.md",
		"codex/sdd-orchestrator.md",
	}
	requiredControls := []string{
		"Mandatory Delegation Triggers",
		"non-skippable hard gates",
		"fully mandatory",
		"4-file rule",
		"Multi-file write rule",
		"Lifecycle receipt rule",
		"Incident rule",
		"Long-session rule",
		"Fresh review rule",
		"Semantic guard",
		"execution, not delegation",
		"not a substitute for delegation",
	}
	for _, path := range paths {
		content := MustRead(path)
		for _, want := range requiredControls {
			if !strings.Contains(content, want) {
				t.Fatalf("%s missing non-skippable delegation guard %q", path, want)
			}
		}

		triggerSection := firstMarkdownSection(content,
			"### Mandatory Delegation Triggers",
			"#### Mandatory Delegation Triggers",
		)
		if triggerSection == "" {
			t.Fatalf("%s missing Mandatory Delegation Triggers section", path)
		}

		lifecycleLine := markdownLineContaining(triggerSection, "**Lifecycle receipt rule**")
		if !lineContainsAll(
			"before commit",
			"stage every reviewed path",
			"without changing content or mode",
			"gentle-ai review validate --gate pre-commit --cwd <repo>",
			"--lineage <known-lineage>",
			"before push, PR, or release",
			"content-bound receipt",
			"gentle-ai review validate --gate <gate> --cwd <repo>",
			"same exact `--lineage`",
			"Never fall back to inventory discovery",
			"launch a lens",
			"Judgment Day",
			"new budget at a repeated gate",
		)(lifecycleLine) {
			t.Fatalf("%s lifecycle gate must validate the existing receipt without launching a lens, Judgment Day, or a new budget: %q", path, lifecycleLine)
		}

		incidentLine := markdownLineContaining(triggerSection, "**Incident rule**")
		if !lineContainsAll(
			"remain immutable",
			"validate the existing receipt",
			"explicit scope action",
			"not reopened review",
		)(incidentLine) {
			t.Fatalf("%s incident gate must prove immutable targets and validate the existing receipt: %q", path, incidentLine)
		}

		freshReviewLine := markdownLineContaining(triggerSection, "**Fresh review rule**")
		if !lineContainsAll(
			"fresh adversarial lenses",
			"only inside one explicit",
			"`review/start(target)`",
		)(freshReviewLine) {
			t.Fatalf("%s fresh review rule must bind adversarial review to one explicit review/start target: %q", path, freshReviewLine)
		}
	}
}

func TestOrchestratorLifecycleGatesRetainKnownLineage(t *testing.T) {
	for _, agent := range []string{"antigravity", "claude", "codex", "cursor", "gemini", "generic", "hermes", "kimi", "kiro", "opencode", "qwen", "windsurf"} {
		content := MustRead(agent + "/sdd-orchestrator.md")
		if !strings.Contains(content, "--lineage <known-lineage>") || strings.Contains(content, "Let the facade discover authority") {
			t.Errorf("%s orchestrator does not retain exact lineage", agent)
		}
	}
}

func TestOrchestratorsRejectDelegationBypassLanguage(t *testing.T) {
	contents := map[string]string{
		"claude/sdd-orchestrator.md":   MustRead("claude/sdd-orchestrator.md"),
		"opencode/sdd-orchestrator.md": MustRead("opencode/sdd-orchestrator.md"),
		"codex/sdd-orchestrator.md":    MustRead("codex/sdd-orchestrator.md"),
	}
	for path, content := range contents {
		for _, forbidden := range []string{
			"MUST delegate, complete the required fresh review/audit",
			"why delegation would be unsafe or wasteful",
			"delegate one writer or continue inline only if",
			"pause and delegate instead of silently continuing monolithically",
			"delegate a writer, or require a fresh review",
		} {
			if strings.Contains(content, forbidden) {
				t.Fatalf("%s contains delegation bypass wording %q", path, forbidden)
			}
		}

		contentWords := normalizedWords(content)
		for _, forbidden := range []string{
			"delegate a writer or require a fresh review",
		} {
			if strings.Contains(contentWords, normalizedWords(forbidden)) {
				t.Fatalf("%s contains equivalent delegation bypass wording %q", path, forbidden)
			}
		}
	}

	codex := contents["codex/sdd-orchestrator.md"]
	for _, forbidden := range []string{
		"## Solo Path (default)",
		"Run each SDD phase inline, in dependency order, without spawning sub-agents",
		"fall back to the **Solo path**",
		"complete it inline",
	} {
		if strings.Contains(codex, forbidden) {
			t.Fatalf("codex/sdd-orchestrator.md contains solo-path bypass wording %q", forbidden)
		}
	}
	for _, want := range []string{
		"## Delegated Path (default",
		"### Blocking Delegation Contract",
		"Codex sub-agents MUST be treated as waited handoffs, not fire-and-forget background jobs.",
		"You MAY launch more than one independent sub-agent when useful",
		"`wait_agent` for every spawned agent in that batch",
		"Parallel does not mean background",
		"## Graceful Degradation Path (tooling unavailable only)",
		"do not run the full phase pipeline inline as a normal fallback",
	} {
		if !strings.Contains(codex, want) {
			t.Fatalf("codex/sdd-orchestrator.md missing guarded degradation wording %q", want)
		}
	}
	for _, forbidden := range []string{
		"both `spawn_agent` calls before either `wait_agent`",
	} {
		if strings.Contains(codex, forbidden) {
			t.Fatalf("codex/sdd-orchestrator.md contains fire-and-forget delegation wording %q", forbidden)
		}
	}
}

func normalizedWords(s string) string {
	var b strings.Builder
	lastWasSpace := true
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastWasSpace = false
			continue
		}

		if !lastWasSpace {
			b.WriteByte(' ')
			lastWasSpace = true
		}
	}

	return strings.TrimSpace(b.String())
}

// TestAllEmbeddedAssetsAreReadable verifies that every expected embedded file
// can be loaded via Read() without error. This catches missing/misnamed files
// at test time rather than at runtime.
func TestAllEmbeddedAssetsAreReadable(t *testing.T) {
	expectedFiles := []string{
		// Canonical Engram protocol asset (full/slim/passive-capture/compact
		// marker sections — see design.md Decision 3).
		"engram/protocol.md",

		// Claude agent files
		"claude/output-style-neutral.md",
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
		"claude/commands/sdd-status.md",
		"claude/commands/sdd-verify.md",
		"claude/agents/sdd-init.md",
		"claude/agents/sdd-onboard.md",
		"claude/agents/review-risk.md",
		"claude/agents/review-readability.md",
		"claude/agents/review-reliability.md",
		"claude/agents/review-resilience.md",
		"claude/agents/review-refuter.md",

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
		"opencode/commands/sdd-status.md",
		"opencode/commands/sdd-verify.md",

		// Gemini agent files
		"gemini/sdd-orchestrator.md",

		// Antigravity agent files
		"antigravity/sdd-orchestrator.md",

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
		"cursor/agents/review-risk.md",
		"cursor/agents/review-readability.md",
		"cursor/agents/review-reliability.md",
		"cursor/agents/review-resilience.md",
		"cursor/agents/review-refuter.md",

		// Kiro agent files
		"kiro/agents/review-risk.md",
		"kiro/agents/review-readability.md",
		"kiro/agents/review-reliability.md",
		"kiro/agents/review-resilience.md",
		"kiro/agents/review-refuter.md",

		// Kimi agent files
		"kimi/persona-gentleman.md",
		"kimi/output-style-gentleman.md",
		"kimi/output-style-neutral.md",
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
		"kimi/agents/review-risk.yaml",
		"kimi/agents/review-readability.yaml",
		"kimi/agents/review-reliability.yaml",
		"kimi/agents/review-resilience.yaml",
		"kimi/agents/review-refuter.yaml",
		"kimi/agents/review-risk.md",
		"kimi/agents/review-readability.md",
		"kimi/agents/review-reliability.md",
		"kimi/agents/review-resilience.md",
		"kimi/agents/review-refuter.md",

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
		"skills/_shared/sdd-status-contract.md",

		// Hermes agent files
		"hermes/sdd-orchestrator.md",
		"hermes/persona-gentleman.md",
		"hermes/persona-neutral.md",

		// Foundation skills
		"skills/go-testing/SKILL.md",
		"skills/go-testing/references/examples.md",
		"skills/skill-creator/SKILL.md",
		"skills/skill-creator/references/skill-style-guide.md",
		"skills/skill-improver/SKILL.md",
		"skills/skill-improver/references/skill-style-guide.md",
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

func TestSDDVerifyAuthorityPreflightDenialEnvelopeContract(t *testing.T) {
	const denialFields = `authority_only_failure: true
missing_review_authority: true
substantive_failure: false
command_failed: false
observed_authority_revision: sha256:{observed-authority-revision}`

	for _, path := range []string{
		"skills/sdd-verify/SKILL.md",
		"skills/sdd-verify/references/report-format.md",
	} {
		content := MustRead(path)
		for _, want := range []string{
			denialFields,
			"test_exit_code: 125",
			"build_exit_code: 125",
			"must not be executed",
		} {
			if !strings.Contains(content, want) {
				t.Fatalf("%s missing authority-preflight denial contract %q", path, want)
			}
		}
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
	if len(commandEntries) != 12 {
		t.Fatalf("opencode commands count = %d, want 12", len(commandEntries))
	}
	wantCommands := map[string]bool{"skill-creator.md": true, "skill-registry.md": true}
	for _, entry := range commandEntries {
		delete(wantCommands, entry.Name())
	}
	for name := range wantCommands {
		t.Fatalf("opencode embedded commands missing %q", name)
	}

	pluginEntries, err := FS.ReadDir("opencode/plugins")
	if err != nil {
		t.Fatalf("ReadDir(opencode/plugins) error = %v", err)
	}
	if len(pluginEntries) != 3 {
		t.Fatalf("opencode plugins count = %d, want 3", len(pluginEntries))
	}
	wantPlugins := map[string]bool{"model-variants.ts": true, "review-result-artifacts.ts": true, "skill-registry.ts": true}
	for _, entry := range pluginEntries {
		if !wantPlugins[entry.Name()] {
			t.Fatalf("unexpected plugin entry = %q", entry.Name())
		}
	}
}

func TestReviewResultArtifactsPluginContract(t *testing.T) {
	source, err := Read("opencode/plugins/review-result-artifacts.ts")
	if err != nil {
		t.Fatalf("Read(review-result-artifacts.ts) error = %v", err)
	}
	for _, want := range []string{
		`spawn("gentle-ai"`,
		`"review", "capture-result"`,
		`"review", "preserve-result"`,
		`"--lineage", binding.lineage`,
		`"--target", binding.target`,
		`"--lens", binding.lens`,
		`"--order", String(binding.order)`,
		`"--input", "-"`,
		`"--preflight"`,
		`GENTLE_AI_REVIEW_CWD`,
		`"tool.execute.before"`,
		`output.args.background === true`,
		`await preflightCapture(captureCwd(worktree, directory), parseBinding(output.args.prompt, output.args.subagent_type))`,
		`!BINDING.test(input.args.prompt)`,
		`const lens = input.args.subagent_type`,
		`const binding = parseBinding(input.args.prompt, lens)`,
		`const cwd = captureCwd(worktree, directory)`,
		// The replayable payload is extracted exactly once before capture, so a
		// capture failure preserves the extracted strict JSON, never the task
		// envelope that `review capture-result --input` would reject on replay.
		`result = reviewerResult(output.output)`,
		`output.output = await captureResult(cwd, binding, result)`,
		`throw await preservedCaptureFailure(cwd, binding, result, cause)`,
		// Envelope extraction itself can fail; only then is the raw envelope
		// preserved, under a distinct extraction-failure cause.
		`throw await preservedCaptureFailure(cwd, binding, output.output, cause)`,
		`raw reviewer result preserved for recovery`,
		`raw reviewer result could not be preserved`,
		// Double failure (capture and preserve both failed) must embed the
		// bounded raw payload in the thrown error so the transcript retains it.
		`raw reviewer result follows for manual recovery`,
		`PRESERVE_EMBED_LIMIT`,
		// An older installed gentle-ai without --preflight must degrade
		// gracefully instead of hard-blocking every bound lens launch.
		`flag provided but not defined`,
		`export default ReviewResultArtifactsPlugin`,
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("review-result-artifacts.ts missing %q", want)
		}
	}
	if strings.Contains(source, `.slice("review-".length)`) {
		t.Fatal("review-result-artifacts.ts must preserve the exact full selected lens; found review- prefix stripping")
	}
	for _, forbidden := range []string{"writeFile", "link(", "chmod(", "createHash", "export {", "export const"} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("review-result-artifacts.ts must delegate native persistence; found %q", forbidden)
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

	// Per-invocation tmp path: OpenCode loads the plugin twice within the
	// same process when started with `--port`. Both loads share the same
	// PID, so a fixed `.tmp` name races with itself and the second rename()
	// fails with ENOENT. The tmp name must include a per-invocation random
	// suffix (randomBytes) to be unique across both loads, and it must be
	// constructed from cacheDir plus the cache basename so this invocation can
	// track and clean only its own temp file if the write path fails.
	for _, want := range []string{
		`const MODEL_VARIANTS_CACHE_FILE = "model-variants.json"`,
		"const finalPath = path.join(cacheDir, MODEL_VARIANTS_CACHE_FILE)",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("model-variants.ts missing constant-based cache path contract %q", want)
		}
	}
	tmpPathPattern := regexp.MustCompile("tmpPath\\s*=\\s*path\\.join\\(\\s*cacheDir\\s*,\\s*`\\$\\{\\s*MODEL_VARIANTS_CACHE_FILE\\s*\\}\\.\\$\\{\\s*randomBytes\\([^)]*\\)\\s*\\.\\s*toString\\(\\s*[\"']hex[\"']\\s*\\)\\s*\\}\\.tmp`\\s*\\)")
	if !tmpPathPattern.MatchString(src) {
		t.Errorf("model-variants.ts tmp path must use path.join(cacheDir, randomized basename) to be unique across plugin double-loads within the same process")
	}

	// Own-temp cleanup: this randomized temp path has not shipped yet, so there
	// are no previous randomized orphan files to scan at startup. The plugin
	// should only best-effort remove the temp file created by this invocation
	// when it still exists after failure; after rename, the temp file is consumed.
	for _, want := range []string{
		"finally",
		"removeOwnTempFile(tmpPath)",
		"await rm(tmpPath, { force: true })",
		"tmpPath = undefined",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("model-variants.ts missing own-temp cleanup contract %q", want)
		}
	}
	for _, forbidden := range []string{
		"removeStaleModelVariantsTempFiles",
		"STALE_TEMP_FILE_AGE_MS",
		"mtimeMs",
		"Date.now()",
	} {
		if strings.Contains(src, forbidden) {
			t.Errorf("model-variants.ts must not use stale temp cleanup by age; found %q", forbidden)
		}
	}
	if strings.Contains(src, "setTimeout") {
		t.Errorf("model-variants.ts must not use setTimeout for temp cleanup")
	}
}

func TestSkillRegistryPluginContract(t *testing.T) {
	source, err := Read("opencode/plugins/skill-registry.ts")
	if err != nil {
		t.Fatalf("Read(skill-registry.ts) error = %v", err)
	}
	src := string(source)

	for _, want := range []string{
		"execFile",
		"skill-registry",
		"refresh",
		"--quiet",
		"--no-gitignore",
		"--cwd",
		"input.directory",
		"input.worktree",
		"timeout: 30_000",
		"console.error",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("skill-registry.ts missing %q", want)
		}
	}
	if strings.Contains(src, "exec(") {
		t.Fatal("skill-registry.ts must use execFile, not shell exec")
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

	for _, name := range []string{"agents", "commands", "persona-gentleman.md", "sdd-orchestrator.md"} {
		if !seen[name] {
			t.Fatalf("claude embedded assets missing %q", name)
		}
	}
	// engram-protocol.md moved to the canonical engram/protocol.md asset
	// (design.md Decision 3) — it MUST NOT ship a stale duplicate under claude/.
	if seen["engram-protocol.md"] {
		t.Fatal("claude embedded assets must not ship a stale engram-protocol.md — content now lives in engram/protocol.md")
	}

	commandEntries, err := FS.ReadDir("claude/commands")
	if err != nil {
		t.Fatalf("ReadDir(claude/commands) error = %v", err)
	}
	if len(commandEntries) != 10 {
		t.Fatalf("claude commands count = %d, want 10", len(commandEntries))
	}

	agentEntries, err := FS.ReadDir("claude/agents")
	if err != nil {
		t.Fatalf("ReadDir(claude/agents) error = %v", err)
	}
	if len(agentEntries) != 18 {
		t.Fatalf("claude agents count = %d, want 18", len(agentEntries))
	}
}

// TestEngramEmbeddedAssetLayout verifies the canonical protocol asset
// directory introduced by the consolidation (design.md Decision 3).
func TestEngramEmbeddedAssetLayout(t *testing.T) {
	entries, err := FS.ReadDir("engram")
	if err != nil {
		t.Fatalf("ReadDir(engram) error = %v", err)
	}

	seen := map[string]bool{}
	for _, entry := range entries {
		seen[entry.Name()] = true
	}

	if !seen["protocol.md"] {
		t.Fatal("engram embedded assets missing \"protocol.md\"")
	}
}

func TestFourRReviewAgentAssets(t *testing.T) {
	reviewAgents := []string{"review-risk", "review-readability", "review-reliability", "review-resilience"}
	nativeDirs := []string{"claude/agents", "cursor/agents", "kiro/agents"}
	agentRules := map[string][]string{
		"review-risk": {
			"Rule sources: ai-course-2 slides",
			"Flag when secrets, tokens, API keys, JWT secrets, or DB URLs are hardcoded",
			"Block when authz is enforced only in the frontend",
			"Do not flag when React default escaping is used",
		},
		"review-readability": {
			"Rule sources: ai-course-2 slides",
			"Flag magic numbers that should be named constants",
			"Flag long parameter lists that should be parameter objects",
			"Do not flag a small helper or inline constant",
		},
		"review-reliability": {
			"Rule sources: ai-course-2 slides",
			"Block behavior changes without tests that assert externally visible contract",
			"Block when CI can pass with `test.only`",
			"Do not flag intentional reliance on built-in async waiting/trace visibility",
		},
		"review-resilience": {
			"Rule sources: ai-course-2 slides",
			"Flag failures with no fallback, retry, or graceful-degradation path",
			"prod error rate > 1% investigate, > 2% emergency, > 5% all hands",
			"Do not flag explicitly low-impact expected issues",
		},
	}

	for _, dir := range nativeDirs {
		for _, agent := range reviewAgents {
			content := MustRead(dir + "/" + agent + ".md")
			for _, want := range []string{"read-only reviewer", "severity: BLOCKER | CRITICAL | WARNING | SUGGESTION", "No findings."} {
				if !strings.Contains(content, want) {
					t.Fatalf("%s/%s.md missing %q", dir, agent, want)
				}
			}
			for _, want := range agentRules[agent] {
				if !strings.Contains(content, want) {
					t.Fatalf("%s/%s.md missing concrete 4R rule %q", dir, agent, want)
				}
			}
		}
	}

	for _, agent := range reviewAgents {
		md := MustRead("kimi/agents/" + agent + ".md")
		yaml := MustRead("kimi/agents/" + agent + ".yaml")
		if !strings.Contains(md, "No findings.") || !strings.Contains(yaml, "system_prompt_path: ./"+agent+".md") {
			t.Fatalf("kimi review agent %s missing prompt or YAML binding", agent)
		}
		for _, want := range agentRules[agent] {
			if !strings.Contains(md, want) {
				t.Fatalf("kimi review agent %s missing concrete 4R rule %q", agent, want)
			}
		}
	}

	for _, overlay := range []string{"opencode/sdd-overlay-single.json", "opencode/sdd-overlay-multi.json"} {
		content := MustRead(overlay)
		for _, agent := range reviewAgents {
			if !strings.Contains(content, `"`+agent+`"`) || !strings.Contains(content, "No findings.") {
				t.Fatalf("%s missing OpenCode review agent %s", overlay, agent)
			}
			for _, want := range agentRules[agent] {
				want = strings.ReplaceAll(want, "`", "")
				if !strings.Contains(content, want) {
					t.Fatalf("%s review agent %s missing concrete 4R rule %q", overlay, agent, want)
				}
			}
		}
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
		"Use the `question` tool for SDD Session Preflight",
		"Ask all four preflight groups in one single `question` tool call",
		"OpenCode can render the groups as tabs",
		"Do NOT run this as a sequential wizard",
		"Do NOT issue four separate `question` tool calls",
		"The single `question` tool call must contain these four localized groups in this order",
		"Match the user's current language and active persona",
		"Treat the preflight UI as direct orchestrator conversation",
		"not as a generated technical artifact",
		"Technical artifacts still default to English",
		"this UI follows the user's conversation language/persona",
		"Do NOT mix languages inside one grouped question",
		"Do NOT show option codes",
		"Do NOT show canonical values",
		"map the selected human labels to canonical values internally",
		"¿Quiere ajustar algo o continuamos?",
		"Artifacts: OpenSpec, Engram, Both",
		"Review: 400 lines, 800 lines, Other",
		"### SDD Entry Routing (MANDATORY)",
		"Never launch `sdd-apply` just because the user asked to implement a feature",
		"In **Interactive** mode, between phases",
		"Ask before launching the next phase",
		"Interactive approval is phase-scoped",
		"approve only the immediate next phase",
		"Before the `sdd-propose` phase in interactive mode",
		"proposal question round",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("opencode/sdd-orchestrator.md missing required preflight wording %q", required)
		}
	}
}

func TestOpenCodeSDDOrchestratorPreflightDoesNotUseVisibleCodesOrCanonicalUIValues(t *testing.T) {
	content := MustRead("opencode/sdd-orchestrator.md")
	start := strings.Index(content, "User-facing preflight question format:")
	if start < 0 {
		t.Fatal("opencode/sdd-orchestrator.md missing preflight question format block")
	}
	end := strings.Index(content[start:], "Map answers to canonical values")
	if end < 0 {
		t.Fatal("opencode/sdd-orchestrator.md missing end of preflight question format block")
	}
	uiBlock := content[start : start+end]

	for _, forbidden := range []string{"A1", "A2", "B1", "C1", "D1", "`interactive`", "`openspec`", "`ask-always`"} {
		if strings.Contains(uiBlock, forbidden) {
			t.Fatalf("preflight UI instructions should not expose option codes or canonical values; found %q", forbidden)
		}
	}
}

func TestClaudeSDDWorkflowRequiresSessionPreflight(t *testing.T) {
	content := MustRead("claude/sdd-orchestrator-workflow.md")

	for _, required := range []string{
		"### SDD Session Preflight (HARD GATE)",
		"Before executing ANY SDD command or natural-language SDD request",
		"**Execution mode**",
		"**Artifact store**",
		"**Chained PR strategy**",
		"**Review budget**",
		"`openspec/config.yaml`, existing SDD artifacts, previous `sdd-init` results, or installed SDD assets do NOT satisfy session preflight",
		"Use the built-in `AskUserQuestion` tool for SDD Session Preflight",
		"Do NOT render the full preflight menu as plain chat text",
		"Ask all four preflight groups in one single `AskUserQuestion` tool call",
		"Do NOT run this as a sequential wizard",
		"Do NOT issue four separate `AskUserQuestion` tool calls",
		"Match the user's current language and active persona",
		"Do NOT show option codes",
		"Do NOT show canonical values",
		"map the selected human labels to canonical values internally",
		"1. Pace: Interactive, Automatic.",
		"2. Artifacts: OpenSpec, Engram, Both.",
		"3. PRs: Ask me, Single PR, Chained, Auto.",
		"4. Review: 400 lines, 800 lines, Other.",
		"### SDD Entry Routing (MANDATORY)",
		"Never launch `sdd-apply` just because the user asked to implement a feature",
		"Only launch `sdd-apply` when all are true",
		"If any dependency is missing, STOP and propose `/sdd-new` or `/sdd-ff`; do not implement",
		"or `hybrid` when Engram is callable",
		"Both -> `hybrid`",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("claude/sdd-orchestrator-workflow.md missing required preflight wording %q", required)
		}
	}

	for _, forbidden := range []string{
		"`question` tool",
		"groups as tabs",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("claude/sdd-orchestrator-workflow.md must use Claude Code's AskUserQuestion mechanics, not OpenCode wording %q", forbidden)
		}
	}

	if strings.Contains(content, "`both`") {
		t.Fatal("claude/sdd-orchestrator-workflow.md must not use `both` as a canonical artifact-store value; the Claude asset vocabulary is `hybrid` end to end (Dispatcher Guard, Artifact Store Policy/Mode)")
	}

	for _, section := range []string{"### Execution Mode", "### Artifact Store Mode"} {
		idx := strings.Index(content, section)
		if idx < 0 {
			t.Fatalf("claude/sdd-orchestrator-workflow.md missing section %q", section)
		}
		body := content[idx+len(section):]
		if end := strings.Index(body, "\n### "); end >= 0 {
			body = body[:end]
		}
		if !strings.Contains(body, "This is collected by `SDD Session Preflight`") {
			t.Fatalf("claude/sdd-orchestrator-workflow.md section %q must state its value is collected by SDD Session Preflight instead of independently re-asking", section)
		}
	}

	preflight := strings.Index(content, "### SDD Session Preflight (HARD GATE)")
	routing := strings.Index(content, "### SDD Entry Routing (MANDATORY)")
	initGuard := strings.Index(content, "### SDD Init Guard (MANDATORY)")
	if !(preflight < routing && routing < initGuard) {
		t.Fatalf("claude/sdd-orchestrator-workflow.md section order must be preflight (%d) < entry routing (%d) < init guard (%d)", preflight, routing, initGuard)
	}
}

func TestSDDFFCommandsHonorInteractiveMode(t *testing.T) {
	for _, path := range []string{
		"opencode/commands/sdd-ff.md",
		"claude/commands/sdd-ff.md",
	} {
		t.Run(path, func(t *testing.T) {
			content := MustRead(path)

			for _, forbidden := range []string{
				"Present a combined summary after ALL phases complete (not between each one).",
			} {
				if strings.Contains(content, forbidden) {
					t.Fatalf("%s must not contain unqualified back-to-back planning instruction %q", path, forbidden)
				}
			}

			for _, required := range []string{
				"Honor the cached execution mode from SDD Session Preflight",
				"In `interactive` mode: run only the next planning phase",
				"Do not launch the following phase until the user confirms",
				"In `auto` mode: run all planning phases back-to-back",
			} {
				if !strings.Contains(content, required) {
					t.Fatalf("%s missing interactive/auto guard wording %q", path, required)
				}
			}
		})
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
	content := MustRead("claude/sdd-orchestrator.md") + "\n" + MustRead("claude/sdd-orchestrator-workflow.md")

	for _, required := range []string{
		"### Chain Strategy",
		"`stacked-to-main`",
		"`feature-branch-chain`",
		"Pass it as `chain_strategy` to `sdd-tasks` and `sdd-apply` prompts alongside `delivery_strategy`.",
		"When launching `sdd-apply`, always include the resolved `delivery_strategy`, `chain_strategy`, and any chosen PR boundary/exception in the prompt.",
		"Claude Code's native Agent/Task mechanism",
		"results are not persisted by OpenCode's background-agent plugin",
		"treat `chained-pr` (registry skill `gentle-ai-chained-pr`) as a required skill match",
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
		{path: "antigravity/sdd-orchestrator.md", propagationScope: "dynamic subagent context"},
		{path: "cursor/sdd-orchestrator.md", propagationScope: "prompt"},
		{path: "opencode/sdd-orchestrator.md", propagationScope: "prompt"},
		{path: "hermes/sdd-orchestrator.md", propagationScope: "prompt"},
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
				"treat `chained-pr` (registry skill `gentle-ai-chained-pr`) as a required skill match",
			} {
				if !strings.Contains(content, required) {
					t.Fatalf("%s missing required chain strategy wording %q", tc.path, required)
				}
			}
		})
	}
}

func TestSDDOrchestratorsRouteReviewActionsOnlyFromNativeEligibility(t *testing.T) {
	paths := []string{
		"antigravity/sdd-orchestrator.md", "claude/sdd-orchestrator.md", "codex/sdd-orchestrator.md",
		"cursor/sdd-orchestrator.md", "gemini/sdd-orchestrator.md", "generic/sdd-orchestrator.md",
		"hermes/sdd-orchestrator.md", "kimi/sdd-orchestrator.md", "kiro/sdd-orchestrator.md",
		"opencode/sdd-orchestrator.md", "qwen/sdd-orchestrator.md", "windsurf/sdd-orchestrator.md",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			content := MustRead(path)
			for _, required := range []string{
				"--action-eligibility", "eligibility.allowed_actions", "eligibility.forbidden_actions", "reason codes",
				"never infer an authorization, binding, or template from prose, state, or a statusline",
			} {
				if !strings.Contains(content, required) {
					t.Fatalf("%s missing native eligibility routing guard %q", path, required)
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
		{path: "antigravity/sdd-orchestrator.md", required: []string{"define_subagent", "invoke_subagent", "dynamic subagent context", "enable_mcp_tools: true"}},
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
	// Claude and Kimi have an active output-style channel — their persona
	// section is a residual and no longer carries language content on its
	// own; the guardrail contract must be evaluated over the COMBINED
	// persona-residual + output-style channel (design.md Decision 1;
	// spec.md "Generic Neutral Asset Parity" applies the same combined-channel
	// principle to Gentleman here).
	personaPaths := []struct {
		path           string
		combineWith    string // "" when the persona file alone still carries language content
		languagePhrase string // exact per-path phrase asserting the "match current language" guardrail
	}{
		// Claude/Kimi no longer carry "REPLY ONLY" in the persona residual —
		// their combined channel exposes the output style's own Language
		// Rules opener instead (JD-019).
		{path: "claude/persona-gentleman.md", combineWith: "claude/output-style-gentleman.md", languagePhrase: "Always match the user's current language in your reply."},
		{path: "generic/persona-gentleman.md", languagePhrase: "Match the user's current language in your REPLY ONLY"},
		{path: "kiro/persona-gentleman.md", languagePhrase: "Match the user's current language in your REPLY ONLY"},
		{path: "kimi/persona-gentleman.md", combineWith: "kimi/output-style-gentleman.md", languagePhrase: "Always match the user's current language in your reply."},
		{path: "opencode/persona-gentleman.md", languagePhrase: "Match the user's current language in your REPLY ONLY"},
	}

	for _, tc := range personaPaths {
		t.Run(tc.path, func(t *testing.T) {
			content := MustRead(tc.path)
			if tc.combineWith != "" {
				content += "\n" + MustRead(tc.combineWith)
			}

			for _, banned := range []string{
				`Say "déjame verificar"`,
				`Spanish input → Rioplatense Spanish (voseo):`,
				`English input → same warm energy:`,
			} {
				if strings.Contains(content, banned) {
					t.Fatalf("%s (combined=%q) still contains language-biasing phrase %q", tc.path, tc.combineWith, banned)
				}
			}

			for _, required := range []string{
				tc.languagePhrase,
				"Do not switch languages unless the user does, asks you to, or you are quoting/translating content.",
				"keep the full reply in natural English with the same warm energy",
			} {
				if !strings.Contains(content, required) {
					t.Fatalf("%s (combined=%q) missing language guardrail %q", tc.path, tc.combineWith, required)
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
				// Decision 4/JD-013: merged bullet replaces the old verbatim wording.
				"keep the full reply in natural English with the same warm energy",
			} {
				if !strings.Contains(content, required) {
					t.Fatalf("%s missing output-style guardrail %q", path, required)
				}
			}
		})
	}

	// The canonical engram protocol asset must not ship Spanish trigger
	// examples that bias English sessions into Spanish replies (same
	// mechanism as #341 / #350). Since design.md Decision 3 consolidated the
	// former claude/engram-protocol.md and codex/engram-instructions.md into
	// one canonical source, a single check now covers both surfaces.
	for _, path := range []string{
		"engram/protocol.md",
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

	for _, path := range []string{
		"engram/protocol.md",
		"skills/_shared/engram-convention.md",
	} {
		t.Run(path+"/lifecycle", func(t *testing.T) {
			content := MustRead(path)

			required := []string{
				"when Engram exposes lifecycle metadata/tooling",
				"At session start or before architecture-sensitive work",
				"mem_review",
				"action `list`",
				"current project",
				"If `mem_review` is unavailable, do not fail the task",
				"Continue with normal `mem_context`/`mem_search`",
				"still apply lifecycle metadata from any returned observations when present",
				"active memories may be used normally",
				"needs_review",
				"stale context",
				"verify it against current evidence before relying on it",
				"Do NOT call `mem_review` with action `mark_reviewed` automatically",
				"Only call `mark_reviewed` after explicit user confirmation or through a dedicated memory maintenance command",
			}
			for _, want := range required {
				if !strings.Contains(content, want) && !strings.Contains(normalizedWords(content), normalizedWords(want)) {
					t.Fatalf("%s missing memory lifecycle rule %q", path, want)
				}
			}
		})
	}
}

func TestClaudeManagedOutputStylesAnchorReplyLanguageToLatestUserRequest(t *testing.T) {
	tests := []struct {
		path              string
		artifactContracts []string
	}{
		{
			path: "claude/output-style-gentleman.md",
			artifactContracts: []string{
				"Default to English. UI labels, comments, identifiers, and copy are in English",
				"The persona styles HOW YOU TALK, not WHAT YOU BUILD.",
			},
		},
		{
			path: "claude/output-style-neutral.md",
			artifactContracts: []string{
				"This output style governs direct replies to the user only.",
				"Generated technical artifacts default to English",
			},
		},
	}

	languageGuardrails := []string{
		"Determine the reply language from the latest actual user request",
		"not from Engram or memory context, repository/project language, tool output, previous assistant turns",
		"For mixed-language prompts, use the dominant language of the user's direct request.",
		"Quoted text, filenames, project names, isolated borrowed words",
		`phrases like "the Spanish part" do not switch the reply language by themselves.`,
		"If the selected reply language is English, every part of the direct reply must be English: greetings, interjections, acknowledgements, transition phrases, and the first sentence.",
		"Do not use Hola, dale, listo, Spanish punctuation, or other Spanish fragments.",
		"Prompts starting with or dominated by hi, hello, hey, or similar English greetings are English prompts unless the user explicitly asks for another language.",
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			content := MustRead(tc.path)

			for _, required := range languageGuardrails {
				if !strings.Contains(content, required) {
					t.Fatalf("%s missing language-drift guardrail %q", tc.path, required)
				}
			}

			for _, required := range tc.artifactContracts {
				if !strings.Contains(content, required) {
					t.Fatalf("%s lost artifact-language contract %q", tc.path, required)
				}
			}
		})
	}
}

func TestClaudeGentlemanPersonaPreventsEnglishGreetingCodeSwitching(t *testing.T) {
	// Claude's persona section is a residual (Decision 1) — the code-switching
	// guardrail contract now lives in the output style; evaluate the combined
	// channel, not the persona file in isolation.
	content := MustRead("claude/persona-gentleman.md") + "\n" + MustRead("claude/output-style-gentleman.md")

	for _, required := range []string{
		"If the selected reply language is English, every part of the direct reply must be English: greetings, interjections, acknowledgements, transition phrases, and the first sentence.",
		"Do not use Hola, dale, listo, Spanish punctuation, or other Spanish fragments.",
		"Prompts starting with or dominated by hi, hello, hey, or similar English greetings are English prompts unless the user explicitly asks for another language.",
		"Do not switch languages unless the user does, asks you to, or you are quoting/translating content.",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("claude/persona-gentleman.md missing code-switching guardrail %q", required)
		}
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

	// We expect 23 skill directories (10 SDD + judgment-day + 6 foundation + 4 sustainable-review + hermes-ephemeral-delegation + _shared).
	if skillDirs != 23 {
		t.Fatalf("expected 23 skill directories, got %d", skillDirs)
	}

	// Verify each skill directory has a SKILL.md.
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if entry.Name() == "_shared" {
			for _, sharedFile := range []string{"persistence-contract.md", "engram-convention.md", "openspec-convention.md", "sdd-phase-common.md", "sdd-status-contract.md", "skill-resolver.md"} {
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

func TestSDDStatusContractMatchesNativeShape(t *testing.T) {
	content := MustRead("skills/_shared/sdd-status-contract.md")

	for _, want := range []string{
		"schemaName: gentle-ai.sdd-status",
		"schemaVersion: 1",
		"changeName: <change-name-or-null>",
		"artifactStore: openspec",
		"mode: repo-local",
		"path: <absolute path to openspec>",
		"changeRoot: <absolute path to openspec/changes/<change> or null>",
		"completed: 0",
		"pending: 0",
		"allComplete: false",
		"proposal: blocked | ready | all_done",
		"specs: blocked | ready | all_done",
		"design: blocked | ready | all_done",
		"tasks: blocked | ready | all_done",
		"relationships:",
		"dependsOn: []",
		"sameDomainActiveChanges: []",
		"phaseInstructions:",
		"blockedReasons: []",
		"Manual fallback status MUST stay shape-compatible with native `gentle-ai.sdd-status` JSON",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("sdd-status-contract missing native-shape field %q", want)
		}
	}

	for _, forbidden := range []string{
		"schemaName: spec-driven",
		"root: <project-or-openspec-root>",
		"changesDir: <openspec/changes or engram topic prefix>",
		"complete: 0",
		"remaining: 0",
		"unchecked: []",
		"warnings: []",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("sdd-status-contract contains legacy field %q", forbidden)
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

			orchestrator, ok := agents["gentle-orchestrator"].(map[string]any)
			if !ok {
				t.Fatalf("%q missing gentle-orchestrator agent", assetPath)
			}
			permissions, ok := orchestrator["permission"].(map[string]any)
			if !ok || permissions["question"] != "allow" {
				t.Fatalf("%q gentle-orchestrator must allow question permission", assetPath)
			}
			tools, ok := orchestrator["tools"].(map[string]any)
			if !ok {
				t.Fatalf("%q gentle-orchestrator missing tools", assetPath)
			}
			replacedTools, ok := tools["__replace__"].(map[string]any)
			if !ok || replacedTools["question"] != true {
				t.Fatalf("%q gentle-orchestrator must enable question tool", assetPath)
			}

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
					for _, want := range []string{"not the orchestrator", "Do NOT delegate", "Do NOT call task", "Do NOT launch sub-agents"} {
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
				t.Errorf("%s contains banned pattern %q — use a safer detection mechanism instead", path, forbidden)
			}
		}
	}
}

// TestOpenCodeCommandsDetectWorkspaceAgentSide guards against parse-time shell
// interpolation for the working directory in OpenCode command files. In
// OpenCode Desktop (Electron), patterns like !pwd and !basename $(pwd) evaluate
// against the Electron app data directory rather than the project workspace
// (issue #74). Command files must instruct the agent to detect the workspace
// via its bash tool (e.g. git rev-parse --show-toplevel) and treat that
// returned path as authoritative.
func TestOpenCodeCommandsDetectWorkspaceAgentSide(t *testing.T) {
	forbiddenPatterns := []string{
		"!`pwd`",
		"!`basename \"$(pwd)\"`",
	}
	const requiredHint = "git rev-parse --show-toplevel"

	entries, err := FS.ReadDir("opencode/commands")
	if err != nil {
		t.Fatalf("ReadDir(opencode/commands) error = %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := "opencode/commands/" + entry.Name()
		content := MustRead(path)
		for _, pat := range forbiddenPatterns {
			if strings.Contains(content, pat) {
				t.Errorf("%s contains banned shell interpolation %q — detect the workspace via the agent's bash tool instead (see #74)", path, pat)
			}
		}
		if strings.Contains(content, "Working directory:") && !strings.Contains(content, requiredHint) {
			t.Errorf("%s mentions \"Working directory:\" without the agent-side detection hint %q (see #74)", path, requiredHint)
		}
	}
}

// TestClaudeCommandsDetectWorkspaceAgentSide guards against parse-time shell
// interpolation for workspace/project context in Claude slash commands. Claude
// Code performs static permission validation before running commands, so forms
// like !`basename "$(pwd)"` can be rejected before the agent starts. Command
// files must instruct the agent to detect the workspace from inside the session.
func TestClaudeCommandsDetectWorkspaceAgentSide(t *testing.T) {
	forbiddenPatterns := []string{
		"!pwd",
		"!`pwd`",
		"!basename $(pwd)",
		"!basename \"$(pwd)\"",
		"!basename '$(pwd)'",
		"!`basename $(pwd)`",
		"!`basename \"$(pwd)\"`",
		"!`basename '$(pwd)'`",
		"!git rev-parse --show-toplevel",
		"!`git rev-parse --show-toplevel`",
	}
	const requiredHint = "git rev-parse --show-toplevel"

	entries, err := FS.ReadDir("claude/commands")
	if err != nil {
		t.Fatalf("ReadDir(claude/commands) error = %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := "claude/commands/" + entry.Name()
		content := MustRead(path)
		for _, pat := range forbiddenPatterns {
			if strings.Contains(content, pat) {
				t.Errorf("%s contains banned Claude parse-time shell interpolation %q — detect workspace/project context agent-side instead (see #837)", path, pat)
			}
		}
		for _, line := range strings.Split(content, "\n") {
			if (strings.Contains(line, "Working directory:") || strings.Contains(line, "Current project:")) && strings.Contains(line, "!") {
				t.Errorf("%s contains parse-time shell interpolation in workspace/project context line %q — detect it agent-side instead (see #837)", path, line)
			}
		}
		if strings.Contains(content, "Working directory:") && !strings.Contains(content, requiredHint) {
			t.Errorf("%s mentions \"Working directory:\" without the agent-side detection hint %q (see #837)", path, requiredHint)
		}
	}
}

// TestOrchestratorsRequireAutomaticGatekeeper asserts that every orchestrator
// template validates every phase boundary and keeps design/apply validation
// artifact-bound rather than silently opening an adversarial code review.
func TestOrchestratorsRequireAutomaticGatekeeper(t *testing.T) {
	paths := []string{
		"antigravity/sdd-orchestrator.md",
		"claude/sdd-orchestrator.md",
		"codex/sdd-orchestrator.md",
		"cursor/sdd-orchestrator.md",
		"gemini/sdd-orchestrator.md",
		"generic/sdd-orchestrator.md",
		"hermes/sdd-orchestrator.md",
		"kimi/sdd-orchestrator.md",
		"kiro/sdd-orchestrator.md",
		"opencode/sdd-orchestrator.md",
		"qwen/sdd-orchestrator.md",
		"windsurf/sdd-orchestrator.md",
	}
	anchors := []string{
		"Automatic Mode Gatekeeper",
		"The gatekeeper runs after every phase",
		"Inline for low-risk phases",
		"Fresh-context phase-contract validator",
		"re-run the same phase exactly once",
		"STOP the automatic chain",
	}
	for _, path := range paths {
		content := MustRead(path)
		if path == "claude/sdd-orchestrator.md" {
			content += "\n" + MustRead("claude/sdd-orchestrator-workflow.md")
		}
		for _, anchor := range anchors {
			if !strings.Contains(content, anchor) {
				t.Fatalf("%s missing Automatic Mode Gatekeeper anchor %q", path, anchor)
			}
		}

		validatorLine := markdownLineContaining(content, "Fresh-context phase-contract validator")
		if !lineContainsAll(
			"sdd-design",
			"sdd-apply",
			"phase artifact against its inputs",
			"not adversarial implementation review",
			"code diff",
			"creates no 4R/Judgment-Day",
			"budget",
		)(validatorLine) {
			t.Fatalf("%s fresh-context phase-contract validator must validate design/apply artifacts against inputs without code-diff review or a 4R/Judgment-Day budget: %q", path, validatorLine)
		}
		if !lineContainsAny("does not inspect the code diff", "inspects no code diff")(validatorLine) {
			t.Fatalf("%s fresh-context phase-contract validator must prohibit code-diff inspection: %q", path, validatorLine)
		}
	}
}

func TestSDDOrchestratorsRouteFreshReviewsToConcreteReviewLenses(t *testing.T) {
	t.Run("rejects lifecycle gates that reopen review", func(t *testing.T) {
		weakContent := `### Mandatory Delegation Triggers (Non-Skippable)
3. **Lifecycle receipt rule**: before commit, push, PR, or release, launch a fresh review through Review Lens Selection.
4. **Incident rule**: after wrong cwd or merge recovery, run Review Lens Selection before continuing.
6. **Fresh review rule**: use fresh context for adversarial review of diffs.

#### Review Lens Selection
- review-risk
- review-resilience
- review-readability
- review-reliability
- If multiple rows match, run the narrow set that covers the risk.
`
		if problems := boundedReviewRoutingProblems(weakContent); len(problems) == 0 {
			t.Fatal("fixture should fail because lifecycle and incident gates reopen review and fresh review lacks an explicit review/start target")
		}
	})

	paths := []string{
		"antigravity/sdd-orchestrator.md",
		"claude/sdd-orchestrator.md",
		"codex/sdd-orchestrator.md",
		"cursor/sdd-orchestrator.md",
		"gemini/sdd-orchestrator.md",
		"generic/sdd-orchestrator.md",
		"hermes/sdd-orchestrator.md",
		"kimi/sdd-orchestrator.md",
		"kiro/sdd-orchestrator.md",
		"opencode/sdd-orchestrator.md",
		"qwen/sdd-orchestrator.md",
		"windsurf/sdd-orchestrator.md",
	}
	requiredLenses := []string{
		"Review Lens Selection",
		"review-risk",
		"review-resilience",
		"review-readability",
		"review-reliability",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			content := MustRead(path)
			section := markdownSection(content, "#### Review Lens Selection")
			if section == "" {
				t.Fatalf("%s missing Review Lens Selection section", path)
			}
			for _, want := range requiredLenses {
				if !strings.Contains(section, want) {
					t.Fatalf("%s Review Lens Selection section missing %q", path, want)
				}
			}

			tierChecks := []struct {
				label    string
				matcher  func(string) bool
				contract string
			}{
				{
					label:    "**Trivial diff**",
					matcher:  lineContainsAll("run no lens"),
					contract: "must route to zero lenses",
				},
				{
					label:    "**Standard diff**",
					matcher:  lineContainsAll("run exactly ONE lens"),
					contract: "must route to exactly one concrete lens",
				},
				{
					label: "**Hot path**",
					matcher: lineContainsAll(
						"run the full 4R set",
						"`review-risk`",
						"`review-resilience`",
						"`review-readability`",
						"`review-reliability`",
					),
					contract: "must route to all four concrete lenses",
				},
			}
			for _, check := range tierChecks {
				line := markdownLineContaining(section, check.label)
				if !check.matcher(line) {
					t.Fatalf("%s Review Lens Selection %s: %q", path, check.contract, line)
				}
			}
			for _, want := range []string{
				"rerun only the originating lens(es) that produced open verified BLOCKER/CRITICAL findings",
				"never rerun clean lenses or lenses with only WARNING/SUGGESTION findings",
				"Native ordinary review keeps its targeted validator and never reruns initial lenses",
			} {
				if !strings.Contains(content, want) {
					t.Fatalf("%s missing ad-hoc severe recheck contract %q", path, want)
				}
			}

			if problems := boundedReviewRoutingProblems(content); len(problems) > 0 {
				t.Fatalf("%s bounded review guidance violates receipt or explicit review-start routing: %s", path, strings.Join(problems, "; "))
			}
		})
	}
}

func boundedReviewRoutingProblems(content string) []string {
	triggerSection := firstMarkdownSection(content,
		"### Mandatory Delegation Triggers",
		"#### Mandatory Phase-Boundary Triggers",
	)
	if triggerSection == "" {
		return []string{"missing Mandatory Delegation Triggers or Mandatory Phase-Boundary Triggers section"}
	}

	checks := []struct {
		label    string
		matcher  func(string) bool
		contract string
	}{
		{
			label: "Lifecycle receipt rule",
			matcher: lineContainsAll(
				"before commit",
				"before push, PR, or release",
				"content-bound receipt",
				"gentle-ai review validate --gate pre-commit --cwd <repo>",
				"--lineage <known-lineage>",
				"gentle-ai review validate --gate <gate> --cwd <repo>",
				"same exact `--lineage`",
				"Never fall back to inventory discovery",
				"never launch a lens",
				"Judgment Day",
				"new budget at a repeated gate",
				"stage every reviewed path",
				"without changing content or mode",
			),
			contract: "must validate the content-bound receipt without launching a lens, Judgment Day, or a new budget",
		},
		{
			label:    "Incident rule",
			matcher:  lineContainsAll("remain immutable", "validate the existing receipt", "explicit scope action", "not reopened review"),
			contract: "must prove target immutability and validate the existing receipt without reopening review",
		},
		{
			label:    "Fresh review rule",
			matcher:  lineContainsAll("fresh adversarial lenses", "only inside one explicit", "`review/start(target)`"),
			contract: "must start fresh adversarial lenses only through an explicit review/start target",
		},
	}

	var problems []string
	for _, check := range checks {
		line := markdownLineContaining(triggerSection, "**"+check.label+"**")
		if line == "" {
			problems = append(problems, check.label+": missing trigger rule")
			continue
		}
		if !check.matcher(line) {
			problems = append(problems, check.label+": "+check.contract)
		}
	}
	if starts := strings.Count(triggerSection, "`review/start(target)`"); starts != 1 {
		problems = append(problems, "Fresh review rule: expected exactly one explicit review/start target in trigger rules")
	}
	return problems
}

func firstMarkdownSection(content string, headings ...string) string {
	for _, heading := range headings {
		if section := markdownSection(content, heading); section != "" {
			return section
		}
	}
	return ""
}

func markdownLineContaining(content, needle string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}

func lineContainsAll(needles ...string) func(string) bool {
	return func(line string) bool {
		for _, needle := range needles {
			if !strings.Contains(line, needle) {
				return false
			}
		}
		return true
	}
}

func lineContainsAny(needles ...string) func(string) bool {
	return func(line string) bool {
		for _, needle := range needles {
			if strings.Contains(line, needle) {
				return true
			}
		}
		return false
	}
}

func markdownSection(content, heading string) string {
	start := strings.Index(content, heading)
	if start == -1 {
		return ""
	}
	section := content[start:]
	end := len(section)
	for _, levelHeading := range []string{"\n#### ", "\n### ", "\n## "} {
		if next := strings.Index(section[len(heading):], levelHeading); next != -1 {
			end = min(end, len(heading)+next)
		}
	}
	return section[:end]
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
