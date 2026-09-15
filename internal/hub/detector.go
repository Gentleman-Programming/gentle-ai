package hub

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

	// Valores predeterminados si no se detectó nada
	if len(result.RecommendedRoles["core"]) == 0 {
		result.RecommendedRoles["core"] = []string{"generic"}
		result.RecommendedRoles["qa"] = []string{"verification"}
	}

	return result, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
