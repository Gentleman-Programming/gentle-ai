package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestFlatReviewStartRejectsBeforeCreatingLegacyAuthority(t *testing.T) {
	repo := initReviewCLIRepo(t)
	policy := filepath.Join(t.TempDir(), "policy.md")
	mirror := filepath.Join(t.TempDir(), "transaction.json")
	if err := os.WriteFile(policy, []byte("legacy policy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := RunReviewStart([]string{
		"--cwd", repo, "--lineage", "flat-start-read-only", "--policy-file", policy,
		"--machine-transaction-out", mirror,
	}, io.Discard)
	if !errors.Is(err, reviewtransaction.ErrLegacyReadOnly) || !strings.Contains(err.Error(), "gentle-ai review start") {
		t.Fatalf("flat review-start error = %v", err)
	}
	store, storeErr := reviewtransaction.AuthoritativeStore(context.Background(), repo, "flat-start-read-only")
	if storeErr != nil {
		t.Fatal(storeErr)
	}
	if _, err := store.LoadChain(); !os.IsNotExist(err) {
		t.Fatalf("flat review-start created v1 authority: %v", err)
	}
	if _, err := os.Stat(mirror); !os.IsNotExist(err) {
		t.Fatalf("flat review-start created mirror: %v", err)
	}
}

// TestReviewFacadeStartRefusesOverExistingV1Authority is Wave 7 S7a's (WU18a)
// end-to-end proof for the v1 collision guard added to the switch-ON v3
// start path in runReviewFacadeStart: with GENTLE_AI_RDD_NEW_LINEAGE set, a
// v3 start over an existing v1 chain must still refuse -- with the exact
// same, pre-existing "choose a new lineage for compact authority" wording
// every other legacy-read-only collision in this codebase shares (see
// review_operation_contract_test.go, review_stop_discoverability_test.go,
// review_failure_contract_test.go, review_status_contract_test.go) -- rather
// than silently freezing a v3 record alongside the live v1 chain. (The
// legacy, switch-OFF branch already had this exact guard unchanged; this
// test specifically exercises the NEW switch-ON copy WU18a added.)
func TestReviewFacadeStartRefusesOverExistingV1Authority(t *testing.T) {
	reviewEnabledHome(t)
	fixture := newLegacyCLIFixture(t, "v1-blocks-v3-start")
	runReviewCLIGit(t, fixture.repo, "add", "-A")
	t.Setenv("GENTLE_AI_RDD_NEW_LINEAGE", "1")

	var output bytes.Buffer
	err := RunReviewFacadeStart([]string{"--cwd", fixture.repo, "--lineage", fixture.lineage}, &output)
	if err == nil {
		t.Fatalf("v3 start over live v1 authority succeeded: %s", output.String())
	}
	var typed *reviewtransaction.LegacyReadOnlyError
	if !errors.Is(err, reviewtransaction.ErrLegacyReadOnly) || !errors.As(err, &typed) ||
		typed.Operation != "review/start" || typed.LineageID != fixture.lineage ||
		!strings.Contains(err.Error(), "choose a new lineage for compact authority") {
		t.Fatalf("v3 start over live v1 authority error = %v", err)
	}
	// Nothing was frozen: no v3 record exists for this lineage id.
	v3Store, storeErr := reviewtransaction.NewLineageAuthorityStore(context.Background(), fixture.repo, fixture.lineage)
	if storeErr != nil {
		t.Fatal(storeErr)
	}
	if _, statErr := os.Stat(v3Store.Dir); !os.IsNotExist(statErr) {
		t.Fatalf("v3 start over live v1 authority created a v3 record: stat err = %v", statErr)
	}
}

// TestReviewFacadeStartRefusesOverExistingV2AuthorityAndNamesRecover is Wave
// 7 S7a's (WU18a) new guard: before this wave, a switch-ON `review start`
// (runReviewFacadeStartNewLineage, called directly) never checked for an
// existing compact-v2 lineage under the same id at all -- the switch-OFF
// legacy branch's own internal conflict detection never runs when the
// switch is on, and the switch-ON path had no guard of its own. Without
// this fix a v3 record could be created silently alongside a LIVE v2
// lineage of the same name. Unlike the v1 case above, a v2 predecessor
// genuinely IS `review recover`'s own supported shape
// (reviewtransaction.CompactAuthoritativeStore) -- so unlike v1's "choose a
// new lineage" (the only real exit for a v1 collision, review recover never
// accepts a v1 chain), this refusal must name review recover as an
// ADDITIONAL, genuinely resolving option.
func TestReviewFacadeStartRefusesOverExistingV2AuthorityAndNamesRecover(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	const lineage = "v2-blocks-v3-start"
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("v2 collision fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	root, err := builder.ResolveRepositoryRoot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rootBuilder := reviewtransaction.SnapshotBuilder{Repo: root}
	snapshot, err := rootBuilder.Build(ctx, reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := rootBuilder.AssessSnapshotRisk(ctx, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	lenses, err := facadeSelectedLenses(assessment, "reliability")
	if err != nil {
		t.Fatal(err)
	}
	policy, err := facadePolicyBytes("")
	if err != nil {
		t.Fatal(err)
	}
	state, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: lineage, Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 1,
		Snapshot: snapshot, PolicyHash: facadePayloadHash(policy), RiskLevel: assessment.Level,
		SelectedLenses: lenses, OriginalChangedLines: &assessment.ChangedLines,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reviewtransaction.StartCompactAuthority(ctx, root, reviewtransaction.CompactStartRequest{
		State: state, ExplicitLineage: true,
	}); err != nil {
		t.Fatalf("start compact-v2 collision fixture: %v", err)
	}

	// A genuinely DIFFERENT candidate than the v2 fixture above: the guard
	// is content-aware (an exact hint-replay of the SAME candidate must not
	// be refused, only a real conflict), so this collision proof needs
	// scope that actually changed, not just a second start attempt.
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("v2 collision fixture, now with different content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GENTLE_AI_RDD_NEW_LINEAGE", "1")

	var output bytes.Buffer
	startErr := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", lineage}, &output)
	if startErr == nil {
		t.Fatalf("v3 start over live v2 authority succeeded: %s", output.String())
	}
	if !strings.Contains(startErr.Error(), "review recover") ||
		!strings.Contains(startErr.Error(), "--predecessor-lineage "+lineage) ||
		!strings.Contains(startErr.Error(), "already governs this lineage id") {
		t.Fatalf("v3 start over live v2 authority error = %v, want it to name review recover with the predecessor pre-filled", startErr)
	}
	// Nothing was frozen: no v3 record exists for this lineage id, and the
	// v2 record is untouched.
	v3Store, storeErr := reviewtransaction.NewLineageAuthorityStore(ctx, repo, lineage)
	if storeErr != nil {
		t.Fatal(storeErr)
	}
	if _, statErr := os.Stat(v3Store.Dir); !os.IsNotExist(statErr) {
		t.Fatalf("v3 start over live v2 authority created a v3 record: stat err = %v", statErr)
	}
	compact, err := reviewtransaction.CompactAuthoritativeStore(ctx, repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compact.Load(); err != nil {
		t.Fatalf("v2 authority untouched check: %v", err)
	}
}

func TestLegacyV1ResumeValidateExportImportRemainUsable(t *testing.T) {
	fixture := newLegacyCLIFixture(t, "legacy-readable")
	var output bytes.Buffer
	if err := RunReviewResume([]string{"--cwd", fixture.repo, "--lineage", fixture.lineage}, &output); err != nil {
		t.Fatalf("legacy resume: %v", err)
	}
	var resumed ReviewResumeResult
	if err := json.Unmarshal(output.Bytes(), &resumed); err != nil || resumed.Transaction.State != reviewtransaction.StateApproved {
		t.Fatalf("legacy resume result = %#v, %v", resumed, err)
	}
	output.Reset()
	runReviewCLIGit(t, fixture.repo, "add", "tracked.txt")
	if err := RunReviewValidate([]string{
		"--cwd", fixture.repo, "--receipt", fixture.receiptPath,
		"--lineage", fixture.lineage, "--gate", string(reviewtransaction.GatePreCommit),
	}, &output); err != nil {
		t.Fatalf("legacy pre-commit validate: %v\n%s", err, output.String())
	}
	assertReviewGateResult(t, output.Bytes(), reviewtransaction.GateAllow)

	runReviewCLIGit(t, fixture.repo, "add", "tracked.txt")
	runReviewCLIGit(t, fixture.repo, "commit", "-qm", "candidate")
	bundlePath := filepath.Join(t.TempDir(), "legacy-bundle.json")
	if err := RunReviewBundleExport([]string{"--cwd", fixture.repo, "--lineage", fixture.lineage, "--out", bundlePath}, io.Discard); err != nil {
		t.Fatalf("legacy export: %v", err)
	}
	bundlePayload, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := reviewtransaction.ParseChainBundle(bundlePayload)
	if err != nil {
		t.Fatal(err)
	}
	request, err := reviewtransaction.BuildNativeGateRequest(context.Background(), fixture.repo, reviewtransaction.NativeGateRequestInput{
		Gate: reviewtransaction.GatePrePush, LineageID: fixture.lineage,
		PolicyArtifact: fixture.policyPath, LedgerArtifact: fixture.ledgerPath, EvidenceArtifact: fixture.evidencePath,
	})
	if err != nil {
		t.Fatal(err)
	}
	request.StoreRevision = bundle.HeadRevision
	request.GenesisRevision = bundle.GenesisRevision
	request.ChainIdentity = bundle.ChainIdentity
	request.BundleDigest = bundle.BundleDigest
	requestPath := filepath.Join(t.TempDir(), "request.json")
	writeReviewCLIJSON(t, requestPath, request)
	runReviewCLIGit(t, fixture.repo, "branch", "reviewed-base", "HEAD^")
	clone := filepath.Join(t.TempDir(), "clone")
	runReviewCLIGit(t, fixture.repo, "clone", "--no-local", fixture.repo, clone)
	if err := RunReviewBundleImport([]string{
		"--cwd", clone, "--bundle", bundlePath, "--receipt", fixture.receiptPath, "--request", requestPath,
	}, io.Discard); err != nil {
		t.Fatalf("legacy import: %v", err)
	}
	if err := RunReviewResume([]string{"--cwd", clone, "--lineage", fixture.lineage}, io.Discard); err != nil {
		t.Fatalf("imported legacy resume: %v", err)
	}
	output.Reset()
	if err := RunReviewValidate([]string{
		"--cwd", clone, "--receipt", fixture.receiptPath,
		"--lineage", fixture.lineage, "--gate", string(reviewtransaction.GatePrePush), "--base-ref", "origin/reviewed-base",
	}, &output); err != nil {
		t.Fatalf("imported legacy validate: %v\n%s", err, output.String())
	}
	importedBundle := filepath.Join(t.TempDir(), "imported-bundle.json")
	if err := RunReviewBundleExport([]string{"--cwd", clone, "--lineage", fixture.lineage, "--out", importedBundle}, io.Discard); err != nil {
		t.Fatalf("imported legacy export: %v", err)
	}
}

func TestLegacyV1MutationCommandsRejectWithoutChangingHead(t *testing.T) {
	fixture := newLegacyCLIFixture(t, "legacy-mutation-rejected")
	headPath := filepath.Join(fixture.store.Dir, "HEAD")
	before, err := os.ReadFile(headPath)
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "input.json")
	if err := os.WriteFile(input, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = RunReviewStep([]string{
		"--cwd", fixture.repo, "--lineage", fixture.lineage,
		"--operation", "begin-final-verification", "--input", input,
	}, io.Discard)
	if !errors.Is(err, reviewtransaction.ErrLegacyReadOnly) {
		t.Fatalf("legacy review-step error = %v", err)
	}
	fresh, err := reviewtransaction.AuthoritativeStore(context.Background(), fixture.repo, fixture.lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := fresh.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fresh.Append(fixture.head, reviewtransaction.Record{Operation: "review/complete-final-verification", Transaction: record.Transaction}); !errors.Is(err, reviewtransaction.ErrLegacyReadOnly) {
		t.Fatalf("legacy direct append error = %v", err)
	}
	after, err := os.ReadFile(headPath)
	if err != nil || !bytes.Equal(after, before) {
		t.Fatalf("legacy mutation changed HEAD: %v", err)
	}
}

func TestLegacyV1ExplicitAndNativeValidationRemainEquivalent(t *testing.T) {
	fixture := newLegacyCLIFixture(t, "legacy-gate-parity")
	runReviewCLIGit(t, fixture.repo, "add", "tracked.txt")
	request, err := reviewtransaction.BuildNativeGateRequest(context.Background(), fixture.repo, reviewtransaction.NativeGateRequestInput{
		Gate: reviewtransaction.GatePreCommit, LineageID: fixture.lineage,
	})
	if err != nil {
		t.Fatal(err)
	}
	requestPath := filepath.Join(t.TempDir(), "request.json")
	writeReviewCLIJSON(t, requestPath, request)
	var native, explicit bytes.Buffer
	if err := RunReviewValidate([]string{
		"--cwd", fixture.repo, "--receipt", fixture.receiptPath,
		"--lineage", fixture.lineage, "--gate", string(reviewtransaction.GatePreCommit),
	}, &native); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewValidate([]string{
		"--cwd", fixture.repo, "--receipt", fixture.receiptPath, "--request", requestPath,
	}, &explicit); err != nil {
		t.Fatal(err)
	}
	var nativeResult, explicitResult ReviewValidateResult
	if err := json.Unmarshal(native.Bytes(), &nativeResult); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(explicit.Bytes(), &explicitResult); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(nativeResult, explicitResult) {
		t.Fatalf("legacy explicit/native results differ:\n%s\n%s", native.String(), explicit.String())
	}
}

func TestReviewSubcommandHelpLabelsLegacyMutationReadOnly(t *testing.T) {
	for _, test := range []struct {
		name string
		run  func([]string, io.Writer) error
	}{
		{name: "review-start", run: RunReviewStart},
		{name: "review-step", run: RunReviewStep},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := test.run([]string{"--help"}, &output); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(strings.ToLower(output.String()), "read-only") {
				t.Fatalf("legacy help does not label mutation read-only:\n%s", output.String())
			}
		})
	}
}

func TestReviewGateActionScopeChangedRequiresExplicitMaintainerAction(t *testing.T) {
	for _, tt := range []struct {
		result reviewtransaction.GateResult
		want   string
	}{
		{reviewtransaction.GateAllow, "continue"},
		{reviewtransaction.GateScopeChanged, "explicit-maintainer-action"},
		{reviewtransaction.GateInvalidated, "explicit-maintainer-action"},
		// organic-dx Phase 3b task 3b.3: STATUS already re-derives escalated
		// recovery eligibility (accounting-only, changed-target, or
		// final-verification-retry), so the gate denial now names
		// review.status instead of a bare stop that told the caller nothing
		// it did not already know.
		{reviewtransaction.GateEscalated, "review.status"},
	} {
		t.Run(string(tt.result), func(t *testing.T) {
			result := ReviewValidateResult{Result: tt.result, Allowed: tt.result == reviewtransaction.GateAllow, Action: reviewGateAction(tt.result)}
			payload, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			if result.Action != tt.want || result.Allowed != (tt.result == reviewtransaction.GateAllow) || strings.Contains(string(payload), "create-new-lineage") {
				t.Fatalf("gate result = %s", payload)
			}
		})
	}
}

type legacyCLIFixture struct {
	repo, lineage, policyPath, ledgerPath, evidencePath, receiptPath string
	store                                                            reviewtransaction.Store
	head                                                             string
}

func newLegacyCLIFixture(t *testing.T, lineage string) legacyCLIFixture {
	t.Helper()
	repo := initReviewCLIRepo(t)
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	configureCLIReviewPublicationRemote(t, repo, branch)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
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
	head = appendLegacyCLIRecord(t, store, head, "review/complete-final-verification", *tx)
	receipt, err := tx.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(store.Dir, "artifacts", "receipt.json")
	if err := reviewtransaction.WriteReceiptAtomic(receiptPath, receipt); err != nil {
		t.Fatal(err)
	}
	return legacyCLIFixture{
		repo: repo, lineage: lineage, policyPath: policyPath, ledgerPath: ledgerPath,
		evidencePath: evidencePath, receiptPath: receiptPath, store: store, head: head,
	}
}

func appendLegacyCLIRecord(t *testing.T, store reviewtransaction.Store, previous, operation string, transaction reviewtransaction.Transaction) string {
	t.Helper()
	revision, err := store.Append(previous, reviewtransaction.Record{Operation: operation, Transaction: transaction})
	if err != nil {
		t.Fatal(err)
	}
	return revision
}

func configureCLIReviewPublicationRemote(t *testing.T, repo, branch string) {
	t.Helper()
	remote := filepath.Join(t.TempDir(), "origin.git")
	runReviewCLIGit(t, repo, "clone", "--bare", repo, remote)
	runReviewCLIGit(t, repo, "remote", "add", "origin", remote)
	runReviewCLIGit(t, repo, "config", "branch."+branch+".remote", "origin")
	runReviewCLIGit(t, repo, "config", "branch."+branch+".merge", "refs/heads/"+branch)
}

func assertReviewGateResult(t *testing.T, payload []byte, want reviewtransaction.GateResult) {
	t.Helper()
	var result ReviewValidateResult
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatal(err)
	}
	if result.Result != want || result.Allowed != (want == reviewtransaction.GateAllow) {
		t.Fatalf("review gate result = %#v, want %q", result, want)
	}
}

func initReviewCLIRepo(t *testing.T) string {
	t.Helper()
	repo, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "init", "-q")
	runReviewCLIGit(t, repo, "config", "user.email", "test@example.com")
	runReviewCLIGit(t, repo, "config", "user.name", "Test")
	runReviewCLIGit(t, repo, "config", "core.autocrlf", "false")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "base")
	return repo
}

func runReviewCLIGit(t *testing.T, repo string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repo}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return string(output)
}

func writeReviewCLIJSON(t *testing.T, path string, value any) {
	t.Helper()
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

// reviewCLIAuthorityRoot and writeReconcileCLIRecord used to live in
// review_reconcile_test.go, retired in Wave 7 S3a along with the CLI verb it
// tested — both helpers are shared, reused across review_repair_test.go,
// review_abandon_test.go, review_partial_capture_deadend_test.go,
// review_incident_recapture_test.go, review_inspect_authority_test.go,
// review_reconcile_batch_test.go, and review_repair_transition_test.go
// (confirmed by grep before the retiring commit), so they moved here rather
// than dying with their original home.

func reviewCLIAuthorityRoot(t *testing.T, repo string) string {
	t.Helper()
	commonDir := filepath.Clean(strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "--path-format=absolute", "--git-common-dir")))
	return filepath.Join(commonDir, "gentle-ai", "review-transactions")
}

// writeReconcileCLIRecord persists one compact-v2 record directly to disk
// (bypassing the product's own write path) so a fixture can seed an exact,
// already-known revision for a test to bind against.
func writeReconcileCLIRecord(t *testing.T, repo string, state reviewtransaction.CompactState) string {
	t.Helper()
	revision, err := reviewtransaction.CompactRevisionForState(state)
	if err != nil {
		t.Fatal(err)
	}
	record := reviewtransaction.CompactRecord{Schema: "gentle-ai.review-state-record/v2", Revision: revision, State: state}
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", state.LineageID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "review-state.json"), append(payload, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	return revision
}
