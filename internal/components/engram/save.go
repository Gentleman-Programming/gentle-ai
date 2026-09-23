package engram

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// SaveTopicRequest is the observation a bounded stdio-MCP `mem_save` persists
// into one Engram topic (D-11). It mirrors the mirror port request shape so the
// caller can declare `mirror ok` only after a write it actually attempted.
type SaveTopicRequest struct {
	// Title is the observation title, e.g. "Skill registry — <projectName>".
	Title string
	// Content is the observation body (the managed index content).
	Content string
	// Type is the Engram observation type, e.g. "config".
	Type string
	// Project scopes the observation to one workspace so two projects cannot
	// overwrite each other's topic_key.
	Project string
	// TopicKey is the stable upsert key, e.g. "skill-registry".
	TopicKey string
	// CapturePrompt is always false for automated artifacts: the binary must
	// never fabricate prompt capture for a write a human did not author.
	CapturePrompt bool
}

// saveRequestID is the JSON-RPC id of the bounded `tools/call`. The initialize
// handshake uses id 1 and is validated by validateInitializeResponse.
const saveRequestID = 2

// SaveTopic performs a bounded stdio-MCP `tools/call` of `mem_save` against the
// installed engram server and always terminates the child. It reuses the
// transport discipline of stdioHandshake (healthprobe.go): newline-delimited
// JSON-RPC 2.0, protocolVersion "2024-11-05", a single hard deadline covering
// spawn plus the whole exchange, and process-tree termination on every return
// path (D-11, T-3).
//
// The bounded sequence is exactly:
//
//	initialize -> notifications/initialized -> tools/call mem_save -> response
//
// Every failure (engram absent, handshake timeout, tool error, garbage output,
// child that exits early) is returned as an ordinary error. Callers map it to
// `mirror failed` and MUST NOT treat it as fatal (REQ-22.11). This function
// never reports success without having read a satisfactory `tools/call`
// response: there is no path that fabricates `mirror ok`.
//
// This is not a full MCP client: no tool listing, no sessions, no retries.
func SaveTopic(ctx context.Context, command string, args []string, req SaveTopicRequest) error {
	callArgs, err := json.Marshal(map[string]any{
		"title":          req.Title,
		"content":        req.Content,
		"type":           req.Type,
		"project":        req.Project,
		"topic_key":      req.TopicKey,
		"capture_prompt": req.CapturePrompt,
	})
	if err != nil {
		return fmt.Errorf("encode mem_save arguments: %w", err)
	}
	return saveTopicExchange(ctx, stdioProbeDeadline(0), command, args, callArgs)
}

func saveTopicExchange(ctx context.Context, timeout time.Duration, name string, args []string, callArgs json.RawMessage) error {
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := execCommandContext(context.Background(), name, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("open engram mcp stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open engram mcp stdout: %w", err)
	}
	terminateTree, err := startProbeProcessTree(cmd)
	if err != nil {
		return fmt.Errorf("start %s: %w", name, err)
	}
	var terminateOnce sync.Once
	var terminateErr error
	terminate := func() error {
		terminateOnce.Do(func() { terminateErr = terminateTree() })
		return terminateErr
	}
	var terminalCause struct {
		sync.Mutex
		err error
	}
	recordTerminalCause := func() {
		terminalCause.Lock()
		defer terminalCause.Unlock()
		terminalCause.err = handshakeContextCause(ctx, probeCtx)
	}
	contextCause := func() error {
		terminalCause.Lock()
		defer terminalCause.Unlock()
		return terminalCause.err
	}
	stopWatcher := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-probeCtx.Done():
			// Prefer a completed I/O result when it won before context cleanup.
			select {
			case <-stopWatcher:
				return
			default:
			}
			recordTerminalCause()
			_ = terminate()
			_ = stdout.Close()
		case <-stopWatcher:
		}
	}()
	waited := false
	defer func() {
		close(stopWatcher)
		<-watcherDone
		_ = stdin.Close()
		_ = terminate()
		_ = stdout.Close()
		if !waited {
			_ = cmd.Wait()
		}
	}()

	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"axiom","version":"0"}}}` + "\n"
	if _, err := io.WriteString(stdin, initialize); err != nil {
		if cause := contextCause(); cause != nil {
			return cause
		}
		return fmt.Errorf("write engram mcp initialize request: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	initialized := false
	for !initialized {
		if !scanner.Scan() {
			if cause := contextCause(); cause != nil {
				return cause
			}
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("read engram mcp output: %w", err)
			}
			return errors.New("engram mcp exited without answering initialize")
		}
		if err := validateInitializeResponse(scanner.Bytes()); err != nil {
			if cause := contextCause(); cause != nil {
				return cause
			}
			return err
		}
		initialized = true
	}

	initializedNotification := `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"
	toolsCall := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"mem_save","arguments":%s}}`+"\n", saveRequestID, callArgs)
	if _, err := io.WriteString(stdin, initializedNotification+toolsCall); err != nil {
		if cause := contextCause(); cause != nil {
			return cause
		}
		return fmt.Errorf("write engram mcp mem_save request: %w", err)
	}

	for {
		if !scanner.Scan() {
			if cause := contextCause(); cause != nil {
				return cause
			}
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("read engram mcp output: %w", err)
			}
			return errors.New("engram mcp exited without answering mem_save")
		}
		done, err := validateSaveResponse(scanner.Bytes(), saveRequestID)
		if err != nil {
			if cause := contextCause(); cause != nil {
				return cause
			}
			return err
		}
		if !done {
			// A well-formed server notification is not a protocol frame
			// response; keep reading until the tools/call result arrives.
			continue
		}

		if err := stdin.Close(); err != nil {
			if cause := contextCause(); cause != nil {
				return cause
			}
			return fmt.Errorf("close engram mcp stdin: %w", err)
		}
		if err := terminate(); err != nil {
			if cause := contextCause(); cause != nil {
				return cause
			}
			return err
		}
		// Drain before Wait: os/exec closes pipe descriptors when reaping the child.
		for scanner.Scan() {
			// Ignore trailing notifications; reject any second response frame.
			if done, err := validateSaveResponse(scanner.Bytes(), saveRequestID); err == nil && done {
				if cause := contextCause(); cause != nil {
					return cause
				}
				return errors.New("engram mcp wrote an unexpected stdout frame after mem_save response")
			}
		}
		if err := contextCause(); err != nil {
			return err
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("read engram mcp output: %w", err)
		}
		waitErr := cmd.Wait()
		waited = true
		if err := contextCause(); err != nil {
			return err
		}
		if !expectedProbeTermination(waitErr) {
			return fmt.Errorf("wait for engram mcp process: %w", waitErr)
		}
		return nil
	}
}

// validateSaveResponse validates one stdout frame while waiting for the
// bounded `tools/call` response. It returns done=false for a well-formed server
// notification (method present, no id) so the caller keeps reading. Any other
// malformed or error frame is an ordinary error: the caller must never turn it
// into `mirror ok`.
func validateSaveResponse(line []byte, wantID int) (bool, error) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return false, errors.New("invalid MCP stdout: empty frame")
	}

	var response struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  json.RawMessage `json:"method"`
		Result  json.RawMessage `json:"result"`
		Error   json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(line, &response); err != nil {
		return false, fmt.Errorf("invalid MCP stdout: malformed JSON-RPC frame: %w", err)
	}
	if response.JSONRPC != "2.0" {
		return false, errors.New(`invalid MCP stdout: jsonrpc must be "2.0"`)
	}

	hasID := len(response.ID) > 0 && !bytes.Equal(bytes.TrimSpace(response.ID), []byte("null"))
	hasMethod := len(response.Method) > 0
	if hasMethod && !hasID {
		// Server notification: not the tools/call response.
		return false, nil
	}
	if hasMethod {
		return false, errors.New("invalid MCP stdout: mem_save response must not include method")
	}
	if !hasID {
		return false, errors.New("invalid MCP stdout: mem_save response requires an id")
	}
	id := bytes.TrimSpace(response.ID)
	want := []byte(fmt.Sprintf("%d", wantID))
	if !bytes.Equal(id, want) {
		return false, errors.New("invalid MCP stdout: unexpected response id")
	}
	if len(response.Error) > 0 {
		if bytes.Equal(bytes.TrimSpace(response.Error), []byte("null")) {
			return false, errors.New("invalid MCP stdout: error must be absent or a JSON-RPC error object")
		}
		return false, fmt.Errorf("engram mcp mem_save returned error: %s", response.Error)
	}
	if len(response.Result) == 0 {
		return false, errors.New("engram mcp mem_save returned invalid result: result is required")
	}
	if err := validateSaveResult(response.Result); err != nil {
		return false, fmt.Errorf("engram mcp mem_save returned invalid result: %w", err)
	}
	return true, nil
}

// validateSaveResult rejects a tool-level error result (`isError: true`): a
// tools/call that reports a failed tool is not a successful write, so the
// transport must not let it become `mirror ok` (T-8).
func validateSaveResult(raw json.RawMessage) error {
	var result map[string]json.RawMessage
	if err := json.Unmarshal(raw, &result); err != nil || result == nil {
		return errors.New("result must be an object")
	}
	if value, ok := result["isError"]; ok {
		var isError bool
		if err := json.Unmarshal(value, &isError); err != nil {
			return errors.New("isError must be a boolean")
		}
		if isError {
			return errors.New("mem_save reported a tool error")
		}
	}
	return nil
}
