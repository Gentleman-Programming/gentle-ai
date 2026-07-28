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
const prompt = ` + "`" + `GENTLE_AI_REVIEW_BINDING ${JSON.stringify(binding)}\nreview the frozen candidate\n` + "`" + `

try {
  if (scenario.startsWith("before")) {
    const output = { args: { subagent_type: "review-risk", prompt } }
    await hooks["tool.execute.before"]({ tool: "task" }, output)
    console.log("NO_ERROR")
  } else {
    const input = { tool: "task", args: { subagent_type: "review-risk", prompt } }
    const output = { output: '{"subject_hash":"sha256:x","findings":[],"evidence":["` + reviewPluginPayloadMarker + `"]}' }
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
	stub := "#!/bin/sh\ncat >/dev/null\nprintf '%s\\n' \"$GENTLE_AI_STUB_STDERR\" >&2\nexit 1\n"
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
		"GENTLE_AI_STUB_STDERR="+nativeStderr,
		"GENTLE_AI_REVIEW_CWD=",
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Skipf("node could not run the TypeScript plugin harness (%v): %s", err, output)
	}
	return strings.TrimSpace(string(output))
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
