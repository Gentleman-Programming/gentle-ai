package handoff

import (
	"fmt"
	"strings"
	"time"
)

// EngramPayload representa la estructura serializable para persistencia en Engram MCP vía mem_save.
type EngramPayload struct {
	Title         string `json:"title"`
	TopicKey      string `json:"topic_key"`
	Type          string `json:"type"`
	Project       string `json:"project,omitempty"`
	CapturePrompt bool   `json:"capture_prompt"`
	Content       string `json:"content"`
}

// ToEngramPayload genera el objeto de observación formateado para guardar en Engram.
func ToEngramPayload(h *Handoff) EngramPayload {
	if h == nil {
		return EngramPayload{}
	}

	topicKey := fmt.Sprintf("sdd/%s/handoff", h.Metadata.Change)
	title := fmt.Sprintf("sdd/%s/handoff (%s -> %s)", h.Metadata.Change, h.Metadata.FromPhase, h.Metadata.ToPhase)

	ts := h.Metadata.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Handoff: %s (%s -> %s)\n\n", h.Metadata.Change, h.Metadata.FromPhase, h.Metadata.ToPhase))
	sb.WriteString(fmt.Sprintf("- **Rol Emisor:** %s (Fase: `%s`)\n", h.Metadata.FromRole, h.Metadata.FromPhase))
	sb.WriteString(fmt.Sprintf("- **Rol Receptor:** %s (Fase: `%s`)\n", h.Metadata.ToRole, h.Metadata.ToPhase))
	sb.WriteString(fmt.Sprintf("- **Estado:** `%s`\n", h.Metadata.Status))
	sb.WriteString(fmt.Sprintf("- **Timestamp:** %s\n\n", ts.Format(time.RFC3339)))

	sb.WriteString("### 1. Resumen Ejecutivo\n")
	sb.WriteString(strings.TrimSpace(h.Sections.ExecutiveSummary) + "\n\n")

	sb.WriteString("### 2. Artefactos Involucrados\n")
	sb.WriteString(strings.TrimSpace(h.Sections.Artifacts) + "\n\n")

	sb.WriteString("### 3. Decisiones Técnicas\n")
	sb.WriteString(strings.TrimSpace(h.Sections.Decisions) + "\n\n")

	sb.WriteString("### 4. Riesgos y Bloqueos\n")
	sb.WriteString(strings.TrimSpace(h.Sections.RisksAndBlockers) + "\n\n")

	sb.WriteString("### 5. Instrucciones Directas para el Rol Receptor\n")
	sb.WriteString(strings.TrimSpace(h.Sections.DirectInstructions) + "\n")

	return EngramPayload{
		Title:         title,
		TopicKey:      topicKey,
		Type:          "architecture",
		CapturePrompt: false,
		Content:       sb.String(),
	}
}
