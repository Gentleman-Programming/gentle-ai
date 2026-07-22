package mcp

import (
	"encoding/json"
)

const (
	// JSONRPCVersion is the supported JSON-RPC version string.
	JSONRPCVersion = "2.0"
)

// Standard JSON-RPC 2.0 error codes.
const (
	ErrCodeParseError     = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternalError  = -32603
)

// Request represents an incoming JSON-RPC 2.0 request or notification.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response represents an outgoing JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// RPCError represents a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	return e.Message
}

// ServerInfo defines the server details returned during MCP initialize.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ServerCapabilities defines capabilities offered by the MCP server.
type ServerCapabilities struct {
	Tools map[string]interface{} `json:"tools"`
}

// InitializeResult defines the result payload for the initialize request.
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
}

// Tool defines an MCP tool schema.
type Tool struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	InputSchema ToolSchema `json:"inputSchema"`
}

// ToolSchema defines the JSON Schema structure for tool parameters.
type ToolSchema struct {
	Type       string                    `json:"type"`
	Properties map[string]PropertySchema `json:"properties,omitempty"`
	Required   []string                  `json:"required,omitempty"`
}

// PropertySchema defines individual property attributes in a JSON schema.
type PropertySchema struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

// ListToolsResult defines the payload for tools/list response.
type ListToolsResult struct {
	Tools []Tool `json:"tools"`
}

// ToolCallParams defines the parameters for a tools/call request.
type ToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// TextContent represents a text item in an MCP tool response.
type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ToolCallResult defines the outcome returned by a tool execution.
type ToolCallResult struct {
	Content []TextContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}
