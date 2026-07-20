package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

func TestNegotiatedReviewStatusReportsFreshStartAndPreservesGlobalStatus(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate behavior\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var startedOutput bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "review-status-fixture"}, &startedOutput); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(startedOutput.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	var global bytes.Buffer
	if err := RunReview([]string{"status", "--cwd", repo}, &global); err != nil {
		t.Fatal(err)
	}
	var globalFields map[string]json.RawMessage
	if err := json.Unmarshal(global.Bytes(), &globalFields); err != nil {
		t.Fatal(err)
	}
	wantGlobalFields := []string{"authoritative", "complete", "diagnostics", "entries", "locks", "operation", "repository", "schema", "status"}
	gotGlobalFields := make([]string, 0, len(globalFields))
	for field := range globalFields {
		gotGlobalFields = append(gotGlobalFields, field)
	}
	sortStrings(gotGlobalFields)
	if !reflect.DeepEqual(gotGlobalFields, wantGlobalFields) {
		t.Fatalf("unnegotiated status fields = %v, want %v\n%s", gotGlobalFields, wantGlobalFields, global.String())
	}

	args := []string{"status", "--contract", ReviewIntegrationContractV1, "--action-eligibility", "--cwd", repo}
	var first, second bytes.Buffer
	if err := RunReview(args, &first); err != nil {
		t.Fatal(err)
	}
	if err := RunReview(args, &second); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatalf("negotiated status changed after restart:\nfirst=%s\nsecond=%s", first.String(), second.String())
	}
	var status ReviewTargetStatusResult
	decoder := json.NewDecoder(bytes.NewReader(first.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&status); err != nil {
		t.Fatal(err)
	}
	if err := status.Validate(); err != nil {
		t.Fatal(err)
	}
	if status.Schema != ReviewIntegrationStatusSchema || status.Contract != ReviewIntegrationContractV1 || status.Operation != "review.status" ||
		status.Applicability != reviewtransaction.TargetApplicabilityCurrent || status.Authority == nil ||
		status.Authority.State != reviewtransaction.StateReviewing || status.Authority.LineageID != started.LineageID ||
		status.Receipt.Status != ReviewReceiptExpectedMissing || status.Receipt.Identity != "" ||
		status.Action != reviewtransaction.TargetStatusActionFinalize || status.Replayability != reviewtransaction.ReplayabilityNotReplayable {
		t.Fatalf("fresh START negotiated status = %#v\n%s", status, first.String())
	}
	if status.Frozen == nil || status.Frozen.Tier != started.RiskLevel || status.Frozen.OriginalChangedLines != started.ChangedLines || status.Frozen.CorrectionBudget != started.CorrectionBudget ||
		status.Projection.Schema != ReviewIntegrationProjectionSchema || !reflect.DeepEqual(status.Projection.Paths, []string{"tracked.txt"}) || status.Projection.IntendedUntracked == nil {
		t.Fatalf("restart projection = %#v", status)
	}
	var document any
	if err := json.Unmarshal(first.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	forbidden := map[string]struct{}{
		"repository": {}, "store_path": {}, "authority_path": {}, "receipt_path": {}, "lock": {}, "locks": {}, "token": {}, "tokens": {}, "directory": {},
	}
	if field := findCapabilityForbiddenField(document, forbidden); field != "" || strings.Contains(first.String(), repo) {
		t.Fatalf("negotiated status exposed provider-private field %q: %s", field, first.String())
	}

	fixture, err := os.ReadFile(filepath.Join("..", "..", "contracts", "review-integration", "v1", "fixtures", "status.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), fixture) {
		t.Fatalf("status fixture mismatch:\ngot=%s\nwant=%s", first.String(), fixture)
	}
	var denied bytes.Buffer
	err = RunReview([]string{"validate", "--cwd", repo, "--lineage", started.LineageID, "--gate", string(reviewtransaction.GatePostApply)}, &denied)
	if err == nil || !strings.Contains(err.Error(), "receipt is not available") || denied.Len() != 0 {
		t.Fatalf("fresh START validate result = %q, %v", denied.String(), err)
	}
	after, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("negotiated status or denied validate mutated authority")
	}
	if _, err := os.Stat(store.ReceiptPath()); !os.IsNotExist(err) {
		t.Fatalf("fresh START unexpectedly materialized receipt: %v", err)
	}
}

func TestNegotiatedReviewStatusContractAndSchemasAreStrict(t *testing.T) {
	repo := initReviewCLIRepo(t)
	var output bytes.Buffer
	err := RunReview([]string{"status", "--contract", "gentle-ai.review-integration/v2", "--cwd", repo}, &output)
	if err == nil {
		t.Fatalf("unsupported status contract = %q, %v", output.String(), err)
	}
	if failure := decodeReviewIntegrationFailure(t, output.Bytes()); failure.Code != "unsupported_contract" {
		t.Fatalf("unsupported status contract failure = %#v", failure)
	}
	root := filepath.Join("..", "..", "contracts", "review-integration", "v1")
	for _, item := range []struct {
		name string
		id   string
	}{
		{name: "status.schema.json", id: ReviewIntegrationStatusSchemaID},
		{name: "projection.schema.json", id: ReviewIntegrationProjectionSchemaID},
	} {
		payload, readErr := os.ReadFile(filepath.Join(root, "schemas", item.name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		var schema map[string]any
		if err := json.Unmarshal(payload, &schema); err != nil {
			t.Fatal(err)
		}
		if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" || schema["$id"] != item.id || schema["additionalProperties"] != false {
			t.Fatalf("%s header = %#v", item.name, schema)
		}
	}
	fixtures := []struct {
		name          string
		applicability reviewtransaction.TargetApplicability
	}{
		{name: "status.fixture.json", applicability: reviewtransaction.TargetApplicabilityCurrent},
		{name: "status-recover.fixture.json", applicability: reviewtransaction.TargetApplicabilityCurrent},
		{name: "status-unrelated.fixture.json", applicability: reviewtransaction.TargetApplicabilityUnrelated},
		{name: "status-ambiguous.fixture.json", applicability: reviewtransaction.TargetApplicabilityAmbiguous},
		{name: "status-corrupted.fixture.json", applicability: reviewtransaction.TargetApplicabilityCorrupted},
	}
	for _, item := range fixtures {
		fixture, readErr := os.ReadFile(filepath.Join(root, "fixtures", item.name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		decoder := json.NewDecoder(bytes.NewReader(fixture))
		decoder.DisallowUnknownFields()
		var status ReviewTargetStatusResult
		if err := decoder.Decode(&status); err != nil {
			t.Fatal(err)
		}
		if err := status.Validate(); err != nil {
			t.Fatalf("%s: %v", item.name, err)
		}
		if status.Applicability != item.applicability {
			t.Fatalf("%s applicability = %q", item.name, status.Applicability)
		}
		if status.Applicability == reviewtransaction.TargetApplicabilityCurrent && status.Authority != nil && status.Authority.Version == reviewtransaction.AuthorityVersionCompact {
			withoutFrozen := status
			withoutFrozen.Frozen = nil
			if err := withoutFrozen.Validate(); err == nil {
				t.Fatalf("%s accepted compact current target without frozen inputs", item.name)
			}
		}
	}
}

func TestNegotiatedPendingFinalizeStatusMatchesPublishedSchema(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	request := facadeFinalizeAttemptRequest(record, nil, nil, facadeRefuterResult{}, nil, 0, false)
	if _, _, err := store.BeginFinalizeAttempt(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReview([]string{"status", "--contract", ReviewIntegrationContractV1, "--action-eligibility", "--cwd", repo, "--lineage", started.LineageID}, &output); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&status); err != nil {
		t.Fatal(err)
	}
	if err := status.Validate(); err != nil {
		t.Fatal(err)
	}
	if status.Action != reviewtransaction.TargetStatusActionReconcileFinalize || status.Replayability != reviewtransaction.ReplayabilityStatusRequired || status.Reconciliation == nil || !status.Reconciliation.Required {
		t.Fatalf("pending negotiated status = %#v", status)
	}
	if status.Eligibility == nil || len(status.Eligibility.AllowedActions) != 1 || status.Eligibility.AllowedActions[0].Action != "stop" ||
		status.Eligibility.AllowedActions[0].ReasonCode != reviewActionForbiddenReconciliation {
		t.Fatalf("pending journal advertised replay guidance: %#v", status.Eligibility)
	}
	forbiddenFinalize := false
	for _, forbidden := range status.Eligibility.ForbiddenActions {
		forbiddenFinalize = forbiddenFinalize || forbidden.Action == "review.finalize" && forbidden.ReasonCode == reviewActionForbiddenReconciliation
	}
	if !forbiddenFinalize {
		t.Fatalf("pending journal did not forbid lineage-only finalize replay: %#v", status.Eligibility)
	}
	assertStatusPayloadMatchesPublishedSchema(t, output.Bytes())
	nonReconciliation := status
	nonReconciliation.Action = reviewtransaction.TargetStatusActionFinalize
	if err := nonReconciliation.Validate(); err == nil || !strings.Contains(err.Error(), "only pending finalize") {
		t.Fatalf("non-reconciliation status accepted reconciliation metadata: %v", err)
	}
	unrelated := status
	unrelated.Applicability = reviewtransaction.TargetApplicabilityUnrelated
	unrelated.Authority = nil
	unrelated.Frozen = nil
	unrelated.Receipt = ReviewTargetStatusReceipt{Status: ReviewReceiptNotApplicable}
	unrelated.Candidates = []string{}
	if err := unrelated.Validate(); err == nil {
		t.Fatal("unrelated target accepted reconcile_finalize")
	}
}

func TestActionEligibilityIsOptionalForV1Consumers(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	startFacadeReview(t, repo)
	type v1Status struct {
		Schema         string                                `json:"schema"`
		Contract       string                                `json:"contract"`
		Operation      string                                `json:"operation"`
		Applicability  reviewtransaction.TargetApplicability `json:"applicability"`
		Authority      *ReviewTargetStatusAuthority          `json:"authority,omitempty"`
		Receipt        ReviewTargetStatusReceipt             `json:"receipt"`
		Action         reviewtransaction.TargetStatusAction  `json:"action"`
		Replayability  reviewtransaction.Replayability       `json:"replayability"`
		Frozen         *ReviewTargetStatusFrozen             `json:"frozen,omitempty"`
		TargetIdentity string                                `json:"target_identity"`
		Projection     ReviewTargetStatusProjection          `json:"projection"`
		Candidates     []string                              `json:"candidates"`
	}
	var legacyOutput bytes.Buffer
	if err := RunReview([]string{"status", "--contract", ReviewIntegrationContractV1, "--cwd", repo}, &legacyOutput); err != nil {
		t.Fatal(err)
	}
	var legacy v1Status
	decodeStrictReviewJSON(t, legacyOutput.Bytes(), &legacy)
	var oldPayload ReviewTargetStatusResult
	decodeStrictReviewJSON(t, legacyOutput.Bytes(), &oldPayload)
	if err := oldPayload.Validate(); err != nil || oldPayload.Eligibility != nil {
		t.Fatalf("old valid v1 payload = %#v, %v", oldPayload, err)
	}
	var currentOutput bytes.Buffer
	if err := RunReview([]string{"status", "--contract", ReviewIntegrationContractV1, "--action-eligibility", "--cwd", repo}, &currentOutput); err != nil {
		t.Fatal(err)
	}
	var current ReviewTargetStatusResult
	decodeStrictReviewJSON(t, currentOutput.Bytes(), &current)
	if current.Eligibility == nil {
		t.Fatalf("new v1 payload = %#v", current)
	}
}

func TestReviewActionEligibilityStopsWithoutCompleteExecutionInputs(t *testing.T) {
	base := func(applicability reviewtransaction.TargetApplicability, state reviewtransaction.State, action reviewtransaction.TargetStatusAction, replayability reviewtransaction.Replayability) ReviewTargetStatusResult {
		native := reviewtransaction.TargetStatusResult{
			Applicability: applicability, AuthorityVersion: reviewtransaction.AuthorityVersionCompact,
			LineageID: "status-input-matrix", State: state, Generation: 1,
			Revision: "sha256:" + strings.Repeat("a", 64), OriginalChangedLines: 2, Tier: reviewtransaction.RiskMedium, CorrectionBudget: 1,
			Action: action, Replayability: replayability,
		}
		status := newReviewTargetStatusResult(native)
		status.TargetIdentity = "sha256:" + strings.Repeat("b", 64)
		status.Projection = publishedStatusFixtureProjection(t)
		status.Eligibility = newReviewActionEligibility(status)
		return status
	}
	for _, tt := range []struct {
		name          string
		applicability reviewtransaction.TargetApplicability
		state         reviewtransaction.State
		action        reviewtransaction.TargetStatusAction
		replayability reviewtransaction.Replayability
		allowed       string
		forbidden     string
	}{
		{"unrelated workspace start", reviewtransaction.TargetApplicabilityUnrelated, "", reviewtransaction.TargetStatusActionStart, reviewtransaction.ReplayabilityNotReplayable, reviewActionForbiddenInputsUnavailable, reviewActionForbiddenUnrelated},
		{"unrelated staged start", reviewtransaction.TargetApplicabilityUnrelated, "", reviewtransaction.TargetStatusActionStart, reviewtransaction.ReplayabilityNotReplayable, reviewActionForbiddenInputsUnavailable, reviewActionForbiddenUnrelated},
		{"unrelated base ref start", reviewtransaction.TargetApplicabilityUnrelated, "", reviewtransaction.TargetStatusActionStart, reviewtransaction.ReplayabilityNotReplayable, reviewActionForbiddenInputsUnavailable, reviewActionForbiddenUnrelated},
		{"unrelated overlay start", reviewtransaction.TargetApplicabilityUnrelated, "", reviewtransaction.TargetStatusActionStart, reviewtransaction.ReplayabilityNotReplayable, reviewActionForbiddenInputsUnavailable, reviewActionForbiddenUnrelated},
		{"reviewing selected lenses", reviewtransaction.TargetApplicabilityCurrent, reviewtransaction.StateReviewing, reviewtransaction.TargetStatusActionFinalize, reviewtransaction.ReplayabilityNotReplayable, reviewActionForbiddenInputsUnavailable, reviewActionForbiddenInputsUnavailable},
		{"pending finalize journal", reviewtransaction.TargetApplicabilityCurrent, reviewtransaction.StateValidating, reviewtransaction.TargetStatusActionReconcileFinalize, reviewtransaction.ReplayabilityStatusRequired, reviewActionForbiddenReconciliation, reviewActionForbiddenReconciliation},
		{"scope changed finalize", reviewtransaction.TargetApplicabilityCurrent, reviewtransaction.StateCorrectionRequired, reviewtransaction.TargetStatusActionFinalize, reviewtransaction.ReplayabilityNotReplayable, reviewActionForbiddenInputsUnavailable, reviewActionForbiddenInputsUnavailable},
		{"terminal validation", reviewtransaction.TargetApplicabilityCurrent, reviewtransaction.StateApproved, reviewtransaction.TargetStatusActionValidate, reviewtransaction.ReplayabilityNotReplayable, reviewActionForbiddenInputsUnavailable, reviewActionForbiddenInputsUnavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			status := base(tt.applicability, tt.state, tt.action, tt.replayability)
			if err := status.Validate(); err != nil {
				t.Fatal(err)
			}
			allowed := status.Eligibility.AllowedActions
			if len(allowed) != 1 || allowed[0].Action != "stop" || allowed[0].ReasonCode != tt.allowed || len(allowed[0].RequiredInputs) != 0 {
				t.Fatalf("eligibility = %#v", status.Eligibility)
			}
			forbiddenStart := false
			for _, forbidden := range status.Eligibility.ForbiddenActions {
				forbiddenStart = forbiddenStart || forbidden.Action == "review.start" && forbidden.ReasonCode == tt.forbidden
			}
			if !forbiddenStart {
				t.Fatalf("missing expected forbidden guidance: %#v", status.Eligibility)
			}
		})
	}
}

func TestNegotiatedReviewFinalizeEligibilityRequiresTargetScopedStatus(t *testing.T) {
	for _, state := range []reviewtransaction.State{
		reviewtransaction.StateReviewing,
		reviewtransaction.StateCorrectionRequired,
		reviewtransaction.StateValidating,
		reviewtransaction.StateApproved,
	} {
		t.Run(string(state), func(t *testing.T) {
			var output bytes.Buffer
			if err := encodeCompactFacadeFinalize(&output, true, true, reviewtransaction.CompactState{LineageID: "review-finalize-eligibility", State: state}, "sha256:"+strings.Repeat("a", 64), reviewtransaction.CompactStore{}, "finalize"); err != nil {
				t.Fatal(err)
			}
			var result ReviewIntegrationFinalizeResult
			decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, output.Bytes()).Result, &result)
			if result.Eligibility == nil || result.Eligibility.ValidateFinalize() != nil {
				t.Fatalf("finalize eligibility = %#v", result.Eligibility)
			}
		})
	}
}

func assertStatusPayloadMatchesPublishedSchema(t *testing.T, payload []byte) {
	t.Helper()
	schemaPath := filepath.Join("..", "..", "contracts", "review-integration", "v1", "schemas", "status.schema.json")
	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Required   []string                   `json:"required"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatal(err)
	}
	for _, required := range schema.Required {
		if _, ok := document[required]; !ok {
			t.Fatalf("status response misses schema-required property %q: %s", required, payload)
		}
	}
	for field := range document {
		if _, ok := schema.Properties[field]; !ok {
			t.Fatalf("status response has property %q outside strict schema: %s", field, payload)
		}
	}
	var reconciliation struct {
		Required bool `json:"required"`
	}
	if err := json.Unmarshal(document["reconciliation"], &reconciliation); err != nil || !reconciliation.Required {
		t.Fatalf("status response violates reconcile_finalize schema branch: %s (%v)", payload, err)
	}
	if !bytes.Contains(schemaBytes, []byte(`"applicability": { "const": "current_target" }`)) ||
		!bytes.Contains(schemaBytes, []byte(`"else": { "not": { "required": ["reconciliation"] } }`)) {
		t.Fatal("published status schema does not constrain reconciliation to current_target/reconcile_finalize")
	}
}

func TestReviewTargetStatusProjectionRejectsNonCanonicalRepositoryPaths(t *testing.T) {
	valid := ReviewTargetStatusProjection{
		Schema: ReviewIntegrationProjectionSchema, Projection: reviewtransaction.ProjectionWorkspace,
		BaseTree: strings.Repeat("a", 40), InitialReviewTree: strings.Repeat("b", 40), CurrentCandidateTree: strings.Repeat("c", 40),
		PathsDigest: "sha256:" + strings.Repeat("a", 64), IntendedUntrackedProof: "sha256:" + strings.Repeat("b", 64),
		InitialSnapshotIdentity: "sha256:" + strings.Repeat("c", 64), CurrentSnapshotIdentity: "sha256:" + strings.Repeat("d", 64),
		Paths: []string{"nested/file.go"}, IntendedUntracked: []string{},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("canonical nested path rejected: %v", err)
	}
	for _, value := range []string{`nested\file.go`, `/absolute`, `C:/volume`, `C:\volume`, `//server/share`, `.`, `./file`, `../file`, `nested/../file`, `nested//file`} {
		t.Run(value, func(t *testing.T) {
			projection := valid
			projection.Paths = []string{value}
			if err := projection.Validate(); err == nil {
				t.Fatalf("non-canonical path %q accepted", value)
			}
		})
	}
}

func TestNegotiatedStatusAcceptsHistoricalApprovedOrdinary4RWithoutCompactFrozenInputs(t *testing.T) {
	tests := []struct {
		name          string
		mutateReceipt func(t *testing.T, path string)
		wantCurrent   bool
	}{
		{name: "canonical receipt", wantCurrent: true},
		{name: "missing receipt", mutateReceipt: func(t *testing.T, path string) {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "corrupt receipt", mutateReceipt: func(t *testing.T, path string) {
			if err := os.WriteFile(path, []byte("{\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lineage := "historical-status-" + strings.ReplaceAll(tt.name, " ", "-")
			fixture := newLegacyCLIFixture(t, lineage)
			if tt.mutateReceipt != nil {
				tt.mutateReceipt(t, fixture.receiptPath)
			}
			before := readLegacyAuthorityTree(t, fixture.store.Dir)
			args := []string{"status", "--contract", ReviewIntegrationContractV1, "--cwd", fixture.repo, "--lineage", lineage}
			var first, second bytes.Buffer
			if err := RunReview(args, &first); err != nil {
				t.Fatalf("historical negotiated status: %v\n%s", err, first.String())
			}
			if err := RunReview(args, &second); err != nil {
				t.Fatalf("historical negotiated status restart: %v\n%s", err, second.String())
			}
			if !bytes.Equal(first.Bytes(), second.Bytes()) {
				t.Fatalf("historical status changed across restart:\n%s\n%s", first.String(), second.String())
			}
			var status ReviewTargetStatusResult
			decodeStrictReviewJSON(t, first.Bytes(), &status)
			if err := status.Validate(); err != nil {
				t.Fatalf("historical negotiated status validation: %v\n%s", err, first.String())
			}
			if status.Frozen != nil {
				t.Fatalf("historical ordinary_4r invented compact frozen inputs: %#v", status.Frozen)
			}
			if tt.wantCurrent {
				identity, err := reviewtransaction.HashArtifact(fixture.receiptPath)
				if err != nil {
					t.Fatal(err)
				}
				if status.Applicability != reviewtransaction.TargetApplicabilityCurrent || status.Authority == nil ||
					status.Authority.Version != reviewtransaction.AuthorityVersionLegacy || status.Authority.State != reviewtransaction.StateApproved ||
					status.Receipt.Status != ReviewReceiptPresent || status.Receipt.Identity != identity ||
					status.Action != reviewtransaction.TargetStatusActionValidate || status.Replayability != reviewtransaction.ReplayabilityNotReplayable ||
					strings.Contains(first.String(), string(ReviewReceiptPublicationPending)) {
					t.Fatalf("historical approved status = %#v\n%s", status, first.String())
				}
			} else if status.Applicability != reviewtransaction.TargetApplicabilityCorrupted || status.Authority != nil ||
				status.Receipt.Status != ReviewReceiptNotApplicable || status.Action != reviewtransaction.TargetStatusActionRepairAuthority ||
				status.Replayability != reviewtransaction.ReplayabilityManualActionRequired {
				t.Fatalf("invalid historical receipt status = %#v\n%s", status, first.String())
			}
			if after := readLegacyAuthorityTree(t, fixture.store.Dir); !reflect.DeepEqual(before, after) {
				t.Fatal("historical negotiated status mutated legacy authority bytes")
			}
		})
	}
}

func TestNegotiatedRuntimeReplaysPublishedV149AuthorityReadOnly(t *testing.T) {
	repo, authorityRoot, receiptPath := newPublishedV149CLIRepo(t)
	before := readLegacyAuthorityTree(t, authorityRoot)
	args := []string{"status", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", "legacy-valid"}
	var first, second bytes.Buffer
	if err := RunReview(args, &first); err != nil {
		t.Fatalf("published v1.49 negotiated status: %v\n%s", err, first.String())
	}
	if err := RunReview(args, &second); err != nil {
		t.Fatalf("published v1.49 negotiated status restart: %v\n%s", err, second.String())
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatalf("published v1.49 status changed across restart:\n%s\n%s", first.String(), second.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, first.Bytes(), &status)
	if err := status.Validate(); err != nil {
		t.Fatalf("published v1.49 status validation: %v\n%s", err, first.String())
	}
	receiptIdentity, err := reviewtransaction.HashArtifact(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if status.Applicability != reviewtransaction.TargetApplicabilityCurrent || status.Authority == nil ||
		status.Authority.Version != reviewtransaction.AuthorityVersionLegacy || status.Authority.State != reviewtransaction.StateApproved ||
		status.Frozen != nil || status.Receipt.Status != ReviewReceiptPresent || status.Receipt.Identity != receiptIdentity ||
		status.Action != reviewtransaction.TargetStatusActionValidate || status.Replayability != reviewtransaction.ReplayabilityNotReplayable {
		t.Fatalf("published v1.49 negotiated status = %#v\n%s", status, first.String())
	}
	if afterStatus := readLegacyAuthorityTree(t, authorityRoot); !reflect.DeepEqual(before, afterStatus) {
		t.Fatal("published v1.49 status mutated authority bytes")
	}

	writeNegotiatedOperationChange(t, repo, "thin")
	var bind bytes.Buffer
	err = RunReview([]string{
		"bind-sdd", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--change", "thin",
		"--lineage", "legacy-valid", "--expected-binding-revision=",
	}, &bind)
	if err == nil {
		t.Fatalf("published v1.49 bind-sdd succeeded: %s", bind.String())
	}
	failure := decodeReviewIntegrationFailure(t, bind.Bytes())
	if failure.Operation != ReviewIntegrationOperationBindSDD || failure.Code != reviewtransaction.LegacyReadOnlyErrorCode ||
		failure.MutationOutcome != ReviewMutationNotStarted || failure.RetrySafe ||
		failure.Replayability != reviewtransaction.ReplayabilityNotReplayable || failure.NextAction != "stop" {
		t.Fatalf("published v1.49 bind-sdd failure = %#v\n%s", failure, bind.String())
	}
	var typed *reviewtransaction.LegacyReadOnlyError
	if !errors.Is(err, reviewtransaction.ErrLegacyReadOnly) || !errors.As(err, &typed) ||
		typed.Operation != "review/bind-sdd" || typed.LineageID != "legacy-valid" {
		t.Fatalf("published v1.49 bind-sdd lost typed cause: %#v", err)
	}
	if afterBind := readLegacyAuthorityTree(t, authorityRoot); !reflect.DeepEqual(before, afterBind) {
		t.Fatal("published v1.49 bind-sdd mutated authority bytes")
	}
}

func TestNegotiatedStatusPreservesManualRecoveryAuthorityContext(t *testing.T) {
	native := reviewtransaction.TargetStatusResult{
		Applicability: reviewtransaction.TargetApplicabilityCurrent, AuthorityVersion: reviewtransaction.AuthorityVersionCompact,
		LineageID: "historical-validator", State: reviewtransaction.StateCorrectionRequired, Generation: 1, Revision: "sha256:" + strings.Repeat("a", 64),
		Action: reviewtransaction.TargetStatusActionRecover, ActionDisposition: reviewtransaction.RecoveryEscalated,
		Replayability: reviewtransaction.ReplayabilityManualActionRequired,
	}
	got := newReviewTargetStatusResult(native)
	if got.Action != reviewtransaction.TargetStatusActionRecover || got.Replayability != reviewtransaction.ReplayabilityManualActionRequired ||
		got.ActionDisposition != reviewtransaction.RecoveryEscalated ||
		got.Authority == nil || got.Authority.LineageID != native.LineageID || got.Authority.Revision != native.Revision {
		t.Fatalf("negotiated recovery status = %#v", got)
	}
}

// TestNegotiatedStatusBindsRecoveryDispositionToRecoverAction pins issue #1469
// Case B at the published contract boundary: a recover recommendation must
// carry the disposition recovery accepts, and nothing else may carry one.
func TestNegotiatedStatusBindsRecoveryDispositionToRecoverAction(t *testing.T) {
	base := func() ReviewTargetStatusResult {
		native := reviewtransaction.TargetStatusResult{
			Applicability: reviewtransaction.TargetApplicabilityCurrent, AuthorityVersion: reviewtransaction.AuthorityVersionCompact,
			LineageID: "disposition-contract", State: reviewtransaction.StateCorrectionRequired, Generation: 1,
			Revision: "sha256:" + strings.Repeat("a", 64), OriginalChangedLines: 2, Tier: reviewtransaction.RiskMedium, CorrectionBudget: 1,
			Action: reviewtransaction.TargetStatusActionRecover, ActionDisposition: reviewtransaction.RecoveryScopeChanged,
			Replayability: reviewtransaction.ReplayabilityManualActionRequired,
		}
		result := newReviewTargetStatusResult(native)
		result.TargetIdentity = "sha256:" + strings.Repeat("b", 64)
		result.Projection = publishedStatusFixtureProjection(t)
		result.Eligibility = newReviewActionEligibility(result)
		return result
	}
	valid := base()
	if err := valid.Validate(); err != nil {
		t.Fatalf("scope-changed recover status rejected: %v", err)
	}
	for _, disposition := range []reviewtransaction.RecoveryDisposition{
		reviewtransaction.RecoveryScopeChanged, reviewtransaction.RecoveryInvalidated, reviewtransaction.RecoveryEscalated,
	} {
		accepted := base()
		accepted.ActionDisposition = disposition
		accepted.Eligibility = newReviewActionEligibility(accepted)
		if err := accepted.Validate(); err != nil {
			t.Fatalf("recover status rejected disposition %q: %v", disposition, err)
		}
	}
	missing := base()
	missing.ActionDisposition = ""
	if err := missing.Validate(); err == nil {
		t.Fatal("recover status accepted without a recovery disposition")
	}
	unsupported := base()
	unsupported.ActionDisposition = reviewtransaction.RecoveryDisposition("corrected")
	if err := unsupported.Validate(); err == nil {
		t.Fatal("recover status accepted an unsupported recovery disposition")
	}
	misplaced := base()
	misplaced.Action = reviewtransaction.TargetStatusActionFinalize
	misplaced.Replayability = reviewtransaction.ReplayabilityNotReplayable
	if err := misplaced.Validate(); err == nil {
		t.Fatal("finalize status accepted a recovery disposition")
	}
	misplaced.ActionDisposition = ""
	misplaced.Eligibility = newReviewActionEligibility(misplaced)
	if err := misplaced.Validate(); err != nil {
		t.Fatalf("finalize status without a disposition rejected: %v", err)
	}
}

func TestReviewActionEligibilityFailsClosedForEscalatedAuthority(t *testing.T) {
	base := func(state reviewtransaction.State, action reviewtransaction.TargetStatusAction, disposition reviewtransaction.RecoveryDisposition, replayability reviewtransaction.Replayability) ReviewTargetStatusResult {
		native := reviewtransaction.TargetStatusResult{
			Applicability: reviewtransaction.TargetApplicabilityCurrent, AuthorityVersion: reviewtransaction.AuthorityVersionCompact,
			LineageID: "escalated-authority", State: state, Generation: 2,
			Revision: "sha256:" + strings.Repeat("a", 64), OriginalChangedLines: 2, Tier: reviewtransaction.RiskMedium, CorrectionBudget: 1,
			Action: action, ActionDisposition: disposition, Replayability: replayability,
		}
		status := newReviewTargetStatusResult(native)
		status.TargetIdentity = "sha256:" + strings.Repeat("b", 64)
		status.Projection = publishedStatusFixtureProjection(t)
		status.Eligibility = newReviewActionEligibility(status)
		return status
	}
	for _, tt := range []struct {
		name            string
		state           reviewtransaction.State
		action          reviewtransaction.TargetStatusAction
		disposition     reviewtransaction.RecoveryDisposition
		replayability   reviewtransaction.Replayability
		wantAllowed     string
		wantAbandonCode string
	}{
		{"terminal captured authority stops", reviewtransaction.StateEscalated, reviewtransaction.TargetStatusActionStop, "", reviewtransaction.ReplayabilityManualActionRequired, "stop", reviewActionForbiddenTerminalEscalated},
		{"unchanged escalated candidate stops", reviewtransaction.StateCorrectionRequired, reviewtransaction.TargetStatusActionStop, "", reviewtransaction.ReplayabilityManualActionRequired, "stop", reviewActionForbiddenUnchangedEscalated},
		{"materially changed candidate recovers explicitly", reviewtransaction.StateCorrectionRequired, reviewtransaction.TargetStatusActionRecover, reviewtransaction.RecoveryEscalated, reviewtransaction.ReplayabilityManualActionRequired, "review.recover", reviewActionForbiddenNotSelected},
		{"pending correction stops", reviewtransaction.StateCorrectionRequired, reviewtransaction.TargetStatusActionReconcileFinalize, "", reviewtransaction.ReplayabilityStatusRequired, "stop", reviewActionForbiddenReconciliation},
		{"pending validation stops", reviewtransaction.StateValidating, reviewtransaction.TargetStatusActionReconcileFinalize, "", reviewtransaction.ReplayabilityStatusRequired, "stop", reviewActionForbiddenReconciliation},
		{"terminal receipt replay remains exact", reviewtransaction.StateApproved, reviewtransaction.TargetStatusActionFinalize, "", reviewtransaction.ReplayabilityExactReplaySafe, "review.finalize", reviewActionForbiddenNotSelected},
	} {
		t.Run(tt.name, func(t *testing.T) {
			status := base(tt.state, tt.action, tt.disposition, tt.replayability)
			if err := status.Validate(); err != nil {
				t.Fatal(err)
			}
			allowed := status.Eligibility.AllowedActions
			if len(allowed) != 1 || allowed[0].Action != tt.wantAllowed {
				t.Fatalf("allowed actions = %#v", allowed)
			}
			if tt.action == reviewtransaction.TargetStatusActionReconcileFinalize && allowed[0].ReasonCode != reviewActionForbiddenReconciliation ||
				tt.replayability == reviewtransaction.ReplayabilityExactReplaySafe && !reflect.DeepEqual(allowed[0].RequiredInputs, []string{"lineage_id"}) {
				t.Fatalf("replay eligibility = %#v", allowed[0])
			}
			if tt.wantAllowed == "review.recover" {
				if allowed[0].Disposition != reviewtransaction.RecoveryEscalated || allowed[0].Binding == nil ||
					allowed[0].Binding.LineageID != status.Authority.LineageID || allowed[0].Binding.Revision != status.Authority.Revision ||
					allowed[0].Binding.TargetIdentity != status.TargetIdentity {
					t.Fatalf("escalated recovery binding = %#v", allowed[0])
				}
			}
			for _, forbidden := range status.Eligibility.ForbiddenActions {
				if forbidden.Action == "review.abandon" && forbidden.ReasonCode != tt.wantAbandonCode {
					t.Fatalf("abandon eligibility = %#v", forbidden)
				}
			}
		})
	}
	status := base(reviewtransaction.StateCorrectionRequired, reviewtransaction.TargetStatusActionRecover, reviewtransaction.RecoveryEscalated, reviewtransaction.ReplayabilityManualActionRequired)
	status.Eligibility.AllowedActions[0].Binding = nil
	if err := status.Validate(); err == nil {
		t.Fatal("fabricated recovery binding was accepted")
	}
}

func TestReviewFinalizeEligibilityCannotPublishRecoveryBinding(t *testing.T) {
	eligibility := ReviewActionEligibility{
		AllowedActions:   []ReviewEligibleAction{{Action: "review.recover", ReasonCode: reviewActionEligibleEscalatedRecovery, RequiredInputs: []string{}, Disposition: reviewtransaction.RecoveryEscalated, Binding: &ReviewActionBinding{}}},
		ForbiddenActions: []ReviewForbiddenAction{},
	}
	if err := eligibility.ValidateFinalize(); err == nil {
		t.Fatal("finalize eligibility accepted a recovery binding")
	}
}

func publishedStatusFixtureProjection(t *testing.T) ReviewTargetStatusProjection {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join("..", "..", "contracts", "review-integration", "v1", "fixtures", "status.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture ReviewTargetStatusResult
	if err := json.Unmarshal(payload, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture.Projection
}

func TestNegotiatedLegacyReceiptStatusNeverUsesCompactPublicationPending(t *testing.T) {
	native := reviewtransaction.TargetStatusResult{
		Applicability:    reviewtransaction.TargetApplicabilityCurrent,
		AuthorityVersion: reviewtransaction.AuthorityVersionLegacy,
		State:            reviewtransaction.StateApproved,
		ReceiptIdentity:  "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	present := newReviewTargetStatusResult(native)
	if present.Receipt.Status != ReviewReceiptPresent || present.Receipt.Identity != native.ReceiptIdentity {
		t.Fatalf("approved legacy receipt = %#v", present.Receipt)
	}
	native.ReceiptIdentity = ""
	missing := newReviewTargetStatusResult(native)
	if missing.Receipt.Status == ReviewReceiptPublicationPending || missing.Receipt.Identity != "" {
		t.Fatalf("legacy receipt inherited compact publication semantics: %#v", missing.Receipt)
	}
}

func sortStrings(values []string) {
	for index := 1; index < len(values); index++ {
		for cursor := index; cursor > 0 && values[cursor] < values[cursor-1]; cursor-- {
			values[cursor], values[cursor-1] = values[cursor-1], values[cursor]
		}
	}
}

func newPublishedV149CLIRepo(t *testing.T) (string, string, string) {
	t.Helper()
	repo := t.TempDir()
	runReviewCLIGit(t, repo, "init", "-q")
	runReviewCLIGit(t, repo, "config", "user.email", "test@example.com")
	runReviewCLIGit(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("baseline\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "base")
	if tree := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD^{tree}")); tree != "8c3de935e475844d1dbdaf8a4e68c5ef3d2e7b97" {
		t.Fatalf("published fixture base tree = %s", tree)
	}
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CandidateTree != "551f003a86ddd6ab07e08888e482f3136baa49c6" || !reflect.DeepEqual(snapshot.Paths, []string{"tracked.txt"}) {
		t.Fatalf("published fixture candidate snapshot = %#v", snapshot)
	}

	commonDir := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "--path-format=absolute", "--git-common-dir"))
	authorityRoot := filepath.Join(commonDir, "gentle-ai")
	destination := filepath.Join(authorityRoot, "review-transactions", "v1", "legacy-valid")
	source := filepath.Join("..", "reviewtransaction", "testdata", "v1.49.0-ordinary-4r")
	files := []string{
		"HEAD", "artifacts/receipt.json",
		"events/5608bd6bbd175cd48f0754897f1204e1cae0612d38aeb1af448d5ac4d51c0e9f.json",
		"events/9b7dc5776fcad044ac56798b9ca3c823b53a3486816c27234ff537dbde2ee0ef.json",
		"events/b7d4df583b8e1bb952c6f021e5aeb015cb837cdbf81f827007ca42c29b13278c.json",
		"events/bd3ac2bea5b0c51c7205479d680b907b5b88a88c24be899a7cf0e6843d3d23eb.json",
		"events/d4c310032d9bb4d299277dece13c029b3bae8b9728fa481558c5c2f59d8eed86.json",
	}
	for _, name := range files {
		payload, readErr := os.ReadFile(filepath.Join(source, filepath.FromSlash(name)))
		if readErr != nil {
			t.Fatal(readErr)
		}
		path := filepath.Join(destination, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, payload, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return repo, authorityRoot, filepath.Join(destination, "artifacts", "receipt.json")
}
