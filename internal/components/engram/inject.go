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
	"github.com/gentleman-programming/gentle-ai/internal/agents/codex"
	"github.com/gentleman-programming/gentle-ai/internal/assets"
	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

type InjectionResult struct {
	Changed bool
	Files   []string
}

// bootstrapper is an optional adapter capability: if an adapter implements
// this interface, any injector that writes Jinja modules will first ensure
// the base template (entry point) exists.
type bootstrapper interface {
	BootstrapTemplate(homeDir string) error
}

type piEngramProvisioner interface {
	ProvisionEngramMCP(homeDir string) (changed bool, files []string, err error)
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
	if isVersionedHomebrewCellarPath(p) {
		return "engram", false
	}
	return p, true
}

// engramServerJSON returns the MCP server config bytes, using the absolute
// path to the engram binary if it can be resolved via PATH.
func engramServerJSON() []byte {
	cmd, _ := resolveEngramCommand()
	return engramServerJSONWithCmd(cmd)
}

// engramServerJSONWithCmd returns the MCP server config bytes for a specific
// command.
func engramServerJSONWithCmd(cmd string) []byte {
	cfg := map[string]any{
		"command": cmd,
		"args":    []string{"mcp", "--tools=agent"},
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return append(b, '\n')
}

// engramOverlayJSON returns the settings overlay JSON (used for merge-into-settings
// and MCPConfigFile strategies), with the resolved engram command.
func engramOverlayJSON(agentID model.AgentID, cmd string) []byte {
	var cfg map[string]any
	if agentID == model.AgentOpenCode || agentID == model.AgentKilocode {
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
	} else if agentID == model.AgentOpenClaw {
		cfg = map[string]any{
			"mcp": map[string]any{
				"servers": map[string]any{
					"engram": map[string]any{
						"__replace__": map[string]any{
							"command": cmd,
							"args":    []string{"mcp", "--tools=agent"},
						},
					},
				},
			},
		}
	} else {
		args := []string{"mcp", "--tools=agent"}
		if agentID == model.AgentAntigravity {
			// Antigravity should launch the default Engram MCP server without
			// narrowing the exposed tool set.
			args = []string{"mcp"}
		}
		cfg = map[string]any{
			"mcpServers": map[string]any{
				"engram": map[string]any{
					"command": cmd,
					"args":    args,
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
func vsCodeEngramOverlayJSON(cmd string) []byte {
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

// InjectOptions carries optional configuration for an Inject call.
// Zero value is always safe — all fields have documented defaults.
type InjectOptions struct {
	// CodexMultiAgent controls whether features.multi_agent is written as true
	// in ~/.codex/config.toml. Default (false) writes multi_agent = false, which
	// is the safe no-op value for the experimental Codex multi-agent tool set.
	// Set to true only when the user explicitly opts in via a CLI flag or TUI choice.
	CodexMultiAgent bool

	// CodexCarrilModelAssignments holds the resolved carril→model-id map used
	// when writing SDD profile .config.toml files. nil/empty = use canonical
	// defaults (sdd-strong/sdd-mid=gpt-5.5, sdd-cheap=gpt-5.4-mini).
	CodexCarrilModelAssignments map[string]string

	// CodexModelAssignments holds the resolved phase→effort map used to derive
	// the per-carril reasoning_effort written to SDD profile files.
	// nil/empty = use canonical defaults.
	CodexModelAssignments map[string]model.CodexEffort
}

func Inject(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
	return injectWithOptions(homeDir, homeDir, adapter, InjectOptions{})
}

// InjectWithOptions is like Inject but accepts additional options such as the
// Codex multi-agent opt-in flag. Use this when the caller has a model.Selection
// and needs to forward user-chosen configuration into the injection pass.
func InjectWithOptions(homeDir string, adapter agents.Adapter, opts InjectOptions) (InjectionResult, error) {
	return injectWithOptions(homeDir, homeDir, adapter, opts)
}

// InjectWithPromptDir writes Engram's MCP configuration using configHomeDir and
// writes prompt protocol files using promptDir. This is needed for agents such
// as OpenClaw where MCP is loaded from the global config but instructions are
// read from an active workspace.
func InjectWithPromptDir(configHomeDir, promptDir string, adapter agents.Adapter) (InjectionResult, error) {
	return injectWithOptions(configHomeDir, promptDir, adapter, InjectOptions{})
}

const antigravityEngramPluginJSON = `{
  "name": "gentle-ai-engram",
  "description": "Loads Engram MCP memory tools for Antigravity sessions.",
  "version": "0.1.0"
}
`

const antigravityEngramToolsMessage = "CRITICAL FIRST ACTION — Ensure these Engram MCP tools are available before responding: mem_save, mem_search, mem_context, mem_session_summary, mem_session_start, mem_session_end, mem_get_observation, mem_suggest_topic_key, mem_capture_passive, mem_save_prompt, mem_update, mem_current_project, mem_judge. When available, also load/select optional mem_review for memory lifecycle review; if mem_review is unavailable, continue with the required tools above. If Antigravity defers MCP tools, load/select these tools from the engram MCP server first. Then call mem_context when the user asks about prior work or the session needs project memory."

func antigravityEngramHooksJSON() []byte {
	cfg := map[string]any{
		"gentle-ai-engram-tools": map[string]any{
			"PreInvocation": []any{
				map[string]any{
					"type": "command",
					"command": "printf '%s\\n' '" + mustJSONString(map[string]any{
						"injectSteps": []any{
							map[string]any{"ephemeralMessage": antigravityEngramToolsMessage},
						},
					}) + "'",
				},
			},
		},
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return append(b, '\n')
}

func mustJSONString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func ensureJSONFileIfMissing(path string) (filemerge.WriteResult, error) {
	if _, err := os.Stat(path); err == nil {
		return filemerge.WriteResult{Changed: false}, nil
	} else if !os.IsNotExist(err) {
		return filemerge.WriteResult{}, err
	}
	return filemerge.WriteFileAtomic(path, []byte("{}\n"), 0o644)
}

func installAntigravityEngramPlugin(homeDir, engramCommand string) (bool, []string, error) {
	pluginDir := filepath.Join(homeDir, ".gemini", "antigravity-cli", "plugins", "gentle-ai-engram")
	files := make([]string, 0, 3)
	changed := false

	pluginPath := filepath.Join(pluginDir, "plugin.json")
	pluginWrite, err := filemerge.WriteFileAtomic(pluginPath, []byte(antigravityEngramPluginJSON), 0o644)
	if err != nil {
		return false, nil, fmt.Errorf("write Antigravity Engram plugin manifest: %w", err)
	}
	changed = changed || pluginWrite.Changed
	files = append(files, pluginPath)

	pluginMCPPath := filepath.Join(pluginDir, "mcp_config.json")
	mcpWrite, err := filemerge.WriteFileAtomic(pluginMCPPath, engramOverlayJSON(model.AgentAntigravity, engramCommand), 0o644)
	if err != nil {
		return false, nil, fmt.Errorf("write Antigravity Engram plugin MCP config: %w", err)
	}
	changed = changed || mcpWrite.Changed
	files = append(files, pluginMCPPath)

	hooksPath := filepath.Join(pluginDir, "hooks.json")
	hooksWrite, err := filemerge.WriteFileAtomic(hooksPath, antigravityEngramHooksJSON(), 0o644)
	if err != nil {
		return false, nil, fmt.Errorf("write Antigravity Engram hooks: %w", err)
	}
	changed = changed || hooksWrite.Changed
	files = append(files, hooksPath)

	return changed, files, nil
}

func injectWithOptions(configHomeDir, promptDir string, adapter agents.Adapter, opts InjectOptions) (InjectionResult, error) {
	if provisioner, ok := adapter.(piEngramProvisioner); ok {
		changed, files, err := provisioner.ProvisionEngramMCP(configHomeDir)
		if err != nil {
			return InjectionResult{}, err
		}
		return InjectionResult{Changed: changed, Files: files}, nil
	}

	if !adapter.SupportsMCP() {
		return InjectionResult{}, nil
	}
	if err := validateOpenClawWorkspacePath(promptDir, adapter); err != nil {
		return InjectionResult{}, err
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
		mcpPath := adapter.MCPConfigPath(configHomeDir, "engram")
		cmd := stableEngramCommandForMergedConfig(mcpPath, adapter.Agent())
		content := buildSeparateMCPContent(mcpPath, engramServerJSONWithCmd(cmd))
		mcpWrite, err := filemerge.WriteFileAtomic(mcpPath, content, 0o644)
		if err != nil {
			return InjectionResult{}, err
		}
		changed = changed || mcpWrite.Changed
		files = append(files, mcpPath)

	case model.StrategyMergeIntoSettings:
		settingsPath := adapter.SettingsPath(configHomeDir)
		if settingsPath == "" {
			break
		}
		overlay := engramOverlayJSON(adapter.Agent(), stableEngramCommandForMergedConfig(settingsPath, adapter.Agent()))
		settingsWrite, err := mergeJSONFile(settingsPath, overlay)
		if err != nil {
			return InjectionResult{}, err
		}
		changed = changed || settingsWrite.Changed
		files = append(files, settingsPath)

	case model.StrategyMCPConfigFile:
		mcpPath := adapter.MCPConfigPath(configHomeDir, "engram")
		if mcpPath == "" {
			break
		}
		engramCommand := stableEngramCommandForMergedConfig(mcpPath, adapter.Agent())
		var overlay []byte
		if adapter.Agent() == model.AgentVSCodeCopilot {
			overlay = vsCodeEngramOverlayJSON(engramCommand)
		} else {
			overlay = engramOverlayJSON(adapter.Agent(), engramCommand)
		}

		mcpWrite, err := mergeJSONFile(mcpPath, overlay)
		if err != nil {
			return InjectionResult{}, err
		}
		changed = changed || mcpWrite.Changed
		files = append(files, mcpPath)

		if adapter.Agent() == model.AgentAntigravity {
			settingsWrite, settingsErr := ensureJSONFileIfMissing(adapter.SettingsPath(configHomeDir))
			if settingsErr != nil {
				return InjectionResult{}, fmt.Errorf("ensure Antigravity settings: %w", settingsErr)
			}
			changed = changed || settingsWrite.Changed
			files = append(files, adapter.SettingsPath(configHomeDir))

			pluginChanged, pluginFiles, pluginErr := installAntigravityEngramPlugin(configHomeDir, engramCommand)
			if pluginErr != nil {
				return InjectionResult{}, pluginErr
			}
			changed = changed || pluginChanged
			files = append(files, pluginFiles...)
		}

	case model.StrategyMergeIntoYAML:
		// Hermes: upsert the engram MCP server block under mcp_servers: in
		// ~/.hermes/config.yaml via the comment-preserving YAML string helpers.
		configPath := adapter.MCPConfigPath(configHomeDir, "engram")
		existing, readErr := readFileOrEmpty(configPath)
		if readErr != nil {
			return InjectionResult{}, readErr
		}
		engramCmd := stableEngramCommandForMergedConfig(configPath, adapter.Agent())
		updated := filemerge.UpsertHermesEngramBlock(existing, engramCmd)
		yamlWrite, writeErr := filemerge.WriteFileAtomic(configPath, []byte(updated), 0o644)
		if writeErr != nil {
			return InjectionResult{}, fmt.Errorf("write Hermes engram YAML %q: %w", configPath, writeErr)
		}
		changed = changed || yamlWrite.Changed
		files = append(files, configPath)

	case model.StrategyTOMLFile:
		// Codex: upsert [mcp_servers.engram] block and instruction-file keys
		// in ~/.codex/config.toml, then write instruction files.
		// All TOML mutations are composed in a single pass before writing to
		// ensure idempotency (no intermediate states that differ on re-run).
		configPath := adapter.MCPConfigPath(configHomeDir, "engram")
		if configPath == "" {
			break
		}

		// Determine instruction file paths before mutating the config.
		instructionsPath, compactPath, instrErr := writeCodexInstructionFiles(configHomeDir)
		if instrErr != nil {
			return InjectionResult{}, instrErr
		}

		// Read existing config and apply all mutations in a single pass.
		//
		// Mutation order (matters for idempotency):
		//   1. UpsertTOMLTableKey for [features] and [agents] — these are
		//      applied FIRST so that the section headers are present before
		//      UpsertTopLevelTOMLString and UpsertCodexEngramBlock run. When
		//      the sections already exist, their keys are replaced in place.
		//   2. UpsertTopLevelTOMLString for instruction-file keys — places
		//      top-level keys before the first [section] header (i.e., before
		//      [features]). Running model→experimental in this order is
		//      self-correcting on re-runs (the pair of calls always produces
		//      the same final ordering).
		//   3. UpsertCodexEngramBlock — strips the [mcp_servers.engram] block
		//      and re-appends it at EOF on every call. Running last ensures
		//      engram always ends up at the bottom of the file. On re-runs,
		//      UpsertCodexEngramBlock stops at [features] (the next section
		//      header after engram on disk), so [features] and [agents] are
		//      preserved above engram — the stable layout is:
		//        model_instructions_file / experimental_compact_prompt_file
		//        [features] / [agents]
		//        [mcp_servers.engram]
		existing, err := readFileOrEmpty(configPath)
		if err != nil {
			return InjectionResult{}, err
		}

		// Step 1 — multi-agent SDD enablement keys ([features] and [agents]).
		// features.multi_agent is enabled by default: Codex SDD delegates phases via
		// spawn_agent so the per-phase reasoning_effort table actually applies. The
		// orchestrator asset gracefully falls back to solo execution if the multi-agent
		// tools are unavailable in the session. agents.max_threads/max_depth carry
		// conservative defaults.
		withFeatures := filemerge.UpsertTOMLTableKey(existing, "features", "multi_agent", "true")
		withMaxThreads := filemerge.UpsertTOMLTableKey(withFeatures, "agents", "max_threads", "4")
		withMaxDepth := filemerge.UpsertTOMLTableKey(withMaxThreads, "agents", "max_depth", "2")

		// Step 2 — top-level instruction-file keys (before the first section header).
		withInstr := filemerge.UpsertTopLevelTOMLString(withMaxDepth, "model_instructions_file", instructionsPath)
		withCompact := filemerge.UpsertTopLevelTOMLString(withInstr, "experimental_compact_prompt_file", compactPath)

		// Step 3 — [mcp_servers.engram] block (always last; strip+re-append at EOF).
		engramCmd := stableEngramCommandForMergedConfig(configPath, adapter.Agent())
		withMCP := filemerge.UpsertCodexEngramBlock(withCompact, engramCmd)

		tomlWrite, err := filemerge.WriteFileAtomic(configPath, []byte(withMCP), 0o644)
		if err != nil {
			return InjectionResult{}, err
		}
		changed = changed || tomlWrite.Changed
		files = append(files, configPath)

		// Write gentle-ai SDD model-selection profile files into ~/.codex/.
		// These use the separate-file mechanism from Codex >= 0.134.0 and are
		// selected at runtime via `codex --profile <name>`.
		// codexHomeDir is the ~/.codex directory (the parent of config.toml).
		codexHomeDir := filepath.Dir(configPath)
		profileAssignments := resolveProfileAssignments(opts.CodexCarrilModelAssignments, opts.CodexModelAssignments)
		profilesChanged, profileFiles, profileErr := codex.WriteCodexProfiles(codexHomeDir, profileAssignments)
		if profileErr != nil {
			return InjectionResult{}, profileErr
		}
		changed = changed || profilesChanged
		files = append(files, profileFiles...)
	}

	// 2. Inject Engram memory protocol into system prompt (if supported).
	if adapter.SupportsSystemPrompt() {
		switch adapter.SystemPromptStrategy() {
		case model.StrategyMarkdownSections:
			promptPath := adapter.SystemPromptFile(promptDir)
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

		case model.StrategyJinjaModules:
			// Ensure the base template exists for Jinja-based agents.
			if bs, ok := adapter.(bootstrapper); ok {
				if err := bs.BootstrapTemplate(promptDir); err != nil {
					return InjectionResult{}, fmt.Errorf("bootstrap template: %w", err)
				}
			}

			// Write the Engram protocol as a standalone Jinja include module.
			// The static KIMI.md template references it via {% include "engram-protocol.md" %}.
			configDir := adapter.GlobalConfigDir(promptDir)
			protocolContent := assets.MustRead("claude/engram-protocol.md")
			modulePath := filepath.Join(configDir, "engram-protocol.md")
			mdWrite, err := filemerge.WriteFileAtomic(modulePath, []byte(protocolContent), 0o644)
			if err != nil {
				return InjectionResult{}, err
			}
			changed = changed || mdWrite.Changed
			files = append(files, modulePath)

		default:
			promptPath := adapter.SystemPromptFile(promptDir)
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

func validateOpenClawWorkspacePath(workspaceDir string, adapter agents.Adapter) error {
	if adapter.Agent() == model.AgentOpenClaw && strings.TrimSpace(workspaceDir) == "" {
		return fmt.Errorf("openclaw workspace path is required for workspace-first injection")
	}
	return nil
}

type settingsBootstrapResult struct {
	Changed bool
	Path    string
}

func ensureAntigravitySettings(homeDir string, adapter agents.Adapter) (settingsBootstrapResult, error) {
	settingsPath := adapter.SettingsPath(homeDir)
	if settingsPath == "" {
		return settingsBootstrapResult{}, nil
	}

	if _, err := os.Stat(settingsPath); err == nil {
		return settingsBootstrapResult{Path: settingsPath}, nil
	} else if !os.IsNotExist(err) {
		return settingsBootstrapResult{}, fmt.Errorf("stat antigravity settings %q: %w", settingsPath, err)
	}

	sourcePath := filepath.Join(homeDir, ".gemini", "settings.json")
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return settingsBootstrapResult{}, fmt.Errorf("read gemini settings %q: %w", sourcePath, err)
		}
		content = []byte("{}")
	}

	writeResult, err := filemerge.WriteFileAtomic(settingsPath, content, 0o644)
	if err != nil {
		return settingsBootstrapResult{}, err
	}

	return settingsBootstrapResult{Changed: writeResult.Changed, Path: settingsPath}, nil
}

// writeCodexInstructionFiles writes the Engram memory protocol and compact prompt
// files to ~/.codex/ and returns their paths.
func writeCodexInstructionFiles(homeDir string) (instructionsPath, compactPath string, err error) {
	codexDir := filepath.Join(homeDir, ".codex")
	instructionsPath = filepath.Join(codexDir, "engram-instructions.md")
	compactPath = filepath.Join(codexDir, "engram-compact-prompt.md")

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

func stableEngramCommandForMergedConfig(path string, agentID model.AgentID) string {
	raw, err := osReadFile(path)
	if err == nil {
		if cmd, ok := existingMergedEngramCommand(raw, agentID); ok {
			return stableEngramCommandForExisting(cmd, agentID)
		}
	}

	if isStandardAgent(agentID) {
		return preferredStableEngramCommand()
	}

	cmd, _ := resolveEngramCommand()
	return cmd
}

func stableEngramCommandForExisting(cmd string, agentID model.AgentID) string {
	if isVersionedHomebrewCellarPath(cmd) {
		if stable := preferredStableEngramCommand(); stable != "" {
			return stable
		}
		return "engram"
	}

	return cmd
}

func preferredStableEngramCommand() string {
	p, err := EngramLookPath("engram")
	if err == nil && isStableHomebrewEngramPath(p) {
		return p
	}
	return "engram"
}

func existingMergedEngramCommand(raw []byte, agentID model.AgentID) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}

	// YAML recovery early branch for Hermes: placed before MergeJSONObjects
	// (which would fail on YAML input). ReadYAMLMCPServerCommand scans the
	// ~/.hermes/config.yaml content for the named server's command — no
	// external YAML dependency, read-only. (Decision 9)
	if agentID == model.AgentHermes {
		return filemerge.ReadYAMLMCPServerCommand(string(raw), "engram")
	}

	normalized, err := filemerge.MergeJSONObjects(raw, []byte("{}"))
	if err != nil {
		return "", false
	}

	var root map[string]any
	if err := json.Unmarshal(normalized, &root); err != nil {
		return "", false
	}

	var server any
	switch agentID {
	case model.AgentOpenCode:
		mcp, ok := root["mcp"].(map[string]any)
		if !ok {
			return "", false
		}
		server = mcp["engram"]
	case model.AgentOpenClaw:
		mcp, ok := root["mcp"].(map[string]any)
		if !ok {
			return "", false
		}
		servers, ok := mcp["servers"].(map[string]any)
		if !ok {
			return "", false
		}
		server = servers["engram"]
	case model.AgentVSCodeCopilot:
		servers, ok := root["servers"].(map[string]any)
		if !ok {
			return "", false
		}
		server = servers["engram"]
	default:
		mcpServers, ok := root["mcpServers"].(map[string]any)
		if !ok {
			return "", false
		}
		server = mcpServers["engram"]
	}

	serverMap, ok := server.(map[string]any)
	if !ok {
		return "", false
	}

	return executableFromCommandValue(serverMap["command"])
}

func executableFromCommandValue(command any) (string, bool) {
	switch value := command.(type) {
	case string:
		if value == "" {
			return "", false
		}
		return value, true
	case []any:
		if len(value) == 0 {
			return "", false
		}
		first, ok := value[0].(string)
		if !ok || first == "" {
			return "", false
		}
		return first, true
	default:
		return "", false
	}
}

func isStandardAgent(id model.AgentID) bool {
	switch id {
	case model.AgentOpenCode, model.AgentQwenCode, model.AgentCodex, model.AgentGeminiCLI, model.AgentAntigravity, model.AgentClaudeCode, model.AgentOpenClaw, model.AgentHermes:
		return true
	default:
		return false
	}
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

	cmd, ok := executableFromCommandValue(existing["command"])
	if !ok || !isEngramCommand(cmd) {
		// No command, or not an engram command — use the default.
		return defaultContent
	}
	cmd = stableEngramCommandForExisting(cmd, "")

	// Rebuild with the preserved command and the canonical args (["mcp", "--tools=agent"]).
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

// isEngramCommand reports whether cmd is either a relative "engram" command
// or an absolute path pointing to an engram binary.
func isEngramCommand(cmd string) bool {
	if cmd == "" {
		return false
	}
	base := filepath.Base(cmd)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(base, "engram.exe") || strings.EqualFold(base, "engram")
	}
	return base == "engram"
}

// isAbsoluteEngramPath reports whether path is an absolute filesystem path
// that points to an engram binary.
func isAbsoluteEngramPath(path string) bool {
	return filepath.IsAbs(path) && isEngramCommand(path)
}

func isVersionedHomebrewCellarPath(path string) bool {
	clean := filepath.ToSlash(filepath.Clean(path))
	return strings.Contains(clean, "/Cellar/engram/") && isEngramCommand(clean)
}

func isStableHomebrewEngramPath(path string) bool {
	clean := filepath.ToSlash(filepath.Clean(path))
	return (clean == "/opt/homebrew/bin/engram" || clean == "/usr/local/bin/engram") && isEngramCommand(clean)
}

// resolveProfileAssignments builds the []codex.ProfileAssignment slice used
// to write the three SDD profile .config.toml files. The carril→model map and
// the phase→effort map are resolved independently (they live on different axes)
// so either can be nil and the other still takes effect.
//
//   - nil carrilModels → model for each carril falls back to model.DefaultCarrilModels.
//   - nil phaseEfforts → effort for each carril falls back to the carril's canonical
//     DefaultEffort from model.CodexTierGroups (Recommended preset values).
//
// Single source of truth: tier definitions (phases, default effort, default model)
// are read from model.CodexTierGroups instead of a duplicate local table.
func resolveProfileAssignments(carrilModels map[string]string, phaseEfforts map[string]model.CodexEffort) []codex.ProfileAssignment {
	tiers := model.CodexTierGroups()

	effortRank := map[model.CodexEffort]int{
		model.CodexEffortLow:    0,
		model.CodexEffortMedium: 1,
		model.CodexEffortHigh:   2,
		model.CodexEffortXHigh:  3,
	}

	out := make([]codex.ProfileAssignment, 0, len(tiers))
	for _, t := range tiers {
		// Resolve model: carrilModels override, fall back to canonical default.
		mdl := t.Model
		if v, ok := carrilModels[t.Profile]; ok && v != "" {
			mdl = v
		}

		// Resolve effort: max over assigned phases, fall back to carril's DefaultEffort.
		eff := t.DefaultEffort
		if len(phaseEfforts) > 0 {
			best := model.CodexEffort("")
			bestRank := -1
			for _, phase := range t.Phases {
				if e, ok := phaseEfforts[phase]; ok {
					if r, ok2 := effortRank[e]; ok2 && r > bestRank {
						bestRank = r
						best = e
					}
				}
			}
			if best != "" {
				eff = best
			}
		}

		out = append(out, codex.ProfileAssignment{
			Profile:         t.Profile,
			Model:           mdl,
			ReasoningEffort: string(eff),
		})
	}
	return out
}
