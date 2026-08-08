package assets

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// reviewPluginHarness is a Node entry point that loads the embedded OpenCode
// review plugin exactly as OpenCode does and reports the message of whichever
// error the selected hook throws, or the resulting prompt/output. It exists
// so the plugin's transport behavior is proven by execution, not by reading
// the source for substrings.
//
// The harness never sets up a `client` argument beyond an empty object and
// never sets OPENCODE_DISABLE_PROJECT_CONFIG or OPENCODE_DISABLE_EXTERNAL_SKILLS:
// the reduced plugin is pure transport and depends on neither (SKILL.md: "No
// OpenCode restart, child isolation, special session, or OPENCODE_DISABLE_*
// variables. An ordinary running session is sufficient.").
const reviewPluginHarness = `import { readFile, writeFile } from "node:fs/promises"
import plugin from "./plugin.mts"

const scenario = process.argv[2]
const cwd = process.argv[3]
const hooks = await plugin({ directory: cwd, worktree: cwd })

const opaque = {
  lens: "review-risk", lineage: "trust-check", order: 0,
  repository_context: "rctx1_" + "a".repeat(64),
  revision: "sha256:" + "b".repeat(64),
  subject_hash: "sha256:" + "c".repeat(64),
  target: "sha256:" + "d".repeat(64),
}
const legacy = { lens: "review-risk", lineage: "trust-check", order: 0, target: "sha256:" + "d".repeat(64) }

const bindingPrompt = (binding: Record<string, unknown>) =>
  ` + "`" + `GENTLE_AI_REVIEW_BINDING ${JSON.stringify(binding)}\nreview the frozen candidate\n` + "`" + `

let prompt = bindingPrompt(opaque)
if (scenario === "before-legacy") prompt = bindingPrompt(legacy)
if (scenario === "before-missing") prompt = "review the frozen candidate\n"
if (scenario === "before-equals") prompt = ` + "`" + `GENTLE_AI_REVIEW_BINDING=${JSON.stringify(opaque)}\nreview the frozen candidate\n` + "`" + `
if (scenario === "before-malformed") prompt = "GENTLE_AI_REVIEW_BINDING {not-json}\nreview the frozen candidate\n"
if (scenario === "before-other-lens") prompt = bindingPrompt({ ...opaque, lens: "review-reliability" })

const runBefore = async (subagentType = "review-risk", background = false) => {
  const output: { args: Record<string, unknown> } = { args: { subagent_type: subagentType, prompt, background } }
  try {
    await hooks["tool.execute.before"]({ tool: "task", sessionID: "session-a", callID: "call-before" }, output)
    return String(output.args.prompt)
  } catch (cause: unknown) {
    return cause instanceof Error ? cause.message : String(cause)
  }
}

const runAfter = async (outputText: string) => {
  const input = { tool: "task", sessionID: "session-a", callID: "call-after", args: { subagent_type: "review-risk", prompt } }
  const output = { title: "", output: outputText, metadata: {} }
  try {
    await hooks["tool.execute.after"](input, output)
    return output.output
  } catch (cause: unknown) {
    return cause instanceof Error ? cause.message : String(cause)
  }
}

try {
  if (scenario.startsWith("sdd-")) {
    const phase = scenario.startsWith("sdd-profile-") ? "sdd-propose-cheap" : scenario === "sdd-unrelated" ? "sdd-custom" : "sdd-propose"
    const result = scenario.includes("malformed") ? "<task_result>broken" : scenario.includes("empty") ? "<task id=\"phase\" state=\"completed\">\n<task_result>\n\n</task_result>\n</task>" : ""
    const artifact = cwd + "/proposal.md"
    await writeFile(artifact, "existing artifact")
    const input = { tool: "task", sessionID: "sdd-session", callID: "call-sdd", args: { subagent_type: phase, prompt: "phase work" } }
    // The task tool's real result metadata carries the child route
    // ({parentSessionId, sessionId, model: {providerID, modelID}}); the
    // hostile shape proves path-like or dump-like values are omitted from
    // the handoff entirely, and the no-model shape proves absence is
    // tolerated rather than invented.
    const metadata = scenario.includes("hostile-model")
      ? { sessionId: "child", model: { providerID: "/home/user/secret-provider", modelID: "x".repeat(400) + " RegionError: request rejected" } }
      : scenario.includes("no-model")
        ? {}
        : { parentSessionId: "sdd-session", sessionId: "child", model: { providerID: "opencode-go", modelID: "deepseek-v4-flash" } }
    const output = { title: "", output: result, metadata }
    let failure = "NO_ERROR"
    try { await hooks["tool.execute.after"](input, output) } catch (cause: unknown) { failure = cause instanceof Error ? cause.message : String(cause) }
    if (scenario === "sdd-lifecycle") {
      let beforeDispose = "NO_ERROR"
      try { await hooks["tool.execute.before"]({ ...input, callID: "call-before-dispose" }, { args: { subagent_type: phase, prompt: "retry" } }) } catch (cause: unknown) { beforeDispose = cause instanceof Error ? cause.message : String(cause) }
      await hooks.dispose?.()
      let afterDispose = "NO_ERROR"
      try { await hooks["tool.execute.before"]({ ...input, callID: "call-after-dispose" }, { args: { subagent_type: phase, prompt: "reuse" } }) } catch (cause: unknown) { afterDispose = cause instanceof Error ? cause.message : String(cause) }
      console.log([failure, beforeDispose, afterDispose, await readFile(artifact, "utf8")].join("\n---\n"))
    } else {
      let downstream = "NOT_ATTEMPTED"
      if (scenario !== "sdd-unrelated") {
        try { await hooks["tool.execute.before"]({ ...input, callID: "call-next", args: { subagent_type: "sdd-apply", prompt: "downstream" } }, { args: { subagent_type: "sdd-apply", prompt: "downstream" } }) } catch (cause: unknown) { downstream = cause instanceof Error ? cause.message : String(cause) }
      }
      console.log([failure, downstream, await readFile(artifact, "utf8")].join("\n---\n"))
    }
  } else if (scenario === "before-background") {
    console.log(await runBefore("review-risk", true))
  } else if (scenario === "before-no-prompt") {
    const output: { args: Record<string, unknown> } = { args: { subagent_type: "review-risk" } }
    try {
      await hooks["tool.execute.before"]({ tool: "task", sessionID: "session-a", callID: "call-before" }, output)
      console.log("NO_ERROR")
    } catch (cause: unknown) {
      console.log(cause instanceof Error ? cause.message : String(cause))
    }
  } else if (scenario.startsWith("before")) {
    console.log(await runBefore())
  } else if (scenario === "after-empty") {
    console.log(await runAfter(""))
  } else if (scenario === "after-malformed") {
    console.log(await runAfter("<task id=\"x\" state=\"completed\">broken"))
  } else if (scenario === "after-empty-envelope") {
    console.log(await runAfter("<task id=\"x\" state=\"completed\">\n<task_result>\n\n</task_result>\n</task>"))
  } else if (scenario === "after-nested") {
    console.log(await runAfter("<task id=\"x\" state=\"completed\">\n<task_result>\n<task id=\"y\" state=\"completed\">\n<task_result>\ninner\n</task_result>\n</task>\n</task_result>\n</task>"))
  } else if (scenario === "after-bare") {
    console.log(JSON.stringify(await runAfter("  ` + reviewPluginPayloadMarker + `  ")))
  } else if (scenario === "after-envelope") {
    console.log(await runAfter("<task id=\"x\" state=\"completed\">\n<task_result>\n` + reviewPluginPayloadMarker + `\n</task_result>\n</task>"))
  } else if (scenario === "after-unrelated") {
    const input = { tool: "task", sessionID: "session-a", callID: "call-after", args: { subagent_type: "review-risk", prompt: "no binding here" } }
    const output = { title: "", output: "untouched", metadata: {} }
    await hooks["tool.execute.after"](input, output)
    console.log(output.output)
  }
} catch (cause: unknown) {
  console.log(cause instanceof Error ? cause.message : String(cause))
}
`

// reviewPluginPayloadMarker is a token that appears only inside the simulated
// reviewer payload, so an output that contains it can only have carried that
// payload through unmodified.
const reviewPluginPayloadMarker = "MARKER-PAYLOAD-9f3a"

// reviewPluginNativeTrustFailure is the failure surface the native CLI emits
// when Git refuses the bound repository for ownership reasons.
const reviewPluginNativeTrustFailure = "git_repository_untrusted: provider-issued review repository context operation failed; " +
	"Git declined to open the bound repository in this process because it is owned by a different account; " +
	"gentle-ai never provisions a safe.directory exception and never bypasses that protection. " +
	"Restart the host process under a Git context that already trusts that repository, then retry the same exact binding"

// reviewPluginStub configures the fake `gentle-ai` binary the harness runs
// against. lensContext feeds the one native call the reduced plugin still
// makes (`review lens-context`); stderr is the generic always-fail fallback
// used by every scenario that needs a failing native binary. argvLog, when
// set, records the exact argv of every invocation, one line per call --
// proof of exactly which native command ran, and with which flags.
type reviewPluginStub struct {
	stderr      string
	lensContext string
	argvLog     string
}

func runReviewPluginScenario(t *testing.T, scenario, nativeStderr string) string {
	t.Helper()
	return runReviewPluginScenarioStub(t, scenario, reviewPluginStub{stderr: nativeStderr})
}

func runReviewPluginScenarioStub(t *testing.T, scenario string, stub reviewPluginStub) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stub native binary requires a POSIX shell")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	source, err := Read("opencode/plugins/review-result-artifacts.ts")
	if err != nil {
		t.Fatalf("Read(review-result-artifacts.ts) error = %v", err)
	}
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	workDir := filepath.Join(root, "work")
	for _, dir := range []string{binDir, workDir} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	// The stub answers `review lens-context` with one finished provider block,
	// exactly like the real native command does for the frozen trees. It is
	// the ONLY native subcommand the reduced plugin ever invokes: there is no
	// capture-result or preserve-result branch left to stub, because the
	// plugin no longer calls either -- it hands the model's raw final text
	// back to its caller instead.
	stubScript := "#!/bin/sh\n" +
		"cat >/dev/null\n" +
		"if [ -n \"$GENTLE_AI_STUB_ARGV_LOG\" ]; then printf '%s\\n' \"$*\" >> \"$GENTLE_AI_STUB_ARGV_LOG\"; fi\n" +
		"if [ \"$2\" = \"lens-context\" ] && [ -n \"$GENTLE_AI_STUB_LENS_CONTEXT\" ]; then\n" +
		"  printf '%s\\n' \"$GENTLE_AI_STUB_LENS_CONTEXT\"\n" +
		"  exit 0\n" +
		"fi\n" +
		"printf '%s\\n' \"$GENTLE_AI_STUB_STDERR\" >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(binDir, "gentle-ai"), []byte(stubScript), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "plugin.mts"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "harness.mts"), []byte(reviewPluginHarness), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(node, "harness.mts", scenario, workDir)
	command.Dir = root
	// Strip any pre-existing OPENCODE_DISABLE_* from the inherited environment:
	// this suite proves the reduced plugin depends on neither, so an ambient
	// value the developer's own shell happens to carry must never mask that.
	base := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "OPENCODE_DISABLE_PROJECT_CONFIG=") || strings.HasPrefix(entry, "OPENCODE_DISABLE_EXTERNAL_SKILLS=") {
			continue
		}
		base = append(base, entry)
	}
	command.Env = append(base,
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GENTLE_AI_STUB_STDERR="+stub.stderr,
		"GENTLE_AI_STUB_LENS_CONTEXT="+stub.lensContext,
		"GENTLE_AI_STUB_ARGV_LOG="+stub.argvLog,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		// node was already confirmed present above: a non-zero exit here
		// means the plugin or harness itself failed (for example a mutant
		// that broke it at parse time), never an environment problem. That
		// must fail the test, not skip it -- a skip would let a
		// parse-breaking mutant through undetected.
		t.Fatalf("TypeScript plugin harness failed (%v): %s", err, output)
	}
	return strings.TrimSpace(string(output))
}

func TestReviewPluginRejectsMissingOrMalformedBindingBeforeReviewerLaunch(t *testing.T) {
	// The reduced plugin no longer field-validates a binding (#2442's
	// resolution under the shared contract: the binding is opaque provider
	// data the plugin passes through, never interprets). Every shape that
	// yields no usable repository-context handle collapses into the one
	// generic pre-launch refusal below, and none of them may reach the
	// native binary -- proven by an always-failing stub that would otherwise
	// leak its stderr into the message.
	for _, scenario := range []string{"before-missing", "before-equals", "before-malformed"} {
		t.Run(scenario, func(t *testing.T) {
			message := runReviewPluginScenarioStub(t, scenario, reviewPluginStub{stderr: "NATIVE-LENS-CONTEXT-MUST-NOT-RUN"})
			if !strings.Contains(message, "requires a repository-context binding") {
				t.Fatalf("invalid binding did not refuse closed: %s", message)
			}
			if strings.Contains(message, "NATIVE-LENS-CONTEXT-MUST-NOT-RUN") {
				t.Fatalf("invalid binding reached the native binary: %s", message)
			}
			if !strings.Contains(message, "The reviewer was not launched") {
				t.Fatalf("invalid-binding refusal lost its exactly-once guarantee: %s", message)
			}
		})
	}
}

// sddEmptySummaryFragment is the truthful empty-result summary (#2677): an
// empty result means the child task produced no output at all -- observed
// when the provider rejects the request before generation -- and the summary
// must say so instead of claiming the child "returned no valid task result".
const sddEmptySummaryFragment = "produced no task output at all"

// sddEmptyCauseFragment names the likeliest cause class for a child that
// produced nothing, so the operator is pointed at the provider boundary
// rather than at the phase's artifact contract.
const sddEmptyCauseFragment = "provider rejected the request before generation (authentication, region, or model access)"

func TestSDDTaskResultFailuresAreTerminalAndScoped(t *testing.T) {
	tests := []struct {
		name        string
		scenario    string
		wantCode    string
		wantPhase   string
		wantBlocked bool
		wantSummary string
		wantModel   string
		forbid      []string
	}{
		{
			name: "empty unsuffixed phase", scenario: "sdd-empty",
			wantCode: "sdd_task_result_empty", wantPhase: "sdd-propose", wantBlocked: true,
			wantSummary: sddEmptySummaryFragment, wantModel: "opencode-go/deepseek-v4-flash",
		},
		{
			name: "empty phase without model metadata", scenario: "sdd-empty-no-model",
			wantCode: "sdd_task_result_empty", wantPhase: "sdd-propose", wantBlocked: true,
			wantSummary: sddEmptySummaryFragment,
		},
		{
			name: "empty phase with hostile model metadata", scenario: "sdd-empty-hostile-model",
			wantCode: "sdd_task_result_empty", wantPhase: "sdd-propose", wantBlocked: true,
			wantSummary: sddEmptySummaryFragment,
			forbid:      []string{"/home/user/secret-provider", "RegionError", "xxxxxxxxxx"},
		},
		{
			name: "malformed profile-suffixed phase", scenario: "sdd-profile-malformed",
			wantCode: "sdd_task_result_malformed", wantPhase: "sdd-propose-cheap", wantBlocked: true,
			wantSummary: "returned no valid task result", wantModel: "opencode-go/deepseek-v4-flash",
			forbid: []string{sddEmptySummaryFragment},
		},
		{name: "unrelated sdd-prefixed agent", scenario: "sdd-unrelated", wantCode: "NO_ERROR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := strings.Split(runReviewPluginScenario(t, tt.scenario, "unused"), "\n---\n")
			if len(parts) != 3 {
				t.Fatalf("scenario output = %q", parts)
			}
			if !strings.Contains(parts[0], tt.wantCode) {
				t.Fatalf("failure = %q, want typed code %q", parts[0], tt.wantCode)
			}
			if tt.wantPhase != "" && !strings.Contains(parts[0], tt.wantPhase) {
				t.Fatalf("failure = %q, want phase %q", parts[0], tt.wantPhase)
			}
			if tt.wantBlocked && (!strings.Contains(parts[1], tt.wantCode) || !strings.Contains(parts[1], "Do not retry or advance SDD")) {
				t.Fatalf("downstream SDD launch was not terminally blocked: %q", parts[1])
			}
			if tt.wantBlocked {
				assertSDDTaskResultHandoff(t, parts[0], tt.wantCode, tt.wantPhase, tt.wantSummary, tt.wantModel)
				assertSDDTaskResultHandoff(t, parts[1], tt.wantCode, tt.wantPhase, tt.wantSummary, tt.wantModel)
			}
			for _, leaked := range tt.forbid {
				if strings.Contains(parts[0], leaked) || strings.Contains(parts[1], leaked) {
					t.Fatalf("failure handoff leaked %q: %q", leaked, parts[:2])
				}
			}
			if !tt.wantBlocked && parts[1] != "NOT_ATTEMPTED" {
				t.Fatalf("unrelated agent unexpectedly entered SDD routing: %q", parts[1])
			}
			if parts[2] != "existing artifact" {
				t.Fatalf("task-result handling mutated the existing artifact: %q", parts[2])
			}
		})
	}
}

func assertSDDTaskResultHandoff(t *testing.T, message, code, phase, summary, model string) {
	t.Helper()
	const prefix = "GENTLE_AI_SDD_FAILURE "
	if !strings.HasPrefix(message, prefix) {
		t.Fatalf("failure lacks a machine-readable handoff: %q", message)
	}
	raw := strings.TrimPrefix(message, prefix)
	var handoff struct {
		SchemaName   string `json:"schemaName"`
		Status       string `json:"status"`
		Code         string `json:"code"`
		Phase        string `json:"phase"`
		TaskModel    string `json:"taskModel"`
		Summary      string `json:"summary"`
		Continuation string `json:"continuation"`
	}
	if err := json.Unmarshal([]byte(raw), &handoff); err != nil {
		t.Fatalf("failure handoff is not JSON: %v: %q", err, message)
	}
	if handoff.SchemaName != "gentle-ai.sdd-task-result-failure/v1" || handoff.Status != "blocked" || handoff.Code != code || handoff.Phase != phase {
		t.Fatalf("failure handoff = %#v", handoff)
	}
	if !strings.Contains(handoff.Summary, summary) {
		t.Fatalf("failure handoff summary = %q, want it to contain %q", handoff.Summary, summary)
	}
	if code == "sdd_task_result_empty" && !strings.Contains(handoff.Summary, sddEmptyCauseFragment) {
		t.Fatalf("empty-result summary does not name the likeliest cause class: %q", handoff.Summary)
	}
	if handoff.TaskModel != model {
		t.Fatalf("failure handoff taskModel = %q, want %q", handoff.TaskModel, model)
	}
	// An absent route is omitted, never invented: the key itself must not
	// appear when no valid provider/model identity was available.
	if model == "" && strings.Contains(raw, `"taskModel"`) {
		t.Fatalf("failure handoff carries an empty taskModel field: %q", raw)
	}
	if !strings.HasPrefix(handoff.Continuation, "gentle-ai sdd-status --cwd '") || !strings.HasSuffix(handoff.Continuation, "' --json") {
		t.Fatalf("failure handoff names no runnable sdd-status continuation: %#v", handoff)
	}
}

func TestReviewPluginDisposeClearsSDDSessionFailure(t *testing.T) {
	parts := strings.Split(runReviewPluginScenario(t, "sdd-lifecycle", "unused"), "\n---\n")
	if len(parts) != 4 {
		t.Fatalf("SDD lifecycle outcomes = %q", parts)
	}
	if !strings.Contains(parts[0], "sdd_task_result_empty") || !strings.Contains(parts[1], "sdd_task_result_empty") {
		t.Fatalf("SDD failure was not retained before dispose: %q", parts[:2])
	}
	if parts[2] != "NO_ERROR" {
		t.Fatalf("dispose retained a failed SDD session for its reused ID: %q", parts[2])
	}
	if parts[3] != "existing artifact" {
		t.Fatalf("SDD lifecycle handling mutated the existing artifact: %q", parts[3])
	}
}

func reviewPluginLensContextBlock(paths ...string) string {
	if len(paths) == 0 {
		paths = []string{"internal/example.go"}
	}
	patches := make([]string, len(paths))
	for index, path := range paths {
		order := strconv.Itoa(index)
		patches[index] = "GENTLE_AI_REVIEW_PATCH " + order + " " + path + "\nPATCH-BODY-" + order + "\nGENTLE_AI_REVIEW_PATCH_END"
	}
	return strings.Join(append([]string{
		`GENTLE_AI_REVIEW_BINDING {"lineage":"trust-check","target":"sha256:` + strings.Repeat("d", 64) +
			`","lens":"review-risk","order":0,"revision":"sha256:` + strings.Repeat("b", 64) +
			`","repository_context":"rctx1_` + strings.Repeat("a", 64) +
			`","subject_hash":"sha256:` + strings.Repeat("c", 64) + `"}`,
		"GENTLE_AI_REVIEW_CONTEXT one finished provider-owned reviewer context block",
		"GENTLE_AI_REVIEW_INSTRUCTION\nYou are the R1 Risk lens of one bounded Gentle AI review.\nGENTLE_AI_REVIEW_INSTRUCTION_END",
	}, append(patches, "GENTLE_AI_REVIEW_CONTEXT_END")...), "\n")
}

// TestReviewPluginInjectsProviderOwnedLensContextWithoutIsolationEnvironment
// is the ordinary-session adapter conformance proof: the plugin injects the
// provider's finished lens context, wholesale replacing the caller-authored
// prompt, in a normal already-running session with no OPENCODE_DISABLE_*
// variable set at all (runReviewPluginScenarioStub strips any ambient value).
func TestReviewPluginInjectsProviderOwnedLensContextWithoutIsolationEnvironment(t *testing.T) {
	block := reviewPluginLensContextBlock("internal/example.go")
	prompt := runReviewPluginScenarioStub(t, "before-valid", reviewPluginStub{lensContext: block})
	if strings.TrimSpace(prompt) != strings.TrimSpace(block) {
		t.Fatalf("injected prompt is not the provider block verbatim:\ngot:  %q\nwant: %q", prompt, block)
	}
	if strings.Contains(prompt, "review the frozen candidate") {
		t.Fatalf("provider injection retained caller-authored prose: %q", prompt)
	}
}

// TestReviewPluginRoutesLensFromLaunchedSubagentNotBinding proves the plugin
// interprets no binding field: the native call's --lens argument is always
// the launched subagent_type, even when the binding's own "lens" field
// claims something else. This is the concrete proof of "the binding is
// opaque provider data the plugin passes through, never interprets."
func TestReviewPluginRoutesLensFromLaunchedSubagentNotBinding(t *testing.T) {
	argvLog := filepath.Join(t.TempDir(), "argv.log")
	runReviewPluginScenarioStub(t, "before-other-lens", reviewPluginStub{
		lensContext: reviewPluginLensContextBlock("internal/example.go"),
		argvLog:     argvLog,
	})
	logged, err := os.ReadFile(argvLog)
	if err != nil {
		t.Fatalf("read argv log: %v", err)
	}
	if !strings.Contains(string(logged), "--lens review-risk") {
		t.Fatalf("native call did not route by the launched subagent: %s", logged)
	}
	if strings.Contains(string(logged), "review-reliability") {
		t.Fatalf("native call was routed by the binding's own claimed lens: %s", logged)
	}
}

// TestReviewPluginRefusesUnterminatedLensContext proves a truncated provider
// block never reaches a reviewer.
func TestReviewPluginRefusesUnterminatedLensContext(t *testing.T) {
	block := reviewPluginLensContextBlock("internal/example.go")
	truncated := strings.TrimSuffix(strings.TrimSpace(block), "\nGENTLE_AI_REVIEW_CONTEXT_END")
	if truncated == strings.TrimSpace(block) {
		t.Fatal("test fixture did not remove the terminator")
	}
	message := runReviewPluginScenarioStub(t, "before-valid", reviewPluginStub{lensContext: truncated})
	if !strings.Contains(message, "GENTLE_AI_REVIEW_CONTEXT_END") || !strings.Contains(message, "partial provider context is never injected") {
		t.Fatalf("unterminated provider context was injected anyway: %s", message)
	}
	if !strings.Contains(message, "The reviewer was not launched") {
		t.Fatalf("terminator refusal lost its exactly-once guarantee: %s", message)
	}
}

// TestReviewPluginRequiresRepositoryContextForLensContext proves the
// injection guard: a binding without repository_context (the legacy shape)
// can never reach `review lens-context` at all, because that command accepts
// only the opaque provider-issued handle and has no --cwd fallback.
func TestReviewPluginRequiresRepositoryContextForLensContext(t *testing.T) {
	message := runReviewPluginScenarioStub(t, "before-legacy", reviewPluginStub{
		stderr: "NATIVE-LENS-CONTEXT-MUST-NOT-RUN",
	})
	if !strings.Contains(message, "requires a repository-context binding") {
		t.Fatalf("legacy binding did not refuse the provider injection closed: %s", message)
	}
	if strings.Contains(message, "NATIVE-LENS-CONTEXT-MUST-NOT-RUN") {
		t.Fatalf("legacy binding attempted a native lens-context call: %s", message)
	}
}

func TestReviewPluginRejectsTaskWithoutPromptField(t *testing.T) {
	message := runReviewPluginScenarioStub(t, "before-no-prompt", reviewPluginStub{
		stderr: "NATIVE-LENS-CONTEXT-MUST-NOT-RUN",
	})
	if message != "review task is missing GENTLE_AI_REVIEW_BINDING" {
		t.Fatalf("missing prompt field message = %q", message)
	}
}

func TestReviewPluginRefusesBackgroundReviewTask(t *testing.T) {
	message := runReviewPluginScenarioStub(t, "before-background", reviewPluginStub{
		stderr: "NATIVE-LENS-CONTEXT-MUST-NOT-RUN",
	})
	if !strings.Contains(message, "foreground") {
		t.Fatalf("background review task was not refused: %s", message)
	}
	if strings.Contains(message, "NATIVE-LENS-CONTEXT-MUST-NOT-RUN") {
		t.Fatalf("background review task reached the native binary: %s", message)
	}
}

// TestReviewPluginForwardsTypedLensContextRefusals pins that a provider
// refusal keeps its named exit. The native command's refusals are typed and
// path-free by construction, and the two that a caller cannot recover from by
// retrying -- an over-budget candidate and an empty patch for a
// content-changing path -- each name a real continuation.
//
// Only the [a-z_]+ code token crosses the boundary; the native prose that
// carries it never reaches the session transcript.
func TestReviewPluginForwardsTypedLensContextRefusals(t *testing.T) {
	for _, test := range []struct {
		name       string
		native     string
		wantCode   string
		wantAction string
	}{
		{
			name:       "budget exceeded",
			native:     "lens_context_budget_exceeded: provider-owned reviewer lens context was not produced; NATIVE-ACTION-PROSE",
			wantCode:   "lens_context_budget_exceeded",
			wantAction: "Split this candidate into smaller reviewable commits",
		},
		{
			name:       "empty patch",
			native:     "lens_context_empty_patch: provider-owned reviewer lens context was not produced; NATIVE-ACTION-PROSE",
			wantCode:   "lens_context_empty_patch",
			wantAction: "stop relaunching",
		},
		{
			name:       "emission conflict",
			native:     "lens_context_emission_conflict: provider-owned reviewer lens context was not produced; NATIVE-ACTION-PROSE",
			wantCode:   "lens_context_emission_conflict",
			wantAction: "audit history is never rewritten",
		},
		{
			name:       "unrecognized code still names an exit",
			native:     "lens_context_inspection_failed: provider-owned reviewer lens context was not produced; NATIVE-ACTION-PROSE",
			wantCode:   "lens_context_inspection_failed",
			wantAction: "Refresh the exact native next_transition",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			message := runReviewPluginScenario(t, "before-valid", test.native)
			if !strings.Contains(message, test.wantCode) {
				t.Fatalf("refusal lost its typed code %q: %s", test.wantCode, message)
			}
			if !strings.Contains(message, test.wantAction) {
				t.Fatalf("refusal names no actionable continuation (%q): %s", test.wantAction, message)
			}
			if strings.Contains(message, "NATIVE-ACTION-PROSE") {
				t.Fatalf("refusal forwarded native prose into the transcript: %s", message)
			}
			if !strings.Contains(message, "The reviewer was not launched") {
				t.Fatalf("refusal lost its exactly-once guarantee: %s", message)
			}
		})
	}
}

// TestReviewPluginSurfacesNativeGitTrustRefusal proves the plugin never
// collapses a native Git trust refusal into "refresh the exact native
// next_transition", which cannot change the Git trust context of an
// already-running host process.
func TestReviewPluginSurfacesNativeGitTrustRefusal(t *testing.T) {
	message := runReviewPluginScenario(t, "before-valid", reviewPluginNativeTrustFailure)
	if message == "NO_ERROR" {
		t.Fatal("preflight did not fail despite an always-failing native binary")
	}
	if !strings.Contains(message, "git_repository_untrusted") {
		t.Fatalf("plugin suppressed the native Git trust refusal: %s", message)
	}
	if strings.Contains(message, "next_transition") {
		t.Fatalf("plugin still advises refreshing the transition for a Git trust refusal: %s", message)
	}
	if !strings.Contains(message, "Restart the host process") {
		t.Fatalf("plugin carries no instruction the caller can carry out: %s", message)
	}
	if !strings.Contains(message, "The reviewer was not launched") {
		t.Fatalf("plugin lost its pre-launch exactly-once guarantee: %s", message)
	}
}

// TestReviewPluginKeepsGenericOpaqueFailureOpaque proves an unstructured
// native preflight failure is scrubbed, not forwarded verbatim, since native
// failures can embed local paths.
func TestReviewPluginKeepsGenericOpaqueFailureOpaque(t *testing.T) {
	leak := "repository_context_unavailable: provider-issued review repository context operation failed; " +
		"failed under /home/someone/private/repo"
	message := runReviewPluginScenario(t, "before-valid", leak)
	if strings.Contains(message, "/home/someone/private/repo") {
		t.Fatalf("plugin forwarded a native path through an opaque binding: %s", message)
	}
	if !strings.Contains(message, "review lens context failed") {
		t.Fatalf("generic opaque failure lost its transport framing: %s", message)
	}
}

// TestReviewPluginReturnsRawReviewerTextWithoutNativeCapture is the
// raw-output-to-native-admission proof: the plugin unwraps the task envelope
// and hands the model's raw final text straight back. It calls no native
// command at all to do this -- capture and admission are entirely native
// Go's job now, run by whatever launched the reviewer task using the exact
// capture operation the negotiated collect step named. argvLog stays empty,
// proving zero native invocations happened in the after-hook.
func TestReviewPluginReturnsRawReviewerTextWithoutNativeCapture(t *testing.T) {
	argvLog := filepath.Join(t.TempDir(), "argv.log")
	output := runReviewPluginScenarioStub(t, "after-envelope", reviewPluginStub{argvLog: argvLog})
	if output != reviewPluginPayloadMarker {
		t.Fatalf("raw reviewer text was not returned verbatim: %q", output)
	}
	if logged, err := os.ReadFile(argvLog); err == nil && len(strings.TrimSpace(string(logged))) > 0 {
		t.Fatalf("plugin invoked a native command while relaying a completed reviewer result: %q", logged)
	}
}

func TestReviewPluginUnwrapsBareOutputWithoutEnvelope(t *testing.T) {
	var got string
	if err := json.Unmarshal([]byte(runReviewPluginScenarioStub(t, "after-bare", reviewPluginStub{})), &got); err != nil {
		t.Fatalf("decode harness output: %v", err)
	}
	if got != reviewPluginPayloadMarker {
		t.Fatalf("bare model output was not passed through trimmed: %q", got)
	}
}

func TestReviewPluginRejectsEmptyReviewerOutput(t *testing.T) {
	message := runReviewPluginScenarioStub(t, "after-empty", reviewPluginStub{})
	if message != "reviewer output must not be empty" {
		t.Fatalf("empty reviewer output message = %q", message)
	}
}

func TestReviewPluginRejectsMalformedTaskEnvelope(t *testing.T) {
	message := runReviewPluginScenarioStub(t, "after-malformed", reviewPluginStub{})
	if message != "reviewer output contains a malformed task result envelope" {
		t.Fatalf("malformed envelope message = %q", message)
	}
}

func TestReviewPluginRejectsEmptyTaskResult(t *testing.T) {
	message := runReviewPluginScenarioStub(t, "after-empty-envelope", reviewPluginStub{})
	if message != "reviewer task result is empty" {
		t.Fatalf("empty task result message = %q", message)
	}
}

func TestReviewPluginRejectsNestedTaskEnvelope(t *testing.T) {
	message := runReviewPluginScenarioStub(t, "after-nested", reviewPluginStub{})
	if message != "reviewer task result contains a nested task envelope" {
		t.Fatalf("nested envelope message = %q", message)
	}
}

// TestReviewPluginIgnoresUnboundTaskOutput proves the after-hook is a no-op
// for a review-agent task whose prompt never carried a binding in the first
// place (for example, one the before-hook already refused): output.output
// must stay exactly as the caller set it.
func TestReviewPluginIgnoresUnboundTaskOutput(t *testing.T) {
	output := runReviewPluginScenarioStub(t, "after-unrelated", reviewPluginStub{})
	if output != "untouched" {
		t.Fatalf("after-hook mutated output for an unbound task: %q", output)
	}
}
