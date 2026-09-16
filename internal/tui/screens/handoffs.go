package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/handoff"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles"
)

// RenderHandoffs renderiza la pantalla del visor de relevos estructurados en la TUI.
func RenderHandoffs(ho *handoff.Handoff, changeName string, errMsg string) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("🤝 Visor de Handoffs Estructurados"))
	b.WriteString("\n\n")

	if errMsg != "" {
		b.WriteString(styles.WarningStyle.Render(errMsg))
		b.WriteString("\n\n")
	}

	if ho == nil {
		b.WriteString(styles.SubtextStyle.Render(fmt.Sprintf("No se encontró un artefacto handoff.md para el cambio '%s'.", changeName)))
		b.WriteString("\n\n")
		b.WriteString(styles.HelpStyle.Render("esc: volver"))
		return styles.FrameStyle.Render(b.String())
	}

	flow := fmt.Sprintf("[%s] ➔ [%s]", strings.ToUpper(string(ho.Metadata.FromPhase)), strings.ToUpper(string(ho.Metadata.ToPhase)))
	roles := fmt.Sprintf("Roles: %s ➔ %s", ho.Metadata.FromRole, ho.Metadata.ToRole)
	status := fmt.Sprintf("Estado: %s | Fecha: %s", strings.ToUpper(string(ho.Metadata.Status)), ho.Metadata.Timestamp.Format("2006-01-02 15:04"))

	b.WriteString(styles.HeadingStyle.Render(flow))
	b.WriteString("  •  ")
	b.WriteString(styles.SubtextStyle.Render(roles))
	b.WriteString("  •  ")
	b.WriteString(styles.SubtextStyle.Render(status))
	b.WriteString("\n\n")

	if ho.Sections.ExecutiveSummary != "" {
		b.WriteString(styles.HeadingStyle.Render("# 1. Resumen Ejecutivo:"))
		b.WriteString("\n")
		b.WriteString(ho.Sections.ExecutiveSummary)
		b.WriteString("\n\n")
	}

	if ho.Sections.Decisions != "" {
		b.WriteString(styles.HeadingStyle.Render("# 3. Decisiones Clave:"))
		b.WriteString("\n")
		b.WriteString(ho.Sections.Decisions)
		b.WriteString("\n\n")
	}

	if ho.Sections.DirectInstructions != "" {
		b.WriteString(styles.HeadingStyle.Render("# 5. Instrucciones Directas:"))
		b.WriteString("\n")
		b.WriteString(ho.Sections.DirectInstructions)
		b.WriteString("\n\n")
	}

	b.WriteString(styles.HelpStyle.Render("esc: volver a gobernanza"))
	return styles.FrameStyle.Render(b.String())
}
