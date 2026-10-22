package organicruntime_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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

// A fork inherits its parent's tools and context; it is not a Markdown reviewer.
func TestClaudeForkAndNamedReviewerToolBoundary(t *testing.T) {
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
		canRead bool
	}{
		{name: "fork", canRead: true},
		{name: "review-risk"},
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
			const sentinel = "CLAUDE_FORK_READ_CONTROL_5251"
			const marker = "CLAUDE_PARENT_ONLY_CONTEXT_5251"
			probe := filepath.Join(home, "probe.txt")
			if err := os.WriteFile(probe, []byte(sentinel+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			fixture := &claudeSubagentFixture{agent: tc.name, probe: probe}
			var handlerMu sync.Mutex
			var childContext bool
			server := httptest.NewServer(claudeForkComparisonHandler(fixture, marker, &childContext, &handlerMu))
			defer server.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
			defer cancel()
			command := organicCommandContext(ctx, binary,
				"--print", "Parent-only context marker: "+marker+". Delegate the probe.",
				"--tools", "Agent,Read", "--permission-mode", "dontAsk",
				"--permission-prompts", "none", "--setting-sources", "project",
				"--settings", `{"disableAllHooks":true,"enabledPlugins":{}}`,
				"--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`,
				"--no-chrome", "--no-session-persistence", "--model", "haiku",
				"--system-prompt", "Only delegate through Agent; do not read files yourself.",
				"--output-format", "stream-json", "--verbose", "--forward-subagent-text")
			command.Dir = home
			for _, entry := range claudeSubagentEnvironment(home, server.URL) {
				if !strings.HasPrefix(entry, "CLAUDE_CODE_FORK_SUBAGENT=") {
					command.Env = append(command.Env, entry)
				}
			}
			// Fork mode remains enabled. Foreground scheduling lets the CLI await
			// the child before the scripted parent ends its turn.
			command.Env = append(command.Env,
				"CLAUDE_CODE_FORK_SUBAGENT=1", "CLAUDE_CODE_DISABLE_BACKGROUND_TASKS=1")
			var stdout, stderr bytes.Buffer
			command.Stdout, command.Stderr = &stdout, &stderr
			if err := command.Run(); err != nil {
				t.Fatalf("comparison process: %v\n%s", err, stderr.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("unexpected stderr: %s", stderr.String())
			}
			var parentDone, childRead bool
			scanner := bufio.NewScanner(&stdout)
			scanner.Buffer(make([]byte, 4096), 8<<20)
			for scanner.Scan() {
				var event struct {
					Type    string `json:"type"`
					Parent  string `json:"parent_tool_use_id"`
					Result  string `json:"result"`
					Error   bool   `json:"is_error"`
					Message struct {
						Content []struct {
							Type string `json:"type"`
							ID   string `json:"id"`
							Name string `json:"name"`
						} `json:"content"`
					} `json:"message"`
				}
				if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
					t.Fatalf("invalid client event: %v", err)
				}
				if event.Type == "result" && event.Parent == "" {
					parentDone = !event.Error && event.Result == "PARENT_DONE"
				}
				if event.Parent == "parent_probe" && event.Type == "assistant" {
					for _, block := range event.Message.Content {
						if block.Type == "tool_use" && block.ID == "child_probe" && block.Name == "Read" {
							childRead = true
						}
					}
				}
			}
			if err := scanner.Err(); err != nil {
				t.Fatal(err)
			}
			handlerMu.Lock()
			defer handlerMu.Unlock()
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			if !parentDone || len(fixture.errors) != 0 || fixture.parentCalls != 1 {
				t.Fatalf("parent completion=%v calls=%d protocol errors=%v", parentDone, fixture.parentCalls, fixture.errors)
			}
			if tc.canRead {
				if !fixture.childRequested || !childRead || !childContext || !containsClaudeTool(fixture.childTools, "Read") || fixture.childError || !strings.Contains(fixture.childResult, sentinel) {
					t.Fatalf("fork proof missing: child=%v readTrace=%v context=%v tools=%v error=%v sentinel=%v parentError=%v asyncAck=%v", fixture.childRequested, childRead, childContext, fixture.childTools, fixture.childError, strings.Contains(fixture.childResult, sentinel), fixture.parentError, strings.Contains(fixture.parentResult, "Async agent launched"))
				}
			} else if fixture.childRequested {
				if childContext || len(fixture.childTools) != 0 || !fixture.childError || strings.Contains(fixture.childResult, sentinel) {
					t.Fatalf("named reviewer inherited tools/context: tools=%v context=%v error=%v", fixture.childTools, childContext, fixture.childError)
				}
			} else if !fixture.parentError || !strings.Contains(strings.ToLower(fixture.parentResult), "zero tools") {
				t.Fatalf("named reviewer did not fail closed: %q", fixture.parentResult)
			}
			data, err := os.ReadFile(probe)
			if err != nil || string(data) != sentinel+"\n" {
				t.Fatalf("probe file changed: %q, %v", data, err)
			}
		})
	}
}

func claudeForkComparisonHandler(fixture *claudeSubagentFixture, marker string, childContext *bool, handlerMu *sync.Mutex) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerMu.Lock()
		defer handlerMu.Unlock()
		var body []byte
		if r.Method == http.MethodPost {
			data, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			body = data
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		var request struct {
			Model string `json:"model"`
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
			Messages json.RawMessage `json:"messages"`
		}
		if r.Method == http.MethodPost && r.URL.Path == "/v1/messages" && json.Unmarshal(body, &request) == nil {
			var messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			}
			var currentTask bool
			var childResult *claudeProbeBlock
			if json.Unmarshal(request.Messages, &messages) == nil {
				task := "Read the probe file at " + fixture.probe + " and return its text."
				// Only inspect direct text in the current user-message suffix.
				// A fork's same turn also contains an inherited parent tool-result
				// placeholder, which must not be mistaken for parent completion.
				for i := len(messages) - 1; i >= 0 && messages[i].Role == "user"; i-- {
					var text string
					if json.Unmarshal(messages[i].Content, &text) == nil {
						currentTask = currentTask || strings.Contains(text, task)
						continue
					}
					var blocks []struct {
						claudeProbeBlock
						Text string `json:"text"`
					}
					if json.Unmarshal(messages[i].Content, &blocks) == nil {
						for _, block := range blocks {
							if block.Type == "tool_result" && block.ToolUseID == "child_probe" && childResult == nil {
								result := block.claudeProbeBlock
								childResult = &result
							}
							if block.Type == "text" && strings.Contains(block.Text, task) {
								currentTask = true
							}
						}
					}
				}
			}
			fixture.mu.Lock()
			// The inherited placeholder and the new Read result can coexist
			// in one user message. The child's own result takes precedence.
			if childResult != nil && fixture.childRequested {
				fixture.childResult, fixture.childError = string(childResult.Content), childResult.IsError
				fixture.mu.Unlock()
				writeClaudeProbeResponse(w, request.Model, "", "", nil, "CHILD_DONE")
				return
			}
			if currentTask && fixture.parentCalls == 1 && !fixture.childRequested {
				fixture.childRequested = true
				for _, tool := range request.Tools {
					fixture.childTools = append(fixture.childTools, tool.Name)
				}
				// Keep the entire message history for context evidence; do not
				// truncate the request to repair response-script classification.
				*childContext = bytes.Contains(request.Messages, []byte(marker))
				fixture.mu.Unlock()
				writeClaudeProbeResponse(w, request.Model, "child_probe", "Read", map[string]any{"file_path": fixture.probe}, "")
				return
			}
			fixture.mu.Unlock()
		}
		fixture.ServeHTTP(w, r)
	})
}

func TestClaudeForkComparisonRoutesCurrentTask(t *testing.T) {
	const marker = "parent-marker"
	const task = "Read the probe file at probe.txt and return its text."
	for _, tc := range []struct {
		name    string
		current []map[string]any
		want    string
	}{
		{
			name: "fork_inherited_parent_result",
			current: []map[string]any{
				{"type": "tool_result", "tool_use_id": "parent_probe", "content": "No tool output."},
				{"type": "text", "text": task},
			},
			want: `"name":"Read"`,
		},
		{
			name: "named_parent_failure",
			current: []map[string]any{
				{"type": "tool_result", "tool_use_id": "parent_probe", "content": "Cannot launch agent with zero tools", "is_error": true},
			},
			want: `"text":"PARENT_DONE"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := &claudeSubagentFixture{agent: "fork", probe: "probe.txt"}
			var context bool
			var mu sync.Mutex
			handler := claudeForkComparisonHandler(fixture, marker, &context, &mu)
			send := func(messages []map[string]any) *httptest.ResponseRecorder {
				t.Helper()
				body, err := json.Marshal(map[string]any{
					"model":    "haiku",
					"tools":    []map[string]string{{"name": "Agent"}, {"name": "Read"}},
					"messages": messages,
				})
				if err != nil {
					t.Fatal(err)
				}
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body)))
				if response.Code != http.StatusOK {
					t.Fatalf("status = %d", response.Code)
				}
				return response
			}
			// The first request starts the parent; actor state is built through HTTP.
			initial := send([]map[string]any{{"role": "user", "content": marker}})
			if !strings.Contains(initial.Body.String(), `"name":"Agent"`) {
				t.Fatal("parent launch missing")
			}
			response := send([]map[string]any{
				{"role": "user", "content": marker + " historical task: " + task},
				{"role": "assistant", "content": []map[string]any{{"type": "tool_use", "id": "parent_probe", "name": "Agent", "input": map[string]string{"prompt": task}}}},
				{"role": "user", "content": tc.current},
			})
			if !strings.Contains(response.Body.String(), tc.want) {
				t.Fatalf("current-turn response lacks %s", tc.want)
			}
			if tc.name == "fork_inherited_parent_result" && strings.Contains(response.Body.String(), "PARENT_DONE") {
				t.Fatal("inherited placeholder prematurely completed parent")
			}
			if tc.name == "named_parent_failure" && strings.Contains(response.Body.String(), `"name":"Read"`) {
				t.Fatal("parent rejection was treated as a child request")
			}
		})
	}
}

func TestClaudeForkComparisonRoutesChildCompletion(t *testing.T) {
	fixture := &claudeSubagentFixture{agent: "fork", probe: "probe.txt"}
	var childContext bool
	var mu sync.Mutex
	handler := claudeForkComparisonHandler(fixture, "parent-marker", &childContext, &mu)
	send := func(content []map[string]any) string {
		t.Helper()
		body, err := json.Marshal(map[string]any{
			"model":    "haiku",
			"tools":    []map[string]string{{"name": "Agent"}, {"name": "Read"}},
			"messages": []map[string]any{{"role": "user", "content": content}},
		})
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body)))
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d", response.Code)
		}
		return response.Body.String()
	}
	if !strings.Contains(send([]map[string]any{{"type": "text", "text": "parent-marker"}}), `"name":"Agent"`) {
		t.Fatal("parent launch missing")
	}
	if !strings.Contains(send([]map[string]any{
		{"type": "tool_result", "tool_use_id": "parent_probe", "content": "No tool output."},
		{"type": "text", "text": "Read the probe file at probe.txt and return its text."},
	}), `"name":"Read"`) {
		t.Fatal("child read missing")
	}
	response := send([]map[string]any{
		{"type": "tool_result", "tool_use_id": "parent_probe", "content": "No tool output."},
		{"type": "tool_result", "tool_use_id": "child_probe", "content": "READ_SENTINEL"},
	})
	if !strings.Contains(response, `"text":"CHILD_DONE"`) || strings.Contains(response, "PARENT_DONE") {
		t.Fatal("child completion was shadowed by inherited parent placeholder")
	}
}
