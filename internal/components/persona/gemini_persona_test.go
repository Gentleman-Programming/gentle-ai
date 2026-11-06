package persona

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/gemini"
	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// personaFingerprint appears once in every generic persona asset and nowhere
// else in the managed guidance, so it counts persona copies in a prompt file.
const personaFingerprint = `Never add "Co-Authored-By"`

func geminiPromptAfterInject(t *testing.T, existing string, id model.PersonaID) string {
	t.Helper()
	home := t.TempDir()
	adapter := gemini.NewAdapter()
	path := adapter.SystemPromptFile(home)
	if existing != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Inject(home, adapter, id); err != nil {
		t.Fatalf("Inject(gemini-cli, %s) error = %v", id, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestInjectGeminiDropsMarkedPersonaLeftByAntigravity is #5608: releases that
// appended Antigravity sections to the shared GEMINI.md left a marked persona
// section there. Gemini CLI writes its persona as the leading body, so the
// preserved managed tail must not carry a second persona copy.
func TestInjectGeminiDropsMarkedPersonaLeftByAntigravity(t *testing.T) {
	engramSection := filemerge.InjectMarkdownSection("", "engram-protocol", "engram body\n")
	legacy := filemerge.InjectMarkdownSection("", "persona", assets.MustRead("generic/persona-gentleman.md"))
	legacy = filemerge.InjectMarkdownSection(legacy, "engram-protocol", "engram body\n")

	for _, id := range []model.PersonaID{model.PersonaNeutral, model.PersonaGentleman} {
		t.Run(string(id), func(t *testing.T) {
			got := geminiPromptAfterInject(t, legacy, id)
			if count := strings.Count(got, personaFingerprint); count != 1 {
				t.Fatalf("GEMINI.md has %d persona copies, want 1:\n%s", count, got)
			}
			if strings.Contains(got, "<!-- gentle-ai:persona -->") {
				t.Fatalf("GEMINI.md kept the marked persona section:\n%s", got)
			}
			if id == model.PersonaNeutral && !strings.Contains(got, engramSection) {
				t.Fatalf("GEMINI.md lost the preserved engram section:\n%s", got)
			}
		})
	}
}

// TestInjectGeminiNeutralKeepsManagedTailByteIdentical pins the Gemini CLI
// output when no marked persona exists: the managed tail is preserved as-is.
func TestInjectGeminiNeutralKeepsManagedTailByteIdentical(t *testing.T) {
	persona := geminiPromptAfterInject(t, "", model.PersonaNeutral)
	tail := filemerge.InjectMarkdownSection("", "engram-protocol", "engram body\n")
	tail = filemerge.InjectMarkdownSection(tail, "agent-routing", "routing body\n")

	got := geminiPromptAfterInject(t, "old neutral persona\n\n"+tail, model.PersonaNeutral)
	want := persona
	if !strings.HasSuffix(want, "\n") {
		want += "\n"
	}
	want += "\n" + tail
	if got != want {
		t.Fatalf("GEMINI.md =\n%q\nwant\n%q", got, want)
	}
}
