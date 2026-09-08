package permissions

import (
	"fmt"
	"os"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
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
//
// The remote-shell deny entries (#4324) cover more than the bare command names:
// Claude Code prefix rules only match the literal start of the command string, so
// an agent can otherwise sidestep Bash(ssh:*) by invoking the interpreter-free
// utilities through an absolute path (/usr/bin/ssh ...), a backslash escape
// (\ssh ...), or a shell resolution wrapper (command ssh ..., exec ssh ...).
// The enumerated directories are the canonical install prefixes of the OpenSSH
// client and rsync across the supported platforms (FHS Linux /bin and /usr/bin,
// custom /usr/local/bin, Apple Silicon /opt/homebrew/bin, NixOS
// /run/current-system/sw/bin). Agents whose ssh lives elsewhere still hit the
// always-on routing guidance "Remote execution boundary" section.
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
      "Edit(**/secrets/*)",
      "Bash(ssh)",
      "Bash(ssh:*)",
      "Bash(/bin/ssh:*)",
      "Bash(/usr/bin/ssh:*)",
      "Bash(/usr/local/bin/ssh:*)",
      "Bash(/opt/homebrew/bin/ssh:*)",
      "Bash(/run/current-system/sw/bin/ssh:*)",
      "Bash(\\ssh:*)",
      "Bash(command ssh:*)",
      "Bash(exec ssh:*)",
      "Bash(scp)",
      "Bash(scp:*)",
      "Bash(/bin/scp:*)",
      "Bash(/usr/bin/scp:*)",
      "Bash(/usr/local/bin/scp:*)",
      "Bash(/opt/homebrew/bin/scp:*)",
      "Bash(/run/current-system/sw/bin/scp:*)",
      "Bash(\\scp:*)",
      "Bash(command scp:*)",
      "Bash(exec scp:*)",
      "Bash(sftp)",
      "Bash(sftp:*)",
      "Bash(/bin/sftp:*)",
      "Bash(/usr/bin/sftp:*)",
      "Bash(/usr/local/bin/sftp:*)",
      "Bash(/opt/homebrew/bin/sftp:*)",
      "Bash(/run/current-system/sw/bin/sftp:*)",
      "Bash(\\sftp:*)",
      "Bash(command sftp:*)",
      "Bash(exec sftp:*)",
      "Bash(rsync)",
      "Bash(rsync:*)",
      "Bash(/bin/rsync:*)",
      "Bash(/usr/bin/rsync:*)",
      "Bash(/usr/local/bin/rsync:*)",
      "Bash(/opt/homebrew/bin/rsync:*)",
      "Bash(/run/current-system/sw/bin/rsync:*)",
      "Bash(\\rsync:*)",
      "Bash(command rsync:*)",
      "Bash(exec rsync:*)"
    ]
  }
}
`)

// openCodeOverlayJSON uses the OpenCode "permission" key with bash/read granularity.
//
// The remote-shell deny entries (#4324) match OpenCode's evaluation semantics
// (v1.18.10 permission/index.ts): rules keep their insertion order, the last
// matching rule wins, and a command matches only the full source text of a
// parsed shell command node — so "*": "allow" must stay first and each deny
// needs its own surface form. A trailing " *" pattern also covers the bare
// invocation because OpenCode compiles "X *" to ^X( .*)?$ (util/wildcard.ts).
// Absolute paths get one entry per canonical install prefix; the "/X *"
// entries additionally cover backslash-escaped invocations (\ssh ...) because
// the matcher normalizes backslashes to forward slashes before matching.
var openCodeOverlayJSON = []byte(`{
  "permission": {
    "bash": {
      "*": "allow",
      "git commit *": "ask",
      "git push *": "ask",
      "git push": "ask",
      "git push --force *": "ask",
      "git rebase *": "ask",
      "git reset --hard *": "ask",
      "ssh": "deny",
      "ssh *": "deny",
      "/bin/ssh *": "deny",
      "/usr/bin/ssh *": "deny",
      "/usr/local/bin/ssh *": "deny",
      "/opt/homebrew/bin/ssh *": "deny",
      "/run/current-system/sw/bin/ssh *": "deny",
      "/ssh *": "deny",
      "command ssh *": "deny",
      "exec ssh *": "deny",
      "scp": "deny",
      "scp *": "deny",
      "/bin/scp *": "deny",
      "/usr/bin/scp *": "deny",
      "/usr/local/bin/scp *": "deny",
      "/opt/homebrew/bin/scp *": "deny",
      "/run/current-system/sw/bin/scp *": "deny",
      "/scp *": "deny",
      "command scp *": "deny",
      "exec scp *": "deny",
      "sftp": "deny",
      "sftp *": "deny",
      "/bin/sftp *": "deny",
      "/usr/bin/sftp *": "deny",
      "/usr/local/bin/sftp *": "deny",
      "/opt/homebrew/bin/sftp *": "deny",
      "/run/current-system/sw/bin/sftp *": "deny",
      "/sftp *": "deny",
      "command sftp *": "deny",
      "exec sftp *": "deny",
      "rsync": "deny",
      "rsync *": "deny",
      "/bin/rsync *": "deny",
      "/usr/bin/rsync *": "deny",
      "/usr/local/bin/rsync *": "deny",
      "/opt/homebrew/bin/rsync *": "deny",
      "/run/current-system/sw/bin/rsync *": "deny",
      "/rsync *": "deny",
      "command rsync *": "deny",
      "exec rsync *": "deny"
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
	settingsPath := TargetPath(homeDir, adapter)
	if settingsPath == "" {
		return InjectionResult{}, nil
	}

	overlay := agentOverlay(adapter.Agent())
	if overlay == nil {
		return InjectionResult{}, nil
	}

	defaults := adapter.Agent() == model.AgentOpenCode || adapter.Agent() == model.AgentKilocode
	writeResult, err := mergeJSONFile(settingsPath, overlay, defaults)
	if err != nil {
		return InjectionResult{}, err
	}

	return InjectionResult{Changed: writeResult.Changed, Files: []string{settingsPath}}, nil
}

func mergeJSONFile(path string, overlay []byte, defaults bool) (filemerge.WriteResult, error) {
	baseJSON, err := osReadFile(path)
	if err != nil {
		return filemerge.WriteResult{}, err
	}

	merge := filemerge.MergeJSONObjectsForPath
	if defaults {
		merge = filemerge.MergeJSONDefaultsForPath
	}
	merged, err := merge(path, baseJSON, overlay)
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
