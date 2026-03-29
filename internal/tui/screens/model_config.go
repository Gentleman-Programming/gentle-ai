package screens

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// ModelConfigOptions returns the ordered list of options shown on the model config screen.
func ModelConfigOptions() []string {
	return []string{
		"Configure Claude models",
		"Configure OpenCode models",
		"Back",
	}
}

// RenderModelConfig renders the model configuration entry screen.
// It shows a 3-option menu: Claude models, OpenCode models, Back.
// cursor indicates which option is currently highlighted.
func RenderModelConfig(cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Model Configuration"))
	b.WriteString("\n\n")

	b.WriteString(styles.SubtextStyle.Render("Choose which AI model to configure:"))
	b.WriteString("\n\n")

	b.WriteString(renderOptions(ModelConfigOptions(), cursor))

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back • q: quit"))

	return styles.FrameStyle.Render(b.String())
}
