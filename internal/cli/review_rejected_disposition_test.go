package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

func TestRejectedValidatorDispositionCompiledCLI(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the CLI in disposable state")
	}
	reviewEnabledHome(t)
	repo, lineage, request := providerCorrectionReadyWithoutVerificationEvidence(t)
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	payload, err := canonicalProviderRoleResult(facadeValidationResult{
		TargetedValidationRequestHash: request.RequestHash, CorrectionTargetIdentity: request.CorrectionTargetIdentity,
		OriginalCriteria: facadeValidationCheck{Passed: true, Evidence: []string{"original correction criteria pass"}},
		CorrectionRegression: facadeValidationCheck{Evidence: []string{"named notification regression"}, Regressions: []reviewtransaction.Regression{{
			ID: "successful-off-notification-suppressed", Location: "tracked.txt:1", Claim: "successful OFF notification is suppressed", ProofRefs: []string{"corrected candidate inspection"},
		}}}, FollowUps: []reviewtransaction.FollowUp{},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, closure, err := reviewProviderCloseTargetedValidatorRaw(t.Context(), repo, store, record.State, record.State.CapturePhaseRevision, payload)
	if err != nil || closure == nil || closure.State != reviewtransaction.StateEscalated {
		t.Fatalf("canonical rejected closure = %#v, %v", closure, err)
	}
	record, err = store.Load()
	if err != nil || !record.State.CorrectionAttemptConsumed() {
		t.Fatalf("rejection must consume correction: %v", err)
	}
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "gentle-ai")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-modcacherw", "-buildvcs=false", "-o", binary, "./cmd/gentle-ai")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}
	run := func(wantSuccess bool, args ...string) []byte {
		t.Helper()
		cmd := exec.CommandContext(ctx, binary, append([]string{"review"}, args...)...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "DO_NOT_TRACK=1", "GENTLE_AI_TELEMETRY=0", "GENTLE_AI_NO_SELF_UPDATE=1")
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		if (err == nil) != wantSuccess {
			t.Fatalf("CLI success=%t, err=%v\nstdout=%s\nstderr=%s", wantSuccess, err, &stdout, &stderr)
		}
		return stdout.Bytes()
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, run(true, "status", "--cwd", repo, "--lineage", lineage, "--contract", ReviewIntegrationContractV2, "--next-transition"), &status)
	if status.Action != reviewtransaction.TargetStatusActionStop || status.NextTransition == nil || status.NextTransition.ReasonCode != "native_stop_required" {
		t.Fatalf("unchanged rejection must still stop: %#v", status)
	}
	var preflight ReviewRepairResult
	decodeStrictReviewJSON(t, run(true, "repair", "--cwd", repo, "--preflight"), &preflight)
	if preflight.DispositionProviderInputs != nil || len(preflight.DispositionSelectors) != 1 {
		t.Fatalf("selectorless preflight must offer an exact choice, not authority: %#v", preflight)
	}
	selector := preflight.DispositionSelectors[0]
	if selector.SuccessorLineageID != lineage || selector.SuccessorExpectedRevision != record.Revision {
		t.Fatalf("incorrect selector: %#v", selector)
	}
	selection := []string{"--successor-lineage", lineage, "--successor-revision", record.Revision}
	decodeStrictReviewJSON(t, run(true, append([]string{"repair", "--cwd", repo, "--preflight"}, selection...)...), &preflight)
	if preflight.DispositionProviderInputs == nil {
		t.Fatal("exact preflight must offer a consent-dependent disposition")
	}
	plan, err := reviewtransaction.DeriveAuthorityDispositionPlanAtRepo(t.Context(), repo, "operator", "release rejected review", selector)
	if err != nil {
		t.Fatal(err)
	}
	args := append([]string{"repair", "--cwd", repo, "--plan-digest", plan.PlanDigest, "--inventory-revision", plan.AuthorityInventoryRevision, "--actor", plan.Actor, "--reason", plan.Reason, "--authorization", "wrong"}, selection...)
	run(false, args...)
	after, err := os.ReadFile(store.StatePath())
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("rejected command changed authority: %v", err)
	}
	args[12] = authorityDispositionAuthorization(plan)
	for attempt := 0; attempt < 2; attempt++ {
		var result ReviewRepairResult
		decodeStrictReviewJSON(t, run(true, args...), &result)
		if result.DispositionExecution == nil || result.DispositionExecution.LineageID != lineage || result.DispositionExecution.Status != string(reviewtransaction.CompactReclaimCommitted) {
			t.Fatalf("disposition execution = %#v", result)
		}
	}
	entries, err := os.ReadDir(filepath.Join(reviewCLIAuthorityRoot(t, repo), "quarantine"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("exactly one retained audit expected: %v, %v", entries, err)
	}
	retained, err := os.ReadFile(filepath.Join(reviewCLIAuthorityRoot(t, repo), "quarantine", entries[0].Name(), "residue", "review-state.json"))
	if err != nil || !bytes.Equal(before, retained) {
		t.Fatalf("historical rejected authority bytes changed: %v", err)
	}
}
