package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v5/internal/reviewtransaction"
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
	run := func(t *testing.T, wantCode int, args ...string) ([]byte, string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, binary, append([]string{"review"}, args...)...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "DO_NOT_TRACK=1", "GENTLE_AI_TELEMETRY=0", "GENTLE_AI_NO_SELF_UPDATE=1")
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != wantCode || (wantCode == 0 && stderr.Len() != 0) {
			t.Fatalf("CLI wanted exit %d, err=%v\nstdout=%s\nstderr=%s", wantCode, err, &stdout, &stderr)
		}
		return stdout.Bytes(), stderr.String()
	}
	read := func(dst any, args ...string) []byte {
		t.Helper()
		out, _ := run(t, 0, args...)
		decodeStrictReviewJSON(t, out, dst)
		return out
	}
	var status ReviewTargetStatusResult
	statusJSON := read(&status, "status", "--cwd", repo, "--lineage", lineage, "--contract", ReviewIntegrationContractV2, "--next-transition")
	if status.Action != reviewtransaction.TargetStatusActionStop || status.NextTransition == nil || status.NextTransition.ReasonCode != "native_stop_required" {
		t.Fatalf("unchanged rejection must still stop: %#v", status)
	}
	var inspection ReviewInspectAuthorityResult
	inspectionJSON := read(&inspection, "inspect-authority", "--cwd", repo)
	if !inspection.Complete || inspection.Totals.CompactEntries != 1 {
		t.Fatalf("expected one active authority: %#v", inspection)
	}

	base := []string{"repair", "--cwd", repo, "--contract", ReviewIntegrationContractV2}
	preflightArgs := append(base, "--preflight")
	var choices ReviewRepairResult
	choicesJSON := read(&choices, preflightArgs...)
	if choices.DispositionProviderInputs != nil || len(choices.DispositionSelectors) != 1 {
		t.Fatalf("selectorless preflight must offer an exact choice, not authority: %#v", choices)
	}
	selector := choices.DispositionSelectors[0]
	if selector.SuccessorLineageID != lineage || selector.SuccessorExpectedRevision != record.Revision || selector.PredecessorLineageID != "" || selector.PredecessorExpectedRevision != "" {
		t.Fatalf("incorrect selector: %#v", selector)
	}
	selection := []string{"--successor-lineage", selector.SuccessorLineageID, "--successor-revision", selector.SuccessorExpectedRevision}
	var v1, selected ReviewRepairResult
	v1JSON := read(&v1, append([]string{"repair", "--cwd", repo, "--preflight"}, selection...)...)
	selectedJSON := read(&selected, append(preflightArgs, selection...)...)
	for _, out := range [][]byte{statusJSON, inspectionJSON, choicesJSON, v1JSON} {
		if bytes.Contains(out, []byte(`"repository_binding"`)) {
			t.Fatalf("repository binding escaped exact v2 preflight: %s", out)
		}
	}
	var public struct {
		Inputs map[string]string `json:"disposition_provider_inputs"`
	}
	// Use public JSON names, not an internal plan or authorization renderer.
	if err := json.Unmarshal(selectedJSON, &public); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"repository_binding", "plan_digest", "authority_inventory_revision"} {
		if !validReviewCapabilitySHA256(public.Inputs[field]) {
			t.Fatalf("exact preflight lacks public authorization input %q: %s", field, selectedJSON)
		}
	}
	if len(public.Inputs) != 3 || bytes.Contains(selectedJSON, []byte(repo)) || bytes.Contains(selectedJSON, []byte(lineage)) || len(selected.DispositionSelectors) != 0 {
		t.Fatalf("exact preflight published more than the path-free provider inputs: %s", selectedJSON)
	}
	schema := compileWholePublishedReviewSchema(t, "v2", "repair.schema.json")
	validatePublishedReviewSchema(t, schema, choicesJSON)
	validatePublishedReviewSchema(t, schema, selectedJSON)
	var document map[string]any
	if err := json.Unmarshal(selectedJSON, &document); err != nil {
		t.Fatal(err)
	}
	inputs := document["disposition_provider_inputs"].(map[string]any)
	for _, field := range []string{"actor", "reason", "authorization"} {
		inputs[field] = "private value"
		if err := schema.Validate(document); err == nil {
			t.Fatalf("schema accepted %s in disposition provider inputs", field)
		}
		delete(inputs, field)
	}

	help, _ := run(t, 0, "repair", "--help")
	_, recipe, found := strings.Cut(string(help), "--authorization is exactly eight LF-separated lines with no trailing newline:\n\n")
	template, _, ended := strings.Cut(recipe, "\n\n")
	if !found || !ended || strings.Count(template, "\n") != 7 || strings.Contains(string(help), "gentle-ai.review-repair-authorization/v1") {
		t.Fatalf("missing disposition authorization recipe: %s", help)
	}
	doc, err := os.ReadFile(filepath.Join(root, "docs", "review-authority-threat-model.md"))
	if err != nil || !strings.Contains(string(doc), "   ```text\n   "+strings.ReplaceAll(template, "\n", "\n   ")+"\n   ```") {
		t.Fatalf("documented binding differs from compiled help: %v", err)
	}
	const actor, reason = "operator", "release rejected review"
	authorization := strings.NewReplacer(
		"<repository_binding>", public.Inputs["repository_binding"],
		"<plan_digest>", public.Inputs["plan_digest"],
		"<authority_inventory_revision>", public.Inputs["authority_inventory_revision"],
		"<actor>", actor, "<reason>", reason,
	).Replace(template)
	args := append(append(base, selection...), "--plan-digest", public.Inputs["plan_digest"], "--inventory-revision", public.Inputs["authority_inventory_revision"], "--actor", actor, "--reason", reason)
	authorityRoot := reviewCLIAuthorityRoot(t, repo)
	quarantine := filepath.Join(authorityRoot, "quarantine")
	for _, test := range []struct{ name, authorization string }{
		{"tampered repository", strings.Replace(authorization, public.Inputs["repository_binding"], "sha256:"+strings.Repeat("0", 64), 1)},
		{"trailing newline", authorization + "\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			stdout, stderr := run(t, 1, append(args, "--authorization", test.authorization)...)
			var failure ReviewIntegrationFailure
			decodeStrictReviewJSON(t, stdout, &failure)
			if failure.MutationOutcome != ReviewMutationNotStarted || failure.Code != "invalid_request" ||
				failure.Cause != "authority disposition plan refused: authorization does not bind to plan_digest and authority_inventory_revision" ||
				stderr != "Error: The negotiated review request is invalid. [invalid_request]\n" ||
				bytes.Contains(stdout, []byte(actor)) || bytes.Contains(stdout, []byte(reason)) {
				t.Fatalf("unexpected refusal: stdout=%s stderr=%q", stdout, stderr)
			}
			if after, err := os.ReadFile(store.StatePath()); err != nil || !bytes.Equal(before, after) {
				t.Fatalf("refused invocation changed authority: %v", err)
			}
			if entries, err := os.ReadDir(quarantine); !os.IsNotExist(err) && (err != nil || len(entries) != 0) {
				t.Fatalf("refused invocation left audit residue: %v, %v", entries, err)
			}
		})
	}
	var committed ReviewRepairDispositionExecution
	for attempt := 0; attempt < 2; attempt++ {
		var result ReviewRepairResult
		out := read(&result, append(args, "--authorization", authorization)...)
		validatePublishedReviewSchema(t, schema, out)
		if result.DispositionExecution == nil || result.DispositionExecution.LineageID != selector.SuccessorLineageID ||
			result.DispositionExecution.Status != "committed" || result.DispositionExecution.AnomalyClass != "rejected_targeted_validator_terminal" ||
			result.DispositionExecution.PlanDigest != public.Inputs["plan_digest"] || result.DispositionExecution.AuthorityInventoryRevision != public.Inputs["authority_inventory_revision"] ||
			bytes.Contains(out, []byte(repo)) || bytes.Contains(out, []byte(actor)) || bytes.Contains(out, []byte(reason)) {
			t.Fatalf("disposition execution = %s", out)
		}
		if attempt == 1 && *result.DispositionExecution != committed {
			t.Fatalf("exact replay changed audit proof: %s", out)
		}
		committed = *result.DispositionExecution
	}
	read(&inspection, "inspect-authority", "--cwd", repo)
	if !inspection.Complete || inspection.Totals.CompactEntries != 0 {
		t.Fatalf("active authority not released: %#v", inspection)
	}
	entries, err := os.ReadDir(quarantine)
	if err != nil || len(entries) != 1 {
		t.Fatalf("exactly one retained audit expected: %v, %v", entries, err)
	}
	retained, err := os.ReadFile(filepath.Join(quarantine, entries[0].Name(), "residue", "review-state.json"))
	if err != nil || !bytes.Equal(before, retained) {
		t.Fatalf("historical rejected authority bytes changed: %v", err)
	}
}
