package reviewtransaction

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type rarVerificationFixture struct {
	repo        string
	repository  *RARAuthorityRepository
	publication RARAuthorityPublication
}

// newRARVerificationFixture seeds one exact published RAR verification
// authority: a real Git repository holding an approved historical v1 review
// authority, plus the owner contracts bound to that exact native receipt.
func newRARVerificationFixture(t *testing.T, name string) rarVerificationFixture {
	t.Helper()
	ctx := context.Background()
	repo := initSnapshotRepo(t)
	lineage := "authority-lineage"

	store, err := AuthoritativeStore(ctx, repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	registry := verificationTestRegistry(t, []string{})
	writeSnapshotFile(t, repo, "tracked.txt", "verified candidate "+name+"\n")
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(ctx, Target{
		Kind: TargetCurrentChanges, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := NewTransaction(Start{
		LineageID: lineage, Mode: ModeOrdinary4R, Generation: 1,
		Snapshot: snapshot, PolicyHash: registry.PolicyHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	head, err := store.Append("", Record{Operation: "review/start", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	if err := freezeTestFindings(tx, []Finding{}); err != nil {
		t.Fatal(err)
	}
	if head, err = store.Append(head, Record{Operation: "review/freeze-findings", Transaction: *tx}); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ClassifyEvidence([]FindingEvidence{}); err != nil {
		t.Fatal(err)
	}
	if head, err = store.Append(head, Record{Operation: "review/classify-evidence", Transaction: *tx}); err != nil {
		t.Fatal(err)
	}
	if err := tx.BeginFinalVerification(); err != nil {
		t.Fatal(err)
	}
	if head, err = store.Append(head, Record{Operation: "review/begin-final-verification", Transaction: *tx}); err != nil {
		t.Fatal(err)
	}
	if err := tx.CompleteFinalVerification(hash("2"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(head, Record{Operation: "review/complete-final-verification", Transaction: *tx}); err != nil {
		t.Fatal(err)
	}
	receipt, err := tx.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	receiptPayload, err := canonicalRARReceiptPayload(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(store.Dir, "artifacts"), 0o700); err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(store.Dir, "artifacts", "receipt.json")
	if err := os.WriteFile(receiptPath, receiptPayload, 0o600); err != nil {
		t.Fatal(err)
	}
	receiptRef, err := HashArtifact(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	subject, err := VerificationSubjectFromSnapshot(tx.Snapshot)
	if err != nil {
		t.Fatal(err)
	}

	applicability := verificationTestApplicability(t, registry, subject.CandidateTree, subject.SnapshotIdentity)
	applicability.Subject = subject
	if applicability.Digest, err = verificationApplicabilityDigest(applicability); err != nil {
		t.Fatal(err)
	}
	if err := applicability.Validate(); err != nil {
		t.Fatal(err)
	}
	plan, err := BuildVerificationPlan(applicability, registry)
	if err != nil {
		t.Fatal(err)
	}
	result := VerificationResultRef{
		Schema: VerificationResultRefSchema, ResultRef: verificationTestHash(name + "-result"),
		Subject: plan.Subject, PolicyHash: plan.PolicyHash,
		PlanDigest: plan.Digest, ApplicabilityDigest: plan.ApplicabilityDigest,
		Aggregate:            VerificationAggregateComplete,
		CompletedObligations: verificationObligationIDs(plan.Obligations),
		EvidenceRefs:         []string{},
	}
	if err := ValidateVerificationResultRef(applicability, registry, plan, result); err != nil {
		t.Fatal(err)
	}
	repository, err := OpenRARAuthorityRepository(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	return rarVerificationFixture{
		repo:       repo,
		repository: repository,
		publication: RARAuthorityPublication{
			LineageID: lineage, ReceiptRef: receiptRef, Applicability: applicability,
			Registry: registry, Plan: plan, Result: result,
		},
	}
}

// TestRARVerificationAuthorityConvergesOnExhaustedRepositoryLock is the
// deterministic reproduction of the #3239 shape: an exact replay of an
// already-published RAR verification authority exhausts the bounded wait on
// the repository LOCK. The lock is the real advisory primitive held on the
// real LOCK path; only the timing is scripted. The honest outcome is
// convergence on the published authority, not a timeout for work that
// already succeeded.
func TestRARVerificationAuthorityConvergesOnExhaustedRepositoryLock(t *testing.T) {
	fixture := newRARVerificationFixture(t, "authority-lock-converge")
	published, err := fixture.repository.Publish(context.Background(), fixture.publication)
	if err != nil {
		t.Fatal(err)
	}
	held, err := acquireRARAuthorityLock(context.Background(), filepath.Join(fixture.repository.root, "LOCK"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.release() }()

	converged, err := fixture.repository.Publish(context.Background(), fixture.publication)
	if err != nil {
		t.Fatalf("Publish() behind a continuously-held repository LOCK with the exact published pair = %v, want convergence", err)
	}
	if !reflect.DeepEqual(published, converged) {
		t.Fatalf("converged verification authority diverged:\npublished=%#v\nconverged=%#v", published, converged)
	}
}

// TestRARVerificationAuthorityLockExhaustionWithoutConvergentPairStaysTyped is
// the guard on the convergence above: exhaustion with genuinely divergent
// state — no published pair, or different contracts addressed to the same
// pair — must keep failing with the typed *AuthorityLockTimeoutError and must
// not converge on a foreign authority. The native-lock subtest proves the
// convergence predicate never accepts the on-disk pair while live native
// authority is inaccessible, and the stale-receipt subtest proves it
// revalidates the live receipt instead of trusting the published bytes; the
// no-pair and divergent subtests prove a false predicate preserves the
// caller's typed timeout, so the properties compose. Do not relax this test
// to make contention disappear.
func TestRARVerificationAuthorityLockExhaustionWithoutConvergentPairStaysTyped(t *testing.T) {
	t.Run("no published pair", func(t *testing.T) {
		fixture := newRARVerificationFixture(t, "authority-lock-missing")
		if err := ensureRARRepositoryRoot(fixture.repository.identity.GitCommonDir, fixture.repository.root, true); err != nil {
			t.Fatal(err)
		}
		held, err := acquireRARAuthorityLock(context.Background(), filepath.Join(fixture.repository.root, "LOCK"))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = held.release() }()

		_, err = fixture.repository.Publish(context.Background(), fixture.publication)
		if !errors.Is(err, ErrAuthorityLockTimeout) {
			t.Fatalf("Publish() behind a held lock without a published pair = %v, want %v", err, ErrAuthorityLockTimeout)
		}
	})

	t.Run("divergent contracts at the exact pair", func(t *testing.T) {
		fixture := newRARVerificationFixture(t, "authority-lock-divergent")
		if _, err := fixture.repository.Publish(context.Background(), fixture.publication); err != nil {
			t.Fatal(err)
		}
		held, err := acquireRARAuthorityLock(context.Background(), filepath.Join(fixture.repository.root, "LOCK"))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = held.release() }()

		divergent := fixture.publication
		divergent.Result.Aggregate = VerificationAggregatePartial
		_, err = fixture.repository.Publish(context.Background(), divergent)
		if !errors.Is(err, ErrAuthorityLockTimeout) {
			t.Fatalf("Publish() behind a held lock with divergent contracts = %v, want %v", err, ErrAuthorityLockTimeout)
		}
	})

	t.Run("native receipt lock held at the exact pair", func(t *testing.T) {
		fixture := newRARVerificationFixture(t, "authority-lock-native")
		if _, err := fixture.repository.Publish(context.Background(), fixture.publication); err != nil {
			t.Fatal(err)
		}
		held, err := acquireMaintenanceLock(
			context.Background(), rarNativeMaintenanceLockPath(t, fixture.repository), maintenanceExclusive,
		)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = held.Release() }()

		// The exact pair is on disk, but the convergence predicate must not
		// accept it while the native receipt maintenance lock is inaccessible:
		// its ResolveReceiptResult call revalidates live native authority through
		// the same lock and fails, so the replay must preserve the original
		// typed timeout instead of converging.
		_, err = fixture.repository.Publish(context.Background(), fixture.publication)
		if !errors.Is(err, ErrAuthorityLockTimeout) || errors.Is(err, ErrRARAuthorityStale) {
			t.Fatalf("Publish() behind the held native receipt maintenance lock with the exact published pair = %v, want %v preserved", err, ErrAuthorityLockTimeout)
		}
	})

	t.Run("stale live receipt refuses convergence at the exact pair", func(t *testing.T) {
		fixture := newRARVerificationFixture(t, "authority-lock-stale")
		if _, err := fixture.repository.Publish(context.Background(), fixture.publication); err != nil {
			t.Fatal(err)
		}
		store, err := AuthoritativeStore(context.Background(), fixture.repo, fixture.publication.LineageID)
		if err != nil {
			t.Fatal(err)
		}
		receiptPath := filepath.Join(store.Dir, "artifacts", "receipt.json")
		if err := os.WriteFile(receiptPath, []byte("{\"tampered\":true}\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		// The convergence path must revalidate live native authority through
		// ResolveReceiptResult, so a receipt that no longer matches the
		// repository keeps it from accepting the on-disk pair. The local
		// convergence closure is intentionally not directly callable, so this
		// asserts the exact resolver contract the closure depends on.
		if _, err := fixture.repository.ResolveReceiptResult(
			context.Background(), fixture.publication.ReceiptRef, fixture.publication.Result.ResultRef,
		); !errors.Is(err, ErrRARAuthorityStale) {
			t.Fatalf("ResolveReceiptResult() on a tampered live receipt = %v, want %v", err, ErrRARAuthorityStale)
		}
	})
}

// rarNativeMaintenanceLockPath returns the exact REVIEW-MAINTENANCE.lock that
// lockNativeReceipt acquires for this repository, derived the same way.
func rarNativeMaintenanceLockPath(t *testing.T, repository *RARAuthorityRepository) string {
	t.Helper()
	base, _, err := reviewAuthorityRoot(context.Background(), repository.identity.RepositoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	return compactMaintenanceLockPath(base)
}
