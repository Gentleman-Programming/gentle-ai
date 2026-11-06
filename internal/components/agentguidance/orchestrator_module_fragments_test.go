package agentguidance

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func testFragmentMarker(id, kind string) string {
	return "<!-- odd-orchestrator-fragment:" + id + ":" + kind + " -->\n"
}

func markedTestFragment(id, body string) string {
	return testFragmentMarker(id, "start") + body + testFragmentMarker(id, "end")
}

var testFragmentIDs = []string{"delegation.a", "skills.c", "delegation.b"}

// testFragmentDoc has two disjoint fragments of one module around a fragment
// of another, and a code fence inside a fragment body.
var testFragmentDoc = "# Core\n\n" + markedTestFragment("delegation.a", "A body\n") + "\nmid\n\n" +
	markedTestFragment("skills.c", "C body\n") + "\n" +
	markedTestFragment("delegation.b", "B body\n\n```text\ncode\n```\n") + "\ntail\n"

func TestDecodeOrchestratorFragmentsReturnsOrderedSpans(t *testing.T) {
	spans, err := decodeOrchestratorFragments(testFragmentDoc, testFragmentIDs)
	if err != nil {
		t.Fatalf("decode error = %v", err)
	}
	var want []orchestratorFragmentSpan
	for _, tc := range []struct{ id, body string }{
		{"delegation.a", "A body\n"},
		{"skills.c", "C body\n"},
		{"delegation.b", "B body\n\n```text\ncode\n```\n"},
	} {
		start := strings.Index(testFragmentDoc, testFragmentMarker(tc.id, "start"))
		bodyStart := start + len(testFragmentMarker(tc.id, "start"))
		bodyEnd := bodyStart + len(tc.body)
		want = append(want, orchestratorFragmentSpan{id: tc.id, start: start, bodyStart: bodyStart, bodyEnd: bodyEnd, end: bodyEnd + len(testFragmentMarker(tc.id, "end"))})
		if testFragmentDoc[bodyStart:bodyEnd] != tc.body {
			t.Fatalf("fixture body for %s is not where expected", tc.id)
		}
	}
	if !reflect.DeepEqual(spans, want) {
		t.Fatalf("spans = %+v, want %+v", spans, want)
	}

	// Dropping exactly the marker lines the spans name equals the strip.
	var withoutMarkers strings.Builder
	cursor := 0
	for _, span := range spans {
		withoutMarkers.WriteString(testFragmentDoc[cursor:span.start])
		withoutMarkers.WriteString(testFragmentDoc[span.bodyStart:span.bodyEnd])
		cursor = span.end
	}
	withoutMarkers.WriteString(testFragmentDoc[cursor:])
	wantStripped := "# Core\n\nA body\n\nmid\n\nC body\n\nB body\n\n```text\ncode\n```\n\ntail\n"
	if got := stripOrchestratorFragmentMarkers(testFragmentDoc); got != wantStripped || withoutMarkers.String() != wantStripped {
		t.Fatalf("strip = %q, span removal = %q, want %q", got, withoutMarkers.String(), wantStripped)
	}

	spans, err = decodeOrchestratorFragments("# Core\n```text\nno markers\n```\n", nil)
	if err != nil || spans != nil {
		t.Fatalf("unmarked document decode = %+v, %v; want no spans", spans, err)
	}
}

func TestDecodeOrchestratorFragmentsFailsClosed(t *testing.T) {
	marker, fragment, valid := testFragmentMarker, markedTestFragment, testFragmentDoc
	for _, tc := range []struct {
		name, doc, want string
		ids             []string
	}{
		{name: "duplicate fragment", doc: valid + fragment("delegation.a", "again\n"), want: "duplicate"},
		{name: "missing fragment", doc: "# Core\n" + fragment("delegation.a", "A\n") + fragment("delegation.b", "B\n"), want: "missing"},
		{name: "reversed markers", doc: strings.Replace(valid, marker("skills.c", "start")+"C body\n"+marker("skills.c", "end"), marker("skills.c", "end")+"C body\n"+marker("skills.c", "start"), 1), want: "without a start"},
		{name: "overlapping fragments", doc: "# Core\n" + marker("delegation.a", "start") + "A\n" + marker("skills.c", "start") + "C\n" + marker("delegation.a", "end") + "x\n" + marker("skills.c", "end") + fragment("delegation.b", "B\n"), want: "overlaps"},
		{name: "end of another open fragment", doc: "# Core\n" + marker("delegation.a", "start") + "A\n" + marker("skills.c", "end") + fragment("delegation.b", "B\n"), want: "overlaps"},
		{name: "unclosed fragment", doc: "# Core\n" + fragment("delegation.a", "A\n") + fragment("skills.c", "C\n") + marker("delegation.b", "start") + "B\n", want: "unclosed"},
		{name: "unknown fragment", doc: valid + fragment("delegation.z", "Z\n"), want: "unknown"},
		{name: "empty fragment", doc: strings.Replace(valid, "C body\n", "\n", 1), want: "empty"},
		{name: "malformed kind", doc: strings.Replace(valid, "skills.c:start", "skills.c:begin", 1), want: "malformed"},
		{name: "marker inside a line", doc: strings.Replace(valid, marker("skills.c", "start"), "text "+marker("skills.c", "start"), 1), want: "malformed"},
		{name: "marker without newline", doc: strings.TrimSuffix(strings.Replace(valid, "\ntail\n", "", 1), "\n"), want: "malformed"},
		{name: "marker with CRLF", doc: strings.Replace(valid, "skills.c:start -->\n", "skills.c:start -->\r\n", 1), want: "malformed"},
		{name: "token mentioned in prose", doc: valid + "See odd-orchestrator-fragment: markers.\n", want: "malformed"},
		{name: "marker inside code fence", doc: "# Core\n```text\n" + fragment("delegation.a", "A\n") + "```\n" + fragment("skills.c", "C\n") + fragment("delegation.b", "B\n"), want: "code fence"},
		{name: "fence left open across an end marker", doc: "# Core\n" + marker("delegation.a", "start") + "```text\nA\n" + marker("delegation.a", "end") + "```\n" + fragment("skills.c", "C\n") + fragment("delegation.b", "B\n"), want: "code fence"},
		{name: "duplicate expected ID", doc: valid, ids: []string{"delegation.a", "skills.c", "delegation.b", "skills.c"}, want: "is duplicate"},
		{name: "malformed expected ID", doc: valid, ids: []string{"delegation.a", "skills.c", "delegation.b", "Skills"}, want: "is malformed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ids := testFragmentIDs
			if tc.ids != nil {
				ids = tc.ids
			}
			spans, err := decodeOrchestratorFragments(tc.doc, ids)
			if !errors.Is(err, errInvalidOrchestratorFragments) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %v containing %q", err, errInvalidOrchestratorFragments, tc.want)
			}
			if spans != nil {
				t.Fatalf("rejected document returned partial spans %+v", spans)
			}
		})
	}
}

// The strip removes only whole well-formed marker lines; anything malformed
// survives, so a renderer that checks for the token afterwards fails closed.
func TestStripOrchestratorFragmentMarkersIsExact(t *testing.T) {
	for _, malformed := range []string{
		"text <!-- odd-orchestrator-fragment:skills.c:start -->\n",
		"<!-- odd-orchestrator-fragment:skills.c:start -->\r\n",
		"<!-- odd-orchestrator-fragment:skills.c:begin -->\n",
		"<!-- odd-orchestrator-fragment:Skills.c:start -->\n",
		"<!-- odd-orchestrator-fragment:skills.c:end -->",
	} {
		doc := "before\n" + malformed
		if got := stripOrchestratorFragmentMarkers(doc); got != doc || !strings.Contains(got, orchestratorFragmentMarkerToken) {
			t.Errorf("strip changed malformed marker %q: %q", malformed, got)
		}
	}
}
