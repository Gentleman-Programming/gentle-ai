package pi

import "encoding/json"

// ModelRoutingDraft contains the assignments supplied to a validate operation.
// An assignment with no model or thinking value inherits the selected setting.
type ModelRoutingDraft map[string]ModelRoutingDraftAssignment

// ModelRoutingDraftAssignment is one optional model-routing draft assignment.
type ModelRoutingDraftAssignment struct {
	Model    *string               `json:"model,omitempty"`
	Thinking *ModelRoutingThinking `json:"thinking,omitempty"`
}

// MarshalJSON encodes a nil draft as the empty object accepted by the protocol.
func (d ModelRoutingDraft) MarshalJSON() ([]byte, error) {
	if d == nil {
		return []byte("{}"), nil
	}
	type draft ModelRoutingDraft
	return json.Marshal(draft(d))
}

func cloneModelRoutingDraft(draft ModelRoutingDraft) ModelRoutingDraft {
	if draft == nil {
		return nil
	}
	clone := make(ModelRoutingDraft, len(draft))
	for name, assignment := range draft {
		copyAssignment := ModelRoutingDraftAssignment{}
		if assignment.Model != nil {
			model := *assignment.Model
			copyAssignment.Model = &model
		}
		if assignment.Thinking != nil {
			thinking := *assignment.Thinking
			copyAssignment.Thinking = &thinking
		}
		clone[name] = copyAssignment
	}
	return clone
}
