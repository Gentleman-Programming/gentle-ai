package reviewtransaction

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// CompactDecisionRequest is one human decision on a paused decision_required
// lineage. The decision vocabulary is closed (continue|stop); the actor and
// reason are the human attribution journaled with the decision.
type CompactDecisionRequest struct {
	LineageID        string
	ExpectedRevision string
	Decision         string
	Actor            string
	Reason           string
}

// DecideCompactStore resolves one paused decision_required lineage. A continue
// resumes the reviewing authority with the answered question cleared; a stop
// escalates with the frozen question and journal retained. The write is the
// same compare-and-swap as every other compact authority mutation: a stale
// expected revision surfaces the typed conflict and proves the loser mutated
// nothing. An exact replay of an already-committed decision (lineage, revision,
// decision, actor, and reason all equal) converges on the committed record.
func DecideCompactStore(ctx context.Context, repo string, request CompactDecisionRequest) (CompactRecord, error) {
	if err := ctx.Err(); err != nil {
		return CompactRecord{}, err
	}
	if err := validateLineageID(request.LineageID); err != nil {
		return CompactRecord{}, fmt.Errorf("review decide requires a valid lineage id: %w", err)
	}
	if request.Decision != CompactDecisionContinue && request.Decision != CompactDecisionStop {
		return CompactRecord{}, fmt.Errorf("review decide requires decision %q or %q; re-run `gentle-ai review decide` with --decision continue or --decision stop", CompactDecisionContinue, CompactDecisionStop)
	}
	if strings.TrimSpace(request.Actor) == "" || strings.TrimSpace(request.Reason) == "" {
		return CompactRecord{}, errors.New("review decide requires a non-empty actor and reason; re-run `gentle-ai review decide` with --actor and --reason supplied")
	}
	if strings.TrimSpace(request.ExpectedRevision) == "" {
		return CompactRecord{}, errors.New("review decide requires the exact current store revision; read it from `gentle-ai review status` entries[].revision")
	}
	store, err := CompactAuthoritativeStore(ctx, repo, request.LineageID)
	if err != nil {
		return CompactRecord{}, err
	}
	record, err := store.Load()
	if err != nil {
		if os.IsNotExist(err) {
			return CompactRecord{}, fmt.Errorf("review decide refused: lineage %q holds no compact authority state; run `gentle-ai review status --cwd <repo>` to list the persisted authorities", request.LineageID)
		}
		return CompactRecord{}, fmt.Errorf("load decide target: %w", err)
	}
	if record.State.State != StateDecisionRequired {
		if compactDecisionReplay(record, request) {
			return record, nil
		}
		// refusal:by-design human-authority: only the paused question itself can be decided; anything else needs its own operation
		return CompactRecord{}, fmt.Errorf("review decide refused: lineage %q holds %q authority; only a decision_required authority may be decided", request.LineageID, record.State.State)
	}
	// The entry is fully validated and bound to the expected revision BEFORE
	// the successor is appended: the journal entry rides the compare-and-swap,
	// so a decision is never visible without its actor and reason, and an
	// interrupted decide never leaves a half-journaled state.
	entry := CompactDecisionEntry{
		Decision: request.Decision, Actor: strings.TrimSpace(request.Actor),
		Reason: strings.TrimSpace(request.Reason), Revision: request.ExpectedRevision,
	}
	next := record.State
	next.DecisionHistory = append(append([]CompactDecisionEntry{}, next.DecisionHistory...), entry)
	next.DecisionEpoch++
	switch request.Decision {
	case CompactDecisionContinue:
		setCompactStateExit(&next, StateReviewing)
		next.FixFindingIDs = []string{}
		next.Decision = nil
	case CompactDecisionStop:
		setCompactStateExit(&next, StateEscalated)
	}
	revision, err := store.Replace(request.ExpectedRevision, "review/decide", next)
	if err != nil {
		return CompactRecord{}, err
	}
	return CompactRecord{Schema: record.Schema, Revision: revision, State: next}, nil
}

// compactDecisionReplay answers whether a loaded record already IS the exact
// decision this request describes: the last journaled entry must equal the
// request's lineage-bound entry and the state must match that decision.
func compactDecisionReplay(record CompactRecord, request CompactDecisionRequest) bool {
	state := record.State
	if len(state.DecisionHistory) == 0 {
		return false
	}
	want := CompactDecisionEntry{
		Decision: request.Decision, Actor: strings.TrimSpace(request.Actor),
		Reason: strings.TrimSpace(request.Reason), Revision: request.ExpectedRevision,
	}
	if state.DecisionHistory[len(state.DecisionHistory)-1] != want {
		return false
	}
	switch request.Decision {
	case CompactDecisionContinue:
		return state.State == StateReviewing && state.Decision == nil
	case CompactDecisionStop:
		return state.State == StateEscalated
	}
	return false
}
