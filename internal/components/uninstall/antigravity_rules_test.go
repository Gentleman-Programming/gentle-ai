package uninstall

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/gemini"
	"github.com/gentleman-programming/gentle-ai/v4/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/antigravityrules"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/persona"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

const legacyGeminiUser = "# Personal Gemini rules\n\nPrefer tables.\n"

// installedAntigravityRulesForTest installs persona and Engram for Antigravity,
// publishes the rules, and leaves a user rule plus a legacy GEMINI.md.
func installedAntigravityRulesForTest(t *testing.T) (*Service, string, string) {
	t.Helper()
	home := t.TempDir()
	restore := gemini.LookPathOverride
	gemini.LookPathOverride = func(string) (string, error) { return "", exec.ErrNotFound }
	t.Cleanup(func() { gemini.LookPathOverride = restore })
	engram.SetLookPathForTest(t, "/opt/homebrew/bin/engram", "")

	svc, err := NewService(home, t.TempDir(), "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	adapter, _ := svc.registry.Get(model.AgentAntigravity)
	if _, err := persona.Inject(home, adapter, model.PersonaGentleman); err != nil {
		t.Fatal(err)
	}
	if _, err := engram.Inject(home, adapter); err != nil {
		t.Fatal(err)
	}
	if _, err := antigravityrules.Publish(home); err != nil {
		t.Fatal(err)
	}
	userRule := filepath.Join(antigravityrules.RulesDir(home), "team.md")
	if err := os.WriteFile(userRule, []byte("---\ntrigger: always_on\n---\n\nteam rule\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	legacy := filemerge.InjectMarkdownSection(legacyGeminiUser, "persona", "legacy persona\n")
	if err := os.WriteFile(antigravityrules.LegacyPromptPath(home), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	return svc, home, userRule
}

func ownedRuleNames(t *testing.T, home string) string {
	t.Helper()
	owned, err := antigravityrules.OwnedRulePaths(home)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(owned))
	for _, path := range owned {
		names = append(names, filepath.Base(path))
	}
	return strings.Join(names, ",")
}

func TestPartialUninstallAntigravityRetiresPublishedRulesAndLegacySections(t *testing.T) {
	svc, home, userRule := installedAntigravityRulesForTest(t)
	published, err := antigravityrules.OwnedRulePaths(home)
	if err != nil || len(published) == 0 {
		t.Fatalf("setup published no rules: %v %v", published, err)
	}

	result, err := svc.PartialUninstall([]model.AgentID{model.AgentAntigravity}, nil)
	if err != nil {
		t.Fatalf("PartialUninstall() error = %v", err)
	}

	if got := ownedRuleNames(t, home); got != "" {
		t.Fatalf("published rules survived uninstall: %s", got)
	}
	for _, path := range published {
		if !slices.Contains(result.RemovedFiles, path) {
			t.Errorf("removed files do not report %s: %v", path, result.RemovedFiles)
		}
		if !slices.ContainsFunc(result.Manifest.Entries, func(entry backup.ManifestEntry) bool { return entry.OriginalPath == path }) {
			t.Errorf("uninstall snapshot does not cover %s", path)
		}
	}
	if _, err := os.Stat(userRule); err != nil {
		t.Fatalf("user rule was removed: %v", err)
	}
	if _, err := os.Stat(antigravityrules.StagingPath(home)); !os.IsNotExist(err) {
		t.Fatalf("staging file survived uninstall; stat err = %v", err)
	}
	data, err := os.ReadFile(antigravityrules.LegacyPromptPath(home))
	if err != nil || string(data) != legacyGeminiUser {
		t.Fatalf("GEMINI.md = %q (%v), want %q", data, err, legacyGeminiUser)
	}
}

func TestPartialUninstallAntigravityComponentRepublishesRemainingRules(t *testing.T) {
	svc, home, _ := installedAntigravityRulesForTest(t)

	if _, err := svc.PartialUninstall([]model.AgentID{model.AgentAntigravity}, []model.ComponentID{model.ComponentEngram}); err != nil {
		t.Fatalf("PartialUninstall(engram) error = %v", err)
	}

	if got := ownedRuleNames(t, home); got != "gentle-ai-01-persona.md" {
		t.Fatalf("rules after Engram uninstall = %q, want only the persona rule", got)
	}
	data, err := os.ReadFile(antigravityrules.LegacyPromptPath(home))
	if err != nil || !strings.Contains(string(data), "<!-- gentle-ai:persona -->") {
		t.Fatalf("component uninstall must not rewrite the legacy GEMINI.md; got %q (%v)", data, err)
	}
}

// TestPartialUninstallAntigravityKeepsRetainedGuidanceWithoutManualNote: the
// routing guidance is retained on uninstall for every agent, so a staging file
// that still carries it must not be reported as non-managed leftovers.
func TestPartialUninstallAntigravityKeepsRetainedGuidanceWithoutManualNote(t *testing.T) {
	svc, home, _ := installedAntigravityRulesForTest(t)
	staging := antigravityrules.StagingPath(home)
	data, err := os.ReadFile(staging)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staging, []byte(filemerge.InjectMarkdownSection(string(data), "agent-routing", "routing\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := svc.PartialUninstall([]model.AgentID{model.AgentAntigravity}, nil)
	if err != nil {
		t.Fatalf("PartialUninstall() error = %v", err)
	}
	for _, note := range result.ManualActions {
		if strings.Contains(note, filepath.Dir(staging)) {
			t.Fatalf("uninstall reported the Gentle AI staging directory as non-managed: %q", note)
		}
	}
	if got := ownedRuleNames(t, home); got != "gentle-ai-01-agent-routing.md" {
		t.Fatalf("rules after uninstall = %q, want only the retained routing rule", got)
	}
}

func TestPartialUninstallAntigravityCleansUpWorkspaceGuidanceAndPreservesUserContent(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()

	restore := gemini.LookPathOverride
	gemini.LookPathOverride = func(string) (string, error) { return "", exec.ErrNotFound }
	t.Cleanup(func() { gemini.LookPathOverride = restore })
	engram.SetLookPathForTest(t, "/opt/homebrew/bin/engram", "")

	svc, err := NewService(home, workspace, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	// Setup workspace staging and AGENTS.md with Antigravity sections + user instructions
	wsStagingPath := antigravityrules.StagingPath(workspace)
	if err := os.MkdirAll(filepath.Dir(wsStagingPath), 0o755); err != nil {
		t.Fatal(err)
	}
	wsStaging := filemerge.InjectMarkdownSection("", "persona", "Workspace persona.\n")
	wsStaging = filemerge.InjectMarkdownSection(wsStaging, "engram-protocol", "Workspace engram.\n")
	if err := os.WriteFile(wsStagingPath, []byte(wsStaging), 0o644); err != nil {
		t.Fatal(err)
	}

	wsAgentsPath := antigravityrules.AgentsMDPath(workspace)
	userInstructions := "# Project Readme\n\nMust pass CI.\n\n"
	wsAgentsContent := userInstructions + wsStaging
	if err := os.WriteFile(wsAgentsPath, []byte(wsAgentsContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Publish initially in workspace
	if _, err := antigravityrules.Publish(workspace, antigravityrules.WithWorkspaceScope(true)); err != nil {
		t.Fatalf("Publish(workspace) error = %v", err)
	}

	// Also setup home so uninstall has home staging
	homeStagingPath := antigravityrules.StagingPath(home)
	if err := os.MkdirAll(filepath.Dir(homeStagingPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(homeStagingPath, []byte(wsStaging), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := antigravityrules.Publish(home); err != nil {
		t.Fatalf("Publish(home) error = %v", err)
	}

	// Perform full Antigravity uninstall
	result, err := svc.PartialUninstall([]model.AgentID{model.AgentAntigravity}, nil)
	if err != nil {
		t.Fatalf("PartialUninstall() error = %v", err)
	}

	// 1. Workspace AGENTS.md must preserve user content
	afterData, err := os.ReadFile(wsAgentsPath)
	if err != nil {
		t.Fatalf("ReadFile(workspace AGENTS.md) error = %v", err)
	}
	afterContent := string(afterData)
	if !strings.Contains(afterContent, "Must pass CI.") {
		t.Fatalf("user content was removed from workspace AGENTS.md:\n%s", afterContent)
	}
	// 2. Antigravity sections must be removed
	if strings.Contains(afterContent, "<!-- gentle-ai:persona -->") {
		t.Fatalf("persona section survived in workspace AGENTS.md:\n%s", afterContent)
	}
	if strings.Contains(afterContent, "<!-- gentle-ai:engram-protocol -->") {
		t.Fatalf("engram-protocol section survived in workspace AGENTS.md:\n%s", afterContent)
	}
	// 3. Workspace config AGENTS.md must be removed
	if _, err := os.Stat(antigravityrules.ConfigAgentsMDPath(workspace)); !os.IsNotExist(err) {
		t.Fatalf("workspace config AGENTS.md survived uninstall: %v", err)
	}
	// 4. Workspace AGENTS.md must be recorded in backup manifest
	foundInBackup := false
	for _, entry := range result.Manifest.Entries {
		if entry.OriginalPath == wsAgentsPath {
			foundInBackup = true
			break
		}
	}
	if !foundInBackup {
		t.Fatalf("workspace AGENTS.md was not included in uninstall backup snapshot: %+v", result.Manifest.Entries)
	}
}
