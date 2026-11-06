package geminimodules_test

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/geminimodules"
)

// rootWith is the GEMINI.md layout Publish writes: the import block first,
// so no user text can open a code region around it, then the user's bytes.
func rootWith(imports, user string) string {
	return filemerge.InjectMarkdownSection("", geminimodules.ImportSectionID, imports) + user
}

// TestPublishAndRetireKeepUserModules proves ownership comes from the
// manifest, not the file name.
func TestPublishAndRetireKeepUserModules(t *testing.T) {
	home := t.TempDir()
	dir := geminimodules.ModulesDir(home)
	write(t, filepath.Join(dir, "99-mine.md"), "mine\n", 0o644)
	write(t, filepath.Join(dir, "01-notes.md"), "notes\n", 0o644)
	steps := map[string]string{"publish": stagingWith(personaAsset, "engram-protocol"), "publish with fewer sections": personaAsset, "retire": ""}
	for _, step := range []string{"publish", "publish with fewer sections", "retire"} {
		write(t, geminimodules.StagingPath(home), steps[step], 0o644)
		publish := geminimodules.Publish
		if step == "retire" {
			publish = geminimodules.Retire
		}
		if _, err := publish(home); err != nil {
			t.Fatalf("%s error = %v", step, err)
		}
		if read(t, filepath.Join(dir, "99-mine.md")) != "mine\n" || read(t, filepath.Join(dir, "01-notes.md")) != "notes\n" {
			t.Fatalf("%s changed a user file shaped like a module", step)
		}
	}
	if owned, _ := geminimodules.OwnedModulePaths(home); len(owned) != 0 {
		t.Fatalf("Retire() left owned modules %v", owned)
	}
}

func TestPublishRefusesToOverwriteUnownedModule(t *testing.T) {
	home := t.TempDir()
	mine := filepath.Join(geminimodules.ModulesDir(home), "01-persona.md")
	write(t, mine, "my persona notes\n", 0o644)
	write(t, geminimodules.StagingPath(home), personaAsset, 0o644)

	if _, err := geminimodules.Publish(home); err == nil || !strings.Contains(err.Error(), strconv.Quote(mine)) {
		t.Fatalf("Publish() error = %v, want a refusal naming %s", err, mine)
	}
	if got := read(t, mine); got != "my persona notes\n" {
		t.Fatalf("unowned module rewritten to %q", got)
	}
}

// TestPublishWritesImportBlockFirst keeps a stray backtick or unclosed fence
// in user text from putting the imports in a code region, where Gemini CLI
// ignores them, and restores the user bytes exactly on Retire.
func TestPublishWritesImportBlockFirst(t *testing.T) {
	for _, user := range []string{"Use `foo to start.\n\n```sh\nunclosed fence\n", "\n" + userContent} {
		home := t.TempDir()
		write(t, geminimodules.StagingPath(home), personaAsset, 0o644)
		write(t, geminimodules.RootPath(home), user, 0o644)
		if _, err := geminimodules.Publish(home); err != nil {
			t.Fatalf("Publish() error = %v", err)
		}
		if got, want := read(t, geminimodules.RootPath(home)), rootWith(importBlock("01-persona.md"), user); got != want {
			t.Fatalf("GEMINI.md =\n%q\nwant\n%q", got, want)
		}
		if _, err := geminimodules.Retire(home); err != nil || read(t, geminimodules.RootPath(home)) != user {
			t.Fatalf("GEMINI.md after Retire (%v) is not the user bytes %q", err, user)
		}
	}
}
