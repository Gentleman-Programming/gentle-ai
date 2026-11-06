package agentguidance

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// #5256 marks the shared orchestrator text that may move into on-demand
// modules with whole-line fragment markers:
//
//	<!-- odd-orchestrator-fragment:<module>.<part>:start -->
//	<!-- odd-orchestrator-fragment:<module>.<part>:end -->
//
// This file owns only that marker grammar: decoding a document into ordered
// byte spans and stripping the exact marker lines. It knows nothing about
// runtimes, module files or pointers.

// orchestratorFragmentMarkerToken identifies a fragment marker in any form; a
// line carrying it that is not a well-formed marker line fails closed.
const orchestratorFragmentMarkerToken = "odd-orchestrator-fragment:"

var errInvalidOrchestratorFragments = errors.New("invalid orchestrator module fragments")

// orchestratorFragmentMarkerLine matches one whole well-formed marker line;
// stripping exactly these lines restores the bytes the markers were added to.
var orchestratorFragmentMarkerLine = regexp.MustCompile(`(?m)^<!-- odd-orchestrator-fragment:[a-z]+\.[a-z0-9-]+:(?:start|end) -->\n`)

var orchestratorFragmentMarker = regexp.MustCompile(`^<!-- odd-orchestrator-fragment:([a-z]+\.[a-z0-9-]+):(start|end) -->$`)

var orchestratorFragmentID = regexp.MustCompile(`^[a-z]+\.[a-z0-9-]+$`)

// orchestratorFragmentSpan locates one marked fragment: start is the first
// byte of its start marker line, [bodyStart, bodyEnd) holds the bytes between
// its marker lines, and end is the byte after its end marker line.
type orchestratorFragmentSpan struct {
	id                             string
	start, bodyStart, bodyEnd, end int
}

func stripOrchestratorFragmentMarkers(content string) string {
	return orchestratorFragmentMarkerLine.ReplaceAllString(content, "")
}

// decodeOrchestratorFragments returns the spans of doc in order of
// appearance. Every expected ID must appear exactly once; any duplicate,
// missing, unknown, reversed, overlapping, unclosed, empty, malformed or
// code-fenced marker fails the whole decode without a partial result.
func decodeOrchestratorFragments(doc string, ids []string) ([]orchestratorFragmentSpan, error) {
	invalid := func(format string, args ...any) ([]orchestratorFragmentSpan, error) {
		return nil, fmt.Errorf("%w: "+format, append([]any{errInvalidOrchestratorFragments}, args...)...)
	}
	expected := map[string]bool{}
	for _, id := range ids {
		if !orchestratorFragmentID.MatchString(id) {
			return invalid("expected fragment ID %q is malformed", id)
		}
		if expected[id] {
			return invalid("expected fragment ID %q is duplicate", id)
		}
		expected[id] = true
	}

	var spans []orchestratorFragmentSpan
	seen := map[string]bool{}
	var open *orchestratorFragmentSpan
	fenced := false
	for offset, lineNumber := 0, 1; offset < len(doc); lineNumber++ {
		next := len(doc)
		if end := strings.IndexByte(doc[offset:], '\n'); end >= 0 {
			next = offset + end + 1
		}
		line := doc[offset:next]
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
		}
		if !strings.Contains(line, orchestratorFragmentMarkerToken) {
			offset = next
			continue
		}
		match := orchestratorFragmentMarker.FindStringSubmatch(strings.TrimSuffix(line, "\n"))
		switch {
		case match == nil || !strings.HasSuffix(line, "\n"):
			return invalid("malformed fragment marker on line %d", lineNumber)
		case fenced:
			return invalid("fragment marker inside a code fence on line %d", lineNumber)
		}
		id, kind := match[1], match[2]
		switch {
		case !expected[id]:
			return invalid("unknown fragment %q on line %d", id, lineNumber)
		case kind == "start" && open != nil:
			return invalid("fragment %q overlaps open fragment %q on line %d", id, open.id, lineNumber)
		case kind == "start" && seen[id]:
			return invalid("duplicate fragment %q on line %d", id, lineNumber)
		case kind == "start":
			seen[id] = true
			open = &orchestratorFragmentSpan{id: id, start: offset, bodyStart: next}
		case open == nil:
			return invalid("fragment %q ends without a start on line %d", id, lineNumber)
		case open.id != id:
			return invalid("fragment %q overlaps open fragment %q on line %d", id, open.id, lineNumber)
		default:
			if strings.TrimSpace(doc[open.bodyStart:offset]) == "" {
				return invalid("fragment %q is empty", id)
			}
			open.bodyEnd, open.end = offset, next
			spans = append(spans, *open)
			open = nil
		}
		offset = next
	}
	if open != nil {
		return invalid("fragment %q is unclosed", open.id)
	}
	for _, id := range ids {
		if !seen[id] {
			return invalid("fragment %q is missing", id)
		}
	}
	return spans, nil
}
