// This file is the by-design envelope layer of the refusal ratchet: a
// closed-vocabulary marker form for asset and prompt collectors proving every
// terminal directive either names a runnable `gentle-ai review <verb>`
// continuation or declares a closed-vocabulary shape explaining why no
// command can honestly exist there.
//
// What this file does NOT do: walk a document. ParseByDesignEnvelope parses
// ONE directive line. Document-walking helpers for the asset and prompt
// collectors live in refusal_resolution_ratchet_assets.go and reuse
// ParseByDesignEnvelope per line.
package cli

import (
	"fmt"
	"regexp"
)

// RefusalRatchetByDesignShapes is the closed by-design vocabulary shared with
// the bench classifier (bench/classify.go); both sides read the same three
// keys, so the ratchet header and the bench JSON stay in agreement.
var RefusalRatchetByDesignShapes = map[string]bool{
	"operator-knowledge": true, // the product cannot know a value only the operator has
	"world-action":       true, // the exit is an action in the world, not a command
	"human-authority":    true, // the block is a human decision by design
}

// byDesignMarkerRegexp matches the markdown marker form:
//
//	<!-- by-design: <shape> -->
//
// where <shape> is one of the closed vocabulary. The marker is invisible to
// commonmark and to most renderers.
var byDesignMarkerRegexp = regexp.MustCompile(`<!--\s*by-design:\s*([a-z-]+)\s*-->`)

// byDesignVerbRegexp captures the verb in `gentle-ai review <verb>` references.
// Requiring the literal " review " (with surrounding spaces) keeps the verb
// namespace unambiguous.
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
	Shape string
	// Literal is the verbatim directive text the envelope parsed.
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
// The caller decides which lines to pass in. Lines that carry no marker and
// no verb produce a "neither" error -- a line at every site the collector
// touches must name an exit or declare a by-design shape.
func ParseByDesignEnvelope(line string, lineNo int) (ByDesignEnvelope, error) {
	env := ByDesignEnvelope{Line: lineNo, Literal: line}

	marker := byDesignMarkerRegexp.FindStringSubmatch(line)
	verb := byDesignVerbRegexp.FindStringSubmatch(line)

	if marker != nil {
		shape := marker[1]
		if !RefusalRatchetByDesignShapes[shape] {
			// refusal:by-design world-action: an unknown marker shape is a fixture bug; repair the markdown, no command can fix it
			return ByDesignEnvelope{}, fmt.Errorf("line %d: marker shape %q is not in the closed vocabulary (operator-knowledge, world-action, human-authority)", lineNo, marker[1])
		}
		env.Shape = shape
	}
	if verb != nil {
		env.Verb = verb[1]
		// Enforce dispatchability: the ByDesignEnvelope type documents that
		// "Verb is the dispatched review verb iff the directive names one".
		// A verb that is not in ReviewDispatchableReviewVerbs is a fixture
		// bug — the directive claims a runnable exit that does not exist.
		dispatched, err := ReviewDispatchableReviewVerbs()
		if err != nil {
			return ByDesignEnvelope{}, fmt.Errorf("line %d: dispatchability check failed: %w", lineNo, err)
		}
		if !dispatched[env.Verb] {
			// refusal:by-design world-action: a non-dispatchable verb in a directive is a fixture bug; repair the markdown or the dispatch surface, no command can fix it
			return ByDesignEnvelope{}, fmt.Errorf("line %d: directive names review verb %q which is not in the dispatchable set; the directive claims a runnable exit that does not exist", lineNo, env.Verb)
		}
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
