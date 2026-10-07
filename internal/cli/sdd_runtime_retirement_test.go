package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/codex"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

const (
	legacyCodexPromptUser  = "# My Codex notes\n\nKeep this.\n"
	legacyCodexPromptBlock = "\n<!-- gentle-ai:sdd-orchestrator -->\n# Agent Teams Lite — Orchestrator Instructions\n<!-- /gentle-ai:sdd-orchestrator -->\n"
	kimiSDDInclude         = "{% include \"sdd-orchestrator.md\" ignore missing %}\n"
)

type retiredRuntimeFiles struct {
	owned, kept, rewritten map[string][]byte
	userProfile, prompt    string
	hub                    string
}

// seedRetiredRuntimeFiles lays out what v1.x to v3.7.0 left for Codex and
// Kimi beside a current kimi-code layout, whose legacy KIMI.md no sync
// rewrites any more.
func seedRetiredRuntimeFiles(t *testing.T, home string) retiredRuntimeFiles {
	t.Helper()
	codexHome := filepath.Join(home, ".codex")
	files := retiredRuntimeFiles{
		owned: map[string][]byte{
			filepath.Join(codexHome, "sdd-strong.config.toml"):  releasedSDDAsset(t, "v3.7.0", "codex-sdd-strong.config.toml"),
			filepath.Join(codexHome, "sdd-mid.config.toml"):     []byte("model = \"my-model\"\n\nmodel_reasoning_effort = \"high\"\n"),
			filepath.Join(home, ".kimi", "sdd-orchestrator.md"): releasedSDDAsset(t, "v3.7.0", "kimi-sdd-orchestrator.md"),
		},
		userProfile: filepath.Join(codexHome, "sdd-cheap.config.toml"),
		prompt:      filepath.Join(codexHome, "agents.md"),
		hub:         filepath.Join(home, ".kimi", "KIMI.md"),
	}
	files.kept = map[string][]byte{files.userProfile: []byte("user-content\n")}
	files.rewritten = map[string][]byte{
		files.prompt: []byte(legacyCodexPromptUser + legacyCodexPromptBlock),
		files.hub:    releasedSDDAsset(t, "v3.7.0", "kimi-KIMI.md"),
	}
	for _, group := range []map[string][]byte{files.owned, files.kept, files.rewritten} {
		for path, data := range group {
			mustWriteFile(t, path, data)
		}
	}
	if err := os.MkdirAll(filepath.Join(home, ".kimi-code"), 0o755); err != nil {
		t.Fatal(err)
	}
	// v1.7.10 to v1.31.0 wrote the lowercase agents.md; on a
	// case-insensitive filesystem it is the active AGENTS.md.
	active := filepath.Join(codexHome, "AGENTS.md")
	if a, err := os.Stat(files.prompt); err == nil {
		if b, err := os.Stat(active); err == nil && os.SameFile(a, b) {
			t.Skip("case-insensitive filesystem: agents.md is the active AGENTS.md")
		}
	}
	return files
}

// TestRunSyncRetiresCodexProfilesAndKimiModule covers #5157 for Codex and
// Kimi: sync removes the SDD profiles and the SDD Jinja module a release
// rendered, the released include of that module, and the SDD block of the
// lowercase agents.md no routing guidance migrates. User bytes are kept and
// reported, everything is snapshotted, and a second sync changes nothing.
func TestRunSyncRetiresCodexProfilesAndKimiModule(t *testing.T) {
	home := t.TempDir()
	setSyncTestHome(t, home)
	t.Cleanup(codex.SetRuntimeVersionCommandForTest("", exec.ErrNotFound))
	files := seedRetiredRuntimeFiles(t, home)
	agents := string(model.AgentCodex) + "," + string(model.AgentKimi)

	result, err := RunSync([]string{"--agents", agents})
	if err != nil {
		t.Fatalf("RunSync() error = %v", err)
	}
	snapshot := backupManifestEntries(t, home)
	for path := range files.owned {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Errorf("owned retired SDD file survived sync: %s (%v)", path, err)
		}
		if _, ok := snapshot[path]; !ok {
			t.Errorf("rollback snapshot omitted removed file %s", path)
		}
		if !slices.Contains(result.ChangedFiles, path) {
			t.Errorf("sync did not report removed file %s", path)
		}
	}
	if got, err := os.ReadFile(files.userProfile); err != nil || string(got) != "user-content\n" {
		t.Errorf("sync changed the user's profile: %q, %v", got, err)
	}
	if !slices.ContainsFunc(result.ManualActions, func(action string) bool {
		return strings.Contains(action, files.userProfile) && strings.Contains(action, "move or delete it")
	}) {
		t.Errorf("preserved profile not reported: %v", result.ManualActions)
	}
	if got, _ := os.ReadFile(files.prompt); string(got) != legacyCodexPromptUser {
		t.Errorf("agents.md = %q, want only the user's notes", got)
	}
	hub := string(releasedSDDAsset(t, "v3.7.0", "kimi-KIMI.md"))
	if got, _ := os.ReadFile(files.hub); string(got) != strings.Replace(hub, kimiSDDInclude, "", 1) {
		t.Errorf("legacy KIMI.md = %q, want only the SDD include removed", got)
	}
	for path := range files.rewritten {
		if _, ok := snapshot[path]; !ok {
			t.Errorf("rollback snapshot omitted rewritten file %s", path)
		}
		if !slices.Contains(result.ChangedFiles, path) {
			t.Errorf("sync did not report rewritten file %s", path)
		}
	}
	if _, ok := snapshot[filepath.Join(home, ".codex", "sdd-cheap.config.toml")]; !ok {
		t.Error("rollback snapshot omitted the inspected user profile")
	}

	again, err := RunSync([]string{"--agents", agents})
	if err != nil {
		t.Fatalf("second RunSync() error = %v", err)
	}
	for _, path := range again.ChangedFiles {
		if strings.Contains(path, "sdd-") || path == files.prompt || path == files.hub {
			t.Errorf("second sync touched %s", path)
		}
	}
}

// A sync that fails after retirement restores every removed and rewritten
// file from the pipeline snapshot, byte for byte.
func TestSyncRollbackRestoresRetiredCodexAndKimiFiles(t *testing.T) {
	home := t.TempDir()
	setSyncTestHome(t, home)
	t.Cleanup(codex.SetRuntimeVersionCommandForTest("", exec.ErrNotFound))
	files := seedRetiredRuntimeFiles(t, home)
	selection := BuildSyncSelection(SyncFlags{}, []model.AgentID{model.AgentCodex, model.AgentKimi})
	rt, err := newSyncRuntimeWithScope(home, selection, ScopeGlobal)
	if err != nil {
		t.Fatal(err)
	}
	plan := rt.stagePlan()
	last := -1
	for i, step := range plan.Apply {
		if strings.HasPrefix(step.ID(), "sync:agent:retire-sdd-assets:") {
			last = i
		}
	}
	if last < 0 {
		t.Fatal("sync plan has no SDD asset retirement step")
	}
	retired := false
	plan.Apply = append(plan.Apply[:last+1:last+1], sddRetirementFailingStep{observe: func() {
		_, err := os.Lstat(filepath.Join(home, ".kimi", "sdd-orchestrator.md"))
		prompt, _ := os.ReadFile(files.prompt)
		retired = os.IsNotExist(err) && string(prompt) == legacyCodexPromptUser
	}})
	if execution := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy()).Execute(plan); execution.Err == nil {
		t.Fatal("injected failure did not fail the sync")
	}
	if !retired {
		t.Fatal("retirement did not run before the failure")
	}
	for _, group := range []map[string][]byte{files.owned, files.kept, files.rewritten} {
		for path, data := range group {
			if got, err := os.ReadFile(path); err != nil || string(got) != string(data) {
				t.Errorf("rollback did not restore %s byte for byte: %v", path, err)
			}
		}
	}
}

// Workspace sync never delivers routing guidance to Claude Code or Codex,
// so the v3.7.0 SDD block in their project prompt files is retired there,
// and nothing at home is touched.
func TestRunSyncWorkspaceRetiresSDDBlockFromProjectPrompts(t *testing.T) {
	home := t.TempDir()
	setSyncTestHome(t, home)
	t.Cleanup(codex.SetRuntimeVersionCommandForTest("", exec.ErrNotFound))
	workspace := t.TempDir()
	mustWriteFile(t, filepath.Join(workspace, ".git", "HEAD"), []byte("ref: refs/heads/main\n"))
	block := "<!-- gentle-ai:sdd-orchestrator -->\n## SDD Orchestrator\n<!-- /gentle-ai:sdd-orchestrator -->\n"
	prompts := map[string]string{
		filepath.Join(workspace, ".claude", "CLAUDE.md"): "# Project rules\n",
		filepath.Join(workspace, ".codex", "AGENTS.md"):  "# Codex project rules\n",
	}
	for path, user := range prompts {
		mustWriteFile(t, path, []byte(user+"\n"+block))
	}
	homePrompt := filepath.Join(home, ".claude", "CLAUDE.md")
	mustWriteFile(t, homePrompt, []byte(block))
	t.Chdir(workspace)

	agents := string(model.AgentClaudeCode) + "," + string(model.AgentCodex)
	result, err := RunSync([]string{"--agents", agents, "--scope", "workspace"})
	if err != nil {
		t.Fatalf("workspace RunSync() error = %v", err)
	}
	for path, user := range prompts {
		// The persona component also manages its own section here.
		if got, _ := os.ReadFile(path); !strings.HasPrefix(string(got), user) || strings.Contains(string(got), "sdd-orchestrator") || strings.Contains(string(got), "SDD Orchestrator") {
			t.Errorf("%s = %q, want %q without the SDD block", path, got, user)
		}
		if !slices.Contains(result.ChangedFiles, path) {
			t.Errorf("workspace sync did not report %s", path)
		}
	}
	if got, _ := os.ReadFile(homePrompt); string(got) != block {
		t.Errorf("workspace sync touched the home prompt: %q", got)
	}
}

// Install retires the same inventory as sync.
func TestRunInstallRetiresKimiSDDModule(t *testing.T) {
	home := installTestHome(t)
	setSyncTestHome(t, home)
	module := filepath.Join(home, ".kimi", "sdd-orchestrator.md")
	mustWriteFile(t, module, releasedSDDAsset(t, "v1.21.0", "kimi-sdd-orchestrator.md"))
	if _, err := RunInstall([]string{"--agent", string(model.AgentKimi), "--component", "skills"}, system.DetectionResult{}); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if _, err := os.Lstat(module); !os.IsNotExist(err) {
		t.Errorf("install kept the owned Kimi SDD module: %v", err)
	}
	if _, ok := backupManifestEntries(t, home)[module]; !ok {
		t.Error("install snapshot omitted the Kimi SDD module")
	}
}

// A dotfiles-symlinked ~/.codex or ~/.kimi is never entered: sync neither
// edits nor snapshots the prompts behind it, and reports them.
func TestRunSyncDoesNotEditPromptsBehindSymlinkedRuntimeDirectories(t *testing.T) {
	home := t.TempDir()
	setSyncTestHome(t, home)
	t.Cleanup(codex.SetRuntimeVersionCommandForTest("", exec.ErrNotFound))
	dotfiles := t.TempDir()
	prompt := filepath.Join(dotfiles, "codex", "agents.md")
	hub := filepath.Join(dotfiles, "kimi", "KIMI.md")
	contents := map[string]string{
		prompt: legacyCodexPromptUser + legacyCodexPromptBlock,
		hub:    string(releasedSDDAsset(t, "v3.7.0", "kimi-KIMI.md")),
	}
	for path, data := range contents {
		mustWriteFile(t, path, []byte(data))
	}
	for name, target := range map[string]string{".codex": filepath.Dir(prompt), ".kimi": filepath.Dir(hub)} {
		if err := os.Symlink(target, filepath.Join(home, name)); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
	}
	if err := os.MkdirAll(filepath.Join(home, ".kimi-code"), 0o755); err != nil {
		t.Fatal(err)
	}
	active := filepath.Join(dotfiles, "codex", "AGENTS.md")
	if a, err := os.Stat(prompt); err == nil {
		if b, err := os.Stat(active); err == nil && os.SameFile(a, b) {
			t.Skip("case-insensitive filesystem: agents.md is the active AGENTS.md")
		}
	}

	result, err := RunSync([]string{"--agents", string(model.AgentCodex) + "," + string(model.AgentKimi)})
	if err != nil {
		t.Fatalf("RunSync() error = %v", err)
	}
	snapshot := backupManifestEntries(t, home)
	for path, data := range contents {
		if got, err := os.ReadFile(path); err != nil || string(got) != data {
			t.Errorf("sync edited %s behind a symlinked directory: %q, %v", path, got, err)
		}
	}
	for _, path := range []string{filepath.Join(home, ".codex", "agents.md"), filepath.Join(home, ".kimi", "KIMI.md")} {
		if _, ok := snapshot[path]; ok {
			t.Errorf("snapshot declares %s behind a symlinked directory", path)
		}
		if !slices.ContainsFunc(result.ManualActions, func(action string) bool {
			return strings.Contains(action, path) && strings.Contains(action, "not a real directory")
		}) {
			t.Errorf("%s not reported: %v", path, result.ManualActions)
		}
	}
}

// A workspace-scoped install of v1.31.0 to v3.7.0 wrote the Codex profiles,
// lowercase agents.md, Kimi module, and KIMI.md under the workspace, so a
// workspace-scoped sync retires them there and leaves home alone.
func TestRunSyncWorkspaceRetiresCodexAndKimiFilesInTheWorkspace(t *testing.T) {
	home := t.TempDir()
	setSyncTestHome(t, home)
	t.Cleanup(codex.SetRuntimeVersionCommandForTest("", exec.ErrNotFound))
	workspace := t.TempDir()
	mustWriteFile(t, filepath.Join(workspace, ".git", "HEAD"), []byte("ref: refs/heads/main\n"))
	homeFiles := seedRetiredRuntimeFiles(t, home)
	wsFiles := seedRetiredRuntimeFiles(t, workspace)
	t.Chdir(workspace)

	if _, err := RunSync([]string{"--agents", string(model.AgentCodex) + "," + string(model.AgentKimi), "--scope", "workspace"}); err != nil {
		t.Fatalf("workspace RunSync() error = %v", err)
	}
	for path := range wsFiles.owned {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Errorf("workspace sync kept %s: %v", path, err)
		}
	}
	if got, _ := os.ReadFile(wsFiles.prompt); string(got) != legacyCodexPromptUser {
		t.Errorf("workspace agents.md = %q", got)
	}
	if got, _ := os.ReadFile(wsFiles.hub); strings.Contains(string(got), kimiSDDInclude) {
		t.Errorf("workspace KIMI.md kept the SDD include: %q", got)
	}
	for _, group := range []map[string][]byte{homeFiles.owned, homeFiles.kept, homeFiles.rewritten} {
		for path, data := range group {
			if got, err := os.ReadFile(path); err != nil || string(got) != string(data) {
				t.Errorf("workspace sync touched home file %s: %v", path, err)
			}
		}
	}
}

// Claude Code's lazy SDD workflow is proven by replaying each release's
// writer: sync removes a release render and snapshots it, and keeps and
// reports a copy with custom model assignments.
func TestRunSyncRetiresReplayedClaudeSDDWorkflow(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content []byte
		owned   bool
	}{
		{"release render", releasedSDDAsset(t, "v3.7.0", "claude-sdd-orchestrator-workflow.md"), true},
		{"custom assignments", []byte(strings.Replace(string(releasedSDDAsset(t, "v2.0.0", "claude-sdd-orchestrator-workflow-performance.md")), "| sdd-explore | sonnet |", "| sdd-explore | haiku |", 1)), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			setSyncTestHome(t, home)
			workflow := filepath.Join(home, ".claude", "skills", "_shared", "sdd-orchestrator-workflow.md")
			mustWriteFile(t, workflow, tc.content)

			result, err := RunSync([]string{"--agents", string(model.AgentClaudeCode)})
			if err != nil {
				t.Fatalf("RunSync() error = %v", err)
			}
			if _, ok := backupManifestEntries(t, home)[workflow]; !ok {
				t.Error("sync snapshot omitted the Claude SDD workflow")
			}
			_, statErr := os.Lstat(workflow)
			if tc.owned {
				if !os.IsNotExist(statErr) || !slices.Contains(result.ChangedFiles, workflow) {
					t.Fatalf("release render not retired: %v, changed %v", statErr, result.ChangedFiles)
				}
				return
			}
			if got, err := os.ReadFile(workflow); err != nil || string(got) != string(tc.content) {
				t.Fatalf("sync changed the user's workflow: %v", err)
			}
			if !slices.ContainsFunc(result.ManualActions, func(action string) bool {
				return strings.Contains(action, workflow) && strings.Contains(action, "move or delete it")
			}) {
				t.Errorf("preserved workflow not reported: %v", result.ManualActions)
			}
		})
	}
}
