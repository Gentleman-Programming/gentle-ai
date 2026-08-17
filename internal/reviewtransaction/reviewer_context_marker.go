package reviewtransaction

// ReviewerContextMarker names the header and terminator that frame one
// provider-injected reviewer context block. Header and Terminator are
// same-typed and used adjacently when framing a block, so they are carried as
// one value rather than as two independently-passed strings a caller could
// mismatch.
type ReviewerContextMarker struct {
	Header     string
	Terminator string
}

// newReviewerContextMarker derives Terminator from Header in one place, so a
// table entry can never record a terminator that does not actually close its
// own header.
func newReviewerContextMarker(header string) ReviewerContextMarker {
	return ReviewerContextMarker{Header: header, Terminator: header + "_END"}
}

// reviewerContextMarkers is the closed provider→marker table. It is keyed by
// plain string rather than model.AgentID so this package never depends on
// internal/model.
//
//   - "" is the omitted-flag default: today's generic marker, kept as an
//     explicit table entry rather than a caller-side branch so "flag omitted"
//     is one supported case inside the single resolver.
//   - "claude-code" resolves to the marker Claude's installed reviewer already
//     requires.
//   - "opencode" resolves to the generic marker, verbatim and unchanged, but
//     now an explicit table entry instead of a coincidence.
//
// Every other value -- including recognized-but-unmapped runtime identities --
// has no entry and therefore resolves to ok=false. Adding a new runtime here
// must be a deliberate table edit, never an inferred fallback.
var reviewerContextMarkers = map[string]ReviewerContextMarker{
	"":            newReviewerContextMarker("GENTLE_AI_REVIEW_CONTEXT"),
	"claude-code": newReviewerContextMarker("GENTLE_AI_CLAUDE_REVIEW_CONTEXT"),
	"opencode":    newReviewerContextMarker("GENTLE_AI_REVIEW_CONTEXT"),
}

// ReviewerContextMarkerFor resolves the header/terminator pair a caller's
// generated reviewer expects, keyed by the exact agent identity string.
func ReviewerContextMarkerFor(agent string) (ReviewerContextMarker, bool) {
	marker, found := reviewerContextMarkers[agent]
	return marker, found
}
