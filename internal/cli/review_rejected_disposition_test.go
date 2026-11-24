package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v5/internal/reviewtransaction"
)

func TestRejectedValidatorDispositionCompiledCLI(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the CLI in disposable state")
	}
	t.Run("ordinary path", func(t *testing.T) { testRejectedValidatorDispositionCompiledCLI(t, false) })
	t.Run("path with Markdown delimiters", func(t *testing.T) {
		if _, err := os.Stat("/bin/sh"); err != nil {
			t.Skip("requires /bin/sh")
		}
		dir := filepath.Join(t.TempDir(), "repo`. west $literal ```` tail")
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("TMPDIR", dir)
		t.Run("verbatim shell preflight", func(t *testing.T) { testRejectedValidatorDispositionCompiledCLI(t, true) })
	})
}

func testRejectedValidatorDispositionCompiledCLI(t *testing.T, backtickPath bool) {
	t.Helper()
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
	captureArgs := []string{
		"capture-validation", "--cwd", repo, "--lineage", lineage, "--target", request.CorrectionTargetIdentity,
		"--expected-revision", record.State.CapturePhaseRevision, "--request-hash", request.RequestHash,
		"--agent", "pi", "--input", writeReviewCLIRawInput(t, payload),
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
	var closure reviewLastEventClosureResult
	closureJSON := read(&closure, captureArgs...)
	validatePublishedReviewSchema(t, compileWholePublishedReviewSchema(t, "v2", "last-event-closure.schema.json"), closureJSON)
	if closure.State != reviewtransaction.StateEscalated || closure.LineageID != lineage || closure.Escalation == nil ||
		closure.Escalation.Cause != "targeted_validator_rejected" || closure.TargetedValidatorEvidence == nil ||
		closure.TargetedValidatorEvidence.CorrectionRegression.Passed || closure.StatusContinuation != nil || closure.Acknowledgement != nil {
		t.Fatalf("expected a non-actionable, canonical rejected closure: %s", closureJSON)
	}
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if replay, _ := run(t, 0, captureArgs...); !bytes.Equal(replay, closureJSON) {
		t.Fatalf("exact capture replay changed closure:\nfresh=%s\nreplay=%s", closureJSON, replay)
	}
	var status ReviewTargetStatusResult
	statusArgs := []string{"status", "--cwd", repo, "--lineage", lineage, "--contract", ReviewIntegrationContractV2, "--next-transition"}
	statusJSON := read(&status, statusArgs...)
	if status.Action != reviewtransaction.TargetStatusActionStop || status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionStop ||
		status.NextTransition.ReasonCode != "native_stop_required" || status.NextTransition.Execute != nil || status.NextTransition.Collect != nil ||
		status.NextTransition.Continuation != nil || status.Authority == nil || status.Authority.State != closure.State ||
		status.Authority.LineageID != closure.LineageID || status.Authority.Revision != closure.StoreRevision {
		t.Fatalf("unchanged rejection must still stop on the exact closure authority: %s", statusJSON)
	}
	validatePublishedReviewSchema(t, compileWholeNativeStatusSchema(t, "status-v9.schema.json"), statusJSON)
	if status.Forecast == nil || status.Forecast.Horizon != ForecastHorizonTerminal || len(status.Forecast.Steps) != 1 ||
		status.Forecast.Steps[0].Step != 1 || status.Forecast.Steps[0].Kind != status.NextTransition.Kind ||
		status.Forecast.Steps[0].ReasonCode != status.NextTransition.ReasonCode {
		t.Fatalf("expected structural terminal forecast: %s", statusJSON)
	}
	for _, tc := range []struct{ name, lineage, contract string }{
		{"selectorless", "", ReviewIntegrationContractV2},
		{"unrelated lineage", "review-unrelated", ReviewIntegrationContractV2},
		{"v1", lineage, ReviewIntegrationContractV1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"status", "--cwd", repo, "--contract", tc.contract, "--next-transition"}
			if tc.lineage != "" {
				args = append(args, "--lineage", tc.lineage)
			}
			out, _ := run(t, 0, args...)
			if bytes.Contains(out, []byte("gentle-ai review repair")) || tc.contract == ReviewIntegrationContractV1 && bytes.Contains(out, []byte(`"forecast"`)) {
				t.Fatalf("terminal handoff escaped its bound v2 STATUS: %s", out)
			}
		})
	}
	t.Run("RDD disabled", func(t *testing.T) {
		reviewEnabledHome(t)
		if _, err := SetGlobalReviewMode(t.Context(), repo, false); err != nil {
			t.Fatal(err)
		}
		var disabled ReviewTargetStatusResult
		out := read(&disabled, statusArgs...)
		if disabled.NextTransition == nil || disabled.NextTransition.ReasonCode != "native_stop_required" || bytes.Contains(out, []byte("gentle-ai review repair")) {
			t.Fatalf("terminal handoff changed STATUS for an existing lineage with RDD disabled: %s", out)
		}
		var unrelated ReviewTargetStatusResult
		args := slices.Clone(statusArgs)
		args[4] = "review-unrelated"
		out = read(&unrelated, args...)
		if unrelated.NextTransition == nil || unrelated.NextTransition.ReasonCode != "rdd_disabled" || bytes.Contains(out, []byte("gentle-ai review repair")) {
			t.Fatalf("terminal handoff escaped into an rdd_disabled stop: %s", out)
		}
	})
	var inspection ReviewInspectAuthorityResult
	inspectArgs := []string{"inspect-authority", "--cwd", repo}
	inspectionJSON := read(&inspection, inspectArgs...)
	if !inspection.Complete || inspection.Totals.CompactEntries != 1 {
		t.Fatalf("expected one active authority: %#v", inspection)
	}

	command, guidance := rejectedValidatorPrintedPreflight(t, closure.Action, backtickPath)
	if !strings.HasPrefix(closure.Action, "the targeted validator rejected the correction; the lineage is stopped. ") || guidance != "Compare a successor-only disposition selector's successor_lineage_id and successor_expected_revision to this closure's lineage_id and store_revision. Only on an exact match, consult `gentle-ai review repair --help`. Preflight neither authorizes nor performs release." {
		t.Fatalf("closure action lacks read-only preflight and exact-selector guidance: %q", closure.Action)
	}
	argv, err := SplitPrintedCommandWords(command)
	wantArgv := []string{"gentle-ai", "review", "repair", "--cwd=" + repo, "--contract=" + ReviewIntegrationContractV2, "--preflight=true"}
	if err != nil || !slices.Equal(argv, wantArgv) {
		t.Fatalf("closure printed unexpected preflight argv %q: %v", argv, err)
	}
	preflightArgs := argv[2:]
	base := preflightArgs[: len(preflightArgs)-1 : len(preflightArgs)-1]
	var choices ReviewRepairResult
	choicesJSON := read(&choices, preflightArgs...)
	if choices.DispositionProviderInputs != nil || len(choices.DispositionSelectors) != 1 {
		t.Fatalf("selectorless preflight must offer an exact choice, not authority: %#v", choices)
	}
	selector := choices.DispositionSelectors[0]
	if selector.SuccessorLineageID != closure.LineageID || selector.SuccessorExpectedRevision != closure.StoreRevision || selector.PredecessorLineageID != "" || selector.PredecessorExpectedRevision != "" {
		t.Fatalf("incorrect selector: %#v", selector)
	}
	description := status.Forecast.Steps[0].Description
	statusCommand, statusBinding := rejectedValidatorPrintedPreflight(t, description, backtickPath)
	if statusCommand != command || !strings.Contains(statusBinding, "authority.lineage_id and authority.revision") || strings.Contains(statusBinding, "closure") {
		t.Fatalf("bound STATUS lacks read-only preflight for its own authority: %q", description)
	}
	statusArgv, err := SplitPrintedCommandWords(statusCommand)
	if err != nil || !slices.Equal(statusArgv, argv) {
		t.Fatalf("STATUS printed unexpected preflight argv %q: %v", statusArgv, err)
	}
	var statusChoices ReviewRepairResult
	var statusPreflightJSON []byte
	if backtickPath {
		git, err := exec.LookPath("git")
		if err != nil || !filepath.IsAbs(git) {
			t.Fatalf("expected absolute path to fixture Git: %q, %v", git, err)
		}
		if pandoc, err := exec.LookPath("pandoc"); err == nil {
			for _, text := range []string{closure.Action, description} {
				render := exec.CommandContext(ctx, pandoc, "--from=commonmark", "--to=html")
				render.Stdin = strings.NewReader(text)
				out, err := render.Output()
				_, code, found := strings.Cut(string(out), "<code>")
				literal, _, closed := strings.Cut(code, "</code>")
				if err != nil || !found || !closed || html.UnescapeString(literal) != command {
					t.Fatalf("CommonMark did not render the complete command: %v\n%s", err, out)
				}
			}
		} else {
			t.Log("pandoc unavailable; skipped CommonMark rendering smoke check")
		}
		for _, printed := range []string{command, statusCommand} {
			shell := exec.CommandContext(ctx, "/bin/sh", "-c", printed)
			shell.Dir = t.TempDir() // The printed --cwd must be sufficient.
			shell.Env = append(shell.Environ(), "PATH="+filepath.Dir(binary)+string(os.PathListSeparator)+filepath.Dir(git),
				"DO_NOT_TRACK=1", "GENTLE_AI_TELEMETRY=0", "GENTLE_AI_NO_SELF_UPDATE=1")
			var stderr bytes.Buffer
			shell.Stderr = &stderr
			out, err := shell.Output()
			if err != nil || stderr.Len() != 0 || !bytes.Equal(out, choicesJSON) {
				t.Fatalf("verbatim preflight differed: err=%v stdout=%s stderr=%s", err, out, &stderr)
			}
			statusPreflightJSON = out
		}
		decodeStrictReviewJSON(t, statusPreflightJSON, &statusChoices)
	} else {
		statusPreflightJSON = read(&statusChoices, statusArgv[2:]...)
	}
	if out := statusPreflightJSON; !bytes.Equal(out, choicesJSON) ||
		statusChoices.DispositionSelectors[0].SuccessorLineageID != status.Authority.LineageID ||
		statusChoices.DispositionSelectors[0].SuccessorExpectedRevision != status.Authority.Revision {
		t.Fatalf("STATUS preflight did not find its exact authority: %s", out)
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
	const badBinding = "authority disposition plan refused: authorization does not bind to plan_digest and authority_inventory_revision"
	for _, test := range []struct{ name, authorization, cause string }{
		{"missing authorization", "", "review repair leaf authority disposition execution requires --plan-digest --inventory-revision --actor --reason --authorization; run `gentle-ai review repair --preflight` first to obtain --plan-digest and --inventory-revision"},
		{"tampered repository", strings.Replace(authorization, public.Inputs["repository_binding"], "sha256:"+strings.Repeat("0", 64), 1), badBinding},
		{"trailing newline", authorization + "\n", badBinding},
	} {
		t.Run(test.name, func(t *testing.T) {
			invocation := args
			if test.authorization != "" {
				invocation = append(args, "--authorization", test.authorization)
			}
			stdout, stderr := run(t, 1, invocation...)
			var failure ReviewIntegrationFailure
			decodeStrictReviewJSON(t, stdout, &failure)
			if failure.MutationOutcome != ReviewMutationNotStarted || failure.Code != "invalid_request" ||
				failure.Cause != test.cause ||
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
	if after, _ := run(t, 0, statusArgs...); !bytes.Equal(after, statusJSON) {
		t.Fatalf("inspection and refused executions changed terminal STATUS: %s", after)
	}
	if after, _ := run(t, 0, inspectArgs...); !bytes.Equal(after, inspectionJSON) {
		t.Fatalf("inspection and refused executions changed authority inventory: %s", after)
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

func rejectedValidatorPrintedPreflight(t *testing.T, text string, backtickPath bool) (command, guidance string) {
	t.Helper()
	_, rendered, found := strings.Cut(text, "For read-only maintainer inspection, run ")
	body := strings.TrimLeft(rendered, "`")
	delimiter := rendered[:len(rendered)-len(body)]
	command, guidance, closed := strings.Cut(body, delimiter+". ")
	if !found || !closed || delimiter == "" || strings.Contains(command, delimiter) ||
		(!backtickPath && len(delimiter) != 1) || (backtickPath && len(delimiter) <= 4) {
		t.Fatalf("preflight has an ambiguous CommonMark delimiter: %q", rendered)
	}
	return command, guidance
}
