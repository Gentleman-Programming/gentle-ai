package skillregistry

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
)

const (
	RegistryRelPath = ".atl/skill-registry.md"
	CacheRelPath    = ".atl/.skill-registry.cache.json"
	RegistrySchema  = 5
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

// SkillEntry describes a discovered skill with its name, location, and description.
type SkillEntry struct {
	Name        string
	Path        string
	Description string
}

// Result reports the outcome of a skill registry generation or load operation.
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
		filepath.Join(home, ".kimi-code", "skills"),
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
		filepath.Join(home, ".trae", "skills"),
	}
}

// ProjectSkillDirs returns the standard project-level directories searched for skills.
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

// Regenerate scans project and user skill directories, regenerates the registry
// markdown if the fingerprint changed (or if forced), and writes the cache file.
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

	wasLoadedDrifted := false
	if strings.HasPrefix(cached, "loaded:") {
		registryBytes, readErr := os.ReadFile(registryPath)
		if readErr == nil {
			sha256Sum := fmt.Sprintf("loaded:%x", sha256.Sum256(registryBytes))
			sha1Sum := fmt.Sprintf("loaded:%x", sha1.Sum(registryBytes))
			if !force && (cached == sha256Sum || cached == sha1Sum) {
				// Migrate legacy sha1 fingerprint to sha256 if needed
				if cached == sha1Sum && cached != sha256Sum {
					cacheBytes, err := json.MarshalIndent(cacheFile{Fingerprint: sha256Sum}, "", "  ")
					if err == nil {
						cacheBytes = append(cacheBytes, '\n')
						_, _ = filemerge.WriteFileAtomic(cachePath, cacheBytes, 0o644)
					}
				}
				return Result{
					Regenerated: false,
					SkillCount:  countRegistrySkills(string(registryBytes)),
					Reason:      "manually-loaded",
					Registry:    registryPath,
					Cache:       cachePath,
				}, nil
			}
		}
		wasLoadedDrifted = true
	} else if !force && cached == fp && fileExists(registryPath) {
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
	cacheBytes, err := json.MarshalIndent(cacheFile{Fingerprint: fp}, "", "  ")
	if err != nil {
		return Result{}, err
	}
	cacheBytes = append(cacheBytes, '\n')
	if err := commitRegistryPair(registryPath, []byte(md), cachePath, cacheBytes); err != nil {
		return Result{}, err
	}

	reason := "fingerprint-changed"
	if wasLoadedDrifted {
		reason = "loaded-drifted"
	}
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
	files, err := findAllSkillFiles(existingDirs)
	if err != nil {
		return nil
	}
	entries := make([]SkillEntry, 0, len(files))
	for _, file := range files {
		if entry, ok := LoadSkill(file); ok {
			entries = append(entries, entry)
		}
	}
	return dedupeBySkillName(entries, cwd)
}

// EnsureATLIgnored appends the .atl/ entry to .gitignore in cwd if not already present.
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

// Fingerprint computes a deterministic hash of all skill file paths, timestamps, sizes, and contents.
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

// LoadSkill reads a SKILL.md file and extracts its name and description frontmatter.
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

// RenderRegistry formats the skill registry markdown document containing scanned sources and skill tables.
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

// findAllSkillFiles scans each root exactly one level deep for
// <root>/<skill>/SKILL.md, the Agent Skills layout. A single-level scan is
// deliberate: it follows symlinked skill directories (dotfiles/nix setups,
// which filepath.WalkDir silently skips) and never indexes nested fixture or
// example SKILL.md files that would otherwise pollute the registry with
// phantom entries named after their parent directory.
func findAllSkillFiles(dirs []string) ([]string, error) {
	var out []string
	for _, root := range dirs {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			// dirExists/fileExists use os.Stat, so symlinked skill dirs and
			// symlinked SKILL.md files are resolved instead of skipped.
			skillDir := filepath.Join(root, entry.Name())
			if !dirExists(skillDir) {
				continue
			}
			candidate := filepath.Join(skillDir, "SKILL.md")
			if fileExists(candidate) {
				out = append(out, candidate)
			}
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

// hasRegistryMarkers reports whether content contains the mandatory markdown
// headers and table structure of a valid skill registry.
func hasRegistryMarkers(content string) bool {
	return strings.Contains(content, "# Skill Registry") &&
		strings.Contains(content, "## Skills") &&
		strings.Contains(content, "| Skill | Trigger / description | Scope | Path |") &&
		strings.Contains(content, "| --- | --- | --- | --- |")
}

// countRegistrySkills counts the skill rows in the registry's markdown table.
func countRegistrySkills(content string) int {
	idx := strings.Index(content, "## Skills")
	if idx == -1 {
		return 0
	}
	sub := content[idx:]
	headerIdx := strings.Index(sub, "| Skill | Trigger / description | Scope | Path |")
	if headerIdx == -1 {
		return 0
	}
	tableSub := sub[headerIdx:]
	lines := strings.Split(tableSub, "\n")
	count := 0
	inTable := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "| ---") {
			inTable = true
			continue
		}
		if inTable {
			if !strings.HasPrefix(trimmed, "|") {
				if count > 0 || trimmed != "" {
					break
				}
				continue
			}
			count++
		}
	}
	return count
}

// commitRegistryPair writes the registry markdown and its cache metadata file
// atomically, rolling back any partial changes if either write fails.
func commitRegistryPair(registryPath string, registryBytes []byte, cachePath string, cacheBytes []byte) error {
	var prevRegistry, prevCache []byte
	regExisted := false
	cacheExisted := false

	if data, err := os.ReadFile(registryPath); err == nil {
		prevRegistry = data
		regExisted = true
	}
	if data, err := os.ReadFile(cachePath); err == nil {
		prevCache = data
		cacheExisted = true
	}

	if _, err := filemerge.WriteFileAtomic(registryPath, registryBytes, 0o644); err != nil {
		return fmt.Errorf("write registry: %w", err)
	}

	cacheResult, cacheErr := filemerge.WriteFileAtomic(cachePath, cacheBytes, 0o644)
	if cacheErr != nil {
		writeErr := fmt.Errorf("write registry cache: %w", cacheErr)
		var regRollbackErr error
		if regExisted {
			if _, err := filemerge.WriteFileAtomic(registryPath, prevRegistry, 0o644); err != nil {
				regRollbackErr = fmt.Errorf("rollback registry: %w", err)
			}
		} else {
			if err := os.Remove(registryPath); err != nil && !os.IsNotExist(err) {
				regRollbackErr = fmt.Errorf("rollback registry remove: %w", err)
			}
		}

		var cacheRollbackErr error
		if cacheResult.Changed {
			if cacheExisted {
				if _, err := filemerge.WriteFileAtomic(cachePath, prevCache, 0o644); err != nil {
					cacheRollbackErr = fmt.Errorf("rollback registry cache: %w", err)
				}
			} else {
				if err := os.Remove(cachePath); err != nil && !os.IsNotExist(err) {
					cacheRollbackErr = fmt.Errorf("rollback registry cache remove: %w", err)
				}
			}
		}

		return errors.Join(writeErr, regRollbackErr, cacheRollbackErr)
	}

	return nil
}

// PreparedLoad represents a validated curated skill registry ready to be
// committed to the project's .atl directory.
type PreparedLoad struct {
	LoadPath     string
	Cwd          string
	RegistryPath string
	CachePath    string
	Content      []byte
	Fingerprint  string
	SkillCount   int
}

// PrepareLoadRegistry validates and parses a curated skill registry file without
// modifying the project filesystem, gitignore, or registry cache. It returns a
// PreparedLoad that callers can inspect and subsequently commit.
func PrepareLoadRegistry(loadPath, cwd string) (PreparedLoad, error) {
	if strings.TrimSpace(loadPath) == "" {
		return PreparedLoad{}, fmt.Errorf("load path cannot be empty")
	}
	if strings.TrimSpace(cwd) == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return PreparedLoad{}, fmt.Errorf("resolve cwd: %w", err)
		}
	}
	cwd = filepath.Clean(cwd)

	cleanLoadPath := filepath.Clean(loadPath)
	data, err := os.ReadFile(cleanLoadPath)
	if err != nil && !filepath.IsAbs(cleanLoadPath) {
		if dataCwd, errCwd := os.ReadFile(filepath.Join(cwd, cleanLoadPath)); errCwd == nil {
			data = dataCwd
			err = nil
			cleanLoadPath = filepath.Join(cwd, cleanLoadPath)
		}
	}
	if err != nil {
		return PreparedLoad{}, fmt.Errorf("read curated registry %q: %w", loadPath, err)
	}

	contentStr := string(data)
	if !hasRegistryMarkers(contentStr) {
		return PreparedLoad{}, fmt.Errorf("invalid skill registry format: %s missing required table or section markers", loadPath)
	}

	sum := sha256.Sum256(data)
	fp := fmt.Sprintf("loaded:%x", sum)
	count := countRegistrySkills(contentStr)

	return PreparedLoad{
		LoadPath:     cleanLoadPath,
		Cwd:          cwd,
		RegistryPath: filepath.Join(cwd, RegistryRelPath),
		CachePath:    filepath.Join(cwd, CacheRelPath),
		Content:      data,
		Fingerprint:  fp,
		SkillCount:   count,
	}, nil
}

// Commit writes the prepared registry and its cache metadata to the project's
// .atl directory. When force is false and the destination matches curated content
// with a valid cache fingerprint, it reports a cache-hit without rewriting.
// If the destination has drifted or force is true, it restores the curated bytes.
func (p PreparedLoad) Commit(force bool) (Result, error) {
	if err := os.MkdirAll(filepath.Dir(p.RegistryPath), 0o755); err != nil {
		return Result{}, fmt.Errorf("create .atl directory: %w", err)
	}

	cached := readCachedFingerprint(p.CachePath)
	if !force && cached == p.Fingerprint {
		current, readErr := os.ReadFile(p.RegistryPath)
		if readErr == nil && string(current) == string(p.Content) {
			return Result{
				Regenerated: false,
				SkillCount:  p.SkillCount,
				Reason:      "cache-hit",
				Registry:    p.RegistryPath,
				Cache:       p.CachePath,
			}, nil
		}
	}

	cacheBytes, err := json.MarshalIndent(cacheFile{Fingerprint: p.Fingerprint}, "", "  ")
	if err != nil {
		return Result{}, err
	}
	cacheBytes = append(cacheBytes, '\n')

	if err := commitRegistryPair(p.RegistryPath, p.Content, p.CachePath, cacheBytes); err != nil {
		return Result{}, err
	}

	reason := "loaded"
	if force {
		reason = "forced"
	}
	return Result{
		Regenerated: true,
		SkillCount:  p.SkillCount,
		Reason:      reason,
		Registry:    p.RegistryPath,
		Cache:       p.CachePath,
	}, nil
}
