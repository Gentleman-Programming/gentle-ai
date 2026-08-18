package skillregistry

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
)

const (
	RegistryRelPath = ".atl/skill-registry.md"
	CacheRelPath    = ".atl/.skill-registry.cache.json"
	RegistrySchema  = 6
	sectionMarker   = "## Skills"
	atlIgnoreEntry  = ".atl/"
)

// Excluded skills never appear in the registry. This policy is intentionally
// hardcoded and applied silently: `_shared` and `skill-registry` are internal
// plumbing, and `sdd-*` skills are orchestrator-managed via the SDD workflow,
// not delegator-selected. NOTE: a user skill whose name collides with these
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

type cacheFile struct {
	Fingerprint string `json:"fingerprint"`
}

// Keep these source roots in sync with the gentle-pi skill-registry extension.
func UserSkillDirs(home string) []string {
	return []string{
		// Gentle AI/Pi and generic Agent Skills locations.
		filepath.Join(home, ".pi", "agent", "skills"),
		filepath.Join(home, ".config", "agents", "skills"),
		filepath.Join(home, ".agents", "skills"),
		filepath.Join(home, ".kimi", "skills"),

		// Agent-specific global skill locations supported by Gentle AI adapters.
		filepath.Join(home, ".config", "opencode", "skills"),
		filepath.Join(home, ".config", "kilo", "skills"),
		filepath.Join(home, ".claude", "skills"),
		filepath.Join(home, ".gemini", "skills"),
		filepath.Join(home, ".gemini", "antigravity", "skills"),
		filepath.Join(home, ".gemini", "antigravity-desktop", "skills"),
		filepath.Join(home, ".gemini", "antigravity-cli", "skills"),
		filepath.Join(home, ".cursor", "skills"),
		filepath.Join(home, ".copilot", "skills"),
		filepath.Join(home, ".codex", "skills"),
		filepath.Join(home, ".codeium", "windsurf", "skills"),
		filepath.Join(home, ".qwen", "skills"),
		filepath.Join(home, ".kiro", "skills"),
		filepath.Join(home, ".openclaw", "skills"),
		filepath.Join(home, ".hermes", "skills"),
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
	cwd = filepath.Clean(cwd)
	home = filepath.Clean(home)

	existingDirs := uniqueExistingDirs(append(ProjectSkillDirs(cwd), UserSkillDirs(home)...))
	discovered, err := findAllSkillFiles(existingDirs)
	if err != nil {
		return Result{}, err
	}

	registryPath := filepath.Join(cwd, RegistryRelPath)
	cachePath := filepath.Join(cwd, CacheRelPath)
	fp := Fingerprint(pathsOf(discovered))
	cached := readCachedFingerprint(cachePath)
	if !force && cached == fp && fileExists(registryPath) {
		return Result{Regenerated: false, Reason: "cache-hit", Registry: registryPath, Cache: cachePath}, nil
	}

	entries := make([]SkillEntry, 0, len(discovered))
	for _, ds := range discovered {
		entry, ok := LoadSkill(ds.Path)
		if !ok {
			continue
		}
		entries = append(entries, namespacedEntry(entry, ds.Namespace))
	}
	entries = dedupeBySkillName(entries, cwd)

	sources := make([]string, 0, len(existingDirs))
	for _, dir := range existingDirs {
		rel, err := filepath.Rel(cwd, dir)
		if err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			sources = append(sources, rel)
		} else if err == nil && rel == "." {
			sources = append(sources, ".")
		} else {
			sources = append(sources, dir)
		}
	}

	if err := os.MkdirAll(filepath.Join(cwd, ".atl"), 0o755); err != nil {
		return Result{}, fmt.Errorf("create .atl directory: %w", err)
	}
	md := RenderRegistry(cwd, sources, entries)
	if _, err := filemerge.WriteFileAtomic(registryPath, []byte(md), 0o644); err != nil {
		return Result{}, fmt.Errorf("write registry: %w", err)
	}
	cacheBytes, err := json.MarshalIndent(cacheFile{Fingerprint: fp}, "", "  ")
	if err != nil {
		return Result{}, err
	}
	cacheBytes = append(cacheBytes, '\n')
	if _, err := filemerge.WriteFileAtomic(cachePath, cacheBytes, 0o644); err != nil {
		return Result{}, fmt.Errorf("write registry cache: %w", err)
	}

	reason := "fingerprint-changed"
	if force {
		reason = "forced"
	}
	return Result{Regenerated: true, SkillCount: len(entries), Reason: reason, Registry: registryPath, Cache: cachePath}, nil
}

// List resolves the deduplicated, sorted set of skills that Regenerate would
// index, without writing the registry, cache, or .gitignore. It is the
// read-only inspection path behind `gentle-ai skill-registry list`.
func List(cwd, home string) []SkillEntry {
	cwd = filepath.Clean(cwd)
	home = filepath.Clean(home)
	existingDirs := uniqueExistingDirs(append(ProjectSkillDirs(cwd), UserSkillDirs(home)...))
	discovered, err := findAllSkillFiles(existingDirs)
	if err != nil {
		return nil
	}
	entries := make([]SkillEntry, 0, len(discovered))
	for _, ds := range discovered {
		if entry, ok := LoadSkill(ds.Path); ok {
			entries = append(entries, namespacedEntry(entry, ds.Namespace))
		}
	}
	return dedupeBySkillName(entries, cwd)
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

func Fingerprint(files []string) string {
	lines := make([]string, 0, len(files)+1)
	lines = append(lines, fmt.Sprintf("schema:%d", RegistrySchema))
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			lines = append(lines, file+":missing")
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			lines = append(lines, file+":unreadable")
			continue
		}
		contentSum := sha1.Sum(data)
		lines = append(lines, fmt.Sprintf("%s:%d:%d:%x", file, info.ModTime().UnixNano(), info.Size(), contentSum))
	}
	sort.Strings(lines)
	sum := sha1.Sum([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
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
	lines = append(lines, "<!-- Auto-generated by gentle-ai skill-registry refresh. Run `gentle-ai skill-registry refresh --force` to regenerate. -->", "")
	lines = append(lines, "Last updated: "+time.Now().UTC().Format("2006-01-02"), "")
	lines = append(lines, "## Sources scanned", "")
	for _, src := range sources {
		lines = append(lines, "- "+src)
	}
	lines = append(lines, "", "## Contract", "")
	lines = append(lines, "**Delegator use only.** This registry is an index, not a summary. Any agent that launches subagents reads it to select relevant skills, then passes exact `SKILL.md` paths for the subagent to read before work.", "")
	lines = append(lines, "`SKILL.md` remains the source of truth. Do not inject generated summaries or compact rules by default; pass paths so subagents load the full runtime contract and preserve author intent.", "")
	lines = append(lines, sectionMarker, "")
	lines = append(lines, "| Skill | Trigger / description | Scope | Path |")
	lines = append(lines, "| --- | --- | --- | --- |")
	for _, entry := range entries {
		scope := ScopeForPath(cwd, entry.Path)
		lines = append(lines, fmt.Sprintf("| `%s` | %s | %s | `%s` |", markdownCell(entry.Name), markdownCell(entry.Description), markdownCell(scope), markdownCell(entry.Path)))
	}
	lines = append(lines, "", "## Loading protocol", "")
	lines = append(lines, "1. Match task context and target files against the `Trigger / description` column.")
	lines = append(lines, "2. Pass only the matching `Path` values to the subagent under `## Skills to load before work`.")
	lines = append(lines, "3. Instruct the subagent to read those exact `SKILL.md` files before reading, writing, reviewing, testing, or creating artifacts.")
	lines = append(lines, "4. If no matching skill exists, proceed without project skill injection and report `skill_resolution: none`.")
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

// maxSkillDirDepth bounds recursive descent under each root:
// <root>/<skill>/SKILL.md (depth 1) or <root>/<group>/<skill>/SKILL.md
// (depth 2). The bound also caps symlink-loop recursion.
const maxSkillDirDepth = 4

// noDescendDirs lists directory names that are never descended into while
// looking for a nested skill group. It is consulted only when deciding
// whether to descend past a non-leaf directory — it never suppresses a
// directory that is itself a leaf (contains its own SKILL.md).
var noDescendDirs = map[string]bool{
	"testdata":     true,
	"fixtures":     true,
	"examples":     true,
	"references":   true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	".git":         true,
}

// discoveredSkill is a SKILL.md file found under a scanned root, together
// with the namespace (slash-joined relative directory path) it was found
// under. Namespace is "" for depth-1 skills (<root>/<skill>/SKILL.md).
type discoveredSkill struct {
	Path      string
	Namespace string
}

// findAllSkillFiles scans each root for SKILL.md files up to
// maxSkillDirDepth levels deep, the Agent Skills layout plus one level of
// grouping (e.g. skills/agents/<name>/SKILL.md). The core rule is: a
// directory that contains SKILL.md is a leaf and is never descended into.
// This preserves symlinked skill directories (dotfiles/nix setups, which
// filepath.WalkDir silently skips — os.Stat is used at every level instead)
// and, by construction, keeps nested fixture/example SKILL.md files
// (skills/parent/examples/SKILL.md) from ever being reached, since
// skills/parent/SKILL.md already makes "parent" a leaf. noDescendDirs is an
// additional guard applied only when deciding to descend into a non-leaf
// directory (e.g. skills/agents/testdata/), so it never hides a real leaf.
func findAllSkillFiles(dirs []string) ([]discoveredSkill, error) {
	var out []discoveredSkill
	for _, root := range dirs {
		collectSkillFiles(root, root, 0, &out)
	}
	return out, nil
}

func collectSkillFiles(root, dir string, depth int, out *[]discoveredSkill) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		// dirExists/fileExists use os.Stat, so symlinked skill dirs and
		// symlinked SKILL.md files are resolved instead of skipped.
		child := filepath.Join(dir, entry.Name())
		if !dirExists(child) {
			continue
		}
		candidate := filepath.Join(child, "SKILL.md")
		if fileExists(candidate) {
			namespace := ""
			if rel, relErr := filepath.Rel(root, dir); relErr == nil && rel != "." {
				namespace = filepath.ToSlash(rel)
			}
			*out = append(*out, discoveredSkill{Path: candidate, Namespace: namespace})
			continue // leaf — never descend into a directory with its own SKILL.md
		}
		if depth+1 >= maxSkillDirDepth || noDescendDirs[entry.Name()] {
			continue
		}
		collectSkillFiles(root, child, depth+1, out)
	}
}

func pathsOf(discovered []discoveredSkill) []string {
	out := make([]string, len(discovered))
	for i, ds := range discovered {
		out[i] = ds.Path
	}
	return out
}

// namespacedEntry prefixes entry.Name with the skill's namespace
// (group/skill) when it was discovered at depth 2. Exclusion via
// isExcluded is already applied inside LoadSkill on the bare leaf name,
// before this prefix is added.
func namespacedEntry(entry SkillEntry, namespace string) SkillEntry {
	if namespace == "" {
		return entry
	}
	entry.Name = namespace + "/" + entry.Name
	return entry
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
		} else {
			value = strings.Trim(value, `"'`)
		}
		switch m[1] {
		case "name":
			name = value
		case "description":
			description = value
		}
	}
	return name, description
}

func dedupeBySkillName(entries []SkillEntry, cwd string) []SkillEntry {
	projectPrefix := filepath.Clean(cwd) + string(os.PathSeparator)
	buckets := map[string][]SkillEntry{}
	for _, entry := range entries {
		buckets[entry.Name] = append(buckets[entry.Name], entry)
	}
	out := make([]SkillEntry, 0, len(buckets))
	for _, list := range buckets {
		chosen := list[0]
		for _, entry := range list {
			if strings.HasPrefix(filepath.Clean(entry.Path), projectPrefix) {
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
	projectPrefix := filepath.Clean(cwd) + string(os.PathSeparator)
	if strings.HasPrefix(filepath.Clean(path), projectPrefix) {
		return "project"
	}
	return "user"
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

func readCachedFingerprint(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var cache cacheFile
	if err := json.Unmarshal(data, &cache); err != nil {
		return ""
	}
	return cache.Fingerprint
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
