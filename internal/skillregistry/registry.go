package skillregistry

import (
	"crypto/sha1"
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

	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v3/internal/workspace"
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

type SkillEntry struct {
	Name        string
	Path        string
	Description string
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
	dirs := []string{
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

	// Multirrepo / Monorrepo desacoplado: consultar axiom.yaml o .axiom-workspace
	if cfg, err := workspace.LoadConfig(cwd); err == nil && cfg != nil {
		specsRepo := strings.TrimSpace(cfg.Workspace.SpecsRepository)
		if specsRepo != "" && specsRepo != "." {
			if filepath.IsAbs(specsRepo) {
				dirs = append(dirs, filepath.Join(specsRepo, "skills"))
			} else {
				dirs = append(dirs, filepath.Join(cwd, specsRepo, "skills"))
			}
		}
		for _, role := range cfg.Roles {
			for _, repo := range role.Repositories {
				repoPath := strings.TrimSpace(repo.Path)
				if repoPath != "" && repoPath != "." {
					if filepath.IsAbs(repoPath) {
						dirs = append(dirs, filepath.Join(repoPath, "skills"))
					} else {
						dirs = append(dirs, filepath.Join(cwd, repoPath, "skills"))
					}
				}
			}
		}
	}

	return dirs
}

// Regenerate runs the single scan and writes the unified skills index to its
// destinations (REQ-22.11, D-09), in this order:
//
//  1. .atl/skill-registry.md (fatal)
//  2. .atl/.skill-registry.cache.json (fatal)
//  3. the managed `## Skills` section of AGENTS.md (fatal when the file exists;
//     omitted when it does not — spec §3.6)
//  4. the Engram `skill-registry` topic through opts.Mirror (never fatal:
//     `mirror failed` with exit 0 for the caller — REQ-22.11)
//
// The exclusion filter is applied once, inside the scan (LoadSkill), upstream
// of every destination. When the fingerprint matches and Force is false, no
// destination is written and all three stay byte-identical (Reason ==
// "cache-hit"). A cwd that does not resolve to an existing directory fails
// before any write (T-1 path containment).
// cleanPathArg normalizes a user-supplied path argument (--cwd and the home
// root) before any filesystem work. Windows accepts both separators in one
// path while filepath.Clean only applies the *host* separator rules, so on a
// non-Windows host a backslash-separated argument would resolve to a
// completely different location. Fold the separators first so the argument
// resolves identically on every host; --cwd is user input, never a literal
// filename discovered on disk.
func cleanPathArg(path string) string {
	return filepath.Clean(strings.ReplaceAll(path, "\\", "/"))
}

func Regenerate(cwd, home string, opts RegenerateOptions) (Result, error) {
	cwd = cleanPathArg(cwd)
	home = filepath.Clean(home)
	if !dirExists(cwd) {
		return Result{}, fmt.Errorf("workspace root does not exist: %s", cwd)
	}

	existingDirs := uniqueExistingDirs(append(ProjectSkillDirs(cwd), UserSkillDirs(home)...))
	files, err := findAllSkillFiles(existingDirs)
	if err != nil {
		return Result{}, err
	}

	registryPath := filepath.Join(cwd, RegistryRelPath)
	cachePath := filepath.Join(cwd, CacheRelPath)
	agentsPath := filepath.Join(cwd, AgentsRelPath)
	fp := Fingerprint(files)
	cached := readCachedFingerprint(cachePath)
	if !opts.Force && cached == fp && fileExists(registryPath) {
		return Result{
			Regenerated: false,
			Reason:      "cache-hit",
			Registry:    registryPath,
			Cache:       cachePath,
			Agents:      agentsCacheOutcome(agentsPath),
			// Mirror stays zero: no write was attempted (T-8).
		}, nil
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

	agentsOutcome, agentsErr := writeAgentsIndex(agentsPath, cwd, entries)
	result := Result{
		Regenerated: true,
		SkillCount:  len(entries),
		Reason:      "fingerprint-changed",
		Registry:    registryPath,
		Cache:       cachePath,
		Agents:      agentsOutcome,
	}
	if opts.Force {
		result.Reason = "forced"
	}
	if agentsErr != nil {
		// Fatal destination: earlier destinations already wrote ("partial
		// already emitted"); the error names the destination (spec §1.1).
		return result, agentsErr
	}
	result.Mirror = runMirror(opts.Mirror, cwd, entries)
	return result, nil
}

// runMirror attempts the Engram mirror and reports it as data: a mirror
// failure is never fatal (REQ-22.11). A nil MirrorFunc reports MirrorFailed
// with "mirror not configured" — the command must never claim `mirror ok` for
// a write it never attempted (T-8).
func runMirror(mirror MirrorFunc, cwd string, entries []SkillEntry) MirrorOutcome {
	if mirror == nil {
		return MirrorOutcome{Status: MirrorFailed, Err: errors.New("mirror not configured")}
	}
	req := MirrorRequest{
		TopicKey:      MirrorTopicKey,
		Type:          MirrorType,
		Title:         "Skill registry — " + filepath.Base(cwd),
		Content:       mirrorContent(cwd, entries),
		CapturePrompt: false,
	}
	if err := mirror(req); err != nil {
		return MirrorOutcome{Status: MirrorFailed, Err: err}
	}
	return MirrorOutcome{Status: MirrorOK}
}

// List resolves the deduplicated, sorted set of skills that Regenerate would
// index, without writing the registry, cache, or .gitignore. It is the
// read-only inspection path behind `gentle-ai skill-registry list`.
func List(cwd, home string) []SkillEntry {
	cwd = cleanPathArg(cwd)
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
	lines = append(lines, "<!-- Auto-generated by axiom skill-registry refresh. Run `axiom skill-registry refresh --force` to regenerate. -->", "")
	lines = append(lines, "Last updated: "+time.Now().UTC().Format("2006-01-02"), "")
	lines = append(lines, "## Sources scanned", "")
	for _, src := range sources {
		lines = append(lines, "- "+src)
	}
	lines = append(lines, "", "## Contract", "")
	lines = append(lines, "**Delegator use only.** This registry is an index, not a summary. Any agent that launches subagents reads it to select relevant skills, then passes exact `SKILL.md` paths for the subagent to read before work.", "")
	lines = append(lines, "`SKILL.md` remains the source of truth. Do not inject generated summaries or compact rules by default; pass paths so subagents load the full runtime contract and preserve author intent.", "")
	lines = append(lines, sectionMarker, "")
	lines = append(lines, renderSkillsTable(cwd, entries, PathDiscovered))
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
