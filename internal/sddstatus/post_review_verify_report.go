package sddstatus

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

type postReviewVerifyReportAttestation int

const (
	postReviewVerifyReportUnproven postReviewVerifyReportAttestation = iota
	postReviewVerifyReportRequired
	postReviewVerifyReportBound
)

// classifyPostReviewVerifyReportAttestation proves the exact final verify
// settlement, report bytes, current candidate tree, and one-path receipt delta.
// A missing digest has nothing to attest: it classifies as recoverable once
// the receipt and path anchoring checks succeed, before any snapshot build;
// the legacy caller-owned work-unit label does not govern the recovery offer.
func classifyPostReviewVerifyReportAttestation(
	ctx context.Context,
	repo, workspace, change string,
	ref reviewtransaction.SDDReceiptRef,
	runtime *RuntimeStatus,
	specCounts SpecCounts,
) postReviewVerifyReportAttestation {
	if runtime == nil || runtime.ActiveAttempt != nil || !runtime.Complete || len(runtime.Attempts) == 0 {
		return postReviewVerifyReportUnproven
	}
	settlement := runtime.Attempts[len(runtime.Attempts)-1]
	if settlement.Outcome != AttemptPassed || settlement.FinishCandidateTree == "" || settlement.EvidenceRevision == "" {
		return postReviewVerifyReportUnproven
	}

	store, err := reviewtransaction.CompactAuthoritativeStore(ctx, repo, ref.Lineage)
	if err != nil {
		return postReviewVerifyReportUnproven
	}
	record, err := store.Load()
	if err != nil || record.State.State != reviewtransaction.StateApproved || record.State.Validate() != nil {
		return postReviewVerifyReportUnproven
	}
	receiptPayload, err := os.ReadFile(store.ReceiptPath())
	if err != nil || verifyReportDigest(receiptPayload) != ref.ReceiptHash {
		return postReviewVerifyReportUnproven
	}
	receipt, err := reviewtransaction.ParseCompactReceipt(receiptPayload)
	if err != nil || receipt.LineageID != ref.Lineage {
		return postReviewVerifyReportUnproven
	}
	authoritativeReceipt, err := record.State.Receipt()
	if err != nil || !reflect.DeepEqual(receipt, authoritativeReceipt) ||
		record.State.CurrentSnapshot.CandidateTree != receipt.FinalCandidateTree {
		return postReviewVerifyReportUnproven
	}

	changeRoot, err := resolveBindingChangeRoot(ctx, repo, workspace, change)
	if err != nil {
		return postReviewVerifyReportUnproven
	}
	reportPath := filepath.Join(changeRoot, verifyReportFileName)
	// Mirror captureFinalVerifyReport: the canonical path is anchored at the
	// planning workspace, tree reads at the repository root (which may differ).
	logicalReportPath, err := canonicalVerifyReportPaths(repo, workspace, changeRoot, change)
	if err != nil {
		return postReviewVerifyReportUnproven
	}

	// Legacy records had no native digest and work-unit labels are caller-owned.
	// Their label cannot grant archive authority, but with nothing to attest
	// there is nothing to byte-gate either: the settlement classifies here as
	// the safe recovery offer, before the snapshot build below could hash
	// drifted worktree bytes into the Git object database.
	if settlement.AttestedVerifyReportDigest == "" {
		// The recovery offer is honest only when the receipt-to-current delta
		// is the canonical verify report alone; any other touched path keeps
		// the pre-existing scope-changed routing. git diff against the receipt
		// tree is read-only, so this proof still writes no Git objects.
		delta, err := exec.CommandContext(ctx, "git", "-C", repo, "diff",
			"--name-only", "-z", "--no-renames", receipt.FinalCandidateTree, "--").Output()
		if err != nil {
			return postReviewVerifyReportUnproven
		}
		for _, changed := range strings.Split(string(delta), "\x00") {
			if changed != "" && changed != logicalReportPath {
				return postReviewVerifyReportUnproven
			}
		}
		return postReviewVerifyReportRequired
	}
	// Status classification never writes to the Git object database: this cheap
	// byte gate rejects a worktree report that cannot match the attested digest
	// before any snapshot build hashes drifted bytes; filters only fail closed.
	worktreeReport, err := os.ReadFile(reportPath)
	if err != nil || len(worktreeReport) > MaxVerifyReportBytes ||
		verifyReportDigest(worktreeReport) != settlement.AttestedVerifyReportDigest {
		return postReviewVerifyReportUnproven
	}

	current, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(ctx, reviewtransaction.Target{
		Kind: reviewtransaction.TargetBaseWorkspaceOverlay, BaseRef: receipt.FinalCandidateTree,
		Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
	})
	if err != nil || current.CandidateTree != settlement.FinishCandidateTree ||
		!reflect.DeepEqual(current.Paths, []string{logicalReportPath}) {
		return postReviewVerifyReportUnproven
	}
	payload, err := reviewtransaction.ReadTreeBlob(ctx, repo, current.CandidateTree, logicalReportPath, MaxVerifyReportBytes)
	if err != nil {
		return postReviewVerifyReportUnproven
	}
	admission := ValidateVerifyReportAdmission(string(payload), specCounts)
	if !admission.Valid || admission.Verdict != "pass" || admission.EvidenceRevision != settlement.EvidenceRevision {
		return postReviewVerifyReportUnproven
	}
	// Read-only single-blob-delta proof: current.Paths proved only the report
	// path differs and both ReadTreeBlob calls prove a canonical 100644 blob on
	// each side, so swapping that one blob reproduces the receipt tree exactly
	// without RestoreTreeBlob's object-writing write-tree round-trip.
	if _, err := reviewtransaction.ReadTreeBlob(ctx, repo, receipt.FinalCandidateTree, logicalReportPath, MaxVerifyReportBytes); err != nil {
		return postReviewVerifyReportUnproven
	}
	// Only the explicit current-binary final verification labels can carry the
	// native digest that grants the archive-status exception.
	if !isFinalVerifyWorkUnit(settlement.WorkUnit) || !runtimeRevisionPattern.MatchString(settlement.AttestedVerifyReportDigest) ||
		verifyReportDigest(payload) != settlement.AttestedVerifyReportDigest {
		return postReviewVerifyReportUnproven
	}
	return postReviewVerifyReportBound
}
