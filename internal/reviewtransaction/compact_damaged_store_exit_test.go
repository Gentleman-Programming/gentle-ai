package reviewtransaction

// The damaged-store friction benchmark (bench/journeys ds01-ds05) measured
// four refusals that stop the operator without naming what actually clears
// the block. These tests pin the honest replacements:
//
//   - `review reclaim` on an entry holding authority names `review abandon`,
//     which accepts every shape (pristine, captured, corrupt or unreadable).
//   - `review start` over an invalid authority graph names the sanctioned exit
//     the read-only inspection proves, instead of stopping at the bare graph
//     violation.
//
// Every named continuation is then DRIVEN, so a future refusal whose named
// command dead-ends fails here, not in the field.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// truncateCompactStateFile leaves the first half of a persisted record, which
// is what a process killed mid-write leaves behind: bytes that no longer parse
// at all.
func truncateCompactStateFile(t *testing.T, store CompactStore) []byte {
	t.Helper()
	payload, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	truncated := payload[:len(payload)/2]
	if err := os.WriteFile(store.StatePath(), truncated, 0o644); err != nil {
		t.Fatal(err)
	}
	return truncated
}

// abandonPerEligibility runs the abandonment exactly as an operator following
// a refusal's printed command would: only the lineage, actor and reason.
func abandonPerEligibility(t *testing.T, repo, lineage, _ string) CompactReclaimRecord {
	t.Helper()
	ctx := context.Background()
	eligibility, err := InspectCompactPristineAbandonment(ctx, repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	if !eligibility.Eligible {
		t.Fatalf("successor %q is not abandonable; the named continuation does not run", lineage)
	}
	record, err := AbandonCompactStore(ctx, repo, CompactAbandonRequest{
		LineageID: lineage, Reason: CompactAbandonReasonOperatorDisposition, Actor: "maintainer@example.com",
	})
	if err != nil {
		t.Fatalf("abandon %q: %v", lineage, err)
	}
	if record.Status != CompactReclaimCommitted {
		t.Fatalf("abandon %q record = %#v", lineage, record)
	}
	return record.CompactReclaimRecord
}

func requireAuthoritativeInventory(t *testing.T, repo string) {
	t.Helper()
	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || !report.Authoritative {
		t.Fatalf("store does not govern after the named continuation ran: %#v", report)
	}
}

// TestReconcileRefusesUnreadableSuccessorWithDiagnosis used to pin the ds05
// shape directly against ReconcileInvalidRecoveryEdge: an unreadable
// successor record answered a bare decoder error rather than the
// inspection's own classification. Wave 7 S3b retired that provider (its
// verb, `review reconcile-authority`, retired one slice earlier in S3a) --
// the identical shape is already covered, end to end, by
// TestReclaimRefusalNamesTheOperationThatAdmitsTheShape's own "unreadable
// record" subtest below, which drives `review reclaim`'s refusal (the
// retained, live surface) over the same truncated-record fixture and proves
// the same inspection classification.

// TestReclaimRefusalNamesTheOperationThatAdmitsTheShape pins the reclaim →
// reconcile circle the benchmark measured. reclaim refused every
// authority-holding entry with the same prose pointer at reconcile-authority,
// including entries reconcile then refuses (forged bindings) or cannot even
// load (truncated records). The refusal now names the operation that admits
// the entry's actual shape, and each named operation is driven to prove it
// clears the block.
func TestReclaimRefusalNamesTheOperationThatAdmitsTheShape(t *testing.T) {
	ctx := context.Background()

	reclaim := func(t *testing.T, repo, lineage string) string {
		t.Helper()
		_, err := ReclaimIncompleteCompactStore(ctx, repo, CompactReclaimRequest{
			LineageID: lineage, Reason: "clear the damaged entry", Actor: "maintainer@example.com",
		})
		if err == nil {
			t.Fatal("reclaim accepted a store entry holding authority")
		}
		return err.Error()
	}

	t.Run("pristine forged successor names abandon", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		_, successor, _ := forgedRecoveryPair(t, repo, "reclaim", "forged reclaim target\n")
		refusal := reclaim(t, repo, successor.State.LineageID)
		for _, want := range []string{
			"gentle-ai review abandon",
			"--lineage \"" + successor.State.LineageID + "\"",
		} {
			if !strings.Contains(refusal, want) {
				t.Fatalf("reclaim refusal does not name %q:\n%s", want, refusal)
			}
		}
		if strings.Contains(refusal, "gentle-ai review reconcile-authority") {
			t.Fatalf("reclaim names reconcile for an edge reconcile refuses (named dead end):\n%s", refusal)
		}
		abandonPerEligibility(t, repo, successor.State.LineageID, "clear the damaged entry")
		requireAuthoritativeInventory(t, repo)
	})

	t.Run("successor holding review metadata names abandonment", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		_, successor, _ := forgedRecoveryPair(t, repo, "reclaim-captured", "forged captured reclaim target\n", func(state *CompactState) {
			store, err := CompactAuthoritativeStore(t.Context(), repo, state.LineageID)
			if err != nil {
				t.Fatal(err)
			}
			for order := range state.SelectedLenses {
				captureCompactLens(t, store, *state, order)
			}
			record, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			recovery := state.Recovery
			*state = record.State
			state.Recovery = recovery
			view, err := state.CompactReviewView()
			if err != nil {
				t.Fatal(err)
			}
			if err := state.CompleteReview(CompactReviewInput{LensResults: view.LensResults, RefuterOutcomes: view.RefuterOutcomes}); err != nil {
				t.Fatal(err)
			}
		})
		refusal := reclaim(t, repo, successor.State.LineageID)
		for _, want := range []string{
			"gentle-ai review abandon",
		} {
			if !strings.Contains(refusal, want) {
				t.Fatalf("reclaim refusal does not carry %q:\n%s", want, refusal)
			}
		}
		abandonPerEligibility(t, repo, successor.State.LineageID, "clear the damaged entry")
		requireAuthoritativeInventory(t, repo)
	})

	t.Run("unreadable record names abandon, which quarantines it uninterpreted", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		_, successor, successorStore := forgedRecoveryPair(t, repo, "reclaim-halved", "half-written reclaim target\n")
		truncated := truncateCompactStateFile(t, successorStore)
		refusal := reclaim(t, repo, successor.State.LineageID)
		for _, want := range []string{
			"gentle-ai review abandon",
			"--lineage \"" + successor.State.LineageID + "\"",
		} {
			if !strings.Contains(refusal, want) {
				t.Fatalf("reclaim refusal does not carry %q:\n%s", want, refusal)
			}
		}
		if strings.Contains(refusal, "gentle-ai review reconcile-authority") {
			t.Fatalf("reclaim names a retired verb:\n%s", refusal)
		}
		record := abandonPerEligibility(t, repo, successor.State.LineageID, "clear the damaged entry")
		moved, err := os.ReadFile(filepath.Join(record.QuarantinePath, "residue", compactStateFileName))
		if err != nil || !bytes.Equal(moved, truncated) {
			t.Fatalf("quarantine did not byte-preserve the unreadable record: %v", err)
		}
		requireAuthoritativeInventory(t, repo)
	})
}
