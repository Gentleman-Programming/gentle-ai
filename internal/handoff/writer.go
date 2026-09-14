package handoff

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Format serializa una estructura Handoff a su representación textual canónica en Markdown con Frontmatter YAML.
func Format(h *Handoff) (string, error) {
	if h == nil {
		return "", fmt.Errorf("no se puede formatear un handoff nulo")
	}

	metaCopy := h.Metadata
	if metaCopy.Timestamp.IsZero() {
		metaCopy.Timestamp = time.Now().UTC()
	}

	yamlBytes, err := yaml.Marshal(metaCopy)
	if err != nil {
		return "", fmt.Errorf("error al serializar los metadatos YAML: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(string(yamlBytes))
	sb.WriteString("---\n\n")

	// Sección 1: Resumen Ejecutivo
	sb.WriteString(HeaderExecutiveSummary + "\n\n")
	if strings.TrimSpace(h.Sections.ExecutiveSummary) != "" {
		sb.WriteString(strings.TrimSpace(h.Sections.ExecutiveSummary) + "\n\n")
	} else {
		sb.WriteString("*(Sin resumen registrado)*\n\n")
	}

	// Sección 2: Artefactos Modificados y Creados
	sb.WriteString(HeaderArtifacts + "\n\n")
	if strings.TrimSpace(h.Sections.Artifacts) != "" {
		sb.WriteString(strings.TrimSpace(h.Sections.Artifacts) + "\n\n")
	} else {
		sb.WriteString("*(Ninguno)*\n\n")
	}

	// Sección 3: Decisiones Técnicas y Acuerdos
	sb.WriteString(HeaderDecisions + "\n\n")
	if strings.TrimSpace(h.Sections.Decisions) != "" {
		sb.WriteString(strings.TrimSpace(h.Sections.Decisions) + "\n\n")
	} else {
		sb.WriteString("*(Ninguna decisión adicional)*\n\n")
	}

	// Sección 4: Riesgos, Bloqueos y Preguntas Abiertas
	sb.WriteString(HeaderRisksAndBlockers + "\n\n")
	if strings.TrimSpace(h.Sections.RisksAndBlockers) != "" {
		sb.WriteString(strings.TrimSpace(h.Sections.RisksAndBlockers) + "\n\n")
	} else {
		sb.WriteString("*(Sin bloqueos reportados)*\n\n")
	}

	// Sección 5: Instrucciones Directas para el Siguiente Rol
	sb.WriteString(HeaderDirectInstructions + "\n\n")
	if strings.TrimSpace(h.Sections.DirectInstructions) != "" {
		sb.WriteString(strings.TrimSpace(h.Sections.DirectInstructions) + "\n")
	} else {
		sb.WriteString("*(Continuar con la fase asignada)*\n")
	}

	return sb.String(), nil
}

// WriteFile guarda el documento handoff en el archivo de destino en disco.
func WriteFile(filePath string, h *Handoff) error {
	content, err := Format(h)
	if err != nil {
		return err
	}

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("no se pudo crear el directorio contenedor %q: %w", dir, err)
	}

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("error al escribir el archivo de handoff %q: %w", filePath, err)
	}

	return nil
}
