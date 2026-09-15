package dashboard

import (
	"github.com/gentleman-programming/gentle-ai/v2/internal/hub"
	"github.com/gentleman-programming/gentle-ai/v2/internal/multirole"
)

// WorkspaceDTO representa el estado global y configuración del espacio de trabajo.
type WorkspaceDTO struct {
	Name            string              `json:"name"`
	Topology        string              `json:"topology"`
	SpecsRepository string              `json:"specs_repository"`
	Root            string              `json:"root"`
	Roles           map[string]RoleMeta `json:"roles"`
	Compliant       bool                `json:"compliant"`
	Message         string              `json:"message"`
	IsConfigured    bool                `json:"is_configured"`
	DetectedTech    interface{}         `json:"detected_tech,omitempty"`
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

// SkillProposalDTO representa una propuesta pendiente de aprobación en el buzón transitorio.
type SkillProposalDTO struct {
	Name          string            `json:"name"`
	Origin        string            `json:"origin"`
	Source        string            `json:"source"`
	Verified      bool              `json:"verified"`
	Role          string            `json:"role,omitempty"`
	DetectedBy    string            `json:"detected_by"`
	Justification string            `json:"justification"`
	CreatedAt     string            `json:"created_at"`
	SkillMD       string            `json:"skill_md"`
	SHA256        map[string]string `json:"sha256,omitempty"`
}

// SkillActionDTO modela las solicitudes de escaneo, aprobación o rechazo de skills.
type SkillActionDTO struct {
	Name    string `json:"name,omitempty"`
	Role    string `json:"role,omitempty"`
	Offline bool   `json:"offline,omitempty"`
}

// ProjectListDTO encapsula la lista de proyectos registrados y el activo.
type ProjectListDTO struct {
	ActiveWorkspace string        `json:"active_workspace"`
	Projects        []interface{} `json:"projects"`
}

// ProjectSwitchRequest representa la solicitud para conmutar el workspace activo.
type ProjectSwitchRequest struct {
	ID   string `json:"id,omitempty"`
	Path string `json:"path,omitempty"`
}

// ProjectAddRequest representa la solicitud para registrar un proyecto en el Hub.
type ProjectAddRequest struct {
	Path     string `json:"path"`
	Name     string `json:"name,omitempty"`
	Topology string `json:"topology,omitempty"`
}

// ProjectInitRequest representa la solicitud para inicializar un proyecto.
type ProjectInitRequest struct {
	Path     string          `json:"path,omitempty"`
	Name     string          `json:"name,omitempty"`
	Topology string          `json:"topology,omitempty"`
	Roles    []hub.RoleInput `json:"roles,omitempty"`
}

// MigrateCumulativeRequest representa la solicitud para transferir tareas pendientes de un rol no bloqueante.
type MigrateCumulativeRequest struct {
	Change string `json:"change"`
	Role   string `json:"role"`
}

