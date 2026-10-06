package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	kilocodeagent "github.com/gentleman-programming/gentle-ai/v4/internal/agents/kilocode"
	opencodeagent "github.com/gentleman-programming/gentle-ai/v4/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

// releasedOpenCodeSDDAgents reads the agent map of a real v3.7.0 OpenCode
// render (multi mode plus a "fallback" profile): shape "v1" is the `agent`
// map, "v2" the plural native `agents` map 3.7.0 wrote when it detected a
// native config (#5182). Orchestrator and reviewer prompts are trimmed to
// their opening lines; every sdd-* phase entry is byte-for-byte as rendered.
func releasedOpenCodeSDDAgents(t *testing.T, shape string, names ...string) map[string]any {
	t.Helper()
	root, err := filemerge.UnmarshalJSONObject(releasedSDDRender(t, "opencode-"+shape+".json"))
	if err != nil {
		t.Fatal(err)
	}
	key := "agent"
	if shape == "v2" {
		key = "agents"
	}
	all, _ := root[key].(map[string]any)
	picked := map[string]any{}
	for _, name := range names {
		entry, ok := all[name]
		if !ok {
			t.Fatalf("fixture %s has no %s.%s", shape, key, name)
		}
		picked[name] = entry
	}
	return picked
}

func indentedJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.MarshalIndent(value, "  ", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

var userSDDInit = map[string]any{"mode": "subagent", "description": "My own init", "prompt": "Initialize the way I like."}

// openCodeSDDSettingsDocument is a v3.7.0-upgraded OpenCode settings file:
// user comments around the V1 `agent` map and the plural `agents` residue.
func openCodeSDDSettingsDocument(t *testing.T) []byte {
	t.Helper()
	agent := releasedOpenCodeSDDAgents(t, "v1", "gentle-orchestrator", "sdd-apply", "sdd-research", "sdd-apply-fallback", "sdd-research-fallback", "sdd-orchestrator-fallback")
	agent["sdd-init"] = userSDDInit
	agent["my-helper"] = map[string]any{"mode": "subagent", "prompt": "Help me."}
	agents := releasedOpenCodeSDDAgents(t, "v2", "gentle-orchestrator", "explore", "sdd-explore", "sdd-explore-fallback", "sdd-orchestrator-fallback")
	return []byte("// Personal OpenCode settings: keep this note.\n{\n" +
		"  \"$schema\": \"https://opencode.ai/config.json\",\n" +
		"  /* Gentle AI must keep this block comment. */\n" +
		"  \"default_agent\": \"gentle-orchestrator\",\n" +
		"  \"share\": \"disabled\",\n" +
		"  \"agent\": " + indentedJSON(t, agent) + ",\n" +
		"  \"agents\": " + indentedJSON(t, agents) + "\n" +
		"  // Trailing note inside the root object.\n}\n")
}

func openCodeSettingsAgents(t *testing.T, path string) (map[string]any, map[string]any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	root, err := filemerge.UnmarshalJSONObject(data)
	if err != nil {
		t.Fatalf("settings no longer parse: %v\n%s", err, data)
	}
	agent, _ := root["agent"].(map[string]any)
	return agent, root
}

func hasManualAction(actions []string, parts ...string) bool {
	return slices.ContainsFunc(actions, func(action string) bool {
		for _, part := range parts {
			if !strings.Contains(action, part) {
				return false
			}
		}
		return true
	})
}

// TestSyncRetiresOwnedOpenCodeSDDSettingsOnBothRuntimeMajors covers #5157 and
// #5182: a sync removes the retired SDD agent entries Gentle AI wrote to the
// OpenCode settings, in the V1 `agent` map (marked and profile-suffixed
// entries) and in the plural `agents` residue 3.7.0 left, plus the shared
// prompt files it rendered. A same-name agent the user wrote and an edited
// prompt are preserved and reported; JSONC comments survive; the removed
// bytes are in the rollback snapshot; a second sync changes nothing.
func TestSyncRetiresOwnedOpenCodeSDDSettingsOnBothRuntimeMajors(t *testing.T) {
	for _, version := range openCodeRuntimeVersions {
		t.Run(version, func(t *testing.T) {
			home := t.TempDir()
			setSyncTestHome(t, home)
			setOpenCodeTestHome(t, home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
			t.Setenv("OPENCODE_CONFIG_DIR", "")
			stubOpenCodeRuntimeVersion(t, home, version)
			config := opencodeagent.ConfigPath(home)
			settings := filepath.Join(config, "opencode.jsonc")
			mustWriteFile(t, settings, openCodeSDDSettingsDocument(t))
			ownedPrompt := filepath.Join(config, "prompts", "sdd", "sdd-apply.md")
			mustWriteFile(t, ownedPrompt, releasedSDDRender(t, "opencode-prompt-sdd-apply.md"))
			t.Chdir(t.TempDir())
			userPrompt := filepath.Join(config, "prompts", "sdd", "sdd-init.md")
			mustWriteFile(t, userPrompt, []byte("My own init notes.\n"))
			selection := model.Selection{Agents: []model.AgentID{model.AgentOpenCode}}
			if got := syncOpenCodeSettingsPath(home, "", ScopeGlobal, opencodeagent.NewAdapter()); got != settings {
				t.Fatalf("sync settings path = %q, want %q", got, settings)
			}

			result, err := RunSyncWithSelection(home, selection)
			if err != nil {
				t.Fatalf("RunSyncWithSelection() error = %v", err)
			}
			agent, root := openCodeSettingsAgents(t, settings)
			for _, name := range []string{"sdd-apply", "sdd-research", "sdd-apply-fallback", "sdd-research-fallback", "sdd-orchestrator-fallback"} {
				if _, ok := agent[name]; ok {
					t.Errorf("Gentle-owned agent.%s survived sync", name)
				}
			}
			if _, ok := root["agents"]; ok {
				t.Errorf("3.7.0 plural agents residue survived sync: %v", root["agents"])
			}
			if !reflect.DeepEqual(agent["sdd-init"], userSDDInit) {
				t.Errorf("user-defined agent.sdd-init changed: %#v", agent["sdd-init"])
			}
			if _, ok := agent["my-helper"]; !ok {
				t.Error("unrelated user agent removed")
			}
			text := readTextFile(t, settings)
			for _, comment := range []string{"// Personal OpenCode settings: keep this note.", "/* Gentle AI must keep this block comment. */", "// Trailing note inside the root object."} {
				if !strings.Contains(text, comment) {
					t.Errorf("sync dropped JSONC comment %q", comment)
				}
			}
			if _, err := os.Lstat(ownedPrompt); !os.IsNotExist(err) {
				t.Errorf("released SDD prompt survived sync: %v", err)
			}
			if got := readTextFile(t, userPrompt); got != "My own init notes.\n" {
				t.Errorf("user-edited SDD prompt changed: %q", got)
			}
			if !hasManualAction(result.ManualActions, settings, "sdd-init", "move or delete it") {
				t.Errorf("preserved agent.sdd-init not reported: %v", result.ManualActions)
			}
			if !hasManualAction(result.ManualActions, userPrompt, "move or delete it") {
				t.Errorf("preserved SDD prompt not reported: %v", result.ManualActions)
			}
			snapshot := backupManifestEntries(t, home)
			for _, path := range []string{settings, ownedPrompt} {
				if _, ok := snapshot[path]; !ok {
					t.Errorf("rollback snapshot omitted %s", path)
				}
				if !slices.Contains(result.ChangedFiles, path) {
					t.Errorf("sync did not report changed %s", path)
				}
			}

			before := readTextFile(t, settings)
			again, err := RunSyncWithSelection(home, selection)
			if err != nil {
				t.Fatalf("second sync error = %v", err)
			}
			if got := readTextFile(t, settings); got != before {
				t.Errorf("second sync changed settings:\n%s", got)
			}
			for _, path := range again.ChangedFiles {
				if strings.Contains(path, filepath.Join("prompts", "sdd")) {
					t.Errorf("second sync touched %s", path)
				}
			}
		})
	}
}

// A sync that fails after the settings retirement restores the settings and
// the removed prompt byte for byte from the pipeline snapshot.
func TestSyncRollbackRestoresRetiredOpenCodeSDDSettings(t *testing.T) {
	home := t.TempDir()
	setSyncTestHome(t, home)
	setOpenCodeTestHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("OPENCODE_CONFIG_DIR", "")
	stubOpenCodeRuntimeVersion(t, home, "1.18.30")
	config := opencodeagent.ConfigPath(home)
	settings := filepath.Join(config, "opencode.jsonc")
	document := openCodeSDDSettingsDocument(t)
	mustWriteFile(t, settings, document)
	prompt := filepath.Join(config, "prompts", "sdd", "sdd-apply.md")
	released := releasedSDDRender(t, "opencode-prompt-sdd-apply.md")
	mustWriteFile(t, prompt, released)
	t.Chdir(t.TempDir())
	rt, err := newSyncRuntimeWithScope(home, model.Selection{Agents: []model.AgentID{model.AgentOpenCode}}, ScopeGlobal)
	if err != nil {
		t.Fatal(err)
	}
	plan := rt.stagePlan()
	for _, path := range []string{settings, prompt} {
		if !slices.Contains(rt.managedPaths, path) {
			t.Fatalf("sync snapshot does not declare %s", path)
		}
	}
	index := slices.IndexFunc(plan.Apply, func(step pipeline.Step) bool { return step.ID() == "sync:agent:retire-sdd-settings:opencode" })
	if index < 0 {
		t.Fatal("sync plan has no OpenCode SDD settings retirement step")
	}
	retired := false
	plan.Apply = append(plan.Apply[:index+1:index+1], sddRetirementFailingStep{observe: func() {
		_, err := os.Lstat(prompt)
		agent, _ := openCodeSettingsAgents(t, settings)
		_, kept := agent["sdd-apply"]
		retired = os.IsNotExist(err) && !kept
	}})
	execution := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy()).Execute(plan)
	if execution.Err == nil {
		t.Fatal("injected failure did not fail the sync")
	}
	if !retired {
		t.Fatal("retirement did not run before the failure")
	}
	if got, err := os.ReadFile(settings); err != nil || string(got) != string(document) {
		t.Fatalf("rollback did not restore settings bytes: %v", err)
	}
	if got, err := os.ReadFile(prompt); err != nil || string(got) != string(released) {
		t.Fatalf("rollback did not restore the retired prompt: %v", err)
	}
}

// A workspace sync retires owned entries in the project settings it manages
// and never touches the global settings or the global prompt directory.
func TestWorkspaceSyncRetiresOnlyWorkspaceOpenCodeSDDSettings(t *testing.T) {
	home, workspace := t.TempDir(), t.TempDir()
	setSyncTestHome(t, home)
	setOpenCodeTestHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("OPENCODE_CONFIG_DIR", "")
	stubOpenCodeRuntimeVersion(t, home, "1.18.30")
	config := opencodeagent.ConfigPath(home)
	global := filepath.Join(config, "opencode.jsonc")
	globalDocument := openCodeSDDSettingsDocument(t)
	mustWriteFile(t, global, globalDocument)
	prompt := filepath.Join(config, "prompts", "sdd", "sdd-apply.md")
	mustWriteFile(t, prompt, releasedSDDRender(t, "opencode-prompt-sdd-apply.md"))
	project := filepath.Join(workspace, "opencode.json")
	mustWriteFile(t, project, []byte(`{"agent":`+indentedJSON(t, releasedOpenCodeSDDAgents(t, "v1", "sdd-apply", "sdd-apply-fallback"))+`}`))
	if err := os.Mkdir(filepath.Join(workspace, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(workspace)

	if _, err := RunSyncWithSelectionScope(home, model.Selection{Agents: []model.AgentID{model.AgentOpenCode}}, ScopeWorkspace); err != nil {
		t.Fatalf("workspace sync error = %v", err)
	}
	agent, _ := openCodeSettingsAgents(t, project)
	if _, ok := agent["sdd-apply"]; ok {
		t.Error("workspace sync kept a Gentle-owned project agent.sdd-apply")
	}
	if _, ok := agent["sdd-apply-fallback"]; ok {
		t.Error("workspace sync kept a Gentle-owned project agent.sdd-apply-fallback")
	}
	if got := readTextFile(t, global); got != string(globalDocument) {
		t.Error("workspace sync changed the global OpenCode settings")
	}
	if _, err := os.Stat(prompt); err != nil {
		t.Errorf("workspace sync removed a global SDD prompt: %v", err)
	}
}

// Kilocode received the same v3.7.0 overlay and profile entries.
func TestSyncRetiresOwnedKilocodeSDDSettings(t *testing.T) {
	home := t.TempDir()
	setSyncTestHome(t, home)
	setOpenCodeTestHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	settings := kilocodeagent.NewAdapter().SettingsPath(home)
	agent := releasedOpenCodeSDDAgents(t, "v1", "sdd-apply", "sdd-apply-fallback", "sdd-orchestrator-fallback")
	agent["sdd-init"] = userSDDInit
	mustWriteFile(t, settings, []byte(`{"agent":`+indentedJSON(t, agent)+`}`))
	t.Chdir(t.TempDir())

	result, err := RunSyncWithSelection(home, model.Selection{Agents: []model.AgentID{model.AgentKilocode}})
	if err != nil {
		t.Fatalf("RunSyncWithSelection() error = %v", err)
	}
	got, _ := openCodeSettingsAgents(t, settings)
	for _, name := range []string{"sdd-apply", "sdd-apply-fallback", "sdd-orchestrator-fallback"} {
		if _, ok := got[name]; ok {
			t.Errorf("Gentle-owned Kilocode agent.%s survived sync", name)
		}
	}
	if !reflect.DeepEqual(got["sdd-init"], userSDDInit) {
		t.Errorf("user-defined Kilocode agent.sdd-init changed: %#v", got["sdd-init"])
	}
	if !hasManualAction(result.ManualActions, settings, "sdd-init", "move or delete it") {
		t.Errorf("preserved Kilocode agent.sdd-init not reported: %v", result.ManualActions)
	}
	if _, ok := backupManifestEntries(t, home)[settings]; !ok {
		t.Error("rollback snapshot omitted the Kilocode settings")
	}
}

// The legacy marker step never strips the marker from a retired SDD agent:
// that turned Gentle AI's entry into one that looks user-authored, so the
// retirement step could no longer prove ownership and delete it.
func TestSyncMarkerStepNeverStripsRetiredSDDAgentMarkers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.json")
	mustWriteFile(t, path, []byte(`{"agent":`+indentedJSON(t, releasedOpenCodeSDDAgents(t, "v1", "sdd-apply"))+`}`))
	step := openCodeMarkerMigrationSyncStep{path: path}
	if err := step.Run(); err != nil {
		t.Fatal(err)
	}
	agent, _ := openCodeSettingsAgents(t, path)
	if entry, ok := agent["sdd-apply"].(map[string]any); ok && entry["__managed_by"] == nil {
		t.Fatal("marker step stripped the ownership marker from a retired SDD agent")
	}
}

// Install retires the same settings entries and prompts as sync and declares
// them in its snapshot; a workspace-scoped install never touches them.
func TestRunInstallRetiresOwnedOpenCodeSDDSettingsOnlyInGlobalScope(t *testing.T) {
	home := installTestHome(t)
	restoreBackupHome := backup.UserHomeDirFn
	backup.UserHomeDirFn = func() (string, error) { return home, nil }
	t.Cleanup(func() { backup.UserHomeDirFn = restoreBackupHome })
	setOpenCodeTestHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("OPENCODE_CONFIG_DIR", "")
	stubOpenCodeRuntimeVersion(t, home, "1.18.30")
	config := opencodeagent.ConfigPath(home)
	settings := filepath.Join(config, "opencode.json")
	mustWriteFile(t, settings, []byte(`{"agent":`+indentedJSON(t, releasedOpenCodeSDDAgents(t, "v1", "sdd-apply", "sdd-apply-fallback"))+`}`))
	prompt := filepath.Join(config, "prompts", "sdd", "sdd-apply.md")
	mustWriteFile(t, prompt, releasedSDDRender(t, "opencode-prompt-sdd-apply.md"))

	t.Chdir(t.TempDir())
	if _, err := RunInstall([]string{"--agent", "opencode", "--component", "skills", "--scope", "workspace"}, system.DetectionResult{}); err != nil {
		t.Fatalf("workspace RunInstall() error = %v", err)
	}
	// OpenCode routing guidance is global in every scope; SDD retirement is not.
	agent, _ := openCodeSettingsAgents(t, settings)
	for _, name := range []string{"sdd-apply", "sdd-apply-fallback"} {
		if _, ok := agent[name]; !ok {
			t.Errorf("workspace-scoped install retired global agent.%s", name)
		}
	}
	if _, err := os.Stat(prompt); err != nil {
		t.Fatalf("workspace-scoped install removed a global SDD prompt: %v", err)
	}

	if _, err := RunInstall([]string{"--agent", "opencode", "--component", "skills"}, system.DetectionResult{}); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	agent, _ = openCodeSettingsAgents(t, settings)
	for _, name := range []string{"sdd-apply", "sdd-apply-fallback"} {
		if _, ok := agent[name]; ok {
			t.Errorf("install kept Gentle-owned agent.%s", name)
		}
	}
	if _, err := os.Lstat(prompt); !os.IsNotExist(err) {
		t.Errorf("install kept the released SDD prompt: %v", err)
	}
	snapshot := backupManifestEntries(t, home)
	for _, path := range []string{settings, prompt} {
		if _, ok := snapshot[path]; !ok {
			t.Errorf("install snapshot omitted %s", path)
		}
	}
}
