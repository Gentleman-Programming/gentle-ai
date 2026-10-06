package legacyassets

//go:generate go run ../../../scripts/gen-sdd-agent-digests sdd_agent_digests.go opencode_sdd_digests.go

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"gopkg.in/yaml.v3"
)

// SDDAgentFamilies maps each runtime that received native SDD sub-agents to
// the embedded asset directory (internal/assets/<family>/agents) releases
// rendered them from. Every release installed them in adapter.SubAgentsDir.
var SDDAgentFamilies = map[model.AgentID]string{
	model.AgentClaudeCode: "claude",
	model.AgentKiroIDE:    "kiro",
	model.AgentCursor:     "cursor",
	model.AgentKimi:       "kimi",
}

var (
	managedBlockOpen = regexp.MustCompile(`<!-- gentle-ai:([a-z0-9-]+) -->`)
	engramToolPair   = regexp.MustCompile(`mcp__engram__([A-Za-z0-9_]+), mcp__plugin_engram_engram__([A-Za-z0-9_]+)`)
	engramToolSingle = regexp.MustCompile(`(?:mcp__plugin_engram_engram__|mcp__engram__|\{\{ENGRAM_TOOL_PREFIX\}\})([A-Za-z0-9_]+)`)
)

// NormalizeSDDAgent maps a released template and every render of it to the
// same bytes, so ownership compares only content a user could have authored.
// It erases exactly what the SDD renderer substituted or appended:
//   - CRLF line endings become LF;
//   - complete `<!-- gentle-ai:ID -->` ... `<!-- /gentle-ai:ID -->` blocks
//     (CodeGraph guidance, language contract, remote authorization);
//   - in the frontmatter, the `model:` value, `effort:` lines and the
//     {{CLAUDE_EFFORT_FRONTMATTER}} placeholder, the Engram tool prefix
//     expansion, and the CodeGraph tool grant;
//   - trailing whitespace at the end of the document.
//
// Everything else, including description, tools, body, and unmatched
// markers, must equal a released template byte for byte.
func NormalizeSDDAgent(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = removeManagedBlocks(content)
	if body, ok := strings.CutPrefix(content, "---\n"); ok {
		if end := strings.Index(body, "\n---\n"); end >= 0 {
			lines := strings.Split(body[:end], "\n")
			kept := lines[:0]
			for _, line := range lines {
				switch {
				case line == "{{CLAUDE_EFFORT_FRONTMATTER}}", strings.HasPrefix(line, "effort:"):
					continue
				case strings.HasPrefix(line, "model:"):
					line = "model: {{MODEL}}"
				case strings.HasPrefix(line, "tools:"):
					line = engramToolPair.ReplaceAllStringFunc(line, func(pair string) string {
						match := engramToolPair.FindStringSubmatch(pair)
						if match[1] != match[2] {
							return pair
						}
						return "{{ENGRAM}}" + match[1]
					})
					line = engramToolSingle.ReplaceAllString(line, "{{ENGRAM}}$1")
					line = strings.Replace(line, ", mcp__codegraph__codegraph_explore", "", 1)
					line = strings.Replace(line, `, "@codegraph"]`, "]", 1)
				}
				kept = append(kept, line)
			}
			content = "---\n" + strings.Join(kept, "\n") + body[end:]
		}
	}
	return strings.TrimRight(content, " \t\n") + "\n"
}

func removeManagedBlocks(content string) string {
	for search := 0; ; {
		loc := managedBlockOpen.FindStringSubmatchIndex(content[search:])
		if loc == nil {
			return content
		}
		start := search + loc[0]
		closing := "<!-- /gentle-ai:" + content[search+loc[2]:search+loc[3]] + " -->"
		end := strings.Index(content[search+loc[1]:], closing)
		if end < 0 {
			search += loc[1]
			continue
		}
		end += search + loc[1] + len(closing)
		content = strings.TrimRight(content[:start], " \t\n") + "\n" + strings.TrimLeft(content[end:], "\n")
		search = 0
	}
}

// SDDAgentDigest is the registry key of content: SHA-256 of its normalization.
func SDDAgentDigest(content string) string {
	sum := sha256.Sum256([]byte(NormalizeSDDAgent(content)))
	return hex.EncodeToString(sum[:])
}

// RetiredSDDAgentFiles lists every native SDD sub-agent file name a release
// installed for agent, sorted. The shipped-bytes registry is the inventory.
func RetiredSDDAgentFiles(agent model.AgentID) []string {
	family, ok := SDDAgentFamilies[agent]
	if !ok {
		return nil
	}
	var names []string
	for key := range releasedSDDAgentDigests {
		if name, ok := strings.CutPrefix(key, family+"/"); ok {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// RetiredSDDAgentPaths joins RetiredSDDAgentFiles with dir for snapshots.
func RetiredSDDAgentPaths(agent model.AgentID, dir string) []string {
	if dir == "" {
		return nil
	}
	names := RetiredSDDAgentFiles(agent)
	paths := make([]string, 0, len(names))
	for _, name := range names {
		paths = append(paths, filepath.Join(dir, name))
	}
	return paths
}

// OwnsRetiredSDDAgent reports whether content is a render of a template some
// release shipped for this agent family and file name.
func OwnsRetiredSDDAgent(agent model.AgentID, name string, content []byte) bool {
	family, ok := SDDAgentFamilies[agent]
	if !ok || len(content) == 0 {
		return false
	}
	return slices.Contains(releasedSDDAgentDigests[family+"/"+name], SDDAgentDigest(string(content)))
}

// OwnsPreLedgerNativeAgent reports whether content is a retained native agent
// file exactly as a release before the v4.0.0 ownership ledger shipped it.
// Kimi's v3 gentleman.yaml is the only such file: it declared every SDD
// subagent by path, and without this proof a v3-inherited copy could never
// be rewritten, so its SDD subagents could never be retired.
func OwnsPreLedgerNativeAgent(agent model.AgentID, name string, content []byte) bool {
	family, ok := SDDAgentFamilies[agent]
	if !ok || len(content) == 0 {
		return false
	}
	return slices.Contains(releasedPreLedgerNativeAgentDigests[family+"/"+name], SDDAgentDigest(string(content)))
}

// PreserveReason says why retirement kept an inventory file.
type PreserveReason int

const (
	// PreservedUnproven: no release wrote these bytes, or it is not a
	// regular file, so it may hold the user's changes.
	PreservedUnproven PreserveReason = iota + 1
	// PreservedPairUnproven: the file is a release's bytes, but the other
	// half of its Kimi pair is unproven, so removing it would break the pair.
	PreservedPairUnproven
)

// PreservedSDDAgent is one inventory file retirement kept, with its reason.
type PreservedSDDAgent struct {
	Path   string
	Reason PreserveReason
}

// SDDAgentReference is an agent file in the same directory that still
// references owned retired SDD agent files, which were kept so it loads.
type SDDAgentReference struct {
	Parent string
	Agents []string
}

// RetireResult lists the files a retirement removed, the inventory files it
// preserved because ownership could not be proven, and the owned files it kept
// because another agent still references them.
type RetireResult struct {
	Removed    []string
	Preserved  []PreservedSDDAgent
	Referenced []SDDAgentReference
}

// RetireSDDAgents removes the retired SDD sub-agents in dir that Gentle AI
// rendered. Files sharing a stem (Kimi's YAML and its prompt) are one agent:
// it is removed only when every present file is owned. Symlinks, non-regular
// files, and bytes no release rendered are preserved and reported. A file any
// remaining YAML agent in dir still references is never removed, whoever owns
// that agent, so retirement never leaves a dangling subagent path. Callers
// rewrite Gentle-owned parents (Kimi's gentleman.yaml) first so the cleanup
// completes in one run.
func RetireSDDAgents(agent model.AgentID, dir string) (RetireResult, error) {
	var result RetireResult
	// A symlinked agents directory (dotfiles) is followed like every writer
	// follows it; only the agent files themselves must be regular files.
	if info, err := os.Stat(dir); os.IsNotExist(err) {
		return result, nil
	} else if err != nil {
		return result, fmt.Errorf("inspect agents directory %s: %w", dir, err)
	} else if !info.IsDir() {
		return result, nil
	}
	groups := map[string][]string{}
	var stems []string
	for _, name := range RetiredSDDAgentFiles(agent) {
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		if _, seen := groups[stem]; !seen {
			stems = append(stems, stem)
		}
		groups[stem] = append(groups[stem], name)
	}
	slices.Sort(stems)
	present := map[string][]string{}
	removable := map[string]bool{}
	for _, stem := range stems {
		var unproven, owned []string
		for _, name := range groups[stem] {
			path := filepath.Join(dir, name)
			info, err := os.Lstat(path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return result, fmt.Errorf("inspect retired SDD agent %s: %w", path, err)
			}
			present[stem] = append(present[stem], path)
			if !info.Mode().IsRegular() {
				unproven = append(unproven, path)
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return result, fmt.Errorf("read retired SDD agent %s: %w", path, err)
			}
			if OwnsRetiredSDDAgent(agent, name, data) {
				owned = append(owned, path)
			} else {
				unproven = append(unproven, path)
			}
		}
		if len(present[stem]) == 0 {
			continue
		}
		if len(unproven) == 0 {
			removable[stem] = true
			continue
		}
		for _, path := range unproven {
			result.Preserved = append(result.Preserved, PreservedSDDAgent{Path: path, Reason: PreservedUnproven})
		}
		for _, path := range owned {
			result.Preserved = append(result.Preserved, PreservedSDDAgent{Path: path, Reason: PreservedPairUnproven})
		}
	}
	referenced, err := referencedSDDAgents(dir, present, removable)
	if err != nil {
		return result, err
	}
	result.Referenced = referenced
	for _, stem := range stems {
		if !removable[stem] {
			continue
		}
		for _, path := range present[stem] {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return result, fmt.Errorf("remove retired SDD agent %s: %w", path, err)
			}
			result.Removed = append(result.Removed, path)
		}
	}
	slices.SortFunc(result.Preserved, func(a, b PreservedSDDAgent) int { return strings.Compare(a.Path, b.Path) })
	return result, nil
}

// referencedSDDAgents withdraws from removable every stem a YAML agent that
// will remain in dir references, repeating until no kept agent references a
// removable one. It returns the references grouped by parent file.
func referencedSDDAgents(dir string, present map[string][]string, removable map[string]bool) ([]SDDAgentReference, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("list agents directory %s: %w", dir, err)
	}
	stemOf := map[string]string{}
	for stem, paths := range present {
		for _, path := range paths {
			stemOf[path] = stem
		}
	}
	held := map[string]map[string]bool{}
	for changed := true; changed; {
		changed = false
		for _, entry := range entries {
			ext := filepath.Ext(entry.Name())
			parent := filepath.Join(dir, entry.Name())
			if (ext != ".yaml" && ext != ".yml") || removable[stemOf[parent]] {
				continue
			}
			if info, err := os.Stat(parent); err != nil || !info.Mode().IsRegular() {
				continue
			}
			data, err := os.ReadFile(parent)
			if err != nil {
				return nil, fmt.Errorf("read agent %s: %w", parent, err)
			}
			for _, path := range referencedPaths(dir, data, stemOf) {
				stem := stemOf[path]
				if !removable[stem] || stem == stemOf[parent] {
					continue
				}
				delete(removable, stem)
				if held[parent] == nil {
					held[parent] = map[string]bool{}
				}
				held[parent][stem] = true
				changed = true
			}
		}
	}
	var references []SDDAgentReference
	for parent, stems := range held {
		reference := SDDAgentReference{Parent: parent}
		for stem := range stems {
			reference.Agents = append(reference.Agents, present[stem]...)
		}
		slices.Sort(reference.Agents)
		references = append(references, reference)
	}
	slices.SortFunc(references, func(a, b SDDAgentReference) int { return strings.Compare(a.Parent, b.Parent) })
	return references, nil
}

// referencedPaths returns the inventory files a YAML agent references: any
// string value that resolves, relative to dir, to one of them. Unparseable
// YAML references every inventory file whose name it mentions, since keeping
// a file is the safe failure.
func referencedPaths(dir string, data []byte, inventory map[string]string) []string {
	var found []string
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		for path := range inventory {
			if strings.Contains(string(data), filepath.Base(path)) {
				found = append(found, path)
			}
		}
		return found
	}
	var walk func(*yaml.Node)
	walk = func(node *yaml.Node) {
		if node.Kind == yaml.ScalarNode {
			value := strings.TrimSpace(node.Value)
			if value == "" {
				return
			}
			path := filepath.Clean(value)
			if !filepath.IsAbs(path) {
				path = filepath.Join(dir, path)
			}
			if _, ok := inventory[path]; ok {
				found = append(found, path)
			}
		}
		for _, child := range node.Content {
			walk(child)
		}
	}
	walk(&doc)
	return found
}

// ManualActions tells the user what to do with every file retirement kept.
func (r RetireResult) ManualActions() []string {
	actions := make([]string, 0, len(r.Preserved)+len(r.Referenced))
	for _, preserved := range r.Preserved {
		switch preserved.Reason {
		case PreservedPairUnproven:
			actions = append(actions, fmt.Sprintf("Retired SDD agent %s was kept because its pair is user-edited: this file is exactly what a Gentle AI release installed, but removing it alone would break the agent its edited pair defines. SDD was retired in v4.0.0; if you no longer need that agent, move or delete both files.", preserved.Path))
		default:
			actions = append(actions, fmt.Sprintf("Retired SDD agent %s was preserved: Gentle AI cannot prove it wrote this file (its content differs from every released version, or it is not a regular file), so it may contain your changes. SDD was retired in v4.0.0 and this agent is no longer maintained; if you no longer need it, move or delete it.", preserved.Path))
		}
	}
	for _, reference := range r.Referenced {
		actions = append(actions, fmt.Sprintf("%s still references retired SDD agent files %s, so they were kept to avoid breaking it. SDD was retired in v4.0.0: remove those subagent entries from it (or move or delete it), then rerun `gentle-ai sync` to retire them.", reference.Parent, strings.Join(reference.Agents, ", ")))
	}
	return actions
}
