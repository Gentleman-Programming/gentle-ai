// Package kimi provides Kimi Code CLI agent integration.
//
// Integration Note:
// This adapter supports both the legacy Python/uv-based Kimi CLI (~/.kimi)
// and the Node.js-based kimi-code (~/.kimi-code). Path resolution
// prefers ~/.kimi-code when present, falling back to ~/.kimi for backward
// compatibility.
//
// Legacy install: uv tool install kimi-cli
// kimi-code install: npm install -g kimi-code (or official installer)
package kimi

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/capabilitymanifest"
	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v2/internal/installcmd"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

var LookPathOverride = exec.LookPath

type statResult struct {
	isDir bool
	err   error
}

// Adapter implements agents.Adapter for Kimi Code CLI.
type Adapter struct {
	lookPath    func(string) (string, error)
	statPath    func(string) statResult
	pathExists  func(string) bool
	userHomeDir func() (string, error)
	resolver    installcmd.Resolver
}

// NewAdapter creates a new Kimi adapter instance.
func NewAdapter() *Adapter {
	return &Adapter{
		lookPath:    LookPathOverride,
		statPath:    defaultStat,
		pathExists:  defaultPathExists,
		userHomeDir: os.UserHomeDir,
		resolver:    installcmd.NewResolver(),
	}
}

// --- Identity ---

func (a *Adapter) Agent() model.AgentID {
	return model.AgentKimi
}

func (a *Adapter) Tier() model.SupportTier {
	return model.TierFull
}

// --- Detection ---

func (a *Adapter) Detect(_ context.Context, homeDir string) (bool, string, string, bool, error) {
	configPath := ConfigPath(homeDir)

	binaryPath, err := a.findKimi()
	installed := err == nil && binaryPath != ""

	stat := a.statPath(configPath)
	if stat.err != nil {
		if os.IsNotExist(stat.err) {
			return installed, binaryPath, configPath, false, nil
		}
		return false, "", "", false, stat.err
	}

	return installed, binaryPath, configPath, stat.isDir, nil
}

// findKimi searches for kimi in PATH and official fallback locations.
func (a *Adapter) findKimi() (string, error) {
	if path, err := a.lookPath("kimi"); err == nil {
		return path, nil
	}

	home, err := a.userHomeDir()
	if err != nil || home == "" {
		return "", fmt.Errorf("kimi not found in PATH and home directory is unavailable")
	}

	fallbacks := []string{
		filepath.Join(home, ".local", "bin", binaryName()),
		filepath.Join(home, "bin", binaryName()),
	}
	if runtime.GOOS == "windows" {
		fallbacks = append(fallbacks,
			filepath.Join(home, "AppData", "Local", "Microsoft", "WinGet", "Links", "kimi.exe"),
			filepath.Join(home, "AppData", "Roaming", "uv", "bin", "kimi.exe"),
		)
	}

	for _, fb := range fallbacks {
		if a.pathExists(fb) {
			return fb, nil
		}
	}

	return "", fmt.Errorf("kimi not found in PATH or official install locations")
}

// --- Installation ---

func (a *Adapter) CapabilityManifest() capabilitymanifest.AgentCapabilityManifest {
	return capabilitymanifest.MustForAgent(model.AgentKimi)
}

func (a *Adapter) InstallCommand(profile system.PlatformProfile) ([][]string, error) {
	resolver := a.resolver
	if resolver == nil {
		resolver = installcmd.NewResolver()
	}
	return resolver.ResolveAgentInstall(profile, a.Agent())
}

// --- Config paths ---

// resolveConfigDir returns the configuration directory for Kimi.
// It checks KIMI_CODE_HOME env var first, then prefers ~/.kimi-code (current kimi-code)
// when present, falling back to ~/.kimi (legacy).
func (a *Adapter) resolveConfigDir(homeDir string) string {
	if envDir := os.Getenv("KIMI_CODE_HOME"); envDir != "" {
		if info, err := os.Stat(envDir); err == nil && info.IsDir() {
			return envDir
		}
	}
	kimiCodeDir := filepath.Join(homeDir, ".kimi-code")
	if stat := a.statPath(kimiCodeDir); stat.err == nil && stat.isDir {
		return kimiCodeDir
	}
	return filepath.Join(homeDir, ".kimi")
}

// usesKimiCodeLayout reports whether the current Node.js-based kimi-code layout is installed.
// It checks KIMI_CODE_HOME first, then falls back to ~/.kimi-code detection.
func (a *Adapter) usesKimiCodeLayout(homeDir string) bool {
	if envDir := os.Getenv("KIMI_CODE_HOME"); envDir != "" {
		if st := a.statPath(envDir); st.err == nil && st.isDir {
			return true
		}
	}
	st := a.statPath(filepath.Join(homeDir, ".kimi-code"))
	return st.err == nil && st.isDir
}

func (a *Adapter) GlobalConfigDir(homeDir string) string {
	return a.resolveConfigDir(homeDir)
}

func (a *Adapter) SystemPromptDir(homeDir string) string {
	return a.resolveConfigDir(homeDir)
}

func (a *Adapter) SystemPromptFile(homeDir string) string {
	return filepath.Join(a.resolveConfigDir(homeDir), "AGENTS.md")
}

// SkillsDir returns the skills directory path.
//
// For current kimi-code (Node.js) with plugin support, it returns the plugin skills subdirectory
// under the resolved Kimi config directory:
//
//	{resolvedConfigDir}/plugins/managed/gentle-ai/skills
//
// For legacy (Python/uv), it returns the cross-agent shared convention:
//
//	~/.config/agents/skills
//
// Kimi Code CLI discovers skills from multiple sources including:
//   - {resolvedConfigDir}/plugins/managed/gentle-ai/skills (kimi-code plugin)
//   - {resolvedConfigDir}/skills (kimi-code user skills)
//   - ~/.config/agents/skills (generic shared skills)
//   - ~/.agents/skills (generic shared skills)
//
// See: https://moonshotai.github.io/kimi-cli/en/customization/skills.html
func (a *Adapter) SkillsDir(homeDir string) string {
	if a.usesKimiCodeLayout(homeDir) {
		return filepath.Join(a.PluginDir(homeDir), "skills")
	}
	return filepath.Join(homeDir, ".config", "agents", "skills")
}

func (a *Adapter) SettingsPath(homeDir string) string {
	return filepath.Join(a.resolveConfigDir(homeDir), "config.toml")
}

func (a *Adapter) CommandsDir(string) string {
	return ""
}

// AllSkillsDirs returns all directories Kimi Code discovers for skills.
// For current kimi-code: plugin skills, {resolvedConfigDir}/skills, ~/.agents/skills, ~/.config/agents/skills
// For legacy: ~/.config/agents/skills only.
func (a *Adapter) AllSkillsDirs(homeDir string) []string {
	if a.usesKimiCodeLayout(homeDir) {
		return []string{
			filepath.Join(a.PluginDir(homeDir), "skills"),
			filepath.Join(a.resolveConfigDir(homeDir), "skills"),
			filepath.Join(homeDir, ".agents", "skills"),
			filepath.Join(homeDir, ".config", "agents", "skills"),
		}
	}
	return []string{filepath.Join(homeDir, ".config", "agents", "skills")}
}

// AGENTSMDPath returns the path to the Kimi-level AGENTS.md file.
func (a *Adapter) AGENTSMDPath(homeDir string) string {
	return filepath.Join(a.resolveConfigDir(homeDir), "AGENTS.md")
}

// --- Config strategies ---

func (a *Adapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyJinjaModules
}

func (a *Adapter) MCPStrategy() model.MCPStrategy {
	return model.StrategyMCPConfigFile
}

// --- MCP ---

func (a *Adapter) MCPConfigPath(homeDir string, _ string) string {
	return filepath.Join(a.resolveConfigDir(homeDir), "mcp.json")
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

func (a *Adapter) SupportsSkills() bool {
	return a.CapabilityManifest().Features.Skills
}

func (a *Adapter) SupportsSystemPrompt() bool {
	return a.CapabilityManifest().Features.SystemPrompt
}

func (a *Adapter) SupportsMCP() bool {
	return a.CapabilityManifest().Features.MCP
}

// --- Sub-agent support (optional interface) ---
//
// Kimi uses YAML-based agent specs with separate .md system prompts.
// The SDD component copies all files from the embedded agents directory.

func (a *Adapter) SupportsSubAgents() bool {
	return a.CapabilityManifest().Features.FileSubAgents
}

func (a *Adapter) SubAgentsDir(homeDir string) string {
	return filepath.Join(a.resolveConfigDir(homeDir), "agents")
}

func (a *Adapter) EmbeddedSubAgentsDir() string {
	return "kimi/agents"
}

func (a *Adapter) PostInstallMessage(homeDir string) string {
	configDir := a.resolveConfigDir(homeDir)
	skillsRoot := a.SkillsDir(homeDir)

	if a.usesKimiCodeLayout(homeDir) {
		return fmt.Sprintf(`Kimi Code configured!

Usage:
  kimi

Native SDD skills:
  /skill:sdd-init
  /skill:sdd-explore
  /skill:sdd-propose
  /skill:sdd-spec
  /skill:sdd-design
  /skill:sdd-tasks
  /skill:sdd-apply
  /skill:sdd-verify
  /skill:sdd-archive
  /skill:sdd-onboard

Skills root:
  "%s"`, skillsRoot)
	}

	gentlemanYaml := filepath.Join(configDir, "agents", "gentleman.yaml")

	return fmt.Sprintf(`Kimi Code configured!

Usage:
  kimi --agent-file "%s"

Native SDD entrypoints:
  /skill:sdd-init
  /skill:sdd-explore
  /skill:sdd-research
  /skill:sdd-propose
  /skill:sdd-spec
  /skill:sdd-design
  /skill:sdd-tasks
  /skill:sdd-apply
  /skill:sdd-verify
  /skill:sdd-archive
  /skill:sdd-onboard

Skills root:
  "%s"`, gentlemanYaml, skillsRoot)
}

// --- Helpers ---

func defaultStat(path string) statResult {
	info, err := os.Stat(path)
	if err != nil {
		return statResult{err: err}
	}
	return statResult{isDir: info.IsDir()}
}

func defaultPathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ConfigPath returns the configuration directory path.
// It checks KIMI_CODE_HOME env var first, then prefers ~/.kimi-code (current kimi-code)
// when present, falling back to ~/.kimi (legacy).
func ConfigPath(homeDir string) string {
	if envDir := os.Getenv("KIMI_CODE_HOME"); envDir != "" {
		if info, err := os.Stat(envDir); err == nil && info.IsDir() {
			return envDir
		}
	}
	kimiCodeDir := filepath.Join(homeDir, ".kimi-code")
	if info, err := os.Stat(kimiCodeDir); err == nil && info.IsDir() {
		return kimiCodeDir
	}
	return filepath.Join(homeDir, ".kimi")
}

func binaryName() string {
	if runtime.GOOS == "windows" {
		return "kimi.exe"
	}
	return "kimi"
}

// BootstrapTemplate ensures the base AGENTS.md template exists in the agent's config directory.
// It is used by the installation pipeline to guarantee that modular components
// (SDD, Engram) can be included even if the Persona component is not installed.
func (a *Adapter) BootstrapTemplate(homeDir string) error {
	kimiDir := a.GlobalConfigDir(homeDir)
	if err := os.MkdirAll(kimiDir, 0o755); err != nil {
		return fmt.Errorf("create kimi config dir: %w", err)
	}

	skeletonPath := a.SystemPromptFile(homeDir)

	// We always write the skeleton to ensure any missing includes are restored.
	// Since AGENTS.md is the 'router' for modular Jinja components, it should
	// remain managed by the framework.
	content := assets.MustRead("kimi/AGENTS.md")

	// Project instructions remain project-scoped. We do NOT copy cwd-derived
	// AGENTS.md into the global Kimi config because that crosses a provider
	// boundary and would let untrusted repos persist instructions globally.
	content = strings.ReplaceAll(content, "${KIMI_AGENTS_MD}",
		"<!-- Project AGENTS.md is read from the current worktree at runtime; it is not copied into the global Kimi config. -->")

	// Resolve skills content (placeholder for now).
	content = strings.ReplaceAll(content, "${KIMI_SKILLS}", "<!-- Skills loaded from skill directories -->")

	if _, err := filemerge.WriteFileAtomic(skeletonPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write AGENTS.md skeleton: %w", err)
	}

	// Kimi considers config.toml a required file. We create one with sensible
	// defaults if it's missing to satisfy verification during a minimalist install.
	// For current kimi-code, we also append hooks and extra_skill_dirs.
	configPath := a.SettingsPath(homeDir)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		content := resolveConfigTOMLContent()
		if a.usesKimiCodeLayout(homeDir) {
			content += configTOMLKimiCodeExtras()
		}
		if _, err := filemerge.WriteFileAtomic(configPath, []byte(content), 0o644); err != nil {
			return err
		}
	} else if a.usesKimiCodeLayout(homeDir) {
		if _, err := mergeConfigTOMLKimiCodeExtras(configPath); err != nil {
			return fmt.Errorf("merge kimi-code config extras: %w", err)
		}
	}

	// Install the Kimi plugin for current kimi-code. This is best-effort: if plugin
	// directory creation fails (e.g. permissions), log a warning and continue
	// — the core config files are already written.
	if a.usesKimiCodeLayout(homeDir) {
		if err := a.InstallPlugin(homeDir, versions.GentleAI); err != nil {
			fmt.Fprintf(os.Stderr, "warning: plugin install failed (non-fatal): %v\n", err)
		}
	}

	return nil
}

// resolveConfigTOMLContent returns the default TOML config for Kimi Code.
// It enables skill merging and sets permission rules: auto-approve safe tools,
// require manual approval for Bash.
func resolveConfigTOMLContent() string {
	return `default_permission_mode = "manual"
merge_all_available_skills = true

[[permission.rules]]
decision = "allow"
pattern = "Read"

[[permission.rules]]
decision = "allow"
pattern = "Grep"

[[permission.rules]]
decision = "allow"
pattern = "Glob"

[[permission.rules]]
decision = "allow"
pattern = "Write"

[[permission.rules]]
decision = "allow"
pattern = "Edit"

[[permission.rules]]
decision = "allow"
pattern = "Agent"

[[permission.rules]]
decision = "ask"
pattern = "Bash"
`
}

// configTOMLKimiCodeExtras returns the additional TOML content appended to config.toml
// for current Kimi Code installations. It includes lifecycle hooks and cross-tool
// skill directory discovery.
func configTOMLKimiCodeExtras() string {
	return `
# --- Gentle AI kimi-code extras ---

[[hooks]]
event = "SessionStart"
command = 'gentle-ai skill-registry refresh --quiet --no-gitignore --cwd "$PWD" || true'

extra_skill_dirs = ["~/.config/agents/skills", "~/.agents/skills"]
`
}

// kimiCodeExtraSkillDirs are the additional skill directories Gentle AI registers in
// current Kimi Code so that user-scope and extra-scope skills are discoverable.
var kimiCodeExtraSkillDirs = []string{"~/.config/agents/skills", "~/.agents/skills"}

// mergeConfigTOMLKimiCodeExtras updates an existing config.toml with the kimi-code
// extra_skill_dirs setting. It preserves user settings and only adds the
// Gentle AI directories when they are missing.
func mergeConfigTOMLKimiCodeExtras(configPath string) (filemerge.WriteResult, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return filemerge.WriteResult{}, fmt.Errorf("read config %q: %w", configPath, err)
	}
	text := string(data)

	start, end := findTOMLStringArrayBounds(text, "extra_skill_dirs")
	if start == -1 {
		block := "\n# --- Gentle AI kimi-code extras ---\nextra_skill_dirs = " + formatStringArray(kimiCodeExtraSkillDirs)
		if !strings.HasSuffix(text, "\n") {
			block = "\n" + block
		}
		text += block
		return filemerge.WriteFileAtomic(configPath, []byte(text), 0o644)
	}

	existing := parseTOMLStringArrayValues(text[start : end+1])
	merged := mergeStringSlices(existing, kimiCodeExtraSkillDirs)
	if stringSlicesEqual(existing, merged) {
		return filemerge.WriteResult{}, nil
	}

	lineStart := strings.LastIndex(text[:start], "\n") + 1
	nextNL := strings.Index(text[start:], "\n")
	lineEnd := start + len(text[start:])
	if nextNL != -1 {
		lineEnd = start + nextNL
	}
	newLine := "extra_skill_dirs = " + formatStringArray(merged)
	text = text[:lineStart] + newLine + text[lineEnd:]
	return filemerge.WriteFileAtomic(configPath, []byte(text), 0o644)
}

// findTOMLStringArrayBounds locates the start (index of '[') and end (index of
// ']') of the string array associated with key. It tolerates inline comments,
// quoted keys and both single/double quoted strings. Returns -1, -1 when not
// found or unbalanced.
func findTOMLStringArrayBounds(text, key string) (int, int) {
	keyIdx := strings.Index(text, key)
	if keyIdx == -1 {
		return -1, -1
	}
	eqIdx := strings.Index(text[keyIdx:], "=")
	if eqIdx == -1 {
		return -1, -1
	}
	startIdx := keyIdx + eqIdx
	openIdx := strings.Index(text[startIdx:], "[")
	if openIdx == -1 {
		return -1, -1
	}
	i := startIdx + openIdx
	depth := 1
	var inString bool
	var quote byte
	var prev byte
	for i+1 < len(text) {
		i++
		c := text[i]
		if inString {
			if c == quote && prev != '\\' {
				inString = false
			}
		} else {
			switch c {
			case '"', '\'':
				inString = true
				quote = c
			case '[':
				depth++
			case ']':
				depth--
				if depth == 0 {
					return startIdx + openIdx, i
				}
			}
		}
		prev = c
	}
	return -1, -1
}

// parseTOMLStringArrayValues extracts the quoted string values inside a TOML
// array literal such as `["a", 'b']`.
func parseTOMLStringArrayValues(arrayText string) []string {
	var values []string
	var inString bool
	var quote byte
	var buf strings.Builder
	var prev byte
	for i := 0; i < len(arrayText); i++ {
		c := arrayText[i]
		if inString {
			if c == quote && prev != '\\' {
				values = append(values, buf.String())
				buf.Reset()
				inString = false
			} else {
				buf.WriteByte(c)
			}
		} else {
			if c == '"' || c == '\'' {
				inString = true
				quote = c
			}
		}
		prev = c
	}
	return values
}

// formatStringArray returns a compact TOML array literal with double-quoted
// strings, e.g. `["a", "b"]`.
func formatStringArray(values []string) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = fmt.Sprintf("%q", v)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// mergeStringSlices returns a new slice containing all elements of a followed by
// any elements of b not already present, preserving order.
func mergeStringSlices(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, v := range a {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	for _, v := range b {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// stringSlicesEqual reports whether two string slices have the same elements in
// the same order.
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
