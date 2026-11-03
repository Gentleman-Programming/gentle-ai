package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/antigravityrules"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/geminimodules"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
)

// personaCopies counts persona bodies: the fingerprint appears once in every
// generic persona asset and nowhere else in the managed guidance.
func personaCopies(content string) int {
	return strings.Count(content, `Never add "Co-Authored-By"`)
}

// seedBaselineGeminiAndAntigravity reproduces what releases before agy rules
// left when both agents were installed: Antigravity stripped Gemini CLI's
// leading persona and appended a marked one, the only persona copy (#5608).
func seedBaselineGeminiAndAntigravity(t *testing.T, home string) string {
	t.Helper()
	legacy := filemerge.InjectMarkdownSection(legacyGeminiUserContent, "persona", assets.MustRead("generic/persona-gentleman.md"))
	legacy = filemerge.InjectMarkdownSection(legacy, "engram-protocol", "gemini engram protocol\n")
	if err := os.WriteFile(antigravityrules.LegacyPromptPath(home), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := state.Write(home, state.InstallState{InstalledAgents: []string{"gemini-cli", "antigravity"}}); err != nil {
		t.Fatal(err)
	}
	return legacy
}

func TestRunInstallAntigravityKeepsGeminiPersonaWhenGeminiCLIManaged(t *testing.T) {
	home := stubAntigravityHome(t, false)
	legacy := seedBaselineGeminiAndAntigravity(t, home)

	if _, err := RunInstall([]string{"--agent", "antigravity", "--component", "persona", "--component", "engram", "--component", "context7"}, macOSDetectionResult()); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	got := readString(t, antigravityrules.LegacyPromptPath(home))
	if got != legacy {
		t.Fatalf("Antigravity rewrote the GEMINI.md Gemini CLI owns:\n%s", got)
	}
	if n := personaCopies(got); n != 1 {
		t.Fatalf("GEMINI.md has %d persona copies, want 1", n)
	}
	assertPublishedAntigravityRules(t, home, "persona", "engram-protocol")
}

func TestRunInstallAntigravityStripsLegacySectionsWithUnmanagedGeminiOnPath(t *testing.T) {
	home := stubAntigravityHome(t, true)

	if _, err := RunInstall([]string{"--agent", "antigravity", "--component", "persona", "--component", "engram", "--component", "context7"}, macOSDetectionResult()); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	if got := readString(t, antigravityrules.LegacyPromptPath(home)); got != legacyGeminiUserContent {
		t.Fatalf("GEMINI.md = %q, want only the user content: a gemini binary on PATH is not a managed Gemini CLI", got)
	}
}

// TestGeminiAndAntigravityKeepOnePersonaInAnyOrder installs and syncs both
// agents in different orders and personas; GEMINI.md must always carry
// exactly one persona.
func TestGeminiAndAntigravityKeepOnePersonaInAnyOrder(t *testing.T) {
	both := []model.AgentID{model.AgentGeminiCLI, model.AgentAntigravity}
	install := func(agent model.AgentID, persona model.PersonaID) func(*testing.T, string) {
		return func(t *testing.T, _ string) {
			args := []string{"--agent", string(agent), "--persona", string(persona), "--component", "persona", "--component", "engram", "--component", "context7"}
			if _, err := RunInstall(args, macOSDetectionResult()); err != nil {
				t.Fatalf("RunInstall(%v) error = %v", args, err)
			}
		}
	}
	sync := func(agents []model.AgentID, persona model.PersonaID) func(*testing.T, string) {
		return func(t *testing.T, home string) {
			selection := model.Selection{Agents: agents, Components: []model.ComponentID{model.ComponentPersona}, Persona: persona}
			if _, err := RunSyncWithSelection(home, selection); err != nil {
				t.Fatalf("RunSyncWithSelection(%v, %s) error = %v", agents, persona, err)
			}
		}
	}
	tests := []struct {
		name   string
		legacy bool
		steps  []func(*testing.T, string)
	}{
		{name: "gemini then antigravity", steps: []func(*testing.T, string){
			install(model.AgentGeminiCLI, model.PersonaGentleman), install(model.AgentAntigravity, model.PersonaGentleman), sync(both, model.PersonaGentleman)}},
		{name: "antigravity then gemini neutral", steps: []func(*testing.T, string){
			install(model.AgentAntigravity, model.PersonaGentleman), install(model.AgentGeminiCLI, model.PersonaNeutral), sync([]model.AgentID{model.AgentAntigravity}, model.PersonaNeutral)}},
		{name: "baseline both then antigravity sync then gemini neutral", legacy: true, steps: []func(*testing.T, string){
			sync([]model.AgentID{model.AgentAntigravity}, model.PersonaGentleman), install(model.AgentGeminiCLI, model.PersonaNeutral), sync(both, model.PersonaNeutral)}},
		{name: "baseline both then gemini sync neutral", legacy: true, steps: []func(*testing.T, string){
			sync([]model.AgentID{model.AgentGeminiCLI}, model.PersonaNeutral), install(model.AgentAntigravity, model.PersonaNeutral)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := stubAntigravityHome(t, false)
			if err := os.Remove(antigravityrules.LegacyPromptPath(home)); err != nil {
				t.Fatal(err)
			}
			if tt.legacy {
				seedBaselineGeminiAndAntigravity(t, home)
			}
			for i, step := range tt.steps {
				step(t, home)
				// GEMINI.md belongs to Gemini CLI: before Gentle AI manages
				// it, Antigravity must not have created it at all.
				data, err := os.ReadFile(antigravityrules.LegacyPromptPath(home))
				if !antigravityrules.GeminiCLIManaged(home, nil) {
					if !os.IsNotExist(err) {
						t.Fatalf("after step %d Antigravity alone left GEMINI.md: %q (%v)", i+1, data, err)
					}
					continue
				}
				// Gemini CLI loads GEMINI.md plus the modules it imports.
				guidance := string(data)
				if strings.Contains(guidance, "<!-- gentle-ai:"+geminimodules.ImportSectionID+" -->") {
					guidance += importedGuidance(t, home)
				}
				if n := personaCopies(guidance); n != 1 {
					t.Fatalf("after step %d Gemini CLI guidance has %d persona copies, want 1:\n%s", i+1, n, guidance)
				}
			}
		})
	}
}

// TestRunInstallAntigravityRetiresUnmanagedGeminiModules covers a host whose
// Gemini CLI is no longer managed: nothing imports the modules an earlier
// install published, so Antigravity removes them with the import block and
// keeps the user content and other files.
func TestRunInstallAntigravityRetiresUnmanagedGeminiModules(t *testing.T) {
	home := stubAntigravityHome(t, true)
	dir := geminimodules.ModulesDir(home)
	root := filemerge.InjectMarkdownSection(legacyGeminiUserContent, geminimodules.ImportSectionID, "@./gentle-ai/01-persona.md\n")
	for path, content := range map[string]string{
		geminimodules.StagingPath(home):     "persona\n",
		filepath.Join(dir, "01-persona.md"): "persona\n",
		filepath.Join(dir, "notes.md"):      "mine\n",
		geminimodules.RootPath(home):        root,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := RunInstall([]string{"--agent", "antigravity", "--component", "persona", "--component", "engram", "--component", "context7"}, macOSDetectionResult()); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	if got := readString(t, geminimodules.RootPath(home)); got != legacyGeminiUserContent {
		t.Fatalf("GEMINI.md = %q, want only the user content", got)
	}
	for _, path := range []string{geminimodules.StagingPath(home), filepath.Join(dir, "01-persona.md")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s kept; stat err = %v", path, err)
		}
	}
	if got := readString(t, filepath.Join(dir, "notes.md")); got != "mine\n" {
		t.Fatalf("notes.md = %q, want untouched", got)
	}
}
