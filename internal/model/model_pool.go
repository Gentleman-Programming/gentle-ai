package model

// ModelReference identifies a provider/model pair in the format "provider/modelID".
// Example: "anthropic/claude-opus-4-5", "openai/gpt-4o"
type ModelReference string

// ModelPool defines an ordered set of models for a phase.
// Primary is always attempted first; Fallbacks are tried in order if Primary fails.
type ModelPool struct {
	Primary   ModelReference   `json:"primary"`
	Fallbacks []ModelReference `json:"fallbacks,omitempty"`
}

// All returns all models in the pool in priority order: Primary first, then Fallbacks.
// This is useful for iterating through all models during fallback logic.
func (p ModelPool) All() []ModelReference {
	result := make([]ModelReference, 0, 1+len(p.Fallbacks))
	result = append(result, p.Primary)
	result = append(result, p.Fallbacks...)
	return result
}

// IsZero returns true if the pool has no valid primary model.
func (p ModelPool) IsZero() bool {
	return p.Primary == ""
}

// FromModelAssignment creates a ModelPool from a legacy ModelAssignment.
// The resulting pool has only a Primary model with no fallbacks.
func FromModelAssignment(assignment ModelAssignment) ModelPool {
	if assignment.ProviderID == "" || assignment.ModelID == "" {
		return ModelPool{}
	}
	return ModelPool{
		Primary: ModelReference(assignment.ProviderID + "/" + assignment.ModelID),
	}
}

// ToModelAssignment converts the Primary model to a legacy ModelAssignment.
// Fallbacks are ignored since ModelAssignment only supports a single model.
func (p ModelPool) ToModelAssignment() ModelAssignment {
	if p.Primary == "" {
		return ModelAssignment{}
	}
	parts := splitProviderModel(string(p.Primary))
	return ModelAssignment{
		ProviderID: parts[0],
		ModelID:    parts[1],
	}
}

// ModelAssignments maps SDD phase names to their model pools.
// Keys are phase names like "sdd-propose", "sdd-spec", "sdd-design", etc.
type ModelAssignments map[string]ModelPool

// LegacyModelAssignments is the old format for backward compatibility.
// Deprecated: Use ModelAssignments instead.
type LegacyModelAssignments map[string]ModelAssignment

// ToModelAssignments converts a legacy ModelAssignment map to the new ModelAssignments format.
func (l LegacyModelAssignments) ToModelAssignments() ModelAssignments {
	result := make(ModelAssignments, len(l))
	for phase, assignment := range l {
		result[phase] = FromModelAssignment(assignment)
	}
	return result
}

// ToLegacy converts ModelAssignments to the legacy format for backward compatibility
// with code that has not been migrated yet. Note: This only preserves the Primary model.
func (m ModelAssignments) ToLegacy() LegacyModelAssignments {
	result := make(LegacyModelAssignments, len(m))
	for phase, pool := range m {
		result[phase] = pool.ToModelAssignment()
	}
	return result
}

// DefaultModelPool returns a sensible default pool for phases without explicit configuration.
// The default prioritizes capable models for SDD tasks.
func DefaultModelPool() ModelPool {
	return ModelPool{
		Primary:   "anthropic/claude-sonnet-4-20250514",
		Fallbacks: []ModelReference{"openai/gpt-4o", "openai/gpt-3.5-turbo"},
	}
}

// DefaultModelAssignments returns default model pools for all SDD phases.
func DefaultModelAssignments() ModelAssignments {
	return ModelAssignments{
		"sdd-propose": DefaultModelPool(),
		"sdd-spec":    DefaultModelPool(),
		"sdd-design":  DefaultModelPool(),
		"sdd-tasks":   DefaultModelPool(),
		"sdd-apply":   DefaultModelPool(),
		"sdd-verify":  DefaultModelPool(),
	}
}

// ResolvePool returns the model pool for a phase using the following hierarchy:
// 1. Pool explicitly configured in assignments
// 2. Legacy ModelAssignment converted to ModelPool (retrocompatibility)
// 3. Default pool
//
// The assignmentsLegacy parameter supports the old map[string]ModelAssignment format
// for backward compatibility with existing configurations.
func ResolvePool(phase string, assignments ModelAssignments, assignmentsLegacy map[string]ModelAssignment) ModelPool {
	// 1. Check explicit ModelPool configuration
	if pool, ok := assignments[phase]; ok && !pool.IsZero() {
		return pool
	}

	// 2. Fall back to legacy ModelAssignment (retrocompatibility)
	if assignmentsLegacy != nil {
		if assignment, ok := assignmentsLegacy[phase]; ok {
			if assignment.ProviderID != "" && assignment.ModelID != "" {
				return FromModelAssignment(assignment)
			}
		}
	}

	// 3. Return default
	return DefaultModelPool()
}

// splitProviderModel splits a model reference into provider and model ID.
// Returns a two-element slice: [provider, modelID].
// If the reference doesn't contain a "/", returns ["", reference].
func splitProviderModel(ref string) [2]string {
	for i := len(ref) - 1; i >= 0; i-- {
		if ref[i] == '/' {
			return [2]string{ref[:i], ref[i+1:]}
		}
	}
	return [2]string{"", ref}
}