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

// compactAbandonAuthorizationSchema is the first line of the exact six-line
// LF-only abandonment maintainer authorization binding; the remaining lines
// are, in order: lineage, revision, snapshot_identity, actor, and reason.
const compactAbandonAuthorizationSchema = "gentle-ai.review-abandon-authorization/v1"

// CompactAbandonRequest identifies one pristine compact-v2 review lineage to
// quarantine, together with the exact maintainer authorization binding for
// that content.
type CompactAbandonRequest struct {
	LineageID               string
	ExpectedRevision        string
	Reason                  string
	Actor                   string
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

func compactAbandonAuthorizationBinding(lineage, revision, snapshotIdentity, actor, reason string) string {
	return compactAbandonAuthorizationSchema + "\nlineage=" + lineage + "\nrevision=" + revision +
		"\nsnapshot_identity=" + snapshotIdentity +
		"\nactor=" + strings.TrimSpace(actor) + "\nreason=" + strings.TrimSpace(reason)
}

// AbandonPristineCompactStore quarantines one compact-v2 review lineage that
// natively re-derives as pristine: a reviewing authority that never captured
// lens results, findings, corrections, or evidence, or an invalidated
// authority whose underlying reviewing projection is equally pristine. The
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
	switch record.State.State {
	case StateReviewing:
		if !compactPristineReviewing(record.State) {
			return CompactReclaimRecord{}, fmt.Errorf("review abandon refused: reviewing lineage %q is not pristine; it carries review or correction data", request.LineageID)
		}
	case StateInvalidated:
		// compactPristineReviewing hard-requires State == StateReviewing and an
		// empty InvalidationReason, so the invalidated record must be projected
		// back onto its underlying reviewing authority before the pristineness
		// re-derivation; resetting exactly those two fields is load-bearing. The
		// paired non-empty-InvalidationReason check doubles as a structural
		// corruption guard: a persisted invalidated state without a reason never
		// came from Invalidate.
		reviewing := record.State
		reviewing.State, reviewing.InvalidationReason = StateReviewing, ""
		if strings.TrimSpace(record.State.InvalidationReason) == "" || !compactPristineReviewing(reviewing) {
			return CompactReclaimRecord{}, fmt.Errorf("review abandon refused: invalidated lineage %q does not project to a pristine reviewing authority", request.LineageID)
		}
	default:
		return CompactReclaimRecord{}, fmt.Errorf("review abandon refused: lineage %q holds %q authority; only a pristine reviewing or pristine invalidated lineage may be abandoned", request.LineageID, record.State.State)
	}
	items, err := os.ReadDir(dir)
	if err != nil {
		return CompactReclaimRecord{}, fmt.Errorf("inspect abandon target: %w", err)
	}
	residue := make([]string, 0, len(items))
	for _, item := range items {
		if item.Name() != compactStateFileName && compactAuthoritativeArtifact(item.Name()) {
			return CompactReclaimRecord{}, fmt.Errorf("review abandon refused: store entry %q holds authoritative artifact %q beyond its pristine state", request.LineageID, item.Name())
		}
		residue = append(residue, item.Name())
	}
	if record.Revision != request.ExpectedRevision {
		return CompactReclaimRecord{}, fmt.Errorf("%w: expected compact revision %q, current %q", ErrConcurrentUpdate, request.ExpectedRevision, record.Revision)
	}
	if request.MaintainerAuthorization != compactAbandonAuthorizationBinding(request.LineageID, record.Revision, record.State.InitialSnapshot.Identity, request.Actor, request.Reason) {
		return CompactReclaimRecord{}, fmt.Errorf("review abandon requires an exact maintainer authorization binding (schema %s over lineage %s@%s and snapshot %s)",
			compactAbandonAuthorizationSchema, request.LineageID, record.Revision, record.State.InitialSnapshot.Identity)
	}
	stores, err := DiscoverCompactStores(ctx, repo)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	records := make(map[string]CompactRecord, len(stores))
	storeByLineage := make(map[string]CompactStore, len(stores))
	for _, related := range stores {
		relatedRecord, loadErr := related.Load()
		if loadErr != nil {
			return CompactReclaimRecord{}, fmt.Errorf("review abandon refused: related compact authority %q does not load: %w", related.lineageID, loadErr)
		}
		records[relatedRecord.State.LineageID], storeByLineage[relatedRecord.State.LineageID] = relatedRecord, related
	}
	for lineage, related := range records {
		if related.State.Recovery != nil && related.State.Recovery.PredecessorLineageID == request.LineageID {
			return CompactReclaimRecord{}, fmt.Errorf("review abandon refused: lineage %q is superseded by recovery successor %q; superseded history is never abandoned", request.LineageID, lineage)
		}
	}
	delete(records, request.LineageID)
	delete(storeByLineage, request.LineageID)
	if _, err := compactAuthorityLeaves(records, storeByLineage); err != nil {
		return CompactReclaimRecord{}, fmt.Errorf("review abandon refused: remaining authority graph is invalid: %w", err)
	}
	sort.Strings(residue)
	if request.AbandonedAt.IsZero() {
		request.AbandonedAt = time.Now().UTC()
	}
	return quarantineCompactStoreEntry(base, dir, CompactReclaimRecord{
		Schema: CompactReclaimRecordSchema, Status: CompactReclaimPrepared, LineageID: request.LineageID,
		Reason: strings.TrimSpace(request.Reason), Actor: strings.TrimSpace(request.Actor),
		ReclaimedAt: request.AbandonedAt.UTC(), SourcePath: dir, Residue: residue,
		PristineAbandonment: &CompactPristineAbandonmentProof{
			LineageID: request.LineageID, Revision: record.Revision,
			SnapshotIdentity: record.State.InitialSnapshot.Identity,
			State:            record.State.State, InvalidationReason: record.State.InvalidationReason,
		},
	})
}

// replayCommittedCompactAbandonment resolves an abandon request whose lineage
// no longer holds a v2 entry: an exact replay of a committed abandonment
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
		proof := record.PristineAbandonment
		if record.Status != CompactReclaimCommitted || proof == nil {
			continue
		}
		if proof.LineageID != request.LineageID || proof.Revision != request.ExpectedRevision ||
			record.Reason != strings.TrimSpace(request.Reason) || record.Actor != strings.TrimSpace(request.Actor) {
			continue
		}
		if request.MaintainerAuthorization != compactAbandonAuthorizationBinding(proof.LineageID, proof.Revision, proof.SnapshotIdentity, request.Actor, request.Reason) {
			continue
		}
		return record, nil
	}
	return CompactReclaimRecord{}, fmt.Errorf("review abandon refused: lineage %q holds no compact authority state and no committed abandonment record matches the request; if a prior abandonment was interrupted, the prepared reclaim-record.json under %s locates the moved residue for manual reconciliation", request.LineageID, quarantineRoot)
}
