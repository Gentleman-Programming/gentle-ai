package sddstatus

import "errors"

// ErrRuntimeStageUnknown is returned when a vocabulary lookup fails.
var ErrRuntimeStageUnknown = errors.New("runtime stage unknown")

// Stage defines a single logical step in an SDD vocabulary chain.
type Stage struct {
	Label string `json:"label"`
}

// StageVocabulary defines an ordered set of valid stages for a chain.
type StageVocabulary struct {
	ID     string  `json:"id"`
	Stages []Stage `json:"stages"`
}

// Validate ensures the vocabulary is structurally sound: has an ID,
// contains at least two stages, and all stage labels are unique and non-empty.
func (v StageVocabulary) Validate() error {
	if v.ID == "" {
		return errors.New("vocabulary ID cannot be empty")
	}
	if len(v.Stages) < 2 {
		return errors.New("vocabulary must define at least two stages")
	}

	seen := make(map[string]bool)
	for _, s := range v.Stages {
		if s.Label == "" {
			return errors.New("stage label cannot be empty")
		}
		if seen[s.Label] {
			return errors.New("duplicate stage label: " + s.Label)
		}
		seen[s.Label] = true
	}
	return nil
}

// Position returns the 0-indexed position of the given stage label,
// or ErrRuntimeStageUnknown if it is not found in the vocabulary.
func (v StageVocabulary) Position(label string) (int, error) {
	for i, s := range v.Stages {
		if s.Label == label {
			return i, nil
		}
	}
	return -1, ErrRuntimeStageUnknown
}

// At returns the Stage at the specified 0-indexed position,
// or ErrRuntimeStageUnknown if the position is out of bounds.
func (v StageVocabulary) At(position int) (Stage, error) {
	if position < 0 || position >= len(v.Stages) {
		return Stage{}, ErrRuntimeStageUnknown
	}
	return v.Stages[position], nil
}
