package autoskill

import (
	"os"
	"path/filepath"
	"strings"
)

// Miner examina el código fuente local de los repositorios para descubrir patrones idiomáticos internos.
type Miner struct{}

// NewMiner instancia el analizador heurístico de código.
func NewMiner() *Miner {
	return &Miner{}
}

// MinePatterns recorre el repositorio y formula propuestas de skills basadas en convenciones recurrentes.
func (m *Miner) MinePatterns(repoRoot string) ([]MiningProposal, error) {
	var proposals []MiningProposal

	// 1. Minería de Pruebas Tabulares en Go
	if hasTableDrivenTests(repoRoot) {
		proposals = append(proposals, MiningProposal{
			Name:        "axiom-go-table-tests",
			PatternID:   "pattern-table-tests",
			Title:       "Convención de Pruebas Tabulares en Go (Table-Driven Tests)",
			Description: "Estandarización de pruebas unitarias idiomáticas en Go mediante estructuras anónimas y subpruebas t.Run",
			Trigger:     "Al crear o modificar suites de pruebas unitarias *_test.go en paquetes Go",
			Justification: "Detectadas múltiples suites de pruebas unitarias estructuradas con tests := []struct y t.Run en el repositorio",
			SkillMD: `# Convención de Pruebas Tabulares en Go (Table-Driven Tests)

description: "Estandarización de pruebas unitarias idiomáticas en Go mediante estructuras anónimas y subpruebas t.Run"
trigger: "Al crear o modificar suites de pruebas unitarias *_test.go en paquetes Go"
origin: "mined"

---

## Propósito

Asegurar que todas las pruebas unitarias en Go mantengan alta legibilidad, cobertura de casos borde y diagnóstico rápido de fallos mediante el patrón idiomático de Table-Driven Tests recomendado por el equipo de Go y adoptado canónicamente en Axiom.

## Reglas Obligatorias

1. **Estructura de Casos:** Definir los casos de prueba dentro de un slice de structs anónimos con nombres descriptivos:
   ` + "```go" + `
   tests := []struct {
       name     string
       input    string
       expected string
       wantErr  bool
   }{
       {
           name:     "caso exitoso nominal",
           input:    "valido",
           expected: "resultado",
           wantErr:  false,
       },
   }
   ` + "```" + `

2. **Aislamiento con ` + "`t.Run`" + `:** Ejecutar cada caso en su propia subprueba nombrada:
   ` + "```go" + `
   for _, tt := range tests {
       t.Run(tt.name, func(t *testing.T) {
           got, err := FuncionBajoPrueba(tt.input)
           if (err != nil) != tt.wantErr {
               t.Fatalf("FuncionBajoPrueba() error = %v, wantErr %v", err, tt.wantErr)
           }
           if got != tt.expected {
               t.Errorf("FuncionBajoPrueba() = %v, esperado %v", got, tt.expected)
           }
       })
   }
   ` + "```" + `

3. **Cero Salidas Globales:** No usar ` + "`os.Exit`" + ` ni ` + "`panic`" + ` dentro de las funciones de prueba; emplear ` + "`t.Errorf`" + ` para fallos no fatales y ` + "`t.Fatalf`" + ` para precondiciones.
`,
		})
	}

	// 2. Minería de Encapsulación en Paquetes `internal/`
	if hasInternalLayering(repoRoot) {
		proposals = append(proposals, MiningProposal{
			Name:        "axiom-internal-layering",
			PatternID:   "pattern-internal-layering",
			Title:       "Arquitectura y Encapsulación en Paquetes internal/",
			Description: "Convención de diseño de módulos de dominio aislados en internal/ con separación types.go y service.go",
			Trigger:     "Al añadir nuevos módulos, componentes o paquetes de dominio al runtime de Axiom",
			Justification: "Detectada estructura modular canónica con paquetes desacoplados en internal/ (tipos, servicios y tests)",
			SkillMD: `# Arquitectura y Encapsulación en Paquetes internal/

description: "Convención de diseño de módulos de dominio aislados en internal/ con separación types.go y service.go"
trigger: "Al añadir nuevos módulos, componentes o paquetes de dominio al runtime de Axiom"
origin: "mined"

---

## Propósito

Garantizar la protección de la API interna del runtime de Axiom y mantener un alto desacoplamiento entre componentes siguiendo las directrices de la carpeta ` + "`internal/`" + ` de Go.

## Convenciones de Organización

1. **Separación de Responsabilidades:**
   - ` + "`types.go`" + `: Definición exclusiva de estructuras, interfaces, enums y DTOs del dominio.
   - ` + "`service.go`" + ` o lógica específica: Implementación de la lógica de negocio y constructores ` + "`New...`" + `.
   - ` + "`<modulo>_test.go`" + `: Pruebas unitarias de caja negra o caja blanca del paquete.

2. **Inyección de Dependencias:**
   - Los servicios deben recibir sus dependencias o interfaces en el constructor (` + "`NewService(...)`" + `) sin variables globales mutables.
`,
		})
	}

	// 3. Minería de Envoltorio Idiomático de Errores con %w
	if hasIdiomaticErrorWrapping(repoRoot) {
		proposals = append(proposals, MiningProposal{
			Name:        "axiom-idiomatic-error-wrapping",
			PatternID:   "pattern-error-wrapping",
			Title:       "Manejo y Envoltorio Idiomático de Errores en Go (%w)",
			Description: "Uso estricto de fmt.Errorf con el especificador %w para preservación de cadenas de error y errores centinela",
			Trigger:     "Al capturar, propagar o formatear errores en cualquier paquete Go",
			Justification: "Detectado uso recurrente del especificador %w en fmt.Errorf para preservación contextual de errores",
			SkillMD: `# Manejo y Envoltorio Idiomático de Errores en Go (%w)

description: "Uso estricto de fmt.Errorf con el especificador %w para preservación de cadenas de error y errores centinela"
trigger: "Al capturar, propagar o formatear errores en cualquier paquete Go"
origin: "mined"

---

## Propósito

Preservar el contexto de ejecución y la causa raíz de los errores a través de las capas del sistema, permitiendo la inspección mediante ` + "`errors.Is`" + ` y ` + "`errors.As`" + `.

## Reglas Obligatorias

1. **Envoltorio Contextual:** Siempre envolver los errores salientes con contexto:
   ` + "```go" + `
   if err != nil {
       return nil, fmt.Errorf("error cargando configuración desde %s: %w", path, err)
   }
   ` + "```" + `

2. **Prohibido %v o %s para Errores Propagados:** No usar ` + "`%v`" + ` o ` + "`%s`" + ` cuando se requiere inspección de causas raíz; reservar ` + "`%w`" + ` para envolver la interfaz ` + "`error`" + `.
`,
		})
	}

	return proposals, nil
}

func hasTableDrivenTests(root string) bool {
	count := 0
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || count >= 2 {
			return filepath.SkipDir
		}
		if info.IsDir() {
			n := info.Name()
			if n == ".git" || n == "vendor" || n == "node_modules" || n == ".axiom" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(info.Name(), "_test.go") {
			content, readErr := os.ReadFile(path)
			if readErr == nil {
				s := string(content)
				if (strings.Contains(s, "tests := []struct") || strings.Contains(s, "tests := [...]struct") || strings.Contains(s, "testCases := []struct")) && strings.Contains(s, "t.Run(") {
					count++
				}
			}
		}
		return nil
	})
	return count >= 1
}

func hasInternalLayering(root string) bool {
	internalDir := filepath.Join(root, "internal")
	info, err := os.Stat(internalDir)
	if err != nil || !info.IsDir() {
		return false
	}

	entries, err := os.ReadDir(internalDir)
	if err != nil {
		return false
	}

	subdirs := 0
	for _, e := range entries {
		if e.IsDir() {
			subdirs++
		}
	}
	return subdirs >= 2
}

func hasIdiomaticErrorWrapping(root string) bool {
	count := 0
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || count >= 3 {
			return filepath.SkipDir
		}
		if info.IsDir() {
			n := info.Name()
			if n == ".git" || n == "vendor" || n == "node_modules" || n == ".axiom" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(info.Name(), ".go") {
			content, readErr := os.ReadFile(path)
			if readErr == nil {
				if strings.Contains(string(content), `fmt.Errorf(`) && strings.Contains(string(content), `%w`) {
					count++
				}
			}
		}
		return nil
	})
	return count >= 2
}
