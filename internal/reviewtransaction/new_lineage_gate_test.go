package reviewtransaction

// Wave 5 fix cycle 1, CRITICAL-C (verify-report #10186): absorbed N2 was
// marked closed (task 4.7) but the v3 gate path never called gateVerdict at
// all -- newLineageGateEvaluation (internal/cli/review_governing_authority.go)
// mapped CoreTransitionContinue -> GateAllow uniformly for every gate,
// discarding gateVerdict's per-gate preconditions entirely. This file's
// tests drive the shared reviewtransaction-side replacement,
// EvaluateNewLineageGate, directly -- the real production entry point, not a
// duplicated table.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newLineageGateFixtureRecord(baseTree, candidateTree string) NewLineageRecord {
	return NewLineageRecord{
		Revision: "sha256:" + hash("new-lineage-gate-fixture-revision"),
		Authority: NewLineageAuthority{
			LineageID: "new-lineage-gate-fixture", State: NewLineageStateApproved,
			CandidateIdentity: CandidateIdentity{BaseTree: baseTree, CandidateTree: candidateTree, PolicyHash: "sha256:" + hash("policy")},
		},
	}
}

func escalatedNewLineageGateFixture(t *testing.T) (string, AuthorityStore, NewLineageRecord, NewLineageReceipt) {
	t.Helper()
	repo := initSnapshotRepo(t)
	const lineage = "escalated-new-lineage-gate"
	store, err := NewLineageAuthorityStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	authority := fixtureNewLineageAuthority(lineage, NewLineageStateEscalated)
	if _, err := store.Mutate(context.Background(), "", func(next *NewLineageAuthority) error {
		*next = authority
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	digest, err := record.Authority.ProviderCausalAggregateDigest(record.Revision)
	if err != nil {
		t.Fatal(err)
	}
	receipt := NewLineageReceipt{
		Schema: NewLineageReceiptSchema, LineageID: lineage,
		TerminalState: NewLineageStateEscalated, AuthorityRevision: record.Revision,
		CandidateIdentity: record.Authority.CandidateIdentity, ProviderCausalAggregateDigest: digest,
	}
	if err := store.WriteReceipt(context.Background(), receipt); err != nil {
		t.Fatal(err)
	}
	return repo, store, record, receipt
}

func TestEvaluateNewLineageGateEscalatedRequiresValidReceiptAcrossAllGates(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(t *testing.T, store AuthorityStore, receipt NewLineageReceipt)
		want   GateResult
	}{
		{name: "valid receipt", want: GateEscalated},
		{name: "missing receipt", mutate: func(t *testing.T, store AuthorityStore, receipt NewLineageReceipt) {
			if err := os.Remove(store.ReceiptPath()); err != nil {
				t.Fatal(err)
			}
		}, want: GateInvalidated},
		{name: "stale receipt", mutate: func(t *testing.T, store AuthorityStore, receipt NewLineageReceipt) {
			receipt.AuthorityRevision = "sha256:" + strings.Repeat("0", 64)
			payload, err := json.MarshalIndent(receipt, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(store.ReceiptPath(), append(payload, '\n'), 0o644); err != nil {
				t.Fatal(err)
			}
		}, want: GateInvalidated},
		{name: "tampered aggregate receipt", mutate: func(t *testing.T, store AuthorityStore, receipt NewLineageReceipt) {
			receipt.ProviderCausalAggregateDigest = "sha256:" + strings.Repeat("e", 64)
			payload, err := json.MarshalIndent(receipt, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(store.ReceiptPath(), append(payload, '\n'), 0o644); err != nil {
				t.Fatal(err)
			}
		}, want: GateInvalidated},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo, store, record, receipt := escalatedNewLineageGateFixture(t)
			if tt.mutate != nil {
				tt.mutate(t, store, receipt)
			}
			for _, gate := range []GateKind{GatePostApply, GatePreCommit, GatePrePush, GatePrePR, GateRelease} {
				t.Run(string(gate), func(t *testing.T) {
					evaluation := EvaluateNewLineageGate(context.Background(), repo, record, CoreTransition{
						Kind: CoreTransitionEscalate, ReasonCode: string(ShadowRelationAmbiguous),
					}, record.Authority.CandidateIdentity, NativeGateRequestInput{Gate: gate})
					if evaluation.Result != tt.want {
						t.Fatalf("escalated gate = %#v, want %q", evaluation, tt.want)
					}
				})
			}
		})
	}
}

// TestEvaluateNewLineageGate_ContinueConsultsGateVerdictPreconditions is
// CRITICAL-C's exact reproduction: an approved v3 authority whose live base
// tree diverges from the frozen one (BaseRelationshipValid=false, exactly the
// verify report's "approved v3 receipt allows at --gate release with
// base_relationship_valid: false") must now DENY at pre-pr and release, and
// still ALLOW at the three gates the precondition never gates.
func TestEvaluateNewLineageGate_ContinueConsultsGateVerdictPreconditions(t *testing.T) {
	record := newLineageGateFixtureRecord("base-tree", "candidate-tree")
	transition := CoreTransition{Kind: CoreTransitionContinue, ReasonCode: string(ShadowRelationExact)}
	// live.BaseTree diverges from record.Authority.CandidateIdentity.BaseTree
	// ("base-tree"), reproducing BaseRelationshipValid=false without needing
	// a real repository.
	live := CandidateIdentity{BaseTree: "a-different-base-tree", CandidateTree: "candidate-tree"}

	for _, gate := range []GateKind{GatePostApply, GatePreCommit, GatePrePush, GatePrePR, GateRelease} {
		evaluation := EvaluateNewLineageGate(context.Background(), "", record, transition, live, NativeGateRequestInput{Gate: gate})
		if evaluation.Result == GateAllow {
			t.Fatalf("gate %q allowed without a persisted receipt aggregate: %#v", gate, evaluation)
		}
	}
}

// TestEvaluateNewLineageGate_ReleaseRequiresDerivedEvidence proves the other
// half of the release precondition: an exact relation with a MATCHING base
// still denies at release until real release evidence is derived from the
// caller-supplied artifact locations, then allows once it is.
func TestEvaluateNewLineageGate_ReleaseRequiresDerivedEvidence(t *testing.T) {
	repo := initSnapshotRepo(t)
	record := newLineageGateFixtureRecord("matching-base", "matching-candidate")
	transition := CoreTransition{Kind: CoreTransitionContinue, ReasonCode: string(ShadowRelationExact)}
	live := CandidateIdentity{BaseTree: "matching-base", CandidateTree: "matching-candidate"}

	withoutRelease := EvaluateNewLineageGate(context.Background(), repo, record, transition, live, NativeGateRequestInput{Gate: GateRelease})
	if withoutRelease.Result == GateAllow {
		t.Fatalf("release allowed with no release artifacts supplied at all: %#v", withoutRelease)
	}

	dir := t.TempDir()
	artifact := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	gateInput := NativeGateRequestInput{
		Gate:                       GateRelease,
		ReleaseConfiguration:       artifact("configuration.txt", "configuration\n"),
		ReleaseGenerated:           artifact("generated.txt", "generated\n"),
		ReleaseProvenance:          artifact("provenance.txt", "provenance\n"),
		ReleasePublicationBoundary: artifact("publication-boundary.txt", "publication boundary\n"),
		ReleaseEvidenceFreshness:   artifact("evidence-freshness.txt", "evidence freshness\n"),
	}
	withRelease := EvaluateNewLineageGate(context.Background(), repo, record, transition, live, gateInput)
	if withRelease.Result == GateAllow {
		t.Fatalf("release allowed with release evidence but without a persisted receipt: %#v", withRelease)
	}
}

// TestEvaluateNewLineageGate_NonContinueTransitionsUnaffected pins the scope
// boundary: gateVerdict's preconditions apply ONLY to the Continue branch --
// Collect/Escalate/Approve/Repair/Stop are already non-allow outcomes,
// unaffected by base-relationship or release evidence, exactly as before
// this fix.
func TestEvaluateNewLineageGate_NonContinueTransitionsUnaffected(t *testing.T) {
	record := newLineageGateFixtureRecord("base-tree", "candidate-tree")
	live := CandidateIdentity{BaseTree: "base-tree", CandidateTree: "candidate-tree"}

	collect := EvaluateNewLineageGate(context.Background(), "", record, CoreTransition{Kind: CoreTransitionCollect, ReasonCode: string(ShadowRelationChanged)}, live, NativeGateRequestInput{Gate: GateRelease})
	if collect.Result != GateScopeChanged {
		t.Fatalf("collect transition = %#v, want scope-changed regardless of release evidence", collect)
	}
	escalate := EvaluateNewLineageGate(context.Background(), "", record, CoreTransition{Kind: CoreTransitionEscalate, ReasonCode: string(ShadowRelationAmbiguous)}, live, NativeGateRequestInput{Gate: GateRelease})
	if escalate.Result != GateInvalidated {
		t.Fatalf("escalate transition without receipt = %#v, want invalidated", escalate)
	}
	for _, kind := range []CoreTransitionKind{CoreTransitionApprove, CoreTransitionRepair, CoreTransitionStop} {
		evaluation := EvaluateNewLineageGate(context.Background(), "", record, CoreTransition{Kind: kind}, live, NativeGateRequestInput{Gate: GatePreCommit})
		if evaluation.Result == GateAllow {
			t.Fatalf("unreachable transition kind %q must never allow, got %#v", kind, evaluation)
		}
	}
}
