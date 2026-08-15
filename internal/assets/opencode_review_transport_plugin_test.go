package assets

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOpenCodeReviewTransportPluginRelaysOneTaskThroughGo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the relay fixture uses a POSIX shell")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	source, err := Read("opencode/plugins/opencode-review-transport.ts")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "plugin.mts"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	const harness = `import plugin from "./plugin.mts"
const hooks = await plugin({ directory: process.cwd(), worktree: process.cwd() })
const before = { args: { subagent_type: "review-risk", prompt: "Go must receive this original host prompt" } }
await hooks["tool.execute.before"]({ tool: "task", sessionID: "session", callID: "call" }, before)
const after = { output: "untrusted reviewer output", metadata: {} }
await hooks["tool.execute.after"]({ tool: "task", sessionID: "session", callID: "call", args: { subagent_type: "review-risk" } }, after)
console.log(JSON.stringify({ prompt: before.args.prompt, output: after.output }))
`
	if err := os.WriteFile(filepath.Join(root, "harness.mts"), []byte(harness), 0o600); err != nil {
		t.Fatal(err)
	}
	const relay = `#!/bin/sh
IFS= read -r start
printf '%s\n' "$start" >> "$GENTLE_AI_RELAY_LOG"
printf '%s\n' '{"schema":"gentle-ai.provider-transport/v1","operation":"prompt","nonce":"nonce","prompt":"Go-materialized immutable prompt"}'
IFS= read -r complete
printf '%s\n' "$complete" >> "$GENTLE_AI_RELAY_LOG"
printf '%s\n' '{"schema":"gentle-ai.provider-transport/v1","operation":"result","output":"captured"}'
`
	if err := os.WriteFile(filepath.Join(bin, "gentle-ai"), []byte(relay), 0o700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "relay.log")
	command := exec.Command(node, "harness.mts")
	command.Dir = root
	command.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "GENTLE_AI_RELAY_LOG="+logPath)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("transport plugin harness failed: %v\n%s", err, output)
	}
	var result struct {
		Prompt string `json:"prompt"`
		Output string `json:"output"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode relay harness output %q: %v", output, err)
	}
	if result.Prompt != "Go-materialized immutable prompt" || result.Output != "captured" {
		t.Fatalf("relay result = %#v", result)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(log) != "{\"schema\":\"gentle-ai.provider-transport/v1\",\"operation\":\"start\",\"prompt\":\"Go must receive this original host prompt\"}\n{\"schema\":\"gentle-ai.provider-transport/v1\",\"operation\":\"complete\",\"nonce\":\"nonce\",\"output\":\"untrusted reviewer output\"}\n" {
		t.Fatalf("relay frames = %q", log)
	}
}
