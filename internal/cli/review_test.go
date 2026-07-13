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

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
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
		Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{},
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
	repo := t.TempDir()
	runReviewCLIGit(t, repo, "init", "-q")
	runReviewCLIGit(t, repo, "config", "user.email", "test@example.com")
	runReviewCLIGit(t, repo, "config", "user.name", "Test")
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
