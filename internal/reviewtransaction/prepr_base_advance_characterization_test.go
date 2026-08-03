package reviewtransaction

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// This file characterizes deriveBaseAdvanceCompatibility (prepr.go:73) and its
// BaseAdvanceCompatibility.valid() gate (prepr.go:56) directly, ahead of the
// shadow relation algebra wiring a delegation seam to it (Amendment A,
// openspec/changes/rdd-root-simplification-wave1/specs/rdd-candidate-relation-algebra/spec.md:25-41).
// Wave 0 verify SUGGESTION-5 found 4 callers (gate.go:297, compact_gate.go:310,
// compact_gate.go:472, compact_chain.go:424, compact_recovery_binding.go:286 —
// 5 call sites across 4 files) and no direct covering test for the function
// itself. These tests pin CURRENT behavior; they assert what the function
// does today, not what a future refactor should do. No production code in
// this package changes as part of this file.
//
// The function evaluates seven conditions in a fixed, fail-closed order and
// returns on the first violation:
//  1. merge-base tree preservation                  (prepr.go:88)
//  2. path digest identity                           (prepr.go:101)
//  3. patch identity                                 (prepr.go:109)
//  4. path disjointness                              (prepr.go:116)
//  5. conflict-free merge via `merge-tree --write-tree` (prepr.go:119)
//  6. issuer-bound CI attestation + trust root        (prepr.go:127)
//  7. base/HEAD non-advance revalidation              (prepr.go:135-142)
//
// Because evaluation is ordered and fail-closed, breaking condition N while
// leaving conditions 1..N-1 satisfied isolates exactly that condition: the
// function never reaches N+1..7.

// emptyGitTreeSHA1 is git's well-known empty-tree object, which exists in
// every repository without needing to be written. It is used as "any tree
// hash that is not the real merge-base tree" for the condition-1 failure.
const emptyGitTreeSHA1 = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// baseAdvanceScenario configures one run of deriveBaseAdvanceCompatibility
// against a real Git repository. Each hook lets a single sub-test perturb
// exactly one input while leaving every other condition's inputs valid.
type baseAdvanceScenario struct {
	deliveryPath string // defaults to "delivery.txt"
	basePath     string // defaults to "base-only.txt"

	// mutateContent adds an extra commit to the feature branch (same
	// deliveryPath, different bytes) after the receipt is captured from the
	// original delivery commit but before the base advance and refs/snapshot
	// are captured. It isolates patch-identity divergence (condition 3)
	// without changing the delivered path set (condition 2 stays satisfied).
	mutateContent func(t *testing.T, repo string)

	// mutateReceipt perturbs the frozen receipt fields after they are
	// captured from the real repository state, before the function is
	// called. Used to isolate conditions 1 and 2.
	mutateReceipt func(receipt *Receipt)

	// mutateAttestation perturbs the CI attestation payload before it is
	// signed. Used to isolate condition 6.
	mutateAttestation func(attestation *prePRCIAttestation)

	// afterRefsCaptured runs after refs and snapshot are captured (so it
	// cannot affect what the fixture recorded) but before
	// deriveBaseAdvanceCompatibility is called, so it can move live Git state
	// out from under the already-captured refs. Used to isolate condition 7.
	afterRefsCaptured func(t *testing.T, repo string)
}

type baseAdvanceScenarioResult struct {
	proof      BaseAdvanceCompatibility
	err        error
	repo       string
	receipt    Receipt
	snapshot   Snapshot
	refs       *resolvedPrePRRefs
	mergedTree string
}

// runBaseAdvanceScenario builds a real Git repository with a reviewed
// delivery on a feature branch and a compatible base advance on "main", then
// calls deriveBaseAdvanceCompatibility with inputs that satisfy all seven
// conditions unless one of the scenario hooks perturbs exactly one of them.
func runBaseAdvanceScenario(t *testing.T, scenario baseAdvanceScenario) baseAdvanceScenarioResult {
	t.Helper()
	ctx := context.Background()

	deliveryPath := scenario.deliveryPath
	if deliveryPath == "" {
		deliveryPath = "delivery.txt"
	}
	basePath := scenario.basePath
	if basePath == "" {
		basePath = "base-only.txt"
	}

	repo := initSnapshotRepo(t)
	baseCommit := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	gitSnapshot(t, repo, "branch", "main", baseCommit)
	remote := configurePublicationRemote(t, repo, "main")
	gitSnapshot(t, repo, "checkout", "-qb", "feature")

	writeSnapshotFile(t, repo, deliveryPath, "reviewed delivery\n")
	gitSnapshot(t, repo, "add", "--", deliveryPath)
	gitSnapshot(t, repo, "commit", "-m", "delivery")

	baseTree := trimGit(gitSnapshot(t, repo, "rev-parse", baseCommit+"^{tree}"))
	finalCandidateTree := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD^{tree}"))

	builder := SnapshotBuilder{Repo: repo}
	originalPaths, err := builder.changedPaths(ctx, baseTree, finalCandidateTree)
	if err != nil {
		t.Fatalf("changedPaths: %v", err)
	}
	receipt := Receipt{
		BaseTree:           baseTree,
		FinalCandidateTree: finalCandidateTree,
		PathsDigest:        digestPaths(originalPaths),
	}
	if scenario.mutateReceipt != nil {
		scenario.mutateReceipt(&receipt)
	}

	if scenario.mutateContent != nil {
		scenario.mutateContent(t, repo)
	}

	gitSnapshot(t, repo, "checkout", "main")
	writeSnapshotFile(t, repo, basePath, "base advance\n")
	gitSnapshot(t, repo, "add", "--", basePath)
	gitSnapshot(t, repo, "commit", "-m", "advance base")
	gitSnapshot(t, repo, "push", remote, "main")
	gitSnapshot(t, repo, "checkout", "feature")

	selection, err := selectPrePRBoundary(ctx, repo, "")
	if err != nil {
		t.Fatalf("selectPrePRBoundary: %v", err)
	}
	headCommit, err := resolveCommit(ctx, repo, "HEAD")
	if err != nil {
		t.Fatalf("resolveCommit(HEAD): %v", err)
	}
	refs := &resolvedPrePRRefs{Selection: selection, HeadCommit: headCommit}

	snapshot, err := builder.Build(ctx, Target{Kind: TargetBaseDiff, BaseRef: "main"})
	if err != nil {
		t.Fatalf("Build(TargetBaseDiff): %v", err)
	}

	if scenario.afterRefsCaptured != nil {
		scenario.afterRefsCaptured(t, repo)
	}

	var mergedTree string
	if mergedOutput, mergeErr := runGit(ctx, repo, nil, nil, "merge-tree", "--write-tree", selection.Commit, headCommit); mergeErr == nil {
		if fields := strings.Fields(string(mergedOutput)); len(fields) > 0 {
			mergedTree = fields[0]
		}
	}

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}
	attestation := prePRCIAttestation{Schema: prePRCIAttestationSchema, Issuer: "trusted-ci", MergedTree: mergedTree, Status: "success"}
	if scenario.mutateAttestation != nil {
		scenario.mutateAttestation(&attestation)
	}
	attestation.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, prePRCIAttestationPreimage(attestation)))
	attestationPayload, err := json.Marshal(attestation)
	if err != nil {
		t.Fatalf("marshal attestation: %v", err)
	}
	policy := []byte("# Bounded review policy\n\npre_pr_ci_issuer: trusted-ci\npre_pr_ci_ed25519_public_key: " + base64.StdEncoding.EncodeToString(publicKey) + "\n")

	preimages := gateArtifactPreimages{policy: policy, ciAttestation: attestationPayload}
	request := GateRequest{ExternalEvidence: ExternalEvidenceNone, PrePR: &PrePRRequest{CIAttestationArtifact: "ci-attestation-present"}}

	proof, deriveErr := deriveBaseAdvanceCompatibility(ctx, repo, receipt, request, snapshot, refs, preimages)
	return baseAdvanceScenarioResult{proof: proof, err: deriveErr, repo: repo, receipt: receipt, snapshot: snapshot, refs: refs, mergedTree: mergedTree}
}

// TestDeriveBaseAdvanceCompatibilitySucceedsWhenAllSevenConditionsHold is the
// success characterization: every one of the seven conditions holds, so the
// function returns a compatible, valid proof.
func TestDeriveBaseAdvanceCompatibilitySucceedsWhenAllSevenConditionsHold(t *testing.T) {
	result := runBaseAdvanceScenario(t, baseAdvanceScenario{})
	if result.err != nil {
		t.Fatalf("deriveBaseAdvanceCompatibility() error = %v, want nil", result.err)
	}
	if !result.proof.valid() {
		t.Fatalf("proof.valid() = false, proof = %#v", result.proof)
	}
	if !result.proof.Compatible || result.proof.Status != baseAdvanceCompatibleStatus {
		t.Fatalf("proof compatibility/status = %#v", result.proof)
	}
	// Condition 1: merge-base tree preservation.
	if result.proof.OriginalMergeBaseTree != result.receipt.BaseTree {
		t.Fatalf("OriginalMergeBaseTree = %q, want receipt.BaseTree %q", result.proof.OriginalMergeBaseTree, result.receipt.BaseTree)
	}
	// Condition 2: path digest identity.
	if result.proof.DeliveredPathsDigest != result.receipt.PathsDigest {
		t.Fatalf("DeliveredPathsDigest = %q, want receipt.PathsDigest %q", result.proof.DeliveredPathsDigest, result.receipt.PathsDigest)
	}
	// Condition 3: patch identity.
	if result.proof.OriginalPatchIdentity != result.proof.DeliveredPatchIdentity {
		t.Fatalf("patch identity diverged: original=%q delivered=%q", result.proof.OriginalPatchIdentity, result.proof.DeliveredPatchIdentity)
	}
	// Condition 4: path disjointness.
	if !result.proof.PathsDisjoint {
		t.Fatalf("PathsDisjoint = false, want true")
	}
	// Condition 5: conflict-free merge-tree.
	if !validGitTree(result.proof.MergedResultTree) || result.proof.MergedResultTree != result.mergedTree {
		t.Fatalf("MergedResultTree = %q, want valid tree %q", result.proof.MergedResultTree, result.mergedTree)
	}
	// Condition 6: issuer-bound CI attestation + trust root.
	if result.proof.CIAttestationIssuer != "trusted-ci" || result.proof.CIStatus != "success" {
		t.Fatalf("attestation evidence = issuer=%q status=%q", result.proof.CIAttestationIssuer, result.proof.CIStatus)
	}
	// Condition 7: base/HEAD non-advance revalidation — a nil error already
	// proves the final re-check found no drift between the captured refs and
	// live repository state.
	if result.proof.NewBaseTree != result.snapshot.BaseTree {
		t.Fatalf("NewBaseTree = %q, want snapshot.BaseTree %q", result.proof.NewBaseTree, result.snapshot.BaseTree)
	}
}

// TestDeriveBaseAdvanceCompatibilityFailsClosedOnSingleConditionViolation
// breaks exactly one of the seven conditions per sub-test while leaving every
// earlier condition satisfied, proving each condition is independently
// enforced and reports a distinct error.
func TestDeriveBaseAdvanceCompatibilityFailsClosedOnSingleConditionViolation(t *testing.T) {
	tests := []struct {
		name       string
		condition  string
		scenario   baseAdvanceScenario
		wantErrSub string
	}{
		{
			name:      "merge-base tree not preserved",
			condition: "1: merge-base tree preservation",
			scenario: baseAdvanceScenario{
				mutateReceipt: func(receipt *Receipt) { receipt.BaseTree = emptyGitTreeSHA1 },
			},
			wantErrSub: "original reviewed merge-base tree is not preserved",
		},
		{
			name:      "path digest mismatch",
			condition: "2: path digest identity",
			scenario: baseAdvanceScenario{
				mutateReceipt: func(receipt *Receipt) { receipt.PathsDigest = digestPaths([]string{"unrelated-path.txt"}) },
			},
			wantErrSub: "delivered path identity changed",
		},
		{
			name:      "patch identity changed",
			condition: "3: patch identity",
			scenario: baseAdvanceScenario{
				// Same delivered path, different bytes: the path set (and
				// therefore its digest) stays identical to the receipt, but
				// the diff content — and so the patch identity — diverges.
				mutateContent: func(t *testing.T, repo string) {
					writeSnapshotFile(t, repo, "delivery.txt", "changed after review\n")
					gitSnapshot(t, repo, "add", "--", "delivery.txt")
					gitSnapshot(t, repo, "commit", "-m", "unreviewed content change")
				},
			},
			wantErrSub: "delivered patch identity changed",
		},
		{
			name:      "base advance overlaps delivered paths",
			condition: "4: path disjointness",
			scenario: baseAdvanceScenario{
				deliveryPath: "shared.txt",
				basePath:     "shared.txt",
			},
			wantErrSub: "base advance overlaps delivered paths",
		},
		{
			name:      "merge against new base is not conflict-free",
			condition: "5: conflict-free merge via merge-tree --write-tree",
			scenario: baseAdvanceScenario{
				// A file/directory type clash at the same logical name is
				// disjoint by exact-path comparison (condition 4 still
				// passes) but is an unresolvable tree conflict for
				// `git merge-tree --write-tree` (condition 5 fails).
				deliveryPath: "conflict/file.txt",
				basePath:     "conflict",
			},
			wantErrSub: "merge against new base is not conflict-free",
		},
		{
			name:      "CI attestation issuer does not match the receipt-bound trust root",
			condition: "6: issuer-bound CI attestation + trust root",
			scenario: baseAdvanceScenario{
				mutateAttestation: func(attestation *prePRCIAttestation) { attestation.Issuer = "untrusted-ci" },
			},
			wantErrSub: "CI attestation is not successful for the exact merged result",
		},
		{
			name:      "HEAD advances after refs are captured",
			condition: "7: base/HEAD non-advance revalidation",
			scenario: baseAdvanceScenario{
				afterRefsCaptured: func(t *testing.T, repo string) {
					gitSnapshot(t, repo, "commit", "--allow-empty", "-m", "advance head after refs captured")
				},
			},
			wantErrSub: "HEAD advanced during validation",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runBaseAdvanceScenario(t, tt.scenario)
			if result.err == nil {
				t.Fatalf("condition %s: deriveBaseAdvanceCompatibility() error = nil, want failure containing %q", tt.condition, tt.wantErrSub)
			}
			if !strings.Contains(result.err.Error(), tt.wantErrSub) {
				t.Fatalf("condition %s: error = %q, want substring %q", tt.condition, result.err.Error(), tt.wantErrSub)
			}
			if (result.proof != BaseAdvanceCompatibility{}) {
				t.Fatalf("condition %s: proof = %#v, want zero value on failure", tt.condition, result.proof)
			}
		})
	}
}
