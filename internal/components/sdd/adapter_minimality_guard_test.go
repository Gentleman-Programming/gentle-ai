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
// TestAdapterMinimalityGuardExtendsToProviderAdapters below extends the same
// ratchet to the thin Claude/OpenCode transport adapters that live in
// internal/advisoryreview (change #3138, slice 3): those files may only hold
// LookPath/PromptFor seams, one executor call, and an env-allowlist helper --
// never a second implementation of rendering, admission, or transport policy.
//
// TestAdapterMinimalityGuardExtendsToOpenCodeShim extends it one step further
// to the slice-4 native shim (change #3138): opencode_shim.go may own exactly
// the dispatcher contract -- provenance admission, legacy deferral, and the
// one binding route parse -- and nothing else. In particular it must never
// render prompt text, re-declare the result envelope, apply budgets, admit a
// review, or reach into review authority; its one sanctioned JSON decode is
// the opaque binding route, and the glue asset stays hook-free (pinned
// separately by assets_test.go's TestReviewerShimPluginContract).
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

// adapterMinimalityAdapterAllowlists records, per thin transport adapter, the
// only environment variables its child runtime may receive. This mirrors the
// CodexAdapter allowlist (PATH + CODEX_HOME) one-to-one: each runtime gets
// PATH so the process can start, plus its own config-home override so it can
// read its own auth/config without inheriting the caller's ambient state.
var adapterMinimalityAdapterAllowlists = map[string][]string{
	"claude_adapter.go":   {"PATH", "CLAUDE_CONFIG_DIR"},
	"opencode_adapter.go": {"PATH", "OPENCODE_CONFIG_DIR"},
}

// adapterMinimalityAdapterNames is the deterministic order the adapter files
// are checked in (map iteration order is not).
var adapterMinimalityAdapterNames = []string{"claude_adapter.go", "opencode_adapter.go"}

// TestAdapterMinimalityGuardExtendsToProviderAdapters is the slice-3 ratchet
// (SEN-RPC-4, tasks 3.1): the same "adapters only invoke the reviewer and
// return raw bytes plus a transport error" invariant that TestAdapterMinimalityGuard
// holds for this package's production sources must also hold for the thin
// Claude/OpenCode transport adapters in internal/advisoryreview, and it must
// not drift as those files evolve. The new adapter files are the RED target:
// they did not exist until task 3.3, so this test failed by construction the
// moment it was written (missing file), exactly like the slice-2 provider
// RED.
func TestAdapterMinimalityGuardExtendsToProviderAdapters(t *testing.T) {
	for _, name := range adapterMinimalityAdapterNames {
		name := name
		t.Run(name, func(t *testing.T) {
			source := adapterMinimalityAdapterSource(t, name)
			adapterMinimalityEnforceAdapterShape(t, name, source, adapterMinimalityAdapterAllowlists[name])
		})
	}
}

// adapterMinimalityAdapterSource reads one thin transport adapter from
// internal/advisoryreview (two levels above this package's directory: sdd/
// lives under internal/components/, the adapters live under internal/). The
// file is a slice-3 artifact, so reading it is the RED half of the cycle: a
// missing file is a failing test, not a silent skip.
func adapterMinimalityAdapterSource(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", "advisoryreview", name)
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read thin adapter %s: %v (slice-3 RED: %s is created by task 3.3)", path, err, name)
	}
	return string(source)
}

// adapterMinimalityEnforceAdapterShape holds every thinness rule against one
// adapter's production source. Rules 1-4 reuse the exact composite-literal,
// struct-tag, and severity checks the package guard applies to this
// package's own sources; rules 5-8 are adapter-specific and forbid the
// constructs the shared advisory contract assigns to Prompt/Validate alone:
//
//  1. no []string literal re-declares the reviewer result's top-level fields
//  2. no struct re-declares a binding or result envelope shape via json tags
//  3. no const block re-declares the severity enum
//  4. no second copy of the shared provider-injected prompt wording
//  5. no schema/budget/parser/admission identifiers (OutputSchema, budgets,
//     json, Validate/Unmarshal/...) and no prompt renderers
//  6. no capture/preserve/retry/blocking identifiers (SEN-RPC-19/20:
//     strand-nothing, no budget or correction consumption)
//  7. argv pin: no "-C"/"--dir"/git-selector string literal a child runtime
//     could receive (threat matrix, Git repository selection)
//  8. env allowlist: the child process reads exactly this adapter's
//     allowlist from the parent via os.LookupEnv, nothing else (threat
//     matrix, PR commands / env-leak)
func adapterMinimalityEnforceAdapterShape(t *testing.T, name, source string, envAllowlist []string) {
	t.Helper()
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, name, source, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}

	var idents, literals, imports []string
	// ImportSpec.Path is a *ast.BasicLit of kind token.STRING, so every import
	// path would otherwise enter `literals` and trip the argv scan below as a
	// false "carries argv literal". Import policy already has its own rule;
	// skip the path nodes here.
	importPaths := map[ast.Node]bool{}
	for _, spec := range parsed.Imports {
		if spec.Path != nil {
			importPaths[spec.Path] = true
		}
	}
	ast.Inspect(parsed, func(node ast.Node) bool {
		if node == nil {
			return true
		}
		position := fset.Position(node.Pos())
		switch decl := node.(type) {
		case *ast.CompositeLit:
			if values, ok := adapterMinimalityStringSliceElements(decl); ok &&
				adapterMinimalityContainsAll(values, adapterMinimalityRequiredFields) {
				t.Errorf("%s:%d hardcodes the reviewer result's required top-level fields as a literal slice; reference reviewtransaction.NewReviewerResultEnvelope().RequiredTopLevelFields instead",
					name, position.Line)
			}
		case *ast.StructType:
			if tags := adapterMinimalityJSONTags(decl); adapterMinimalityContainsAll(tags, adapterMinimalityRequiredFields) {
				t.Errorf("%s:%d declares a struct whose json tags re-implement the reviewer result envelope; an adapter must never decode or re-declare it",
					name, position.Line)
			}
		case *ast.GenDecl:
			if decl.Tok == token.CONST {
				values := adapterMinimalityConstStringValues(decl)
				if adapterMinimalityCountMatches(values, adapterMinimalitySeverities) >= 3 {
					t.Errorf("%s:%d re-declares the BLOCKER/CRITICAL/WARNING/SUGGESTION severity enum as Go constants; severity meaning has exactly one canonical source outside the adapters",
						name, position.Line)
				}
			}
		case *ast.Ident:
			idents = append(idents, decl.Name)
		case *ast.BasicLit:
			if decl.Kind == token.STRING && !importPaths[decl] {
				if value, err := strconv.Unquote(decl.Value); err == nil {
					literals = append(literals, value)
				}
			}
		case *ast.ImportSpec:
			if decl.Path != nil {
				if path, err := strconv.Unquote(decl.Path.Value); err == nil {
					imports = append(imports, path)
				}
			}
		}
		return true
	})

	if strings.Contains(source, adapterMinimalitySharedWordingMarker) {
		t.Errorf("%s carries a second copy of the shared provider-injected reviewer prompt wording; adapters must never render prompt text (marker %q)",
			name, adapterMinimalitySharedWordingMarker)
	}
	for _, value := range idents {
		lower := strings.ToLower(value)
		for _, forbidden := range []string{"capture", "preserve", "retry", "blocking"} {
			if strings.Contains(lower, forbidden) {
				t.Errorf("%s names identifier %q; adapters must strand nothing and consume no budget or correction state (%s)", name, value, forbidden)
			}
		}
		switch value {
		case "OutputSchema", "ReviewerResultSchema", "MaxEvidenceEntries", "maxResultBytes", "maxEvidenceBytes",
			"Validate", "Unmarshal", "Marshal", "NewDecoder", "Prompt", "LensMandate":
			t.Errorf("%s references %q; schema, budgets, admission, and prompt rendering belong to the provider, never an adapter", name, value)
		}
	}
	for _, value := range idents {
		if strings.HasPrefix(value, "runtimeReviewerPrompt") || value == "advisoryPrompt" {
			t.Errorf("%s references prompt renderer %q; adapters must never render prompt text", name, value)
		}
	}
	for _, path := range imports {
		if strings.Contains(path, "encoding/json") || strings.Contains(path, "reviewtransaction") {
			t.Errorf("%s imports %q; adapters must never parse child output or reach into review authority", name, path)
		}
	}
	for _, value := range literals {
		if value == "-C" || value == "--dir" || strings.Contains(strings.ToLower(value), "git") {
			t.Errorf("%s carries argv literal %q; an adapter child process must never receive a git/repository selector", name, value)
		}
	}

	lookedUp := adapterMinimalityLookupEnvKeys(parsed)
	if len(lookedUp) == 0 {
		t.Errorf("%s never reads the parent environment via os.LookupEnv; the child process must receive this adapter's explicit env allowlist", name)
	}
	for _, key := range lookedUp {
		if !adapterMinimalityContains(envAllowlist, key) {
			t.Errorf("%s looks up environment variable %q outside its allowlist %v; the child process may only receive PATH plus this runtime's config home", name, key, envAllowlist)
		}
	}
}

// adapterMinimalityLookupEnvKeys returns every literal key name passed to
// os.LookupEnv in the parsed file.
func adapterMinimalityLookupEnvKeys(parsed *ast.File) []string {
	var keys []string
	ast.Inspect(parsed, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "LookupEnv" || len(call.Args) != 1 {
			return true
		}
		argument, ok := call.Args[0].(*ast.BasicLit)
		if !ok || argument.Kind != token.STRING {
			return true
		}
		if key, err := strconv.Unquote(argument.Value); err == nil {
			keys = append(keys, key)
		}
		return true
	})
	return keys
}

// adapterMinimalityContains reports whether values contains target.
func adapterMinimalityContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// TestAdapterMinimalityGuardExtendsToOpenCodeShim is the slice-4 ratchet
// (change #3138): the native OpenCode shim (internal/advisoryreview/
// opencode_shim.go) may own provenance admission, legacy deferral, and the
// one binding route parse -- never a second prompt renderer, never envelope
// or severity re-declaration, never budgets or admission, never a reach into
// review authority. The seam type and the refusal constant are the sanctioned
// shim surface; the single encoding/json decode is the opaque binding route,
// the exact one-field parse the legacy plugin performed with JSON.parse. The
// file is a slice-4 artifact, so reading it is the RED half of the cycle: a
// missing or overgrown file fails here, exactly like the slice-3 adapter
// guard.
func TestAdapterMinimalityGuardExtendsToOpenCodeShim(t *testing.T) {
	name := "opencode_shim.go"
	path := filepath.Join("..", "..", "advisoryreview", name)
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read native OpenCode shim %s: %v (slice-4 RED: %s is created by task 4.2)", path, err, name)
	}
	adapterMinimalityEnforceShimShape(t, name, string(source))
}

// adapterMinimalityEnforceShimShape holds every thinness rule against the
// native shim's production source. Rules 1-4 reuse the exact composite-
// literal, struct-tag, severity, and shared-wording checks the package guard
// and the adapter guard apply; rules 5-9 are shim-specific:
//
//  1. no []string literal re-declares the reviewer result's top-level fields
//  2. no struct re-declares a binding or result envelope shape via json tags
//  3. no const block re-declares the severity enum
//  4. no second copy of the shared provider-injected prompt wording
//  5. no schema/budget/admission identifiers (OutputSchema, budgets,
//     Validate, NewDecoder, Prompt renderers, ...) -- deliberately without
//     Unmarshal/Marshal, which the one binding route parse may use
//  6. no capture/preserve/retry/blocking identifiers (SEN-RPC-19/20:
//     strand-nothing, no budget or correction consumption)
//  7. no runtimeReviewerPrompt/advisoryPrompt references -- the shim never
//     renders prompt text
//  8. no reviewtransaction import: authority stays in reviewtransaction, the
//     shim never reaches into it
//  9. encoding/json is allowed for exactly one job, the opaque binding route
//     parse: exactly one import and exactly one json.Unmarshal call site, so
//     no second decode (in particular never a child-output parse) can appear
//     unnoticed
func adapterMinimalityEnforceShimShape(t *testing.T, name, source string) {
	t.Helper()
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, name, source, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}

	var idents, imports []string
	ast.Inspect(parsed, func(node ast.Node) bool {
		if node == nil {
			return true
		}
		position := fset.Position(node.Pos())
		switch decl := node.(type) {
		case *ast.CompositeLit:
			if values, ok := adapterMinimalityStringSliceElements(decl); ok &&
				adapterMinimalityContainsAll(values, adapterMinimalityRequiredFields) {
				t.Errorf("%s:%d hardcodes the reviewer result's required top-level fields as a literal slice; reference reviewtransaction.NewReviewerResultEnvelope().RequiredTopLevelFields instead",
					name, position.Line)
			}
		case *ast.StructType:
			if tags := adapterMinimalityJSONTags(decl); adapterMinimalityContainsAll(tags, adapterMinimalityRequiredFields) {
				t.Errorf("%s:%d declares a struct whose json tags re-implement the reviewer result envelope; the shim must never decode or re-declare it",
					name, position.Line)
			}
		case *ast.GenDecl:
			if decl.Tok == token.CONST {
				values := adapterMinimalityConstStringValues(decl)
				if adapterMinimalityCountMatches(values, adapterMinimalitySeverities) >= 3 {
					t.Errorf("%s:%d re-declares the BLOCKER/CRITICAL/WARNING/SUGGESTION severity enum as Go constants; severity meaning has exactly one canonical source outside the shim",
						name, position.Line)
				}
			}
		case *ast.Ident:
			idents = append(idents, decl.Name)
		case *ast.ImportSpec:
			if decl.Path != nil {
				if path, err := strconv.Unquote(decl.Path.Value); err == nil {
					imports = append(imports, path)
				}
			}
		}
		return true
	})

	if strings.Contains(source, adapterMinimalitySharedWordingMarker) {
		t.Errorf("%s carries the shared provider-injected reviewer prompt wording; the shim must never render prompt text (marker %q)",
			name, adapterMinimalitySharedWordingMarker)
	}
	for _, value := range idents {
		lower := strings.ToLower(value)
		for _, forbidden := range []string{"capture", "preserve", "retry", "blocking"} {
			if strings.Contains(lower, forbidden) {
				t.Errorf("%s names identifier %q; the shim must strand nothing and consume no budget or correction state (%s)", name, value, forbidden)
			}
		}
		switch value {
		case "OutputSchema", "ReviewerResultSchema", "MaxEvidenceEntries", "maxResultBytes", "maxEvidenceBytes",
			"Validate", "NewDecoder", "Prompt", "LensMandate":
			t.Errorf("%s references %q; schema, budgets, admission, and prompt rendering belong to the provider, never the shim", name, value)
		}
	}
	for _, value := range idents {
		if strings.HasPrefix(value, "runtimeReviewerPrompt") || value == "advisoryPrompt" {
			t.Errorf("%s references prompt renderer %q; the shim must never render prompt text", name, value)
		}
	}
	for _, path := range imports {
		if strings.Contains(path, "reviewtransaction") {
			t.Errorf("%s imports %q; the shim must never reach into review authority", name, path)
		}
	}

	// The single sanctioned decode: the opaque binding route parse. A second
	// encoding/json import or a second json.Unmarshal call site would mean the
	// shim started parsing something else (child output above all) and must
	// fail here before it ever ships.
	jsonImports := 0
	for _, path := range imports {
		if path == "encoding/json" {
			jsonImports++
		}
	}
	unmarshalCalls := 0
	ast.Inspect(parsed, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Unmarshal" {
			return true
		}
		receiver, ok := selector.X.(*ast.Ident)
		if ok && receiver.Name == "json" {
			unmarshalCalls++
		}
		return true
	})
	if jsonImports != 1 {
		t.Errorf("%s must import encoding/json exactly once (the binding route parse); got %d import(s)", name, jsonImports)
	}
	if unmarshalCalls != 1 {
		t.Errorf("%s must decode exactly one JSON document (the opaque binding route); got %d json.Unmarshal call site(s)", name, unmarshalCalls)
	}
}
