package mcp_test

import (
	"bytes"
	"encoding/json"
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

	t.Run("Missing required topic argument", func(t *testing.T) {
		reqJSON := `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"sdd_explore","arguments":{}}}` + "\n"
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
			t.Errorf("Expected IsError true for missing required topic")
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
			wantText:    "parameter is required",
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
			wantText:    "parameter is required",
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
			wantText:    "parameter is required",
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
			wantText:    "parameter is required",
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
			var callResult mcp.ToolCallResult
			rawResult, _ := json.Marshal(resp.Result)
			_ = json.Unmarshal(rawResult, &callResult)

			if callResult.IsError != tt.expectError {
				t.Errorf("IsError = %v, want %v", callResult.IsError, tt.expectError)
			}
			if len(callResult.Content) == 0 || !strings.Contains(callResult.Content[0].Text, tt.wantText) {
				t.Errorf("Expected content to contain %q, got: %v", tt.wantText, callResult.Content)
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

func TestServer2MBPayloadHandling(t *testing.T) {
	server := mcp.NewServer()

	// Generate a 500KB string payload (exceeds default scanner limit of 64KB)
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

	t.Run("Parse Error (-32700)", func(t *testing.T) {
		reqJSON := `{"jsonrpc":"2.0", invalid json` + "\n"
		in := strings.NewReader(reqJSON)
		out := &bytes.Buffer{}

		_ = server.Serve(in, out)

		resp := parseResponse(t, out.Bytes())
		if resp.Error == nil || resp.Error.Code != mcp.ErrCodeParseError {
			t.Errorf("Expected ErrCodeParseError (-32700), got: %v", resp.Error)
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
