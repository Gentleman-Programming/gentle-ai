package legacyassets

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
)

// The OpenCode family (OpenCode, Kilocode) received the retired SDD agents as
// settings entries, not files: sdd-orchestrator and sdd-<phase>, each also
// suffixed with a profile name (sdd-apply-fallback). Releases wrote them in
// two shapes: the V1 `agent` map and the native `agents` map 3.7.0 wrote when
// it detected a native config, which a V1 host refuses to load (#5182).

// openCodeSDDMarker is the ownership marker v2.7.0 through v3.7.0 wrote on
// every entry of the SDD overlay, in both shapes.
const openCodeSDDMarker = "gentle-ai/sdd"

const (
	openCodeSDDOrchestrator = "sdd-orchestrator"
	profilePlaceholder      = "{{PROFILE}}"
)

var (
	profileName      = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)
	promptFileRef    = regexp.MustCompile(`^\{file:(.+)\}$`)
	permissionEffect = map[string]bool{"allow": true, "ask": true, "deny": true}
)

// NormalizeOpenCodeSDDPrompt maps a released prompt and every render of it to
// the same text: CRLF line endings become LF, the complete managed blocks the
// renderer appended (CodeGraph guidance, language contract, remote
// authorization) are erased, and surrounding whitespace is trimmed.
func NormalizeOpenCodeSDDPrompt(content string) string {
	content = removeManagedBlocks(strings.ReplaceAll(content, "\r\n", "\n"))
	return strings.TrimSpace(content) + "\n"
}

// OpenCodeSDDPromptDigest is the registry key of a prompt: SHA-256 of its
// normalization.
func OpenCodeSDDPromptDigest(content string) string {
	sum := sha256.Sum256([]byte(NormalizeOpenCodeSDDPrompt(content)))
	return hex.EncodeToString(sum[:])
}

// IsOpenCodeSDDPromptFileRef reports whether prompt is the {file:...}
// reference a release wrote for phase: a path ending in prompts/sdd/<phase>.md.
func IsOpenCodeSDDPromptFileRef(prompt, phase string) bool {
	match := promptFileRef.FindStringSubmatch(prompt)
	if match == nil {
		return false
	}
	cleaned := path.Clean(strings.ReplaceAll(match[1], `\`, "/"))
	return cleaned == "prompts/sdd/"+phase+".md" || strings.HasSuffix(cleaned, "/prompts/sdd/"+phase+".md")
}

// OpenCodeSDDRegistry holds, per retired SDD agent (sdd-orchestrator or
// sdd-<phase>), every value releases wrote to OpenCode settings: descriptions
// ({{PROFILE}} stands for a profile name), normalized inline prompt digests,
// and orchestrator prompt headings. The generator verifies a registry under
// construction against released golden renders through the same methods.
type OpenCodeSDDRegistry struct {
	Descriptions map[string][]string
	Prompts      map[string][]string
	Headings     map[string][]string
}

var releasedOpenCodeSDD = OpenCodeSDDRegistry{
	Descriptions: releasedOpenCodeSDDDescriptions,
	Prompts:      releasedOpenCodeSDDPrompts,
	Headings:     releasedOpenCodeSDDOrchestratorHeadings,
}

// RetiredOpenCodeSDDAgent splits name into the retired SDD agent a release
// generated it from and the profile suffix, if any (sdd-apply-fallback is
// sdd-apply for profile "fallback"). Any other name is not retired SDD.
func RetiredOpenCodeSDDAgent(name string) (base, profile string, ok bool) {
	return releasedOpenCodeSDD.Retired(name)
}

// OwnsOpenCodeSDDAgent reports whether entry, a V1 `agent` or native `agents`
// value under a retired SDD name, is Gentle AI's: it carries the v2.7.0+
// ownership marker, or every field holds a value some release wrote for that
// agent. Model and variant are the user's assignment and prove nothing.
func OwnsOpenCodeSDDAgent(name string, entry map[string]any) bool {
	return releasedOpenCodeSDD.Owns(name, entry)
}

// Retired is RetiredOpenCodeSDDAgent over this registry.
func (r OpenCodeSDDRegistry) Retired(name string) (base, profile string, ok bool) {
	for candidate := range r.Descriptions {
		if name == candidate {
			return candidate, "", true
		}
		suffix, found := strings.CutPrefix(name, candidate+"-")
		if found && profileName.MatchString(suffix) && len(candidate) > len(base) {
			base, profile = candidate, suffix
		}
	}
	return base, profile, base != ""
}

// Owns is OwnsOpenCodeSDDAgent over this registry.
func (r OpenCodeSDDRegistry) Owns(name string, entry map[string]any) bool {
	base, profile, ok := r.Retired(name)
	if !ok {
		return false
	}
	if entry["__managed_by"] == openCodeSDDMarker {
		return true
	}
	prompt, hasPrompt := entry["prompt"].(string)
	if system, native := entry["system"].(string); native {
		if hasPrompt {
			return false
		}
		prompt, hasPrompt = system, true
	}
	if !hasPrompt {
		return false
	}
	description, _ := entry["description"].(string)
	if profile != "" && base == openCodeSDDOrchestrator {
		description = strings.Replace(description, "("+profile+" profile)", "("+profilePlaceholder+" profile)", 1)
	}
	if !slices.Contains(r.Descriptions[base], description) {
		return false
	}
	for key, value := range entry {
		var valid bool
		switch key {
		case "prompt", "system", "description":
			valid = true
		case "model", "variant":
			_, valid = value.(string)
		case "mode":
			valid = value == "subagent"
			if base == openCodeSDDOrchestrator {
				valid = value == "primary" || value == "all"
			}
		case "hidden":
			valid = value == true && base != openCodeSDDOrchestrator
		case "tools":
			valid = boolMap(value)
		case "permission":
			valid = permissionMap(value, base == openCodeSDDOrchestrator)
		case "permissions":
			valid = permissionRules(value, base == openCodeSDDOrchestrator)
		}
		if !valid {
			return false
		}
	}
	if base != openCodeSDDOrchestrator {
		return entry["hidden"] == true && (IsOpenCodeSDDPromptFileRef(prompt, base) || slices.Contains(r.Prompts[base], OpenCodeSDDPromptDigest(prompt)))
	}
	if slices.Contains(r.Prompts[base], OpenCodeSDDPromptDigest(prompt)) {
		return true
	}
	// A rendered orchestrator prompt embeds model tables and profile names,
	// so its released heading is the proof.
	for _, line := range strings.Split(prompt, "\n") {
		if strings.HasPrefix(line, "# ") {
			return slices.Contains(r.Headings[base], strings.TrimSpace(line))
		}
	}
	return false
}

func boolMap(value any) bool {
	tools, ok := value.(map[string]any)
	if !ok {
		return false
	}
	for _, enabled := range tools {
		if _, ok := enabled.(bool); !ok {
			return false
		}
	}
	return true
}

// permissionMap accepts the V1 permission objects releases wrote: phase
// agents denied tools; orchestrators allowed question and delegated tasks.
func permissionMap(value any, orchestrator bool) bool {
	permission, ok := value.(map[string]any)
	if !ok {
		return false
	}
	for action, rule := range permission {
		if !orchestrator {
			if effect, ok := rule.(string); !ok || !permissionEffect[effect] {
				return false
			}
			continue
		}
		switch action {
		case "question":
			if rule != "allow" {
				return false
			}
		case "task":
			targets, ok := rule.(map[string]any)
			if !ok {
				return false
			}
			for target, effect := range targets {
				if !delegationRule(target, effect) {
					return false
				}
			}
		default:
			return false
		}
	}
	return true
}

// permissionRules is permissionMap for the native rule list 3.7.0 converted
// V1 permissions into (task became the subagent action).
func permissionRules(value any, orchestrator bool) bool {
	rules, ok := value.([]any)
	if !ok {
		return false
	}
	for _, raw := range rules {
		rule, ok := raw.(map[string]any)
		if !ok || len(rule) != 3 {
			return false
		}
		action, _ := rule["action"].(string)
		resource, _ := rule["resource"].(string)
		effect, _ := rule["effect"].(string)
		if action == "" || resource == "" || !permissionEffect[effect] {
			return false
		}
		if !orchestrator {
			continue
		}
		switch action {
		case "question":
			if resource != "*" || effect != "allow" {
				return false
			}
		case "subagent", "task":
			if !delegationRule(resource, effect) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// delegationRule matches the orchestrator task allowlist releases generated.
func delegationRule(target string, effect any) bool {
	if effect != "allow" && effect != "deny" {
		return false
	}
	switch {
	case target == "*", target == "general", target == "explore":
		return true
	}
	for _, prefix := range []string{"sdd-", "jd-", "review-"} {
		if strings.HasPrefix(target, prefix) {
			return true
		}
	}
	return false
}

// OpenCodeSettingsResult reports one settings retirement. Entries are named
// <map>.<agent>, for example agent.sdd-apply or agents.gentle-orchestrator.
type OpenCodeSettingsResult struct {
	Path    string
	Changed bool
	Removed []string
	// Preserved lists retired SDD names Gentle AI cannot prove it wrote.
	Preserved []string
	// Commented lists Gentle AI's entries kept because comments are attached.
	Commented []string
	// Unsupported is set when the settings path is not a regular file.
	Unsupported bool
}

// RetireOpenCodeSDDSettings removes from an OpenCode-family settings file the
// retired SDD agents Gentle AI wrote (OwnsOpenCodeSDDAgent), every entry of
// the native `agents` map that carries the SDD overlay's marker (dropping the
// map once empty), and the orchestrator task allowlist entries that named a
// retired agent no longer present. Edits are local to the removed members, so
// comments and formatting elsewhere survive; a member with comments attached
// is kept and reported. A missing file is not an error.
func RetireOpenCodeSDDSettings(settingsPath string) (OpenCodeSettingsResult, error) {
	result := OpenCodeSettingsResult{Path: settingsPath}
	info, err := os.Lstat(settingsPath)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("inspect settings %s: %w", settingsPath, err)
	}
	if !info.Mode().IsRegular() {
		result.Unsupported = true
		return result, nil
	}
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		return result, fmt.Errorf("read settings %s: %w", settingsPath, err)
	}
	root, err := filemerge.UnmarshalJSONObject(raw)
	if err != nil {
		return result, fmt.Errorf("refuse malformed settings %s: %w", settingsPath, err)
	}
	updated := raw
	remaining := map[string]bool{}
	for _, section := range []string{"agent", "agents"} {
		agents, _ := root[section].(map[string]any)
		var owned []string
		for _, name := range sortedKeys(agents) {
			entry, _ := agents[name].(map[string]any)
			_, _, retired := RetiredOpenCodeSDDAgent(name)
			switch {
			case retired && entry != nil && OwnsOpenCodeSDDAgent(name, entry):
				owned = append(owned, name)
			case section == "agents" && entry != nil && entry["__managed_by"] == openCodeSDDMarker:
				// The native map is the SDD overlay's own output (#5182).
				owned = append(owned, name)
			case retired:
				result.Preserved = append(result.Preserved, section+"."+name)
				remaining[name] = true
			default:
				remaining[name] = true
			}
		}
		next, kept, err := filemerge.RemoveJSONCMembers(updated, []string{section}, owned, section == "agents")
		if err != nil {
			return result, fmt.Errorf("retire SDD agents in %s: %w", settingsPath, err)
		}
		updated = next
		for _, name := range owned {
			if slices.Contains(kept, name) {
				result.Commented = append(result.Commented, section+"."+name)
				remaining[name] = true
			} else {
				result.Removed = append(result.Removed, section+"."+name)
			}
		}
	}
	agents, _ := root["agent"].(map[string]any)
	orchestrator, _ := agents["gentle-orchestrator"].(map[string]any)
	permission, _ := orchestrator["permission"].(map[string]any)
	task, _ := permission["task"].(map[string]any)
	var stale []string
	for _, target := range sortedKeys(task) {
		_, _, retired := RetiredOpenCodeSDDAgent(target)
		if (retired || target == "sdd-*") && task[target] == "allow" && !remaining[target] {
			stale = append(stale, target)
		}
	}
	// A stale allowlist entry kept for its comments is harmless: its agent
	// is gone, so it delegates nowhere.
	updated, _, err = filemerge.RemoveJSONCMembers(updated, []string{"agent", "gentle-orchestrator", "permission", "task"}, stale, false)
	if err != nil {
		return result, fmt.Errorf("retire SDD task permissions in %s: %w", settingsPath, err)
	}
	if string(updated) == string(raw) {
		return result, nil
	}
	written, err := filemerge.WriteFileAtomic(settingsPath, updated, filemerge.ExistingFileMode(settingsPath, 0o644))
	result.Changed = written.Changed
	if err != nil {
		return result, fmt.Errorf("write settings %s: %w", settingsPath, err)
	}
	return result, nil
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// ManualActions tells the user what to do with every entry retirement kept.
func (r OpenCodeSettingsResult) ManualActions() []string {
	var actions []string
	if r.Unsupported {
		actions = append(actions, fmt.Sprintf("%s is not a regular file, so Gentle AI did not inspect it for retired SDD agents. SDD was retired in v4.0.0: if it defines sdd-* agents you no longer need, move or delete them.", r.Path))
	}
	for _, entry := range r.Preserved {
		actions = append(actions, fmt.Sprintf("Retired SDD agent %s in %s was preserved: Gentle AI cannot prove it wrote this entry (its fields differ from every released version), so it may contain your changes. SDD was retired in v4.0.0 and this agent is no longer maintained; if you no longer need it, move or delete it.", entry, r.Path))
	}
	for _, entry := range r.Commented {
		actions = append(actions, fmt.Sprintf("Retired SDD agent %s in %s was kept because comments are attached to it: Gentle AI wrote this entry, but removing it would discard your notes. SDD was retired in v4.0.0: move your comments and then move or delete it, or rerun `gentle-ai sync` to retire it.", entry, r.Path))
	}
	return actions
}

// PromptRetireResult lists the shared SDD prompt files a retirement removed
// and the ones it preserved because no release rendered their bytes.
type PromptRetireResult struct {
	Removed   []string
	Preserved []string
}

// RetireOpenCodeSDDPrompts removes the shared SDD prompt files in dir whose
// normalized bytes some release rendered, then the directory once empty.
// Symlinks, non-regular files, and edited files are preserved and reported.
func RetireOpenCodeSDDPrompts(dir string) (PromptRetireResult, error) {
	var result PromptRetireResult
	for _, phase := range promptPhases {
		file := filepath.Join(dir, phase+".md")
		info, err := os.Lstat(file)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return result, fmt.Errorf("inspect retired SDD prompt %s: %w", file, err)
		}
		owned := false
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(file)
			if err != nil {
				return result, fmt.Errorf("read retired SDD prompt %s: %w", file, err)
			}
			owned = slices.Contains(releasedOpenCodeSDDPromptFiles[phase+".md"], OpenCodeSDDPromptDigest(string(data)))
		}
		if !owned {
			result.Preserved = append(result.Preserved, file)
			continue
		}
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			return result, fmt.Errorf("remove retired SDD prompt %s: %w", file, err)
		}
		result.Removed = append(result.Removed, file)
	}
	if len(result.Removed) > 0 {
		// Only an empty directory is removed; anything else stays.
		_ = os.Remove(dir)
	}
	return result, nil
}

// ManualActions tells the user what to do with every prompt retirement kept.
func (r PromptRetireResult) ManualActions() []string {
	actions := make([]string, 0, len(r.Preserved))
	for _, file := range r.Preserved {
		actions = append(actions, fmt.Sprintf("Retired SDD prompt %s was preserved: Gentle AI cannot prove it wrote this file (its content differs from every released version, or it is not a regular file), so it may contain your changes. SDD was retired in v4.0.0 and no agent loads it anymore; if you no longer need it, move or delete it.", file))
	}
	return actions
}
