package screens

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
	"github.com/gentleman-programming/gentle-ai/v3/internal/tui/styles"
)

func DetectionOptions() []string {
	return []string{"Continuar", "Volver"}
}

func RenderDetection(result system.DetectionResult, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Detección del Sistema"))
	b.WriteString("\n\n")

	supportedText := styles.ErrorStyle.Render("No")
	if result.System.Supported {
		supportedText = styles.SuccessStyle.Render("Sí")
	}

	shellName := filepath.Base(result.System.Shell)

	b.WriteString(fmt.Sprintf("  %s  %s\n", styles.HeadingStyle.Render("SO"), styles.UnselectedStyle.Render(fmt.Sprintf("%s (%s)", result.System.OS, result.System.Arch))))
	b.WriteString(fmt.Sprintf("  %s  %s\n", styles.HeadingStyle.Render("Intérprete"), styles.UnselectedStyle.Render(shellName)))
	b.WriteString(fmt.Sprintf("  %s  %s\n", styles.HeadingStyle.Render("Compatible"), supportedText))
	b.WriteString("\n")

	if len(result.Tools) > 0 {
		b.WriteString(styles.HeadingStyle.Render("Herramientas"))
		b.WriteString("\n")
		keys := make([]string, 0, len(result.Tools))
		for key := range result.Tools {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			status := result.Tools[key]
			indicator := styles.ErrorStyle.Render("no encontrada")
			if status.Installed {
				indicator = styles.SuccessStyle.Render("encontrada")
			}
			b.WriteString(fmt.Sprintf("  %s: %s\n", styles.UnselectedStyle.Render(key), indicator))
		}
		b.WriteString("\n")
	}

	if len(result.Dependencies.Dependencies) > 0 {
		b.WriteString(styles.HeadingStyle.Render("Dependencias"))
		b.WriteString("\n")
		for _, dep := range result.Dependencies.Dependencies {
			var indicator string
			if dep.Installed {
				version := dep.Version
				if version == "" {
					version = "encontrada"
				}
				indicator = styles.SuccessStyle.Render(version)
			} else {
				label := "no encontrada"
				if dep.Required {
					label = "NO ENCONTRADA (requerida)"
				}
				indicator = styles.ErrorStyle.Render(label)
			}

			suffix := ""
			if !dep.Required {
				suffix = styles.SubtextStyle.Render(" (opcional)")
			}

			b.WriteString(fmt.Sprintf("  %s: %s%s\n",
				styles.UnselectedStyle.Render(dep.Name), indicator, suffix))
		}

		if len(result.Dependencies.MissingRequired) > 0 {
			b.WriteString("\n")
			b.WriteString(styles.WarningStyle.Render(
				fmt.Sprintf("Requeridas ausentes: %s",
					strings.Join(result.Dependencies.MissingRequired, ", "))))
			b.WriteString("\n")
		}

		b.WriteString("\n")
	}

	if len(result.Configs) > 0 {
		b.WriteString(styles.HeadingStyle.Render("Configuraciones Detectadas"))
		b.WriteString("\n")
		for _, config := range result.Configs {
			indicator := styles.ErrorStyle.Render("ausente")
			if config.Exists {
				indicator = styles.SuccessStyle.Render("presente")
			}
			b.WriteString(fmt.Sprintf("  %s: %s\n", styles.UnselectedStyle.Render(config.Agent), indicator))
		}
		b.WriteString("\n")
	}

	b.WriteString(renderOptions(DetectionOptions(), cursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navegar • enter: seleccionar • esc: volver"))

	return b.String()
}
