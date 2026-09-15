package hub

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Detector inspecciona un directorio de código fuente para inferir tecnologías y dependencias.
type Detector struct{}

// NewDetector instancia un nuevo detector de tecnologías.
func NewDetector() *Detector {
	return &Detector{}
}

// Detect analiza los archivos presentes en dirPath y devuelve un TechDetection estructurado.
func (d *Detector) Detect(dirPath string) (*TechDetection, error) {
	absPath, err := filepath.Abs(dirPath)
	if err != nil {
		return nil, err
	}

	result := &TechDetection{
		PrimaryLanguage:  "generic",
		Frameworks:       make([]string, 0),
		DetectedFiles:    make([]string, 0),
		RecommendedRoles: make(map[string][]string),
	}

	// 1. Detección Go
	if fileExists(filepath.Join(absPath, "go.mod")) {
		result.PrimaryLanguage = "go"
		result.DetectedFiles = append(result.DetectedFiles, "go.mod")
		result.Frameworks = append(result.Frameworks, "go")
		result.RecommendedRoles["core"] = []string{"go"}
		result.RecommendedRoles["qa"] = []string{"go-test"}
		result.HasTests = true
	}

	// 2. Detección .NET / C#
	csprojMatches, _ := filepath.Glob(filepath.Join(absPath, "*.csproj"))
	slnMatches, _ := filepath.Glob(filepath.Join(absPath, "*.sln"))
	if len(csprojMatches) > 0 || len(slnMatches) > 0 {
		if result.PrimaryLanguage == "generic" {
			result.PrimaryLanguage = "csharp"
		}
		for _, m := range append(csprojMatches, slnMatches...) {
			result.DetectedFiles = append(result.DetectedFiles, filepath.Base(m))
		}
		result.Frameworks = append(result.Frameworks, "dotnet")
		result.RecommendedRoles["core"] = []string{"csharp", "dotnet"}
		result.RecommendedRoles["qa"] = []string{"dotnet-test"}
		result.HasTests = true
	}

	// 3. Detección Node.js / TypeScript / JavaScript
	pkgJsonPath := filepath.Join(absPath, "package.json")
	if fileExists(pkgJsonPath) {
		result.DetectedFiles = append(result.DetectedFiles, "package.json")
		if result.PrimaryLanguage == "generic" {
			if fileExists(filepath.Join(absPath, "tsconfig.json")) {
				result.PrimaryLanguage = "typescript"
				result.DetectedFiles = append(result.DetectedFiles, "tsconfig.json")
			} else {
				result.PrimaryLanguage = "javascript"
			}
		}

		if data, err := os.ReadFile(pkgJsonPath); err == nil {
			var pkg map[string]interface{}
			if err := json.Unmarshal(data, &pkg); err == nil {
				deps := make(map[string]bool)
				for _, depKey := range []string{"dependencies", "devDependencies", "peerDependencies"} {
					if raw, ok := pkg[depKey].(map[string]interface{}); ok {
						for k := range raw {
							deps[strings.ToLower(k)] = true
						}
					}
				}

				if deps["react"] || deps["react-dom"] {
					result.Frameworks = append(result.Frameworks, "react")
				}
				if deps["next"] {
					result.Frameworks = append(result.Frameworks, "nextjs")
				}
				if deps["vue"] || deps["nuxt"] {
					result.Frameworks = append(result.Frameworks, "vue")
				}
				if deps["svelte"] || deps["@sveltejs/kit"] {
					result.Frameworks = append(result.Frameworks, "svelte")
				}
				if deps["express"] || deps["fastify"] || deps["hono"] {
					result.Frameworks = append(result.Frameworks, "node-backend")
				}
				if deps["vitest"] || deps["jest"] || deps["playwright"] || deps["cypress"] {
					result.HasTests = true
				}
			}
		}

		if len(result.RecommendedRoles["core"]) == 0 {
			result.RecommendedRoles["core"] = []string{result.PrimaryLanguage}
			result.RecommendedRoles["qa"] = []string{"test-runner"}
		}
	}

	// 4. Detección Rust
	if fileExists(filepath.Join(absPath, "Cargo.toml")) {
		result.DetectedFiles = append(result.DetectedFiles, "Cargo.toml")
		if result.PrimaryLanguage == "generic" {
			result.PrimaryLanguage = "rust"
			result.Frameworks = append(result.Frameworks, "cargo")
			result.RecommendedRoles["core"] = []string{"rust"}
			result.RecommendedRoles["qa"] = []string{"cargo-test"}
			result.HasTests = true
		}
	}

	// 5. Detección Python
	pyprojectPath := filepath.Join(absPath, "pyproject.toml")
	reqsPath := filepath.Join(absPath, "requirements.txt")
	if fileExists(pyprojectPath) || fileExists(reqsPath) {
		if fileExists(pyprojectPath) {
			result.DetectedFiles = append(result.DetectedFiles, "pyproject.toml")
		}
		if fileExists(reqsPath) {
			result.DetectedFiles = append(result.DetectedFiles, "requirements.txt")
		}
		if result.PrimaryLanguage == "generic" {
			result.PrimaryLanguage = "python"
			result.Frameworks = append(result.Frameworks, "python")
			result.RecommendedRoles["core"] = []string{"python"}
			result.RecommendedRoles["qa"] = []string{"pytest"}
			result.HasTests = true
		}
	}

	// 6. Detección de SDD previo (openspec, .openspec, specs)
	detectExistingSDD(absPath, result)

	// 7. Detección de agentes, prompts y skills preexistentes (.agents/skills, .github/copilot, etc.)
	detectExistingAgents(absPath, result)

	// Valores predeterminados si no se detectó nada
	if len(result.RecommendedRoles["core"]) == 0 {
		result.RecommendedRoles["core"] = []string{"generic"}
		result.RecommendedRoles["qa"] = []string{"verification"}
	}

	return result, nil
}

type openSpecConfig struct {
	Schema   string `yaml:"schema"`
	Language string `yaml:"language"`
	Context  string `yaml:"context"`
	Testing  struct {
		WorkspaceCommand string `yaml:"workspace_command"`
		Framework        string `yaml:"framework"`
	} `yaml:"testing"`
	Projects []struct {
		Path        string `yaml:"path"`
		Stack       string `yaml:"stack"`
		TestCommand string `yaml:"test_command"`
	} `yaml:"projects"`
}

func detectExistingSDD(root string, result *TechDetection) {
	// Verificar openspec o .openspec
	openSpecDir := filepath.Join(root, "openspec")
	dotOpenSpecDir := filepath.Join(root, ".openspec")

	if dirExists(openSpecDir) || dirExists(dotOpenSpecDir) {
		result.HasExistingSDD = true
		if dirExists(openSpecDir) {
			result.SpecsRepository = "openspec"
		} else {
			result.SpecsRepository = ".openspec"
		}
		result.DetectedFiles = append(result.DetectedFiles, result.SpecsRepository)
	}

	// Verificar si existe openspec/config.yaml
	cfgPath := filepath.Join(root, "openspec", "config.yaml")
	if !fileExists(cfgPath) {
		cfgPath = filepath.Join(root, ".openspec", "config.yaml")
	}

	if fileExists(cfgPath) {
		result.HasExistingSDD = true
		result.DetectedFiles = append(result.DetectedFiles, "config.yaml")

		data, err := os.ReadFile(cfgPath)
		if err == nil {
			var osCfg openSpecConfig
			if err := yaml.Unmarshal(data, &osCfg); err == nil {
				result.DomainContext = strings.TrimSpace(osCfg.Context)

				if len(osCfg.Projects) > 0 {
					rolesMap := make(map[string]*RoleInput)

					for _, p := range osCfg.Projects {
						pClean := filepath.ToSlash(p.Path)
						dp := DetectedProject{
							Path:        pClean,
							Stack:       p.Stack,
							TestCommand: p.TestCommand,
						}

						lowerPath := strings.ToLower(pClean)
						lowerStack := strings.ToLower(p.Stack)

						// Inferir rol
						roleKey := "core"
						roleName := "Core Domain & Logic"
						if strings.Contains(lowerPath, "web") || strings.Contains(lowerPath, "ui") || strings.Contains(lowerPath, "front") || strings.Contains(lowerStack, "blazor") || strings.Contains(lowerStack, "react") || strings.Contains(lowerStack, "vue") {
							roleKey = "web"
							roleName = "Web UI & Presentation"
						} else if strings.Contains(lowerPath, "test") || strings.Contains(lowerPath, "tests") || strings.Contains(lowerPath, "unit") || strings.Contains(lowerStack, "test") || strings.Contains(lowerStack, "xunit") {
							roleKey = "qa"
							roleName = "Quality Assurance & Tests"
						}

						dp.Role = roleKey
						result.Projects = append(result.Projects, dp)

						// Agrupar en rolesMap
						if existing, ok := rolesMap[roleKey]; ok {
							existing.Repositories = append(existing.Repositories, pClean)
							if p.Stack != "" && !sliceContains(existing.Tech, p.Stack) {
								existing.Tech = append(existing.Tech, extractTechTokens(p.Stack)...)
							}
						} else {
							rolesMap[roleKey] = &RoleInput{
								Key:          roleKey,
								Name:         roleName,
								Repositories: []string{pClean},
								NonBlocking:  false,
								Tech:         extractTechTokens(p.Stack),
							}
						}
					}

					// Pasar a slice ordenado
					order := []string{"core", "web", "qa"}
					for _, k := range order {
						if r, ok := rolesMap[k]; ok {
							r.Tech = deduplicateStrings(r.Tech)
							result.ConfiguredRoles = append(result.ConfiguredRoles, *r)
							delete(rolesMap, k)
						}
					}
					for _, r := range rolesMap {
						r.Tech = deduplicateStrings(r.Tech)
						result.ConfiguredRoles = append(result.ConfiguredRoles, *r)
					}
				}
			}
		}
	}
}

func detectExistingAgents(root string, result *TechDetection) {
	// 1. Escanear .agents/skills
	agentsSkillsDir := filepath.Join(root, ".agents", "skills")
	if dirExists(agentsSkillsDir) {
		entries, err := os.ReadDir(agentsSkillsDir)
		if err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				skillFile := filepath.Join(agentsSkillsDir, e.Name(), "SKILL.md")
				if fileExists(skillFile) {
					category := "tech"
					role := ""
					nameLower := strings.ToLower(e.Name())

					if strings.HasPrefix(nameLower, "sdd-") || strings.Contains(nameLower, "spec") || strings.Contains(nameLower, "propose") || strings.Contains(nameLower, "archive") || strings.Contains(nameLower, "verify") || strings.Contains(nameLower, "commit") {
						category = "process"
					} else if strings.Contains(nameLower, "blazor") || strings.Contains(nameLower, "front") || strings.Contains(nameLower, "tailwind") || strings.Contains(nameLower, "accessib") || strings.Contains(nameLower, "web") {
						category = "tech"
						role = "web"
					} else if strings.Contains(nameLower, "csharp") || strings.Contains(nameLower, "dotnet") || strings.Contains(nameLower, "async") || strings.Contains(nameLower, "aspnet") {
						category = "tech"
						role = "core"
					} else if strings.Contains(nameLower, "test") || strings.Contains(nameLower, "xunit") || strings.Contains(nameLower, "qa") {
						category = "tech"
						role = "qa"
					}

					desc := extractDescription(skillFile)
					result.AdoptedSkills = append(result.AdoptedSkills, AdoptedSkillInfo{
						Name:        e.Name(),
						Path:        filepath.ToSlash(filepath.Join(".agents", "skills", e.Name(), "SKILL.md")),
						Role:        role,
						Category:    category,
						Description: desc,
					})
				}
			}
		}
	}

	// 2. Escanear .agents/rules
	agentsRulesDir := filepath.Join(root, ".agents", "rules")
	if dirExists(agentsRulesDir) {
		entries, err := os.ReadDir(agentsRulesDir)
		if err == nil {
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
					relPath := filepath.ToSlash(filepath.Join(".agents", "rules", e.Name()))
					result.AdoptedSkills = append(result.AdoptedSkills, AdoptedSkillInfo{
						Name:        strings.TrimSuffix(e.Name(), ".md"),
						Path:        relPath,
						Category:    "rule",
						Description: "Regla de gobernanza preexistente",
					})
				}
			}
		}
	}

	// 3. Escanear .github/copilot-instructions.md
	copilotPath := filepath.Join(root, ".github", "copilot-instructions.md")
	if fileExists(copilotPath) {
		result.AdoptedSkills = append(result.AdoptedSkills, AdoptedSkillInfo{
			Name:        "copilot-instructions",
			Path:        filepath.ToSlash(filepath.Join(".github", "copilot-instructions.md")),
			Category:    "process",
			Description: "Instrucciones de GitHub Copilot para el repositorio",
		})
	}
}

func extractDescription(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "description:") {
			d := strings.TrimSpace(strings.TrimPrefix(trimmed, "description:"))
			return strings.Trim(d, `"'`)
		}
	}
	return ""
}

func extractTechTokens(stack string) []string {
	var tokens []string
	lower := strings.ToLower(stack)
	keywords := []string{"dotnet", "csharp", "blazor", "xunit", "react", "next", "vue", "tailwind", "go", "python", "rust"}
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			tokens = append(tokens, kw)
		}
	}
	if len(tokens) == 0 && stack != "" {
		tokens = append(tokens, strings.ToLower(strings.ReplaceAll(stack, " ", "-")))
	}
	return tokens
}

func sliceContains(s []string, item string) bool {
	for _, x := range s {
		if strings.EqualFold(x, item) {
			return true
		}
	}
	return false
}

func deduplicateStrings(s []string) []string {
	seen := make(map[string]bool)
	var res []string
	for _, x := range s {
		clean := strings.TrimSpace(x)
		if clean != "" && !seen[clean] {
			seen[clean] = true
			res = append(res, clean)
		}
	}
	return res
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

