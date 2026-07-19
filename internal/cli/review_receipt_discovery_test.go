package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

func assertScopeChangeRecovery(t *testing.T, failure ReviewIntegrationFailure, lineage, privatePath string) {
	if failure.Context == nil || failure.Context.ScopeChange == nil {
		t.Fatalf("scope-change context = %#v", failure)
	}
	scope := failure.Context.ScopeChange
	if scope.PredecessorLineageID != lineage || scope.PredecessorRevision == "" || scope.Expected.CandidateTree == "" || scope.Expected.PathsDigest == "" || scope.Actual.CandidateTree == "" || scope.Actual.PathsDigest == "" || scope.DifferingPathCount != 1 || !reflect.DeepEqual(failure.RequiredInputs, []string{"predecessor_lineage_id", "expected_predecessor_revision", "successor_lineage_id", "disposition", "reason", "actor"}) || !reflect.DeepEqual(scope.RecoveryRequiredInputs, failure.RequiredInputs) {
		t.Fatalf("scope-change provenance = %#v", failure)
	}
	payload, err := json.Marshal(failure)
	if err != nil || strings.Contains(string(payload), privatePath) || strings.Contains(string(payload), `"paths"`) || strings.Contains(string(payload), `"differing_paths"`) {
		t.Fatalf("scope-change failure exposed private paths: %s, %v", payload, err)
	}
}

func TestUnqualifiedGateDiscoverySelectsOneExactReceiptAcrossUnrelatedHistory(t *testing.T) {
	repo := initReviewCLIRepo(t)
	first, _ := approveDiscoveryMarkdown(t, repo, "review-discovery-first", "docs/first.md", "first\n")
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", "first reviewed target")
	second, store := approveDiscoveryMarkdown(t, repo, "review-discovery-second", "docs/second.md", "second\n")
	receipt, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	receipt = bytes.Replace(receipt, []byte(`"selected_lenses": []`), []byte(`"selected_lenses": null`), 1)
	if err := os.WriteFile(store.ReceiptPath(), receipt, 0o644); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &output); err != nil {
		t.Fatalf("one exact receipt among unrelated history: %v\n%s", err, output.String())
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, output.Bytes()).Result, &result)
	if !result.Allowed || result.Context.LineageID != second.LineageID || result.Context.LineageID == first.LineageID {
		t.Fatalf("exact receipt selection = %#v", result)
	}
}

func TestReceiptlessTerminalLegacyChainIsInventoryReadableButNeverGateAuthority(t *testing.T) {
	fixture := newLegacyCLIFixture(t, "legacy-pre-receipt")
	if err := os.Remove(fixture.receiptPath); err != nil {
		t.Fatal(err)
	}

	report, err := reviewtransaction.InventoryAuthority(context.Background(), fixture.repo)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || !report.Authoritative {
		t.Fatalf("receiptless terminal legacy chain forced incomplete inventory = %#v", report)
	}
	if len(report.Entries) != 1 || report.Entries[0].Status != reviewtransaction.AuthorityStatusHistorical {
		t.Fatalf("receiptless terminal legacy entry = %#v", report.Entries)
	}

	gateInput := reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePostApply}
	_, _, discoveryErr := discoverCompactFacadeGateReview(context.Background(), fixture.repo, "", gateInput)
	var discovery *ReviewReceiptDiscoveryError
	if !errors.As(discoveryErr, &discovery) || discovery.Kind != ReviewReceiptMissing {
		t.Fatalf("receiptless legacy gate discovery = %#v, %v", discovery, discoveryErr)
	}
	if exact := legacyExactFacadeGateLineages(context.Background(), fixture.repo, gateInput); exact != 0 {
		t.Fatalf("receiptless legacy chain counted as exact gate lineage %d times", exact)
	}
	if _, _, _, legacyErr := discoverFacadeReview(context.Background(), fixture.repo, fixture.lineage, true); legacyErr == nil {
		t.Fatal("receiptless legacy chain was discovered as terminal facade authority")
	}

	var output bytes.Buffer
	err = RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", fixture.repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &output)
	if err == nil {
		t.Fatal("receiptless legacy chain acted as gate authority")
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Code != "receipt_missing" || failure.MutationOutcome != ReviewMutationNotStarted || failure.RetrySafe {
		t.Fatalf("receiptless legacy gate failure = %#v", failure)
	}
}

func TestUnqualifiedGateDiscoveryReturnsTypedMissingAndScopeChanged(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		var output bytes.Buffer
		err := RunReview([]string{
			"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
			"--gate", string(reviewtransaction.GatePostApply),
		}, &output)
		if err == nil {
			t.Fatal("missing receipt validation succeeded")
		}
		failure := decodeReviewIntegrationFailure(t, output.Bytes())
		if failure.Code != "receipt_missing" || failure.MutationOutcome != ReviewMutationNotStarted || failure.RetrySafe || failure.NextAction != "stop" {
			t.Fatalf("missing receipt failure = %#v", failure)
		}
	})

	t.Run("scope changed", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		_, _ = approveDiscoveryMarkdown(t, repo, "review-discovery-scope", "docs/reviewed.md", "reviewed\n")
		if err := os.WriteFile(filepath.Join(repo, "docs", "reviewed.md"), []byte("drifted\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		err := RunReview([]string{
			"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
			"--gate", string(reviewtransaction.GatePostApply),
		}, &output)
		if err == nil {
			t.Fatal("scope-changed receipt validation succeeded")
		}
		failure := decodeReviewIntegrationFailure(t, output.Bytes())
		if failure.Code != "receipt_scope_changed" || failure.AuthorityApplicability != "current_target" || failure.RetrySafe ||
			failure.Replayability != reviewtransaction.ReplayabilityManualActionRequired || failure.NextAction != "explicit-maintainer-action" {
			t.Fatalf("scope-changed receipt failure = %#v", failure)
		}
		assertScopeChangeRecovery(t, failure, "review-discovery-scope", "docs/reviewed.md")
	})

	t.Run("unrelated", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		_, store := approveDiscoveryMarkdown(t, repo, "review-discovery-unrelated", "docs/reviewed.md", "reviewed\n")
		runReviewCLIGit(t, repo, "add", "-A")
		runReviewCLIGit(t, repo, "commit", "-qm", "reviewed target")
		if err := os.WriteFile(filepath.Join(repo, "unrelated.txt"), []byte("unrelated\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		runReviewCLIGit(t, repo, "add", "-A")
		runReviewCLIGit(t, repo, "commit", "-qm", "unrelated target")
		record, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		assessment, err := reviewtransaction.AssessCompactGateTarget(context.Background(), repo, record.State, reviewtransaction.NativeGateRequestInput{
			Gate:      reviewtransaction.GateRelease,
			LineageID: record.State.LineageID,
		})
		if err != nil {
			t.Fatalf("assess unrelated release target: %v", err)
		}
		if assessment.Applicability != reviewtransaction.CompactGateTargetUnrelated {
			t.Fatalf("unrelated release applicability = %q; expected=%#v actual=%#v", assessment.Applicability, assessment.Expected, assessment.Actual)
		}
		var output bytes.Buffer
		err = RunReview([]string{
			"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
			"--gate", string(reviewtransaction.GateRelease),
		}, &output)
		if err == nil {
			t.Fatal("unrelated receipt validation succeeded")
		}
		failure := decodeReviewIntegrationFailure(t, output.Bytes())
		if failure.Code != "receipt_unrelated" || failure.AuthorityApplicability != "unrelated" || failure.RetrySafe || failure.NextAction != "stop" {
			t.Fatalf("unrelated receipt failure = %#v", failure)
		}
	})
}

func TestUnqualifiedGateDiscoveryRoutesCommittedNextSliceWorkspace(t *testing.T) {
	// #1401: after one approved slice is committed exactly as reviewed, new
	// dirty tracked work on top must classify as unrelated or scope-changed,
	// never as authority_corrupted against the healthy predecessor.
	t.Run("unrelated dirty tracked next slice", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		_, _ = approveDiscoveryMarkdown(t, repo, "review-discovery-next-slice", "docs/reviewed.md", "reviewed\n")
		runReviewCLIGit(t, repo, "add", "-A")
		runReviewCLIGit(t, repo, "commit", "-qm", "reviewed target")
		if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("next slice\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		err := RunReview([]string{
			"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
			"--gate", string(reviewtransaction.GatePostApply),
		}, &output)
		if err == nil {
			t.Fatal("next-slice workspace validation succeeded")
		}
		failure := decodeReviewIntegrationFailure(t, output.Bytes())
		if failure.Code == "authority_corrupted" {
			t.Fatalf("healthy committed receipt misrouted as corrupted: %#v", failure)
		}
		if failure.Code != "receipt_unrelated" || failure.AuthorityApplicability != "unrelated" || failure.RetrySafe || failure.NextAction != "stop" {
			t.Fatalf("committed next-slice workspace failure = %#v", failure)
		}
	})

	t.Run("overlapping dirty tracked next slice", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		_, _ = approveDiscoveryMarkdown(t, repo, "review-discovery-next-slice-overlap", "docs/reviewed.md", "reviewed\n")
		runReviewCLIGit(t, repo, "add", "-A")
		runReviewCLIGit(t, repo, "commit", "-qm", "reviewed target")
		if err := os.WriteFile(filepath.Join(repo, "docs", "reviewed.md"), []byte("drifted after commit\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		err := RunReview([]string{
			"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
			"--gate", string(reviewtransaction.GatePostApply),
		}, &output)
		if err == nil {
			t.Fatal("overlapping next-slice validation succeeded")
		}
		failure := decodeReviewIntegrationFailure(t, output.Bytes())
		if failure.Code == "authority_corrupted" {
			t.Fatalf("healthy committed receipt misrouted as corrupted: %#v", failure)
		}
		if failure.Code != "receipt_scope_changed" || failure.AuthorityApplicability != "current_target" || failure.RetrySafe ||
			failure.Replayability != reviewtransaction.ReplayabilityManualActionRequired || failure.NextAction != "explicit-maintainer-action" {
			t.Fatalf("overlapping next-slice failure = %#v", failure)
		}
		assertScopeChangeRecovery(t, failure, "review-discovery-next-slice-overlap", "docs/reviewed.md")
	})
}

func TestUnqualifiedGateDiscoveryRejectsMultipleExactReceiptsButExplicitLineageIsDirect(t *testing.T) {
	repo := initReviewCLIRepo(t)
	started, store := approveDiscoveryMarkdown(t, repo, "review-discovery-exact-a", "docs/exact.md", "exact\n")
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	lines := record.State.OriginalChangedLines
	clone, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: "review-discovery-exact-b", Mode: reviewtransaction.ModeOrdinaryBounded, Generation: record.State.Generation,
		Snapshot: record.State.InitialSnapshot, PolicyHash: record.State.PolicyHash, RiskLevel: record.State.RiskLevel,
		SelectedLenses: []string{}, OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	cloneStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, clone.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := cloneStore.Replace("", "review/start", clone)
	if err != nil {
		t.Fatal(err)
	}
	if err := clone.CompleteReview(reviewtransaction.CompactReviewInput{LensResults: []reviewtransaction.LensResult{}}); err != nil {
		t.Fatal(err)
	}
	revision, err = cloneStore.Replace(revision, "review/complete-review", clone)
	if err != nil {
		t.Fatal(err)
	}
	if err := clone.CompleteVerification([]byte("independent duplicate fixture evidence"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := cloneStore.Replace(revision, "review/complete-verification", clone); err != nil {
		t.Fatal(err)
	}
	cloneReceipt, err := clone.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := reviewtransaction.WriteCompactReceiptAtomic(cloneStore.ReceiptPath(), cloneReceipt); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	err = RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &output)
	if err == nil {
		t.Fatal("multiple exact receipts were selected implicitly")
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Code != "receipt_ambiguous" || failure.AuthorityApplicability != "ambiguous" || failure.RetrySafe ||
		len(failure.RequiredInputs) != 1 || failure.RequiredInputs[0] != "lineage_id" {
		t.Fatalf("ambiguous receipt failure = %#v", failure)
	}

	output.Reset()
	if err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", started.LineageID,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &output); err != nil {
		t.Fatalf("explicit exact lineage failed: %v\n%s", err, output.String())
	}
}

func TestUnqualifiedGateDiscoveryRequiresSelectionForMultipleScopeChangedReceipts(t *testing.T) {
	for _, tt := range []struct {
		name       string
		projection reviewtransaction.Projection
		gate       reviewtransaction.GateKind
	}{
		{name: "workspace", projection: reviewtransaction.ProjectionWorkspace, gate: reviewtransaction.GatePostApply},
		{name: "staged", projection: reviewtransaction.ProjectionStaged, gate: reviewtransaction.GatePreCommit},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			lineages := []string{"review-discovery-scope-a-" + tt.name, "review-discovery-scope-b-" + tt.name}
			logicalPath := "docs/scope-" + tt.name + ".md"
			_, first := approveDiscoveryMarkdownProjection(t, repo, lineages[0], logicalPath, "reviewed\n", tt.projection)
			cloneApprovedDiscoveryAuthority(t, repo, first, lineages[1])
			if err := os.WriteFile(filepath.Join(repo, filepath.FromSlash(logicalPath)), []byte("drifted\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if tt.projection == reviewtransaction.ProjectionStaged {
				runReviewCLIGit(t, repo, "add", "-A")
			}

			gateInput := reviewtransaction.NativeGateRequestInput{Gate: tt.gate}
			_, _, discoveryErr := discoverCompactFacadeGateReview(context.Background(), repo, "", gateInput)
			var discovery *ReviewReceiptDiscoveryError
			if !errors.As(discoveryErr, &discovery) || discovery.Kind != ReviewReceiptAmbiguous || !reflect.DeepEqual(discovery.Candidates, lineages) || discovery.Context != nil {
				t.Fatalf("multiple scope-changed discovery = %#v, %v", discovery, discoveryErr)
			}

			var output bytes.Buffer
			err := RunReview([]string{
				"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--gate", string(tt.gate),
			}, &output)
			if err == nil {
				t.Fatal("multiple scope-changed receipts were selected implicitly")
			}
			failure := decodeReviewIntegrationFailure(t, output.Bytes())
			if failure.Code != "receipt_ambiguous" || failure.AuthorityApplicability != "ambiguous" || failure.Context != nil || failure.RetrySafe ||
				failure.Replayability != reviewtransaction.ReplayabilityManualActionRequired || failure.NextAction != "review.status" ||
				!reflect.DeepEqual(failure.RequiredInputs, []string{"lineage_id"}) {
				t.Fatalf("multiple scope-changed failure = %#v", failure)
			}

			output.Reset()
			if err := RunReview([]string{
				"status", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--projection", string(tt.projection),
			}, &output); err != nil {
				t.Fatalf("target-scoped status: %v\n%s", err, output.String())
			}
			var status ReviewTargetStatusResult
			decodeStrictReviewJSON(t, output.Bytes(), &status)
			if status.Applicability != reviewtransaction.TargetApplicabilityAmbiguous || status.Action != reviewtransaction.TargetStatusActionSelectLineage ||
				status.Replayability != reviewtransaction.ReplayabilityStatusRequired || !reflect.DeepEqual(status.Candidates, lineages) {
				t.Fatalf("multiple scope-changed status = %#v", status)
			}

			for _, lineage := range lineages {
				output.Reset()
				err := RunReview([]string{
					"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", lineage, "--gate", string(tt.gate),
				}, &output)
				if err == nil {
					t.Fatalf("explicit scope-changed lineage %s unexpectedly passed", lineage)
				}
				explicit := decodeReviewIntegrationFailure(t, output.Bytes())
				if explicit.Code != "gate_scope_changed" || explicit.NextAction != "explicit-maintainer-action" {
					t.Fatalf("explicit scope-changed lineage %s failure = %#v", lineage, explicit)
				}
				assertScopeChangeRecovery(t, explicit, lineage, logicalPath)
			}
		})
	}
}

func TestUnscopedGateDiscoveryToleratesCorruptedUnrelatedLegacyInventory(t *testing.T) {
	repo := initReviewCLIRepo(t)
	started, _ := approveDiscoveryMarkdown(t, repo, "review-discovery-valid", "docs/valid.md", "valid\n")
	commonDir := filepath.Clean(string(bytes.TrimSpace([]byte(runReviewCLIGit(t, repo, "rev-parse", "--path-format=absolute", "--git-common-dir")))))
	broken := filepath.Join(commonDir, "gentle-ai", "review-transactions", "v1", "unrelated-broken")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "HEAD"), []byte("not-a-revision\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var explicit bytes.Buffer
	statePath := filepath.Join(commonDir, "gentle-ai", "review-transactions", "v2", started.LineageID, "review-state.json")
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		var current bytes.Buffer
		if err := RunReview([]string{
			"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", started.LineageID,
			"--gate", string(reviewtransaction.GatePostApply),
		}, &current); err != nil {
			t.Fatalf("explicit lineage was poisoned by unrelated inventory: %v\n%s", err, current.String())
		}
		if attempt == 0 {
			explicit = current
		} else if current.String() != explicit.String() {
			t.Fatalf("repeated explicit validation changed bytes:\n%s\n%s", explicit.String(), current.String())
		}
	}
	after, err := os.ReadFile(statePath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("explicit validation mutated authority: %v", err)
	}

	var unscoped bytes.Buffer
	if err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &unscoped); err != nil {
		t.Fatalf("unscoped discovery was poisoned by corrupted unrelated legacy inventory: %v\n%s", err, unscoped.String())
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, unscoped.Bytes()).Result, &result)
	if !result.Allowed || result.Context.LineageID != started.LineageID {
		t.Fatalf("unscoped discovery across corrupted legacy inventory = %#v", result)
	}
	if strings.Contains(unscoped.String(), broken) || strings.Contains(unscoped.String(), "not-a-revision") {
		t.Fatalf("unscoped discovery exposed private payload: %s", unscoped.String())
	}
	brokenHead, err := os.ReadFile(filepath.Join(broken, "HEAD"))
	if err != nil || string(brokenHead) != "not-a-revision\n" {
		t.Fatalf("unscoped discovery mutated corrupted legacy inventory: %v", err)
	}
}

func TestUnscopedGateDiscoveryExcludesTamperedLegacyReceiptFromCandidates(t *testing.T) {
	repo := initReviewCLIRepo(t)
	started, _ := approveDiscoveryMarkdown(t, repo, "review-discovery-valid", "docs/valid.md", "valid\n")
	legacyStore, authoritative := approveLegacyDiscoveryChain(t, repo, "review-legacy-tampered")
	tampered := authoritative
	tampered.EvidenceHash = "sha256:" + strings.Repeat("ab", 32)
	if tampered.EvidenceHash == authoritative.EvidenceHash {
		tampered.EvidenceHash = "sha256:" + strings.Repeat("cd", 32)
	}
	receiptPath := filepath.Join(legacyStore.Dir, "artifacts", "receipt.json")
	if err := reviewtransaction.WriteReceiptAtomic(receiptPath, tampered); err != nil {
		t.Fatal(err)
	}

	report, err := reviewtransaction.InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	mismatch := false
	for _, entry := range report.Entries {
		if entry.LineageID == "review-legacy-tampered" && entry.Version == reviewtransaction.AuthorityVersionLegacy &&
			entry.Status == reviewtransaction.AuthorityStatusInvalid &&
			reflect.DeepEqual(entry.Problems, []string{"legacy receipt does not match terminal authority"}) {
			mismatch = true
		}
	}
	if !mismatch {
		t.Fatalf("fixture did not produce a receipt-mismatch legacy entry: %#v", report.Entries)
	}

	gateInput := reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePostApply}
	if exact := legacyExactFacadeGateLineages(context.Background(), repo, gateInput); exact != 0 {
		t.Fatalf("tampered legacy receipt counted as exact gate candidate: %d", exact)
	}

	var unscoped bytes.Buffer
	if err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &unscoped); err != nil {
		t.Fatalf("unscoped discovery was poisoned by tampered legacy receipt: %v\n%s", err, unscoped.String())
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, unscoped.Bytes()).Result, &result)
	if !result.Allowed || result.Context.LineageID != started.LineageID {
		t.Fatalf("unscoped discovery across tampered legacy receipt = %#v", result)
	}
}

func approveLegacyDiscoveryChain(t *testing.T, repo, lineage string) (reviewtransaction.Store, reviewtransaction.Receipt) {
	t.Helper()
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.md")
	ledgerPath := filepath.Join(dir, "ledger.json")
	evidencePath := filepath.Join(dir, "evidence.txt")
	if err := os.WriteFile(policyPath, []byte("legacy bounded policy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ledger, err := reviewtransaction.CanonicalLedger([]reviewtransaction.Finding{})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ledgerPath, ledger, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(evidencePath, []byte("legacy verification passed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	policyHash, _ := reviewtransaction.HashArtifact(policyPath)
	ledgerHash, _ := reviewtransaction.HashLedgerArtifact(ledgerPath)
	evidenceHash, _ := reviewtransaction.HashArtifact(evidencePath)
	tx, err := reviewtransaction.NewTransaction(reviewtransaction.Start{
		LineageID: lineage, Mode: reviewtransaction.ModeOrdinary4R, Generation: 1,
		Snapshot: snapshot, PolicyHash: policyHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.AuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	head := appendLegacyCLIRecord(t, store, "", "review/start", *tx)
	if err := tx.FreezeFindings([]reviewtransaction.Finding{}, ledger, ledgerHash); err != nil {
		t.Fatal(err)
	}
	head = appendLegacyCLIRecord(t, store, head, "review/freeze-findings", *tx)
	if _, err := tx.ClassifyEvidence([]reviewtransaction.FindingEvidence{}); err != nil {
		t.Fatal(err)
	}
	head = appendLegacyCLIRecord(t, store, head, "review/classify-evidence", *tx)
	if err := tx.BeginFinalVerification(); err != nil {
		t.Fatal(err)
	}
	head = appendLegacyCLIRecord(t, store, head, "review/begin-final-verification", *tx)
	if err := tx.CompleteFinalVerification(evidenceHash, true); err != nil {
		t.Fatal(err)
	}
	appendLegacyCLIRecord(t, store, head, "review/complete-final-verification", *tx)
	receipt, err := tx.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := reviewtransaction.WriteReceiptAtomic(filepath.Join(store.Dir, "artifacts", "receipt.json"), receipt); err != nil {
		t.Fatal(err)
	}
	return store, receipt
}

func TestUnscopedGateDiscoveryFailsClosedOnCorruptedCompactLeaf(t *testing.T) {
	repo := initReviewCLIRepo(t)
	approveDiscoveryMarkdown(t, repo, "review-discovery-valid", "docs/valid.md", "valid\n")
	commonDir := filepath.Clean(string(bytes.TrimSpace([]byte(runReviewCLIGit(t, repo, "rev-parse", "--path-format=absolute", "--git-common-dir")))))
	broken := filepath.Join(commonDir, "gentle-ai", "review-transactions", "v2", "unrelated-broken")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "review-state.json"), []byte("{\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var unscoped bytes.Buffer
	err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &unscoped)
	if err == nil {
		t.Fatal("unscoped discovery ignored corrupted compact leaf")
	}
	failure := decodeReviewIntegrationFailure(t, unscoped.Bytes())
	if failure.Code != "authority_corrupted" || failure.AuthorityApplicability != "corrupted" || failure.CauseCategory != "record_or_graph_invalid" || failure.RetrySafe || failure.NextAction != "stop" {
		t.Fatalf("corrupted compact leaf failure = %#v", failure)
	}
	if strings.Contains(unscoped.String(), broken) {
		t.Fatalf("corrupted compact leaf failure exposed private payload: %s", unscoped.String())
	}
}

func TestExplicitMalformedLineageFailsClosedWithoutMutation(t *testing.T) {
	repo := initReviewCLIRepo(t)
	started, store := approveDiscoveryMarkdown(t, repo, "review-selected-malformed", "docs/selected.md", "selected\n")
	malformed := []byte("{\n")
	if err := os.WriteFile(store.StatePath(), malformed, 0o644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err := RunReview([]string{"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", started.LineageID, "--gate", string(reviewtransaction.GatePostApply)}, &output)
	if err == nil {
		t.Fatal("selected malformed lineage validated")
	}
	after, readErr := os.ReadFile(store.StatePath())
	if readErr != nil || !bytes.Equal(after, malformed) || strings.Contains(output.String(), store.StatePath()) || strings.Contains(output.String(), "unexpected end") {
		t.Fatalf("selected malformed lineage was exposed or mutated: %v\n%s", readErr, output.String())
	}
}

func TestUnqualifiedPrePRDiscoveryComposesExactSequentialCompactReceipts(t *testing.T) {
	repo := initReviewCLIRepo(t)
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	remote := filepath.Join(t.TempDir(), "remote.git")
	runReviewCLIGit(t, repo, "clone", "--bare", repo, remote)
	runReviewCLIGit(t, repo, "remote", "add", "origin", remote)

	lineages := []string{"review-chain-first", "review-chain-second", "review-chain-third"}
	for index, lineage := range lineages {
		approveDiscoveryMarkdown(t, repo, lineage, "docs/segment-"+string(rune('a'+index))+".md", "reviewed\n")
		runReviewCLIGit(t, repo, "add", "-A")
		runReviewCLIGit(t, repo, "commit", "-qm", "deliver "+lineage)
	}

	var output bytes.Buffer
	if err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePrePR), "--base-ref", "origin/" + branch,
	}, &output); err != nil {
		t.Fatalf("composed pre-PR facade validation: %v\n%s", err, output.String())
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, output.Bytes()).Result, &result)
	if !result.Allowed || result.Context.LineageID != lineages[2] || result.Context.ChainIdentity == "" || result.Context.PrePRBoundary == nil {
		t.Fatalf("composed pre-PR result = %#v", result)
	}

	output.Reset()
	err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", lineages[2],
		"--gate", string(reviewtransaction.GatePrePR), "--base-ref", "origin/" + branch,
	}, &output)
	if err == nil {
		t.Fatal("explicit terminal lineage unexpectedly entered composition")
	}
}

func TestUnqualifiedPrePRDiscoveryComposesSequentialReceiptsForSamePath(t *testing.T) {
	repo := initReviewCLIRepo(t)
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	remote := filepath.Join(t.TempDir(), "remote.git")
	runReviewCLIGit(t, repo, "clone", "--bare", repo, remote)
	runReviewCLIGit(t, repo, "remote", "add", "origin", remote)

	for index, lineage := range []string{"review-chain-overlap-first", "review-chain-overlap-second", "review-chain-overlap-third"} {
		approveDiscoveryMarkdown(t, repo, lineage, "docs/shared.md", "reviewed "+strings.Repeat(string(rune('a'+index)), index+1)+"\n")
		runReviewCLIGit(t, repo, "add", "-A")
		runReviewCLIGit(t, repo, "commit", "-qm", "deliver "+lineage)
	}

	var output bytes.Buffer
	if err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePrePR), "--base-ref", "origin/" + branch,
	}, &output); err != nil {
		t.Fatalf("same-path composed pre-PR validation: %v\n%s", err, output.String())
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, output.Bytes()).Result, &result)
	if !result.Allowed || result.Context.LineageID != "review-chain-overlap-third" {
		t.Fatalf("same-path composed pre-PR result = %#v", result)
	}
}

func TestUnqualifiedPrePRDiscoveryReconcilesCurrentChangesReceiptAcrossDivergedBase(t *testing.T) {
	repo := initReviewCLIRepo(t)
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	initialCommit := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	remote := filepath.Join(t.TempDir(), "remote.git")
	runReviewCLIGit(t, repo, "clone", "--bare", repo, remote)
	runReviewCLIGit(t, repo, "remote", "add", "origin", remote)
	started, _ := approveDiscoveryMarkdown(t, repo, "review-current-diverged-base", "docs/single.md", "reviewed\n")
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", "deliver reviewed change")
	runReviewCLIGit(t, repo, "checkout", "-qb", "base-advance", initialCommit)
	if err := os.WriteFile(filepath.Join(repo, "base-only.txt"), []byte("base advance\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "base-only.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "advance publication base")
	runReviewCLIGit(t, repo, "push", "-q", "origin", "base-advance:refs/heads/"+branch)
	runReviewCLIGit(t, repo, "checkout", "-q", branch)

	var output bytes.Buffer
	if err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePrePR), "--base-ref", "origin/" + branch,
	}, &output); err != nil {
		t.Fatalf("diverged-base current-changes pre-PR validation: %v\n%s", err, output.String())
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, output.Bytes()).Result, &result)
	if !result.Allowed || result.Context.LineageID != started.LineageID || result.Context.BaseAdvance == nil ||
		!result.Context.BaseAdvance.Compatible || result.Context.BaseAdvance.Status != "current-changes-boundary-compatible" {
		t.Fatalf("diverged-base current-changes pre-PR result = %#v", result)
	}
}

func TestUnqualifiedPrePRDiscoveryKeepsExactSingleReceiptContext(t *testing.T) {
	repo := initReviewCLIRepo(t)
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	remote := filepath.Join(t.TempDir(), "remote.git")
	runReviewCLIGit(t, repo, "clone", "--bare", repo, remote)
	runReviewCLIGit(t, repo, "remote", "add", "origin", remote)
	started, store := approveDiscoveryMarkdown(t, repo, "review-single-pre-pr", "docs/single.md", "reviewed\n")
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", "deliver exact single receipt")
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePrePR), "--base-ref", "origin/" + branch,
	}, &output); err != nil {
		t.Fatalf("exact single pre-PR validation: %v\n%s", err, output.String())
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, output.Bytes()).Result, &result)
	if !result.Allowed || result.Context.LineageID != started.LineageID || result.Context.StoreRevision != record.Revision || result.Context.ChainIdentity != record.Revision {
		t.Fatalf("exact single receipt context changed = %#v", result.Context)
	}
}

func approveDiscoveryMarkdown(t *testing.T, repo, lineage, logicalPath, content string) (ReviewFacadeStartResult, reviewtransaction.CompactStore) {
	return approveDiscoveryMarkdownProjection(t, repo, lineage, logicalPath, content, reviewtransaction.ProjectionWorkspace)
}

func approveDiscoveryMarkdownProjection(t *testing.T, repo, lineage, logicalPath, content string, projection reviewtransaction.Projection) (ReviewFacadeStartResult, reviewtransaction.CompactStore) {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(logicalPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if projection == reviewtransaction.ProjectionStaged {
		runReviewCLIGit(t, repo, "add", "-A")
	}
	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", lineage, "--projection", string(projection)}, &output); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, output.Bytes(), &started)
	if started.RiskLevel != reviewtransaction.RiskLow {
		t.Fatalf("discovery fixture risk = %q", started.RiskLevel)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", lineage}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	return started, store
}

func cloneApprovedDiscoveryAuthority(t *testing.T, repo string, source reviewtransaction.CompactStore, lineage string) reviewtransaction.CompactStore {
	t.Helper()
	record, err := source.Load()
	if err != nil {
		t.Fatal(err)
	}
	lines := record.State.OriginalChangedLines
	clone, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: lineage, Mode: reviewtransaction.ModeOrdinaryBounded, Generation: record.State.Generation,
		Snapshot: record.State.InitialSnapshot, PolicyHash: record.State.PolicyHash, RiskLevel: record.State.RiskLevel,
		SelectedLenses: append([]string{}, record.State.SelectedLenses...), OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	cloneStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, clone.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := cloneStore.Replace("", "review/start", clone)
	if err != nil {
		t.Fatal(err)
	}
	if err := clone.CompleteReview(reviewtransaction.CompactReviewInput{LensResults: []reviewtransaction.LensResult{}}); err != nil {
		t.Fatal(err)
	}
	revision, err = cloneStore.Replace(revision, "review/complete-review", clone)
	if err != nil {
		t.Fatal(err)
	}
	if err := clone.CompleteVerification([]byte("independent duplicate fixture evidence"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := cloneStore.Replace(revision, "review/complete-verification", clone); err != nil {
		t.Fatal(err)
	}
	receipt, err := clone.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := reviewtransaction.WriteCompactReceiptAtomic(cloneStore.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	return cloneStore
}
