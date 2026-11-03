package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/gemini"
	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/geminimodules"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/pipeline"
)

var geminiImportLine = regexp.MustCompile(`^@\./[A-Za-z0-9_.\-/]+\.md$`)

// geminiImportTargets checks the import block of GEMINI.md against the Gemini
// CLI import grammar: one `@./relative.md` token per line, outside code
// fences, resolving under the directory of GEMINI.md to an existing file. It
// returns the targets in import order.
func geminiImportTargets(t *testing.T, base string) []string {
	t.Helper()
	root := readString(t, geminimodules.RootPath(base))
	open := "<!-- gentle-ai:" + geminimodules.ImportSectionID + " -->\n"
	start := strings.Index(root, open)
	end := strings.Index(root, "<!-- /gentle-ai:"+geminimodules.ImportSectionID+" -->")
	if start < 0 || end < start {
		t.Fatalf("GEMINI.md has no import block:\n%s", root)
	}
	// Gemini CLI finds code regions in the whole file, so only an import
	// block ahead of every user byte is safe from a stray backtick.
	if start != 0 {
		t.Fatalf("import block is not the first thing in GEMINI.md, where user text could put it in a code region:\n%s", root)
	}
	dir := filepath.Dir(geminimodules.RootPath(base))
	var targets []string
	for _, line := range strings.Split(strings.TrimSuffix(root[start+len(open):end], "\n"), "\n") {
		if !geminiImportLine.MatchString(line) || strings.Contains(line, "..") {
			t.Fatalf("import line %q does not match the Gemini CLI import grammar", line)
		}
		target := filepath.Join(dir, strings.TrimPrefix(line, "@"))
		if !strings.HasPrefix(target, dir+string(filepath.Separator)) {
			t.Fatalf("import %q resolves outside %s", line, dir)
		}
		if info, err := os.Stat(target); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("import target %s is not a file: %v", target, err)
		}
		targets = append(targets, target)
	}
	return targets
}

// importedGuidance concatenates the import targets of GEMINI.md in order.
func importedGuidance(t *testing.T, base string) string {
	t.Helper()
	joined := ""
	for _, target := range geminiImportTargets(t, base) {
		joined += readString(t, target)
	}
	return joined
}

func installGemini(t *testing.T, args ...string) {
	t.Helper()
	if _, err := RunInstall(args, macOSDetectionResult()); err != nil {
		t.Fatalf("RunInstall(%v) error = %v", args, err)
	}
}

func TestGeminiRootImportLinesMatchGeminiCLIImportGrammar(t *testing.T) {
	home := stubAntigravityHome(t, true)
	stray := "Use `foo to start.\n\n```sh\nunclosed fence\n\n" + readString(t, geminimodules.RootPath(home))
	if err := os.WriteFile(geminimodules.RootPath(home), []byte(stray), 0o644); err != nil {
		t.Fatal(err)
	}
	installGemini(t, "--agent", "gemini-cli", "--component", "persona", "--component", "engram", "--component", "context7")

	targets := geminiImportTargets(t, home)
	if len(targets) < 3 || !strings.HasSuffix(targets[0], "01-persona.md") {
		t.Fatalf("import targets = %v, want the persona module first and every section after it", targets)
	}
	if got := importedGuidance(t, home); got != readString(t, gemini.NewAdapter().SystemPromptFile(home)) {
		t.Fatal("the imported modules do not reproduce the guidance the writers staged")
	}
}

func TestRunInstallGeminiAndAntigravityKeepsTheGeminiRootSmall(t *testing.T) {
	home := stubAntigravityHome(t, true)
	installGemini(t, "--agent", "gemini-cli", "--agent", "antigravity", "--component", "persona", "--component", "engram", "--component", "context7")

	root := readString(t, geminimodules.RootPath(home))
	if len(root) >= 2048 {
		t.Fatalf("GEMINI.md is %d bytes, want < 2 KB of user content and import lines:\n%s", len(root), root)
	}
	if _, user, _ := strings.Cut(root, "<!-- /gentle-ai:"+geminimodules.ImportSectionID+" -->\n"); user != legacyGeminiUserContent {
		t.Fatalf("GEMINI.md after the import block = %q, want only the user content", user)
	}
	if n := personaCopies(root + importedGuidance(t, home)); n != 1 {
		t.Fatalf("Gemini CLI guidance carries %d persona copies, want 1", n)
	}
	assertPublishedAntigravityRules(t, home, "persona", "engram-protocol")
}

func TestRunInstallGeminiMigratesMonolithicGEMINIMD(t *testing.T) {
	home := stubAntigravityHome(t, true)
	monolithic := assets.MustRead("generic/persona-gentleman.md") + "\n" +
		filemerge.InjectMarkdownSection("", "engram-protocol", "engram guidance from an older release\n") + "\n# My notes\n"
	if err := os.WriteFile(geminimodules.RootPath(home), []byte(monolithic), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(geminimodules.RootPath(home), 0o600); err != nil {
		t.Fatal(err)
	}

	// The neutral persona writer keeps the sections later writers own.
	installGemini(t, "--agent", "gemini-cli", "--persona", "neutral", "--component", "persona")

	root := readString(t, geminimodules.RootPath(home))
	if !strings.Contains(root, "# My notes\n") || personaCopies(root) != 0 || strings.Contains(root, "engram guidance") {
		t.Fatalf("GEMINI.md = %q, want the user notes and the import block only", root)
	}
	// Windows has no POSIX permission bits to preserve.
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(geminimodules.RootPath(home)); err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("GEMINI.md mode changed: %v %v", info.Mode(), err)
		}
	}
	guidance := importedGuidance(t, home)
	if personaCopies(guidance) != 1 || !strings.Contains(guidance, "engram guidance from an older release") {
		t.Fatal("the migrated persona and the unselected engram section must survive as modules")
	}
}

func TestSyncGeminiModulesIsIdempotent(t *testing.T) {
	home := stubAntigravityHome(t, true)
	selection := model.Selection{Agents: []model.AgentID{model.AgentGeminiCLI, model.AgentAntigravity}, Components: []model.ComponentID{model.ComponentPersona}, Persona: model.PersonaGentleman}
	if _, err := RunSyncWithSelection(home, selection); err != nil {
		t.Fatalf("first sync error = %v", err)
	}
	result, err := RunSyncWithSelection(home, selection)
	if err != nil {
		t.Fatalf("second sync error = %v", err)
	}
	for _, path := range result.ChangedFiles {
		if path == geminimodules.RootPath(home) || strings.HasPrefix(path, geminimodules.ModulesDir(home)) {
			t.Errorf("second sync reported %s as changed", path)
		}
	}
}

func TestRunInstallGeminiRollbackLeavesNoNewModules(t *testing.T) {
	home := stubAntigravityHome(t, true)
	before := readString(t, geminimodules.RootPath(home))
	previous := installStagePlan
	t.Cleanup(func() { installStagePlan = previous })
	published := false
	installStagePlan = func(rt *installRuntime) pipeline.StagePlan {
		plan := previous(rt)
		plan.Apply = append(plan.Apply, antigravityTestStep{id: "test:break-staging", run: func() error {
			owned, _ := geminimodules.OwnedModulePaths(home)
			published = len(owned) > 2
			return os.Remove(geminimodules.StagingPath(home))
		}})
		return plan
	}

	_, err := RunInstall([]string{"--agent", "gemini-cli", "--component", "persona", "--component", "engram"}, macOSDetectionResult())
	if err == nil || !strings.Contains(err.Error(), "post-apply verification failed") {
		t.Fatalf("RunInstall() error = %v, want a post-apply verification failure", err)
	}
	if !published {
		t.Fatal("the publisher did not write modules before verification failed")
	}
	if owned, _ := geminimodules.OwnedModulePaths(home); len(owned) != 0 {
		t.Fatalf("rollback left modules behind: %v", owned)
	}
	if got := readString(t, geminimodules.RootPath(home)); got != before {
		t.Fatalf("GEMINI.md after rollback = %q, want %q", got, before)
	}
}
