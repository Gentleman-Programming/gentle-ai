package planner

import (
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

type Resolver interface {
	Resolve(selection model.Selection) (ResolvedPlan, error)
}

type ResolvedPlan struct {
	Agents            []model.AgentID
	UnsupportedAgents []model.AgentID
	OrderedComponents []model.ComponentID
	AddedDependencies []model.ComponentID
	PlatformDecision  PlatformDecision
}

type ReviewPayload struct {
	Agents            []model.AgentID
	UnsupportedAgents []model.AgentID
	Persona           model.PersonaID
	Preset            model.PresetID
	Components        []ComponentAction
	AddedDependencies []model.ComponentID
	PlatformDecision  PlatformDecision

	// Skills holds the individual skill IDs selected by the user (Issue #145).
	// Only populated when the Skills component is selected.
	Skills []model.SkillID

	// StrictTDD reflects the user's Strict TDD Mode choice (Issue #149).
	// Only meaningful when HasSDD is true.
	StrictTDD bool

	// HasSDD is true when the SDD component is present in the resolved plan (Issue #149).
	// Controls whether the Strict TDD row is shown in the review screen.
	HasSDD bool
}

type PlatformDecision struct {
	OS             string
	LinuxDistro    string
	PackageManager string
	Supported      bool
}

func PlatformDecisionFromProfile(profile system.PlatformProfile) PlatformDecision {
	return PlatformDecision{
		OS:             profile.OS,
		LinuxDistro:    profile.LinuxDistro,
		PackageManager: profile.PackageManager,
		Supported:      profile.Supported,
	}
}

type ComponentAction struct {
	ID     model.ComponentID
	Action string
}
