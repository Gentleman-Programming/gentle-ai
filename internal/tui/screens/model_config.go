package screens

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles"
)

// ModelConfigOptions returns the ordered list of options shown on the model config screen.
func ModelConfigOptions() []string {
	return []string{
		"Configurar modelos de Claude",
		"Configurar modelos de OpenCode",
		"Configurar modelos de Kiro",
		"Configurar modelos de Codex",
		"Volver",
	}
}

// RenderModelConfig renders the model configuration entry screen in Spanish.
// It shows a 4-option menu: Claude, OpenCode, Kiro, Codex, Volver.
// cursor indicates which option is currently highlighted.
func RenderModelConfig(cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Configuración de Modelos"))
	b.WriteString("\n\n")

	b.WriteString(styles.SubtextStyle.Render("Elige qué motor de IA deseas configurar:"))
	b.WriteString("\n\n")

	b.WriteString(renderOptions(ModelConfigOptions(), cursor))

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navegar • enter: seleccionar • esc: volver • q: salir"))

	return styles.FrameStyle.Render(b.String())
}
