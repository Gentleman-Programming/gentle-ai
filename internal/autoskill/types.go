package autoskill

import "time"

// OriginType indica la fuente de origen de una skill propuesta.
type OriginType string

const (
	OriginMidudev OriginType = "midudev"
	OriginMined   OriginType = "mined"
)

// DetectConfig define las reglas declarativas para identificar una tecnología.
type DetectConfig struct {
	Packages       []string `json:"packages,omitempty"`
	ConfigFiles    []string `json:"config_files,omitempty"`
	FileExtensions []string `json:"file_extensions,omitempty"`
}

// Technology representa una tecnología mapeada con skills asociadas.
type Technology struct {
	ID     string       `json:"id"`
	Name   string       `json:"name"`
	Detect DetectConfig `json:"detect"`
	Skills []string     `json:"skills"`
}

// SKILLS_MAP contiene el mapa canónico de tecnologías reconocidas por defecto en Axiom.
var SKILLS_MAP = []Technology{
	{
		ID:   "react",
		Name: "React",
		Detect: DetectConfig{
			Packages: []string{"react", "react-dom"},
		},
		Skills: []string{
			"vercel-labs/agent-skills/react-best-practices",
		},
	},
	{
		ID:   "nextjs",
		Name: "Next.js",
		Detect: DetectConfig{
			Packages:    []string{"next"},
			ConfigFiles: []string{"next.config.js", "next.config.mjs", "next.config.ts"},
		},
		Skills: []string{
			"vercel-labs/next-skills/next-best-practices",
		},
	},
	{
		ID:   "vue",
		Name: "Vue",
		Detect: DetectConfig{
			Packages: []string{"vue"},
		},
		Skills: []string{
			"antfu/skills/vue-best-practices",
		},
	},
	{
		ID:   "nuxt",
		Name: "Nuxt",
		Detect: DetectConfig{
			Packages:    []string{"nuxt"},
			ConfigFiles: []string{"nuxt.config.js", "nuxt.config.ts"},
		},
		Skills: []string{
			"antfu/skills/nuxt",
		},
	},
	{
		ID:   "tailwind",
		Name: "Tailwind CSS",
		Detect: DetectConfig{
			Packages:    []string{"tailwindcss"},
			ConfigFiles: []string{"tailwind.config.js", "tailwind.config.mjs", "tailwind.config.ts"},
		},
		Skills: []string{
			"tailwind-best-practices",
		},
	},
	{
		ID:   "go",
		Name: "Go",
		Detect: DetectConfig{
			ConfigFiles:    []string{"go.mod"},
			FileExtensions: []string{".go"},
		},
		Skills: []string{
			"go-best-practices",
		},
	},
	{
		ID:   "docker",
		Name: "Docker",
		Detect: DetectConfig{
			ConfigFiles: []string{"Dockerfile", "docker-compose.yml", "docker-compose.yaml", "compose.yaml"},
		},
		Skills: []string{
			"docker-best-practices",
		},
	},
}

// RegistryReview resume el dictamen de seguridad auditado por midudev/autoskills.
type RegistryReview struct {
	Status     string   `json:"status"`
	Flags      []string `json:"flags"`
	Summary    string   `json:"summary"`
	ReviewedAt string   `json:"reviewedAt,omitempty"`
}

// RegistrySkillEntry describe una skill dentro del manifiesto index.json.
type RegistrySkillEntry struct {
	Source     string            `json:"source"`
	SkillPath  string            `json:"skillPath"`
	CommitSHA  string            `json:"commitSha,omitempty"`
	Files      []string          `json:"files"`
	SHA256     map[string]string `json:"sha256"`
	BundleHash string            `json:"bundleHash,omitempty"`
	Review     RegistryReview    `json:"review,omitempty"`
}

// RegistryIndex representa la raíz del manifiesto oficial de midudev/autoskills.
type RegistryIndex struct {
	Version     int                           `json:"version"`
	GeneratedAt string                        `json:"generatedAt"`
	Skills      map[string]RegistrySkillEntry `json:"skills"`
}

// ProposalMetadata contiene la trazabilidad y auditoría de una propuesta depositada en el buzón.
type ProposalMetadata struct {
	Name          string            `json:"name"`
	Origin        OriginType        `json:"origin"`
	Source        string            `json:"source"`
	Files         []string          `json:"files"`
	SHA256        map[string]string `json:"sha256"`
	Verified      bool              `json:"verified"`
	Role          string            `json:"role,omitempty"`
	DetectedBy    string            `json:"detected_by"`
	Justification string            `json:"justification"`
	CreatedAt     time.Time         `json:"created_at"`
}

// SkillProposal modela una propuesta completa en memoria para la CLI y el Dashboard Web.
type SkillProposal struct {
	Metadata ProposalMetadata `json:"metadata"`
	SkillMD  string           `json:"skill_md"`
}

// MiningProposal representa un patrón heurístico descubierto por el minero local.
type MiningProposal struct {
	Name          string `json:"name"`
	PatternID     string `json:"pattern_id"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	Trigger       string `json:"trigger"`
	SkillMD       string `json:"skill_md"`
	Justification string `json:"justification"`
}

// DetectedTech describe una tecnología identificada en el proyecto vinculada a un rol.
type DetectedTech struct {
	Tech Technology `json:"tech"`
	Role string     `json:"role,omitempty"`
	Path string     `json:"path"`
}

// ScanReport resume los hallazgos de una ejecución de escaneo de autoskills.
type ScanReport struct {
	DetectedTechnologies []string        `json:"detected_technologies"`
	SkillsProposed       []SkillProposal `json:"skills_proposed"`
	MinedSkillsCount     int             `json:"mined_skills_count"`
	RegistrySkillsCount  int             `json:"registry_skills_count"`
	TotalInInbox         int             `json:"total_in_inbox"`
}
