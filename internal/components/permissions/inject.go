package permissions

import (
	"fmt"
	"os"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

type InjectionResult struct {
	Changed bool
	Files   []string
}

// TargetPath returns the file path that permission injection creates or updates
// for the adapter, or an empty string when the agent has no supported
// permission injection target. Codex has no target: gentle-ai relies on
// Codex's built-in default permissions and only cleans up the previously
// injected profile from an existing config.
func TargetPath(homeDir string, adapter agents.Adapter) string {
	if agentOverlay(adapter.Agent()) == nil {
		return ""
	}
	return adapter.SettingsPath(homeDir)
}

// CleanupPath returns the file that permission cleanup may modify for the
// adapter without ever creating it, or an empty string when the agent has no
// cleanup target. Unlike TargetPath it is not an injection output: callers
// that snapshot files for backup/rollback should include it when it exists or
// when its absence cannot be confirmed, and it must never be treated as a
// required post-apply file.
func CleanupPath(homeDir string, adapter agents.Adapter) string {
	if adapter.Agent() == model.AgentCodex {
		return adapter.MCPConfigPath(homeDir, "")
	}
	return ""
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

// openCodeOverlayJSON uses the OpenCode "permission" key with bash/read granularity.
var openCodeOverlayJSON = []byte(`{
  "permission": {
    "bash": {
      "*": "allow",
      "git commit *": "ask",
      "git push *": "ask",
      "git push": "ask",
      "git push --force *": "ask",
      "git rebase *": "ask",
      "git reset --hard *": "ask"
    },
    "read": {
      "*": "allow",
      "*.env": "deny",
      "*.env.*": "deny",
      "**/.env": "deny",
      "**/.env.*": "deny",
      "**/secrets/**": "deny",
      "**/credentials.json": "deny",
      "**/.ssh/**": "deny",
      "**/.credentials/**": "deny",
      "**/Library/Keychains/**": "deny",
      "**/.aws/credentials": "deny",
      "**/.config/gh/hosts.yml": "deny",
      "**/*.pem": "deny",
      "**/*.key": "deny"
    }
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

var codexLegacyPermissionValues = [][2]string{
	{`permissions.gentle-dev.description`, `"Comfortable local development profile with workspace writes, network access, and read-only access to Git and Nix/Home Manager metadata."`},
	{`permissions.gentle-dev.network.enabled`, `true`},
	{`permissions.gentle-dev.network.domains."*"`, `"allow"`},
	{`permissions.gentle-dev.filesystem.glob_scan_max_depth`, `6`},
	{`permissions.gentle-dev.filesystem.":minimal"`, `"read"`},
	{`permissions.gentle-dev.filesystem."~/.config/git"`, `"read"`},
	{`permissions.gentle-dev.filesystem."~/.gitconfig"`, `"read"`},
	{`permissions.gentle-dev.filesystem."~/.local/state/nix/profiles/home-manager/home-path"`, `"read"`},
	{`permissions.gentle-dev.filesystem."~/.nix-profile"`, `"read"`},
	{`permissions.gentle-dev.filesystem."/nix/store"`, `"read"`},
	{`permissions.gentle-dev.filesystem.":tmpdir"`, `"write"`},
	{`permissions.gentle-dev.filesystem.":slash_tmp"`, `"write"`},
	{`permissions.gentle-dev.filesystem.":root"."."`, `"read"`},
	{`permissions.gentle-dev.filesystem.":workspace_roots"."."`, `"write"`},
	{`permissions.gentle-dev.filesystem.":workspace_roots".".git/**"`, `"write"`},
	{`permissions.gentle-dev.filesystem.":workspace_roots"."**/.env"`, `"deny"`},
	{`permissions.gentle-dev.filesystem.":workspace_roots"."**/*.pem"`, `"deny"`},
	{`permissions.gentle-dev.filesystem.":workspace_roots"."**/*.key"`, `"deny"`},
	{`permissions.gentle-dev.workspace_roots."~"`, `true`},
}

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
		// Codex relies on its built-in default permissions; no overlay is
		// injected. Inject only cleans up the retired gentle-dev profile.
		return nil
	case model.AgentHermes:
		// Hermes permission format is undocumented — no overlay is injected (§14).
		return nil
	default:
		return nil
	}
}

func Inject(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
	if adapter.Agent() == model.AgentCodex {
		return injectCodexPermissions(homeDir, adapter)
	}

	settingsPath := adapter.SettingsPath(homeDir)
	if settingsPath == "" {
		return InjectionResult{}, nil
	}

	overlay := agentOverlay(adapter.Agent())
	if overlay == nil {
		return InjectionResult{}, nil
	}

	writeResult, err := mergeJSONFile(settingsPath, overlay)
	if err != nil {
		return InjectionResult{}, err
	}

	return InjectionResult{Changed: writeResult.Changed, Files: []string{settingsPath}}, nil
}

// injectCodexPermissions no longer writes a permission profile: gentle-ai
// relies on Codex's built-in default permissions, which stay compatible with
// toolchains the retired restrictive gentle-dev profile broke (#1398). It only
// migrates an existing config by removing exactly what gentle-ai previously
// injected — the "on-request" approval policy, the "gentle-dev" default
// profile pointer, and exact legacy permission entries — leaving user-authored
// or modified values byte-untouched even under permissions.gentle-dev.
//
// Deprecated path: this is a migration-only cleanup. Once a deprecation
// window has passed and installed configs no longer carry the injected
// profile, it can be removed together with CleanupPath.
func injectCodexPermissions(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
	configPath := adapter.MCPConfigPath(homeDir, "")
	baseTOML, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No config file: nothing to clean, and cleanup never creates one.
			return InjectionResult{}, nil
		}
		return InjectionResult{}, fmt.Errorf("read Codex config TOML %q: %w", configPath, err)
	}

	cleaned := filemerge.RemoveTopLevelTOMLKeyIfValue(string(baseTOML), "approval_policy", "\"on-request\"")
	cleaned = filemerge.RemoveTopLevelTOMLKeyIfValue(cleaned, "default_permissions", "\"gentle-dev\"")
	for _, legacy := range codexLegacyPermissionValues {
		cleaned = filemerge.RemoveTOMLKeyIfValue(cleaned, legacy[0], legacy[1])
	}
	cleaned = filemerge.RemoveEmptyTOMLTableTree(cleaned, "permissions.gentle-dev")
	if cleaned == string(baseTOML) {
		return InjectionResult{}, nil
	}

	writeResult, err := filemerge.WriteFileAtomic(configPath, []byte(cleaned), 0o644)
	if err != nil {
		return InjectionResult{}, err
	}

	return InjectionResult{Changed: writeResult.Changed, Files: []string{configPath}}, nil
}

func mergeJSONFile(path string, overlay []byte) (filemerge.WriteResult, error) {
	baseJSON, err := osReadFile(path)
	if err != nil {
		return filemerge.WriteResult{}, err
	}

	merged, err := filemerge.MergeJSONObjects(baseJSON, overlay)
	if err != nil {
		return filemerge.WriteResult{}, err
	}

	return filemerge.WriteFileAtomic(path, merged, 0o644)
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
