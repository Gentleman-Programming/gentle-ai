package screens

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles"
)

// ModelConfigOptions returns the ordered list of options shown on the model config screen.
func ModelConfigOptions(piAvailable ...bool) []string {
	options := []string{"Configure Claude models", "Configure OpenCode models", "Configure Kiro models", "Configure Codex models"}
	if len(piAvailable) > 0 && piAvailable[0] {
		options = append(options, "Configure Pi models")
	}
	return append(options, "Back")
}

// RenderModelConfig renders the model configuration entry screen.
func RenderModelConfig(cursor int, piAvailable ...bool) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Model Configuration"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Choose which AI model to configure:"))
	b.WriteString("\n\n")
	b.WriteString(renderOptions(ModelConfigOptions(piAvailable...), cursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back • q: quit"))

	return styles.FrameStyle.Render(b.String())
}
