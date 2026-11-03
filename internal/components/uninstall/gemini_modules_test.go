package uninstall

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/geminimodules"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
)

func geminiUninstallService(t *testing.T, files map[string]string) (*Service, string) {
	t.Helper()
	home := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := state.Write(home, state.InstallState{InstalledAgents: []string{"gemini-cli"}}); err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(home, t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	svc.snapshotter = stubSnapshotter{}
	return svc, home
}

func TestPartialUninstallGeminiRemovesModulesAndImportBlock(t *testing.T) {
	staging := filemerge.InjectMarkdownSection(assets.MustRead("generic/persona-gentleman.md"), "engram-protocol", "engram\n")
	svc, home := geminiUninstallService(t, map[string]string{".gemini/gentle-ai/guidance.md": staging, ".gemini/GEMINI.md": legacyGeminiUser})
	if _, err := geminimodules.Publish(home); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.PartialUninstall([]model.AgentID{model.AgentGeminiCLI}, []model.ComponentID{model.ComponentPersona, model.ComponentEngram}); err != nil {
		t.Fatalf("PartialUninstall() error = %v", err)
	}

	if got := readTestFile(t, geminimodules.RootPath(home)); got != legacyGeminiUser {
		t.Fatalf("GEMINI.md = %q, want only the user content", got)
	}
	for _, path := range []string{geminimodules.StagingPath(home), geminimodules.ModulesDir(home)} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s kept; stat err = %v", path, err)
		}
	}
}

// TestPartialUninstallGeminiMigratesMonolithicGEMINIMD covers an install by an
// older release: the guidance still sits in GEMINI.md, so uninstall moves it
// to the staging file first and removes exactly what the components own.
func TestPartialUninstallGeminiMigratesMonolithicGEMINIMD(t *testing.T) {
	monolithic := assets.MustRead("generic/persona-gentleman.md") + "\n" +
		filemerge.InjectMarkdownSection(filemerge.InjectMarkdownSection("", "engram-protocol", "engram\n"), "agent-routing", "routing\n") + "\n# My notes\n"
	svc, home := geminiUninstallService(t, map[string]string{".gemini/GEMINI.md": monolithic})

	if _, err := svc.PartialUninstall([]model.AgentID{model.AgentGeminiCLI}, []model.ComponentID{model.ComponentPersona, model.ComponentEngram}); err != nil {
		t.Fatalf("PartialUninstall() error = %v", err)
	}

	root := readTestFile(t, geminimodules.RootPath(home))
	want := filemerge.InjectMarkdownSection("", geminimodules.ImportSectionID, "@./gentle-ai/01-agent-routing.md\n") + "\n# My notes\n"
	if root != want {
		t.Fatalf("GEMINI.md =\n%q\nwant\n%q", root, want)
	}
	module := readTestFile(t, filepath.Join(geminimodules.ModulesDir(home), "01-agent-routing.md"))
	if !strings.Contains(module, "routing") || strings.Contains(module+root, "Senior Architect") || strings.Contains(module, "engram") {
		t.Fatalf("remaining module = %q, want only the retained routing section", module)
	}
}
