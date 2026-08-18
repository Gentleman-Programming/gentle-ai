package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestReviewFacadeKeepsInvalidatedAuthorityNonPoisoningAndNonUsable(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	started := startFacadeReview(t, repo)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReviewInvalidate([]string{
		"--cwd", repo, "--lineage", started.LineageID,
		"--expected-revision", record.Revision, "--reason", "operator abandoned",
	}, &output); err != nil {
		t.Fatalf("invalidate pristine compact authority: %v", err)
	}

	output.Reset()
	if err := RunReviewStatus([]string{"--cwd", repo}, &output); err != nil {
		t.Fatal(err)
	}
	var report reviewtransaction.AuthorityStatusReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if !report.Complete || !report.Authoritative || report.Status != reviewtransaction.AuthorityStatusInvalidated {
		t.Fatalf("invalidated authority poisoned status report = %#v", report)
	}
	if len(report.Entries) != 1 || report.Entries[0].Status != reviewtransaction.AuthorityStatusInvalidated || len(report.Entries[0].Problems) != 0 {
		t.Fatalf("invalidated authority entry = %#v", report.Entries)
	}

	output.Reset()
	if err := RunReview([]string{
		"status", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", started.LineageID,
	}, &output); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&status); err != nil {
		t.Fatal(err)
	}
	if err := status.Validate(); err != nil {
		t.Fatal(err)
	}
	if status.Applicability != reviewtransaction.TargetApplicabilityCurrent || status.Authority == nil ||
		status.Authority.LineageID != started.LineageID || status.Authority.State != reviewtransaction.StateInvalidated ||
		status.Action != reviewtransaction.TargetStatusActionRecover || status.ActionDisposition != reviewtransaction.RecoveryInvalidated ||
		status.Replayability != reviewtransaction.ReplayabilityManualActionRequired {
		t.Fatalf("negotiated invalidated status = %#v", status)
	}

	output.Reset()
	err = RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &output)
	if err == nil {
		t.Fatal("invalidated authority was accepted as usable gate authority")
	}
	var denied ReviewGateDeniedError
	if !errors.As(err, &denied) || denied.Context.Denial == nil ||
		denied.Context.Denial.Stage != "receipt-discovery" || denied.Context.Denial.Code != string(ReviewReceiptMissing) {
		t.Fatalf("invalidated authority gate denial = %#v, want receipt-discovery %q", err, ReviewReceiptMissing)
	}
}
