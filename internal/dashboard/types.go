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

// CreateIncrementRequest define los parámetros para crear un nuevo incremento SDD.
type CreateIncrementRequest struct {
	Name         string `json:"name"`
	Intent       string `json:"intent"`
	Type         string `json:"type,omitempty"`          // "feature", "fix", "refactor", "architecture"
	ProposalBody string `json:"proposal_body,omitempty"` // INC-19: cuerpo ya renderizado (promoción ODD), sustituye la plantilla generada cuando no está vacío
}

// CreateIncrementResponse reporta el resultado de la creación de un incremento.
type CreateIncrementResponse struct {
	Success bool   `json:"success"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// IncrementActionRequest define la acción a ejecutar sobre un incremento activo.
type IncrementActionRequest struct {
	Name string `json:"name"`
}

// IncrementActionResponse reporta el resultado de una acción SDD (continue, verify, etc.).
type IncrementActionResponse struct {
	Success    bool   `json:"success"`
	ChangeName string `json:"change_name"`
	Action     string `json:"action"`
	Output     string `json:"output"`
	Error      string `json:"error,omitempty"`
}

// CreateHandoffRequest define los campos para crear o actualizar un handoff estructurado.
type CreateHandoffRequest struct {
	Change           string `json:"change"`
	FromPhase        string `json:"from_phase"`
	ToPhase          string `json:"to_phase"`
	FromRole         string `json:"from_role"`
	ToRole           string `json:"to_role"`
	Status           string `json:"status"`
	ExecutiveSummary string `json:"executive_summary"`
	Artifacts        string `json:"artifacts"`
	Decisions        string `json:"decisions"`
	Risks            string `json:"risks"`
	Instructions     string `json:"instructions"`
}

// CreateHandoffResponse reporta el resultado del registro del handoff.
type CreateHandoffResponse struct {
	Success  bool   `json:"success"`
	Change   string `json:"change"`
	FilePath string `json:"file_path"`
	Message  string `json:"message"`
}

// DoctorCheck representa el resultado de un chequeo individual de salud del sistema.
type DoctorCheck struct {
	Name           string `json:"name"`
	Category       string `json:"category"` // "agent", "tool", "environment"
	Status         string `json:"status"`   // "ok", "warning", "error"
	Details        string `json:"details"`
	Recommendation string `json:"recommendation,omitempty"`
}

// DoctorReport agrupa los diagnósticos de salud del ecosistema Axiom.
type DoctorReport struct {
	Timestamp string        `json:"timestamp"`
	Healthy   bool          `json:"healthy"`
	Checks    []DoctorCheck `json:"checks"`
}

// BackupItem describe un snapshot o respaldo gestionado en ~/.axiom/backups/.
type BackupItem struct {
	Name        string   `json:"name"`
	Created     string   `json:"created"`
	Description string   `json:"description"`
	Pinned      bool     `json:"pinned"`
	Files       []string `json:"files,omitempty"`
	SizeBytes   int64    `json:"size_bytes,omitempty"`
}

// BackupActionRequest define la solicitud para crear o restaurar un respaldo.
type BackupActionRequest struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// EcosystemActionResponse reporta el resultado de operaciones como Sync o Upgrade.
type EcosystemActionResponse struct {
	Success bool     `json:"success"`
	Action  string   `json:"action"`
	Message string   `json:"message"`
	Output  []string `json:"output,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// ModelConfigItem describe la configuración o asignación de un modelo de IA.
type ModelConfigItem struct {
	Agent       string `json:"agent"`
	Role        string `json:"role"`
	Model       string `json:"model"`
	Reasoning   string `json:"reasoning,omitempty"`
	Environment string `json:"environment,omitempty"`
}

// ModelAssignmentsDTO agrupa la configuración de modelos de IA y la persona activa.
type ModelAssignmentsDTO struct {
	ActivePersona string            `json:"active_persona"`
	Assignments   []ModelConfigItem `json:"assignments"`
}

// ODDCreateRequest define el parámetro para crear un documento vivo ODD
// nuevo mediante POST /api/odd (REQ-19.5, REQ-19.13). Los DTO de respuesta
// del carril ODD reutilizan los tipos del dominio internal/odd en vez de
// duplicar su forma: sólo las peticiones de entrada tienen DTO propio [D-15].
type ODDCreateRequest struct {
	Feature string `json:"feature"`
}

// ODDPromoteRequest define los parámetros para promover un documento vivo
// ODD a una propuesta SDD sembrada, o previsualizarla sin escribir nada,
// mediante POST /api/odd/promote (REQ-19.8, REQ-19.9, REQ-19.13). [D-15]
type ODDPromoteRequest struct {
	Feature string `json:"feature"`
	Name    string `json:"name,omitempty"`
	DryRun  bool   `json:"dry_run,omitempty"`
}

// ODDMirrorRequest define el parámetro para comparar un documento vivo ODD
// contra su espejo de recuperación en Engram mediante
// POST /api/odd/check-mirror (REQ-19.4, REQ-19.7). [D-15]
type ODDMirrorRequest struct {
	Feature string `json:"feature"`
}
