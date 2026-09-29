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
}

type antigravityPreToolUseResponse struct {
	Decision  string            `json:"decision"`
	Overwrite map[string]string `json:"overwrite,omitempty"`
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

func handleAntigravityPreToolUse(payload antigravityHookPayload, stdout io.Writer) error {
	resp := antigravityPreToolUseResponse{
		Decision: "allow",
	}
	cid := strings.TrimSpace(payload.ConversationID)
	if cid != "" {
		resp.Overwrite = map[string]string{
			"session_id": cid,
		}
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(resp)
}

func handleAntigravityStop(payload antigravityHookPayload, stdout io.Writer) error {
	cid := strings.TrimSpace(payload.ConversationID)
	if cid != "" {
		endEngramSessionBestEffort(cid)
	}

	resp := antigravityStopResponse{
		Decision: "allow",
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(resp)
}

func endEngramSessionBestEffort(sessionID string) {
	port := strings.TrimSpace(os.Getenv("ENGRAM_PORT"))
	if port == "" {
		port = "7437"
	}

	apiURL := fmt.Sprintf("http://127.0.0.1:%s/sessions/%s/end", port, url.PathEscape(sessionID))
	body, _ := json.Marshal(map[string]string{"summary": ""})

	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if token := strings.TrimSpace(os.Getenv("ENGRAM_HTTP_TOKEN")); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := engramHookHTTPClient.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}
