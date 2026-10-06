package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// STATUS proves a reviewer slot is fillable before offering it, and the block
// it measures must carry the handle the driving runtime will really receive:
// the sealed rctx3 handle for an OpenCode-driven STATUS, the rctx2 digest for
// every other runtime -- whatever runtime the lineage froze at START.

func TestPiDrivenStatusOnAnOpenCodeLineageNeverCreatesTheSealingKey(t *testing.T) {
	if testing.Short() {
		t.Skip("requires git fixtures")
	}
	home := reviewEnabledHome(t)
	repo, started, _, _ := openCodeSealedLineage(t, "pi-driven-key")
	key := filepath.Join(home, ".gentle-ai", "review-context.key")
	if err := os.Remove(key); err != nil {
		t.Fatalf("the OpenCode START did not create the sealing key: %v", err)
	}
	raw, status := openCodeSealedStatus(t, repo, started.LineageID, model.AgentPi)
	if bytes.Contains(raw, []byte("rctx3_")) || status.NextTransition.Collect == nil {
		t.Fatalf("Pi-driven STATUS = %s", raw)
	}
	if _, err := os.Stat(key); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a Pi-driven STATUS created the OpenCode sealing key: %v", err)
	}
}

func TestOpenCodeDrivenStatusOnAPiLineageMeasuresTheSealedHandle(t *testing.T) {
	if testing.Short() {
		t.Skip("requires git fixtures")
	}
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	const base = 1000
	writeSealedBudgetCandidate(t, repo, base)
	probe := runNegotiatedReviewStartWith(t, repo, "sealed-budget-probe", "--agent", string(model.AgentPi))
	_, probeRecord := loadOpenCodeRelayAuthority(t, repo, probe.LineageID)

	// The largest patch growth each handle format still fits, measured on the
	// exact block the lens slot would carry.
	digest, err := reviewtransaction.DeriveReviewRepositoryContextHandle(t.Context(), repo, sealedBudgetBinding(probeRecord))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := reviewtransaction.DeriveOpenCodeReviewRepositoryContextHandle(t.Context(), repo, sealedBudgetBinding(probeRecord))
	if err != nil {
		t.Fatal(err)
	}
	digestSlack := sealedBudgetSlack(t, repo, probeRecord, digest)
	sealedSlack := sealedBudgetSlack(t, repo, probeRecord, sealed)
	if digestSlack-sealedSlack < 32 {
		t.Fatalf("handle formats leave slack %d (rctx2) vs %d (rctx3); the boundary is not observable", digestSlack, sealedSlack)
	}

	// A candidate that fits with rctx2 but not with rctx3.
	writeSealedBudgetCandidate(t, repo, base+(digestSlack+sealedSlack)/2)
	started := runNegotiatedReviewStartWith(t, repo, "sealed-budget-limit", "--agent", string(model.AgentPi))
	_, piStatus := openCodeSealedStatus(t, repo, started.LineageID, "")
	if piStatus.NextTransition.Kind != reviewNextTransitionCollect {
		t.Fatalf("Pi-driven STATUS at the rctx2 limit = %#v, want its reviewer slots", piStatus.NextTransition)
	}
	_, openCodeStatus := openCodeSealedStatus(t, repo, started.LineageID, model.AgentOpenCode)
	if openCodeStatus.NextTransition.Kind != reviewNextTransitionStop || openCodeStatus.NextTransition.ReasonCode != "lens_context_budget_exceeded" {
		t.Fatalf("OpenCode-driven STATUS past the rctx3 limit = %#v, want lens_context_budget_exceeded", openCodeStatus.NextTransition)
	}
}

// writeSealedBudgetCandidate writes one authored code path whose last line
// grows its patch byte for byte.
func writeSealedBudgetCandidate(t *testing.T, repo string, length int) {
	t.Helper()
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\n// a"+strings.Repeat("x", length)+"\n", 0o644)
}

func sealedBudgetBinding(record reviewtransaction.CompactRecord) reviewtransaction.ReviewRepositoryContextBinding {
	return reviewtransaction.ReviewRepositoryContextBinding{
		LineageID: record.State.LineageID, TargetIdentity: record.State.InitialSnapshot.Identity, Revision: record.State.CapturePhaseRevision,
	}
}

// sealedBudgetSlack returns the largest number of bytes the candidate patch
// may still grow while every selected lens block carrying handle fits.
func sealedBudgetSlack(t *testing.T, repo string, record reviewtransaction.CompactRecord, handle string) int {
	t.Helper()
	ctx := context.Background()
	state := record.State
	if len(state.SelectedLenses) == 0 {
		t.Fatal("the budget candidate selected no lens; nothing would be measured")
	}
	fits := func(pad int) bool {
		deps := reviewLensContextDependencies()
		inner := deps.inspect
		deps.inspect = func(ctx context.Context, inspector reviewLensCandidateInspector, operation string, index int, side string) ([]byte, error) {
			payload, err := inner(ctx, inspector, operation, index, side)
			if err == nil && operation == "patch" {
				payload = append(bytes.TrimRight(payload, "\n"), strings.Repeat("x", pad)...)
			}
			return payload, err
		}
		inspector, err := deps.prepare(reviewtransaction.SnapshotBuilder{Repo: repo}, ctx, state.InitialSnapshot)
		if err != nil {
			t.Fatal(err)
		}
		defer deps.close(inspector)
		frozen := inspector.FrozenCandidateContext()
		for order, lens := range state.SelectedLenses {
			if frozen.ChangedPathManifest[0].Generated {
				t.Fatal("the budget candidate was classified as generated")
			}
			subject, err := reviewtransaction.NewArtifactSubject(state, state.CapturePhaseRevision, frozen, lens, order, "")
			if err != nil {
				t.Fatal(err)
			}
			_, err = reviewLensContextBlock(ctx, deps, inspector, reviewLensContextBinding{
				Lineage: state.LineageID, Target: state.InitialSnapshot.Identity, Lens: lens, Order: order,
				Revision: state.CapturePhaseRevision, RepositoryContext: handle, SubjectHash: subject.SubjectHash,
			}, subject, frozen, state.RuntimeAgent)
			var refusal *reviewLensContextError
			if errors.As(err, &refusal) && refusal.Code == "lens_context_budget_exceeded" {
				return false
			}
			if err != nil {
				t.Fatal(err)
			}
		}
		return true
	}
	low, high := 0, 256<<10
	if !fits(low) || fits(high) {
		t.Fatalf("budget search bounds are not a boundary: fits(0)=%v fits(%d)=%v", fits(low), high, fits(high))
	}
	for high-low > 1 {
		middle := (low + high) / 2
		if fits(middle) {
			low = middle
		} else {
			high = middle
		}
	}
	return low
}

// An unsafe sealing key is an environment fault, not lifecycle state: an
// OpenCode-driven STATUS that has to seal refuses with the key's repair and
// mutates nothing, while every other runtime never consults the key.
func TestOpenCodeStatusWithAnUnsafeSealingKeyRefusesWithItsRepair(t *testing.T) {
	if testing.Short() {
		t.Skip("requires git fixtures")
	}
	if runtime.GOOS == "windows" {
		t.Skip("POSIX key file modes")
	}
	home := reviewEnabledHome(t)
	repo, started, store, _ := openCodeSealedLineage(t, "unsafe-key-status")
	key := filepath.Join(home, ".gentle-ai", "review-context.key")
	keyBytes, err := os.ReadFile(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(key, 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	err = RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--lineage", started.LineageID,
		"--agent", string(model.AgentOpenCode), "--next-transition",
	}, &output)
	if err == nil {
		t.Fatalf("OpenCode STATUS with an unsafe key succeeded:\n%s", output.String())
	}
	var failure *ReviewIntegrationFailureError
	if !errors.As(err, &failure) || failure.Failure.Code != "repository_context_key_unsafe" ||
		failure.Failure.MutationOutcome != ReviewMutationNotStarted || failure.Failure.NextAction != "stop" ||
		!strings.Contains(failure.Failure.Cause, "review-context.key") || !strings.Contains(failure.Failure.Cause, "chmod 600") ||
		!strings.Contains(failure.Failure.Cause, "gentle-ai review status") {
		t.Fatalf("unsafe key STATUS refusal = %v (%#v), want a typed refusal naming the key repair", err, failure)
	}
	if strings.Contains(err.Error()+output.String()+failure.Failure.Cause, string(keyBytes)) {
		t.Fatal("the unsafe key refusal leaked key bytes")
	}
	after, err := os.ReadFile(store.StatePath())
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("unsafe key STATUS mutated compact authority: %v", err)
	}

	raw, piStatus := openCodeSealedStatus(t, repo, started.LineageID, model.AgentPi)
	if piStatus.NextTransition.Kind != reviewNextTransitionCollect || bytes.Contains(raw, []byte("rctx3_")) {
		t.Fatalf("Pi-driven STATUS with an unsafe OpenCode key = %s", raw)
	}
	if info, err := os.Stat(key); err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("a STATUS repaired or replaced the unsafe key on its own: %v, %v", info, err)
	}
}
