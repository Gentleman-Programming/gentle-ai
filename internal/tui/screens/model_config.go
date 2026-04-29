package screens

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

const (
	ModelConfigOptionClaude   = "Configure Claude models"
	ModelConfigOptionOpenCode = "Configure OpenCode models"
	ModelConfigOptionKiro     = "Configure Kiro models"
	ModelConfigOptionPI       = "Configure PI models"
	ModelConfigOptionBack     = "Back"
)

// ModelConfigOptions returns the ordered list of options shown on the model config screen.
func ModelConfigOptions() []string {
	return ModelConfigOptionsForCapabilities(false)
}

// ModelConfigOptionsForCapabilities returns model config options with PI
// multi-model controls shown only when capability support is enabled.
func ModelConfigOptionsForCapabilities(piSupportsMultiModel bool) []string {
	options := []string{
		ModelConfigOptionClaude,
		ModelConfigOptionOpenCode,
		ModelConfigOptionKiro,
	}

	if piSupportsMultiModel {
		options = append(options, ModelConfigOptionPI)
	}

	options = append(options, ModelConfigOptionBack)

	return options
}

// RenderModelConfigWithCapabilities renders the model configuration entry
// screen with optional PI capability controls and warning message.
func RenderModelConfigWithCapabilities(cursor int, piSupportsMultiModel bool, warning string) string {
	options := ModelConfigOptionsForCapabilities(piSupportsMultiModel)

	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Model Configuration"))
	b.WriteString("\n\n")

	b.WriteString(styles.SubtextStyle.Render("Choose which AI model to configure:"))
	b.WriteString("\n")
	if warning != "" {
		b.WriteString("\n")
		b.WriteString(styles.WarningStyle.Render(warning))
	}
	b.WriteString("\n\n")

	b.WriteString(renderOptions(options, cursor))

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back • q: quit"))

	return styles.FrameStyle.Render(b.String())
}

// RenderModelConfig renders the model configuration entry screen.
// It shows a 4-option menu: Claude models, OpenCode models, Kiro models, Back.
// cursor indicates which option is currently highlighted.
func RenderModelConfig(cursor int) string {
	return RenderModelConfigWithCapabilities(cursor, false, "")
}
