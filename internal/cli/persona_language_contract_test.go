package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

func TestNormalizePersonaRemapsGentlemanNeutralArtifacts(t *testing.T) {
	got, remapped, err := normalizePersona("gentleman-neutral-artifacts")
	if err != nil {
		t.Fatalf("normalizePersona() error = %v", err)
	}
	if got != model.PersonaNeutral {
		t.Fatalf("normalizePersona() = %q, want %q", got, model.PersonaNeutral)
	}
	if !remapped {
		t.Fatal("normalizePersona() remapped = false, want true for the legacy alias")
	}
}

func TestNormalizePersonaDoesNotFlagCanonicalPersonas(t *testing.T) {
	for _, value := range []string{"", "gentleman", "neutral", "custom"} {
		_, remapped, err := normalizePersona(value)
		if err != nil {
			t.Fatalf("normalizePersona(%q) error = %v", value, err)
		}
		if remapped {
			t.Fatalf("normalizePersona(%q) remapped = true, want false", value)
		}
	}
}

func TestNormalizeInstallFlagsPrintsAliasRemapNotice(t *testing.T) {
	var buf bytes.Buffer
	previous := personaNoticeWriter
	personaNoticeWriter = &buf
	defer func() { personaNoticeWriter = previous }()

	input, err := NormalizeInstallFlags(InstallFlags{Persona: "gentleman-neutral-artifacts"}, system.DetectionResult{})
	if err != nil {
		t.Fatalf("NormalizeInstallFlags() error = %v", err)
	}
	if input.Selection.Persona != model.PersonaNeutral {
		t.Fatalf("Selection.Persona = %q, want %q", input.Selection.Persona, model.PersonaNeutral)
	}
	if !strings.Contains(buf.String(), personaAliasRemapNotice) {
		t.Fatalf("notice not printed; got %q", buf.String())
	}
}
