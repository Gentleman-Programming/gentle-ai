// Package odd implementa el dominio del carril ágil de Organic Driven
// Development (ODD): el documento vivo odd/tasks/<feature>.md, su modelo de
// datos, la validación de nombre y la plantilla canónica. Fases posteriores
// de este mismo paquete añaden el análisis, el almacén de ficheros, el
// espejo de recuperación en Engram y la promoción a una propuesta SDD.
//
// Este paquete es una hoja del árbol de dependencias de Axiom: no importa
// ningún otro paquete de internal/ salvo internal/multirole. Cualquier
// necesidad de invocar el andamiador de incrementos SDD se expresa como un
// puerto que implementan los adaptadores de internal/cli, internal/dashboard
// e internal/tui, nunca como una importación directa.
package odd

import "github.com/gentleman-programming/gentle-ai/v2/internal/multirole"

// Status refleja el carril activo del documento vivo. Los valores se
// escriben literalmente en el fichero y se muestran al usuario en
// castellano, porque el documento es un artefacto redactado y leído por
// humanos. [D-14]
type Status string

const (
	// StatusActive indica que el documento está en curso en el carril ODD.
	StatusActive Status = "activo"
	// StatusPromoted indica que el documento ya fue promovido a un cambio SDD.
	StatusPromoted Status = "promovido"
)

// SectionID identifica cada una de las doce secciones canónicas del
// documento vivo (REQ-19.1).
type SectionID string

const (
	SectionObjective       SectionID = "objetivo"
	SectionProblem         SectionID = "problema"
	SectionWhy             SectionID = "porque"
	SectionScope           SectionID = "alcance"
	SectionConstraints     SectionID = "restricciones"
	SectionAuthorizedScope SectionID = "alcance-autorizado"
	SectionChecklist       SectionID = "checklist"
	SectionAcceptance      SectionID = "criterios-de-aceptacion"
	SectionChecks          SectionID = "comprobaciones"
	SectionProgress        SectionID = "progreso"
	SectionEvidence        SectionID = "evidencia"
	SectionNextStep        SectionID = "siguiente-paso"
)

// CanonicalSections enumera las doce secciones en su orden canónico
// (REQ-19.1). La plantilla, el analizador y el renderizador de promoción
// leen de aquí: es la única fuente de verdad del orden.
var CanonicalSections = []SectionID{
	SectionObjective,
	SectionProblem,
	SectionWhy,
	SectionScope,
	SectionConstraints,
	SectionAuthorizedScope,
	SectionChecklist,
	SectionAcceptance,
	SectionChecks,
	SectionProgress,
	SectionEvidence,
	SectionNextStep,
}

// Task es una entrada del checklist accionable con identificador estable
// (REQ-19.3). El identificador no se reasigna ante una reordenación del
// checklist.
type Task struct {
	ID   string `json:"id"` // "T1", "T2", …; vacío si la línea no lo declara
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// Document modela odd/tasks/<feature>.md ya analizado.
type Document struct {
	Feature    string                     `json:"feature"`
	Path       string                     `json:"path"` // relativa a la raíz del workspace
	Status     Status                     `json:"status"`
	PromotedTo string                     `json:"promoted_to,omitempty"`
	PromotedAt string                     `json:"promoted_at,omitempty"`
	Sections   map[SectionID]string       `json:"sections"`
	Tasks      []Task                     `json:"tasks"`
	Progress   multirole.RoleTaskProgress `json:"progress"`
	Warnings   []string                   `json:"warnings,omitempty"` // secciones ausentes, tareas sin ID
	Raw        string                     `json:"-"`                  // contenido íntegro: base de comparación del espejo
}

// FeatureSummary es una proyección resumida de Document pensada para
// listados: axiom odd status, GET /api/odd y la pantalla ODD de la TUI. No
// transporta el contenido íntegro de las secciones ni el texto crudo del
// documento: sólo lo necesario para mostrar progreso y estado de promoción.
type FeatureSummary struct {
	Feature    string                     `json:"feature"`
	Path       string                     `json:"path"`
	Status     Status                     `json:"status"`
	PromotedTo string                     `json:"promoted_to,omitempty"`
	PromotedAt string                     `json:"promoted_at,omitempty"`
	Progress   multirole.RoleTaskProgress `json:"progress"`
	Warnings   []string                   `json:"warnings,omitempty"`
}

// StatusReport agrega el resultado de `axiom odd status`: el listado
// completo de resúmenes de feature leídos de odd/tasks/. Es una forma
// aditiva por diseño: la incorporación del espejo de recuperación en Engram
// (fase posterior de este mismo paquete) le añade información de
// sincronización sin romper esta base.
type StatusReport struct {
	Features []FeatureSummary `json:"features"`
}
