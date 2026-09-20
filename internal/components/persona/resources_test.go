package persona_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/components/persona"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

func TestResourcePlanOutputStylePaths(t *testing.T) {
	dir := t.TempDir()
	axiom := filepath.Join(dir, "axiom.md")
	gentleman := filepath.Join(dir, "gentleman.md")
	neutral := filepath.Join(dir, "neutral.md")

	tests := []struct {
		name    string
		persona model.PersonaID
		want    persona.OutputStylePaths
	}{
		{
			name:    "axiom writes its selected style and removes retired gentleman and neutral",
			persona: model.PersonaAxiom,
			want: persona.OutputStylePaths{
				Write:  axiom,
				Backup: []string{axiom, gentleman, neutral},
				Remove: []string{gentleman, neutral},
			},
		},
		{
			name:    "gentleman writes its selected style and removes retired axiom",
			persona: model.PersonaGentleman,
			want: persona.OutputStylePaths{
				Write:  gentleman,
				Backup: []string{axiom, gentleman, neutral},
				Remove: []string{axiom},
			},
		},
		{
			name:    "neutral writes its selected style and removes retired axiom and gentleman",
			persona: model.PersonaNeutral,
			want: persona.OutputStylePaths{
				Write:  neutral,
				Backup: []string{axiom, gentleman, neutral},
				Remove: []string{axiom, gentleman},
			},
		},
		{
			name:    "legacy neutral alias writes neutral and removes retired axiom and gentleman",
			persona: model.PersonaGentlemanNeutralArtifacts,
			want: persona.OutputStylePaths{
				Write:  neutral,
				Backup: []string{axiom, gentleman, neutral},
				Remove: []string{axiom, gentleman},
			},
		},
		{
			name:    "custom manages no output styles",
			persona: model.PersonaCustom,
			want:    persona.OutputStylePaths{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := persona.ResourcePlanFor(tt.persona).OutputStylePaths(dir)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ResourcePlanFor(%q).OutputStylePaths() = %#v, want %#v", tt.persona, got, tt.want)
			}
		})
	}
}
