package reviewtransaction

import "sort"

// AdvisoryDisposition names, out loud, why one frozen finding on an approved
// lineage did not block. The routing itself is not new: CompactReviewView
// derives it from admitted role values, and this vocabulary changes none of it.
// What was missing is that a consumer reading an approved receipt saw only the
// severity string and had to infer the consequence from it -- the exact inference
// a field incident got wrong, "fixing" a WARNING on an approved candidate and
// burning a second full review cycle on it.
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
	// AdvisoryRefuted is a severe finding the refuter knocked
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
// finding listed in the admitted view's FixFindingIDs blocked and was corrected,
// so it is excluded rather than relabelled, and a lineage that is not
// StateApproved yields nothing at all because its dispositions are not settled.
func AdvisoryFindings(state CompactState) []AdvisoryFinding {
	if state.State != StateApproved {
		return nil
	}
	view, err := state.CompactReviewView()
	if err != nil {
		return nil
	}
	selected := nonBlockingFindingsOf(view)
	if len(selected) == 0 {
		return nil
	}
	advisory := make([]AdvisoryFinding, len(selected))
	for index, finding := range selected {
		advisory[index] = AdvisoryFinding{
			ID: finding.ID, Lens: finding.Lens, Location: finding.Location,
			Severity: finding.Severity, Disposition: finding.Disposition,
		}
	}
	return advisory
}

// nonBlockingFindingsOf is the one selection both projections share: every
// admitted finding outside the view's FixFindingIDs whose recorded outcome
// advisoryDisposition names, sorted by finding ID. An inconclusive or
// corroborated severe finding is never selected.
func nonBlockingFindingsOf(view CompactReviewView) []NonBlockingFinding {
	corrected := stringSet(view.FixFindingIDs)
	selected := make([]NonBlockingFinding, 0, len(view.Findings))
	for _, finding := range view.Findings {
		if _, blocked := corrected[finding.ID]; blocked {
			continue
		}
		disposition, ok := advisoryDisposition(finding, view.Outcomes[finding.ID])
		if !ok {
			continue
		}
		selected = append(selected, NonBlockingFinding{
			ID: finding.ID, Lens: finding.Lens, Location: finding.Location,
			Severity: finding.Severity, Disposition: disposition, Claim: finding.Claim,
		})
	}
	sort.Slice(selected, func(left, right int) bool { return selected[left].ID < selected[right].ID })
	return selected
}

// NonBlockingStatement explains the non-blocking findings of a review that
// stopped in correction_required or escalated. It never says the review
// passed: the findings to correct and the unresolved findings travel
// elsewhere, and none of them is listed here.
const NonBlockingStatement = "Every finding listed here is non-blocking: it is not a finding to correct and not an unresolved finding that stopped this review, and it needs no correction in this review. A `follow_up` finding is separate later work; a `refuted` finding needs none."

// NonBlockingFinding is one non-blocking finding of a stopped review, with the
// reviewer's admitted claim text. The claim is data for the reader; nothing
// routes on it.
type NonBlockingFinding struct {
	ID          string              `json:"id"`
	Lens        string              `json:"lens,omitempty"`
	Location    string              `json:"location,omitempty"`
	Severity    string              `json:"severity"`
	Disposition AdvisoryDisposition `json:"disposition"`
	Claim       string              `json:"claim"`
}

// NonBlockingFindingSet pairs the findings with their statement.
type NonBlockingFindingSet struct {
	Statement string               `json:"statement"`
	Findings  []NonBlockingFinding `json:"findings"`
}

// NonBlockingFindingSetFor projects the non-blocking findings of a compact
// lineage in StateCorrectionRequired or StateEscalated, with the same
// selection as AdvisoryFindings. It returns an explicit empty set when every
// finding blocks, and nil for any other state or an underivable view. It is a
// pure read and decides nothing.
func NonBlockingFindingSetFor(state CompactState) *NonBlockingFindingSet {
	if state.State != StateCorrectionRequired && state.State != StateEscalated {
		return nil
	}
	view, err := state.CompactReviewView()
	if err != nil {
		return nil
	}
	return &NonBlockingFindingSet{Statement: NonBlockingStatement, Findings: nonBlockingFindingsOf(view)}
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
