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

	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
)

const (
	RegistryRelPath = ".atl/skill-registry.md"
	CacheRelPath    = ".atl/.skill-registry.cache.json"
	RegistrySchema  = 4
	sectionMarker   = "## Skills"
	atlIgnoreEntry  = ".atl/"
)

var (
	excludeNames    = map[string]bool{"_shared": true, "skill-registry": true}
	excludePrefixes = []string{"sdd-"}
	frontmatterLine = regexp.MustCompile(`^(\w+):\s*(.*)$`)
)

type SkillEntry struct {
	Name        string
	Path        string
	Description string
	Scope       string
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
	files, err := findAllSkillFiles(existingDirs)
	if err != nil {
		return Result{}, err
	}

	registryPath := filepath.Join(cwd, RegistryRelPath)
	cachePath := filepath.Join(cwd, CacheRelPath)
	fp := Fingerprint(files)
	cached := readCachedFingerprint(cachePath)
	if !force && cached == fp && fileExists(registryPath) {
		return Result{Regenerated: false, Reason: "cache-hit", Registry: registryPath, Cache: cachePath}, nil
	}

	entries := make([]SkillEntry, 0, len(files))
	for _, file := range files {
		entry, ok := LoadSkill(file)
		if ok {
			entries = append(entries, entry)
		}
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
	return os.WriteFile(gitignorePath, []byte(existing+prefix+header+atlIgnoreEntry+"\n"), 0o644)
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
		lines = append(lines, fmt.Sprintf("%s:%d:%d", file, info.ModTime().UnixNano(), info.Size()))
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
	name, desc, body := parseFrontmatter(string(data))
	if strings.TrimSpace(name) == "" {
		name = filepath.Base(filepath.Dir(file))
	}
	if isExcluded(name) {
		return SkillEntry{}, false
	}
	_ = body
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
		scope := entry.Scope
		if scope == "" {
			scope = scopeForPath(cwd, entry.Path)
		}
		lines = append(lines, fmt.Sprintf("| `%s` | %s | %s | `%s` |", markdownCell(entry.Name), markdownCell(entry.Description), markdownCell(scope), markdownCell(entry.Path)))
	}
	lines = append(lines, "", "## Loading protocol", "")
	lines = append(lines, "1. Match task context and target files against the `Trigger / description` column.")
	lines = append(lines, "2. Pass only the matching `Path` values to the subagent under `## Skills to load before work`.")
	lines = append(lines, "3. Instruct the subagent to read those exact `SKILL.md` files before reading, writing, reviewing, testing, or creating artifacts.")
	lines = append(lines, "4. If no matching skill exists, proceed without project skill injection and report `skill_resolution: none`.")
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

func findAllSkillFiles(dirs []string) ([]string, error) {
	var out []string
	for _, root := range dirs {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() && d.Name() == "SKILL.md" {
				out = append(out, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
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

func parseFrontmatter(source string) (name, description, body string) {
	source = strings.ReplaceAll(source, "\r\n", "\n")
	source = strings.ReplaceAll(source, "\r", "\n")
	if !strings.HasPrefix(source, "---\n") {
		return "", "", source
	}
	end := strings.Index(source[4:], "\n---")
	if end == -1 {
		return "", "", source
	}
	end += 4
	fm := source[4:end]
	body = strings.TrimPrefix(source[end+4:], "\n")
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
	return name, description, body
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

func scopeForPath(cwd, path string) string {
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
