package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

func TestRetiredBindSDDRejectsBothPublishedContractsWithoutAnEnvelope(t *testing.T) {
	for _, contract := range []string{ReviewIntegrationContractV1, ReviewIntegrationContractV2} {
		t.Run(contract, func(t *testing.T) {
			var output bytes.Buffer
			err := RunReview([]string{"bind-sdd", "--contract", contract, "--cwd", t.TempDir()}, &output)
			if err == nil || !strings.Contains(err.Error(), "unknown review command") {
				t.Fatalf("retired bind-sdd error = %v", err)
			}
			if output.Len() != 0 {
				t.Fatalf("retired bind-sdd emitted an operation envelope: %q", output.String())
			}
		})
	}
}

func TestNegotiatedReviewOperationsRejectInvalidContractsBeforeMutation(t *testing.T) {
	reviewEnabledHome(t)
	for _, contract := range []string{"", "gentle-ai.review-integration/v3"} {
		t.Run("validate_"+contract, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			writeNegotiatedOperationChange(t, repo, "thin")
			lineage := "review-invalid-read-boundary"
			fixture := seedHistoricalCompatibilityApprovedCompactReceipt(t, repo, lineage, reviewtransaction.Target{
				Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
			})
			store := fixture.Store
			for _, call := range []struct {
				name string
				args []string
			}{
				{name: "validate", args: []string{"validate", "--contract=" + contract, "--cwd", repo, "--lineage", lineage, "--gate", string(reviewtransaction.GatePostApply)}},
			} {
				var output bytes.Buffer
				err := RunReview(call.args, &output)
				if err == nil {
					t.Fatalf("invalid %s contract %q = %q, %v", call.name, contract, output.String(), err)
				}
				failure := decodeReviewIntegrationFailure(t, output.Bytes())
				wantCode := "unsupported_contract"
				if contract == "" {
					wantCode = "empty_contract"
				}
				if failure.Code != wantCode || failure.MutationOutcome != ReviewMutationNotStarted {
					t.Fatalf("invalid %s contract %q failure = %#v", call.name, contract, failure)
				}
			}
			assertArtifactRevision(t, store, fixture.Record.Revision)
			if _, err := os.Stat(reviewOperationBindingPath(store, "thin")); !os.IsNotExist(err) {
				t.Fatalf("invalid contract published runtime state: %v", err)
			}
		})
	}
}

func TestNegotiatedReviewOperationSchemaAndFixturesAreStrict(t *testing.T) {
	root := filepath.Join("..", "..", "contracts", "review-integration", "v1")
	schemaPayload, err := os.ReadFile(filepath.Join(root, "schemas", "operation.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(schemaPayload, &schema); err != nil {
		t.Fatal(err)
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" ||
		schema["$id"] != ReviewIntegrationOperationSchemaID || schema["additionalProperties"] != false {
		t.Fatalf("operation schema header = %#v", schema)
	}
	payload, err := os.ReadFile(filepath.Join(root, "fixtures", "operation.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	envelope := decodeReviewOperationEnvelope(t, payload)
	if err := envelope.Validate(); err != nil {
		t.Fatal(err)
	}
	assertNoPrivateReviewOperationFields(t, payload)
}

func startReviewOperationFixture(t *testing.T, repo, lineage string) ReviewFacadeStartResult {
	t.Helper()
	startedBytes, err := runLegacyFacadeStartForTestBytes(t, []string{"--cwd", repo, "--lineage", lineage})
	if err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, startedBytes, &started)
	return started
}

func writeNegotiatedOperationChange(t *testing.T, repo, change string) {
	t.Helper()
	for path, content := range map[string]string{
		"tasks.md": "- [x] 1.1 Done\n", "proposal.md": "# Proposal\n", "design.md": "# Design\n", "specs/binding/spec.md": "# Spec\n",
	} {
		writeReviewStartCandidate(t, repo, "openspec/changes/"+change+"/"+path, content, 0o644)
	}
}

func decodeReviewOperationEnvelope(t *testing.T, payload []byte) ReviewIntegrationOperationResult {
	t.Helper()
	var envelope ReviewIntegrationOperationResult
	decodeStrictReviewJSON(t, payload, &envelope)
	if err := envelope.Validate(); err != nil {
		t.Fatal(err)
	}
	return envelope
}

func decodeStrictReviewJSON(t *testing.T, payload []byte, target any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		t.Fatal(err)
	}
}

func strictReviewJSONFields(t *testing.T, payload []byte) []string {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sortStrings(names)
	return names
}

func assertNoPrivateReviewOperationFields(t *testing.T, payload []byte) {
	t.Helper()
	var document any
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatal(err)
	}
	forbidden := map[string]struct{}{
		"model": {}, "provider": {}, "profile": {}, "cwd": {}, "repository": {}, "store_path": {}, "receipt_path": {}, "binding_path": {},
	}
	if field := findCapabilityForbiddenField(document, forbidden); field != "" {
		t.Fatalf("negotiated operation exposed private field %q: %s", field, payload)
	}
}

func readReviewOperationFile(t *testing.T, path string) []byte {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func reviewOperationBindingPath(store reviewtransaction.CompactStore, change string) string {
	common := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(store.Dir))))
	return filepath.Join(common, "gentle-ai", "sdd-runtime", "v1", change, "HEAD")
}

// Issue gentle-shell#213: a read-only STATUS that exceeds its aggregate budget
// is the very command that completes the authority inventory, so its timeout
// must stay retryable. A STATUS carrying repair or recovery authorization
// inputs keeps the conservative stop route.
func TestNegotiatedStatusAggregateTimeoutRetriesOnlyReadOnlyRequests(t *testing.T) {
	originalRunner := reviewFacadeCommandRunner
	originalTimeout := reviewFacadeOperationTimeout
	t.Cleanup(func() {
		reviewFacadeCommandRunner = originalRunner
		reviewFacadeOperationTimeout = originalTimeout
	})
	reviewFacadeOperationTimeout = 25 * time.Millisecond
	reviewFacadeCommandRunner = func(context.Context, []string, io.Writer) error { time.Sleep(100 * time.Millisecond); return nil }
	for _, tt := range []struct {
		name          string
		args          []string
		retrySafe     bool
		replayability reviewtransaction.Replayability
		nextAction    string
	}{
		{
			name: "read only", args: []string{"status", "--contract", ReviewIntegrationContractV1},
			retrySafe: true, replayability: reviewtransaction.ReplayabilityNotReplayable, nextAction: "retry",
		},
		{
			name: "read only projection selectors", args: []string{"status", "--contract", ReviewIntegrationContractV1, "--lineage", "status-timeout", "--next-transition", "--action-eligibility"},
			retrySafe: true, replayability: reviewtransaction.ReplayabilityNotReplayable, nextAction: "retry",
		},
		{
			name: "repair authorization", args: []string{"status", "--contract", ReviewIntegrationContractV1, "--repair-actor", "maintainer", "--repair-reason", "fix"},
			replayability: reviewtransaction.ReplayabilityManualActionRequired, nextAction: "stop",
		},
		{
			name: "recovery authorization", args: []string{"status", "--contract", ReviewIntegrationContractV1, "--recovery-successor-lineage=successor", "--recovery-actor", "maintainer"},
			replayability: reviewtransaction.ReplayabilityManualActionRequired, nextAction: "stop",
		},
		{
			name: "unparseable arguments", args: []string{"status", "--contract", ReviewIntegrationContractV1, "--unknown-flag"},
			replayability: reviewtransaction.ReplayabilityManualActionRequired, nextAction: "stop",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := RunReview(tt.args, &output); err == nil {
				t.Fatal("timed out STATUS succeeded")
			}
			failure := decodeReviewIntegrationFailure(t, output.Bytes())
			if failure.Operation != "review.status" || failure.Code != "operation_timeout" || failure.Phase != "pre_native" ||
				failure.MutationOutcome != ReviewMutationNotStarted || failure.AuthorityApplicability != "not_evaluated" ||
				failure.RetrySafe != tt.retrySafe || failure.Replayability != tt.replayability || failure.NextAction != tt.nextAction {
				t.Fatalf("STATUS timeout failure = %#v", failure)
			}
			if err := failure.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Every other operation keeps its pre-#213 timeout classification: mutating
// requests route to STATUS with an unknown outcome, everything else stops.
func TestReviewOperationTimeoutClassificationUnchangedOutsideReadOnlyStatus(t *testing.T) {
	for _, metadata := range reviewIntegrationOperationRegistry {
		if metadata.Operation == "review.status" {
			continue
		}
		argSets := [][]string{nil}
		if metadata.ReadOnlyFlag != "" {
			argSets = append(argSets, []string{"--" + metadata.ReadOnlyFlag + "=true"}, []string{"--" + metadata.ReadOnlyFlag + "=false"})
		}
		for _, args := range argSets {
			t.Run(metadata.Operation+"/"+strings.Join(args, " "), func(t *testing.T) {
				failure := reviewOperationTimeoutFailure(ReviewIntegrationFailure{Operation: metadata.Operation}, metadata.Operation, args)
				want := ReviewIntegrationFailure{
					Operation: metadata.Operation, Code: "operation_timeout",
					Message: "The negotiated review operation exceeded its aggregate time budget.",
					Phase:   "pre_native", MutationOutcome: ReviewMutationNotStarted, AuthorityApplicability: "not_evaluated",
					Replayability: reviewtransaction.ReplayabilityManualActionRequired, NextAction: "stop",
				}
				if reviewIntegrationOperationMutates(metadata, args) {
					want.Phase, want.MutationOutcome = "native_running", ReviewMutationUnknown
					want.Replayability, want.NextAction = reviewtransaction.ReplayabilityStatusRequired, "review.status"
				}
				if !reflect.DeepEqual(failure, want) {
					t.Fatalf("timeout failure = %#v, want %#v", failure, want)
				}
			})
		}
	}
}

// Pins the independent verifier's end-to-end probes for gentle-shell#213: the
// real STATUS route, with exclusive maintenance held so every request reaches
// the aggregate deadline. Read-only selectors stay retryable; any repair-* or
// recovery-* authorization input, or an argument outside the STATUS flag
// shape, keeps the stop route; argument forms that only look like
// authorization inputs are refused at preflight, never classified as a
// retryable timeout.
func TestNegotiatedStatusDeadlineRetryTruthUnderMaintenanceLock(t *testing.T) {
	repo := initReviewCLIRepo(t)
	started := startFacadeReview(t, repo)
	lockContext, cancelLock := context.WithTimeout(context.Background(), time.Second)
	defer cancelLock()
	exclusive, err := reviewtransaction.AcquireReviewMaintenanceExclusive(lockContext, repo)
	if err != nil {
		t.Fatal(err)
	}
	defer exclusive.Release()

	originalTimeout := reviewFacadeOperationTimeout
	t.Cleanup(func() { reviewFacadeOperationTimeout = originalTimeout })
	reviewFacadeOperationTimeout = 50 * time.Millisecond

	base := []string{"status", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", started.LineageID}
	retry := func(extra ...string) []string { return extra }
	for _, tt := range []struct {
		name  string
		extra []string
		code  string
		retry bool
		next  string
	}{
		{name: "plain", extra: retry(), code: "operation_timeout", retry: true, next: "retry"},
		{name: "gate", extra: retry("--gate", "pre-commit"), code: "operation_timeout", retry: true, next: "retry"},
		{name: "action eligibility", extra: retry("--action-eligibility"), code: "operation_timeout", retry: true, next: "retry"},
		{name: "next transition", extra: retry("--next-transition"), code: "operation_timeout", retry: true, next: "retry"},
		{name: "phase timings", extra: retry("--phase-timings"), code: "operation_timeout", retry: true, next: "retry"},
		{name: "escalate and lenses", extra: retry("--next-transition", "--escalate-item", "2", "--escalate-reason", "weakens a permission check", "--lenses", "risk", "--lenses-reason", "touches authority"), code: "operation_timeout", retry: true, next: "retry"},

		{name: "repair actor", extra: []string{"--repair-actor", "maintainer"}, code: "operation_timeout", next: "stop"},
		{name: "repair reason", extra: []string{"--repair-reason", "fix"}, code: "operation_timeout", next: "stop"},
		{name: "repair authorization", extra: []string{"--repair-authorization", "token"}, code: "operation_timeout", next: "stop"},
		{name: "recovery successor lineage", extra: []string{"--recovery-successor-lineage", "successor"}, code: "operation_timeout", next: "stop"},
		{name: "recovery reason", extra: []string{"--recovery-reason", "fix"}, code: "operation_timeout", next: "stop"},
		{name: "recovery actor", extra: []string{"--recovery-actor", "maintainer"}, code: "operation_timeout", next: "stop"},
		{name: "recovery authorization", extra: []string{"--recovery-authorization", "token"}, code: "operation_timeout", next: "stop"},
		{name: "single dash equals", extra: []string{"-repair-actor=maintainer"}, code: "operation_timeout", next: "stop"},
		{name: "empty repair value", extra: []string{"--repair-reason="}, code: "operation_timeout", next: "stop"},
		{name: "empty recovery value", extra: []string{"--recovery-successor-lineage="}, code: "operation_timeout", next: "stop"},
		{name: "repository context", extra: []string{"--repository-context", "context"}, code: "operation_timeout", next: "stop"},

		{name: "positional authorization", extra: []string{"positional", "--repair-authorization", "x"}, code: "invalid_request", retry: true, next: "correct_request"},
		{name: "after terminator", extra: []string{"--", "--recovery-authorization", "x"}, code: "invalid_request", retry: true, next: "correct_request"},
		{name: "bool value consumed as positional", extra: []string{"--committed-only", "true", "--repair-authorization", "x"}, code: "invalid_request", retry: true, next: "correct_request"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := RunReview(append(append([]string{}, base...), tt.extra...), &output); err == nil {
				t.Fatal("STATUS succeeded while exclusive maintenance held")
			}
			failure := decodeReviewIntegrationFailure(t, output.Bytes())
			phase := "pre_native"
			if tt.code == "invalid_request" {
				phase = "preflight"
			}
			if failure.Operation != "review.status" || failure.Code != tt.code || failure.Phase != phase ||
				failure.MutationOutcome != ReviewMutationNotStarted || failure.RetrySafe != tt.retry || failure.NextAction != tt.next {
				t.Fatalf("STATUS deadline failure = %#v", failure)
			}
			if tt.code != "operation_timeout" && failure.NextAction == "retry" {
				t.Fatalf("deceptive authorization form classified as a retryable timeout: %#v", failure)
			}
			if err := failure.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
