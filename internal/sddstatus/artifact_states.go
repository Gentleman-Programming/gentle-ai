package sddstatus

// artifactStates carries one field per SDD artifact whose readiness a v2 status
// reports, and statesFor is the only builder of a status artifact map.
type artifactStates struct {
	Proposal      ArtifactState
	Specs         ArtifactState
	Design        ArtifactState
	Tasks         ArtifactState
	ApplyProgress ArtifactState
	VerifyReport  ArtifactState
}

// artifactStateField binds one wire key to its artifactStates field. The v2
// list deliberately contains only planning, apply, and verification artifacts;
// review artifacts are not status context or state.
type artifactStateField struct {
	key   string
	field func(*artifactStates) *ArtifactState
}

// artifactStateFields is the single authority for the v2 artifact key set.
var artifactStateFields = []artifactStateField{
	{key: "proposal", field: func(states *artifactStates) *ArtifactState { return &states.Proposal }},
	{key: "specs", field: func(states *artifactStates) *ArtifactState { return &states.Specs }},
	{key: "design", field: func(states *artifactStates) *ArtifactState { return &states.Design }},
	{key: "tasks", field: func(states *artifactStates) *ArtifactState { return &states.Tasks }},
	{key: "applyProgress", field: func(states *artifactStates) *ArtifactState { return &states.ApplyProgress }},
	{key: "verifyReport", field: func(states *artifactStates) *ArtifactState { return &states.VerifyReport }},
}

// artifactStateKeys reports, in wire order, the artifact keys every store
// declares.
func artifactStateKeys(_ ArtifactStore) []string {
	keys := make([]string, 0, len(artifactStateFields))
	for _, field := range artifactStateFields {
		keys = append(keys, field.key)
	}
	return keys
}

// statesFor builds the artifact map for one store. A field left unset means the
// resolver never found that artifact, which is exactly ArtifactMissing.
func (states artifactStates) statesFor(store ArtifactStore) map[string]ArtifactState {
	built := make(map[string]ArtifactState, len(artifactStateFields))
	for _, field := range artifactStateFields {
		state := *field.field(&states)
		if state == "" {
			state = ArtifactMissing
		}
		built[field.key] = state
	}
	return built
}
