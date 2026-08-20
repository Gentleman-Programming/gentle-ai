package communitytool

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

// Regression coverage for issue #3473: sync wrote managed files into agents the
// user never selected, because the CodeGraph injectors discovered agents purely
// from the filesystem. Codex and Cursor merely existing on the machine was
// enough for their system prompt files to be rewritten on every sync.

// selectOnlyOpenCodeWithCodexAndCursorPresent builds the reported environment:
// three agents installed on disk, only OpenCode selected in Gentle AI.
func selectOnlyOpenCodeWithCodexAndCursorPresent(t *testing.T) (home, opencodeDir, codexPrompt, cursorPrompt string) {
	t.Helper()
	home = t.TempDir()

	opencodeDir = filepath.Join(home, ".config", "opencode")
	codexPrompt = filepath.Join(home, ".codex", "AGENTS.md")
	cursorPrompt = filepath.Join(home, ".cursor", "rules", "gentle-ai.mdc")

	mustWrite(t, filepath.Join(opencodeDir, "opencode.json"), "{}\n")
	mustWrite(t, codexPrompt, "user codex instructions\n")
	mustWrite(t, cursorPrompt, "user cursor rules\n")

	if err := state.Write(home, state.InstallState{
		InstalledAgents: []string{string(model.AgentOpenCode)},
	}); err != nil {
		t.Fatalf("state.Write: %v", err)
	}
	return home, opencodeDir, codexPrompt, cursorPrompt
}

func TestInjectCodeGraphGuidanceSkipsUnselectedAgents(t *testing.T) {
	home, opencodeDir, codexPrompt, cursorPrompt := selectOnlyOpenCodeWithCodexAndCursorPresent(t)

	result, err := InjectCodeGraphGuidance(home)
	if err != nil {
		t.Fatalf("InjectCodeGraphGuidance() error = %v", err)
	}

	for _, path := range result.Files {
		if !strings.HasPrefix(path, opencodeDir+string(filepath.Separator)) {
			t.Errorf("InjectCodeGraphGuidance() touched unselected agent file %q", path)
		}
	}

	for _, path := range []string{codexPrompt, cursorPrompt} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", path, err)
		}
		if strings.Contains(string(content), "## CodeGraph") {
			t.Errorf("guidance was injected into unselected agent %q:\n%s", path, content)
		}
	}
}

func TestCodeGraphManagedPathsAreScopedToSelection(t *testing.T) {
	home, _, codexPrompt, cursorPrompt := selectOnlyOpenCodeWithCodexAndCursorPresent(t)

	managed := CodeGraphManagedPaths(home)

	for _, unwanted := range []string{codexPrompt, cursorPrompt} {
		if slices.Contains(managed, unwanted) {
			t.Errorf("CodeGraphManagedPaths() = %v, must not manage unselected %q", managed, unwanted)
		}
	}
}
