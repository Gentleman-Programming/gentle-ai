package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// This file is the published-schema conformance suite for live envelopes: it
// drives the real CLI (or the exact envelope constructors) and validates the
// emitted bytes against the schemas published in contracts/review-integration/.
// Every test here is the pin that keeps one emitter and its published schema
// from drifting apart, the exact divergence class the cross-lane battery found.

// compileWholePublishedReviewSchema compiles one published schema file with
// every contract schema (v1 and v2) registered, so cross-file $refs resolve
// exactly as an external consumer compiling the published artifacts would.
func compileWholePublishedReviewSchema(t *testing.T, version, name string) *jsonschema.Schema {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "contracts", "review-integration"))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseRegexpEngine(reviewSchemaRegexpEngine)
	for _, contractVersion := range []string{"v1", "v2"} {
		paths, err := filepath.Glob(filepath.Join(root, contractVersion, "schemas", "*.schema.json"))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range paths {
			payload, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var document map[string]any
			if err := json.Unmarshal(payload, &document); err != nil {
				t.Fatal(err)
			}
			if err := compiler.AddResource(document["$id"].(string), document); err != nil {
				t.Fatal(err)
			}
		}
	}
	schema, err := compiler.Compile("https://gentle-ai.dev/contracts/review-integration/" + version + "/schemas/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func validatePublishedReviewSchema(t *testing.T, schema *jsonschema.Schema, payload []byte) {
	t.Helper()
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatalf("live envelope diverges from its published schema: %v\nenvelope: %s", err, payload)
	}
}

// TestNegotiatedStartEnvelopeMatchesPublishedStartSchemaV3 pins the whole live
// negotiated START envelope to the published start/v3 schema. The battery
// found the live emitter publishing repository_context.event_id and .outcome
// (real, load-bearing fields validated by
// validateReviewRepositoryContextReference) that the schema did not admit.
func TestNegotiatedStartEnvelopeMatchesPublishedStartSchemaV3(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc conformance() {}\n", 0o644)

	var output bytes.Buffer
	if err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo,
		"--lineage", "published-schema-start",
	}), &output); err != nil {
		t.Fatal(err)
	}
	started := decodeNegotiatedReviewStart(t, output.Bytes())
	if started.Schema != ReviewIntegrationStartSchema {
		t.Fatalf("negotiated START schema = %q, want %q", started.Schema, ReviewIntegrationStartSchema)
	}
	// The divergence needs the reconciled event identity present, so this
	// test proves the live emitter really publishes it before validating.
	if started.RepositoryContext == nil || started.RepositoryContext.EventID == "" || started.RepositoryContext.Outcome == "" {
		t.Fatalf("negotiated START repository context carries no event identity: %#v", started.RepositoryContext)
	}
	schema := compileWholePublishedReviewSchema(t, "v2", "start.schema.json")
	validatePublishedReviewSchema(t, schema, output.Bytes())
}

// TestNegotiatedStatusEnvelopeMatchesPublishedStatusSchemaV5 pins the whole
// live negotiated STATUS envelope, including the top-level repository_context
// reference the battery found missing from status-v5.schema.json.
func TestNegotiatedStatusEnvelopeMatchesPublishedStatusSchemaV5(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc conformance() {}\n", 0o644)
	runNegotiatedReviewStart(t, repo, "published-schema-status")

	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2,
		"--agent", string(model.AgentClaudeCode), "--next-transition",
	}, &output); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if status.Schema != ReviewIntegrationStatusSchemaV5 {
		t.Fatalf("negotiated STATUS schema = %q, want %q", status.Schema, ReviewIntegrationStatusSchemaV5)
	}
	// The divergence needs the top-level repository context present, so this
	// test proves the live emitter really publishes it before validating.
	if status.RepositoryContext == nil {
		t.Fatalf("negotiated STATUS carries no top-level repository context: %s", output.String())
	}
	schema := compileWholePublishedReviewSchema(t, "v2", "status-v5.schema.json")
	validatePublishedReviewSchema(t, schema, output.Bytes())
}

// TestStatusDispositionProjectionMatchesPublishedStatusSchemaV5 ties the
// emitter bytes of the negotiated-route disposition preview
// (ReviewRepairDispositionProviderInputs, populated by STATUS when a closed
// closure disposition plan derives) to the published status-v5 schema. Like
// the top-level repository_context the battery caught, this optional
// projection is real emitter output the strict schema must admit.
func TestStatusDispositionProjectionMatchesPublishedStatusSchemaV5(t *testing.T) {
	t.Parallel()
	fixture, err := os.ReadFile(filepath.Join("..", "..", "contracts", "review-integration", "v2", "fixtures", "status-v5.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(fixture, &document); err != nil {
		t.Fatal(err)
	}
	disposition, err := json.Marshal(ReviewRepairDispositionProviderInputs{
		PlanDigest:                 "sha256:" + strings.Repeat("a", 64),
		AuthorityInventoryRevision: "sha256:" + strings.Repeat("b", 64),
		SeedLineageID:              "review-disposition-seed",
		SeedExpectedRevision:       "sha256:" + strings.Repeat("c", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	var dispositionDocument map[string]any
	if err := json.Unmarshal(disposition, &dispositionDocument); err != nil {
		t.Fatal(err)
	}
	document["disposition"] = dispositionDocument
	payload, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	schema := compileWholePublishedReviewSchema(t, "v2", "status-v5.schema.json")
	validatePublishedReviewSchema(t, schema, payload)
}

// TestConsentFixtureMatchesPublishedConsentSchemaV3 compiles the published
// consent-v3 schema against the pinned consent-v3 fixture — the exact live
// bytes TestConsentQuestionMatchesVersionedFixture captures from the emitter.
// The battery found the schema pinning `--agent claude-code` inside the choice
// invocations while the emitter (and therefore the fixture) omits the agent
// token whenever the caller declared no runtime.
func TestConsentFixtureMatchesPublishedConsentSchemaV3(t *testing.T) {
	t.Parallel()
	payload, err := os.ReadFile(filepath.Join("..", "..", "contracts", "review-integration", "v2", "fixtures", "consent-v3.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	var question ReviewIntegrationConsentResult
	if err := json.Unmarshal(payload, &question); err != nil {
		t.Fatal(err)
	}
	for _, choice := range question.Choices {
		if strings.Contains(choice.Invocation, "--agent") {
			t.Fatalf("fixture invocation unexpectedly declares a runtime agent: %q", choice.Invocation)
		}
	}
	schema := compileWholePublishedReviewSchema(t, "v2", "consent-v3.schema.json")
	validatePublishedReviewSchema(t, schema, payload)
}

// TestConsentEnvelopeWithDeclaredRuntimeMatchesPublishedConsentSchemaV3 drives
// the live relay-declared START with each runtime the consent validator
// accepts and validates the emitted envelope, so the published agent domain
// covers every identity the emitter can actually publish (#2676). Pi joins
// the domain through its host-relay handshake: the emitter legitimately
// publishes agent "pi" once the relay contract is declared, so the published
// schema must admit it (cross-lane battery conformance finding).
func TestConsentEnvelopeWithDeclaredRuntimeMatchesPublishedConsentSchemaV3(t *testing.T) {
	schema := compileWholePublishedReviewSchema(t, "v2", "consent-v3.schema.json")
	for _, agent := range []model.AgentID{model.AgentClaudeCode, model.AgentOpenCode, model.AgentCodex, model.AgentPi} {
		t.Run(string(agent), func(t *testing.T) {
			if agent == model.AgentPi {
				t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
			}
			reviewEnabledHome(t)
			repo := initReviewCLIRepo(t)
			stubReviewConsole(t, false, "")
			writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)

			output := runConsentRelayStart(t, boundNegotiatedStartArgs(t, []string{
				"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo,
				"--agent", string(agent),
				"--lineage", "published-schema-consent", "--consent", "relay",
			}))
			question := decodeConsentQuestion(t, output.Bytes())
			if question.Agent != string(agent) {
				t.Fatalf("consent agent = %q, want %q", question.Agent, agent)
			}
			validatePublishedReviewSchema(t, schema, output.Bytes())
		})
	}
}

// TestPiHostRelayStatusEnvelopeMatchesPublishedStatusSchemaV5 pins the whole
// live negotiated STATUS envelope the Pi host relay receives: the materialize
// branch (review_next_transition.go's reviewCaptureInput) renders the
// reviewer_result collect input with a capture-result submission descriptor,
// real emitter output the published status-v5 schema must admit (cross-lane
// battery conformance finding).
func TestPiHostRelayStatusEnvelopeMatchesPublishedStatusSchemaV5(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, _, record, _ := newCandidateInspectionReview(t, "candidate\n", true)

	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", record.State.LineageID, "--contract", ReviewIntegrationContractV2,
		"--agent", string(model.AgentPi), "--next-transition",
	}, &output); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
		status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("pi host relay transition = %#v", status.NextTransition)
	}
	// The divergence needs the capture-result submission present, so this
	// test proves the live emitter really publishes it before validating.
	submission := status.NextTransition.Collect.Inputs[0].Submission
	if submission == nil || submission.OperationToken != "capture-result" {
		t.Fatalf("pi host relay submission = %#v", submission)
	}
	schema := compileWholePublishedReviewSchema(t, "v2", "status-v5.schema.json")
	validatePublishedReviewSchema(t, schema, output.Bytes())
}

// TestReviewGateResultEnvelopeMatchesPublishedSchema walks one low-risk
// lifecycle to its approved receipt, runs the delivery gate, and validates the
// emitted gentle-ai.review-gate-result/v1 envelope against its published
// schema — the envelope the battery found had no published schema at all.
func TestReviewGateResultEnvelopeMatchesPublishedSchema(t *testing.T) {
	reviewEnabledHome(t)
	// Compiled before the printed-command execution below changes the working
	// directory, because the contracts/ tree is addressed repo-relative.
	schema := compileWholePublishedReviewSchema(t, "v2", "gate-result.schema.json")
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "docs/guide.md", "# Guide\n\npurely passive documentation\n", 0o644)

	var output bytes.Buffer
	if err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo,
		"--lineage", "published-schema-gate",
	}), &output); err != nil {
		t.Fatal(err)
	}
	started := decodeNegotiatedReviewStart(t, output.Bytes())
	if started.RiskLevel != reviewtransaction.RiskLow || started.LensesRequired {
		t.Fatalf("low candidate start = risk %q lenses_required %v", started.RiskLevel, started.LensesRequired)
	}

	output.Reset()
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2,
		"--agent", string(model.AgentClaudeCode), "--next-transition",
	}, &output); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if status.NextTransition == nil || status.NextTransition.Execute == nil || status.NextTransition.Execute.Operation != "review.finalize" {
		t.Fatalf("next transition = %#v", status.NextTransition)
	}
	words := reviewShellWords(t, status.NextTransition.Execute.Command)
	if len(words) < 3 || words[0] != "gentle-ai" || words[1] != "review" {
		t.Fatalf("finalize command = %q", status.NextTransition.Execute.Command)
	}
	// The provider-rendered finalize command runs from the repository it was
	// issued for, exactly as the negotiated protocol prescribes.
	t.Chdir(repo)
	output.Reset()
	if err := RunReview(words[2:], &output); err != nil {
		t.Fatalf("finalize: %v\n%s", err, output.String())
	}

	output.Reset()
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePostApply)}, &output); err != nil {
		t.Fatalf("post-apply gate: %v\n%s", err, output.String())
	}
	assertReviewGateResult(t, output.Bytes(), reviewtransaction.GateAllow)
	var gate ReviewValidateResult
	decodeStrictReviewJSON(t, output.Bytes(), &gate)
	if gate.Schema != ReviewValidateSchema {
		t.Fatalf("gate result schema = %q, want %q", gate.Schema, ReviewValidateSchema)
	}
	validatePublishedReviewSchema(t, schema, output.Bytes())
}

// TestOpenCodeProviderRoleEnvelopeMatchesPublishedSchema validates the exact
// bytes the OpenCode transport publishes for a captured provider role result
// against its published schema — the second envelope the battery found had no
// published schema.
func TestOpenCodeProviderRoleEnvelopeMatchesPublishedSchema(t *testing.T) {
	t.Parallel()
	schema := compileWholePublishedReviewSchema(t, "v2", "opencode-provider-role.schema.json")
	for _, role := range []reviewerprovider.Role{reviewerprovider.RoleRefuter, reviewerprovider.RoleTargetedValidator} {
		payload := openCodeProviderRoleResultEnvelope(role)
		var decoded struct {
			Schema   string `json:"schema"`
			Role     string `json:"role"`
			Captured bool   `json:"captured"`
		}
		if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.Schema != "gentle-ai.opencode-review-provider-role/v1" || decoded.Role != string(role) || !decoded.Captured {
			t.Fatalf("provider role envelope = %s", payload)
		}
		validatePublishedReviewSchema(t, schema, []byte(payload))
	}
}
