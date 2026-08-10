package engram

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/claude"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"gopkg.in/yaml.v3"
)

// ErrNotInstalled reports that the supported bare Engram command could not be
// resolved through PATH. Callers (the doctor) map this to "not probed" instead
// of a transport failure, because binary presence is already covered by the
// tool:engram check.
var ErrNotInstalled = errors.New("engram binary not found on PATH")

// stdioProbeTimeout is the hard deadline for the whole stdio probe: spawning
// the engram MCP process plus the initialize handshake round-trip.
var stdioProbeTimeout = 5 * time.Second

// stdioHandshakeFn is a package-level seam over the real handshake (built on
// the execCommandContext precedent) so internal/cli doctor tests can pin the
// probe outcome without spawning a real process.
var stdioHandshakeFn = stdioHandshake

// StdioCommand is one stdio MCP command persisted in an installed agent's
// configuration. Source names the configuration file that supplied it.
type StdioCommand struct {
	Command string
	Args    []string
	Source  string
}

// ProbeStdio verifies the exact command and arguments persisted in an agent
// configuration. It deliberately does not resolve a replacement through the
// doctor's PATH: an absolute command in the config must be the process that is
// checked. Only a bare Engram command maps to ErrNotInstalled; a missing path
// or custom command is a broken configured transport.
func ProbeStdio(ctx context.Context, command string, args ...string) error {
	err := stdioHandshakeFn(ctx, command, args...)
	if isBareEngramCommand(command) && errors.Is(err, exec.ErrNotFound) {
		return ErrNotInstalled
	}
	return err
}

func isBareEngramCommand(command string) bool {
	if strings.ContainsAny(command, `/\\:`) {
		return false
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(command, "engram") || strings.EqualFold(command, "engram.exe")
	}
	return command == "engram"
}

// stdioHandshake spawns name with args and performs a minimal MCP initialize
// handshake over the newline-delimited JSON-RPC stdio transport. A successful
// probe proves only the bounded initialize exchange: it terminates the child,
// drains all stdout already produced to EOF, and rejects any extra frame.
func stdioHandshake(ctx context.Context, name string, args ...string) error {
	probeCtx, cancel := context.WithTimeout(ctx, stdioProbeTimeout)
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

	request := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"gentle-ai-doctor","version":"0"}}}` + "\n"
	if _, err := io.WriteString(stdin, request); err != nil {
		return fmt.Errorf("write engram mcp initialize request: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if err := validateInitializeResponse(scanner.Bytes()); err != nil {
			if cause := contextCause(); cause != nil {
				return cause
			}
			return err
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
			if err := validateInitializeResponse(scanner.Bytes()); err != nil {
				if cause := contextCause(); cause != nil {
					return cause
				}
				return err
			}
			return errors.New("engram mcp wrote an unexpected stdout frame after initialize response")
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
	if err := contextCause(); err != nil {
		return err
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read engram mcp output: %w", err)
	}
	return errors.New("engram mcp exited without answering initialize")
}

func expectedProbeTermination(err error) bool {
	if err == nil || errors.Is(err, os.ErrProcessDone) {
		return true
	}
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
}

func handshakeContextCause(ctx, probeCtx context.Context) error {
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	return context.Cause(probeCtx)
}

func validateInitializeResponse(line []byte) error {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return errors.New("invalid MCP stdout: empty frame")
	}

	var response struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  json.RawMessage `json:"method"`
		Result  json.RawMessage `json:"result"`
		Error   json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(line, &response); err != nil {
		return fmt.Errorf("invalid MCP stdout: malformed JSON-RPC frame: %w", err)
	}
	if response.JSONRPC != "2.0" {
		return errors.New(`invalid MCP stdout: jsonrpc must be "2.0"`)
	}
	if !bytes.Equal(bytes.TrimSpace(response.ID), []byte("1")) {
		return errors.New("invalid MCP stdout: unexpected response id")
	}
	if len(response.Method) > 0 {
		return errors.New("invalid MCP stdout: initialize response must not include method")
	}
	if len(response.Error) > 0 {
		if bytes.Equal(bytes.TrimSpace(response.Error), []byte("null")) {
			return errors.New("invalid MCP stdout: error must be absent or a JSON-RPC error object")
		}
		return fmt.Errorf("engram mcp initialize returned error: %s", response.Error)
	}
	if len(response.Result) == 0 {
		return errors.New("engram mcp initialize returned invalid result: result is required")
	}
	if err := validateInitializeResult(response.Result); err != nil {
		return fmt.Errorf("engram mcp initialize returned invalid result: %w", err)
	}
	return nil
}

func validateInitializeResult(raw json.RawMessage) error {
	var result map[string]json.RawMessage
	if err := json.Unmarshal(raw, &result); err != nil || result == nil {
		return errors.New("result must be an object")
	}

	if value, ok := result["protocolVersion"]; !ok || !isNonEmptyJSONString(value) {
		return errors.New("protocolVersion must be a non-empty string")
	}

	capabilities, ok := result["capabilities"]
	if !ok || !isJSONObject(capabilities) {
		return errors.New("capabilities must be an object")
	}

	serverInfo, ok := result["serverInfo"]
	if !ok {
		return errors.New("serverInfo is required")
	}
	var server map[string]json.RawMessage
	if err := json.Unmarshal(serverInfo, &server); err != nil || server == nil {
		return errors.New("serverInfo must be an object")
	}
	if !isNonEmptyJSONString(server["name"]) || !isNonEmptyJSONString(server["version"]) {
		return errors.New("serverInfo.name and serverInfo.version must be non-empty strings")
	}
	return nil
}

func isJSONObject(raw json.RawMessage) bool {
	var object map[string]json.RawMessage
	return json.Unmarshal(raw, &object) == nil && object != nil
}

func isNonEmptyJSONString(raw json.RawMessage) bool {
	var value string
	return json.Unmarshal(raw, &value) == nil && strings.TrimSpace(value) != ""
}

// ReadPersistedStdioCommands returns the unique stdio commands that installed
// agents currently persist for Engram. The doctor probes these values verbatim,
// rather than deriving a fresh command from its own environment.
func ReadPersistedStdioCommands(homeDir string, installedAgentIDs []string) ([]StdioCommand, error) {
	commands := make([]StdioCommand, 0, len(installedAgentIDs))
	seenPaths := make(map[string]struct{}, len(installedAgentIDs))
	seenCommands := make(map[string]struct{}, len(installedAgentIDs))

	for _, agentID := range installedAgentIDs {
		adapter, err := agents.NewAdapter(model.AgentID(agentID))
		if err != nil || !adapter.SupportsMCP() {
			continue
		}

		paths := []string{adapter.MCPConfigPath(homeDir, "engram")}
		if adapter.Agent() == model.AgentClaudeCode {
			// Claude Code reads user-scope MCP entries from ~/.claude.json.
			paths = []string{claude.UserConfigPath(homeDir)}
		}

		for _, path := range paths {
			if path == "" {
				continue
			}
			if _, seen := seenPaths[path]; seen {
				continue
			}
			seenPaths[path] = struct{}{}

			command, found, err := readPersistedStdioCommand(path, adapter.Agent())
			if err != nil {
				return nil, err
			}
			if !found {
				continue
			}
			command.Source = path
			key := command.Command + "\x00" + strings.Join(command.Args, "\x00")
			if _, seen := seenCommands[key]; seen {
				continue
			}
			seenCommands[key] = struct{}{}
			commands = append(commands, command)
		}
	}

	return commands, nil
}

func readPersistedStdioCommand(path string, agentID model.AgentID) (StdioCommand, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return StdioCommand{}, false, nil
		}
		return StdioCommand{}, false, fmt.Errorf("read persisted engram MCP configuration %q: %w", path, err)
	}

	switch strings.ToLower(filepath.Ext(path)) {
	case ".toml":
		command, found, err := readTOMLEngramCommand(string(raw))
		if err != nil {
			return StdioCommand{}, false, fmt.Errorf("parse persisted engram MCP configuration %q: %w", path, err)
		}
		return command, found, nil
	case ".yaml", ".yml":
		command, found, err := readYAMLEngramCommand(raw)
		if err != nil {
			return StdioCommand{}, false, fmt.Errorf("parse persisted engram MCP configuration %q: %w", path, err)
		}
		return command, found, nil
	default:
		root, err := filemerge.UnmarshalJSONObject(raw)
		if err != nil {
			return StdioCommand{}, false, fmt.Errorf("parse persisted engram MCP configuration %q: %w", path, err)
		}
		server, found := engramServerFromJSON(root, agentID)
		if !found {
			return StdioCommand{}, false, nil
		}
		command, err := stdioCommandFromServer(server)
		if err != nil {
			return StdioCommand{}, false, fmt.Errorf("parse persisted engram MCP configuration %q: %w", path, err)
		}
		return command, true, nil
	}
}

func engramServerFromJSON(root map[string]any, agentID model.AgentID) (map[string]any, bool) {
	if _, directCommand := root["command"]; directCommand {
		return root, true
	}

	var server any
	switch agentID {
	case model.AgentOpenCode, model.AgentKilocode:
		server = nestedJSONObject(root["mcp"])["engram"]
	case model.AgentOpenClaw:
		server = nestedJSONObject(nestedJSONObject(root["mcp"])["servers"])["engram"]
	case model.AgentVSCodeCopilot:
		server = nestedJSONObject(root["servers"])["engram"]
	default:
		server = nestedJSONObject(root["mcpServers"])["engram"]
	}

	entry, ok := server.(map[string]any)
	return entry, ok
}

func nestedJSONObject(value any) map[string]any {
	object, _ := value.(map[string]any)
	return object
}

func readTOMLEngramCommand(raw string) (StdioCommand, bool, error) {
	var config struct {
		MCPServers map[string]struct {
			Command string   `toml:"command"`
			Args    []string `toml:"args"`
		} `toml:"mcp_servers"`
	}
	if _, err := toml.Decode(raw, &config); err != nil {
		return StdioCommand{}, false, err
	}

	server, found := config.MCPServers["engram"]
	if !found {
		return StdioCommand{}, false, nil
	}
	if strings.TrimSpace(server.Command) == "" {
		return StdioCommand{}, true, errors.New("command must be a non-empty string")
	}
	return StdioCommand{Command: server.Command, Args: server.Args}, true, nil
}

func readYAMLEngramCommand(raw []byte) (StdioCommand, bool, error) {
	var root map[string]any
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return StdioCommand{}, false, err
	}
	server := nestedJSONObject(nestedJSONObject(root["mcp_servers"])["engram"])
	if server == nil {
		return StdioCommand{}, false, nil
	}
	command, err := stdioCommandFromServer(server)
	if err != nil {
		return StdioCommand{}, false, err
	}
	return command, true, nil
}

func stdioCommandFromServer(server map[string]any) (StdioCommand, error) {
	commandValue, ok := server["command"]
	if !ok {
		return StdioCommand{}, errors.New("command is required")
	}

	command, commandArgs, commandIsArray, err := splitCommand(commandValue)
	if err != nil {
		return StdioCommand{}, err
	}
	argsValue, hasArgs := server["args"]
	if commandIsArray && hasArgs {
		return StdioCommand{}, errors.New("command array must not also define args")
	}
	if commandIsArray {
		return StdioCommand{Command: command, Args: commandArgs}, nil
	}
	if !hasArgs {
		return StdioCommand{Command: command}, nil
	}
	args, err := stringSlice(argsValue)
	if err != nil {
		return StdioCommand{}, fmt.Errorf("args: %w", err)
	}
	return StdioCommand{Command: command, Args: args}, nil
}

func splitCommand(value any) (string, []string, bool, error) {
	if command, ok := value.(string); ok {
		if strings.TrimSpace(command) == "" {
			return "", nil, false, errors.New("command must be a non-empty string")
		}
		return command, nil, false, nil
	}

	values, err := stringSlice(value)
	if err != nil || len(values) == 0 || strings.TrimSpace(values[0]) == "" {
		return "", nil, false, errors.New("command must be a non-empty string or string array")
	}
	return values[0], values[1:], true, nil
}

func stringSlice(value any) ([]string, error) {
	values, ok := value.([]any)
	if !ok {
		return nil, errors.New("must be a string array")
	}
	result := make([]string, len(values))
	for i, value := range values {
		item, ok := value.(string)
		if !ok {
			return nil, errors.New("must be a string array")
		}
		result[i] = item
	}
	return result, nil
}
