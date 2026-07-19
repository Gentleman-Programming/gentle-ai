package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

func malformedLegacyFreezeCLIFixture(t *testing.T) (repo, lineage, head string) {
	t.Helper()
	repo = initReviewCLIRepo(t)
	lineage = "cli-malformed-legacy-freeze"
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := reviewtransaction.NewTransaction(reviewtransaction.Start{
		LineageID: lineage, Mode: reviewtransaction.ModeOrdinary4R, Generation: 1,
		Snapshot: snapshot, PolicyHash: "sha256:" + strings.Repeat("ab", 32),
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
	genesis := appendLegacyCLIRecord(t, store, "", "review/start", *tx)
	ledger, err := reviewtransaction.CanonicalLedger([]reviewtransaction.Finding{})
	if err != nil {
		t.Fatal(err)
	}
	ledgerPath := filepath.Join(t.TempDir(), "ledger.json")
	if err := os.WriteFile(ledgerPath, ledger, 0o644); err != nil {
		t.Fatal(err)
	}
	ledgerHash, err := reviewtransaction.HashLedgerArtifact(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.FreezeFindings([]reviewtransaction.Finding{}, ledger, ledgerHash); err != nil {
		t.Fatal(err)
	}
	tx.EvidenceHash = "sha256:" + strings.Repeat("cd", 32)
	record := reviewtransaction.Record{
		Schema: reviewtransaction.RecordSchema, Operation: "review/freeze-findings",
		PreviousRevision: genesis, Transaction: *tx,
	}
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	payload = append(payload, '\n')
	sum := sha256.Sum256(payload)
	head = "sha256:" + hex.EncodeToString(sum[:])
	if err := os.MkdirAll(filepath.Join(store.Dir, "events"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Dir, "events", strings.TrimPrefix(head, "sha256:")+".json"), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Dir, "HEAD"), []byte(head+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return repo, lineage, head
}

func TestReviewQuarantineLegacyRestoresAuthoritativeInventory(t *testing.T) {
	repo, lineage, head := malformedLegacyFreezeCLIFixture(t)
	diagnostic := "historical findings freeze changed unrelated transaction state"
	disposition := reviewtransaction.LegacyMalformedFreezeQuarantineDisposition
	reason, actor := "retire malformed shipped legacy history", "maintainer@example.com"
	// The command derives the binding over the canonical repository root
	// (filepath.Abs -> EvalSymlinks -> Clean). On Windows CI t.TempDir()
	// yields 8.3 short-name components (e.g. RUNNER~1) that EvalSymlinks
	// expands, so a binding built from the raw repo path would never match.
	// Resolve the same canonical root the command uses.
	repository, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	authorization := "gentle-ai.review-legacy-quarantine-authorization/v1\nrepository=" + repository +
		"\nlineage=" + lineage + "\nrevision=" + head + "\ndiagnostic=" + diagnostic +
		"\ndisposition=" + disposition + "\nactor=" + actor + "\nreason=" + reason

	var output bytes.Buffer
	if err := RunReview([]string{
		"quarantine-legacy", "--cwd", repo, "--lineage", lineage,
		"--expected-revision", head, "--diagnostic", diagnostic, "--disposition", disposition,
		"--reason", reason, "--actor", actor, "--maintainer-authorization", authorization,
	}, &output); err != nil {
		t.Fatalf("review quarantine-legacy: %v\n%s", err, output.String())
	}
	var result ReviewLegacyQuarantineResult
	decodeStrictReviewJSON(t, output.Bytes(), &result)
	if result.Operation != "review/quarantine-legacy" || result.Record.Status != reviewtransaction.CompactReclaimCommitted ||
		result.Record.MalformedLegacyFreeze == nil || result.Record.MalformedLegacyFreeze.EventRevision != head {
		t.Fatalf("legacy quarantine result = %#v", result)
	}
	report, err := reviewtransaction.InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || !report.Authoritative {
		t.Fatalf("post-quarantine inventory = %#v", report)
	}

	var help bytes.Buffer
	if err := RunReview([]string{"help"}, &help); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(help.String(), "quarantine-legacy") {
		t.Fatalf("review help omits quarantine-legacy: %s", help.String())
	}
}
