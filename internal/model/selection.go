package model

type Selection struct {
	Agents           []AgentID
	Components       []ComponentID
	Skills           []SkillID
	Persona          PersonaID
	Preset           PresetID
	SDDMode          SDDModeID
	ModelAssignments ModelAssignments // key = sub-agent name (e.g., "sdd-propose")
}

func (s Selection) HasAgent(agent AgentID) bool {
	for _, current := range s.Agents {
		if current == agent {
			return true
		}
	}

	return false
}

func (s Selection) HasComponent(component ComponentID) bool {
	for _, current := range s.Components {
		if current == component {
			return true
		}
	}

	return false
}
