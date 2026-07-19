package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCompactPrePRChainAllowsExactThreeReceiptComposition(t *testing.T) {
	fixture := newCompactPrePRChainFixture(t, 3)

	got, attempted := EvaluateCompactPrePRChain(context.Background(), fixture.repo, fixture.input())

	if !attempted || got.Result != GateAllow {
		t.Fatalf("three-receipt composition = %#v, attempted %t", got, attempted)
	}
	if got.Context.CandidateTree != fixture.receipts[2].FinalCandidateTree || got.Context.BaseTree != fixture.receipts[0].BaseTree || got.Context.ChainIdentity == "" {
		t.Fatalf("composed proof context = %#v", got.Context)
	}
}

func TestCompactPrePRChainLeavesExactSingleReceiptToDirectEvaluation(t *testing.T) {
	fixture := newCompactPrePRChainFixture(t, 1)
	input := fixture.input()
	input.LineageID = fixture.states[0].LineageID

	direct := EvaluateCompactGate(context.Background(), fixture.repo, fixture.receipts[0], input)
	composed, attempted := EvaluateCompactPrePRChain(context.Background(), fixture.repo, fixture.input())

	if direct.Result != GateAllow {
		t.Fatalf("direct exact receipt = %#v", direct)
	}
	if attempted || composed.Result == GateAllow {
		t.Fatalf("single receipt entered composition = %#v, attempted %t", composed, attempted)
	}
}

func TestCompactPrePRChainRejectsInvalidMembersAndBindings(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, fixture *compactPrePRChainFixture)
	}{
		{
			name: "nonterminal member",
			mutate: func(t *testing.T, fixture *compactPrePRChainFixture) {
				replaceCompactChainState(t, fixture.stores[1], func(state *CompactState) {
					state.State = StateReviewing
					state.LensResults = []LensResult{}
					state.Findings = []Finding{}
					state.Classifications = map[string]FindingEvidence{}
					state.Outcomes = map[string]EvidenceOutcome{}
					state.FixFindingIDs = []string{}
					state.FollowUps = []FollowUp{}
					state.EvidenceHash = ""
				})
			},
		},
		{
			name: "missing receipt",
			mutate: func(t *testing.T, fixture *compactPrePRChainFixture) {
				if err := os.Remove(fixture.stores[1].ReceiptPath()); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "tampered receipt",
			mutate: func(t *testing.T, fixture *compactPrePRChainFixture) {
				payload, err := os.ReadFile(fixture.stores[1].ReceiptPath())
				if err != nil {
					t.Fatal(err)
				}
				payload = bytes.Replace(payload, []byte(fixture.receipts[1].PolicyHash), []byte(hash("a")), 1)
				if err := os.WriteFile(fixture.stores[1].ReceiptPath(), payload, 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "tampered state",
			mutate: func(t *testing.T, fixture *compactPrePRChainFixture) {
				payload, err := os.ReadFile(fixture.stores[1].StatePath())
				if err != nil {
					t.Fatal(err)
				}
				payload = bytes.Replace(payload, []byte(fixture.states[1].PolicyHash), []byte(hash("b")), 1)
				if err := os.WriteFile(fixture.stores[1].StatePath(), payload, 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "policy substitution",
			mutate: func(t *testing.T, fixture *compactPrePRChainFixture) {
				replaceCompactChainReceipt(t, fixture.stores[1], func(receipt *CompactReceipt) { receipt.PolicyHash = hash("c") })
			},
		},
		{
			name: "evidence substitution",
			mutate: func(t *testing.T, fixture *compactPrePRChainFixture) {
				replaceCompactChainReceipt(t, fixture.stores[1], func(receipt *CompactReceipt) { receipt.EvidenceHash = hash("d") })
			},
		},
		{
			name: "fix substitution",
			mutate: func(t *testing.T, fixture *compactPrePRChainFixture) {
				replaceCompactChainReceipt(t, fixture.stores[1], func(receipt *CompactReceipt) { receipt.FixDeltaHash = hash("e") })
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newCompactPrePRChainFixture(t, 3)
			tt.mutate(t, fixture)

			got, attempted := EvaluateCompactPrePRChain(context.Background(), fixture.repo, fixture.input())

			if !attempted || got.Result == GateAllow {
				t.Fatalf("invalid member authorized = %#v, attempted %t", got, attempted)
			}
		})
	}
}

func TestCompactPrePRChainRejectsGapFinalMismatchAndReorderedAuthority(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, fixture *compactPrePRChainFixture)
	}{
		{
			name: "gap",
			mutate: func(t *testing.T, fixture *compactPrePRChainFixture) {
				if err := os.RemoveAll(fixture.stores[1].Dir); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "final tree mismatch",
			mutate: func(t *testing.T, fixture *compactPrePRChainFixture) {
				writeSnapshotFile(t, fixture.repo, "unreviewed.txt", "unreviewed final tree\n")
				gitSnapshot(t, fixture.repo, "add", "unreviewed.txt")
				gitSnapshot(t, fixture.repo, "commit", "-m", "unreviewed final commit")
			},
		},
		{
			name: "reordered member",
			mutate: func(t *testing.T, fixture *compactPrePRChainFixture) {
				first, second := fixture.stores[0].ReceiptPath(), fixture.stores[1].ReceiptPath()
				firstPayload, err := os.ReadFile(first)
				if err != nil {
					t.Fatal(err)
				}
				secondPayload, err := os.ReadFile(second)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(first, secondPayload, 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(second, firstPayload, 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newCompactPrePRChainFixture(t, 3)
			tt.mutate(t, fixture)

			got, attempted := EvaluateCompactPrePRChain(context.Background(), fixture.repo, fixture.input())

			if !attempted || got.Result == GateAllow {
				t.Fatalf("broken chain authorized = %#v, attempted %t", got, attempted)
			}
		})
	}
}

func TestCompactPrePRChainRejectsSegmentAndPublicationPathsOutsideGenesis(t *testing.T) {
	fixture := newCompactPrePRChainFixtureWithHiddenPath(t)

	got, attempted := EvaluateCompactPrePRChain(context.Background(), fixture.repo, fixture.input())

	if !attempted || got.Result == GateAllow {
		t.Fatalf("touched-then-reverted path authorized = %#v, attempted %t", got, attempted)
	}
}

func TestCompactPrePRChainRejectsTransientTreeWithinAuthorizedPath(t *testing.T) {
	fixture := newCompactPrePRChainFixture(t, 0)
	reviewed := "reviewed boundary\n"
	fixture.addApprovedSegmentWithPolicy(t, 0, false, hash("1"), compactPrePRSegmentOptions{
		content: reviewed,
		beforeBoundary: func(path string) {
			if err := os.WriteFile(path, []byte("transient secret\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			gitSnapshot(t, fixture.repo, "add", "-A")
			gitSnapshot(t, fixture.repo, "commit", "-m", "transient secret")
			if err := os.WriteFile(path, []byte(reviewed), 0o644); err != nil {
				t.Fatal(err)
			}
		},
	})
	fixture.addApprovedSegment(t, 1, false)

	got, attempted := EvaluateCompactPrePRChain(context.Background(), fixture.repo, fixture.input())
	if !attempted || got.Result == GateAllow {
		t.Fatalf("transient tree within genesis path = %#v, attempted %t", got, attempted)
	}
}

func TestCompactPrePRChainClassifiesCompleteAggregateRisk(t *testing.T) {
	for _, tt := range []struct {
		name        string
		memberLines int
		wantAllow   bool
	}{
		{name: "two 400-line low-risk receipts", memberLines: 400},
		{name: "two 200-line low-risk receipts", memberLines: 200, wantAllow: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newCompactPrePRChainFixture(t, 0)
			for index := 0; index < 2; index++ {
				fixture.addApprovedSegmentWithPolicy(t, index, false, hash("1"), compactPrePRSegmentOptions{
					logicalPath: "segment-" + string(rune('a'+index)) + ".md",
					content:     strings.Repeat("documentation\n", tt.memberLines),
				})
				state := fixture.states[index]
				if state.RiskLevel != RiskLow || state.OriginalChangedLines != tt.memberLines {
					t.Fatalf("member risk = %s, lines = %d", state.RiskLevel, state.OriginalChangedLines)
				}
			}

			got, attempted := EvaluateCompactPrePRChain(context.Background(), fixture.repo, fixture.input())
			if !attempted || (got.Result == GateAllow) != tt.wantAllow {
				t.Fatalf("aggregate risk result = %#v, attempted %t", got, attempted)
			}
			if !tt.wantAllow && !strings.Contains(got.Reason, "full-target review") {
				t.Fatalf("high aggregate risk reason = %q", got.Reason)
			}
		})
	}
}

func TestCompactPrePRChainAllowsEmptyPublicationCommits(t *testing.T) {
	fixture := newCompactPrePRChainFixture(t, 0)
	fixture.addApprovedSegment(t, 0, false)
	gitSnapshot(t, fixture.repo, "commit", "--allow-empty", "-m", "empty between boundaries")
	fixture.addApprovedSegment(t, 1, false)
	gitSnapshot(t, fixture.repo, "commit", "--allow-empty", "-m", "empty after final boundary")

	got, attempted := EvaluateCompactPrePRChain(context.Background(), fixture.repo, fixture.input())
	if !attempted || got.Result != GateAllow {
		t.Fatalf("empty publication commits = %#v, attempted %t", got, attempted)
	}
}

func TestCompactPrePRChainRejectsEscalatedAndSupersededMembers(t *testing.T) {
	t.Run("escalated member", func(t *testing.T) {
		fixture := newCompactPrePRChainFixture(t, 3)
		record, err := fixture.stores[1].Load()
		if err != nil {
			t.Fatal(err)
		}
		record.State.State = StateEscalated
		_, payload, err := makeCompactRecord(record.State)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fixture.stores[1].StatePath(), payload, 0o644); err != nil {
			t.Fatal(err)
		}
		receipt, err := record.State.Receipt()
		if err != nil {
			t.Fatal(err)
		}
		writeTestCompactReceipt(t, fixture.stores[1].ReceiptPath(), receipt)

		got, attempted := EvaluateCompactPrePRChain(context.Background(), fixture.repo, fixture.input())
		if !attempted || got.Result == GateAllow {
			t.Fatalf("escalated member authorized = %#v, attempted %t", got, attempted)
		}
	})

	t.Run("superseded member", func(t *testing.T) {
		fixture := newCompactPrePRChainFixture(t, 3)
		successor := recoverCompactChainMember(t, fixture, 1)
		defer os.Remove(filepath.Join(fixture.repo, "recovery-expansion.txt"))

		got, attempted := EvaluateCompactPrePRChain(context.Background(), fixture.repo, fixture.input())
		if !attempted || got.Result == GateAllow {
			t.Fatalf("superseded member authorized = %#v, attempted %t, successor %s", got, attempted, successor.State.LineageID)
		}
	})

	t.Run("invalid recovery ancestry", func(t *testing.T) {
		fixture := newCompactPrePRChainFixture(t, 3)
		successor := recoverCompactChainMember(t, fixture, 1)
		store, err := CompactAuthoritativeStore(context.Background(), fixture.repo, successor.State.LineageID)
		if err != nil {
			t.Fatal(err)
		}
		successor.State.Recovery.PredecessorRevision = hash("9")
		_, payload, err := makeCompactRecord(successor.State)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(store.StatePath(), payload, 0o644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(filepath.Join(fixture.repo, "recovery-expansion.txt"))

		got, attempted := EvaluateCompactPrePRChain(context.Background(), fixture.repo, fixture.input())
		if !attempted || got.Result == GateAllow {
			t.Fatalf("invalid recovery ancestry authorized = %#v, attempted %t", got, attempted)
		}
	})
}

func TestCompactPrePRChainRejectsForkConvergenceAndCycle(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, fixture *compactPrePRChainFixture)
	}{
		{name: "fork", mutate: addCompactChainFork},
		{name: "convergence", mutate: addCompactChainConvergence},
		{name: "cycle", mutate: addCompactChainCycle},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newCompactPrePRChainFixture(t, 3)
			tt.mutate(t, fixture)

			got, attempted := EvaluateCompactPrePRChain(context.Background(), fixture.repo, fixture.input())
			if !attempted || got.Result == GateAllow {
				t.Fatalf("ambiguous graph authorized = %#v, attempted %t", got, attempted)
			}
		})
	}
}

func TestCompactPrePRChainRejectsMultipleViableChains(t *testing.T) {
	fixture := newCompactPrePRChainFixture(t, 3)
	for index, state := range fixture.states {
		cloneApprovedCompactChainState(t, fixture.repo, state, "compact-chain-duplicate-"+string(rune('a'+index)))
	}

	got, attempted := EvaluateCompactPrePRChain(context.Background(), fixture.repo, fixture.input())
	if !attempted || got.Result == GateAllow || !strings.Contains(got.Reason, "multiple viable") {
		t.Fatalf("multiple viable chains = %#v, attempted %t", got, attempted)
	}
}

func TestCompactPrePRChainRejectsIncompatibleSelectedBase(t *testing.T) {
	fixture := newCompactPrePRChainFixture(t, 3)
	advanceCompactChainRemote(t, fixture, "segment-a.txt")

	got, attempted := EvaluateCompactPrePRChain(context.Background(), fixture.repo, fixture.input())
	if !attempted || got.Result == GateAllow {
		t.Fatalf("incompatible selected base = %#v, attempted %t", got, attempted)
	}
}

func TestCompactPrePRChainRequiresSignedCompatibleBaseAdvance(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	policyPayload := []byte("pre_pr_ci_issuer: trusted-ci\npre_pr_ci_ed25519_public_key: " + base64.StdEncoding.EncodeToString(publicKey) + "\n")
	policyPath := filepath.Join(dir, "policy.md")
	if err := os.WriteFile(policyPath, policyPayload, 0o644); err != nil {
		t.Fatal(err)
	}
	fixture := newCompactPrePRChainFixtureWithPolicy(t, 3, hashArtifactPayload(policyPayload))
	newBase := advanceCompactChainRemote(t, fixture, "base-only.txt")

	unsigned, attempted := EvaluateCompactPrePRChain(context.Background(), fixture.repo, fixture.input())
	if !attempted || unsigned.Result == GateAllow {
		t.Fatalf("unsigned compatible base advance = %#v, attempted %t", unsigned, attempted)
	}

	mergedOutput, err := runGit(context.Background(), fixture.repo, nil, nil, "merge-tree", "--write-tree", newBase, fixture.commits[len(fixture.commits)-1])
	if err != nil {
		t.Fatal(err)
	}
	attestation := prePRCIAttestation{
		Schema: prePRCIAttestationSchema, Issuer: "trusted-ci", MergedTree: strings.Fields(string(mergedOutput))[0], Status: "success",
	}
	attestation.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, prePRCIAttestationPreimage(attestation)))
	attestationPayload, err := json.Marshal(attestation)
	if err != nil {
		t.Fatal(err)
	}
	attestationPath := filepath.Join(dir, "attestation.json")
	if err := os.WriteFile(attestationPath, attestationPayload, 0o644); err != nil {
		t.Fatal(err)
	}
	input := fixture.input()
	input.PolicyArtifact = policyPath
	input.PrePRCIAttestation = attestationPath

	signed, attempted := EvaluateCompactPrePRChain(context.Background(), fixture.repo, input)
	if !attempted || signed.Result != GateAllow || signed.Context.BaseAdvance == nil || !signed.Context.BaseAdvance.Compatible {
		t.Fatalf("signed compatible base advance = %#v, attempted %t", signed, attempted)
	}
}

func TestCompactPrePRChainIgnoresUnrelatedLegacyAuthority(t *testing.T) {
	fixture := newCompactPrePRChainFixture(t, 3)
	writeSnapshotFile(t, fixture.repo, "legacy.txt", "legacy authority\n")
	transaction, _, _ := nativeGateFixture(t, fixture.repo, "mixed-legacy-authority")
	store, err := AuthoritativeStore(context.Background(), fixture.repo, transaction.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	appendApprovedStoreChain(t, store, transaction)
	receipt, err := transaction.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteReceiptAtomic(filepath.Join(store.Dir, "artifacts", "receipt.json"), receipt); err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, fixture.repo, "reset", "--hard", "HEAD")

	got, attempted := EvaluateCompactPrePRChain(context.Background(), fixture.repo, fixture.input())
	if !attempted || got.Result != GateAllow {
		t.Fatalf("unrelated legacy/compact authority = %#v, attempted %t", got, attempted)
	}
}

func TestCompactPrePRChainRejectsApplicableLegacyAuthority(t *testing.T) {
	fixture := newCompactPrePRChainFixture(t, 3)
	persistApplicableLegacyAuthority(t, fixture, "applicable-legacy-authority")
	got, attempted := EvaluateCompactPrePRChain(context.Background(), fixture.repo, fixture.input())
	if !attempted || got.Result == GateAllow || !strings.Contains(got.Reason, "legacy-v1") {
		t.Fatalf("applicable legacy/compact authority = %#v, attempted %t", got, attempted)
	}
}

func persistApplicableLegacyAuthority(t *testing.T, fixture *compactPrePRChainFixture, lineage string) {
	t.Helper()
	snapshot, err := (SnapshotBuilder{Repo: fixture.repo}).Build(context.Background(), Target{Kind: TargetBaseDiff, BaseRef: fixture.base, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := NewTransaction(Start{LineageID: lineage, Mode: ModeOrdinary4R, Generation: 1, Snapshot: snapshot, PolicyHash: hash("1")})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	ledger, _ := CanonicalLedger([]Finding{})
	if err := tx.FreezeFindings([]Finding{}, ledger, hashArtifactPayload(ledger)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ClassifyEvidence([]FindingEvidence{}); err != nil {
		t.Fatal(err)
	}
	if err := tx.BeginFinalVerification(); err != nil {
		t.Fatal(err)
	}
	if err := tx.CompleteFinalVerification(hash("3"), true); err != nil {
		t.Fatal(err)
	}
	store, _ := AuthoritativeStore(context.Background(), fixture.repo, lineage)
	appendApprovedStoreChain(t, store, *tx)
	receipt, _ := tx.Receipt()
	if err := WriteReceiptAtomic(filepath.Join(store.Dir, "artifacts", "receipt.json"), receipt); err != nil {
		t.Fatal(err)
	}
}

func TestCompactPrePRChainRejectsMissingGitObjectDuringFinalAuthorization(t *testing.T) {
	fixture := newCompactPrePRChainFixture(t, 3)
	tree := fixture.receipts[len(fixture.receipts)-1].FinalCandidateTree
	objectPath := filepath.Join(fixture.repo, ".git", "objects", tree[:2], tree[2:])
	payload, err := os.ReadFile(objectPath)
	if err != nil {
		t.Fatal(err)
	}
	originalHook := finalGateAuthorizationHook
	finalGateAuthorizationHook = func() {
		finalGateAuthorizationHook = originalHook
		if err := os.Remove(objectPath); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		finalGateAuthorizationHook = originalHook
		if err := os.WriteFile(objectPath, payload, 0o444); err != nil {
			t.Fatal(err)
		}
	})

	got, attempted := EvaluateCompactPrePRChain(context.Background(), fixture.repo, fixture.input())
	if !attempted || got.Result == GateAllow || !strings.Contains(got.Reason, "final authorization") {
		t.Fatalf("missing Git object = %#v, attempted %t", got, attempted)
	}
}

func TestCompactPrePRChainFinalAuthorizationRejectsMutations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, fixture *compactPrePRChainFixture)
	}{
		{
			name: "HEAD",
			mutate: func(t *testing.T, fixture *compactPrePRChainFixture) {
				gitSnapshot(t, fixture.repo, "commit", "--allow-empty", "-m", "move HEAD during authorization")
			},
		},
		{
			name: "selected base",
			mutate: func(t *testing.T, fixture *compactPrePRChainFixture) {
				gitSnapshot(t, fixture.repo, "push", "origin", fixture.commits[0]+":refs/heads/"+fixture.branch)
			},
		},
		{
			name: "remote identity",
			mutate: func(t *testing.T, fixture *compactPrePRChainFixture) {
				other := filepath.Join(t.TempDir(), "other.git")
				gitSnapshot(t, fixture.repo, "init", "--bare", other)
				gitSnapshot(t, fixture.repo, "remote", "set-url", "origin", other)
			},
		},
		{
			name: "authority",
			mutate: func(t *testing.T, fixture *compactPrePRChainFixture) {
				payload, err := os.ReadFile(fixture.stores[1].ReceiptPath())
				if err != nil {
					t.Fatal(err)
				}
				payload = bytes.Replace(payload, []byte(fixture.receipts[1].EvidenceHash), []byte(hash("f")), 1)
				if err := os.WriteFile(fixture.stores[1].ReceiptPath(), payload, 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newCompactPrePRChainFixture(t, 3)
			originalHook := finalGateAuthorizationHook
			finalGateAuthorizationHook = func() {
				finalGateAuthorizationHook = originalHook
				tt.mutate(t, fixture)
			}
			t.Cleanup(func() { finalGateAuthorizationHook = originalHook })

			got, attempted := EvaluateCompactPrePRChain(context.Background(), fixture.repo, fixture.input())

			if !attempted || got.Result == GateAllow || !strings.Contains(got.Reason, "final authorization") {
				t.Fatalf("concurrent %s mutation = %#v, attempted %t", tt.name, got, attempted)
			}
		})
	}
}

func TestCompactPrePRChainValidationLeavesRepositoryAndAuthorityBytesUnchanged(t *testing.T) {
	fixture := newCompactPrePRChainFixture(t, 3)
	beforeStatus := gitSnapshot(t, fixture.repo, "status", "--porcelain=v1")
	before := compactChainAuthorityBytes(t, fixture.stores)

	first, attempted := EvaluateCompactPrePRChain(context.Background(), fixture.repo, fixture.input())
	second, replayed := EvaluateCompactPrePRChain(context.Background(), fixture.repo, fixture.input())

	if !attempted || !replayed || first.Result != GateAllow || !reflect.DeepEqual(first, second) {
		t.Fatalf("deterministic composition = first %#v, second %#v", first, second)
	}
	if afterStatus := gitSnapshot(t, fixture.repo, "status", "--porcelain=v1"); afterStatus != beforeStatus {
		t.Fatalf("repository status changed: before %q, after %q", beforeStatus, afterStatus)
	}
	if after := compactChainAuthorityBytes(t, fixture.stores); !reflect.DeepEqual(after, before) {
		t.Fatal("composition changed authority or receipt bytes")
	}
}

func TestCompactStoresShareRepositoryWriteLock(t *testing.T) {
	fixture := newCompactPrePRChainFixture(t, 2)
	held, err := acquireStoreLock(fixture.stores[0].lockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer held.release()
	if _, err := acquireStoreLock(fixture.stores[1].lockPath); !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("second compact store lock error = %v, want concurrent update", err)
	}
}

type compactPrePRChainFixture struct {
	repo     string
	remote   string
	branch   string
	base     string
	commits  []string
	states   []CompactState
	stores   []CompactStore
	receipts []CompactReceipt
}

type compactPrePRSegmentOptions struct {
	logicalPath    string
	content        string
	beforeBoundary func(path string)
}

func newCompactPrePRChainFixture(t *testing.T, count int) *compactPrePRChainFixture {
	return newCompactPrePRChainFixtureWithPolicy(t, count, hash("1"))
}

func newCompactPrePRChainFixtureWithPolicy(t *testing.T, count int, policyHash string) *compactPrePRChainFixture {
	t.Helper()
	repo := initSnapshotRepo(t)
	branch := currentBranch(context.Background(), repo)
	fixture := &compactPrePRChainFixture{
		repo: repo, branch: branch, base: trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD")),
		commits: []string{}, states: []CompactState{}, stores: []CompactStore{}, receipts: []CompactReceipt{},
	}
	fixture.remote = configurePublicationRemote(t, repo, branch)
	for index := 0; index < count; index++ {
		fixture.addApprovedSegmentWithPolicy(t, index, false, policyHash)
	}
	return fixture
}

func newCompactPrePRChainFixtureWithHiddenPath(t *testing.T) *compactPrePRChainFixture {
	t.Helper()
	fixture := newCompactPrePRChainFixture(t, 0)
	fixture.addApprovedSegment(t, 0, true)
	fixture.addApprovedSegment(t, 1, false)
	fixture.addApprovedSegment(t, 2, false)
	return fixture
}

func (fixture *compactPrePRChainFixture) addApprovedSegment(t *testing.T, index int, hiddenPath bool) {
	fixture.addApprovedSegmentWithPolicy(t, index, hiddenPath, hash("1"))
}

func (fixture *compactPrePRChainFixture) addApprovedSegmentWithPolicy(t *testing.T, index int, hiddenPath bool, policyHash string, overrides ...compactPrePRSegmentOptions) {
	t.Helper()
	options := compactPrePRSegmentOptions{logicalPath: "segment-" + string(rune('a'+index)) + ".txt", content: "reviewed segment\n"}
	if len(overrides) > 1 {
		t.Fatal("compact segment accepts at most one options value")
	}
	if len(overrides) == 1 {
		if overrides[0].logicalPath != "" {
			options.logicalPath = overrides[0].logicalPath
		}
		if overrides[0].content != "" {
			options.content = overrides[0].content
		}
		options.beforeBoundary = overrides[0].beforeBoundary
	}
	path := filepath.Join(fixture.repo, options.logicalPath)
	if err := os.WriteFile(path, []byte(options.content), 0o644); err != nil {
		t.Fatal(err)
	}
	lineage := "compact-chain-segment-" + string(rune('a'+index))
	state := newCompactStartStateForTarget(t, fixture.repo, lineage, Target{
		Kind: TargetCurrentChanges, IntendedUntracked: []string{filepath.Base(path)},
	})
	state.PolicyHash = policyHash
	state, receipt := persistApprovedCompactState(t, fixture.repo, state)
	store, err := CompactAuthoritativeStore(context.Background(), fixture.repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	fixture.states = append(fixture.states, state)
	fixture.stores = append(fixture.stores, store)
	fixture.receipts = append(fixture.receipts, receipt)
	if hiddenPath {
		writeSnapshotFile(t, fixture.repo, "hidden.txt", "must not be published\n")
		gitSnapshot(t, fixture.repo, "add", "-A")
		gitSnapshot(t, fixture.repo, "commit", "-m", "touch hidden path")
		if err := os.Remove(filepath.Join(fixture.repo, "hidden.txt")); err != nil {
			t.Fatal(err)
		}
	}
	if options.beforeBoundary != nil {
		options.beforeBoundary(path)
	}
	gitSnapshot(t, fixture.repo, "add", "-A")
	gitSnapshot(t, fixture.repo, "commit", "-m", "deliver reviewed segment")
	fixture.commits = append(fixture.commits, trimGit(gitSnapshot(t, fixture.repo, "rev-parse", "HEAD")))
}

func recoverCompactChainMember(t *testing.T, fixture *compactPrePRChainFixture, index int) CompactRecord {
	t.Helper()
	writeSnapshotFile(t, fixture.repo, "recovery-expansion.txt", "expanded recovery scope\n")
	snapshot, err := (SnapshotBuilder{Repo: fixture.repo}).Build(context.Background(), Target{
		Kind: TargetCurrentChanges, IntendedUntracked: []string{"recovery-expansion.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	risk, lines, err := (SnapshotBuilder{Repo: fixture.repo}).ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	lenses := []string{}
	if risk == RiskMedium {
		lenses = []string{LensReliability}
	} else if risk == RiskHigh {
		lenses = append([]string(nil), supportedLenses...)
	}
	successor, err := NewCompactState(Start{
		LineageID: "compact-chain-recovery", Mode: ModeOrdinaryBounded,
		Generation: fixture.states[index].Generation + 1, Snapshot: snapshot,
		PolicyHash: fixture.states[index].PolicyHash, RiskLevel: risk,
		SelectedLenses: lenses, OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	predecessor, err := fixture.stores[index].Load()
	if err != nil {
		t.Fatal(err)
	}
	record, err := RecoverCompactAuthority(context.Background(), fixture.repo, CompactRecoveryRequest{
		PredecessorLineageID: predecessor.State.LineageID, ExpectedPredecessorRevision: predecessor.Revision,
		Successor: successor, Disposition: RecoveryScopeChanged, Reason: "test scope expansion", Actor: "maintainer",
	})
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func cloneApprovedCompactChainState(t *testing.T, repo string, source CompactState, lineage string) {
	t.Helper()
	clone := source
	clone.LineageID = lineage
	store, err := CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	_, payload, err := makeCompactRecord(clone)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(store.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	receipt, err := clone.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
}

func addCompactChainFork(t *testing.T, fixture *compactPrePRChainFixture) {
	t.Helper()
	head := trimGit(gitSnapshot(t, fixture.repo, "rev-parse", "HEAD"))
	gitSnapshot(t, fixture.repo, "reset", "--hard", fixture.base)
	writeSnapshotFile(t, fixture.repo, "alternate.txt", "alternate branch\n")
	state := newCompactStartStateForTarget(t, fixture.repo, "compact-chain-fork", Target{
		Kind: TargetCurrentChanges, IntendedUntracked: []string{"alternate.txt"},
	})
	persistApprovedCompactState(t, fixture.repo, state)
	gitSnapshot(t, fixture.repo, "reset", "--hard", head)
}

func addCompactChainConvergence(t *testing.T, fixture *compactPrePRChainFixture) {
	t.Helper()
	state := newCompactStartStateForTarget(t, fixture.repo, "compact-chain-convergence", Target{
		Kind: TargetBaseDiff, BaseRef: fixture.commits[0], IntendedUntracked: []string{},
	})
	persistApprovedCompactState(t, fixture.repo, state)
}

func addCompactChainCycle(t *testing.T, fixture *compactPrePRChainFixture) {
	t.Helper()
	for index := range fixture.receipts {
		if err := os.Remove(filepath.Join(fixture.repo, "segment-"+string(rune('a'+index))+".txt")); err != nil {
			t.Fatal(err)
		}
	}
	state := newCompactStartStateForTarget(t, fixture.repo, "compact-chain-cycle", Target{
		Kind: TargetCurrentChanges, IntendedUntracked: []string{},
	})
	persistApprovedCompactState(t, fixture.repo, state)
	gitSnapshot(t, fixture.repo, "reset", "--hard", "HEAD")
}

func advanceCompactChainRemote(t *testing.T, fixture *compactPrePRChainFixture, path string) string {
	t.Helper()
	clone := filepath.Join(t.TempDir(), "base-clone")
	gitSnapshot(t, fixture.repo, "clone", fixture.remote, clone)
	gitSnapshot(t, clone, "config", "user.email", "test@example.com")
	gitSnapshot(t, clone, "config", "user.name", "Test User")
	writeSnapshotFile(t, clone, path, "advanced base\n")
	gitSnapshot(t, clone, "add", path)
	gitSnapshot(t, clone, "commit", "-m", "advance selected base")
	gitSnapshot(t, clone, "push", "origin", fixture.branch)
	newBase := trimGit(gitSnapshot(t, clone, "rev-parse", "HEAD"))
	gitSnapshot(t, fixture.repo, "fetch", "origin", fixture.branch+":refs/remotes/origin/"+fixture.branch)
	return newBase
}

func (fixture *compactPrePRChainFixture) input() NativeGateRequestInput {
	return NativeGateRequestInput{Gate: GatePrePR, BaseRef: "origin/" + fixture.branch}
}

func replaceCompactChainState(t *testing.T, store CompactStore, mutate func(*CompactState)) {
	t.Helper()
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	mutate(&record.State)
	record, payload, err := makeCompactRecord(record.State)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}
}

func replaceCompactChainReceipt(t *testing.T, store CompactStore, mutate func(*CompactReceipt)) {
	t.Helper()
	payload, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := ParseCompactReceipt(payload)
	if err != nil {
		t.Fatal(err)
	}
	mutate(&receipt)
	payload, err = json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.ReceiptPath(), append(payload, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func compactChainAuthorityBytes(t *testing.T, stores []CompactStore) map[string]string {
	t.Helper()
	result := make(map[string]string, len(stores)*2)
	for _, store := range stores {
		for _, path := range []string{store.StatePath(), store.ReceiptPath()} {
			payload, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			result[path] = string(payload)
		}
	}
	return result
}
