package handoff

import (
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v3/internal/workspace"
)

const sampleValidHandoff = `---
change: inc-02-test
from_phase: spec
to_phase: design
from_role: tech-lead
to_role: architect
timestamp: 2026-09-14T10:00:00Z
status: ready
---

## 1. Resumen Ejecutivo

Especificación de requerimientos completada y validada con 5 requerimientos y 10 escenarios.

## 2. Artefactos Modificados y Creados

- openspec/changes/inc-02-test/proposal.md
- openspec/changes/inc-02-test/spec.md

## 3. Decisiones Técnicas y Acuerdos

Se acordó el esquema YAML en frontmatter y cinco secciones en español.

## 4. Riesgos, Bloqueos y Preguntas Abiertas

Ninguno. El equipo está alineado.

## 5. Instrucciones Directas para el Siguiente Rol

Proceder con la redacción del design.md detallando los paquetes de Go necesarios.
`

func TestParseValidHandoff(t *testing.T) {
	h, err := ParseBytes([]byte(sampleValidHandoff))
	if err != nil {
		t.Fatalf("ParseBytes devolvió error inesperado: %v", err)
	}

	if h.Metadata.Change != "inc-02-test" {
		t.Errorf("Change esperado 'inc-02-test', obtenido %q", h.Metadata.Change)
	}
	if h.Metadata.FromPhase != PhaseSpec {
		t.Errorf("FromPhase esperado 'spec', obtenido %q", h.Metadata.FromPhase)
	}
	if h.Metadata.ToPhase != PhaseDesign {
		t.Errorf("ToPhase esperado 'design', obtenido %q", h.Metadata.ToPhase)
	}
	if h.Metadata.FromRole != "tech-lead" {
		t.Errorf("FromRole esperado 'tech-lead', obtenido %q", h.Metadata.FromRole)
	}
	if h.Metadata.ToRole != "architect" {
		t.Errorf("ToRole esperado 'architect', obtenido %q", h.Metadata.ToRole)
	}
	if h.Metadata.Status != StatusReady {
		t.Errorf("Status esperado 'ready', obtenido %q", h.Metadata.Status)
	}

	if !strings.Contains(h.Sections.ExecutiveSummary, "Especificación de requerimientos completada") {
		t.Errorf("ExecutiveSummary no contiene el texto esperado: %q", h.Sections.ExecutiveSummary)
	}
	if !strings.Contains(h.Sections.Artifacts, "spec.md") {
		t.Errorf("Artifacts no contiene 'spec.md': %q", h.Sections.Artifacts)
	}
	if !strings.Contains(h.Sections.Decisions, "YAML en frontmatter") {
		t.Errorf("Decisions no contiene 'YAML en frontmatter': %q", h.Sections.Decisions)
	}
	if !strings.Contains(h.Sections.RisksAndBlockers, "Ninguno") {
		t.Errorf("RisksAndBlockers esperado 'Ninguno', obtenido: %q", h.Sections.RisksAndBlockers)
	}
	if !strings.Contains(h.Sections.DirectInstructions, "Proceder con la redacción") {
		t.Errorf("DirectInstructions no contiene el texto esperado: %q", h.Sections.DirectInstructions)
	}
}

func TestRoundTripFormatParse(t *testing.T) {
	original, err := ParseBytes([]byte(sampleValidHandoff))
	if err != nil {
		t.Fatalf("Error en parse inicial: %v", err)
	}

	formatted, err := Format(original)
	if err != nil {
		t.Fatalf("Format devolvió error: %v", err)
	}

	reparsed, err := ParseBytes([]byte(formatted))
	if err != nil {
		t.Fatalf("ParseBytes tras format devolvió error: %v", err)
	}

	if original.Metadata.Change != reparsed.Metadata.Change ||
		original.Metadata.FromPhase != reparsed.Metadata.FromPhase ||
		original.Metadata.ToPhase != reparsed.Metadata.ToPhase ||
		original.Metadata.FromRole != reparsed.Metadata.FromRole ||
		original.Metadata.ToRole != reparsed.Metadata.ToRole ||
		original.Metadata.Status != reparsed.Metadata.Status {
		t.Errorf("Metadatos no coinciden tras round-trip: %+v vs %+v", original.Metadata, reparsed.Metadata)
	}

	if original.Sections.ExecutiveSummary != reparsed.Sections.ExecutiveSummary ||
		original.Sections.DirectInstructions != reparsed.Sections.DirectInstructions {
		t.Errorf("Secciones no coinciden tras round-trip")
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		errContains string
	}{
		{
			name:        "sin delimitador frontal",
			input:       "change: foo\nfrom_phase: bar",
			errContains: "no contiene un encabezado frontmatter YAML",
		},
		{
			name:        "sin delimitador de cierre",
			input:       "---\nchange: foo\nfrom_phase: bar\n",
			errContains: "no tiene delimitador de cierre",
		},
		{
			name: "sección faltante",
			input: `---
change: test
from_phase: spec
to_phase: design
from_role: dev
to_role: qa
timestamp: 2026-09-14T10:00:00Z
status: ready
---

## 1. Resumen Ejecutivo
ok
## 2. Artefactos Modificados y Creados
ok
## 3. Decisiones Técnicas y Acuerdos
ok
## 5. Instrucciones Directas para el Siguiente Rol
ok
`,
			errContains: "sección obligatoria faltante",
		},
		{
			name: "secciones en orden inverso",
			input: `---
change: test
from_phase: spec
to_phase: design
from_role: dev
to_role: qa
timestamp: 2026-09-14T10:00:00Z
status: ready
---

## 2. Artefactos Modificados y Creados
ok
## 1. Resumen Ejecutivo
ok
## 3. Decisiones Técnicas y Acuerdos
ok
## 4. Riesgos, Bloqueos y Preguntas Abiertas
ok
## 5. Instrucciones Directas para el Siguiente Rol
ok
`,
			errContains: "no respetan el orden canónico",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseBytes([]byte(tc.input))
			if err == nil {
				t.Fatalf("Se esperaba error conteniendo %q pero no hubo error", tc.errContains)
			}
			if !strings.Contains(err.Error(), tc.errContains) {
				t.Errorf("Mensaje de error %q no contiene %q", err.Error(), tc.errContains)
			}
		})
	}
}

func TestValidateTransitions(t *testing.T) {
	wsCfg := &workspace.WorkspaceConfig{
		Roles: map[string]workspace.RoleConfig{
			"tech-lead": {Name: "Technical Lead"},
			"architect": {Name: "Software Architect"},
			"developer": {Name: "Senior Developer"},
			"qa":        {Name: "Quality Assurance"},
		},
	}

	validHandoff := func(from, to Phase, status Status, fromRole, toRole string) *Handoff {
		return &Handoff{
			Metadata: Metadata{
				Change:    "inc-02-test",
				FromPhase: from,
				ToPhase:   to,
				FromRole:  fromRole,
				ToRole:    toRole,
				Timestamp: time.Now(),
				Status:    status,
			},
			Sections: Sections{
				ExecutiveSummary:   "Resumen listo",
				Artifacts:          "Todos",
				Decisions:          "Ninguna",
				RisksAndBlockers:   "Cero",
				DirectInstructions: "Avanzar",
			},
		}
	}

	// 1. Avance hacia adelante válido
	hValid := validHandoff(PhaseDesign, PhaseTasks, StatusReady, "architect", "developer")
	if err := Validate(hValid, wsCfg); err != nil {
		t.Errorf("Transición design -> tasks debió ser válida, falló: %v", err)
	}

	// 2. Salto de fase ilegal
	hJump := validHandoff(PhasePropose, PhaseApply, StatusReady, "tech-lead", "developer")
	if err := Validate(hJump, wsCfg); err == nil {
		t.Errorf("Salto propose -> apply debió ser rechazado")
	} else if !strings.Contains(err.Error(), "transición de fase ilegal") {
		t.Errorf("Mensaje inesperado: %v", err)
	}

	// 3. Retroceso por remediación válido con estado blocked
	hRemediation := validHandoff(PhaseVerify, PhaseApply, StatusBlocked, "qa", "developer")
	if err := Validate(hRemediation, wsCfg); err != nil {
		t.Errorf("Remediación verify -> apply con status 'blocked' debió ser válida, falló: %v", err)
	}

	// 4. Retroceso rechazado con estado ready
	hRemediationReady := validHandoff(PhaseVerify, PhaseApply, StatusReady, "qa", "developer")
	if err := Validate(hRemediationReady, wsCfg); err == nil {
		t.Errorf("Retroceso verify -> apply con status 'ready' debió ser rechazado")
	}

	// 5. Rol no existente en workspace
	hUnknownRole := validHandoff(PhaseDesign, PhaseTasks, StatusReady, "unknown-role", "developer")
	if err := Validate(hUnknownRole, wsCfg); err == nil {
		t.Errorf("Rol no existente debió ser rechazado")
	} else if !strings.Contains(err.Error(), "no está declarado") {
		t.Errorf("Mensaje de error inesperado: %v", err)
	}

	// 6. Sección vacía rechazada
	hEmptySec := validHandoff(PhaseDesign, PhaseTasks, StatusReady, "architect", "developer")
	hEmptySec.Sections.Decisions = "   "
	if err := Validate(hEmptySec, wsCfg); err == nil {
		t.Errorf("Sección en blanco debió ser rechazada")
	}
}

func TestToEngramPayload(t *testing.T) {
	h := &Handoff{
		Metadata: Metadata{
			Change:    "auth-jwt",
			FromPhase: PhaseSpec,
			ToPhase:   PhaseDesign,
			FromRole:  "lead",
			ToRole:    "arch",
			Timestamp: time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC),
			Status:    StatusReady,
		},
		Sections: Sections{
			ExecutiveSummary:   "Spec lista",
			Artifacts:          "spec.md",
			Decisions:          "JWT con expiración de 15m",
			RisksAndBlockers:   "Ninguno",
			DirectInstructions: "Diseñar claims",
		},
	}

	payload := ToEngramPayload(h)
	if payload.TopicKey != "sdd/auth-jwt/handoff" {
		t.Errorf("TopicKey inesperado: %q", payload.TopicKey)
	}
	if payload.Type != "architecture" {
		t.Errorf("Type inesperado: %q", payload.Type)
	}
	if !strings.Contains(payload.Content, "JWT con expiración de 15m") {
		t.Errorf("El contenido no incluye las decisiones")
	}
}
