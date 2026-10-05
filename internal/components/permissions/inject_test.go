package permissions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/antigravity"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/claude"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/codex"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/cursor"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/gemini"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/hermes"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/kilocode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/vscode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

func TestSelectedPermissionsRefuseNestedCommentsAndLockedMode(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		mode          os.FileMode
	}{
		{"nested comments", "{\"permission\":{\"bash\":{// keep\n\"*\":\"deny\"}}}\n", 0o600},
		{"locked mode", "{\"permission\":{}}\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if runtime.GOOS == "windows" && tc.mode != 0o600 {
				t.Skip("file permission bits are not supported on Windows")
			}
			path := filepath.Join(t.TempDir(), "opencode.jsonc")
			original := []byte(tc.content)
			if err := os.WriteFile(path, original, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, tc.mode); err != nil {
				t.Fatal(err)
			}
			_, err := InjectAtPath(path, opencode.NewAdapter())
			if err == nil || !strings.Contains(err.Error(), "refuse") {
				t.Fatalf("want actionable refusal, got %v", err)
			}
			info, statErr := os.Stat(path)
			if statErr != nil {
				t.Fatal(statErr)
			}
			if runtime.GOOS != "windows" && info.Mode().Perm() != tc.mode {
				t.Fatalf("settings mode changed: %04o", info.Mode().Perm())
			}
			if err := os.Chmod(path, 0o600); err != nil {
				t.Fatal(err)
			}
			got, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(got) != tc.content {
				t.Fatalf("settings bytes changed: %q", got)
			}
		})
	}
}

// The selected-settings refusal is OpenCode-only; other agents keep the base
// writer behavior for a dotfiles-managed (symlinked) settings file.
func TestNonOpenCodeSymlinkedPermissionsKeepBaseWriterBehavior(t *testing.T) {
	home := t.TempDir()
	adapter := claudeAdapter()
	settings := TargetPath(home, adapter)
	target := filepath.Join(home, "dotfiles", "settings.json")
	original := []byte("{\"permissions\":{}}\n")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, settings); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, err := Inject(home, adapter)
	if err == nil || !strings.Contains(err.Error(), "refusing to read symlink") || strings.Contains(err.Error(), "select a regular settings file") {
		t.Fatalf("Inject() error = %v; want base writer symlink error, not the OpenCode refusal", err)
	}
	if link, err := os.Readlink(settings); err != nil || link != target {
		t.Fatalf("settings symlink changed: %q, %v", link, err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != string(original) {
		t.Fatalf("settings target changed: %q, %v", got, err)
	}
}

func TestSelectedPermissionsRefuseSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "user.jsonc")
	selected := filepath.Join(dir, "opencode.jsonc")
	if err := os.WriteFile(target, []byte("{\"permission\":{}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, selected); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := InjectAtPath(selected, opencodeAdapter()); err == nil || !strings.Contains(err.Error(), "select a regular settings file") {
		t.Fatalf("InjectAtPath() error = %v; want OpenCode symlink refusal", err)
	}
}

func TestSelectedPermissionsPreservePrivateMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.jsonc")
	if err := os.WriteFile(path, []byte("{\"permission\":{}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := InjectAtPath(path, opencode.NewAdapter()); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		return // POSIX permission bits are not preserved on Windows.
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("settings mode = %v, error = %v; want 0600", info, err)
	}
}

func TestSelectedPermissionsNeverWidenLockedMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission bits are not supported on Windows")
	}
	path := filepath.Join(t.TempDir(), "opencode.json")
	if err := os.WriteFile(path, []byte(`{"permission":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	_, _ = InjectAtPath(path, opencode.NewAdapter()) // Refusal is safe when unreadable.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0 && mode != 0o600 {
		t.Fatalf("locked settings widened: mode = %04o", mode)
	}
}

// Fixture port of OpenCode v1.2.27 util/wildcard.ts and permission/service.ts:
// anchored, case-sensitive POSIX matching, optional trailing " *", last match wins.
// Inputs are strings only; this does not emulate bash.ts's tree-sitter extraction.
func remoteAction(t *testing.T, raw []byte, command string) string {
	t.Helper()
	return permissionAction(t, raw, "bash", command)
}

// permissionAction resolves one OpenCode permission request the way OpenCode
// does: rules are wildcard patterns (`*` spans any characters, `?` one), and
// the last matching rule wins. A trailing " *" also matches the bare command.
func permissionAction(t *testing.T, raw []byte, tool, input string) string {
	t.Helper()
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	var scalar string
	if json.Unmarshal(root["permission"], &scalar) == nil {
		return scalar
	}
	var permission map[string]json.RawMessage
	if err := json.Unmarshal(root["permission"], &permission); err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(permission[tool], &scalar) == nil {
		return scalar
	}
	if permission[tool] == nil {
		return "allow"
	}
	decoder := json.NewDecoder(strings.NewReader(string(permission[tool])))
	_, _ = decoder.Token()
	action := "ask"
	for decoder.More() {
		key, _ := decoder.Token()
		var value string
		if err := decoder.Decode(&value); err != nil {
			t.Fatal(err)
		}
		pattern := regexp.QuoteMeta(strings.ReplaceAll(key.(string), `\`, "/"))
		pattern = strings.ReplaceAll(strings.ReplaceAll(pattern, `\*`, ".*"), `\?`, ".")
		if strings.HasSuffix(pattern, " .*") {
			pattern = strings.TrimSuffix(pattern, " .*") + "( .*)?"
		}
		if regexp.MustCompile("(?s)^" + pattern + "$").MatchString(strings.ReplaceAll(input, `\`, "/")) {
			action = value
		}
	}
	return action
}

// TestOpenCodePermissionsMirrorPiSafetyModel pins the Pi safety model for
// OpenCode and Kilocode: everything is allowed by design, recognized
// destructive commands are denied (Pi hard deny) or confirmed (Pi confirm),
// remote commands keep their approval (#4324), and secret-bearing paths cannot
// be read, written, or edited.
func TestOpenCodePermissionsMirrorPiSafetyModel(t *testing.T) {
	cases := []struct {
		tool, input, want string
	}{
		// Allowed by design.
		{"bash", "git status", "allow"},
		{"bash", "git commit -m fix", "allow"},
		{"bash", "go test ./...", "allow"},
		{"bash", "rm notes.txt", "allow"},
		{"bash", "git reset HEAD~1", "allow"},
		// Pi hard deny.
		{"bash", "rm -rf /", "deny"},
		{"bash", "rm -rf ~", "deny"},
		{"bash", "rm -rf ~/projects", "deny"},
		{"bash", "rm -rf $HOME", "deny"},
		{"bash", "rm -rf $HOME/projects", "deny"},
		{"bash", "rm -rf ${HOME}", "deny"},
		{"bash", "rm -rf .", "deny"},
		{"bash", "rm -rf ..", "deny"},
		{"bash", "rm -fr /", "deny"},
		{"bash", "rm -r ~", "deny"},
		{"bash", "git reset --hard", "deny"},
		{"bash", "git reset --hard HEAD~1", "deny"},
		{"bash", "git clean -fd", "deny"},
		{"bash", "git clean -fdx", "deny"},
		{"bash", "git push --force", "deny"},
		{"bash", "git push --force-with-lease origin main", "deny"},
		{"bash", "git push origin main --force", "deny"},
		{"bash", "git push -f origin main", "deny"},
		{"bash", "chmod -R 777 .", "deny"},
		{"bash", "chown -R me:me .", "deny"},
		// Hard-deny forms found by independent verification.
		{"bash", "git clean -f", "deny"},
		{"bash", "git clean -fx", "deny"},
		{"bash", "git clean -xf", "deny"},
		{"bash", "git clean --force", "deny"},
		{"bash", "git reset HEAD~1 --hard", "deny"},
		{"bash", "git reset -q --hard", "deny"},
		{"bash", "rm -Rf ./", "deny"},
		{"bash", "rm -fR ../", "deny"},
		{"bash", "rm -rfv ~", "deny"},
		{"bash", "rm -vrf /", "deny"},
		{"bash", "rm -Rfv .", "deny"},
		{"bash", "rm -rf build /", "deny"},
		{"bash", `rm -rf "$HOME"`, "deny"},
		{"bash", "sudo rm -rf /", "deny"},
		{"bash", "git -C repo push --force", "deny"},
		{"bash", "git -C repo reset --hard", "deny"},
		{"bash", "git push -uf origin main", "deny"},
		{"bash", "git clean -xdf", "deny"},
		{"bash", "git clean -x -f", "deny"},
		{"bash", "sudo rm -R /", "deny"},
		{"bash", "sudo rm -fR /", "deny"},
		{"bash", "rm -frv ~", "deny"},
		{"bash", "rm -rvf /", "deny"},
		// Forms that must stay allowed.
		{"bash", "git clean -n", "allow"},
		{"bash", "git clean -n -- fixtures/", "allow"},
		{"bash", "git clean -n -e '*.conf'", "allow"},
		{"bash", "git clean -n src/foo", "allow"},
		{"bash", "rm -f report.txt", "allow"},
		// Pi confirm.
		{"bash", "rm -rfv build", "ask"},
		{"bash", "git -C repo push", "ask"},
		{"bash", "git push --follow-tags", "ask"},
		{"bash", `psql -c "DROP TABLE users"`, "ask"},
		{"bash", "rm -rf build", "ask"},
		{"bash", "rm -r dist", "ask"},
		{"bash", "find . -name '*.tmp' -delete", "ask"},
		{"bash", "git push", "ask"},
		{"bash", "git push origin main", "ask"},
		{"bash", "git rebase main", "ask"},
		{"bash", "git branch -D feature", "ask"},
		{"bash", "npm publish", "ask"},
		// Remote shell utilities are denied by default (#4324).
		{"bash", "ssh example.invalid", "deny"},
		{"bash", "rsync source destination", "deny"},
		// Ordinary files stay readable and editable.
		{"read", "src/main.go", "allow"},
		{"edit", "src/main.go", "allow"},
		{"read", "go.mod", "allow"},
	}
	// Secret-bearing paths: read and edit (edit covers write and patch).
	for _, path := range []string{".env", "app/.env", ".env.local", "app/.env.production", ".env_backup", "secrets/token", "app/secrets/db.json", "/home/u/.ssh/id_ed25519", ".ssh/config", "server.pem", "keys/tls.key", "cert.p12", "cert.pfx", "/home/u/.aws/credentials", "/home/u/.config/gh/hosts.yml", "/home/u/.config/gh/hosts.yaml", "/home/u/.credentials/token", "/Users/u/Library/Keychains/login.keychain-db", "/home/u/.ssh", "secrets", ".aws/credentials", ".config/gh/hosts.yml"} {
		cases = append(cases, struct{ tool, input, want string }{"read", path, "deny"}, struct{ tool, input, want string }{"edit", path, "deny"})
	}
	for _, id := range []model.AgentID{model.AgentOpenCode, model.AgentKilocode} {
		t.Run(string(id), func(t *testing.T) {
			home := t.TempDir()
			adapter, _ := agents.NewAdapter(id)
			if _, err := Inject(home, adapter); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(adapter.SettingsPath(home))
			if err != nil {
				t.Fatal(err)
			}
			for _, tc := range cases {
				if got := permissionAction(t, raw, tc.tool, tc.input); got != tc.want {
					t.Errorf("%s %q: got %s, want %s", tc.tool, tc.input, got, tc.want)
				}
			}
		})
	}
}

// Upgrading a settings file written by the previous overlay keeps existing
// rules authoritative (the writer never lets new rules override them), so a
// legacy "git push *": "ask" still governs a force push. Nothing destructive
// may resolve to allow, and secret-bearing paths become uneditable.
func TestOpenCodePermissionsUpgradeFromPreviousOverlay(t *testing.T) {
	previous := `{"permission":{"bash":{"*":"allow","git commit *":"ask","git push *":"ask","git push":"ask","git push --force *":"ask","git rebase *":"ask","git reset --hard *":"ask","ssh":"ask","ssh *":"ask"},"read":{"*":"allow","*.env":"deny","**/*.pem":"deny"}}}`
	home := t.TempDir()
	adapter, _ := agents.NewAdapter(model.AgentOpenCode)
	path := adapter.SettingsPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(previous), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Inject(home, adapter); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"git push --force", "git push -f origin main", "git reset --hard HEAD~1", "rm -rf ~", "git clean -fd", "chown -R me:me ."} {
		if got := permissionAction(t, raw, "bash", command); got == "allow" {
			t.Errorf("upgraded bash %q resolved to allow", command)
		}
	}
	for _, path := range []string{".env", "server.pem", "/home/u/.ssh/id_ed25519"} {
		if got := permissionAction(t, raw, "edit", path); got != "deny" {
			t.Errorf("upgraded edit %q = %s, want deny", path, got)
		}
	}
}

func TestRemoteMatcherBoundaryFixtures(t *testing.T) {
	// #4324 deny policy: absolute-path, backslash-escape, and shell resolution
	// wrapper invocations of the remote shell utilities are denied by the
	// overlay itself. Pattern-only rules are bypassable through these forms
	// (the command token is not the bare utility name), so the deny entries
	// enumerate them explicitly instead of relying on guidance alone.
	for _, input := range []string{"/usr/bin/ssh example.invalid", "/bin/scp file example.invalid:file", "\\ssh example.invalid", "command ssh example.invalid", "exec rsync -a src dst"} {
		if got := remoteAction(t, openCodeOverlayJSON, input); got != "deny" {
			t.Errorf("bypass invocation %q not denied: %s", input, got)
		}
	}
	// These remain matcher inputs, not shell programs. bash.ts extracts command
	// nodes separately; no claim is made about parsing arbitrary shell syntax.
	for _, input := range []string{"env ssh example.invalid", "true && ssh example.invalid", `python -c 'import subprocess'`} {
		if got := remoteAction(t, openCodeOverlayJSON, input); got != "allow" {
			t.Errorf("unsupported matcher input %q unexpectedly intercepted: %s", input, got)
		}
	}
}

func TestRemoteCommandApprovalDefaults(t *testing.T) {
	for _, id := range []model.AgentID{model.AgentOpenCode, model.AgentKilocode} {
		for _, seed := range []string{`{}`, `{"permission":{"bash":{"*":"allow"}}}`, `{"permission":{"bash":{"*":"allow","ssh*":"deny"}}}`, `{"permission":"deny"}`, `{"permission":{"bash":"ask"}}`, `{"permission":{"bash":"deny"}}`, `{"permission":{"bash":{"*":"deny"}}}`, `{"permission":{"bash":{"*":"allow","ssh *":"deny"}}}`, `{"permission":{"bash":{"*":"allow","ssh*":"allow"}}}`} {
			t.Run(string(id)+seed, func(t *testing.T) {
				home := t.TempDir()
				adapter, _ := agents.NewAdapter(id)
				path := adapter.SettingsPath(home)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
					t.Fatal(err)
				}
				if _, err := Inject(home, adapter); err != nil {
					t.Fatal(err)
				}
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				for _, command := range []string{"ssh", "ssh example.invalid", "scp", "scp file example.invalid:file", "sftp", "sftp example.invalid", "rsync", "rsync source destination"} {
					// #4324 deny policy: the overlay denies the remote shell
					// utilities by default instead of asking per command; a human
					// who wants them re-enables them explicitly in their own config.
					want := "deny"
					if seed == `{"permission":{"bash":"ask"}}` {
						want = "ask" // Explicit personal scalar ask is not silently rewritten.
					}
					if strings.Contains(seed, `"ssh*":"allow"`) && strings.HasPrefix(command, "ssh") {
						want = "allow" // Explicit personal allow is not silently rewritten.
					}
					if got := remoteAction(t, raw, command); got != want {
						t.Errorf("%q: got %s, want %s", command, got, want)
					}
				}
				if result, err := Inject(home, adapter); err != nil || result.Changed {
					t.Fatalf("repeat injection = %+v, %v", result, err)
				}
			})
		}
	}
}

// remoteShellTools lists the remote shell utilities the overlay denies to keep
// agents inside the workspace boundary (#4324).
var remoteShellTools = []string{"ssh", "scp", "sftp", "rsync"}

var remoteShellPathPrefixes = []string{
	"/bin/",
	"/usr/bin/",
	"/usr/local/bin/",
	"/opt/homebrew/bin/",
	"/run/current-system/sw/bin/",
}

// remoteShellEscapeForms returns every deny surface one tool needs beyond the
// bare name: one entry per absolute install prefix, the root-level absolute
// form (which OpenCode's matcher also reaches for backslash-escaped
// invocations because it normalizes backslashes to forward slashes), and the
// shell resolution wrappers.
func remoteShellEscapeForms(tool, openCodeSuffix, claudeCodeSuffix string) (openCode []string, claudeCode []string) {
	for _, prefix := range remoteShellPathPrefixes {
		openCode = append(openCode, prefix+tool+openCodeSuffix)
		claudeCode = append(claudeCode, "Bash("+prefix+tool+claudeCodeSuffix+")")
	}
	openCode = append(openCode, "/"+tool+openCodeSuffix, "command "+tool+openCodeSuffix, "exec "+tool+openCodeSuffix)
	claudeCode = append(claudeCode, "Bash(\\"+tool+claudeCodeSuffix+")", "Bash(command "+tool+claudeCodeSuffix+")", "Bash(exec "+tool+claudeCodeSuffix+")")
	return openCode, claudeCode
}

// TestInjectOpenCodeDeniesRemoteShellUtilities verifies that the remote shell
// utilities used to reach machines outside the workspace (ssh, scp, sftp,
// rsync) are denied in the OpenCode/Kilocode bash permission map in their
// exact and wildcard forms — including absolute-path invocations, which
// OpenCode's full-command matcher treats as distinct patterns — and that
// these deny entries coexist with the global bash allow (#4324).
func TestInjectOpenCodeDeniesRemoteShellUtilities(t *testing.T) {
	remoteDenyRules := []string{
		"ssh",
		"ssh *",
		"scp",
		"scp *",
		"sftp",
		"sftp *",
		"rsync",
		"rsync *",
	}
	for _, tool := range remoteShellTools {
		openCode, _ := remoteShellEscapeForms(tool, " *", " *")
		remoteDenyRules = append(remoteDenyRules, openCode...)
	}

	tests := []struct {
		name    string
		adapter agents.Adapter
	}{
		{"opencode", opencodeAdapter()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			if _, err := Inject(home, tt.adapter); err != nil {
				t.Fatalf("Inject() error = %v", err)
			}

			settingsPath := tt.adapter.SettingsPath(home)
			content, err := os.ReadFile(settingsPath)
			if err != nil {
				t.Fatalf("read settings file %q: %v", settingsPath, err)
			}

			var settings map[string]any
			if err := json.Unmarshal(content, &settings); err != nil {
				t.Fatalf("unmarshal settings json: %v", err)
			}

			permNode, ok := settings["permission"].(map[string]any)
			if !ok {
				t.Fatalf("permission node missing or invalid: %#v", settings["permission"])
			}

			bashNode, ok := permNode["bash"].(map[string]any)
			if !ok {
				t.Fatalf("bash node missing or invalid: %#v", permNode["bash"])
			}

			for _, pattern := range remoteDenyRules {
				t.Run(pattern, func(t *testing.T) {
					val, exists := bashNode[pattern]
					if !exists {
						t.Errorf("bash deny map missing pattern %q; got: %v", pattern, bashNode)
						return
					}
					if val != "deny" {
						t.Errorf("pattern %q has value %q, want %q", pattern, val, "deny")
					}
				})
			}

			// The remote denials must coexist with the global bash allow.
			if bashNode["*"] != "allow" {
				t.Errorf("global bash allow rule \"*\" missing or changed after Inject; got: %v", bashNode["*"])
			}
		})
	}
}

// TestInjectClaudeCodeDeniesRemoteShellUtilities verifies that the remote shell
// utilities used to reach machines outside the workspace (ssh, scp, sftp,
// rsync) are denied in the Claude Code deny list in their exact and wildcard
// command forms, plus the absolute-path and shell resolution prefix forms
// that bypass a bare-command prefix rule (#4324).
func TestInjectClaudeCodeDeniesRemoteShellUtilities(t *testing.T) {
	remoteDenyRules := []string{
		"Bash(ssh)",
		"Bash(ssh:*)",
		"Bash(scp)",
		"Bash(scp:*)",
		"Bash(sftp)",
		"Bash(sftp:*)",
		"Bash(rsync)",
		"Bash(rsync:*)",
	}
	for _, tool := range remoteShellTools {
		_, claudeCode := remoteShellEscapeForms(tool, " *", ":*")
		remoteDenyRules = append(remoteDenyRules, claudeCode...)
	}

	home := t.TempDir()
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	// Pre-existing settings with a sibling key under permissions (not deny),
	// mirroring the default-deny test: the remote denials must land even when
	// a permissions block is already present.
	existing := `{\n  "permissions": {\n    "defaultMode": "default"\n  }\n}`
	if err := os.WriteFile(settingsPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := Inject(home, claudeAdapter()); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings file: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(content, &settings); err != nil {
		t.Fatalf("unmarshal settings json: %v", err)
	}
	perms, ok := settings["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("permissions node missing or invalid: %#v", settings["permissions"])
	}
	denyList, ok := perms["deny"].([]any)
	if !ok {
		t.Fatalf("deny list missing or invalid: %#v", perms["deny"])
	}
	denySet := make(map[string]bool)
	for _, entry := range denyList {
		if v, ok := entry.(string); ok {
			denySet[v] = true
		}
	}

	for _, rule := range remoteDenyRules {
		if !denySet[rule] {
			t.Errorf("remote shell deny rule %q was not present; got: %v", rule, denyList)
		}
	}

	// The overlay wins for defaultMode because arrays replace but maps deep-merge.
	mode, _ := perms["defaultMode"].(string)
	if mode != "bypassPermissions" {
		t.Errorf("expected defaultMode=bypassPermissions after overlay, got %q", mode)
	}
}

// TestInjectClaudeCodeRemoteShellDenyIdempotent verifies that calling Inject
// twice with the Claude adapter does not duplicate any remote-shell deny rule
// and reports Changed == false on the second pass (#4324).
func TestInjectClaudeCodeRemoteShellDenyIdempotent(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	first, err := Inject(home, claudeAdapter())
	if err != nil {
		t.Fatalf("first Inject() error = %v", err)
	}
	if !first.Changed {
		t.Fatalf("first Inject() should report Changed = true")
	}

	second, err := Inject(home, claudeAdapter())
	if err != nil {
		t.Fatalf("second Inject() error = %v", err)
	}
	if second.Changed {
		t.Errorf("second Inject() should report Changed = false, got true")
	}

	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings file: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(content, &settings); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	perms, ok := settings["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("permissions node missing")
	}
	denyList, ok := perms["deny"].([]any)
	if !ok {
		t.Fatalf("deny list missing")
	}

	// Count every remote-shell deny rule and assert each appears exactly once.
	seen := make(map[string]int)
	for _, entry := range denyList {
		if v, ok := entry.(string); ok {
			for _, tool := range remoteShellTools {
				if strings.Contains(v, tool) {
					seen[v]++
					break
				}
			}
		}
	}
	if len(seen) == 0 {
		t.Fatalf("no remote-shell deny rules found in deny list")
	}
	for rule, count := range seen {
		if count > 1 {
			t.Errorf("remote shell deny rule %q appears %d times, want 1", rule, count)
		}
	}
}

func claudeAdapter() agents.Adapter      { return claude.NewAdapter() }
func opencodeAdapter() agents.Adapter    { return opencode.NewAdapter() }
func geminiAdapter() agents.Adapter      { return gemini.NewAdapter() }
func cursorAdapter() agents.Adapter      { return cursor.NewAdapter() }
func vscodeAdapter() agents.Adapter      { return vscode.NewAdapter() }
func codexAdapter() agents.Adapter       { return codex.NewAdapter() }
func antigravityAdapter() agents.Adapter { return antigravity.NewAdapter() }
func hermesAdapter() agents.Adapter      { return hermes.NewAdapter() }

// codexInjectedLegacyConfig mirrors a config.toml produced by the retired
// gentle-dev permission profile injection, wrapped in user-authored content.
const codexInjectedLegacyConfig = `model = "gpt-5.5"
approval_policy = "on-request"
default_permissions = "gentle-dev"

[permissions.gentle-dev]
description = "Comfortable local development profile with workspace writes, network access, and read-only access to Git and Nix/Home Manager metadata."

[permissions.gentle-dev.network]
enabled = true

[permissions.gentle-dev.network.domains]
"*" = "allow"

[permissions.gentle-dev.filesystem]
glob_scan_max_depth = 6
":minimal" = "read"
"~/.config/git" = "read"
"~/.gitconfig" = "read"
"~/.local/state/nix/profiles/home-manager/home-path" = "read"
"~/.nix-profile" = "read"
"/nix/store" = "read"
":tmpdir" = "write"
":slash_tmp" = "write"

[permissions.gentle-dev.filesystem.":root"]
"." = "read"

[permissions.gentle-dev.filesystem.":workspace_roots"]
"." = "write"
".git/**" = "write"
"**/.env" = "deny"
"**/*.pem" = "deny"
"**/*.key" = "deny"

[permissions.gentle-dev.workspace_roots]
"~" = true

[mcp_servers.engram]
command = "engram"
args = ["mcp", "--tools=agent"]
`

func writeCodexConfig(t *testing.T, home, content string) string {
	t.Helper()
	configPath := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return configPath
}

// TestInjectHermesSkipsPermissions verifies that Hermes returns nil (no file written)
// because Hermes permission format is undocumented — §14 of spec.
func TestInjectHermesSkipsPermissions(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, hermesAdapter())
	if err != nil {
		t.Fatalf("Inject(hermes) error = %v", err)
	}
	if result.Changed {
		t.Fatal("Inject(hermes) changed = true, want false (no file should be written)")
	}
	if len(result.Files) != 0 {
		t.Fatalf("Inject(hermes) files = %v, want [] (no file should be written)", result.Files)
	}

	// Confirm no config.yaml or settings file was created.
	hermesDir := filepath.Join(home, ".hermes")
	if _, err := os.Stat(hermesDir); err == nil {
		t.Fatal("Inject(hermes) created ~/.hermes directory, want no files written")
	}
}

func TestInjectOpenCodeIsIdempotent(t *testing.T) {
	home := t.TempDir()

	first, err := Inject(home, opencodeAdapter())
	if err != nil {
		t.Fatalf("Inject() first error = %v", err)
	}
	if !first.Changed {
		t.Fatalf("Inject() first changed = false")
	}

	second, err := Inject(home, opencodeAdapter())
	if err != nil {
		t.Fatalf("Inject() second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("Inject() second changed = true")
	}

	path := filepath.Join(home, ".config", "opencode", "opencode.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected config file %q: %v", path, err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}

	text := string(content)
	if !strings.Contains(text, `"permission"`) {
		t.Fatal("opencode.json missing permission key")
	}
	if strings.Contains(text, `"permissions"`) {
		t.Fatal("opencode.json should use 'permission' (singular), not 'permissions'")
	}
	if !strings.Contains(text, `"bash"`) {
		t.Fatal("opencode.json permission missing bash section")
	}
	if !strings.Contains(text, `"read"`) {
		t.Fatal("opencode.json permission missing read section")
	}
}

func TestInjectAddsEnvToDenyList(t *testing.T) {
	home := t.TempDir()

	if _, err := Inject(home, claudeAdapter()); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings file %q: %v", settingsPath, err)
	}

	var settings map[string]any
	if err := json.Unmarshal(content, &settings); err != nil {
		t.Fatalf("unmarshal settings json: %v", err)
	}

	permissionsNode, ok := settings["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("permissions node missing or invalid: %#v", settings["permissions"])
	}

	denyList, ok := permissionsNode["deny"].([]any)
	if !ok {
		t.Fatalf("deny list missing or invalid: %#v", permissionsNode["deny"])
	}

	for _, entry := range denyList {
		if value, ok := entry.(string); ok && value == "Read(.env)" {
			return
		}
	}

	t.Fatalf("deny list missing explicit .env rule: %#v", denyList)
}

func TestInjectClaudeCodeUsesBypassPermissions(t *testing.T) {
	home := t.TempDir()

	if _, err := Inject(home, claudeAdapter()); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings file: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(content, &settings); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	perms, ok := settings["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("permissions node missing")
	}

	mode, ok := perms["defaultMode"].(string)
	if !ok || mode != "bypassPermissions" {
		t.Fatalf("expected defaultMode=bypassPermissions, got %q", mode)
	}
}

func TestInjectGeminiCLIUsesAutoEditMode(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, geminiAdapter())
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject() changed = false")
	}

	settingsPath := filepath.Join(home, ".gemini", "settings.json")
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings file: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(content, &settings); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	general, ok := settings["general"].(map[string]any)
	if !ok {
		t.Fatalf("general node missing: %#v", settings)
	}

	mode, ok := general["defaultApprovalMode"].(string)
	if !ok || mode != "auto_edit" {
		t.Fatalf("expected defaultApprovalMode=auto_edit, got %q", mode)
	}

	// Ensure no Claude Code keys leaked
	if _, exists := settings["permissions"]; exists {
		t.Fatal("gemini settings should not contain 'permissions' key")
	}
}

func TestInjectVSCodeCopilotUsesAutoApprove(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))

	adapter := vscodeAdapter()
	result, err := Inject(home, adapter)
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject() changed = false")
	}

	settingsPath := adapter.SettingsPath(home)
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings file: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(content, &settings); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	autoApprove, ok := settings["chat.tools.autoApprove"].(bool)
	if !ok || !autoApprove {
		t.Fatalf("expected chat.tools.autoApprove=true, got %v", settings["chat.tools.autoApprove"])
	}

	// Ensure no Claude Code keys leaked
	if _, exists := settings["permissions"]; exists {
		t.Fatal("vscode settings should not contain 'permissions' key")
	}
}

func TestInjectVSCodeCopilotMergesIntoJSONCSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))

	adapter := vscodeAdapter()
	settingsPath := adapter.SettingsPath(home)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	baseSettings := `{
	  // User has comments and trailing commas in VS Code settings
	  "editor.formatOnSave": true,
	  "files.exclude": {
	    "**/.git": true,
	  },
	}
`
	if err := os.WriteFile(settingsPath, []byte(baseSettings), 0o644); err != nil {
		t.Fatalf("WriteFile(settings.json) error = %v", err)
	}

	result, err := Inject(home, adapter)
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject() changed = false")
	}

	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings file: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(content, &settings); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	autoApprove, ok := settings["chat.tools.autoApprove"].(bool)
	if !ok || !autoApprove {
		t.Fatalf("expected chat.tools.autoApprove=true, got %v", settings["chat.tools.autoApprove"])
	}

	if settings["editor.formatOnSave"] != true {
		t.Fatalf("expected editor.formatOnSave=true, got %v", settings["editor.formatOnSave"])
	}
}

func TestInjectCursorSkipsPermissions(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, cursorAdapter())
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if result.Changed {
		t.Fatal("Inject() for Cursor should not change anything (permissions via cli-config.json)")
	}
	if len(result.Files) != 0 {
		t.Fatalf("Inject() for Cursor should return no files, got %v", result.Files)
	}
}

func TestInjectAntigravitySkipsPermissions(t *testing.T) {
	overlay := agentOverlay(model.AgentAntigravity)
	if overlay != nil {
		t.Errorf("expected nil overlay for Antigravity, got %s", overlay)
	}
}

// TestInjectCodexNeverWritesConfig pins the decision that gentle-ai writes
// nothing to Codex's permissions configuration — neither a profile nor the
// legacy migration that used to strip one. Codex refuses to load a config that
// defines a [permissions.*] profile without default_permissions, so a cleanup
// that removed the pointer while a profile survived stopped Codex from starting
// at all (#1794). Whoever still carries an old gentle-dev profile keeps it.
//
// Byte equality alone is not the assertion: an atomic rewrite that happens to
// produce identical bytes is still a write. Every fixture is backdated so the
// modification time proves the file was left alone.
func TestInjectCodexNeverWritesConfig(t *testing.T) {
	backdated := time.Date(2020, time.January, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name    string
		content string
	}{
		{name: "user reported blocker", content: "approval_policy = \"on-request\"\ndefault_permissions = \"gentle-dev\"\n\n[permissions.gentle-dev]\nnetwork.enabled = true\n"},
		{name: "fully injected legacy profile", content: codexInjectedLegacyConfig},
		{name: "residual generated profile", content: "model = \"gpt-5.5\"\n\n[permissions.gentle-dev]\n\n[permissions.gentle-dev.workspace_roots]\n"},
		{name: "user owned gentle-dev content", content: "approval_policy = \"never\"\ndefault_permissions = \"gentle-dev\"\n\n[permissions.custom]\ndescription = \"user profile\"\n\n[permissions.gentle-dev] # user-owned\nuser_note = \"keep\"\n"},
		{name: "quoted gentle-dev forms", content: "permissions.\"gentle-dev\".workspace_roots.\"~\" = true\n"},
		{name: "dotted user value", content: "default_permissions = \"gentle-dev\"\npermissions.gentle-dev.custom_flag = true\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			configPath := writeCodexConfig(t, home, tt.content)
			if err := os.Chtimes(configPath, backdated, backdated); err != nil {
				t.Fatalf("Chtimes() error = %v", err)
			}

			result, err := Inject(home, codexAdapter())
			if err != nil {
				t.Fatalf("Inject() error = %v", err)
			}
			if result.Changed {
				t.Error("Inject(codex) changed = true, want false")
			}
			if len(result.Files) != 0 {
				t.Errorf("Inject(codex) files = %v, want none", result.Files)
			}

			content, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatalf("read config.toml: %v", err)
			}
			if string(content) != tt.content {
				t.Errorf("config.toml = %q, want byte-identical %q", content, tt.content)
			}

			info, err := os.Stat(configPath)
			if err != nil {
				t.Fatalf("Stat(config.toml) error = %v", err)
			}
			if !info.ModTime().UTC().Equal(backdated) {
				t.Errorf("config.toml mtime = %v, want %v — the file was written, not left alone", info.ModTime().UTC(), backdated)
			}
		})
	}
}

func TestInjectCodexMissingConfigDoesNothing(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, codexAdapter())
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if result.Changed {
		t.Fatal("Inject() changed = true, want false for missing config")
	}
	if len(result.Files) != 0 {
		t.Fatalf("Inject() files = %v, want none for missing config", result.Files)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex")); !os.IsNotExist(err) {
		t.Fatalf("Inject() created ~/.codex (stat err = %v), want no file creation", err)
	}
}

func TestTargetPathCodexHasNoInjectionTarget(t *testing.T) {
	home := t.TempDir()
	if got := TargetPath(home, codexAdapter()); got != "" {
		t.Fatalf("TargetPath(codex) = %q, want empty (Codex relies on built-in defaults)", got)
	}
}

// TestInjectClaudeCodeSensitivePathsDenied verifies that the default sensitive-path
// deny list is present in the Claude Code permissions block.
func TestInjectClaudeCodeSensitivePathsDenied(t *testing.T) {
	sensitivePatterns := []string{
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
	}

	home := t.TempDir()
	if _, err := Inject(home, claudeAdapter()); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings file %q: %v", settingsPath, err)
	}

	var settings map[string]any
	if err := json.Unmarshal(content, &settings); err != nil {
		t.Fatalf("unmarshal settings json: %v", err)
	}

	permissionsNode, ok := settings["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("permissions node missing or invalid: %#v", settings["permissions"])
	}

	denyList, ok := permissionsNode["deny"].([]any)
	if !ok {
		t.Fatalf("deny list missing or invalid: %#v", permissionsNode["deny"])
	}

	denySet := make(map[string]bool, len(denyList))
	for _, entry := range denyList {
		if v, ok := entry.(string); ok {
			denySet[v] = true
		}
	}

	for _, pattern := range sensitivePatterns {
		t.Run(pattern, func(t *testing.T) {
			if !denySet[pattern] {
				t.Errorf("deny list missing pattern %q; got: %v", pattern, denyList)
			}
		})
	}
}

// TestInjectOpenCodeSensitivePathsDenied verifies that the default sensitive-path
// deny list is present in the OpenCode/Kilocode read permissions block.
func TestInjectOpenCodeSensitivePathsDenied(t *testing.T) {
	sensitivePatterns := []string{
		"**/.ssh/**",
		"**/.credentials/**",
		"**/Library/Keychains/**",
		"**/.aws/credentials",
		"**/.config/gh/hosts.yml",
		"**/*.pem",
		"**/*.key",
	}

	tests := []struct {
		name    string
		adapter agents.Adapter
	}{
		{"opencode", opencodeAdapter()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			if _, err := Inject(home, tt.adapter); err != nil {
				t.Fatalf("Inject() error = %v", err)
			}

			settingsPath := tt.adapter.SettingsPath(home)
			content, err := os.ReadFile(settingsPath)
			if err != nil {
				t.Fatalf("read settings file %q: %v", settingsPath, err)
			}

			var settings map[string]any
			if err := json.Unmarshal(content, &settings); err != nil {
				t.Fatalf("unmarshal settings json: %v", err)
			}

			permNode, ok := settings["permission"].(map[string]any)
			if !ok {
				t.Fatalf("permission node missing or invalid: %#v", settings["permission"])
			}

			readNode, ok := permNode["read"].(map[string]any)
			if !ok {
				t.Fatalf("read node missing or invalid: %#v", permNode["read"])
			}

			for _, pattern := range sensitivePatterns {
				t.Run(pattern, func(t *testing.T) {
					val, exists := readNode[pattern]
					if !exists {
						t.Errorf("read deny list missing pattern %q", pattern)
						return
					}
					if val != "deny" {
						t.Errorf("pattern %q has value %q, want %q", pattern, val, "deny")
					}
				})
			}
		})
	}
}

// TestInjectClaudeCodeDefaultDenyRulesApplied ensures that the default deny
// rules (including sensitive paths) are written into settings.json even when
// a pre-existing permissions block is already present with other top-level keys.
func TestInjectClaudeCodeDefaultDenyRulesApplied(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	// Pre-existing settings with a sibling key under permissions (not deny).
	existing := `{
  "permissions": {
    "defaultMode": "default"
  }
}`
	if err := os.WriteFile(settingsPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := Inject(home, claudeAdapter()); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings file: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(content, &settings); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	perms, ok := settings["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("permissions node missing")
	}

	denyList, ok := perms["deny"].([]any)
	if !ok {
		t.Fatalf("deny list missing")
	}

	denySet := make(map[string]bool, len(denyList))
	for _, entry := range denyList {
		if v, ok := entry.(string); ok {
			denySet[v] = true
		}
	}

	// Sensitive-path rules must be present after overlay application.
	for _, rule := range []string{"Read(.ssh/*)", "Read(**/*.pem)", "Read(**/*.key)"} {
		if !denySet[rule] {
			t.Errorf("default deny rule %q was not present; got: %v", rule, denyList)
		}
	}

	// The overlay wins for defaultMode because arrays replace but maps deep-merge.
	mode, _ := perms["defaultMode"].(string)
	if mode != "bypassPermissions" {
		t.Errorf("expected defaultMode=bypassPermissions after overlay, got %q", mode)
	}
}

// TestInjectOpenCodePreservesExistingDenyRules ensures that user-managed read deny
// entries already present in settings.json are not removed when the overlay is applied.
func TestInjectOpenCodePreservesExistingDenyRules(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	existing := `{
  "permission": {
    "read": {
      "**/my-secret/**": "deny"
    }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := Inject(home, opencodeAdapter()); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings file: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(content, &settings); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	permNode, ok := settings["permission"].(map[string]any)
	if !ok {
		t.Fatalf("permission node missing")
	}

	readNode, ok := permNode["read"].(map[string]any)
	if !ok {
		t.Fatalf("read node missing")
	}

	// Original user rule must still be present
	if readNode["**/my-secret/**"] != "deny" {
		t.Errorf("user-managed read deny rule '**/my-secret/**' was removed; got: %v", readNode)
	}

	// New sensitive-path rules must also be present
	if readNode["**/.ssh/**"] != "deny" {
		t.Errorf("default read deny rule '**/.ssh/**' was not added; got: %v", readNode)
	}
}

func TestKilocodePermissionsKeepBaseDuplicateKeyBehavior(t *testing.T) {
	home := t.TempDir()
	adapter := kilocode.NewAdapter()
	settings := TargetPath(home, adapter)
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte("{\"permission\":{\"bash\":{\"ssh\":\"deny\",\"ssh\":\"allow\"}}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Inject(home, adapter); err != nil {
		t.Fatalf("Inject(kilocode) error = %v; want base merge, not the OpenCode duplicate-key refusal", err)
	}
	raw, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("settings not valid JSON after merge: %v\n%s", err, raw)
	}
	if _, ok := root["permission"].(map[string]any); !ok {
		t.Fatalf("permission missing after merge: %s", raw)
	}
}
