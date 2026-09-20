package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/multirole"
	"github.com/gentleman-programming/gentle-ai/v3/internal/tui/styles"
)

// MultiRoleInfo describe la información de un rol para la TUI.
type MultiRoleInfo struct {
	ID         string
	Name       string
	GatePolicy string
}

// MultiRoleOptions genera las opciones para la pantalla de monitor multi-rol.
func MultiRoleOptions(roles []MultiRoleInfo) []string {
	options := make([]string, 0, len(roles)+2)
	for _, r := range roles {
		policy := r.GatePolicy
		if policy == "" {
			policy = "blocking"
		}
		label := fmt.Sprintf("• [%s] %s ── Compuerta: %s", r.ID, r.Name, strings.ToUpper(policy))
		options = append(options, label)
	}
	options = append(options, "⚡ Migrar tareas diferidas pendientes (Fan-In acumulativo)")
	options = append(options, "Volver a gobernanza")
	return options
}

// RenderMultiRole renderiza el monitor de concurrencia y compuertas en la TUI.
func RenderMultiRole(changeName string, roles []MultiRoleInfo, barrier *multirole.BarrierReport, cursor int, message string) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("👥 Monitor Multi-Rol y Barrera Fan-In"))
	b.WriteString("\n\n")

	if changeName != "" {
		b.WriteString(styles.SubtextStyle.Render(fmt.Sprintf("Inspeccionando cambio activo: %s", changeName)))
		b.WriteString("\n\n")
	}

	if barrier != nil {
		if barrier.Satisfied {
			b.WriteString(styles.SuccessStyle.Render("✓ Barrera Fan-In ABIERTA: Todos los roles obligatorios completaron su verificación."))
		} else {
			reason := "Requisitos de rol pendientes"
			if len(barrier.Blockers) > 0 {
				reason = strings.Join(barrier.Blockers, "; ")
			}
			b.WriteString(styles.WarningStyle.Render(fmt.Sprintf("⚠ Barrera Fan-In BLOQUEADA: %s", reason)))
		}
		b.WriteString("\n\n")
	}

	if message != "" {
		b.WriteString(styles.WarningStyle.Render(message))
		b.WriteString("\n\n")
	}

	if len(roles) == 0 {
		b.WriteString(styles.SubtextStyle.Render("No se encontraron roles declarados en axiom.yaml o design.md."))
		b.WriteString("\n\n")
	}

	options := MultiRoleOptions(roles)
	b.WriteString(renderOptions(options, cursor))

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navegar • enter: seleccionar • esc: volver"))

	return styles.FrameStyle.Render(b.String())
}
