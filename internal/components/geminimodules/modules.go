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
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
)

// ImportSectionID marks the only Gentle AI block GEMINI.md keeps.
const ImportSectionID = "gemini-cli-imports"

// ownedModulePattern is the only name shape Publish writes inside ModulesDir,
// so stale-module cleanup never touches other files there.
var ownedModulePattern = regexp.MustCompile(`^\d{2}-[A-Za-z0-9_.-]+\.md$`)

// Module is one rendered module file.
type Module struct {
	Name    string
	Content string
}

// Result reports the files an operation changed or removed.
type Result struct {
	Changed bool
	Files   []string
}

func (r *Result) add(path string) {
	r.Changed = true
	r.Files = append(r.Files, path)
}

// RootPath is the GEMINI.md Gemini CLI loads globally.
func RootPath(baseDir string) string { return filepath.Join(baseDir, ".gemini", "GEMINI.md") }

// ModulesDir is the Gentle AI-owned directory holding the modules.
func ModulesDir(baseDir string) string { return filepath.Join(baseDir, ".gemini", "gentle-ai") }

// StagingPath is the Gemini CLI adapter's SystemPromptFile, inside ModulesDir
// but outside the owned module names.
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

// stripSections removes every complete gentle-ai marker section, preserving
// every byte outside them.
func stripSections(content string) string {
	for _, section := range antigravityrules.Sections(content) {
		content = filemerge.InjectMarkdownSection(content, section.ID, "")
	}
	return content
}

// Publish renders the staging file into ModulesDir, removes the owned modules
// it no longer produces, and rewrites GEMINI.md as the content outside
// Gentle AI marker sections plus one import block. A missing or emptied
// staging file retires every module and the import block.
func Publish(baseDir string) (Result, error) {
	staging, err := os.ReadFile(StagingPath(baseDir))
	if err != nil && !os.IsNotExist(err) {
		return Result{}, fmt.Errorf("read Gemini CLI guidance staging file: %w", err)
	}
	modules := Render(string(staging))
	result := Result{}
	wanted := map[string]bool{}
	imports := ""
	for _, module := range modules {
		path := filepath.Join(ModulesDir(baseDir), module.Name)
		wanted[path] = true
		imports += "@./gentle-ai/" + module.Name + "\n"
		write, err := filemerge.WriteFileAtomic(path, []byte(module.Content), 0o644)
		if err != nil {
			return result, fmt.Errorf("write Gemini CLI module %q: %w", path, err)
		}
		if write.Changed {
			result.add(path)
		}
	}
	owned, err := OwnedModulePaths(baseDir)
	if err != nil {
		return result, err
	}
	for _, path := range owned {
		if wanted[path] {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return result, fmt.Errorf("remove stale Gemini CLI module %q: %w", path, err)
		}
		result.add(path)
	}

	root := RootPath(baseDir)
	raw, err := os.ReadFile(root)
	if err != nil && !os.IsNotExist(err) {
		return result, fmt.Errorf("read %q: %w", root, err)
	}
	updated := stripSections(string(raw))
	if imports != "" {
		updated = filemerge.InjectMarkdownSection(updated, ImportSectionID, imports)
	}
	if updated != string(raw) {
		if err := writeOrRemove(root, updated); err != nil {
			return result, err
		}
		result.add(root)
	}
	if len(modules) == 0 {
		// Only an emptied directory goes; it is Gentle AI's own, and the
		// files it held are already reported.
		_ = os.Remove(ModulesDir(baseDir))
	}
	return result, nil
}

// Retire removes the staging file, every owned module, the import block and
// the emptied module directory, preserving the user's GEMINI.md content.
func Retire(baseDir string) (Result, error) {
	result := Result{}
	staging := StagingPath(baseDir)
	if err := os.Remove(staging); err == nil {
		result.add(staging)
	} else if !os.IsNotExist(err) {
		return result, fmt.Errorf("remove Gemini CLI guidance staging file: %w", err)
	}
	published, err := Publish(baseDir)
	result.Changed = result.Changed || published.Changed
	result.Files = append(result.Files, published.Files...)
	return result, err
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

// writeOrRemove writes content keeping the file mode, or removes the file
// when nothing but whitespace is left.
func writeOrRemove(path, content string) error {
	if strings.TrimSpace(content) == "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove emptied %q: %w", path, err)
		}
		return nil
	}
	if _, err := filemerge.WriteFileAtomic(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	return nil
}
