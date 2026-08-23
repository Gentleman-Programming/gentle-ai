package reviewtransaction

import "sort"

// AdvisoryDisposition names, out loud, why one frozen finding on an approved
// lineage did not block. The routing itself is not new: it is already recorded
// in CompactState.Outcomes and CompactState.FixFindingIDs, and this vocabulary
// changes none of it. What was missing is that a consumer reading an approved
// receipt saw only the severity string and had to infer the consequence from
// it -- the exact inference a field incident got wrong, "fixing" a WARNING on
// an approved candidate and burning a second full review cycle on it.
type AdvisoryDisposition string

const (
	// AdvisoryInformational is a non-severe finding (WARNING or SUGGESTION).
	// It never entered classification, never opened a follow-up, and requires
	// no action of any kind.
	AdvisoryInformational AdvisoryDisposition = "informational"
	// AdvisoryFollowUp is a severe finding whose cause was proved to lie
	// outside the frozen candidate (pre-existing or base-only). It is recorded
	// as a follow-up for separate later work and never reopens this review.
	AdvisoryFollowUp AdvisoryDisposition = "follow_up"
	// AdvisoryRefuted is a severe inferential finding the refuter knocked
	// down. It carries no residual obligation at all.
	AdvisoryRefuted AdvisoryDisposition = "refuted"
)

// AdvisoryStatement is the one sentence that answers the question the
// disposition vocabulary alone still leaves open: what a consumer should do
// next. It is deliberately explicit that the approval stands and that no
// correction transition exists to take.
const AdvisoryStatement = "This review is approved and its receipt stands. Every finding listed here is non-blocking: none opened a correction, none reopens this review, and no correction transition is offered for this candidate. Treat them as separate later work, never as a reason to re-run review on this candidate."

// AdvisoryFinding is one frozen non-blocking finding projected with its
// explicit disposition. It carries identity, severity, and location so a
// consumer can locate the finding it already read, and nothing else: the claim
// and proof refs stay where they were captured.
type AdvisoryFinding struct {
	ID          string              `json:"id"`
	Lens        string              `json:"lens,omitempty"`
	Location    string              `json:"location,omitempty"`
	Severity    string              `json:"severity"`
	Disposition AdvisoryDisposition `json:"disposition"`
}

// AdvisoryFindingSet is the whole projection: the findings plus the sentence
// that says the approval stands.
type AdvisoryFindingSet struct {
	Statement string            `json:"statement"`
	Findings  []AdvisoryFinding `json:"findings"`
}

// AdvisoryFindings projects every non-blocking frozen finding of a terminally
// approved lineage, sorted by finding ID for a deterministic payload.
//
// It is a pure read of already-decided state and decides nothing itself: a
// finding listed in FixFindingIDs blocked and was corrected, so it is excluded
// rather than relabelled, and a lineage that is not StateApproved yields
// nothing at all because its dispositions are not yet settled.
func AdvisoryFindings(state CompactState) []AdvisoryFinding {
	if state.State != StateApproved {
		return nil
	}
	corrected := stringSet(state.FixFindingIDs)
	advisory := make([]AdvisoryFinding, 0, len(state.Findings))
	for _, finding := range state.Findings {
		if _, blocked := corrected[finding.ID]; blocked {
			continue
		}
		disposition, ok := advisoryDisposition(finding, state.Outcomes[finding.ID])
		if !ok {
			continue
		}
		advisory = append(advisory, AdvisoryFinding{
			ID: finding.ID, Lens: finding.Lens, Location: finding.Location,
			Severity: finding.Severity, Disposition: disposition,
		})
	}
	if len(advisory) == 0 {
		return nil
	}
	sort.Slice(advisory, func(left, right int) bool { return advisory[left].ID < advisory[right].ID })
	return advisory
}

// AdvisoryFindingSetFor pairs the projection with its statement, or reports
// nothing when the lineage has no non-blocking finding to explain.
func AdvisoryFindingSetFor(state CompactState) *AdvisoryFindingSet {
	findings := AdvisoryFindings(state)
	if len(findings) == 0 {
		return nil
	}
	return &AdvisoryFindingSet{Statement: AdvisoryStatement, Findings: findings}
}

// advisoryDisposition maps one already-recorded outcome to its named
// disposition. An outcome this function does not recognize is skipped rather
// than guessed: an unnamed routing must never be announced as harmless.
func advisoryDisposition(finding Finding, outcome EvidenceOutcome) (AdvisoryDisposition, bool) {
	switch outcome {
	case OutcomeInfo:
		if isSevereSeverity(finding.Severity) {
			return AdvisoryFollowUp, true
		}
		return AdvisoryInformational, true
	case OutcomeRefuted:
		return AdvisoryRefuted, true
	default:
		return "", false
	}
}
