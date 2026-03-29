package engram

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/assets"
	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

type InjectionResult struct {
	Changed bool
	Files   []string
}

// EngramLookPath is the function used to resolve the engram binary path.
// It is a package-level variable so it can be replaced in tests — both from
// within the engram package and from external test packages (e.g. golden_test.go).
// In production it is set to exec.LookPath.
var EngramLookPath = exec.LookPath

// SetLookPathForTest replaces EngramLookPath with a mock for the duration of
// a test and restores the original after the test completes. Exported so that
// external test packages (e.g. golden_test.go in components) can control the
// resolved engram path.
func SetLookPathForTest(t interface {
	Helper()
	Cleanup(func())
}, result, errMsg string) {
	t.Helper()
	orig := EngramLookPath
	EngramLookPath = func(string) (string, error) {
		if errMsg != "" {
			return "", fmt.Errorf("%s", errMsg)
		}
		return result, nil
	}
	t.Cleanup(func() { EngramLookPath = orig })
}

// resolveEngramCommand attempts to resolve the engram binary to an absolute
// path using exec.LookPath. If found, it returns the absolute path and true.
// If not found (e.g. binary not yet installed), it returns "engram" and false.
// This is used to write the most stable command possible into MCP configs:
// an absolute path survives across environments where PATH is not fully
// inherited (e.g. Windsurf, IDEs that launch without a login shell).
func resolveEngramCommand() (string, bool) {
	p, err := EngramLookPath("engram")
	if err != nil || p == "" {
		return "engram", false
	}
	return p, true
}

// engramServerJSON returns the MCP server config bytes, using the absolute
// path to the engram binary if it can be resolved via PATH.
func engramServerJSON() []byte {
	cmd, _ := resolveEngramCommand()
	cfg := map[string]any{
		"command": cmd,
		"args":    []string{"mcp", "--tools=agent"},
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return append(b, '\n')
}

// engramOverlayJSON returns the settings overlay JSON (used for merge-into-settings
// and MCPConfigFile strategies), with the resolved engram command.
func engramOverlayJSON(agentID model.AgentID) []byte {
	cmd, _ := resolveEngramCommand()
	var cfg map[string]any
	if agentID == model.AgentOpenCode {
		// OpenCode 1.3.3+ requires command as an array for type:local servers.
		// The separate "args" field is not accepted; all args must be in the
		// command array itself.
		//
		// Use the __replace__ sentinel so that MergeJSONObjects replaces the
		// entire mcp.engram object atomically instead of deep-merging into it.
		// Without this, users upgrading from v1.11.3 (which had a separate
		// "args" key) would end up with both "args" and the new array "command"
		// in their config, which is invalid for OpenCode 1.3.3.
		cfg = map[string]any{
			"mcp": map[string]any{
				"engram": map[string]any{
					"__replace__": map[string]any{
						"command": []string{cmd, "mcp", "--tools=agent"},
						"type":    "local",
					},
				},
			},
		}
	} else {
		cfg = map[string]any{
			"mcpServers": map[string]any{
				"engram": map[string]any{
					"command": cmd,
					"args":    []string{"mcp", "--tools=agent"},
				},
			},
		}
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return append(b, '\n')
}

// vsCodeEngramOverlayJSON is the VS Code mcp.json overlay using the "servers" key.
// Uses --tools=agent per engram contract.
// VS Code uses a fixed "servers" key structure rather than mcpServers, so it
// is kept as a separate helper.
func vsCodeEngramOverlayJSON() []byte {
	cmd, _ := resolveEngramCommand()
	cfg := map[string]any{
		"servers": map[string]any{
			"engram": map[string]any{
				"command": cmd,
				"args":    []string{"mcp", "--tools=agent"},
			},
		},
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return append(b, '\n')
}

func Inject(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
	if !adapter.SupportsMCP() {
		return InjectionResult{}, nil
	}

	files := make([]string, 0, 2)
	changed := false

	// 1. Write MCP server config using the adapter's strategy.
	switch adapter.MCPStrategy() {
	case model.StrategySeparateMCPFiles:
		// Engram v1.10.3+ writes an absolute path for the command field when
		// `engram setup <agent>` is invoked. gentle-ai's Inject() runs after
		// engram setup, so we must preserve any absolute command path already
		// present instead of silently overwriting it with the relative "engram".
		// See: https://github.com/Gentleman-Programming/gentle-ai/issues (engram absolute path regression)
		mcpPath := adapter.MCPConfigPath(homeDir, "engram")
		content := buildSeparateMCPContent(mcpPath, engramServerJSON())
		mcpWrite, err := filemerge.WriteFileAtomic(mcpPath, content, 0o644)
		if err != nil {
			return InjectionResult{}, err
		}
		changed = changed || mcpWrite.Changed
		files = append(files, mcpPath)

	case model.StrategyMergeIntoSettings:
		settingsPath := adapter.SettingsPath(homeDir)
		if settingsPath == "" {
			break
		}
		overlay := engramOverlayJSON(adapter.Agent())
		settingsWrite, err := mergeJSONFile(settingsPath, overlay)
		if err != nil {
			return InjectionResult{}, err
		}
		changed = changed || settingsWrite.Changed
		files = append(files, settingsPath)

	case model.StrategyMCPConfigFile:
		mcpPath := adapter.MCPConfigPath(homeDir, "engram")
		if mcpPath == "" {
			break
		}
		var overlay []byte
		if adapter.Agent() == model.AgentVSCodeCopilot {
			overlay = vsCodeEngramOverlayJSON()
		} else {
			overlay = engramOverlayJSON(adapter.Agent())
		}

		mcpWrite, err := mergeJSONFile(mcpPath, overlay)
		if err != nil {
			return InjectionResult{}, err
		}
		changed = changed || mcpWrite.Changed
		files = append(files, mcpPath)

	case model.StrategyTOMLFile:
		// Codex: upsert [mcp_servers.engram] block and instruction-file keys
		// in ~/.codex/config.toml, then write instruction files.
		// All TOML mutations are composed in a single pass before writing to
		// ensure idempotency (no intermediate states that differ on re-run).
		configPath := adapter.MCPConfigPath(homeDir, "engram")
		if configPath == "" {
			break
		}

		// Determine instruction file paths before mutating the config.
		instructionsPath, compactPath, instrErr := writeCodexInstructionFiles(homeDir)
		if instrErr != nil {
			return InjectionResult{}, instrErr
		}

		// Read existing config and apply all mutations in one pass.
		existing, err := readFileOrEmpty(configPath)
		if err != nil {
			return InjectionResult{}, err
		}
		engramCmd, _ := resolveEngramCommand()
		withMCP := filemerge.UpsertCodexEngramBlock(existing, engramCmd)
		withInstr := filemerge.UpsertTopLevelTOMLString(withMCP, "model_instructions_file", instructionsPath)
		withCompact := filemerge.UpsertTopLevelTOMLString(withInstr, "experimental_compact_prompt_file", compactPath)

		tomlWrite, err := filemerge.WriteFileAtomic(configPath, []byte(withCompact), 0o644)
		if err != nil {
			return InjectionResult{}, err
		}
		changed = changed || tomlWrite.Changed
		files = append(files, configPath)
	}

	// 2. Inject Engram memory protocol into system prompt (if supported).
	if adapter.SupportsSystemPrompt() {
		switch adapter.SystemPromptStrategy() {
		case model.StrategyMarkdownSections:
			promptPath := adapter.SystemPromptFile(homeDir)
			protocolContent := assets.MustRead("claude/engram-protocol.md")

			existing, err := readFileOrEmpty(promptPath)
			if err != nil {
				return InjectionResult{}, err
			}

			updated := filemerge.InjectMarkdownSection(existing, "engram-protocol", protocolContent)

			mdWrite, err := filemerge.WriteFileAtomic(promptPath, []byte(updated), 0o644)
			if err != nil {
				return InjectionResult{}, err
			}
			changed = changed || mdWrite.Changed
			files = append(files, promptPath)

		default:
			promptPath := adapter.SystemPromptFile(homeDir)
			protocolContent := assets.MustRead("claude/engram-protocol.md")

			existing, err := readFileOrEmpty(promptPath)
			if err != nil {
				return InjectionResult{}, err
			}

			updated := filemerge.InjectMarkdownSection(existing, "engram-protocol", protocolContent)

			mdWrite, err := filemerge.WriteFileAtomic(promptPath, []byte(updated), 0o644)
			if err != nil {
				return InjectionResult{}, err
			}
			changed = changed || mdWrite.Changed
			files = append(files, promptPath)
		}
	}

	return InjectionResult{Changed: changed, Files: files}, nil
}

// writeCodexInstructionFiles writes the Engram memory protocol and compact prompt
// files to ~/.codex/ and returns their paths.
func writeCodexInstructionFiles(homeDir string) (instructionsPath, compactPath string, err error) {
	codexDir := homeDir + "/.codex"
	instructionsPath = codexDir + "/engram-instructions.md"
	compactPath = codexDir + "/engram-compact-prompt.md"

	instrContent := assets.MustRead("codex/engram-instructions.md")
	instrWrite, err := filemerge.WriteFileAtomic(instructionsPath, []byte(instrContent), 0o644)
	if err != nil {
		return "", "", fmt.Errorf("write codex engram-instructions.md: %w", err)
	}
	_ = instrWrite

	compactContent := assets.MustRead("codex/engram-compact-prompt.md")
	compactWrite, err := filemerge.WriteFileAtomic(compactPath, []byte(compactContent), 0o644)
	if err != nil {
		return "", "", fmt.Errorf("write codex engram-compact-prompt.md: %w", err)
	}
	_ = compactWrite

	return instructionsPath, compactPath, nil
}

func mergeJSONFile(path string, overlay []byte) (filemerge.WriteResult, error) {
	baseJSON, err := osReadFile(path)
	if err != nil {
		return filemerge.WriteResult{}, err
	}

	merged, err := filemerge.MergeJSONObjects(baseJSON, overlay)
	if err != nil {
		return filemerge.WriteResult{}, err
	}

	return filemerge.WriteFileAtomic(path, merged, 0o644)
}

var osReadFile = func(path string) ([]byte, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read json file %q: %w", path, err)
	}

	return content, nil
}

func readFileOrEmpty(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read file %q: %w", path, err)
	}
	return string(data), nil
}

// buildSeparateMCPContent returns the content to write to the MCP server JSON
// file for agents that use the StrategySeparateMCPFiles strategy (e.g. Claude
// Code).
//
// Engram v1.10.3+ writes an absolute command path when `engram setup` is run.
// gentle-ai runs Inject() after setup, so we must not overwrite that absolute
// path with the relative "engram" string from defaultEngramServerJSON.
//
// Logic:
//   - If the file does not exist yet, return defaultContent unchanged.
//   - If the file exists but cannot be parsed as JSON, return defaultContent.
//   - If the parsed JSON has a "command" value that is an absolute path to the
//     engram binary, rebuild the config using that command and the canonical
//     args (["mcp", "--tools=agent"]) so that the absolute path is preserved
//     and the correct flags are always present.
//   - Otherwise (relative command or other value), return defaultContent.
func buildSeparateMCPContent(mcpPath string, defaultContent []byte) []byte {
	raw, err := os.ReadFile(mcpPath)
	if err != nil {
		// File does not exist or is not readable — use the default.
		return defaultContent
	}

	var existing map[string]any
	if err := json.Unmarshal(raw, &existing); err != nil {
		// Malformed JSON — use the default.
		return defaultContent
	}

	cmd, ok := existing["command"].(string)
	if !ok || !isAbsoluteEngramPath(cmd) {
		// No command, or not an absolute path — use the default.
		return defaultContent
	}

	// Rebuild with the preserved absolute command and the canonical args.
	rebuilt := map[string]any{
		"command": cmd,
		"args":    []string{"mcp", "--tools=agent"},
	}
	encoded, err := json.MarshalIndent(rebuilt, "", "  ")
	if err != nil {
		// Should be impossible with a plain map — use the default as fallback.
		return defaultContent
	}
	return append(encoded, '\n')
}

// isAbsoluteEngramPath reports whether path is an absolute filesystem path
// that points to an engram binary.
//
// Engram setup writes the full resolved path of the binary it was invoked
// from, so any absolute path ending in "engram" (Unix) or "engram.exe"
// (Windows) is considered valid.
func isAbsoluteEngramPath(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	base := filepath.Base(path)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(base, "engram.exe") || strings.EqualFold(base, "engram")
	}
	return base == "engram"
}
