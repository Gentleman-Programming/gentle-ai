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
	RegistryRelPath      = ".atl/skill-registry.md"
	CacheRelPath         = ".atl/.skill-registry.cache.json"
	RegistrySchema       = 3
	sectionMarker        = "## Selected skills and compact rules"
	atlIgnoreEntry       = ".atl/"
	fallbackCompactRules = "No compact rules declared; delegators should load the full skill file before direct work, or pass an explicit fallback path only when Project Standards cannot be injected."
)

var (
	excludeNames          = map[string]bool{"_shared": true, "skill-registry": true}
	excludePrefixes       = []string{"sdd-"}
	compactHeading        = regexp.MustCompile(`(?i)^##\s+Compact Rules\s*$`)
	h2Heading             = regexp.MustCompile(`^##\s+(.+?)\s*$`)
	nextH2                = regexp.MustCompile(`^##\s+`)
	bulletLine            = regexp.MustCompile(`^-\s+(.+)$`)
	orderedListLine       = regexp.MustCompile(`^\d+[.)]\s+(.+)$`)
	fallbackRuleHeadings  = []string{"Hard Rules", "Critical Rules", "Critical Patterns", "Voice Rules", "Decision Gates"}
	maxExtractedRuleCount = 15
	frontmatterLine       = regexp.MustCompile(`^(\w+):\s*(.*)$`)
)

type SkillEntry struct {
	Name        string
	Path        string
	Description string
	Rules       []string
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
	rules := extractCompactRules(body)
	if len(rules) == 0 {
		rules = []string{fallbackCompactRules}
	}
	return SkillEntry{Name: name, Path: file, Description: desc, Rules: rules}, true
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
	lines = append(lines, "**Delegator use only.** Any agent that launches subagents reads this registry to resolve compact rules, then injects matching rule text into subagent prompts under `## Project Standards (auto-resolved)`.", "")
	lines = append(lines, "Subagents still read their assigned executor/phase skill. During normal runtime, they do **not** independently discover or load additional project/user `SKILL.md` files or this registry; project/user rules arrive pre-digested. Explicit fallback loading is degraded self-healing and must be reported in `skill_resolution` as `fallback-registry` or `fallback-path`.", "")
	lines = append(lines, sectionMarker, "")
	for _, entry := range entries {
		lines = append(lines, "### "+entry.Name)
		lines = append(lines, "- Path: "+entry.Path)
		if strings.TrimSpace(entry.Description) != "" {
			lines = append(lines, "- Trigger: "+entry.Description)
		}
		lines = append(lines, "- Rules:")
		for _, rule := range entry.Rules {
			lines = append(lines, "  - "+rule)
		}
		lines = append(lines, "")
	}
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
	for _, line := range strings.Split(fm, "\n") {
		m := frontmatterLine.FindStringSubmatch(line)
		if len(m) != 3 {
			continue
		}
		value := strings.TrimSpace(m[2])
		value = strings.Trim(value, `"'`)
		switch m[1] {
		case "name":
			name = value
		case "description":
			description = value
		}
	}
	return name, description, body
}

func extractCompactRules(body string) []string {
	if rules := extractRulesFromHeadings(body, []string{"Compact Rules"}); len(rules) > 0 {
		return rules
	}
	return extractRulesFromHeadings(body, fallbackRuleHeadings)
}

func extractRulesFromHeadings(body string, headings []string) []string {
	wanted := map[string]bool{}
	for _, heading := range headings {
		wanted[normalizeHeading(heading)] = true
	}

	inSection := false
	var rules []string
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimRight(raw, " \t")
		if m := h2Heading.FindStringSubmatch(line); len(m) == 2 {
			inSection = wanted[normalizeHeading(m[1])]
			continue
		}
		if !inSection {
			continue
		}
		if nextH2.MatchString(line) {
			inSection = false
			continue
		}
		if rule, ok := extractRuleLine(line); ok {
			rules = append(rules, rule)
			if len(rules) >= maxExtractedRuleCount {
				return rules
			}
		}
	}
	return rules
}

func extractRuleLine(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", false
	}
	if m := bulletLine.FindStringSubmatch(trimmed); len(m) == 2 {
		return strings.TrimSpace(m[1]), true
	}
	if m := orderedListLine.FindStringSubmatch(trimmed); len(m) == 2 {
		return strings.TrimSpace(m[1]), true
	}
	if strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|") {
		return extractRuleTableRow(trimmed)
	}
	return "", false
}

func extractRuleTableRow(line string) (string, bool) {
	inner := strings.Trim(line, "|")
	cells := strings.Split(inner, "|")
	if len(cells) < 2 {
		return "", false
	}
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	if isTableSeparator(cells) || isTableHeader(cells) || cells[0] == "" || cells[1] == "" {
		return "", false
	}
	return cells[0] + ": " + cells[1], true
}

func isTableSeparator(cells []string) bool {
	for _, cell := range cells {
		trimmed := strings.Trim(cell, " -:")
		if trimmed != "" {
			return false
		}
	}
	return true
}

func isTableHeader(cells []string) bool {
	if len(cells) < 2 {
		return false
	}
	first := normalizeHeading(cells[0])
	second := normalizeHeading(cells[1])
	return (first == "rule" && second == "requirement") || (first == "target" && second == "test pattern")
}

func normalizeHeading(heading string) string {
	return strings.ToLower(strings.TrimSpace(heading))
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
