package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/livingdoc"
	"github.com/gentleman-programming/gentle-ai/v3/internal/tui/styles"
)

// LivingDocOptions construye las opciones para la pantalla de especificaciones vivas.
func LivingDocOptions(specs []livingdoc.LivingSpecEntry) []string {
	options := make([]string, 0, len(specs)+2)
	for _, s := range specs {
		title := s.Title
		if title == "" {
			title = s.Domain
		}
		label := fmt.Sprintf("• %s (%s) ── %d requerimientos, %d escenarios",
			title, s.Domain, len(s.Requirements), s.TotalScenarios)
		options = append(options, label)
	}
	options = append(options, "↻ Sincronizar catálogo maestro (openspec/INDEX.md)")
	options = append(options, "Volver a gobernanza")
	return options
}

// RenderLivingDoc renderiza la pantalla de especificaciones vivas en la TUI.
func RenderLivingDoc(specs []livingdoc.LivingSpecEntry, cursor int, message string) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("📚 Catálogo de Especificaciones Vivas"))
	b.WriteString("\n\n")

	b.WriteString(styles.SubtextStyle.Render("Especificaciones consolidadas en openspec/specs/ y sincronizadas en INDEX.md:"))
	b.WriteString("\n\n")

	if message != "" {
		b.WriteString(styles.SuccessStyle.Render(message))
		b.WriteString("\n\n")
	}

	if len(specs) == 0 {
		b.WriteString(styles.SubtextStyle.Render("No se encontraron especificaciones consolidadas aún en openspec/specs/."))
		b.WriteString("\n\n")
	}

	options := LivingDocOptions(specs)
	b.WriteString(renderOptions(options, cursor))

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navegar • enter: seleccionar • s: sincronizar • esc: volver"))

	return styles.FrameStyle.Render(b.String())
}
