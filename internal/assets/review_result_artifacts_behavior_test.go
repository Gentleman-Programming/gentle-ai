package assets

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type nativePluginInvocation struct {
	Args  []string `json:"args"`
	Stdin string   `json:"stdin"`
}

func TestReviewResultArtifactsPluginEnrichesCaptureFromProviderContext(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Node.js")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("requires Node.js")
	}
	t.Run("findings and evidence receive provider-owned capture metadata", func(t *testing.T) {
		invocations, output := runReviewResultArtifactsHarness(t, false, false, false)
		if output != `{"captured":true}` {
			t.Fatalf("plugin output = %q", output)
		}
		if len(invocations) != 2 || !containsArgument(invocations[0].Args, "--preflight") ||
			invocations[1].Args[0] != "review" || invocations[1].Args[1] != "capture-result" {
			t.Fatalf("native invocations = %#v", invocations)
		}
		var capture map[string]any
		if err := json.Unmarshal([]byte(invocations[1].Stdin), &capture); err != nil {
			t.Fatalf("decode capture stdin: %v\n%s", err, invocations[1].Stdin)
		}
		want := map[string]any{
			"subject_hash": "sha256:" + strings.Repeat("a", 64),
			"inspection":   map[string]any{"status": "completed", "paths": []any{"z-last.go", "a-first.go"}},
			"findings":     []any{},
			"evidence":     []any{"inspected exact frozen candidate"},
		}
		if !equalJSON(capture, want) {
			t.Fatalf("capture stdin = %#v, want %#v", capture, want)
		}
	})

	t.Run("binding drift fails before capture", func(t *testing.T) {
		invocations, output := runReviewResultArtifactsHarness(t, true, false, false)
		if len(invocations) != 2 || !containsArgument(invocations[0].Args, "--preflight") ||
			invocations[1].Args[0] != "review" || invocations[1].Args[1] != "preserve-result" {
			t.Fatalf("binding drift reached capture: %#v", invocations)
		}
		if !strings.Contains(output, "repository_context_capture_failed") || !strings.Contains(output, "preserved for recovery") {
			t.Fatalf("binding drift error = %q", output)
		}
	})

	t.Run("provider binding fails before launch when preflight is unavailable", func(t *testing.T) {
		invocations, output := runReviewResultArtifactsHarness(t, false, true, false)
		if len(invocations) != 1 || !containsArgument(invocations[0].Args, "--preflight") {
			t.Fatalf("missing preflight launched reviewer or capture: %#v", invocations)
		}
		if !strings.Contains(output, "requires capture preflight") {
			t.Fatalf("missing preflight error = %q", output)
		}
	})

	t.Run("capture failure preserves the provider-enriched replay payload", func(t *testing.T) {
		invocations, output := runReviewResultArtifactsHarness(t, false, false, true)
		if len(invocations) != 3 || invocations[1].Args[1] != "capture-result" || invocations[2].Args[1] != "preserve-result" {
			t.Fatalf("capture recovery invocations = %#v", invocations)
		}
		if invocations[1].Stdin != invocations[2].Stdin || !strings.Contains(invocations[2].Stdin, `"subject_hash":"sha256:`) ||
			!strings.Contains(invocations[2].Stdin, `"inspection":{"status":"completed"`) {
			t.Fatalf("preserved replay payload = %q, capture payload = %q", invocations[2].Stdin, invocations[1].Stdin)
		}
		if !strings.Contains(output, "preserved for recovery") {
			t.Fatalf("capture recovery error = %q", output)
		}
	})
}

func runReviewResultArtifactsHarness(t *testing.T, drift, unsupportedPreflight, captureFail bool) ([]nativePluginInvocation, string) {
	t.Helper()
	dir := t.TempDir()
	plugin := filepath.Join(dir, "review-result-artifacts.mts")
	if err := os.WriteFile(plugin, []byte(MustRead("opencode/plugins/review-result-artifacts.ts")), 0o600); err != nil {
		t.Fatal(err)
	}
	mockSource := filepath.Join(dir, "mock-gentle-ai.go")
	if err := os.WriteFile(mockSource, []byte(`package main
import (
  "encoding/json"
  "io"
  "os"
  "slices"
)
func main() {
  stdin, _ := io.ReadAll(os.Stdin)
  invocation, _ := json.Marshal(map[string]any{"args": os.Args[1:], "stdin": string(stdin)})
  log, _ := os.OpenFile(os.Getenv("GENTLE_AI_TEST_LOG"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
  if log != nil { _, _ = log.Write(append(invocation, '\n')); _ = log.Close() }
  if slices.Contains(os.Args[1:], "--preflight") && os.Getenv("GENTLE_AI_TEST_UNSUPPORTED_PREFLIGHT") == "true" {
    _, _ = os.Stderr.WriteString("flag provided but not defined: -preflight")
    os.Exit(1)
  }
  if slices.Contains(os.Args[1:], "--preflight") { _, _ = os.Stdout.WriteString(os.Getenv("GENTLE_AI_TEST_PREFLIGHT")); return }
  if len(os.Args) > 2 && os.Args[1] == "review" && os.Args[2] == "capture-result" && os.Getenv("GENTLE_AI_TEST_CAPTURE_FAIL") == "true" {
    _, _ = os.Stderr.WriteString("simulated capture failure")
    os.Exit(1)
  }
  _, _ = os.Stdout.WriteString("{\"captured\":true}")
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	mock := filepath.Join(dir, "gentle-ai")
	if strings.EqualFold(filepath.Ext(os.Args[0]), ".exe") {
		mock += ".exe"
	}
	build := exec.Command("go", "build", "-o", mock, mockSource)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build native mock: %v\n%s", err, output)
	}
	harness := filepath.Join(dir, "harness.mjs")
	if err := os.WriteFile(harness, []byte(`
import pluginFactory from "./review-result-artifacts.mts"
const subject = "sha256:" + "a".repeat(64)
const binding = {
  lineage: "review-provider-binding", target: "sha256:" + "b".repeat(64), lens: "review-reliability", order: 0,
  revision: "sha256:" + "c".repeat(64), repository_context: "rctx1_" + "d".repeat(64), subject_hash: subject,
}
const args = {subagent_type: binding.lens, prompt: `+"`"+`GENTLE_AI_REVIEW_BINDING ${JSON.stringify(binding)}
Review the frozen candidate.`+"`"+`, background: false}
const hooks = await pluginFactory({directory: process.cwd(), worktree: process.cwd()})
try {
  await hooks["tool.execute.before"]({tool: "task"}, {args})
} catch (error) {
  process.stdout.write(error instanceof Error ? error.message : String(error))
  process.exit(0)
}
if (process.env.GENTLE_AI_TEST_DRIFT === "true") args.prompt = args.prompt.replace(subject, "sha256:" + "e".repeat(64))
const output = {output: '{"findings":[],"evidence":["inspected exact frozen candidate"]}'}
try {
  await hooks["tool.execute.after"]({tool: "task", args}, output)
  process.stdout.write(output.output)
} catch (error) {
  process.stdout.write(error instanceof Error ? error.message : String(error))
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "native.jsonl")
	preflight, err := json.Marshal(map[string]any{
		"artifact_subject": map[string]any{"subject_hash": "sha256:" + strings.Repeat("a", 64)},
		"candidate_diff":   map[string]any{"encoding": "base64", "content": "ZGlmZg=="},
		"changed_path_manifest": []any{
			map[string]any{"path": "z-last.go"}, map[string]any{"path": "a-first.go"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("node", harness)
	command.Dir = dir
	command.Env = append(os.Environ(),
		"PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GENTLE_AI_TEST_LOG="+logPath,
		"GENTLE_AI_TEST_PREFLIGHT="+string(preflight),
		"GENTLE_AI_TEST_DRIFT="+map[bool]string{false: "false", true: "true"}[drift],
		"GENTLE_AI_TEST_UNSUPPORTED_PREFLIGHT="+map[bool]string{false: "false", true: "true"}[unsupportedPreflight],
		"GENTLE_AI_TEST_CAPTURE_FAIL="+map[bool]string{false: "false", true: "true"}[captureFail],
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run plugin harness: %v\n%s", err, output)
	}
	payload, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(payload)), "\n")
	invocations := make([]nativePluginInvocation, 0, len(lines))
	for _, line := range lines {
		var invocation nativePluginInvocation
		if err := json.Unmarshal([]byte(line), &invocation); err != nil {
			t.Fatal(err)
		}
		invocations = append(invocations, invocation)
	}
	return invocations, string(output)
}

func containsArgument(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func equalJSON(left, right any) bool {
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return string(leftJSON) == string(rightJSON)
}
