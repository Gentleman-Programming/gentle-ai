package skillregistry

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

const (
	RegistryRelPath = ".atl/skill-registry.md"
	CacheRelPath    = ".atl/.skill-registry.cache.json"
	sectionMarker   = "## Skills"
	atlIgnoreEntry  = ".atl/"
)

// Excluded skills never appear in the registry. This policy is intentionally
// hardcoded and applied silently: `_shared` and `skill-registry` are internal
// plumbing, and `sdd-*` skills belong to the SDD workflow retired in v4.0.0.
// Install and sync retire the copies Gentle AI can prove it wrote, in the
// global skill roots and, for a workspace-scoped run, in the workspace ones
// (<workspace>/.claude/skills, for example). They keep edited copies, copies
// behind symlinked directories, and everything in roots Gentle AI does not
// manage (Pi, other project skill directories), so the exclusion keeps those
// leftovers, which call removed commands, out of delegation. NOTE: a user skill whose name collides with these
// (e.g. any name starting with `sdd-`) is dropped without warning. Revisit as
// configuration if that collision ever becomes a real constraint.
var (
	excludeNames    = map[string]bool{"_shared": true, "skill-registry": true}
	excludePrefixes = []string{"sdd-"}
	frontmatterLine = regexp.MustCompile(`^(\w+):\s*(.*)$`)
)

type SkillEntry struct {
	Name        string
	Path        string
	Description string
}

type Result struct {
	Regenerated bool
	SkillCount  int
	Reason      string
	Registry    string
	Cache       string
}

// Keep these source roots in sync with the gentle-pi skill-registry extension.
func UserSkillDirs(home string) []string {
	return []string{
		// Gentle AI/Pi and generic Agent Skills locations.
		filepath.Join(home, ".pi", "agent", "skills"),
		filepath.Join(home, ".config", "agents", "skills"),
		filepath.Join(home, ".agents", "skills"),
		filepath.Join(home, ".kimi-code", "skills"),
		filepath.Join(home, ".kimi", "skills"),

		// Agent-specific global skill locations supported by Gentle AI adapters.
		filepath.Join(home, ".config", "opencode", "skills"),
		filepath.Join(home, ".config", "kilo", "skills"),
		filepath.Join(home, ".claude", "skills"),
		filepath.Join(home, ".gemini", "skills"),
		filepath.Join(home, ".gemini", "config", "skills"),
		filepath.Join(home, ".gemini", "antigravity", "skills"),
		filepath.Join(home, ".gemini", "antigravity-desktop", "skills"),
		filepath.Join(home, ".gemini", "antigravity-cli", "skills"),
		filepath.Join(home, ".cursor", "skills"),
		filepath.Join(home, ".copilot", "skills"),
		filepath.Join(system.CodexConfigDir(home), "skills"),
		filepath.Join(home, ".codeium", "windsurf", "skills"),
		filepath.Join(home, ".qwen", "skills"),
		filepath.Join(home, ".kiro", "skills"),
		filepath.Join(home, ".openclaw", "skills"),
		filepath.Join(home, ".hermes", "skills"),
		filepath.Join(home, ".trae", "skills"),
	}
}

func ProjectSkillDirs(cwd string) []string {
	return []string{
		// Generic project skills first: repo-local intent beats user/global skills.
		filepath.Join(cwd, "skills"),

		// Agent-native workspace skill locations.
		filepath.Join(cwd, ".opencode", "skills"),
		filepath.Join(cwd, ".claude", "skills"),
		filepath.Join(cwd, ".gemini", "skills"),
		filepath.Join(cwd, ".trae", "skills"),
		filepath.Join(cwd, ".cursor", "skills"),
		filepath.Join(cwd, ".github", "skills"),
		filepath.Join(cwd, ".codex", "skills"),
		filepath.Join(cwd, ".qwen", "skills"),
		filepath.Join(cwd, ".kiro", "skills"),
		filepath.Join(cwd, ".openclaw", "skills"),

		// Gentle AI/Pi and generic Agent Skills workspace locations.
		filepath.Join(cwd, ".pi", "skills"),
		filepath.Join(cwd, ".agent", "skills"),
		filepath.Join(cwd, ".agents", "skills"),
		filepath.Join(cwd, ".atl", "skills"),
		filepath.Join(cwd, ".hermes", "skills"),
	}
}

func Regenerate(cwd, home string, force bool) (Result, error) {
	cwd, err := filepath.Abs(cwd)
	if err != nil {
		return Result{}, fmt.Errorf("resolve cwd: %w", err)
	}
	home, err = filepath.Abs(home)
	if err != nil {
		return Result{}, fmt.Errorf("resolve home: %w", err)
	}
	// Force remains an accepted deliberate refresh; every refresh now scans and
	// compares rendered bytes, so it does not rewrite an identical registry.
	_ = force
	dirs, entries := discover(cwd, home)
	sources := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		if rel, ok := relativeWithin(cwd, dir); ok {
			sources = append(sources, rel)
		} else {
			sources = append(sources, dir)
		}
	}

	registryPath := filepath.Join(cwd, RegistryRelPath)
	cachePath := filepath.Join(cwd, CacheRelPath)
	if err := os.MkdirAll(filepath.Join(cwd, ".atl"), 0o755); err != nil {
		return Result{}, fmt.Errorf("create .atl directory: %w", err)
	}
	registryBytes := []byte(RenderRegistry(cwd, sources, entries))
	written, err := filemerge.WriteFileAtomic(registryPath, registryBytes, 0o644)
	if err != nil {
		return Result{}, fmt.Errorf("write registry: %w", err)
	}
	// This is a derived hint, never an authority for skipping a disk scan or
	// registry comparison. Publish it only after the registry; the pair is not
	// atomic, and the next refresh repairs any mismatch.
	sum := sha256.Sum256(registryBytes)
	cacheBytes := []byte(fmt.Sprintf("{\"schema\":1,\"registry_sha256\":\"%x\"}\n", sum))
	if _, err := filemerge.WriteFileAtomic(cachePath, cacheBytes, 0o644); err != nil {
		return Result{}, fmt.Errorf("write registry cache: %w", err)
	}
	reason := "unchanged"
	if written.Changed {
		reason = "content-changed"
	}
	return Result{Regenerated: written.Changed, SkillCount: len(entries), Reason: reason, Registry: registryPath, Cache: cachePath}, nil
}

// List resolves the same disk winners as Regenerate without writing files.
func List(cwd, home string) []SkillEntry {
	cwd, err := filepath.Abs(cwd)
	if err != nil {
		return nil
	}
	home, err = filepath.Abs(home)
	if err != nil {
		return nil
	}
	_, entries := discover(cwd, home)
	return entries
}

func discover(cwd, home string) ([]string, []SkillEntry) {
	dirs := uniqueExistingDirs(append(ProjectSkillDirs(cwd), UserSkillDirs(home)...))
	files := findAllSkillFiles(dirs)
	entries := make([]SkillEntry, 0, len(files))
	for _, file := range files {
		if entry, ok := LoadSkill(file); ok {
			entries = append(entries, entry)
		}
	}
	return dirs, dedupeBySkillName(entries, cwd)
}

func EnsureATLIgnored(cwd string) error {
	gitignorePath := filepath.Join(cwd, ".gitignore")
	existingBytes, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read .gitignore: %w", err)
	}
	existing := string(existingBytes)
	for _, line := range strings.Split(existing, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == ".atl" || trimmed == atlIgnoreEntry {
			return nil
		}
	}
	prefix := ""
	if len(existing) > 0 && !strings.HasSuffix(existing, "\n") {
		prefix = "\n"
	}
	header := ""
	if !strings.Contains(existing, "# Local AI runtime state") && !strings.Contains(existing, "# Local Pi runtime state") {
		header = "# Local AI runtime state\n"
	}
	// Atomic write guards against concurrent startup hooks (Codex + OpenCode +
	// Claude) racing on .gitignore, matching how the registry file is written.
	if _, err := filemerge.WriteFileAtomic(gitignorePath, []byte(existing+prefix+header+atlIgnoreEntry+"\n"), 0o644); err != nil {
		return fmt.Errorf("write .gitignore: %w", err)
	}
	return nil
}

func LoadSkill(file string) (SkillEntry, bool) {
	data, err := os.ReadFile(file)
	if err != nil {
		return SkillEntry{}, false
	}
	name, desc := parseFrontmatter(string(data))
	if strings.TrimSpace(name) == "" {
		name = filepath.Base(filepath.Dir(file))
	}
	if isExcluded(name) {
		return SkillEntry{}, false
	}
	return SkillEntry{Name: name, Path: file, Description: desc}, true
}

func RenderRegistry(cwd string, sources []string, entries []SkillEntry) string {
	projectName := filepath.Base(cwd)
	var lines []string
	lines = append(lines, "# Skill Registry — "+projectName, "")
	lines = append(lines, "<!-- Shared disk skill registry; generated by Gentle Shell or Gentle AI. -->", "")
	lines = append(lines, "## Sources scanned", "")
	for _, src := range sources {
		lines = append(lines, "- "+src)
	}
	lines = append(lines, "", "## Contract", "")
	lines = append(lines, "**Delegator use only.** This disk index is not a summary and excludes session-only runtime skills. Pi delegators use their effective session view; other delegators use these rows. Pass exact `SKILL.md` paths to subagents.", "")
	lines = append(lines, "`SKILL.md` remains the source of truth. Do not inject generated summaries or compact rules by default; pass paths so subagents load the full runtime contract and preserve author intent.", "")
	lines = append(lines, sectionMarker, "")
	lines = append(lines, "| Skill | Trigger / description | Scope | Path |")
	lines = append(lines, "| --- | --- | --- | --- |")
	for _, entry := range entries {
		scope := ScopeForPath(cwd, entry.Path)
		lines = append(lines, fmt.Sprintf("| `%s` | %s | %s | `%s` |", markdownCell(entry.Name), markdownCell(entry.Description), markdownCell(scope), markdownCell(entry.Path)))
	}
	lines = append(lines, "", "## Loading protocol", "")
	lines = append(lines, "1. Match task context and target files against the effective view's `Trigger / description` column.")
	lines = append(lines, "2. Pass only the matching `Path` values to the subagent under `## Skills to load before work`.")
	lines = append(lines, "3. Instruct the subagent to read those exact `SKILL.md` files before reading, writing, reviewing, testing, or creating artifacts.")
	lines = append(lines, "4. If no matching skill exists, proceed without project skill injection and report `skill_resolution: none`.")
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

// Direct skills precede all one-level category skills within each root.
// Direct symlinks are followed, but categories and nested directories must be
// real directories; never descend under a child with its own SKILL.md.
func findAllSkillFiles(dirs []string) []string {
	var out []string
	for _, root := range dirs {
		children, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		var direct, nested, categories []string
		for _, child := range children {
			if isExcluded(child.Name()) {
				continue
			}
			dir := filepath.Join(root, child.Name())
			if !dirExists(dir) {
				continue
			}
			candidate := filepath.Join(dir, "SKILL.md")
			if fileExists(candidate) {
				direct = append(direct, candidate)
			} else if child.IsDir() {
				categories = append(categories, dir)
			}
		}
		for _, category := range categories {
			children, err := os.ReadDir(category)
			if err != nil {
				continue
			}
			for _, child := range children {
				if !child.IsDir() || isExcluded(child.Name()) {
					continue
				}
				candidate := filepath.Join(category, child.Name(), "SKILL.md")
				if fileExists(candidate) {
					nested = append(nested, candidate)
				}
			}
		}
		// Directory-name order is not necessarily full-path order: for
		// example, foo-bar/SKILL.md precedes foo/SKILL.md by byte value.
		sort.Strings(direct)
		sort.Strings(nested)
		out = append(out, direct...)
		out = append(out, nested...)
	}
	return out
}

func uniqueExistingDirs(dirs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, dir := range dirs {
		clean := filepath.Clean(dir)
		if seen[clean] || !dirExists(clean) {
			continue
		}
		seen[clean] = true
		if _, err := os.ReadDir(clean); err != nil {
			continue
		}
		out = append(out, clean)
	}
	return out
}

func parseFrontmatter(source string) (name, description string) {
	source = strings.ReplaceAll(source, "\r\n", "\n")
	source = strings.ReplaceAll(source, "\r", "\n")
	if !strings.HasPrefix(source, "---\n") {
		return "", ""
	}
	end := strings.Index(source[4:], "\n---")
	if end == -1 {
		return "", ""
	}
	end += 4
	fm := source[4:end]
	lines := strings.Split(fm, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		m := frontmatterLine.FindStringSubmatch(line)
		if len(m) != 3 {
			continue
		}
		value := strings.TrimSpace(m[2])
		if value == ">" || value == "|-" || value == "|" || value == ">-" {
			var block []string
			for i+1 < len(lines) {
				next := lines[i+1]
				if strings.TrimSpace(next) == "" {
					block = append(block, "")
					i++
					continue
				}
				if !strings.HasPrefix(next, " ") && !strings.HasPrefix(next, "\t") {
					break
				}
				block = append(block, strings.TrimSpace(next))
				i++
			}
			value = strings.Join(block, " ")
		} else if len(value) >= 2 && (value[0] == '\'' || value[0] == '"') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}
		switch m[1] {
		case "name":
			name = value
		case "description":
			description = value
		}
	}
	return name, strings.Join(strings.Fields(description), " ")
}

func dedupeBySkillName(entries []SkillEntry, cwd string) []SkillEntry {
	buckets := map[string][]SkillEntry{}
	for _, entry := range entries {
		buckets[entry.Name] = append(buckets[entry.Name], entry)
	}
	out := make([]SkillEntry, 0, len(buckets))
	for _, list := range buckets {
		chosen := list[0]
		for _, entry := range list {
			if ScopeForPath(cwd, entry.Path) == "project" {
				chosen = entry
				break
			}
		}
		out = append(out, chosen)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ScopeForPath reports whether a skill path is project-local or user-global,
// relative to cwd.
func ScopeForPath(cwd, path string) string {
	if rel, ok := relativeWithin(cwd, path); ok && rel != "." {
		return "project"
	}
	return "user"
}

func relativeWithin(cwd, path string) (string, bool) {
	rel, err := filepath.Rel(filepath.Clean(cwd), filepath.Clean(path))
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", false
	}
	return rel, true
}

func markdownCell(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", "\\|")
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "—"
	}
	return trimmed
}

func isExcluded(name string) bool {
	if excludeNames[name] {
		return true
	}
	for _, prefix := range excludePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
