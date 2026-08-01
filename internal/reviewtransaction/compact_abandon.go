package reviewtransaction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// CompactAbandonAuthorizationSchema is the first line of the exact six-line
// LF-only abandonment maintainer authorization binding; the remaining lines
// are, in order: lineage, revision, snapshot_identity, actor, and reason.
//
// It is exported so a refusal in another package that names `review abandon`
// as its continuation can print the template and be checked against the schema
// this gate actually verifies, rather than against a copy of the string.
const CompactAbandonAuthorizationSchema = "gentle-ai.review-abandon-authorization/v1"

// CompactIncompleteAbandonAuthorizationSchema binds the second, narrower
// abandonment class: a reviewing lineage whose state is still pristine but
// whose store already holds captured reviewer results for SOME of its selected
// lenses. Its own schema exists so the ordinary pristine token can never
// authorize discarding captured work by accident.
const CompactIncompleteAbandonAuthorizationSchema = "gentle-ai.review-incomplete-abandon-authorization/v1"

// CompactAbandonRequest identifies one pristine compact-v2 review lineage to
// quarantine, together with the exact maintainer authorization binding for
// that content.
type CompactAbandonRequest struct {
	LineageID        string
	ExpectedRevision string
	Reason           string
	Actor            string
	// IncompleteInspection selects the incomplete-inspection abandonment class
	// instead of the pristine one. It is never inferred from the store: a
	// lineage holding captured results is refused unless the maintainer named
	// this class explicitly and authorized it with the matching schema, so the
	// act of discarding reviewed work is always deliberate.
	IncompleteInspection    bool
	MaintainerAuthorization string
	AbandonedAt             time.Time
}

// CompactPristineAbandonmentProof records the natively re-derived pristineness
// of one abandoned lineage inside the quarantine audit record. Pristineness is
// a property of the persisted bytes and store topology only; the live worktree
// is never rebuilt, so stale lineages remain abandonable.
type CompactPristineAbandonmentProof struct {
	LineageID          string `json:"lineage_id"`
	Revision           string `json:"revision"`
	SnapshotIdentity   string `json:"snapshot_identity"`
	State              State  `json:"state"`                         // "reviewing" or "invalidated"
	InvalidationReason string `json:"invalidation_reason,omitempty"` // when pre-invalidated
}

// CompactIncompleteAbandonmentProof records what the incomplete-inspection
// abandonment discarded. Unlike the pristine proof, this class always destroys
// real reviewer work, so the captured and uncaptured lenses are both named: the
// audit record has to show exactly how much of the plan had run and which lens
// never reported when the maintainer retired the review.
type CompactIncompleteAbandonmentProof struct {
	LineageID        string   `json:"lineage_id"`
	Revision         string   `json:"revision"`
	SnapshotIdentity string   `json:"snapshot_identity"`
	CapturedLenses   []string `json:"captured_lenses"`
	UncapturedLenses []string `json:"uncaptured_lenses"`
}

// RenderCompactAbandonAuthorization renders the abandonment authorization
// binding over the supplied values. The refusal path renders it with
// placeholder tokens to print an operator-facing template, and the verifier
// below binds over this same function, so the template a maintainer is told
// to fill in and the bytes the gate accepts can never drift apart.
func RenderCompactAbandonAuthorization(lineage, revision, snapshotIdentity, actor, reason string) string {
	return compactAbandonAuthorizationBinding(lineage, revision, snapshotIdentity, actor, reason)
}

// RenderCompactIncompleteAbandonAuthorization renders the incomplete-inspection
// binding. The captured lenses are bound verbatim, in selected-lens order, so
// the token stops matching the moment another lens reports: a maintainer can
// only discard the exact amount of review work they read about.
func RenderCompactIncompleteAbandonAuthorization(lineage, revision, snapshotIdentity string, capturedLenses []string, actor, reason string) string {
	return compactIncompleteAbandonAuthorizationBinding(lineage, revision, snapshotIdentity, capturedLenses, actor, reason)
}

func compactIncompleteAbandonAuthorizationBinding(lineage, revision, snapshotIdentity string, capturedLenses []string, actor, reason string) string {
	return CompactIncompleteAbandonAuthorizationSchema + "\nlineage=" + lineage + "\nrevision=" + revision +
		"\nsnapshot_identity=" + snapshotIdentity +
		"\ncaptured_lenses=" + strings.Join(capturedLenses, ",") +
		"\nactor=" + strings.TrimSpace(actor) + "\nreason=" + strings.TrimSpace(reason)
}

// compactCapturedLensPartition splits the selected lenses into the ones whose
// reviewer result is already persisted and the ones still missing, in canonical
// selected-lens order. It reads the store directory rather than the state
// because capture-result persists an artifact per lens and leaves the state
// pristine until finalize folds them in — which is exactly why a partially
// captured lineage passes the pristine state rule yet fails the residue scan.
func compactCapturedLensPartition(storeDir string, selectedLenses []string) (captured, uncaptured []string, err error) {
	captured, uncaptured = make([]string, 0, len(selectedLenses)), make([]string, 0, len(selectedLenses))
	for order, lens := range selectedLenses {
		path := filepath.Join(storeDir, CompactReviewerResultsDir, fmt.Sprintf("%02d-%s.json", order, lens))
		if _, err := os.Stat(path); err == nil {
			captured = append(captured, lens)
			continue
		} else if !os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("inspect reviewer result for lens %q: %w", lens, err)
		}
		uncaptured = append(uncaptured, lens)
	}
	return captured, uncaptured, nil
}

func compactAbandonAuthorizationBinding(lineage, revision, snapshotIdentity, actor, reason string) string {
	return CompactAbandonAuthorizationSchema + "\nlineage=" + lineage + "\nrevision=" + revision +
		"\nsnapshot_identity=" + snapshotIdentity +
		"\nactor=" + strings.TrimSpace(actor) + "\nreason=" + strings.TrimSpace(reason)
}

// compactAbandonablePristineState is the single pristineness rule this file
// applies. AbandonPristineCompactStore calls it to decide, and
// InspectCompactPristineAbandonment calls the same function to PREDICT, so a
// refusal elsewhere that names `review abandon` as its continuation can never
// name it for a lineage this gate would then reject.
//
// compactPristineReviewing hard-requires State == StateReviewing and an empty
// InvalidationReason, so an invalidated record is projected back onto its
// underlying reviewing authority before the re-derivation; resetting exactly
// those two fields is load-bearing. The paired non-empty-InvalidationReason
// check doubles as a structural corruption guard: a persisted invalidated
// state without a reason never came from Invalidate.
func compactAbandonablePristineState(state CompactState) bool {
	switch state.State {
	case StateReviewing:
		return compactPristineReviewing(state)
	case StateInvalidated:
		reviewing := state
		reviewing.State, reviewing.InvalidationReason = StateReviewing, ""
		return strings.TrimSpace(state.InvalidationReason) != "" && compactPristineReviewing(reviewing)
	}
	return false
}

// CompactAbandonEligibility reports whether `review abandon` would accept one
// lineage right now, together with the two persisted values the authorization
// binding has to carry. Both are published so a caller naming the abandonment
// can print them concrete instead of sending the operator to look them up.
type CompactAbandonEligibility struct {
	Eligible         bool
	Revision         string
	SnapshotIdentity string
}

// InspectCompactPristineAbandonment answers, read-only, whether
// AbandonPristineCompactStore would accept this lineage. It applies the same
// pristineness rule, the same residue rule, the same superseded rule and the
// same remaining-graph rule that gate the real operation, and takes no lock and
// writes nothing.
//
// It is fail-closed by construction: every unreadable, missing, mixed-store,
// non-pristine, artifact-holding, superseded or graph-breaking target answers
// not eligible. The maintainer authorization and the compare-and-swap are
// deliberately NOT predicted — they are the operator's own act, and a probe
// that claimed to know them would be claiming the maintainer had already
// decided.
func InspectCompactPristineAbandonment(ctx context.Context, repo, lineage string) (CompactAbandonEligibility, error) {
	if err := ctx.Err(); err != nil {
		return CompactAbandonEligibility{}, err
	}
	if err := validateLineageID(lineage); err != nil {
		return CompactAbandonEligibility{}, nil
	}
	base, _, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return CompactAbandonEligibility{}, err
	}
	dir := filepath.Join(base, "v2", lineage)
	record, err := (CompactStore{Dir: dir, lineageID: lineage}).Load()
	if err != nil {
		return CompactAbandonEligibility{}, nil
	}
	if _, statErr := os.Stat(filepath.Join(base, "v1", lineage)); statErr == nil || !os.IsNotExist(statErr) {
		return CompactAbandonEligibility{}, nil
	}
	if !compactAbandonablePristineState(record.State) {
		return CompactAbandonEligibility{}, nil
	}
	items, err := os.ReadDir(dir)
	if err != nil {
		return CompactAbandonEligibility{}, nil
	}
	for _, item := range items {
		if item.Name() != compactStateFileName && compactAuthoritativeArtifact(item.Name()) {
			return CompactAbandonEligibility{}, nil
		}
	}
	stores, err := DiscoverCompactStores(ctx, repo)
	if err != nil {
		return CompactAbandonEligibility{}, err
	}
	records := make(map[string]CompactRecord, len(stores))
	for _, related := range stores {
		relatedRecord, loadErr := related.Load()
		if loadErr != nil {
			return CompactAbandonEligibility{}, nil
		}
		records[relatedRecord.State.LineageID] = relatedRecord
	}
	for _, related := range records {
		if related.State.Recovery != nil && related.State.Recovery.PredecessorLineageID == lineage {
			return CompactAbandonEligibility{}, nil
		}
	}
	if graphErr := compactAuthorityRemovalRegression(records, compactRecordsWithout(records, lineage)); graphErr != nil {
		return CompactAbandonEligibility{}, nil
	}
	return CompactAbandonEligibility{
		Eligible: true, Revision: record.Revision, SnapshotIdentity: record.State.InitialSnapshot.Identity,
	}, nil
}

// AbandonPristineCompactStore quarantines one compact-v2 review lineage under
// either of two classes.
//
// The default class is pristine: a reviewing authority that never captured lens
// results, findings, corrections, or evidence, or an invalidated authority
// whose underlying reviewing projection is equally pristine.
//
// The second class, selected only by CompactAbandonRequest.IncompleteInspection
// and its own authorization schema, retires a reviewing lineage whose selected
// plan can never finish: some lenses captured a result and at least one never
// could, so the negotiated route would otherwise ask for the missing result
// forever with no terminal state reachable. It discards the review — it never
// approves it — and names the captured and uncaptured lenses in the audit
// proof so the destroyed work stays visible. The
// entry moves whole — never deleted — into the audited quarantine together
// with the re-derived proof, so it leaves the walked inventory without
// destroying history. Terminal, corrected, artifact-holding, superseded, and
// stale-revision targets are refused, as is any move that would invalidate
// the remaining authority graph. Pristineness is proven from persisted bytes
// and store topology alone; the live worktree is never consulted. An exact
// replay of a committed abandonment converges on the committed record.
func AbandonPristineCompactStore(ctx context.Context, repo string, request CompactAbandonRequest) (CompactReclaimRecord, error) {
	if err := ctx.Err(); err != nil {
		return CompactReclaimRecord{}, err
	}
	if err := validateLineageID(request.LineageID); err != nil {
		return CompactReclaimRecord{}, err
	}
	if strings.TrimSpace(request.Reason) == "" || strings.TrimSpace(request.Actor) == "" {
		return CompactReclaimRecord{}, errors.New("review abandon requires a non-empty reason and actor")
	}
	if strings.TrimSpace(request.ExpectedRevision) == "" {
		return CompactReclaimRecord{}, errors.New("review abandon requires the exact current store revision")
	}
	base, _, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	versionRoot := filepath.Join(base, "v2")
	dir := filepath.Join(versionRoot, request.LineageID)
	lock, err := acquireStoreLock(filepath.Join(versionRoot, "LOCK"))
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	defer lock.release()
	store := CompactStore{Dir: dir, lineageID: request.LineageID}
	record, err := store.Load()
	if err != nil {
		if os.IsNotExist(err) {
			return replayCommittedCompactAbandonment(base, request)
		}
		return CompactReclaimRecord{}, fmt.Errorf("load abandon target: %w", err)
	}
	if _, statErr := os.Stat(filepath.Join(base, "v1", request.LineageID)); statErr == nil {
		return CompactReclaimRecord{}, fmt.Errorf("review abandon refused: lineage %q also exists in the legacy-v1 authority store; mixed-store collisions require explicit maintainer reconciliation", request.LineageID)
	} else if !os.IsNotExist(statErr) {
		return CompactReclaimRecord{}, fmt.Errorf("inspect same-lineage legacy authority: %w", statErr)
	}
	// The incomplete-inspection class is deliberately narrower than the
	// pristine one in every dimension except the reviewer-results residue it
	// exists to tolerate: reviewing only (never invalidated), state still
	// pristine, and strictly between one and all-but-one lenses captured. A
	// fully captured plan is refused on purpose — that review can finalize, and
	// letting it be abandoned would turn this into a way to drop findings.
	var capturedLenses, uncapturedLenses []string
	if request.IncompleteInspection {
		if record.State.State != StateReviewing {
			return CompactReclaimRecord{}, fmt.Errorf("review abandon refused: incomplete-inspection abandonment applies only to a reviewing lineage; %q holds %q authority. See where this review actually is with `gentle-ai review status --cwd %q --lineage %s`",
				request.LineageID, record.State.State, repo, request.LineageID)
		}
		if !compactAbandonablePristineState(record.State) {
			return CompactReclaimRecord{}, fmt.Errorf("review abandon refused: reviewing lineage %q is not pristine; it carries review or correction data", request.LineageID)
		}
		capturedLenses, uncapturedLenses, err = compactCapturedLensPartition(dir, record.State.SelectedLenses)
		if err != nil {
			return CompactReclaimRecord{}, fmt.Errorf("inspect incomplete review capture: %w", err)
		}
		if len(capturedLenses) == 0 {
			return CompactReclaimRecord{}, fmt.Errorf("review abandon refused: lineage %q captured no reviewer result, so it is an ordinary pristine abandonment. Re-run without --incomplete-inspection; `gentle-ai review abandon` with no flags prints that class's template and where every value is read",
				request.LineageID)
		}
		if len(uncapturedLenses) == 0 {
			return CompactReclaimRecord{}, fmt.Errorf("review abandon refused: lineage %q captured every selected lens and can finalize; incomplete-inspection abandonment never discards a complete review. Continue it with `gentle-ai review status --cwd %q --lineage %s --next-transition`",
				request.LineageID, repo, request.LineageID)
		}
	} else {
		switch record.State.State {
		case StateReviewing:
			if !compactAbandonablePristineState(record.State) {
				return CompactReclaimRecord{}, fmt.Errorf("review abandon refused: reviewing lineage %q is not pristine; it carries review or correction data", request.LineageID)
			}
		case StateInvalidated:
			if !compactAbandonablePristineState(record.State) {
				return CompactReclaimRecord{}, fmt.Errorf("review abandon refused: invalidated lineage %q does not project to a pristine reviewing authority", request.LineageID)
			}
		default:
			return CompactReclaimRecord{}, fmt.Errorf("review abandon refused: lineage %q holds %q authority; only a pristine reviewing or pristine invalidated lineage may be abandoned", request.LineageID, record.State.State)
		}
	}
	items, err := os.ReadDir(dir)
	if err != nil {
		return CompactReclaimRecord{}, fmt.Errorf("inspect abandon target: %w", err)
	}
	residue := make([]string, 0, len(items))
	for _, item := range items {
		// The captured reviewer results are the one authoritative artifact the
		// incomplete class tolerates, because tolerating it IS the class. A
		// receipt, a finalize journal, or an in-flight atomic write still
		// refuses here: those mean the lineage moved past reviewing, and this
		// class only ever retires a review that never reached a verdict.
		if request.IncompleteInspection && item.Name() == CompactReviewerResultsDir {
			residue = append(residue, item.Name())
			continue
		}
		if item.Name() != compactStateFileName && compactAuthoritativeArtifact(item.Name()) {
			// Refusing is correct: abandoning an entry that holds captured
			// review work would discard it. But the refusal has to leave the
			// operator somewhere. `review reclaim` already ends its own
			// cascade this way, and when reconciliation, reclaim, classified
			// repair, invalidate and this all refuse in turn, the diagnosis
			// plus escalation is the only honest exit left.
			return CompactReclaimRecord{}, fmt.Errorf("review abandon refused: store entry %q holds authoritative artifact %q beyond its pristine state,"+
				" and abandoning it would discard captured review work."+
				" Nothing quarantines this shape today; the entry stays exactly as persisted."+
				" Capture the complete machine-readable diagnosis with `gentle-ai review inspect-authority --cwd %q` and escalate that report",
				request.LineageID, item.Name(), repo)
		}
		residue = append(residue, item.Name())
	}
	if record.Revision != request.ExpectedRevision {
		return CompactReclaimRecord{}, fmt.Errorf("%w: expected compact revision %q, current %q", ErrConcurrentUpdate, request.ExpectedRevision, record.Revision)
	}
	if request.IncompleteInspection {
		if request.MaintainerAuthorization != compactIncompleteAbandonAuthorizationBinding(
			request.LineageID, record.Revision, record.State.InitialSnapshot.Identity, capturedLenses, request.Actor, request.Reason) {
			// refusal:by-design human-authority: the binding is the maintainer's own claim that they read this lineage's frozen state and accept discarding the captured lenses. Emitting the token here would remove the very act the gate exists to require.
			return CompactReclaimRecord{}, fmt.Errorf("review abandon requires an exact maintainer authorization binding (schema %s over lineage %s@%s, snapshot %s and captured lenses %s)",
				CompactIncompleteAbandonAuthorizationSchema, request.LineageID, record.Revision, record.State.InitialSnapshot.Identity, strings.Join(capturedLenses, ","))
		}
	} else if request.MaintainerAuthorization != compactAbandonAuthorizationBinding(request.LineageID, record.Revision, record.State.InitialSnapshot.Identity, request.Actor, request.Reason) {
		return CompactReclaimRecord{}, fmt.Errorf("review abandon requires an exact maintainer authorization binding (schema %s over lineage %s@%s and snapshot %s)",
			CompactAbandonAuthorizationSchema, request.LineageID, record.Revision, record.State.InitialSnapshot.Identity)
	}
	stores, err := DiscoverCompactStores(ctx, repo)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	records := make(map[string]CompactRecord, len(stores))
	for _, related := range stores {
		relatedRecord, loadErr := related.Load()
		if loadErr != nil {
			return CompactReclaimRecord{}, fmt.Errorf("review abandon refused: related compact authority %q does not load: %w", related.lineageID, loadErr)
		}
		records[relatedRecord.State.LineageID] = relatedRecord
	}
	for lineage, related := range records {
		if related.State.Recovery != nil && related.State.Recovery.PredecessorLineageID == request.LineageID {
			return CompactReclaimRecord{}, fmt.Errorf("review abandon refused: lineage %q is superseded by recovery successor %q; superseded history is never abandoned", request.LineageID, lineage)
		}
	}
	if err := compactAuthorityRemovalRegression(records, compactRecordsWithout(records, request.LineageID)); err != nil {
		return CompactReclaimRecord{}, fmt.Errorf("review abandon refused: %w", err)
	}
	sort.Strings(residue)
	if request.AbandonedAt.IsZero() {
		request.AbandonedAt = time.Now().UTC()
	}
	quarantined := CompactReclaimRecord{
		Schema: CompactReclaimRecordSchema, Status: CompactReclaimPrepared, LineageID: request.LineageID,
		Reason: strings.TrimSpace(request.Reason), Actor: strings.TrimSpace(request.Actor),
		ReclaimedAt: request.AbandonedAt.UTC(), SourcePath: dir, Residue: residue,
	}
	if request.IncompleteInspection {
		quarantined.IncompleteAbandonment = &CompactIncompleteAbandonmentProof{
			LineageID: request.LineageID, Revision: record.Revision,
			SnapshotIdentity: record.State.InitialSnapshot.Identity,
			CapturedLenses:   capturedLenses, UncapturedLenses: uncapturedLenses,
		}
	} else {
		quarantined.PristineAbandonment = &CompactPristineAbandonmentProof{
			LineageID: request.LineageID, Revision: record.Revision,
			SnapshotIdentity: record.State.InitialSnapshot.Identity,
			State:            record.State.State, InvalidationReason: record.State.InvalidationReason,
		}
	}
	return quarantineCompactStoreEntry(ctx, base, dir, quarantined)
}

// replayCommittedCompactAbandonment resolves an abandon request whose lineage
// no longer holds a v2 entry: an exact replay finishes a prepared or
// re-emits the committed record so repeated identical requests converge
// without minting a second quarantine; anything else is refused.
func replayCommittedCompactAbandonment(base string, request CompactAbandonRequest) (CompactReclaimRecord, error) {
	quarantineRoot := filepath.Join(base, "quarantine")
	entries, err := os.ReadDir(quarantineRoot)
	if err != nil && !os.IsNotExist(err) {
		return CompactReclaimRecord{}, fmt.Errorf("inspect quarantine for abandonment replay: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		payload, readErr := os.ReadFile(filepath.Join(quarantineRoot, entry.Name(), "reclaim-record.json"))
		if readErr != nil {
			continue
		}
		var record CompactReclaimRecord
		if json.Unmarshal(payload, &record) != nil {
			continue
		}
		if record.Status != CompactReclaimCommitted && (record.Status != CompactReclaimPrepared || !request.IncompleteInspection) {
			continue
		}
		// Each class replays only against its own proof, so a
		// pristine token can never converge onto an incomplete-inspection
		// record (or the reverse) and report success for an abandonment the
		// maintainer did not authorize.
		var lineageID, revision, snapshotIdentity, binding string
		if request.IncompleteInspection {
			proof := record.IncompleteAbandonment
			if proof == nil {
				continue
			}
			lineageID, revision, snapshotIdentity = proof.LineageID, proof.Revision, proof.SnapshotIdentity
			binding = compactIncompleteAbandonAuthorizationBinding(lineageID, revision, snapshotIdentity, proof.CapturedLenses, request.Actor, request.Reason)
		} else {
			proof := record.PristineAbandonment
			if proof == nil {
				continue
			}
			lineageID, revision, snapshotIdentity = proof.LineageID, proof.Revision, proof.SnapshotIdentity
			binding = compactAbandonAuthorizationBinding(lineageID, revision, snapshotIdentity, request.Actor, request.Reason)
		}
		if lineageID != request.LineageID || revision != request.ExpectedRevision ||
			record.Reason != strings.TrimSpace(request.Reason) || record.Actor != strings.TrimSpace(request.Actor) {
			continue
		}
		if request.MaintainerAuthorization != binding {
			continue
		}
		if record.Status == CompactReclaimPrepared {
			quarantineDir := filepath.Join(quarantineRoot, entry.Name())
			residue, statErr := os.Lstat(filepath.Join(quarantineDir, "residue"))
			if record.LineageID != request.LineageID || record.SourcePath != filepath.Join(base, "v2", request.LineageID) || record.QuarantinePath != quarantineDir || statErr != nil || !residue.IsDir() {
				continue
			}
			record.Status = CompactReclaimCommitted
			if persistErr := persistReclaimRecord(record); persistErr != nil {
				return record, fmt.Errorf("commit prepared abandonment replay: %w", persistErr)
			}
		}
		return record, nil
	}
	return CompactReclaimRecord{}, fmt.Errorf("review abandon refused: lineage %q holds no compact authority state and no committed abandonment record matches the request; if a prior abandonment was interrupted, the prepared reclaim-record.json under %s locates the moved residue for manual reconciliation", request.LineageID, quarantineRoot)
}
