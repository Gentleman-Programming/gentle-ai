package screens

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles"
)

// PiModelPickerState holds the state for the Pi subscription model picker screen.
type PiModelPickerState struct {
	ActivePreset model.PiSubscription
}

// NewPiModelPickerState creates a new state initialized with the active preset.
func NewPiModelPickerState(active model.PiSubscription) PiModelPickerState {
	if active == "" {
		active = model.PiPresetClaudeBalanced
	}
	return PiModelPickerState{ActivePreset: active}
}

// PiModelPickerOptionCount returns the number of selectable rows on the screen (presets + Back).
func PiModelPickerOptionCount() int {
	return len(model.PiSubscriptionsOrder()) + 1
}

// RenderPiModelPicker renders the single-screen subscription and tier preset selector for Pi.
func RenderPiModelPicker(state PiModelPickerState, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Configure Pi Agent Models"))
	b.WriteString("\n\n")

	b.WriteString(styles.SubtextStyle.Render("Choose your provider tier to assign models and reasoning effort across all 17+ subagents:"))
	b.WriteString("\n\n")

	presets := model.PiSubscriptionsOrder()
	var currentGroup string

	for i, sub := range presets {
		isCursor := i == cursor
		isActive := state.ActivePreset == sub

		// Group header
		group := getPresetGroup(sub)
		if group != currentGroup {
			currentGroup = group
			b.WriteString(styles.HeadingStyle.Render("── " + currentGroup + " ──"))
			b.WriteString("\n")
		}

		label := model.PiSubscriptionLabel(sub)
		if isActive {
			label += " (current)"
		}

		if isCursor {
			b.WriteString(styles.SelectedStyle.Render(styles.Cursor + label))
		} else {
			b.WriteString(styles.UnselectedStyle.Render("  " + label))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	backIdx := len(presets)
	if cursor == backIdx {
		b.WriteString(styles.SelectedStyle.Render(styles.Cursor + "Back"))
	} else {
		b.WriteString(styles.UnselectedStyle.Render("  Back"))
	}
	b.WriteString("\n\n")

	// Detail preview box for the highlighted preset
	if cursor >= 0 && cursor < len(presets) {
		highlighted := presets[cursor]
		mapping := model.PiPresetForSubscription(highlighted)
		desc := model.PiSubscriptionDescription(highlighted)

		b.WriteString(styles.HeadingStyle.Render("Configuration Preview:"))
		b.WriteString("\n")
		b.WriteString(styles.HelpStyle.Render("  Summary:    " + desc))
		b.WriteString("\n")

		reasoning := mapping["sdd-design"]
		code := mapping["sdd-apply"]
		light := mapping["sdd-archive"]

		b.WriteString(styles.SubtextStyle.Render(
			"  Reasoning:  " + reasoning.Model + " (thinking: " + formatEffort(reasoning.Thinking) + ")",
		))
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render(
			"  Execution:  " + code.Model + " (thinking: " + formatEffort(code.Thinking) + ")",
		))
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render(
			"  Light work: " + light.Model + " (thinking: " + formatEffort(light.Thinking) + ")",
		))
		b.WriteString("\n\n")
	}

	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select preset & sync • esc: back"))

	return styles.FrameStyle.Render(b.String())
}

// HandlePiModelPickerNav handles keyboard input on the Pi model picker screen.
// Returns handled=true and the selected PiSubscription when a preset is chosen.
func HandlePiModelPickerNav(key string, state *PiModelPickerState, cursor int) (bool, model.PiSubscription) {
	presets := model.PiSubscriptionsOrder()
	if key == "enter" {
		if cursor >= 0 && cursor < len(presets) {
			chosen := presets[cursor]
			state.ActivePreset = chosen
			return true, chosen
		}
	}
	return false, ""
}

func getPresetGroup(sub model.PiSubscription) string {
	s := string(sub)
	switch {
	case strings.HasPrefix(s, "claude"):
		return "Anthropic via API key"
	case strings.HasPrefix(s, "codex"):
		return "Codex (OpenAI)"
	case strings.HasPrefix(s, "kiro"):
		return "Kiro (Frontier)"
	case strings.HasPrefix(s, "budget"):
		return "Budget / Open Source"
	default:
		return "Other"
	}
}

func formatEffort(effort string) string {
	if effort == "" {
		return "default"
	}
	return effort
}
