package reviewtransaction

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRejectedValidatorDispositionRequiresExactAuthorizedRelease(t *testing.T) {
	ctx := context.Background()
	repo, state := persistCanonicalRejectedValidatorAuthority(t, "release-rejected-validator")
	report, records, err := loadCompactRecoveryRecords(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	record := records[state.LineageID]
	selector := AuthorityDispositionSelector{SuccessorLineageID: state.LineageID, SuccessorExpectedRevision: record.Revision}
	if _, err := DeriveAuthorityDispositionPlanAtRepo(ctx, repo, "operator", "release rejected review"); err == nil {
		t.Fatal("selectorless derivation must not release valid rejected authority")
	}
	plan, err := DeriveAuthorityDispositionPlanAtRepo(ctx, repo, "operator", "release rejected review", selector)
	if err != nil {
		t.Fatalf("exact rejected-validator disposition unavailable: %v", err)
	}
	if plan.AnomalyClass != "rejected_targeted_validator_terminal" || !reflect.DeepEqual(plan.Closure, []string{state.LineageID}) {
		t.Fatalf("unexpected disposition: %#v", plan)
	}
	if !report.Complete || !state.CorrectionAttemptConsumed() || state.State != StateEscalated {
		t.Fatal("fixture must retain conclusive rejection and consumed correction")
	}
	store, err := CompactAuthoritativeStore(ctx, repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	sourceBefore, err := os.ReadFile(filepath.Join(repo, "tracked.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RepairAuthorityDisposition(ctx, repo, plan.PlanDigest, plan.AuthorityInventoryRevision, plan.Actor, plan.Reason, "wrong authorization", selector); err == nil {
		t.Fatal("release without exact authorization succeeded")
	}
	after, err := os.ReadFile(store.StatePath())
	if err != nil || string(after) != string(before) {
		t.Fatalf("rejected authorization changed authority: %v", err)
	}
	authorization := authorityDispositionAuthorizationBinding(plan)
	released, err := RepairAuthorityDisposition(ctx, repo, plan.PlanDigest, plan.AuthorityInventoryRevision, plan.Actor, plan.Reason, authorization, selector)
	if err != nil {
		t.Fatal(err)
	}
	retained, err := os.ReadFile(filepath.Join(released.QuarantinePath, "residue", compactStateFileName))
	if err != nil || string(retained) != string(before) {
		t.Fatalf("release must preserve exact rejected verdict bytes: %v", err)
	}
	if released.AuthorityDisposition == nil || released.AuthorityDisposition.PlanDigest != plan.PlanDigest {
		t.Fatal("missing digest-bound audit proof")
	}
	sourceAfter, err := os.ReadFile(filepath.Join(repo, "tracked.txt"))
	if err != nil || string(sourceAfter) != string(sourceBefore) {
		t.Fatalf("release changed candidate source: %v", err)
	}
	replay, err := RepairAuthorityDisposition(ctx, repo, plan.PlanDigest, plan.AuthorityInventoryRevision, plan.Actor, plan.Reason, authorization, selector)
	if err != nil || replay.QuarantinePath != released.QuarantinePath {
		t.Fatalf("authorized replay did not converge: %#v, %v", replay, err)
	}
}

func TestRejectedValidatorDispositionRejectsStaleOrReferencedAuthority(t *testing.T) {
	ctx := context.Background()
	repo, state := persistCanonicalRejectedValidatorAuthority(t, "release-negative-controls")
	report, records, err := loadCompactRecoveryRecords(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	_, binding, err := authorityRepairRoot(repo)
	if err != nil {
		t.Fatal(err)
	}
	selector := AuthorityDispositionSelector{SuccessorLineageID: state.LineageID, SuccessorExpectedRevision: records[state.LineageID].Revision}
	stale := selector
	stale.SuccessorExpectedRevision = "sha256:stale"
	if _, err := deriveAuthorityDispositionPlan(report, records, binding, "operator", "release", stale); err == nil {
		t.Fatal("stale revision admitted")
	}
	for _, edge := range []CompactRecoveryEdgeInspection{
		{PredecessorLineageID: state.LineageID, SuccessorLineageID: "dependent"},
		{PredecessorLineageID: "parent", SuccessorLineageID: state.LineageID},
	} {
		withEdge := report
		withEdge.Edges = append(append([]CompactRecoveryEdgeInspection{}, report.Edges...), edge)
		if _, err := deriveAuthorityDispositionPlan(withEdge, records, binding, "operator", "release", selector); err == nil {
			t.Fatalf("referenced rejected authority admitted: %#v", edge)
		}
	}
	for _, status := range []State{StateApproved, StateInvalidated, StateCorrectionRequired} {
		changed := records[state.LineageID]
		changed.State.State = status
		modified := map[string]CompactRecord{state.LineageID: changed}
		if _, err := deriveAuthorityDispositionPlan(report, modified, binding, "operator", "release", selector); err == nil {
			t.Fatalf("non-rejected state %q admitted", status)
		}
	}
}
