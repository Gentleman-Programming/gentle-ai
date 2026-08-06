package sddstatus

// artifactStates carries one field per SDD artifact whose readiness a status
// reports, and statesFor is the only builder of a status artifact map.
//
// Issue #2346: before this, three call sites each wrote their own artifact map
// literal (Resolve, resolveEngramStatus, baseStatus) and baseStatus hardcoded
// an OpenSpec-shaped map that blockedEngramStatus then relabelled "engram"
// without rebuilding it. The v1 projection derives its required key set from
// the store, so it demanded a "reviewPolicy" entry nobody had populated and
// rejected the whole status with the Go zero value in the message:
// `unsupported SDD v1 artifact "reviewPolicy" state ""`. Deriving both the map
// and the projection's key set from artifactStateFields, and taking the store
// as a build-time input rather than a post-hoc relabel, removes the ability
// for the two to disagree instead of teaching the projection to tolerate it.
type artifactStates struct {
	Proposal      ArtifactState
	Specs         ArtifactState
	Design        ArtifactState
	Tasks         ArtifactState
	ApplyProgress ArtifactState
	VerifyReport  ArtifactState
	ReviewPolicy  ArtifactState
	ReviewLedger  ArtifactState
	ReviewReceipt ArtifactState
	ReviewBundle  ArtifactState
	ReviewContext ArtifactState
	ReviewState   ArtifactState
}

// artifactStateField binds one wire key to its artifactStates field and to the
// stores that declare it.
type artifactStateField struct {
	key   string
	field func(*artifactStates) *ArtifactState
	// stores lists the artifact stores that declare this key. An empty list
	// means every store declares it.
	stores []ArtifactStore
}

func (field artifactStateField) declaredBy(store ArtifactStore) bool {
	if len(field.stores) == 0 {
		return true
	}
	for _, declared := range field.stores {
		if declared == store {
			return true
		}
	}
	return false
}

// artifactStateFields is the single authority for the SDD v1 artifact key set.
// Both artifactStateKeys (what the projection demands) and statesFor (what the
// builders supply) read it, so an artifact added here reaches every store's
// builder and the projection together, and one added anywhere else reaches
// neither.
//
// reviewPolicy is Engram-only by ratified contract: assets/skills/_shared/
// sdd-status-contract.md states that reviewPolicy is present in artifactPaths
// and contextFiles, while "its artifact-state entry is present only for Engram
// status and omitted otherwise".
var artifactStateFields = []artifactStateField{
	{key: "proposal", field: func(states *artifactStates) *ArtifactState { return &states.Proposal }},
	{key: "specs", field: func(states *artifactStates) *ArtifactState { return &states.Specs }},
	{key: "design", field: func(states *artifactStates) *ArtifactState { return &states.Design }},
	{key: "tasks", field: func(states *artifactStates) *ArtifactState { return &states.Tasks }},
	{key: "applyProgress", field: func(states *artifactStates) *ArtifactState { return &states.ApplyProgress }},
	{key: "verifyReport", field: func(states *artifactStates) *ArtifactState { return &states.VerifyReport }},
	{key: "reviewPolicy", field: func(states *artifactStates) *ArtifactState { return &states.ReviewPolicy }, stores: []ArtifactStore{ArtifactStoreEngram}},
	{key: "reviewLedger", field: func(states *artifactStates) *ArtifactState { return &states.ReviewLedger }},
	{key: "reviewReceipt", field: func(states *artifactStates) *ArtifactState { return &states.ReviewReceipt }},
	{key: "reviewBundle", field: func(states *artifactStates) *ArtifactState { return &states.ReviewBundle }},
	{key: "reviewContext", field: func(states *artifactStates) *ArtifactState { return &states.ReviewContext }},
	{key: "reviewState", field: func(states *artifactStates) *ArtifactState { return &states.ReviewState }},
}

// artifactStateKeys reports, in wire order, the artifact keys the given store
// declares.
func artifactStateKeys(store ArtifactStore) []string {
	keys := make([]string, 0, len(artifactStateFields))
	for _, field := range artifactStateFields {
		if field.declaredBy(store) {
			keys = append(keys, field.key)
		}
	}
	return keys
}

// statesFor builds the artifact map for one store. A field left unset means
// the resolver never found that artifact, which is exactly ArtifactMissing, so
// a caller cannot produce a key with an illegal state by omission.
func (states artifactStates) statesFor(store ArtifactStore) map[string]ArtifactState {
	built := make(map[string]ArtifactState, len(artifactStateFields))
	for _, field := range artifactStateFields {
		if !field.declaredBy(store) {
			continue
		}
		state := *field.field(&states)
		if state == "" {
			state = ArtifactMissing
		}
		built[field.key] = state
	}
	return built
}
