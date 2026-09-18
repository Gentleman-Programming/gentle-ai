package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles"
)

// SDDIncrementInfo modela la información de un incremento para la TUI.
type SDDIncrementInfo struct {
	Name           string
	Type           string // "active" o "archived"
	Phase          string
	TasksTotal     int
	TasksCompleted int
	ProgressPct    int
}

// SDDIncrementsOptions construye la lista de opciones para el ciclo de vida de incrementos.
func SDDIncrementsOptions(increments []SDDIncrementInfo) []string {
	options := make([]string, 0, len(increments)+4)
	for _, inc := range increments {
		prefix := "▶"
		if inc.Type == "archived" {
			prefix = "✓"
		}
		label := fmt.Sprintf("%s %s [%s] (%d/%d tareas, %d%%)",
			prefix, inc.Name, strings.ToUpper(inc.Phase), inc.TasksCompleted, inc.TasksTotal, inc.ProgressPct)
		options = append(options, label)
	}
	options = append(options, "+ Crear nuevo incremento SDD")
	options = append(options, "▶ Avanzar fase del cambio seleccionado (sdd continue)")
	options = append(options, "✓ Validar reporte de verificación (sdd verify-validate)")
	options = append(options, "Volver a gobernanza")
	return options
}

// RenderSDDIncrements renderiza la pantalla del tablero de incrementos en la TUI.
func RenderSDDIncrements(increments []SDDIncrementInfo, cursor int, message string) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("📋 Ciclo de Vida de Incrementos SDD"))
	b.WriteString("\n\n")

	b.WriteString(styles.SubtextStyle.Render("Seguimiento de cambios activos y especificaciones archivadas en openspec/:"))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render("Carril activo: SDD · esc → Gobernanza → «6» para el carril ágil (ODD)"))
	b.WriteString("\n\n")

	if message != "" {
		b.WriteString(styles.WarningStyle.Render(message))
		b.WriteString("\n\n")
	}

	if len(increments) == 0 {
		b.WriteString(styles.SubtextStyle.Render("No se encontraron incrementos activos en openspec/changes/."))
		b.WriteString("\n\n")
	}

	options := SDDIncrementsOptions(increments)
	b.WriteString(renderOptions(options, cursor))

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navegar • enter: seleccionar/ejecutar • esc: volver"))

	return styles.FrameStyle.Render(b.String())
}
