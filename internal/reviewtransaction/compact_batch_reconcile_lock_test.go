package reviewtransaction

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const compactBatchTestActor = "maintainer@example.com"
const compactBatchTestReason = "atomically quarantine the declared invalid recovery edges"

func TestPrepareCompactBatchReconciliationLocksAndBindsExactPlan(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, declarations := compactBatchPlanningFixtureInRepo(t, repo)
	wantDeclarations := cloneCompactRecoveryEdges(declarations)

	preparation, err := PrepareCompactBatchReconciliation(
		context.Background(), repo, declarations, compactBatchTestActor, compactBatchTestReason,
	)
	if err != nil {
		t.Fatalf("prepare compact batch reconciliation: %v", err)
	}
	if preparation.Schema != CompactBatchReconcilePreparationSchema ||
		!validSHA256(preparation.RepositorySHA256) || !validSHA256(preparation.PlanSHA256) ||
		!validSHA256(preparation.InvalidEdgesSHA256) || !validSHA256(preparation.ValidDescendantSuffixesSHA256) ||
		!validSHA256(preparation.QuarantineEntriesSHA256) {
		t.Fatalf("preparation guards = %#v", preparation)
	}
	if !reflect.DeepEqual(declarations, wantDeclarations) ||
		!reflect.DeepEqual(preparation.DeclaredInvalidEdges, preparation.Plan.InvalidEdges) {
		t.Fatalf("preparation declarations/plan = %#v", preparation)
	}
	if preparation.RequiredMaintainerAuthorization == "" ||
		strings.Contains(preparation.RequiredMaintainerAuthorization, preContractFixtureAuthorization) ||
		!strings.Contains(preparation.RequiredMaintainerAuthorization, "plan_sha256="+preparation.PlanSHA256) ||
		!strings.Contains(preparation.RequiredMaintainerAuthorization, "valid_descendant_suffixes_sha256="+preparation.ValidDescendantSuffixesSHA256) {
		t.Fatalf("preparation authorization binding = %q", preparation.RequiredMaintainerAuthorization)
	}

	request := compactBatchRequestFromPreparation(preparation)
	validated, err := validateCompactBatchReconcileRequest(context.Background(), repo, request)
	if err != nil {
		t.Fatalf("validate exact compact batch request: %v", err)
	}
	if !reflect.DeepEqual(validated, preparation.Plan) {
		t.Fatalf("validated plan = %#v, want %#v", validated, preparation.Plan)
	}

	preparedDeclarations := cloneCompactRecoveryEdges(preparation.DeclaredInvalidEdges)
	declarations[0].AnomalyClasses[0] = "mutated-input"
	if !reflect.DeepEqual(preparation.DeclaredInvalidEdges, preparedDeclarations) {
		t.Fatal("preparation aliases declaration input")
	}
}

func TestValidateCompactBatchReconciliationRejectsGuardAndSnapshotDrift(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*testing.T, string, *CompactBatchReconcileRequest)
		wantErr string
	}{
		{
			name: "plan digest changed",
			mutate: func(_ *testing.T, _ string, request *CompactBatchReconcileRequest) {
				request.ExpectedPlanSHA256 = hash("different-plan")
			},
			wantErr: "expected plan digest does not bind the supplied plan",
		},
		{
			name: "valid descendant suffix changed",
			mutate: func(_ *testing.T, _ string, request *CompactBatchReconcileRequest) {
				request.ExpectedPlan.ValidDescendantSuffixes[0].Entries[0].Revision = hash("different-suffix")
			},
			wantErr: "expected plan digest does not bind the supplied plan",
		},
		{
			name: "maintainer binding changed",
			mutate: func(_ *testing.T, _ string, request *CompactBatchReconcileRequest) {
				request.MaintainerAuthorization += "\nextra=true"
			},
			wantErr: "exact maintainer authorization binding",
		},
		{
			name: "new undeclared anomaly",
			mutate: func(t *testing.T, repo string, _ *CompactBatchReconcileRequest) {
				inspectRecoveryPair(t, repo, "late-invalid", false, preContractFixtureAuthorization)
			},
			wantErr: "declaration set is incomplete",
		},
		{
			name: "stale source revision",
			mutate: func(t *testing.T, repo string, request *CompactBatchReconcileRequest) {
				lineage := "independent-successor"
				store, err := CompactAuthoritativeStore(context.Background(), repo, lineage)
				inspectNoError(t, err)
				record, err := store.Load()
				inspectNoError(t, err)
				record.State.Recovery.RecoveredAt = record.State.Recovery.RecoveredAt.Add(time.Minute)
				writeCompactFixtureRecord(t, store, record.State)
			},
			wantErr: "locked recovery edge",
		},
		{
			name: "corrupt compact evidence",
			mutate: func(t *testing.T, repo string, request *CompactBatchReconcileRequest) {
				lineage := request.ExpectedPlan.QuarantineEntries[0].LineageID
				store, err := CompactAuthoritativeStore(context.Background(), repo, lineage)
				inspectNoError(t, err)
				inspectNoError(t, os.WriteFile(store.StatePath(), []byte("not json\n"), 0o644))
			},
			wantErr: "inspection is incomplete",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			_, declarations := compactBatchPlanningFixtureInRepo(t, repo)
			preparation, err := PrepareCompactBatchReconciliation(context.Background(), repo, declarations, compactBatchTestActor, compactBatchTestReason)
			if err != nil {
				t.Fatal(err)
			}
			request := compactBatchRequestFromPreparation(preparation)
			tt.mutate(t, repo, &request)
			if _, err := validateCompactBatchReconcileRequest(context.Background(), repo, request); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validation error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestPrepareCompactBatchReconciliationCoordinatesMaintenanceThenV2Lock(t *testing.T) {
	t.Run("maintenance contention honors cancellation", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		_, declarations := compactBatchPlanningFixtureInRepo(t, repo)
		base, _, err := reviewAuthorityRoot(context.Background(), repo)
		inspectNoError(t, err)
		held, err := acquireMaintenanceLock(context.Background(), compactMaintenanceLockPath(base), maintenanceExclusive)
		inspectNoError(t, err)
		defer held.Release()
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
		defer cancel()
		if _, err := PrepareCompactBatchReconciliation(ctx, repo, declarations, compactBatchTestActor, compactBatchTestReason); !errors.Is(err, ErrAuthorityLockCancelled) {
			t.Fatalf("maintenance contention error = %v", err)
		}
	})

	t.Run("v2 contention releases the exclusive maintenance lease", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		_, declarations := compactBatchPlanningFixtureInRepo(t, repo)
		base, _, err := reviewAuthorityRoot(context.Background(), repo)
		inspectNoError(t, err)
		v2LockPath := filepath.Join(base, "v2", "LOCK")
		held, err := acquireLocalStoreLock(v2LockPath)
		inspectNoError(t, err)
		if _, err := PrepareCompactBatchReconciliation(context.Background(), repo, declarations, compactBatchTestActor, compactBatchTestReason); !errors.Is(err, ErrConcurrentUpdate) {
			t.Fatalf("v2 contention error = %v", err)
		}
		inspectNoError(t, held.release())
		shared, err := acquireMaintenanceLock(context.Background(), compactMaintenanceLockPath(base), maintenanceShared)
		if err != nil {
			t.Fatalf("exclusive maintenance lease leaked after v2 contention: %v", err)
		}
		inspectNoError(t, shared.Release())
	})
}

func compactBatchRequestFromPreparation(preparation CompactBatchReconcilePreparation) CompactBatchReconcileRequest {
	return CompactBatchReconcileRequest{
		Schema:                  CompactBatchReconcileRequestSchema,
		DeclaredInvalidEdges:    cloneCompactRecoveryEdges(preparation.DeclaredInvalidEdges),
		ExpectedPlan:            preparation.Plan,
		ExpectedPlanSHA256:      preparation.PlanSHA256,
		Actor:                   compactBatchTestActor,
		Reason:                  compactBatchTestReason,
		MaintainerAuthorization: preparation.RequiredMaintainerAuthorization,
	}
}
