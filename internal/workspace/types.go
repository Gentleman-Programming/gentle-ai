package workspace

// TopologyType define las topologías de espacio de trabajo admitidas en Axiom.
type TopologyType string

const (
	// TopologyMonorepoEmbedded: código y especificaciones conviven en el mismo repositorio.
	TopologyMonorepoEmbedded TopologyType = "monorepo-embedded"

	// TopologyMonorepoDecoupled: código en monorrepo y especificaciones en repositorio independiente dentro de la carpeta maestra.
	TopologyMonorepoDecoupled TopologyType = "monorepo-decoupled"

	// TopologyMultirepo: múltiples repositorios de código por roles y repositorio canónico de especificaciones obligatorio.
	TopologyMultirepo TopologyType = "multirepo"
)

// WorkspaceSection encapsula la identidad, topología y rutas base del espacio de trabajo.
type WorkspaceSection struct {
	Name            string       `yaml:"name"`
	Topology        TopologyType `yaml:"topology"`
	SpecsRepository string       `yaml:"specs_repository"`
	Root            string       `yaml:"root,omitempty"`
}

// RepositoryEntry define una ruta a un repositorio o componente local.
type RepositoryEntry struct {
	Path string `yaml:"path"`
}

// RoleConfig define las responsabilidades, tecnologías y repositorios asignados a un rol.
type RoleConfig struct {
	Name         string            `yaml:"name"`
	Repositories []RepositoryEntry `yaml:"repositories"`
	Tech         []string          `yaml:"tech,omitempty"`
}

// GovernanceConfig define configuraciones transversales del proyecto.
type GovernanceConfig struct {
	Language         string `yaml:"language,omitempty"`
	SharedMemory     string `yaml:"shared_memory,omitempty"`
	SemanticAnalysis string `yaml:"semantic_analysis,omitempty"`
}

// WorkspaceConfig es la raíz del archivo de configuración axiom.yaml.
type WorkspaceConfig struct {
	Workspace  WorkspaceSection      `yaml:"workspace"`
	Roles      map[string]RoleConfig `yaml:"roles"`
	Governance GovernanceConfig      `yaml:"governance,omitempty"`
}

// ValidationReport contiene el resultado determinista de la validación del espacio de trabajo.
type ValidationReport struct {
	Valid         bool         `json:"valid"`
	Topology      TopologyType `json:"topology"`
	WorkspaceRoot string       `json:"workspace_root"`
	CheckedPaths  []string     `json:"checked_paths"`
	Errors        []string     `json:"errors"`
	Warnings      []string     `json:"warnings"`
}
