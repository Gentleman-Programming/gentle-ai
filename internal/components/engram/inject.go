package engram

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/claude"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/codex"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

type InjectionResult struct {
	Changed bool
	Files   []string
	// CodexServiceTier is non-nil only when this injection wrote a new Codex
	// service_tier or retired the managed one; it is the tier Gentle AI owns
	// afterwards ("" = none). Callers record it as the managed tier; nil keeps
	// the previously recorded value.
	CodexServiceTier *string
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
	if bin, ok := CallerBinary(); ok {
		return bin, true
	}
	p, err := EngramLookPath("engram")
	if err != nil || p == "" {
		return "engram", false
	}
	if isVersionedHomebrewCellarPath(p) {
		return "engram", false
	}
	return p, true
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
	// OpenCodeSettingsPath overrides only the OpenCode MCP settings merge target.
	// Empty preserves adapter resolution for other callers and agents.
	OpenCodeSettingsPath string
	// CodexMultiAgent controls whether features.multi_agent is written as true
	// in ~/.codex/config.toml. Default (false) writes multi_agent = false, which
	// is the safe no-op value for the experimental Codex multi-agent tool set.
	// Set to true only when the user explicitly opts in via a CLI flag or TUI choice.
	CodexMultiAgent bool

	// CodexOrchestratorAssignment updates top-level model settings when non-nil.
	// nil preserves the user's existing main-session configuration.
	CodexOrchestratorAssignment *model.CodexOrchestratorAssignment

	// CodexServiceTier writes the top-level service_tier when it is a new,
	// non-empty selection (different from CodexManagedServiceTier).
	CodexServiceTier string
	// CodexManagedServiceTier is the service_tier Gentle AI previously wrote.
	// With no CodexServiceTier selected it is removed only while config.toml
	// still holds exactly that value; any other user value is left untouched.
	CodexManagedServiceTier string

	// CodexCarrilModelAssignments retains saved model choices for ODD workers.
	// Existing legacy carril keys remain readable, but no SDD profiles are written.
	CodexCarrilModelAssignments map[string]string

	// CodexModelAssignments retains saved effort choices for ODD/RDD routing.
	CodexModelAssignments map[string]model.CodexEffort

	// Version carries the raw installed engram binary version string (e.g.
	// "engram 1.18.0"), as returned by VerifyVersion(). It feeds the
	// Decision 1 version-gate: the Claude Code CLAUDE.md section only
	// renders slim when Version parses to >= v1.4.0. Empty, unknown, or
	// unparseable values fall back to the full section (safe default). A
	// raw string (rather than a bool) is required to support the inclusive
	// at-floor v1.4.0 boundary comparison.
	Version string
}

func Inject(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
	return injectWithOptions(homeDir, homeDir, adapter, InjectOptions{}, true)
}

// InjectWithOptions is like Inject but accepts additional options such as the
// Codex multi-agent opt-in flag. Use this when the caller has a model.Selection
// and needs to forward user-chosen configuration into the injection pass.
func InjectWithOptions(homeDir string, adapter agents.Adapter, opts InjectOptions) (InjectionResult, error) {
	return injectWithOptions(homeDir, homeDir, adapter, opts, true)
}

// InjectWorkspaceWithOptions preserves the existing workspace-scoped delivery.
// Claude's user registry migration is intentionally limited to user scope.
func InjectWorkspaceWithOptions(workspaceDir string, adapter agents.Adapter, opts InjectOptions) (InjectionResult, error) {
	return injectWithOptions(workspaceDir, workspaceDir, adapter, opts, false)
}

// InjectWithPromptDir writes Engram's MCP configuration using configHomeDir and
// writes prompt protocol files using promptDir. This is needed for agents such
// as OpenClaw where MCP is loaded from the global config but instructions are
// read from an active workspace.
func InjectWithPromptDir(configHomeDir, promptDir string, adapter agents.Adapter) (InjectionResult, error) {
	return injectWithOptions(configHomeDir, promptDir, adapter, InjectOptions{}, true)
}

const antigravityEngramPluginJSON = `{
  "name": "gentle-ai-engram",
  "description": "Loads Engram MCP memory tools for Antigravity sessions.",
  "version": "0.1.0"
}
`

const antigravityEngramToolsMessage = "CRITICAL FIRST ACTION — Ensure these Engram MCP tools are available before responding: mem_save, mem_search, mem_context, mem_session_summary, mem_session_start, mem_session_end, mem_get_observation, mem_suggest_topic_key, mem_capture_passive, mem_save_prompt, mem_update, mem_current_project, mem_judge. When available, also load/select optional mem_review for memory lifecycle review; if mem_review is unavailable, continue with the required tools above. If Antigravity defers MCP tools, load/select these tools from the engram MCP server first. Loading tools never resolves the project: before any initial mem_context, follow the Engram startup protocol (call mem_current_project and apply its workspace, ambiguous, and no-project checks), then pass only the resolved project to mem_context when the user asks about prior work or the session needs project memory."

// antigravityEngramHookTimeout is the explicit handler timeout in agy's hook
// shape {type, command, timeout}; the hook only prints a static message.
const antigravityEngramHookTimeout = 10

// antigravityEngramHooksJSON registers the Engram guidance as a SessionStart
// hook, so agy injects it once per session instead of before every model call
// (#5895).
func antigravityEngramHooksJSON() []byte {
	return antigravityEngramHooksJSONFor("SessionStart", antigravityEngramHookTimeout)
}

// legacyAntigravityEngramHooksJSON lists hooks.json bytes earlier gentle-ai
// builds wrote: the per-invocation hook and the SessionStart hook without an
// explicit timeout. They stay recognized as managed so migration and
// uninstall can remove them.
func legacyAntigravityEngramHooksJSON() [][]byte {
	return [][]byte{
		antigravityEngramHooksJSONFor("PreInvocation", 0),
		antigravityEngramHooksJSONFor("SessionStart", 0),
	}
}

// antigravityEngramHooksJSONFor renders the hook for event; a zero timeout
// omits the field, reproducing the legacy handler shape.
func antigravityEngramHooksJSONFor(event string, timeout int) []byte {
	handler := map[string]any{
		"type": "command",
		"command": "printf '%s\\n' '" + mustJSONString(map[string]any{
			"injectSteps": []any{
				map[string]any{"ephemeralMessage": antigravityEngramToolsMessage},
			},
		}) + "'",
	}
	if timeout > 0 {
		handler["timeout"] = timeout
	}
	cfg := map[string]any{
		"gentle-ai-engram-tools": map[string]any{
			event: []any{handler},
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

// writeAntigravityFileAtomic is the private writer boundary for the Antigravity
// injection pass (managed global rewrite, settings bootstrap, plugin
// manifest/MCP/hooks). Production default is the real durable writer; tests
// route real bytes through it and then inject an error at one exact write so
// landed-vs-before-replacement accounting is proved against files actually on
// disk.
var writeAntigravityFileAtomic = filemerge.WriteFileAtomic

func ensureJSONFileIfMissing(path string) (filemerge.WriteResult, error) {
	if _, err := os.Stat(path); err == nil {
		return filemerge.WriteResult{Changed: false}, nil
	} else if !os.IsNotExist(err) {
		return filemerge.WriteResult{}, err
	}
	return writeAntigravityFileAtomic(path, []byte("{}\n"), 0o644)
}

// validateAntigravityGlobalMCPConfig prevalidates the shared global Antigravity
// MCP config before any injection writes (#1635). A missing, empty, or
// whitespace-only file is accepted without normalization; anything else must
// be readable and parse as a JSON object. Malformed JSON, unreadable paths,
// and non-object top-level values (including null) are an error here, so
// injection never mutates any file or activates the plugin next to input it
// could not classify.
func validateAntigravityGlobalMCPConfig(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read Antigravity global MCP config %q: %w", path, err)
	}
	if strings.TrimSpace(string(raw)) == "" {
		return nil
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return fmt.Errorf("parse Antigravity global MCP config %q: %w", path, err)
	}
	if root == nil {
		return fmt.Errorf("parse Antigravity global MCP config %q: top-level JSON value is not an object", path)
	}
	return nil
}

func injectAntigravityMutationsUnderLock(configHomeDir, settingsPath, mcpPath string) (bool, []string, error) {
	// Revalidate under the lock: the shared global config must still be
	// classifiable before any mutation, so concurrent rewrites between the
	// pre-lock read and this read are classified, not raced.
	if err := validateAntigravityGlobalMCPConfig(mcpPath); err != nil {
		return false, nil, err
	}
	// The engram command is read from the shared configs; resolve it under
	// the lock so the merged config reflects the latest shared state.
	engramCommand := stableAntigravityEngramCommand(configHomeDir, mcpPath)
	changed := false
	files := make([]string, 0, 4)
	settingsWrite, settingsErr := ensureJSONFileIfMissing(settingsPath)
	if settingsErr != nil {
		// #1635: a landed settings creation is reported even alongside
		// the error; a failure before replacement claims no mutation.
		changed = changed || settingsWrite.Changed
		if settingsWrite.Changed {
			files = append(files, settingsPath)
		}
		return changed, files, fmt.Errorf("ensure Antigravity settings: %w", settingsErr)
	}
	changed = changed || settingsWrite.Changed
	if settingsWrite.Changed {
		files = append(files, settingsPath)
	}

	// #5988: Antigravity reads global MCP configurations natively from
	// ~/.gemini/config/mcp_config.json. Plugin-based MCP loading requires Google
	// cloud authentication and fails with "cannot get the signed-in user" in
	// local/offline/ACP sessions. Merge engram directly into mcpPath alongside
	// existing servers (e.g. context7).
	overlay := engramOverlayJSON(model.AgentAntigravity, engramCommand)
	baseJSON, readErr := osReadFile(mcpPath)
	if readErr != nil {
		return changed, files, readErr
	}
	merged, mergeErr := filemerge.MergeJSONObjectsForPath(mcpPath, baseJSON, overlay)
	if mergeErr != nil {
		return changed, files, mergeErr
	}
	mcpWrite, err := writeAntigravityFileAtomic(mcpPath, merged, filemerge.ExistingFileMode(mcpPath, 0o644))
	changed = changed || mcpWrite.Changed
	if mcpWrite.Changed {
		files = append(files, mcpPath)
	}
	if err != nil {
		return changed, files, fmt.Errorf("merge Antigravity global MCP config %q: %w", mcpPath, err)
	}

	// Retire any managed plugin assets left behind by older versions under either
	// ~/.gemini/config/plugins/gentle-ai-engram or ~/.gemini/antigravity-cli/plugins/gentle-ai-engram.
	retired, retireErr := RetireManagedAntigravityPlugins(configHomeDir)
	changed = changed || len(retired) > 0
	files = append(files, retired...)
	return changed, files, retireErr
}

// injectAntigravityOwnership runs every managed Antigravity mutation under
// the cooperative single-writer coordination lock (#1635 B). The flow is
// side-effect-free prevalidate → acquire → revalidate → mutate: a malformed
// or unreadable global config is refused before the lock root or any other
// path exists, the shared global config is reread under the lock, and the
// lease release error is joined into the operation's error. The lock is
// acquired once per injection pass — never nested or reacquired.
func injectAntigravityOwnership(configHomeDir, settingsPath, mcpPath string) (bool, []string, error) {
	if err := validateAntigravityGlobalMCPConfig(mcpPath); err != nil {
		return false, nil, err
	}
	lease, err := acquireAntigravityCoordinationLock(configHomeDir)
	if err != nil {
		return false, nil, err
	}
	changed, files, mutErr := injectAntigravityMutationsUnderLock(configHomeDir, settingsPath, mcpPath)
	if releaseErr := lease.Release(); releaseErr != nil {
		return changed, files, errors.Join(mutErr, fmt.Errorf("release Antigravity coordination lock: %w", releaseErr))
	}
	return changed, files, mutErr
}

func injectWithOptions(configHomeDir, promptDir string, adapter agents.Adapter, opts InjectOptions, userScope bool) (InjectionResult, error) {
	if adapter.Agent() == model.AgentCodex {
		if err := system.ValidateCodexHome(configHomeDir); err != nil {
			return InjectionResult{}, err
		}
	}
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
	var codexServiceTier *string

	// 1. Write MCP server config using the adapter's strategy.
	switch adapter.MCPStrategy() {
	case model.StrategySeparateMCPFiles:
		if adapter.Agent() == model.AgentClaudeCode && userScope {
			result, err := injectClaudeUserConfig(configHomeDir, adapter)
			// A landed mutation is reported even alongside the error: the managed
			// legacy config is deleted before its empty parent is pruned.
			changed = changed || result.Changed
			files = append(files, result.Files...)
			if err != nil {
				return InjectionResult{Changed: changed, Files: files}, err
			}
			break
		}
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
		if adapter.Agent() == model.AgentOpenCode && opts.OpenCodeSettingsPath != "" {
			settingsPath = opts.OpenCodeSettingsPath
		}
		if settingsPath == "" {
			break
		}
		overlay := engramOverlayJSON(adapter.Agent(), stableEngramCommandForMergedConfig(settingsPath, adapter.Agent()))
		if adapter.Agent() == model.AgentOpenCode {
			var err error
			overlay, err = openCodeEngramOverlay(settingsPath)
			if err != nil {
				return InjectionResult{}, err
			}
		}
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
		if adapter.Agent() == model.AgentAntigravity {
			// #5988: Antigravity registers Engram in global ~/.gemini/config/mcp_config.json
			// natively alongside other MCP servers (e.g. Context7). Any legacy
			// plugin assets left behind by older releases are retired.
			//
			// #1635 B: the mutation sequence runs under the cooperative
			// coordination lock (prevalidate → acquire → revalidate → mutate);
			// the shared protocol prompt below is deliberately outside it.
			owned, ownedFiles, ownedErr := injectAntigravityOwnership(configHomeDir, adapter.SettingsPath(configHomeDir), mcpPath)
			changed = changed || owned
			files = append(files, ownedFiles...)
			if ownedErr != nil {
				return InjectionResult{Changed: changed, Files: files}, ownedErr
			}
			break
		}

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
		runtimeErr := codex.ValidateGPT56Runtime()
		if runtimeErr != nil && !codex.IsGPT56RuntimeUnavailable(runtimeErr) {
			return InjectionResult{}, runtimeErr
		}

		// Determine instruction file paths before mutating the config.
		instructionsPath, compactPath, instructionsChanged, instructionFiles, instrErr := writeCodexInstructionFiles(configHomeDir)
		if instrErr != nil {
			return InjectionResult{}, instrErr
		}
		changed = changed || instructionsChanged
		files = append(files, instructionFiles...)

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

		// Step 1 — multi-agent ODD delegation keys ([features] and [agents]).
		// features.multi_agent is enabled by default for bounded ODD workers. The
		// orchestrator asset gracefully falls back to solo execution if the multi-agent
		// tools are unavailable in the session. agents.max_threads/max_depth carry
		// conservative defaults.
		withFeatures := upsertCodexTableKeyBeforeMCPServers(existing, "features", "multi_agent", "true")
		withMaxThreads := upsertCodexTableKeyBeforeMCPServers(withFeatures, "agents", "max_threads", "4")
		withMaxDepth := upsertCodexTableKeyBeforeMCPServers(withMaxThreads, "agents", "max_depth", "2")

		// Step 2 — top-level instruction-file keys (before the first section header).
		withInstr := filemerge.UpsertTopLevelTOMLString(withMaxDepth, "model_instructions_file", instructionsPath)
		withCompact := filemerge.UpsertTopLevelTOMLString(withInstr, "experimental_compact_prompt_file", compactPath)
		if opts.CodexOrchestratorAssignment != nil {
			withCompact = filemerge.UpsertTopLevelTOMLString(withCompact, "model", opts.CodexOrchestratorAssignment.Model)
			withCompact = filemerge.UpsertTopLevelTOMLString(withCompact, "model_reasoning_effort", string(opts.CodexOrchestratorAssignment.Effort))
		}
		// Invalid tiers are dropped, and an unchanged selection leaves
		// service_tier exactly as the user keeps it.
		desiredTier, managedTier := validServiceTier(opts.CodexServiceTier), validServiceTier(opts.CodexManagedServiceTier)
		var changedTier *string
		if desiredTier != managedTier {
			if desiredTier != "" {
				withCompact = filemerge.UpsertTopLevelTOMLString(withCompact, "service_tier", desiredTier)
			} else if line, ok := rootCodexServiceTierLine(withCompact, managedTier); ok {
				lines := strings.Split(withCompact, "\n")
				withCompact = strings.Join(append(lines[:line], lines[line+1:]...), "\n")
			}
			changedTier = &desiredTier
		}

		// Step 3 — [mcp_servers.engram] block (always last; strip+re-append at EOF).
		engramCmd := stableEngramCommandForMergedConfig(configPath, adapter.Agent())
		withMCP := filemerge.UpsertCodexEngramBlock(withCompact, engramCmd)

		tomlWrite, err := filemerge.WriteFileAtomic(configPath, []byte(withMCP), 0o644)
		if err != nil {
			return InjectionResult{}, err
		}
		changed = changed || tomlWrite.Changed
		files = append(files, configPath)
		codexServiceTier = changedTier

		// Retired SDD-only profile files, including user-customized copies, are
		// deliberately neither created nor modified by Engram injection.
	}

	// 2. Inject Engram memory protocol into system prompt (if supported).
	if adapter.SupportsSystemPrompt() {
		switch adapter.SystemPromptStrategy() {
		case model.StrategyMarkdownSections:
			promptPath := adapter.SystemPromptFile(promptDir)
			protocolContent := protocolFor(adapter.Agent(), opts)

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
			protocolContent := protocolFor(adapter.Agent(), opts)
			modulePath := filepath.Join(configDir, "engram-protocol.md")
			mdWrite, err := filemerge.WriteFileAtomic(modulePath, []byte(protocolContent), 0o644)
			if err != nil {
				return InjectionResult{}, err
			}
			changed = changed || mdWrite.Changed
			files = append(files, modulePath)

		default:
			promptPath := adapter.SystemPromptFile(promptDir)
			protocolContent := protocolFor(adapter.Agent(), opts)

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

	return InjectionResult{Changed: changed, Files: files, CodexServiceTier: codexServiceTier}, nil
}

// upsertCodexTableKeyBeforeMCPServers behaves like filemerge.UpsertTOMLTableKey,
// except that a missing table is created before the first [mcp_servers.*]
// table instead of at EOF. Context7 and Engram both strip and re-append their
// MCP block at EOF, so MCP servers must stay contiguous at the end of the file:
// a table appended after an existing MCP block would be reordered by the next
// Context7 upsert, and install and sync would never produce the same bytes.
func upsertCodexTableKeyBeforeMCPServers(content, section, key, rawValue string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	firstMCP := -1
	// Table headers are only recognized outside TOML multiline strings, so a
	// developer_instructions value that mentions "[mcp_servers.x]" is text.
	var multiline byte
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		inString := multiline != 0
		multiline = filemerge.ScanTOMLMultilineString(line, multiline)
		if inString {
			continue
		}
		if trimmed == "["+section+"]" {
			return filemerge.UpsertTOMLTableKey(content, section, key, rawValue)
		}
		if firstMCP < 0 && strings.HasPrefix(trimmed, "[mcp_servers.") {
			firstMCP = i
		}
	}
	if firstMCP < 0 {
		return filemerge.UpsertTOMLTableKey(content, section, key, rawValue)
	}
	head := filemerge.UpsertTOMLTableKey(strings.Join(lines[:firstMCP], "\n"), section, key, rawValue)
	return head + "\n" + strings.Join(lines[firstMCP:], "\n")
}

func injectClaudeUserConfig(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
	// The plugin and direct MCP entry expose the same Engram tools. When the
	// plugin is enabled, suppress direct registration without deleting any
	// existing entry: matching config shape is not proof that gentle-ai owns it.
	if claudeEngramPluginEnabled(homeDir) {
		return InjectionResult{}, nil
	}

	legacyPath := adapter.MCPConfigPath(homeDir, "engram")
	command := stableEngramCommandForMergedConfig(claude.UserConfigPath(homeDir), model.AgentClaudeCode)
	legacyManaged := false
	if raw, err := os.ReadFile(legacyPath); err == nil {
		if legacyCommand, ok := managedLegacyClaudeEngramCommand(raw); ok {
			command = stableEngramCommandForExisting(legacyCommand, model.AgentClaudeCode)
			legacyManaged = true
		}
	} else if !os.IsNotExist(err) {
		return InjectionResult{}, fmt.Errorf("read legacy Claude Engram config %q: %w", legacyPath, err)
	}

	writeResult, registryPath, err := claude.MergeUserConfig(homeDir, engramOverlayJSON(model.AgentClaudeCode, command))
	if err != nil {
		return InjectionResult{}, err
	}
	result := InjectionResult{Changed: writeResult.Changed, Files: []string{registryPath}}
	if !legacyManaged {
		return result, nil
	}
	removed, err := RemoveManagedLegacyClaudeConfig(legacyPath)
	if err != nil {
		// The managed legacy file is deleted before its empty parent is pruned,
		// so a prune failure still lands a mutation. Report the removed path
		// alongside the error instead of claiming nothing changed.
		if removed {
			result.Changed = true
			result.Files = append(result.Files, legacyPath)
		}
		return result, err
	}
	if !removed {
		return result, nil
	}
	result.Changed = true
	result.Files = append(result.Files, legacyPath)
	return result, nil
}

func claudeEngramPluginEnabled(homeDir string) bool {
	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		return false
	}

	var settings struct {
		EnabledPlugins map[string]bool `json:"enabledPlugins"`
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		return false
	}
	return settings.EnabledPlugins["engram@engram"]
}

func validateOpenClawWorkspacePath(workspaceDir string, adapter agents.Adapter) error {
	if adapter.Agent() == model.AgentOpenClaw && strings.TrimSpace(workspaceDir) == "" {
		return fmt.Errorf("openclaw workspace path is required for workspace-first injection")
	}
	return nil
}

// writeCodexInstructionFiles writes the Engram memory protocol and compact prompt
// files to the resolved Codex home (honoring CODEX_HOME when the supplied home
// is the real user home — issue #5031) and returns their paths and write results.
func writeCodexInstructionFiles(homeDir string) (instructionsPath, compactPath string, changed bool, files []string, err error) {
	codexDir := system.CodexConfigDir(homeDir)
	instructionsPath = filepath.Join(codexDir, "engram-instructions.md")
	compactPath = filepath.Join(codexDir, "engram-compact-prompt.md")

	instrContent := codexInstructions()
	instrWrite, err := filemerge.WriteFileAtomic(instructionsPath, []byte(instrContent), 0o644)
	if err != nil {
		return "", "", false, nil, fmt.Errorf("write codex engram-instructions.md: %w", err)
	}
	changed = instrWrite.Changed
	files = append(files, instructionsPath)

	compactContent := codexCompact()
	compactWrite, err := filemerge.WriteFileAtomic(compactPath, []byte(compactContent), 0o644)
	if err != nil {
		return "", "", false, nil, fmt.Errorf("write codex engram-compact-prompt.md: %w", err)
	}
	changed = changed || compactWrite.Changed
	files = append(files, compactPath)

	return instructionsPath, compactPath, changed, files, nil
}

func mergeJSONFile(path string, overlay []byte) (filemerge.WriteResult, error) {
	baseJSON, err := osReadFile(path)
	if err != nil {
		return filemerge.WriteResult{}, err
	}

	merged, err := filemerge.MergeJSONObjectsForPath(path, baseJSON, overlay)
	if err != nil {
		return filemerge.WriteResult{}, err
	}

	return filemerge.WriteFileAtomic(path, merged, filemerge.ExistingFileMode(path, 0o644))
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

func stableEngramCommandForExisting(cmd string, _ model.AgentID) string {
	if isVersionedHomebrewCellarPath(cmd) {
		if stable := preferredStableEngramCommand(); stable != "" {
			return stable
		}
		return "engram"
	}

	return cmd
}

func stableAntigravityEngramCommand(homeDir, globalPath string) string {
	paths := []string{
		globalPath,
		filepath.Join(AntigravityPluginDir(homeDir), "mcp_config.json"),
		filepath.Join(AntigravityLegacyPluginDir(homeDir), "mcp_config.json"),
	}
	for _, path := range paths {
		raw, err := osReadFile(path)
		if err != nil {
			continue
		}
		// #5930: a preserved command that no longer resolves leaves agy with a
		// disconnected Engram, so it falls through to the fresh-install command.
		if cmd, ok := existingMergedEngramCommand(raw, model.AgentAntigravity); ok && isEngramCommand(cmd) && engramCommandResolves(cmd) {
			return stableEngramCommandForExisting(cmd, model.AgentAntigravity)
		}
	}
	return preferredStableEngramCommand()
}

// engramCommandResolves reports whether cmd can still be launched: a bare name
// must be on PATH, and a path must name an existing executable file. It is a
// variable so ownership-transfer tests can keep their fixed fixture commands.
var engramCommandResolves = func(cmd string) bool {
	if filepath.Base(cmd) == cmd {
		p, err := EngramLookPath(cmd)
		return err == nil && p != ""
	}
	info, err := os.Stat(cmd)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return runtime.GOOS == "windows" || info.Mode().Perm()&0o111 != 0
}

func preferredStableEngramCommand() string {
	if bin, ok := CallerBinary(); ok {
		return bin
	}
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
		server, _ = opencode.MCPEntry(root, "engram")
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

func managedLegacyClaudeEngramCommand(content []byte) (string, bool) {
	var server map[string]any
	if err := json.Unmarshal(content, &server); err != nil || len(server) != 2 {
		return "", false
	}
	command, ok := server["command"].(string)
	if !ok || !isEngramCommand(command) {
		return "", false
	}
	args, ok := server["args"].([]any)
	if !ok || len(args) != 2 || args[0] != "mcp" || args[1] != "--tools=agent" {
		return "", false
	}
	return command, true
}

// IsManagedLegacyClaudeConfig reports whether content has the exact legacy
// standalone Engram server shape emitted by Gentle AI.
func IsManagedLegacyClaudeConfig(content []byte) bool {
	_, ok := managedLegacyClaudeEngramCommand(content)
	return ok
}

// RemoveManagedLegacyClaudeConfig removes only the exact standalone shape
// emitted by Gentle AI. Its parent is removed only when the same real directory
// remains empty after that managed file is deleted; symlinks are never unlinked.
func RemoveManagedLegacyClaudeConfig(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect managed legacy Claude Engram config %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return false, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read managed legacy Claude Engram config %q: %w", path, err)
	}
	if !IsManagedLegacyClaudeConfig(content) {
		return false, nil
	}
	if err := os.Remove(path); err != nil {
		return false, fmt.Errorf("remove managed legacy Claude Engram config %q: %w", path, err)
	}
	if err := removeManagedLegacyClaudeParent(filepath.Dir(path)); err != nil {
		return true, err
	}
	return true, nil
}

func removeManagedLegacyClaudeParent(parent string) error {
	info, err := os.Lstat(parent)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect managed legacy Claude directory %q: %w", parent, err)
	}
	if !info.IsDir() {
		return nil
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		return fmt.Errorf("read managed legacy Claude directory %q: %w", parent, err)
	}
	if len(entries) != 0 {
		return nil
	}
	current, err := os.Lstat(parent)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reinspect managed legacy Claude directory %q: %w", parent, err)
	}
	if !current.IsDir() || !os.SameFile(info, current) {
		return nil
	}
	if err := os.Remove(parent); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove empty managed legacy Claude directory %q: %w", parent, err)
	}
	return nil
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

func isVersionedHomebrewCellarPath(path string) bool {
	clean := filepath.ToSlash(filepath.Clean(path))
	return strings.Contains(clean, "/Cellar/engram/") && isEngramCommand(clean)
}

func isStableHomebrewEngramPath(path string) bool {
	clean := filepath.ToSlash(filepath.Clean(path))
	switch clean {
	case "/opt/homebrew/bin/engram", "/usr/local/bin/engram", "/home/linuxbrew/.linuxbrew/bin/engram":
		return isEngramCommand(clean)
	default:
		return false
	}
}

// ValidateOpenCodeSettings applies the refusals the OpenCode Engram merge
// applies to settingsPath (locked, non-regular or symlinked file, malformed
// JSONC, duplicate or escaped keys, comments inside the touched value) without
// writing, so install can refuse unsafe input before the external
// `engram setup opencode` side effects. An empty path is accepted.
func ValidateOpenCodeSettings(settingsPath string) error {
	if settingsPath == "" {
		return nil
	}
	overlay, err := openCodeEngramOverlay(settingsPath)
	if err != nil {
		return err
	}
	base, err := osReadFile(settingsPath)
	if err != nil {
		return err
	}
	_, err = filemerge.MergeJSONObjectsForPath(settingsPath, base, overlay)
	return err
}

// openCodeEngramOverlay refuses a locked selected settings file, then returns
// the Engram overlay in the format the file already uses.
func openCodeEngramOverlay(settingsPath string) ([]byte, error) {
	if err := filemerge.RefuseLockedSettingsFile(settingsPath); err != nil {
		return nil, err
	}
	overlay := engramOverlayJSON(model.AgentOpenCode, stableEngramCommandForMergedConfig(settingsPath, model.AgentOpenCode))
	return nativeOpenCodeEngramOverlay(settingsPath, overlay)
}

// nativeOpenCodeEngramOverlay updates only the managed server in its existing
// format, preserving native disabled, environment, and timeout settings.
func nativeOpenCodeEngramOverlay(path string, overlay []byte) ([]byte, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return overlay, nil
	}
	if err != nil {
		return nil, err
	}
	root, err := filemerge.UnmarshalJSONObject(data)
	if err != nil {
		return nil, err
	}
	mcp, _ := root["mcp"].(map[string]any)
	if _, native := mcp["servers"]; !native && !opencode.NativeConfig(root) {
		return overlay, nil
	}
	patch, err := filemerge.UnmarshalJSONObject(overlay)
	if err != nil {
		return nil, err
	}
	patchMCP, _ := patch["mcp"].(map[string]any)
	server, _ := patchMCP["engram"].(map[string]any)
	if replacement, ok := server["__replace__"].(map[string]any); ok {
		server = replacement
	}
	patch["mcp"] = map[string]any{"servers": map[string]any{"engram": server}}
	return json.Marshal(patch)
}

func validServiceTier(tier string) string {
	if !model.ValidCodexServiceTier(tier) {
		return ""
	}
	return tier
}

// rootCodexServiceTierLine finds the root-scope line that is byte-for-byte the
// service_tier assignment Gentle AI writes for tier. Lines inside multiline
// strings are content, not assignments, and never match.
func rootCodexServiceTierLine(content, tier string) (int, bool) {
	if tier == "" {
		return 0, false
	}
	want := fmt.Sprintf("service_tier = %q", tier)
	var multilineQuote byte
	for i, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if multilineQuote == 0 {
			if strings.HasPrefix(trimmed, "[") {
				return 0, false
			}
			if trimmed == want {
				return i, true
			}
		}
		multilineQuote = filemerge.ScanTOMLMultilineString(line, multilineQuote)
	}
	return 0, false
}
