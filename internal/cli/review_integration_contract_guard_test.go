package cli

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// This file generalizes the pattern already proven by
// review_next_transition_docs_test.go (every stop reason code in source must
// have a docs continuation row) and review_next_transition_test.go's schema
// validation (an emitted payload is validated against the published JSON
// schema). It exists so the community-reported defect class from this task
// -- docs/review-integration.md promising a name or invocation no emitter
// actually backs, and an unnegotiated CLI response silently omitting data the
// contract requires without saying so -- cannot ship again.
//
// Scope (honesty contract): a fully general "extract every promise from
// English prose" checker is not tractable. This file covers three precise,
// mechanically-extracted, enumerable subsets instead:
//
//  1. Every gentle-ai.*/vN schema identifier literal docs/review-integration.md
//     quotes in backticks (Guard A).
//  2. Every `gentle-ai review <verb>` command docs/review-integration.md names
//     as required (Guard B).
//  3. The mode-completeness rule: every command that branches on negotiated
//     --contract (found via the reviewIntegrationNegotiation call sites, not
//     memorized) must be classified, and its classification must be backed by
//     a real source pattern (Guard C).
//
// What this deliberately does NOT cover: free-form prose promises (e.g. "the
// provider validates X"), field names inside Markdown tables that are not
// also backtick-quoted schema identifiers or gentle-ai commands, and doc
// sections outside docs/review-integration.md. A narrow guard that genuinely
// fails closed beats a broad one that silently passes.

// reviewIntegrationDocSchemaIDRegexp extracts every backtick-quoted
// gentle-ai.*/vN (or /vN.M) schema identity literal from the doc's prose.
var reviewIntegrationDocSchemaIDRegexp = regexp.MustCompile("`(gentle-ai\\.[a-z0-9.\\-]+/v[0-9]+(?:\\.[0-9]+)?)`")

// reviewIntegrationDocCommandVerbRegexp extracts every `gentle-ai review
// <verb>` command name the doc names, from both fenced code blocks and
// inline code spans (both use the same literal token sequence).
var reviewIntegrationDocCommandVerbRegexp = regexp.MustCompile(`gentle-ai review ([a-z][a-z-]*)`)

func readReviewIntegrationDoc(t *testing.T) string {
	t.Helper()
	docs, err := os.ReadFile("../../docs/review-integration.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(docs)
}

// TestEveryDocumentedSchemaIdentityIsImplemented is Guard A. It fails closed
// in the one direction that matters for a documentation promise: a schema
// identity docs/review-integration.md quotes but no production Go source
// (internal/cli, internal/reviewtransaction) ever emits is a promise nothing
// satisfies.
func TestEveryDocumentedSchemaIdentityIsImplemented(t *testing.T) {
	docs := readReviewIntegrationDoc(t)
	matches := reviewIntegrationDocSchemaIDRegexp.FindAllStringSubmatch(docs, -1)
	if len(matches) == 0 {
		t.Fatal("found no gentle-ai.*/vN schema identifiers in docs/review-integration.md; the extraction regexp is stale")
	}
	ids := map[string]bool{}
	for _, match := range matches {
		ids[match[1]] = true
	}

	sources := reviewIntegrationProductionSources(t)
	for id := range ids {
		found := false
		for _, source := range sources {
			if strings.Contains(source, id) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("docs/review-integration.md documents schema identity %q, but no production Go source under internal/cli or internal/reviewtransaction emits that literal", id)
		}
	}
}

// TestEveryDocumentedReviewCommandIsReal is Guard B. It fails closed when the
// doc names a `gentle-ai review <verb>` invocation that is not one of the
// verbs runReviewCommandContext/runReviewCommand actually dispatch --
// extracted mechanically from their case labels, not hand-maintained.
func TestEveryDocumentedReviewCommandIsReal(t *testing.T) {
	docs := readReviewIntegrationDoc(t)
	matches := reviewIntegrationDocCommandVerbRegexp.FindAllStringSubmatch(docs, -1)
	if len(matches) == 0 {
		t.Fatal("found no `gentle-ai review <verb>` commands in docs/review-integration.md; the extraction regexp is stale")
	}
	documented := map[string]bool{}
	for _, match := range matches {
		documented[match[1]] = true
	}

	// reviewDispatchableReviewVerbs, not reviewCommandDispatchVerbs: the app
	// layer routes `review mode` BEFORE the facade so the kill switch stays
	// reachable when review authority itself is unreadable, and a guard that
	// only reads the facade's switch calls that real command undocumented.
	// The sibling continuation guard learned this first; both now share the
	// one extractor that owns the complete answer.
	dispatched := reviewDispatchableReviewVerbs(t)
	for verb := range documented {
		if !dispatched[verb] {
			t.Errorf("docs/review-integration.md requires `gentle-ai review %s`, but no dispatch reaches it from the facade switches or the app pre-dispatch", verb)
		}
	}
}

// reviewIntegrationTestingGuidePath is the community-facing testing guide.
// Its Flow 11 is the flow an external tester reported as FAIL: step 3 told the
// tester to copy-paste the emitted command, and no emitter produced one.
const reviewIntegrationTestingGuidePath = "../../docs/testing/organic-rdd-testing-guide.md"

// reviewIntegrationGuideTransitionFieldRegexp extracts every backtick-quoted
// next_transition field path the testing guide points a tester at, e.g.
// `next_transition.execute.command`. It is deliberately rooted at
// next_transition: a general dotted-token extractor also matches operation
// names ("review.start") and filenames ("requirements.txt"), which are not
// payload paths at all. A narrow guard that genuinely fails closed beats a
// broad one that has to be taught about exceptions.
var reviewIntegrationGuideTransitionFieldRegexp = regexp.MustCompile("`(next_transition(?:\\.[a-z_]+)+)`")

// TestEveryTestingGuideTransitionFieldIsEmitted is Guard D. It generalizes the
// exact defect this change closed: docs/testing/organic-rdd-testing-guide.md
// Flow 11 instructed a tester to copy-paste an emitted command, and no
// emitter produced one anywhere in the payload. Every next_transition field
// path the guide quotes must be backed by a real JSON tag on a production Go
// struct under internal/cli or internal/reviewtransaction.
func TestEveryTestingGuideTransitionFieldIsEmitted(t *testing.T) {
	guide, err := os.ReadFile(reviewIntegrationTestingGuidePath)
	if err != nil {
		t.Fatal(err)
	}
	matches := reviewIntegrationGuideTransitionFieldRegexp.FindAllStringSubmatch(string(guide), -1)
	if len(matches) == 0 {
		t.Fatal("found no backtick-quoted next_transition field paths in the testing guide; the extraction regexp is stale")
	}
	source, err := os.ReadFile("review_next_transition.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, match := range matches {
		segments := strings.Split(match[1], ".")
		// next_transition is the STATUS/FINALIZE property name whose value is a
		// ReviewNextTransition; the walk starts inside that struct.
		structName := "ReviewNextTransition"
		for _, segment := range segments[1:] {
			next, found := reviewStructFieldType(t, string(source), structName, segment)
			if !found {
				t.Errorf("the testing guide points a tester at %q, but %s carries no %q JSON field", match[1], structName, segment)
				break
			}
			structName = next
		}
	}
}

// reviewStructFieldRegexp matches one struct field line and captures its Go
// type and its JSON name, e.g. `Execute *ReviewTransitionExecution
// \`json:"execute,omitempty"\“.
var reviewStructFieldRegexp = regexp.MustCompile("^\\s*\\w+\\s+([\\[\\]\\*\\w.]+)\\s+`json:\"([a-z0-9_]+)")

// reviewStructFieldType finds one named JSON field inside one named struct in
// the given source and returns the Go type name it points at, so Guard D can
// walk a dotted payload path struct by struct instead of accepting any
// same-named tag anywhere in the tree.
func reviewStructFieldType(t *testing.T, source, structName, jsonName string) (string, bool) {
	t.Helper()
	marker := regexp.MustCompile(`(?m)^type ` + regexp.QuoteMeta(structName) + ` struct \{$`)
	loc := marker.FindStringIndex(source)
	if loc == nil {
		t.Fatalf("struct %s not found in review_next_transition.go", structName)
	}
	for _, line := range strings.Split(source[loc[1]:], "\n") {
		if strings.HasPrefix(line, "}") {
			break
		}
		field := reviewStructFieldRegexp.FindStringSubmatch(line)
		if field == nil || field[2] != jsonName {
			continue
		}
		return strings.TrimLeft(field[1], "*[]"), true
	}
	return "", false
}

// reviewIntegrationProductionSources reads every non-test .go file under
// internal/cli and internal/reviewtransaction, so Guard A only credits a
// schema identity actually implemented in production code -- never a test
// fixture that merely echoes the doc's own literal back.
func reviewIntegrationProductionSources(t *testing.T) []string {
	t.Helper()
	var sources []string
	for _, dir := range []string{".", "../reviewtransaction"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			payload, err := os.ReadFile(dir + "/" + name)
			if err != nil {
				t.Fatal(err)
			}
			sources = append(sources, string(payload))
		}
	}
	return sources
}

// reviewCommandDispatchCaseRegexp extracts a quoted case label, e.g.
// `case "start":` or `case "start", "status":` (only the first label of a
// multi-label case is meaningful here; none of the two switches this test
// reads use multi-label cases for review verbs).
var reviewCommandDispatchCaseRegexp = regexp.MustCompile(`^\s*case "([a-z][a-z-]*)":`)

// reviewCommandDispatchVerbs mechanically extracts every case label inside
// runReviewCommandContext and runReviewCommand in review_facade.go -- the two
// switches RunReview ultimately dispatches every `gentle-ai review <verb>`
// invocation through. It deliberately does NOT scan the whole file: other
// switches in the same file (severity, lens name) also have quoted string
// case labels that would otherwise false-positive.
func reviewCommandDispatchVerbs(t *testing.T) map[string]bool {
	t.Helper()
	source, err := os.ReadFile("review_facade.go")
	if err != nil {
		t.Fatal(err)
	}
	verbs := map[string]bool{}
	inScope := false
	for _, line := range strings.Split(string(source), "\n") {
		switch {
		case strings.HasPrefix(line, "func runReviewCommandContext(") || strings.HasPrefix(line, "func runReviewCommand("):
			inScope = true
			continue
		case strings.HasPrefix(line, "func "):
			inScope = false
			continue
		}
		if !inScope {
			continue
		}
		if match := reviewCommandDispatchCaseRegexp.FindStringSubmatch(line); match != nil {
			verbs[match[1]] = true
		}
	}
	return verbs
}

// reviewIntegrationNegotiationCallRegexp finds every call site of
// reviewIntegrationNegotiation(flags, *contract) so the dual-mode command set
// below is enumerated from the source, not from memory. It intentionally
// excludes the function definition itself (which does not call itself).
var reviewIntegrationNegotiationCallRegexp = regexp.MustCompile(`negotiated,\s*err\s*:?=\s*reviewIntegrationNegotiation\(`)

var reviewFuncHeaderRegexp = regexp.MustCompile(`^func (\(\w+ \*?\w+\) )?(\w+)\(`)

// reviewIntegrationNegotiationCallSites returns the name of every function
// across internal/cli's production .go files whose body calls
// reviewIntegrationNegotiation(...). This is Guard C's mechanical enumeration
// of "commands with both modes" the task requires.
func reviewIntegrationNegotiationCallSites(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var sites []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		payload, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		currentFunc := ""
		for _, line := range strings.Split(string(payload), "\n") {
			if match := reviewFuncHeaderRegexp.FindStringSubmatch(line); match != nil {
				currentFunc = match[2]
				continue
			}
			if reviewIntegrationNegotiationCallRegexp.MatchString(line) && currentFunc != "" {
				sites = append(sites, currentFunc)
				currentFunc = ""
			}
		}
	}
	sort.Strings(sites)
	return sites
}

// reviewIntegrationModeCompletenessClassification pins the exact, currently
// exhaustive classification of every dual-mode command's mode-completeness
// evidence. Adding a NEW reviewIntegrationNegotiation call site without
// adding it here fails TestReviewIntegrationDualModeCommandsAreClassified
// closed; removing a call site this map still names does the same.
//
//   - "vacuous": the unnegotiated and negotiated results are the exact same
//     value (proven by encodeReviewIntegrationOperation(stdout, negotiated,
//     <op>, X, X) passing the identical identifier twice), so there is no
//     data gap to hint about.
//   - "explicit-refusal": the negotiated-only data is opt-in behind a flag,
//     and requesting it without --contract is refused with a message naming
//     the exact --contract requirement, instead of silently omitting it.
//   - "hinted": the unnegotiated result can silently omit contract-only data
//     the caller needs (this was the reported defect, scoped to `review
//     start`), so the fix is an additive hint field naming the exact
//     negotiated rerun.
var reviewIntegrationModeCompletenessClassification = map[string]string{
	"runReviewFacadeStart":            "hinted",
	"runReviewFacadeFinalize":         "explicit-refusal",
	"runReviewFacadeValidate":         "vacuous",
	"runReviewBindSDD":                "vacuous",
	"runReviewRetryFinalVerification": "vacuous",
}

// TestReviewIntegrationDualModeCommandsAreClassified is Guard C's enumeration
// half: it fails closed in both directions against
// reviewIntegrationModeCompletenessClassification above -- a real dual-mode
// command with no classification, and a classification naming a command that
// no longer branches on negotiation.
func TestReviewIntegrationDualModeCommandsAreClassified(t *testing.T) {
	sites := reviewIntegrationNegotiationCallSites(t)
	if len(sites) == 0 {
		t.Fatal("found no reviewIntegrationNegotiation(...) call sites under internal/cli; the extraction is stale")
	}
	found := map[string]bool{}
	for _, site := range sites {
		found[site] = true
		if _, ok := reviewIntegrationModeCompletenessClassification[site]; !ok {
			t.Errorf("%s calls reviewIntegrationNegotiation but has no mode-completeness classification; classify it in reviewIntegrationModeCompletenessClassification", site)
		}
	}
	for site := range reviewIntegrationModeCompletenessClassification {
		if !found[site] {
			t.Errorf("reviewIntegrationModeCompletenessClassification names %s, but it no longer calls reviewIntegrationNegotiation", site)
		}
	}
}

// reviewIntegrationEncodeOperationCallRegexp extracts the two result
// arguments passed to encodeReviewIntegrationOperation(stdout, negotiated|
// true, <operation constant>, <legacy>, <public>).
var reviewIntegrationEncodeOperationCallRegexp = regexp.MustCompile(`encodeReviewIntegrationOperation\(stdout, (?:negotiated|true), (ReviewIntegrationOperation\w+), (\w+), (\w+)(?:, (?:\*?\w+|""))?\)`)

// TestReviewIntegrationVacuousModeClassificationIsProvenBySource is Guard C's
// evidence half for the "vacuous" classification: it re-derives, from every
// encodeReviewIntegrationOperation(...) call site in review_facade.go and
// review_final_verification_retry.go, whether the legacy and public result
// arguments are the literal same identifier. Any command classified
// "vacuous" above whose call site(s) pass two DIFFERENT identifiers fails
// closed -- the structural proof no longer holds and the classification must
// be revisited (most likely to "hinted" or "explicit-refusal").
func TestReviewIntegrationVacuousModeClassificationIsProvenBySource(t *testing.T) {
	operationToFunc := map[string]string{
		"ReviewIntegrationOperationValidate":               "runReviewFacadeValidate",
		"ReviewIntegrationOperationBindSDD":                "runReviewBindSDD",
		"ReviewIntegrationOperationRetryFinalVerification": "runReviewRetryFinalVerification",
		"ReviewIntegrationOperationFinalize":               "runReviewFacadeFinalize",
	}
	vacuousByOperation := map[string]bool{}
	sawOperation := map[string]bool{}
	for _, file := range []string{"review_facade.go", "review_final_verification_retry.go"} {
		payload, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range reviewIntegrationEncodeOperationCallRegexp.FindAllStringSubmatch(string(payload), -1) {
			operation, legacy, public := match[1], match[2], match[3]
			sawOperation[operation] = true
			same := legacy == public
			if seen, ok := vacuousByOperation[operation]; ok && seen != same {
				t.Fatalf("%s has inconsistent encodeReviewIntegrationOperation call sites: one passes identical legacy/public arguments and another does not", operation)
			}
			vacuousByOperation[operation] = same
		}
	}
	if len(sawOperation) == 0 {
		t.Fatal("found no encodeReviewIntegrationOperation(...) call sites; the extraction regexp is stale")
	}
	for operation, fn := range operationToFunc {
		if !sawOperation[operation] {
			t.Fatalf("expected %s to be emitted via encodeReviewIntegrationOperation, but no call site was found", operation)
		}
		classification := reviewIntegrationModeCompletenessClassification[fn]
		isVacuous := vacuousByOperation[operation]
		switch classification {
		case "vacuous":
			if !isVacuous {
				t.Errorf("%s is classified %q, but its encodeReviewIntegrationOperation call passes different legacy/public arguments; the negotiated form now carries data the unnegotiated form lacks, so the classification is stale", fn, classification)
			}
		case "explicit-refusal", "hinted":
			if isVacuous {
				t.Errorf("%s is classified %q, but its encodeReviewIntegrationOperation call passes the identical legacy/public argument; there is no longer a data gap to refuse or hint about, so the classification is stale", fn, classification)
			}
		}
	}
}

// TestReviewIntegrationNonVacuousModeClassificationsAreEvidenced is Guard C's
// evidence half for "explicit-refusal" and "hinted": it proves each such
// function's own body actually contains the named guard, rather than trusting
// the classification map's comment.
func TestReviewIntegrationNonVacuousModeClassificationsAreEvidenced(t *testing.T) {
	cases := []struct {
		fn       string
		file     string
		evidence string
	}{
		// Issue #2447 replaced the named reviewStartNegotiateContractHint
		// constant with an up-front refusal for base-diff/workspace-overlay
		// candidates that select lenses (see runReviewFacadeStart's refusal
		// at reviewPreflightDirectRouteUncompletableReason). The additive
		// Hint field survives for the one shape #2447 left untouched --
		// workspace-mode candidates whose selected lenses still need the
		// negotiated form's repository_context -- so the evidence now points
		// at that call site instead of the removed constant.
		{fn: "runReviewFacadeStart", file: "review_facade.go", evidence: "reviewNegotiatedStartCommand(authority.InitialSnapshot, *runtimeAgent)"},
		{fn: "runReviewFacadeFinalize", file: "review_facade.go", evidence: "reviewContractRequiredForActionEligibilityReason"},
	}
	for _, testCase := range cases {
		t.Run(testCase.fn, func(t *testing.T) {
			if reviewIntegrationModeCompletenessClassification[testCase.fn] == "vacuous" {
				t.Fatalf("test case %s is stale: it is now classified vacuous, drop it from this table", testCase.fn)
			}
			payload, err := os.ReadFile(testCase.file)
			if err != nil {
				t.Fatal(err)
			}
			body := reviewExtractFuncBody(t, string(payload), testCase.fn)
			if !strings.Contains(body, testCase.evidence) {
				t.Errorf("%s no longer references %q; its mode-completeness classification is unproven", testCase.fn, testCase.evidence)
			}
		})
	}
}

// reviewExtractFuncBody returns the exact source text of one top-level
// function, matched by brace counting from `func <name>(` (or `func
// (receiver) <name>(`) to its closing brace, so evidence checks only ever see
// that one function's body.
func reviewExtractFuncBody(t *testing.T, source, name string) string {
	t.Helper()
	marker := regexp.MustCompile(`(?m)^func (\(\w+ \*?\w+\) )?` + regexp.QuoteMeta(name) + `\(`)
	loc := marker.FindStringIndex(source)
	if loc == nil {
		t.Fatalf("function %s not found", name)
	}
	rest := source[loc[0]:]
	brace := strings.IndexByte(rest, '{')
	if brace < 0 {
		t.Fatalf("function %s has no body", name)
	}
	depth := 0
	for index := brace; index < len(rest); index++ {
		switch rest[index] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return rest[:index+1]
			}
		}
	}
	t.Fatalf("function %s body never closes", name)
	return ""
}
