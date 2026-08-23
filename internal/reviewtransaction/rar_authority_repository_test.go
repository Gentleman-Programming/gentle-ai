package reviewtransaction

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"testing"
)

type rarCompactFixture struct {
	repo       string
	state      CompactState
	store      CompactStore
	receipt    CompactReceipt
	receiptRef string
	repository *RARAuthorityRepository
	request    RARAuthorityPublication
}

func TestRARAuthorityPublishesAndResolvesExactNativePreimages(t *testing.T) {
	fixture := newRARCompactFixture(t, "rar-exact")

	published, err := fixture.repository.Publish(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if published.Receipt.Version != RARReceiptCompactV2 ||
		published.Receipt.Compact == nil ||
		!CompactReceiptEqual(*published.Receipt.Compact, fixture.receipt) ||
		published.Receipt.ReceiptRef != fixture.receiptRef ||
		published.Result.ResultRef != fixture.request.Result.ResultRef {
		t.Fatalf("published RAR authority = %#v", published)
	}

	byResult, err := fixture.repository.ResolveResult(
		context.Background(),
		fixture.request.Result.ResultRef,
	)
	if err != nil {
		t.Fatal(err)
	}
	byPair, err := fixture.repository.ResolveReceiptResult(
		context.Background(),
		fixture.receiptRef,
		fixture.request.Result.ResultRef,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(byResult, published) || !reflect.DeepEqual(byPair, published) {
		t.Fatalf("resolved authorities differ:\nresult=%#v\npair=%#v\npublished=%#v", byResult, byPair, published)
	}
	terminal, err := fixture.repository.ResolveTerminalReview(
		context.Background(),
		fixture.receiptRef,
		fixture.request.Result.ResultRef,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(terminal, published) ||
		terminal.Receipt.LineageID() != fixture.state.LineageID ||
		terminal.Receipt.CandidateTree() != fixture.state.CurrentSnapshot.CandidateTree ||
		terminal.Receipt.PathsDigest() != fixture.state.CurrentSnapshot.PathsDigest ||
		terminal.Receipt.PolicyHash() != fixture.state.PolicyHash {
		t.Fatalf("terminal RAR seam = %#v", terminal)
	}

	object := fixture.repository.objectPath(published.AuthorityRef)
	result := fixture.repository.resultIndexPath(published.Result.ResultRef)
	pair := fixture.repository.pairIndexPath(published.Receipt.ReceiptRef, published.Result.ResultRef)
	for _, path := range []string{object, result, pair} {
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.Mode().IsRegular() {
			t.Fatalf("RAR immutable path %q = %#v, %v", path, info, statErr)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("RAR immutable path %q mode = %o", path, info.Mode().Perm())
		}
	}
	if runtime.GOOS != "windows" {
		for _, path := range []string{
			fixture.repository.root,
			filepath.Join(fixture.repository.root, "objects"),
			filepath.Join(fixture.repository.root, "by-result"),
			filepath.Join(fixture.repository.root, "by-receipt-result"),
		} {
			info, statErr := os.Lstat(path)
			if statErr != nil || info.Mode().Perm() != 0o700 {
				t.Fatalf("RAR private directory %q mode = %#v, %v", path, info, statErr)
			}
		}
	}

	subdirectory := filepath.Join(fixture.repo, "nested")
	if err := os.Mkdir(subdirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	fromSubdirectory, err := OpenRARAuthorityRepository(context.Background(), subdirectory)
	if err != nil {
		t.Fatal(err)
	}
	if fromSubdirectory.root != fixture.repository.root {
		t.Fatalf("RAR root differs by cwd: %q != %q", fromSubdirectory.root, fixture.repository.root)
	}
}

func TestRARAuthorityRejectsCandidatePolicyReceiptAndResultMismatches(t *testing.T) {
	t.Run("candidate", func(t *testing.T) {
		fixture := newRARCompactFixture(t, "rar-candidate")
		subject := fixture.request.Result.Subject
		subject.CandidateTree = tree("e")
		subject.SnapshotIdentity = verificationTestHash("other-candidate")
		fixture.request = rarPublicationForSubject(
			t,
			fixture.state.LineageID,
			fixture.receiptRef,
			subject,
			fixture.state.PolicyHash,
			fixture.request.Result.ResultRef,
		)
		if _, err := fixture.repository.Publish(context.Background(), fixture.request); err == nil {
			t.Fatal("Publish() accepted a tuple for a different candidate")
		}
	})

	t.Run("policy", func(t *testing.T) {
		fixture := newRARCompactFixture(t, "rar-policy")
		fixture.request = rarPublicationForSubject(
			t,
			fixture.state.LineageID,
			fixture.receiptRef,
			fixture.request.Result.Subject,
			verificationTestHash("different-policy"),
			fixture.request.Result.ResultRef,
		)
		if _, err := fixture.repository.Publish(context.Background(), fixture.request); err == nil {
			t.Fatal("Publish() accepted a tuple for a different policy")
		}
	})

	t.Run("stored preimage paths", func(t *testing.T) {
		fixture := newRARCompactFixture(t, "rar-paths")
		published, err := fixture.repository.Publish(context.Background(), fixture.request)
		if err != nil {
			t.Fatal(err)
		}
		subject := fixture.request.Result.Subject
		subject.PathsDigest = verificationTestHash("different-paths")
		subject.SnapshotIdentity = verificationTestHash("different-paths-snapshot")
		request := rarPublicationForSubject(
			t,
			fixture.state.LineageID,
			fixture.receiptRef,
			subject,
			fixture.state.PolicyHash,
			verificationTestHash("different-paths-result"),
		)
		forged := RARVerificationAuthority{
			Schema:        RARVerificationAuthoritySchema,
			Receipt:       published.Receipt,
			Applicability: request.Applicability,
			Registry:      request.Registry,
			Plan:          request.Plan,
			Result:        request.Result,
		}
		forged.AuthorityRef, err = rarVerificationAuthorityDigest(forged)
		if err != nil {
			t.Fatal(err)
		}
		if err := forged.Validate(); err == nil {
			t.Fatal("RARVerificationAuthority.Validate() accepted receipt/result path drift")
		}
	})

	t.Run("receipt", func(t *testing.T) {
		fixture := newRARCompactFixture(t, "rar-receipt")
		fixture.request.ReceiptRef = verificationTestHash("different-receipt")
		if _, err := fixture.repository.Publish(context.Background(), fixture.request); err == nil {
			t.Fatal("Publish() accepted a different native receipt ref")
		}
	})

	t.Run("invalid result before storage", func(t *testing.T) {
		fixture := newRARCompactFixture(t, "rar-invalid-result")
		fixture.request.Result.CompletedObligations = []string{}
		if _, err := fixture.repository.Publish(context.Background(), fixture.request); err == nil {
			t.Fatal("Publish() accepted an invalid verification result")
		}
		if _, err := os.Lstat(fixture.repository.root); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("invalid result created RAR storage: %v", err)
		}
	})
}

func TestRARAuthorityDetectsTamperAndStaleNativeReceipt(t *testing.T) {
	t.Run("content object tamper", func(t *testing.T) {
		fixture := newRARCompactFixture(t, "rar-object-tamper")
		published, err := fixture.repository.Publish(context.Background(), fixture.request)
		if err != nil {
			t.Fatal(err)
		}
		path := fixture.repository.objectPath(published.AuthorityRef)
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.repository.ResolveResult(context.Background(), fixture.request.Result.ResultRef); !errors.Is(err, ErrRARAuthorityCorrupt) {
			t.Fatalf("ResolveResult() tamper error = %v", err)
		}
	})

	t.Run("native receipt removed", func(t *testing.T) {
		fixture := newRARCompactFixture(t, "rar-native-stale")
		if _, err := fixture.repository.Publish(context.Background(), fixture.request); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(fixture.store.ReceiptPath()); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.repository.ResolveReceiptResult(
			context.Background(),
			fixture.receiptRef,
			fixture.request.Result.ResultRef,
		); !errors.Is(err, ErrRARAuthorityStale) {
			t.Fatalf("ResolveReceiptResult() stale error = %v", err)
		}
	})

	t.Run("wrong exact pair", func(t *testing.T) {
		fixture := newRARCompactFixture(t, "rar-wrong-pair")
		if _, err := fixture.repository.Publish(context.Background(), fixture.request); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.repository.ResolveReceiptResult(
			context.Background(),
			verificationTestHash("wrong-pair"),
			fixture.request.Result.ResultRef,
		); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("ResolveReceiptResult() wrong pair error = %v", err)
		}
	})
}

func TestRARResultAuthorityRejectsReplacedGitIdentityWithoutPublishing(t *testing.T) {
	fixture := newRARCompactFixture(t, "rar-result-git-identity-swap")
	originalGit := filepath.Join(fixture.repo, ".git")
	retiredGit := filepath.Join(fixture.repo, ".git-retired")
	if err := os.Rename(originalGit, retiredGit); err != nil {
		t.Skipf("cannot replace Git control directory on this platform: %v", err)
	}
	if err := runSnapshotGit(fixture.repo, "init"); err != nil {
		t.Fatalf("reinitialize Git repository: %v", err)
	}

	if _, err := fixture.repository.Publish(
		context.Background(),
		fixture.request,
	); !errors.Is(err, ErrRepositoryIdentityChanged) {
		t.Fatalf("Publish() after Git identity replacement error = %v", err)
	}
	replacementRoot := filepath.Join(
		originalGit,
		"gentle-ai",
		"review-transactions",
		rarAuthorityDirectory,
		rarAuthorityVersion,
	)
	if _, err := os.Lstat(replacementRoot); !os.IsNotExist(err) {
		t.Fatalf("stale RAR result handle published under replacement Git identity: %v", err)
	}
}

func TestRARAuthorityExactReplayConflictAndConcurrentPublication(t *testing.T) {
	t.Run("exact replay and same-result conflict", func(t *testing.T) {
		fixture := newRARCompactFixture(t, "rar-replay")
		first, err := fixture.repository.Publish(context.Background(), fixture.request)
		if err != nil {
			t.Fatal(err)
		}
		replay, err := fixture.repository.Publish(context.Background(), fixture.request)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(first, replay) {
			t.Fatalf("exact replay changed authority:\nfirst=%#v\nreplay=%#v", first, replay)
		}

		conflict := fixture.request
		conflict.Result.Aggregate = VerificationAggregateFailed
		if err := ValidateVerificationResultRef(
			conflict.Applicability,
			conflict.Registry,
			conflict.Plan,
			conflict.Result,
		); err != nil {
			t.Fatalf("conflict fixture is not independently valid: %v", err)
		}
		if _, err := fixture.repository.Publish(context.Background(), conflict); !errors.Is(err, ErrRARAuthorityConflict) {
			t.Fatalf("Publish() conflict error = %v", err)
		}
		resolved, err := fixture.repository.ResolveResult(context.Background(), first.Result.ResultRef)
		if err != nil || !reflect.DeepEqual(resolved, first) {
			t.Fatalf("conflict changed first authority: %#v, %v", resolved, err)
		}
	})

	t.Run("concurrent exact publication converges", func(t *testing.T) {
		fixture := newRARCompactFixture(t, "rar-concurrent")
		const workers = 8
		results := make(chan RARVerificationAuthority, workers)
		errs := make(chan error, workers)
		var group sync.WaitGroup
		for index := 0; index < workers; index++ {
			group.Add(1)
			go func() {
				defer group.Done()
				authority, err := fixture.repository.Publish(context.Background(), fixture.request)
				results <- authority
				errs <- err
			}()
		}
		group.Wait()
		close(results)
		close(errs)
		for err := range errs {
			if err != nil {
				t.Errorf("concurrent Publish() error = %v", err)
			}
		}
		var first *RARVerificationAuthority
		for authority := range results {
			if first == nil {
				copy := authority
				first = &copy
				continue
			}
			if !reflect.DeepEqual(*first, authority) {
				t.Errorf("concurrent publication diverged:\nfirst=%#v\nother=%#v", *first, authority)
			}
		}
	})
}

func TestRARAuthorityReadsExactHistoricalV1Receipt(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\nhistorical RAR\n")
	transaction, receipt, _ := nativeGateFixture(t, repo, "rar-historical")
	store, err := AuthoritativeStore(context.Background(), repo, transaction.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	revision := appendApprovedStoreChain(t, store, transaction)
	if err := WriteReceiptAtomic(filepath.Join(store.Dir, "artifacts", "receipt.json"), receipt); err != nil {
		t.Fatal(err)
	}
	payload, err := canonicalRARReceiptPayload(receipt)
	if err != nil {
		t.Fatal(err)
	}
	receiptDigest := sha256.Sum256(payload)
	receiptRef := fmt.Sprintf("sha256:%x", receiptDigest)
	subject, err := VerificationSubjectFromSnapshot(transaction.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	request := rarPublicationForSubject(
		t,
		transaction.LineageID,
		receiptRef,
		subject,
		transaction.PolicyHash,
		verificationTestHash("historical-result"),
	)
	repository, err := OpenRARAuthorityRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	published, err := repository.Publish(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if published.Receipt.Version != RARReceiptHistoricalV1 ||
		published.Receipt.Historical == nil ||
		published.Receipt.Compact != nil ||
		published.Receipt.AuthorityRevision != revision ||
		!reflect.DeepEqual(*published.Receipt.Historical, receipt) {
		t.Fatalf("historical native receipt = %#v", published.Receipt)
	}
	resolved, err := repository.ResolveReceiptResult(
		context.Background(),
		receiptRef,
		request.Result.ResultRef,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resolved, published) {
		t.Fatalf("historical resolution differs:\nresolved=%#v\npublished=%#v", resolved, published)
	}
}

func TestRARAuthorityNeverDowngradesPresentCompactAuthorityToHistoricalV1(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\nhistorical collision\n")
	transaction, receipt, _ := nativeGateFixture(t, repo, "rar-no-downgrade")
	historical, err := AuthoritativeStore(context.Background(), repo, transaction.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	appendApprovedStoreChain(t, historical, transaction)
	if err := WriteReceiptAtomic(filepath.Join(historical.Dir, "artifacts", "receipt.json"), receipt); err != nil {
		t.Fatal(err)
	}
	receiptPayload, err := canonicalRARReceiptPayload(receipt)
	if err != nil {
		t.Fatal(err)
	}
	subject, err := VerificationSubjectFromSnapshot(transaction.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	receiptDigest := sha256.Sum256(receiptPayload)
	request := rarPublicationForSubject(
		t,
		transaction.LineageID,
		fmt.Sprintf("sha256:%x", receiptDigest),
		subject,
		transaction.PolicyHash,
		verificationTestHash("no-downgrade-result"),
	)

	compact, err := CompactAuthoritativeStore(context.Background(), repo, transaction.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(compact.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(compact.StatePath(), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repository, err := OpenRARAuthorityRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Publish(context.Background(), request); err == nil {
		t.Fatal("Publish() downgraded a present invalid compact lineage to historical v1")
	}
	if _, err := os.Lstat(repository.root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("downgrade denial created RAR authority: %v", err)
	}
}

func TestRARAuthorityFailsClosedForUnsafeRootAndDurabilityFailure(t *testing.T) {
	t.Run("symlinked private root", func(t *testing.T) {
		fixture := newRARCompactFixture(t, "rar-symlink")
		privateRoot := filepath.Dir(fixture.repository.root)
		outside := t.TempDir()
		if err := os.Symlink(outside, privateRoot); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if _, err := fixture.repository.Publish(context.Background(), fixture.request); err == nil {
			t.Fatal("Publish() accepted a symlinked private root")
		}
		entries, err := os.ReadDir(outside)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Fatalf("symlink target mutated: %v", entries)
		}
	})

	t.Run("broad private root is rejected without chmod", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Unix mode assertion")
		}
		fixture := newRARCompactFixture(t, "rar-broad-root")
		privateRoot := filepath.Dir(fixture.repository.root)
		if err := os.Mkdir(privateRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.repository.Publish(context.Background(), fixture.request); err == nil {
			t.Fatal("Publish() accepted a broad private root")
		}
		info, err := os.Lstat(privateRoot)
		if err != nil || info.Mode().Perm() != 0o755 {
			t.Fatalf("broad root was mutated: %#v, %v", info, err)
		}
	})

	t.Run("directory sync failure is retry safe", func(t *testing.T) {
		fixture := newRARCompactFixture(t, "rar-sync-failure")
		original := syncReviewDirectory
		failed := false
		syncReviewDirectory = func(path string) error {
			if !failed {
				failed = true
				return errors.New("injected directory sync failure")
			}
			return original(path)
		}
		_, publishErr := fixture.repository.Publish(context.Background(), fixture.request)
		syncReviewDirectory = original
		t.Cleanup(func() { syncReviewDirectory = original })
		if publishErr == nil {
			t.Fatal("Publish() ignored directory sync failure")
		}
		if _, err := fixture.repository.Publish(context.Background(), fixture.request); err != nil {
			t.Fatalf("Publish() retry after sync failure = %v", err)
		}
	})
}

func newRARCompactFixture(t *testing.T, lineage string) rarCompactFixture {
	t.Helper()
	repo := initSnapshotRepo(t)
	state, store, receipt := approvedCompactRevisionFixture(t, repo, lineage)
	payload, err := canonicalRARReceiptPayload(receipt)
	if err != nil {
		t.Fatal(err)
	}
	receiptDigest := sha256.Sum256(payload)
	receiptRef := fmt.Sprintf("sha256:%x", receiptDigest)
	subject, err := VerificationSubjectFromSnapshot(state.CurrentSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := OpenRARAuthorityRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	return rarCompactFixture{
		repo:       repo,
		state:      state,
		store:      store,
		receipt:    receipt,
		receiptRef: receiptRef,
		repository: repository,
		request: rarPublicationForSubject(
			t,
			lineage,
			receiptRef,
			subject,
			state.PolicyHash,
			verificationTestHash(lineage+"-result"),
		),
	}
}

func rarPublicationForSubject(
	t *testing.T,
	lineage string,
	receiptRef string,
	subject VerificationSubject,
	policyHash string,
	resultRef string,
) RARAuthorityPublication {
	t.Helper()
	registry, err := NewVerificationPlanRegistry(
		policyHash,
		[]string{},
		[]VerificationObligation{{
			ID:             "unit",
			RequirementRef: verificationTestHash(lineage + "-requirement"),
			ArgvRef:        verificationTestHash(lineage + "-argv"),
			CapabilityRef:  "host.exec",
			Cost:           VerificationCostQuick,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	applicability := VerificationApplicability{
		Schema:            VerificationApplicabilitySchema,
		ClassifierVersion: VerificationClassifierVersion,
		Subject:           subject,
		PolicyHash:        policyHash,
		RegistryDigest:    registry.Digest,
		Decision:          VerificationApplicable,
		ContentActivity:   VerificationContentActive,
		Reasons: []VerificationApplicabilityReason{{
			Code: VerificationReasonActiveSource,
			Path: "internal/app.go",
		}},
		EvidenceRefs: []string{},
	}
	applicability.Digest, err = verificationApplicabilityDigest(applicability)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildVerificationPlan(applicability, registry)
	if err != nil {
		t.Fatal(err)
	}
	result := VerificationResultRef{
		Schema:               VerificationResultRefSchema,
		ResultRef:            resultRef,
		Subject:              subject,
		PolicyHash:           policyHash,
		PlanDigest:           plan.Digest,
		ApplicabilityDigest:  applicability.Digest,
		Aggregate:            VerificationAggregateComplete,
		CompletedObligations: []string{"unit"},
		EvidenceRefs:         []string{},
	}
	if err := ValidateVerificationResultRef(applicability, registry, plan, result); err != nil {
		t.Fatal(err)
	}
	return RARAuthorityPublication{
		LineageID:     lineage,
		ReceiptRef:    receiptRef,
		Applicability: applicability,
		Registry:      registry,
		Plan:          plan,
		Result:        result,
	}
}
