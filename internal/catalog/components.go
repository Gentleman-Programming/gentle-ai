package catalog

import "github.com/gentleman-programming/gentle-ai/internal/model"

type Component struct {
	ID          model.ComponentID
	Name        string
	Description string
}

var mvpComponents = []Component{
	{ID: model.ComponentEngram, Name: "Engram", Description: "Persistent cross-session memory"},
	{ID: model.ComponentSDD, Name: "SDD", Description: "Spec-driven development workflow"},
	{ID: model.ComponentSkills, Name: "Skills", Description: "Curated coding skill library"},
	{ID: model.ComponentContext7, Name: "Context7", Description: "Latest framework and library docs"},
	{ID: model.ComponentPersona, Name: "Persona", Description: "Gentleman, neutral or custom behavior"},
	{ID: model.ComponentPermission, Name: "Permissions", Description: "Security-first defaults and guardrails"},
	{ID: model.ComponentGGA, Name: "GGA", Description: "Gentleman Guardian Angel — AI provider switcher"},
	{ID: model.ComponentTheme, Name: "Theme", Description: "Gentleman Kanagawa theme overlay"},
	{ID: model.ComponentClaudeTheme, Name: "Claude Gentleman Theme", Description: "Claude Code Gentleman custom theme"},
	{ID: model.ComponentOpenCodeGentleLogo, Name: "OpenCode Gentle Logo", Description: "OpenCode home logo TUI plugin with Braille rose"},
}

func MVPComponents() []Component {
	components := make([]Component, len(mvpComponents))
	copy(components, mvpComponents)
	return components
}
