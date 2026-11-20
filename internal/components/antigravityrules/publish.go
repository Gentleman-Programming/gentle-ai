package antigravityrules

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/antigravity"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
)

// ownedRulePattern is the only name shape Publish writes, so stale-file
// cleanup never touches user rules outside the gentle-ai-NN- namespace.
var ownedRulePattern = regexp.MustCompile(`^gentle-ai-\d{2}-[A-Za-z0-9_.-]+\.md$`)

// Result reports the files Publish or MigrateLegacyPrompt changed or removed.
type Result struct {
	Changed bool
	Files   []string
}

// StagingPath is the gentle-ai-owned file the prompt writers append to.
func StagingPath(baseDir string) string {
	return antigravity.NewAdapter().SystemPromptFile(baseDir)
}

// RulesDir is agy's shared global rules directory.
func RulesDir(baseDir string) string {
	return filepath.Join(baseDir, ".gemini", "config", "rules")
}

// ConfigAgentsMDPath is the unified instructions file in Antigravity's shared
// config directory, providing a complete ruleset for external harnesses and ACP.
func ConfigAgentsMDPath(baseDir string) string {
	return filepath.Join(baseDir, ".gemini", "config", "AGENTS.md")
}

// AgentsMDPath is the standard project / workspace instructions file loaded by
// Antigravity (and other agents) in workspaces and parent directories.
func AgentsMDPath(baseDir string) string {
	return filepath.Join(baseDir, "AGENTS.md")
}

// LegacyPromptPath is the Gemini-compatible prompt older releases appended
// Antigravity sections to.
func LegacyPromptPath(baseDir string) string {
	return filepath.Join(baseDir, ".gemini", "GEMINI.md")
}

// Option configures Publish behavior.
type Option func(*publishOptions)

type publishOptions struct {
	isWorkspace  *bool
	activeAgents []model.AgentID
}

// WithWorkspaceScope explicitly specifies whether Publish is targeting a workspace.
// When true, baseDir/AGENTS.md is created as a regular file even if baseDir has no .git directory.
func WithWorkspaceScope(isWorkspace bool) Option {
	return func(o *publishOptions) {
		o.isWorkspace = &isWorkspace
	}
}

// WithActiveAgents supplies the active or retained agents for section preservation checks.
func WithActiveAgents(agents []model.AgentID) Option {
	return func(o *publishOptions) {
		o.activeAgents = agents
	}
}

// managedSectionIDs is the complete set of section IDs Gentle AI injects into markdown prompt files.
var managedSectionIDs = map[string]bool{
	"persona":            true,
	"gemini-cli-imports": true,
	"engram-protocol":    true,
	"agent-routing":      true,
	"strict-tdd-mode":    true,
	"orchestrator":       true,
	"sdd-orchestrator":   true,
}

// OpenClawManaged reports whether Gentle AI manages OpenClaw, which also loads
// and writes AGENTS.md in homeDir or workspaceDir.
func OpenClawManaged(baseDir string, activeAgents []model.AgentID) bool {
	if slices.Contains(activeAgents, model.AgentOpenClaw) {
		return true
	}
	if persisted, err := state.Read(baseDir); err == nil && slices.Contains(persisted.InstalledAgents, string(model.AgentOpenClaw)) {
		return true
	}
	if home, err := os.UserHomeDir(); err == nil && filepath.Clean(home) != filepath.Clean(baseDir) {
		if persisted, err := state.Read(home); err == nil && slices.Contains(persisted.InstalledAgents, string(model.AgentOpenClaw)) {
			return true
		}
	}
	return false
}

// otherAgentNeedsSection reports whether an agent other than Antigravity (such as OpenClaw)
// still requires this section in AGENTS.md.
func otherAgentNeedsSection(baseDir string, sectionID string, activeAgents []model.AgentID) bool {
	if OpenClawManaged(baseDir, activeAgents) {
		switch sectionID {
		case "engram-protocol", "agent-routing", "strict-tdd-mode", "orchestrator", "sdd-orchestrator":
			return true
		}
	}
	return false
}

// Publish renders the staging file into RulesDir, manages the unified
// AGENTS.md for Antigravity ACP and external harnesses, and removes the Gentle
// AI rules it no longer produces. A missing or emptied staging file retires
// every owned rule and managed AGENTS.md section. Files outside the owned name
// pattern, or without one of the managed frontmatters, are never touched.
func Publish(baseDir string, opts ...Option) (Result, error) {
	var options publishOptions
	for _, opt := range opts {
		opt(&options)
	}

	staging, err := os.ReadFile(StagingPath(baseDir))
	if err != nil && !os.IsNotExist(err) {
		return Result{}, fmt.Errorf("read Antigravity guidance staging file: %w", err)
	}
	rules := Render(string(staging))
	dir := RulesDir(baseDir)
	result := Result{}
	wanted := map[string]bool{}
	for _, rule := range rules {
		path := filepath.Join(dir, rule.Name)
		wanted[path] = true
		write, err := filemerge.WriteFileAtomic(path, []byte(rule.Content), 0o644)
		if err != nil {
			return result, fmt.Errorf("write Antigravity rule %q: %w", path, err)
		}
		if write.Changed {
			result.Changed = true
			result.Files = append(result.Files, path)
		}
	}

	owned, err := OwnedRulePaths(baseDir)
	if err != nil {
		return result, err
	}
	for _, path := range owned {
		if wanted[path] {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return result, fmt.Errorf("remove stale Antigravity rule %q: %w", path, err)
		}
		result.Changed = true
		result.Files = append(result.Files, path)
	}

	if err := reconcileAgentsMD(baseDir, string(staging), options, &result); err != nil {
		return result, err
	}
	return result, nil
}

func reconcileAgentsMD(baseDir string, staging string, options publishOptions, result *Result) error {
	configAgentsPath := ConfigAgentsMDPath(baseDir)
	agentsPath := AgentsMDPath(baseDir)

	hasStaging := strings.TrimSpace(staging) != ""

	// 1. Manage .gemini/config/AGENTS.md
	if hasStaging {
		write, err := filemerge.WriteFileAtomic(configAgentsPath, []byte(staging), 0o644)
		if err != nil {
			return fmt.Errorf("write Antigravity config AGENTS.md: %w", err)
		}
		if write.Changed {
			result.Changed = true
			result.Files = append(result.Files, configAgentsPath)
		}
	} else {
		if err := os.Remove(configAgentsPath); err == nil {
			result.Changed = true
			result.Files = append(result.Files, configAgentsPath)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("remove Antigravity config AGENTS.md: %w", err)
		}
	}

	// 2. Manage baseDir/AGENTS.md
	info, lstatErr := os.Lstat(agentsPath)
	if lstatErr != nil && !os.IsNotExist(lstatErr) {
		return fmt.Errorf("stat AGENTS.md %q: %w", agentsPath, lstatErr)
	}
	if lstatErr == nil && info.IsDir() {
		return fmt.Errorf("expected %q to be a file, got directory", agentsPath)
	}

	isWorkspace := resolveIsWorkspace(baseDir, options)

	if lstatErr == nil && (info.Mode()&os.ModeSymlink != 0) {
		target, readlinkErr := os.Readlink(agentsPath)
		if readlinkErr != nil {
			return fmt.Errorf("readlink %q: %w", agentsPath, readlinkErr)
		}

		resolvedTarget := target
		if !filepath.IsAbs(resolvedTarget) {
			resolvedTarget = filepath.Clean(filepath.Join(filepath.Dir(agentsPath), resolvedTarget))
		} else {
			resolvedTarget = filepath.Clean(resolvedTarget)
		}

		isManagedConfigLink := resolvedTarget == filepath.Clean(configAgentsPath)
		isLegacyGeminiLink := resolvedTarget == filepath.Clean(LegacyPromptPath(baseDir))

		if !hasStaging {
			if isManagedConfigLink || isLegacyGeminiLink {
				if err := os.Remove(agentsPath); err == nil {
					result.Changed = true
					result.Files = append(result.Files, agentsPath)
				} else if !os.IsNotExist(err) {
					return fmt.Errorf("remove AGENTS.md symlink: %w", err)
				}
			}
			return nil
		}

		if isWorkspace {
			if isManagedConfigLink || isLegacyGeminiLink {
				raw, err := os.ReadFile(agentsPath)
				if err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("read %q: %w", agentsPath, err)
				}
				original := string(raw)

				updated := reconcileAgentsMDContent(original, staging, baseDir, options)
				if strings.TrimSpace(updated) == "" {
					if err := os.Remove(agentsPath); err != nil && !os.IsNotExist(err) {
						return fmt.Errorf("remove emptied %q: %w", agentsPath, err)
					}
					result.Changed = true
					result.Files = append(result.Files, agentsPath)
					return nil
				}

				if err := replaceSymlinkWithRegularFile(agentsPath, []byte(updated), 0o644); err != nil {
					return err
				}
				result.Changed = true
				result.Files = append(result.Files, agentsPath)
				return nil
			}
			return nil
		}

		if isLegacyGeminiLink {
			relTarget := filepath.Join(".gemini", "config", "AGENTS.md")
			if err := replaceSymlink(agentsPath, relTarget); err != nil {
				return fmt.Errorf("migrate legacy AGENTS.md symlink: %w", err)
			}
			result.Changed = true
			result.Files = append(result.Files, agentsPath)
			return nil
		}

		if isManagedConfigLink {
			return nil
		}
		return nil
	}

	if lstatErr == nil && info.Mode().IsRegular() {
		raw, err := os.ReadFile(agentsPath)
		if err != nil {
			return fmt.Errorf("read %q: %w", agentsPath, err)
		}
		original := string(raw)

		updated := reconcileAgentsMDContent(original, staging, baseDir, options)

		if strings.TrimSpace(updated) == "" {
			if err := os.Remove(agentsPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove emptied %q: %w", agentsPath, err)
			}
			result.Changed = true
			result.Files = append(result.Files, agentsPath)
			return nil
		}

		if updated != original {
			write, err := filemerge.WriteFileAtomic(agentsPath, []byte(updated), info.Mode().Perm())
			if err != nil {
				return fmt.Errorf("write %q: %w", agentsPath, err)
			}
			if write.Changed {
				result.Changed = true
				result.Files = append(result.Files, agentsPath)
			}
		}
		return nil
	}

	if os.IsNotExist(lstatErr) && hasStaging {
		if isWorkspace {
			write, err := filemerge.WriteFileAtomic(agentsPath, []byte(staging), 0o644)
			if err != nil {
				return fmt.Errorf("write workspace %q: %w", agentsPath, err)
			}
			if write.Changed {
				result.Changed = true
				result.Files = append(result.Files, agentsPath)
			}
			return nil
		}

		relTarget := filepath.Join(".gemini", "config", "AGENTS.md")
		if err := os.Symlink(relTarget, agentsPath); err == nil {
			result.Changed = true
			result.Files = append(result.Files, agentsPath)
		} else {
			write, err := filemerge.WriteFileAtomic(agentsPath, []byte(staging), 0o644)
			if err != nil {
				return fmt.Errorf("write fallback %q: %w", agentsPath, err)
			}
			if write.Changed {
				result.Changed = true
				result.Files = append(result.Files, agentsPath)
			}
		}
	}

	return nil
}

func resolveIsWorkspace(baseDir string, options publishOptions) bool {
	if options.isWorkspace != nil {
		return *options.isWorkspace
	}
	if _, err := os.Stat(filepath.Join(baseDir, ".git")); err == nil {
		return true
	}
	return false
}

func reconcileAgentsMDContent(original string, staging string, baseDir string, options publishOptions) string {
	stagedSections := Sections(staging)
	stagedMap := make(map[string]string, len(stagedSections))
	for _, sec := range stagedSections {
		stagedMap[sec.ID] = sec.Body
	}

	updated := original
	// 1. Update or inject all sections present in staging
	for _, sec := range stagedSections {
		updated = filemerge.InjectMarkdownSection(updated, sec.ID, sec.Body)
	}

	// 2. Identify sections present in original that should be removed:
	// Any managed section that is NOT in staging, provided no other active agent needs it.
	for _, sec := range Sections(original) {
		if _, inStaging := stagedMap[sec.ID]; inStaging {
			continue
		}
		if !managedSectionIDs[sec.ID] {
			continue
		}
		if otherAgentNeedsSection(baseDir, sec.ID, options.activeAgents) {
			continue
		}
		updated = filemerge.InjectMarkdownSection(updated, sec.ID, "")
	}

	return updated
}

func replaceSymlinkWithRegularFile(path string, content []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".agents-regular-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file for regular %q: %w", path, err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file for regular %q: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp file for regular %q: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file for regular %q: %w", path, err)
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("chmod temp file for regular %q: %w", path, err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace symlink with regular file %q: %w", path, err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("verify regular file replacement %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("replace symlink with regular file %q: destination is not a regular file", path)
	}
	readBack, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("verify regular file replacement %q: %w", path, err)
	}
	if !bytes.Equal(readBack, content) {
		return fmt.Errorf("replace symlink with regular file %q: content mismatch after rename", path)
	}
	return nil
}

func replaceSymlink(symlinkPath string, target string) error {
	dir := filepath.Dir(symlinkPath)
	tmpLink := filepath.Join(dir, fmt.Sprintf(".agents-link-%d-%d.tmp", os.Getpid(), time.Now().UnixNano()))
	if err := os.Symlink(target, tmpLink); err != nil {
		return fmt.Errorf("create temporary symlink: %w", err)
	}
	defer func() {
		_ = os.Remove(tmpLink)
	}()

	if err := os.Rename(tmpLink, symlinkPath); err != nil {
		return fmt.Errorf("replace symlink %q: %w", symlinkPath, err)
	}
	got, err := os.Readlink(symlinkPath)
	if err != nil {
		return fmt.Errorf("verify symlink replacement %q: %w", symlinkPath, err)
	}
	if got != target {
		return fmt.Errorf("replace symlink %q: link does not point to %q (got %q)", symlinkPath, target, got)
	}
	return nil
}

// OwnedRulePaths lists the rule files Publish owns that exist on disk.
func OwnedRulePaths(baseDir string) ([]string, error) {
	dir := RulesDir(baseDir)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read Antigravity rules directory: %w", err)
	}
	var paths []string
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !ownedRulePattern.MatchString(entry.Name()) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read Antigravity rule %q: %w", path, err)
		}
		if managedHeaderPattern.Match(content) {
			paths = append(paths, path)
		}
	}
	return paths, nil
}

// ManagedPaths returns every rule file Publish may write or remove for the
// current staging content, for backup snapshots. It is best effort: an
// unreadable rules directory or staging file contributes no paths, and
// Publish itself reports those errors.
func ManagedPaths(baseDir string) []string {
	paths, _ := OwnedRulePaths(baseDir)
	staging, _ := os.ReadFile(StagingPath(baseDir))
	for _, rule := range Render(string(staging)) {
		path := filepath.Join(RulesDir(baseDir), rule.Name)
		if !slices.Contains(paths, path) {
			paths = append(paths, path)
		}
	}
	for _, path := range []string{ConfigAgentsMDPath(baseDir), AgentsMDPath(baseDir)} {
		if !slices.Contains(paths, path) {
			paths = append(paths, path)
		}
	}
	return paths
}

// GeminiCLIManaged reports whether Gentle AI manages Gemini CLI, which then
// owns ~/.gemini/GEMINI.md: it is selected in this run or recorded in
// state.json. A gemini binary on PATH alone does not count. An unreadable
// state file counts as managed so the shared file is preserved.
func GeminiCLIManaged(homeDir string, selected []model.AgentID) bool {
	if slices.Contains(selected, model.AgentGeminiCLI) {
		return true
	}
	persisted, err := state.Read(homeDir)
	if err != nil {
		return !os.IsNotExist(err)
	}
	return slices.Contains(persisted.InstalledAgents, string(model.AgentGeminiCLI))
}

// MigrateLegacyPrompt removes the Gentle AI sections older releases appended
// for Antigravity to ~/.gemini/GEMINI.md, preserving every byte outside them.
// It does nothing while Gentle AI manages Gemini CLI: that file is then Gemini
// CLI's prompt, and its persona writer drops a duplicated marked persona
// section itself (#5608).
func MigrateLegacyPrompt(baseDir string, geminiCLIManaged bool) (Result, error) {
	if geminiCLIManaged {
		return Result{}, nil
	}
	path := LegacyPromptPath(baseDir)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return Result{}, nil
	}
	if err != nil {
		return Result{}, fmt.Errorf("stat %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return Result{}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Result{}, fmt.Errorf("read %q: %w", path, err)
	}
	original := string(raw)

	var ids []string
	for _, match := range openMarkerPattern.FindAllStringSubmatch(original, -1) {
		if !slices.Contains(ids, match[1]) {
			ids = append(ids, match[1])
		}
	}
	updated := original
	for _, id := range ids {
		if !strings.Contains(updated, "<!-- /gentle-ai:"+id+" -->") {
			continue
		}
		updated = filemerge.InjectMarkdownSection(updated, id, "")
	}
	if updated == original {
		return Result{}, nil
	}
	if strings.TrimSpace(updated) == "" {
		if err := os.Remove(path); err != nil {
			return Result{}, fmt.Errorf("remove emptied %q: %w", path, err)
		}
		return Result{Changed: true, Files: []string{path}}, nil
	}
	if _, err := filemerge.WriteFileAtomic(path, []byte(updated), info.Mode().Perm()); err != nil {
		return Result{}, err
	}
	return Result{Changed: true, Files: []string{path}}, nil
}
