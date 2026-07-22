package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"
)

// ToolHandler represents a function handling an MCP tool invocation.
type ToolHandler func(args map[string]interface{}) (*ToolCallResult, error)

// Server represents a stdio JSON-RPC 2.0 MCP server.
type Server struct {
	mu       sync.RWMutex
	tools    map[string]Tool
	handlers map[string]ToolHandler
	writer   io.Writer
	version  string
}

// NewServer creates a new Server pre-populated with standard SDD reasoning tools.
// An optional build version string can be supplied; defaults to "1.0.0" if omitted.
func NewServer(version ...string) *Server {
	ver := "1.0.0"
	if len(version) > 0 && version[0] != "" {
		ver = version[0]
	}
	s := &Server{
		tools:    make(map[string]Tool),
		handlers: make(map[string]ToolHandler),
		version:  ver,
	}

	s.registerDefaultTools()
	return s
}

// SetVersion sets the build version string returned in serverInfo during initialize.
func (s *Server) SetVersion(version string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.version = version
}

// RegisterTool registers a new MCP tool with its execution handler.
func (s *Server) RegisterTool(tool Tool, handler ToolHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[tool.Name] = tool
	s.handlers[tool.Name] = handler
}

// registerDefaultTools populates default SDD reasoning tools into the server.
func (s *Server) registerDefaultTools() {
	s.mu.Lock()
	defer s.mu.Unlock()

	defaultHandlers := map[string]ToolHandler{
		"sdd_explore": HandleSDDExplore,
		"sdd_review":  HandleSDDReview,
		"sdd_propose": HandleSDDPropose,
		"sdd_spec":    HandleSDDSpec,
		"sdd_design":  HandleSDDDesign,
		"sdd_tasks":   HandleSDDTasks,
	}

	for _, tool := range DefaultSDDTools() {
		if handler, ok := defaultHandlers[tool.Name]; ok {
			s.tools[tool.Name] = tool
			s.handlers[tool.Name] = handler
		}
	}
}

// Serve starts processing stdio JSON-RPC 2.0 requests from r and writing responses to w.
// Uses bufio.Reader to process stdio payloads line-by-line and recover cleanly from parse errors.
func (s *Server) Serve(r io.Reader, w io.Writer) error {
	s.mu.Lock()
	s.writer = w
	s.mu.Unlock()

	reader := bufio.NewReader(r)
	var buf bytes.Buffer

	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) == 0 && readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return readErr
		}

		buf.Write(line)
		trimmed := bytes.TrimSpace(buf.Bytes())
		if len(trimmed) == 0 {
			buf.Reset()
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					return nil
				}
				return readErr
			}
			continue
		}

		var raw json.RawMessage
		if err := json.Unmarshal(trimmed, &raw); err != nil {
			if isIncompleteJSON(err, trimmed) && readErr == nil {
				continue
			}
			buf.Reset()
			if writeErr := s.writeError(nil, ErrCodeParseError, "Parse error: invalid JSON payload", nil); writeErr != nil {
				return writeErr
			}
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					return nil
				}
				return readErr
			}
			continue
		}

		buf.Reset()
		if err := s.handleMessage(raw); err != nil {
			return err
		}

		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return readErr
		}
	}
}

func isIncompleteJSON(err error, data []byte) bool {
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return true
	}
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return syntaxErr.Offset >= int64(len(data))
	}
	return false
}

func (s *Server) handleMessage(data []byte) error {
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		return s.writeError(nil, ErrCodeParseError, "Parse error: invalid JSON payload", nil)
	}

	// Validate JSON-RPC version
	if req.JSONRPC != JSONRPCVersion {
		return s.writeError(req.ID, ErrCodeInvalidRequest, "Invalid JSON-RPC version, expected '2.0'", nil)
	}

	switch req.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if len(req.Params) > 0 {
			_ = json.Unmarshal(req.Params, &params)
		}
		if params.ProtocolVersion != "" && params.ProtocolVersion != "2024-11-05" {
			return s.writeError(req.ID, ErrCodeInvalidParams, fmt.Sprintf("Unsupported protocol version: %s", params.ProtocolVersion), nil)
		}

		s.mu.RLock()
		ver := s.version
		s.mu.RUnlock()
		if ver == "" {
			ver = "1.0.0"
		}

		result := InitializeResult{
			ProtocolVersion: "2024-11-05",
			Capabilities: ServerCapabilities{
				Tools: map[string]interface{}{},
			},
			ServerInfo: ServerInfo{
				Name:    "gentle-ai",
				Version: ver,
			},
		}
		return s.writeResult(req.ID, result)

	case "notifications/initialized":
		// Notification method, no response returned.
		return nil

	case "ping":
		return s.writeResult(req.ID, map[string]interface{}{})

	case "tools/list":
		s.mu.RLock()
		toolsList := make([]Tool, 0, len(s.tools))
		for _, tool := range s.tools {
			toolsList = append(toolsList, tool)
		}
		s.mu.RUnlock()

		sort.Slice(toolsList, func(i, j int) bool {
			return toolsList[i].Name < toolsList[j].Name
		})

		return s.writeResult(req.ID, ListToolsResult{Tools: toolsList})

	case "tools/call":
		var params ToolCallParams
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &params); err != nil {
				return s.writeError(req.ID, ErrCodeInvalidParams, "Invalid params for tools/call", nil)
			}
		}

		s.mu.RLock()
		handler, exists := s.handlers[params.Name]
		s.mu.RUnlock()

		if !exists {
			return s.writeResult(req.ID, ToolCallResult{
				Content: []TextContent{{Type: "text", Text: fmt.Sprintf("Tool not found: %s", params.Name)}},
				IsError: true,
			})
		}

		res, err := handler(params.Arguments)
		if err != nil {
			var rpcErr *RPCError
			if errors.As(err, &rpcErr) {
				return s.writeError(req.ID, rpcErr.Code, rpcErr.Message, rpcErr.Data)
			}
			return s.writeResult(req.ID, ToolCallResult{
				Content: []TextContent{{Type: "text", Text: fmt.Sprintf("Tool execution error: %v", err)}},
				IsError: true,
			})
		}

		if res == nil {
			res = &ToolCallResult{
				Content: []TextContent{{Type: "text", Text: ""}},
			}
		}
		return s.writeResult(req.ID, res)

	default:
		return s.writeError(req.ID, ErrCodeMethodNotFound, fmt.Sprintf("Method not found: %s", req.Method), nil)
	}
}

func (s *Server) writeResult(id interface{}, result interface{}) error {
	if id == nil {
		return nil
	}
	resp := Response{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Result:  result,
	}
	return s.sendResponse(resp)
}

func (s *Server) writeError(id interface{}, code int, message string, data interface{}) error {
	if id == nil && code != ErrCodeParseError {
		return nil
	}
	resp := Response{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	return s.sendResponse(resp)
}

func (s *Server) sendResponse(resp Response) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.writer == nil {
		return fmt.Errorf("server writer is nil")
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal response error: %w", err)
	}

	data = append(data, '\n')
	_, err = s.writer.Write(data)
	return err
}
