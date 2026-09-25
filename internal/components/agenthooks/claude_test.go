package agenthooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

func TestClaudeSkillRegistryCommandForWindows(t *testing.T) {
	const want = `powershell -NoProfile -Command 'if (Test-Path env:CLAUDE_PROJECT_DIR) { $dir = $env:CLAUDE_PROJECT_DIR } else { $dir = $PWD }; gentle-ai skill-registry refresh --quiet --no-gitignore --cwd "$dir"; exit 0'`
	if got := claudeSkillRegistryCommand("windows"); got != want {
		t.Fatalf("Windows hook command = %q, want %q", got, want)
	}
}

func TestClaudeSkillRegistryWindowsMigration(t *testing.T) {
	adapter, err := agents.NewAdapter(model.AgentClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	canonical := claudeSkillRegistryCommand("windows")
	for _, tc := range []struct {
		name, userPromptHooks string
		wantPromptHooks       int
	}{
		{"legacy alone", `[{"matcher":"","hooks":[{"type":"command","command":` + mustJSON(t, claudeLegacySkillRegistryCommand) + `}]}]`, 1},
		{"legacy alongside canonical", `[{"matcher":"","hooks":[{"type":"command","command":` + mustJSON(t, claudeLegacySkillRegistryCommand) + `},{"type":"command","command":` + mustJSON(t, canonical) + `}]}]`, 1},
		{"canonical only in another event", `[{"matcher":"","hooks":[{"type":"command","command":"echo keep"}]}]`, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			settingsPath := adapter.SettingsPath(home)
			if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
				t.Fatal(err)
			}
			seed := `{"hooks":{"SessionStart":[{"matcher":"","hooks":[{"type":"command","command":` + mustJSON(t, canonical) + `}]}],"UserPromptSubmit":` + tc.userPromptHooks + `}}`
			if err := os.WriteFile(settingsPath, []byte(seed), 0o644); err != nil {
				t.Fatal(err)
			}
			result, err := installSkillRegistry(home, adapter, "windows")
			if err != nil || !result.Changed {
				t.Fatalf("first Windows install: %+v %v", result, err)
			}
			data, err := os.ReadFile(settingsPath)
			if err != nil {
				t.Fatal(err)
			}
			var root struct {
				Hooks map[string][]struct {
					Hooks []struct {
						Command string `json:"command"`
					} `json:"hooks"`
				} `json:"hooks"`
			}
			if err := json.Unmarshal(data, &root); err != nil {
				t.Fatal(err)
			}
			if len(root.Hooks["UserPromptSubmit"]) != tc.wantPromptHooks || len(root.Hooks["SessionStart"]) != 1 || root.Hooks["SessionStart"][0].Hooks[0].Command != canonical {
				t.Fatalf("unexpected hook events after migration: %s", data)
			}
			var commands []string
			for _, entry := range root.Hooks["UserPromptSubmit"] {
				for _, hook := range entry.Hooks {
					commands = append(commands, hook.Command)
				}
			}
			if strings.Count(strings.Join(commands, "\n"), canonical) != 1 || strings.Contains(strings.Join(commands, "\n"), claudeLegacySkillRegistryCommand) {
				t.Fatalf("canonical command missing or legacy command retained: %q", commands)
			}
			second, err := installSkillRegistry(home, adapter, "windows")
			if err != nil || second.Changed {
				t.Fatalf("second Windows install: %+v %v", second, err)
			}
			after, err := os.ReadFile(settingsPath)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatalf("migration not byte-idempotent: %v", err)
			}
		})
	}
}

func mustJSON(t *testing.T, value string) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestClaudeUserPromptSubmitHookExecutesPowerShellCommandWithSpecialChars
// exercises the generated hook command, not a reconstructed approximation.
func TestClaudeUserPromptSubmitHookExecutesPowerShellCommandWithSpecialChars(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeName := "gentle-ai"
	if runtime.GOOS == "windows" {
		fakeName = "gentle-ai.exe"
	}
	testBinary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// A copy, not a symlink, works on Windows too.
	if err := copyFakeExecutable(testBinary, filepath.Join(bin, fakeName)); err != nil {
		t.Fatal(err)
	}
	projectDir := filepath.Join(root, "Weird Path", "John's project", "$dollar (paren) & and")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	adapter, err := agents.NewAdapter(model.AgentClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	result, err := InstallSkillRegistry(root, adapter)
	if err != nil || !result.Changed {
		t.Fatalf("install skill-registry hook: %+v %v", result, err)
	}
	settingsPath := adapter.SettingsPath(root)
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Hooks struct {
			UserPromptSubmit []struct {
				Matcher string `json:"matcher"`
				Hooks   []struct {
					Type    string `json:"type"`
					Command string `json:"command"`
				} `json:"hooks"`
			} `json:"UserPromptSubmit"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parse generated settings %q: %v\n%s", settingsPath, err, data)
	}
	if len(settings.Hooks.UserPromptSubmit) != 1 || len(settings.Hooks.UserPromptSubmit[0].Hooks) != 1 {
		t.Fatalf("unexpected generated hook shape:\n%s", data)
	}
	settingsCommand := settings.Hooks.UserPromptSubmit[0].Hooks[0].Command

	// PowerShell strips outer quotes from -Command; pass the body directly.
	var execName string
	var execArgs []string
	if runtime.GOOS == "windows" {
		const wrapper = `powershell -NoProfile -Command '`
		if !strings.HasPrefix(settingsCommand, wrapper) || !strings.HasSuffix(settingsCommand, "'") {
			t.Fatalf("Windows settings command lost the powershell wrapper: %q", settingsCommand)
		}
		body := strings.TrimSuffix(strings.TrimPrefix(settingsCommand, wrapper), "'")
		execName = "powershell"
		execArgs = []string{"-NoProfile", "-Command", body}
	} else {
		execName = "/bin/sh"
		execArgs = []string{"-c", settingsCommand}
	}
	if _, err := exec.LookPath(execName); err != nil {
		if runtime.GOOS == "windows" {
			t.Fatalf("powershell.exe not found on PATH on a Windows host: %v", err)
		}
		t.Skipf("%s is not on PATH; cannot execute the hook command on this host", execName)
	}

	fakeLogPath := filepath.Join(root, "fake-gentle-ai.log")
	childEnv := make([]string, 0, len(os.Environ())+4)
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "PATH=") ||
			strings.HasPrefix(kv, "GENTLE_AI_FAKE_LOG=") ||
			strings.HasPrefix(kv, "GENTLE_AI_FAKE_EXIT=") {
			continue
		}
		childEnv = append(childEnv, kv)
	}
	childEnv = append(childEnv,
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GENTLE_AI_FAKE_LOG="+fakeLogPath,
		"GENTLE_AI_FAKE_EXIT=7",
		"CLAUDE_PROJECT_DIR="+projectDir,
	)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, execName, execArgs...)
	cmd.Dir = projectDir
	cmd.Env = childEnv
	stdinPayload := "hook stdin probe from TestClaudeUserPromptSubmitHook\n"
	cmd.Stdin = strings.NewReader(stdinPayload)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("hook command exited non-zero (a failing child must not break the hook):\ncommand: %s\nerr: %v\noutput:\n%s", settingsCommand, err, output)
	}
	logBytes, err := os.ReadFile(fakeLogPath)
	if err != nil {
		t.Fatalf("fake gentle-ai log %q not written: %v\ncommand output:\n%s", fakeLogPath, err, output)
	}
	logText := string(logBytes)
	lines := strings.Split(logText, "\n")
	wantArgv := []string{"skill-registry", "refresh", "--quiet", "--no-gitignore", "--cwd", projectDir}
	gotArgv := make([]string, 0, len(wantArgv))
	for _, line := range lines {
		if !strings.HasPrefix(line, "argv[") {
			continue
		}
		eq := strings.Index(line, "=")
		if eq < 0 {
			t.Fatalf("malformed argv log line %q:\n%s", line, logText)
		}
		gotArgv = append(gotArgv, line[eq+1:])
	}
	if !reflect.DeepEqual(gotArgv, wantArgv) {
		t.Fatalf("fake gentle-ai argv mismatch (special-character CLAUDE_PROJECT_DIR did not survive argument reconstruction):\n got: %#v\nwant: %#v\nlog:\n%s\ncommand output:\n%s", gotArgv, wantArgv, logText, output)
	}
	if !strings.Contains(logText, fmt.Sprintf("stdin-bytes=%d", len(stdinPayload))) || !strings.Contains(logText, stdinPayload) {
		t.Fatalf("fake gentle-ai did not record the stdin payload:\nlog:\n%s\ncommand output:\n%s", logText, output)
	}
	if !strings.Contains(logText, "exit=7\n") {
		t.Fatalf("fake gentle-ai did not record exit code 7:\nlog:\n%s\ncommand output:\n%s", logText, output)
	}
	if !strings.Contains(logText, "env-CLAUDE_PROJECT_DIR="+projectDir+"\n") {
		t.Fatalf("CLAUDE_PROJECT_DIR not propagated into the fake:\nlog:\n%s\ncommand output:\n%s", logText, output)
	}
}

func copyFakeExecutable(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}

func TestClaudeRetainedHooksPreserveExistingEvents(t *testing.T) {
	adapter, err := agents.NewAdapter(model.AgentClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	path := adapter.SettingsPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	seed := `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo keep"}]}],"Stop":[{"matcher":"","hooks":[{"type":"command","command":"echo existing stop"}]}],"SessionStart":[{"matcher":"startup","hooks":[{"type":"command","command":"echo existing session-start"}]}]}}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := InstallRetainedClaudeHooks(home, adapter)
	if err != nil || !first.Changed {
		t.Fatalf("first install: %+v %v", first, err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"echo keep", "echo existing stop", "echo existing session-start"} {
		if !strings.Contains(string(before), expected) {
			t.Fatalf("existing hook %q lost: %s", expected, before)
		}
	}
	for _, tc := range []struct {
		command string
		count   int
	}{{"gentle-ai review stop-hook --agent claude-code", 2}, {"gentle-ai telemetry runtime claude --json", 2}, {`"async": true`, 2}, {`"matcher": "startup|resume|clear|compact"`, 1}} {
		if strings.Count(string(before), tc.command) != tc.count {
			t.Fatalf("%q count = %d, want %d", tc.command, strings.Count(string(before), tc.command), tc.count)
		}
	}
	second, err := InstallRetainedClaudeHooks(home, adapter)
	if err != nil || second.Changed {
		t.Fatalf("second install: %+v %v", second, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, before) {
		t.Fatalf("repeat changed bytes: %v", err)
	}
}

func TestClaudeRetainedHooksRejectMalformedSchemaWithoutWrite(t *testing.T) {
	adapter, err := agents.NewAdapter(model.AgentClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	for _, seed := range []string{`{"hooks":{"Stop":{}}}`, `{"hooks":{"SubagentStop":{}}}`, `{"hooks":{"SessionStart":{}}}`, `{"hooks":[]}`, `{"hooks":`} {
		t.Run(seed, func(t *testing.T) {
			home := t.TempDir()
			path := adapter.SettingsPath(home)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
				t.Fatal(err)
			}
			if result, err := InstallRetainedClaudeHooks(home, adapter); err == nil || result.Changed {
				t.Fatalf("invalid settings accepted: %+v %v", result, err)
			}
			if after, err := os.ReadFile(path); err != nil || string(after) != seed {
				t.Fatalf("invalid settings changed: %q %v", after, err)
			}
		})
	}
}

func TestClaudeRetainedHooksRejectsNonRegularPath(t *testing.T) {
	adapter, err := agents.NewAdapter(model.AgentClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	path := adapter.SettingsPath(home)
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	if result, err := InstallRetainedClaudeHooks(home, adapter); err == nil || result.Changed {
		t.Fatalf("non-regular path accepted: %+v %v", result, err)
	}
}

func TestClaudeRetainedHooksWithoutSDD(t *testing.T) {
	adapter, err := agents.NewAdapter(model.AgentClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	path := adapter.SettingsPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"hooks":{"Stop":[{"matcher":"","hooks":[{"type":"command","command":"echo keep"}]}]}}`)
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	first, err := InstallRetainedClaudeHooks(home, adapter)
	if err != nil || !first.Changed {
		t.Fatalf("first: %+v %v", first, err)
	}
	want := []byte("{\n  \"hooks\": {\n    \"SessionStart\": [\n      {\n        \"hooks\": [\n          {\n            \"command\": \"gentle-ai review stop-hook --agent claude-code\",\n            \"timeout\": 30,\n            \"type\": \"command\"\n          }\n        ],\n        \"matcher\": \"startup|resume|clear|compact\"\n      }\n    ],\n    \"Stop\": [\n      {\n        \"hooks\": [\n          {\n            \"command\": \"echo keep\",\n            \"type\": \"command\"\n          }\n        ],\n        \"matcher\": \"\"\n      },\n      {\n        \"hooks\": [\n          {\n            \"command\": \"gentle-ai review stop-hook --agent claude-code\",\n            \"timeout\": 60,\n            \"type\": \"command\"\n          }\n        ],\n        \"matcher\": \"\"\n      },\n      {\n        \"hooks\": [\n          {\n            \"async\": true,\n            \"command\": \"gentle-ai telemetry runtime claude --json\",\n            \"timeout\": 5,\n            \"type\": \"command\"\n          }\n        ],\n        \"matcher\": \"\"\n      }\n    ],\n    \"SubagentStop\": [\n      {\n        \"hooks\": [\n          {\n            \"async\": true,\n            \"command\": \"gentle-ai telemetry runtime claude --json\",\n            \"timeout\": 5,\n            \"type\": \"command\"\n          }\n        ],\n        \"matcher\": \"\"\n      }\n    ]\n  }\n}\n")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("byte mismatch:\n%s", got)
	}
	for _, expected := range []struct {
		command string
		count   int
	}{{"gentle-ai review stop-hook --agent claude-code", 2}, {"gentle-ai telemetry runtime claude --json", 2}, {`"async": true`, 2}, {`"matcher": "startup|resume|clear|compact"`, 1}} {
		if strings.Count(string(got), expected.command) != expected.count {
			t.Fatalf("%q count = %d, want %d", expected.command, strings.Count(string(got), expected.command), expected.count)
		}
	}
	second, err := InstallRetainedClaudeHooks(home, adapter)
	if err != nil || second.Changed {
		t.Fatalf("second: %+v %v", second, err)
	}
	got, err = os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("idempotent bytes: %v", err)
	}
}
