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

func TestNormalizeInstallFlagsPersonaFlagForcesPersonaComponent(t *testing.T) {
	input, err := NormalizeInstallFlags(InstallFlags{
		Persona:    "neutral",
		Components: []string{string(model.ComponentEngram)},
	}, system.DetectionResult{})
	if err != nil {
		t.Fatalf("NormalizeInstallFlags() error = %v", err)
	}
	found := false
	for _, component := range input.Selection.Components {
		if component == model.ComponentPersona {
			found = true
		}
	}
	if !found {
		t.Fatalf("explicit --persona must force the persona component; got %v", input.Selection.Components)
	}
}

func TestNormalizeInstallFlagsCustomPersonaDoesNotForceComponent(t *testing.T) {
	input, err := NormalizeInstallFlags(InstallFlags{
		Persona:    "custom",
		Components: []string{string(model.ComponentEngram)},
	}, system.DetectionResult{})
	if err != nil {
		t.Fatalf("NormalizeInstallFlags() error = %v", err)
	}
	for _, component := range input.Selection.Components {
		if component == model.ComponentPersona {
			t.Fatal("--persona custom means unmanaged; it must not force the persona component")
		}
	}
}
