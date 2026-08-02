package reviewtransaction

// The damaged-store friction benchmark (bench/journeys ds01–ds05) measured
// four refusals that stop the operator without naming what actually clears the
// block, plus one continuation that names an operation which then cannot even
// load its target. These tests pin the honest replacements:
//
//   - `review reclaim` on an entry holding authority names the operation that
//     admits THAT entry's shape — reconcile for a reconcilable anomaly class,
//     abandon for a pristine entry the abandonment gate's own prediction
//     accepts — and, when nothing admits it, says so precisely and names the
//     machine-readable diagnosis instead of a command that would then refuse.
//   - `review reconcile-authority` on a half-written record refuses with the
//     inspection's own classification (malformed_compact_state) rather than
//     dying on the JSON decoder's `unexpected EOF`.
//   - `review start` over an invalid authority graph names the sanctioned exit
//     the read-only inspection proves, exactly as the reconcile refusal
//     already does, instead of stopping at the bare graph violation.
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
// a refusal's template would: the persisted values come from the same
// read-only prediction the refusal rendered, and only the operator-owned
// actor and reason are supplied here.
func abandonPerEligibility(t *testing.T, repo, lineage, reason string) CompactReclaimRecord {
	t.Helper()
	ctx := context.Background()
	eligibility, err := InspectCompactPristineAbandonment(ctx, repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	if !eligibility.Eligible {
		t.Fatalf("successor %q is not abandonable; the named continuation does not run", lineage)
	}
	request := CompactAbandonRequest{
		LineageID: lineage, ExpectedRevision: eligibility.Revision,
		Reason: reason, Actor: "maintainer@example.com",
	}
	request.MaintainerAuthorization = RenderCompactAbandonAuthorization(
		lineage, eligibility.Revision, eligibility.SnapshotIdentity, request.Actor, request.Reason)
	record, err := AbandonPristineCompactStore(ctx, repo, request)
	if err != nil {
		t.Fatalf("abandon %q: %v", lineage, err)
	}
	if record.Status != CompactReclaimCommitted {
		t.Fatalf("abandon %q record = %#v", lineage, record)
	}
	return record
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

// TestReconcileRefusesUnreadableSuccessorWithDiagnosis pins the ds05 shape:
// the operation the reclaim refusal names for an invalid recovery successor
// answered `load reconcile successor: unexpected EOF` for a record truncated
// mid-write — the continuation named for the case could not load the case. A
// decoder error is never an operator-facing answer. Reconciliation cannot
// admit an unreadable record — its whole design re-derives proof from readable
// state, and admitting bytes that prove nothing is a maintainer policy
// decision — so the refusal must say exactly that, carry the inspection's own
// classification, and name the diagnosis to capture.
func TestReconcileRefusesUnreadableSuccessorWithDiagnosis(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)
	predecessor, successor, successorStore := forgedRecoveryPair(t, repo, "halved", "half-written target\n")
	truncated := truncateCompactStateFile(t, successorStore)

	_, err := ReconcileInvalidRecoveryEdge(ctx, repo, forgedReconcileRequest(predecessor, successor))
	if err == nil {
		t.Fatal("reconcile admitted an unreadable successor record")
	}
	refusal := err.Error()
	if strings.HasPrefix(refusal, "load reconcile successor:") {
		t.Fatalf("a bare decoder error is not an operator-facing answer: %q", refusal)
	}
	for _, want := range []string{
		"malformed_compact_state",
		"maintainer policy decision",
		"gentle-ai review inspect-authority",
	} {
		if !strings.Contains(refusal, want) {
			t.Fatalf("reconcile refusal does not carry %q:\n%s", want, refusal)
		}
	}

	// The refusal mutated nothing: the truncated bytes stay exactly as found.
	after, err := os.ReadFile(successorStore.StatePath())
	if err != nil || !bytes.Equal(after, truncated) {
		t.Fatalf("refused reconcile mutated the unreadable record: %v", err)
	}

	// The named diagnosis answers: inspection classifies the entry exactly as
	// the refusal claimed.
	report, err := InspectCompactRecoveryEdges(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if report.Complete || report.Totals.EntryDiagnostics != 1 {
		t.Fatalf("inspection does not report the unreadable entry: %#v", report.Totals)
	}
	diagnostic := report.EntryDiagnostics[0]
	if diagnostic.LineageID != successor.State.LineageID || diagnostic.Problem != "malformed_compact_state" {
		t.Fatalf("inspection diagnostic = %#v", diagnostic)
	}
}

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

	t.Run("reconcilable pre-contract edge names reconcile with the persisted values", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor, _, successor, _ := preContractRecoveryFixture(t, repo, preContractFixtureAuthorization, nil)
		refusal := reclaim(t, repo, successor.State.LineageID)
		for _, want := range []string{
			"gentle-ai review reconcile-authority",
			"--predecessor-lineage \"" + predecessor.State.LineageID + "\"",
			"--expected-predecessor-revision \"" + predecessor.Revision + "\"",
			"--successor-lineage \"" + successor.State.LineageID + "\"",
			"--expected-successor-revision \"" + successor.Revision + "\"",
			compactReconcileAuthorizationSchema,
		} {
			if !strings.Contains(refusal, want) {
				t.Fatalf("reclaim refusal does not name %q:\n%s", want, refusal)
			}
		}
		// Drive exactly the operation the refusal named; the block clears.
		if _, err := ReconcileInvalidRecoveryEdge(ctx, repo, preContractReconcileRequest(predecessor, successor)); err != nil {
			t.Fatalf("the named reconcile does not run: %v", err)
		}
		requireAuthoritativeInventory(t, repo)
	})

	t.Run("pristine forged successor names abandon", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		_, successor, _ := forgedRecoveryPair(t, repo, "reclaim", "forged reclaim target\n")
		refusal := reclaim(t, repo, successor.State.LineageID)
		for _, want := range []string{
			"gentle-ai review abandon",
			"--lineage \"" + successor.State.LineageID + "\"",
			"--expected-revision \"" + successor.Revision + "\"",
			CompactAbandonAuthorizationSchema,
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

	t.Run("successor holding captured results names no clearing command", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		_, successor, _ := forgedRecoveryPair(t, repo, "reclaim-captured", "forged captured reclaim target\n", func(state *CompactState) {
			results := make([]LensResult, 0, len(state.SelectedLenses))
			for _, lens := range state.SelectedLenses {
				results = append(results, LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"reviewed once"}})
			}
			if err := state.CompleteReview(CompactReviewInput{
				LensResults: results, Classifications: []FindingEvidence{}, RefuterOutcomes: []EvidenceResult{},
			}); err != nil {
				t.Fatal(err)
			}
		})
		refusal := reclaim(t, repo, successor.State.LineageID)
		for _, want := range []string{
			"never discarded",
			"gentle-ai review inspect-authority",
		} {
			if !strings.Contains(refusal, want) {
				t.Fatalf("reclaim refusal does not carry %q:\n%s", want, refusal)
			}
		}
		for _, deadEnd := range []string{"gentle-ai review abandon", "gentle-ai review reconcile-authority"} {
			if strings.Contains(refusal, deadEnd) {
				t.Fatalf("reclaim names %q for a shape it does not clear (named dead end):\n%s", deadEnd, refusal)
			}
		}
	})

	t.Run("unreadable record names the diagnosis, not an operation that cannot load it", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		_, successor, successorStore := forgedRecoveryPair(t, repo, "reclaim-halved", "half-written reclaim target\n")
		truncateCompactStateFile(t, successorStore)
		refusal := reclaim(t, repo, successor.State.LineageID)
		for _, want := range []string{
			"malformed_compact_state",
			"gentle-ai review inspect-authority",
		} {
			if !strings.Contains(refusal, want) {
				t.Fatalf("reclaim refusal does not carry %q:\n%s", want, refusal)
			}
		}
		for _, deadEnd := range []string{"gentle-ai review abandon", "gentle-ai review reconcile-authority"} {
			if strings.Contains(refusal, deadEnd) {
				t.Fatalf("reclaim names %q for a record neither can load (named dead end):\n%s", deadEnd, refusal)
			}
		}
	})
}

// TestStartOverInvalidGraphRefusalNamesSanctionedExit pins the `review start`
// half: a fresh start over a damaged authority graph refused with the bare
// graph violation and named nothing. The refusal now carries the sanctioned
// exit the read-only inspection proves — the same pattern the reconcile
// refusal already uses — and driving that exit makes the same start succeed.
func TestStartOverInvalidGraphRefusalNamesSanctionedExit(t *testing.T) {
	ctx := context.Background()

	freshStart := func(t *testing.T, repo, lineage string) error {
		t.Helper()
		writeSnapshotFile(t, repo, "tracked.txt", "fresh start target for "+lineage+"\n")
		_, err := StartCompactAuthority(ctx, repo, CompactStartRequest{State: newCompactTestState(t, repo, lineage)})
		return err
	}

	t.Run("dangling predecessor names the abandonment that clears it", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor, successor, successorStore := forgedRecoveryPair(t, repo, "dangling", "dangling start target\n")
		if err := os.RemoveAll(filepath.Join(filepath.Dir(successorStore.Dir), predecessor.State.LineageID)); err != nil {
			t.Fatal(err)
		}
		err := freshStart(t, repo, "start-over-dangling")
		if err == nil {
			t.Fatal("start succeeded over a dangling predecessor")
		}
		refusal := err.Error()
		for _, want := range []string{
			"dangling predecessor",
			"gentle-ai review abandon",
			"--lineage \"" + successor.State.LineageID + "\"",
			"--expected-revision \"" + successor.Revision + "\"",
			CompactAbandonAuthorizationSchema,
			"gentle-ai review inspect-authority",
		} {
			if !strings.Contains(refusal, want) {
				t.Fatalf("start refusal does not name %q:\n%s", want, refusal)
			}
		}
		abandonPerEligibility(t, repo, successor.State.LineageID, "its predecessor is gone")
		if err := freshStart(t, repo, "start-over-dangling"); err != nil {
			t.Fatalf("start still refuses after the named exit ran: %v", err)
		}
	})

	t.Run("pre-contract edge names the reconcile that clears it", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor, _, successor, _ := preContractRecoveryFixture(t, repo, preContractFixtureAuthorization, nil)
		err := freshStart(t, repo, "start-over-pre-contract")
		if err == nil {
			t.Fatal("start succeeded over an invalid recovery edge")
		}
		refusal := err.Error()
		for _, want := range []string{
			"exact maintainer authorization binding",
			"gentle-ai review reconcile-authority",
			"--expected-predecessor-revision \"" + predecessor.Revision + "\"",
			"--expected-successor-revision \"" + successor.Revision + "\"",
			compactReconcileAuthorizationSchema,
		} {
			if !strings.Contains(refusal, want) {
				t.Fatalf("start refusal does not name %q:\n%s", want, refusal)
			}
		}
		if _, err := ReconcileInvalidRecoveryEdge(ctx, repo, preContractReconcileRequest(predecessor, successor)); err != nil {
			t.Fatalf("the named reconcile does not run: %v", err)
		}
		if err := freshStart(t, repo, "start-over-pre-contract"); err != nil {
			t.Fatalf("start still refuses after the named exit ran: %v", err)
		}
	})

	t.Run("forged pristine successor names the abandonment that clears it", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		_, successor, _ := forgedRecoveryPair(t, repo, "start", "forged start target\n")
		err := freshStart(t, repo, "start-over-forged")
		if err == nil {
			t.Fatal("start succeeded over a forged recovery edge")
		}
		refusal := err.Error()
		for _, want := range []string{
			"gentle-ai review abandon",
			"--expected-revision \"" + successor.Revision + "\"",
			CompactAbandonAuthorizationSchema,
		} {
			if !strings.Contains(refusal, want) {
				t.Fatalf("start refusal does not name %q:\n%s", want, refusal)
			}
		}
		abandonPerEligibility(t, repo, successor.State.LineageID, "the recovery edge cannot be admitted")
		if err := freshStart(t, repo, "start-over-forged"); err != nil {
			t.Fatalf("start still refuses after the named exit ran: %v", err)
		}
	})
}
