package dashboard

import "github.com/gentleman-programming/gentle-ai/v2/internal/multirole"

// WorkspaceDTO representa el estado global y configuración del espacio de trabajo.
type WorkspaceDTO struct {
	Name            string              `json:"name"`
	Topology        string              `json:"topology"`
	SpecsRepository string              `json:"specs_repository"`
	Root            string              `json:"root"`
	Roles           map[string]RoleMeta `json:"roles"`
	Compliant       bool                `json:"compliant"`
	Message         string              `json:"message"`
}

// RoleMeta describe un rol dentro del espacio de trabajo.
type RoleMeta struct {
	Name         string   `json:"name"`
	GatePolicy   string   `json:"gate_policy"`
	Repositories []string `json:"repositories"`
	Tech         []string `json:"tech"`
}

// IncrementSummaryDTO resume el estado y progreso de un cambio SDD.
type IncrementSummaryDTO struct {
	Name           string `json:"name"`
	Type           string `json:"type"` // "active" o "archived"
	Phase          string `json:"phase"`
	TasksTotal     int    `json:"tasks_total"`
	TasksCompleted int    `json:"tasks_completed"`
	ProgressPct    int    `json:"progress_pct"`
	Date           string `json:"date,omitempty"`
}

// IncrementDetailDTO contiene el detalle estructurado de un cambio y sus artefactos.
type IncrementDetailDTO struct {
	Summary       IncrementSummaryDTO      `json:"summary"`
	HasProposal   bool                     `json:"has_proposal"`
	HasSpec       bool                     `json:"has_spec"`
	HasDesign     bool                     `json:"has_design"`
	HasTasks      bool                     `json:"has_tasks"`
	HasVerify     bool                     `json:"has_verify"`
	HasArchive    bool                     `json:"has_archive"`
	Proposal      string                   `json:"proposal,omitempty"`
	Spec          string                   `json:"spec,omitempty"`
	Design        string                   `json:"design,omitempty"`
	TasksContent  string                   `json:"tasks_content,omitempty"`
	VerifyReport  string                   `json:"verify_report,omitempty"`
	ArchiveReport string                   `json:"archive_report,omitempty"`
	BarrierReport *multirole.BarrierReport `json:"barrier_report,omitempty"`
}

// SkillDTO representa una skill disponible en el proyecto.
type SkillDTO struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Description string `json:"description"`
	Trigger     string `json:"trigger,omitempty"`
}
