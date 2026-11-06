package organicruntime_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// Unlike the native-adapter proof, this drives Agent through the real client.
// Responses are scripted, so neither an account nor model inference is needed.
// It is not a network-namespace proof; the Docker lane owns that boundary.
func TestClaudeMarkdownSubagentsEnforceEmptyTools(t *testing.T) {
	if testing.Short() || os.Getenv("GENTLE_AI_CLAUDE_SUBAGENT_E2E") != "1" {
		t.Skip("set GENTLE_AI_CLAUDE_SUBAGENT_E2E=1 for the local Claude subagent proof")
	}
	binary, err := exec.LookPath("claude")
	if err != nil {
		t.Fatal(err)
	}
	version, err := exec.Command(binary, "--version").Output()
	if err != nil || strings.TrimSpace(string(version)) != "2.1.289 (Claude Code)" {
		t.Fatalf("Claude version = %q, %v; this proof requires 2.1.289", version, err)
	}
	for _, tc := range []struct {
		name    string
		control string
		canRead bool
	}{
		{name: "review-risk"},
		{name: "review-readability"},
		{name: "review-reliability"},
		{name: "review-resilience"},
		{name: "review-refuter"},
		{name: "probe-explicit-read", control: "tools: Read\n", canRead: true},
		{name: "probe-inherited-read", canRead: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			adapter, err := agents.NewAdapter(model.AgentClaudeCode)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{}); err != nil {
				t.Fatal(err)
			}
			if tc.canRead {
				definition := "---\nname: " + tc.name + "\ndescription: Local tool isolation control.\nmodel: haiku\n" + tc.control + "---\nReturn the requested probe text.\n"
				if err := os.WriteFile(filepath.Join(adapter.SubAgentsDir(home), tc.name+".md"), []byte(definition), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			const sentinel = "CLAUDE_LOCAL_READ_CONTROL_5251"
			probe := filepath.Join(home, "probe.txt")
			if err := os.WriteFile(probe, []byte(sentinel+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			fixture := &claudeSubagentFixture{agent: tc.name, probe: probe}
			server := httptest.NewServer(fixture)
			defer server.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
			defer cancel()
			command := organicCommandContext(ctx, binary,
				"--print", "Delegate the probe to the named custom agent.",
				"--tools", "Agent,Read", "--permission-mode", "dontAsk",
				"--permission-prompts", "none", "--setting-sources", "project",
				"--settings", `{"disableAllHooks":true,"enabledPlugins":{}}`,
				"--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`,
				"--no-chrome", "--no-session-persistence", "--model", "haiku",
				"--system-prompt", "Only delegate through Agent; do not read files yourself.",
				"--output-format", "json")
			command.Dir = home
			command.Env = claudeSubagentEnvironment(home, server.URL)
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("Claude subagent process: %v\n%s", err, output)
			}
			var result struct {
				IsError bool   `json:"is_error"`
				Result  string `json:"result"`
			}
			if err := json.Unmarshal(output, &result); err != nil || result.IsError || result.Result != "PARENT_DONE" {
				t.Fatalf("Claude result = %s, decode error %v", output, err)
			}
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			if len(fixture.errors) != 0 {
				t.Fatalf("local API protocol errors: %v", fixture.errors)
			}
			if fixture.parentCalls != 1 || fixture.parentResult == "" {
				t.Fatalf("Agent invocation missing: calls=%d result=%q", fixture.parentCalls, fixture.parentResult)
			}
			if tc.canRead {
				if !fixture.childRequested || !containsClaudeTool(fixture.childTools, "Read") || fixture.childError || !strings.Contains(fixture.childResult, sentinel) {
					t.Fatalf("read control did not execute: requested=%v tools=%v error=%v result=%q", fixture.childRequested, fixture.childTools, fixture.childError, fixture.childResult)
				}
			} else if fixture.childRequested {
				if len(fixture.childTools) != 0 || !fixture.childError || strings.Contains(fixture.childResult, sentinel) {
					t.Fatalf("empty-tools subagent was not isolated: tools=%v error=%v result=%q", fixture.childTools, fixture.childError, fixture.childResult)
				}
			} else if !fixture.parentError || !strings.Contains(strings.ToLower(fixture.parentResult), "zero tools") {
				t.Fatalf("subagent did not run or fail closed for zero tools: error=%v result=%q", fixture.parentError, fixture.parentResult)
			}
			data, err := os.ReadFile(probe)
			if err != nil || string(data) != sentinel+"\n" {
				t.Fatalf("probe file changed: %q, %v", data, err)
			}
		})
	}
}

func claudeSubagentEnvironment(home, endpoint string) []string {
	// A fresh config directory and synthetic API key avoid reading or refreshing
	// real credentials. Do not forward provider, proxy, or Claude customizations.
	var env []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(key)
		if strings.HasPrefix(upper, "ANTHROPIC_") || strings.HasPrefix(upper, "CLAUDE_") || strings.HasSuffix(upper, "_PROXY") || upper == "NO_PROXY" {
			continue
		}
		env = append(env, entry)
	}
	return append(env,
		"ANTHROPIC_BASE_URL="+endpoint, "ANTHROPIC_API_KEY=local-loopback-key",
		"CLAUDE_CONFIG_DIR="+filepath.Join(home, "client-config"),
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1", "CLAUDE_CODE_FORK_SUBAGENT=0",
		"DISABLE_AUTOUPDATER=1", "NO_PROXY=127.0.0.1,localhost")
}

func containsClaudeTool(tools []string, name string) bool {
	for _, tool := range tools {
		if tool == name {
			return true
		}
	}
	return false
}

type claudeSubagentFixture struct {
	mu             sync.Mutex
	agent, probe   string
	parentCalls    int
	parentResult   string
	parentError    bool
	childRequested bool
	childTools     []string
	childResult    string
	childError     bool
	errors         []string
}

type claudeProbeBlock struct {
	Type      string          `json:"type"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
	Content   json.RawMessage `json:"content"`
}

func (f *claudeSubagentFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Method == http.MethodHead && r.URL.Path == "/api/hello" {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost || r.URL.Path != "/v1/messages" {
		f.errors = append(f.errors, "unexpected endpoint "+r.Method+" "+r.URL.Path)
		http.NotFound(w, r)
		return
	}
	var request struct {
		Model string `json:"model"`
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(&request); err != nil {
		f.errors = append(f.errors, "decode request: "+err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	for i := len(request.Messages) - 1; i >= 0; i-- {
		var blocks []claudeProbeBlock
		if json.Unmarshal(request.Messages[i].Content, &blocks) != nil {
			continue
		}
		for _, block := range blocks {
			if block.Type != "tool_result" {
				continue
			}
			switch block.ToolUseID {
			case "parent_probe":
				f.parentResult, f.parentError = string(block.Content), block.IsError
				writeClaudeProbeResponse(w, request.Model, "", "", nil, "PARENT_DONE")
				return
			case "child_probe":
				f.childResult, f.childError = string(block.Content), block.IsError
				writeClaudeProbeResponse(w, request.Model, "", "", nil, "CHILD_DONE")
				return
			}
		}
	}
	var tools []string
	for _, tool := range request.Tools {
		tools = append(tools, tool.Name)
	}
	if f.parentCalls == 0 {
		if !containsClaudeTool(tools, "Agent") || !containsClaudeTool(tools, "Read") {
			f.errors = append(f.errors, fmt.Sprintf("parent tools = %v; want Agent and Read", tools))
		}
		f.parentCalls++
		writeClaudeProbeResponse(w, request.Model, "parent_probe", "Agent", map[string]any{
			"subagent_type": f.agent, "description": "Probe custom-agent tool isolation",
			"prompt": "Read the probe file at " + f.probe + " and return its text.",
		}, "")
		return
	}
	if f.childRequested {
		f.errors = append(f.errors, "unexpected repeated child request")
		writeClaudeProbeResponse(w, request.Model, "", "", nil, "UNEXPECTED_REQUEST")
		return
	}
	f.childRequested, f.childTools = true, tools
	// Deliberately request Read even when it is not advertised. This challenges
	// runtime enforcement rather than relying on the reviewer's prompt to obey.
	writeClaudeProbeResponse(w, request.Model, "child_probe", "Read", map[string]any{"file_path": f.probe}, "")
}

func writeClaudeProbeResponse(w http.ResponseWriter, model, id, tool string, input map[string]any, text string) {
	w.Header().Set("Content-Type", "text/event-stream")
	emit := func(event string, data any) {
		encoded, _ := json.Marshal(data)
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, encoded)
	}
	emit("message_start", map[string]any{"type": "message_start", "message": map[string]any{
		"id": "msg_probe", "type": "message", "role": "assistant", "model": model,
		"content": []any{}, "stop_reason": nil, "stop_sequence": nil,
		"usage": map[string]int{"input_tokens": 1, "output_tokens": 1},
	}})
	stopReason := "end_turn"
	if tool != "" {
		stopReason = "tool_use"
		emit("content_block_start", map[string]any{"type": "content_block_start", "index": 0,
			"content_block": map[string]any{"type": "tool_use", "id": id, "name": tool, "input": map[string]any{}},
		})
		encoded, _ := json.Marshal(input)
		emit("content_block_delta", map[string]any{"type": "content_block_delta", "index": 0,
			"delta": map[string]any{"type": "input_json_delta", "partial_json": string(encoded)},
		})
	} else {
		emit("content_block_start", map[string]any{"type": "content_block_start", "index": 0,
			"content_block": map[string]any{"type": "text", "text": ""},
		})
		emit("content_block_delta", map[string]any{"type": "content_block_delta", "index": 0,
			"delta": map[string]any{"type": "text_delta", "text": text},
		})
	}
	emit("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
	emit("message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": stopReason, "stop_sequence": nil}, "usage": map[string]int{"output_tokens": 1}})
	emit("message_stop", map[string]any{"type": "message_stop"})
}
