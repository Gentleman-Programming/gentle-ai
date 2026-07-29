package cli

import (
	"os"
	"regexp"
	"strings"
	"testing"
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

// reviewStopReasonDocsSection returns the text of docs/review-integration.md
// strictly between the reviewStopReasonDocsTableHeading heading and the next
// heading of the same or a higher level, so the row regexp only ever sees
// this one table.
func reviewStopReasonDocsSection(t *testing.T, docs string) string {
	t.Helper()
	start := strings.Index(docs, reviewStopReasonDocsTableHeading)
	if start < 0 {
		t.Fatalf("docs/review-integration.md is missing the %q heading", reviewStopReasonDocsTableHeading)
	}
	rest := docs[start+len(reviewStopReasonDocsTableHeading):]
	if end := strings.Index(rest, "\n## "); end >= 0 {
		rest = rest[:end]
	}
	if end := strings.Index(rest, "\n### "); end >= 0 {
		rest = rest[:end]
	}
	return rest
}
