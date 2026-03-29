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
	{ID: model.ComponentRTK, Name: "RTK", Description: "Rust Token Killer — CLI proxy that reduces LLM token consumption by 60-90%"},
	{ID: model.ComponentTheme, Name: "Theme", Description: "Gentleman Kanagawa theme overlay (future)"},
}

func MVPComponents() []Component {
	components := make([]Component, len(mvpComponents))
	copy(components, mvpComponents)
	return components
}
