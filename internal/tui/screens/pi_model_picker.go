package screens

import (
	"fmt"
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
		active = model.PiSubscriptionClaude
	}
	return PiModelPickerState{ActivePreset: active}
}

// PiModelPickerOptionCount returns the number of selectable rows on the screen (presets + Back).
func PiModelPickerOptionCount() int {
	return len(model.PiSubscriptionsOrder()) + 1
}

// RenderPiModelPicker renders the subscription preset selection screen for Pi subagents.
func RenderPiModelPicker(state PiModelPickerState, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Configure Pi Agent Models"))
	b.WriteString("\n\n")

	b.WriteString(styles.SubtextStyle.Render("Select your active AI subscription to assign optimal models to all 17+ Pi subagents:"))
	b.WriteString("\n\n")

	presets := model.PiSubscriptionsOrder()
	for i, sub := range presets {
		isCursor := i == cursor
		isActive := state.ActivePreset == sub

		label := formatSubscriptionLabel(sub)
		if isActive {
			label += " (current)"
		}

		if isCursor {
			b.WriteString(styles.SelectedStyle.Render(styles.Cursor + label))
		} else {
			b.WriteString(styles.UnselectedStyle.Render("  " + label))
		}
		b.WriteString("\n")

		desc := model.PiSubscriptionDescription(sub)
		if desc != "" {
			b.WriteString(styles.HelpStyle.Render("    " + desc))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	backIdx := len(presets)
	if cursor == backIdx {
		b.WriteString(styles.SelectedStyle.Render(styles.Cursor + "Back"))
	} else {
		b.WriteString(styles.UnselectedStyle.Render("  Back"))
	}
	b.WriteString("\n\n")

	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select preset • esc: back"))

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

func formatSubscriptionLabel(sub model.PiSubscription) string {
	switch sub {
	case model.PiSubscriptionClaude:
		return "Claude Subscription (Anthropic Opus / Sonnet / Haiku)"
	case model.PiSubscriptionCodex:
		return "Codex Subscription (OpenAI GPT-5.6 Sol / Terra / Luna)"
	case model.PiSubscriptionKiro:
		return "Kiro Subscription (Frontier Claude 4.8 / 4.6 / 4.5)"
	case model.PiSubscriptionBudget:
		return "Budget / Open Source (DeepSeek Reasoner & Chat)"
	default:
		return fmt.Sprintf("%s Subscription", sub)
	}
}
