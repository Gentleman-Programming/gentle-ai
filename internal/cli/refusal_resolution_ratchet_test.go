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
// WHAT THIS RATCHET PROVES: every refusal-origin site in the production
// sources of internal/cli, internal/reviewtransaction, and internal/sddstatus
// either
//
//  (a) names a runnable continuation (a `gentle-ai ...` invocation in the
//      message),
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
// Known false negatives, by construction: propagation sites (fmt.Errorf with
// %w) are exempt as plumbing, so a wrap that is itself the operator's
// terminal framing of a foreign error (os, git) escapes; refusals emitted
// through custom error-struct fields or printed directly to a stream are not
// error-constructor calls and are not seen; the handful of constructor calls
// whose message is a runtime value are counted and logged, never analyzed.
// Known false positives: an origin site that is genuinely internal plumbing
// still counts and must be annotated -- the honest shape for a programmer
// invariant is world-action ("the exit is an action, not a command: edit the
// code"), which the bench vocabulary names verbatim.

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

type refusalRatchetSite struct {
	file        string // slash path relative to the repository root
	line        int
	constructor string // "errors.New" or "fmt.Errorf"
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
	// problems are hard failures -- malformed markers, unknown shapes,
	// contradictory claims, orphaned markers. They are never baseline-able,
	// because a marker that cannot be trusted poisons the exemption it grants.
	problems []string
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
			"\treturn errors.New(\"blocked: run gentle-ai review finalize\")\n"+
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
	baseline := map[string]bool{}
	for _, line := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
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

	removed := 0
	for key := range baseline {
		if _, still := current[key]; !still {
			removed++
		}
	}
	if removed > 0 {
		t.Logf("note: %d baselined entries now name a resolution, are annotated, or are gone.", removed)
		t.Logf("      Tighten the baseline with: GENTLE_AI_REFUSAL_RATCHET_UPDATE=1 go test ./internal/cli -run TestEveryProductionRefusalNamesResolutionOrDeclaresByDesign -count=1")
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
		for _, name := range names {
			source, err := os.ReadFile(filepath.Join(target.dir, name))
			if err != nil {
				t.Fatal(err)
			}
			analysis, err := refusalRatchetAnalyzeSource(path.Join(target.prefix, name), string(source))
			if err != nil {
				t.Fatalf("parse %s: %v", path.Join(target.prefix, name), err)
			}
			merged.violations = append(merged.violations, analysis.violations...)
			merged.satisfiedNamed += analysis.satisfiedNamed
			merged.satisfiedAnnotated += analysis.satisfiedAnnotated
			merged.exemptWraps += analysis.exemptWraps
			merged.unanalyzable = append(merged.unanalyzable, analysis.unanalyzable...)
			merged.problems = append(merged.problems, analysis.problems...)
		}
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

// refusalRatchetAnalyzeSource parses one production source file and classifies
// every errors.New / fmt.Errorf call in it.
func refusalRatchetAnalyzeSource(fileLabel, source string) (refusalRatchetAnalysis, error) {
	var analysis refusalRatchetAnalysis
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, fileLabel, source, parser.ParseComments)
	if err != nil {
		return analysis, err
	}

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

		marker := markers[line]
		if marker == nil {
			marker = markers[line-1]
		}
		if marker != nil {
			marker.consumed = true
		}

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

		named := refusalRatchetNamedContinuationRegexp.MatchString(message)
		switch {
		case marker != nil && marker.malformed != "":
			analysis.problems = append(analysis.problems, marker.malformed)
		case marker != nil && !refusalRatchetByDesignShapes[marker.shape]:
			analysis.problems = append(analysis.problems, fmt.Sprintf(
				"%s:%d declares by-design shape %q, which is not in the closed vocabulary (operator-knowledge, world-action, human-authority); an unrecognized shape is a corpus error, never a pass",
				fileLabel, line, marker.shape))
		case marker != nil && named:
			analysis.problems = append(analysis.problems, fmt.Sprintf(
				"%s:%d is contradictory: the message names a `gentle-ai` continuation AND the site declares by-design %q; a refusal either has a runnable exit or it does not -- the two claims are mutually exclusive",
				fileLabel, line, marker.shape))
		case marker != nil:
			analysis.satisfiedAnnotated++
		case named:
			analysis.satisfiedNamed++
		default:
			analysis.violations = append(analysis.violations, site)
		}
		return true
	})

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
	return analysis, nil
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
