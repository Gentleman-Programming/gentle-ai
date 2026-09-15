package hub

import "time"

// WorkspaceRecord modela un proyecto registrado en el Hub global de Axiom.
type WorkspaceRecord struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	Topology     string    `json:"topology"`
	RegisteredAt time.Time `json:"registered_at"`
	LastAccessed time.Time `json:"last_accessed"`
	IsConfigured bool      `json:"is_configured"`
	Tech         []string  `json:"tech,omitempty"`
}

// HubConfig representa el archivo de configuración central ~/.axiom/workspaces.json.
type HubConfig struct {
	Version         string            `json:"version"`
	ActiveWorkspace string            `json:"active_workspace"` // ID o ruta
	Workspaces      []WorkspaceRecord `json:"workspaces"`
}

// RoleInput define un rol provisto desde la UI o CLI para la configuración del proyecto.
type RoleInput struct {
	Key          string   `json:"key"`
	Name         string   `json:"name"`
	Repositories []string `json:"repositories"`
	NonBlocking  bool     `json:"non_blocking"`
	Tech         []string `json:"tech,omitempty"`
}

// DetectedProject representa un subproyecto o componente encontrado en la estructura existente.
type DetectedProject struct {
	Path        string `json:"path"`
	Stack       string `json:"stack"`
	TestCommand string `json:"test_command,omitempty"`
	Role        string `json:"role,omitempty"`
}

// AdoptedSkillInfo describe una skill o agente preexistente descubierto en el proyecto adoptado.
type AdoptedSkillInfo struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Role        string `json:"role,omitempty"`
	Category    string `json:"category"` // "process", "tech", "rule"
	Description string `json:"description"`
}

// TechDetection encapsula los hallazgos del detector de tecnologías sobre un repositorio.
type TechDetection struct {
	PrimaryLanguage  string              `json:"primary_language"`
	Frameworks       []string            `json:"frameworks"`
	HasTests         bool                `json:"has_tests"`
	RecommendedRoles map[string][]string `json:"recommended_roles"`
	DetectedFiles    []string            `json:"detected_files"`
	HasExistingSDD   bool                `json:"has_existing_sdd"`
	SpecsRepository  string              `json:"specs_repository,omitempty"`
	DomainContext    string              `json:"domain_context,omitempty"`
	Projects         []DetectedProject   `json:"projects,omitempty"`
	ConfiguredRoles  []RoleInput         `json:"configured_roles,omitempty"`
	AdoptedSkills    []AdoptedSkillInfo  `json:"adopted_skills,omitempty"`
}

// InitOptions define las opciones de configuración para inicializar un proyecto.
type InitOptions struct {
	Path     string
	Name     string
	Topology string // monorepo-embedded, multirepo
	Force    bool
	Roles    []RoleInput
}

// InitResult detalla los resultados de la operación de inicialización.
type InitResult struct {
	ConfigPath         string
	Record             WorkspaceRecord
	CreatedFiles       []string
	AlreadyExisted     bool
	AdoptedSkillsCount int
}

