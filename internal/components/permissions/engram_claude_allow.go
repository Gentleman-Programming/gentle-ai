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

// engramClaudeAutoAllowRuleGapsFor returns the candidate rules not already
// covered by permissionsNode's allow/ask/deny entries, in deterministic
// (server, then tool) order.
func engramClaudeAutoAllowRuleGapsFor(permissionsNode map[string]any) []string {
	existingRules := claudeCodePermissionRuleStrings(permissionsNode, "allow", "ask", "deny")
	var missing []string
	for _, server := range engramClaudeMCPServerNames {
		for _, tool := range engramClaudeAutoAllowTools {
			rule := "mcp__" + server + "__" + tool
			if !claudeCodeRuleSetCovers(existingRules, server, rule) {
				missing = append(missing, rule)
			}
		}
	}
	return missing
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
// normalization: stripped comments and trailing commas) is left untouched.
// This is a narrow, additive healer, not a general settings writer, and a
// file it cannot understand is a file it must not guess at.
func EnsureEngramClaudeAutoAllowRules(homeDir string) error {
	path := filepath.Join(homeDir, ".claude", "settings.json")

	raw, err := osReadFile(path)
	if err != nil {
		return err
	}

	root, err := filemerge.UnmarshalJSONObject(raw)
	if err != nil {
		return nil
	}

	permissionsNode, _ := root["permissions"].(map[string]any)
	missing := engramClaudeAutoAllowRuleGapsFor(permissionsNode)
	if len(missing) == 0 {
		return nil
	}

	if permissionsNode == nil {
		permissionsNode = map[string]any{}
		root["permissions"] = permissionsNode
	}
	existingAllow, _ := permissionsNode["allow"].([]any)
	missingAny := make([]any, len(missing))
	for i, rule := range missing {
		missingAny[i] = rule
	}
	permissionsNode["allow"] = append(append([]any{}, existingAllow...), missingAny...)

	encoded, err := filemerge.MarshalJSONPreservingPermissions(raw, root)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')

	_, err = filemerge.WriteFileAtomic(path, encoded, filemerge.ExistingFileMode(path, 0o644))
	return err
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
func claudeCodeRuleSetCovers(existingRules []string, server, candidate string) bool {
	serverRule := "mcp__" + server
	for _, rule := range existingRules {
		if rule == candidate || rule == serverRule {
			return true
		}
		if !strings.ContainsAny(rule, "*?") {
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
