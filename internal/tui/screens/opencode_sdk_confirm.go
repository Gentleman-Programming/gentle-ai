package screens

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/tui/styles"
)

// RenderOpenCodeSDKConfirm asks for a package mutation separately from Review.
func RenderOpenCodeSDKConfirm(dependency, manager, configDir string, cursor int) string {
	var b strings.Builder
	b.WriteString(styles.TitleStyle.Render("OpenCode SDK installation"))
	b.WriteString("\n\n")
	b.WriteString("Package: " + dependency + "\n")
	b.WriteString("Package manager for fresh installation: " + manager + "\n")
	b.WriteString("OpenCode config directory: " + configDir + "\n\n")
	b.WriteString("Only a fresh config without a manifest, lockfile or project manager config is eligible.\n")
	b.WriteString("This runs " + manager + " with network access and creates package.json, its lockfile and node_modules.\n")
	b.WriteString("Automatic install uses registry.npmjs.org without your package-manager credentials and ignores install scripts.\n")
	b.WriteString("Package-manager changes are NOT covered by Gentle AI rollback.\n\n")
	b.WriteString(renderOptions([]string{"Install SDK for this invocation", "No / Back"}, cursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))
	return b.String()
}
