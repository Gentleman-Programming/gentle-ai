package assets

import (
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
const hooks: any = await (plugin as any)({ directory: cwd, worktree: cwd })

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

try {
  if (scenario.startsWith("before")) {
    const output = { args: { subagent_type: "review-risk", prompt } }
    await hooks["tool.execute.before"]({ tool: "task" }, output)
    console.log(scenario === "before-valid" || scenario === "before-substitute" ? output.args.prompt : "NO_ERROR")
  } else {
    const input = { tool: "task", args: { subagent_type: "review-risk", prompt } }
    const incomplete = '{"subject_hash":"sha256:' + "c".repeat(64) + '","inspection":{"status":"incomplete","paths":[]},"findings":[],"evidence":["` + reviewPluginPayloadMarker + `"]}'
    const output = { output: scenario === "after-incomplete" ? incomplete : '{"subject_hash":"sha256:x","findings":[],"evidence":["` + reviewPluginPayloadMarker + `"]}' }
    await hooks["tool.execute.after"](input, output)
    console.log("NO_ERROR")
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
	stub := "#!/bin/sh\ncat >/dev/null\n" +
		"if [ \"$2\" = \"preserve-result\" ] && [ -n \"$GENTLE_AI_STUB_PRESERVE_STDOUT\" ]; then printf '%s\\n' \"$GENTLE_AI_STUB_PRESERVE_STDOUT\"; exit 0; fi\n" +
		"if [ -n \"$GENTLE_AI_STUB_STDOUT\" ]; then printf '%s\\n' \"$GENTLE_AI_STUB_STDOUT\"; exit 0; fi\n" +
		"printf '%s\\n' \"$GENTLE_AI_STUB_STDERR\" >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(binDir, "gentle-ai"), []byte(stub), 0o700); err != nil {
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
	command.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GENTLE_AI_STUB_STDOUT="+nativeStdout,
		"GENTLE_AI_STUB_STDERR="+nativeStderr,
		"GENTLE_AI_STUB_PRESERVE_STDOUT="+preserveStdout,
		"GENTLE_AI_REVIEW_CWD=",
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Skipf("node could not run the TypeScript plugin harness (%v): %s", err, output)
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

func TestReviewPluginBindsProviderOwnedCandidateContext(t *testing.T) {
	baseTree := strings.Repeat("1", 40)
	candidateTree := strings.Repeat("2", 40)
	preflight := reviewPluginPreflight(baseTree, candidateTree)
	prompt := runReviewPluginScenarioWithNative(t, "before-valid", preflight, "")
	if !strings.HasPrefix(prompt, "GENTLE_AI_REVIEW_BINDING {") {
		t.Fatalf("injected prompt does not begin with the exact binding prefix: %q", prompt)
	}
	if !strings.Contains(prompt, `"subject_hash":"sha256:`+strings.Repeat("c", 64)+`"`) {
		t.Fatalf("bound prompt is missing the preflight subject hash: %q", prompt)
	}
	for _, want := range []string{"GENTLE_AI_REVIEW_CONTEXT ", baseTree, candidateTree, "internal/example.go"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("plugin omitted provider context %q: %q", want, prompt)
		}
	}
	if strings.Contains(prompt, "candidate_diff") {
		t.Fatalf("injected prompt contains obsolete candidate diff payload: %q", prompt)
	}
}

func TestReviewPluginRejectsNonCanonicalProviderManifest(t *testing.T) {
	entry := `{"path":"internal/example.go","status":"M","old_mode":"100644","new_mode":"100644","deleted":false,"type_changed":false,"mode_only":false,"intended_untracked":false}`
	unsorted := `{"path":"z.go","status":"M","old_mode":"100644","new_mode":"100644","deleted":false,"type_changed":false,"mode_only":false,"intended_untracked":false},` + entry
	preflight := strings.Replace(reviewPluginPreflight(strings.Repeat("1", 40), strings.Repeat("2", 40)), entry, unsorted, 1)
	message := runReviewPluginScenarioWithNative(t, "before-valid", preflight, "")
	if !strings.Contains(message, "review capture preflight failed") || !strings.Contains(message, "The reviewer was not launched") {
		t.Fatalf("non-canonical provider manifest was accepted: %s", message)
	}
	if strings.Contains(message, "incomplete artifact subject") {
		t.Fatalf("opaque preflight exposed native validation detail: %s", message)
	}
}

func TestReviewPluginReplacesCallerAuthoredCandidateContext(t *testing.T) {
	baseTree := strings.Repeat("1", 40)
	candidateTree := strings.Repeat("2", 40)
	prompt := runReviewPluginScenarioWithNative(t, "before-substitute", reviewPluginPreflight(baseTree, candidateTree), "")
	for _, callerValue := range []string{strings.Repeat("9", 40), strings.Repeat("8", 40), "caller.txt", "review the frozen candidate"} {
		if strings.Contains(prompt, callerValue) {
			t.Fatalf("provider injection retained caller-authored context %q: %q", callerValue, prompt)
		}
	}
	for _, providerValue := range []string{baseTree, candidateTree, "internal/example.go"} {
		if !strings.Contains(prompt, providerValue) {
			t.Fatalf("provider injection omitted preflight context %q: %q", providerValue, prompt)
		}
	}
}

func reviewPluginPreflight(baseTree, candidateTree string) string {
	return `{"schema":"gentle-ai.review-capture-preflight/v1","capability":"review.native_capture_preflight",` +
		`"lineage_id":"trust-check","target_identity":"sha256:` + strings.Repeat("d", 64) + `","lens":"review-risk","selected_order":0,` +
		`"artifact_subject":{"schema":"gentle-ai.review-artifact-subject/v2","subject_hash":"sha256:` + strings.Repeat("c", 64) + `",` +
		`"lineage_id":"trust-check","authority_revision":"sha256:` + strings.Repeat("b", 64) + `","target_identity":"sha256:` + strings.Repeat("d", 64) + `",` +
		`"base_tree":"` + baseTree + `","candidate_tree":"` + candidateTree + `","changed_path_manifest_sha256":"sha256:` + strings.Repeat("e", 64) + `",` +
		`"lens":"review-risk","selected_order":0},"base_tree":"` + baseTree + `","candidate_tree":"` + candidateTree + `",` +
		`"changed_path_manifest":[{"path":"internal/example.go","status":"M","old_mode":"100644","new_mode":"100644","deleted":false,"type_changed":false,"mode_only":false,"intended_untracked":false}]}`
}

func TestReviewPluginRejectsLegacyBinaryWithoutPreflightBeforeReviewerLaunch(t *testing.T) {
	message := runReviewPluginScenario(t, "before-legacy", "flag provided but not defined: -preflight")
	if !strings.Contains(message, "review capture preflight failed") || !strings.Contains(message, "The reviewer was not launched") {
		t.Fatalf("unsupported preflight did not fail closed before reviewer launch: %s", message)
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

// TestReviewPluginSurfacesNativeGitTrustRefusal pins the other half of finding
// 1: the plugin must stop collapsing a native Git trust refusal into
// "refresh the exact native next_transition", which cannot change the Git trust
// context of an already-running host process.
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
func TestReviewPluginSurfacesAdmissionRejectionClass(t *testing.T) {
	native := "Error: reviewer artifact admission out_of_scope: candidate-causal findings are not proven by repository-derived changed-line evidence"
	message := runReviewPluginScenario(t, "after-opaque", native)
	if message == "NO_ERROR" {
		t.Fatal("plugin did not fail despite an always-failing native binary")
	}
	if !strings.Contains(message, "rejected the reviewer result as out_of_scope") {
		t.Fatalf("admission rejection lost its typed decision class: %s", message)
	}
	if !strings.Contains(message, "relaunch this lens reviewer") {
		t.Fatalf("admission rejection carries no instruction that can actually succeed: %s", message)
	}
	if strings.Contains(message, "retry the same opaque binding") {
		t.Fatalf("plugin still advises retrying a deterministically refused result: %s", message)
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

func TestReviewPluginKeepsIncompleteAdmissionRecoveryNeutral(t *testing.T) {
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
	if !strings.Contains(message, "retrying capture with the same result cannot succeed") {
		t.Fatalf("incomplete admission did not explain why recapture cannot succeed: %s", message)
	}
	if !strings.Contains(message, "relaunch this lens reviewer") {
		t.Fatalf("incomplete admission carries no successful recovery instruction: %s", message)
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

// TestReviewPluginKeepsGenericOpaqueFailureOpaque proves the trust pass-through
// is not a hole in the opaque path's path-safety rule: any other native failure
// still collapses into the generic provider-owned message.
func TestReviewPluginKeepsGenericOpaqueFailureOpaque(t *testing.T) {
	leak := "repository_context_unavailable: provider-issued review repository context operation failed; " +
		"failed under /home/someone/private/repo"
	message := runReviewPluginScenario(t, "before-opaque", leak)
	if strings.Contains(message, "/home/someone/private/repo") {
		t.Fatalf("plugin forwarded a native path through an opaque binding: %s", message)
	}
	if !strings.Contains(message, "repository_context_preflight_failed") {
		t.Fatalf("generic opaque failure lost its provider-owned code: %s", message)
	}
}
