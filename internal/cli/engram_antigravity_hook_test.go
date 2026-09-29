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

func TestEngramAntigravityHookPreToolUseWithConversationID(t *testing.T) {
	stdin := strings.NewReader(`{
		"conversationId": "conv-12345",
		"toolCall": {
			"name": "mem_save",
			"args": {"title": "Test Observation"}
		},
		"stepIdx": 2
	}`)
	var stdout, stderr bytes.Buffer

	err := runEngramAntigravityHook([]string{"--event", "PreToolUse"}, stdin, &stdout, &stderr)
	if err != nil {
		t.Fatalf("runEngramAntigravityHook error = %v, stderr: %s", err, stderr.String())
	}

	var resp antigravityPreToolUseResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response error = %v; raw:\n%s", err, stdout.String())
	}

	if resp.Decision != "allow" {
		t.Errorf("resp.Decision = %q, want allow", resp.Decision)
	}
	if got := resp.Overwrite["session_id"]; got != "conv-12345" {
		t.Errorf("resp.Overwrite[session_id] = %q, want conv-12345", got)
	}
}

func TestEngramAntigravityHookPreToolUseWithoutConversationID(t *testing.T) {
	stdin := strings.NewReader(`{
		"toolCall": {
			"name": "mem_save"
		}
	}`)
	var stdout, stderr bytes.Buffer

	err := runEngramAntigravityHook([]string{"--event", "PreToolUse"}, stdin, &stdout, &stderr)
	if err != nil {
		t.Fatalf("runEngramAntigravityHook error = %v", err)
	}

	var resp antigravityPreToolUseResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response error = %v; raw:\n%s", err, stdout.String())
	}

	if resp.Decision != "allow" {
		t.Errorf("resp.Decision = %q, want allow", resp.Decision)
	}
	if len(resp.Overwrite) != 0 {
		t.Errorf("resp.Overwrite = %v, want empty", resp.Overwrite)
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
