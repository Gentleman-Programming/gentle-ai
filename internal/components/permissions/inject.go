package permissions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

var codexPermissionsGOOS = runtime.GOOS
var codexPermissionsLookPath = exec.LookPath

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
	if prefix, ok := existingLinuxbrewPrefix(); ok {
		readPaths = append(readPaths, strconv.Quote(prefix))
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

	if codexPermissionsGOOS == "windows" {
		merged = removeCodexHomeWorkspaceRoot(merged)
	} else {
		merged = filemerge.UpsertTOMLTableKey(merged, "permissions.gentle-dev.workspace_roots", `"~"`, "true")
	}

	writeResult, err := filemerge.WriteFileAtomic(configPath, []byte(merged), 0o644)
	if err != nil {
		return InjectionResult{}, err
	}

	return InjectionResult{Changed: writeResult.Changed, Files: []string{configPath}}, nil
}

func existingLinuxbrewPrefix() (string, bool) {
	if codexPermissionsGOOS != "linux" {
		return "", false
	}

	brewPath, err := codexPermissionsLookPath("brew")
	if err != nil || !filepath.IsAbs(brewPath) || filepath.Base(brewPath) != "brew" || filepath.Base(filepath.Dir(brewPath)) != "bin" {
		return "", false
	}

	prefix := filepath.Dir(filepath.Dir(brewPath))
	info, err := os.Stat(prefix)
	if err != nil || !info.IsDir() {
		return "", false
	}
	return prefix, true
}

func removeCodexHomeWorkspaceRoot(content string) string {
	targetPath := []string{"permissions", "gentle-dev", "workspace_roots", "~"}
	var tablePath []string
	lines := strings.SplitAfter(content, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if path, ok := parseTOMLTableHeader(line); ok {
			tablePath = path
			kept = append(kept, line)
			continue
		}
		keyPath, ok := parseTOMLAssignmentKey(line)
		if ok && equalTOMLKeyPath(append(tablePath, keyPath...), targetPath) {
			continue
		}
		kept = append(kept, line)
	}

	return strings.Join(kept, "")
}

func parseTOMLTableHeader(line string) ([]string, bool) {
	code := strings.TrimSpace(tomlCodeBeforeComment(line))
	if len(code) < 3 || code[0] != '[' {
		return nil, false
	}

	if strings.HasPrefix(code, "[[") {
		if len(code) < 5 || !strings.HasSuffix(code, "]]") {
			return nil, false
		}
		return parseTOMLKeyPath(code[2 : len(code)-2])
	}
	if !strings.HasSuffix(code, "]") {
		return nil, false
	}
	return parseTOMLKeyPath(code[1 : len(code)-1])
}

func parseTOMLAssignmentKey(line string) ([]string, bool) {
	code := tomlCodeBeforeComment(line)
	equals := tomlIndexOutsideQuotes(code, '=')
	if equals == -1 {
		return nil, false
	}
	return parseTOMLKeyPath(code[:equals])
}

func tomlCodeBeforeComment(line string) string {
	if comment := tomlIndexOutsideQuotes(line, '#'); comment != -1 {
		return line[:comment]
	}
	return line
}

func tomlIndexOutsideQuotes(text string, target byte) int {
	var quote byte
	escaped := false
	for i := 0; i < len(text); i++ {
		char := text[i]
		if quote == '"' && escaped {
			escaped = false
			continue
		}
		if quote == '"' && char == '\\' {
			escaped = true
			continue
		}
		if char == '"' || char == '\'' {
			if quote == 0 {
				quote = char
			} else if quote == char {
				quote = 0
			}
			continue
		}
		if quote == 0 && char == target {
			return i
		}
	}
	return -1
}

func parseTOMLKeyPath(text string) ([]string, bool) {
	var path []string
	for pos := 0; ; {
		pos = skipTOMLKeyWhitespace(text, pos)
		part, next, ok := parseTOMLKeyPart(text, pos)
		if !ok {
			return nil, false
		}
		path = append(path, part)
		pos = skipTOMLKeyWhitespace(text, next)
		if pos == len(text) {
			return path, true
		}
		if text[pos] != '.' {
			return nil, false
		}
		pos++
	}
}

func skipTOMLKeyWhitespace(text string, pos int) int {
	for pos < len(text) && (text[pos] == ' ' || text[pos] == '\t') {
		pos++
	}
	return pos
}

func parseTOMLKeyPart(text string, pos int) (string, int, bool) {
	if pos >= len(text) {
		return "", pos, false
	}
	if text[pos] == '\'' {
		if end := strings.IndexByte(text[pos+1:], '\''); end != -1 {
			return text[pos+1 : pos+1+end], pos + end + 2, true
		}
		return "", pos, false
	}
	if text[pos] == '"' {
		for end := pos + 1; end < len(text); end++ {
			if text[end] == '\\' {
				end++
				continue
			}
			if text[end] == '"' {
				value, err := strconv.Unquote(text[pos : end+1])
				return value, end + 1, err == nil
			}
		}
		return "", pos, false
	}

	end := pos
	for end < len(text) && isBareTOMLKeyByte(text[end]) {
		end++
	}
	if end == pos {
		return "", pos, false
	}
	return text[pos:end], end, true
}

func isBareTOMLKeyByte(char byte) bool {
	return char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '_' || char == '-'
}

func equalTOMLKeyPath(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
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
