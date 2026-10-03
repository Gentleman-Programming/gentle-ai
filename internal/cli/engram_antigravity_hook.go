package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var engramHookHTTPClient = &http.Client{
	Timeout: 3 * time.Second,
}

type antigravityHookPayload struct {
	ConversationID string         `json:"conversationId"`
	StepIdx        int            `json:"stepIdx,omitempty"`
	ToolCall       map[string]any `json:"toolCall,omitempty"`
	WorkspacePaths []string       `json:"workspacePaths,omitempty"`
	FullyIdle      *bool          `json:"fullyIdle,omitempty"`
}

type antigravityPreToolUseResponse struct {
	Decision  string         `json:"decision"`
	Overwrite map[string]any `json:"overwrite,omitempty"`
}

type antigravityStopResponse struct {
	Decision string `json:"decision"`
}

// RunEngram dispatches `gentle-ai engram` subcommands.
func RunEngram(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: gentle-ai engram hook-antigravity --event <PreToolUse|Stop>")
	}
	switch args[0] {
	case "hook-antigravity":
		return RunEngramAntigravityHook(args[1:], stdout)
	default:
		return fmt.Errorf("unknown engram subcommand %q; available: hook-antigravity", args[0])
	}
}

// RunEngramAntigravityHook is the entry point for Antigravity native hooks.
func RunEngramAntigravityHook(args []string, stdout io.Writer) error {
	return runEngramAntigravityHook(args, os.Stdin, stdout, os.Stderr)
}

func runEngramAntigravityHook(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("engram hook-antigravity", flag.ContinueOnError)
	flags.SetOutput(stderr)
	event := flags.String("event", "", "required; Antigravity hook event name (PreToolUse or Stop)")
	if err := flags.Parse(args); err != nil {
		return err
	}

	eventName := strings.TrimSpace(*event)
	if eventName == "" {
		return fmt.Errorf("engram hook-antigravity requires --event (PreToolUse or Stop)")
	}

	raw, err := io.ReadAll(io.LimitReader(stdin, 1<<20)) // 1MB limit
	if err != nil {
		return fmt.Errorf("read Antigravity hook payload: %w", err)
	}

	var payload antigravityHookPayload
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &payload); err != nil {
			return fmt.Errorf("decode Antigravity hook payload: %w", err)
		}
	}

	switch eventName {
	case "PreToolUse":
		return handleAntigravityPreToolUse(payload, stdout)
	case "Stop":
		return handleAntigravityStop(payload, stdout)
	default:
		return fmt.Errorf("unsupported Antigravity hook event %q; want PreToolUse or Stop", eventName)
	}
}

func isEngramWriteTool(name string) bool {
	switch strings.Trim(strings.TrimSpace(name), `"`) {
	case "mem_save", "mem_session_summary", "mem_capture_passive":
		return true
	default:
		return false
	}
}

func cleanToolArgString(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.Trim(strings.TrimSpace(s), `"`)
}

func handleAntigravityPreToolUse(payload antigravityHookPayload, stdout io.Writer) error {
	resp := antigravityPreToolUseResponse{
		Decision: "allow",
	}
	cid := strings.TrimSpace(payload.ConversationID)
	if cid == "" {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(resp)
	}

	toolName := ""
	var toolArgs map[string]any
	if payload.ToolCall != nil {
		if n, ok := payload.ToolCall["name"].(string); ok {
			toolName = strings.Trim(strings.TrimSpace(n), `"`)
		}
		if a, ok := payload.ToolCall["args"].(map[string]any); ok {
			toolArgs = a
		}
	}

	if toolName == "call_mcp_tool" {
		serverName := cleanToolArgString(toolArgs["ServerName"])
		targetTool := cleanToolArgString(toolArgs["ToolName"])
		if serverName == "engram" && isEngramWriteTool(targetTool) {
			ensureEngramSessionBestEffort(cid, payload.WorkspacePaths)
			resp.Overwrite = map[string]any{
				"Arguments": injectSessionIDIntoMCPArgs(toolArgs["Arguments"], cid),
			}
		}
	} else if isEngramWriteTool(toolName) || (strings.HasPrefix(toolName, "mcp_engram_") && isEngramWriteTool(strings.TrimPrefix(toolName, "mcp_engram_"))) || toolName == "" {
		ensureEngramSessionBestEffort(cid, payload.WorkspacePaths)
		resp.Overwrite = map[string]any{
			"session_id": cid,
		}
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(resp)
}

func injectSessionIDIntoMCPArgs(rawArgs any, cid string) any {
	if s, ok := rawArgs.(string); ok {
		var inner map[string]any
		if err := json.Unmarshal([]byte(s), &inner); err == nil && inner != nil {
			inner["session_id"] = cid
			b, _ := json.Marshal(inner)
			return string(b)
		}
	} else if m, ok := rawArgs.(map[string]any); ok {
		cp := make(map[string]any, len(m)+1)
		for k, v := range m {
			cp[k] = v
		}
		cp["session_id"] = cid
		return cp
	}
	return map[string]any{"session_id": cid}
}

func handleAntigravityStop(payload antigravityHookPayload, stdout io.Writer) error {
	cid := strings.TrimSpace(payload.ConversationID)
	if cid != "" && (payload.FullyIdle == nil || *payload.FullyIdle) {
		endEngramSessionBestEffort(cid)
	}

	resp := antigravityStopResponse{Decision: "allow"}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(resp)
}

func engramHTTPReq(method, path string, body any) (*http.Response, error) {
	port := strings.TrimSpace(os.Getenv("ENGRAM_PORT"))
	if port == "" {
		port = "7437"
	}
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, fmt.Sprintf("http://127.0.0.1:%s%s", port, path), r)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token := strings.TrimSpace(os.Getenv("ENGRAM_HTTP_TOKEN")); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return engramHookHTTPClient.Do(req)
}

func ensureEngramSessionBestEffort(sessionID string, workspacePaths []string) {
	if sessionID == "" {
		return
	}
	dir := ""
	if len(workspacePaths) > 0 {
		dir = strings.TrimSpace(workspacePaths[0])
	}
	if dir == "" {
		dir, _ = os.Getwd()
	}
	project := resolveEngramProject(dir)
	if project == "" {
		return
	}
	resp, err := engramHTTPReq(http.MethodPost, "/sessions", map[string]string{
		"id":        sessionID,
		"project":   project,
		"directory": dir,
	})
	if err == nil {
		_ = resp.Body.Close()
	}
}

func resolveEngramProject(dir string) string {
	if dir == "" {
		return ""
	}
	resp, err := engramHTTPReq(http.MethodGet, "/project/current?cwd="+url.QueryEscape(dir), nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return ""
	}
	defer resp.Body.Close()
	var data struct {
		Project string `json:"project"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return ""
	}
	return strings.TrimSpace(data.Project)
}

func endEngramSessionBestEffort(sessionID string) {
	resp, err := engramHTTPReq(http.MethodPost, "/sessions/"+url.PathEscape(sessionID)+"/end", map[string]string{"summary": ""})
	if err == nil {
		_ = resp.Body.Close()
	}
}
