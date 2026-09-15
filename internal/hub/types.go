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

// TechDetection encapsula los hallazgos del detector de tecnologías sobre un repositorio.
type TechDetection struct {
	PrimaryLanguage  string              `json:"primary_language"`
	Frameworks       []string            `json:"frameworks"`
	HasTests         bool                `json:"has_tests"`
	RecommendedRoles map[string][]string `json:"recommended_roles"`
	DetectedFiles    []string            `json:"detected_files"`
}

// InitOptions define las opciones de configuración para inicializar un proyecto.
type InitOptions struct {
	Path     string
	Name     string
	Topology string // monorepo-embedded, multirepo
	Force    bool
}

// InitResult detalla los resultados de la operación de inicialización.
type InitResult struct {
	ConfigPath     string
	Record         WorkspaceRecord
	CreatedFiles   []string
	AlreadyExisted bool
}
