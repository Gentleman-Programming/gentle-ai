package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/antigravityrules"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/planner"
)

const legacyGeminiUserContent = "# My own Gemini rules\n\nAlways cite sources.\n"

// stubAntigravityHome fakes an installed Antigravity whose GEMINI.md still
// carries the sections an older release appended, with no gemini binary.
func stubAntigravityHome(t *testing.T, geminiOnPath bool) string {
	t.Helper()
	home := t.TempDir()
	restoreHome, restoreCommand, restoreLookPath := osUserHomeDir, runCommand, cmdLookPath
	t.Cleanup(func() { osUserHomeDir, runCommand, cmdLookPath = restoreHome, restoreCommand, restoreLookPath })
	osUserHomeDir = func() (string, error) { return home, nil }
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = func(name string) (string, error) {
		if name == "gemini" && !geminiOnPath {
			return "", exec.ErrNotFound
		}
		return "/usr/local/bin/" + name, nil
	}
	if err := os.MkdirAll(filepath.Join(home, ".gemini", "antigravity"), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := filemerge.InjectMarkdownSection(legacyGeminiUserContent, "persona", "legacy antigravity persona\n")
	legacy = filemerge.InjectMarkdownSection(legacy, "engram-protocol", "legacy engram protocol\n")
	if err := os.WriteFile(antigravityrules.LegacyPromptPath(home), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

func assertPublishedAntigravityRules(t *testing.T, home string, sections ...string) {
	t.Helper()
	owned, err := antigravityrules.OwnedRulePaths(home)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(owned, "\n")
	for _, section := range sections {
		if !strings.Contains(joined, "-"+section) {
			t.Errorf("no published rule for section %q; got:\n%s", section, joined)
		}
	}
	for _, path := range owned {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() > antigravityrules.MaxRuleFileBytes {
			t.Errorf("%s is %d bytes, want <= %d", path, info.Size(), antigravityrules.MaxRuleFileBytes)
		}
	}
}

func readString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return string(data)
}

func TestRunInstallAntigravityPublishesRulesAndRetiresLegacyGeminiSections(t *testing.T) {
	home := stubAntigravityHome(t, false)

	result, err := RunInstall([]string{"--agent", "antigravity", "--component", "persona", "--component", "engram", "--component", "context7"}, macOSDetectionResult())
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if !result.Verify.Ready {
		t.Fatalf("verification ready = false")
	}

	assertPublishedAntigravityRules(t, home, "persona", "engram-protocol", "orchestrator", "agent-routing")
	if got := readString(t, antigravityrules.LegacyPromptPath(home)); got != legacyGeminiUserContent {
		t.Fatalf("GEMINI.md = %q, want only the user content %q", got, legacyGeminiUserContent)
	}
}

func TestRunInstallGeminiCLIOnlyLeavesAntigravitySurfacesUntouched(t *testing.T) {
	home := t.TempDir()
	restoreHome, restoreCommand, restoreLookPath := osUserHomeDir, runCommand, cmdLookPath
	t.Cleanup(func() { osUserHomeDir, runCommand, cmdLookPath = restoreHome, restoreCommand, restoreLookPath })
	osUserHomeDir = func() (string, error) { return home, nil }
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = func(name string) (string, error) { return "/usr/local/bin/" + name, nil }

	if _, err := RunInstall([]string{"--agent", "gemini-cli", "--component", "persona", "--component", "engram", "--component", "context7"}, macOSDetectionResult()); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	gemini := importedGuidance(t, home)
	for _, want := range []string{"<!-- gentle-ai:engram-protocol -->", "<!-- gentle-ai:agent-routing -->"} {
		if !strings.Contains(gemini, want) {
			t.Errorf("Gemini CLI guidance imported by GEMINI.md missing %q", want)
		}
	}
	for _, path := range []string{antigravityrules.RulesDir(home), antigravityrules.StagingPath(home)} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("Gemini CLI install created Antigravity surface %s; stat err = %v", path, err)
		}
	}
}

func TestRunSyncAntigravityRepublishesRulesAndReportsThem(t *testing.T) {
	home := stubAntigravityHome(t, false)
	stale := filepath.Join(antigravityrules.RulesDir(home), "gentle-ai-09-retired.md")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("---\ntrigger: always_on\n---\n\nold\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	selection := model.Selection{Agents: []model.AgentID{model.AgentAntigravity}, Components: []model.ComponentID{model.ComponentPersona}, Persona: model.PersonaGentleman}
	result, err := RunSyncWithSelection(home, selection)
	if err != nil {
		t.Fatalf("RunSyncWithSelection() error = %v", err)
	}

	assertPublishedAntigravityRules(t, home, "persona", "agent-routing")
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale owned rule survived sync; stat err = %v", err)
	}
	if got := readString(t, antigravityrules.LegacyPromptPath(home)); got != legacyGeminiUserContent {
		t.Fatalf("GEMINI.md = %q, want %q", got, legacyGeminiUserContent)
	}
	changed := strings.Join(result.ChangedFiles, "\n")
	if !strings.Contains(changed, antigravityrules.RulesDir(home)) || !strings.Contains(changed, antigravityrules.LegacyPromptPath(home)) {
		t.Fatalf("sync changed files do not report the rules and GEMINI.md migration:\n%s", changed)
	}

	again, err := RunSyncWithSelection(home, selection)
	if err != nil {
		t.Fatalf("second RunSyncWithSelection() error = %v", err)
	}
	if strings.Contains(strings.Join(again.ChangedFiles, "\n"), antigravityrules.RulesDir(home)) {
		t.Fatalf("idempotent sync reported rule changes: %v", again.ChangedFiles)
	}
}

func TestInstallAndSyncBackupsCoverAntigravityRules(t *testing.T) {
	home := stubAntigravityHome(t, false)
	stale := filepath.Join(antigravityrules.RulesDir(home), "gentle-ai-09-retired.md")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("---\ntrigger: always_on\n---\n\nold\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	selection := model.Selection{Agents: []model.AgentID{model.AgentAntigravity}, Components: []model.ComponentID{model.ComponentPersona}}

	syncTargets, err := syncBackupTargetsScoped(home, "", ScopeGlobal, selection, resolveAdapters(selection.Agents))
	if err != nil {
		t.Fatal(err)
	}
	installTargets, err := installBackupTargets(home, "", ScopeGlobal, selection, planner.ResolvedPlan{Agents: selection.Agents, OrderedComponents: selection.Components}, false)
	if err != nil {
		t.Fatal(err)
	}
	for flow, targets := range map[string][]string{"sync": syncTargets, "install": installTargets} {
		for _, want := range []string{stale, antigravityrules.LegacyPromptPath(home), antigravityrules.StagingPath(home)} {
			if !strings.Contains(strings.Join(targets, "\n"), want) {
				t.Errorf("%s backup targets missing %s", flow, want)
			}
		}
	}
}
