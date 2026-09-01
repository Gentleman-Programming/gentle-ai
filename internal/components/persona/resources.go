package persona

import (
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// OutputStylePaths describes the output-style resources owned for one persona.
// Write is also the post-apply verification target. Backup may include a retired
// style because a switch removes it and must be recoverable.
type OutputStylePaths struct {
	Write  string
	Backup []string
	Remove []string
}

// OutputStyle describes the selected managed output style.
type OutputStyle struct {
	Name      string
	File      string
	AssetPath string
}

// ResourcePlan is the ephemeral managed-resource definition for one persona.
// It derives write, backup, verification, and removal paths without storing state.
type ResourcePlan struct {
	outputStyle *OutputStyle
	retired     []string
}

var managedOutputStyles = []OutputStyle{
	{Name: "Gentleman", File: "gentleman.md", AssetPath: "claude/output-style-gentleman.md"},
	{Name: "Neutral", File: "neutral.md", AssetPath: "claude/output-style-neutral.md"},
}

func canonicalPersona(persona model.PersonaID) model.PersonaID {
	if persona == model.PersonaGentlemanNeutralArtifacts {
		return model.PersonaNeutral
	}
	return persona
}

// ResourcePlanFor returns the managed resources selected by persona.
func ResourcePlanFor(persona model.PersonaID) ResourcePlan {
	persona = canonicalPersona(persona)
	switch {
	case isGentlemanConversationPersona(persona):
		return ResourcePlan{outputStyle: &managedOutputStyles[0]}
	case persona == model.PersonaNeutral:
		return ResourcePlan{outputStyle: &managedOutputStyles[1], retired: []string{managedOutputStyles[0].File}}
	default:
		return ResourcePlan{}
	}
}

// OutputStyle returns the selected output-style definition, if this persona owns one.
func (p ResourcePlan) OutputStyle() (OutputStyle, bool) {
	if p.outputStyle == nil {
		return OutputStyle{}, false
	}
	return *p.outputStyle, true
}

// OutputStylePaths derives all output-style paths for a delivery directory.
func (p ResourcePlan) OutputStylePaths(dir string) OutputStylePaths {
	if dir == "" || p.outputStyle == nil {
		return OutputStylePaths{}
	}

	paths := OutputStylePaths{
		Write:  filepath.Join(dir, p.outputStyle.File),
		Backup: make([]string, 0, len(managedOutputStyles)),
	}
	for _, style := range managedOutputStyles {
		paths.Backup = append(paths.Backup, filepath.Join(dir, style.File))
	}
	for _, file := range p.retired {
		paths.Remove = append(paths.Remove, filepath.Join(dir, file))
	}
	return paths
}
