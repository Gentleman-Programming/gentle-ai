package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestEngramAntigravityHookPreToolUse(t *testing.T) {
	tests := []struct {
		name      string
		payload   string
		wantDec   string
		checkOver func(t *testing.T, over map[string]any)
	}{
		{
			name:    "direct tool with cid",
			payload: `{"conversationId":"c-1","toolCall":{"name":"mem_save"}}`,
			wantDec: "allow",
			checkOver: func(t *testing.T, over map[string]any) {
				if over["session_id"] != "c-1" {
					t.Errorf("session_id = %v, want c-1", over["session_id"])
				}
			},
		},
		{
			name:    "direct tool without cid",
			payload: `{"toolCall":{"name":"mem_save"}}`,
			wantDec: "allow",
			checkOver: func(t *testing.T, over map[string]any) {
				if len(over) != 0 {
					t.Errorf("expected empty overwrite, got %v", over)
				}
			},
		},
		{
			name:    "call_mcp_tool string arguments",
			payload: `{"conversationId":"c-2","toolCall":{"name":"call_mcp_tool","args":{"ServerName":"engram","ToolName":"mem_save","Arguments":"{\"title\":\"T\"}"}}}`,
			wantDec: "allow",
			checkOver: func(t *testing.T, over map[string]any) {
				s, ok := over["Arguments"].(string)
				if !ok || !strings.Contains(s, `"session_id":"c-2"`) {
					t.Errorf("Arguments string missing session_id: %v", over["Arguments"])
				}
			},
		},
		{
			name:    "call_mcp_tool map arguments",
			payload: `{"conversationId":"c-3","toolCall":{"name":"call_mcp_tool","args":{"ServerName":"engram","ToolName":"mem_session_summary","Arguments":{"content":"text"}}}}`,
			wantDec: "allow",
			checkOver: func(t *testing.T, over map[string]any) {
				m, ok := over["Arguments"].(map[string]any)
				if !ok || m["session_id"] != "c-3" {
					t.Errorf("Arguments map missing session_id: %v", over["Arguments"])
				}
			},
		},
		{
			name:    "call_mcp_tool non-engram server",
			payload: `{"conversationId":"c-4","toolCall":{"name":"call_mcp_tool","args":{"ServerName":"notion","ToolName":"search"}}}`,
			wantDec: "allow",
			checkOver: func(t *testing.T, over map[string]any) {
				if len(over) != 0 {
					t.Errorf("expected no overwrite for notion, got %v", over)
				}
			},
		},
		{
			name:    "call_mcp_tool engram read tool",
			payload: `{"conversationId":"c-5","toolCall":{"name":"call_mcp_tool","args":{"ServerName":"engram","ToolName":"mem_search"}}}`,
			wantDec: "allow",
			checkOver: func(t *testing.T, over map[string]any) {
				if len(over) != 0 {
					t.Errorf("expected no overwrite for mem_search, got %v", over)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := runEngramAntigravityHook([]string{"--event", "PreToolUse"}, strings.NewReader(tc.payload), &stdout, &stderr)
			if err != nil {
				t.Fatalf("runEngramAntigravityHook error = %v", err)
			}
			var resp antigravityPreToolUseResponse
			if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal error = %v; raw:\n%s", err, stdout.String())
			}
			if resp.Decision != tc.wantDec {
				t.Errorf("resp.Decision = %q, want %q", resp.Decision, tc.wantDec)
			}
			tc.checkOver(t, resp.Overwrite)
		})
	}
}

func TestEngramAntigravityHookStopClosesSession(t *testing.T) {
	var requestedPath, requestedAuth, requestedMethod string
	var requestedBody []byte

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		requestedAuth = r.Header.Get("Authorization")
		requestedMethod = r.Method
		requestedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parse test server url error = %v", err)
	}

	t.Setenv("ENGRAM_PORT", u.Port())
	t.Setenv("ENGRAM_HTTP_TOKEN", "secret-token-xyz")

	stdin := strings.NewReader(`{
		"conversationId": "sess-9988",
		"terminationReason": "model_stop"
	}`)
	var stdout, stderr bytes.Buffer

	err = runEngramAntigravityHook([]string{"--event", "Stop"}, stdin, &stdout, &stderr)
	if err != nil {
		t.Fatalf("runEngramAntigravityHook error = %v", err)
	}

	var resp antigravityStopResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response error = %v; raw:\n%s", err, stdout.String())
	}

	if resp.Decision != "allow" {
		t.Errorf("resp.Decision = %q, want allow", resp.Decision)
	}
	if requestedMethod != http.MethodPost {
		t.Errorf("requestedMethod = %q, want POST", requestedMethod)
	}
	if requestedPath != "/sessions/sess-9988/end" {
		t.Errorf("requestedPath = %q, want /sessions/sess-9988/end", requestedPath)
	}
	if requestedAuth != "Bearer secret-token-xyz" {
		t.Errorf("requestedAuth = %q, want Bearer secret-token-xyz", requestedAuth)
	}
	if !strings.Contains(string(requestedBody), `"summary":""`) {
		t.Errorf("requestedBody = %s, want summary empty string", string(requestedBody))
	}
}

func TestEngramAntigravityHookStopGracefulOnServerDown(t *testing.T) {
	// Point to an unused local port where no server is listening
	t.Setenv("ENGRAM_PORT", "59999")

	stdin := strings.NewReader(`{"conversationId": "sess-error-test"}`)
	var stdout, stderr bytes.Buffer

	err := runEngramAntigravityHook([]string{"--event", "Stop"}, stdin, &stdout, &stderr)
	if err != nil {
		t.Fatalf("runEngramAntigravityHook should succeed best-effort even when server is down; got error = %v", err)
	}

	var resp antigravityStopResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response error = %v", err)
	}
	if resp.Decision != "allow" {
		t.Errorf("resp.Decision = %q, want allow", resp.Decision)
	}
}

func TestEngramAntigravityHookStopDefersWhenNotFullyIdle(t *testing.T) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	t.Setenv("ENGRAM_PORT", u.Port())

	notIdle := false
	payload, _ := json.Marshal(antigravityHookPayload{
		ConversationID: "sess-background-busy",
		FullyIdle:      &notIdle,
	})
	var stdout, stderr bytes.Buffer

	err := runEngramAntigravityHook([]string{"--event", "Stop"}, bytes.NewReader(payload), &stdout, &stderr)
	if err != nil {
		t.Fatalf("runEngramAntigravityHook error = %v", err)
	}

	if called {
		t.Errorf("Stop should NOT call /sessions/end when fullyIdle is false")
	}
}

func TestEngramAntigravityHookPreToolUseRegistersSessionBestEffort(t *testing.T) {
	var gotProjectCwd, gotSessionID, gotSessionProject, gotSessionDir string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/project/current":
			gotProjectCwd = r.URL.Query().Get("cwd")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"project": "gentle-ai-test"})
		case "/sessions":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			gotSessionID = body["id"]
			gotSessionProject = body["project"]
			gotSessionDir = body["directory"]
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	t.Setenv("ENGRAM_PORT", u.Port())

	stdin := strings.NewReader(`{
		"conversationId": "sess-auto-register",
		"workspacePaths": ["/Users/jd/test/workspace"],
		"toolCall": {
			"name": "call_mcp_tool",
			"args": {
				"ServerName": "engram",
				"ToolName": "mem_save",
				"Arguments": {"title": "Test Auto Register"}
			}
		}
	}`)
	var stdout, stderr bytes.Buffer

	err := runEngramAntigravityHook([]string{"--event", "PreToolUse"}, stdin, &stdout, &stderr)
	if err != nil {
		t.Fatalf("runEngramAntigravityHook error = %v", err)
	}

	if gotProjectCwd != "/Users/jd/test/workspace" {
		t.Errorf("gotProjectCwd = %q, want /Users/jd/test/workspace", gotProjectCwd)
	}
	if gotSessionID != "sess-auto-register" {
		t.Errorf("gotSessionID = %q, want sess-auto-register", gotSessionID)
	}
	if gotSessionProject != "gentle-ai-test" {
		t.Errorf("gotSessionProject = %q, want gentle-ai-test", gotSessionProject)
	}
	if gotSessionDir != "/Users/jd/test/workspace" {
		t.Errorf("gotSessionDir = %q, want /Users/jd/test/workspace", gotSessionDir)
	}
}

func TestEngramAntigravityHookValidation(t *testing.T) {
	t.Run("missing event flag", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		err := runEngramAntigravityHook(nil, strings.NewReader("{}"), &stdout, &stderr)
		if err == nil {
			t.Fatal("expected error when --event is omitted, got nil")
		}
	})

	t.Run("unsupported event", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		err := runEngramAntigravityHook([]string{"--event", "UnknownEvent"}, strings.NewReader("{}"), &stdout, &stderr)
		if err == nil || !strings.Contains(err.Error(), "unsupported Antigravity hook event") {
			t.Fatalf("expected unsupported event error, got: %v", err)
		}
	})

	t.Run("RunEngram unknown subcommand", func(t *testing.T) {
		var stdout bytes.Buffer
		err := RunEngram([]string{"unknown-subcmd"}, &stdout)
		if err == nil || !strings.Contains(err.Error(), "unknown engram subcommand") {
			t.Fatalf("expected unknown subcommand error, got: %v", err)
		}
	})

	t.Run("RunEngram empty args", func(t *testing.T) {
		var stdout bytes.Buffer
		err := RunEngram(nil, &stdout)
		if err == nil {
			t.Fatal("expected usage error on empty args, got nil")
		}
	})
}
