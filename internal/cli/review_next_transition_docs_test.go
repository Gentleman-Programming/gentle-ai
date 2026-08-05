package cli

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
)

// reviewStopTransitionCallRegexp extracts every literal reason code passed to
// reviewStopTransition(...) in review_next_transition.go. A non-literal call
// (a variable or expression) would not match here and must be converted to a
// literal before this test can see it — that is deliberate: the docs table
// below can only ever cover reason codes it can read as plain text.
var reviewStopTransitionCallRegexp = regexp.MustCompile(`reviewStopTransition\("([a-z_]+)"\)`)

// reviewStopReasonDocsTableHeading marks the start of the docs table this
// test cross-checks. reviewStopReasonDocsTableRowRegexp then extracts the
// reason code named at the start of each row inside that section only —
// docs/review-integration.md contains several other tables whose first
// column is also a single backtick-quoted word (gates, applicability, ...),
// so matching the whole file would false-positive against those.
const reviewStopReasonDocsTableHeading = "### Continue after a stop reason code"

var reviewStopReasonDocsTableRowRegexp = regexp.MustCompile("(?m)^\\| `([a-z_]+)` \\|")

// TestEveryReviewStopReasonCodeHasADocsContinuation pins that every stop
// reason code newReviewNextTransition (and its helpers) can emit from
// internal/cli/review_next_transition.go has exactly one row in the
// "Continue after a stop reason code" table in docs/review-integration.md.
// It fails closed in both directions: a reason code added to the Go source
// without a matching docs row, and a docs row naming a reason code the
// source no longer emits — so the table can never silently drift from the
// wire contract it documents.
func TestEveryReviewStopReasonCodeHasADocsContinuation(t *testing.T) {
	source, err := os.ReadFile("review_next_transition.go")
	if err != nil {
		t.Fatal(err)
	}
	matches := reviewStopTransitionCallRegexp.FindAllStringSubmatch(string(source), -1)
	if len(matches) == 0 {
		t.Fatal("found no reviewStopTransition(\"...\") call sites in review_next_transition.go; the extraction regexp is stale")
	}
	sourceCodes := map[string]bool{}
	for _, match := range matches {
		sourceCodes[match[1]] = true
	}

	docs, err := os.ReadFile("../../docs/review-integration.md")
	if err != nil {
		t.Fatal(err)
	}
	section := reviewStopReasonDocsSection(t, string(docs))
	rows := reviewStopReasonDocsTableRowRegexp.FindAllStringSubmatch(section, -1)
	if len(rows) == 0 {
		t.Fatal("found no rows in the \"Continue after a stop reason code\" table in docs/review-integration.md; the table heading or row shape moved")
	}
	docCodes := map[string]bool{}
	for _, row := range rows {
		docCodes[row[1]] = true
	}

	for code := range sourceCodes {
		if !docCodes[code] {
			t.Errorf("reason code %q is emitted by review_next_transition.go but has no row in the docs/review-integration.md stop-reason-code table", code)
		}
	}
	for code := range docCodes {
		if !sourceCodes[code] {
			t.Errorf("docs/review-integration.md documents reason code %q, which review_next_transition.go no longer emits", code)
		}
	}
}

// reviewLedgerContractAsset is the shipped, embedded orchestrator contract
// every runtime's rendered review protocol draws from (internal/assets is
// go:embed'd into the binary; docs/ is not — see internal/assets/assets.go).
// A consuming orchestrator can read this file; it cannot read docs/.
const reviewLedgerContractAsset = "skills/_shared/review-ledger-contract.md"

// TestEveryReviewStopReasonCodeHasAShippedContinuation is
// TestEveryReviewStopReasonCodeHasADocsContinuation's sibling for the
// channel an orchestrator can actually read. docs/review-integration.md's
// "Continue after a stop reason code" table is correct and complete, but
// internal/assets/assets.go's go:embed tree does not cover docs/, so nothing
// shipped ever carries it to a consumer that is contractually forbidden from
// routing off anything but the returned next_transition and the shipped
// contract. This test requires the SAME heading and row shape to exist,
// covering the SAME codes, inside the shipped
// internal/assets/skills/_shared/review-ledger-contract.md asset — the one
// file every runtime's rendered orchestrator prompt actually embeds.
func TestEveryReviewStopReasonCodeHasAShippedContinuation(t *testing.T) {
	source, err := os.ReadFile("review_next_transition.go")
	if err != nil {
		t.Fatal(err)
	}
	matches := reviewStopTransitionCallRegexp.FindAllStringSubmatch(string(source), -1)
	if len(matches) == 0 {
		t.Fatal("found no reviewStopTransition(\"...\") call sites in review_next_transition.go; the extraction regexp is stale")
	}
	sourceCodes := map[string]bool{}
	for _, match := range matches {
		sourceCodes[match[1]] = true
	}

	contract := assets.MustRead(reviewLedgerContractAsset)
	section := reviewStopReasonDocsSection(t, contract)
	rows := reviewStopReasonDocsTableRowRegexp.FindAllStringSubmatch(section, -1)
	if len(rows) == 0 {
		t.Fatalf("found no rows in the %q table in the shipped %s asset; the contract never embeds the stop-reason continuation table an orchestrator can read", reviewStopReasonDocsTableHeading, reviewLedgerContractAsset)
	}
	assetCodes := map[string]bool{}
	for _, row := range rows {
		assetCodes[row[1]] = true
	}

	for code := range sourceCodes {
		if !assetCodes[code] {
			t.Errorf("reason code %q is emitted by review_next_transition.go but has no row in the shipped %s stop-reason table", code, reviewLedgerContractAsset)
		}
	}
	for code := range assetCodes {
		if !sourceCodes[code] {
			t.Errorf("shipped %s documents reason code %q, which review_next_transition.go no longer emits", reviewLedgerContractAsset, code)
		}
	}

	// The universal self-service exit (blocking-budget rule 2) must be
	// reachable from every row whose continuation names no other runnable
	// `gentle-ai` command, so a terminal stop the product cannot resolve
	// automatically never reads as "nothing more to do" on the one channel
	// the orchestrator is allowed to route from.
	namesOtherContinuation := regexp.MustCompile("`gentle-ai [a-z][a-z-]*|`--[a-z][a-z-]*")
	for _, line := range strings.Split(section, "\n") {
		row := reviewStopReasonDocsTableRowRegexp.FindStringSubmatch(line)
		if row == nil {
			continue
		}
		code := row[1]
		if strings.Contains(line, "gentle-ai review mode disable") {
			continue
		}
		if !namesOtherContinuation.MatchString(line) {
			t.Errorf("shipped %s row for %q names no runnable `gentle-ai` command, no `--flag` to pass on the same invocation, and no `gentle-ai review mode disable` fallback, so this stop reads as a dead end", reviewLedgerContractAsset, code)
		}
	}
}

// TestReviewStopReasonDocsSectionStopsAtTheNextHeadingOfAnyLevel is the
// RED-first proof that reviewStopReasonDocsSection's boundary is real, not
// accidental. The original implementation truncated sequentially at the
// first "\n## " and then the first "\n### " inside what remained; both
// substring checks require an EXACT hash count, so a heading with any other
// depth (one hash, four hashes, ...) is invisible to both and its content --
// including any table row shaped like the reason-code table's own rows --
// silently leaks into the captured section. Verified by execution: this
// test fails against the two-step implementation because the four-hash
// heading below matches neither "\n## " nor "\n### " as an exact substring.
func TestReviewStopReasonDocsSectionStopsAtTheNextHeadingOfAnyLevel(t *testing.T) {
	doc := reviewStopReasonDocsTableHeading + "\n\n" +
		"section body\n\n" +
		"| `real_code` | continuation |\n\n" +
		"#### A deeper heading no exact \"## \" or \"### \" substring check ever matches\n\n" +
		"| `leaked_code` | this row must never be captured |\n\n" +
		"## A later top-level heading\n\n" +
		"| `also_leaked_code` | this row must never be captured either |\n"
	section := reviewStopReasonDocsSection(t, doc)
	if !strings.Contains(section, "real_code") {
		t.Fatalf("section lost its own real row: %q", section)
	}
	for _, leaked := range []string{"leaked_code", "also_leaked_code"} {
		if strings.Contains(section, leaked) {
			t.Fatalf("section swallowed content past the next heading (%q leaked): %q", leaked, section)
		}
	}
}

// reviewStopReasonDocsCompleteDocuments is the shared document set both
// completeness guards below scan: the shipped asset every runtime embeds,
// and the docs/review-integration.md source it was distilled from.
func reviewStopReasonDocsCompleteDocuments(t *testing.T) map[string]string {
	t.Helper()
	docsBytes, err := os.ReadFile("../../docs/review-integration.md")
	if err != nil {
		t.Fatal(err)
	}
	return map[string]string{
		"shipped asset (" + reviewLedgerContractAsset + ")": assets.MustRead(reviewLedgerContractAsset),
		"docs/review-integration.md":                        string(docsBytes),
	}
}

// reviewStatusNextTransitionInvocationRegexp matches any backtick-quoted
// `gentle-ai review status ... --next-transition` invocation.
var reviewStatusNextTransitionInvocationRegexp = regexp.MustCompile("`gentle-ai review status[^`]*--next-transition`")

// TestNamedReviewStatusNextTransitionIsAlwaysComplete is the execution-based
// RED-first proof for adversarial finding F1: `gentle-ai review status
// --next-transition` alone is refused by this real CLI --
//
//	Error: --action-eligibility and --next-transition require --contract gentle-ai.review-integration/v1
//
// -- and even the error's own suggested `--contract` names the LEGACY v1
// schema, not the v2 contract this table's own Route section requires
// routing from. The only form that actually runs is `gentle-ai review
// status --cwd <repo> --contract gentle-ai.review-integration/v2 --agent
// claude-code --next-transition` (confirmed by execution: exit 0, real
// next_transition JSON returned). Every backtick-quoted invocation in the
// shipped asset and its docs source must be this complete form.
func TestNamedReviewStatusNextTransitionIsAlwaysComplete(t *testing.T) {
	for label, content := range reviewStopReasonDocsCompleteDocuments(t) {
		for _, invocation := range reviewStatusNextTransitionInvocationRegexp.FindAllString(content, -1) {
			if !strings.Contains(invocation, "--contract gentle-ai.review-integration/v2") || !reviewAgentBindingRegexp.MatchString(invocation) {
				t.Errorf("%s: %s is incomplete -- the real CLI refuses --next-transition without --contract gentle-ai.review-integration/v2 and a bound --agent (verified by execution)", label, invocation)
			}
		}
	}
}

// reviewAgentBindingRegexp requires a bound `--agent` value without pinning
// which one. Pinning `claude-code` here is what let issue #2440 through: the
// shipped asset renders into EVERY runtime's instructions, so it carries the
// substitution placeholder and the renderer binds the runtime that is actually
// executing. What must never happen is an --agent flag with nothing behind it,
// which the real CLI refuses.
var reviewAgentBindingRegexp = regexp.MustCompile("--agent [^ `]+")

// reviewReopenResultsInvocationRegexp matches any backtick-quoted
// `gentle-ai review reopen-results ...` invocation.
var reviewReopenResultsInvocationRegexp = regexp.MustCompile("`gentle-ai review reopen-results[^`]*`")

// TestNamedReviewReopenResultsIsAlwaysComplete is the execution-based
// RED-first proof for adversarial finding F7: `gentle-ai review
// reopen-results --prepare --quarantine-lens <lens>` alone is refused --
//
//	Error: review reopen-results requires --cwd, --lineage, --expected-revision, --target, --reason, and --actor
//
// (verified by execution). Every backtick-quoted invocation must name all
// six required flags.
func TestNamedReviewReopenResultsIsAlwaysComplete(t *testing.T) {
	const bareNominalReference = "`gentle-ai review reopen-results`"
	requiredFlags := []string{"--cwd", "--lineage", "--expected-revision", "--target", "--reason", "--actor"}
	for label, content := range reviewStopReasonDocsCompleteDocuments(t) {
		for _, invocation := range reviewReopenResultsInvocationRegexp.FindAllString(content, -1) {
			// A bare, flagless mention is a nominal reference to the command
			// ("`gentle-ai review reopen-results` is a bounded maintenance
			// operation..."), not an attempted invocation -- only a span that
			// already carries at least one flag is claiming to be runnable.
			if invocation == bareNominalReference {
				continue
			}
			for _, flag := range requiredFlags {
				if !strings.Contains(invocation, flag) {
					t.Errorf("%s: %s is missing required %s -- the real CLI refuses reopen-results without it (verified by execution)", label, invocation, flag)
				}
			}
		}
	}
}

// reviewModeDisableInvocationRegexp matches any backtick-quoted `gentle-ai
// review mode disable ...` invocation.
var reviewModeDisableInvocationRegexp = regexp.MustCompile("`gentle-ai review mode disable[^`]*`")

// TestNamedReviewModeDisableIsAlwaysCloneScoped is the execution-based
// RED-first proof for adversarial finding F6: `--scope` defaults to
// `global` (review_mode.go's own flag default), so `gentle-ai review mode
// disable` with no `--scope` disables receipt-driven development for every
// repository on the machine, not just the one the orchestrator meant.
// Verified by execution: the bare form writes ~/.gentle-ai/state.json;
// `--scope clone --cwd <repo>` writes only under that repository's own
// .git/gentle-ai directory. Every invocation in the shipped asset must name
// the clone-scoped form.
func TestNamedReviewModeDisableIsAlwaysCloneScoped(t *testing.T) {
	content := assets.MustRead(reviewLedgerContractAsset)
	invocations := reviewModeDisableInvocationRegexp.FindAllString(content, -1)
	if len(invocations) == 0 {
		t.Fatal("found no `gentle-ai review mode disable` invocations in the shipped asset; the extraction is stale")
	}
	for _, invocation := range invocations {
		if !strings.Contains(invocation, "--scope clone") || !strings.Contains(invocation, "--cwd <repo>") {
			t.Errorf("shipped asset: %s defaults to global scope if run as printed (verified by execution: omitting --scope writes ~/.gentle-ai/state.json machine-wide) -- name --scope clone --cwd <repo> instead", invocation)
		}
	}
}

// TestShippedUnchangedOrUnverifiedAuthorityNamesTheRealPrecondition is the
// execution-based RED-first proof for adversarial finding F4: `gentle-ai
// review start` on a candidate whose target is unchanged from the current
// authority does not start a fresh lineage -- it resumes the SAME one
// (confirmed by execution: the response reports `"action": "resumed"` with
// the identical lineage_id). Naming only `gentle-ai review start` loops the
// consumer back to the same stop. The row/entry must disclose that the
// candidate needs to change first.
func TestShippedUnchangedOrUnverifiedAuthorityNamesTheRealPrecondition(t *testing.T) {
	for label, content := range reviewStopReasonDocsCompleteDocuments(t) {
		section := reviewStopReasonDocsSection(t, content)
		row := reviewStopReasonTableRow(t, section, "unchanged_or_unverified_authority")
		lowered := strings.ToLower(row)
		if !strings.Contains(lowered, "resum") {
			t.Errorf("%s: unchanged_or_unverified_authority row never discloses that `review start` on an unchanged candidate only resumes the same lineage: %q", label, row)
		}
		if !strings.Contains(lowered, "change") {
			t.Errorf("%s: unchanged_or_unverified_authority row never states that the candidate must change first: %q", label, row)
		}
	}
}

// reviewStopReasonTableRow returns the single table-row line for the given
// reason code inside section, or fails the test if it is not found.
func reviewStopReasonTableRow(t *testing.T, section, code string) string {
	t.Helper()
	prefix := "| `" + code + "` |"
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	t.Fatalf("no table row found for reason code %q", code)
	return ""
}

// reviewStopReasonDocsNextHeadingRegexp matches the start of ANY markdown
// heading line (one to six leading `#`s). A section's own content always
// ends where the next heading begins, at any depth -- unlike the two exact-
// hash-count substring checks this replaced, which only recognized `## ` and
// `### ` and silently let a heading of any other depth (one hash, four
// hashes, ...) and everything under it leak into the captured section.
var reviewStopReasonDocsNextHeadingRegexp = regexp.MustCompile(`(?m)^#{1,6} `)

// reviewStopReasonDocsSection returns the text of the given document
// strictly between the reviewStopReasonDocsTableHeading heading and the next
// heading at any depth, so the row regexp only ever sees this one table.
// Shared by the docs/review-integration.md check and the shipped-asset
// check; both name their own source in the failure message.
func reviewStopReasonDocsSection(t *testing.T, docs string) string {
	t.Helper()
	start := strings.Index(docs, reviewStopReasonDocsTableHeading)
	if start < 0 {
		t.Fatalf("document is missing the %q heading", reviewStopReasonDocsTableHeading)
	}
	rest := docs[start+len(reviewStopReasonDocsTableHeading):]
	if loc := reviewStopReasonDocsNextHeadingRegexp.FindStringIndex(rest); loc != nil {
		rest = rest[:loc[0]]
	}
	return rest
}
