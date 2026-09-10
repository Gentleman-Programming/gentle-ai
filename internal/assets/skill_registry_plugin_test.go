package assets

import (
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

// TestSkillRegistryPluginDescribesRefreshFailures verifies that the
// skill-registry plugin's `describeRefreshFailure` helper emits a single
// actionable line and distinguishes a missing-binary ENOENT from an
// invalid-cwd ENOENT, per decode2's CHANGES_REQUESTED on PR #3048.
func TestSkillRegistryPluginDescribesRefreshFailures(t *testing.T) {
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
	if err := os.WriteFile(root+"/plugin.mts", []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	const harness = `import { describeRefreshFailure } from "./plugin.mts"

const cwdNormal  = "/Users/me/repos/example"
const cwdUnsafe  = "/tmp/weird\\nname\\u0000; rm -rf"

const errEnoentSpawn = Object.assign(new Error("spawn gentle-ai ENOENT"), {
  code: "ENOENT", syscall: "spawn gentle-ai", path: "gentle-ai",
})
const errEnoentAccess = Object.assign(new Error("ENOENT: no such file or directory, access '/nonexistent'"), {
  code: "ENOENT", syscall: "access", path: "/nonexistent",
})
const errMultiLineOther = Object.assign(new Error("first line\\nsecond line"), {
  code: "ECONNREFUSED",
})

const out1 = describeRefreshFailure(errEnoentSpawn,    cwdNormal)
const out2 = describeRefreshFailure(errEnoentAccess,   cwdNormal)
const out3 = describeRefreshFailure(errMultiLineOther, cwdNormal)
const out4 = describeRefreshFailure(errEnoentSpawn,    cwdUnsafe)

console.log(JSON.stringify({
  binaryMissing:        out1,
  invalidCwd:           out2,
  otherErrorMultiLine:  out3,
  binaryMissingUnsafeCwd: out4,
}))
`
	if err := os.WriteFile(root+"/harness.mts", []byte(harness), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(node, "harness.mts")
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("skill-registry harness failed: %v\n%s", err, output)
	}
	var result struct {
		BinaryMissing          string `json:"binaryMissing"`
		InvalidCwd             string `json:"invalidCwd"`
		OtherErrorMultiLine    string `json:"otherErrorMultiLine"`
		BinaryMissingUnsafeCwd string `json:"binaryMissingUnsafeCwd"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode harness output %q: %v", output, err)
	}

	// Every output must be a single actionable line (no embedded newline).
	for _, c := range []struct {
		label, got string
	}{
		{"binaryMissing", result.BinaryMissing},
		{"invalidCwd", result.InvalidCwd},
		{"otherErrorMultiLine", result.OtherErrorMultiLine},
		{"binaryMissingUnsafeCwd", result.BinaryMissingUnsafeCwd},
	} {
		if strings.ContainsAny(c.got, "\r\n") {
			t.Errorf("%s must not embed a newline: %q", c.label, c.got)
		}
		if !strings.HasPrefix(c.got, "[skill-registry]") {
			t.Errorf("%s must keep the existing prefix: %q", c.label, c.got)
		}
	}

	// ENOENT from a spawn syscall must be diagnosed as a missing binary and
	// name the manual continuation command.
	if !strings.Contains(result.BinaryMissing, "gentle-ai executable was not found on the PATH") {
		t.Errorf("binaryMissing must name the missing binary: %q", result.BinaryMissing)
	}
	if !strings.Contains(result.BinaryMissing, "gentle-ai skill-registry refresh --cwd") {
		t.Errorf("binaryMissing must include the manual continuation command: %q", result.BinaryMissing)
	}

	// ENOENT from any non-spawn syscall (e.g. access/stat on the cwd) must
	// NOT be blamed on the binary.
	if strings.Contains(result.InvalidCwd, "executable was not found on the PATH") {
		t.Errorf("invalidCwd must not falsely blame the binary: %q", result.InvalidCwd)
	}
	if !strings.Contains(result.InvalidCwd, "could not access the working directory") {
		t.Errorf("invalidCwd must name the cwd: %q", result.InvalidCwd)
	}

	// Non-ENOENT failures must surface the error code and keep every error
	// message on a single line.
	if !strings.Contains(result.OtherErrorMultiLine, "code=ECONNREFUSED") {
		t.Errorf("otherErrorMultiLine must include the error code: %q", result.OtherErrorMultiLine)
	}
	if strings.Contains(result.OtherErrorMultiLine, "\n") {
		t.Errorf("otherErrorMultiLine must collapse multi-line messages: %q", result.OtherErrorMultiLine)
	}

	// The cwd in shell examples must be sanitized: no embedded newline or
	// shell metacharacter.
	if strings.Contains(result.BinaryMissingUnsafeCwd, "\n") {
		t.Errorf("binaryMissingUnsafeCwd must not embed a newline: %q", result.BinaryMissingUnsafeCwd)
	}
	if !strings.HasSuffix(result.BinaryMissingUnsafeCwd, ".") {
		t.Errorf("binaryMissingUnsafeCwd must keep the trailing best-effort suffix: %q", result.BinaryMissingUnsafeCwd)
	}
	if !strings.Contains(result.BinaryMissingUnsafeCwd, "weird\\\\nname") {
		t.Errorf("binaryMissingUnsafeCwd must embed the unsafe cwd via JSON.stringify so shell metacharacters are not active: %q", result.BinaryMissingUnsafeCwd)
	}
}
