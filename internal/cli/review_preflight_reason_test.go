package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// TestNegotiatedReviewStartStaleTargetCarriesItsSpecificReason drives the
// exact defect through the real negotiated route: `review start --target
// <identity>` refuses because anything wrote into the workspace after the
// identity was derived. The human surface has always named that reason
// verbatim; the negotiated envelope collapsed it into "The negotiated review
// request is invalid." with an empty required_inputs and next_action
// correct_request -- an instruction the caller cannot act on, because no
// edit to the request text can make a stale snapshot fresh again.
func TestNegotiatedReviewStartStaleTargetCarriesItsSpecificReason(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)

	// This is exactly what `review status --next-transition` hands back: a
	// fully bound START proposal whose --target is the identity of the
	// workspace as it stood at that instant.
	proposed := boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", "review-stale-target",
	})
	// A linter, a build, a test run, or a redirected command output between
	// the proposal and its execution changes the snapshot the identity was
	// derived from.
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\necho again\n", 0o644)

	var output bytes.Buffer
	if err := RunReview(proposed, &output); err == nil {
		t.Fatalf("stale negotiated START succeeded: %s", output.String())
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Operation != "review.start" || failure.Phase != "preflight" ||
		failure.MutationOutcome != ReviewMutationNotStarted {
		t.Fatalf("stale START envelope identity = %#v", failure)
	}
	if failure.Code != reviewPreflightStaleTargetCode {
		t.Fatalf("stale START code = %q, want %q\n%s", failure.Code, reviewPreflightStaleTargetCode, output.String())
	}
	if failure.NextAction != "review.status" {
		t.Fatalf("stale START next_action = %q, want review.status\n%s", failure.NextAction, output.String())
	}
	if !strings.Contains(failure.Cause, "review start target does not match the freshly built snapshot") {
		t.Fatalf("stale START cause = %q, want the specific native refusal\n%s", failure.Cause, output.String())
	}
	if failure.Message == reviewIntegrationGenericPreflightMessage {
		t.Fatalf("stale START kept the opaque generic message\n%s", output.String())
	}
	if len(failure.RequiredInputs) != 0 {
		t.Fatalf("stale START invented required inputs %v; no caller-supplied input fixes a stale snapshot", failure.RequiredInputs)
	}
	assertReviewFailureMatchesPublishedSchema(t, failure)
}

// TestNegotiatedReviewPreflightRefusalsNeverCollapseToTheBareEnvelope is the
// structural guard. It does not hardcode today's refusals: it parses every
// non-test source file in this package, enumerates every `reviewPreflightError`
// / `reviewPreflightRefusal` call site, and drives each one's real refusal text
// through the negotiated mapping. A refusal added tomorrow is picked up from
// source automatically.
//
// It asserts MODE PARITY, not merely non-emptiness. The reported defect was a
// negotiated envelope that said only "The negotiated review request is
// invalid." for a condition whose human surface named
// "review start target does not match the freshly built snapshot": two modes,
// two different amounts of information, for the same refusal. The rule below
// makes that class fail for every refusal at once.
//
// THE PARITY RULE. The human (non-negotiated) surface returns the refusal text
// verbatim on the error. For the same refusal, every DISTINGUISHING TOKEN of
// that human text must be recoverable from the negotiated envelope's
// machine-branchable fields (code, message, next_action, required_inputs) plus
// `cause`, taken together. A distinguishing token is a whitespace-separated
// word of the human text that:
//
//	(1) is at least reviewPreflightParityMinimumTokenRunes runes long, so
//	    articles and prepositions do not dominate the comparison;
//	(2) contains no printf verb, because the human surface substitutes a runtime
//	    value there and the literal "%s" is not information either mode carries;
//	(3) is not a generic connective (reviewPreflightParityStopWords); and
//	(4) SURVIVES THE PUBLISHED PRIVACY GATE.
//
// Clause (4) is the deliberate design point the task calls out. `cause` passes
// through reviewScrubDefectReportField, which truncates at the first newline
// and redacts path-, email- and env-shaped substrings, and the published
// failure schema bounds `cause` at 4000 runes. A refusal that names an absolute
// path therefore CANNOT reach the caller with that path intact, and demanding
// it would be demanding a privacy violation. So the required set is the human
// text minus exactly what the published privacy gate and the published length
// bound remove -- not a naive substring equality, which would fail the moment a
// refusal wraps an `open /some/path: ...` error.
//
// The rule is applied here as a PROPERTY, not as a re-run of the mapping: it
// never asks where in the envelope a token lives, so a future design that moves
// specificity out of `cause` and into `code`/`required_inputs` still passes,
// while one that drops it fails. Its known boundary is that the exemption in
// clause (4) is defined by the product's own privacy gate, so broadening that
// gate would shrink what parity demands. That is fenced by requiring every
// single site to retain at least one distinguishing token: a gate that redacted
// everything would fail this test rather than silently hollow it out.
func TestNegotiatedReviewPreflightRefusalsNeverCollapseToTheBareEnvelope(t *testing.T) {
	sites := collectReviewPreflightRefusalSites(t)
	if len(sites) < 40 {
		t.Fatalf("enumerated only %d preflight refusal sites; the survey found far more, so the AST walk is broken", len(sites))
	}
	literal, tokens, unmeasurable := 0, 0, 0
	for _, site := range sites {
		if site.message == "" {
			// The argument is an error value rather than a literal (a wrapped
			// read error, a typed error). Those are covered by the
			// forwarded-error shapes below; what matters here is that the
			// site exists and passes something.
			if site.forwardsError {
				continue
			}
			t.Fatalf("%s:%d passes neither a message nor an error to the preflight refusal", site.file, site.line)
		}
		literal++
		failure := newReviewIntegrationFailure("review.start", nil, reviewPreflightError(errors.New(site.message)))
		if isBareGenericPreflightEnvelope(failure) {
			t.Fatalf("%s:%d collapses to the bare generic envelope: %#v", site.file, site.line, failure)
		}
		if err := failure.Validate(); err != nil {
			t.Fatalf("%s:%d produced an invalid envelope: %v", site.file, site.line, err)
		}
		required := assertNegotiatedReviewPreflightParity(t, fmt.Sprintf("%s:%d", site.file, site.line), site.message, failure)
		tokens += required
		if required == 0 {
			unmeasurable++
			t.Logf("%s:%d carries no static distinguishing text (%q); parity is unmeasurable from source for this site", site.file, site.line, site.message)
		}
	}
	if literal == 0 {
		t.Fatal("no literal-message refusal site was enumerated")
	}
	// A walk where most sites became unmeasurable would report success while
	// checking almost nothing, so the measurable majority is required outright.
	if unmeasurable*2 >= literal {
		t.Fatalf("%d of %d literal refusal sites carry no comparable static text; the parity rule is no longer measuring the refusal surface", unmeasurable, literal)
	}
	t.Logf("checked mode parity for %d of %d literal refusal sites, requiring %d distinguishing tokens", literal-unmeasurable, literal, tokens)

	// Refusals that forward an error value instead of a literal must carry it
	// too, whatever its shape -- and must reach the same parity bar.
	forwarded := []error{
		errors.New("resolve reviewing authority: no such authority"),
		fmt.Errorf("read reviewer result: %w", errors.New("unexpected end of JSON input")),
		errors.Join(errors.New("first native refusal"), errors.New("second native refusal")),
		&ErrReviewFinalizeNoTransition{LineageID: "review-forwarded"},
	}
	for _, cause := range forwarded {
		failure := newReviewIntegrationFailure(ReviewIntegrationOperationFinalize, nil, reviewPreflightError(cause))
		if isBareGenericPreflightEnvelope(failure) {
			t.Fatalf("forwarded preflight cause %q collapsed to the bare generic envelope: %#v", cause, failure)
		}
		if err := failure.Validate(); err != nil {
			t.Fatalf("forwarded preflight cause %q produced an invalid envelope: %v", cause, err)
		}
		assertNegotiatedReviewPreflightParity(t, "forwarded", cause.Error(), failure)
	}
}

// TestNegotiatedReviewPreflightParityHoldsForTheRealDualModeSurfaces proves the
// parity rule against the two surfaces as they are actually produced, rather
// than against a reconstructed refusal: the same invalid request is run once
// without --contract (human surface, reason on the error) and once with it
// (negotiated envelope on stdout).
func TestNegotiatedReviewPreflightParityHoldsForTheRealDualModeSurfaces(t *testing.T) {
	repo := initReviewCLIRepo(t)
	tests := []struct {
		name string
		args []string
	}{
		{name: "unexpected argument", args: []string{"status", "surplus", "--cwd", repo}},
		{name: "unknown lineage", args: []string{"finalize", "--cwd", repo, "--lineage", "review-absent", "--captured-results"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var humanOutput bytes.Buffer
			humanErr := RunReview(test.args, &humanOutput)
			if humanErr == nil {
				t.Fatalf("human surface accepted an invalid request: %s", humanOutput.String())
			}
			negotiated := append([]string{test.args[0], "--contract", ReviewIntegrationContractV1}, test.args[1:]...)
			var negotiatedOutput bytes.Buffer
			if err := RunReview(negotiated, &negotiatedOutput); err == nil {
				t.Fatalf("negotiated surface accepted an invalid request: %s", negotiatedOutput.String())
			}
			failure := decodeReviewIntegrationFailure(t, negotiatedOutput.Bytes())
			assertNegotiatedReviewPreflightParity(t, test.name, humanErr.Error(), failure)
			assertReviewFailureMatchesPublishedSchema(t, failure)
		})
	}
}

const (
	// reviewPreflightParityMinimumTokenRunes is the length at which a word
	// starts carrying refusal-specific information rather than grammar.
	reviewPreflightParityMinimumTokenRunes = 4
	// reviewPreflightParityCauseRuneLimit mirrors failure.schema.json's own
	// published maxLength for `cause`. Anything past it cannot reach the caller
	// in that field no matter how the mapping is written.
	reviewPreflightParityCauseRuneLimit = 4000
)

// reviewPreflightParityStopWords are words that appear in refusals of every
// kind and therefore distinguish nothing. Keeping the list this small is
// deliberate: the fewer exemptions, the more the rule actually demands.
var reviewPreflightParityStopWords = map[string]bool{
	"that": true, "this": true, "with": true, "from": true, "than": true, "them": true,
	"they": true, "then": true, "when": true, "were": true, "will": true, "have": true,
	"been": true, "into": true, "only": true, "must": true, "must,": true, "same": true,
	"both": true, "here": true, "each": true, "also": true, "such": true, "does": true,
}

// assertNegotiatedReviewPreflightParity applies the parity rule described on
// TestNegotiatedReviewPreflightRefusalsNeverCollapseToTheBareEnvelope and
// returns how many distinguishing tokens it required. A return of 0 means the
// site carried no static distinguishing text to compare at all.
func assertNegotiatedReviewPreflightParity(t *testing.T, site, human string, failure ReviewIntegrationFailure) int {
	t.Helper()
	candidate, required := reviewPreflightParityDistinguishingTokens(human)
	if len(candidate) == 0 {
		// The refusal text is entirely printf verbs (review_artifact.go's
		// `fmt.Errorf("%s.%s", baseMessage, clause)` is the one such site
		// today), so the AST collector sees no static word this rule could
		// compare in either mode. Unmeasurable from source is reported, never
		// silently counted as passing -- and never asserted against, because
		// there is nothing to assert.
		return 0
	}
	if len(required) == 0 {
		// Static distinguishing text existed and the published privacy gate
		// removed all of it. This is the fence that stops clause (4) from
		// hollowing the rule out: a gate broad enough to erase an entire
		// refusal fails here instead of making parity vacuously true.
		t.Errorf("%s: the human refusal %q carries distinguishing words %v, but the published privacy gate removes every one of them, so mode parity would be vacuous",
			site, human, candidate)
		return 0
	}
	recoverable := strings.ToLower(strings.Join(append([]string{
		failure.Code, failure.Message, failure.NextAction, failure.Cause,
	}, failure.RequiredInputs...), " "))
	for _, word := range required {
		if !strings.Contains(recoverable, word) {
			t.Errorf("%s: the human surface names %q but the negotiated envelope carries nothing that recovers it; the negotiated mode is less specific than the human one\nhuman:     %q\nenvelope:  %#v",
				site, word, human, failure)
		}
	}
	return len(required)
}

// reviewPreflightParityDistinguishingTokens splits one human refusal into the
// words that carry refusal-specific information (clauses 1-3 of the parity
// rule) and, of those, the ones the negotiated envelope must let a caller
// recover (clause 4). Clause 4 is applied last and separately on purpose: the
// difference between the two return values is exactly what the published
// privacy gate and the published `cause` length bound remove, and no negotiated
// field is permitted to carry that.
func reviewPreflightParityDistinguishingTokens(human string) (candidate, required []string) {
	// Issue #1881 follow-up: the unanticipated-residue treatment appends the
	// defect-report clause to the HUMAN error line only. The clause is a
	// decoration around a local report path and the issues URL — both of which
	// the published privacy gate removes from any envelope field — so its
	// framing words carry no refusal information for parity to demand. The
	// refusal text being compared is everything before the clause.
	if index := strings.Index(human, reviewToolFaultDefectClausePrefix); index >= 0 {
		human = human[:index]
	}
	visible := reviewScrubDefectReportField(strings.ReplaceAll(human, "\r", "\n"))
	if runes := []rune(visible); len(runes) > reviewPreflightParityCauseRuneLimit {
		visible = string(runes[:reviewPreflightParityCauseRuneLimit])
	}
	visible = strings.ToLower(visible)
	seen := map[string]bool{}
	for _, field := range strings.Fields(strings.ToLower(human)) {
		word := strings.Trim(field, "`'\"(),.;:?!")
		if len([]rune(word)) < reviewPreflightParityMinimumTokenRunes ||
			strings.Contains(word, "%") ||
			reviewPreflightParityStopWords[word] ||
			seen[word] {
			continue
		}
		seen[word] = true
		candidate = append(candidate, word)
		if strings.Contains(visible, word) {
			required = append(required, word)
		}
	}
	return candidate, required
}

// TestNegotiatedReviewPreflightCauseIsPrivacyGatedAndSchemaBounded pins the
// split: `cause` is human-readable prose, so it passes the same field-level
// privacy gate every other caller-visible narrative string does, and it stays
// inside the published schema's single-line, 4000-character bound.
func TestNegotiatedReviewPreflightCauseIsPrivacyGatedAndSchemaBounded(t *testing.T) {
	tests := []struct {
		name    string
		cause   error
		wantNot string
		want    string
	}{
		{
			name:    "absolute path",
			cause:   errors.New("read reviewer result: open /home/someone/secret/token.json: no such file"),
			wantNot: "/home/someone/secret/token.json",
			want:    "read reviewer result: open",
		},
		{
			name:    "multi line",
			cause:   errors.New("validate FINALIZE live target: mismatch\nsecond line with detail"),
			wantNot: "second line with detail",
			want:    "validate FINALIZE live target: mismatch",
		},
		{
			name:    "carriage return",
			cause:   errors.New("validate FINALIZE current snapshot: mismatch\r\ntrailing"),
			wantNot: "trailing",
			want:    "validate FINALIZE current snapshot: mismatch",
		},
		{
			name:  "over long",
			cause: errors.New("bounded: " + strings.Repeat("x", 8000)),
			want:  "bounded: ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failure := newReviewIntegrationFailure(ReviewIntegrationOperationFinalize, nil, reviewPreflightError(tt.cause))
			if tt.wantNot != "" && strings.Contains(failure.Cause, tt.wantNot) {
				t.Fatalf("cause leaked %q: %q", tt.wantNot, failure.Cause)
			}
			if !strings.Contains(failure.Cause, tt.want) {
				t.Fatalf("cause = %q, want it to keep %q", failure.Cause, tt.want)
			}
			if strings.ContainsAny(failure.Cause, "\r\n") {
				t.Fatalf("cause is not single-line: %q", failure.Cause)
			}
			if count := len([]rune(failure.Cause)); count > 4000 {
				t.Fatalf("cause is %d runes, over the published 4000 bound", count)
			}
			if err := failure.Validate(); err != nil {
				t.Fatalf("privacy-gated cause failed envelope validation: %v", err)
			}
			assertReviewFailureMatchesPublishedSchema(t, failure)
		})
	}
}

// TestNegotiatedReviewPreflightNamesRequiredInputsOnlyWhenTheyAreReal proves
// the honest half of required_inputs: it is populated when the refusal
// genuinely identifies contract-level inputs the caller must supply, and left
// empty -- never invented -- when the refusal is not about inputs at all.
func TestNegotiatedReviewPreflightNamesRequiredInputsOnlyWhenTheyAreReal(t *testing.T) {
	named := newReviewIntegrationFailure(
		ReviewIntegrationOperationRetryFinalVerification, nil,
		reviewPreflightRefusal(
			reviewPreflightMissingInputsReason(
				"predecessor_lineage_id", "expected_predecessor_revision", "successor_lineage_id",
				"incident", "actor", "reason", "maintainer_authorization",
			),
			errors.New("review retry-final-verification requires --predecessor-lineage, --expected-predecessor-revision, --successor-lineage, --incident, --actor, --reason, and --maintainer-authorization"),
		),
	)
	want := []string{
		"predecessor_lineage_id", "expected_predecessor_revision", "successor_lineage_id",
		"incident", "actor", "reason", "maintainer_authorization",
	}
	if !reflect.DeepEqual(named.RequiredInputs, want) || named.NextAction != "correct_request" ||
		named.Code != reviewIntegrationInvalidRequestCode {
		t.Fatalf("named-input refusal = %#v", named)
	}
	if err := named.Validate(); err != nil {
		t.Fatal(err)
	}
	assertReviewFailureMatchesPublishedSchema(t, named)

	// A shape refusal names no contract input, so it must keep an empty list
	// rather than fabricate one.
	shape := newReviewIntegrationFailure("review.status", nil,
		reviewPreflightError(errors.New(`unexpected review status argument "surplus"`)))
	if len(shape.RequiredInputs) != 0 || shape.NextAction != "correct_request" ||
		shape.Code != reviewIntegrationInvalidRequestCode {
		t.Fatalf("shape refusal = %#v", shape)
	}
	if shape.Cause == "" {
		t.Fatalf("shape refusal named nothing: %#v", shape)
	}
}

// TestReviewPreflightHumanSurfaceStillCarriesTheReasonVerbatim guards the
// surface that was already correct: without --contract the caller gets the
// raw refusal text on the error, and nothing at all on stdout.
func TestReviewPreflightHumanSurfaceStillCarriesTheReasonVerbatim(t *testing.T) {
	var output bytes.Buffer
	err := RunReview([]string{"status", "surplus"}, &output)
	if err == nil {
		t.Fatal("non-negotiated invalid status request succeeded")
	}
	if output.Len() != 0 {
		t.Fatalf("non-negotiated refusal wrote to stdout: %q", output.String())
	}
	if !strings.Contains(err.Error(), `unexpected review status argument "surplus"`) {
		t.Fatalf("human refusal = %q, want the verbatim reason", err.Error())
	}
	var negotiatedErr *ReviewIntegrationFailureError
	if errors.As(err, &negotiatedErr) {
		t.Fatalf("non-negotiated refusal became a negotiated envelope: %v", err)
	}
}

func isBareGenericPreflightEnvelope(failure ReviewIntegrationFailure) bool {
	return failure.Code == reviewIntegrationInvalidRequestCode &&
		failure.Message == reviewIntegrationGenericPreflightMessage &&
		len(failure.RequiredInputs) == 0 &&
		failure.NextAction == "correct_request" &&
		failure.Cause == ""
}

type reviewPreflightRefusalSite struct {
	file          string
	line          int
	message       string
	forwardsError bool
}

// collectReviewPreflightRefusalSites walks this package's own source and
// returns every preflight refusal construction it can see. Deriving the set
// from source rather than a hand-written table is the point: a refusal added
// later is enumerated without anyone remembering to add it here.
func collectReviewPreflightRefusalSites(t *testing.T) []reviewPreflightRefusalSite {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	sites := []reviewPreflightRefusalSite{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, name, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", name, parseErr)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			ident, ok := call.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			var argument ast.Expr
			switch ident.Name {
			case "reviewPreflightError":
				if len(call.Args) != 1 {
					return true
				}
				argument = call.Args[0]
			case "reviewPreflightRefusal":
				if len(call.Args) != 2 {
					return true
				}
				argument = call.Args[1]
			default:
				return true
			}
			site := reviewPreflightRefusalSite{file: name, line: fset.Position(call.Pos()).Line}
			site.message = staticReviewErrorMessage(argument)
			site.forwardsError = site.message == ""
			sites = append(sites, site)
			return true
		})
	}
	return sites
}

// staticReviewErrorMessage extracts the literal refusal text from
// errors.New("...") and fmt.Errorf("...", ...). Format verbs stay as written;
// the mapping under test never parses the text, it only has to carry it.
func staticReviewErrorMessage(expr ast.Expr) string {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) == 0 {
		return ""
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	pkg, ok := selector.X.(*ast.Ident)
	if !ok {
		return ""
	}
	switch {
	case pkg.Name == "errors" && selector.Sel.Name == "New":
	case pkg.Name == "fmt" && selector.Sel.Name == "Errorf":
	default:
		return ""
	}
	literal, ok := call.Args[0].(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return ""
	}
	value, err := strconv.Unquote(literal.Value)
	if err != nil {
		return ""
	}
	return value
}

// assertReviewFailureMatchesPublishedSchema validates the emitted envelope
// against contracts/review-integration/v1/schemas/failure.schema.json, which
// is additionalProperties: false.
func assertReviewFailureMatchesPublishedSchema(t *testing.T, failure ReviewIntegrationFailure) {
	t.Helper()
	root := filepath.Join("..", "..", "contracts", "review-integration", "v1", "schemas")
	compiler := jsonschema.NewCompiler()
	payload, err := os.ReadFile(filepath.Join(root, "failure.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var resource any
	if err := json.Unmarshal(payload, &resource); err != nil {
		t.Fatal(err)
	}
	if err := compiler.AddResource(ReviewIntegrationFailureSchemaID, resource); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(ReviewIntegrationFailureSchemaID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(failure)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatalf("emitted failure envelope rejected by the published v1 schema: %v\n%s", err, encoded)
	}
}
