package reviewtransaction

// The reviewer envelope markers are declared exactly once, here, because they
// are the only contract shared by two halves that ship separately: the renderer
// that materializes a lens context and the installed agent definition that
// admits it. A reviewer runs with no execution tools, so the prompt is its only
// channel; a marker the definition requires but the renderer never emits makes
// every rendered prompt inadmissible, and no caller can route around it.
//
// That is exactly what issue #2777 was: a second Claude-only spelling of the
// context marker lived beside the renderer's, the two drifted, and the Claude
// path could not reach a terminal receipt at all. The names are constants in a
// package both halves already import so the drift is not merely fixed but
// unrepresentable. A runtime difference belongs in which process supplies the
// block, never in what the block is called.
const (
	// ReviewerBindingMarker prefixes the one-line binding JSON that opens every
	// reviewer task.
	ReviewerBindingMarker = "GENTLE_AI_REVIEW_BINDING"
	// ReviewerContextMarker opens the immutable candidate evidence block, and
	// ReviewerContextTerminator closes it.
	ReviewerContextMarker     = "GENTLE_AI_REVIEW_CONTEXT"
	ReviewerContextTerminator = ReviewerContextMarker + "_END"
)
