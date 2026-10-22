package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/pi"
	"github.com/gentleman-programming/gentle-ai/v4/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/legacyassets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// uninstallSDDAgents are the runtimes the uninstall retirement tests remove.
var uninstallSDDAgents = []model.AgentID{
	model.AgentClaudeCode, model.AgentQwenCode, model.AgentCodex, model.AgentKimi,
	model.AgentKiroIDE, model.AgentCursor, model.AgentOpenCode, model.AgentPi,
}

func uninstallSDDTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	setSyncTestHome(t, home)
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	return home
}

// openCodeSDDUninstallSettings is a v3.7.0 OpenCode settings file: one
// released phase agent beside the user's own sdd-init.
func openCodeSDDUninstallSettings(t *testing.T) []byte {
	t.Helper()
	agent := releasedOpenCodeSDDAgents(t, "v1", "sdd-apply")
	agent["sdd-init"] = userSDDInit
	return []byte("{\n  \"agent\": " + indentedJSON(t, agent) + "\n}\n")
}

// TestUninstallRetiresOwnedSDDInventoryForEveryRuntime covers #5157 for
// uninstall: removing a runtime retires the SDD files, settings entries, and
// prompt text releases installed for it with the inventory and ownership
// proof install and sync use. User bytes are kept and reported, Pi files
// are never touched, and the snapshot restores everything uninstall changed.
func TestUninstallRetiresOwnedSDDInventoryForEveryRuntime(t *testing.T) {
	home := uninstallSDDTestHome(t)
	owned, edited := seedRetiredSDDAssets(t, home)
	runtimeFiles := seedRetiredRuntimeFiles(t, home)
	for path, data := range runtimeFiles.owned {
		owned[path] = data
	}
	owned[filepath.Join(home, ".claude", "agents", "sdd-apply.md")] = releasedSDDRender(t, "claude-sdd-apply.md")
	owned[filepath.Join(home, ".kiro", "agents", "sdd-apply.md")] = releasedSDDRender(t, "kiro-sdd-apply.md")
	owned[filepath.Join(home, ".cursor", "agents", "sdd-apply.md")] = releasedSDDRender(t, "cursor-sdd-apply.md")
	prompts := legacyassets.SharedPromptDir(home)
	owned[filepath.Join(prompts, "sdd-apply.md")] = releasedSDDRender(t, "opencode-prompt-sdd-apply.md")
	kept := map[string][]byte{
		runtimeFiles.userProfile: runtimeFiles.kept[runtimeFiles.userProfile],
		filepath.Join(home, ".claude", "commands", "gentle-sdd-verify.md"): edited[filepath.Join(home, ".claude", "commands", "gentle-sdd-verify.md")],
		filepath.Join(home, ".qwen", "skills", "sdd-apply", "SKILL.md"):    edited[filepath.Join(home, ".qwen", "skills", "sdd-apply", "SKILL.md")],
		filepath.Join(home, ".kiro", "agents", "sdd-verify.md"):            []byte("my own verifier\n"),
		filepath.Join(prompts, "sdd-explore.md"):                           []byte("my own explore prompt\n"),
	}
	piHome := pi.AgentConfigPath(home)
	untouched := map[string][]byte{
		filepath.Join(piHome, "agents", "sdd-apply.md"):         releasedSDDRender(t, "claude-sdd-apply.md"),
		filepath.Join(piHome, "skills", "sdd-init", "SKILL.md"): releasedSDDAsset(t, "v3.7.0", "skill-sdd-init.md"),
	}
	for _, group := range []map[string][]byte{owned, kept, untouched} {
		for path, data := range group {
			mustWriteFile(t, path, data)
		}
	}
	settings := filepath.Join(home, ".config", "opencode", "opencode.json")
	mustWriteFile(t, settings, openCodeSDDUninstallSettings(t))
	claudePrompt := filepath.Join(home, ".claude", "CLAUDE.md")
	mustWriteFile(t, claudePrompt, []byte(legacyCodexPromptUser+legacyCodexPromptBlock))
	rewritten := map[string][]byte{
		settings: openCodeSDDUninstallSettings(t), filepath.Join(home, ".claude", "settings.json"): []byte(claudeSettingsWithSDDHook),
		claudePrompt: []byte(legacyCodexPromptUser + legacyCodexPromptBlock),
	}
	for path, data := range runtimeFiles.rewritten {
		rewritten[path] = data
	}

	result, err := RunUninstallWithSelection(home, t.TempDir(), uninstallSDDAgents, nil)
	if err != nil {
		t.Fatalf("RunUninstallWithSelection() error = %v", err)
	}
	snapshot := map[string]bool{}
	for _, entry := range result.Manifest.Entries {
		if entry.Existed {
			snapshot[entry.OriginalPath] = true
		}
	}
	for path := range owned {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Errorf("owned retired SDD file survived uninstall: %s (%v)", path, err)
		}
		if !slices.Contains(result.RemovedFiles, path) {
			t.Errorf("uninstall did not report removed file %s", path)
		}
		if !snapshot[path] {
			t.Errorf("uninstall snapshot omitted removed file %s", path)
		}
	}
	for path, data := range kept {
		if got, err := os.ReadFile(path); err != nil || string(got) != string(data) {
			t.Errorf("uninstall changed the user's %s: %q, %v", path, got, err)
		}
		if !slices.ContainsFunc(result.ManualActions, func(action string) bool {
			return strings.Contains(action, path) && strings.Contains(action, "move or delete it")
		}) {
			t.Errorf("preserved %s not reported: %v", path, result.ManualActions)
		}
	}
	for path, data := range untouched {
		if got, err := os.ReadFile(path); err != nil || string(got) != string(data) {
			t.Errorf("uninstall touched Pi file %s: %v", path, err)
		}
	}
	if got, _ := os.ReadFile(claudePrompt); string(got) != legacyCodexPromptUser {
		t.Errorf("CLAUDE.md = %q, want only the user's notes: no routing guidance migrates its SDD block", got)
	}
	if got, _ := os.ReadFile(runtimeFiles.prompt); string(got) != legacyCodexPromptUser {
		t.Errorf("agents.md = %q, want only the user's notes", got)
	}
	if got, _ := os.ReadFile(runtimeFiles.hub); strings.Contains(string(got), kimiSDDInclude) {
		t.Errorf("legacy KIMI.md kept the SDD include: %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(home, ".claude", "settings.json")); strings.Contains(string(got), "sdd-preflight-hook") {
		t.Errorf("Claude settings kept the SDD preflight hook: %s", got)
	}
	got, _ := os.ReadFile(settings)
	if strings.Contains(string(got), `"sdd-apply"`) || !strings.Contains(string(got), `"My own init"`) {
		t.Errorf("OpenCode settings = %s, want the released sdd-apply removed and the user's sdd-init kept", got)
	}
	if !slices.ContainsFunc(result.ManualActions, func(action string) bool {
		return strings.Contains(action, "agent.sdd-init") && strings.Contains(action, "move or delete it")
	}) {
		t.Errorf("preserved OpenCode sdd-init not reported: %v", result.ManualActions)
	}
	for path := range rewritten {
		if !snapshot[path] {
			t.Errorf("uninstall snapshot omitted rewritten file %s", path)
		}
	}

	if err := (backup.RestoreService{}).Restore(result.Manifest); err != nil {
		t.Fatalf("restore uninstall snapshot: %v", err)
	}
	for _, group := range []map[string][]byte{owned, kept, rewritten} {
		for path, data := range group {
			if got, err := os.ReadFile(path); err != nil || string(got) != string(data) {
				t.Errorf("restore did not bring back %s byte for byte: %v", path, err)
			}
		}
	}
}

// Uninstall never enters a directory that is not a real directory: retired
// SDD files behind a symlinked agents, skills, or prompts directory are the
// user's, so they are kept, reported, and left out of the snapshot.
func TestUninstallNeverEntersSymlinkedSDDDirectories(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	home := uninstallSDDTestHome(t)
	outside := t.TempDir()
	targets := map[string][]byte{
		filepath.Join(outside, "agents", "sdd-apply.md"):         releasedSDDRender(t, "cursor-sdd-apply.md"),
		filepath.Join(outside, "skills", "sdd-init", "SKILL.md"): releasedSDDAsset(t, "v3.7.0", "skill-sdd-init.md"),
		filepath.Join(outside, "prompts", "sdd-apply.md"):        releasedSDDRender(t, "opencode-prompt-sdd-apply.md"),
	}
	for path, data := range targets {
		mustWriteFile(t, path, data)
	}
	links := map[string]string{
		filepath.Join(home, ".cursor", "agents"): filepath.Join(outside, "agents"),
		filepath.Join(home, ".qwen", "skills"):   filepath.Join(outside, "skills"),
		legacyassets.SharedPromptDir(home):       filepath.Join(outside, "prompts"),
	}
	for link, target := range links {
		if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
	}

	result, err := RunUninstallWithSelection(home, t.TempDir(), []model.AgentID{model.AgentCursor, model.AgentOpenCode, model.AgentQwenCode}, nil)
	if err != nil {
		t.Fatalf("RunUninstallWithSelection() error = %v", err)
	}
	for path, data := range targets {
		if got, err := os.ReadFile(path); err != nil || string(got) != string(data) {
			t.Errorf("uninstall changed %s behind a symlink: %v", path, err)
		}
	}
	for link, target := range links {
		if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("uninstall replaced the symlink %s: %v", link, err)
		}
		if !slices.ContainsFunc(result.ManualActions, func(action string) bool {
			return strings.Contains(action, link) && strings.Contains(action, "not a real directory")
		}) {
			t.Errorf("symlinked %s not reported: %v", link, result.ManualActions)
		}
		for path := range targets {
			if rel, err := filepath.Rel(target, path); err == nil && !strings.HasPrefix(rel, "..") {
				through := filepath.Join(link, rel)
				for _, entry := range result.Manifest.Entries {
					if entry.OriginalPath == through {
						t.Errorf("uninstall snapshot declares %s through the symlink %s", through, link)
					}
				}
			}
		}
	}
}
