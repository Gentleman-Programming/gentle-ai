// This file is the by-design envelope layer of the refusal ratchet: a
// closed-vocabulary marker form that asset and prompt collectors use to
// prove every terminal directive (an imperative sentence that routes the
// reader to a CLI verb) either names a runnable `gentle-ai review <verb>`
// continuation or declares a closed-vocabulary shape explaining why no
// command can honestly exist there.
//
// The two layers MUST agree on the closed vocabulary. refusalRatchetByDesignShapes
// (test-only, in refusal_resolution_ratchet_test.go) and the ByDesign* constants
// in bench/classify.go already pin each other; the production-side
// byDesignShapeSet below is the third copy and TestByDesignEnvelopeVocabularyMatch
// pins it against refusalRatchetByDesignShapes so the three cannot fork.
//
// What this file does NOT do: walk a document. ParseByDesignEnvelope parses ONE
// directive line; ParseMarkdownByDesignEnvelope (in
// refusal_resolution_ratchet_assets.go) walks a whole document and threads the
// named-verb resolver through ReviewDispatchableReviewVerbs, failing closed on
// any verb that does not dispatch.
package cli

import (
	"fmt"
	"regexp"
	"strings"
)

// ByDesignShape is one of the closed by-design shapes the ratchet accepts.
// Mirrored from refusalRatchetByDesignShapes for production callers.
type ByDesignShape string

const (
	byDesignOperatorKnowledge ByDesignShape = "operator-knowledge"
	byDesignWorldAction       ByDesignShape = "world-action"
	byDesignHumanAuthority    ByDesignShape = "human-authority"
)

// byDesignShapeSet is the production-side closed by-design vocabulary. It MUST
// stay identical to refusalRatchetByDesignShapes and to the ByDesign* constants
// in bench/classify.go; TestByDesignEnvelopeVocabularyMatch enforces the
// mechanical half of that, the bench classifier pins its own half.
var byDesignShapeSet = map[string]bool{
	"operator-knowledge": true,
	"world-action":       true,
	"human-authority":    true,
}

// byDesignMarkerRegexp matches the markdown marker form:
//
//	<!-- by-design: <shape> -->
//
// where <shape> is one of the closed vocabulary. The marker is invisible to
// commonmark and to most renderers; the prefix `by-design:` is intentionally
// shorter than the Go source prefix (the // refusalRatchetMarkerHint marker
// defined in refusal_resolution_ratchet_test.go) so the two never collide,
// but both parse against the same closed shape set.
var byDesignMarkerRegexp = regexp.MustCompile(`<!--\s*by-design:\s*([a-z-]+)\s*-->`)

// byDesignVerbRegexp captures the verb in `gentle-ai review <verb>` references.
// Requiring the literal " review " (with surrounding spaces) keeps the verb
// namespace unambiguous: the existing refusalRatchetNamedContinuationRegexp
// matches every `gentle-ai <anything>`, which is the broader structural
// check; this narrower regex is the gate the dispatchability check runs
// against.
var byDesignVerbRegexp = regexp.MustCompile(`gentle-ai review ([a-z][a-z-]*)`)

// ByDesignEnvelope is one parsed directive from an envelope reason field, an
// asset markdown directive, or a prompt instructions block. An envelope
// satisfies a terminal directive iff it EITHER names a runnable verb (Verb
// non-empty) OR declares a closed-vocabulary by-design shape (Shape non-empty)
// explaining why no command can honestly exist there. Both arms are mutually
// exclusive: a directive that names a verb AND a marker is contradictory and
// fails closed.
type ByDesignEnvelope struct {
	// Line is the 1-based line number the directive was extracted from.
	Line int
	// Verb is the dispatched review verb iff the directive names one.
	Verb string
	// Shape is the by-design shape iff the directive declares one.
	Shape ByDesignShape
	// Literal is the verbatim directive text the envelope parsed. Used for
	// error reporting and for the per-(file, directive-text) baseline key.
	Literal string
}

// IsNamed returns true iff the envelope names a runnable review verb.
func (e ByDesignEnvelope) IsNamed() bool { return e.Verb != "" }

// IsAnnotated returns true iff the envelope declares a closed-vocabulary
// by-design marker.
func (e ByDesignEnvelope) IsAnnotated() bool { return e.Shape != "" }

// ParseByDesignEnvelope parses one directive line and returns the envelope it
// satisfies, or an error when the line is contradictory (names a verb AND
// declares a marker), declares an unknown shape, or carries neither.
//
// The caller decides which lines to pass in. The asset walker (in
// refusal_resolution_ratchet_assets.go) pre-filters for `gentle-ai review `
// or `by-design:` substrings; callers that skip that pre-filter will see the
// "neither" error for any prose-only line. This is intentional: a line that
// carries no marker and no verb is a violation at every site the collector
// touches, so the parser fails loudly rather than passing it silently.
func ParseByDesignEnvelope(line string, lineNo int) (ByDesignEnvelope, error) {
	env := ByDesignEnvelope{Line: lineNo, Literal: line}

	marker := byDesignMarkerRegexp.FindStringSubmatch(line)
	verb := byDesignVerbRegexp.FindStringSubmatch(line)

	if marker != nil {
		shape := ByDesignShape(marker[1])
		if !byDesignShapeSet[string(shape)] {
			// refusal:by-design world-action: an unknown marker shape is a fixture bug; repair the markdown, no command can fix it
			return ByDesignEnvelope{}, fmt.Errorf("line %d: marker shape %q is not in the closed vocabulary (operator-knowledge, world-action, human-authority)", lineNo, marker[1])
		}
		env.Shape = shape
	}
	if verb != nil {
		env.Verb = verb[1]
	}

	if env.IsNamed() && env.IsAnnotated() {
		// refusal:by-design world-action: a contradictory marker is a fixture bug; pick one claim, no command can fix it
		return ByDesignEnvelope{}, fmt.Errorf("line %d: directive names the dispatched review verb %q AND declares by-design %q; a refusal either has a runnable exit or it does not -- the two claims are mutually exclusive", lineNo, env.Verb, env.Shape)
	}
	if !env.IsNamed() && !env.IsAnnotated() {
		// refusal:by-design world-action: an empty directive is a fixture bug; name a verb or add a marker, no command can fix it
		return ByDesignEnvelope{}, fmt.Errorf("line %d: directive carries neither a runnable review-verb continuation nor a by-design marker; the operator has no exit", lineNo)
	}
	return env, nil
}

// byDesignDirectivePreFilter is the quick scan ParseMarkdownByDesignEnvelope
// uses before invoking ParseByDesignEnvelope. Lines that match neither
// substring are skipped silently -- prose may mention `gentle-ai` in passing
// without being a directive.
func byDesignDirectivePreFilter(line string) bool {
	return strings.Contains(line, "gentle-ai review ") || strings.Contains(line, "by-design:")
}
