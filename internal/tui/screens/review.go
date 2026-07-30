package screens

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/planner"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles"
)

func ReviewOptions() []string {
	return []string{"Install", "Back"}
}

func RenderReview(payload planner.ReviewPayload, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Review and Confirm"))
	b.WriteString("\n\n")

	b.WriteString("  " + styles.HeadingStyle.Render("Agents") + "  " + styles.UnselectedStyle.Render(joinIDs(payload.Agents)) + "\n")
	b.WriteString("  " + styles.HeadingStyle.Render("Persona") + "  " + styles.UnselectedStyle.Render(reviewPersonaLabel(payload.Persona)) + "\n")
	b.WriteString("  " + styles.HeadingStyle.Render("Preset") + "  " + styles.UnselectedStyle.Render(reviewPresetLabel(payload.Preset)) + "\n")
	b.WriteString("\n")

	if len(payload.Components) > 0 {
		autoSet := make(map[model.ComponentID]struct{}, len(payload.AddedDependencies))
		for _, dep := range payload.AddedDependencies {
			autoSet[dep] = struct{}{}
		}

		b.WriteString(styles.HeadingStyle.Render("Components"))
		b.WriteString("\n")
		for _, comp := range payload.Components {
			badge := styles.SubtextStyle.Render("selected")
			if _, isAuto := autoSet[comp.ID]; isAuto {
				badge = styles.WarningStyle.Render("auto-dependency")
			}
			b.WriteString("  " + styles.UnselectedStyle.Render(string(comp.ID)) + " " + badge + "\n")
		}

		// Issue #145: show individual skill names when the Skills component is selected.
		if len(payload.Skills) > 0 {
			b.WriteString(styles.HeadingStyle.Render("  Skills"))
			b.WriteString("\n")
			for _, skill := range payload.Skills {
				b.WriteString("    " + styles.SubtextStyle.Render(string(skill)) + "\n")
			}
		}

		// Issue #149: show Strict TDD status when SDD is in the plan.
		if payload.HasSDD {
			strictLabel := "Disabled"
			if payload.StrictTDD {
				strictLabel = "Enabled"
			}
			b.WriteString("  " + styles.HeadingStyle.Render("Strict TDD") + "  " + styles.UnselectedStyle.Render(strictLabel) + "\n")
		}

		b.WriteString("\n")
	}

	if len(payload.UnsupportedAgents) > 0 {
		b.WriteString(styles.WarningStyle.Render("Unsupported agents: " + joinIDs(payload.UnsupportedAgents)))
		b.WriteString("\n\n")
	}

	b.WriteString(renderOptions(ReviewOptions(), cursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("enter: install • esc: back"))

	return b.String()
}

func joinIDs[T ~string](values []T) string {
	if len(values) == 0 {
		return "none"
	}

	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, string(value))
	}

	return strings.Join(parts, ", ")
}

func reviewPersonaLabel(persona model.PersonaID) string {
	if persona == model.PersonaCustom {
		return "keep existing persona unmanaged"
	}
	// Keep the persona ID visible: this is the confirm-before-write screen, so
	// the reader must be able to see the exact value that lands in state.json,
	// not only its prose description. The two Gentleman variants differ solely
	// by the "(legacy alias)" suffix, which is too easy to miss on its own.
	if description, ok := personaDescriptions[persona]; ok {
		return string(persona) + " — " + description
	}
	return string(persona)
}

func reviewPresetLabel(preset model.PresetID) string {
	if preset == model.PresetCustom {
		return "choose components and skills manually"
	}

	return string(preset)
}
