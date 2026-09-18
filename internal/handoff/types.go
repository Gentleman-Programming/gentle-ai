package handoff

import (
	"time"
)

// Phase representa una fase válida en el ciclo de vida SDD de Axiom.
type Phase string

const (
	PhaseExplore Phase = "explore"
	PhasePropose Phase = "propose"
	PhaseSpec    Phase = "spec"
	PhaseDesign  Phase = "design"
	PhaseTasks   Phase = "tasks"
	PhaseApply   Phase = "apply"
	PhaseVerify  Phase = "verify"
	PhaseArchive Phase = "archive"
)

// Status representa el estado de disposición de un relevo.
type Status string

const (
	StatusReady              Status = "ready"
	StatusBlocked            Status = "blocked"
	StatusNeedsClarification Status = "needs_clarification"
)

// Constantes con los encabezados canónicos de las cinco secciones obligatorias en español.
const (
	HeaderExecutiveSummary   = "## 1. Resumen Ejecutivo"
	HeaderArtifacts          = "## 2. Artefactos Modificados y Creados"
	HeaderDecisions          = "## 3. Decisiones Técnicas y Acuerdos"
	HeaderRisksAndBlockers   = "## 4. Riesgos, Bloqueos y Preguntas Abiertas"
	HeaderDirectInstructions = "## 5. Instrucciones Directas para el Siguiente Rol"
)

// Metadata encapsula las propiedades del Frontmatter YAML en el documento handoff.md.
type Metadata struct {
	Change    string    `yaml:"change"`
	FromPhase Phase     `yaml:"from_phase"`
	ToPhase   Phase     `yaml:"to_phase"`
	FromRole  string    `yaml:"from_role"`
	ToRole    string    `yaml:"to_role"`
	Timestamp time.Time `yaml:"timestamp"`
	Status    Status    `yaml:"status"`
}

// Sections contiene el contenido textual de cada una de las 5 secciones obligatorias.
type Sections struct {
	ExecutiveSummary   string
	Artifacts          string
	Decisions          string
	RisksAndBlockers   string
	DirectInstructions string
}

// Handoff representa un relevo formal estructurado entre fases o roles en Axiom.
type Handoff struct {
	Metadata Metadata
	Sections Sections
}
