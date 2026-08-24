package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// This file is the RED-first proof for the Flow 11 defect an external
// black-box tester reported: the negotiated next-transition execution payload
// names a dotted logical operation ("review.start") and per-argument tokens
// ("--target=sha256:..."), but never the literal command line a caller can
// paste. The product changelog promises "transitions run exactly as they are
// printed", and docs/testing/organic-rdd-testing-guide.md Flow 11 step 3 tells
// the tester to copy-paste that command -- which was never emitted anywhere in
// the payload.

// reviewStartTransitionForCommand builds the exact "fresh target" review.start
// execute transition STATUS emits when no authority governs the candidate.
func reviewStartTransitionForCommand(t *testing.T, lineage string, kind reviewtransaction.TargetKind) ReviewNextTransition {
	t.Helper()
	status := ReviewTargetStatusResult{
		Applicability:  reviewtransaction.TargetApplicabilityUnrelated,
		TargetIdentity: "sha256:" + strings.Repeat("b", 64),
		Projection: ReviewTargetStatusProjection{
			Kind: kind, Projection: reviewtransaction.ProjectionWorkspace,
			BaseTree: strings.Repeat("c", 40), CurrentCandidateTree: strings.Repeat("d", 40),
			// A fresh candidate that really has changes: base and candidate
			// trees already differ here, and an empty workspace path set now
			// routes to a base-ref collection instead (issue #2584).
			Paths: []string{"internal/cli/review_next_transition.go"},
		},
	}
	got := newReviewNextTransition(status, nil, nil, nil, reviewNextTransitionInput{StartLineage: lineage})
	if got.Kind != reviewNextTransitionExecute || got.Execute == nil || got.Execute.Operation != "review.start" {
		t.Fatalf("next transition = %#v, want an execute review.start transition", got)
	}
	return got
}

// TestReviewNextTransitionExecuteEmitsRunnableCommand is the core defect
// proof: the execute payload must carry the complete, literally runnable
// command line, assembled from the canonical tool name, the registry-owned
// verb, and the arguments already present in the payload, in their existing
// order and in --flag=value form.
func TestReviewNextTransitionExecuteEmitsRunnableCommand(t *testing.T) {
	got := reviewStartTransitionForCommand(t, "review-start-command", reviewtransaction.TargetCurrentChanges)
	want := "gentle-ai review start" +
		" --contract=gentle-ai.review-integration/v1" +
		" --target=sha256:" + strings.Repeat("b", 64) +
		" --projection=workspace" +
		" --lineage=review-start-command"
	if got.Execute.Command != want {
		t.Fatalf("execute command = %q, want %q", got.Execute.Command, want)
	}
}

func TestReviewNextTransitionV2StartCommandCarriesConsentRelay(t *testing.T) {
	status := ReviewTargetStatusResult{
		Contract:       ReviewIntegrationContractV2,
		Applicability:  reviewtransaction.TargetApplicabilityUnrelated,
		TargetIdentity: "sha256:" + strings.Repeat("b", 64),
		Projection: ReviewTargetStatusProjection{
			Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace,
			BaseTree: strings.Repeat("c", 40), CurrentCandidateTree: strings.Repeat("d", 40),
			Paths: []string{"internal/cli/review_next_transition.go"},
		},
	}
	got := newReviewNextTransition(status, nil, nil, nil, reviewNextTransitionInput{StartLineage: "review-v2-consent-command"})
	want := "gentle-ai review start" +
		" --contract=gentle-ai.review-integration/v2" +
		" --target=sha256:" + strings.Repeat("b", 64) +
		" --projection=workspace" +
		" --lineage=review-v2-consent-command" +
		" --consent=relay"
	if got.Execute == nil || got.Execute.Command != want {
		t.Fatalf("v2 execute command = %#v, want %q", got.Execute, want)
	}
	for _, argument := range got.Execute.Arguments {
		if !strings.Contains(got.Execute.Command, argument.Token) {
			t.Fatalf("v2 execute command dropped token %q: %s", argument.Token, got.Execute.Command)
		}
	}
}

// TestReviewNextTransitionExecuteCommandUsesEveryArgumentToken proves the
// command's argument portion is exactly the payload's own tokens, in the
// payload's own order -- never invented, reordered, or dropped.
func TestReviewNextTransitionExecuteCommandUsesEveryArgumentToken(t *testing.T) {
	got := reviewStartTransitionForCommand(t, "review-token-order", reviewtransaction.TargetCurrentChanges)
	fields := strings.Split(got.Execute.Command, " ")
	if len(fields) != 3+len(got.Execute.Arguments) {
		t.Fatalf("execute command = %q, want exactly 3 command words plus %d argument tokens", got.Execute.Command, len(got.Execute.Arguments))
	}
	for index, argument := range got.Execute.Arguments {
		if argument.Token == "" {
			t.Fatalf("argument %q carries no token", argument.Name)
		}
		if fields[3+index] != argument.Token {
			t.Fatalf("command field %d = %q, want the payload token %q", 3+index, fields[3+index], argument.Token)
		}
	}
}

// TestReviewNextTransitionExecuteCommandRendersBooleanFlagsWithEquals pins the
// exact shape parseReviewFlags (internal/cli/review.go) accepts. Its recorded
// DECISION keeps the parser strict: a detached boolean value
// ("--committed-only true") is refused everywhere in the review command
// family, so an emitted command that used one would be unrunnable by policy.
func TestReviewNextTransitionExecuteCommandRendersBooleanFlagsWithEquals(t *testing.T) {
	got := reviewStartTransitionForCommand(t, "review-boolean-command", reviewtransaction.TargetBaseDiff)
	if !strings.Contains(got.Execute.Command, "--committed-only=true") {
		t.Fatalf("execute command = %q, want it to render the boolean flag as --committed-only=true", got.Execute.Command)
	}
	if strings.Contains(got.Execute.Command, "--committed-only true") {
		t.Fatalf("execute command = %q, must never emit a space-separated boolean value", got.Execute.Command)
	}
}

// TestReviewNextTransitionExecuteCommandUsesCanonicalToolName proves the
// command never leaks the invoking binary path. os.Args[0] during `go test` is
// a temporary build artifact; emitting it would produce a command that only
// runs on the machine that generated the payload.
func TestReviewNextTransitionExecuteCommandUsesCanonicalToolName(t *testing.T) {
	got := reviewStartTransitionForCommand(t, "review-canonical-tool", reviewtransaction.TargetCurrentChanges)
	if !strings.HasPrefix(got.Execute.Command, "gentle-ai review ") {
		t.Fatalf("execute command = %q, want it to start with the canonical tool name", got.Execute.Command)
	}
	if strings.Contains(got.Execute.Command, os.Args[0]) {
		t.Fatalf("execute command = %q leaks the invoking binary path %q", got.Execute.Command, os.Args[0])
	}
}

// reviewTransitionExecutionOperationEnum reads the published operation enum
// straight out of one shipped status schema, so this enumeration can never
// degrade into a hardcoded list of today's operations.
func reviewTransitionExecutionOperationEnum(t *testing.T, schemaFile string) []string {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join("..", "..", "contracts", "review-integration", "v1", "schemas", schemaFile))
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(payload, &schema); err != nil {
		t.Fatal(err)
	}
	defs, ok := schema["$defs"].(map[string]any)
	if !ok {
		t.Fatalf("%s has no $defs", schemaFile)
	}
	execution, ok := defs["transition_execution"].(map[string]any)
	if !ok {
		t.Fatalf("%s has no $defs/transition_execution", schemaFile)
	}
	properties, ok := execution["properties"].(map[string]any)
	if !ok {
		t.Fatalf("%s transition_execution has no properties", schemaFile)
	}
	operation, ok := properties["operation"].(map[string]any)
	if !ok {
		t.Fatalf("%s transition_execution has no operation property", schemaFile)
	}
	raw, ok := operation["enum"].([]any)
	if !ok || len(raw) == 0 {
		t.Fatalf("%s transition_execution operation has no enum", schemaFile)
	}
	operations := make([]string, 0, len(raw))
	for _, value := range raw {
		name, ok := value.(string)
		if !ok {
			t.Fatalf("%s transition_execution operation enum holds a non-string %#v", schemaFile, value)
		}
		operations = append(operations, name)
	}
	return operations
}

// reviewTransitionOperationsWithoutRegistryEntry pins the exact, currently
// exhaustive set of published transition operations that resolve to NO
// runnable command, and why.
//
// It is EMPTY, and that is the point: every operation either status schema
// publishes as an execute transition resolves to a verb review_facade.go
// really dispatches.
//
// It held "review.recover" until issue #1864. The reasoning recorded there was
// that adding a registry row was not a local fix, because the registry also
// drives reviewIntegrationOperationNames() and therefore the published
// capabilities `operations` array, whose schemas pin an exact maxItems and a
// closed enum. That reasoning was right about the constraint and wrong about
// the conclusion: the fix was to stop conflating "this row owns a runnable CLI
// verb" with "this operation is part of the published negotiated surface".
// reviewIntegrationOperationMetadata.Negotiated now separates them, so
// review.recover owns its verb without appearing in any published contract.
//
// This map fails closed in BOTH directions below, so it can never rot: a new
// unresolved operation fails, and an operation named here that later DOES
// resolve fails too, forcing the entry to be removed.
var reviewTransitionOperationsWithoutRegistryEntry = map[string]string{}

// TestEveryPublishedTransitionOperationProducesARunnableCommand is the test
// that makes this defect impossible to reintroduce for a FUTURE operation: it
// enumerates the published operation enum of both shipped status schemas
// rather than a hardcoded list, and requires every single entry to resolve to
// a runnable command whose verb is a verb review_facade.go actually
// dispatches.
func TestEveryPublishedTransitionOperationProducesARunnableCommand(t *testing.T) {
	dispatched := reviewCommandDispatchVerbs(t)
	if len(dispatched) == 0 {
		t.Fatal("found no dispatched review command verbs in review_facade.go; the extraction is stale")
	}
	published := map[string]bool{}
	for _, schemaFile := range []string{"status.schema.json", "status-v2.schema.json"} {
		for _, operation := range reviewTransitionExecutionOperationEnum(t, schemaFile) {
			published[operation] = true
			verb, resolved := reviewTransitionCommandVerb(operation)
			reason, known := reviewTransitionOperationsWithoutRegistryEntry[operation]
			if !resolved {
				if !known {
					t.Errorf("%s publishes transition operation %q, but it resolves to no runnable command verb and is not a recorded registry gap", schemaFile, operation)
				}
				continue
			}
			if known {
				t.Errorf("%s transition operation %q now resolves to verb %q, so the recorded registry gap (%s) is stale: drop it from reviewTransitionOperationsWithoutRegistryEntry", schemaFile, operation, verb, reason)
			}
			if !dispatched[verb] {
				t.Errorf("%s publishes transition operation %q, which resolves to verb %q, but review_facade.go dispatches no such command", schemaFile, operation, verb)
			}
			command := reviewTransitionCommandLine(operation, []ReviewTransitionArgument{{Name: "lineage", Value: "review-enum", Token: "--lineage=review-enum"}})
			if command != "gentle-ai review "+verb+" --lineage=review-enum" {
				t.Errorf("%s transition operation %q renders command %q", schemaFile, operation, command)
			}
		}
	}
	for operation := range reviewTransitionOperationsWithoutRegistryEntry {
		if !published[operation] {
			t.Errorf("reviewTransitionOperationsWithoutRegistryEntry names %q, but no published status schema declares it as a transition operation", operation)
		}
	}
}

// TestUnresolvedTransitionOperationEmitsNoHalfCommand proves the fail-closed
// half: an operation with no registry-owned verb yields no command at all,
// never "gentle-ai review  --flag=value" or any other half-assembled line.
func TestUnresolvedTransitionOperationEmitsNoHalfCommand(t *testing.T) {
	for operation := range reviewTransitionOperationsWithoutRegistryEntry {
		if command := reviewTransitionCommandLine(operation, []ReviewTransitionArgument{{Name: "lineage", Value: "review-gap", Token: "--lineage=review-gap"}}); command != "" {
			t.Errorf("unresolved operation %q rendered command %q, want no command at all", operation, command)
		}
	}
	if command := reviewTransitionCommandLine("review.not-an-operation", nil); command != "" {
		t.Errorf("unknown operation rendered command %q, want no command at all", command)
	}
}

// TestReviewTransitionCommandVerbIsOwnedByTheOperationRegistry pins the single
// policy source: reviewIntegrationOperationRegistry's Command field is the
// authority for every operation it knows, so the command line can never carry
// a verb that silently diverges from negotiated routing.
func TestReviewTransitionCommandVerbIsOwnedByTheOperationRegistry(t *testing.T) {
	for _, metadata := range reviewIntegrationOperationRegistry {
		verb, resolved := reviewTransitionCommandVerb(metadata.Operation)
		if !resolved || verb != metadata.Command {
			t.Errorf("operation %q resolves to verb %q (resolved=%t), want the registry's own command %q", metadata.Operation, verb, resolved, metadata.Command)
		}
	}
}

// TestReviewTransitionCommandQuotesFreeTextValues proves the emitted line is
// runnable even when a value is free text. review.repair carries operator-
// supplied --reason and --actor values (from `review status --repair-reason`
// / `--repair-actor`), which routinely contain spaces. Joining those tokens
// raw would produce a line the shell splits into extra positional arguments,
// which every review verb refuses with "unexpected review <verb> argument" --
// exactly the unrunnable-command class this whole change exists to close.
func TestReviewTransitionCommandQuotesFreeTextValues(t *testing.T) {
	command := reviewTransitionCommandLine("review.repair", []ReviewTransitionArgument{
		{Name: "lineage", Value: "review-quote", Token: "--lineage=review-quote"},
		{Name: "reason", Value: "historical alias repair", Token: "--reason=historical alias repair"},
		{Name: "actor", Value: "o'brien", Token: "--actor=o'brien"},
	})
	want := "gentle-ai review repair --lineage=review-quote '--reason=historical alias repair' '--actor=o'\\''brien'"
	if command != want {
		t.Fatalf("command = %q, want %q", command, want)
	}
}

// TestReviewTransitionCommandQuotedTokensSurviveShellWordSplitting proves the
// quoting above by execution rather than by assertion: a real /bin/sh parses
// the emitted line and reports each argv entry, which must be byte-identical
// to the payload's own tokens.
func TestReviewTransitionCommandQuotedTokensSurviveShellWordSplitting(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is unavailable")
	}
	arguments := []ReviewTransitionArgument{
		{Name: "lineage", Value: "review-quote", Token: "--lineage=review-quote"},
		{Name: "reason", Value: "historical alias repair", Token: "--reason=historical alias repair"},
		{Name: "actor", Value: "o'brien", Token: "--actor=o'brien"},
	}
	command := reviewTransitionCommandLine("review.repair", arguments)
	script := "set -- " + strings.TrimPrefix(command, "gentle-ai review repair ") + "\nfor argument in \"$@\"; do printf '%s\\n' \"$argument\"; done"
	output, err := exec.Command(shell, "-c", script).Output()
	if err != nil {
		t.Fatalf("shell rejected the emitted command %q: %v", command, err)
	}
	got := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
	if len(got) != len(arguments) {
		t.Fatalf("shell split %q into %d arguments %q, want %d", command, len(got), got, len(arguments))
	}
	for index, argument := range arguments {
		if got[index] != argument.Token {
			t.Fatalf("shell argument %d = %q, want the payload token %q", index, got[index], argument.Token)
		}
	}
}

// TestReviewNextTransitionCollectAndStopCarryNoCommand proves the command is
// scoped to the execute form only: a collect transition names an input to
// gather, and a stop transition intentionally contains no command-shaped data.
//
// The collect half matters most now that a native collect input's arguments
// carry runnable tokens. Tokens are per-argument; a command is a complete line.
// `review capture-result` also needs --input pointing at a reviewer result that
// does not exist until a model has run the lens, so no printable line would run
// verbatim and none is emitted.
func TestReviewNextTransitionCollectAndStopCarryNoCommand(t *testing.T) {
	stop := reviewStopTransition("native_stop_required")
	if stop.Execute != nil {
		t.Fatalf("stop transition = %#v, want no execute payload", stop)
	}
	payload, err := json.Marshal(stop)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "\"command\"") {
		t.Fatalf("stop transition payload = %s, want no command field", payload)
	}
	binding := ReviewTransitionBinding{
		LineageID:      "review-collect-no-command",
		Revision:       "sha256:" + strings.Repeat("a", 64),
		TargetIdentity: "sha256:" + strings.Repeat("b", 64),
	}
	collect := reviewCollectTransition("reviewer_results_required", reviewCaptureInput(binding, "review-reliability", 0, nil))
	if collect.Execute != nil {
		t.Fatalf("collect transition = %#v, want no execute payload", collect)
	}
	if collect.Collect.Inputs[0].Arguments[0].Token == "" {
		t.Fatal("native collect input carries no token, so this test would pass vacuously")
	}
	payload, err = json.Marshal(collect)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "\"command\"") {
		t.Fatalf("collect transition payload = %s, want no command field", payload)
	}
}

// TestReviewNextTransitionExecuteCommandValidatesAgainstPublishedSchemas
// proves the emitted payload is admissible under the exact schemas it claims:
// status-v2.schema.json for the STATUS result, and status.schema.json for the
// compatibility next_transition payload.
func TestReviewNextTransitionExecuteCommandValidatesAgainstPublishedSchemas(t *testing.T) {
	got := reviewStartTransitionForCommand(t, "review-schema-command", reviewtransaction.TargetCurrentChanges)
	if got.Execute.Command == "" {
		t.Fatal("execute transition carries no command")
	}
	payload, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	validateAgainstPublishedNextTransitionSchemaV2(t, payload)
	validateAgainstPublishedNextTransitionSchema(t, payload)
}

// TestReviewNextTransitionQuotedCommandValidatesAgainstPublishedSchemas keeps
// the published command pattern honest about the quoted form: a repair
// transition carrying operator free text is still a legal payload, so the
// schema must not accept only the unquoted common case.
func TestReviewNextTransitionQuotedCommandValidatesAgainstPublishedSchemas(t *testing.T) {
	transition := reviewExecuteTransition("repair_authorized", "review.repair",
		[]ReviewTransitionArgument{
			{Name: "lineage", Value: "review-quoted-schema"},
			{Name: "reason", Value: "historical alias repair"},
		},
		[]ReviewTransitionArgument{{Name: "repair_status", Value: "eligible"}},
		ReviewTransitionBinding{LineageID: "review-quoted-schema", Revision: "sha256:" + strings.Repeat("a", 64), TargetIdentity: "sha256:" + strings.Repeat("b", 64)},
		nil)
	if !strings.Contains(transition.Execute.Command, "'--reason=historical alias repair'") {
		t.Fatalf("execute command = %q, want the free-text argument quoted", transition.Execute.Command)
	}
	payload, err := json.Marshal(transition)
	if err != nil {
		t.Fatal(err)
	}
	validateAgainstPublishedNextTransitionSchemaV2(t, payload)
}

// TestPublishedStatusSchemasRequireTokenOnExecutableArguments closes the
// optionality hole the schemas still carry: an execute argument is argv, and
// its exact runnable token is the field docs/testing Flow 11 step 2 depends
// on, so a schema-conforming payload must not be allowed to omit it.
//
// It is deliberately scoped to execute.arguments, because that is the only
// position where a token is unconditional. Preconditions and
// selector_arguments are assertions and a normalized echo, never argv, and a
// collect input carries tokens only when its capture_operation names an
// operation this product performs -- an "external.*" one is performed
// elsewhere and has no argv to render. Requiring token on the shared
// $defs/transition_argument would therefore invalidate the product's own
// payloads.
func TestPublishedStatusSchemasRequireTokenOnExecutableArguments(t *testing.T) {
	for _, schemaFile := range []string{"status.schema.json", "status-v2.schema.json"} {
		payload, err := os.ReadFile(filepath.Join("..", "..", "contracts", "review-integration", "v1", "schemas", schemaFile))
		if err != nil {
			t.Fatal(err)
		}
		var schema map[string]any
		if err := json.Unmarshal(payload, &schema); err != nil {
			t.Fatal(err)
		}
		defs := schema["$defs"].(map[string]any)
		shared, ok := defs["transition_argument"].(map[string]any)
		if !ok {
			t.Fatalf("%s has no $defs/transition_argument", schemaFile)
		}
		for _, field := range schemaStringArray(t, shared["required"]) {
			if field == "token" {
				t.Errorf("%s requires token on the shared $defs/transition_argument, but preconditions, selector_arguments and collect inputs never carry one", schemaFile)
			}
		}
		executable, ok := defs["executable_transition_argument"].(map[string]any)
		if !ok {
			t.Errorf("%s has no $defs/executable_transition_argument, so an execute argument may still omit its runnable token", schemaFile)
			continue
		}
		required := schemaStringArray(t, executable["required"])
		for _, field := range []string{"name", "value", "token"} {
			if !containsString(required, field) {
				t.Errorf("%s executable_transition_argument omits required %q: %v", schemaFile, field, required)
			}
		}
		execution := defs["transition_execution"].(map[string]any)
		properties := execution["properties"].(map[string]any)
		arguments := properties["arguments"].(map[string]any)
		items, ok := arguments["items"].(map[string]any)
		if !ok || items["$ref"] != "#/$defs/executable_transition_argument" {
			t.Errorf("%s transition_execution.arguments items = %#v, want a $ref to executable_transition_argument", schemaFile, arguments["items"])
		}
		command, ok := properties["command"].(map[string]any)
		if !ok || command["type"] != "string" {
			t.Errorf("%s transition_execution declares no command property: %#v", schemaFile, properties["command"])
		}
	}
}

// TestReviewRecoverTransitionEmitsACommandThatRuns is issue #1864 in one test.
//
// A negotiated recovery emitted `kind: execute`, named `review.recover`, and
// carried an empty `command`: the caller was told to run something and handed
// nothing to run. Asserting the string is only half a proof, so this builds a
// real authorized recovery, reads the command the product prints, splits it the
// way a POSIX shell would, and runs exactly those bytes. The recovery has to
// actually land, because a command that parses but does not work is the dead
// end this defect class is about.
func TestReviewRecoverTransitionEmitsACommandThatRuns(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 1 }\n", 0o644)
	startedBytes, err := runLegacyFacadeStartForTestBytes(t, []string{"--cwd", repo, "--lineage", "recover-command"})
	if err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, startedBytes, &started)
	for order := range started.SelectedLenses {
		findings := []facadeFinding{}
		if order == 0 {
			findings = []facadeFinding{{
				Location: "candidate.go:3", Severity: "CRITICAL", Claim: "candidate requires a helper",
				ProofRefs: []string{"candidate.go:3 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
				CausalDisposition: reviewtransaction.CausalIntroduced,
			}}
		}
		captureCLIReviewerResultWithFindings(t, repo, started, order, findings, &bytes.Buffer{})
	}

	// Change the candidate so the live target really differs from the frozen
	// one; recovery onto an unchanged target is refused by design.
	writeReviewStartCandidate(t, repo, "helper.go", "package candidate\n", 0o644)
	probe := selectorTransitionStatus(t, repo, "--lineage", started.LineageID)
	if probe.Authority == nil || probe.Action != reviewtransaction.TargetStatusActionRecover {
		t.Fatalf("recovery probe = action %q authority %#v", probe.Action, probe.Authority)
	}
	const successor, actor, reason = "recover-command-successor", "maintainer", "authorized recovery"
	authorization := "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + started.LineageID +
		"\npredecessor_revision=" + probe.Authority.Revision + "\ntarget_identity=" + probe.TargetIdentity +
		"\nsuccessor_lineage=" + successor + "\nactor=" + actor + "\nreason=" + reason
	status := selectorTransitionStatus(t, repo,
		"--lineage", started.LineageID,
		"--recovery-successor-lineage", successor,
		"--recovery-reason", reason,
		"--recovery-actor", actor,
		"--recovery-authorization", authorization,
	)
	if status.NextTransition == nil || status.NextTransition.Execute == nil ||
		status.NextTransition.Execute.Operation != "review.recover" {
		t.Fatalf("recovery transition = %#v", status.NextTransition)
	}

	// The exact bytes a caller is handed. The authorization is six LF-joined
	// lines, so it is the one argument the product must quote for the printed
	// line to survive a shell.
	want := "gentle-ai review recover" +
		" --predecessor-lineage=" + started.LineageID +
		" --expected-predecessor-revision=" + probe.Authority.Revision +
		" --successor-lineage=" + successor +
		" --disposition=" + string(probe.ActionDisposition) +
		" '--reason=" + reason + "'" +
		" --actor=" + actor +
		" '--maintainer-authorization=" + authorization + "'"
	if status.NextTransition.Execute.Command != want {
		t.Fatalf("recover command = %q\nwant %q", status.NextTransition.Execute.Command, want)
	}

	// Run the printed bytes, not a reassembly of them.
	words := reviewShellWords(t, status.NextTransition.Execute.Command)
	if len(words) < 3 || words[0] != "gentle-ai" || words[1] != "review" {
		t.Fatalf("printed command is not a gentle-ai review invocation: %#v", words)
	}
	t.Chdir(repo)
	var recovered bytes.Buffer
	if err := RunReview(words[2:], &recovered); err != nil {
		t.Fatalf("the printed command did not run: %v\n%s", err, recovered.String())
	}
	var operation ReviewRecoverResult
	decodeStrictReviewJSON(t, recovered.Bytes(), &operation)
	if operation.LineageID != successor || operation.State != reviewtransaction.StateReviewing {
		t.Fatalf("printed recovery command produced %#v, want a reviewing successor %q", operation, successor)
	}
}

// reviewShellWords splits a printed command line the way a POSIX shell would,
// understanding exactly the single quoting reviewTransitionShellWord emits.
func reviewShellWords(t *testing.T, line string) []string {
	t.Helper()
	words := []string{}
	var word strings.Builder
	quoted, escaped, started := false, false, false
	for _, char := range line {
		switch {
		case quoted && char == '\'':
			quoted = false
		case quoted:
			word.WriteRune(char)
		case escaped:
			word.WriteRune(char)
			escaped = false
		case char == '\\':
			escaped, started = true, true
		case char == '\'':
			quoted, started = true, true
		case char == ' ' || char == '\t' || char == '\n':
			if started {
				words = append(words, word.String())
				word.Reset()
				started = false
			}
		default:
			word.WriteRune(char)
			started = true
		}
	}
	if quoted || escaped {
		t.Fatalf("printed command does not close its quoting: %q", line)
	}
	if started {
		words = append(words, word.String())
	}
	return words
}
