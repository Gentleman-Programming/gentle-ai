package handoff

import (
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Parse lee un io.Reader y deserializa un documento handoff.md completo.
func Parse(r io.Reader) (*Handoff, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("error al leer el contenido del handoff: %w", err)
	}
	return ParseBytes(data)
}

// ParseFile lee un archivo en disco y deserializa el handoff.
func ParseFile(filePath string) (*Handoff, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("error al abrir el archivo de handoff %q: %w", filePath, err)
	}
	return ParseBytes(data)
}

// ParseBytes analiza los bytes de un documento handoff.md.
func ParseBytes(data []byte) (*Handoff, error) {
	content := string(data)
	// Normalizar retornos de carro de Windows a formato Unix
	content = strings.ReplaceAll(content, "\r\n", "\n")

	trimmed := strings.TrimLeft(content, " \t\n")
	if !strings.HasPrefix(trimmed, "---") {
		return nil, fmt.Errorf("el documento no contiene un encabezado frontmatter YAML delimitado por '---'")
	}

	// Buscar el delimitador de cierre de frontmatter
	rest := trimmed[3:]
	// Puede haber un salto de línea inmediato tras los tres guiones de apertura
	if strings.HasPrefix(rest, "\n") {
		rest = rest[1:]
	}

	closingIdx := strings.Index(rest, "\n---")
	if closingIdx == -1 {
		return nil, fmt.Errorf("el encabezado frontmatter YAML no tiene delimitador de cierre '---'")
	}

	yamlContent := rest[:closingIdx]
	bodyContent := rest[closingIdx+4:] // saltar \n---

	var meta Metadata
	if err := yaml.Unmarshal([]byte(yamlContent), &meta); err != nil {
		return nil, fmt.Errorf("sintaxis YAML inválida en los metadatos del handoff: %w", err)
	}

	// Validar que las secciones canónicas estén presentes en el cuerpo markdown
	headers := []string{
		HeaderExecutiveSummary,
		HeaderArtifacts,
		HeaderDecisions,
		HeaderRisksAndBlockers,
		HeaderDirectInstructions,
	}

	indices := make([]int, len(headers))
	for i, h := range headers {
		idx := strings.Index(bodyContent, h)
		if idx == -1 {
			return nil, fmt.Errorf("sección obligatoria faltante en handoff.md: %q", h)
		}
		indices[i] = idx
	}

	// Verificar que las secciones aparezcan en orden secuencial
	for i := 0; i < len(indices)-1; i++ {
		if indices[i] >= indices[i+1] {
			return nil, fmt.Errorf("las secciones obligatorias de handoff.md no respetan el orden canónico esperado")
		}
	}

	// Extraer contenido de cada sección
	secValues := make([]string, len(headers))
	for i := 0; i < len(headers); i++ {
		start := indices[i] + len(headers[i])
		var end int
		if i < len(headers)-1 {
			end = indices[i+1]
		} else {
			end = len(bodyContent)
		}
		secValues[i] = strings.TrimSpace(bodyContent[start:end])
	}

	return &Handoff{
		Metadata: meta,
		Sections: Sections{
			ExecutiveSummary:   secValues[0],
			Artifacts:          secValues[1],
			Decisions:          secValues[2],
			RisksAndBlockers:   secValues[3],
			DirectInstructions: secValues[4],
		},
	}, nil
}
