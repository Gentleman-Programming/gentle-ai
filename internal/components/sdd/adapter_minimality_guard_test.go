package sdd

// TestAdapterMinimalityGuard is the structural ratchet for the RDD shared
// advisory reviewer transport migration (skills/rdd-advisory-transport/
// SKILL.md): "adapters only invoke the reviewer and return raw bytes plus a
// transport error... never parse bindings, rebuild prompts, copy schemas,
// apply local budgets... or decide blocking." claudeReviewerPrompt and
// openCodeProviderInjectedReviewerPrompt were unified into one shared
// template (runtimeReviewerPrompt, in boundedreview.go) specifically so this
// package could never again carry two independent copies of the reviewer
// input contract that drift apart on the next edit.
//
// A unit test on rendered prompt bytes cannot prove that invariant: two
// independent renderers can emit identical bytes today and diverge on the
// very next change, with nothing failing until admission behavior silently
// disagrees between runtimes. This guard instead parses the package's own
// production (non-test) source and asserts the SHAPE never grows a second
// implementation:
//
//  1. No hardcoded []string literal re-declares the reviewer result's
//     required top-level field set; that set has one source,
//     reviewtransaction.NewReviewerResultEnvelope().RequiredTopLevelFields.
//  2. No struct type re-declares the reviewer result envelope shape
//     (subject_hash/inspection/findings/evidence) via its own json tags;
//     that shape has one source, reviewtransaction.ReviewerResult.
//  3. No const block re-declares the BLOCKER/CRITICAL/WARNING/SUGGESTION
//     severity enum; severity meaning has exactly one canonical source
//     (reviewtransaction), never a parallel adapter-local copy.
//  4. No OPENCODE_DISABLE_* environment variable is named anywhere in this
//     package. SKILL.md: "No OpenCode restart, child isolation, special
//     session, or OPENCODE_DISABLE_* variables. An ordinary running session
//     is sufficient." -- an adapter that starts requiring one has quietly
//     reintroduced the isolation-transport claim this migration retires.
//  5. Exactly one function renders the shared provider-injected reviewer
//     prompt wording, and it is runtimeReviewerPrompt.
//
// Scope is intentionally the package's production sources only (excluding
// _test.go): test files legitimately declare their own local decode structs
// (see claude_review_network_none_e2e_test.go's subject_hash-tagged struct)
// to assert on captured output, which is not the adapter re-implementing
// admission and must not trip this guard.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// adapterMinimalityRequiredFields is the comparison set this guard checks
// production source against. Production code must read this set from
// reviewtransaction.NewReviewerResultEnvelope().RequiredTopLevelFields, never
// declare it locally -- this slice exists only inside the test binary.
var adapterMinimalityRequiredFields = []string{"subject_hash", "inspection", "findings", "evidence"}

// adapterMinimalitySeverities is the closed severity vocabulary. Its single
// canonical source is reviewtransaction; this package may only describe it in
// prose (the reviewer prompt's "## Severity" bullets), never re-declare it as
// Go constants.
var adapterMinimalitySeverities = []string{"BLOCKER", "CRITICAL", "WARNING", "SUGGESTION"}

// adapterMinimalitySharedWordingMarker is a byte sequence unique to the one
// canonical template that renders the provider-injected reviewer input
// contract (runtimeReviewerPrompt). A second occurrence anywhere in this
// package's production sources means a second, independently-driftable copy
// of the wording claudeReviewerPrompt and openCodeProviderInjectedReviewerPrompt
// used to carry before they were unified.
const adapterMinimalitySharedWordingMarker = "supplies one block from"

func TestAdapterMinimalityGuard(t *testing.T) {
	sources := adapterMinimalityProductionSources(t)

	renderers := 0
	var rendererNames []string

	for file, source := range sources {
		if strings.Contains(source, "OPENCODE_DISABLE_") {
			t.Errorf("%s names an OPENCODE_DISABLE_ environment variable; an ordinary running OpenCode session is sufficient (SKILL.md)", file)
		}

		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, file, source, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}

		ast.Inspect(parsed, func(node ast.Node) bool {
			switch decl := node.(type) {
			case *ast.CompositeLit:
				if values, ok := adapterMinimalityStringSliceElements(decl); ok &&
					adapterMinimalityContainsAll(values, adapterMinimalityRequiredFields) {
					t.Errorf("%s:%d hardcodes the reviewer result's required top-level fields as a literal slice; reference reviewtransaction.NewReviewerResultEnvelope().RequiredTopLevelFields instead",
						file, fset.Position(decl.Pos()).Line)
				}
			case *ast.StructType:
				if tags := adapterMinimalityJSONTags(decl); adapterMinimalityContainsAll(tags, adapterMinimalityRequiredFields) {
					t.Errorf("%s:%d declares a struct whose json tags re-implement the reviewer result envelope; reference reviewtransaction.ReviewerResult instead",
						file, fset.Position(decl.Pos()).Line)
				}
			case *ast.GenDecl:
				if decl.Tok == token.CONST {
					values := adapterMinimalityConstStringValues(decl)
					if adapterMinimalityCountMatches(values, adapterMinimalitySeverities) >= 3 {
						t.Errorf("%s:%d re-declares the BLOCKER/CRITICAL/WARNING/SUGGESTION severity enum as Go constants; severity meaning has exactly one canonical source outside this package",
							file, fset.Position(decl.Pos()).Line)
					}
				}
			case *ast.FuncDecl:
				if decl.Body != nil && adapterMinimalityBodyContainsSharedWording(fset, source, decl.Body) {
					renderers++
					rendererNames = append(rendererNames, decl.Name.Name)
				}
			}
			return true
		})
	}

	sort.Strings(rendererNames)
	if renderers != 1 {
		t.Fatalf("found %d function(s) rendering the shared provider-injected reviewer prompt wording (%v); claudeReviewerPrompt and openCodeProviderInjectedReviewerPrompt must both delegate to the single runtimeReviewerPrompt renderer instead of carrying their own copy",
			renderers, rendererNames)
	}
	if rendererNames[0] != "runtimeReviewerPrompt" {
		t.Fatalf("the sole reviewer-prompt renderer is %q, want runtimeReviewerPrompt; update this guard's expected name if the rename was intentional and reviewed", rendererNames[0])
	}
}

// adapterMinimalityProductionSources reads every non-test .go file in this
// package directory. The threshold below is a broken-walk tripwire, the same
// role beforeChars/total-site-count checks play in this repo's other static
// ratchets (see TestEveryProductionRefusalNamesResolutionOrDeclaresByDesign):
// a walk that suddenly sees far fewer files than the package actually has is
// broken, not clean.
func adapterMinimalityProductionSources(t *testing.T) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	sources := map[string]string{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatal(err)
		}
		sources[name] = string(content)
	}
	if len(sources) < 5 {
		t.Fatalf("enumerated only %d production source file(s) in internal/components/sdd; the walk is broken", len(sources))
	}
	return sources
}

// adapterMinimalityBodyContainsSharedWording slices the exact source bytes
// of one function body (by byte offset, from the same source string that was
// parsed) and checks it for the shared-wording marker. Slicing the real
// source rather than re-printing the AST keeps this a byte-level check: the
// marker must appear in the literal Go source, not in a semantically
// equivalent reconstruction of it.
func adapterMinimalityBodyContainsSharedWording(fset *token.FileSet, source string, body *ast.BlockStmt) bool {
	start := fset.Position(body.Pos()).Offset
	end := fset.Position(body.End()).Offset
	if start < 0 || end > len(source) || start > end {
		return false
	}
	return strings.Contains(source[start:end], adapterMinimalitySharedWordingMarker)
}

// adapterMinimalityStringSliceElements returns a composite literal's element
// values when it is a slice-of-string literal built entirely from string
// constants (e.g. []string{"a", "b"}), and false otherwise. A literal with
// any non-constant or non-string element is not a static field-list
// re-declaration and is reported as not-a-match rather than guessed at.
func adapterMinimalityStringSliceElements(lit *ast.CompositeLit) ([]string, bool) {
	arrayType, ok := lit.Type.(*ast.ArrayType)
	if !ok || arrayType.Len != nil {
		return nil, false
	}
	ident, ok := arrayType.Elt.(*ast.Ident)
	if !ok || ident.Name != "string" {
		return nil, false
	}
	values := make([]string, 0, len(lit.Elts))
	for _, elt := range lit.Elts {
		basic, ok := elt.(*ast.BasicLit)
		if !ok || basic.Kind != token.STRING {
			return nil, false
		}
		value, err := strconv.Unquote(basic.Value)
		if err != nil {
			return nil, false
		}
		values = append(values, value)
	}
	return values, true
}

// adapterMinimalityJSONTags returns the `json:"..."` field names a struct
// type declares, skipping untagged fields and the "-" (omit) tag.
func adapterMinimalityJSONTags(structType *ast.StructType) []string {
	var tags []string
	if structType.Fields == nil {
		return tags
	}
	for _, field := range structType.Fields.List {
		if field.Tag == nil {
			continue
		}
		raw, err := strconv.Unquote(field.Tag.Value)
		if err != nil {
			continue
		}
		name := strings.Split(reflect.StructTag(raw).Get("json"), ",")[0]
		if name != "" && name != "-" {
			tags = append(tags, name)
		}
	}
	return tags
}

// adapterMinimalityConstStringValues returns every string constant value a
// GenDecl(CONST) block declares, across all its specs.
func adapterMinimalityConstStringValues(decl *ast.GenDecl) []string {
	var values []string
	for _, spec := range decl.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, value := range valueSpec.Values {
			basic, ok := value.(*ast.BasicLit)
			if !ok || basic.Kind != token.STRING {
				continue
			}
			if unquoted, err := strconv.Unquote(basic.Value); err == nil {
				values = append(values, unquoted)
			}
		}
	}
	return values
}

// adapterMinimalityContainsAll reports whether haystack contains every
// element of needles. An empty haystack never matches: a struct or literal
// with no relevant tags/elements is not a re-declaration of anything.
func adapterMinimalityContainsAll(haystack, needles []string) bool {
	if len(haystack) == 0 {
		return false
	}
	set := make(map[string]bool, len(haystack))
	for _, value := range haystack {
		set[value] = true
	}
	for _, needle := range needles {
		if !set[needle] {
			return false
		}
	}
	return true
}

// adapterMinimalityCountMatches counts how many distinct candidates appear
// in values.
func adapterMinimalityCountMatches(values, candidates []string) int {
	set := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		set[candidate] = true
	}
	seen := make(map[string]bool, len(values))
	count := 0
	for _, value := range values {
		if set[value] && !seen[value] {
			seen[value] = true
			count++
		}
	}
	return count
}
