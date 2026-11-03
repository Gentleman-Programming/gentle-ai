package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/gemini"
	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/geminimodules"
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
	if strings.Contains(root[:start], "```") || strings.Contains(root[:start], "~~~") {
		t.Fatalf("import block may sit inside a code fence:\n%s", root)
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
	if got := filemerge.InjectMarkdownSection(root, geminimodules.ImportSectionID, ""); got != legacyGeminiUserContent {
		t.Fatalf("GEMINI.md outside the import block = %q, want only the user content", got)
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
	if info, err := os.Stat(geminimodules.RootPath(home)); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("GEMINI.md mode changed: %v %v", info.Mode(), err)
	}
	guidance := importedGuidance(t, home)
	if personaCopies(guidance) != 1 || !strings.Contains(guidance, "engram guidance from an older release") {
		t.Fatal("the migrated persona and the unselected engram section must survive as modules")
	}
}
