package permissions

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
)

// engramClaudeAutoAllowTools are the non-destructive Engram MCP tools Claude
// Code's "auto" permission mode must allow so Engram's session-binding
// PreToolUse hook rewrite (hookSpecificOutput.updatedInput) can take effect.
// mem_delete and mem_merge_projects are deliberately excluded: those stay
// behind the ordinary auto-mode classifier instead of being pre-approved.
var engramClaudeAutoAllowTools = []string{
	"mem_save",
	"mem_update",
	"mem_review",
	"mem_save_prompt",
	"mem_pin",
	"mem_unpin",
	"mem_session_summary",
	"mem_session_start",
	"mem_session_end",
	"mem_capture_passive",
	"mem_judge",
	"mem_compare",
}

// engramClaudeMCPServerNames are the two server names Claude Code may have
// registered Engram's MCP tools under: the bare "engram" MCP server, and the
// "plugin_engram_engram" server name a Gentle AI plugin install exposes the
// same tools under. Both prefixes are already used together elsewhere in this
// codebase (internal/components/legacyassets/sdd_agents.go,
// internal/components/reviewassets/install.go).
var engramClaudeMCPServerNames = []string{"engram", "plugin_engram_engram"}

// engramClaudeAutoAllowRules returns every candidate Claude Code
// permissions.allow rule string this fix may add: each of
// engramClaudeAutoAllowTools under each of engramClaudeMCPServerNames.
func engramClaudeAutoAllowRules() []string {
	rules := make([]string, 0, len(engramClaudeAutoAllowTools)*len(engramClaudeMCPServerNames))
	for _, server := range engramClaudeMCPServerNames {
		for _, tool := range engramClaudeAutoAllowTools {
			rules = append(rules, "mcp__"+server+"__"+tool)
		}
	}
	return rules
}

// EngramClaudeAutoAllowRuleGaps returns the Engram auto-mode allow rules
// still missing from the Claude Code settings.json content in raw, without
// writing anything. The doctor's engram:claude_auto_allow check uses this to
// surface a gap that EnsureEngramClaudeAutoAllowRules would otherwise only
// repair silently on the next install or sync.
//
// A raw document that cannot be parsed as JSON reports every candidate rule
// as missing: a settings.json the doctor cannot verify must not be reported
// as healthy.
func EngramClaudeAutoAllowRuleGaps(raw []byte) []string {
	root, err := filemerge.UnmarshalJSONObject(raw)
	if err != nil {
		return engramClaudeAutoAllowRules()
	}
	permissionsNode, _ := root["permissions"].(map[string]any)
	return engramClaudeAutoAllowRuleGapsFor(permissionsNode)
}

// EngramClaudeAutoAllowRuleReport is EngramClaudeAutoAllowStatus's result: it
// tells the doctor whether raw parsed at all, and when it did, which
// candidate rules are missing from every permission list versus restricted by
// an existing ask or deny entry. Claude Code evaluates deny, then ask, then
// allow, so a rule only an ask/deny entry matches stays blocked or prompts —
// that is not the same as Pass, but EngramClaudeAutoAllowRuleGaps (and
// EnsureEngramClaudeAutoAllowRules) correctly leave it out of Missing too,
// since adding a contradictory allow entry next to the user's own
// restriction would not change Claude Code's evaluation order.
type EngramClaudeAutoAllowRuleReport struct {
	Parsed     bool
	Missing    []string
	Restricted []string

	// Incompatible is true when raw parsed but has a shape the repair writer
	// refuses to touch (a non-object document or "permissions", or a
	// non-array "permissions.allow"), so sync cannot resolve the gap and only
	// a manual edit can.
	Incompatible bool
}

// EngramClaudeAutoAllowStatus is EngramClaudeAutoAllowRuleGaps's superset: it
// additionally tells apart a rule the settings content could not be parsed
// well enough to verify at all (Parsed is false) from one genuinely covered
// by an ask/deny restriction rather than by allow (Restricted).
func EngramClaudeAutoAllowStatus(raw []byte) EngramClaudeAutoAllowRuleReport {
	root, err := filemerge.UnmarshalJSONObject(raw)
	if err != nil {
		return EngramClaudeAutoAllowRuleReport{}
	}
	permissionsNode, _ := root["permissions"].(map[string]any)
	missing, restricted := engramClaudeAutoAllowRuleStatusFor(permissionsNode)
	return EngramClaudeAutoAllowRuleReport{
		Parsed:       true,
		Missing:      missing,
		Restricted:   restricted,
		Incompatible: engramClaudeSettingsShapeIncompatible(root),
	}
}

// engramClaudeSettingsShapeIncompatible reports whether root has a shape
// EnsureEngramClaudeAutoAllowRules refuses to touch: a non-object document, a
// "permissions" value that is not an object, or a "permissions.allow" value
// that is not an array.
func engramClaudeSettingsShapeIncompatible(root map[string]any) bool {
	if root == nil {
		return true
	}
	rawPermissions, hasPermissions := root["permissions"]
	if !hasPermissions {
		return false
	}
	permissionsNode, ok := rawPermissions.(map[string]any)
	if !ok {
		return true
	}
	rawAllow, hasAllow := permissionsNode["allow"]
	if !hasAllow {
		return false
	}
	_, ok = rawAllow.([]any)
	return !ok
}

// engramClaudeAutoAllowRuleGapsFor returns the candidate rules not already
// covered by permissionsNode's allow entries or restricted by its ask/deny
// entries, in deterministic (server, then tool) order.
func engramClaudeAutoAllowRuleGapsFor(permissionsNode map[string]any) []string {
	missing, _ := engramClaudeAutoAllowRuleStatusFor(permissionsNode)
	return missing
}

// engramClaudeAutoAllowRuleStatusFor partitions every candidate rule into
// missing (covered by neither allow nor ask/deny), restricted (covered by an
// ask/deny entry but not by allow), or implicitly allowed (covered by an
// allow entry, reported in neither slice), in deterministic (server, then
// tool) order.
//
// Allow coverage never counts a glob whose server segment itself is a
// wildcard (such as "mcp__*__mem_save"): Claude Code does not honor that
// shape as covering a specific server's tool (Gentleman-Programming/engram
// issue tracker and anthropics/claude-code#73052), so treating it as allowed
// would leave the real tool denied while this helper reports it healthy.
// Ask/deny coverage keeps matching every glob shape, including that one,
// since a broad restriction is still a restriction regardless of which
// segment the wildcard sits in.
func engramClaudeAutoAllowRuleStatusFor(permissionsNode map[string]any) (missing, restricted []string) {
	allowRules := claudeCodePermissionRuleStrings(permissionsNode, "allow")
	restrictionRules := claudeCodePermissionRuleStrings(permissionsNode, "ask", "deny")
	for _, server := range engramClaudeMCPServerNames {
		for _, tool := range engramClaudeAutoAllowTools {
			rule := "mcp__" + server + "__" + tool
			switch {
			case claudeCodeRuleSetCovers(restrictionRules, server, rule, true):
				// Claude Code evaluates deny, then ask, then allow, so a
				// restriction wins even when an allow rule also matches. Keep
				// the tool out of missing so no conflicting allow is added.
				restricted = append(restricted, rule)
			case claudeCodeRuleSetCovers(allowRules, server, rule, false):
				// Fully allowed; reported in neither slice.
			default:
				missing = append(missing, rule)
			}
		}
	}
	return missing, restricted
}

// EnsureEngramClaudeAutoAllowRules adds the missing Engram auto-mode allow
// rules to Claude Code's settings.json under homeDir.
//
// `engram setup claude-code` normally seeds these allow rules itself, but
// gentle-ai invokes it non-interactively (internal/cli/run.go's
// componentApplyStep.Run, case model.ComponentEngram, via executeCommand,
// which never sets cmd.Stdin). The setup command's "Add to allowlist? (y/N)"
// prompt then reads immediate EOF from the closed stdin and always answers
// "Skipped" — every gentle-ai-installed Claude Code silently loses these
// rules, and Engram's memory-write tools (mem_save and friends) are denied by
// Claude Code's auto permission mode with "no verdict" because the
// PreToolUse hook rewrite never gets a chance to run
// (Gentleman-Programming/engram#2231, open upstream). This writes the rules
// directly instead, without depending on Engram's interactive prompt at all —
// the approach Snakeblack/ospec-workflow#477 used to fix the identical
// problem in its own installer.
//
// Existing permissions.allow/ask/deny entries are preserved exactly: a
// candidate rule is skipped when it is already present verbatim, or already
// covered by a broader existing rule (an MCP server-level entry such as
// "mcp__engram", or any other existing glob that already matches it). Missing
// rules are appended to permissions.allow; nothing is ever removed or
// reordered. Repeated calls are idempotent.
//
// A settings.json that cannot be parsed as JSON (even after JSONC-style
// normalization: stripped comments and trailing commas), that decodes to a
// JSON value other than an object (for example a bare `null` document), or
// whose existing "permissions" or "permissions.allow" value is not the
// object/array shape this helper expects, is left untouched. This is a
// narrow, additive healer, not a general settings writer, and a file it
// cannot understand — or a value it would otherwise have to coerce and
// silently replace — is a file it must not guess at.
//
// It does not guard the publish below against a concurrent editor of the same
// file: WriteFileAtomic compares only against the bytes this call itself
// read, so a save landing between that read and this write can be
// overwritten. Closing that window needs a lock shared with every other
// settings.json writer in this package, not one added here alone; until that
// exists, this helper accepts the same narrow race every other writer in
// this package already has.
//
// The returned bool reports whether it rewrote the file, so a best-effort
// caller such as sync can still record the repair instead of under-reporting
// "nothing changed".
func EnsureEngramClaudeAutoAllowRules(homeDir string) (bool, error) {
	path := filepath.Join(homeDir, ".claude", "settings.json")

	raw, err := osReadFile(path)
	if err != nil {
		return false, err
	}

	root, err := filemerge.UnmarshalJSONObject(raw)
	if err != nil || root == nil {
		return false, nil
	}

	rawPermissions, hasPermissions := root["permissions"]
	permissionsNode, permissionsOK := rawPermissions.(map[string]any)
	if hasPermissions && !permissionsOK {
		return false, nil
	}

	missing := engramClaudeAutoAllowRuleGapsFor(permissionsNode)
	if len(missing) == 0 {
		return false, nil
	}

	rawAllow, hasAllow := permissionsNode["allow"]
	existingAllow, allowOK := rawAllow.([]any)
	if hasAllow && !allowOK {
		return false, nil
	}

	if permissionsNode == nil {
		permissionsNode = map[string]any{}
		root["permissions"] = permissionsNode
	}
	missingAny := make([]any, len(missing))
	for i, rule := range missing {
		missingAny[i] = rule
	}
	permissionsNode["allow"] = append(append([]any{}, existingAllow...), missingAny...)

	encoded, err := filemerge.MarshalJSONPreservingPermissions(raw, root)
	if err != nil {
		return false, err
	}
	encoded = append(encoded, '\n')

	if _, err := filemerge.WriteFileAtomic(path, encoded, filemerge.ExistingFileMode(path, 0o644)); err != nil {
		return false, err
	}
	return true, nil
}

// claudeCodePermissionRuleStrings collects every string rule present under
// the named permission lists (allow/ask/deny) of permissionsNode.
func claudeCodePermissionRuleStrings(permissionsNode map[string]any, lists ...string) []string {
	var rules []string
	for _, list := range lists {
		entries, _ := permissionsNode[list].([]any)
		for _, entry := range entries {
			if rule, ok := entry.(string); ok {
				rules = append(rules, rule)
			}
		}
	}
	return rules
}

// claudeCodeRuleSetCovers reports whether candidate (an
// "mcp__<server>__<tool>" rule) is already covered by one of existingRules:
// an exact match, the bare "mcp__<server>" entry Claude Code treats as
// covering every tool that server exposes, or any other existing glob rule
// ("*" spans any run of characters, "?" matches exactly one) that already
// matches candidate.
//
// When allowWildcardServer is false, a glob whose own server segment is
// itself a wildcard (such as "mcp__*__mem_save") is skipped rather than
// matched: see hasWildcardMCPServerSegment's doc comment for why. Pass true
// for an ask/deny rule set, where that shape is still a real restriction
// regardless of which segment the wildcard sits in.
func claudeCodeRuleSetCovers(existingRules []string, server, candidate string, allowWildcardServer bool) bool {
	serverRule := "mcp__" + server
	for _, rule := range existingRules {
		if rule == candidate || rule == serverRule {
			return true
		}
		if !strings.ContainsAny(rule, "*?") {
			continue
		}
		if !allowWildcardServer && hasWildcardMCPServerSegment(rule) {
			continue
		}
		pattern := regexp.QuoteMeta(rule)
		pattern = strings.ReplaceAll(pattern, `\*`, ".*")
		pattern = strings.ReplaceAll(pattern, `\?`, ".")
		if matched, err := regexp.MatchString("^"+pattern+"$", candidate); err == nil && matched {
			return true
		}
	}
	return false
}

// hasWildcardMCPServerSegment reports whether rule starts with "mcp__" and
// has a wildcard character in its server segment: everything up to the next
// "__", or the whole remainder when there is none. That covers
// "mcp__*__mem_save", "mcp__eng*" and a bare "mcp__*". Claude Code accepts an
// allow glob only after a literal "mcp__<server>__" prefix and skips the rest
// (https://code.claude.com/docs/en/permissions), so counting such a rule as
// allow coverage would report a tool healthy while Claude Code still does not
// allow it.
func hasWildcardMCPServerSegment(rule string) bool {
	rest, ok := strings.CutPrefix(rule, "mcp__")
	if !ok {
		return false
	}
	serverSegment, _, _ := strings.Cut(rest, "__")
	return strings.ContainsAny(serverSegment, "*?")
}
