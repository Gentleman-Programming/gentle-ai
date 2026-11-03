package geminimodules_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/geminimodules"
)

const userContent = "# My own Gemini rules\n\nAlways cite sources.\n"

var personaAsset = assets.MustRead("generic/persona-gentleman.md")

// stagingWith builds staging the way the Gemini CLI writers do: the persona
// body first, then the marker sections later writers append.
func stagingWith(persona string, sections ...string) string {
	content := persona
	for _, id := range sections {
		content = filemerge.InjectMarkdownSection(content, id, id+" guidance\n")
	}
	return content
}

func write(t *testing.T, path, content string, perm os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return string(data)
}

func importBlock(names ...string) string {
	lines := ""
	for _, name := range names {
		lines += "@./gentle-ai/" + name + "\n"
	}
	return lines
}

func TestRenderPartitionsStagingIntoOrderedSectionModules(t *testing.T) {
	staging := stagingWith(personaAsset, "engram-protocol", "agent-routing")

	modules := geminimodules.Render(staging)

	var names []string
	joined := ""
	for _, module := range modules {
		names = append(names, module.Name)
		joined += module.Content
	}
	if got, want := strings.Join(names, ","), "01-persona.md,02-engram-protocol.md,03-agent-routing.md"; got != want {
		t.Fatalf("module names = %s, want %s", got, want)
	}
	if joined != staging {
		t.Fatalf("concatenated modules differ from the staging content:\n%q\nwant\n%q", joined, staging)
	}
	if strings.Contains(modules[0].Content, "<!-- gentle-ai:") || !strings.HasPrefix(modules[0].Content, personaAsset[:40]) {
		t.Fatalf("persona module = %q, want only the persona body", modules[0].Content)
	}
	if !strings.HasPrefix(modules[1].Content, "<!-- gentle-ai:engram-protocol -->\n") || strings.Contains(modules[1].Content, "agent-routing") {
		t.Fatalf("engram module = %q, want exactly its marker section", modules[1].Content)
	}
}

func TestRenderWithoutPersonaStartsAtTheFirstSection(t *testing.T) {
	staging := "\n" + stagingWith("", "engram-protocol")
	modules := geminimodules.Render(staging)
	if len(modules) != 1 || modules[0].Name != "01-engram-protocol.md" || modules[0].Content != staging {
		t.Fatalf("Render() = %+v, want one engram module holding the whole staging", modules)
	}
	if got := geminimodules.Render(" \n"); len(got) != 0 {
		t.Fatalf("Render(blank) = %+v, want no modules", got)
	}
}
