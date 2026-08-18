package reviewerprovider

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// reviewerAdapterImplementations is deliberately closed. Adding an adapter
// requires naming it here and classifying it before it can enter the provider
// boundary, rather than relying on a filename glob that silently grows scope.
var reviewerAdapterImplementations = map[string]string{
	"claude_adapter.go": "ClaudeAdapter",
	"codex_adapter.go":  "CodexAdapter",
	"pi_adapter.go":     "PiAdapter",
}

// TestAdapterMinimalityGuard protects the Go-owned review boundary. An adapter
// may invoke a host with Invocation.Prompt and return raw bytes or a transport
// error. Binding, prompt construction, schema, budgets, parsing, admission,
// capture, retry, and correction remain outside this package.
func TestAdapterMinimalityGuard(t *testing.T) {
	sources := reviewerProviderProductionSources(t)
	for _, violation := range reviewerAdapterInventoryViolations(sources) {
		t.Error(violation)
	}
	for file := range reviewerAdapterImplementations {
		for _, violation := range reviewerAdapterSourceViolations(file, sources[file]) {
			t.Error(violation)
		}
	}
}

func TestReviewerAdapterGuardDetectsSemanticOwnership(t *testing.T) {
	violations := reviewerAdapterSourceViolations("bad_adapter.go", []byte("package reviewerprovider\n"+`
import (
  "encoding/json"
  "github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)
type binding struct { SubjectHash string `+"`json:\"subject_hash\"`"+` }
func (adapter *BadAdapter) Review(ctx context.Context, invocation Invocation) ([]byte, error) {
  _ = json.Valid(nil)
  _ = reviewtransaction.ReviewerResultSchema
  _ = AdmitArtifact
  _ = CaptureAdmittedReviewerResult
  _ = CorrectionPlan
  _ = NewInvocation([]byte("GENTLE_AI_REVIEW_CONTEXT"))
  prompt := invocation.Prompt()
  if len(prompt) > 0 { return nil, nil }
  for retry := 0; retry < 1; retry++ {}
  return nil, nil
}
`))
	for _, want := range []string{
		"imports provider-owned review semantics", "uses provider-owned identifier ReviewerResultSchema",
		"uses provider-owned identifier AdmitArtifact", "uses provider-owned identifier CaptureAdmittedReviewerResult",
		"uses provider-owned identifier CorrectionPlan", "uses provider-owned identifier NewInvocation",
		"contains provider prompt text", "declares reviewer binding field subject_hash", "measures adapter bytes", "contains a loop",
	} {
		if !slices.Contains(violations, want) {
			t.Fatalf("violations = %v, missing %q", violations, want)
		}
	}
}

func TestReviewerAdapterGuardRequiresExplicitClassification(t *testing.T) {
	sources := map[string][]byte{
		"claude_adapter.go": []byte("package reviewerprovider\ntype ClaudeAdapter struct{}\nfunc (adapter *ClaudeAdapter) Review() {}\n"),
		"codex_adapter.go":  []byte("package reviewerprovider\ntype CodexAdapter struct{}\nfunc (adapter *CodexAdapter) Review() {}\n"),
		"future.go": []byte(`package reviewerprovider
func (adapter *FutureAdapter) Review() {}
`),
	}
	if violations := reviewerAdapterInventoryViolations(sources); !slices.Contains(violations, "unclassified adapter implementation future.go") {
		t.Fatalf("inventory violations = %v, want unclassified adapter", violations)
	}
}

func reviewerProviderProductionSources(t *testing.T) map[string][]byte {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	sources := make(map[string][]byte)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatal(err)
		}
		sources[name] = source
	}
	return sources
}

func reviewerAdapterInventoryViolations(sources map[string][]byte) []string {
	var violations []string
	for file, source := range sources {
		if !reviewerAdapterImplementation(file, source) {
			continue
		}
		if _, classified := reviewerAdapterImplementations[file]; !classified {
			violations = append(violations, "unclassified adapter implementation "+file)
		}
	}
	for file := range reviewerAdapterImplementations {
		source, found := sources[file]
		if !found {
			violations = append(violations, "classified adapter implementation is missing "+file)
			continue
		}
		if err := reviewerAdapterClassificationValid(source, reviewerAdapterImplementations[file]); err != nil {
			violations = append(violations, file+" "+err.Error())
		}
	}
	sort.Strings(violations)
	return violations
}

func reviewerAdapterClassificationValid(source []byte, typeName string) error {
	parsed, err := parser.ParseFile(token.NewFileSet(), "adapter.go", source, 0)
	if err != nil {
		return fmt.Errorf("does not parse: %w", err)
	}
	declared, reviewMethod := false, false
	for _, declaration := range parsed.Decls {
		switch value := declaration.(type) {
		case *ast.GenDecl:
			for _, spec := range value.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if ok && typeSpec.Name.Name == typeName {
					declared = true
				}
			}
		case *ast.FuncDecl:
			if value.Recv != nil && value.Name.Name == "Review" && receiverTypeName(value.Recv) == typeName {
				reviewMethod = true
			}
		}
	}
	if !declared {
		return fmt.Errorf("does not declare classified type %s", typeName)
	}
	if !reviewMethod {
		return fmt.Errorf("does not implement Review on classified type %s", typeName)
	}
	return nil
}

func receiverTypeName(fields *ast.FieldList) string {
	if fields == nil || len(fields.List) != 1 {
		return ""
	}
	expression := fields.List[0].Type
	if pointer, ok := expression.(*ast.StarExpr); ok {
		expression = pointer.X
	}
	identifier, _ := expression.(*ast.Ident)
	if identifier == nil {
		return ""
	}
	return identifier.Name
}

func reviewerAdapterImplementation(file string, source []byte) bool {
	if strings.HasSuffix(file, "_adapter.go") {
		return true
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), file, source, 0)
	if err != nil {
		return true
	}
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Recv != nil && function.Name.Name == "Review" {
			return true
		}
	}
	return false
}

func reviewerAdapterSourceViolations(file string, source []byte) []string {
	parsed, err := parser.ParseFile(token.NewFileSet(), file, source, 0)
	if err != nil {
		return []string{"parse " + file + ": " + err.Error()}
	}

	violations := make(map[string]bool)
	add := func(message string) { violations[message] = true }
	promptAliases := map[string]bool{}
	for _, imported := range parsed.Imports {
		path := strings.Trim(imported.Path.Value, `"`)
		if strings.Contains(path, "json") || strings.Contains(path, "parser") ||
			strings.Contains(path, "/internal/cli") || strings.Contains(path, "/internal/reviewtransaction") {
			add("imports provider-owned review semantics")
		}
	}

	forbiddenIdentifiers := map[string]bool{
		"AdmitArtifact": true, "ArtifactAdmission": true, "ArtifactAdmissionRequest": true, "ArtifactSubject": true,
		"CaptureAdmittedReviewerResult": true, "CanonicalizeReviewerResult": true,
		"CorrectionPlan": true, "DiscoverReviewerContextLevel": true,
		"ExtractBoundedSingleJSONObject": true, "FrozenCandidateContext": true,
		"LensContextEmission": true, "MaxEvidenceEntries": true,
		"MaxFrozenCandidateDiffBytes": true, "NewInvocation": true,
		"PublishLensContextEmission": true, "ReviewerContextLevel": true,
		"ReviewerResult": true, "ReviewerResultSchema": true,
		"TargetedValidationRequest": true, "ValidateReviewerResult": true,
		"reviewLensContextBinding": true, "reviewResultArtifactLimit": true,
	}
	forbiddenBindingTags := map[string]bool{
		"lineage": true, "target": true, "lens": true, "order": true, "revision": true,
		"repository_context": true, "subject_hash": true,
	}
	ast.Inspect(parsed, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 || !invocationPromptCall(assignment.Rhs[0]) {
			return true
		}
		identifier, ok := assignment.Lhs[0].(*ast.Ident)
		if ok {
			promptAliases[identifier.Name] = true
		}
		return true
	})
	ast.Inspect(parsed, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.Ident:
			if forbiddenIdentifiers[value.Name] {
				add("uses provider-owned identifier " + value.Name)
			}
		case *ast.Field:
			if value.Tag == nil {
				break
			}
			tag, unquoteErr := strconv.Unquote(value.Tag.Value)
			if unquoteErr == nil {
				jsonName := strings.Split(reflect.StructTag(tag).Get("json"), ",")[0]
				if forbiddenBindingTags[jsonName] {
					add("declares reviewer binding field " + jsonName)
				}
			}
		case *ast.BasicLit:
			if value.Kind == token.STRING {
				literal, unquoteErr := strconv.Unquote(value.Value)
				if unquoteErr == nil && (strings.Contains(literal, "GENTLE_AI_REVIEW_") || strings.Contains(literal, "subject_hash")) {
					add("contains provider prompt text")
				}
			}
		case *ast.CallExpr:
			if identifier, ok := value.Fun.(*ast.Ident); ok && identifier.Name == "len" && len(value.Args) == 1 &&
				(invocationPromptCall(value.Args[0]) || promptAlias(value.Args[0], promptAliases)) {
				add("measures adapter bytes")
			}
		case *ast.ForStmt, *ast.RangeStmt:
			add("contains a loop")
		}
		return true
	})

	result := make([]string, 0, len(violations))
	for violation := range violations {
		result = append(result, violation)
	}
	sort.Strings(result)
	return result
}

func invocationPromptCall(expression ast.Expr) bool {
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	return ok && selector.Sel.Name == "Prompt"
}

func promptAlias(expression ast.Expr, aliases map[string]bool) bool {
	identifier, ok := expression.(*ast.Ident)
	return ok && aliases[identifier.Name]
}
