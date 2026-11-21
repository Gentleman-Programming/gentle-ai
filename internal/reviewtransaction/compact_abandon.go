package reviewtransaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// CompactAbandonAuthorizationSchema is the schema string stamped on the
// CompactAbandonmentProof of every abandonment audit record. Abandonment no
// longer takes a maintainer authorization binding (T7, rdd-simplification L9);
// the name is frozen because persisted audit records and the replay below
// recognize it, and contract versions are not bumped.
const CompactAbandonAuthorizationSchema = "gentle-ai.review-abandon-authorization/v2"

// CompactAbandonRequest identifies one review lineage to declare lost and
// quarantine. There is deliberately no expected revision and no authorization
// binding: the caller holds the v2 store lock for the whole operation, so no
// writer can change the lineage between the read and the quarantine, and the
// quarantine byte-preserves everything it moves.
type CompactAbandonRequest struct {
	LineageID   string
	Reason      string
	Actor       string
	AbandonedAt time.Time
}

// CompactAbandonResult is the outcome of one abandonment. The embedded record
// is the requested lineage's own v2 quarantine (or, for a lineage that only
// exists as a legacy-v1 entry, that entry's). Cascaded lists every other entry
// quarantined with it: recovery successors, leaf first, and a same-lineage
// legacy-v1 entry.
type CompactAbandonResult struct {
	CompactReclaimRecord
	Cascaded []CompactReclaimRecord
}

// CompactPristineAbandonmentProof remains only to decode pre-v2 quarantine
// records during historical audit readback. New abandonments always write
// CompactAbandonmentProof.
type CompactPristineAbandonmentProof struct {
	LineageID          string `json:"lineage_id"`
	Revision           string `json:"revision"`
	SnapshotIdentity   string `json:"snapshot_identity"`
	State              State  `json:"state"`                         // "reviewing" or "invalidated"
	InvalidationReason string `json:"invalidation_reason,omitempty"` // when pre-invalidated
}

// CompactIncompleteAbandonmentProof remains only to decode pre-v2 quarantine
// records during historical audit readback. New abandonments always write
// CompactAbandonmentProof.
type CompactIncompleteAbandonmentProof struct {
	LineageID        string   `json:"lineage_id"`
	Revision         string   `json:"revision"`
	SnapshotIdentity string   `json:"snapshot_identity"`
	CapturedLenses   []string `json:"captured_lenses"`
	UncapturedLenses []string `json:"uncaptured_lenses"`
}

const (
	CompactAbandonReasonOperatorDisposition = "operator_disposition"
	CompactAbandonReasonRetiredSchema       = "retired_schema"
)

// CompactDiscardedWorkSummary describes the review work an abandonment
// discards. Unverifiable is set when the summary could not be derived (the
// record is unreadable, absent, or its captured results no longer re-admit):
// abandonment never refuses over that, it records that the loss is unknown.
type CompactDiscardedWorkSummary struct {
	CapturedLensResults []string `json:"captured_lens_results"`
	FindingsPresent     bool     `json:"findings_present"`
	Unverifiable        bool     `json:"unverifiable,omitempty"`
}

type CompactAbandonmentProof struct {
	Schema           string                      `json:"schema"`
	LineageID        string                      `json:"lineage_id"`
	Revision         string                      `json:"revision"`
	SnapshotIdentity string                      `json:"snapshot_identity"`
	DiscardedWork    CompactDiscardedWorkSummary `json:"discarded_work"`
}

// compactAbandonDiscardedWork derives the discarded-work summary and degrades
// to "unverifiable" instead of failing: abandonment is the exit for exactly
// the records whose captured evidence can no longer be re-admitted.
func compactAbandonDiscardedWork(ctx context.Context, store CompactStore, record CompactRecord) (CompactDiscardedWorkSummary, error) {
	if err := ctx.Err(); err != nil {
		return CompactDiscardedWorkSummary{}, err
	}
	summary, err := compactDiscardedWorkSummary(ctx, store, record)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return CompactDiscardedWorkSummary{}, ctxErr
		}
		return CompactDiscardedWorkSummary{CapturedLensResults: []string{}, Unverifiable: true}, nil
	}
	return summary, nil
}

func compactDiscardedWorkSummary(ctx context.Context, store CompactStore, record CompactRecord) (CompactDiscardedWorkSummary, error) {
	view, err := record.State.CompactReviewView()
	if err != nil {
		return CompactDiscardedWorkSummary{}, fmt.Errorf("derive admitted discarded work: %w", err)
	}
	captured := make([]string, 0, len(record.State.AdmittedRoleResults))
	for _, entry := range record.State.AdmittedRoleResults {
		if !record.State.IsAccountingOnlyAdmittedRoleResult(entry) && entry.Role == CompactRoleLens {
			captured = append(captured, fmt.Sprintf("%02d-%s", entry.SelectedOrder, entry.Lens))
		}
	}
	sort.Strings(captured)
	summary := CompactDiscardedWorkSummary{CapturedLensResults: captured, FindingsPresent: len(view.Findings) > 0}
	if len(captured) == 0 {
		return summary, nil
	}
	if store.repo == "" {
		return CompactDiscardedWorkSummary{}, errors.New("reviewer result store has no repository") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	builder := SnapshotBuilder{Repo: store.repo}
	frozen, err := builder.FrozenCandidateContext(ctx, record.State.InitialSnapshot)
	if err != nil {
		return CompactDiscardedWorkSummary{}, fmt.Errorf("derive frozen reviewer context: %w", err)
	}
	for _, entry := range record.State.AdmittedRoleResults {
		if entry.Role != CompactRoleLens {
			continue
		}
		value, err := canonicalCompactRoleValue(entry.Value)
		if err != nil || compactPreservedPayloadDigest(append(value, '\n')) != entry.ArtifactDigest {
			return CompactDiscardedWorkSummary{}, errors.New("admitted reviewer record value is invalid") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		var envelope compactAdmittedReviewerResult
		decoder := json.NewDecoder(bytes.NewReader(value))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&envelope); err != nil || decoder.Decode(new(struct{})) != io.EOF {
			return CompactDiscardedWorkSummary{}, errors.New("decode admitted reviewer record value") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		nativeFrozen, expected, err := artifactSubjectForSchema(ctx, builder, record.State, entry.CapturePhaseRevision,
			frozen, entry.Lens, entry.SelectedOrder, envelope.Subject.CorrectionTargetIdentity, envelope.Subject.Schema)
		if err != nil || envelope.Schema != admittedReviewerResultSchemaForSubject(expected) || envelope.Subject != expected ||
			envelope.Admission.Validate(expected, append(append([]byte(nil), envelope.Result...), '\n')) != nil {
			return CompactDiscardedWorkSummary{}, errors.New("admit captured reviewer record value") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		result, admitted := reAdmitCompactReviewerResult(ctx, envelope, expected, nativeFrozen)
		if !admitted || result.ResultHash != entry.ResultHash {
			return CompactDiscardedWorkSummary{}, errors.New("admit captured reviewer record value") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
	}
	return summary, nil
}

// CompactAbandonEligibility reports whether `review abandon` would accept one
// lineage right now. Revision, SnapshotIdentity and DiscardedWork are filled
// only when the record is readable; they describe what the abandonment would
// discard and are informational, never a binding the caller must echo back.
type CompactAbandonEligibility struct {
	Eligible         bool
	Revision         string
	SnapshotIdentity string
	DiscardedWork    CompactDiscardedWorkSummary
}

// InspectCompactPristineAbandonment answers, read-only, whether
// AbandonCompactStore would act on this lineage: abandonment accepts any
// lineage that holds a v2 or legacy-v1 entry, whatever its state or
// readability. It takes no lock and writes nothing.
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
	v2Present, err := compactAbandonEntryPresent(dir)
	if err != nil {
		return CompactAbandonEligibility{}, err
	}
	v1Present, err := compactAbandonEntryPresent(filepath.Join(base, "v1", lineage))
	if err != nil {
		return CompactAbandonEligibility{}, err
	}
	if !v2Present && !v1Present {
		return CompactAbandonEligibility{}, nil
	}
	eligibility := CompactAbandonEligibility{Eligible: true}
	if v2Present {
		store := CompactStore{Dir: dir, lineageID: lineage, repo: repo}
		if record, loadErr := store.Load(); loadErr == nil {
			eligibility.Revision = record.Revision
			eligibility.SnapshotIdentity = record.State.InitialSnapshot.Identity
			if discarded, summaryErr := compactDiscardedWorkSummary(ctx, store, record); summaryErr == nil {
				eligibility.DiscardedWork = discarded
			} else {
				eligibility.DiscardedWork = CompactDiscardedWorkSummary{CapturedLensResults: []string{}, Unverifiable: true}
			}
		}
	}
	return eligibility, nil
}

func compactAbandonEntryPresent(path string) (bool, error) {
	if _, err := os.Lstat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect abandon target: %w", err)
	}
	return true, nil
}

// compactAbandonDescendants returns every recovery successor reachable from
// lineage through readable records, deepest first, so removing them in the
// returned order never leaves a successor whose predecessor is already gone.
func compactAbandonDescendants(records map[string]CompactRecord, lineage string) []string {
	children := map[string][]string{}
	for candidate, record := range records {
		if record.State.Recovery != nil {
			parent := record.State.Recovery.PredecessorLineageID
			children[parent] = append(children[parent], candidate)
		}
	}
	depth := map[string]int{lineage: 0}
	queue := []string{lineage}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		next := children[current]
		sort.Strings(next)
		for _, child := range next {
			if _, seen := depth[child]; seen {
				continue
			}
			depth[child] = depth[current] + 1
			queue = append(queue, child)
		}
	}
	delete(depth, lineage)
	ordered := make([]string, 0, len(depth))
	for descendant := range depth {
		ordered = append(ordered, descendant)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if depth[ordered[i]] != depth[ordered[j]] {
			return depth[ordered[i]] > depth[ordered[j]]
		}
		return ordered[i] < ordered[j]
	})
	return ordered
}

type compactAbandonStep struct {
	lineage string
	dir     string
	residue []string
	proof   CompactAbandonmentProof
}

func compactAbandonResidue(dir string) ([]string, error) {
	items, err := os.ReadDir(dir)
	if err != nil {
		if info, statErr := os.Lstat(dir); statErr == nil && !info.IsDir() {
			return []string{}, nil
		}
		return nil, fmt.Errorf("inspect abandon target: %w", err)
	}
	residue := make([]string, 0, len(items))
	for _, item := range items {
		residue = append(residue, item.Name())
	}
	sort.Strings(residue)
	return residue, nil
}

// AbandonCompactStore declares one review lineage lost and quarantines it, with
// no human authorization binding. It accepts every state:
//
//   - any lifecycle state, including terminal approved, escalated and
//     invalidated authority;
//   - an unreadable or missing review-state.json (including an interrupted
//     START), quarantined whole without interpreting it;
//   - a lineage that is the predecessor of recovery successors: the
//     successors are abandoned with it, deepest first, so no orphan recovery
//     edge ever remains. Ancestors and unrelated lineages are never touched;
//   - a same-lineage legacy-v1 entry, quarantined alongside it, or a lineage
//     that exists only as a legacy-v1 entry.
//
// A discarded-work summary that cannot be derived degrades to "unverifiable".
// The v2 store lock is taken without waiting: a live holder fails the call with
// the lock-busy error before anything moves. Every entry is byte-preserved in
// the audited quarantine; nothing is deleted. On a partial failure the records
// already committed (and the prepared record in flight) are returned alongside
// the error.
func AbandonCompactStore(ctx context.Context, repo string, request CompactAbandonRequest) (CompactAbandonResult, error) {
	if err := ctx.Err(); err != nil {
		return CompactAbandonResult{}, err
	}
	if err := validateLineageID(request.LineageID); err != nil {
		return CompactAbandonResult{}, err
	}
	if strings.TrimSpace(request.Reason) == "" || strings.TrimSpace(request.Actor) == "" {
		return CompactAbandonResult{}, errors.New("review abandon requires a non-empty reason and actor")
	}
	base, _, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return CompactAbandonResult{}, err
	}
	versionRoot := filepath.Join(base, "v2")
	dir := filepath.Join(versionRoot, request.LineageID)
	v1Dir := filepath.Join(base, "v1", request.LineageID)
	lock, err := acquireStoreLock(filepath.Join(versionRoot, "LOCK"))
	if err != nil {
		return CompactAbandonResult{}, err
	}
	defer lock.release()
	v2Present, err := compactAbandonEntryPresent(dir)
	if err != nil {
		return CompactAbandonResult{}, err
	}
	v1Present, err := compactAbandonEntryPresent(v1Dir)
	if err != nil {
		return CompactAbandonResult{}, err
	}
	if !v2Present && !v1Present {
		replayed, replayErr := replayCommittedCompactAbandonment(base, request)
		return CompactAbandonResult{CompactReclaimRecord: replayed}, replayErr
	}
	if v1Present {
		// A legacy lineage lock is its own kernel lock, not covered by the v2
		// one. Probe without creating anything: a live holder fails fast.
		if status := probeAuthorityRepairLock(filepath.Join(v1Dir, "LOCK")); status == authorityRepairLockHeld || status == authorityRepairLockUnknown {
			return CompactAbandonResult{}, storeLockBusyError{}
		}
	}

	var steps []compactAbandonStep
	if v2Present {
		scan, scanErr := scanCompactAuthority(ctx, repo)
		if scanErr != nil {
			return CompactAbandonResult{}, scanErr
		}
		victims := compactAbandonDescendants(scan.records, request.LineageID)
		for _, lineage := range victims {
			record := scan.records[lineage]
			discarded, summaryErr := compactAbandonDiscardedWork(ctx, scan.stores[lineage], record)
			if summaryErr != nil {
				return CompactAbandonResult{}, summaryErr
			}
			residue, residueErr := compactAbandonResidue(scan.stores[lineage].Dir)
			if residueErr != nil {
				return CompactAbandonResult{}, residueErr
			}
			steps = append(steps, compactAbandonStep{lineage: lineage, dir: scan.stores[lineage].Dir, residue: residue, proof: CompactAbandonmentProof{
				Schema: CompactAbandonAuthorizationSchema, LineageID: lineage, Revision: record.Revision,
				SnapshotIdentity: record.State.InitialSnapshot.Identity, DiscardedWork: discarded,
			}})
		}
		target := compactAbandonStep{lineage: request.LineageID, dir: dir, proof: CompactAbandonmentProof{
			Schema: CompactAbandonAuthorizationSchema, LineageID: request.LineageID,
			DiscardedWork: CompactDiscardedWorkSummary{CapturedLensResults: []string{}, Unverifiable: true},
		}}
		store := CompactStore{Dir: dir, lineageID: request.LineageID, repo: repo}
		record, loadErr := store.LoadContext(ctx)
		switch {
		case loadErr == nil:
			discarded, summaryErr := compactAbandonDiscardedWork(ctx, store, record)
			if summaryErr != nil {
				return CompactAbandonResult{}, summaryErr
			}
			target.proof.Revision = record.Revision
			target.proof.SnapshotIdentity = record.State.InitialSnapshot.Identity
			target.proof.DiscardedWork = discarded
		case errors.Is(loadErr, os.ErrNotExist) || !IsCompactAuthorityOperationalFailure(loadErr):
			// Missing review-state.json (an interrupted START) or a record that
			// cannot be read: quarantined whole without being interpreted.
		default:
			return CompactAbandonResult{}, fmt.Errorf("load abandon target: %w", loadErr)
		}
		if target.residue, err = compactAbandonResidue(dir); err != nil {
			return CompactAbandonResult{}, err
		}
		steps = append(steps, target)
		// The cascade removes every readable successor of the target, so the
		// remaining graph can only have lost edges. Verify that invariant
		// before anything moves instead of trusting it.
		removed := scan.records
		for _, step := range steps {
			removed = compactRecordsWithout(removed, step.lineage)
		}
		if graphErr := compactAuthorityRemovalRegression(scan.records, removed); graphErr != nil {
			return CompactAbandonResult{}, fmt.Errorf("review abandon refused: %w", graphErr)
		}
	}
	if v1Present {
		residue, residueErr := compactAbandonResidue(v1Dir)
		if residueErr != nil {
			return CompactAbandonResult{}, residueErr
		}
		steps = append(steps, compactAbandonStep{lineage: request.LineageID, dir: v1Dir, residue: residue, proof: CompactAbandonmentProof{
			Schema: CompactAbandonAuthorizationSchema, LineageID: request.LineageID,
			DiscardedWork: CompactDiscardedWorkSummary{CapturedLensResults: []string{}, Unverifiable: true},
		}})
	}

	if request.AbandonedAt.IsZero() {
		request.AbandonedAt = time.Now().UTC()
	}
	records := make([]CompactReclaimRecord, 0, len(steps))
	for _, step := range steps {
		proof := step.proof
		quarantined, quarantineErr := quarantineCompactStoreEntry(ctx, base, step.dir, CompactReclaimRecord{
			Schema: CompactReclaimRecordSchema, Status: CompactReclaimPrepared, LineageID: step.lineage,
			Reason: strings.TrimSpace(request.Reason), Actor: strings.TrimSpace(request.Actor),
			ReclaimedAt: request.AbandonedAt.UTC(), SourcePath: step.dir, Residue: step.residue, Abandonment: &proof,
		})
		if quarantineErr != nil {
			return compactAbandonResult(request.LineageID, base, append(records, quarantined)), quarantineErr
		}
		records = append(records, quarantined)
	}
	return compactAbandonResult(request.LineageID, base, records), nil
}

// compactAbandonResult picks the requested lineage's own v2 record as the
// primary one; everything else is reported as cascaded, in quarantine order.
func compactAbandonResult(lineage, base string, records []CompactReclaimRecord) CompactAbandonResult {
	primary := -1
	for index, record := range records {
		if record.LineageID == lineage && record.SourcePath == filepath.Join(base, "v2", lineage) {
			primary = index
		}
	}
	if primary < 0 {
		for index, record := range records {
			if record.LineageID == lineage {
				primary = index
			}
		}
	}
	if primary < 0 {
		return CompactAbandonResult{CompactReclaimRecord: records[len(records)-1], Cascaded: records[:len(records)-1]}
	}
	result := CompactAbandonResult{CompactReclaimRecord: records[primary]}
	for index, record := range records {
		if index != primary {
			result.Cascaded = append(result.Cascaded, record)
		}
	}
	return result
}

// replayCommittedCompactAbandonment resolves an abandon request whose lineage
// holds no entry any more: an exact replay (same lineage, reason and actor)
// re-emits the committed record so repeated identical requests converge
// without minting a second quarantine; anything else is refused.
func replayCommittedCompactAbandonment(base string, request CompactAbandonRequest) (CompactReclaimRecord, error) {
	quarantineRoot := filepath.Join(base, "quarantine")
	entries, err := os.ReadDir(quarantineRoot)
	if err != nil && !os.IsNotExist(err) {
		return CompactReclaimRecord{}, fmt.Errorf("inspect quarantine for abandonment replay: %w", err)
	}
	var matched *CompactReclaimRecord
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		payload, readErr := os.ReadFile(filepath.Join(quarantineRoot, entry.Name(), "reclaim-record.json"))
		if readErr != nil {
			continue
		}
		var record CompactReclaimRecord
		if json.Unmarshal(payload, &record) != nil || record.Status != CompactReclaimCommitted {
			continue
		}
		proof := record.Abandonment
		if proof == nil || proof.Schema != CompactAbandonAuthorizationSchema || proof.LineageID != request.LineageID ||
			record.Reason != strings.TrimSpace(request.Reason) || record.Actor != strings.TrimSpace(request.Actor) {
			continue
		}
		if matched == nil || record.SourcePath == filepath.Join(base, "v2", request.LineageID) {
			copied := record
			matched = &copied
		}
	}
	if matched != nil {
		return *matched, nil
	}
	// refusal:by-design operator-knowledge: the named lineage is absent; the operator must name an existing one, which a flagless `gentle-ai review abandon --cwd <repo>` lists
	return CompactReclaimRecord{}, fmt.Errorf("review abandon refused: lineage %q holds no review authority entry and no committed abandonment record matches the request; if a prior abandonment was interrupted, the prepared reclaim-record.json under %s locates the moved residue for manual reconciliation", request.LineageID, quarantineRoot)
}
