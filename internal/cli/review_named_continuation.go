// This file is the production-side mirror of reviewDispatchableReviewVerbs in
// review_named_continuation_test.go. The test wrapper takes a *testing.T and
// calls t.Fatal on extraction failure; this export returns an error so the
// asset and prompt collectors (which are NOT test files) can ask the same
// question without dragging the test runner into production builds.
//
// The two mechanical extractions that own the answer are unchanged:
// review_facade.go's own dispatch switches, and internal/app's review
// pre-dispatch (the kill switch must stay reachable when review authority
// itself is disabled, so it never appears in review_facade.go's switch).
// Both fail closed -- when the extraction stops finding anything, the
// returned error names the surface so the caller knows which side is stale.
package cli

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// reviewAppPreDispatchRegexp captures the review sub-verbs internal/app's
// `case "review":` arm dispatches BEFORE handing the rest to RunReview.
var reviewAppPreDispatchRegexp = regexp.MustCompile(`args\[1\] == "([a-z][a-z-]*)"`)

// ReviewDispatchableReviewVerbs returns every verb `gentle-ai review <verb>`
// really reaches, from the two mechanical extractions that own the answer:
// review_facade.go's own dispatch switches, and internal/app's review
// pre-dispatch. It returns an error (rather than calling t.Fatal) so callers
// outside the test package -- the asset and prompt collectors -- can ask the
// same question and translate the failure into the contract they own.
func ReviewDispatchableReviewVerbs() (map[string]bool, error) {
	facade, err := reviewFacadeDispatchVerbs()
	if err != nil {
		return nil, fmt.Errorf("facade dispatch: %w", err)
	}
	if len(facade) == 0 {
		// refusal:by-design world-action: empty extraction means review_facade.go lost its dispatch labels; the fix is in source, not on the CLI
		return nil, fmt.Errorf("found no dispatched review command verbs in review_facade.go; the extraction is stale")
	}
	pre, err := reviewAppPreDispatchVerbs()
	if err != nil {
		return nil, fmt.Errorf("app pre-dispatch: %w", err)
	}
	if len(pre) == 0 {
		// refusal:by-design world-action: empty extraction means app.go lost its `case "review":` arm; the fix is in source, not on the CLI
		return nil, fmt.Errorf("found no review sub-verb pre-dispatch in internal/app/app.go's `case \"review\":` block; the extraction is stale")
	}
	verbs := map[string]bool{}
	for v := range facade {
		verbs[v] = true
	}
	for v := range pre {
		verbs[v] = true
	}
	return verbs, nil
}

// reviewAppPreDispatchVerbs reads internal/app/app.go and returns the
// sub-verbs the `case "review":` arm dispatches before RunReview. Limiting
// the extraction to that arm's source range keeps an args[1] comparison
// belonging to a different command from being picked up.
func reviewAppPreDispatchVerbs() (map[string]bool, error) {
	source, err := os.ReadFile(filepath.Join("..", "app", "app.go"))
	if err != nil {
		return nil, fmt.Errorf("read internal/app/app.go: %w", err)
	}
	text := string(source)
	const marker = "\t\tcase \"review\":\n"
	start := strings.Index(text, marker)
	if start < 0 {
		// refusal:by-design world-action: the arm was removed from app.go; restore it in source
		return nil, fmt.Errorf("internal/app/app.go has no `case \"review\":` arm; the extraction is stale")
	}
	rest := text[start+len(marker):]
	if end := strings.Index(rest, "\t\tcase \""); end >= 0 {
		rest = rest[:end]
	}
	verbs := map[string]bool{}
	for _, match := range reviewAppPreDispatchRegexp.FindAllStringSubmatch(rest, -1) {
		verbs[match[1]] = true
	}
	return verbs, nil
}

// reviewFacadeDispatchVerbs parses review_facade.go and returns every verb
// the runReviewCommandContext and runReviewCommand dispatch switches own.
func reviewFacadeDispatchVerbs() (map[string]bool, error) {
	source, err := os.ReadFile("review_facade.go")
	if err != nil {
		return nil, fmt.Errorf("read review_facade.go: %w", err)
	}
	return reviewFacadeDispatchVerbsFromSource(source)
}

func reviewFacadeDispatchVerbsFromSource(source []byte) (map[string]bool, error) {
	file, err := parser.ParseFile(token.NewFileSet(), "review_facade.go", source, 0)
	if err != nil {
		return nil, fmt.Errorf("parse review_facade.go: %w", err)
	}
	owners := map[string]string{}
	found := map[string]bool{}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil || (function.Name.Name != "runReviewCommandContext" && function.Name.Name != "runReviewCommand") {
			continue
		}
		if found[function.Name.Name] {
			// refusal:by-design world-action: a duplicate dispatch function is a source-level bug; dedupe in code
			return nil, fmt.Errorf("review facade declares %s more than once", function.Name.Name)
		}
		found[function.Name.Name] = true
		dispatch, err := reviewFacadeDispatchSwitch(function)
		if err != nil {
			return nil, err
		}
		for _, clause := range dispatch.Body.List {
			caseClause, ok := clause.(*ast.CaseClause)
			if !ok {
				// refusal:by-design world-action: a malformed switch in review_facade.go is a source-level bug; repair the file
				return nil, fmt.Errorf("%s dispatch switch contains %T instead of a case clause", function.Name.Name, clause)
			}
			for _, expression := range caseClause.List {
				literal, ok := expression.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					// refusal:by-design world-action: a non-literal case label in the dispatch switch is a source-level bug; repair the file
					return nil, fmt.Errorf("%s dispatch case expression = %T, want string literal", function.Name.Name, expression)
				}
				verb, unquoteErr := strconv.Unquote(literal.Value)
				if unquoteErr != nil {
					return nil, fmt.Errorf("unquote %s dispatch case: %w", function.Name.Name, unquoteErr)
				}
				if owner, exists := owners[verb]; exists {
					// refusal:by-design world-action: a verb claimed by two dispatch functions is a source-level bug; dedupe in code
					return nil, fmt.Errorf("review command %q dispatched by both %s and %s", verb, owner, function.Name.Name)
				}
				owners[verb] = function.Name.Name
			}
		}
	}
	for _, name := range []string{"runReviewCommandContext", "runReviewCommand"} {
		if !found[name] {
			// refusal:by-design world-action: a missing dispatch function in review_facade.go is a source-level bug; restore the file
			return nil, fmt.Errorf("review_facade.go has no %s function", name)
		}
	}
	verbs := map[string]bool{}
	for v := range owners {
		verbs[v] = true
	}
	return verbs, nil
}

func reviewFacadeDispatchSwitch(function *ast.FuncDecl) (*ast.SwitchStmt, error) {
	var dispatch *ast.SwitchStmt
	for _, statement := range function.Body.List {
		switchStatement, ok := statement.(*ast.SwitchStmt)
		if !ok || !reviewFacadeDispatchTag(switchStatement.Tag) {
			continue
		}
		if dispatch != nil {
			// refusal:by-design world-action: a second dispatch switch in one function is a source-level bug; collapse in code
			return nil, fmt.Errorf("%s has more than one top-level review command dispatch switch", function.Name.Name)
		}
		dispatch = switchStatement
	}
	if dispatch == nil {
		// refusal:by-design world-action: a missing dispatch switch is a source-level bug; restore the file
		return nil, fmt.Errorf("%s has no top-level `switch args[0]` dispatch", function.Name.Name)
	}
	return dispatch, nil
}

func reviewFacadeDispatchTag(expr ast.Expr) bool {
	index, ok := expr.(*ast.IndexExpr)
	if !ok {
		return false
	}
	argsIdent, ok := index.X.(*ast.Ident)
	if !ok || argsIdent.Name != "args" {
		return false
	}
	indexLit, ok := index.Index.(*ast.BasicLit)
	return ok && indexLit.Kind == token.INT && indexLit.Value == "0"
}
