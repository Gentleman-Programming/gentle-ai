package mcp_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/mcp"
)

func parseResponse(t *testing.T, raw []byte) mcp.Response {
	t.Helper()
	var resp mcp.Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("Failed to parse JSON response: %v. Raw: %s", err, string(raw))
	}
	return resp
}

func TestServerInitialize(t *testing.T) {
	server := mcp.NewServer()

	reqJSON := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}` + "\n"
	in := strings.NewReader(reqJSON)
	out := &bytes.Buffer{}

	if err := server.Serve(in, out); err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}

	resp := parseResponse(t, out.Bytes())
	if resp.JSONRPC != "2.0" {
		t.Errorf("Expected jsonrpc '2.0', got %q", resp.JSONRPC)
	}
	if resp.Error != nil {
		t.Fatalf("Expected no error, got %v", resp.Error)
	}

	var initResult mcp.InitializeResult
	rawResult, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("Failed to marshal result: %v", err)
	}
	if err := json.Unmarshal(rawResult, &initResult); err != nil {
		t.Fatalf("Failed to unmarshal InitializeResult: %v", err)
	}

	if initResult.ServerInfo.Name != "gentle-ai" {
		t.Errorf("Expected server name 'gentle-ai', got %q", initResult.ServerInfo.Name)
	}
	if initResult.ProtocolVersion == "" {
		t.Error("Expected non-empty protocol version")
	}
}

func TestServerToolsList(t *testing.T) {
	server := mcp.NewServer()

	reqJSON := `{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n"
	in := strings.NewReader(reqJSON)
	out := &bytes.Buffer{}

	if err := server.Serve(in, out); err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}

	resp := parseResponse(t, out.Bytes())
	if resp.Error != nil {
		t.Fatalf("Expected no error, got %v", resp.Error)
	}

	var listResult mcp.ListToolsResult
	rawResult, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("Failed to marshal result: %v", err)
	}
	if err := json.Unmarshal(rawResult, &listResult); err != nil {
		t.Fatalf("Failed to unmarshal ListToolsResult: %v", err)
	}

	toolNames := make(map[string]bool)
	for _, tool := range listResult.Tools {
		toolNames[tool.Name] = true
	}

	expectedTools := []string{"sdd_explore", "sdd_review", "sdd_propose", "sdd_spec", "sdd_design", "sdd_tasks"}
	for _, name := range expectedTools {
		if !toolNames[name] {
			t.Errorf("Expected tool %q to be present in tools/list", name)
		}
	}
}

func TestServerToolsCallSDDExplore(t *testing.T) {
	server := mcp.NewServer()

	t.Run("Valid topic argument", func(t *testing.T) {
		reqJSON := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"sdd_explore","arguments":{"topic":"MCP Integration","context":"Claude Desktop test"}}}` + "\n"
		in := strings.NewReader(reqJSON)
		out := &bytes.Buffer{}

		if err := server.Serve(in, out); err != nil {
			t.Fatalf("Serve returned error: %v", err)
		}

		resp := parseResponse(t, out.Bytes())
		if resp.Error != nil {
			t.Fatalf("Expected no error, got %v", resp.Error)
		}

		var callResult mcp.ToolCallResult
		rawResult, err := json.Marshal(resp.Result)
		if err != nil {
			t.Fatalf("Failed to marshal result: %v", err)
		}
		if err := json.Unmarshal(rawResult, &callResult); err != nil {
			t.Fatalf("Failed to unmarshal ToolCallResult: %v", err)
		}

		if callResult.IsError {
			t.Errorf("Expected IsError false, got true")
		}
		if len(callResult.Content) == 0 {
			t.Fatal("Expected non-empty content slice")
		}
		if !strings.Contains(callResult.Content[0].Text, "MCP Integration") {
			t.Errorf("Expected output to contain topic, got: %s", callResult.Content[0].Text)
		}
	})

	t.Run("Missing required topic argument returns ErrCodeInvalidParams", func(t *testing.T) {
		reqJSON := `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"sdd_explore","arguments":{}}}` + "\n"
		in := strings.NewReader(reqJSON)
		out := &bytes.Buffer{}

		if err := server.Serve(in, out); err != nil {
			t.Fatalf("Serve returned error: %v", err)
		}

		resp := parseResponse(t, out.Bytes())
		if resp.Error == nil {
			t.Fatal("Expected error response for missing required topic, got nil")
		}
		if resp.Error.Code != mcp.ErrCodeInvalidParams {
			t.Errorf("Expected code %d, got %d", mcp.ErrCodeInvalidParams, resp.Error.Code)
		}
	})
}

func TestServerToolsCallSDDReview(t *testing.T) {
	server := mcp.NewServer()

	reqJSON := `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"sdd_review","arguments":{"artifact":"internal/mcp/server.go","focus":"security"}}}` + "\n"
	in := strings.NewReader(reqJSON)
	out := &bytes.Buffer{}

	if err := server.Serve(in, out); err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}

	resp := parseResponse(t, out.Bytes())
	if resp.Error != nil {
		t.Fatalf("Expected no error, got %v", resp.Error)
	}

	var callResult mcp.ToolCallResult
	rawResult, _ := json.Marshal(resp.Result)
	_ = json.Unmarshal(rawResult, &callResult)

	if callResult.IsError {
		t.Errorf("Expected IsError false")
	}
	if len(callResult.Content) == 0 {
		t.Fatal("Expected non-empty content")
	}
	if !strings.Contains(callResult.Content[0].Text, "SDD 4R Review Protocol") {
		t.Errorf("Expected 4R review protocol text, got: %s", callResult.Content[0].Text)
	}
}

func TestServerToolsCallAdditionalSDDTools(t *testing.T) {
	server := mcp.NewServer()

	tests := []struct {
		name        string
		toolName    string
		arguments   string
		wantText    string
		expectError bool
	}{
		{
			name:        "sdd_propose valid",
			toolName:    "sdd_propose",
			arguments:   `{"change":"Add MCP server","scope":"internal/mcp"}`,
			wantText:    "SDD Proposal Protocol",
			expectError: false,
		},
		{
			name:        "sdd_propose missing change",
			toolName:    "sdd_propose",
			arguments:   `{}`,
			wantText:    "missing required string parameter",
			expectError: true,
		},
		{
			name:        "sdd_spec valid",
			toolName:    "sdd_spec",
			arguments:   `{"feature":"JSON-RPC Stdio"}`,
			wantText:    "SDD Specification Protocol",
			expectError: false,
		},
		{
			name:        "sdd_spec missing feature",
			toolName:    "sdd_spec",
			arguments:   `{}`,
			wantText:    "missing required string parameter",
			expectError: true,
		},
		{
			name:        "sdd_design valid",
			toolName:    "sdd_design",
			arguments:   `{"spec":"MCP stdio spec"}`,
			wantText:    "SDD Technical Design Protocol",
			expectError: false,
		},
		{
			name:        "sdd_design missing spec",
			toolName:    "sdd_design",
			arguments:   `{}`,
			wantText:    "missing required string parameter",
			expectError: true,
		},
		{
			name:        "sdd_tasks valid",
			toolName:    "sdd_tasks",
			arguments:   `{"design":"MCP Server Design"}`,
			wantText:    "SDD Task Breakdown Protocol",
			expectError: false,
		},
		{
			name:        "sdd_tasks missing design",
			toolName:    "sdd_tasks",
			arguments:   `{}`,
			wantText:    "missing required string parameter",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqJSON := fmt.Sprintf(`{"jsonrpc":"2.0","id":100,"method":"tools/call","params":{"name":%q,"arguments":%s}}`+"\n", tt.toolName, tt.arguments)
			in := strings.NewReader(reqJSON)
			out := &bytes.Buffer{}

			if err := server.Serve(in, out); err != nil {
				t.Fatalf("Serve returned error: %v", err)
			}

			resp := parseResponse(t, out.Bytes())
			if tt.expectError {
				if resp.Error == nil {
					t.Fatalf("Expected error response, got nil")
				}
				if resp.Error.Code != mcp.ErrCodeInvalidParams {
					t.Errorf("Expected code %d, got %d", mcp.ErrCodeInvalidParams, resp.Error.Code)
				}
				if !strings.Contains(resp.Error.Message, tt.wantText) {
					t.Errorf("Expected error message containing %q, got %q", tt.wantText, resp.Error.Message)
				}
			} else {
				if resp.Error != nil {
					t.Fatalf("Expected no error, got %v", resp.Error)
				}
				var callResult mcp.ToolCallResult
				rawResult, _ := json.Marshal(resp.Result)
				_ = json.Unmarshal(rawResult, &callResult)

				if len(callResult.Content) == 0 || !strings.Contains(callResult.Content[0].Text, tt.wantText) {
					t.Errorf("Expected content to contain %q, got: %v", tt.wantText, callResult.Content)
				}
			}
		})
	}
}

func TestServerToolsCallUnknownTool(t *testing.T) {
	server := mcp.NewServer()

	reqJSON := `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"unknown_tool","arguments":{}}}` + "\n"
	in := strings.NewReader(reqJSON)
	out := &bytes.Buffer{}

	if err := server.Serve(in, out); err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}

	resp := parseResponse(t, out.Bytes())
	var callResult mcp.ToolCallResult
	rawResult, _ := json.Marshal(resp.Result)
	_ = json.Unmarshal(rawResult, &callResult)

	if !callResult.IsError {
		t.Errorf("Expected IsError true for unknown tool")
	}
	if !strings.Contains(callResult.Content[0].Text, "Tool not found") {
		t.Errorf("Expected 'Tool not found' text, got: %s", callResult.Content[0].Text)
	}
}

func TestServerMultilineJSON(t *testing.T) {
	server := mcp.NewServer()

	prettyJSON := `{
		"jsonrpc": "2.0",
		"id": 99,
		"method": "ping"
	}`
	in := strings.NewReader(prettyJSON)
	out := &bytes.Buffer{}

	if err := server.Serve(in, out); err != nil {
		t.Fatalf("Serve returned error for multiline JSON: %v", err)
	}

	resp := parseResponse(t, out.Bytes())
	if resp.Error != nil {
		t.Fatalf("Expected no error, got %v", resp.Error)
	}
	if resp.ID != float64(99) {
		t.Errorf("Expected ID 99, got %v", resp.ID)
	}
}

func TestServerNotificationErrors(t *testing.T) {
	server := mcp.NewServer()

	tests := []struct {
		name string
		json string
	}{
		{
			name: "Notification with invalid JSON-RPC version",
			json: `{"jsonrpc":"1.0","method":"ping"}`,
		},
		{
			name: "Notification with unknown method",
			json: `{"jsonrpc":"2.0","method":"non_existent_method"}`,
		},
		{
			name: "Notification with invalid tool params",
			json: `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"sdd_explore","arguments":{}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := strings.NewReader(tt.json)
			out := &bytes.Buffer{}

			if err := server.Serve(in, out); err != nil {
				t.Fatalf("Serve error: %v", err)
			}

			if out.Len() != 0 {
				t.Errorf("Expected no response written for notification error, got: %s", out.String())
			}
		})
	}
}

func TestServerInvalidParamsTypeSafety(t *testing.T) {
	server := mcp.NewServer()

	tests := []struct {
		name string
		json string
	}{
		{
			name: "Non-string topic in sdd_explore",
			json: `{"jsonrpc":"2.0","id":201,"method":"tools/call","params":{"name":"sdd_explore","arguments":{"topic":123}}}`,
		},
		{
			name: "Whitespace-only topic in sdd_explore",
			json: `{"jsonrpc":"2.0","id":202,"method":"tools/call","params":{"name":"sdd_explore","arguments":{"topic":"   "}}}`,
		},
		{
			name: "Non-string context in sdd_explore",
			json: `{"jsonrpc":"2.0","id":203,"method":"tools/call","params":{"name":"sdd_explore","arguments":{"topic":"Valid", "context": true}}}`,
		},
		{
			name: "Non-string artifact in sdd_review",
			json: `{"jsonrpc":"2.0","id":204,"method":"tools/call","params":{"name":"sdd_review","arguments":{"artifact": 999}}}`,
		},
		{
			name: "Non-string focus in sdd_review",
			json: `{"jsonrpc":"2.0","id":205,"method":"tools/call","params":{"name":"sdd_review","arguments":{"focus": []}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := strings.NewReader(tt.json)
			out := &bytes.Buffer{}

			if err := server.Serve(in, out); err != nil {
				t.Fatalf("Serve error: %v", err)
			}

			resp := parseResponse(t, out.Bytes())
			if resp.Error == nil {
				t.Fatalf("Expected ErrCodeInvalidParams error response, got nil")
			}
			if resp.Error.Code != mcp.ErrCodeInvalidParams {
				t.Errorf("Expected error code %d, got %d", mcp.ErrCodeInvalidParams, resp.Error.Code)
			}
		})
	}
}

type errWriter struct{}

func (e *errWriter) Write(p []byte) (n int, err error) {
	return 0, errors.New("simulated write error")
}

func TestServerWriteError(t *testing.T) {
	server := mcp.NewServer()

	reqJSON := `{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n"
	in := strings.NewReader(reqJSON)
	out := &errWriter{}

	err := server.Serve(in, out)
	if err == nil {
		t.Fatal("Expected Serve to return error on write error, got nil")
	}
	if !strings.Contains(err.Error(), "simulated write error") {
		t.Errorf("Expected 'simulated write error', got %v", err)
	}
}

func TestServer2MBPayloadHandling(t *testing.T) {
	server := mcp.NewServer()

	largeString := strings.Repeat("A", 500*1024)
	reqJSON := fmt.Sprintf(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"sdd_explore","arguments":{"topic":"%s"}}}`, largeString) + "\n"

	in := strings.NewReader(reqJSON)
	out := &bytes.Buffer{}

	if err := server.Serve(in, out); err != nil {
		t.Fatalf("Serve failed on large payload (>64KB): %v", err)
	}

	resp := parseResponse(t, out.Bytes())
	if resp.Error != nil {
		t.Fatalf("Expected successful processing of large payload, got error: %v", resp.Error)
	}

	var callResult mcp.ToolCallResult
	rawResult, _ := json.Marshal(resp.Result)
	_ = json.Unmarshal(rawResult, &callResult)

	if callResult.IsError {
		t.Errorf("Expected successful tool call execution on large payload")
	}
}

func TestServerJSONRPCErrorHandling(t *testing.T) {
	server := mcp.NewServer()

	t.Run("Parse Error emits response with id null", func(t *testing.T) {
		reqJSON := `{"jsonrpc":"2.0", invalid json` + "\n"
		in := strings.NewReader(reqJSON)
		out := &bytes.Buffer{}

		_ = server.Serve(in, out)
		resp := parseResponse(t, out.Bytes())
		if resp.Error == nil || resp.Error.Code != mcp.ErrCodeParseError {
			t.Errorf("Expected ErrCodeParseError (-32700), got: %v", resp.Error)
		}
		if resp.ID != nil {
			t.Errorf("Expected ID nil (null), got: %v", resp.ID)
		}
	})

	t.Run("Invalid JSON-RPC Version (-32600)", func(t *testing.T) {
		reqJSON := `{"jsonrpc":"1.0","id":10,"method":"ping"}` + "\n"
		in := strings.NewReader(reqJSON)
		out := &bytes.Buffer{}

		_ = server.Serve(in, out)

		resp := parseResponse(t, out.Bytes())
		if resp.Error == nil || resp.Error.Code != mcp.ErrCodeInvalidRequest {
			t.Errorf("Expected ErrCodeInvalidRequest (-32600), got: %v", resp.Error)
		}
	})

	t.Run("Unknown Method (-32601)", func(t *testing.T) {
		reqJSON := `{"jsonrpc":"2.0","id":11,"method":"non_existent_method"}` + "\n"
		in := strings.NewReader(reqJSON)
		out := &bytes.Buffer{}

		_ = server.Serve(in, out)

		resp := parseResponse(t, out.Bytes())
		if resp.Error == nil || resp.Error.Code != mcp.ErrCodeMethodNotFound {
			t.Errorf("Expected ErrCodeMethodNotFound (-32601), got: %v", resp.Error)
		}
	})

	t.Run("Notification ignored without response", func(t *testing.T) {
		reqJSON := `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"
		in := strings.NewReader(reqJSON)
		out := &bytes.Buffer{}

		if err := server.Serve(in, out); err != nil {
			t.Fatalf("Serve error: %v", err)
		}

		if out.Len() != 0 {
			t.Errorf("Expected no output for notification, got: %s", out.String())
		}
	})
}

func TestServerCustomToolRegistration(t *testing.T) {
	server := mcp.NewServer()

	customTool := mcp.Tool{
		Name:        "custom_test_tool",
		Description: "Custom test tool",
		InputSchema: mcp.ToolSchema{Type: "object"},
	}

	server.RegisterTool(customTool, func(args map[string]interface{}) (*mcp.ToolCallResult, error) {
		return &mcp.ToolCallResult{
			Content: []mcp.TextContent{{Type: "text", Text: "Custom response"}},
		}, nil
	})

	reqJSON := `{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"custom_test_tool","arguments":{}}}` + "\n"
	in := strings.NewReader(reqJSON)
	out := &bytes.Buffer{}

	if err := server.Serve(in, out); err != nil {
		t.Fatalf("Serve error: %v", err)
	}

	resp := parseResponse(t, out.Bytes())
	var callResult mcp.ToolCallResult
	rawResult, _ := json.Marshal(resp.Result)
	_ = json.Unmarshal(rawResult, &callResult)

	if len(callResult.Content) == 0 || callResult.Content[0].Text != "Custom response" {
		t.Errorf("Expected custom response text, got %v", callResult.Content)
	}
}

func TestServerRecoversFromMalformedJSON(t *testing.T) {
	server := mcp.NewServer()

	reqJSON := "invalid json payload\n" + `{"jsonrpc":"2.0","id":42,"method":"ping"}` + "\n"
	in := strings.NewReader(reqJSON)
	out := &bytes.Buffer{}

	if err := server.Serve(in, out); err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("Expected 2 response lines, got %d: %s", len(lines), out.String())
	}

	resp1 := parseResponse(t, []byte(lines[0]))
	if resp1.Error == nil || resp1.Error.Code != mcp.ErrCodeParseError {
		t.Errorf("Expected first response ErrCodeParseError, got: %v", resp1.Error)
	}
	if resp1.ID != nil {
		t.Errorf("Expected first response ID nil, got: %v", resp1.ID)
	}

	resp2 := parseResponse(t, []byte(lines[1]))
	if resp2.Error != nil {
		t.Errorf("Expected second response no error, got: %v", resp2.Error)
	}
	if resp2.ID != float64(42) {
		t.Errorf("Expected second response ID 42, got: %v", resp2.ID)
	}
}
