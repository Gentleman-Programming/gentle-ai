package assets

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const reviewPluginPayloadMarker = `{"subject_hash":"sha256:cccc","inspection":{"status":"completed","paths":["internal/example.go"]},"findings":[],"evidence":["inspected immutable evidence"]}`

const reviewPluginHarness = `import plugin from "./plugin.mts"

const scenario = process.argv[2]
const cwd = process.argv[3]
const hooks = await plugin({ directory: cwd, worktree: cwd })
const opaque = { lens: "review-risk", lineage: "trust-check", order: 0, repository_context: "rctx1_" + "a".repeat(64), revision: "sha256:" + "b".repeat(64), subject_hash: "sha256:" + "c".repeat(64), target: "sha256:" + "d".repeat(64) }
const legacy = { lens: "review-risk", lineage: "trust-check", order: 0, target: "sha256:" + "d".repeat(64) }
const prompt = (binding) => ` + "`" + `GENTLE_AI_REVIEW_BINDING ${JSON.stringify(binding)}
changed_path_manifest: [{"path":"internal/example.go","status":"M"}]
` + "`" + `

const after = async (binding) => {
  const input = { tool: "task", sessionID: "session-a", args: { subagent_type: "review-risk", prompt: prompt(binding) } }
  const output = { output: "<task id=\"x\" state=\"completed\">\n<task_result>\n" + process.env.REVIEWER_RESULT + "\n</task_result>\n</task>" }
  await hooks["tool.execute.after"](input, output)
  return output.output
}

if (scenario === "v2") {
  const before = { args: { subagent_type: "review-risk", prompt: prompt(opaque) } }
  await hooks["tool.execute.before"]({ tool: "task", sessionID: "session-a" }, before)
  const inspected = await hooks.tool.gentle_ai_review_inspect.execute({ binding: JSON.stringify(opaque), operation: "patch", path_index: "0" })
  const captured = await after(opaque)
  console.log(JSON.stringify({ prompt: before.args.prompt, inspected, captured, files: await (await import("node:fs/promises")).readdir(cwd) }))
} else if (scenario === "legacy") {
  console.log(await after(legacy))
} else if (scenario === "malformed-v2") {
  const malformed = { ...opaque }
  delete malformed.subject_hash
  try {
    console.log(await after(malformed))
  } catch (cause) {
    console.log(cause instanceof Error ? cause.message : String(cause))
  }
} else if (scenario === "background") {
  try {
    await hooks["tool.execute.before"]({ tool: "task", sessionID: "session-a" }, { args: { subagent_type: "review-risk", prompt: prompt(opaque), background: true } })
    console.log("NO_ERROR")
  } catch (cause) {
    console.log(cause instanceof Error ? cause.message : String(cause))
  }
}
`

type reviewPluginStub struct {
	argvLog  string
	stdinLog string
	inspect  string
	captured string
}

func runReviewPluginScenario(t *testing.T, scenario string, stub reviewPluginStub) string {
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
		t.Fatal(err)
	}
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	workDir := filepath.Join(root, "work")
	pluginDir := filepath.Join(root, "node_modules", "@opencode-ai", "plugin")
	for _, dir := range []string{binDir, workDir, pluginDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	pluginModule := `export const tool = (definition) => definition; tool.schema = { string: () => ({ optional: () => ({}) }) };`
	if err := os.WriteFile(filepath.Join(pluginDir, "index.mjs"), []byte(pluginModule), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "package.json"), []byte(`{"type":"module","exports":"./index.mjs"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stubScript := "#!/bin/sh\n" +
		"if [ -n \"$GENTLE_AI_STUB_ARGV_LOG\" ]; then printf '%s\\n' \"$*\" >> \"$GENTLE_AI_STUB_ARGV_LOG\"; fi\n" +
		"if [ \"$2\" = \"inspect-candidate\" ]; then printf '%s' \"$GENTLE_AI_STUB_INSPECT\"; exit 0; fi\n" +
		"if [ \"$2\" = \"capture-result\" ]; then cat > \"$GENTLE_AI_STUB_STDIN_LOG\"; printf '%s' \"$GENTLE_AI_STUB_CAPTURED\"; exit 0; fi\n" +
		"exit 1\n"
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
	command.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GENTLE_AI_STUB_ARGV_LOG="+stub.argvLog,
		"GENTLE_AI_STUB_STDIN_LOG="+stub.stdinLog,
		"GENTLE_AI_STUB_INSPECT="+stub.inspect,
		"GENTLE_AI_STUB_CAPTURED="+stub.captured,
		"REVIEWER_RESULT="+reviewPluginPayloadMarker,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("TypeScript plugin harness failed (%v): %s", err, output)
	}
	return strings.TrimSpace(string(output))
}

func TestReviewPluginUsesIndexedInspectionAndStdinCapture(t *testing.T) {
	argvLog := filepath.Join(t.TempDir(), "argv.log")
	stdinLog := filepath.Join(t.TempDir(), "stdin.log")
	output := runReviewPluginScenario(t, "v2", reviewPluginStub{
		argvLog: argvLog, stdinLog: stdinLog,
		inspect: "frozen patch bytes\n", captured: `{"operation":"review/capture-result"}`,
	})
	var result struct {
		Prompt    string   `json:"prompt"`
		Inspected string   `json:"inspected"`
		Captured  string   `json:"captured"`
		Files     []string `json:"files"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode plugin result: %v: %s", err, output)
	}
	if !strings.Contains(result.Prompt, "changed_path_manifest") || strings.Contains(result.Prompt, "frozen patch bytes") {
		t.Fatalf("plugin rewrote the v2 binding/manifest prompt: %q", result.Prompt)
	}
	if result.Inspected != "frozen patch bytes\n" || result.Captured != `{"operation":"review/capture-result"}` {
		t.Fatalf("inspection/capture output = %#v", result)
	}
	if len(result.Files) != 0 {
		t.Fatalf("reviewer transport wrote a worktree artifact: %v", result.Files)
	}
	argv, err := os.ReadFile(argvLog)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"review inspect-candidate", "--operation patch", "--path-index 0", "review capture-result", "--input -"} {
		if !strings.Contains(string(argv), want) {
			t.Fatalf("native argv missing %q: %s", want, argv)
		}
	}
	payload, err := os.ReadFile(stdinLog)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != reviewPluginPayloadMarker {
		t.Fatalf("capture stdin = %q, want reviewer stdout %q", payload, reviewPluginPayloadMarker)
	}
}

func TestReviewPluginLeavesLegacyOutputUncaptured(t *testing.T) {
	if output := runReviewPluginScenario(t, "legacy", reviewPluginStub{}); output != reviewPluginPayloadMarker {
		t.Fatalf("legacy output = %q, want raw reviewer output", output)
	}
}

func TestReviewPluginRefusesMalformedV2Binding(t *testing.T) {
	if output := runReviewPluginScenario(t, "malformed-v2", reviewPluginStub{}); !strings.Contains(output, "v2 reviewer binding is malformed") {
		t.Fatalf("malformed v2 binding was not refused closed: %q", output)
	}
}

func TestReviewPluginRejectsBackgroundReviewer(t *testing.T) {
	if output := runReviewPluginScenario(t, "background", reviewPluginStub{}); !strings.Contains(output, "foreground") {
		t.Fatalf("background reviewer was not refused: %q", output)
	}
}
