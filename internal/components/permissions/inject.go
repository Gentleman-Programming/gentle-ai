package permissions

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
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
      "Read(.ssh/*)",
      "Edit(.ssh/*)",
      "Read(.credentials/*)",
      "Edit(.credentials/*)",
      "Read(Library/Keychains/*)",
      "Edit(Library/Keychains/*)",
      "Read(.aws/credentials)",
      "Edit(.aws/credentials)",
      "Read(.config/gh/hosts.yml)",
      "Edit(.config/gh/hosts.yml)",
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

	merge := filemerge.MergeJSONObjectsForPath
	switch adapter.Agent() {
	case model.AgentOpenCode:
		merge = filemerge.MergeOpenCodeJSONDefaultsForPath
	case model.AgentKilocode:
		merge = filemerge.MergeJSONDefaultsForPath
	}
	if adapter.Agent() == model.AgentOpenCode || adapter.Agent() == model.AgentKilocode {
		defaultsMerge := merge
		merge = func(path string, base, overlay []byte) ([]byte, error) {
			return defaultsMerge(path, base, withoutWildcardAllowsUnderUserDeny(base, overlay))
		}
	}
	writeResult, err := mergeJSONFile(settingsPath, overlay, merge)
	if err != nil {
		return InjectionResult{}, err
	}

	return InjectionResult{Changed: writeResult.Changed, Files: []string{settingsPath}}, nil
}

// withoutWildcardAllowsUnderUserDeny drops the overlay's "*": "allow" tool
// rules when the user's root permission denies by default. Merged into a tool
// map the user scoped, such an allow is the last match and would loosen the
// user's deny; the overlay's asks and denies only tighten, so they stay.
func withoutWildcardAllowsUnderUserDeny(base, overlay []byte) []byte {
	root, err := filemerge.UnmarshalJSONObject(base)
	if err != nil {
		return overlay
	}
	permission, _ := root["permission"].(map[string]any)
	if permission["*"] != "deny" {
		return overlay
	}
	defaults, err := filemerge.UnmarshalJSONObject(overlay)
	if err != nil {
		return overlay
	}
	rules, _ := defaults["permission"].(map[string]any)
	for _, value := range rules {
		if patterns, ok := value.(map[string]any); ok && patterns["*"] == "allow" {
			delete(patterns, "*")
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
