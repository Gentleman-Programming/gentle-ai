package catalog

import "github.com/gentleman-programming/gentle-ai/v2/internal/model"

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
	{ID: model.ComponentPersona, Name: "Persona", Description: "Managed agent behavior and conversation tone"},
	{ID: model.ComponentPermission, Name: "Permissions", Description: "Security-first defaults and guardrails"},
	{ID: model.ComponentGGA, Name: "GGA", Description: "Gentleman Guardian Angel — AI provider switcher"},
	{ID: model.ComponentTheme, Name: "OpenCode Theme", Description: "Visual polish: OpenCode color theme"},
	{ID: model.ComponentClaudeTheme, Name: "Gentleman Visual Themes", Description: "Visual polish: Gentleman and Gentleman Cute theme assets"},
	{ID: model.ComponentOpenCodeGentleLogo, Name: "OpenCode Logo", Description: "Visual polish: OpenCode home logo plugin"},
}

func MVPComponents() []Component {
	components := make([]Component, len(mvpComponents))
	copy(components, mvpComponents)
	return components
}
