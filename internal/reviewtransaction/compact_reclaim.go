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

	"github.com/gentleman-programming/gentle-ai/v5/internal/pathquote"
)

const CompactReclaimRecordSchema = "gentle-ai.review-reclaim-record/v1"

const (
	CompactReclaimPrepared  = "prepared"
	CompactReclaimCommitted = "committed"
)

var reclaimQuarantineResidue = os.Rename
var persistReclaimRecord = persistCompactReclaimRecord
var compactReclaimPhaseHook = func(ctx context.Context, _ string, _ CompactReclaimRecord) error { return ctx.Err() }

const (
	compactReclaimPhasePrepared  = "prepared"
	compactReclaimPhaseRenamed   = "renamed"
	compactReclaimPhaseCommitted = "committed"
)

// CompactReclaimRequest identifies one explicit incomplete compact-v2 store
// entry to quarantine, together with the maintainer audit inputs.
type CompactReclaimRequest struct {
	LineageID   string
	Reason      string
	Actor       string
	ReclaimedAt time.Time
}

// CompactReclaimRecord is the persisted audit record for one quarantined
// incomplete store entry; it mirrors the recovery provenance shape. Status is
// "committed" only after the residue rename landed and the record rewrite
// succeeded. A record stuck at "prepared" means the rename may or may not
// have run; the discriminator is the residue/ subdirectory next to the
// record: absent means the residue was never moved, present means it was
// moved but the committed rewrite did not land.
type CompactReclaimRecord struct {
	Schema         string    `json:"schema"`
	Status         string    `json:"status"`
	LineageID      string    `json:"lineage_id"`
	Reason         string    `json:"reason"`
	Actor          string    `json:"actor"`
	ReclaimedAt    time.Time `json:"reclaimed_at"`
	SourcePath     string    `json:"source_path"`
	QuarantinePath string    `json:"quarantine_path"`
	Residue        []string  `json:"residue"`
	// InvalidRecoveryEdge carries the natively re-derived edge-invalidity
	// proof; it is set only for reconcile-authority quarantines of the
	// unchanged-target class.
	InvalidRecoveryEdge *CompactInvalidRecoveryEdgeProof `json:"invalid_recovery_edge,omitempty"`
	// MalformedRecoveryAuthorization carries the natively re-derived
	// pre-contract authorization proof; it is set only for reconcile-authority
	// quarantines of the malformed-recovery-authorization class.
	MalformedRecoveryAuthorization *CompactMalformedRecoveryAuthorizationProof `json:"malformed_recovery_authorization,omitempty"`
	// PristineAbandonment carries the natively re-derived pristineness proof;
	// it is set only for review-abandon quarantines of pristine reviewing or
	// pristine invalidated lineages.
	PristineAbandonment *CompactPristineAbandonmentProof `json:"pristine_abandonment,omitempty"`
	// IncompleteAbandonment carries the natively re-derived captured/uncaptured
	// lens partition; it is set only for review-abandon quarantines of a
	// reviewing lineage whose selected plan never finished reporting.
	IncompleteAbandonment *CompactIncompleteAbandonmentProof `json:"incomplete_abandonment,omitempty"`
	// Abandonment carries the single v2 authorization proof.
	Abandonment *CompactAbandonmentProof `json:"abandonment,omitempty"`
	// MalformedLegacyFreeze, LegacyAliasRepair, ClassifiedAuthorityRepair and
	// LegacyFixScopeQuarantine are the proofs of quarantines the retired
	// legacy-v1 and classified repair providers committed. Legacy v1 is unsupported, so a record carrying one is closed
	// history: it stays decodable and listable, byte-preserved as opaque JSON,
	// and nothing re-derives or interprets its proof.
	MalformedLegacyFreeze     json.RawMessage `json:"malformed_legacy_freeze,omitempty"`
	LegacyAliasRepair         json.RawMessage `json:"legacy_alias_repair,omitempty"`
	ClassifiedAuthorityRepair json.RawMessage `json:"classified_authority_repair,omitempty"`
	LegacyFixScopeQuarantine  json.RawMessage `json:"legacy_fix_scope_quarantine,omitempty"`
	// AuthorityDisposition binds a leaf disposition-plan quarantine
	// (authority_disposition_execute.go) to the exact digest-bound
	// AuthorityDispositionPlan it executed. It is set only for quarantines
	// executeAuthorityDisposition creates.
	AuthorityDisposition *AuthorityDispositionProof `json:"authority_disposition,omitempty"`
}

// compactAuthoritativeArtifact reports whether a store-entry name carries
// review authority: state, receipt, finalize journal, or interrupted
// atomic-write payloads that may hold any of them. Canonical reviewer results
// live inside review-state.json, not in a sibling directory. It must cover every
// file the compact store writes under a lineage directory; the shared names live
// beside CompactStore in compact_store.go.
func compactAuthoritativeArtifact(name string) bool {
	switch name {
	case compactStateFileName, compactReceiptFileName, compactFinalizeJournalFileName:
		return true
	}
	return strings.HasPrefix(name, ".atomic-")
}

func compactStoreHoldsAuthority(items []os.DirEntry) bool {
	for _, item := range items {
		if compactAuthoritativeArtifact(item.Name()) {
			return true
		}
	}
	return false
}

// ReclaimIncompleteCompactStore moves one incomplete compact-v2 store entry —
// no review-state.json and no other authoritative artifact — into an audited
// quarantine directory under the review authority root. It never deletes
// residue and never fabricates authority state. On partial failure after the
// prepared record is persisted, it returns the populated prepared record
// (QuarantinePath and Residue set) alongside a non-nil error; callers must
// not discard the record on error — it locates the quarantine or orphan for
// reconciliation.
func ReclaimIncompleteCompactStore(ctx context.Context, repo string, request CompactReclaimRequest) (CompactReclaimRecord, error) {
	if err := ctx.Err(); err != nil {
		return CompactReclaimRecord{}, err
	}
	if err := validateLineageID(request.LineageID); err != nil {
		return CompactReclaimRecord{}, err
	}
	if strings.TrimSpace(request.Reason) == "" || strings.TrimSpace(request.Actor) == "" {
		return CompactReclaimRecord{}, errors.New("review reclaim requires a non-empty reason and actor")
	}
	base, root, err := reviewAuthorityRoot(ctx, repo)
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
	items, err := os.ReadDir(dir)
	if err != nil {
		return CompactReclaimRecord{}, fmt.Errorf("inspect reclaim target: %w", err)
	}
	residue := make([]string, 0, len(items))
	for _, item := range items {
		if compactAuthoritativeArtifact(item.Name()) {
			return CompactReclaimRecord{}, compactReclaimAuthorityRefusal(ctx, root, request.LineageID, item.Name())
		}
		residue = append(residue, item.Name())
	}
	sort.Strings(residue)
	if request.ReclaimedAt.IsZero() {
		request.ReclaimedAt = time.Now().UTC()
	}
	return quarantineCompactStoreEntry(ctx, base, dir, CompactReclaimRecord{
		Schema: CompactReclaimRecordSchema, Status: CompactReclaimPrepared, LineageID: request.LineageID,
		Reason: strings.TrimSpace(request.Reason), Actor: strings.TrimSpace(request.Actor),
		ReclaimedAt: request.ReclaimedAt.UTC(), SourcePath: dir, Residue: residue,
	})
}

// compactReclaimAuthorityRefusal explains why reclaim does not apply to a
// store entry holding an authoritative artifact. Reclaim only quarantines
// entries that never held authority; every other shape is declared lost and
// quarantined whole by `review abandon`, which accepts any state, including an
// unreadable or missing review-state.json, and needs the maintainer's
// authorization of exactly what moves.
func compactReclaimAuthorityRefusal(ctx context.Context, repo, lineageID, artifact string) error {
	refused := fmt.Sprintf("review reclaim refused: store entry %q holds authoritative artifact %q, and reclaim only quarantines entries that never held authority.", lineageID, artifact)
	eligibility, err := InspectCompactPristineAbandonment(ctx, repo, lineageID)
	if err != nil || !eligibility.Eligible {
		return fmt.Errorf("%s Capture the complete machine-readable diagnosis with `gentle-ai review inspect-authority --cwd %s` and escalate that report", refused, pathquote.Quote(repo))
	}
	return fmt.Errorf("%s `gentle-ai review abandon` declares it lost and quarantines it whole: %s", refused, compactAbandonCommandText(repo, lineageID, eligibility))
}

// quarantineCompactStoreEntry runs the shared two-phase audited move for one
// store entry: create the quarantine directory, persist the prepared record,
// rename the entry into residue/, then rewrite the record as committed. On
// partial failure after the prepared record persisted, it returns the
// populated prepared record alongside a non-nil error.
func quarantineCompactStoreEntry(ctx context.Context, base, dir string, record CompactReclaimRecord) (CompactReclaimRecord, error) {
	if err := ctx.Err(); err != nil {
		return CompactReclaimRecord{}, err
	}
	quarantineRoot := filepath.Join(base, "quarantine")
	if err := ensureCanonicalReviewQuarantineRoot(base, quarantineRoot); err != nil {
		return CompactReclaimRecord{}, err
	}
	if err := os.MkdirAll(quarantineRoot, 0o755); err != nil {
		return CompactReclaimRecord{}, err
	}
	if err := ensureCanonicalReviewQuarantineRoot(base, quarantineRoot); err != nil {
		return CompactReclaimRecord{}, err
	}
	if err := SyncReviewDirectory(base); err != nil {
		return CompactReclaimRecord{}, fmt.Errorf("sync review authority root after quarantine creation: %w", err)
	}
	quarantineDir, err := os.MkdirTemp(quarantineRoot, record.LineageID+"-")
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	cleanupUnprepared := func(cause error) error {
		if removeErr := os.RemoveAll(quarantineDir); removeErr != nil {
			return errors.Join(cause, fmt.Errorf("remove unidentified quarantine directory %q: %w", quarantineDir, removeErr))
		}
		if syncErr := SyncReviewDirectory(quarantineRoot); syncErr != nil {
			return errors.Join(cause, fmt.Errorf("sync quarantine cleanup for %q: %w", quarantineDir, syncErr))
		}
		return cause
	}
	if err := SyncReviewDirectory(quarantineRoot); err != nil {
		return CompactReclaimRecord{}, cleanupUnprepared(fmt.Errorf("sync quarantine root after reclaim directory creation: %w", err))
	}
	record.QuarantinePath = quarantineDir
	if err := persistReclaimRecord(record); err != nil {
		return CompactReclaimRecord{}, cleanupUnprepared(err)
	}
	if err := compactReclaimPhaseHook(ctx, compactReclaimPhasePrepared, record); err != nil {
		return record, fmt.Errorf("reclaim prepared before residue mutation: %w", err)
	}
	if err := reclaimQuarantineResidue(dir, filepath.Join(quarantineDir, "residue")); err != nil {
		return record, fmt.Errorf("reclaim was prepared at %s but quarantining the residue failed: %w", quarantineDir, err)
	}
	if err := SyncReviewDirectory(filepath.Dir(dir)); err != nil {
		return record, fmt.Errorf("residue was quarantined at %s but syncing the source version root failed: %w", quarantineDir, err)
	}
	if err := SyncReviewDirectory(quarantineDir); err != nil {
		return record, fmt.Errorf("residue was quarantined at %s but syncing the quarantine directory failed: %w", quarantineDir, err)
	}
	if err := compactReclaimPhaseHook(ctx, compactReclaimPhaseRenamed, record); err != nil {
		return record, fmt.Errorf("residue was quarantined before audit commit: %w", err)
	}
	committed := record
	committed.Status = CompactReclaimCommitted
	if err := persistReclaimRecord(committed); err != nil {
		return record, fmt.Errorf("residue was quarantined at %s but the reclaim audit record could not be marked committed: %w", quarantineDir, err)
	}
	if err := compactReclaimPhaseHook(ctx, compactReclaimPhaseCommitted, committed); err != nil {
		return committed, fmt.Errorf("reclaim audit committed before readback: %w", err)
	}
	return committed, nil
}

// ensureCanonicalReviewQuarantineRoot rejects symlinked authority components
// before a quarantine operation can create, inspect, or rename inside them.
func ensureCanonicalReviewQuarantineRoot(base, quarantineRoot string) error {
	commonDir := filepath.Dir(filepath.Dir(base))
	if filepath.Clean(base) != filepath.Join(commonDir, "gentle-ai", "review-transactions") ||
		filepath.Clean(quarantineRoot) != filepath.Join(base, "quarantine") {
		return errors.New("review quarantine refused noncanonical authority root")
	}
	for _, component := range []string{"gentle-ai", "review-transactions", "quarantine"} {
		path := filepath.Join(commonDir, component)
		if component == "review-transactions" {
			path = base
		}
		if component == "quarantine" {
			path = quarantineRoot
		}
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect review quarantine root component %q: %w", component, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("review quarantine refused unsafe authority root component %q", component)
		}
		canonical, err := filepath.EvalSymlinks(path)
		if err != nil || filepath.Clean(canonical) != path {
			return fmt.Errorf("review quarantine refused noncanonical authority root component %q", component)
		}
	}
	return nil
}

func persistCompactReclaimRecord(record CompactReclaimRecord) error {
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(record.QuarantinePath, "reclaim-record.json"), append(payload, '\n'), 0o644); err != nil {
		return fmt.Errorf("persist %s reclaim audit record: %w", record.Status, err)
	}
	return nil
}

// AuthorityDispositionClosureManifestSchema identifies
// AuthorityDispositionClosureManifest's shape.
const AuthorityDispositionClosureManifestSchema = "gentle-ai.review-authority-disposition-closure-manifest/v1"

// AuthorityDispositionClosureManifest is Wave 6's forensic-only record of an
// N-node closure disposition (design decision D5): written once, inside the
// SEED node's own quarantine directory, alongside residue/ and
// reclaim-record.json, only after every closure member has committed. It
// binds the ordered closure to the one plan_digest that already governs
// every per-node AuthorityDispositionProof, so a human inspecting the seed's
// quarantine directory sees the whole closure without reconstructing it from
// N separate reclaim-record.json files.
//
// It is never a lifecycle dependency: per-node resume state lives entirely in
// each node's own reclaim-record.json plus the residue/ discriminator
// (discoverAuthorityDispositionRecord/resumeAuthorityDispositionRecord);
// recovery never reads this file. Design caps RDD at two operational
// artifacts — this is deliberately not a third.
type AuthorityDispositionClosureManifest struct {
	Schema                     string   `json:"schema"`
	PlanDigest                 string   `json:"plan_digest"`
	AuthorityInventoryRevision string   `json:"authority_inventory_revision"`
	AnomalyClass               string   `json:"anomaly_class"`
	OrderedClosure             []string `json:"ordered_closure"`
	Disposed                   []string `json:"disposed"`
}

// writeAuthorityDispositionClosureManifest persists manifest as
// closure-manifest.json inside quarantineDir (the seed's own quarantine
// directory), reusing writeAtomic exactly like persistCompactReclaimRecord's
// reclaim-record.json.
func writeAuthorityDispositionClosureManifest(quarantineDir string, manifest AuthorityDispositionClosureManifest) error {
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(quarantineDir, "closure-manifest.json"), append(payload, '\n'), 0o644); err != nil {
		return fmt.Errorf("persist authority disposition closure manifest: %w", err)
	}
	return nil
}
