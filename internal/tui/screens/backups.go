package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v3/internal/tui/styles"
)

// BackupMaxVisible is the maximum number of backup items shown at once.
// Exported so model.go can compute scroll adjustments.
const BackupMaxVisible = 10

// RenderBackups renders the backup selection screen with scroll support.
// It uses manifest.DisplayLabel() to show source + timestamp for each backup.
// pinErr, when non-nil, is shown as an inline error message below the list.
func RenderBackups(backups []backup.Manifest, cursor int, scrollOffset int, pinErr error) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Gestión de Respaldos"))
	b.WriteString("\n\n")

	if len(backups) == 0 {
		b.WriteString(styles.WarningStyle.Render("No se encontraron respaldos todavía."))
		b.WriteString("\n\n")
		b.WriteString(renderOptions([]string{"Volver"}, 0))
		return b.String()
	}
	if !hasAxiomBackup(backups) {
		b.WriteString(styles.WarningStyle.Render("No hay respaldos de Axiom. Los respaldos históricos de Gentle AI no se seleccionan automáticamente; elige uno expresamente si quieres restaurarlo."))
		b.WriteString("\n\n")
	}

	end := scrollOffset + BackupMaxVisible
	if end > len(backups) {
		end = len(backups)
	}

	if scrollOffset > 0 {
		b.WriteString(styles.SubtextStyle.Render("  ↑ más"))
		b.WriteString("\n")
	}

	for i := scrollOffset; i < end; i++ {
		snapshot := backups[i]
		// Use DisplayLabel for richer labels: "install — 2026-03-22 15:04 (5 files)"
		// Falls back to "unknown source — 2026-03-22 15:04" for old manifests.
		displayLabel := fmt.Sprintf("%s  [%s]", snapshot.DisplayLabel(), snapshot.Origin.Label())
		if snapshot.CreatedByVersion != "" {
			displayLabel = fmt.Sprintf("%s  [v%s]", displayLabel, snapshot.CreatedByVersion)
		}
		if snapshot.Description != "" {
			displayLabel = fmt.Sprintf("%s  — %s", displayLabel, snapshot.Description)
		}
		label := fmt.Sprintf("%s  (%s)", snapshot.ID, displayLabel)
		focused := i == cursor
		if focused {
			b.WriteString(styles.SelectedStyle.Render(styles.Cursor + label))
		} else {
			b.WriteString(styles.UnselectedStyle.Render("  " + label))
		}
		b.WriteString("\n")
	}

	if end < len(backups) {
		b.WriteString(styles.SubtextStyle.Render("  ↓ más"))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(renderOptions([]string{"Volver"}, cursor-len(backups)))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navegar • enter: restaurar • r: renombrar • d: eliminar • p: fijar/desfijar • esc: volver"))

	if pinErr != nil {
		b.WriteString("\n")
		b.WriteString(styles.ErrorStyle.Render("error al fijar: " + pinErr.Error()))
	}

	return b.String()
}

// RenderRestoreConfirm renders the restore confirmation screen.
// It shows the backup identity and asks the user to confirm or cancel.
// Cursor 0 = "Restaurar", Cursor 1 = "Cancelar".
func RenderRestoreConfirm(manifest backup.Manifest, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Restaurar Respaldo"))
	b.WriteString("\n\n")

	b.WriteString(styles.HeadingStyle.Render("Respaldo: "))
	b.WriteString(styles.SelectedStyle.Render(manifest.ID))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render(fmt.Sprintf("%s  [%s]", manifest.DisplayLabel(), manifest.Origin.Label())))
	b.WriteString("\n\n")

	b.WriteString(styles.WarningStyle.Render("Esto sobrescribirá tu configuración actual."))
	b.WriteString("\n\n")

	b.WriteString(renderOptions([]string{"Restaurar", "Cancelar"}, cursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navegar • enter: seleccionar • esc: volver"))

	return b.String()
}

func hasAxiomBackup(backups []backup.Manifest) bool {
	for _, manifest := range backups {
		if manifest.Origin == backup.BackupOriginAxiom {
			return true
		}
	}
	return false
}

// RenderRestoreResult renders the restore result screen.
// Shows a success message when err is nil, or an error message with details.
func RenderRestoreResult(manifest backup.Manifest, err error) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Resultado de la Restauración"))
	b.WriteString("\n\n")

	if err == nil {
		b.WriteString(styles.SuccessStyle.Render("✓ Restauración completada con éxito"))
		b.WriteString("\n\n")
		b.WriteString(styles.SubtextStyle.Render("Restaurado: "))
		b.WriteString(styles.SelectedStyle.Render(manifest.ID))
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render(manifest.DisplayLabel()))
		b.WriteString("\n\n")
		b.WriteString(styles.UnselectedStyle.Render("Tu configuración ha sido restaurada a partir de este respaldo."))
	} else {
		b.WriteString(styles.ErrorStyle.Render("✗ Fallo en la restauración"))
		b.WriteString("\n\n")
		b.WriteString(styles.SubtextStyle.Render("Respaldo: "))
		b.WriteString(styles.SelectedStyle.Render(manifest.ID))
		b.WriteString("\n\n")
		b.WriteString(styles.HeadingStyle.Render("Error:"))
		b.WriteString("\n")
		b.WriteString(styles.ErrorStyle.Render("  " + err.Error()))
		b.WriteString("\n\n")
		b.WriteString(styles.SubtextStyle.Render("Tus archivos no fueron modificados."))
	}

	b.WriteString("\n\n")
	b.WriteString(styles.HelpStyle.Render("enter: volver a respaldos • esc: volver"))

	return b.String()
}

// RenderDeleteConfirm renders the delete confirmation screen.
// Shows backup info and asks the user to confirm or cancel the deletion.
// Cursor 0 = "Eliminar", Cursor 1 = "Cancelar".
func RenderDeleteConfirm(manifest backup.Manifest, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Eliminar Respaldo"))
	b.WriteString("\n\n")

	b.WriteString(styles.HeadingStyle.Render("Respaldo: "))
	b.WriteString(styles.SelectedStyle.Render(manifest.ID))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render(manifest.DisplayLabel()))
	b.WriteString("\n\n")

	b.WriteString(styles.WarningStyle.Render("¿Seguro que deseas eliminar permanentemente este respaldo?"))
	b.WriteString("\n")
	b.WriteString(styles.WarningStyle.Render("Esta acción no se puede deshacer."))
	b.WriteString("\n\n")

	b.WriteString(renderOptions([]string{"Eliminar", "Cancelar"}, cursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navegar • enter: seleccionar • esc: volver"))

	return b.String()
}

// RenderDeleteResult renders the delete result screen.
// Shows a success message when err is nil, or an error message with details.
func RenderDeleteResult(manifest backup.Manifest, err error) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Resultado de la Eliminación"))
	b.WriteString("\n\n")

	if err == nil {
		b.WriteString(styles.SuccessStyle.Render("✓ Respaldo eliminado con éxito"))
		b.WriteString("\n\n")
		b.WriteString(styles.SubtextStyle.Render("Eliminado: "))
		b.WriteString(styles.SelectedStyle.Render(manifest.ID))
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render(manifest.DisplayLabel()))
		b.WriteString("\n\n")
		b.WriteString(styles.UnselectedStyle.Render("El respaldo ha sido eliminado permanentemente."))
	} else {
		b.WriteString(styles.ErrorStyle.Render("✗ Fallo al eliminar"))
		b.WriteString("\n\n")
		b.WriteString(styles.SubtextStyle.Render("Respaldo: "))
		b.WriteString(styles.SelectedStyle.Render(manifest.ID))
		b.WriteString("\n\n")
		b.WriteString(styles.HeadingStyle.Render("Error:"))
		b.WriteString("\n")
		b.WriteString(styles.ErrorStyle.Render("  " + err.Error()))
		b.WriteString("\n\n")
		b.WriteString(styles.SubtextStyle.Render("El directorio del respaldo aún podría existir."))
	}

	b.WriteString("\n\n")
	b.WriteString(styles.HelpStyle.Render("enter: volver a respaldos • esc: volver"))

	return b.String()
}

// RenderRenameBackup renders the rename backup screen with a text input field.
// Shows current description and a text field for the new description.
func RenderRenameBackup(manifest backup.Manifest, inputText string, cursorPos int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Renombrar Respaldo"))
	b.WriteString("\n\n")

	b.WriteString(styles.HeadingStyle.Render("Respaldo: "))
	b.WriteString(styles.SelectedStyle.Render(manifest.ID))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render(manifest.DisplayLabel()))
	b.WriteString("\n\n")

	if manifest.Description != "" {
		b.WriteString(styles.SubtextStyle.Render("Descripción actual: "))
		b.WriteString(styles.UnselectedStyle.Render(manifest.Description))
		b.WriteString("\n\n")
	}

	b.WriteString(styles.HeadingStyle.Render("Nueva descripción:"))
	b.WriteString("\n")

	// Render text input with cursor indicator.
	runes := []rune(inputText)
	var inputDisplay strings.Builder
	for i, r := range runes {
		if i == cursorPos {
			inputDisplay.WriteString(styles.SelectedStyle.Render("|"))
		}
		inputDisplay.WriteRune(r)
	}
	if cursorPos == len(runes) {
		inputDisplay.WriteString(styles.SelectedStyle.Render("|"))
	}

	b.WriteString(styles.UnselectedStyle.Render("  > "))
	b.WriteString(inputDisplay.String())
	b.WriteString("\n\n")

	b.WriteString(styles.HelpStyle.Render("enter: guardar • esc: cancelar • ←/→: mover cursor • retroceso: borrar"))

	return b.String()
}
