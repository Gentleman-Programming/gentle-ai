package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/hub"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles"
)

// HubProjectsOptions construye las opciones para la pantalla de proyectos del Hub.
func HubProjectsOptions(projects []hub.WorkspaceRecord, activePath string) []string {
	options := make([]string, 0, len(projects)+3)
	for _, p := range projects {
		activeTag := ""
		if p.Path == activePath {
			activeTag = " [ACTIVO]"
		}
		label := fmt.Sprintf("• %s (%s)%s ── %s", p.Name, p.Topology, activeTag, p.Path)
		options = append(options, label)
	}
	options = append(options, "+ Vincular un proyecto existente al Hub")
	options = append(options, "⚡ Inicializar proyecto actual con Axiom (axiom init)")
	options = append(options, "Volver a gobernanza")
	return options
}

// RenderHubProjects renderiza la pantalla de gestión multi-proyecto de Axiom.
func RenderHubProjects(projects []hub.WorkspaceRecord, activePath string, cursor int, message string) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("📁 Proyectos del Hub Global"))
	b.WriteString("\n\n")

	b.WriteString(styles.SubtextStyle.Render("Espacios de trabajo registrados en ~/.axiom/workspaces.json:"))
	b.WriteString("\n\n")

	if message != "" {
		b.WriteString(styles.WarningStyle.Render(message))
		b.WriteString("\n\n")
	}

	if len(projects) == 0 {
		b.WriteString(styles.SubtextStyle.Render("No hay proyectos registrados en el Hub todavía."))
		b.WriteString("\n\n")
	}

	options := HubProjectsOptions(projects, activePath)
	b.WriteString(renderOptions(options, cursor))

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navegar • enter: seleccionar/conmutar • esc: volver"))

	return styles.FrameStyle.Render(b.String())
}
