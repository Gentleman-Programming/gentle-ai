package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func assertApprovedCompactAuthorityBurned(t *testing.T, store reviewtransaction.CompactStore, lineage string) {
	t.Helper()
	base := filepath.Dir(filepath.Dir(store.Dir))
	for _, path := range []string{
		filepath.Join(base, "v2", lineage),
		filepath.Join(base, "effect-markers", "v1", lineage),
		filepath.Join(base, "incidents", lineage),
	} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("approved compact authority survived at %s: %v", path, err)
		}
	}
	for _, namespace := range []string{"witnesses", "proofs", "snapshots", "journal-outcomes"} {
		if _, err := os.Lstat(filepath.Join(base, namespace)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("approved compact finalize created forbidden %s namespace: %v", namespace, err)
		}
	}
}

func finalizeCapturedApprovedCompactReview(t *testing.T, repo string, started ReviewFacadeStartResult) ReviewFacadeFinalizeResult {
	t.Helper()
	for order := range started.SelectedLenses {
		captureCLIReviewerResult(t, repo, started, order)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--captured-results=true"}, io.Discard); err != nil {
		t.Fatalf("finalize captured reviewer results: %v", err)
	}
	evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
	if err := os.WriteFile(evidencePath, []byte("focused final verification passed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--evidence", evidencePath}, &output); err != nil {
		t.Fatalf("finalize approved compact review: %v", err)
	}
	var finalized ReviewFacadeFinalizeResult
	if err := json.Unmarshal(output.Bytes(), &finalized); err != nil {
		t.Fatal(err)
	}
	return finalized
}

func TestApprovedCompactFinalizeBurnsAuthorityAndOmitsReceiptPath(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	started := startHighRiskCLIReview(t, repo)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}

	finalized := finalizeCapturedApprovedCompactReview(t, repo, started)
	if finalized.State != reviewtransaction.StateApproved || finalized.Action != reviewApprovedCompactBurnedFinalizeAction {
		t.Fatalf("approved compact finalize = %#v, want approved burned terminal action", finalized)
	}
	if finalized.ReceiptPath != "" {
		t.Fatalf("approved compact finalize returned deleted receipt path %q", finalized.ReceiptPath)
	}
	assertApprovedCompactAuthorityBurned(t, store, started.LineageID)
}

func TestApprovedCompactFinalizeBurnedTerminalOmitsDeliveryRouting(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	started := startHighRiskCLIReview(t, repo)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	for order := range started.SelectedLenses {
		captureCLIReviewerResult(t, repo, started, order)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--captured-results=true"}, io.Discard); err != nil {
		t.Fatalf("finalize captured reviewer results: %v", err)
	}
	evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
	if err := os.WriteFile(evidencePath, []byte("focused final verification passed\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--evidence", evidencePath,
		"--contract", ReviewIntegrationContractV2, "--next-transition",
	}, &output); err != nil {
		t.Fatalf("finalize approved compact review: %v", err)
	}
	envelope := decodeReviewOperationEnvelope(t, output.Bytes())
	var finalized ReviewIntegrationFinalizeResult
	decodeStrictReviewJSON(t, envelope.Result, &finalized)
	const wantAction = "the approved review completed and burned; delivery follows ordinary repository policy"
	if finalized.State != reviewtransaction.StateApproved || finalized.Action != wantAction {
		t.Fatalf("burned terminal result = %#v, want approved action %q", finalized, wantAction)
	}
	if finalized.NextTransition != nil {
		t.Fatalf("burned terminal result advertised a next review transition: %#v", finalized.NextTransition)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(envelope.Result, &fields); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"next_transition", "forecast", "receipt_path"} {
		if _, present := fields[forbidden]; present {
			t.Fatalf("burned terminal result exposed %q: %s", forbidden, envelope.Result)
		}
	}
	for _, forbidden := range []string{"validate delivery", "review validate", "delivery_gate_required", "receipt", "approval gate", "disabled RDD"} {
		if bytes.Contains(bytes.ToLower(envelope.Result), []byte(forbidden)) {
			t.Fatalf("burned terminal result exposed stale delivery routing %q: %s", forbidden, envelope.Result)
		}
	}
	assertApprovedCompactAuthorityBurned(t, store, started.LineageID)
}

func TestApprovedCompactBurnStagesStatusThenFinalizeInOneRetry(t *testing.T) {
	for _, phase := range []string{"live", "prepared", "deleting"} {
		t.Run(phase, func(t *testing.T) {
			reviewEnabledHome(t)
			fixture := prepareFacadeReceiptPending(t)
			receipt, err := fixture.pending.State.Receipt()
			if err != nil {
				t.Fatal(err)
			}
			if err := reviewtransaction.WriteCompactReceiptAtomic(fixture.store.ReceiptPath(), receipt); err != nil {
				t.Fatal(err)
			}
			pending, err := fixture.store.PendingFinalizeAttempt()
			if err != nil || pending == nil {
				t.Fatalf("pending approved finalize attempt = %#v, %v", pending, err)
			}
			if err := fixture.store.MarkFinalizeAttemptReceiptPublished(pending.Request.RequestDigest); err != nil {
				t.Fatal(err)
			}
			if err := fixture.store.CompleteFinalizeAttempt(pending.Request.RequestDigest); err != nil {
				t.Fatal(err)
			}
			base := filepath.Dir(filepath.Dir(fixture.store.Dir))
			staging := ""
			if phase != "live" {
				digest := sha256.Sum256([]byte(fixture.pending.Revision))
				staging = filepath.Join(base, fmt.Sprintf(".review-burn-v2-%s-%x-%s", fixture.started.LineageID, digest, phase))
				if err := os.Mkdir(staging, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(fixture.store.Dir, filepath.Join(staging, "authority")); err != nil {
					t.Fatal(err)
				}
			}
			beforeStatus := snapshotAuthorityTree(t, base)

			var statusOutput bytes.Buffer
			if err := RunReview([]string{
				"status", "--contract", ReviewIntegrationContractV2, "--next-transition", "--cwd", fixture.repo,
				"--lineage", fixture.started.LineageID,
			}, &statusOutput); err != nil {
				t.Fatalf("read-only STATUS for %s burn stage: %v\n%s", phase, err, statusOutput.String())
			}
			if afterStatus := snapshotAuthorityTree(t, base); afterStatus != beforeStatus {
				t.Fatalf("STATUS mutated %s burn staging", phase)
			}
			var status ReviewTargetStatusResult
			decodeStrictReviewJSON(t, statusOutput.Bytes(), &status)
			if status.Authority == nil || status.Authority.Version != reviewtransaction.AuthorityVersionCompact ||
				status.Authority.State != reviewtransaction.StateApproved || status.Receipt.Status != ReviewReceiptPresent ||
				status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionExecute ||
				status.NextTransition.Execute == nil || status.NextTransition.Execute.Operation != "review.finalize" ||
				status.NextTransition.Execute.Binding.LineageID != fixture.started.LineageID {
				t.Fatalf("STATUS for %s burn stage = %#v", phase, status)
			}

			var finalizedOutput bytes.Buffer
			if err := RunReviewFacadeFinalize([]string{
				"--cwd", fixture.repo, "--lineage", fixture.started.LineageID,
			}, &finalizedOutput); err != nil {
				t.Fatalf("FINALIZE recovery for %s burn stage: %v", phase, err)
			}
			finalized := assertApprovedBurnedCompactFacadeFinalize(t, finalizedOutput.Bytes())
			if finalized.LineageID != fixture.started.LineageID {
				t.Fatalf("FINALIZE recovered wrong lineage: %#v", finalized)
			}
			assertApprovedCompactAuthorityBurned(t, fixture.store, fixture.started.LineageID)
			if staging != "" {
				if _, err := os.Lstat(staging); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("FINALIZE left %s burn staging: %v", phase, err)
				}
			}
			if err := RunReviewFacadeFinalize([]string{
				"--cwd", fixture.repo, "--lineage", fixture.started.LineageID,
			}, io.Discard); err == nil {
				t.Fatalf("later FINALIZE accepted total absence after %s burn", phase)
			}
		})
	}
}
func TestApprovedCompactBurnDeletingAuthorityLossUsesExactRevision(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		expected func(string) string
		wantErr  bool
	}{
		{name: "wrong revision", expected: func(string) string { return "sha256:" + strings.Repeat("0", 64) }, wantErr: true},
		{name: "exact revision", expected: func(revision string) string { return revision }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			reviewEnabledHome(t)
			fixture := prepareFacadeReceiptPending(t)
			base := filepath.Dir(filepath.Dir(fixture.store.Dir))
			digest := sha256.Sum256([]byte(fixture.pending.Revision))
			stage := filepath.Join(base, fmt.Sprintf(".review-burn-v2-%s-%x-deleting", fixture.started.LineageID, digest))
			if err := os.Mkdir(stage, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(fixture.store.Dir, filepath.Join(stage, "authority")); err != nil {
				t.Fatal(err)
			}
			if err := os.RemoveAll(filepath.Join(stage, "authority")); err != nil {
				t.Fatal(err)
			}
			before := snapshotAuthorityTree(t, base)
			var output bytes.Buffer
			err := RunReviewFacadeFinalize([]string{
				"--cwd", fixture.repo, "--lineage", fixture.started.LineageID,
				"--expected-revision", testCase.expected(fixture.pending.Revision),
			}, &output)
			if testCase.wantErr {
				if err == nil {
					t.Fatal("FINALIZE accepted a deleting stage with the wrong revision")
				}
				if after := snapshotAuthorityTree(t, base); after != before {
					t.Fatal("wrong-revision FINALIZE changed deleting burn staging")
				}
				return
			}
			if err != nil {
				t.Fatalf("FINALIZE exact deleting-stage recovery: %v", err)
			}
			assertApprovedBurnedCompactFacadeFinalize(t, output.Bytes())
			assertApprovedCompactAuthorityBurned(t, fixture.store, fixture.started.LineageID)
		})
	}
}
func TestExactApprovedCompactBurnIgnoresMalformedForeignStage(t *testing.T) {
	reviewEnabledHome(t)
	fixture := prepareFacadeReceiptPending(t)
	base := filepath.Dir(filepath.Dir(fixture.store.Dir))
	foreign := filepath.Join(base, ".review-burn-v2-foreign-burn-"+strings.Repeat("0", 64)+"-prepared")
	if err := os.Mkdir(foreign, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(foreign, "unexpected"), []byte("foreign residue"), 0o600); err != nil {
		t.Fatal(err)
	}
	var statusOutput bytes.Buffer
	if err := RunReview([]string{
		"status", "--contract", ReviewIntegrationContractV2, "--next-transition", "--cwd", fixture.repo,
		"--lineage", fixture.started.LineageID,
	}, &statusOutput); err != nil {
		t.Fatalf("exact STATUS was blocked by foreign staging: %v", err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &status)
	if status.Authority == nil || status.Authority.LineageID != fixture.started.LineageID {
		t.Fatalf("exact STATUS selected foreign staging: %#v", status)
	}
	var finalizedOutput bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", fixture.repo, "--lineage", fixture.started.LineageID}, &finalizedOutput); err != nil {
		t.Fatalf("exact FINALIZE was blocked by foreign staging: %v", err)
	}
	assertApprovedBurnedCompactFacadeFinalize(t, finalizedOutput.Bytes())
	if got, err := os.ReadFile(filepath.Join(foreign, "unexpected")); err != nil || string(got) != "foreign residue" {
		t.Fatalf("exact FINALIZE changed foreign staging: %q, %v", got, err)
	}
	var selectorless bytes.Buffer
	if err := RunReview([]string{"status", "--contract", ReviewIntegrationContractV2, "--next-transition", "--cwd", fixture.repo}, &selectorless); err != nil {
		t.Fatalf("selectorless STATUS was blocked by foreign staging: %v", err)
	}
	var result ReviewTargetStatusResult
	decodeStrictReviewJSON(t, selectorless.Bytes(), &result)
	if result.Applicability != reviewtransaction.TargetApplicabilityUnrelated || result.NextTransition == nil ||
		result.NextTransition.Kind != reviewNextTransitionExecute || result.NextTransition.Execute == nil ||
		result.NextTransition.Execute.Operation != "review.start" {
		t.Fatalf("selectorless STATUS did not ignore malformed foreign staging: %#v", result)
	}
}
func TestApprovedCompactBurnStatusFailsClosedForUnsafeStaging(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		setup func(*testing.T, facadeReceiptPendingFixture, string)
	}{
		{
			name: "malformed",
			setup: func(t *testing.T, fixture facadeReceiptPendingFixture, base string) {
				t.Helper()
				if err := os.Mkdir(filepath.Join(base, ".review-burn-v2-"+fixture.started.LineageID+"-not-a-digest-prepared"), 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "multiple",
			setup: func(t *testing.T, fixture facadeReceiptPendingFixture, base string) {
				t.Helper()
				digest := sha256.Sum256([]byte(fixture.pending.Revision))
				for _, phase := range []string{"prepared", "deleting"} {
					if err := os.Mkdir(filepath.Join(base, fmt.Sprintf(".review-burn-v2-%s-%x-%s", fixture.started.LineageID, digest, phase)), 0o700); err != nil {
						t.Fatal(err)
					}
				}
			},
		},
		{
			name: "symlinked",
			setup: func(t *testing.T, fixture facadeReceiptPendingFixture, base string) {
				t.Helper()
				digest := sha256.Sum256([]byte(fixture.pending.Revision))
				stage := filepath.Join(base, fmt.Sprintf(".review-burn-v2-%s-%x-prepared", fixture.started.LineageID, digest))
				if err := os.Symlink(t.TempDir(), stage); err != nil {
					t.Skipf("symlink staging is unavailable: %v", err)
				}
			},
		},
		{
			name: "conflicting",
			setup: func(t *testing.T, fixture facadeReceiptPendingFixture, base string) {
				t.Helper()
				digest := sha256.Sum256([]byte(fixture.pending.Revision))
				stage := filepath.Join(base, fmt.Sprintf(".review-burn-v2-%s-%x-prepared", fixture.started.LineageID, digest))
				if err := os.Mkdir(stage, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(fixture.store.Dir, filepath.Join(stage, "authority")); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(fixture.store.Dir, 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "mismatched",
			setup: func(t *testing.T, fixture facadeReceiptPendingFixture, base string) {
				t.Helper()
				stage := filepath.Join(base, ".review-burn-v2-"+fixture.started.LineageID+"-"+strings.Repeat("0", 64)+"-prepared")
				if err := os.Mkdir(stage, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(fixture.store.Dir, filepath.Join(stage, "authority")); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			reviewEnabledHome(t)
			fixture := prepareFacadeReceiptPending(t)
			base := filepath.Dir(filepath.Dir(fixture.store.Dir))
			testCase.setup(t, fixture, base)
			before := ""
			if testCase.name != "symlinked" {
				before = snapshotAuthorityTree(t, base)
			}

			var output bytes.Buffer
			err := RunReview([]string{
				"status", "--contract", ReviewIntegrationContractV2, "--next-transition", "--cwd", fixture.repo,
				"--lineage", fixture.started.LineageID,
			}, &output)
			if err == nil {
				var status ReviewTargetStatusResult
				decodeStrictReviewJSON(t, output.Bytes(), &status)
				if status.Applicability != reviewtransaction.TargetApplicabilityCorrupted ||
					status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionStop {
					t.Fatalf("unsafe %s staging fell through ordinary STATUS: %#v", testCase.name, status)
				}
			}
			if before != "" {
				if after := snapshotAuthorityTree(t, base); after != before {
					t.Fatalf("STATUS mutated unsafe %s staging", testCase.name)
				}
			}
		})
	}
}

func TestEscalatedCompactFinalizeDoesNotBurnAuthority(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfour\n", 0o644)
	started := startFacadeReview(t, repo)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	resultPath := filepath.Join(t.TempDir(), "reviewer.json")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{Findings: []facadeFinding{{
		Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "candidate regression",
		ProofRefs: []string{"tracked.txt:5 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
		CausalDisposition: reviewtransaction.CausalIntroduced,
	}}, Evidence: []string{"reviewed frozen candidate"}})
	var output bytes.Buffer
	if err := finalizeReviewCLIArgs(t, repo, []string{
		"--cwd", repo, "--lineage", started.LineageID, "--result", resultPath, "--correction-lines", "3",
	}, &output); err != nil {
		t.Fatalf("finalize escalated compact review: %v", err)
	}
	var finalized ReviewFacadeFinalizeResult
	if err := json.Unmarshal(output.Bytes(), &finalized); err != nil {
		t.Fatal(err)
	}
	if finalized.State != reviewtransaction.StateEscalated {
		t.Fatalf("escalated compact finalize state = %q, want %q", finalized.State, reviewtransaction.StateEscalated)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if record.State.State != reviewtransaction.StateEscalated {
		t.Fatalf("escalated compact authority was burned or changed: %#v", record.State)
	}
}

func TestApprovedCompactTerminalFinalizeReplaysBurnAuthorityOnce(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		prepare func(t *testing.T, fixture facadeReceiptPendingFixture)
	}{
		{
			name:    "receipt publication replay",
			prepare: func(t *testing.T, fixture facadeReceiptPendingFixture) {},
		},
		{
			name: "pending journal after receipt publication",
			prepare: func(t *testing.T, fixture facadeReceiptPendingFixture) {
				receipt, err := fixture.pending.State.Receipt()
				if err != nil {
					t.Fatal(err)
				}
				if err := reviewtransaction.WriteCompactReceiptAtomic(fixture.store.ReceiptPath(), receipt); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "completed terminal attempt",
			prepare: func(t *testing.T, fixture facadeReceiptPendingFixture) {
				receipt, err := fixture.pending.State.Receipt()
				if err != nil {
					t.Fatal(err)
				}
				if err := reviewtransaction.WriteCompactReceiptAtomic(fixture.store.ReceiptPath(), receipt); err != nil {
					t.Fatal(err)
				}
				pending, err := fixture.store.PendingFinalizeAttempt()
				if err != nil || pending == nil {
					t.Fatalf("pending terminal attempt = %#v, %v", pending, err)
				}
				if err := fixture.store.MarkFinalizeAttemptReceiptPublished(pending.Request.RequestDigest); err != nil {
					t.Fatal(err)
				}
				if err := fixture.store.CompleteFinalizeAttempt(pending.Request.RequestDigest); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			reviewEnabledHome(t)
			fixture := prepareFacadeReceiptPending(t)
			testCase.prepare(t, fixture)

			var output bytes.Buffer
			if err := RunReviewFacadeFinalize([]string{"--cwd", fixture.repo, "--lineage", fixture.started.LineageID}, &output); err != nil {
				t.Fatalf("terminal compact finalize replay: %v", err)
			}
			var finalized ReviewFacadeFinalizeResult
			if err := json.Unmarshal(output.Bytes(), &finalized); err != nil {
				t.Fatal(err)
			}
			if finalized.State != reviewtransaction.StateApproved || finalized.Action != reviewApprovedCompactBurnedFinalizeAction || finalized.ReceiptPath != "" {
				t.Fatalf("terminal compact replay = %#v, want approved burned terminal action without receipt path", finalized)
			}
			assertApprovedCompactAuthorityBurned(t, fixture.store, fixture.started.LineageID)

			if err := RunReviewFacadeFinalize([]string{"--cwd", fixture.repo, "--lineage", fixture.started.LineageID}, io.Discard); err == nil {
				t.Fatal("burned approved compact authority finalized more than once")
			}
		})
	}
}
