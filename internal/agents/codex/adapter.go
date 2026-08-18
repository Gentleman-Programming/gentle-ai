package codex

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/capabilitymanifest"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

var LookPathOverride = exec.LookPath

// capabilitiesProbeTimeout bounds the live `codex debug models` invocation
// inside Capabilities. The picker renders inside 2s or it falls back to
// curated — a hung CLI must never block the TUI.
const capabilitiesProbeTimeout = 2 * time.Second

// runCapabilitiesCommand is the package-level hook for the
// `codex debug models` invocation. Tests swap it via
// SetCapabilitiesProbeForTest; production code calls the default below.
var runCapabilitiesCommand = func(ctx context.Context, binaryPath string) ([]byte, error) {
	return exec.CommandContext(ctx, binaryPath, "debug", "models").CombinedOutput()
}

// curatedFallbackModelID is the model identifier the curated fallback
// uses when the runtime catalog is unavailable. sol is the broadest of the
// three Codex rails (low through ultra), so it is the safest default when
// the picker cannot ask the CLI.
const curatedFallbackModelID = "gpt-5.6-sol"

type statResult struct {
	isDir bool
	err   error
}

type Adapter struct {
	lookPath func(string) (string, error)
	statPath func(string) statResult
}

func NewAdapter() *Adapter {
	return &Adapter{
		lookPath: LookPathOverride,
		statPath: defaultStat,
	}
}

// --- Identity ---

func (a *Adapter) Agent() model.AgentID {
	return model.AgentCodex
}

func (a *Adapter) Tier() model.SupportTier {
	return model.TierFull
}

// --- Capability discovery ---

// Capabilities returns the Codex capability record the picker renders
// for the requested model: reasoning/speed/service tiers, the multi-agent
// version stamp, and the provenance of the data. It is a synchronous
// sibling of Tier(): Tier() reports agent support (Full/Partial);
// Capabilities reports what the model itself can do. The two never share
// fields and never replace each other.
//
// Behavior:
//   - Invoke `codex debug models` with a 2s timeout. On success, parse
//     the real envelope `{"models":[{slug, reasoning, speed_tiers,
//     service_tiers, multi_agent_version}, ...]}` and look up the
//     entry whose slug matches modelID. Stamp CapabilitySource = "runtime".
//     If the model is not in the catalog, fall back to the curated row for
//     modelID (or the conservative `unknown` row if modelID is empty).
//   - On any runtime failure (lookup, timeout, non-zero exit, parse error,
//     missing slug) fall back to the curated row for modelID. The picker
//     MUST receive a non-nil record even when the CLI is missing or
//     hanging — discovery never blocks the UI.
//
// The method is intentionally synchronous: no goroutines, no tea.Cmd,
// returns before the picker's first Update(). Callers wire it into the
// picker's initial state.
func (a *Adapter) Capabilities(ctx context.Context, lookup func(string) (string, error), modelID string) (model.CapabilityRecord, error) {
	lookPath := a.lookPath
	if lookup != nil {
		lookPath = lookup
	}

	if lookPath == nil {
		return curatedFallbackForModel(modelID), nil
	}

	binaryPath, err := lookPath("codex")
	if err != nil || binaryPath == "" {
		return curatedFallbackForModel(modelID), nil
	}

	probeCtx, cancel := context.WithTimeout(ctx, capabilitiesProbeTimeout)
	defer cancel()

	output, err := runCapabilitiesCommand(probeCtx, binaryPath)
	if err != nil {
		// Distinguish "codex not on PATH" errors from a hung timeout using
		// context.DeadlineExceeded so future diagnostics can attribute the
		// fallback correctly. We always fall back, regardless.
		if errors.Is(err, context.DeadlineExceeded) {
			return curatedFallbackForModel(modelID), nil
		}
		// Non-zero exit also falls back. The combined output may carry an
		// error message we deliberately ignore — the picker only needs the
		// curated matrix in this branch.
		_ = output
		return curatedFallbackForModel(modelID), nil
	}

	// Empty modelID falls back to the legacy flat parser: the picker
	// handed us an empty ID (pre-picker-bootstrap path), so we accept
	// whatever flat payload the CLI returned. A real model ID requires
	// the wrapping envelope shape; the helper returns an error that the
	// caller maps into the curated fallback.
	if strings.TrimSpace(modelID) == "" {
		rec, parseErr := model.RecordFromRuntime(output)
		if parseErr != nil {
			return curatedFallbackForModel(modelID), nil
		}
		return rec, nil
	}
	rec, parseErr := model.RecordFromRuntimeForModel(output, modelID)
	if parseErr != nil {
		// Model missing from runtime catalog OR runtime payload failed to
		// validate. Either way, the curated row for modelID is the right
		// answer (defaults to the conservative unknown row when modelID
		// has no curated entry).
		return curatedFallbackForModel(modelID), nil
	}
	return rec, nil
}

// curatedFallback returns the curated CapabilityRecord stamped with
// CapabilitySource = "curated" for the historical single-row fallback
// (the broadest of the three Codex rails). It survives the cut-over to
// per-model fallbacks so existing call sites stay compiling.
func curatedFallback() model.CapabilityRecord {
	return model.RecordFromCurated(curatedFallbackModelID)
}

// curatedFallbackForModel returns the curated CapabilityRecord stamped
// with CapabilitySource = "curated" for the requested model id. When
// modelID is empty or unknown to the curated matrix, it returns the
// conservative unknown row (only low/medium/high reasoning, no speed
// or service tiers).
func curatedFallbackForModel(modelID string) model.CapabilityRecord {
	if strings.TrimSpace(modelID) == "" {
		return curatedFallback()
	}
	return model.RecordFromCurated(modelID)
}

// --- Detection ---

func (a *Adapter) Detect(_ context.Context, homeDir string) (bool, string, string, bool, error) {
	configPath := filepath.Join(homeDir, ".codex")

	binaryPath, err := a.lookPath("codex")
	installed := err == nil

	stat := a.statPath(configPath)
	if stat.err != nil {
		if os.IsNotExist(stat.err) {
			return installed, binaryPath, configPath, false, nil
		}
		return false, "", "", false, stat.err
	}

	return installed, binaryPath, configPath, stat.isDir, nil
}

// --- Installation ---

func (a *Adapter) CapabilityManifest() capabilitymanifest.AgentCapabilityManifest {
	return capabilitymanifest.MustForAgent(model.AgentCodex)
}

// InstallCommand returns the display-only command shown when Codex is not
// detected — gentle-ai never executes this (see agentInstallStep in
// internal/cli/run.go). Codex CLI installs via npm on all platforms;
// postinstall scripts are blocked to mitigate supply-chain risk. The version
// advises "latest" rather than a pin: a human reads and runs this, and a
// hardcoded version goes stale the moment a newer Codex ships.
func (a *Adapter) InstallCommand(profile system.PlatformProfile) ([][]string, error) {
	const pkg = "@openai/codex@latest"
	if profile.OS == "linux" && !profile.NpmWritable {
		return [][]string{{"sudo", "npm", "install", "-g", "--ignore-scripts", pkg}}, nil
	}
	return [][]string{{"npm", "install", "-g", "--ignore-scripts", pkg}}, nil
}

// --- Config paths ---

func (a *Adapter) GlobalConfigDir(homeDir string) string {
	return filepath.Join(homeDir, ".codex")
}

func (a *Adapter) SystemPromptDir(homeDir string) string {
	return filepath.Join(homeDir, ".codex")
}

func (a *Adapter) SystemPromptFile(homeDir string) string {
	return filepath.Join(homeDir, ".codex", "AGENTS.md")
}

func (a *Adapter) SkillsDir(homeDir string) string {
	return filepath.Join(homeDir, ".codex", "skills")
}

func (a *Adapter) SettingsPath(_ string) string {
	// Codex has no known settings.json path; permissions component skips nil-overlay agents.
	return ""
}

// --- Config strategies ---

func (a *Adapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyFileReplace
}

func (a *Adapter) MCPStrategy() model.MCPStrategy {
	return model.StrategyTOMLFile
}

// --- MCP ---

// MCPConfigPath returns the path to Codex's TOML config file (~/.codex/config.toml).
// The serverName argument is ignored — Codex uses a single config file for all MCP servers.
func (a *Adapter) MCPConfigPath(homeDir string, _ string) string {
	return filepath.Join(homeDir, ".codex", "config.toml")
}

// --- Optional capabilities ---

func (a *Adapter) SupportsOutputStyles() bool {
	return a.CapabilityManifest().Features.OutputStyles
}

func (a *Adapter) OutputStyleDir(_ string) string {
	return ""
}

func (a *Adapter) SupportsSlashCommands() bool {
	return a.CapabilityManifest().Features.SlashCommands
}

func (a *Adapter) CommandsDir(_ string) string {
	return ""
}

func (a *Adapter) SupportsSubAgents() bool {
	return a.CapabilityManifest().Features.FileSubAgents
}

func (a *Adapter) SubAgentsDir(_ string) string {
	return ""
}

func (a *Adapter) EmbeddedSubAgentsDir() string {
	return ""
}

func (a *Adapter) SupportsSkills() bool {
	return a.CapabilityManifest().Features.Skills
}

func (a *Adapter) SupportsSystemPrompt() bool {
	return a.CapabilityManifest().Features.SystemPrompt
}

// SupportsMCP returns true — Codex supports MCP via ~/.codex/config.toml.
func (a *Adapter) SupportsMCP() bool {
	return a.CapabilityManifest().Features.MCP
}

// RenderCodexPhaseEfforts implements codexModelResolver. It delegates to
// model.RenderCodexPhaseEfforts so that inject.go can substitute the
// {{CODEX_PHASE_EFFORTS}} placeholder in the Codex orchestrator asset.
func (a *Adapter) RenderCodexPhaseEfforts(assignments map[string]model.CodexEffort, carrilModels map[string]string) string {
	return model.RenderCodexPhaseEfforts(assignments, carrilModels)
}

func defaultStat(path string) statResult {
	info, err := os.Stat(path)
	if err != nil {
		return statResult{err: err}
	}

	return statResult{isDir: info.IsDir()}
}
