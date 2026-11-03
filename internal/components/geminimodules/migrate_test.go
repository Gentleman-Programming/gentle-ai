package geminimodules_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/geminimodules"
)

func TestMigrateLegacyRootMovesGentleAIContentIntoStaging(t *testing.T) {
	monolithic := stagingWith(personaAsset, "engram-protocol")
	tests := []struct {
		name, root, wantRoot, wantStaging string
		changed                           bool
	}{
		{name: "persona preamble and sections", root: monolithic, wantStaging: monolithic, changed: true},
		{name: "user preamble kept", root: stagingWith(userContent, "engram-protocol"), wantRoot: userContent, wantStaging: "<!-- gentle-ai:engram-protocol -->\nengram-protocol guidance\n<!-- /gentle-ai:engram-protocol -->\n", changed: true},
		{name: "import root untouched", root: filemerge.InjectMarkdownSection(userContent, geminimodules.ImportSectionID, importBlock("01-persona.md")), changed: false},
		{name: "only user content", root: userContent, changed: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			write(t, geminimodules.RootPath(home), tt.root, 0o600)

			result, err := geminimodules.MigrateLegacyRoot(home)
			if err != nil {
				t.Fatalf("MigrateLegacyRoot() error = %v", err)
			}
			if result.Changed != tt.changed {
				t.Fatalf("changed = %v, want %v", result.Changed, tt.changed)
			}
			if !tt.changed {
				if got := read(t, geminimodules.RootPath(home)); got != tt.root {
					t.Fatalf("GEMINI.md rewritten to %q", got)
				}
				return
			}
			if got := read(t, geminimodules.StagingPath(home)); got != tt.wantStaging {
				t.Fatalf("staging =\n%q\nwant\n%q", got, tt.wantStaging)
			}
			got, err := os.ReadFile(geminimodules.RootPath(home))
			if tt.wantRoot == "" {
				if !os.IsNotExist(err) {
					t.Fatalf("emptied GEMINI.md kept: %q (%v)", got, err)
				}
				return
			}
			if string(got) != tt.wantRoot {
				t.Fatalf("GEMINI.md = %q, want the user bytes %q", got, tt.wantRoot)
			}
		})
	}
}

func TestMigrateThenPublishKeepsUserBytesAndOnePersona(t *testing.T) {
	home := t.TempDir()
	legacy := filemerge.InjectMarkdownSection(userContent, "persona", personaAsset)
	legacy = filemerge.InjectMarkdownSection(legacy, "engram-protocol", "engram\n")
	write(t, geminimodules.RootPath(home), legacy, 0o644)

	if _, err := geminimodules.MigrateLegacyRoot(home); err != nil {
		t.Fatal(err)
	}
	if _, err := geminimodules.Publish(home); err != nil {
		t.Fatal(err)
	}

	root := read(t, geminimodules.RootPath(home))
	if !strings.HasSuffix(root, "\n"+userContent) || strings.Count(root, "<!-- gentle-ai:") != 1 {
		t.Fatalf("GEMINI.md = %q, want the user bytes plus one import block", root)
	}
	guidance := ""
	for _, module := range geminimodules.Render(read(t, geminimodules.StagingPath(home))) {
		guidance += read(t, filepath.Join(geminimodules.ModulesDir(home), module.Name))
	}
	if n := strings.Count(root+guidance, `Never add "Co-Authored-By"`); n != 1 {
		t.Fatalf("persona copies = %d, want 1", n)
	}
}

// TestMigrateLegacyRootAfterDowngradeKeepsOnePersona covers an older release
// syncing over the current layout: it put a persona body and sections back
// beside the import block. They move to the staging file again, replacing the
// stale staged copies, and the import block and user bytes stay.
func TestMigrateLegacyRootAfterDowngradeKeepsOnePersona(t *testing.T) {
	home := t.TempDir()
	write(t, geminimodules.StagingPath(home), stagingWith(personaAsset, "codegraph-guidance"), 0o644)
	if _, err := geminimodules.Publish(home); err != nil {
		t.Fatal(err)
	}
	block := read(t, geminimodules.RootPath(home))
	downgraded := personaAsset + "\n" + block + filemerge.InjectMarkdownSection(userContent, "engram-protocol", "newer engram\n")
	write(t, geminimodules.RootPath(home), downgraded, 0o644)

	if _, err := geminimodules.MigrateLegacyRoot(home); err != nil {
		t.Fatalf("MigrateLegacyRoot() error = %v", err)
	}
	if _, err := geminimodules.Publish(home); err != nil {
		t.Fatal(err)
	}

	root := read(t, geminimodules.RootPath(home))
	if want := rootWith(importBlock("01-persona.md", "02-codegraph-guidance.md", "03-engram-protocol.md"), userContent); root != want {
		t.Fatalf("GEMINI.md =\n%q\nwant\n%q", root, want)
	}
	staging := read(t, geminimodules.StagingPath(home))
	if n := strings.Count(staging, `Never add "Co-Authored-By"`); n != 1 || !strings.Contains(staging, "newer engram") || !strings.Contains(staging, "codegraph-guidance guidance") {
		t.Fatalf("staging = %q, want one persona, the moved section and the kept one", staging)
	}
}
