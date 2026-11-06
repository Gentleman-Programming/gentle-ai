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

// runPluginHarness executes a Node harness written next to a copy of the real
// skill-registry plugin source and returns the combined harness output. When
// envOverrides is non-empty, those variables replace the inherited ones so a
// test can control, for example, the child PATH.
func runPluginHarness(t *testing.T, harness string, envOverrides map[string]string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the harness uses POSIX shell semantics")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	source, err := Read("opencode/plugins/skill-registry.ts")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "plugin.mts"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "harness.mts"), []byte(harness), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(node, "harness.mts")
	command.Dir = root
	if len(envOverrides) > 0 {
		childEnv := make([]string, 0, len(os.Environ())+len(envOverrides))
		for _, kv := range os.Environ() {
			key, _, _ := strings.Cut(kv, "=")
			if _, overridden := envOverrides[key]; overridden {
				continue
			}
			childEnv = append(childEnv, kv)
		}
		for key, value := range envOverrides {
			childEnv = append(childEnv, key+"="+value)
		}
		command.Env = childEnv
	}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("skill-registry harness failed: %v\n%s", err, output)
	}
	return string(output)
}

func decodeHarnessJSON(t *testing.T, output string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(output), target); err != nil {
		t.Fatalf("decode harness output %q: %v", output, err)
	}
}

// assertSpawnEnoentDiagnostic pins the spawn-ENOENT diagnostic contract: a
// single physical line, causally neutral about the failure (a live
// Bun/OpenCode occurrence had the binary and PATH present, and the error
// does not identify the missing resource, so the plugin must never claim
// PATH absence or blame the binary), naming the OpenCode runtime context,
// and suggesting a manual continuation that preserves --no-gitignore via a
// literal <project> placeholder instead of interpolated shell quoting.
func assertSpawnEnoentDiagnostic(t *testing.T, label, got string) {
	t.Helper()
	if strings.ContainsAny(got, "\r\n") {
		t.Errorf("%s must be a single physical line: %q", label, got)
	}
	if strings.Contains(got, "was not found on the PATH") {
		t.Errorf("%s must not assert PATH absence as the cause: %q", label, got)
	}
	if !strings.Contains(got, "could not complete the gentle-ai skill-registry refresh") {
		t.Errorf("%s must state the refresh could not be completed: %q", label, got)
	}
	if !strings.Contains(got, "the missing resource was not identified") {
		t.Errorf("%s must state the missing resource was not identified: %q", label, got)
	}
	if strings.Contains(got, "gentle-ai executable") {
		t.Errorf("%s must not attribute the missing resource to the gentle-ai executable: %q", label, got)
	}
	if !strings.Contains(got, "OpenCode") {
		t.Errorf("%s must name the OpenCode runtime context: %q", label, got)
	}
	if !strings.Contains(got, "Once the OpenCode runtime environment is valid") {
		t.Errorf("%s must use neutral follow-up guidance: %q", label, got)
	}
	if !strings.Contains(got, "gentle-ai skill-registry refresh --no-gitignore --cwd <project>") {
		t.Errorf("%s must suggest a manual continuation matching the automatic refresh: %q", label, got)
	}
	if strings.Contains(got, "--cwd '") {
		t.Errorf("%s must not interpolate a shell-quoted cwd into the suggested command: %q", label, got)
	}
	if !strings.Contains(got, "does not block startup") {
		t.Errorf("%s must keep the best-effort statement: %q", label, got)
	}
}

// TestSkillRegistryPluginDescribesRefreshFailures verifies the
// describeRefreshFailure contract against real control-character fixtures.
func TestSkillRegistryPluginDescribesRefreshFailures(t *testing.T) {
	const harness = `import { describeRefreshFailure } from "./plugin.mts"

// Fixtures build real control characters at runtime so the sanitizer is
// exercised against actual CR/LF/NUL bytes, never JavaScript escape literals.
const CR = String.fromCharCode(13)
const LF = String.fromCharCode(10)
const NUL = String.fromCharCode(0)

const cwdNormal = "/Users/me/repos/example"
const cwdRealControlChars = "/tmp/weird" + LF + "name" + NUL + "dir"

const errEnoentSpawn = Object.assign(new Error("spawn gentle-ai ENOENT"), {
  code: "ENOENT", syscall: "spawn gentle-ai", path: "gentle-ai",
})
const errEnoentNoSyscall = Object.assign(new Error("spawn gentle-ai ENOENT"), {
  code: "ENOENT",
})
const errEnoentAccess = Object.assign(new Error("ENOENT: no such file or directory, access '/nonexistent'"), {
  code: "ENOENT", syscall: "access", path: "/nonexistent",
})
const errRealControlMessage = Object.assign(
  new Error("first" + CR + LF + "second" + NUL + "third"),
  { code: "ECONNREFUSED" },
)
// Plain structured errors (not Error instances) must keep their message.
const errStructuredPlain = {
  code: "EIO",
  message: "disk" + CR + LF + "failure" + NUL + "now",
}

console.log(JSON.stringify({
  binaryMissing:          describeRefreshFailure(errEnoentSpawn, cwdNormal),
  binaryMissingNoSyscall: describeRefreshFailure(errEnoentNoSyscall, cwdNormal),
  invalidCwd:             describeRefreshFailure(errEnoentAccess, cwdNormal),
  realControlChars:       describeRefreshFailure(errEnoentSpawn, cwdRealControlChars),
  controlCharMessage:     describeRefreshFailure(errRealControlMessage, cwdNormal),
  structuredPlain:        describeRefreshFailure(errStructuredPlain, cwdNormal),
}))
`
	output := runPluginHarness(t, harness, nil)
	var result map[string]string
	decodeHarnessJSON(t, output, &result)

	// Every diagnostic must stay on one physical row and keep the stable
	// prefix, even with real CR/LF/NUL input bytes.
	for label, got := range result {
		if strings.ContainsAny(got, "\r\n") {
			t.Errorf("%s must be a single physical line: %q", label, got)
		}
		if !strings.HasPrefix(got, "[skill-registry]") {
			t.Errorf("%s must keep the [skill-registry] prefix: %q", label, got)
		}
	}

	// Spawn ENOENT (with or without a syscall field) is classified neutrally.
	for _, label := range []string{"binaryMissing", "binaryMissingNoSyscall"} {
		assertSpawnEnoentDiagnostic(t, label, result[label])
	}

	// Non-spawn ENOENT (access/stat on the working directory) must not be
	// blamed on the binary or reported as an incomplete refresh, and must
	// name the cwd instead.
	if got := result["invalidCwd"]; !strings.Contains(got, "could not access the working directory") || strings.Contains(got, "could not complete the gentle-ai skill-registry refresh") {
		t.Errorf("invalidCwd must name the cwd, not the binary: %q", got)
	}

	// Real LF/NUL bytes in the cwd must be JSON-escaped, keeping the
	// diagnostic on one physical row without activating shell metacharacters.
	if got := result["realControlChars"]; true {
		if !strings.Contains(got, "weird\\nname") {
			t.Errorf("realControlChars must escape the real LF: %q", got)
		}
		if !strings.Contains(got, "\\u0000") {
			t.Errorf("realControlChars must escape the real NUL: %q", got)
		}
	}

	// Real CR/LF/NUL bytes in a foreign error message must be collapsed so
	// the diagnostic stays on one physical row.
	if got := result["controlCharMessage"]; !strings.Contains(got, "first second third") {
		t.Errorf("controlCharMessage must collapse real CR/LF/NUL to one line: %q", got)
	}

	// A plain structured error (not an Error instance) must keep its message
	// with real CR/LF/NUL collapsed, instead of degrading to [object Object].
	if got := result["structuredPlain"]; !strings.Contains(got, "code=EIO") || !strings.Contains(got, "disk failure now") || strings.Contains(got, "[object Object]") {
		t.Errorf("structuredPlain must keep the structured message on one line: %q", got)
	}
}

// TestSkillRegistryPluginMissingExecutableLifecycle executes the real plugin
// with a real project marker and a PATH that cannot resolve gentle-ai. It
// proves the plugin call returns without waiting for the child failure and
// that the eventual asynchronous diagnostic is exactly one actionable line.
func TestSkillRegistryPluginMissingExecutableLifecycle(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(project, ".atl"), 0o755); err != nil {
		t.Fatal(err)
	}
	emptyPath := filepath.Join(root, "empty-bin")
	if err := os.MkdirAll(emptyPath, 0o755); err != nil {
		t.Fatal(err)
	}
	const harness = `import { SkillRegistryPlugin } from "./plugin.mts"

const diag = []
// Capture diagnostics without forwarding them, so stdout stays pure JSON.
console.error = (...args) => { diag.push(args.join(" ")) }

const project = process.env.PROJECT_DIR
const started = Date.now()
const result = await SkillRegistryPlugin({ worktree: project, directory: project })
const returnMs = Date.now() - started
// Discriminates non-blocking from merely-fast failure: a regression that
// awaits the child failure inside the plugin call would already have a
// diagnostic here.
const diagAtReturn = diag.length

// Bounded poll for the eventual asynchronous diagnostic.
let waitedMs = 0
while (diag.length === 0 && waitedMs < 15000) {
  await new Promise((resolve) => setTimeout(resolve, 25))
  waitedMs += 25
}

console.log(JSON.stringify({
  result: JSON.stringify(result),
  returnMs,
  waitedMs,
  diagAtReturn,
  diag,
}))
`
	output := runPluginHarness(t, harness, map[string]string{
		"PATH":        emptyPath,
		"PROJECT_DIR": project,
	})
	var result struct {
		Result       string   `json:"result"`
		ReturnMs     int64    `json:"returnMs"`
		WaitedMs     int64    `json:"waitedMs"`
		DiagAtReturn int      `json:"diagAtReturn"`
		Diag         []string `json:"diag"`
	}
	decodeHarnessJSON(t, output, &result)

	// The plugin call itself returned the best-effort marker object.
	if result.Result != "{}" {
		t.Errorf("plugin must return its best-effort result object: %q", result.Result)
	}
	// The plugin call returned without waiting for the child failure: the
	// spawn rejection is asynchronous and must never block startup. The
	// zero-diagnostic snapshot discriminates non-blocking from a merely
	// fast failure.
	if result.ReturnMs > 5000 {
		t.Errorf("plugin call must return promptly, got %dms", result.ReturnMs)
	}
	if result.DiagAtReturn != 0 {
		t.Errorf("plugin call must return before the child failure diagnostic, got %d diagnostic(s) at return", result.DiagAtReturn)
	}
	// Exactly one actionable diagnostic must eventually arrive.
	if len(result.Diag) != 1 {
		t.Fatalf("expected exactly one diagnostic, got %d: %q", len(result.Diag), result.Diag)
	}
	assertSpawnEnoentDiagnostic(t, "diag", result.Diag[0])
	if !strings.Contains(result.Diag[0], `"`+project+`"`) {
		t.Errorf("diagnostic must include the JSON-quoted project directory: %q", result.Diag[0])
	}
	_ = result.WaitedMs // bounded-poll telemetry, asserted via Diag above
}
