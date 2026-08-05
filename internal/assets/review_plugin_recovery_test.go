package assets

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// reviewPluginHarness is a Node entry point that loads the embedded OpenCode
// review plugin exactly as OpenCode does and reports the message of whichever
// error the selected hook throws. It exists so the plugin's recovery paths are
// proven by execution, not by reading the source for substrings.
const reviewPluginHarness = `import plugin from "./plugin.mts"

const scenario = process.argv[2]
const cwd = process.argv[3]
const configInstructions = JSON.parse(process.env.GENTLE_AI_STUB_CONFIG_INSTRUCTIONS || "[]")
const configError = process.env.GENTLE_AI_STUB_CONFIG_ERROR || ""
const client = {
  config: {
    get: async () => {
      if (configError) throw new Error(configError)
      return { data: { instructions: configInstructions } }
    },
  },
}
const hooks = await plugin({ client, directory: cwd, worktree: cwd })

const opaque = {
  lens: "review-risk", lineage: "trust-check", order: 0,
  repository_context: "rctx1_" + "a".repeat(64),
  revision: "sha256:" + "b".repeat(64),
  subject_hash: "sha256:" + "c".repeat(64),
  target: "sha256:" + "d".repeat(64),
}
const legacy = { lens: "review-risk", lineage: "trust-check", order: 0, target: "sha256:" + "d".repeat(64) }
const binding = scenario.endsWith("legacy") ? legacy : opaque
let prompt = ` + "`" + `GENTLE_AI_REVIEW_BINDING ${JSON.stringify(binding)}\nreview the frozen candidate\n` + "`" + `
if (scenario === "before-substitute") prompt += ` + "`" + `base_tree=${"9".repeat(40)} candidate_tree=${"8".repeat(40)} changed_path_manifest=[{"path":"caller.txt"}]\n` + "`" + `
if (scenario === "before-missing") prompt = "review the frozen candidate\n"
if (scenario === "before-equals") prompt = ` + "`" + `GENTLE_AI_REVIEW_BINDING=${JSON.stringify(binding)}\nreview the frozen candidate\n` + "`" + `
if (scenario === "before-malformed") prompt = "GENTLE_AI_REVIEW_BINDING {not-json}\nreview the frozen candidate\n"
const rebind = (overrides: Record<string, unknown>, drop: string[] = []) => {
  const next: Record<string, unknown> = { ...opaque, ...overrides }
  for (const field of drop) delete next[field]
  return ` + "`" + `GENTLE_AI_REVIEW_BINDING ${JSON.stringify(next)}\nreview the frozen candidate\n` + "`" + `
}
if (scenario === "before-quoted-order") prompt = rebind({ order: "0" })
if (scenario === "before-negative-order") prompt = rebind({ order: -1 })
if (scenario === "before-unknown-shape") prompt = rebind({}, ["repository_context"])
if (scenario === "before-short-target") prompt = rebind({ target: "sha256:dead" })
if (scenario === "before-other-lens") prompt = rebind({ lens: "review-reliability" })
if (scenario === "before-bad-context") prompt = rebind({ repository_context: "/home/someone/private/repo" })

const capture = async (activeHooks: typeof hooks, sessionID: string, marker: string, boundPrompt: string = prompt) => {
  const input = { tool: "task", sessionID, callID: "call-" + marker, args: { subagent_type: "review-risk", prompt: boundPrompt } }
  const incomplete = '{"subject_hash":"sha256:' + "c".repeat(64) + '","inspection":{"status":"incomplete","paths":[]},"findings":[],"evidence":["` + reviewPluginPayloadMarker + `"]}'
  const output = { title: "", output: marker === "incomplete" ? incomplete : '{"subject_hash":"sha256:' + "c".repeat(64) + '","findings":[],"evidence":["' + marker + '"]}', metadata: {} }
  try {
    await activeHooks["tool.execute.after"](input, output)
    return output.output
  } catch (cause: unknown) {
    return cause instanceof Error ? cause.message : String(cause)
  }
}

try {
  if (scenario.startsWith("before")) {
    const output = { args: { subagent_type: "review-risk", prompt } }
    await hooks["tool.execute.before"]({ tool: "task", sessionID: "session-a", callID: "call-before" }, output)
    console.log(scenario === "before-valid" || scenario === "before-substitute" ? output.args.prompt : "NO_ERROR")
  } else if (scenario === "after-state") {
    const outcomes = [
      await capture(hooks, "session-a", "first"), await capture(hooks, "session-b", "other-session"),
      await capture(hooks, "session-a", "corrected"), await capture(hooks, "session-a", "after-terminal"),
    ]
    console.log(outcomes.join("\n---\n"))
  } else if (scenario === "after-success") {
    console.log([
      await capture(hooks, "session-a", "first"), await capture(hooks, "session-a", "capture-success"),
      await capture(hooks, "session-a", "after-success"),
    ].join("\n---\n"))
  } else if (scenario === "after-lifecycle") {
    const outcomes = [await capture(hooks, "session-delete", "before-delete")]
    await hooks.event?.({ event: { type: "session.deleted", properties: { info: { id: "session-delete" } } } })
    outcomes.push(await capture(hooks, "session-delete", "after-delete"), await capture(hooks, "session-dispose", "before-dispose"))
    await hooks.dispose?.()
    outcomes.push(await capture(hooks, "session-dispose", "after-dispose"))
    console.log(outcomes.join("\n---\n"))
  } else if (scenario === "after-no-lifecycle") {
    let perSessionAllowed = 0
    let lastSession = ""
    for (let index = 0; index <= 8; index++) {
      const variedPrompt = ` + "`" + `GENTLE_AI_REVIEW_BINDING ${JSON.stringify({ ...opaque, order: index })}\nreview\n` + "`" + `
      lastSession = await capture(hooks, "session-full", "no-lifecycle", variedPrompt)
      if (lastSession.includes("exactly once")) perSessionAllowed++
    }
    const globalHooks = await plugin({ client, directory: cwd, worktree: cwd })
    let globalAllowed = 0
    let lastGlobal = ""
    for (let index = 0; index <= 64; index++) {
      lastGlobal = await capture(globalHooks, ` + "`" + `session-${index}` + "`" + `, "no-lifecycle")
      if (lastGlobal.includes("exactly once")) globalAllowed++
    }
    console.log(` + "`" + `${perSessionAllowed},${globalAllowed}\n---\n${lastSession}\n---\n${lastGlobal}` + "`" + `)
  } else {
    console.log(await capture(hooks, "session-a", scenario === "after-incomplete" ? "incomplete" : "` + reviewPluginPayloadMarker + `"))
  }
} catch (cause: unknown) {
  console.log(cause instanceof Error ? cause.message : String(cause))
}
`

// reviewPluginPayloadMarker is a token that appears only inside the simulated
// reviewer payload, so a message that contains it can only have embedded that
// payload.
const reviewPluginPayloadMarker = "MARKER-PAYLOAD-9f3a"

// reviewPluginNativeTrustFailure is the failure surface the native CLI now
// emits when Git refuses the bound repository for ownership reasons. It is
// exactly `reviewGitTrustRefusalCode: ...; reviewGitTrustRefusalAction` from
// internal/cli/review_incident.go.
const reviewPluginNativeTrustFailure = "git_repository_untrusted: provider-issued review repository context operation failed; " +
	"Git declined to open the bound repository in this process because it is owned by a different account; " +
	"gentle-ai never provisions a safe.directory exception and never bypasses that protection. " +
	"Restart the host process under a Git context that already trusts that repository, then retry the same exact binding"

// runReviewPluginScenario executes one plugin hook against a stub `gentle-ai`
// that always fails with nativeStderr, and returns the thrown error message.
func runReviewPluginScenario(t *testing.T, scenario, nativeStderr string) string {
	return runReviewPluginScenarioWithNative(t, scenario, "", nativeStderr)
}

func runReviewPluginScenarioWithNative(t *testing.T, scenario, nativeStdout, nativeStderr string) string {
	return runReviewPluginScenarioWithNativeAndPreservation(t, scenario, nativeStdout, nativeStderr, "")
}

func runReviewPluginScenarioWithNativeAndPreservation(t *testing.T, scenario, nativeStdout, nativeStderr, preserveStdout string) string {
	return runReviewPluginScenarioStub(t, scenario, reviewPluginStub{
		stdout: nativeStdout, stderr: nativeStderr, preserveStdout: preserveStdout,
	})
}

// reviewPluginStub configures the fake `gentle-ai` binary the harness runs
// against. lensContext feeds the provider-injected reviewer-context path
// (`review lens-context`); stdout/stderr remain the generic fallback used by
// the capture/preserve-result scenarios and by every scenario that needs an
// always-failing native binary.
type reviewPluginStub struct {
	stdout         string
	stderr         string
	preserveStdout string
	lensContext    string
	// omitProjectConfigIsolation/omitExternalSkillsIsolation deliberately
	// leave OPENCODE_DISABLE_PROJECT_CONFIG/OPENCODE_DISABLE_EXTERNAL_SKILLS
	// unset. Every scenario is protected (both set to "1") by default, since
	// that is the shape a real launch must always have; only the dedicated
	// isolation-refusal tests opt out.
	omitProjectConfigIsolation  bool
	omitExternalSkillsIsolation bool
	// configInstructions/configError feed the harness's mock
	// `client.config.get()`. Every scenario resolves an empty instructions
	// array (no remote entries) by default; only the dedicated
	// remote-instructions tests set these.
	configInstructions []string
	configError        string
}

func runReviewPluginScenarioStub(t *testing.T, scenario string, stub reviewPluginStub) string {
	t.Helper()
	env := []string{
		"GENTLE_AI_STUB_LENS_CONTEXT=" + stub.lensContext,
		"GENTLE_AI_STUB_CONFIG_INSTRUCTIONS=" + marshalReviewPluginInstructions(t, stub.configInstructions),
		"GENTLE_AI_STUB_CONFIG_ERROR=" + stub.configError,
	}
	env = append(env, isolationEnvironment(stub)...)
	return runReviewPluginScenarioWithEnv(t, scenario, stub.stdout, stub.stderr, stub.preserveStdout,
		env...,
	)
}

func marshalReviewPluginInstructions(t *testing.T, instructions []string) string {
	t.Helper()
	if instructions == nil {
		instructions = []string{}
	}
	encoded, err := json.Marshal(instructions)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func isolationEnvironment(stub reviewPluginStub) []string {
	env := make([]string, 0, 2)
	if !stub.omitProjectConfigIsolation {
		env = append(env, "OPENCODE_DISABLE_PROJECT_CONFIG=1")
	}
	if !stub.omitExternalSkillsIsolation {
		env = append(env, "OPENCODE_DISABLE_EXTERNAL_SKILLS=1")
	}
	return env
}

// runReviewPluginScenarioWithEnv is the same harness with extra environment
// entries, so a scenario can prove what the plugin does with a hostile ambient
// environment rather than only with a hostile native binary.
func runReviewPluginScenarioWithEnv(t *testing.T, scenario, nativeStdout, nativeStderr, preserveStdout string, extraEnv ...string) string {
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
	// exactly like the real native command does for the frozen trees: the
	// whole reviewer context is one native call now, so the stub needs no
	// per-operation or per-path-index dispatch.
	stubScript := "#!/bin/sh\npayload=$(cat)\n" +
		"if [ -n \"$GENTLE_AI_STUB_CWD_LOG\" ]; then printf '%s\\n' \"$PWD\" >> \"$GENTLE_AI_STUB_CWD_LOG\"; fi\n" +
		"if [ \"$2\" = \"lens-context\" ] && [ -n \"$GENTLE_AI_STUB_LENS_CONTEXT\" ]; then\n" +
		"  printf '%s\\n' \"$GENTLE_AI_STUB_LENS_CONTEXT\"\n" +
		"  exit 0\n" +
		"fi\n" +
		"if [ \"$2\" = \"capture-result\" ]; then\n" +
		"  case \"$payload\" in *capture-success*) printf '%s\\n' 'CAPTURED'; exit 0;; esac\n" +
		"fi\n" +
		"if [ \"$2\" = \"preserve-result\" ] && [ -n \"$GENTLE_AI_STUB_PRESERVE_STDOUT\" ]; then printf '%s\\n' \"$GENTLE_AI_STUB_PRESERVE_STDOUT\"; exit 0; fi\n" +
		"if [ -n \"$GENTLE_AI_STUB_STDOUT\" ]; then printf '%s\\n' \"$GENTLE_AI_STUB_STDOUT\"; exit 0; fi\n" +
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
	// Strip any pre-existing isolation variables from the inherited
	// environment before setting them explicitly: exec.Cmd.Env does not
	// deduplicate repeated keys, and this test must control both variables
	// exactly, never inherit an ambient value that happens to match.
	base := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "OPENCODE_DISABLE_PROJECT_CONFIG=") || strings.HasPrefix(entry, "OPENCODE_DISABLE_EXTERNAL_SKILLS=") {
			continue
		}
		base = append(base, entry)
	}
	env := append(base,
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GENTLE_AI_STUB_STDOUT="+nativeStdout,
		"GENTLE_AI_STUB_STDERR="+nativeStderr,
		"GENTLE_AI_STUB_PRESERVE_STDOUT="+preserveStdout,
		"GENTLE_AI_REVIEW_CWD=",
		"GENTLE_AI_STUB_CWD_LOG=",
	)
	command.Env = append(env, extraEnv...)
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

func TestReviewPluginRejectsInvalidBindingBeforeReviewerLaunch(t *testing.T) {
	tests := []struct {
		name    string
		wantErr string
	}{
		{name: "missing", wantErr: "review task is missing GENTLE_AI_REVIEW_BINDING"},
		{name: "equals", wantErr: "review task is missing GENTLE_AI_REVIEW_BINDING"},
		{name: "malformed", wantErr: "review task binding is malformed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := runReviewPluginScenarioWithNative(t, "before-"+tt.name, `{"unexpected":"native call"}`, "")
			if message != tt.wantErr {
				t.Fatalf("invalid binding result = %q, want %q", message, tt.wantErr)
			}
		})
	}
}

// reviewPluginLensContextBinding is the exact provider-authored binding the
// harness's opaque task binding addresses. `gentle-ai review lens-context`
// emits this as its first line, and every field is provider-derived: the
// caller relays no machine value into it.
const reviewPluginLensContextBinding = `{"lineage":"trust-check","target":"sha256:` + reviewPluginTargetSuffix +
	`","lens":"review-risk","order":0,"revision":"sha256:` + reviewPluginRevisionSuffix +
	`","repository_context":"rctx1_` + reviewPluginHandleSuffix +
	`","subject_hash":"sha256:` + reviewPluginSubjectSuffix + `"}`

// These four suffixes are the exact opaque values the Node harness builds its
// task binding from. The provider block must address the same candidate, or
// the plugin refuses the injection.
const (
	reviewPluginHandleSuffix   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	reviewPluginRevisionSuffix = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	reviewPluginSubjectSuffix  = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	reviewPluginTargetSuffix   = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
)

func reviewPluginLensContext(baseTree, candidateTree string) string {
	return reviewPluginLensContextWithPaths(baseTree, candidateTree, []string{"internal/example.go"})
}

// reviewPluginLensContextWithPaths builds a block shaped exactly like the one
// `gentle-ai review lens-context` emits: the provider-authored binding line,
// the provider-authored capture context, the reviewer's charge and result
// schema, discovery, one patch section per canonical manifest index, and the
// terminator. The plugin injects these bytes verbatim; it assembles nothing.
func reviewPluginLensContextWithPaths(baseTree, candidateTree string, paths []string) string {
	entries := make([]string, len(paths))
	patches := make([]string, len(paths))
	for index, path := range paths {
		entries[index] = `{"path":"` + path + `","status":"M","old_mode":"100644","new_mode":"100644","deleted":false,"type_changed":false,"mode_only":false,"intended_untracked":false}`
		patches[index] = fmt.Sprintf("GENTLE_AI_REVIEW_PATCH %d %s\nPATCH-BODY-%d\nGENTLE_AI_REVIEW_PATCH_END", index, path, index)
	}
	context := `{"schema":"gentle-ai.review-capture-preflight/v1","capability":"review.native_capture_preflight",` +
		`"lineage_id":"trust-check","target_identity":"sha256:` + reviewPluginTargetSuffix + `","lens":"review-risk","selected_order":0,` +
		`"artifact_subject":{"schema":"gentle-ai.review-artifact-subject/v2","subject_hash":"sha256:` + reviewPluginSubjectSuffix + `",` +
		`"lineage_id":"trust-check","authority_revision":"sha256:` + reviewPluginRevisionSuffix + `","target_identity":"sha256:` + reviewPluginTargetSuffix + `",` +
		`"base_tree":"` + baseTree + `","candidate_tree":"` + candidateTree + `","changed_path_manifest_sha256":"sha256:` + strings.Repeat("e", 64) + `",` +
		`"lens":"review-risk","selected_order":0},"base_tree":"` + baseTree + `","candidate_tree":"` + candidateTree + `",` +
		`"changed_path_manifest":[` + strings.Join(entries, ",") + `]}`
	return strings.Join(append([]string{
		"GENTLE_AI_REVIEW_BINDING " + reviewPluginLensContextBinding,
		"GENTLE_AI_REVIEW_CONTEXT " + context,
		"GENTLE_AI_REVIEW_INSTRUCTION\nYou are the R1 Risk lens of one bounded Gentle AI review.\nGENTLE_AI_REVIEW_INSTRUCTION_END",
		"GENTLE_AI_REVIEW_RESULT_SCHEMA\n{}\nGENTLE_AI_REVIEW_RESULT_SCHEMA_END",
		"GENTLE_AI_REVIEW_NAME_STATUS\nM\t" + paths[0] + "\nGENTLE_AI_REVIEW_NAME_STATUS_END",
		"GENTLE_AI_REVIEW_NUMSTAT\n3\t1\t" + paths[0] + "\nGENTLE_AI_REVIEW_NUMSTAT_END",
	}, append(patches, "GENTLE_AI_REVIEW_CONTEXT_END")...), "\n")
}

// TestReviewPluginInjectsProviderOwnedLensContext pins the restored
// shell-less transport: the plugin asks the provider for the one finished
// reviewer lens context and injects those exact bytes into the reviewer's
// prompt before the reviewer ever launches. No shell and no read tool exist
// on the reviewer side, so this injected block is provably the only byte
// source of the prompt the reviewer's own turn receives.
func TestReviewPluginInjectsProviderOwnedLensContext(t *testing.T) {
	baseTree := strings.Repeat("1", 40)
	candidateTree := strings.Repeat("2", 40)
	block := reviewPluginLensContext(baseTree, candidateTree)
	prompt := runReviewPluginScenarioStub(t, "before-valid", reviewPluginStub{lensContext: block})
	if strings.TrimSpace(prompt) != strings.TrimSpace(block) {
		t.Fatalf("injected prompt is not the provider block verbatim:\ngot:  %q\nwant: %q", prompt, block)
	}
	if !strings.HasPrefix(prompt, "GENTLE_AI_REVIEW_BINDING {") {
		t.Fatalf("injected prompt does not begin with the exact binding prefix: %q", prompt)
	}
	for _, want := range []string{
		"GENTLE_AI_REVIEW_CONTEXT ", baseTree, candidateTree, "internal/example.go",
		`"subject_hash":"sha256:` + reviewPluginSubjectSuffix + `"`,
		"GENTLE_AI_REVIEW_NAME_STATUS\nM\tinternal/example.go\nGENTLE_AI_REVIEW_NAME_STATUS_END",
		"GENTLE_AI_REVIEW_NUMSTAT\n3\t1\tinternal/example.go\nGENTLE_AI_REVIEW_NUMSTAT_END",
		"GENTLE_AI_REVIEW_PATCH 0 internal/example.go\nPATCH-BODY-0\nGENTLE_AI_REVIEW_PATCH_END",
		"GENTLE_AI_REVIEW_CONTEXT_END",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("plugin omitted provider context %q: %q", want, prompt)
		}
	}
	if strings.Contains(prompt, "candidate_diff") {
		t.Fatalf("injected prompt contains obsolete candidate diff payload: %q", prompt)
	}
}

// TestReviewPluginOrdersEvidenceByManifestIndex proves multi-path evidence
// reaches the reviewer once per canonical manifest index, in exact order,
// each carrying its own index and literal path. The provider orders it; this
// test proves the plugin does not reorder, drop, or re-wrap it.
func TestReviewPluginOrdersEvidenceByManifestIndex(t *testing.T) {
	block := reviewPluginLensContextWithPaths(strings.Repeat("1", 40), strings.Repeat("2", 40), []string{"internal/a.go", "internal/b.go"})
	prompt := runReviewPluginScenarioStub(t, "before-valid", reviewPluginStub{lensContext: block})
	first := strings.Index(prompt, "GENTLE_AI_REVIEW_PATCH 0 internal/a.go")
	second := strings.Index(prompt, "GENTLE_AI_REVIEW_PATCH 1 internal/b.go")
	if first < 0 || second < 0 || first > second {
		t.Fatalf("path evidence is not in exact manifest order: %q", prompt)
	}
	if !strings.Contains(prompt, "PATCH-BODY-0") || !strings.Contains(prompt, "PATCH-BODY-1") {
		t.Fatalf("plugin omitted per-path patch content: %q", prompt)
	}
}

// TestReviewPluginReplacesCallerAuthoredCandidateContext proves the
// provider-injected prompt wholesale replaces caller-authored text: nothing
// the caller wrote (a stale tree, a fabricated manifest, or free prose)
// survives into the launched reviewer's prompt.
func TestReviewPluginReplacesCallerAuthoredCandidateContext(t *testing.T) {
	baseTree := strings.Repeat("1", 40)
	candidateTree := strings.Repeat("2", 40)
	prompt := runReviewPluginScenarioStub(t, "before-substitute", reviewPluginStub{
		lensContext: reviewPluginLensContext(baseTree, candidateTree),
	})
	for _, callerValue := range []string{strings.Repeat("9", 40), strings.Repeat("8", 40), "caller.txt", "review the frozen candidate"} {
		if strings.Contains(prompt, callerValue) {
			t.Fatalf("provider injection retained caller-authored context %q: %q", callerValue, prompt)
		}
	}
	for _, providerValue := range []string{baseTree, candidateTree, "internal/example.go", "PATCH-BODY-0"} {
		if !strings.Contains(prompt, providerValue) {
			t.Fatalf("provider injection omitted provider context %q: %q", providerValue, prompt)
		}
	}
}

// TestReviewPluginFailsClosedWhenLensContextFails is the mutation proof for
// the injection path: if the provider refuses or fails to produce the lens
// context, the reviewer must never launch with the caller-authored prompt as
// a fallback. A mutant that swallowed the failure and fell through to
// `output.args.prompt` unchanged would leave the caller's own text in the
// result; this test fails red against that mutant.
func TestReviewPluginFailsClosedWhenLensContextFails(t *testing.T) {
	message := runReviewPluginScenario(t, "before-valid", "NATIVE-LENS-CONTEXT-FAILED")
	if !strings.Contains(message, "review lens context failed") {
		t.Fatalf("lens-context failure did not stop the reviewer launch closed: %s", message)
	}
	if !strings.Contains(message, "The reviewer was not launched") {
		t.Fatalf("lens-context failure lost its exactly-once guarantee: %s", message)
	}
	if strings.Contains(message, "review the frozen candidate") {
		t.Fatalf("lens-context failure fell back to the caller-authored prompt: %s", message)
	}
}

// TestReviewPluginForwardsTypedLensContextRefusals pins that a provider
// refusal keeps its named exit. The native command's refusals are typed and
// path-free by construction, and the two that a caller cannot recover from by
// retrying -- an over-budget candidate and an empty patch for a
// content-changing path -- each name a real continuation. Collapsing them
// into "retry the same binding" would send the caller into a loop that
// deterministically fails, which is the whole reason these codes exist.
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

// TestReviewPluginRefusesLensContextBoundToAnotherCandidate proves the plugin
// checks that the opaque handle the caller supplied really addresses the
// lineage, target, revision, and order the caller also claimed in its own
// binding. The provider authored both sides of the returned block, so this is
// not a trust check on the provider: it is what makes a caller that pointed a
// review at a different candidate fail here, loudly, instead of silently
// capturing a reviewer result into some other lineage later.
func TestReviewPluginRefusesLensContextBoundToAnotherCandidate(t *testing.T) {
	baseTree := strings.Repeat("1", 40)
	candidateTree := strings.Repeat("2", 40)
	for _, test := range []struct {
		name string
		from string
		to   string
	}{
		{name: "lineage", from: `"lineage":"trust-check"`, to: `"lineage":"other-lineage"`},
		{name: "target", from: `"target":"sha256:` + reviewPluginTargetSuffix + `"`, to: `"target":"sha256:` + strings.Repeat("7", 64) + `"`},
		{name: "revision", from: `"revision":"sha256:` + reviewPluginRevisionSuffix + `"`, to: `"revision":"sha256:` + strings.Repeat("6", 64) + `"`},
		{name: "order", from: `"order":0`, to: `"order":3`},
	} {
		t.Run(test.name, func(t *testing.T) {
			block := reviewPluginLensContext(baseTree, candidateTree)
			poisoned := strings.Replace(block, "GENTLE_AI_REVIEW_BINDING "+reviewPluginLensContextBinding,
				"GENTLE_AI_REVIEW_BINDING "+strings.Replace(reviewPluginLensContextBinding, test.from, test.to, 1), 1)
			if poisoned == block {
				t.Fatalf("test fixture did not substitute %q", test.from)
			}
			message := runReviewPluginScenarioStub(t, "before-valid", reviewPluginStub{lensContext: poisoned})
			if !strings.Contains(message, "binds a different candidate than the task claimed") {
				t.Fatalf("mismatched provider binding was injected anyway: %s", message)
			}
			if !strings.Contains(message, "The reviewer was not launched") {
				t.Fatalf("binding-mismatch refusal lost its exactly-once guarantee: %s", message)
			}
		})
	}
}

// TestReviewPluginRefusesUnterminatedLensContext proves a truncated provider
// block never reaches a reviewer. The provider assembles the whole block in
// memory before writing a byte precisely so this cannot happen, but a
// transport that lost the tail would hand the lens a partial candidate view
// it could still report as clean, which is the one failure this whole surface
// exists to make impossible.
func TestReviewPluginRefusesUnterminatedLensContext(t *testing.T) {
	block := reviewPluginLensContext(strings.Repeat("1", 40), strings.Repeat("2", 40))
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

// TestReviewPluginOpaqueDoubleFailurePreservesPayload pins the symmetry the
// external report identified: when capture AND durable preservation both fail,
// an opaque repository_context binding must retain the same bounded copy of the
// reviewer payload the legacy --cwd binding already retains. Both bindings
// resolve the same repository, so one environmental refusal can fail both, and
// on the opaque path the transcript was the only remaining copy.
func TestReviewPluginOpaqueDoubleFailurePreservesPayload(t *testing.T) {
	for _, scenario := range []string{"after-opaque", "after-legacy"} {
		t.Run(scenario, func(t *testing.T) {
			message := runReviewPluginScenario(t, scenario, "resolve failed")
			if message == "NO_ERROR" {
				t.Fatal("plugin did not fail despite an always-failing native binary")
			}
			if !strings.Contains(message, "raw reviewer result follows for manual recovery") {
				t.Fatalf("double failure dropped its last-resort payload fallback: %s", message)
			}
			if !strings.Contains(message, reviewPluginPayloadMarker) {
				t.Fatalf("double failure did not preserve the reviewer payload: %s", message)
			}
		})
	}
}

// TestReviewPluginPostLaunchTrustRefusalStaysActionable pins that the typed
// trust refusal keeps its carry-outable instruction on the post-launch capture
// path too, where the reviewer has already been spent and the payload is the
// only thing left to protect.
func TestReviewPluginPostLaunchTrustRefusalStaysActionable(t *testing.T) {
	message := runReviewPluginScenario(t, "after-opaque", reviewPluginNativeTrustFailure)
	if !strings.Contains(message, "git_repository_untrusted") {
		t.Fatalf("post-launch failure suppressed the native Git trust refusal: %s", message)
	}
	if !strings.Contains(message, "Restart the host process") {
		t.Fatalf("post-launch trust refusal carries no instruction the caller can carry out: %s", message)
	}
	if !strings.Contains(message, reviewPluginPayloadMarker) {
		t.Fatalf("post-launch trust refusal lost the reviewer payload: %s", message)
	}
}

// TestReviewPluginSurfacesNativeGitTrustRefusal pins the other half of
// finding 1: the plugin must stop collapsing a native Git trust refusal into
// "refresh the exact native next_transition", which cannot change the Git
// trust context of an already-running host process.
func TestReviewPluginSurfacesNativeGitTrustRefusal(t *testing.T) {
	message := runReviewPluginScenario(t, "before-opaque", reviewPluginNativeTrustFailure)
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

// TestReviewPluginSurfacesAdmissionRejectionClass pins the diagnosability fix
// from the first live 4R run: a reviewer result the native admission refused
// (for example a severe finding anchored to an unchanged line) collapsed into
// "retry the same opaque binding" — advice that deterministically fails,
// because recapturing identical bytes can never satisfy admission. The opaque
// message must carry the typed decision class and direct the caller to
// relaunch the reviewer, while the native diagnostic prose (which can embed
// payload text) stays out of the transcript.
func TestReviewPluginTerminatesUnstructuredAdmissionRejection(t *testing.T) {
	native := "Error: reviewer artifact admission out_of_scope: candidate-causal findings are not proven by repository-derived changed-line evidence"
	message := runReviewPluginScenario(t, "after-opaque", native)
	if message == "NO_ERROR" {
		t.Fatal("plugin did not fail despite an always-failing native binary")
	}
	if !strings.Contains(message, "rejected the reviewer result as out_of_scope") {
		t.Fatalf("admission rejection lost its typed decision class: %s", message)
	}
	if !strings.Contains(message, "reviewer_admission_recovery_unavailable") || !strings.Contains(message, "stop relaunching") {
		t.Fatalf("unstructured admission rejection was not terminal: %s", message)
	}
	if strings.Contains(message, "relaunch this lens reviewer") || strings.Contains(message, "retry the same binding") {
		t.Fatalf("unstructured admission rejection authorized a blind retry: %s", message)
	}
	if strings.Contains(message, "severe findings must anchor") {
		t.Fatalf("admission rejection inferred a severe-finding cause without structured evidence: %s", message)
	}
	if strings.Contains(message, "changed-line evidence") {
		t.Fatalf("plugin forwarded native admission diagnostic prose through an opaque binding: %s", message)
	}
	if !strings.Contains(message, reviewPluginPayloadMarker) {
		t.Fatalf("admission rejection did not preserve the reviewer payload: %s", message)
	}
}

func TestReviewPluginTerminatesIncompleteAdmissionWithoutDiagnostic(t *testing.T) {
	nativeDiagnosticMarker := "NATIVE-DIAGNOSTIC-4e7c"
	native := "Error: reviewer artifact admission incomplete: " + nativeDiagnosticMarker +
		` rejected payload {"evidence":["` + reviewPluginPayloadMarker + `"]}`
	message := runReviewPluginScenarioWithNativeAndPreservation(
		t,
		"after-incomplete",
		"",
		native,
		`{"reference":"incident/reviewer-result.json"}`,
	)
	if message == "NO_ERROR" {
		t.Fatal("plugin did not fail despite native incomplete admission")
	}
	if !strings.Contains(message, "rejected the reviewer result as incomplete") {
		t.Fatalf("incomplete admission lost its typed decision: %s", message)
	}
	if !strings.Contains(message, "reviewer_admission_recovery_unavailable") || !strings.Contains(message, "without a safe actionable diagnostic") {
		t.Fatalf("incomplete admission was not terminal and actionable: %s", message)
	}
	if strings.Contains(message, "relaunch this lens reviewer") {
		t.Fatalf("incomplete admission authorized a blind relaunch: %s", message)
	}
	if strings.Contains(message, "severe findings must anchor") {
		t.Fatalf("incomplete admission inferred a severe-finding cause: %s", message)
	}
	if strings.Contains(message, nativeDiagnosticMarker) {
		t.Fatalf("incomplete admission leaked native diagnostic prose: %s", message)
	}
	if strings.Contains(message, reviewPluginPayloadMarker) {
		t.Fatalf("incomplete admission leaked the rejected reviewer payload: %s", message)
	}
}

func TestReviewPluginSurfacesStructuredLocationRecoveryDiagnostic(t *testing.T) {
	native := `Error: reviewer artifact admission out_of_scope: reviewer finding location is invalid; ` +
		`admission_diagnostic={"code":"invalid_finding_location","finding_id":"R3-001",` +
		`"location":"internal/a.go:207-221","reason":"line_suffix_not_integer"}`
	message := runReviewPluginScenarioWithNativeAndPreservation(
		t, "after-opaque", "", native, `{"reference":"rinc1_safe"}`,
	)
	for _, want := range []string{
		"rejected the reviewer result as out_of_scope",
		`finding R3-001 at "internal/a.go:207-221": line_suffix_not_integer`,
		"relaunch this lens reviewer",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("structured recovery message %q does not contain %q", message, want)
		}
	}
	if strings.Contains(message, "reviewer finding location is invalid") {
		t.Fatalf("structured recovery leaked native diagnostic prose: %s", message)
	}
	for _, location := range []string{
		`C:\Users\private\repo\a.go:207-221`,
		"internal:../private/a.go:207-221",
		"internal/a.go:7:/home/private/repo.go",
		"internal/a.go:7:https://private.example/repo",
	} {
		unsafe := strings.Replace(native, "internal/a.go:207-221", location, 1)
		message = runReviewPluginScenarioWithNativeAndPreservation(t, "after-opaque", "", unsafe, `{"reference":"rinc1_safe"}`)
		if strings.Contains(message, location) || strings.Contains(message, "finding R3-001") {
			t.Fatalf("unsafe structured diagnostic escaped opaque filtering: %s", message)
		}
	}
	for _, incompatible := range []string{
		strings.Replace(native, `"code":"invalid_finding_location"`, `"code":"candidate_causality_unproven"`, 1),
		strings.Replace(native, `"reason":"line_suffix_not_integer"`, `"reason":"line_not_changed_by_candidate"`, 1),
	} {
		message = runReviewPluginScenarioWithNativeAndPreservation(t, "after-opaque", "", incompatible, `{"reference":"rinc1_safe"}`)
		if strings.Contains(message, "finding R3-001") || strings.Contains(message, "internal/a.go:207-221") {
			t.Fatalf("incompatible structured diagnostic escaped filtering: %s", message)
		}
	}
}

func TestReviewPluginScopesAndReclaimsAdmissionRecovery(t *testing.T) {
	native := `Error: reviewer artifact admission out_of_scope: native detail must stay opaque; admission_diagnostic={"code":"candidate_causality_unproven","finding_id":"R3-001","location":"internal/a.go:7","reason":"line_not_changed_by_candidate"}`
	message := runReviewPluginScenarioWithNativeAndPreservation(t, "after-state", "", native, `{"reference":"rinc1_safe"}`)
	parts := strings.Split(message, "\n---\n")
	if len(parts) != 4 {
		t.Fatalf("bounded recovery outcomes = %q", message)
	}
	for _, index := range []int{0, 1, 3} {
		if !strings.Contains(parts[index], "exactly once") || strings.Contains(parts[index], "recovery_exhausted") {
			t.Fatalf("outcome %d did not hold an independent first attempt: %s", index, parts[index])
		}
	}
	if !strings.Contains(parts[2], "reviewer_admission_recovery_exhausted") || !strings.Contains(parts[2], "stop relaunching") {
		t.Fatalf("same session and STATUS binding did not become terminal: %s", parts[2])
	}
	if !strings.Contains(parts[0], "line_not_changed_by_candidate") {
		t.Fatalf("first refusal lost its actionable diagnostic: %s", parts[0])
	}
}

func TestReviewPluginReclaimsSuccessfulAdmissionRecovery(t *testing.T) {
	native := `reviewer artifact admission out_of_scope: opaque; admission_diagnostic={"code":"candidate_causality_unproven","finding_id":"R3-001","location":"internal/a.go:7","reason":"line_not_changed_by_candidate"}`
	parts := strings.Split(runReviewPluginScenarioWithNativeAndPreservation(t, "after-success", "", native, `{"reference":"rinc1_safe"}`), "\n---\n")
	if len(parts) != 3 || !strings.Contains(parts[0], "exactly once") || parts[1] != "CAPTURED" || !strings.Contains(parts[2], "exactly once") {
		t.Fatalf("successful capture did not reclaim its recovery state: %q", parts)
	}
}

func TestReviewPluginClearsRecoveryOnLifecycleHooks(t *testing.T) {
	native := `reviewer artifact admission out_of_scope: opaque; admission_diagnostic={"code":"invalid_finding_location","finding_id":"R3-001","location":"internal/a.go:7-9","reason":"line_suffix_not_integer"}`
	parts := strings.Split(runReviewPluginScenarioWithNativeAndPreservation(t, "after-lifecycle", "", native, `{"reference":"rinc1_safe"}`), "\n---\n")
	if len(parts) != 4 {
		t.Fatalf("lifecycle outcomes = %q", parts)
	}
	for index, part := range parts {
		if !strings.Contains(part, "exactly once") || strings.Contains(part, "recovery_exhausted") {
			t.Fatalf("lifecycle outcome %d retained stale recovery state: %s", index, part)
		}
	}
}

func TestReviewPluginBoundsRecoveryWithoutLifecycleEvents(t *testing.T) {
	native := `reviewer artifact admission out_of_scope: opaque; admission_diagnostic={"code":"invalid_finding_location","finding_id":"R3-001","location":"internal/a.go:7-9","reason":"line_suffix_not_integer"}`
	parts := strings.Split(runReviewPluginScenarioWithNativeAndPreservation(t, "after-no-lifecycle", "", native, `{"reference":"rinc1_safe"}`), "\n---\n")
	if len(parts) != 3 || parts[0] != "8,64" ||
		!strings.Contains(parts[1], "reviewer_admission_recovery_unavailable") ||
		!strings.Contains(parts[2], "reviewer_admission_recovery_unavailable") {
		t.Fatalf("no-lifecycle fallback was not bounded: %q", parts)
	}
}

// TestReviewPluginKeepsGenericOpaqueFailureOpaque proves the trust
// pass-through is not a hole in the opaque path's path-safety rule: any
// other native preflight failure still collapses into the generic
// provider-owned message.
func TestReviewPluginKeepsGenericOpaqueFailureOpaque(t *testing.T) {
	leak := "repository_context_unavailable: provider-issued review repository context operation failed; " +
		"failed under /home/someone/private/repo"
	message := runReviewPluginScenario(t, "before-opaque", leak)
	if strings.Contains(message, "/home/someone/private/repo") {
		t.Fatalf("plugin forwarded a native path through an opaque binding: %s", message)
	}
	if !strings.Contains(message, "repository_context_lens_context_failed") {
		t.Fatalf("generic opaque failure lost its provider-owned code: %s", message)
	}
}

// TestReviewPluginRefusesReviewerLaunchWithoutIsolationEnvironment closes the
// CRITICAL gap: OpenCode assembles every session's *system* prompt (not the
// `task` args.prompt this plugin controls) from the agent's base prompt plus
// an environment block, every AGENTS.md/CLAUDE.md/CONTEXT.md found walking up
// from the worktree root, local `instructions` glob entries, and the live
// skill catalog -- unconditionally, regardless of the agent's tools map.
// Measured against real OpenCode 1.18.10: a review-risk session with no bash
// and no read tool still received a marker planted in AGENTS.md after the
// candidate froze, verbatim, in its system message. The only verified way to
// close it is OPENCODE_DISABLE_PROJECT_CONFIG and OPENCODE_DISABLE_EXTERNAL_SKILLS,
// so the plugin refuses to launch the reviewer at all unless both are set.
func TestReviewPluginRefusesReviewerLaunchWithoutIsolationEnvironment(t *testing.T) {
	block := reviewPluginLensContext(strings.Repeat("1", 40), strings.Repeat("2", 40))
	tests := []struct {
		name                        string
		omitProjectConfigIsolation  bool
		omitExternalSkillsIsolation bool
		wantNamed                   []string
	}{
		{name: "both missing", omitProjectConfigIsolation: true, omitExternalSkillsIsolation: true,
			wantNamed: []string{"OPENCODE_DISABLE_PROJECT_CONFIG", "OPENCODE_DISABLE_EXTERNAL_SKILLS"}},
		{name: "project config missing", omitProjectConfigIsolation: true,
			wantNamed: []string{"OPENCODE_DISABLE_PROJECT_CONFIG"}},
		{name: "external skills missing", omitExternalSkillsIsolation: true,
			wantNamed: []string{"OPENCODE_DISABLE_EXTERNAL_SKILLS"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message := runReviewPluginScenarioStub(t, "before-valid", reviewPluginStub{
				lensContext:                 block,
				omitProjectConfigIsolation:  test.omitProjectConfigIsolation,
				omitExternalSkillsIsolation: test.omitExternalSkillsIsolation,
			})
			for _, want := range test.wantNamed {
				if !strings.Contains(message, want) {
					t.Errorf("refusal does not name missing variable %q: %s", want, message)
				}
			}
			if !strings.Contains(message, "The reviewer was not launched") {
				t.Errorf("isolation refusal lost its exactly-once guarantee: %s", message)
			}
			if !strings.Contains(message, "AGENTS.md") || !strings.Contains(message, "skill catalog") {
				t.Errorf("isolation refusal does not explain what it is protecting against: %s", message)
			}
		})
	}
}

// TestReviewPluginAllowsReviewerLaunchWithIsolationEnvironment is the
// positive twin: with both variables set (every other test in this file's
// default), the reviewer launches normally. This pins that the gate is
// exact -- it refuses only when a real gap exists, never unconditionally.
func TestReviewPluginAllowsReviewerLaunchWithIsolationEnvironment(t *testing.T) {
	prompt := runReviewPluginScenarioStub(t, "before-valid", reviewPluginStub{
		lensContext: reviewPluginLensContext(strings.Repeat("1", 40), strings.Repeat("2", 40)),
	})
	if !strings.HasPrefix(prompt, "GENTLE_AI_REVIEW_BINDING {") {
		t.Fatalf("reviewer did not launch with both isolation variables set: %q", prompt)
	}
}

// TestReviewPluginRefusesRemoteInstructionsEntry closes the residual the
// isolation-environment gate cannot: OpenCode fetches an http(s)://
// `instructions` config entry unconditionally into every session's system
// prompt, from any config layer, regardless of OPENCODE_DISABLE_PROJECT_CONFIG.
// Verified against real OpenCode 1.18.10 via the plugin's own
// `client.config.get()` (the same endpoint the OpenCode TUI/CLI itself reads
// effective config from): a poisoned local HTTP endpoint referenced this way
// still reached the reviewer with both isolation variables set. The plugin
// now reads the effective config through that same client and refuses
// outright when a remote instructions entry is present.
func TestReviewPluginRefusesRemoteInstructionsEntry(t *testing.T) {
	baseTree := strings.Repeat("1", 40)
	candidateTree := strings.Repeat("2", 40)
	for _, test := range []struct {
		name        string
		instruction string
	}{
		{name: "http", instruction: "http://attacker.invalid/poison.md"},
		{name: "https", instruction: "https://attacker.invalid/poison.md"},
	} {
		t.Run(test.name, func(t *testing.T) {
			message := runReviewPluginScenarioStub(t, "before-valid", reviewPluginStub{
				lensContext:        reviewPluginLensContext(baseTree, candidateTree),
				configInstructions: []string{"docs/local-instructions.md", test.instruction},
			})
			if !strings.Contains(message, "refuses a remote `instructions` entry") {
				t.Fatalf("remote instructions entry was not refused: %s", message)
			}
			if !strings.Contains(message, test.instruction) {
				t.Fatalf("refusal does not name the offending entry %q: %s", test.instruction, message)
			}
			if strings.Contains(message, "docs/local-instructions.md") {
				t.Fatalf("refusal named a local instructions entry, not just the remote one: %s", message)
			}
			if !strings.Contains(message, "The reviewer was not launched") {
				t.Fatalf("remote-instructions refusal lost its exactly-once guarantee: %s", message)
			}
		})
	}
}

// TestReviewPluginAllowsLocalOnlyInstructions proves the guard above has no
// false positive: an effective config with only local (non-http) instructions
// entries, or none at all, must not be refused.
func TestReviewPluginAllowsLocalOnlyInstructions(t *testing.T) {
	baseTree := strings.Repeat("1", 40)
	candidateTree := strings.Repeat("2", 40)
	for _, test := range []struct {
		name         string
		instructions []string
	}{
		{name: "no instructions configured", instructions: nil},
		{name: "local file only", instructions: []string{"docs/local-instructions.md"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			prompt := runReviewPluginScenarioStub(t, "before-valid", reviewPluginStub{
				lensContext:        reviewPluginLensContext(baseTree, candidateTree),
				configInstructions: test.instructions,
			})
			if !strings.HasPrefix(prompt, "GENTLE_AI_REVIEW_BINDING {") {
				t.Fatalf("local-only instructions config was refused: %q", prompt)
			}
		})
	}
}

// TestReviewPluginRefusesWhenEffectiveConfigIsUnverifiable proves the plugin
// fails closed, not open, when it cannot read the effective config at all:
// an unverifiable configuration cannot be ruled safe, so the reviewer must
// not launch on the strength of an absent error alone.
func TestReviewPluginRefusesWhenEffectiveConfigIsUnverifiable(t *testing.T) {
	baseTree := strings.Repeat("1", 40)
	candidateTree := strings.Repeat("2", 40)
	message := runReviewPluginScenarioStub(t, "before-valid", reviewPluginStub{
		lensContext: reviewPluginLensContext(baseTree, candidateTree),
		configError: "NATIVE-CONFIG-ENDPOINT-UNAVAILABLE",
	})
	if !strings.Contains(message, "could not verify the effective configuration") {
		t.Fatalf("an unreadable effective config did not refuse the reviewer launch: %s", message)
	}
	if !strings.Contains(message, "NATIVE-CONFIG-ENDPOINT-UNAVAILABLE") {
		t.Fatalf("refusal lost the underlying config-read failure: %s", message)
	}
	if !strings.Contains(message, "The reviewer was not launched") {
		t.Fatalf("unverifiable-config refusal lost its exactly-once guarantee: %s", message)
	}
}
