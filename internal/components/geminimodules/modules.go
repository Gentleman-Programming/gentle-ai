// Package geminimodules delivers the Gentle AI guidance staged for Gemini CLI
// as module files that ~/.gemini/GEMINI.md imports.
//
// Every Gentle AI prompt writer targets the adapter's SystemPromptFile, a
// staging file nothing loads. Publish splits it, in order, into
// ~/.gemini/gentle-ai/NN-<section-id>.md and keeps one managed block of
// `@./gentle-ai/...` import lines in GEMINI.md, which Gemini CLI expands.
// Antigravity also loads GEMINI.md but does not expand those imports, so it
// no longer receives a duplicated, truncated copy of the guidance (#1989).
package geminimodules

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/antigravityrules"
)

// ownedModulePattern is the only name shape Publish writes inside ModulesDir,
// so stale-module cleanup never touches other files there.
var ownedModulePattern = regexp.MustCompile(`^\d{2}-[A-Za-z0-9_.-]+\.md$`)

// Module is one rendered module file.
type Module struct {
	Name    string
	Content string
}

// RootPath is the GEMINI.md Gemini CLI loads globally.
func RootPath(baseDir string) string { return filepath.Join(baseDir, ".gemini", "GEMINI.md") }

// ModulesDir is the Gentle AI-owned directory holding the modules.
func ModulesDir(baseDir string) string { return filepath.Join(baseDir, ".gemini", "gentle-ai") }

// StagingPath is the Gemini CLI guidance staging file, inside ModulesDir but
// outside the owned module names.
func StagingPath(baseDir string) string { return filepath.Join(ModulesDir(baseDir), "guidance.md") }

// Render partitions the staging content into modules: the persona body the
// persona writer puts before the first marker section, then every complete
// marker section with the text up to the next one. Concatenating the module
// contents in order yields the staging content unchanged.
func Render(staging string) []Module {
	var modules []Module
	add := func(id, content string) {
		modules = append(modules, Module{Name: fmt.Sprintf("%02d-%s.md", len(modules)+1, id), Content: content})
	}
	sections := antigravityrules.Sections(staging)
	if len(sections) == 0 {
		if strings.TrimSpace(staging) != "" {
			add("persona", staging)
		}
		return modules
	}
	start := 0
	if strings.TrimSpace(staging[:sections[0].Start]) != "" {
		add("persona", staging[:sections[0].Start])
		start = sections[0].Start
	}
	for i, section := range sections {
		end := len(staging)
		if i+1 < len(sections) {
			end = sections[i+1].Start
		}
		add(section.ID, staging[start:end])
		start = end
	}
	return modules
}

// OwnedModulePaths lists the module files Publish owns that exist on disk.
func OwnedModulePaths(baseDir string) ([]string, error) {
	dir := ModulesDir(baseDir)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read Gemini CLI module directory: %w", err)
	}
	var paths []string
	for _, entry := range entries {
		if entry.Type().IsRegular() && ownedModulePattern.MatchString(entry.Name()) {
			paths = append(paths, filepath.Join(dir, entry.Name()))
		}
	}
	return paths, nil
}

// ManagedPaths returns GEMINI.md, the staging file and every module Publish
// may write or remove for the current staging content, for backup snapshots.
// It is best effort: unreadable inputs contribute no module paths, and
// Publish itself reports those errors.
func ManagedPaths(baseDir string) []string {
	paths, _ := OwnedModulePaths(baseDir)
	staging, _ := os.ReadFile(StagingPath(baseDir))
	for _, module := range Render(string(staging)) {
		if path := filepath.Join(ModulesDir(baseDir), module.Name); !slices.Contains(paths, path) {
			paths = append(paths, path)
		}
	}
	return append(paths, StagingPath(baseDir), RootPath(baseDir))
}
