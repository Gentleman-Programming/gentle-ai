package cli

// This file is the OTHER direction of the "a refusal must name something that
// works" contract, the direction the 2026-07-27 detection-gap audit found
// missing (docs/audits/2026-07-27-detection-gap-audit.md, fix #1).
//
// TestEveryNamedReviewContinuationIsStructurallyReal proves every continuation
// a refusal NAMES really exists. Nothing proved that a refusal whose
// resolution exists gets NAMED: a refusal with an existing, unnamed exit is
// invisible to that guard, to the deadcode ratchet (dispatch-reachable code is
// "live"), and to every inward-facing test. That single asymmetry is the
// mechanism behind the abandon token, the reset, `reopen-results`, the
// stranded successor, and the reconciliation dead end.
//
// WHAT COUNTS AS A REFUSAL-ORIGIN SITE. The old walk enumerated error
// CONSTRUCTORS, so a refusal escaped by changing how its bytes were produced
// rather than what they said. #3471 and #2416 are two symptoms of that one
// gap. The walk now collects by CARRIER -- by how the sentence reaches a
// reader -- and there are three:
//
//  1. `errors.New` / `fmt.Errorf` with a statically known message.
//  2. A `return` of a statically known string from an `Error() string`
//     method. Not a heuristic about what "looks like" a refusal: the value
//     that method returns IS the sentence the operator reads, exactly like
//     the argument of `errors.New`. #3471 is what its absence cost --
//     `GateRemoteFetchRequiredError.Error()` moved a governed refusal into
//     that shape and silently left coverage, and the only trace was a
//     baseline entry that matched nothing.
//  3. A refusal-explanation struct field (#2416), which a consuming agent
//     acts on exactly as it acts on an error string. Carriers 1 and 2 are
//     ENFORCED against the baseline; carrier 3 is walked, listed, and capped
//     but not yet frozen site by site, for the reasons written at
//     refusalRatchetFieldCarriedCeiling. Do not read this file as claiming
//     the third carrier is governed to the same standard as the first two.
//
// Still uncollected, and named so the boundary is not mistaken for coverage:
// envelope reason codes, embedded asset and skill prose, and agent-facing
// terminal prompts (#2415, audited at 113 dead ends). Those need their own
// collectors and their own baseline; nothing here sees them.
//
// WHAT THIS RATCHET PROVES: every refusal-origin site in the production
// sources of internal/cli, internal/reviewtransaction, and internal/sddstatus
// either
//
//  (a) names a runnable continuation (a `gentle-ai ...` invocation) in the
//      message the operator actually reads -- which includes text composed in
//      from a package-local string-returning helper, so moving guidance behind
//      a helper neither loses coverage nor loses credit for it,
//  (b) carries an adjacent `// refusal:by-design <shape>: <reason>` marker
//      whose shape comes from the CLOSED vocabulary the bench classifier
//      already uses -- operator-knowledge, world-action, human-authority --
//      stating why no command can honestly exist there, or
//  (c) is frozen in the baseline (.refusal-ratchet-baseline.txt at the repo
//      root), which may only shrink. Regenerate after fixing entries with:
//
//        GENTLE_AI_REFUSAL_RATCHET_UPDATE=1 go test ./internal/cli \
//          -run TestEveryProductionRefusalNamesResolutionOrDeclaresByDesign -count=1
//
// Why a ratchet and not a clean gate: these packages already carry thousands
// of refusal-origin sites. Demanding zero at birth means the guard never
// exists; freezing today's violations and refusing growth is the part that
// pays for itself. Baseline entries are keyed by (file, message), never by
// line number, so unrelated edits above a site never churn the baseline --
// but editing a refusal's message re-opens the question, which is exactly the
// moment the message owes the operator a continuation.
//
// WHAT THIS DOES NOT PROVE, stated plainly so nobody trusts it further than
// it goes: it cannot catch a WRONG named continuation -- one that parses,
// runs, and does not help. Only outward-facing tests that dispatch the named
// command and require the block to clear prove that, case by case. The
// structural half of naming is proven by TestEveryNamedReviewContinuation-
// IsStructurallyReal: that guard catches named-but-nonexistent, this one
// catches existent-but-unnamed. One direction each; together they still only
// bound the problem, they do not close it.
//
// A BASELINE ENTRY THAT MATCHES NOTHING IS A FAILURE, not a note. The
// baseline is keyed by (file, message), so an entry can stop matching for two
// very different reasons: the refusal was fixed or removed (shrink the
// baseline, the ratchet's whole point), or it moved into a shape the analyzer
// cannot see (coverage was lost and nobody decided to lose it). Those are
// separated: an entry whose (file, message) still names an ANALYZED site that
// is no longer a violation is logged as a tighten-the-baseline note, while an
// entry matching no analyzed site at all fails, because from here the two
// causes are indistinguishable and only a human can tell them apart.
//
// WHY A MARKER PLUS A HELPER-SUPPLIED COMMAND IS NOT CONTRADICTORY: the
// contradictory-claims rule is judged on the sentence authored AT the site.
// A by-design marker claims that no command resolves THIS condition; a shared
// helper that appends a generic way out of the practice (`review mode
// disable`) is a different author's claim about a different thing, and the
// two are not mutually exclusive. Judging the rule on the composed text
// instead would make the only compliant shape "hide the guidance behind a
// helper", which is exactly the unanalyzable shape #3471 is about. The
// tradeoff is stated plainly: an author who extracts a one-line helper purely
// to dodge the contradiction check will not be caught by it. That is a
// deliberate, visible act, and unlike before the extracted text is still
// analyzed and still governed.
//
// Known false negatives, by construction: propagation sites (fmt.Errorf with
// %w) are exempt as plumbing, so a wrap that is itself the operator's
// terminal framing of a foreign error (os, git) escapes; refusals printed
// directly to a stream are not seen; every message whose value is built at
// runtime is counted and logged, never analyzed -- which includes the
// `Error() string` methods that return `fmt.Sprintf(...)` (measured at 57
// across the three packages when #3471 landed). Governing those means
// deciding whether a Sprintf inside Error() is terminal framing or the
// propagation analogue of a %w wrap, which is a judgment this walk cannot
// make and is deliberately not smuggled in here. Guidance reached through a
// METHOD call rather than a package-local function is likewise not composed
// in. Known false positives: an origin site
// that is genuinely internal plumbing still counts and must be annotated --
// the honest shape for a programmer invariant is world-action ("the exit is
// an action, not a command: edit the code"), which the bench vocabulary names
// verbatim.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// refusalRatchetByDesignShapes is the closed by-design vocabulary. It MUST
// stay identical to the bench classifier's (bench/classify.go), so there is
// one taxonomy, not two; TestRefusalRatchetVocabularyMatchesBenchClassifier
// enforces that mechanically.
var refusalRatchetByDesignShapes = map[string]bool{
	"operator-knowledge": true, // the product cannot know a value only the operator has
	"world-action":       true, // the exit is an action in the world, not a command
	"human-authority":    true, // the block is a human decision by design
}

// refusalRatchetMarkerHint is the substring that identifies a marker ATTEMPT.
// Any comment containing it must parse as a valid marker attached to an
// in-scope site; a malformed or orphaned attempt is an error, never silence.
const refusalRatchetMarkerHint = "refusal:by-design"

// refusalRatchetMarkerRegexp parses the full marker grammar:
//
//	// refusal:by-design <shape>: <reason>
var refusalRatchetMarkerRegexp = regexp.MustCompile(`^refusal:by-design\s+([a-z-]+):\s*(\S.*)$`)

// refusalRatchetNamedContinuationRegexp matches an explicit runnable
// continuation. Requiring a lowercase letter directly after "gentle-ai "
// excludes prose that mentions the product name without naming a command.
var refusalRatchetNamedContinuationRegexp = regexp.MustCompile(`gentle-ai [a-z][a-z-]*`)

// refusalRatchetErrorMethodOrigin labels a refusal returned from an
// `Error() string` method. It sits in the same slot as the constructor name so
// every report names the shape that produced the sentence.
const refusalRatchetErrorMethodOrigin = "Error() string"

type refusalRatchetSite struct {
	file        string // slash path relative to the repository root
	line        int
	constructor string // "errors.New", "fmt.Errorf", or refusalRatchetErrorMethodOrigin
	message     string
}

// baselineKey is the site's identity in the frozen baseline: file and message,
// never line, so unrelated edits do not churn and message edits re-open the
// question.
func (s refusalRatchetSite) baselineKey() string {
	return s.file + "\t" + strconv.Quote(s.message)
}

type refusalRatchetAnalysis struct {
	violations         []refusalRatchetSite
	satisfiedNamed     int
	satisfiedAnnotated int
	exemptWraps        int
	unanalyzable       []refusalRatchetSite
	// analyzed holds the baseline key of every site whose message is
	// statically known, whatever its classification. It is what makes "this
	// baselined refusal was fixed" distinguishable from "this baselined
	// refusal matches nothing the analyzer can see any more".
	analyzed map[string]bool
	// fieldViolations are refusal-explanation struct fields (#2416) whose
	// literal names no exit. They are reported and capped, not frozen site by
	// site -- see refusalRatchetFieldCarriedCeiling.
	fieldViolations []refusalRatchetSite
	fieldSatisfied  int
	// fieldCarried holds the baseline key of every field-carried refusal, so a
	// refusal that MOVED from an error constructor into a struct field is
	// reported as a move rather than as a disappearance.
	fieldCarried map[string]bool
	// problems are hard failures -- malformed markers, unknown shapes,
	// contradictory claims, orphaned markers. They are never baseline-able,
	// because a marker that cannot be trusted poisons the exemption it grants.
	problems []string
}

func (a *refusalRatchetAnalysis) record(key string) {
	if a.analyzed == nil {
		a.analyzed = map[string]bool{}
	}
	a.analyzed[key] = true
}

func (a *refusalRatchetAnalysis) merge(other refusalRatchetAnalysis) {
	a.violations = append(a.violations, other.violations...)
	a.satisfiedNamed += other.satisfiedNamed
	a.satisfiedAnnotated += other.satisfiedAnnotated
	a.exemptWraps += other.exemptWraps
	a.unanalyzable = append(a.unanalyzable, other.unanalyzable...)
	a.problems = append(a.problems, other.problems...)
	a.fieldViolations = append(a.fieldViolations, other.fieldViolations...)
	a.fieldSatisfied += other.fieldSatisfied
	for key := range other.analyzed {
		a.record(key)
	}
	for key := range other.fieldCarried {
		if a.fieldCarried == nil {
			a.fieldCarried = map[string]bool{}
		}
		a.fieldCarried[key] = true
	}
}

// refusalRatchetBaselineDrift separates the two reasons a frozen entry can
// stop matching a violation. Conflating them is the #3471 defect: the note
// "N baselined entries now name a resolution, are annotated, or are gone" read
// as progress while one of those entries was a refusal that had quietly left
// coverage.
type refusalRatchetBaselineDrift struct {
	// absent entries match no analyzed site at all: fixed, deleted, reworded
	// -- or moved into a shape the analyzer cannot see.
	absent []string
	// resolved entries still name an analyzed site that is no longer a
	// violation, so the baseline can simply be tightened.
	resolved []string
}

func refusalRatchetClassifyBaselineDrift(baseline []string, analyzed map[string]bool, current map[string]refusalRatchetSite) refusalRatchetBaselineDrift {
	var drift refusalRatchetBaselineDrift
	for _, key := range baseline {
		if key == "" {
			continue
		}
		if _, still := current[key]; still {
			continue
		}
		if analyzed[key] {
			drift.resolved = append(drift.resolved, key)
			continue
		}
		drift.absent = append(drift.absent, key)
	}
	sort.Strings(drift.absent)
	sort.Strings(drift.resolved)
	return drift
}

// --- Tests ---------------------------------------------------------------

// TestRefusalRatchetVocabularyMatchesBenchClassifier proves the marker
// vocabulary and the bench classifier's by-design vocabulary are the same
// set, extracted mechanically from bench/classify.go, so the taxonomy cannot
// fork. bench is its own module (package main), so importing it is not an
// option; the extraction fails closed when it stops finding anything.
func TestRefusalRatchetVocabularyMatchesBenchClassifier(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "..", "bench", "classify.go"))
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "classify.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	benchShapes := map[string]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		decl, ok := node.(*ast.GenDecl)
		if !ok || decl.Tok != token.CONST {
			return true
		}
		for _, spec := range decl.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Names) != 1 || len(value.Values) != 1 {
				continue
			}
			if !strings.HasPrefix(value.Names[0].Name, "ByDesign") {
				continue
			}
			literal, ok := value.Values[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				continue
			}
			shape, unquoteErr := strconv.Unquote(literal.Value)
			if unquoteErr != nil {
				continue
			}
			benchShapes[shape] = true
		}
		return false
	})
	if len(benchShapes) == 0 {
		t.Fatal("extracted no ByDesign* string constants from bench/classify.go; the extraction is stale")
	}
	for shape := range benchShapes {
		if !refusalRatchetByDesignShapes[shape] {
			t.Errorf("bench vocabulary has shape %q; the ratchet vocabulary does not -- the taxonomy has forked", shape)
		}
	}
	for shape := range refusalRatchetByDesignShapes {
		if !benchShapes[shape] {
			t.Errorf("ratchet vocabulary has shape %q; bench/classify.go does not -- the taxonomy has forked", shape)
		}
	}
}

// TestRefusalRatchetClassifiesSyntheticSites is the unit contract of the
// analyzer: every rule demonstrated on synthetic source, including the two
// failure modes that must fail LOUDLY (unknown shape, contradictory claims).
func TestRefusalRatchetClassifiesSyntheticSites(t *testing.T) {
	const header = "package synthetic\n\nimport (\n\t\"errors\"\n\t\"fmt\"\n)\n\nfunc use(err error) error { return err }\n\n"

	t.Run("bare refusal is a violation naming its exact site", func(t *testing.T) {
		analysis := refusalRatchetMustAnalyze(t, header+
			"func f() error {\n\treturn errors.New(\"stuck with no exit named\")\n}\n")
		if len(analysis.problems) != 0 {
			t.Fatalf("unexpected problems: %v", analysis.problems)
		}
		if len(analysis.violations) != 1 {
			t.Fatalf("want 1 violation, got %d", len(analysis.violations))
		}
		got := analysis.violations[0]
		if got.message != "stuck with no exit named" || got.line == 0 || got.file == "" {
			t.Fatalf("violation does not name its site: %+v", got)
		}
	})

	t.Run("naming a gentle-ai continuation satisfies", func(t *testing.T) {
		analysis := refusalRatchetMustAnalyze(t, header+
			"func f(l string) error {\n\treturn fmt.Errorf(\"blocked: run `gentle-ai review reopen-results --lineage %s`\", l)\n}\n")
		if len(analysis.violations) != 0 || analysis.satisfiedNamed != 1 {
			t.Fatalf("want 1 named satisfaction and no violations, got %+v", analysis)
		}
	})

	t.Run("valid by-design marker satisfies, above or trailing", func(t *testing.T) {
		analysis := refusalRatchetMustAnalyze(t, header+
			"func f() error {\n"+
			"\t// refusal:by-design operator-knowledge: only the operator knows where the checkout is\n"+
			"\treturn errors.New(\"no checkout path given\")\n"+
			"}\n"+
			"func g() error {\n"+
			"\treturn errors.New(\"edit the conflicted file first\") // refusal:by-design world-action: the exit is an edit, not a command\n"+
			"}\n")
		if len(analysis.problems) != 0 {
			t.Fatalf("unexpected problems: %v", analysis.problems)
		}
		if len(analysis.violations) != 0 || analysis.satisfiedAnnotated != 2 {
			t.Fatalf("want 2 annotated satisfactions and no violations, got %+v", analysis)
		}
	})

	t.Run("unknown shape is a vocabulary error, never a pass", func(t *testing.T) {
		analysis := refusalRatchetMustAnalyze(t, header+
			"func f() error {\n"+
			"\t// refusal:by-design because-reasons: trust me\n"+
			"\treturn errors.New(\"stuck\")\n"+
			"}\n")
		if len(analysis.problems) != 1 || !strings.Contains(analysis.problems[0], "because-reasons") || !strings.Contains(analysis.problems[0], "operator-knowledge") {
			t.Fatalf("want one vocabulary error naming the bogus shape and the closed vocabulary, got %+v", analysis.problems)
		}
		if analysis.satisfiedAnnotated != 0 {
			t.Fatal("a bogus shape must never satisfy the ratchet")
		}
	})

	t.Run("marker plus named continuation is contradictory", func(t *testing.T) {
		analysis := refusalRatchetMustAnalyze(t, header+
			"func f() error {\n"+
			"\t// refusal:by-design human-authority: a maintainer must decide\n"+
			"\treturn errors.New(\"blocked: run gentle-ai review status --next-transition\")\n"+
			"}\n")
		if len(analysis.problems) != 1 || !strings.Contains(analysis.problems[0], "contradictory") {
			t.Fatalf("want one contradictory-claims error, got %+v", analysis.problems)
		}
	})

	t.Run("malformed marker attempt is an error, never silence", func(t *testing.T) {
		analysis := refusalRatchetMustAnalyze(t, header+
			"func f() error {\n"+
			"\t// refusal:by-design somebody said this is fine\n"+
			"\treturn errors.New(\"stuck\")\n"+
			"}\n")
		if len(analysis.problems) != 1 || !strings.Contains(analysis.problems[0], "malformed") {
			t.Fatalf("want one malformed-marker error, got %+v", analysis.problems)
		}
	})

	t.Run("orphaned marker attempt is an error", func(t *testing.T) {
		analysis := refusalRatchetMustAnalyze(t, header+
			"// refusal:by-design world-action: attached to nothing\n\n"+
			"func f() error {\n\treturn nil\n}\n")
		if len(analysis.problems) != 1 || !strings.Contains(analysis.problems[0], "no refusal-origin site") {
			t.Fatalf("want one orphaned-marker error, got %+v", analysis.problems)
		}
	})

	t.Run("wrap sites are exempt propagation, not origins", func(t *testing.T) {
		analysis := refusalRatchetMustAnalyze(t, header+
			"func f(p string, err error) error {\n\treturn fmt.Errorf(\"open %s: %w\", p, err)\n}\n")
		if len(analysis.violations) != 0 || analysis.exemptWraps != 1 {
			t.Fatalf("want 1 exempt wrap and no violations, got %+v", analysis)
		}
	})

	t.Run("marker on a wrap site is an error: it exempts nothing", func(t *testing.T) {
		analysis := refusalRatchetMustAnalyze(t, header+
			"func f(err error) error {\n"+
			"\t// refusal:by-design world-action: this site is plumbing\n"+
			"\treturn fmt.Errorf(\"load: %w\", err)\n"+
			"}\n")
		if len(analysis.problems) != 1 || !strings.Contains(analysis.problems[0], "propagation") {
			t.Fatalf("want one marker-on-propagation error, got %+v", analysis.problems)
		}
	})

	t.Run("dynamic message is counted unanalyzable, not judged", func(t *testing.T) {
		analysis := refusalRatchetMustAnalyze(t, header+
			"func f(msg string) error {\n\treturn errors.New(msg)\n}\n")
		if len(analysis.violations) != 0 || len(analysis.unanalyzable) != 1 {
			t.Fatalf("want 1 unanalyzable site and no violations, got %+v", analysis)
		}
	})

	t.Run("concatenated literals are analyzed as one message", func(t *testing.T) {
		analysis := refusalRatchetMustAnalyze(t, header+
			"func f() error {\n\treturn errors.New(\"blocked: run \" + \"gentle-ai review status\")\n}\n")
		if analysis.satisfiedNamed != 1 || len(analysis.violations) != 0 {
			t.Fatalf("want the concatenated name to satisfy, got %+v", analysis)
		}
	})

	t.Run("sentinel var declarations are origins too", func(t *testing.T) {
		analysis := refusalRatchetMustAnalyze(t, header+
			"var errStuck = errors.New(\"sentinel with no exit named\")\n")
		if len(analysis.violations) != 1 {
			t.Fatalf("want the sentinel to be a violation, got %+v", analysis)
		}
	})
}

// TestRefusalRatchetAnalyzesRefusalsReturnedAsStrings is #3471's first half:
// the exact shape `GateRemoteFetchRequiredError.Error()` used to leave
// coverage with. A refusal is governed by what it says, not by which construct
// produced it.
func TestRefusalRatchetAnalyzesRefusalsReturnedAsStrings(t *testing.T) {
	const header = "package synthetic\n\nimport \"fmt\"\n\nvar _ = fmt.Sprintf\n\ntype fetchRequired struct{ remote string }\n\n"

	t.Run("a returned refusal with no exit is a violation naming its site", func(t *testing.T) {
		analysis := refusalRatchetMustAnalyze(t, header+
			"func (err *fetchRequired) Error() string {\n"+
			"\treturn \"advertised base commit is not available locally; fetch before validation\"\n"+
			"}\n")
		if len(analysis.problems) != 0 {
			t.Fatalf("unexpected problems: %v", analysis.problems)
		}
		if len(analysis.violations) != 1 {
			t.Fatalf("want the returned refusal to be a violation, got %+v", analysis)
		}
		got := analysis.violations[0]
		if got.message != "advertised base commit is not available locally; fetch before validation" {
			t.Fatalf("violation message = %q", got.message)
		}
		if got.constructor != refusalRatchetErrorMethodOrigin || got.line == 0 {
			t.Fatalf("violation does not name the shape that produced it: %+v", got)
		}
	})

	t.Run("a returned refusal that names its continuation satisfies", func(t *testing.T) {
		analysis := refusalRatchetMustAnalyze(t, header+
			"func (err *fetchRequired) Error() string {\n"+
			"\treturn \"blocked: run `gentle-ai review status` first\"\n"+
			"}\n")
		if len(analysis.violations) != 0 || analysis.satisfiedNamed != 1 {
			t.Fatalf("want 1 named satisfaction and no violations, got %+v", analysis)
		}
	})

	t.Run("a returned refusal accepts the by-design marker", func(t *testing.T) {
		analysis := refusalRatchetMustAnalyze(t, header+
			"func (err *fetchRequired) Error() string {\n"+
			"\t// refusal:by-design world-action: the exit is a fetch, not a command this product owns\n"+
			"\treturn \"advertised base commit is not available locally\"\n"+
			"}\n")
		if len(analysis.problems) != 0 {
			t.Fatalf("unexpected problems: %v", analysis.problems)
		}
		if len(analysis.violations) != 0 || analysis.satisfiedAnnotated != 1 {
			t.Fatalf("want 1 annotated satisfaction and no violations, got %+v", analysis)
		}
	})

	t.Run("a runtime-built returned message is unanalyzable, not judged", func(t *testing.T) {
		analysis := refusalRatchetMustAnalyze(t, header+
			"func (err *fetchRequired) Error() string {\n"+
			"\treturn fmt.Sprintf(\"fetch %s first\", err.remote)\n"+
			"}\n")
		if len(analysis.violations) != 0 || len(analysis.unanalyzable) != 1 {
			t.Fatalf("want 1 unanalyzable site and no violations, got %+v", analysis)
		}
	})

	t.Run("an ordinary string-returning helper is not a refusal", func(t *testing.T) {
		// The counter-proof for the rule: breadth here would flag every
		// returned string in the repository. Only the error interface's own
		// method is a refusal carrier.
		analysis := refusalRatchetMustAnalyze(t, header+
			"func lensLabel() string {\n\treturn \"Review CAPTURE-RESULT\"\n}\n")
		if len(analysis.violations) != 0 || len(analysis.unanalyzable) != 0 {
			t.Fatalf("a plain label must not be classified as a refusal, got %+v", analysis)
		}
	})
}

// TestRefusalRatchetAnalyzesGuidanceComposedFromHelpers is #3471's third half:
// the contradictory-claims rule must not make the compliant shape an
// unanalyzable one. Both directions are proven -- the helper's text counts,
// and the marker plus that helper is no longer a hard error -- because
// either alone would leave the trap open.
func TestRefusalRatchetAnalyzesGuidanceComposedFromHelpers(t *testing.T) {
	const header = "package synthetic\n\nimport (\n\t\"errors\"\n\t\"fmt\"\n)\n\nvar _ = errors.New\n\n" +
		"func exitGuidance() string {\n\treturn \"; exit receipt-driven review with `gentle-ai review mode disable --scope clone`\"\n}\n\n"

	t.Run("guidance behind a helper still names the continuation", func(t *testing.T) {
		analysis := refusalRatchetMustAnalyze(t, header+
			"func f() error {\n"+
			"\treturn fmt.Errorf(\"the active runtime is not eligible for immutable receipt review%s\", exitGuidance())\n"+
			"}\n")
		if len(analysis.problems) != 0 {
			t.Fatalf("unexpected problems: %v", analysis.problems)
		}
		if len(analysis.violations) != 0 || analysis.satisfiedNamed != 1 {
			t.Fatalf("want the helper-supplied command to satisfy, got %+v", analysis)
		}
	})

	t.Run("a named string constant is composed in too", func(t *testing.T) {
		analysis := refusalRatchetMustAnalyze(t, header+
			"const freshReview = \"start a new one with `gentle-ai review start`\"\n\n"+
			"func f() error {\n\treturn fmt.Errorf(\"review scope changed: %s\", freshReview)\n}\n")
		if len(analysis.violations) != 0 || analysis.satisfiedNamed != 1 {
			t.Fatalf("want the constant-supplied command to satisfy, got %+v", analysis)
		}
	})

	t.Run("a by-design marker plus helper-supplied guidance is not contradictory", func(t *testing.T) {
		analysis := refusalRatchetMustAnalyze(t, header+
			"func f() error {\n"+
			"\t// refusal:by-design world-action: runtimes outside the fixed policy cannot receive immutable review authority\n"+
			"\treturn fmt.Errorf(\"the active runtime is not eligible for immutable receipt review%s\", exitGuidance())\n"+
			"}\n")
		if len(analysis.problems) != 0 {
			t.Fatalf("a shared exit appended by a helper is not the site's own claim: %v", analysis.problems)
		}
		if len(analysis.violations) != 0 || analysis.satisfiedAnnotated != 1 {
			t.Fatalf("want 1 annotated satisfaction and no violations, got %+v", analysis)
		}
	})

	t.Run("a by-design marker plus a command the author wrote is still contradictory", func(t *testing.T) {
		// The rule narrowed, it did not disappear.
		analysis := refusalRatchetMustAnalyze(t, header+
			"func f() error {\n"+
			"\t// refusal:by-design human-authority: a maintainer must decide\n"+
			"\treturn errors.New(\"blocked: run gentle-ai review status --next-transition\")\n"+
			"}\n")
		if len(analysis.problems) != 1 || !strings.Contains(analysis.problems[0], "contradictory") {
			t.Fatalf("want one contradictory-claims error, got %+v", analysis.problems)
		}
	})
}

// TestRefusalRatchetReportsFieldCarriedRefusals is #2416: a refusal handed to a
// consumer through a reason field is the same dead end as a bare error string,
// and the walk must see it without inventing refusals out of every literal.
func TestRefusalRatchetReportsFieldCarriedRefusals(t *testing.T) {
	const header = "package synthetic\n\ntype bridge struct {\n\tRelevant bool\n\tReason   string\n\tLabel    string\n}\n\n"

	t.Run("a reason field with no exit is collected", func(t *testing.T) {
		analysis := refusalRatchetMustAnalyze(t, header+
			"func f() bridge {\n\treturn bridge{Relevant: true, Reason: \"no eligible path-bound compact authority found\"}\n}\n")
		if len(analysis.fieldViolations) != 1 {
			t.Fatalf("want the reason field collected, got %+v", analysis.fieldViolations)
		}
		if analysis.fieldViolations[0].message != "no eligible path-bound compact authority found" {
			t.Fatalf("field site does not carry its sentence: %+v", analysis.fieldViolations[0])
		}
	})

	t.Run("a reason field that names its exit is satisfied, constant included", func(t *testing.T) {
		analysis := refusalRatchetMustAnalyze(t, header+
			"const freshReview = \"run `gentle-ai review start`\"\n\n"+
			"func f() bridge {\n\treturn bridge{Reason: \"scope changed; \" + freshReview}\n}\n")
		if len(analysis.fieldViolations) != 0 || analysis.fieldSatisfied != 1 {
			t.Fatalf("want 1 satisfied field and no violations, got %+v", analysis)
		}
	})

	t.Run("fields outside the closed carrier set are never refusals", func(t *testing.T) {
		analysis := refusalRatchetMustAnalyze(t, header+
			"func f() bridge {\n\treturn bridge{Label: \"Review CAPTURE-RESULT\"}\n}\n")
		if len(analysis.fieldViolations) != 0 || analysis.fieldSatisfied != 0 {
			t.Fatalf("a label field must not be classified as a refusal, got %+v", analysis)
		}
	})

	t.Run("a reason field is never mistaken for an enforced baseline site", func(t *testing.T) {
		// Field-carried sites are capped, not frozen. Letting them into the
		// baseline key space would silently retire an error-constructor entry.
		analysis := refusalRatchetMustAnalyze(t, header+
			"func f() bridge {\n\treturn bridge{Reason: \"no eligible path-bound compact authority found\"}\n}\n")
		if len(analysis.violations) != 0 || len(analysis.analyzed) != 0 {
			t.Fatalf("field carriers must stay out of the enforced set, got %+v", analysis)
		}
	})
}

// TestRefusalRatchetReportsStaleBaselineEntries is #3471's second half. Before
// this, all three cases below collapsed into one count -- "N baselined entries
// now name a resolution, are annotated, or are gone" -- and the one that meant
// LOST COVERAGE read exactly like the two that meant progress.
func TestRefusalRatchetReportsStaleBaselineEntries(t *testing.T) {
	const fixed = "internal/reviewtransaction/gate.go\t\"a refusal that now names its exit\""
	const gone = "internal/reviewtransaction/gate.go\t\"advertised base commit is not available locally; fetch before validation\""
	const live = "internal/reviewtransaction/gate.go\t\"a refusal still frozen\""

	analyzed := map[string]bool{fixed: true, live: true}
	current := map[string]refusalRatchetSite{live: {file: "internal/reviewtransaction/gate.go", message: "a refusal still frozen"}}

	drift := refusalRatchetClassifyBaselineDrift([]string{fixed, gone, live, ""}, analyzed, current)

	if len(drift.absent) != 1 || drift.absent[0] != gone {
		t.Fatalf("an entry matching no analyzed site must be reported as absent, got %+v", drift.absent)
	}
	if len(drift.resolved) != 1 || drift.resolved[0] != fixed {
		t.Fatalf("an entry whose site is analyzed and fixed must be reported as resolved, got %+v", drift.resolved)
	}
	if len(drift.absent)+len(drift.resolved) != 1+1 {
		t.Fatalf("a still-violating entry must not drift at all: %+v", drift)
	}
}

// TestEveryProductionRefusalNamesResolutionOrDeclaresByDesign is the ratchet.
// It analyzes every production source file of the three audited packages and
// fails on any refusal-origin site that neither names a runnable continuation
// nor carries a valid by-design marker nor is frozen in the baseline. The
// baseline may only shrink.
func TestEveryProductionRefusalNamesResolutionOrDeclaresByDesign(t *testing.T) {
	analysis := refusalRatchetAnalyzeProductionSources(t)

	total := len(analysis.violations) + analysis.satisfiedNamed + analysis.satisfiedAnnotated + analysis.exemptWraps + len(analysis.unanalyzable)
	// The survey that motivated this guard counted ~2,500 constructor sites
	// across these packages. A walk that suddenly sees a fraction of that is
	// broken, not clean.
	if total < 1000 {
		t.Fatalf("enumerated only %d error-constructor sites; the survey found ~2,500, so the walk is broken", total)
	}
	t.Logf("sites: %d total = %d violations + %d named + %d annotated + %d exempt wraps + %d unanalyzable",
		total, len(analysis.violations), analysis.satisfiedNamed, analysis.satisfiedAnnotated, analysis.exemptWraps, len(analysis.unanalyzable))
	for _, site := range analysis.unanalyzable {
		t.Logf("unanalyzable (runtime-built message, out of scope by construction): %s:%d %s", site.file, site.line, site.constructor)
	}
	defer refusalRatchetReportFieldCarriedRefusals(t, analysis)
	for _, problem := range analysis.problems {
		t.Error(problem)
	}
	if t.Failed() {
		return
	}

	current := map[string]refusalRatchetSite{}
	for _, site := range analysis.violations {
		current[site.baselineKey()] = site
	}
	baselinePath := filepath.Join("..", "..", ".refusal-ratchet-baseline.txt")

	if os.Getenv("GENTLE_AI_REFUSAL_RATCHET_UPDATE") == "1" {
		keys := make([]string, 0, len(current))
		for key := range current {
			keys = append(keys, key)
		}
		// sort.Strings is byte-wise -- C collation by construction, on every
		// platform. The deadcode ratchet's collation bug (baseline sorted
		// under en_US.UTF-8, compared under C, every entry reported as new)
		// cannot recur here because no locale-dependent tool ever touches
		// this file.
		sort.Strings(keys)
		if err := os.WriteFile(baselinePath, []byte(strings.Join(keys, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("baseline updated: %d entries", len(keys))
		return
	}

	raw, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("missing baseline %s -- run: GENTLE_AI_REFUSAL_RATCHET_UPDATE=1 go test ./internal/cli -run TestEveryProductionRefusalNamesResolutionOrDeclaresByDesign -count=1 (%v)", baselinePath, err)
	}
	baselineEntries := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	baseline := map[string]bool{}
	for _, line := range baselineEntries {
		if line != "" {
			baseline[line] = true
		}
	}

	newKeys := make([]string, 0)
	for key := range current {
		if !baseline[key] {
			newKeys = append(newKeys, key)
		}
	}
	sort.Strings(newKeys)
	for _, key := range newKeys {
		site := current[key]
		t.Errorf("NEW refusal with no named resolution: %s:%d %s(%q)\n"+
			"  Either name the runnable continuation in the message (`gentle-ai ...`),\n"+
			"  or, if no command can honestly exist here, annotate the site:\n"+
			"    // refusal:by-design <operator-knowledge|world-action|human-authority>: <why>",
			site.file, site.line, site.constructor, site.message)
	}

	drift := refusalRatchetClassifyBaselineDrift(baselineEntries, analysis.analyzed, current)
	for _, key := range drift.absent {
		moved := ""
		if analysis.fieldCarried[key] {
			moved = "\n  This exact sentence is now carried by a refusal-explanation STRUCT FIELD in the\n" +
				"  same file. It did not get fixed; it changed carrier."
		}
		t.Errorf("STALE baseline entry matches no analyzed refusal site: %s%s\n"+
			"  A frozen entry that matches nothing has exactly two causes and they are opposites.\n"+
			"  If the refusal was removed, reworded, or now names its exit, shrink the baseline:\n"+
			"    GENTLE_AI_REFUSAL_RATCHET_UPDATE=1 go test ./internal/cli -run TestEveryProductionRefusalNamesResolutionOrDeclaresByDesign -count=1\n"+
			"  If instead it moved into a shape this analyzer cannot see -- a runtime-built\n"+
			"  message, a stream write, a carrier outside the covered set -- then it left\n"+
			"  coverage without anyone deciding to stop governing it (#3471). Put it back in an\n"+
			"  analyzable shape rather than deleting the entry.", key, moved)
	}
	for _, key := range drift.resolved {
		t.Logf("baselined refusal now names a resolution or is annotated: %s", key)
	}
	if len(drift.resolved) > 0 {
		t.Logf("      Tighten the baseline with: GENTLE_AI_REFUSAL_RATCHET_UPDATE=1 go test ./internal/cli -run TestEveryProductionRefusalNamesResolutionOrDeclaresByDesign -count=1")
	}
}

// refusalRatchetFieldCarriedCeiling freezes the field-carried refusal debt
// discovered when #2416's carrier was added to the walk. It is a CEILING, not
// a per-site baseline, and that is a deliberate, disclosed compromise:
//
//   - Freezing 70 unexamined sentences into .refusal-ratchet-baseline.txt
//     would launder unreviewed debt into the same artifact that holds reviewed
//     debt, and that file's only value is that every line in it was looked at.
//   - Leaving the carrier out entirely keeps the escape open, which is the
//     defect.
//   - The set is not clean. Measured on the day it landed, 2 of the 70 are
//     ALLOW explanations wearing a refusal-shaped field name -- sddstatus
//     review_gate.go's "approved receipt exactly matches authoritative native
//     state and the current repository" and status.go's "explicit bound
//     compact authority exactly matches the current repository". Demanding
//     that those name a `gentle-ai` command would be the checker inventing a
//     refusal. Field name alone is not the criterion, and nothing static can
//     separate the two here: both sit next to a `Result:` whose value is a
//     variable. Per-site freezing would carve that error into the baseline.
//
// So the class is walked, every offender is listed on failure, and the count
// may only go down. What this does NOT do is stop one field refusal being
// swapped for another at the same count. Burning this down site by site is the
// second pass, and until it happens this number is the honest statement of how
// much of the carrier is governed: none of it individually, all of it in bulk.
const refusalRatchetFieldCarriedCeiling = 70

func refusalRatchetReportFieldCarriedRefusals(t *testing.T, analysis refusalRatchetAnalysis) {
	t.Helper()
	sort.Slice(analysis.fieldViolations, func(i, j int) bool {
		if analysis.fieldViolations[i].file != analysis.fieldViolations[j].file {
			return analysis.fieldViolations[i].file < analysis.fieldViolations[j].file
		}
		return analysis.fieldViolations[i].line < analysis.fieldViolations[j].line
	})
	t.Logf("field-carried refusals (#2416): %d name no exit, %d satisfied; ceiling %d",
		len(analysis.fieldViolations), analysis.fieldSatisfied, refusalRatchetFieldCarriedCeiling)
	for _, site := range analysis.fieldViolations {
		t.Logf("field-carried refusal names no exit: %s:%d %s = %q", site.file, site.line, site.constructor, site.message)
	}
	if len(analysis.fieldViolations) > refusalRatchetFieldCarriedCeiling {
		t.Errorf("field-carried refusals with no named exit grew from %d to %d.\n"+
			"  Every offender is listed above. Name the runnable continuation in the new one\n"+
			"  (`gentle-ai ...`), or annotate it:\n"+
			"    // refusal:by-design <operator-knowledge|world-action|human-authority>: <why>\n"+
			"  Raising refusalRatchetFieldCarriedCeiling is not one of the exits.",
			refusalRatchetFieldCarriedCeiling, len(analysis.fieldViolations))
	}
}

// --- Analyzer ------------------------------------------------------------

// refusalRatchetProductionDirs maps each audited package directory (relative
// to this package) to its repository-root-relative slash prefix. These three
// packages are where every escape in the detection-gap audit lived; other
// packages are excluded until they earn an entry the same way.
var refusalRatchetProductionDirs = []struct{ dir, prefix string }{
	{".", "internal/cli"},
	{filepath.Join("..", "reviewtransaction"), "internal/reviewtransaction"},
	{filepath.Join("..", "sddstatus"), "internal/sddstatus"},
}

// refusalRatchetFile is one production source, labelled by its
// repository-root-relative slash path.
type refusalRatchetFile struct {
	label  string
	source string
}

func refusalRatchetAnalyzeProductionSources(t *testing.T) refusalRatchetAnalysis {
	t.Helper()
	var merged refusalRatchetAnalysis
	for _, target := range refusalRatchetProductionDirs {
		entries, err := os.ReadDir(target.dir)
		if err != nil {
			t.Fatal(err)
		}
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			names = append(names, name)
		}
		sort.Strings(names)
		files := make([]refusalRatchetFile, 0, len(names))
		for _, name := range names {
			source, err := os.ReadFile(filepath.Join(target.dir, name))
			if err != nil {
				t.Fatal(err)
			}
			files = append(files, refusalRatchetFile{label: path.Join(target.prefix, name), source: string(source)})
		}
		// One package at a time, because guidance composed in from a helper
		// is only resolvable within the package that declares it.
		analysis, err := refusalRatchetAnalyzePackage(files)
		if err != nil {
			t.Fatalf("parse %s: %v", target.prefix, err)
		}
		merged.merge(analysis)
	}
	return merged
}

func refusalRatchetMustAnalyze(t *testing.T, source string) refusalRatchetAnalysis {
	t.Helper()
	analysis, err := refusalRatchetAnalyzeSource("synthetic/synthetic.go", source)
	if err != nil {
		t.Fatalf("parse synthetic source: %v", err)
	}
	return analysis
}

type refusalRatchetMarker struct {
	line   int // line the marker occupies (its comment's end line)
	shape  string
	reason string
	// malformed carries the problem text when the attempt does not parse.
	malformed string
	consumed  bool
}

// refusalRatchetAnalyzeSource classifies a single source file as a
// one-file package.
func refusalRatchetAnalyzeSource(fileLabel, source string) (refusalRatchetAnalysis, error) {
	return refusalRatchetAnalyzePackage([]refusalRatchetFile{{label: fileLabel, source: source}})
}

// refusalRatchetAnalyzePackage parses every production source of one package
// and classifies its refusal-origin sites. The package, not the file, is the
// unit because the text an operator reads can be composed from a helper
// declared in a sibling file, and a message half-resolved is a message
// misjudged.
func refusalRatchetAnalyzePackage(files []refusalRatchetFile) (refusalRatchetAnalysis, error) {
	var analysis refusalRatchetAnalysis
	fset := token.NewFileSet()
	parsed := make([]*ast.File, 0, len(files))
	for _, file := range files {
		syntax, err := parser.ParseFile(fset, file.label, file.source, parser.ParseComments)
		if err != nil {
			return analysis, err
		}
		parsed = append(parsed, syntax)
	}
	resolver := refusalRatchetNewTextResolver(parsed)
	for index, syntax := range parsed {
		analysis.merge(refusalRatchetAnalyzeFile(fset, syntax, files[index].label, resolver))
	}
	sort.Strings(analysis.problems)
	return analysis, nil
}

func refusalRatchetAnalyzeFile(fset *token.FileSet, file *ast.File, fileLabel string, resolver *refusalRatchetTextResolver) refusalRatchetAnalysis {
	var analysis refusalRatchetAnalysis

	markers := map[int]*refusalRatchetMarker{}
	for _, group := range file.Comments {
		for _, comment := range group.List {
			text := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(comment.Text, "//"), "/*"))
			text = strings.TrimSpace(strings.TrimSuffix(text, "*/"))
			if !strings.Contains(text, refusalRatchetMarkerHint) {
				continue
			}
			line := fset.Position(comment.End()).Line
			match := refusalRatchetMarkerRegexp.FindStringSubmatch(text)
			if match == nil {
				markers[line] = &refusalRatchetMarker{line: line, malformed: fmt.Sprintf(
					"%s:%d has a malformed refusal:by-design marker (%q); the grammar is `refusal:by-design <shape>: <reason>`",
					fileLabel, line, text)}
				continue
			}
			markers[line] = &refusalRatchetMarker{line: line, shape: match[1], reason: strings.TrimSpace(match[2])}
		}
	}

	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		constructor := refusalRatchetConstructorName(call)
		if constructor == "" || len(call.Args) == 0 {
			return true
		}
		line := fset.Position(call.Pos()).Line
		site := refusalRatchetSite{file: fileLabel, line: line, constructor: constructor}

		marker := refusalRatchetMarkerAt(markers, line)

		message, literal := refusalRatchetLiteralString(call.Args[0])
		if !literal {
			if marker != nil && marker.malformed == "" {
				// A marker cannot vouch for bytes nobody can read statically.
				analysis.problems = append(analysis.problems, fmt.Sprintf(
					"%s:%d carries a refusal:by-design marker on a runtime-built message; the marker exempts nothing it cannot see", fileLabel, line))
			}
			analysis.unanalyzable = append(analysis.unanalyzable, site)
			return true
		}
		site.message = message
		analysis.record(site.baselineKey())

		if constructor == "fmt.Errorf" && strings.Contains(message, "%w") {
			if marker != nil {
				problem := marker.malformed
				if problem == "" {
					problem = fmt.Sprintf("%s:%d carries a refusal:by-design marker on a propagation site (%%w wrap); wraps are exempt plumbing and the marker exempts nothing", fileLabel, line)
				}
				analysis.problems = append(analysis.problems, problem)
			}
			analysis.exemptWraps++
			return true
		}

		analysis.classify(&site, marker, message, refusalRatchetOperatorText(resolver, call.Args))
		return true
	})

	refusalRatchetInspectErrorMethods(fset, file, fileLabel, resolver, markers, &analysis)
	refusalRatchetInspectFieldCarriers(fset, file, fileLabel, resolver, markers, &analysis)

	for _, marker := range markers {
		if marker.consumed {
			continue
		}
		problem := marker.malformed
		if problem == "" {
			problem = fmt.Sprintf("%s:%d has a refusal:by-design marker adjacent to no refusal-origin site; a marker attached to nothing exempts nothing", fileLabel, marker.line)
		}
		analysis.problems = append(analysis.problems, problem)
	}
	sort.Strings(analysis.problems)
	return analysis
}

// classify applies the shared rules to one site whose message is statically
// known. authored is the sentence written AT the site; operator is everything
// the reader actually receives, helper-composed text included.
func (a *refusalRatchetAnalysis) classify(site *refusalRatchetSite, marker *refusalRatchetMarker, authored, operator string) {
	namedAtSite := refusalRatchetNamedContinuationRegexp.MatchString(authored)
	namedToOperator := namedAtSite || refusalRatchetNamedContinuationRegexp.MatchString(operator)
	switch {
	case marker != nil && marker.malformed != "":
		a.problems = append(a.problems, marker.malformed)
	case marker != nil && !refusalRatchetByDesignShapes[marker.shape]:
		a.problems = append(a.problems, fmt.Sprintf(
			"%s:%d declares by-design shape %q, which is not in the closed vocabulary (operator-knowledge, world-action, human-authority); an unrecognized shape is a corpus error, never a pass",
			site.file, site.line, marker.shape))
	case marker != nil && namedAtSite:
		// Judged on the AUTHORED sentence only. See the header: a shared
		// helper appending a generic way out of the practice is a different
		// claim, and making that combination illegal is what pushed guidance
		// into shapes nothing could analyze.
		a.problems = append(a.problems, fmt.Sprintf(
			"%s:%d is contradictory: the message names a `gentle-ai` continuation AND the site declares by-design %q; a refusal either has a runnable exit or it does not -- the two claims are mutually exclusive",
			site.file, site.line, marker.shape))
	case marker != nil:
		a.satisfiedAnnotated++
	case namedToOperator:
		a.satisfiedNamed++
	default:
		a.violations = append(a.violations, *site)
	}
}

// refusalRatchetInspectErrorMethods collects the second carrier: a statically
// known string RETURNED from an `Error() string` method. That value is the
// refusal an operator reads, exactly like the argument of errors.New, and
// #3471 is the record of what its absence cost.
func refusalRatchetInspectErrorMethods(fset *token.FileSet, file *ast.File, fileLabel string, resolver *refusalRatchetTextResolver, markers map[int]*refusalRatchetMarker, analysis *refusalRatchetAnalysis) {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Body == nil || fn.Name.Name != "Error" {
			continue
		}
		if fn.Type.Params != nil && len(fn.Type.Params.List) != 0 {
			continue
		}
		if fn.Type.Results == nil || len(fn.Type.Results.List) != 1 {
			continue
		}
		result, ok := fn.Type.Results.List[0].Type.(*ast.Ident)
		if !ok || result.Name != "string" {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			ret, ok := node.(*ast.ReturnStmt)
			if !ok || len(ret.Results) != 1 {
				return true
			}
			// An error constructed inside Error() is already the first
			// carrier's business; counting it twice would double-report it.
			if call, isCall := ret.Results[0].(*ast.CallExpr); isCall && refusalRatchetConstructorName(call) != "" {
				return true
			}
			line := fset.Position(ret.Pos()).Line
			site := refusalRatchetSite{file: fileLabel, line: line, constructor: refusalRatchetErrorMethodOrigin}
			marker := refusalRatchetMarkerAt(markers, line)
			message, literal := refusalRatchetLiteralString(ret.Results[0])
			if !literal {
				if marker != nil && marker.malformed == "" {
					analysis.problems = append(analysis.problems, fmt.Sprintf(
						"%s:%d carries a refusal:by-design marker on a runtime-built message; the marker exempts nothing it cannot see", fileLabel, line))
				}
				analysis.unanalyzable = append(analysis.unanalyzable, site)
				return true
			}
			site.message = message
			analysis.record(site.baselineKey())
			analysis.classify(&site, marker, message, resolver.text(ret.Results[0], 0, map[string]bool{}))
			return true
		})
	}
}

// refusalRatchetFieldCarriers is the closed allowlist of struct fields that
// carry a refusal explanation to a consumer (#2416). It is an allowlist and
// not a rule about field names, and the difference is load-bearing: lowercase
// `reason` is deliberately absent because in these packages it names a
// store-category description ("per-lineage preserved raw reviewer results"),
// which is not a refusal and must never be asked to name a command. Field
// NAME alone is not the criterion, which is also why this carrier is reported
// and capped rather than frozen site by site -- see the ceiling below.
var refusalRatchetFieldCarriers = map[string]bool{
	"Reason":  true,
	"Message": true,
	"message": true,
}

// refusalRatchetInspectFieldCarriers collects the third carrier: refusal text
// assigned to a refusal-explanation struct field. A consuming agent acts on
// that field exactly as it acts on an error string, so a bare literal there is
// the same dead end.
func refusalRatchetInspectFieldCarriers(fset *token.FileSet, file *ast.File, fileLabel string, resolver *refusalRatchetTextResolver, markers map[int]*refusalRatchetMarker, analysis *refusalRatchetAnalysis) {
	ast.Inspect(file, func(node ast.Node) bool {
		pair, ok := node.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		key, ok := pair.Key.(*ast.Ident)
		if !ok || !refusalRatchetFieldCarriers[key.Name] {
			return true
		}
		// Only text the author fixed AT the assignment: a literal, a
		// concatenation, or a named constant. A field whose value is a call is
		// deliberately skipped -- the union of a helper's several returns is
		// not a sentence any single consumer ever reads, and reporting it as
		// one would be the analyzer inventing a refusal.
		operator := refusalRatchetStaticFieldText(pair.Value, resolver)
		if operator == "" {
			return true
		}
		message, literal := refusalRatchetLiteralString(pair.Value)
		if !literal {
			message = operator
		}
		line := fset.Position(pair.Pos()).Line
		site := refusalRatchetSite{file: fileLabel, line: line, constructor: key.Name + ": string", message: message}
		if analysis.fieldCarried == nil {
			analysis.fieldCarried = map[string]bool{}
		}
		analysis.fieldCarried[site.baselineKey()] = true
		marker := refusalRatchetMarkerAt(markers, line)
		if marker != nil || refusalRatchetNamedContinuationRegexp.MatchString(operator) {
			analysis.fieldSatisfied++
			return true
		}
		analysis.fieldViolations = append(analysis.fieldViolations, site)
		return true
	})
}

// refusalRatchetStaticFieldText renders a field value only when the author
// spelled it out at the assignment: a literal, a concatenation of them, or a
// named constant. Call expressions resolve to nothing here on purpose.
func refusalRatchetStaticFieldText(expr ast.Expr, resolver *refusalRatchetTextResolver) string {
	switch typed := expr.(type) {
	case *ast.BasicLit, *ast.Ident:
		return resolver.text(typed, 0, map[string]bool{})
	case *ast.ParenExpr:
		return refusalRatchetStaticFieldText(typed.X, resolver)
	case *ast.BinaryExpr:
		if typed.Op != token.ADD {
			return ""
		}
		return refusalRatchetStaticFieldText(typed.X, resolver) + refusalRatchetStaticFieldText(typed.Y, resolver)
	}
	return ""
}

func refusalRatchetMarkerAt(markers map[int]*refusalRatchetMarker, line int) *refusalRatchetMarker {
	marker := markers[line]
	if marker == nil {
		marker = markers[line-1]
	}
	if marker != nil {
		marker.consumed = true
	}
	return marker
}

// --- Text resolution across the package ----------------------------------

// refusalRatchetResolveDepth bounds helper composition. Guidance is composed
// one or two helpers deep in practice; the bound exists so a cycle or a deep
// chain cannot hang the walk.
const refusalRatchetResolveDepth = 6

// refusalRatchetTextResolver renders the compile-time-visible text of an
// expression: string literals, concatenations, package-level string constants,
// and calls to package-local functions that return strings. Runtime holes
// render as nothing, so what remains is exactly the part of the sentence an
// author fixed at compile time -- which is the only part a static ratchet can
// honestly judge.
type refusalRatchetTextResolver struct {
	consts map[string]ast.Expr
	funcs  map[string]*ast.FuncDecl
}

func refusalRatchetNewTextResolver(files []*ast.File) *refusalRatchetTextResolver {
	resolver := &refusalRatchetTextResolver{consts: map[string]ast.Expr{}, funcs: map[string]*ast.FuncDecl{}}
	for _, file := range files {
		for _, decl := range file.Decls {
			switch typed := decl.(type) {
			case *ast.GenDecl:
				if typed.Tok != token.CONST && typed.Tok != token.VAR {
					continue
				}
				for _, spec := range typed.Specs {
					value, ok := spec.(*ast.ValueSpec)
					if !ok || len(value.Names) != 1 || len(value.Values) != 1 {
						continue
					}
					resolver.consts[value.Names[0].Name] = value.Values[0]
				}
			case *ast.FuncDecl:
				if typed.Recv != nil || typed.Body == nil {
					continue
				}
				if typed.Type.Results == nil || len(typed.Type.Results.List) != 1 {
					continue
				}
				ident, ok := typed.Type.Results.List[0].Type.(*ast.Ident)
				if !ok || ident.Name != "string" {
					continue
				}
				resolver.funcs[typed.Name.Name] = typed
			}
		}
	}
	return resolver
}

func (r *refusalRatchetTextResolver) text(expr ast.Expr, depth int, visiting map[string]bool) string {
	if expr == nil || depth > refusalRatchetResolveDepth {
		return ""
	}
	switch typed := expr.(type) {
	case *ast.BasicLit:
		if typed.Kind != token.STRING {
			return ""
		}
		value, err := strconv.Unquote(typed.Value)
		if err != nil {
			return ""
		}
		return value
	case *ast.ParenExpr:
		return r.text(typed.X, depth, visiting)
	case *ast.BinaryExpr:
		if typed.Op != token.ADD {
			return ""
		}
		// Concatenation is contiguous: `"run " + cmdConst` must be able to
		// spell a command across the seam.
		return r.text(typed.X, depth, visiting) + r.text(typed.Y, depth, visiting)
	case *ast.Ident:
		value, ok := r.consts[typed.Name]
		if !ok || visiting[typed.Name] {
			return ""
		}
		visiting[typed.Name] = true
		defer delete(visiting, typed.Name)
		return r.text(value, depth+1, visiting)
	case *ast.CallExpr:
		return r.callText(typed, depth, visiting)
	}
	return ""
}

func (r *refusalRatchetTextResolver) callText(call *ast.CallExpr, depth int, visiting map[string]bool) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		decl, ok := r.funcs[fun.Name]
		if !ok || visiting[fun.Name] {
			return ""
		}
		visiting[fun.Name] = true
		defer delete(visiting, fun.Name)
		return r.funcText(decl, depth+1, visiting)
	case *ast.SelectorExpr:
		// Only the two standard-library composers that assemble operator
		// prose. Descending into arbitrary calls would drag in text the
		// operator never reads.
		pkg, ok := fun.X.(*ast.Ident)
		if !ok || (pkg.Name != "fmt" && pkg.Name != "strings") {
			return ""
		}
		return r.joinArguments(call.Args, depth, visiting)
	}
	return ""
}

// joinArguments separates arguments with a newline so no `gentle-ai <verb>`
// can be spelled by accident across two unrelated values.
func (r *refusalRatchetTextResolver) joinArguments(args []ast.Expr, depth int, visiting map[string]bool) string {
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		if rendered := r.text(arg, depth+1, visiting); rendered != "" {
			parts = append(parts, rendered)
		}
	}
	return strings.Join(parts, "\n")
}

func (r *refusalRatchetTextResolver) funcText(decl *ast.FuncDecl, depth int, visiting map[string]bool) string {
	if decl == nil || decl.Body == nil {
		return ""
	}
	parts := []string{}
	ast.Inspect(decl.Body, func(node ast.Node) bool {
		ret, ok := node.(*ast.ReturnStmt)
		if !ok || len(ret.Results) != 1 {
			return true
		}
		if rendered := r.text(ret.Results[0], depth, visiting); rendered != "" {
			parts = append(parts, rendered)
		}
		return true
	})
	return strings.Join(parts, "\n")
}

// refusalRatchetOperatorText is the sentence the operator actually reads for
// one refusal-origin site: the author's own literal plus whatever the
// arguments compose into it.
func refusalRatchetOperatorText(resolver *refusalRatchetTextResolver, args []ast.Expr) string {
	return resolver.joinArguments(args, -1, map[string]bool{})
}

func refusalRatchetConstructorName(call *ast.CallExpr) string {
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
		return "errors.New"
	case pkg.Name == "fmt" && selector.Sel.Name == "Errorf":
		return "fmt.Errorf"
	}
	return ""
}

// refusalRatchetLiteralString resolves an argument to its compile-time string
// value: a string literal, or a concatenation of string literals. Anything
// else is a runtime value and unanalyzable.
func refusalRatchetLiteralString(expr ast.Expr) (string, bool) {
	switch typed := expr.(type) {
	case *ast.BasicLit:
		if typed.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(typed.Value)
		if err != nil {
			return "", false
		}
		return value, true
	case *ast.BinaryExpr:
		if typed.Op != token.ADD {
			return "", false
		}
		left, ok := refusalRatchetLiteralString(typed.X)
		if !ok {
			return "", false
		}
		right, ok := refusalRatchetLiteralString(typed.Y)
		if !ok {
			return "", false
		}
		return left + right, true
	case *ast.ParenExpr:
		return refusalRatchetLiteralString(typed.X)
	}
	return "", false
}
