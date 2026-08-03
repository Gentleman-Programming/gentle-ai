package reviewtransaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// AuthorityDispositionProofSchema identifies AuthorityDispositionProof's shape.
const AuthorityDispositionProofSchema = "gentle-ai.review-authority-disposition-proof/v1"

// errAuthorityDispositionCardinality is returned whenever a plan's closure
// does not have cardinality exactly one — Wave 2's whole executor scope
// (#1892's historical exact-binding edge shape). Larger closures (#2014's
// descendant closure, #1656's multi-lineage shape) escalate to a future wave
// (Wave 6); this refusal names both without committing to a delivery date
// (rdd-leaf-disposition-execution / "Cardinality-One Admission", "Refusal
// Names Diagnosis and Escalation Artifact, Not a Roadmap Promise").
var errAuthorityDispositionCardinality = errors.New("authority disposition execution refused: closure cardinality is not one") // refusal:by-design human-authority: admitting a larger closure is a future-wave product decision (Wave 6), not a command this slice can run today

// admitLeafDisposition requires closure(S) to have cardinality exactly one —
// the one shape this executor mutates. Any other cardinality refuses before
// any lock acquisition or I/O (design decision 5: cardinality is executor
// admission policy, not a plan-shape constraint — a future wave relaxes only
// this function).
func admitLeafDisposition(plan AuthorityDispositionPlan) error {
	if len(plan.SeedSet) != 1 || len(plan.Closure) != 1 || plan.Closure[0] != plan.SeedSet[0] {
		return fmt.Errorf("%w: closure has %d member(s), want exactly 1; multi-lineage shapes (#1656, #2014) escalate to a future wave (Wave 6) with no delivery-date commitment",
			errAuthorityDispositionCardinality, len(plan.Closure))
	}
	return nil
}

// AdmitAuthorityDispositionLeaf is the exported form of admitLeafDisposition
// for Slice S3's `review repair` CLI wiring and SanctionedCompactRecoveryExits
// (compact_inspect.go) — both need the identical, unrelaxed cardinality-one
// admission predicate the executor itself enforces, so neither ever
// advertises or accepts a plan the executor would then refuse.
func AdmitAuthorityDispositionLeaf(plan AuthorityDispositionPlan) error {
	return admitLeafDisposition(plan)
}

// AuthorityDispositionProof carries the natively re-derived
// AuthorityDispositionPlan binding inside the quarantine audit record; it is
// set only for disposition-plan-bound quarantines this file executes. The
// Authorization is recorded only by SHA-256 digest, matching
// classifyCompactRecoveryEdgeAnomalies' idiom for authorization bytes
// (compact_reconcile.go): the recorded binding itself stays byte-preserved
// in the quarantined residue.
type AuthorityDispositionProof struct {
	Schema                     string            `json:"schema"`
	PlanDigest                 string            `json:"plan_digest"`
	AuthorityInventoryRevision string            `json:"authority_inventory_revision"`
	AnomalyClass               string            `json:"anomaly_class"`
	SeedSet                    []string          `json:"ordered_seed_set"`
	Closure                    []string          `json:"ordered_closure"`
	ExpectedRevisions          map[string]string `json:"expected_revisions"`
	AuthorizationSHA256        string            `json:"authorization_sha256"`
}

// executeAuthorityDisposition is the one executor that consumes an
// AuthorityDispositionPlan with a populated Authorization and admits only a
// cardinality-one leaf closure (rdd-leaf-disposition-execution). It follows
// design.md's Data Flow exactly: admitLeafDisposition on the submitted plan,
// then an exclusive maintenance lock, then a fresh re-derivation under that
// lock compared by digest (CAS over expected_revisions and every other
// derived field), then Authorization validated against the digest-bound
// plan (mandatory obligation (b) — validateAuthorityDispositionAuthorization
// is reused verbatim from authority_disposition_plan.go, never
// reimplemented), then replay discovery, then quarantine. The lock is
// released before the retained-graph readback: the quarantine mutation is
// already durably committed by then, and readback reuses the ordinary
// InspectCompactRecoveryEdges seam, which cannot run while this executor's
// own exclusive lease is still held (see
// loadCompactRecoveryRecordsUnderMaintenanceHold). It stays unexported: it
// has no CLI entrypoint of its own in this slice — Slice S3 wires it
// (through repairAuthorityDispositionAtRepo, authority_repair.go) behind the
// existing `review repair` verb (rdd-authority-disposition-plan / "No New
// Public Repair Verb").
func executeAuthorityDisposition(ctx context.Context, repo string, plan AuthorityDispositionPlan) (CompactReclaimRecord, error) {
	if err := ctx.Err(); err != nil {
		return CompactReclaimRecord{}, err
	}
	if err := admitLeafDisposition(plan); err != nil {
		return CompactReclaimRecord{}, err
	}
	root, err := (SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	base, binding, err := authorityRepairRoot(root)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	if plan.RepositoryBinding != binding {
		// refusal:by-design operator-knowledge: only the operator knows which repository the plan was derived against; the fix is supplying the matching --cwd, not a command this refusal can name
		return CompactReclaimRecord{}, errors.New("authority disposition execution refused: plan is bound to a different repository")
	}
	seed := plan.SeedSet[0]

	maintenance, err := acquireMaintenanceLock(ctx, compactMaintenanceLockPath(base), maintenanceExclusive)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	record, err := lockedAuthorityDispositionMutation(ctx, maintenance, base, root, binding, seed, plan)
	if err != nil {
		return record, err
	}
	return readBackAuthorityDisposition(ctx, root, record)
}

// lockedAuthorityDispositionMutation runs every step that must happen while
// base's exclusive maintenance lock is held: replay discovery, lock+CAS
// reinspection, Authorization validation, and the byte-preserving
// quarantine. It always releases maintenance before returning.
func lockedAuthorityDispositionMutation(ctx context.Context, maintenance *MaintenanceLock, base, root, binding, seed string, plan AuthorityDispositionPlan) (CompactReclaimRecord, error) {
	defer maintenance.Release()

	if existing, found, err := discoverAuthorityDispositionRecord(ctx, base, seed, plan.PlanDigest); err != nil {
		return CompactReclaimRecord{}, err
	} else if found {
		return existing, nil
	}

	report, records, err := loadCompactRecoveryRecordsUnderMaintenanceHold(ctx, root)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	currentPlan, err := deriveAuthorityDispositionPlan(report, records, binding, plan.Actor, plan.Reason)
	if err != nil {
		return CompactReclaimRecord{}, fmt.Errorf("authority disposition execution refused: re-derivation under lock did not reproduce a closed classification: %w", err)
	}
	if err := admitLeafDisposition(currentPlan); err != nil {
		return CompactReclaimRecord{}, err
	}
	if currentPlan.PlanDigest != plan.PlanDigest {
		return CompactReclaimRecord{}, fmt.Errorf("%w: plan digest drifted under lock re-derivation (expected_revisions or authority_inventory_revision changed)", ErrConcurrentUpdate)
	}
	currentInventoryRevision, err := authorityInventoryRevision(records)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	if err := validateAuthorityDispositionAuthorization(plan, currentInventoryRevision); err != nil {
		return CompactReclaimRecord{}, err
	}

	targetRecord, found := records[seed]
	expectedRevision, expectedFound := plan.ExpectedRevisions[seed]
	if !found || !expectedFound || targetRecord.Revision != expectedRevision {
		return CompactReclaimRecord{}, fmt.Errorf("%w: expected revision for %q drifted", ErrConcurrentUpdate, seed)
	}

	dir := filepath.Join(base, "v2", seed)
	items, err := os.ReadDir(dir)
	if err != nil {
		return CompactReclaimRecord{}, fmt.Errorf("inspect authority disposition target: %w", err)
	}
	residue := make([]string, 0, len(items))
	for _, item := range items {
		residue = append(residue, item.Name())
	}
	sort.Strings(residue)

	recordedAuthorization := sha256.Sum256([]byte(plan.Authorization))
	return quarantineCompactStoreEntry(ctx, base, dir, CompactReclaimRecord{
		Schema: CompactReclaimRecordSchema, Status: CompactReclaimPrepared, LineageID: seed,
		Reason: plan.Reason, Actor: plan.Actor, ReclaimedAt: time.Now().UTC(), SourcePath: dir, Residue: residue,
		AuthorityDisposition: &AuthorityDispositionProof{
			Schema: AuthorityDispositionProofSchema, PlanDigest: plan.PlanDigest,
			AuthorityInventoryRevision: plan.AuthorityInventoryRevision, AnomalyClass: plan.AnomalyClass,
			SeedSet: append([]string(nil), plan.SeedSet...), Closure: append([]string(nil), plan.Closure...),
			ExpectedRevisions:   cloneAuthorityDispositionRevisions(plan.ExpectedRevisions),
			AuthorizationSHA256: "sha256:" + hex.EncodeToString(recordedAuthorization[:]),
		},
	})
}

// loadCompactRecoveryRecordsUnderMaintenanceHold mirrors
// loadCompactRecoveryRecords's exact read-and-classify body
// (compact_inspect.go) for a caller that already holds base's exclusive
// maintenance lock. Calling the ordinary seam here would self-contend:
// CompactStore.Load acquires its own shared maintenance lock per entry
// (compact_store.go acquireReadMaintenance), and flock is not reentrant
// across file descriptors even within the same process, so a second lock
// attempt while this executor's own exclusive lease is held would time out
// on every entry. This reuses the exact same classification algorithm
// (inspectCompactRecoveryRecordSet) the seam calls — only the per-entry file
// read differs: loadCompactRecordLocked (the package's existing
// "uncoordinated read for a caller that already holds the required
// coordination" primitive, compact_store.go) instead of Load.
func loadCompactRecoveryRecordsUnderMaintenanceHold(ctx context.Context, repo string) (CompactRecoveryInspectionReport, map[string]CompactRecord, error) {
	report := CompactRecoveryInspectionReport{Complete: true, Valid: true, Edges: []CompactRecoveryEdgeInspection{}, EntryDiagnostics: []CompactRecoveryEntryDiagnostic{}}
	base, root, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return report, nil, err
	}
	if err := ensureNoPreparedCompactBatchReconciliation(base); err != nil {
		return report, nil, err
	}
	versionRoot := filepath.Join(base, "v2")
	entries, err := os.ReadDir(versionRoot)
	if os.IsNotExist(err) {
		return report, map[string]CompactRecord{}, nil
	}
	if err != nil {
		return report, nil, fmt.Errorf("inspect compact authority v2 root: %w", err)
	}
	records := make(map[string]CompactRecord, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return report, nil, err
		}
		if !entry.IsDir() {
			if entry.Name() != "LOCK" {
				report.EntryDiagnostics = append(report.EntryDiagnostics, CompactRecoveryEntryDiagnostic{
					LineageID: entry.Name(), Problem: compactInspectionEntryUnexpected,
				})
			}
			continue
		}
		report.Totals.CompactEntries++
		store := CompactStore{Dir: filepath.Join(versionRoot, entry.Name()), lineageID: entry.Name(), repo: root,
			lockPath: filepath.Join(versionRoot, "LOCK"), maintenanceLockPath: compactMaintenanceLockPath(base)}
		record, loadErr := store.loadCompactRecordLocked()
		if loadErr != nil {
			report.EntryDiagnostics = append(report.EntryDiagnostics, CompactRecoveryEntryDiagnostic{
				LineageID: entry.Name(), Problem: compactRecoveryEntryProblem(loadErr),
			})
			continue
		}
		report.Totals.LoadedEntries++
		records[record.State.LineageID] = record
	}
	report, err = inspectCompactRecoveryRecordSet(ctx, records, report)
	return report, records, err
}

// discoverAuthorityDispositionRecord finds an existing quarantine record for
// seed whose AuthorityDisposition binds the exact planDigest, so a replayed
// execution converges without moving the entry a second time (exact replay,
// rdd-leaf-disposition-execution / "Exact Replay Converges Without
// Double-Move"). A record still Prepared (a crash between the residue
// rename and the committed rewrite) is resumed to completion first, mirroring
// resumePreparedClassifiedAuthorityRepair's discriminator
// (classified_authority_replay.go): the residue/ subdirectory's presence
// decides whether the rename already ran.
func discoverAuthorityDispositionRecord(ctx context.Context, base, seed, planDigest string) (CompactReclaimRecord, bool, error) {
	quarantineRoot := filepath.Join(base, "quarantine")
	if err := ensureCanonicalReviewQuarantineRoot(base, quarantineRoot); err != nil {
		return CompactReclaimRecord{}, false, err
	}
	entries, err := os.ReadDir(quarantineRoot)
	if os.IsNotExist(err) {
		return CompactReclaimRecord{}, false, nil
	}
	if err != nil {
		return CompactReclaimRecord{}, false, fmt.Errorf("inspect authority disposition quarantine: %w", err)
	}
	var matched *CompactReclaimRecord
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return CompactReclaimRecord{}, false, err
		}
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), seed+"-") {
			continue
		}
		payload, readErr := os.ReadFile(filepath.Join(quarantineRoot, entry.Name(), "reclaim-record.json"))
		if readErr != nil {
			if os.IsNotExist(readErr) {
				continue
			}
			return CompactReclaimRecord{}, false, fmt.Errorf("read authority disposition quarantine record: %w", readErr)
		}
		var record CompactReclaimRecord
		if err := json.Unmarshal(payload, &record); err != nil {
			return CompactReclaimRecord{}, false, fmt.Errorf("decode authority disposition quarantine record: %w", err)
		}
		if record.LineageID != seed || record.AuthorityDisposition == nil || record.AuthorityDisposition.PlanDigest != planDigest {
			continue
		}
		if matched != nil {
			return CompactReclaimRecord{}, false, errors.New("authority disposition execution refused: duplicate quarantine records for the same plan digest; run `gentle-ai review inspect-authority` and escalate the report")
		}
		found := record
		matched = &found
	}
	if matched == nil {
		return CompactReclaimRecord{}, false, nil
	}
	if matched.Status == CompactReclaimPrepared {
		resumed, err := resumeAuthorityDispositionRecord(ctx, *matched)
		return resumed, true, err
	}
	return *matched, true, nil
}

// resumeAuthorityDispositionRecord completes a prepared-but-interrupted
// disposition quarantine, reusing canonicalClassifiedRepairDirectory
// (classified_authority_replay.go) to decide whether the residue rename
// already ran.
func resumeAuthorityDispositionRecord(ctx context.Context, record CompactReclaimRecord) (CompactReclaimRecord, error) {
	residuePath := filepath.Join(record.QuarantinePath, "residue")
	sourceExists, err := canonicalClassifiedRepairDirectory(record.SourcePath)
	if err != nil {
		return record, err
	}
	residueExists, err := canonicalClassifiedRepairDirectory(residuePath)
	if err != nil {
		return record, err
	}
	if sourceExists == residueExists {
		return record, errors.New("authority disposition execution refused: ambiguous prepared residue state; run `gentle-ai review inspect-authority` and escalate the report")
	}
	if sourceExists {
		if err := reclaimQuarantineResidue(record.SourcePath, residuePath); err != nil {
			return record, fmt.Errorf("resume authority disposition residue quarantine: %w", err)
		}
		if err := SyncReviewDirectory(filepath.Dir(record.SourcePath)); err != nil {
			return record, err
		}
		if err := SyncReviewDirectory(record.QuarantinePath); err != nil {
			return record, err
		}
		if err := compactReclaimPhaseHook(ctx, compactReclaimPhaseRenamed, record); err != nil {
			return record, err
		}
	}
	committed := record
	committed.Status = CompactReclaimCommitted
	if err := persistReclaimRecord(committed); err != nil {
		return record, err
	}
	if err := compactReclaimPhaseHook(ctx, compactReclaimPhaseCommitted, committed); err != nil {
		return committed, err
	}
	return committed, nil
}

// readBackAuthorityDisposition re-runs classification over the retained
// graph and refuses to report success unless it revalidates as
// Complete && Valid with no dangling reference to the quarantined entry
// (rdd-leaf-disposition-execution / "Retained-Graph Revalidation Before
// Success").
func readBackAuthorityDisposition(ctx context.Context, root string, record CompactReclaimRecord) (CompactReclaimRecord, error) {
	if record.Status != CompactReclaimCommitted {
		return record, fmt.Errorf("authority disposition execution refused: readback observed a non-committed record; run `gentle-ai review inspect-authority --cwd %q` and escalate the report", root)
	}
	report, err := InspectCompactRecoveryEdges(ctx, root)
	if err != nil {
		return record, fmt.Errorf("authority disposition readback: %w", err)
	}
	if !report.Complete || !report.Valid {
		return record, fmt.Errorf("authority disposition execution refused: retained-graph readback did not revalidate cleanly; run `gentle-ai review inspect-authority --cwd %q` and escalate the report", root)
	}
	for _, edge := range report.Edges {
		if edge.PredecessorLineageID == record.LineageID || edge.SuccessorLineageID == record.LineageID {
			return record, fmt.Errorf("authority disposition execution refused: retained graph still references the quarantined entry; run `gentle-ai review inspect-authority --cwd %q` and escalate the report", root)
		}
	}
	return record, nil
}

func cloneAuthorityDispositionRevisions(revisions map[string]string) map[string]string {
	cloned := make(map[string]string, len(revisions))
	for lineage, revision := range revisions {
		cloned[lineage] = revision
	}
	return cloned
}
