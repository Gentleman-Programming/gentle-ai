package permissions

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/opencodeagents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
)

type InjectionResult struct {
	Changed bool
	Files   []string
}

// TargetPath returns the file path that permission injection creates or updates
// for the adapter, or an empty string when the agent has no supported
// permission injection target. Codex has no target: gentle-ai relies on Codex's
// built-in default permissions and does not write its permissions config at all.
func TargetPath(homeDir string, adapter agents.Adapter) string {
	if agentOverlay(adapter.Agent()) == nil {
		return ""
	}
	return adapter.SettingsPath(homeDir)
}

// claudeCodeOverlayJSON sets Claude Code to bypassPermissions mode (auto-accept all).
// Valid modes: "acceptEdits", "bypassPermissions", "default", "dontAsk", "plan".
var claudeCodeOverlayJSON = []byte(`{
  "permissions": {
    "defaultMode": "bypassPermissions",
    "deny": [
      "Bash(rm -rf /)",
      "Bash(sudo rm -rf /)",
      "Bash(rm -rf ~)",
      "Bash(sudo rm -rf ~)",
      "Read(.env)",
      "Read(.env.*)",
      "Edit(.env)",
      "Edit(.env.*)",
      "Read(~/.ssh/**)",
      "Edit(~/.ssh/**)",
      "Read(~/.credentials/**)",
      "Edit(~/.credentials/**)",
      "Read(~/Library/Keychains/**)",
      "Edit(~/Library/Keychains/**)",
      "Read(~/.aws/credentials)",
      "Edit(~/.aws/credentials)",
      "Read(~/.config/gh/hosts.yml)",
      "Edit(~/.config/gh/hosts.yml)",
      "Read(**/*.pem)",
      "Edit(**/*.pem)",
      "Read(**/*.key)",
      "Edit(**/*.key)",
      "Read(**/secrets/*)",
      "Edit(**/secrets/*)"
    ]
  }
}
`)

// geminiCLIOverlayJSON sets Gemini CLI to "auto_edit" mode (auto-approve edit tools).
var geminiCLIOverlayJSON = []byte(`{
  "general": {
    "defaultApprovalMode": "auto_edit"
  }
}
`)

// qwenCodeOverlayJSON sets Qwen Code to "auto_edit" mode (auto-approve edits, manual approval for shell commands).
var qwenCodeOverlayJSON = []byte(`{
  "permissions": {
    "defaultMode": "auto_edit"
  }
}
`)

// vscodeCopilotOverlayJSON enables auto-approve for VS Code Copilot chat tools.
var vscodeCopilotOverlayJSON = []byte(`{
  "chat.tools.autoApprove": true
}
`)

// agentOverlay returns the correct permission overlay for the given agent,
// or nil if the agent does not support permission injection via settings.json.
func agentOverlay(id model.AgentID) []byte {
	switch id {
	case model.AgentClaudeCode:
		return claudeCodeOverlayJSON
	case model.AgentOpenCode, model.AgentKilocode:
		return openCodeOverlayJSON
	case model.AgentGeminiCLI:
		return geminiCLIOverlayJSON
	case model.AgentQwenCode:
		return qwenCodeOverlayJSON
	case model.AgentAntigravity:
		// Antigravity manages permissions via IDE UI (Artifact Review Policy /
		// Terminal Command Auto Execution). No injectable settings.json schema.
		return nil
	case model.AgentVSCodeCopilot:
		return vscodeCopilotOverlayJSON
	case model.AgentCursor:
		// Cursor manages permissions via cli-config.json, not settings.json.
		return nil
	case model.AgentCodex:
		// Codex relies on its built-in default permissions. gentle-ai writes
		// nothing to Codex's permissions config — not a profile, and not the
		// legacy cleanup that used to strip one. Codex refuses to load a config
		// that defines a [permissions.*] profile without default_permissions,
		// so a cleanup removing the pointer while a user profile survived left
		// Codex unable to start (#1794). An old gentle-dev profile stays until
		// its owner removes it.
		return nil
	case model.AgentHermes:
		// Hermes permission format is undocumented — no overlay is injected (§14).
		return nil
	default:
		return nil
	}
}

func Inject(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
	return InjectAtPath(TargetPath(homeDir, adapter), adapter)
}

// InjectAtPath writes the permission overlay to the caller-selected settings
// path while preserving each adapter's permission capability check. Only the
// OpenCode selected settings refuse symlinked, non-regular or locked files;
// other agents keep the shared writer behavior.
func InjectAtPath(settingsPath string, adapter agents.Adapter) (InjectionResult, error) {
	if settingsPath == "" {
		return InjectionResult{}, nil
	}

	overlay := agentOverlay(adapter.Agent())
	if overlay == nil {
		return InjectionResult{}, nil
	}
	if adapter.Agent() == model.AgentOpenCode {
		if err := filemerge.RefuseLockedSettingsFile(settingsPath); err != nil {
			return InjectionResult{}, err
		}
	}

	// Native V2 documents take the ordered `permissions` rule list: convert
	// the safety overlay to rules and merge them beside the user's rules.
	// Everything stays legacy for legacy documents and other adapters.
	if adapter.Agent() == model.AgentOpenCode {
		raw, readErr := os.ReadFile(settingsPath)
		if readErr != nil && !os.IsNotExist(readErr) {
			return InjectionResult{}, readErr
		}
		if readErr == nil {
			nativeOverlay, native, err := openCodeNativeOverlay(raw)
			if err != nil {
				return InjectionResult{}, err
			}
			if native {
				writeResult, err := mergeJSONFile(settingsPath, nativeOverlay, filemerge.MergeJSONObjectsForPath)
				if err != nil {
					return InjectionResult{}, err
				}
				return InjectionResult{Changed: writeResult.Changed, Files: []string{settingsPath}}, nil
			}
		}
	}

	merge := filemerge.MergeJSONObjectsForPath
	switch adapter.Agent() {
	case model.AgentClaudeCode:
		merge = mergeClaudeCodePermissionsForPath
	case model.AgentOpenCode:
		merge = filemerge.MergeOpenCodeJSONDefaultsForPath
	case model.AgentKilocode:
		merge = filemerge.MergeJSONDefaultsForPath
	}
	if adapter.Agent() == model.AgentOpenCode || adapter.Agent() == model.AgentKilocode {
		defaultsMerge := merge
		merge = func(path string, base, overlay []byte) ([]byte, error) {
			return defaultsMerge(path, base, withoutRulesLooserThanUserDefault(base, overlay))
		}
	}
	writeResult, err := mergeJSONFile(settingsPath, overlay, merge)
	if err != nil {
		return InjectionResult{}, err
	}

	return InjectionResult{Changed: writeResult.Changed, Files: []string{settingsPath}}, nil
}

// openCodeNativeOverlay converts the OpenCode safety overlay into the native
// ordered `permissions` rule list for documents that already are native V2.
// Existing user rules keep their order; missing overlay rules are appended
// (the legacy object evaluator walks patterns in byte order and the emitted
// rules are sorted by resource, so precedence is preserved exactly). The
// second return reports whether the document is native; a false value falls
// back to the legacy object merge.
func openCodeNativeOverlay(raw []byte) ([]byte, bool, error) {
	root, err := filemerge.UnmarshalJSONObject(raw)
	if err != nil {
		return nil, false, err
	}
	if !opencode.NativeConfig(root) {
		return nil, false, nil
	}
	legacy, err := filemerge.UnmarshalJSONObject(openCodeOverlayJSON)
	if err != nil {
		return nil, false, err
	}
	permission, _ := legacy["permission"].(map[string]any)
	rules := opencodeagents.NativeRules(permission)
	existing, _ := root["permissions"].([]any)
	current := make([]map[string]any, 0, len(existing))
	for _, item := range existing {
		rule, _ := item.(map[string]any)
		if rule == nil {
			continue
		}
		current = append(current, rule)
	}
	overlay, err := json.Marshal(map[string]any{"permissions": opencodeagents.MergeRuleLists(current, rules)})
	if err != nil {
		return nil, false, err
	}
	return overlay, true, nil
}

// staleClaudeCodeDenyRules maps the cwd-relative home rules earlier releases
// wrote to their home-anchored replacements (#5280).
var staleClaudeCodeDenyRules = map[string]string{
	"Read(.ssh/*)":               "Read(~/.ssh/**)",
	"Edit(.ssh/*)":               "Edit(~/.ssh/**)",
	"Read(.credentials/*)":       "Read(~/.credentials/**)",
	"Edit(.credentials/*)":       "Edit(~/.credentials/**)",
	"Read(Library/Keychains/*)":  "Read(~/Library/Keychains/**)",
	"Edit(Library/Keychains/*)":  "Edit(~/Library/Keychains/**)",
	"Read(.aws/credentials)":     "Read(~/.aws/credentials)",
	"Edit(.aws/credentials)":     "Edit(~/.aws/credentials)",
	"Read(.config/gh/hosts.yml)": "Read(~/.config/gh/hosts.yml)",
	"Edit(.config/gh/hosts.yml)": "Edit(~/.config/gh/hosts.yml)",
}

// mergeClaudeCodePermissionsForPath seeds deny only when absent. Existing lists,
// including an explicit empty list, belong to the user; reinjection must neither
// replace them nor append defaults that undo a deliberately narrowed policy. It
// only anchors stale package rules in place, dropping any duplicate.
func mergeClaudeCodePermissionsForPath(path string, base, overlay []byte) ([]byte, error) {
	existing, err := filemerge.UnmarshalJSONObject(base)
	if err != nil {
		return filemerge.MergeJSONObjectsForPath(path, base, overlay)
	}
	permissions, _ := existing["permissions"].(map[string]any)
	if _, exists := permissions["deny"]; !exists {
		return filemerge.MergeJSONObjectsForPath(path, base, overlay)
	}
	defaults, err := filemerge.UnmarshalJSONObject(overlay)
	if err != nil {
		return nil, err
	}
	if defaultPermissions, ok := defaults["permissions"].(map[string]any); ok {
		delete(defaultPermissions, "deny")
		if deny, ok := permissions["deny"].([]any); ok {
			defaultPermissions["deny"] = anchorStaleClaudeCodeDenyRules(deny)
		}
	}
	overlay, err = json.Marshal(defaults)
	if err != nil {
		return nil, err
	}
	return filemerge.MergeJSONObjectsForPath(path, base, overlay)
}

func anchorStaleClaudeCodeDenyRules(deny []any) []any {
	present := map[string]bool{}
	for _, rule := range deny {
		if name, ok := rule.(string); ok {
			present[name] = true
		}
	}
	anchored := make([]any, 0, len(deny))
	for _, rule := range deny {
		name, _ := rule.(string)
		if replacement, stale := staleClaudeCodeDenyRules[name]; stale {
			if present[replacement] {
				continue
			}
			present[replacement] = true
			rule = replacement
		}
		anchored = append(anchored, rule)
	}
	return anchored
}

// withoutRulesLooserThanUserDefault drops overlay tool rules looser than the
// user's root permission "*" (deny is stricter than ask, ask than allow).
// Merged into a tool map the user scoped, such a rule follows the root "*" and,
// as the last match, would loosen the user's default.
func withoutRulesLooserThanUserDefault(base, overlay []byte) []byte {
	strictness := map[string]int{"allow": 0, "ask": 1, "deny": 2}
	root, err := filemerge.UnmarshalJSONObject(base)
	if err != nil {
		return overlay
	}
	permission, _ := root["permission"].(map[string]any)
	rootAction, _ := permission["*"].(string)
	if patterns, ok := permission["*"].(map[string]any); ok {
		rootAction, _ = patterns["*"].(string)
	}
	floor, ok := strictness[rootAction]
	if !ok || floor == 0 {
		return overlay
	}
	defaults, err := filemerge.UnmarshalJSONObject(overlay)
	if err != nil {
		return overlay
	}
	rules, _ := defaults["permission"].(map[string]any)
	for _, value := range rules {
		patterns, _ := value.(map[string]any)
		for pattern, action := range patterns {
			if name, _ := action.(string); strictness[name] < floor {
				delete(patterns, pattern)
			}
		}
	}
	encoded, err := json.Marshal(defaults)
	if err != nil {
		return overlay
	}
	return encoded
}

func mergeJSONFile(path string, overlay []byte, merge func(string, []byte, []byte) ([]byte, error)) (filemerge.WriteResult, error) {
	baseJSON, err := osReadFile(path)
	if err != nil {
		return filemerge.WriteResult{}, err
	}

	merged, err := merge(path, baseJSON, overlay)
	if err != nil {
		return filemerge.WriteResult{}, err
	}

	return filemerge.WriteFileAtomic(path, merged, filemerge.ExistingFileMode(path, 0o644))
}

var osReadFile = func(path string) ([]byte, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read json file %q: %w", path, err)
	}

	return content, nil
}
