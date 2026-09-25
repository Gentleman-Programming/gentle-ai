package assets

import (
	"io/fs"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TestODDOnlyEmbeddedInventory guards every shipped runtime, not just one
// install layout. Mixed orchestrators are retained and rewritten separately.
func TestODDOnlyEmbeddedInventory(t *testing.T) {
	var forbidden []string
	orchestrators := map[string]bool{}
	err := fs.WalkDir(FS, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		parts := strings.Split(path, "/")
		name := entry.Name()
		for _, part := range parts {
			if strings.HasPrefix(part, "sdd-") || strings.HasPrefix(part, "gentle-sdd-") || part == "sdd" || part == "openspec-convention.md" {
				// Existing orchestrators are mixed ODD/RDD assets; their
				// SDD content must be rewritten, not removed wholesale.
				if name != "sdd-orchestrator.md" {
					forbidden = append(forbidden, path)
				}
				break
			}
		}
		if name == "orchestrator.md" || name == "sdd-orchestrator.md" {
			orchestrators[parts[0]] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(orchestrators) == 0 {
		t.Fatal("embedded inventory has no retained orchestrator assets")
	}
	for _, runtime := range []string{"claude", "opencode", "gemini", "antigravity", "codex", "cursor", "kiro", "kimi", "hermes", "generic", "qwen", "windsurf"} {
		if !orchestrators[runtime] {
			t.Errorf("%s lost its ODD/RDD orchestrator asset", runtime)
		}
	}
	if len(forbidden) != 0 {
		t.Errorf("embedded inventory still ships SDD-only assets: %s", strings.Join(forbidden, ", "))
	}
}

// TestKimiInstalledPromptAndAgentReferences guards the embedded Kimi entrypoints
// rather than only the standalone orchestrator asset.
func TestKimiInstalledPromptAndAgentReferences(t *testing.T) {
	prompt := MustRead("kimi/KIMI.md")
	agent := MustRead("kimi/agents/gentleman.yaml")
	for path, content := range map[string]string{"kimi/KIMI.md": prompt, "kimi/agents/gentleman.yaml": agent} {
		if match := regexp.MustCompile(`(?i)\bsdd(?:[-/]|\b)|spec-driven|openspec`).FindString(content); match != "" {
			t.Errorf("%s offers retired SDD reference %q", path, match)
		}
	}
	for _, include := range []string{"strict-tdd-mode.md", "agent-routing.md"} {
		if !strings.Contains(prompt, `{% include "`+include+`" ignore missing %}`) {
			t.Errorf("Kimi system prompt lost %s", include)
		}
	}
	if !strings.Contains(MustRead("kimi/orchestrator.md"), "Organic Driven Development Is The Default Workflow") ||
		!strings.Contains(MustRead("kimi/orchestrator.md"), "Review Execution Contract") {
		t.Error("Kimi ODD/RDD orchestrator asset is missing its active contracts")
	}
	if !strings.Contains(agent, "name: gentleman") || !strings.Contains(agent, "extend: default") {
		t.Error("Kimi gentleman agent lost its identity/default extension")
	}
	// Check each configured subagent path against the embedded inventory; no
	// user-owned installed files are read or modified by this contract test.
	refs := regexp.MustCompile(`(?m)^\s+(?:system_prompt_path|path): (\S+)\s*$`).FindAllStringSubmatch(agent, -1)
	if len(refs) == 0 {
		t.Fatal("Kimi gentleman agent has no system prompt reference")
	}
	for _, ref := range refs {
		var path string
		switch {
		case strings.HasPrefix(ref[1], "../"):
			path = "kimi/" + strings.TrimPrefix(ref[1], "../")
		case strings.HasPrefix(ref[1], "./"):
			path = "kimi/agents/" + strings.TrimPrefix(ref[1], "./")
		default:
			t.Errorf("unexpected Kimi agent reference %q", ref[1])
			continue
		}
		if _, err := fs.Stat(FS, path); err != nil {
			t.Errorf("Kimi agent reference %s is not embedded: %v", path, err)
		}
	}
}

// TestSharedReferencesDoNotAdvertiseSDD checks the content installed under
// skills/_shared, not only the skill names in the embedded inventory.
func TestSharedReferencesDoNotAdvertiseSDD(t *testing.T) {
	entries, err := fs.ReadDir(FS, "skills/_shared")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no shared references embedded")
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := "skills/_shared/" + entry.Name()
		t.Run(entry.Name(), func(t *testing.T) {
			for lineNumber, line := range strings.Split(MustRead(path), "\n") {
				if sharedReferenceInvitesSDD(line) {
					t.Errorf("%s:%d advertises retired workflow: %s", path, lineNumber+1, line)
				}
			}
		})
	}
}

func sharedReferenceInvitesSDD(line string) bool {
	lower := strings.ToLower(line)
	// Historical negative controls are evidence, not instructions. Section
	// markers are stable composer bindings, not prose offered to the agent.
	if strings.Contains(lower, "historical negative control:") || strings.HasPrefix(strings.TrimSpace(lower), "<!-- sdd-orchestrator-section:") {
		return false
	}
	return regexp.MustCompile(`\bsdd\b|sdd[-/]|openspec|spec-driven`).MatchString(lower)
}

func TestSharedReferenceNegativeControls(t *testing.T) {
	for _, tc := range []struct {
		line string
		want bool
	}{
		{"Historical negative control: do not run sdd-apply or create OpenSpec files.", false},
		{"<!-- sdd-orchestrator-section:Language Domain Contract:start -->", false},
		{"Run sdd-apply and persist in openspec/.", true},
		{"Use spec-driven development for this task.", true},
	} {
		if got := sharedReferenceInvitesSDD(tc.line); got != tc.want {
			t.Errorf("sharedReferenceInvitesSDD(%q) = %v, want %v", tc.line, got, tc.want)
		}
	}
}

func TestClaudeODDOnlyOrchestrator(t *testing.T) {
	content := MustRead("claude/orchestrator.md")
	for _, required := range []string{
		"{{GENTLE_AI_ODD_SECTION:Organic Driven Development Is The Default Workflow (MANDATORY)}}",
		"{{GENTLE_AI_ODD_SECTION:Language Domain Contract}}",
		"{{GENTLE_AI_ODD_SECTION:Delegated Verification Gate (MANDATORY)}}",
		"Lossless Blocking Prompts", "Gentle AI Provider Defect Handoff",
		"Native Checking Contract", "Review Execution Contract", "Mandatory Delegation Triggers",
	} {
		if !strings.Contains(content, required) {
			t.Errorf("Claude orchestrator missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"{{GENTLE_AI_SDD_SECTION:", "SDD Workflow", "SDD Edit-Authority", "sdd-apply",
		"sdd-verify", "sdd-orchestrator-workflow.md", "optional SDD", "Optional SDD rule",
		"sdd-model-assignments", "SDD phase", "OpenSpec", "{{GENTLE_AI_RESEARCH_LIFECYCLE}}",
	} {
		if strings.Contains(content, forbidden) {
			t.Errorf("Claude orchestrator retains %q", forbidden)
		}
	}
}

func TestCodexEmbeddedOrchestratorDoesNotPromiseUninstalledAssignments(t *testing.T) {
	content := MustRead("codex/orchestrator.md")
	if strings.Contains(content, "{{CODEX_ODD_ASSIGNMENTS}}") {
		t.Fatal("embedded Codex orchestrator has no installed renderer for ODD assignments")
	}
	if !strings.Contains(content, "worker classes, not installed named agents") {
		t.Fatal("Codex orchestrator must identify the delegation boundary")
	}
}

func TestPrimaryODDOnlyOrchestrator(t *testing.T) {
	for _, runtime := range []string{"codex", "opencode", "generic"} {
		t.Run(runtime, func(t *testing.T) {
			content := MustRead(runtime + "/orchestrator.md")
			for _, required := range []string{
				"{{GENTLE_AI_ODD_SECTION:Organic Driven Development Is The Default Workflow (MANDATORY)}}",
				"{{GENTLE_AI_ODD_SECTION:Delegated Verification Gate (MANDATORY)}}",
				"Lossless Blocking Prompts", "Gentle AI Provider Defect Handoff",
				"Mandatory Delegation Triggers", "Native Checking Contract", "Review Execution Contract",
			} {
				if !strings.Contains(content, required) {
					t.Errorf("missing %q", required)
				}
			}
			for _, forbidden := range []string{
				"{{GENTLE_AI_SDD_SECTION:", "{{GENTLE_AI_RESEARCH_LIFECYCLE}}",
				"SDD Workflow", "SDD Edit-Authority", "SDD Session Preflight", "sdd-apply",
				"sdd-verify", "sdd-init", "sdd-model-assignments", "optional SDD",
				"Optional SDD rule", "SDD phase", "OpenSpec", "openspec", "{{CODEX_PHASE_EFFORTS}}",
			} {
				if strings.Contains(content, forbidden) {
					t.Errorf("retains %q", forbidden)
				}
			}
			if runtime == "opencode" {
				for _, required := range []string{"Delegation Visibility (OpenCode Desktop)", "native `explore` agent", "`general` agent", "Sub-Agent Launch Deduplication", "Sub-Agent Context Protocol"} {
					if !strings.Contains(content, required) {
						t.Errorf("missing OpenCode contract %q", required)
					}
				}
			}
			if runtime == "codex" {
				for _, required := range []string{"Capability Check", "Blocking Delegation Contract", "Skill Loading for Delegation", "Graceful Degradation Path"} {
					if !strings.Contains(content, required) {
						t.Errorf("missing Codex contract %q", required)
					}
				}
			}
		})
	}
}

// TestRemainingODDOnlyOrchestrators covers mixed prompts retained for migration.
// Their RDD, language, and delegation contracts must survive SDD removal.
func TestRemainingODDOnlyOrchestrators(t *testing.T) {
	for _, runtime := range []string{"antigravity", "cursor", "gemini", "hermes", "kimi", "kiro", "qwen", "windsurf"} {
		t.Run(runtime, func(t *testing.T) {
			content := MustRead(runtime + "/orchestrator.md")
			for _, required := range []string{
				"{{GENTLE_AI_ODD_SECTION:Organic Driven Development Is The Default Workflow (MANDATORY)}}",
				"{{GENTLE_AI_ODD_SECTION:Language Domain Contract}}",
				"{{GENTLE_AI_ODD_SECTION:Delegated Verification Gate (MANDATORY)}}",
				"Lossless Blocking Prompts", "Gentle AI Provider Defect Handoff",
				"Mandatory Delegation Triggers", "Native Checking Contract", "Review Execution Contract",
			} {
				if !strings.Contains(content, required) {
					t.Errorf("missing %q", required)
				}
			}
			for _, forbidden := range []string{
				"{{GENTLE_AI_SDD_SECTION:", "{{GENTLE_AI_RESEARCH_LIFECYCLE}}",
				"SDD Workflow", "SDD Edit-Authority", "SDD Session Preflight", "sdd-apply",
				"sdd-verify", "sdd-init", "sdd-model-assignments", "optional SDD",
				"Optional SDD rule", "SDD phase", "OpenSpec", "openspec",
			} {
				if strings.Contains(content, forbidden) {
					t.Errorf("retains %q", forbidden)
				}
			}
		})
	}
}

// TestOpenCodeTelemetryRuntimePlugin uses a hermetic Node subprocess and a fake
// native boundary. Fixture is published V1 (not V2):
// https://unpkg.com/@opencode-ai/plugin@1.18.30/dist/index.d.ts
// https://unpkg.com/@opencode-ai/sdk@1.18.30/dist/gen/types.gen.d.ts
func TestOpenCodeTelemetryRuntimePlugin(t *testing.T) {
	source, err := Read("opencode/plugins/telemetry-runtime.ts")
	if err != nil {
		t.Fatal(err)
	}
	// No filesystem, retry timer, identity cache or native self-spawn may be
	// introduced by the host adapter. Native no-disk behavior has HTTP tests.
	for _, forbidden := range []string{"node:fs", "node:crypto", "sessionID", "info.id", "batch_id", "setTimeout(", "setInterval(", "MAX_ATTEMPTS", "new Map", `"flush"`, `"ingest"`} {
		if strings.Contains(string(source), forbidden) {
			t.Fatalf("runtime plugin contains %s", forbidden)
		}
	}
	const harness = `import { strict as assert } from "node:assert"
import childProcess from "node:child_process"
import { syncBuiltinESMExports } from "node:module"
const calls = []
const held = []
childProcess.execFile = (file, args, options, callback) => {
  assert.equal(file,"gentle-ai")
  assert.deepEqual(args,["telemetry","runtime","opencode","--json"])
  assert.equal(options.timeout,4000); assert.equal(options.maxBuffer,1024)
  const call = { args, body: "", killed:false }; calls.push(call)
  held.push(callback)
  return { stdin: { on() {}, end(body) { call.body = body || "" } }, kill() {call.killed=true} }
}
syncBuiltinESMExports()
const { default: plugin } = await import("./plugin.mts")
for (const key of ["DO_NOT_TRACK","GENTLE_AI_TELEMETRY","CI","GITHUB_ACTIONS"]) delete process.env[key]
const hooks = await plugin({})
const tick = async () => { await new Promise(resolve => setImmediate(resolve)) }
const info = { role:"assistant", time:{created:1,completed:2}, providerID:"anthropic", modelID:"claude-opus-5", mode:"sdd-apply", path:{cwd:"PRIVATE_PATH"}, parts:["PRIVATE_PROMPT"], error:{name:"APIError",data:{statusCode:429,message:"PRIVATE_ERROR"}} }
// No source identifiers are necessary or read, even locally.
Object.defineProperty(info,"id",{get(){throw new Error("source id read")}})
Object.defineProperty(info,"sessionID",{get(){throw new Error("session id read")}})
const event = value => hooks.event({event:{type:"message.updated",properties:{info:value}}})
await event({...info,role:"user"}); await event({...info,time:{created:1}}); await event({...info,summary:true})
assert.equal(calls.length,0)
for(const [key,value] of [["DO_NOT_TRACK"," yes "],["CI","true"],["GITHUB_ACTIONS","yes"],["GENTLE_AI_TELEMETRY","0"]]) {
 process.env[key]=value;await event(info);delete process.env[key]
}
assert.equal(calls.length,0)
// The native callback is deliberately held: it represents blocked HTTP. The
// hook must resolve now, without waiting for the process or its network result.
let returned=false
void event(info).then(()=>{returned=true})
await tick();assert.equal(returned,true);assert.equal(calls.length,1)
assert(!calls[0].body.includes("PRIVATE"))
const envelope=JSON.parse(calls[0].body)
assert.deepEqual(Object.keys(envelope).sort(),["info","schema"])
assert.equal(envelope.schema,"gentle-ai.telemetry-opencode/v1")
assert.equal(envelope.info.agent,"sdd-apply")
assert.equal(envelope.info.tokens,undefined)
held.shift()(new Error("PRIVATE_NATIVE_ERROR"),"")
await tick();await tick();assert.equal(calls.length,1) // failure never retries
await event(info);assert.equal(calls.length,2) // a new event is a new attempt
held.shift()(null,JSON.stringify({schema:"gentle-ai.telemetry-runtime-send/v1",decision:"discarded"}))
await tick();assert.equal(calls.length,2) // discarded metrics stay discarded
await event({...info,mode:"x".repeat(65)});assert.equal(calls.length,3)
assert.equal(JSON.parse(calls[2].body).info.agent,undefined);held.shift()(null,"")
await event({...info,mode:"bad\nagent"});assert.equal(calls.length,4)
assert.equal(JSON.parse(calls[3].body).info.agent,undefined);held.shift()(null,"")
// No backlog: saturation discards new events rather than scheduling work.
for(let i=0;i<100;i++) await event(info)
assert.equal(held.length,32);assert.equal(calls.length,36)
for(const cb of held.splice(0))cb(new Error("timeout"),"")
await tick();assert.equal(calls.length,36)
await event({...info,modelID:"x".repeat(16385)});assert.equal(calls.length,36)
await event(info);assert.equal(calls.length,37)
await hooks.dispose();assert.equal(calls[36].killed,true)
await event(info);await tick();assert.equal(calls.length,37)
console.log("ok")
`
	output, log := runOpenCodeTransportPluginHarness(t, map[string]string{"plugin.mts": string(source)}, harness, "#!/bin/sh\nexit 99\n")
	if output != "ok\n" || log != "" {
		t.Fatal("unexpected plugin output or native process")
	}
}

// retiredWorkRunCeremonyTokens enumerates the managed-WorkRun control-plane
// vocabulary that organic routing retires. Prompt assets are the one place this
// ceremony can outlive its Go source, because nothing compiles them — so every
// token is checked against every orchestrator instead of a sample.
var retiredWorkRunCeremonyTokens = []string{
	"work-capabilities",
	"work-start",
	"work-advance",
	"work-route",
	"work-status",
	"work-transition",
	"work-reconcile",
	"work-verification-decide",
	"WorkRun",
	"authorizedTransition",
	"Capability stop rule",
	"connectorSessionRef",
	"GENTLE_AI_PRODUCTIVE_RUNTIME",
	"{{GENTLE_AI_RUNTIME_AGENT_ID}}",
	"--contract gentle-ai.work-",
}

func TestODDOrchestratorsCarryNoRetiredWorkRunCeremony(t *testing.T) {
	paths := allODDOrchestratorAssetPaths(t)

	for _, path := range paths {
		// Fold case so a re-cased reintroduction ("workrun", "WORK-START")
		// cannot slip past the guard.
		content := strings.ToLower(MustRead(path))
		for _, token := range retiredWorkRunCeremonyTokens {
			t.Run(path+"#"+token, func(t *testing.T) {
				if strings.Contains(content, strings.ToLower(token)) {
					t.Fatalf("%s retains retired WorkRun ceremony token %q", path, token)
				}
			})
		}
	}
}

func TestOrchestratorsProjectOrganicRouting(t *testing.T) {
	paths := allODDOrchestratorAssetPaths(t)

	for _, path := range paths {
		content := MustRead(path)
		for _, required := range []string{
			"Mandatory Delegation Triggers",
			"Bounded read rule", "read 1–3 files inline",
			"4-file rule", "understanding requires 4+ files",
			"Write rule", "2+ non-trivial files",
			"Context rule", "reading that prepares a write", "broad research",
			"Mandatory Delegation Triggers", "delegated direct",
		} {
			if !strings.Contains(strings.ToLower(content), strings.ToLower(required)) {
				t.Fatalf("%s missing organic routing/native authority contract %q", path, required)
			}
		}
		for _, retired := range []string{
			"#### Review Lens Selection", "run exactly ONE lens", "run the full 4R set",
			"review/start(target)", "gentle-ai.review-integration/v1 --next-transition",
		} {
			if strings.Contains(content, retired) {
				t.Fatalf("%s retained prompt-owned review ceremony %q", path, retired)
			}
		}

		delegationHeading := "### Delegation Rules"
		if path == "codex/orchestrator.md" {
			delegationHeading = "## General Delegation Rules (Always Active)"
		}
		start := strings.Index(content, delegationHeading)
		end := strings.Index(content, "#### Mandatory Delegation Triggers")
		if start < 0 || end <= start {
			t.Fatalf("%s missing bounded general delegation section", path)
		}
		delegation := content[start:end]
		for _, required := range []string{"delegated direct", "does this inflate"} {
			if !strings.Contains(strings.ToLower(delegation), strings.ToLower(required)) {
				t.Fatalf("%s general delegation section missing route-neutral clause %q", path, required)
			}
		}
		for _, forbidden := range []string{
			"4+ files) | — | ✅ `sdd-explore`",
			"4+ files) | — | ✅ run as sdd-explore",
			"multiple files, new logic) | — | ✅ run as sdd-apply",
			"tests, builds, installs | — | ✅ `sdd-verify`",
			"Phase boundaries are not optional",
		} {
			if strings.Contains(delegation, forbidden) {
				t.Fatalf("%s general delegation section routes ordinary work through SDD %q", path, forbidden)
			}
		}
	}
}

func TestAllShippedOrchestratorsKeepDeliveryUnmanaged(t *testing.T) {
	const ordinaryDelivery = "Commit, push, PR, direct-main, emergency, and release gates are informational and unmanaged; ordinary repository policy decides delivery and they never reopen review for unchanged content."
	const receiptValidation = "Commit, push, PR, direct-main, emergency, and release gates validate the same exact owner-issued receipt/authorization"

	for _, path := range allSDDOrchestratorAssetPaths(t) {
		content := MustRead(path)
		if !strings.Contains(content, ordinaryDelivery) {
			t.Fatalf("%s does not leave delivery to ordinary repository policy", path)
		}
		if strings.Contains(content, receiptValidation) {
			t.Fatalf("%s retains receipt-gated delivery guidance", path)
		}
	}
}

func TestOrchestratorsRejectDelegationBypassLanguage(t *testing.T) {
	contents := make(map[string]string)
	for _, path := range allODDOrchestratorAssetPaths(t) {
		contents[path] = MustRead(path)
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

	codex := contents["codex/orchestrator.md"]
	for _, forbidden := range []string{
		"## Solo Path (default)",
		"Run each SDD phase inline, in dependency order, without spawning sub-agents",
		"fall back to the **Solo path**",
		"complete it inline",
	} {
		if strings.Contains(codex, forbidden) {
			t.Fatalf("codex/orchestrator.md contains solo-path bypass wording %q", forbidden)
		}
	}
	for _, want := range []string{
		"### Blocking Delegation Contract",
		"Codex sub-agents MUST be treated as waited handoffs, not fire-and-forget background jobs.",
		"`wait_agent` for every spawned agent in that batch",
		"Parallel does not mean background",
		"## Graceful Degradation Path (tooling unavailable only)",
	} {
		if !strings.Contains(codex, want) {
			t.Fatalf("codex/orchestrator.md missing guarded degradation wording %q", want)
		}
	}
	for _, forbidden := range []string{
		"both `spawn_agent` calls before either `wait_agent`",
	} {
		if strings.Contains(codex, forbidden) {
			t.Fatalf("codex/orchestrator.md contains fire-and-forget delegation wording %q", forbidden)
		}
	}
}

// allODDOrchestratorAssetPaths discovers the shipped top-level runtime prompts.
// Requiring the known runtime roots prevents an accidentally empty glob or a
// dropped agent from silently narrowing the negative guards.
func allODDOrchestratorAssetPaths(t *testing.T) []string {
	t.Helper()
	paths, err := fs.Glob(FS, "*/orchestrator.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, runtime := range []string{"antigravity", "claude", "codex", "cursor", "gemini", "generic", "hermes", "kimi", "kiro", "opencode", "qwen", "windsurf"} {
		path := runtime + "/orchestrator.md"
		if !slices.Contains(paths, path) {
			t.Errorf("missing retained ODD orchestrator %s", path)
		}
	}
	if len(paths) == 0 {
		t.Fatal("no retained ODD orchestrator assets found")
	}
	return paths
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

// TestAllEmbeddedAssetsAreReadable checks every shipped asset through the
// public reader and pins the ODD routing and review payloads that must survive.
func TestAllEmbeddedAssetsAreReadable(t *testing.T) {
	for _, path := range append(allODDOrchestratorAssetPaths(t),
		"skills/_shared/odd-orchestrator-sections.md",
		"skills/_shared/review-ledger-contract.md",
		"opencode/plugins/opencode-review-transport.ts",
		"claude/agents/review-risk.md",
		"skills/go-testing/SKILL.md",
	) {
		if _, err := fs.Stat(FS, path); err != nil {
			t.Errorf("retained ODD/review payload %s is missing: %v", path, err)
		}
	}
	count := 0
	err := fs.WalkDir(FS, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		count++
		t.Run(path, func(t *testing.T) {
			content, err := Read(path)
			if err != nil {
				t.Fatalf("Read(%q) error = %v", path, err)
			}
			if len(strings.TrimSpace(content)) == 0 || len(content) < 50 {
				t.Fatalf("Read(%q) returned empty or suspiciously short content (%d bytes)", path, len(content))
			}
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("embedded asset inventory is empty")
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

	for _, name := range []string{"commands", "plugins", "persona-gentleman.md", "background-subagents.md", "orchestrator.md"} {
		if !seen[name] {
			t.Fatalf("opencode embedded assets missing %q", name)
		}
	}

	commandEntries, err := FS.ReadDir("opencode/commands")
	if err != nil {
		t.Fatalf("ReadDir(opencode/commands) error = %v", err)
	}
	if len(commandEntries) != 2 {
		t.Fatalf("opencode commands count = %d, want 2 retained commands", len(commandEntries))
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
	if len(pluginEntries) != 4 {
		t.Fatalf("opencode plugins count = %d, want 4 retained plugins", len(pluginEntries))
	}
	wantPlugins := map[string]bool{"telemetry-runtime.ts": true, "model-variants.ts": true, "opencode-review-transport.ts": true, "skill-registry.ts": true}
	for _, entry := range pluginEntries {
		if !wantPlugins[entry.Name()] {
			t.Fatalf("unexpected plugin entry = %q", entry.Name())
		}
	}
}

func TestOpenCodeBackgroundPolicyMarkersAreBalanced(t *testing.T) {
	content := MustRead("opencode/background-subagents.md")
	const (
		start = "<!-- gentle-ai:opencode-background-subagents -->"
		end   = "<!-- /gentle-ai:opencode-background-subagents -->"
	)
	trimmed := strings.TrimSpace(content)
	if strings.Count(trimmed, start) != 1 || strings.Count(trimmed, end) != 1 {
		t.Fatalf("background policy marker cardinality = (%d, %d), want (1, 1)", strings.Count(trimmed, start), strings.Count(trimmed, end))
	}
	if !strings.HasPrefix(trimmed, start+"\n") || !strings.HasSuffix(trimmed, "\n"+end) {
		t.Fatalf("background policy markers are not balanced around the complete asset")
	}
}

// TestOpenCodeReviewTransportPluginContract pins the adapter-minimality
// boundary: the plugin correlates one host Task with one Go process, while Go
// owns all prompt, schema, admission, and capture semantics.
func TestOpenCodeReviewTransportPluginContract(t *testing.T) {
	source, err := Read("opencode/plugins/opencode-review-transport.ts")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`gentle-ai.provider-transport/v1`, `"review", "opencode-transport"`, `RELAY_REGISTRY_KEY`, `reviewRelayRegistry()`, `output.args.prompt = (await relay.prompt).prompt`, `output.output = await registration.relay.complete(output.output)`, `"tool.execute.before"`, `"tool.execute.after"`,
		// A refused relay start must fail the Task loudly and never launch an
		// unbound child: the before hook poisons the Task prompt and the after
		// hook replaces child output with the typed refusal, so a host runtime
		// that swallows hook errors still cannot deliver an unbound child's
		// prose as a reviewer completion.
		`opencode_review_transport_relay_refused`, `refused.set(key, reason)`, `output.args.prompt = relayRefusedPrompt(reason)`, `output.output = relayRefusedOutput(refusal)`} {
		if !strings.Contains(source, want) {
			t.Fatalf("transport plugin missing %q", want)
		}
	}
	for _, forbidden := range []string{"GENTLE_AI_REVIEW_BINDING", "repository_context", "review lens-context", "capture-result", "preserve-result", "opencode_runtime_provenance", "JSON.parse(output.output)", "writeFile", "link(", "chmod("} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("transport plugin retains Go-owned behavior %q", forbidden)
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
		// Non-project guard: a fresh OpenCode directory can resolve to "/" or
		// another non-project location; the plugin must skip silently instead
		// of spawning a refresh that pollutes or fails at startup (#skill-registry-root-guard).
		"isProjectRoot",
		"homedir()",
		".git",
		".atl",
		"console.error",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("skill-registry.ts missing %q", want)
		}
	}
	// stdout belongs to OpenCode commands whose output gentle-ai parses
	// (`opencode models --verbose`); plugin logging must stay on stderr.
	for _, forbidden := range []string{"console.info", "console.log"} {
		if strings.Contains(src, forbidden) {
			t.Fatalf("skill-registry.ts must not log to stdout via %q", forbidden)
		}
	}
	if strings.Contains(src, "exec(") {
		t.Fatal("skill-registry.ts must use execFile, not shell exec")
	}
	if guardIdx, spawnIdx := strings.Index(src, "isProjectRoot"), strings.Index(src, "execFileAsync("); guardIdx == -1 || spawnIdx == -1 || guardIdx >= spawnIdx {
		t.Fatalf("skill-registry.ts must guard before spawning; isProjectRoot@%d execFileAsync(@%d", guardIdx, spawnIdx)
	}
	worktreeIdx := strings.Index(src, "input.worktree")
	directoryIdx := strings.Index(src, "input.directory")
	if worktreeIdx == -1 || directoryIdx == -1 {
		t.Fatal("skill-registry.ts must contain both input.worktree and input.directory")
	}
	if worktreeIdx >= directoryIdx {
		t.Errorf("skill-registry.ts must use input.worktree before input.directory; got worktree@%d >= directory@%d", worktreeIdx, directoryIdx)
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

	for _, name := range []string{"agents", "persona-gentleman.md", "orchestrator.md"} {
		if !seen[name] {
			t.Fatalf("claude embedded assets missing %q", name)
		}
	}
	// engram-protocol.md moved to the canonical engram/protocol.md asset
	// (design.md Decision 3) — it MUST NOT ship a stale duplicate under claude/.
	if seen["engram-protocol.md"] {
		t.Fatal("claude embedded assets must not ship a stale engram-protocol.md — content now lives in engram/protocol.md")
	}

	if seen["commands"] {
		t.Fatal("claude embedded assets still ship retired SDD commands")
	}

	agentEntries, err := FS.ReadDir("claude/agents")
	if err != nil {
		t.Fatalf("ReadDir(claude/agents) error = %v", err)
	}
	if len(agentEntries) != 8 {
		t.Fatalf("claude agents count = %d, want 8 retained review/Judgment Day agents", len(agentEntries))
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
		if !strings.Contains(md, "name: "+agent+"\n") || !strings.Contains(yaml, "system_prompt_path: ./"+agent+".md") {
			t.Fatalf("kimi review agent %s missing frontmatter name or YAML binding", agent)
		}
	}

	// OpenCode reviews use the retained transport plugin rather than an SDD
	// overlay. TestOpenCodeReviewTransportPluginContract pins that route.
}

func TestRetiredSDDLedgerGuidanceAbsentFromDistributedAssets(t *testing.T) {
	paths := []string{"skills/work-unit-commits/SKILL.md"}
	for _, lens := range []string{"risk", "readability", "reliability", "resilience"} {
		path := "kimi/agents/review-" + lens + ".md"
		paths = append(paths, path)
		content := MustRead(path)
		if !strings.HasPrefix(content, "---\nname: review-"+lens+"\n") ||
			!strings.Contains(content, "\nmodel: inherit\nreadonly: true\nbackground: false\n---\n") {
			t.Errorf("%s lost its native lens frontmatter", path)
		}
		if !strings.Contains(content, "native") {
			t.Errorf("%s does not describe native rendering", path)
		}
	}
	for _, path := range paths {
		content := strings.ToLower(MustRead(path))
		for _, retired := range []string{"sdd-tasks", "sdd relationship", "review ledger contract", "openspec/changes/", "sdd/{change-name}/review-ledger"} {
			if strings.Contains(content, retired) {
				t.Errorf("%s retains retired guidance %q", path, retired)
			}
		}
	}
	skill := MustRead("skills/work-unit-commits/SKILL.md")
	for _, retained := range []string{"## PR Relationship", "## ODD Relationship", "400", "Budget is not code-golf"} {
		if !strings.Contains(skill, retained) {
			t.Errorf("work-unit skill lost %q", retained)
		}
	}
}

func TestOpenCodeODDOrchestratorDoesNotOwnSessionPreflight(t *testing.T) {
	content := MustRead("opencode/orchestrator.md")
	for _, retired := range []string{
		"### SDD Session Preflight (HARD GATE)", "Before executing ANY SDD command or natural-language SDD request",
		"Use the `question` tool for SDD Session Preflight", "all four preflight groups in one single `question` tool call",
		"four localized groups in this order", "Review: 400 lines, 800 lines, Other",
		"Interactive -> `interactive`", "OpenSpec -> `openspec`", "Ask me -> `ask-on-risk`",
		"User-facing preflight question format:", "Map answers to canonical values", "A1", "A2", "B1", "C1", "D1",
	} {
		if strings.Contains(content, retired) {
			t.Fatalf("raw opencode/sdd-orchestrator.md still owns retired preflight content %q", retired)
		}
	}
}

func TestOpenCodeODDOrchestratorDelegationVisibility(t *testing.T) {
	content := MustRead("opencode/orchestrator.md")

	for _, required := range []string{
		"<!-- gentle-ai:opencode-desktop-delegation-progress -->",
		"#### Delegation Visibility (OpenCode Desktop)",
		"`delegate` or `task`",
		"assistant-visible status line immediately before the call",
		"When the call returns",
		"⏳ Delegating {phase} to {agent}...",
		"✅ {agent} completed — {status}",
		"⚠️ {agent} returned {status} — {short reason}",
		"15 tokens or fewer",
		"25 tokens or fewer",
		"executor prompts",
		"<!-- /gentle-ai:opencode-desktop-delegation-progress -->",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("opencode/sdd-orchestrator.md missing delegation visibility wording %q", required)
		}
	}

	if !strings.Contains(content, "#### Delegation Visibility (OpenCode Desktop)") {
		t.Fatal("OpenCode delegation visibility is missing")
	}
}

func TestOpenCodeODDOrchestratorPreflightDoesNotUseVisibleCodesOrCanonicalUIValues(t *testing.T) {
	content := MustRead("opencode/orchestrator.md")
	if start := strings.Index(content, "User-facing preflight question format:"); start >= 0 {
		t.Fatalf("raw opencode/sdd-orchestrator.md still owns preflight UI at %d", start)
	}
	if end := strings.Index(content, "Map answers to canonical values"); end >= 0 {
		t.Fatalf("raw opencode/sdd-orchestrator.md still owns preflight mappings at %d", end)
	}
	for _, forbidden := range []string{"A1", "A2", "B1", "C1", "D1", "Interactive -> `interactive`", "OpenSpec -> `openspec`", "Ask me -> `ask-on-risk`"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("raw opencode/sdd-orchestrator.md still owns preflight content %q", forbidden)
		}
	}
}

func TestPlatformNativeODDOrchestratorsAvoidOpenCodePersistenceClaims(t *testing.T) {
	tests := []struct {
		path     string
		required []string
	}{
		{path: "kimi/orchestrator.md", required: []string{"multiagent:Task", "bounded general worker"}},
		{path: "kiro/orchestrator.md", required: []string{"native subagents", "bounded ODD work"}},
		{path: "windsurf/orchestrator.md", required: []string{"solo-agent", "There are no sub-agents", "bounded ODD work"}},
		{path: "antigravity/orchestrator.md", required: []string{"define_subagent", "invoke_subagent", "bounded ODD work"}},
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

	for _, path := range allODDOrchestratorAssetPaths(t) {
		t.Run(path, func(t *testing.T) {
			if strings.Contains(MustRead(path), "haceme un SDD para X") {
				t.Fatalf("%s still contains a Spanish example that biases English sessions", path)
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

	// Only retained skills with embedded files count; deleted SDD directories
	// contain no embedded assets.
	if skillDirs != 16 {
		t.Fatalf("expected 16 retained skill directories, got %d", skillDirs)
	}

	// Verify each skill directory has a SKILL.md.
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if entry.Name() == "_shared" {
			for _, sharedFile := range []string{"README.md", "persistence-contract.md", "engram-convention.md", "odd-orchestrator-sections.md", "review-ledger-contract.md", "review-ledger-contract-pi.md", "research-lifecycle.md", "skill-resolver.md"} {
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

// TestCommandsDoNotUseEchoNPwd guards against the nested-subshell pattern
// `echo -n "$(pwd)"` (and the basename variant) in retained command assets.
// The Claude SDD command directory was removed; OpenCode commands remain.
func TestCommandsDoNotUseEchoNPwd(t *testing.T) {
	forbidden := `echo -n "$(pwd)"`

	for _, dir := range []string{"opencode/commands"} {
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

func TestODDOrchestratorsDoNotRequireRetiredRuntimeAttempts(t *testing.T) {
	for _, path := range allODDOrchestratorAssetPaths(t) {
		content := MustRead(path)
		for _, retired := range []string{"sdd-attempt acquire", "sdd-attempt settle", "Native Runtime Attempt Authority"} {
			if strings.Contains(content, retired) {
				t.Errorf("%s retains %q", path, retired)
			}
		}
	}
}

func TestODDOrchestratorsProjectNativeCheckingWithoutPromptOwnedLenses(t *testing.T) {
	for _, path := range allODDOrchestratorAssetPaths(t) {
		content := MustRead(path)
		section := markdownSection(content, "#### Native Checking Contract")
		if section == "" {
			t.Fatalf("%s missing Native Checking Contract", path)
		}
		for _, required := range []string{
			"Native RAR owns verification applicability",
			"bounded zero/one/four-lens plan",
			"never select lenses or author PASS",
			"passive ordinary document or image",
			"structural readback",
			"trivial passive documentation-only edit",
			"structural readback is the complete proportional check",
			"do not open a separate semantic-verification or heavy review ceremony",
			"applicable verifier is unavailable",
			"preserve the typed unavailable result",
			"never invent PASS, retry indefinitely, or escalate into extra ceremony",
			"quick check runs once",
			"Long or very-long work gets one cost/side-effect forecast",
			"Needs your decision",
			"Functional proof and adversarial review both project as **Checking**",
			"at most one scoped correction",
			"never reopen review for unchanged content",
		} {
			if !strings.Contains(section, required) {
				t.Fatalf("%s native checking contract missing %q", path, required)
			}
		}
		for _, retired := range []string{
			"Review Lens Selection", "review-risk", "review-readability",
			"review-reliability", "review-resilience", "loop-until-dry",
		} {
			if strings.Contains(content, retired) {
				t.Fatalf("%s retained prompt-owned review mechanism %q", path, retired)
			}
		}
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

func TestODDOrchestratorAssetsScopedToParent(t *testing.T) {
	for _, assetPath := range allODDOrchestratorAssetPaths(t) {
		t.Run(assetPath, func(t *testing.T) {
			content := MustRead(assetPath)
			if !strings.Contains(content, "Bind this to") || !strings.Contains(content, "Do NOT apply it to") {
				t.Fatalf("%s must scope its orchestrator instructions away from workers", assetPath)
			}
		})
	}
}
