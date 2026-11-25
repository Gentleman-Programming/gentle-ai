package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/gentleman-programming/gentle-ai/v5/internal/reviewtransaction"
)

func TestPiTargetedValidatorMaterializedAdmission(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the CLI against disposable text fixtures")
	}
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	binary := filepath.Join(t.TempDir(), "gentle-ai")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.CommandContext(t.Context(), "go", "build", "-buildvcs=false", "-o", binary, "./cmd/gentle-ai")
	build.Dir = filepath.Join("..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}

	for _, tc := range []struct {
		name, refusal string
		state         reviewtransaction.State
		schemaValid   bool
	}{
		{name: "completed approval", state: reviewtransaction.StateApproved, schemaValid: true},
		{name: "completed rejection", state: reviewtransaction.StateEscalated, schemaValid: true},
		{name: "legacy rejection", state: reviewtransaction.StateEscalated, schemaValid: true},
		{name: "absent follow-ups", refusal: "provider targeted validator result requires passed checks and an explicit follow_ups array"},
		{name: "null follow-ups", refusal: "provider targeted validator result requires passed checks and an explicit follow_ups array"},
		{name: "wrong request echo", refusal: "targeted validation result does not bind the provider-owned correction request", schemaValid: true},
		{name: "wrong target echo", refusal: "targeted validation result does not bind the provider-owned correction request", schemaValid: true},
		{name: "unnamed regression", refusal: "targeted validator reported a regression verdict without naming any regression"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, lineage, _ := providerCorrectionReadyWithoutVerificationEvidence(t)
			invoke := func(args ...string) ([]byte, string, int) {
				t.Helper()
				cmd := exec.CommandContext(t.Context(), binary, append([]string{"review"}, args...)...)
				cmd.Dir = repo
				cmd.Env = append(os.Environ(), "DO_NOT_TRACK=1", "GENTLE_AI_TELEMETRY=0", "GENTLE_AI_NO_SELF_UPDATE=1")
				var stdout, stderr bytes.Buffer
				cmd.Stdout, cmd.Stderr = &stdout, &stderr
				if err := cmd.Run(); cmd.ProcessState == nil {
					t.Fatalf("cannot run %s: %v", args[0], err)
				}
				return stdout.Bytes(), stderr.String(), cmd.ProcessState.ExitCode()
			}
			run := func(wantCode int, wantStderr string, args ...string) []byte {
				t.Helper()
				stdout, stderr, code := invoke(args...)
				if code != wantCode || stderr != wantStderr {
					t.Fatalf("want exit %d and stderr %q, got exit %d\nstdout=%s\nstderr=%s", wantCode, wantStderr, code, stdout, stderr)
				}
				return stdout
			}
			statusArgs := []string{"status", "--cwd", repo, "--lineage", lineage, "--contract", ReviewIntegrationContractV2, "--agent", "pi", "--next-transition"}
			before := run(0, "", statusArgs...)
			var status ReviewTargetStatusResult
			decodeStrictReviewJSON(t, before, &status)
			if status.Authority == nil || status.NextTransition == nil || status.NextTransition.ReasonCode != "targeted_validation_required" ||
				status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 {
				t.Fatalf("expected one offered validator slot: %s", before)
			}
			input := status.NextTransition.Collect.Inputs[0]
			submission := input.Submission
			if input.CaptureOperation != "review.capture-validation" || submission == nil || submission.Value == nil || input.ValidationRequest == nil {
				t.Fatalf("expected Pi validator materialize/submit descriptor: %s", before)
			}
			args := []string{submission.OperationToken}
			for _, arg := range input.Arguments {
				args = append(args, arg.Token)
			}
			prompt := run(0, "", args...)
			_, schemaJSON, _ := bytes.Cut(prompt, []byte("\n\nOutput schema:\n"))
			var schemaDoc any
			decodeStrictReviewJSON(t, schemaJSON, &schemaDoc)
			compiler := jsonschema.NewCompiler()
			compiler.UseRegexpEngine(reviewSchemaRegexpEngine)
			if err := compiler.AddResource(input.Schema, schemaDoc); err != nil {
				t.Fatal(err)
			}
			schema, err := compiler.Compile(input.Schema)
			if err != nil {
				t.Fatal(err)
			}
			if got := run(0, "", statusArgs...); !bytes.Equal(got, before) {
				t.Fatalf("materialization changed STATUS: %s", got)
			}

			var result map[string]any
			decodeStrictReviewJSON(t, providerTargetedValidationPayload(t, *input.ValidationRequest), &result)
			original, regression := result["original_criteria"].(map[string]any), result["correction_regression"].(map[string]any)
			if strings.HasPrefix(tc.name, "completed") {
				original["inspection"] = map[string]any{"status": "completed"}
				regression["inspection"] = map[string]any{"status": "completed"}
			}
			switch tc.name {
			case "completed rejection", "legacy rejection":
				original["regressions"] = []any{}
				regression["passed"] = false
				regression["evidence"] = []any{"tracked.txt:5 replaces the old terminal marker"}
				regression["regressions"] = []any{map[string]any{"location": "tracked.txt:5", "claim": "The fixed marker breaks a consumer expecting four", "proof_refs": []any{"tracked.txt:5 frozen diff"}}}
				result["follow_ups"] = []any{map[string]any{"observation": "Check downstream marker consumers", "proof_refs": []any{"tracked.txt:5"}}}
			case "absent follow-ups":
				delete(result, "follow_ups")
			case "null follow-ups":
				result["follow_ups"] = nil
			case "wrong request echo":
				result["targeted_validation_request_hash"] = input.ValidationRequest.CorrectionTargetIdentity
			case "wrong target echo":
				result["correction_target_identity"] = input.ValidationRequest.RequestHash
			case "unnamed regression":
				regression["passed"] = false
			}
			if err := schema.Validate(result); (err == nil) != tc.schemaValid {
				t.Fatalf("want printed schema valid=%t, got %v", tc.schemaValid, err)
			}
			payload, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			args = append([]string{submission.OperationToken}, submission.ArgumentTokens...)
			i := submission.Value.SubstitutionLocation + 1
			args[i] = strings.ReplaceAll(args[i], "{{value}}", writeReviewCLIRawInput(t, payload))
			if tc.refusal != "" {
				var failure ReviewIntegrationFailure
				decodeStrictReviewJSON(t, run(1, "Error: "+tc.refusal+" [invalid_request]\n", args...), &failure)
				if failure.Code != "invalid_request" || failure.Cause != tc.refusal || failure.MutationOutcome != ReviewMutationNotStarted || failure.NextAction != "correct_request" {
					t.Fatalf("expected an actionable refusal before mutation: %+v", failure)
				}
				if got := run(0, "", statusArgs...); !bytes.Equal(got, before) {
					t.Fatalf("refusal changed bound STATUS: %s", got)
				}
				return
			}
			stdout, stderr, code := invoke(args...)
			if code != 0 || stderr != "" {
				t.Fatalf("wanted admission, got exit %d (STATUS unchanged=%t)\nstdout=%s\nstderr=%s", code, bytes.Equal(run(0, "", statusArgs...), before), stdout, stderr)
			}
			var closure reviewLastEventClosureResult
			decodeStrictReviewJSON(t, stdout, &closure)
			if closure.Operation != "review/capture-validation" || closure.State != tc.state || closure.LineageID != lineage || closure.StoreRevision == status.Authority.Revision {
				t.Fatalf("unexpected validator closure: %s", stdout)
			}
			if tc.state == reviewtransaction.StateApproved {
				if closure.Acknowledgement == nil || closure.Escalation != nil {
					t.Fatalf("approval did not offer acknowledgement: %s", stdout)
				}
				return
			}
			var after ReviewTargetStatusResult
			decodeStrictReviewJSON(t, run(0, "", statusArgs...), &after)
			if closure.Escalation == nil || closure.Escalation.Cause != "targeted_validator_rejected" || closure.Acknowledgement != nil ||
				after.Authority == nil || after.Authority.Revision != closure.StoreRevision || after.NextTransition == nil || after.NextTransition.ReasonCode != "native_stop_required" {
				t.Fatalf("rejection did not stop on the admitted evidence: %s", stdout)
			}
			delete(original, "regressions") // empty optional array is omitted from the canonical evidence
			delete(original, "inspection")
			delete(regression, "inspection")
			canonical, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			var want reviewtransaction.CompactTargetedValidatorEvidence
			decodeStrictReviewJSON(t, canonical, &want)
			if !reflect.DeepEqual(closure.TargetedValidatorEvidence, &want) {
				t.Fatalf("rejection changed observed evidence: %s", stdout)
			}
		})
	}
}
