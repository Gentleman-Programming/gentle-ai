package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestNativeCaptureCollectArgumentsCarryRunnableTokens is the RED-first proof
// that a collect input naming an operation this product performs publishes the
// exact argv it expects. `review.capture-result` is not a logical name a caller
// has to interpret: it is `gentle-ai review capture-result`, and the input's
// arguments are literally that command's flags. Emitting only {name, value}
// left every caller re-deriving "--" + name + "=" + value by hand, which is
// where the published testing guide lost --repository-context once and then
// paired it with --cwd (a combination capture-result refuses outright).
//
// The flag names are cross-checked against the real capture-result FlagSet
// rather than a list copied into this test, so a renamed or dropped flag fails
// here instead of shipping a token that cannot run.
func TestNativeCaptureCollectArgumentsCarryRunnableTokens(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc collect() {}\n", 0o644)
	started := runNegotiatedReviewStart(t, repo, "collect-token-binding")
	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1,
		"--lineage", started.LineageID, "--next-transition",
	}, &output); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if status.NextTransition == nil || status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) == 0 {
		t.Fatalf("next transition = %#v", status.NextTransition)
	}
	input := status.NextTransition.Collect.Inputs[0]
	if input.CaptureOperation != reviewCaptureResultCaptureOperation {
		t.Fatalf("capture operation = %q, want %q", input.CaptureOperation, reviewCaptureResultCaptureOperation)
	}
	accepted := reviewVerbFlagNames(t, "capture-result")
	for _, argument := range input.Arguments {
		want := "--" + argument.Name + "=" + argument.Value
		if argument.Token != want {
			t.Errorf("collect argument %q token = %q, want %q", argument.Name, argument.Token, want)
		}
		if _, real := accepted[argument.Name]; !real {
			t.Errorf("collect argument %q is not a flag `gentle-ai review capture-result` accepts: %v", argument.Name, sortedFlagNames(accepted))
		}
	}
}

// TestExternalCaptureCollectArgumentsStayUntokenized keeps the boundary honest
// from the other side. An "external.*" capture operation is performed where
// this product does not run, so its arguments are values to hand to whoever
// performs it, not argv for a command that exists here. Rendering them as
// flags would invent a command line nothing can execute.
func TestExternalCaptureCollectArgumentsStayUntokenized(t *testing.T) {
	status := ReviewTargetStatusResult{
		Applicability:  reviewtransaction.TargetApplicabilityAmbiguous,
		TargetIdentity: "sha256:" + strings.Repeat("a", 64),
		Candidates:     []string{"review-one", "review-two"},
	}
	transition := newReviewNextTransition(status, nil, nil, nil, nil, reviewNextTransitionInput{})
	if transition.Kind != reviewNextTransitionCollect || transition.Collect == nil || len(transition.Collect.Inputs) != 1 {
		t.Fatalf("ambiguous transition = %#v", transition)
	}
	input := transition.Collect.Inputs[0]
	if !strings.HasPrefix(input.CaptureOperation, "external.") {
		t.Fatalf("capture operation = %q, want an external one", input.CaptureOperation)
	}
	for _, argument := range input.Arguments {
		if argument.Token != "" {
			t.Errorf("external collect argument %q carries token %q; it is a value to hand elsewhere, not argv", argument.Name, argument.Token)
		}
	}
	payload, err := json.Marshal(transition)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), `"token"`) {
		t.Fatalf("external collect payload = %s, want no token field", payload)
	}
}

// TestCollectCaptureOperationKindsAreClassifiedFromSource enumerates every
// capture operation the transition source actually emits and asserts each one
// is classified, rather than trusting that only the two kinds seen today will
// ever exist. A third prefix -- or a native verb that is not a real command --
// fails here instead of silently landing in whichever branch happens to match.
func TestCollectCaptureOperationKindsAreClassifiedFromSource(t *testing.T) {
	operations := captureOperationsFromTransitionSource(t)
	if len(operations) == 0 {
		t.Fatal("found no capture operations in the transition source")
	}
	native, external := 0, 0
	for _, operation := range operations {
		transition := reviewCollectTransition("capture_operation_kind_probe", ReviewTransitionInput{
			Name: "probe", Schema: "gentle-ai.review-capture-operation-probe/v1", CaptureOperation: operation,
			Arguments: []ReviewTransitionArgument{{Name: "lineage", Value: "review-probe"}},
		})
		got := transition.Collect.Inputs[0].Arguments[0].Token
		switch {
		case strings.HasPrefix(operation, "review."):
			native++
			if got != "--lineage=review-probe" {
				t.Errorf("native capture operation %q argument token = %q, want %q", operation, got, "--lineage=review-probe")
			}
			verb := strings.TrimPrefix(operation, "review.")
			if len(reviewVerbFlagNames(t, verb)) == 0 {
				t.Errorf("native capture operation %q names no runnable `gentle-ai review %s` command", operation, verb)
			}
		case strings.HasPrefix(operation, "external."):
			external++
			if got != "" {
				t.Errorf("external capture operation %q argument token = %q, want none", operation, got)
			}
		default:
			t.Errorf("capture operation %q belongs to no classified kind", operation)
		}
	}
	if native == 0 || external == 0 {
		t.Fatalf("capture operation kinds found: %d native, %d external; want both represented", native, external)
	}
}

// TestTokenizedCollectTransitionValidatesAgainstPublishedSchemas proves the
// tokenized collect payload is admissible under the exact schemas it claims,
// in both published status versions.
func TestTokenizedCollectTransitionValidatesAgainstPublishedSchemas(t *testing.T) {
	binding := ReviewTransitionBinding{
		LineageID:      "review-collect-schema",
		Revision:       "sha256:" + strings.Repeat("a", 64),
		TargetIdentity: "sha256:" + strings.Repeat("b", 64),
	}
	transition := reviewCollectTransition("verification_evidence_required", ReviewTransitionInput{
		Name: "evidence", Schema: reviewtransaction.VerificationEvidenceRecordSchema,
		CaptureOperation: "review.capture-evidence", Arguments: reviewBindingArguments(binding),
	})
	if transition.Collect.Inputs[0].Arguments[0].Token == "" {
		t.Fatal("native collect transition carries no token to validate")
	}
	payload, err := json.Marshal(transition)
	if err != nil {
		t.Fatal(err)
	}
	validateAgainstPublishedNextTransitionSchemaV2(t, payload)
	validateAgainstPublishedNextTransitionSchema(t, payload)
}

// captureOperationsFromTransitionSource reads the transition source and
// returns every capture_operation it can emit. Reading the source is
// deliberate: the point is to discover the kinds that exist rather than to
// restate a list this test already believes. An unresolvable constant fails
// closed instead of narrowing the enumeration silently.
func captureOperationsFromTransitionSource(t *testing.T) []string {
	t.Helper()
	source, err := os.ReadFile("review_next_transition.go")
	if err != nil {
		t.Fatal(err)
	}
	constants := map[string]string{"reviewCaptureResultCaptureOperation": reviewCaptureResultCaptureOperation}
	pattern := regexp.MustCompile(`CaptureOperation:\s*(?:"([^"]*)"|([A-Za-z_][A-Za-z0-9_.]*))`)
	seen := map[string]struct{}{}
	operations := make([]string, 0)
	for _, match := range pattern.FindAllStringSubmatch(string(source), -1) {
		operation := match[1]
		if operation == "" {
			resolved, known := constants[match[2]]
			if !known {
				t.Fatalf("capture operation constant %q is not resolvable by this test", match[2])
			}
			operation = resolved
		}
		if _, repeated := seen[operation]; repeated {
			continue
		}
		seen[operation] = struct{}{}
		operations = append(operations, operation)
	}
	return operations
}

// reviewVerbFlagNames returns the flag names one `gentle-ai review <verb>`
// command really accepts, read from the command's own FlagSet through its
// rendered help rather than from a list duplicated in a test.
func reviewVerbFlagNames(t *testing.T, verb string) map[string]struct{} {
	t.Helper()
	var help bytes.Buffer
	if err := RunReview([]string{verb, "--help"}, &help); err != nil {
		t.Fatalf("review %s --help: %v\n%s", verb, err, help.String())
	}
	pattern := regexp.MustCompile(`^--([a-z][a-z0-9-]*) <value>$`)
	names := map[string]struct{}{}
	for _, line := range strings.Split(help.String(), "\n") {
		if match := pattern.FindStringSubmatch(strings.TrimSpace(line)); match != nil {
			names[match[1]] = struct{}{}
		}
	}
	return names
}

func sortedFlagNames(names map[string]struct{}) []string {
	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sortStrings(sorted)
	return sorted
}
