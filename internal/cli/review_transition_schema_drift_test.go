package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const (
	reviewV2TransitionBindingSchema   = "transition-binding.schema.json"
	reviewV2TransitionExecutionSchema = "transition-execution.schema.json"
	reviewV2AcknowledgementRef        = "transition-execution.schema.json#/$defs/approved_acknowledgement_execution"
	reviewV2ExecutionRef              = "transition-execution.schema.json"
	reviewV2LastEventExecutionRef     = "transition-execution.schema.json#/$defs/last_event_status_execution"
	reviewV2StartStatusExecutionRef   = "transition-execution.schema.json#/$defs/start_status_execution"
)

func TestV2TransitionSchemasStayLocalSharedAndPackaged(t *testing.T) {
	root := reviewV2SchemaDirectory(t)
	documents := make(map[string]map[string]any)
	for _, name := range []string{
		reviewV2TransitionBindingSchema,
		reviewV2TransitionExecutionSchema,
		"start.schema.json",
		"start-v4.schema.json",
		"status.schema.json",
		"status-v4.schema.json",
		"status-v5.schema.json",
		"last-event-closure.schema.json",
	} {
		documents[name] = readReviewV2SchemaDocument(t, root, name)
	}

	for name, wantID := range map[string]string{
		reviewV2TransitionBindingSchema:   "https://gentle-ai.dev/contracts/review-integration/v2/schemas/transition-binding.schema.json",
		reviewV2TransitionExecutionSchema: "https://gentle-ai.dev/contracts/review-integration/v2/schemas/transition-execution.schema.json",
	} {
		if got := documents[name]["$id"]; got != wantID {
			t.Fatalf("%s id = %#v, want %q", name, got, wantID)
		}
	}

	executionDefs := reviewSchemaObject(t, documents[reviewV2TransitionExecutionSchema]["$defs"], reviewV2TransitionExecutionSchema+" $defs")
	standard := reviewSchemaObject(t, executionDefs["standard_execution"], "standard execution")
	standardProperties := reviewSchemaObject(t, standard["properties"], "standard execution properties")
	if got := reviewSchemaRef(t, standardProperties["binding"], "standard execution binding"); got != reviewV2TransitionBindingSchema {
		t.Fatalf("standard execution binding ref = %q, want %q", got, reviewV2TransitionBindingSchema)
	}

	startProperties := reviewSchemaObject(t, documents["start.schema.json"]["properties"], "START properties")
	if got := reviewSchemaRef(t, startProperties["acknowledgement"], "START acknowledgement"); got != reviewV2AcknowledgementRef {
		t.Fatalf("START acknowledgement ref = %q, want %q", got, reviewV2AcknowledgementRef)
	}
	startV4Properties := reviewSchemaObject(t, documents["start-v4.schema.json"]["properties"], "START v4 properties")
	if got := reviewSchemaRef(t, startV4Properties["acknowledgement"], "START v4 acknowledgement"); got != reviewV2AcknowledgementRef {
		t.Fatalf("START v4 acknowledgement ref = %q, want %q", got, reviewV2AcknowledgementRef)
	}
	startV4Continuation := reviewSchemaObject(t, startV4Properties["next_transition"], "START v4 next_transition")
	continuationProperties := reviewSchemaObject(t, startV4Continuation["properties"], "START v4 next_transition properties")
	if got := reviewSchemaRef(t, continuationProperties["execute"], "START v4 execute"); got != reviewV2StartStatusExecutionRef {
		t.Fatalf("START v4 execute ref = %q, want %q", got, reviewV2StartStatusExecutionRef)
	}
	for _, name := range []string{"status.schema.json", "status-v4.schema.json", "status-v5.schema.json"} {
		nextTransition := reviewSchemaObject(t,
			reviewSchemaObject(t, documents[name]["$defs"], name+" $defs")["next_transition"],
			name+" next_transition",
		)
		properties := reviewSchemaObject(t, nextTransition["properties"], name+" next_transition properties")
		if got := reviewSchemaRef(t, properties["execute"], name+" execute"); got != reviewV2ExecutionRef {
			t.Fatalf("%s execute ref = %q, want %q", name, got, reviewV2ExecutionRef)
		}
	}
	closureProperties := reviewSchemaObject(t, documents["last-event-closure.schema.json"]["properties"], "last-event closure properties")
	if got := reviewSchemaRef(t, closureProperties["status_continuation"], "last-event status continuation"); got != reviewV2LastEventExecutionRef {
		t.Fatalf("last-event status continuation ref = %q, want %q", got, reviewV2LastEventExecutionRef)
	}
	closureOperation := reviewSchemaObject(t, closureProperties["operation"], "last-event closure operation")
	if operations := schemaStringArray(t, closureOperation["enum"]); !reflect.DeepEqual(operations, []string{
		"review/capture-result", "review.capture-correction-plan", "review.capture-refuter", "review/capture-validation",
	}) {
		t.Fatalf("last-event closure operation vocabulary = %v, want the exact production operation spellings", operations)
	}
	if got := reviewSchemaRef(t, closureProperties["acknowledgement"], "last-event acknowledgement"); got != reviewV2AcknowledgementRef {
		t.Fatalf("last-event acknowledgement ref = %q, want %q", got, reviewV2AcknowledgementRef)
	}

	for _, name := range []string{"start.schema.json", "start-v4.schema.json", "status-v5.schema.json"} {
		defs := reviewSchemaObject(t, documents[name]["$defs"], name+" $defs")
		if _, exists := defs["acknowledgement_execution"]; exists {
			t.Fatalf("%s retains a divergent acknowledgement definition", name)
		}
	}
	for name, document := range documents {
		for _, ref := range reviewSchemaRefs(document) {
			if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
				t.Fatalf("%s has an external ref %q", name, ref)
			}
			path := strings.SplitN(ref, "#", 2)[0]
			if path == "" {
				continue
			}
			target := filepath.Clean(filepath.Join(root, filepath.Dir(name), path))
			if _, err := os.Stat(target); err != nil {
				t.Fatalf("%s local ref %q does not resolve offline: %v", name, ref, err)
			}
			if !strings.HasPrefix(path, "transition-") {
				continue
			}
			rel, err := filepath.Rel(root, target)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				t.Fatalf("%s shared transition ref %q escapes the packaged v2 schema directory", name, ref)
			}
		}
	}

	for _, name := range []string{reviewV2TransitionBindingSchema, reviewV2TransitionExecutionSchema} {
		if schema := compileWholePublishedReviewSchema(t, "v2", name); schema == nil {
			t.Fatalf("compiled %s is nil", name)
		}
	}
}

func TestV2TransitionSchemasAcceptProviderPayloadsAndRejectDrift(t *testing.T) {
	executionSchema := compileWholePublishedReviewSchema(t, "v2", reviewV2TransitionExecutionSchema)

	reviewEnabledHome(t)
	preflightRepo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, preflightRepo, "docs/preflight.md", "# Candidate\n", 0o644)
	var preflightOutput bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", preflightRepo, "--contract", ReviewIntegrationContractV2, "--next-transition",
	}, &preflightOutput); err != nil {
		t.Fatalf("preflight STATUS: %v\n%s", err, preflightOutput.String())
	}
	var preflight ReviewTargetStatusResult
	decodeStrictReviewJSON(t, preflightOutput.Bytes(), &preflight)
	if preflight.NextTransition == nil || preflight.NextTransition.Execute == nil || preflight.NextTransition.Execute.Operation != "review.start" {
		t.Fatalf("preflight transition = %#v, want executable START", preflight.NextTransition)
	}
	validateReviewTransitionExecutionSchema(t, executionSchema, preflight.NextTransition.Execute)

	startRepo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, startRepo, "docs/approved.md", "# Approved\n", 0o644)
	var startOutput bytes.Buffer
	if err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--cwd", startRepo, "--contract", ReviewIntegrationContractV2, "--lineage", "transition-schema-start",
	}), &startOutput); err != nil {
		t.Fatalf("approved START: %v\n%s", err, startOutput.String())
	}
	startSchema := compileWholePublishedReviewSchema(t, "v2", "start-v4.schema.json")
	validatePublishedReviewSchema(t, startSchema, startOutput.Bytes())
	var started ReviewIntegrationStartResult
	decodeStrictReviewJSON(t, startOutput.Bytes(), &started)
	if started.Acknowledgement == nil || started.State != reviewtransaction.StateApproved || started.Action != "closed" {
		t.Fatalf("approved START = %#v, want pending acknowledgement", started)
	}
	validateReviewTransitionExecutionSchema(t, executionSchema, started.Acknowledgement)

	var statusOutput bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", startRepo, "--lineage", started.LineageID,
		"--contract", ReviewIntegrationContractV2, "--next-transition",
	}, &statusOutput); err != nil {
		t.Fatalf("approved STATUS: %v\n%s", err, statusOutput.String())
	}
	statusV5Schema := compileWholeNativeStatusSchema(t, "status-v5.schema.json")
	validatePublishedReviewSchema(t, statusV5Schema, statusOutput.Bytes())
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &status)
	if status.NextTransition == nil || status.NextTransition.Execute == nil || status.NextTransition.ReasonCode != "approved_acknowledgement_required" {
		t.Fatalf("approved STATUS transition = %#v", status.NextTransition)
	}
	if !reflect.DeepEqual(started.Acknowledgement, status.NextTransition.Execute) {
		t.Fatalf("START acknowledgement = %#v, STATUS acknowledgement = %#v", started.Acknowledgement, status.NextTransition.Execute)
	}
	validateReviewTransitionExecutionSchema(t, executionSchema, status.NextTransition.Execute)
	for _, name := range []string{"status.schema.json", "status-v4.schema.json", "status-v5.schema.json"} {
		payload, err := json.Marshal(status.NextTransition)
		if err != nil {
			t.Fatal(err)
		}
		validateAgainstPublishedStatusNextTransitionSchema(t, "v2", name, payload)
	}

	closureRepo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, closureRepo, "candidate.go", "package candidate\n\nfunc value() int { return 1 }\n", 0o644)
	closureStart := runNegotiatedReviewStart(t, closureRepo, "transition-schema-closure")
	var closureOutput bytes.Buffer
	for order := range closureStart.SelectedLenses {
		destination := &bytes.Buffer{}
		if order == len(closureStart.SelectedLenses)-1 {
			destination = &closureOutput
		}
		captureCleanCLIReviewerResult(t, closureRepo, ReviewFacadeStartResult{
			LineageID: closureStart.LineageID, TargetIdentity: closureStart.RepositoryContext.TargetIdentity, SelectedLenses: closureStart.SelectedLenses,
		}, order, destination)
	}
	closureSchema := compileWholePublishedReviewSchema(t, "v2", "last-event-closure.schema.json")
	validatePublishedReviewSchema(t, closureSchema, closureOutput.Bytes())
	var closure reviewLastEventClosureResult
	decodeStrictReviewJSON(t, closureOutput.Bytes(), &closure)
	if closure.Acknowledgement == nil || closure.State != reviewtransaction.StateApproved {
		t.Fatalf("last-event closure = %#v, want pending acknowledgement", closure)
	}
	validateReviewTransitionExecutionSchema(t, executionSchema, closure.Acknowledgement)

	var closureStatusOutput bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", closureRepo, "--lineage", closureStart.LineageID,
		"--contract", ReviewIntegrationContractV2, "--next-transition",
	}, &closureStatusOutput); err != nil {
		t.Fatalf("last-event STATUS: %v\n%s", err, closureStatusOutput.String())
	}
	var closureStatus ReviewTargetStatusResult
	decodeStrictReviewJSON(t, closureStatusOutput.Bytes(), &closureStatus)
	if closureStatus.NextTransition == nil || !reflect.DeepEqual(closure.Acknowledgement, closureStatus.NextTransition.Execute) {
		t.Fatalf("last-event acknowledgement = %#v, STATUS acknowledgement = %#v", closure.Acknowledgement, closureStatus.NextTransition)
	}

	for _, mutate := range []struct {
		name  string
		apply func(map[string]any)
	}{
		{name: "malformed binding", apply: func(document map[string]any) {
			document["binding"].(map[string]any)["target_identity"] = "sha256:invalid"
		}},
		{name: "wrong acknowledgement token", apply: func(document map[string]any) {
			document["arguments"].([]any)[4].(map[string]any)["value"] = "invalid"
		}},
		{name: "wrong approved precondition", apply: func(document map[string]any) {
			document["preconditions"].([]any)[0].(map[string]any)["value"] = "reviewing"
		}},
		{name: "unknown acknowledgement field", apply: func(document map[string]any) {
			document["unexpected"] = true
		}},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			document := reviewTransitionExecutionDocument(t, closure.Acknowledgement)
			mutate.apply(document)
			if err := executionSchema.Validate(document); err == nil {
				t.Fatalf("transition execution schema accepted %s: %#v", mutate.name, document)
			}
		})
	}
}

func TestV2LastEventStatusContinuationHasItsOwnStrictExecutionShape(t *testing.T) {
	closureSchema := compileWholePublishedReviewSchema(t, "v2", "last-event-closure.schema.json")
	const revision = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	continuation := reviewCorrectionStatusContinuation("/frozen/repository", reviewtransaction.CompactState{
		LineageID: "last-event-status",
		InitialSnapshot: reviewtransaction.Snapshot{
			Kind: reviewtransaction.TargetCurrentChanges, Identity: revision,
		},
	}, revision, "")
	if continuation == nil {
		t.Fatal("last-event correction continuation was not constructed")
	}
	closure := map[string]any{
		"schema":              reviewLastEventClosureSchema,
		"operation":           "review/capture-result",
		"lineage_id":          "last-event-status",
		"state":               string(reviewtransaction.StateCorrectionRequired),
		"action":              "candidate-caused severe findings require one bounded correction",
		"status_continuation": reviewTransitionExecutionDocument(t, continuation),
		"store_revision":      revision,
	}
	if err := closureSchema.Validate(closure); err != nil {
		t.Fatalf("strict last-event closure rejected the production status continuation: %v\n%#v", err, closure)
	}
	closure["status_continuation"].(map[string]any)["operation"] = "review.start"
	if err := closureSchema.Validate(closure); err == nil {
		t.Fatalf("last-event closure accepted a non-status continuation: %#v", closure)
	}
}

func reviewV2SchemaDirectory(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "contracts", "review-integration", "v2", "schemas")
}

func readReviewV2SchemaDocument(t *testing.T, root, name string) map[string]any {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return document
}

func reviewSchemaObject(t *testing.T, value any, label string) map[string]any {
	t.Helper()
	document, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s = %#v, want object", label, value)
	}
	return document
}

func reviewSchemaRef(t *testing.T, value any, label string) string {
	t.Helper()
	ref, ok := reviewSchemaObject(t, value, label)["$ref"].(string)
	if !ok {
		t.Fatalf("%s has no string $ref: %#v", label, value)
	}
	return ref
}

func reviewSchemaRefs(value any) []string {
	var refs []string
	var visit func(any)
	visit = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			if ref, ok := value["$ref"].(string); ok {
				refs = append(refs, ref)
			}
			for _, child := range value {
				visit(child)
			}
		case []any:
			for _, child := range value {
				visit(child)
			}
		}
	}
	visit(value)
	return refs
}

func validateReviewTransitionExecutionSchema(t *testing.T, schema *jsonschema.Schema, execution *ReviewTransitionExecution) {
	t.Helper()
	document := reviewTransitionExecutionDocument(t, execution)
	if err := schema.Validate(document); err != nil {
		t.Fatalf("transition execution schema rejected provider payload: %v\n%#v", err, document)
	}
}

func reviewTransitionExecutionDocument(t *testing.T, execution *ReviewTransitionExecution) map[string]any {
	t.Helper()
	payload, err := json.Marshal(execution)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatal(err)
	}
	return document
}
