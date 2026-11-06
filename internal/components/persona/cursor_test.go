package persona

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/cursor"
	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

func TestCursorRuleActivation(t *testing.T) {
	for _, persona := range []model.PersonaID{model.PersonaGentleman, model.PersonaNeutral} {
		t.Run(string(persona), func(t *testing.T) {
			home := t.TempDir()
			adapter := cursor.NewAdapter()
			result, err := Inject(home, adapter, persona)
			if err != nil || !result.Changed {
				t.Fatalf("Inject = %+v, %v", result, err)
			}
			data, err := os.ReadFile(adapter.SystemPromptFile(home))
			if err != nil {
				t.Fatal(err)
			}
			asset := "generic/persona-gentleman.md"
			if persona == model.PersonaNeutral {
				asset = "generic/persona-neutral.md"
			}
			want := "---\ndescription: Gentle AI rules\nalwaysApply: true\n---\n\n" + assets.MustRead(asset)
			if string(data) != want {
				t.Fatal("Cursor rule must wrap the unchanged persona body in activation frontmatter")
			}
		})
	}
}

func TestCursorRuleSyncRepairsAndPreservesManagedSections(t *testing.T) {
	const suffix = "<!-- gentle-ai:sdd -->\nExisting SDD instructions\n<!-- /gentle-ai:sdd -->\n\n<!-- gentle-ai:engram -->\nExisting memory instructions\n<!-- /gentle-ai:engram -->\n"
	for _, persona := range []model.PersonaID{model.PersonaGentleman, model.PersonaNeutral} {
		for _, frontmatter := range []string{"", "---\ndescription: Old rules\nalwaysApply: true\n---\n\n"} {
			name := string(persona) + "/legacy"
			if frontmatter != "" {
				name = string(persona) + "/wrapped"
			}
			t.Run(name, func(t *testing.T) {
				home := t.TempDir()
				adapter := cursor.NewAdapter()
				path := adapter.SystemPromptFile(home)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(frontmatter+"Old persona\n\n"+suffix), 0o644); err != nil {
					t.Fatal(err)
				}
				result, err := InjectForSync(home, adapter, persona)
				if err != nil || !result.Changed {
					t.Fatalf("InjectForSync = %+v, %v", result, err)
				}
				first, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				text := string(first)
				if !strings.HasPrefix(text, "---\ndescription: Gentle AI rules\nalwaysApply: true\n---\n\n") || strings.Count(text, "alwaysApply: true") != 1 {
					t.Fatal("sync must produce exactly one activation declaration")
				}
				if !strings.HasSuffix(text, suffix) || strings.Count(text, suffix) != 1 || strings.Contains(text, "Old persona") {
					t.Fatal("sync must replace the persona and preserve managed sections exactly once")
				}
				result, err = InjectForSync(home, adapter, persona)
				if err != nil || result.Changed {
					t.Fatalf("second sync = %+v, %v; expected no change", result, err)
				}
				second, err := os.ReadFile(path)
				if err != nil || string(second) != text {
					t.Fatalf("second sync changed bytes: %v", err)
				}
			})
		}
	}
}
