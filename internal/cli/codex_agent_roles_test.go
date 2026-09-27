package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/gentleman-programming/gentle-ai/v3/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/planner"
	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
)

type codexRoleTOML struct {
	Name                  string   `toml:"name"`
	Description           string   `toml:"description"`
	DeveloperInstructions string   `toml:"developer_instructions"`
	NicknameCandidates    []string `toml:"nickname_candidates"`
}

func readCodexOwnershipLedger(t *testing.T, dir string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, reviewassets.OwnershipLedgerFilename))
	if err != nil {
		t.Fatalf("read ownership ledger: %v", err)
	}
	var ledger struct {
		Version int               `json:"version"`
		Files   map[string]string `json:"files"`
	}
	if err := json.Unmarshal(raw, &ledger); err != nil {
		t.Fatalf("decode ownership ledger: %v", err)
	}
	return ledger.Files
}

func codexAgentRoleDirSnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		files[entry.Name()] = string(content)
	}
	return files
}

// TestCodexInstallWritesManagedAgentRolesWithOwnershipLedger is the inventory
// contract: a fresh `install --agent codex` writes all 8 managed role files
// under ~/.codex/agents, each schema-valid and ledger-owned, and never touches
// ~/.codex/config.toml.
func TestCodexInstallWritesManagedAgentRolesWithOwnershipLedger(t *testing.T) {
	home := t.TempDir()
	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(codexDir, "config.toml")
	configBytes := []byte("[agents]\nmax_depth = 2\nmax_threads = 4\n")
	if err := os.WriteFile(configPath, configBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	selection := model.Selection{Agents: []model.AgentID{model.AgentCodex}}
	runInstallInjectionSteps(t, newTestInstallRuntime(t, home, selection))

	dir := reviewassets.CodexAgentRoleDir(home)
	names := reviewassets.CodexAgentRoleNames()
	if len(names) != 8 {
		t.Fatalf("managed Codex role count = %d, want 8", len(names))
	}
	ledger := readCodexOwnershipLedger(t, dir)
	for _, name := range names {
		path := filepath.Join(dir, name)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("managed role %s missing: %v", name, err)
		}
		var parsed codexRoleTOML
		if _, err := toml.Decode(string(raw), &parsed); err != nil {
			t.Errorf("role %s is not schema-valid TOML: %v", name, err)
		}
		if parsed.Name == "" || parsed.Description == "" || parsed.DeveloperInstructions == "" || len(parsed.NicknameCandidates) == 0 {
			t.Errorf("role %s is missing required Codex fields: %+v", name, parsed)
		}
		sum := sha256.Sum256(raw)
		if got := ledger[name]; got != hex.EncodeToString(sum[:]) {
			t.Errorf("ledger entry for %s = %q, want sha256 of installed bytes", name, got)
		}
	}
	if got, err := os.ReadFile(configPath); err != nil || string(got) != string(configBytes) {
		t.Fatalf("~/.codex/config.toml changed: %q, %v; want untouched", got, err)
	}
}

// TestCodexInstallAgentRolesAreByteIdenticalOnReRun pins idempotency.
func TestCodexInstallAgentRolesAreByteIdenticalOnReRun(t *testing.T) {
	home := t.TempDir()
	selection := model.Selection{Agents: []model.AgentID{model.AgentCodex}}
	rt := newTestInstallRuntime(t, home, selection)
	runInstallInjectionSteps(t, rt)

	dir := reviewassets.CodexAgentRoleDir(home)
	before := codexAgentRoleDirSnapshot(t, dir)
	runInstallInjectionSteps(t, rt)
	if after := codexAgentRoleDirSnapshot(t, dir); !reflect.DeepEqual(before, after) {
		t.Fatalf("second install changed ~/.codex/agents bytes")
	}
}

// TestCodexSyncWritesManagedAgentRolesIdempotently covers the sync wiring.
func TestCodexSyncWritesManagedAgentRolesIdempotently(t *testing.T) {
	home := t.TempDir()
	selection := model.Selection{Agents: []model.AgentID{model.AgentCodex}}
	runSyncInjectionSteps(t, home, selection)

	dir := reviewassets.CodexAgentRoleDir(home)
	before := codexAgentRoleDirSnapshot(t, dir)
	for _, name := range reviewassets.CodexAgentRoleNames() {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("sync did not install managed role %s: %v", name, err)
		}
	}
	runSyncInjectionSteps(t, home, selection)
	if after := codexAgentRoleDirSnapshot(t, dir); !reflect.DeepEqual(before, after) {
		t.Fatalf("second sync changed ~/.codex/agents bytes")
	}
}

// TestCodexInstallPreservesUserAuthoredAgentRole is the user-file protection
// contract: an unowned ~/.codex/agents/<role>.toml survives install untouched.
func TestCodexInstallPreservesUserAuthoredAgentRole(t *testing.T) {
	home := t.TempDir()
	dir := reviewassets.CodexAgentRoleDir(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	userPath := filepath.Join(dir, "review-risk.toml")
	userBytes := []byte("name = \"review-risk\"\ndescription = \"my own risk reviewer\"\ndeveloper_instructions = \"mine\"\n")
	if err := os.WriteFile(userPath, userBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	selection := model.Selection{Agents: []model.AgentID{model.AgentCodex}}
	rt := newTestInstallRuntime(t, home, selection)
	runInstallInjectionSteps(t, rt)

	if got, err := os.ReadFile(userPath); err != nil || string(got) != string(userBytes) {
		t.Fatalf("user-authored role = %q, %v; want preserved", got, err)
	}
	if !strings.Contains(strings.Join(rt.state.nativeReviewActions, "\n"), userPath) {
		t.Fatalf("preserved user role not reported as a manual action: %v", rt.state.nativeReviewActions)
	}
	for _, name := range reviewassets.CodexAgentRoleNames() {
		if name == "review-risk.toml" {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("managed role %s missing after preserving a user file: %v", name, err)
		}
	}
	if _, ok := readCodexOwnershipLedger(t, dir)["review-risk.toml"]; ok {
		t.Error("ownership ledger must not claim the user-authored role")
	}
}

// TestCodexWorkspaceScopedInstallKeepsAgentRolesHomeBased locks the CODEX_HOME
// exception: a workspace-scoped install still writes the 9 Codex role files
// (8 roles + ledger) under HOME/.codex/agents and nothing under
// <workspace>/.codex/agents, while the rest of the workspace-scoped Codex files
// still land under <workspace>/.codex/.
func TestCodexWorkspaceScopedInstallKeepsAgentRolesHomeBased(t *testing.T) {
	home, workspace := t.TempDir(), t.TempDir()
	selection := model.Selection{Agents: []model.AgentID{model.AgentCodex}}
	rt, err := newInstallRuntime(home, ScopeWorkspace, ChannelStable, selection, planner.ResolvedPlan{Agents: selection.Agents}, system.PlatformProfile{})
	if err != nil {
		t.Fatalf("newInstallRuntime() error = %v", err)
	}
	rt.workspaceDir = workspace
	runInstallInjectionSteps(t, rt)

	homeDir := reviewassets.CodexAgentRoleDir(home)
	rolePaths := append(append([]string{}, reviewassets.CodexAgentRoleNames()...), reviewassets.OwnershipLedgerFilename)
	if len(rolePaths) != 9 {
		t.Fatalf("expected 9 home-based Codex role paths (8 roles + ledger), got %d", len(rolePaths))
	}
	for _, name := range rolePaths {
		if _, err := os.Stat(filepath.Join(homeDir, name)); err != nil {
			t.Fatalf("workspace-scoped install must write %s under HOME: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(workspace, ".codex", "agents", name)); !os.IsNotExist(err) {
			t.Fatalf("workspace-scoped install wrote <workspace>/.codex/agents/%s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(workspace, ".codex", "agents")); !os.IsNotExist(err) {
		t.Fatalf("workspace-scoped install created <workspace>/.codex/agents: %v", err)
	}
	// Scope still applies to the rest of the Codex files.
	if _, err := os.Stat(filepath.Join(workspace, ".codex", "AGENTS.md")); err != nil {
		t.Fatalf("workspace-scoped Codex guidance should land under <workspace>/.codex: %v", err)
	}
	// Backup inventory stays home-based and consistent with the writer.
	targets, err := backupTargets(home, workspace, ScopeWorkspace, selection, planner.ResolvedPlan{Agents: selection.Agents})
	if err != nil {
		t.Fatalf("backupTargets() error = %v", err)
	}
	for _, name := range rolePaths {
		if !containsPath(targets, filepath.Join(homeDir, name)) {
			t.Errorf("backupTargets missing home-based Codex role path %s", name)
		}
		if containsPath(targets, filepath.Join(workspace, ".codex", "agents", name)) {
			t.Errorf("backupTargets must not contain a workspace-based Codex role path %s", name)
		}
	}
}

// TestCodexAgentRolesIgnoreCodexHomeForTempHome documents the real-user-home
// guard in reviewassets.CodexAgentRoleDir: an absolute CODEX_HOME is honoured
// only for the real user home, so a lifecycle run under an isolated temp HOME
// still installs every managed role under <temp home>/.codex/agents and never
// writes into an ambient CODEX_HOME. This is the case a temp-home lifecycle
// test can pin; the override branch itself is covered in the reviewassets unit
// test against os.UserHomeDir().
func TestCodexAgentRolesIgnoreCodexHomeForTempHome(t *testing.T) {
	home := t.TempDir()
	codexHome := filepath.Join(t.TempDir(), "codex-home")
	t.Setenv("CODEX_HOME", codexHome)

	selection := model.Selection{Agents: []model.AgentID{model.AgentCodex}}
	runInstallInjectionSteps(t, newTestInstallRuntime(t, home, selection))

	dir := filepath.Join(home, ".codex", "agents")
	for _, name := range reviewassets.CodexAgentRoleNames() {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("managed role %s not under the temp home: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, reviewassets.OwnershipLedgerFilename)); err != nil {
		t.Fatalf("ownership ledger not under the temp home: %v", err)
	}
	if _, err := os.Stat(filepath.Join(codexHome, "agents")); !os.IsNotExist(err) {
		t.Fatalf("temp-home install wrote into the ambient CODEX_HOME: %v", err)
	}
}
