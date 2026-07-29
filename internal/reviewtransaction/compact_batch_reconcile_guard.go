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
	"reflect"
	"strings"
)

const CompactBatchReconcilePreparationSchema = "gentle-ai.review-batch-reconcile-preparation/v1"
const CompactBatchReconcileRequestSchema = "gentle-ai.review-batch-reconcile-request/v1"
const compactBatchReconcileAuthorizationSchema = "gentle-ai.review-batch-reconcile-authorization/v1"

// CompactBatchReconcilePreparation is the immutable authorization template
// produced from one exclusively locked authority snapshot.
type CompactBatchReconcilePreparation struct {
	Schema                          string                          `json:"schema"`
	RepositorySHA256                string                          `json:"repository_sha256"`
	PlanSHA256                      string                          `json:"plan_sha256"`
	InvalidEdgesSHA256              string                          `json:"invalid_edges_sha256"`
	ValidDescendantSuffixesSHA256   string                          `json:"valid_descendant_suffixes_sha256"`
	QuarantineEntriesSHA256         string                          `json:"quarantine_entries_sha256"`
	DeclaredInvalidEdges            []CompactRecoveryEdgeInspection `json:"declared_invalid_edges"`
	Plan                            CompactBatchReconcilePlan       `json:"plan"`
	RequiredMaintainerAuthorization string                          `json:"required_maintainer_authorization"`
}

// CompactBatchReconcileRequest carries the exact prepared plan and the
// maintainer's byte-exact authorization for a later locked revalidation.
type CompactBatchReconcileRequest struct {
	Schema                  string                          `json:"schema"`
	DeclaredInvalidEdges    []CompactRecoveryEdgeInspection `json:"declared_invalid_edges"`
	ExpectedPlan            CompactBatchReconcilePlan       `json:"expected_plan"`
	ExpectedPlanSHA256      string                          `json:"expected_plan_sha256"`
	Actor                   string                          `json:"actor"`
	Reason                  string                          `json:"reason"`
	MaintainerAuthorization string                          `json:"maintainer_authorization"`
}

type compactBatchReconcileLocks struct {
	base        string
	repository  string
	versionRoot string
	maintenance *MaintenanceLock
	store       *storeLock
}

// PrepareCompactBatchReconciliation obtains exclusive maintenance ownership,
// then the compact-v2 lock, and derives the exact plan and authorization
// template. It performs no authority mutation.
func PrepareCompactBatchReconciliation(ctx context.Context, repo string, declaredInvalid []CompactRecoveryEdgeInspection, actor, reason string) (CompactBatchReconcilePreparation, error) {
	actor, reason, err := validateCompactBatchReconcileIdentity(actor, reason)
	if err != nil {
		return CompactBatchReconcilePreparation{}, err
	}
	locks, err := acquireCompactBatchReconcileLocks(ctx, repo, false)
	if err != nil {
		return CompactBatchReconcilePreparation{}, err
	}
	defer locks.release()
	snapshot, err := loadCompactBatchReconcileSnapshotLocked(ctx, locks)
	if err != nil {
		return CompactBatchReconcilePreparation{}, err
	}
	plan, err := PlanCompactBatchReconciliation(snapshot, declaredInvalid)
	if err != nil {
		return CompactBatchReconcilePreparation{}, err
	}
	return newCompactBatchReconcilePreparation(locks.repository, plan, actor, reason)
}

// validateCompactBatchReconcileRequest repeats preparation under the same lock
// order and refuses any stale record, classification, declaration, suffix, or
// authorization. Mutation code calls the locked variant without releasing the
// guards between validation and publication.
func validateCompactBatchReconcileRequest(ctx context.Context, repo string, request CompactBatchReconcileRequest) (CompactBatchReconcilePlan, error) {
	if err := validateCompactBatchReconcileRequestShape(request); err != nil {
		return CompactBatchReconcilePlan{}, err
	}
	locks, err := acquireCompactBatchReconcileLocks(ctx, repo, false)
	if err != nil {
		return CompactBatchReconcilePlan{}, err
	}
	defer locks.release()
	return validateCompactBatchReconcileRequestLocked(ctx, locks, request)
}

func validateCompactBatchReconcileRequestLocked(ctx context.Context, locks *compactBatchReconcileLocks, request CompactBatchReconcileRequest) (CompactBatchReconcilePlan, error) {
	snapshot, err := loadCompactBatchReconcileSnapshotLocked(ctx, locks)
	if err != nil {
		return CompactBatchReconcilePlan{}, err
	}
	if !snapshot.Inspection.Complete {
		return CompactBatchReconcilePlan{}, compactBatchPlanErrorf("inspection is incomplete")
	}
	if err := rejectCompactBatchStaleDeclarations(snapshot.Inspection, request.DeclaredInvalidEdges); err != nil {
		return CompactBatchReconcilePlan{}, err
	}
	plan, err := PlanCompactBatchReconciliation(snapshot, request.DeclaredInvalidEdges)
	if err != nil {
		return CompactBatchReconcilePlan{}, err
	}
	if !reflect.DeepEqual(plan, request.ExpectedPlan) {
		return CompactBatchReconcilePlan{}, fmt.Errorf("%w: locked authority plan no longer matches the prepared plan", ErrConcurrentUpdate)
	}
	actor, reason, err := validateCompactBatchReconcileIdentity(request.Actor, request.Reason)
	if err != nil {
		return CompactBatchReconcilePlan{}, err
	}
	preparation, err := newCompactBatchReconcilePreparation(locks.repository, plan, actor, reason)
	if err != nil {
		return CompactBatchReconcilePlan{}, err
	}
	if request.ExpectedPlanSHA256 != preparation.PlanSHA256 {
		return CompactBatchReconcilePlan{}, fmt.Errorf("%w: locked plan digest changed", ErrConcurrentUpdate)
	}
	if request.MaintainerAuthorization != preparation.RequiredMaintainerAuthorization {
		return CompactBatchReconcilePlan{}, fmt.Errorf("review reconcile-authority-batch requires an exact maintainer authorization binding (schema %s over repository, plan, declared invalid edges, valid descendant suffixes, quarantine entries, actor, and reason)", compactBatchReconcileAuthorizationSchema)
	}
	return plan, nil
}

func validateCompactBatchReconcileRequestShape(request CompactBatchReconcileRequest) error {
	if request.Schema != CompactBatchReconcileRequestSchema {
		return errors.New("review reconcile-authority-batch requires the exact request schema")
	}
	if request.DeclaredInvalidEdges == nil || request.ExpectedPlan.InvalidEdges == nil ||
		strings.TrimSpace(request.ExpectedPlanSHA256) == "" || strings.TrimSpace(request.MaintainerAuthorization) == "" {
		return errors.New("review reconcile-authority-batch requires declared invalid edges, an expected plan and digest, and maintainer authorization")
	}
	if _, _, err := validateCompactBatchReconcileIdentity(request.Actor, request.Reason); err != nil {
		return err
	}
	digest, err := compactBatchReconcileDigest("plan", request.ExpectedPlan)
	if err != nil {
		return err
	}
	if request.ExpectedPlanSHA256 != digest {
		return errors.New("review reconcile-authority-batch expected plan digest does not bind the supplied plan")
	}
	return nil
}

func validateCompactBatchReconcileIdentity(actor, reason string) (string, string, error) {
	actor, reason = strings.TrimSpace(actor), strings.TrimSpace(reason)
	if actor == "" || reason == "" {
		return "", "", errors.New("review reconcile-authority-batch requires a non-empty actor and reason")
	}
	if strings.ContainsAny(actor, "\r\n") || strings.ContainsAny(reason, "\r\n") {
		return "", "", errors.New("review reconcile-authority-batch actor and reason must each fit on one LF binding line")
	}
	return actor, reason, nil
}

func acquireCompactBatchReconcileLocks(ctx context.Context, repo string, allowPreparedBatch bool) (*compactBatchReconcileLocks, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	base, repository, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return nil, err
	}
	var maintenance *MaintenanceLock
	if allowPreparedBatch {
		maintenance, err = acquireMaintenanceLockForCompactBatch(ctx, compactMaintenanceLockPath(base))
	} else {
		maintenance, err = acquireMaintenanceLock(ctx, compactMaintenanceLockPath(base), maintenanceExclusive)
	}
	if err != nil {
		return nil, err
	}
	versionRoot := filepath.Join(base, "v2")
	local, err := acquireLocalStoreLock(filepath.Join(versionRoot, "LOCK"))
	if err != nil {
		_ = maintenance.Release()
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		_ = local.release()
		_ = maintenance.Release()
		return nil, err
	}
	return &compactBatchReconcileLocks{
		base: base, repository: repository, versionRoot: versionRoot,
		maintenance: maintenance, store: local,
	}, nil
}

func (locks *compactBatchReconcileLocks) release() error {
	if locks == nil {
		return nil
	}
	var storeErr, maintenanceErr error
	if locks.store != nil {
		storeErr = locks.store.release()
		locks.store = nil
	}
	if locks.maintenance != nil {
		maintenanceErr = locks.maintenance.Release()
		locks.maintenance = nil
	}
	return errors.Join(storeErr, maintenanceErr)
}

func loadCompactBatchReconcileSnapshotLocked(ctx context.Context, locks *compactBatchReconcileLocks) (CompactBatchReconcileSnapshot, error) {
	report := CompactRecoveryInspectionReport{
		Complete: true, Valid: true, Edges: []CompactRecoveryEdgeInspection{},
		EntryDiagnostics: []CompactRecoveryEntryDiagnostic{},
	}
	entries, err := os.ReadDir(locks.versionRoot)
	if os.IsNotExist(err) {
		return CompactBatchReconcileSnapshot{Inspection: report, Records: []CompactRecord{}}, nil
	}
	if err != nil {
		return CompactBatchReconcileSnapshot{}, fmt.Errorf("inspect compact batch authority: %w", err)
	}
	records := make(map[string]CompactRecord, len(entries))
	ordered := make([]CompactRecord, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return CompactBatchReconcileSnapshot{}, err
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
		store := CompactStore{Dir: filepath.Join(locks.versionRoot, entry.Name()), lineageID: entry.Name(), repo: locks.repository}
		record, loadErr := store.loadCompactRecordLocked()
		if loadErr != nil {
			report.EntryDiagnostics = append(report.EntryDiagnostics, CompactRecoveryEntryDiagnostic{
				LineageID: entry.Name(), Problem: compactRecoveryEntryProblem(loadErr),
			})
			continue
		}
		report.Totals.LoadedEntries++
		records[record.State.LineageID] = record
		ordered = append(ordered, record)
	}
	report, err = inspectCompactRecoveryRecordSet(ctx, records, report)
	if err != nil {
		return CompactBatchReconcileSnapshot{}, err
	}
	return CompactBatchReconcileSnapshot{Inspection: report, Records: ordered}, nil
}

func rejectCompactBatchStaleDeclarations(inspection CompactRecoveryInspectionReport, declared []CompactRecoveryEdgeInspection) error {
	current := make(map[string]CompactRecoveryEdgeInspection, len(inspection.Edges))
	for _, edge := range inspection.Edges {
		current[edge.SuccessorLineageID] = edge
	}
	for _, expected := range declared {
		observed, found := current[expected.SuccessorLineageID]
		if !found {
			return fmt.Errorf("%w: declared recovery successor %q is absent", ErrConcurrentUpdate, expected.SuccessorLineageID)
		}
		if expected.PredecessorLineageID != observed.PredecessorLineageID ||
			expected.RecordedPredecessorRevision != observed.RecordedPredecessorRevision ||
			expected.ObservedPredecessorRevision != observed.ObservedPredecessorRevision ||
			expected.SuccessorRevision != observed.SuccessorRevision ||
			!reflect.DeepEqual(expected.AnomalyClasses, observed.AnomalyClasses) {
			return fmt.Errorf("%w: locked recovery edge %q changed revision, identity, or anomaly classification", ErrConcurrentUpdate, expected.SuccessorLineageID)
		}
	}
	return nil
}

func newCompactBatchReconcilePreparation(repository string, plan CompactBatchReconcilePlan, actor, reason string) (CompactBatchReconcilePreparation, error) {
	repositorySHA256, err := compactBatchReconcileDigest("repository", filepath.Clean(repository))
	if err != nil {
		return CompactBatchReconcilePreparation{}, err
	}
	planSHA256, err := compactBatchReconcileDigest("plan", plan)
	if err != nil {
		return CompactBatchReconcilePreparation{}, err
	}
	invalidEdgesSHA256, err := compactBatchReconcileDigest("invalid-edges", plan.InvalidEdges)
	if err != nil {
		return CompactBatchReconcilePreparation{}, err
	}
	suffixesSHA256, err := compactBatchReconcileDigest("valid-descendant-suffixes", plan.ValidDescendantSuffixes)
	if err != nil {
		return CompactBatchReconcilePreparation{}, err
	}
	quarantineSHA256, err := compactBatchReconcileDigest("quarantine-entries", plan.QuarantineEntries)
	if err != nil {
		return CompactBatchReconcilePreparation{}, err
	}
	preparation := CompactBatchReconcilePreparation{
		Schema:           CompactBatchReconcilePreparationSchema,
		RepositorySHA256: repositorySHA256, PlanSHA256: planSHA256,
		InvalidEdgesSHA256: invalidEdgesSHA256, ValidDescendantSuffixesSHA256: suffixesSHA256,
		QuarantineEntriesSHA256: quarantineSHA256,
		DeclaredInvalidEdges:    cloneCompactRecoveryEdges(plan.InvalidEdges), Plan: plan,
	}
	preparation.RequiredMaintainerAuthorization = compactBatchReconcileAuthorizationSchema +
		"\nrepository_sha256=" + repositorySHA256 +
		"\nplan_sha256=" + planSHA256 +
		"\ninvalid_edges_sha256=" + invalidEdgesSHA256 +
		"\nvalid_descendant_suffixes_sha256=" + suffixesSHA256 +
		"\nquarantine_entries_sha256=" + quarantineSHA256 +
		"\nactor=" + actor + "\nreason=" + reason
	return preparation, nil
}

func compactBatchReconcileDigest(domain string, value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal compact batch %s digest: %w", domain, err)
	}
	sum := sha256.Sum256(append([]byte("gentle-ai.review-batch-reconcile-"+domain+"/v1\x00"), payload...))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
