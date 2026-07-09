package permissions

import (
	"fmt"
	"os"
	"runtime"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

var codexPermissionsGOOS = runtime.GOOS

type InjectionResult struct {
	Changed bool
	Files   []string
}

// TargetPath returns the file path that permission injection creates or updates
// for the adapter, or an empty string when the agent has no supported
// permission injection target.
func TargetPath(homeDir string, adapter agents.Adapter) string {
	if adapter.Agent() == model.AgentCodex {
		return adapter.MCPConfigPath(homeDir, "")
	}
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
		// Codex has no known settings.json path; permissions are skipped.
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

func injectCodexPermissions(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
	configPath := adapter.MCPConfigPath(homeDir, "")
	baseTOML, err := osReadFile(configPath)
	if err != nil {
		return InjectionResult{}, err
	}

	merged := filemerge.UpsertTopLevelTOMLString(string(baseTOML), "approval_policy", "on-request")
	merged = filemerge.UpsertTopLevelTOMLString(merged, "default_permissions", "gentle-dev")
	merged = filemerge.RemoveTOMLTableKeys(merged, "permissions.gentle-dev", []string{"extends"})
	merged = filemerge.UpsertTOMLTableKey(merged, "permissions.gentle-dev", "description", `"Comfortable local development profile with workspace writes, network access, and read-only access to Git and Nix/Home Manager metadata."`)
	merged = filemerge.RemoveTOMLTableKeys(merged, "permissions.gentle-dev", []string{"glob_scan_max_depth"})
	merged = filemerge.UpsertTOMLTableKey(merged, "permissions.gentle-dev.network", "enabled", "true")
	merged = filemerge.UpsertTOMLTableKey(merged, "permissions.gentle-dev.network.domains", `"*"`, `"allow"`)

	merged = filemerge.RemoveTOMLTableKeys(merged, `permissions.gentle-dev.filesystem.":root"`, []string{`"."`})
	secretDenyPaths := []string{
		`"**/.env"`,
		`"**/.env.local"`,
		`"**/.env.*.local"`,
		`"**/*.pem"`,
		`"**/*.key"`,
		`"**/secrets/**"`,
		`"**/.ssh/**"`,
		`"**/.credentials/**"`,
		`"**/credentials.json"`,
		`"**/.aws/credentials"`,
		`"**/.config/gh/hosts.yml"`,
	}
	merged = filemerge.RemoveTOMLTableKeys(merged, "permissions.gentle-dev.filesystem", secretDenyPaths)
	merged = filemerge.RemoveTOMLTableKeys(merged, `permissions.gentle-dev.filesystem.":workspace_roots"`, []string{
		`"**/.git"`,
		`"**/.git/**"`,
		`"**/.env.*"`,
		`"*.env.*"`,
		`"**/secrets/*"`,
	})

	readPaths := []string{
		`":minimal"`,
		`"~/.config/git"`,
		`"~/.gitconfig"`,
		`"~/.local/state/nix/profiles/home-manager/home-path"`,
		`"~/.nix-profile"`,
	}
	if codexPermissionsGOOS != "windows" {
		readPaths = append(readPaths, `"/nix/store"`)
	}
	for _, path := range readPaths {
		merged = filemerge.UpsertTOMLTableKey(merged, "permissions.gentle-dev.filesystem", path, `"read"`)
	}
	for _, path := range []string{`":tmpdir"`, `":slash_tmp"`} {
		merged = filemerge.UpsertTOMLTableKey(merged, "permissions.gentle-dev.filesystem", path, `"write"`)
	}
	merged = filemerge.UpsertTOMLTableKey(merged, "permissions.gentle-dev.filesystem", "glob_scan_max_depth", "6")
	for _, path := range []string{`"."`, `".git/**"`} {
		merged = filemerge.UpsertTOMLTableKey(merged, `permissions.gentle-dev.filesystem.":workspace_roots"`, path, `"write"`)
	}
	for _, path := range secretDenyPaths {
		merged = filemerge.UpsertTOMLTableKey(merged, `permissions.gentle-dev.filesystem.":workspace_roots"`, path, `"deny"`)
	}

	merged = filemerge.UpsertTOMLTableKey(merged, "permissions.gentle-dev.workspace_roots", `"~"`, "true")

	writeResult, err := filemerge.WriteFileAtomic(configPath, []byte(merged), 0o644)
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
