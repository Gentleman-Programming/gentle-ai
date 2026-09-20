package screens

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/tui/styles"
)

// GovernanceOptions retorna la lista ordenada de opciones del submenú de gobernanza.
func GovernanceOptions() []string {
	return []string{
		"1. Proyectos del Hub (conmutar, registrar, inicializar)",
		"2. Ciclo de Vida de Incrementos SDD (ver fases, crear, avanzar)",
		"3. Monitor Multi-Rol y Barrera Fan-In (inspeccionar, migrar diferidas)",
		"4. Visor de Handoffs Estructurados (relevos formales)",
		"5. Catálogo de Especificaciones Vivas (specs e INDEX.md)",
		"Volver al menú principal",
	}
}

// RenderGovernance renderiza la pantalla del submenú de gobernanza SDD y multi-proyecto.
func RenderGovernance(cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("📁 Proyectos y Gobernanza SDD"))
	b.WriteString("\n\n")

	b.WriteString(styles.SubtextStyle.Render("Selecciona el módulo de gobernanza o gestión sobre el que deseas operar:"))
	b.WriteString("\n\n")

	b.WriteString(renderOptions(GovernanceOptions(), cursor))

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navegar • enter: seleccionar • esc: volver • q: salir"))

	return styles.FrameStyle.Render(b.String())
}
