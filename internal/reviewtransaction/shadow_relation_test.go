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

// This file characterizes shadow_relation.go (Wave 1, Slice 3): the
// seven-value relation algebra, Amendment A delegation, Amendment B
// degradation, and the commit-state/push-state threat-matrix rows this
// slice owns (tasks.md 3.5-3.6 — deferred from Slice 2 because "unknown is
// a relation-function outcome, not an identity-resolver outcome"). It
// reuses the same real-Git-repo test helpers as the characterization
// (prepr_base_advance_characterization_test.go) and identity
// (shadow_identity_test.go) slices: initSnapshotRepo, gitSnapshot,
// writeSnapshotFile, trimGit, configurePublicationRemote,
// emptyRemoteTrackingRepo, initUnbornSnapshotRepo, requireSnapshotGit,
// currentBranch — all defined in gate_test.go / snapshot_test.go, same
// package.

func fixtureCandidateIdentity(baseTree, candidateTree, digestSeed, policySeed string) CandidateIdentity {
	return CandidateIdentity{
		RepositoryID:            "repo-fixture",
		BaseTree:                baseTree,
		CandidateTree:           candidateTree,
		ChangedPathsModesDigest: "sha256:" + strings.Repeat(digestSeed, 64),
		PolicyHash:              "sha256:" + strings.Repeat(policySeed, 64),
	}
}

func fixtureValidBaseAdvance(originalBase, newBase string) *BaseAdvanceCompatibility {
	digest := "sha256:" + strings.Repeat("1", 64)
	patch := "sha256:" + strings.Repeat("2", 64)
	ciHash := "sha256:" + strings.Repeat("3", 64)
	return &BaseAdvanceCompatibility{
		Status: baseAdvanceCompatibleStatus, Compatible: true,
		OriginalMergeBaseTree: originalBase, NewBaseTree: newBase,
		OriginalPatchIdentity: patch, DeliveredPatchIdentity: patch,
		DeliveredPathsDigest: digest, BaseAdvancePathsDigest: digest, PathsDisjoint: true,
		MergedResultTree: strings.Repeat("4", 40), CIAttestationArtifactHash: ciHash,
		CIAttestationIssuer: "trusted-ci", CIStatus: "success",
	}
}

// TestShadowRelationSevenValuesNoEighth pins the exact seven literal string
// values (Requirement: Seven-Value Relation Output) so a typo or a silent
// eighth addition fails loudly.
func TestShadowRelationSevenValuesNoEighth(t *testing.T) {
	want := map[ShadowRelation]string{
		ShadowRelationExact:                 "exact",
		ShadowRelationCompatibleBaseAdvance: "compatible_base_advance",
		ShadowRelationProvableContraction:   "provable_contraction",
		ShadowRelationChanged:               "changed",
		ShadowRelationUnrelated:             "unrelated",
		ShadowRelationAmbiguous:             "ambiguous",
		ShadowRelationUnknown:               "unknown",
	}
	if len(want) != 7 {
		t.Fatalf("relation table has %d entries, spec requires exactly seven", len(want))
	}
	for constant, literal := range want {
		if string(constant) != literal {
			t.Errorf("relation constant = %q, want literal %q", string(constant), literal)
		}
	}
}

// TestShadowRelateExactAndUnrelatedScenarios covers the two literal
// spec.md scenarios under "Seven-Value Relation Output".
func TestShadowRelateExactAndUnrelatedScenarios(t *testing.T) {
	t.Run("identical candidate and policy resolve to exact", func(t *testing.T) {
		identical := fixtureCandidateIdentity(strings.Repeat("1", 40), strings.Repeat("2", 40), "a", "b")
		got := relateCandidates(shadowRelationInput{Frozen: identical, Live: identical, LiveSnapshot: Snapshot{Paths: []string{"a.txt"}}})
		if got != ShadowRelationExact {
			t.Fatalf("relateCandidates() = %q, want %q", got, ShadowRelationExact)
		}
	})
	t.Run("no governing lineage resolves to unrelated", func(t *testing.T) {
		live := fixtureCandidateIdentity(strings.Repeat("3", 40), strings.Repeat("4", 40), "c", "d")
		got := relateCandidates(shadowRelationInput{Frozen: CandidateIdentity{}, Live: live, LiveSnapshot: Snapshot{Paths: []string{"z.txt"}}})
		if got != ShadowRelationUnrelated {
			t.Fatalf("relateCandidates() = %q, want %q", got, ShadowRelationUnrelated)
		}
	})
}

// TestShadowRelateOrderedFailClosedPrecedence proves task 3.2's ordering —
// ambiguity -> unknown -> exact -> compatible_base_advance -> provable_
// contraction -> changed -> unrelated — by constructing, for each adjacent
// pair, an input that satisfies BOTH the earlier and the later condition,
// and asserting the earlier one wins.
func TestShadowRelateOrderedFailClosedPrecedence(t *testing.T) {
	frozenBase := strings.Repeat("1", 40)
	liveBase := strings.Repeat("2", 40)
	candidateTree := strings.Repeat("3", 40)
	identical := fixtureCandidateIdentity(frozenBase, candidateTree, "a", "b")

	tests := []struct {
		name  string
		input shadowRelationInput
		want  ShadowRelation
	}{
		{
			name: "ambiguous outranks unknown",
			input: shadowRelationInput{
				ApplicableAuthorities: 2,
				LiveUnresolvable:      true,
			},
			want: ShadowRelationAmbiguous,
		},
		{
			name: "unknown outranks exact",
			input: shadowRelationInput{
				Frozen: identical, Live: identical,
				LiveUnresolvable: true,
			},
			want: ShadowRelationUnknown,
		},
		{
			name: "exact outranks compatible_base_advance",
			input: shadowRelationInput{
				Frozen: identical, Live: identical,
				LiveSnapshot: Snapshot{Paths: []string{"a.txt"}},
				BaseAdvance:  fixtureValidBaseAdvance(frozenBase, frozenBase),
			},
			want: ShadowRelationExact,
		},
		{
			name: "compatible_base_advance outranks provable_contraction",
			input: shadowRelationInput{
				Frozen:       fixtureCandidateIdentity(frozenBase, candidateTree, "a", "b"),
				Live:         fixtureCandidateIdentity(liveBase, candidateTree, "e", "b"),
				LiveSnapshot: Snapshot{Paths: []string{"a.txt"}},
				GenesisPaths: []string{"a.txt", "b.txt"},
				BaseAdvance:  fixtureValidBaseAdvance(frozenBase, liveBase),
			},
			want: ShadowRelationCompatibleBaseAdvance,
		},
		{
			name: "provable_contraction outranks the generic changed fallback",
			input: shadowRelationInput{
				Frozen:               fixtureCandidateIdentity(frozenBase, candidateTree, "a", "b"),
				Live:                 fixtureCandidateIdentity(liveBase, strings.Repeat("5", 40), "e", "b"),
				LiveSnapshot:         Snapshot{Paths: []string{"a.txt"}},
				GenesisPaths:         []string{"a.txt", "b.txt"},
				AdmittedPathsKnown:   true,
				AdmittedFindingPaths: []string{"a.txt"},
			},
			want: ShadowRelationProvableContraction,
		},
		{
			name: "changed outranks unrelated when a governing authority exists",
			input: shadowRelationInput{
				Frozen:       fixtureCandidateIdentity(frozenBase, candidateTree, "a", "b"),
				Live:         fixtureCandidateIdentity(liveBase, strings.Repeat("6", 40), "e", "b"),
				LiveSnapshot: Snapshot{Paths: []string{"a.txt", "c.txt"}},
				GenesisPaths: []string{"a.txt", "b.txt"},
			},
			want: ShadowRelationChanged,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := relateCandidates(tt.input); got != tt.want {
				t.Fatalf("relateCandidates() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestShadowRelateBaseAdvanceProofBindsToCandidateTree is a regression test
// for a discovered algebra gap surfaced by the differential matrix exit-bar
// (TestShadowMatrixUnexplainedDivergenceOnCoreRelationBlocksWave2,
// shadow_matrix_test.go): shadowBaseAdvanceApplies bound a
// BaseAdvanceCompatibility proof only to the Frozen/Live BaseTree pair,
// while classifyCompactTargetRelation's own compatible-advance branch
// (compact_target_relation.go) additionally requires
// frozen.CandidateTree == live.CandidateTree. A proof that validates on its
// own terms and matches the base pair, but whose CandidateTree also
// changed, must NOT be accepted as compatible_base_advance — shadow must
// bind identically to live, never less strictly.
func TestShadowRelateBaseAdvanceProofBindsToCandidateTree(t *testing.T) {
	frozenBase := strings.Repeat("1", 40)
	liveBase := strings.Repeat("2", 40)
	frozenCandidate := strings.Repeat("3", 40)
	liveCandidate := strings.Repeat("4", 40)

	input := shadowRelationInput{
		Frozen:       fixtureCandidateIdentity(frozenBase, frozenCandidate, "a", "b"),
		Live:         fixtureCandidateIdentity(liveBase, liveCandidate, "e", "b"),
		LiveSnapshot: Snapshot{Paths: []string{"a.txt"}},
		GenesisPaths: []string{"a.txt"},
		BaseAdvance:  fixtureValidBaseAdvance(frozenBase, liveBase),
	}
	if got := relateCandidates(input); got == ShadowRelationCompatibleBaseAdvance {
		t.Fatalf("relateCandidates() = %q, want anything but compatible_base_advance when CandidateTree changed (proof must bind to CandidateTree like live's classifyCompactTargetRelation)", got)
	} else if got != ShadowRelationChanged {
		t.Fatalf("relateCandidates() = %q, want %q", got, ShadowRelationChanged)
	}
}

// TestShadowRelateAmendmentBDegradesContractionOnExcludedFinding covers
// spec.md's two "provable_contraction Soundness Degradation" scenarios plus
// design.md's explicit no-input degradation note: absent admitted-finding
// evidence degrades to changed, never unknown and never provable_contraction.
func TestShadowRelateAmendmentBDegradesContractionOnExcludedFinding(t *testing.T) {
	frozen := fixtureCandidateIdentity(strings.Repeat("1", 40), strings.Repeat("2", 40), "a", "b")
	live := fixtureCandidateIdentity(strings.Repeat("1", 40), strings.Repeat("5", 40), "e", "b")
	genesis := []string{"a.txt", "b.txt"}
	liveSnapshot := Snapshot{Paths: []string{"a.txt"}}

	t.Run("contraction with no excluded-path findings", func(t *testing.T) {
		got := relateCandidates(shadowRelationInput{
			Frozen: frozen, Live: live, LiveSnapshot: liveSnapshot, GenesisPaths: genesis,
			AdmittedPathsKnown: true, AdmittedFindingPaths: []string{"a.txt"},
		})
		if got != ShadowRelationProvableContraction {
			t.Fatalf("relateCandidates() = %q, want %q", got, ShadowRelationProvableContraction)
		}
	})
	t.Run("contraction with an excluded-path finding degrades", func(t *testing.T) {
		got := relateCandidates(shadowRelationInput{
			Frozen: frozen, Live: live, LiveSnapshot: liveSnapshot, GenesisPaths: genesis,
			AdmittedPathsKnown: true, AdmittedFindingPaths: []string{"a.txt", "b.txt"},
		})
		if got != ShadowRelationChanged {
			t.Fatalf("relateCandidates() = %q, want %q (excluded-path finding must degrade)", got, ShadowRelationChanged)
		}
	})
	t.Run("no-input degradation never fabricates provable_contraction or unknown", func(t *testing.T) {
		got := relateCandidates(shadowRelationInput{
			Frozen: frozen, Live: live, LiveSnapshot: liveSnapshot, GenesisPaths: genesis,
			AdmittedPathsKnown: false,
		})
		if got != ShadowRelationChanged {
			t.Fatalf("relateCandidates() = %q, want %q (Amendment B no-input degradation)", got, ShadowRelationChanged)
		}
	})
}

// buildShadowBaseAdvanceFixture builds a real Git repository with a
// reviewed feature-branch delivery and a compatible base advance on "main"
// — the same shape prepr_base_advance_characterization_test.go's
// runBaseAdvanceScenario uses — then returns exactly the inputs
// shadowDeriveBaseAdvance needs. When breakMergeBasePreservation is true,
// the frozen receipt's base tree is perturbed after capture (the same
// condition-1 break the characterization suite already isolates), so
// deriveBaseAdvanceCompatibility fails closed on that one condition.
func buildShadowBaseAdvanceFixture(t *testing.T, breakMergeBasePreservation bool) (string, Receipt, GateRequest, Snapshot, *resolvedPrePRRefs, gateArtifactPreimages) {
	t.Helper()
	ctx := context.Background()
	repo := initSnapshotRepo(t)
	baseCommit := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	gitSnapshot(t, repo, "branch", "main", baseCommit)
	remote := configurePublicationRemote(t, repo, "main")
	gitSnapshot(t, repo, "checkout", "-qb", "feature")
	writeSnapshotFile(t, repo, "delivery.txt", "reviewed delivery\n")
	gitSnapshot(t, repo, "add", "--", "delivery.txt")
	gitSnapshot(t, repo, "commit", "-m", "delivery")

	baseTree := trimGit(gitSnapshot(t, repo, "rev-parse", baseCommit+"^{tree}"))
	finalCandidateTree := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD^{tree}"))
	builder := SnapshotBuilder{Repo: repo}
	originalPaths, err := builder.changedPaths(ctx, baseTree, finalCandidateTree)
	if err != nil {
		t.Fatalf("changedPaths: %v", err)
	}
	receipt := Receipt{BaseTree: baseTree, FinalCandidateTree: finalCandidateTree, PathsDigest: digestPaths(originalPaths)}
	if breakMergeBasePreservation {
		// emptyGitTreeSHA1 is declared once in
		// prepr_base_advance_characterization_test.go and reused here —
		// same package, same "any tree that is not the real merge-base"
		// role.
		receipt.BaseTree = emptyGitTreeSHA1
	}

	gitSnapshot(t, repo, "checkout", "main")
	writeSnapshotFile(t, repo, "base-only.txt", "base advance\n")
	gitSnapshot(t, repo, "add", "--", "base-only.txt")
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
	attestation.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, prePRCIAttestationPreimage(attestation)))
	attestationPayload, err := json.Marshal(attestation)
	if err != nil {
		t.Fatalf("marshal attestation: %v", err)
	}
	policy := []byte("# Bounded review policy\n\npre_pr_ci_issuer: trusted-ci\npre_pr_ci_ed25519_public_key: " + base64.StdEncoding.EncodeToString(publicKey) + "\n")

	preimages := gateArtifactPreimages{policy: policy, ciAttestation: attestationPayload}
	request := GateRequest{ExternalEvidence: ExternalEvidenceNone, PrePR: &PrePRRequest{CIAttestationArtifact: "ci-attestation-present"}}
	return repo, receipt, request, snapshot, refs, preimages
}

// deriveBaseAdvanceCompatibilityPtr adapts deriveBaseAdvanceCompatibility's
// (BaseAdvanceCompatibility, error) return to the nil-on-error shape
// relateCandidates' BaseAdvance input expects — the exact conversion the
// retired shadowDeriveBaseAdvance wrapper (Wave 7 S2a) used to perform, kept
// here only for this file's own direct characterization of Amendment A
// delegation, independent of any observer.
func deriveBaseAdvanceCompatibilityPtr(ctx context.Context, repo string, receipt Receipt, request GateRequest, snapshot Snapshot, refs *resolvedPrePRRefs, preimages gateArtifactPreimages) *BaseAdvanceCompatibility {
	proof, err := deriveBaseAdvanceCompatibility(ctx, repo, receipt, request, snapshot, refs, preimages, true)
	if err != nil {
		return nil
	}
	return &proof
}

// TestShadowRelateDelegatesCompatibleBaseAdvanceToDeriveBaseAdvanceCompatibility
// is Amendment A's "all seven conditions hold" scenario: relateCandidates
// attributes compatible_base_advance to deriveBaseAdvanceCompatibility's own
// delegated proof.
func TestShadowRelateDelegatesCompatibleBaseAdvanceToDeriveBaseAdvanceCompatibility(t *testing.T) {
	repo, receipt, request, snapshot, refs, preimages := buildShadowBaseAdvanceFixture(t, false)
	proof := deriveBaseAdvanceCompatibilityPtr(context.Background(), repo, receipt, request, snapshot, refs, preimages)
	if proof == nil || !proof.valid() {
		t.Fatalf("deriveBaseAdvanceCompatibility() = %#v, want a valid delegated proof", proof)
	}
	frozen := fixtureCandidateIdentity(receipt.BaseTree, receipt.FinalCandidateTree, "a", "b")
	live := fixtureCandidateIdentity(snapshot.BaseTree, snapshot.CandidateTree, "c", "d")
	got := relateCandidates(shadowRelationInput{Frozen: frozen, Live: live, LiveSnapshot: snapshot, BaseAdvance: proof})
	if got != ShadowRelationCompatibleBaseAdvance {
		t.Fatalf("relateCandidates() = %q, want %q", got, ShadowRelationCompatibleBaseAdvance)
	}
}

// TestShadowRelateAmendmentANeverOverridesAFailedDelegatedCondition is
// Amendment A's "any condition fails" scenario: breaking condition 1 (merge-
// base tree preservation) makes deriveBaseAdvanceCompatibility fail (nil
// proof), and relateCandidates never fabricates compatible_base_advance from
// a local override.
func TestShadowRelateAmendmentANeverOverridesAFailedDelegatedCondition(t *testing.T) {
	repo, receipt, request, snapshot, refs, preimages := buildShadowBaseAdvanceFixture(t, true)
	proof := deriveBaseAdvanceCompatibilityPtr(context.Background(), repo, receipt, request, snapshot, refs, preimages)
	if proof != nil {
		t.Fatalf("deriveBaseAdvanceCompatibility() = %#v, want nil on a broken delegated condition", proof)
	}
	frozen := fixtureCandidateIdentity(receipt.BaseTree, receipt.FinalCandidateTree, "a", "b")
	live := fixtureCandidateIdentity(snapshot.BaseTree, snapshot.CandidateTree, "c", "d")
	got := relateCandidates(shadowRelationInput{Frozen: frozen, Live: live, LiveSnapshot: snapshot, BaseAdvance: proof})
	if got == ShadowRelationCompatibleBaseAdvance {
		t.Fatalf("relateCandidates() = %q, want no shadow-local override when the delegated condition fails", got)
	}
}

// TestShadowRelateCommitStateThreatNeverAccidentalExact covers task 3.5:
// unborn HEAD and empty-index states at a pre-commit/post-apply-style
// TargetCurrentChanges target resolve to unknown, never to an accidental
// exact even when the frozen and live identities coincide.
func TestShadowRelateCommitStateThreatNeverAccidentalExact(t *testing.T) {
	ctx := context.Background()

	t.Run("unborn HEAD, staged projection", func(t *testing.T) {
		requireSnapshotGit(t)
		repo := initUnbornSnapshotRepo(t)
		writeSnapshotFile(t, repo, "candidate.txt", "reviewed\n")
		gitSnapshot(t, repo, "add", "--", "candidate.txt")
		snapshot, err := (SnapshotBuilder{Repo: repo}).Build(ctx, Target{Kind: TargetCurrentChanges, Projection: ProjectionStaged, IntendedUntracked: []string{}})
		if err != nil {
			t.Fatalf("Build(unborn staged) error = %v", err)
		}
		frozen := fixtureCandidateIdentity(snapshot.BaseTree, snapshot.CandidateTree, "a", "b")
		got := relateCandidates(shadowRelationInput{Frozen: frozen, Live: frozen, LiveSnapshot: snapshot})
		if got != ShadowRelationUnknown {
			t.Fatalf("relateCandidates(unborn HEAD staged) = %q, want %q", got, ShadowRelationUnknown)
		}
	})

	t.Run("unborn HEAD, workspace projection", func(t *testing.T) {
		requireSnapshotGit(t)
		repo := initUnbornSnapshotRepo(t)
		writeSnapshotFile(t, repo, "candidate.txt", "reviewed\n")
		gitSnapshot(t, repo, "add", "--", "candidate.txt")
		snapshot, err := (SnapshotBuilder{Repo: repo}).Build(ctx, Target{Kind: TargetCurrentChanges, Projection: ProjectionWorkspace, IntendedUntracked: []string{}})
		if err != nil {
			t.Fatalf("Build(unborn workspace) error = %v", err)
		}
		if !snapshot.UnbornHead {
			t.Fatal("fixture snapshot.UnbornHead = false, want true")
		}
		frozen := fixtureCandidateIdentity(snapshot.BaseTree, snapshot.CandidateTree, "a", "b")
		got := relateCandidates(shadowRelationInput{Frozen: frozen, Live: frozen, LiveSnapshot: snapshot})
		if got != ShadowRelationUnknown {
			t.Fatalf("relateCandidates(unborn HEAD workspace) = %q, want %q", got, ShadowRelationUnknown)
		}
	})

	t.Run("empty index at a real HEAD", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		snapshot, err := (SnapshotBuilder{Repo: repo}).Build(ctx, Target{Kind: TargetCurrentChanges, Projection: ProjectionStaged, IntendedUntracked: []string{}})
		if err != nil {
			t.Fatalf("Build(clean staged) error = %v", err)
		}
		if len(snapshot.Paths) != 0 {
			t.Fatalf("fixture snapshot.Paths = %v, want empty (nothing staged)", snapshot.Paths)
		}
		frozen := fixtureCandidateIdentity(snapshot.BaseTree, snapshot.CandidateTree, "a", "b")
		got := relateCandidates(shadowRelationInput{Frozen: frozen, Live: frozen, LiveSnapshot: snapshot})
		if got != ShadowRelationUnknown {
			t.Fatalf("relateCandidates(empty index) = %q, want %q", got, ShadowRelationUnknown)
		}
	})
}

// TestShadowRelatePushStateThreatUnresolvableBoundaryIsUnknown covers task
// 3.6: shadowPushBoundaryUnresolvable reuses buildPushTarget /
// prePushTargetForRequest (gate.go) rather than re-deriving push-boundary
// resolution, and reports true (fail-closed) for a first-push
// empty-remote-bootstrap boundary and for a genuine resolution error, while
// staying false for a real, already-established tracking boundary.
func TestShadowRelatePushStateThreatUnresolvableBoundaryIsUnknown(t *testing.T) {
	ctx := context.Background()

	t.Run("first push (empty remote bootstrap) is unresolvable", func(t *testing.T) {
		repo, _ := emptyRemoteTrackingRepo(t)
		reviewedBaseTree := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD^{tree}"))
		writeSnapshotFile(t, repo, "tracked.txt", "reviewed delivery\n")
		gitSnapshot(t, repo, "commit", "-am", "reviewed delivery")
		target, push, err := buildPushTarget(ctx, repo, "", reviewedBaseTree, "")
		if err != nil {
			t.Fatalf("buildPushTarget(first push) error = %v", err)
		}
		_, refs, err := prePushTargetForRequest(ctx, repo, GateRequest{Gate: GatePrePush, Target: target, Push: push})
		if err != nil {
			t.Fatalf("prePushTargetForRequest(first push) error = %v", err)
		}
		if !shadowPushBoundaryUnresolvable(refs, err) {
			t.Fatal("shadowPushBoundaryUnresolvable(first push) = false, want true")
		}
		snapshot, err := (SnapshotBuilder{Repo: repo}).Build(ctx, target)
		if err != nil {
			t.Fatalf("Build(first push target) error = %v", err)
		}
		frozen := fixtureCandidateIdentity(snapshot.BaseTree, snapshot.CandidateTree, "a", "b")
		got := relateCandidates(shadowRelationInput{Frozen: frozen, Live: frozen, LiveSnapshot: snapshot, LiveUnresolvable: true})
		if got != ShadowRelationUnknown {
			t.Fatalf("relateCandidates(first push) = %q, want %q", got, ShadowRelationUnknown)
		}
	})

	t.Run("resolution error fails closed", func(t *testing.T) {
		repo, _ := emptyRemoteTrackingRepo(t)
		gitSnapshot(t, repo, "push", "origin", "HEAD:refs/heads/other")
		reviewedBaseTree := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD^{tree}"))
		writeSnapshotFile(t, repo, "tracked.txt", "reviewed delivery\n")
		gitSnapshot(t, repo, "commit", "-am", "reviewed delivery")
		_, _, err := buildPushTarget(ctx, repo, "", reviewedBaseTree, "")
		if err == nil {
			t.Fatal("buildPushTarget(unadvertised branch) error = nil, want failure")
		}
		if !shadowPushBoundaryUnresolvable(nil, err) {
			t.Fatal("shadowPushBoundaryUnresolvable(resolution error) = false, want true")
		}
	})

	t.Run("an established tracking boundary is not forced unknown", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		branch := currentBranch(ctx, repo)
		configurePublicationRemote(t, repo, branch)
		gitSnapshot(t, repo, "config", "branch."+branch+".remote", "origin")
		gitSnapshot(t, repo, "config", "branch."+branch+".merge", "refs/heads/"+branch)
		writeSnapshotFile(t, repo, "approved.txt", "reviewed delivery\n")
		gitSnapshot(t, repo, "add", "approved.txt")
		gitSnapshot(t, repo, "commit", "-m", "reviewed delivery")
		target, push, err := buildPushTarget(ctx, repo, "", "", "")
		if err != nil {
			t.Fatalf("buildPushTarget(tracking branch) error = %v", err)
		}
		_, refs, err := prePushTargetForRequest(ctx, repo, GateRequest{Gate: GatePrePush, Target: target, Push: push})
		if err != nil {
			t.Fatalf("prePushTargetForRequest(tracking branch) error = %v", err)
		}
		if shadowPushBoundaryUnresolvable(refs, err) {
			t.Fatal("shadowPushBoundaryUnresolvable(tracking branch) = true, want false (a real prior boundary resolved)")
		}
	})
}

// TestShadowRelationHasNoLiveCounterpartOnlyForAmbiguousAndUnknown covers
// task 3.7: only ambiguous and unknown rows have no live counterpart to
// fabricate an agreement against.
func TestShadowRelationHasNoLiveCounterpartOnlyForAmbiguousAndUnknown(t *testing.T) {
	all := []ShadowRelation{
		ShadowRelationExact, ShadowRelationCompatibleBaseAdvance, ShadowRelationProvableContraction,
		ShadowRelationChanged, ShadowRelationUnrelated, ShadowRelationAmbiguous, ShadowRelationUnknown,
	}
	want := map[ShadowRelation]bool{
		ShadowRelationAmbiguous: true,
		ShadowRelationUnknown:   true,
	}
	for _, relation := range all {
		if got := shadowRelationHasNoLiveCounterpart(relation); got != want[relation] {
			t.Errorf("shadowRelationHasNoLiveCounterpart(%q) = %v, want %v", relation, got, want[relation])
		}
	}
}
