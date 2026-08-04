package reviewtransaction

import (
	"context"
	"testing"
)

func TestLegacyAndCompactGatesDoNotRequireV3ProviderAggregate(t *testing.T) {
	t.Run("legacy receipt", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		_, chain, receiptPath := legacyApprovedChainFixture(t, repo, "legacy-no-v3-aggregate")
		tx := chain.Records[len(chain.Records)-1].Transaction
		live, err := FreezeCandidateIdentity(context.Background(), repo, tx.Snapshot, tx.PolicyHash)
		if err != nil {
			t.Fatal(err)
		}
		evaluation := EvaluateLegacyGate(context.Background(), repo, chain, receiptPath, live, true, CoreValidateEvidence{
			LiveSnapshot: tx.Snapshot, ApplicableAuthorities: 1,
		}, NativeGateRequestInput{Gate: GatePreCommit})
		if evaluation.Result != GateAllow {
			t.Fatalf("legacy gate = %#v, want allow without v3 provider aggregate", evaluation)
		}
	})

	t.Run("compact receipt", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		state, _, receipt := approvedCompactCurrentChangesFixture(t, repo, "compact-no-v3-aggregate", []string{})
		evaluation := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{
			Gate: GatePostApply, LineageID: state.LineageID,
		})
		if evaluation.Result != GateAllow {
			t.Fatalf("compact gate = %#v, want allow without v3 provider aggregate", evaluation)
		}
	})
}
