package sddstatus

// Targeted re-verification describes the evidence scope affected by a review
// correction. The status field is advisory: it never blocks archive readiness
// or redirects delivery away from ordinary repository policy.

const (
	ReVerifyModeTargeted = "targeted"
	ReVerifyModeFull     = "full"
)

// ReVerifyBlock provides advisory review-correction context to the
// orchestrator. SDD requirements, tasks, and verification—not this field—
// determine archive readiness.
type ReVerifyBlock struct {
	Mode   string   `json:"mode"`
	Scope  []string `json:"scope,omitempty"`
	Reason string   `json:"reason"`
}

type correctionEvidence struct {
	applied    bool
	paths      []string
	derivable  bool
	failClosed bool
}
