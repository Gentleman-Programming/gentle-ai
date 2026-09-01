package persona_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/persona"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func TestResourcePlanOutputStylePaths(t *testing.T) {
	dir := t.TempDir()
	gentleman := filepath.Join(dir, "gentleman.md")
	neutral := filepath.Join(dir, "neutral.md")

	tests := []struct {
		name    string
		persona model.PersonaID
		want    persona.OutputStylePaths
	}{
		{
			name:    "gentleman writes its selected style without removing neutral",
			persona: model.PersonaGentleman,
			want: persona.OutputStylePaths{
				Write:  gentleman,
				Backup: []string{gentleman, neutral},
			},
		},
		{
			name:    "neutral writes its selected style and removes retired gentleman",
			persona: model.PersonaNeutral,
			want: persona.OutputStylePaths{
				Write:  neutral,
				Backup: []string{gentleman, neutral},
				Remove: []string{gentleman},
			},
		},
		{
			name:    "legacy neutral alias writes neutral and removes retired gentleman",
			persona: model.PersonaGentlemanNeutralArtifacts,
			want: persona.OutputStylePaths{
				Write:  neutral,
				Backup: []string{gentleman, neutral},
				Remove: []string{gentleman},
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
