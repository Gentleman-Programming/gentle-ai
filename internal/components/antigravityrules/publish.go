package antigravityrules

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

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

// LegacyPromptPath is the Gemini-compatible prompt older releases appended
// Antigravity sections to.
func LegacyPromptPath(baseDir string) string {
	return filepath.Join(baseDir, ".gemini", "GEMINI.md")
}

// Publish renders the staging file into RulesDir and removes the Gentle AI
// rules it no longer produces. A missing or emptied staging file retires every
// owned rule. Files outside the owned name pattern, or without one of the
// managed frontmatters, are never touched.
func Publish(baseDir string) (Result, error) {
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
	return result, nil
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
