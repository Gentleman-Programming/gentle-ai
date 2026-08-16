package reviewtransaction

import "regexp"

// ReviewerContextLevel names the mechanism that put the immutable candidate
// evidence in front of a reviewer. It is recorded so a receipt stays
// classifiable forever; it is never a gate input, and nothing in this product
// compares two levels or ranks them.
//
// The vocabulary is deliberately asymmetric, and that asymmetry is the whole
// forward-compatibility design:
//
//   - On INPUT it is closed. A caller may declare only a mechanism this
//     release actually implements, so an unrecognised declaration fails closed
//     instead of being recorded as fact.
//   - On READ it is open. A persisted level is validated for shape only, never
//     for membership, so a binary built today reads a receipt written by a
//     later release that records a mechanism this one has never heard of. Every
//     other closed vocabulary on the receipt (terminal_state, risk_level,
//     evidence_outcome) rejects its default case; this one must not, because a
//     receipt is issued once and read for the rest of its life.
//
// A level is never inferred. An absent level means "not recorded" — which is
// also what every receipt issued before this field existed carries — and never
// means "the weakest mechanism".
type ReviewerContextLevel string

const (
	// ReviewerContextLevelProviderCommand: a caller ran the provider's own
	// lens-context command and relayed its exact output. The provider authored
	// every machine field; the caller is trusted only to relay bytes.
	ReviewerContextLevelProviderCommand ReviewerContextLevel = "provider_command"
	// ReviewerContextLevelRuntimeInterception: a runtime adapter replaced
	// whatever the caller produced with the provider's own output before the
	// reviewer ran, so relaying is not trusted at all.
	ReviewerContextLevelRuntimeInterception ReviewerContextLevel = "runtime_interception"
	// ReviewerContextLevelProviderContract records Go-materialized and
	// Go-admitted output delivered through the live provider transport after its
	// immutable slot is durable. It records transport provenance, not UI
	// provenance or a claim about how a user interface displayed the result.
	ReviewerContextLevelProviderContract ReviewerContextLevel = "provider_contract"
)

// reviewerContextLevelShape bounds what may be persisted and read back. It
// admits any lowercase snake_case token, which is what keeps an unknown future
// level readable, while still rejecting bytes that would corrupt a receipt or
// smuggle a path, newline, or separator into an audit record.
var reviewerContextLevelShape = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}[a-z0-9]$`)

// ReviewerContextLevelAccepted reports whether a caller may declare this level
// now. It is a closed membership test on purpose: declaring a mechanism this
// release does not implement would record a claim nothing produced.
func ReviewerContextLevelAccepted(level ReviewerContextLevel) bool {
	switch level {
	case ReviewerContextLevelProviderCommand, ReviewerContextLevelRuntimeInterception, ReviewerContextLevelProviderContract:
		return true
	default:
		return false
	}
}

// ReviewerContextLevelWellFormed reports whether a persisted level is
// structurally readable. It is deliberately NOT a membership test: a level this
// binary has never heard of is readable, preserved, and reported unchanged.
func ReviewerContextLevelWellFormed(level ReviewerContextLevel) bool {
	return reviewerContextLevelShape.MatchString(string(level))
}

// lensMandate is the canonical role of one review lens: what the reviewer is
// and what it is charged with inspecting. It lives here, beside the frozen
// candidate machinery, because two independent surfaces need the identical
// words — the runtime agent definitions this product installs, and the
// provider-owned lens context a runtime with no agent definitions consumes.
// A reviewer whose mandate depends on which surface launched it is not the
// same reviewer.
type lensMandate struct {
	title string
	focus string
}

var lensMandates = map[string]lensMandate{
	LensRisk: {
		title: "R1 Risk",
		focus: "Inspect security, authorization, data exposure or loss, unsafe input handling, secrets, and dependency vulnerabilities. Require backend enforcement and concrete exploit or scanner evidence; do not report hypothetical risk without a reachable impact.",
	},
	LensResilience: {
		title: "R4 Resilience",
		focus: "Inspect failure handling, rollback or fix-forward behavior, retry safety, graceful degradation, observability, latency, and load. Require a concrete production failure mode or measured impact; do not report generic operational speculation.",
	},
	LensReadability: {
		title: "R2 Readability",
		focus: "Inspect maintainability defects that obscure behavior: misleading names, duplicated or dead logic, unexplained business constants, unsafe complexity, and missing change context. Report style only when it hides a concrete defect or makes the change unsafe to maintain.",
	},
	LensReliability: {
		title: "R3 Reliability",
		focus: "Inspect behavior, tests, boundaries, invalid inputs, failure paths, determinism, and regressions. Require externally observable assertions at the cheapest useful test level; report missing coverage only when it leaves candidate behavior unproved.",
	},
}

// LensMandate returns the canonical title and inspection focus of one lens.
// It fails closed for an unknown lens rather than inventing a mandate: a
// reviewer launched with no charge would review whatever it felt like.
func LensMandate(lens string) (string, string, bool) {
	mandate, found := lensMandates[lens]
	if !found {
		return "", "", false
	}
	return mandate.title, mandate.focus, true
}
