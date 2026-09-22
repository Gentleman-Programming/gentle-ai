package engram

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setSaveHelperProcess routes the execCommandContext seam through this test
// binary re-executing TestHelperEngramSaveMCPProcess in the given mode, so
// SaveTopic runs against a real child process speaking real stdio.
func setSaveHelperProcess(t *testing.T, mode string, env ...string) func() *exec.Cmd {
	t.Helper()
	orig := execCommandContext
	var helper *exec.Cmd
	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		// -test.timeout keeps a pending runtime timer alive in the helper. An
		// idle helper parked only in select{} has no syscall-blocked goroutine,
		// so on Windows the child runtime declares "all goroutines are asleep",
		// exits before the probe context ends, and the parent reads a clean
		// stdout EOF instead of the context cause.
		cs := append([]string{"-test.run=TestHelperEngramSaveMCPProcess", "-test.timeout=1m", "--", name}, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "GO_ENGRAM_SAVE_HELPER_MODE="+mode)
		cmd.Env = append(cmd.Env, env...)
		// The exchange discards child stderr; surface helper crashes in test output.
		cmd.Stderr = os.Stderr
		helper = cmd
		return cmd
	}
	t.Cleanup(func() { execCommandContext = orig })
	return func() *exec.Cmd { return helper }
}

// TestHelperEngramSaveMCPProcess is not a real test: it is the fake engram
// endpoint for SaveTopic. When re-executed with GO_ENGRAM_SAVE_HELPER_MODE set
// it emulates an engram MCP server over stdio in one of several modes, then
// exits without letting the test framework print to stdout (stdout is the MCP
// channel).
func TestHelperEngramSaveMCPProcess(t *testing.T) {
	mode := os.Getenv("GO_ENGRAM_SAVE_HELPER_MODE")
	if mode == "" {
		return
	}
	defer os.Exit(0)

	helperIn := bufio.NewReader(os.Stdin)

	readRequest := func() (id string, method string, raw string, ok bool) {
		line, err := readHelperLine(helperIn)
		if err != nil {
			return "", "", "", false
		}
		var req struct {
			ID     json.Number `json:"id"`
			Method string      `json:"method"`
		}
		if json.Unmarshal([]byte(line), &req) != nil {
			return "", "", line, true
		}
		return req.ID.String(), req.Method, line, true
	}

	switch mode {
	case "save-garbage":
		fmt.Println("this is not a JSON-RPC message")
		return
	case "save-silent":
		// Never answer anything: exercises the hard deadline.
		select {}
	case "save-early-exit":
		// Exit before answering initialize.
		return
	}

	id, method, _, ok := readRequest()
	if !ok || method != "initialize" {
		return
	}

	if mode == "save-early-exit-after-init" {
		// Answer initialize, then self-terminate before the tools/call response.
		fmt.Printf("{\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":{\"protocolVersion\":\"2024-11-05\",\"capabilities\":{},\"serverInfo\":{\"name\":\"fake-engram\",\"version\":\"0\"}}}\n", id)
		return
	}
	if mode == "save-silent-after-init" {
		// Answer initialize, then never answer the tools/call.
		fmt.Printf("{\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":{\"protocolVersion\":\"2024-11-05\",\"capabilities\":{},\"serverInfo\":{\"name\":\"fake-engram\",\"version\":\"0\"}}}\n", id)
		select {}
	}

	// Healthy initialize response for every remaining mode.
	fmt.Printf("{\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":{\"protocolVersion\":\"2024-11-05\",\"capabilities\":{},\"serverInfo\":{\"name\":\"fake-engram\",\"version\":\"0\"}}}\n", id)

	// Expect notifications/initialized then tools/call.
	for {
		callID, callMethod, raw, ok := readRequest()
		if !ok {
			return
		}
		if strings.HasPrefix(callMethod, "notifications/") {
			continue
		}
		if callMethod != "tools/call" {
			return
		}
		if path := os.Getenv("GO_ENGRAM_SAVE_ARGS_FILE"); path != "" {
			_ = os.WriteFile(path, []byte(raw), 0o600)
		}
		switch mode {
		case "save-ok":
			fmt.Printf("{\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"saved\"}]}}\n", callID)
			// Stay alive until the parent terminates the process tree.
			select {}
		case "save-tool-error":
			fmt.Printf("{\"jsonrpc\":\"2.0\",\"id\":%s,\"error\":{\"code\":-32603,\"message\":\"store locked\"}}\n", callID)
			return
		case "save-tool-is-error":
			fmt.Printf("{\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":{\"isError\":true,\"content\":[{\"type\":\"text\",\"text\":\"rejected\"}]}}\n", callID)
			select {}
		default:
			return
		}
	}
}

// readHelperLine reads one newline-delimited frame from the helper's stdin.
// The reader must be shared across calls: a fresh bufio.Reader per frame would
// drop bytes the previous reader already buffered.
func readHelperLine(in *bufio.Reader) (string, error) {
	line, err := in.ReadString('\n')
	if err != nil {
		if line == "" {
			return "", err
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func TestSaveTopic_HappyPathArgumentsAndTermination(t *testing.T) {
	argsPath := filepath.Join(t.TempDir(), "mem_save_args.json")
	helperCommand := setSaveHelperProcess(t, "save-ok", "GO_ENGRAM_SAVE_ARGS_FILE="+argsPath)

	req := SaveTopicRequest{
		Title:         "Skill registry — demo",
		Content:       "## Skills\n\n| Skill |\n| --- |\n| `demo` |\n",
		Type:          "config",
		Project:       "axiom",
		TopicKey:      "skill-registry",
		CapturePrompt: false,
	}

	err := SaveTopic(context.Background(), "engram", []string{"mcp"}, req)
	if err != nil {
		t.Fatalf("SaveTopic() error = %v, want nil for a healthy MCP server", err)
	}

	command := helperCommand()
	if command == nil {
		t.Fatal("SaveTopic() did not start its helper child")
	}
	if command.ProcessState == nil {
		t.Fatalf("SaveTopic() did not terminate its child on the success path: state=%#v", command.ProcessState)
	}

	raw, readErr := os.ReadFile(argsPath)
	if readErr != nil {
		t.Fatalf("read captured mem_save arguments: %v", readErr)
	}
	var frame struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Method  string `json:"method"`
		Params  struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"params"`
	}
	if err := json.Unmarshal(raw, &frame); err != nil {
		t.Fatalf("captured frame is not JSON: %v\n%s", err, raw)
	}
	if frame.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q, want 2.0", frame.JSONRPC)
	}
	if frame.Method != "tools/call" {
		t.Errorf("method = %q, want tools/call", frame.Method)
	}
	if frame.Params.Name != "mem_save" {
		t.Errorf("params.name = %q, want mem_save", frame.Params.Name)
	}

	var args map[string]any
	if err := json.Unmarshal(frame.Params.Arguments, &args); err != nil {
		t.Fatalf("tools/call arguments are not JSON: %v", err)
	}
	for _, key := range []string{"title", "content", "type", "project", "topic_key", "capture_prompt"} {
		if _, ok := args[key]; !ok {
			t.Errorf("tools/call arguments missing %q: %v", key, args)
		}
	}
	if args["title"] != req.Title {
		t.Errorf("title = %v, want %q", args["title"], req.Title)
	}
	if args["type"] != "config" {
		t.Errorf("type = %v, want config", args["type"])
	}
	if args["project"] != "axiom" {
		t.Errorf("project = %v, want axiom", args["project"])
	}
	if args["topic_key"] != "skill-registry" {
		t.Errorf("topic_key = %v, want skill-registry", args["topic_key"])
	}
	if capture, ok := args["capture_prompt"].(bool); !ok || capture {
		t.Errorf("capture_prompt = %v, want false", args["capture_prompt"])
	}
}

func TestSaveTopic_FailureModesAlwaysTerminateChild(t *testing.T) {
	tests := []struct {
		name         string
		mode         string
		timeout      time.Duration
		wantContains string
	}{
		{
			name:         "tools/call JSON-RPC error",
			mode:         "save-tool-error",
			wantContains: "mem_save returned error",
		},
		{
			name:         "tools/call reports tool-level error",
			mode:         "save-tool-is-error",
			wantContains: "tool error",
		},
		{
			name:         "server silent until deadline",
			mode:         "save-silent-after-init",
			timeout:      150 * time.Millisecond,
			wantContains: "deadline",
		},
		{
			name:         "server fully silent until deadline",
			mode:         "save-silent",
			timeout:      150 * time.Millisecond,
			wantContains: "deadline",
		},
		{
			name:         "garbage stdout",
			mode:         "save-garbage",
			wantContains: "invalid MCP stdout",
		},
		{
			name:         "child exits before initialize",
			mode:         "save-early-exit",
			wantContains: "exited without answering initialize",
		},
		{
			name:         "child exits after initialize before tools/call response",
			mode:         "save-early-exit-after-init",
			wantContains: "exited without answering mem_save",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helperCommand := setSaveHelperProcess(t, tt.mode)
			if tt.timeout != 0 {
				original := stdioProbeTimeout
				stdioProbeTimeout = tt.timeout
				t.Cleanup(func() { stdioProbeTimeout = original })
			}

			err := SaveTopic(context.Background(), "engram", []string{"mcp"}, SaveTopicRequest{
				Title:     "t",
				Content:   "c",
				Type:      "config",
				Project:   "axiom",
				TopicKey:  "skill-registry",
			})
			if err == nil {
				t.Fatal("SaveTopic() error = nil, want failure")
			}
			// Deadline cases surface the context cause; the rest carry a
			// protocol diagnosis. Both must remain ordinary errors.
			if tt.timeout != 0 {
				if err != context.DeadlineExceeded && !strings.Contains(strings.ToLower(err.Error()), tt.wantContains) {
					t.Fatalf("SaveTopic() error = %v, want deadline or %q", err, tt.wantContains)
				}
			} else if !strings.Contains(err.Error(), tt.wantContains) {
				t.Fatalf("SaveTopic() error = %v, want %q", err, tt.wantContains)
			}

			command := helperCommand()
			if command == nil {
				t.Fatal("SaveTopic() did not start its helper child")
			}
			if command.ProcessState == nil {
				t.Fatalf("SaveTopic() left its child running after failure: state=%#v", command.ProcessState)
			}
		})
	}
}
