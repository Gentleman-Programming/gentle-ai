package cli

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// reviewPrintedIdentityRegexp captures the runtime identity any command this
// CLI prints tells the reader to declare.
var reviewPrintedIdentityRegexp = regexp.MustCompile(`--agent +([A-Za-z0-9._<>-]+)`)

func compiledRuntimeIdentities() map[string]bool {
	identities := map[string]bool{}
	for _, agent := range catalog.AllAgents() {
		identities[string(agent.ID)] = true
	}
	return identities
}

// initLensSelectingReviewCLIRepo builds a repository whose committed base diff
// is large enough to select at least one lens, which is the precondition for
// the direct-route refusal that names a negotiated `review start` continuation.
func initLensSelectingReviewCLIRepo(t *testing.T) (repo, baseRef string) {
	t.Helper()

	repo = initReviewCLIRepo(t)
	baseRef = strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))

	body := strings.Repeat("added line for lens selection\n", 400)
	if err := os.WriteFile(filepath.Join(repo, "candidate.txt"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "candidate.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "candidate")
	return repo, baseRef
}

// TestDirectReviewStartRefusalInventsNoRuntimeIdentity is the RED-first proof
// for issue #2885, reproduced against the real CLI.
//
// The direct route refuses `--agent` outright ("review start --agent requires
// a negotiated --contract", verified by execution below), so on this path no
// runtime has declared itself and the CLI does not know who is calling. It
// must therefore omit --agent entirely. The test runs the exact printed command
// from the repository where it was emitted so executability is proved without
// filling a placeholder or guessing a transport identity.
func TestDirectReviewStartRefusalInventsNoRuntimeIdentity(t *testing.T) {
	reviewEnabledHome(t)
	repo, baseRef := initLensSelectingReviewCLIRepo(t)

	var stdout bytes.Buffer
	err := RunReviewFacadeStart([]string{
		"--cwd", repo,
		"--base-ref", baseRef,
		"--committed-only",
	}, &stdout)
	if err == nil {
		t.Fatalf("direct review start unexpectedly succeeded; stdout=%s", stdout.String())
	}
	message := err.Error()
	if !strings.Contains(message, "cannot produce a completable review") {
		t.Fatalf("did not reach the direct-route refusal that names a continuation: %s", message)
	}

	opening := strings.Index(message, "`gentle-ai review start ")
	if opening < 0 {
		t.Fatalf("refusal has no negotiated start command: %s", message)
	}
	closing := strings.IndexByte(message[opening+1:], '`')
	if closing < 0 {
		t.Fatalf("refusal command has no closing backtick: %s", message)
	}
	command := message[opening+1 : opening+1+closing]
	words := reviewShellWords(t, command)
	if strings.Contains(command, "--agent") || strings.Contains(command, reviewUndeclaredRuntimeIdentitySlot) {
		t.Fatalf("unbound recovery command guesses a runtime identity: %s", command)
	}
	if len(words) < 3 || words[0] != "gentle-ai" || words[1] != "review" || words[2] != "start" {
		t.Fatalf("recovery command = %#v, want gentle-ai review start", words)
	}
	t.Chdir(repo)
	var recovered bytes.Buffer
	if err := RunReview(words[2:], &recovered); err != nil {
		t.Fatalf("exact unbound recovery command is not executable: %v\n%s", err, recovered.String())
	}
}

// TestDirectRouteStillRefusesADeclaredRuntime pins the precondition the test
// above depends on. If the direct route ever starts accepting `--agent`, the
// undeclared-caller reasoning stops being the whole story and the printed
// command must echo the declared identity instead.
func TestDirectRouteStillRefusesADeclaredRuntime(t *testing.T) {
	repo, baseRef := initLensSelectingReviewCLIRepo(t)

	for _, agent := range []model.AgentID{model.AgentCodex, model.AgentOpenCode} {
		t.Run(string(agent), func(t *testing.T) {
			var stdout bytes.Buffer
			err := RunReviewFacadeStart([]string{
				"--cwd", repo,
				"--base-ref", baseRef,
				"--committed-only",
				"--agent", string(agent),
			}, &stdout)
			if err == nil {
				t.Fatalf("direct review start with --agent unexpectedly succeeded; stdout=%s", stdout.String())
			}
			if !strings.Contains(err.Error(), "--agent requires a negotiated --contract") {
				t.Fatalf("direct route no longer refuses a declared runtime: %v", err)
			}
		})
	}
}

// TestNegotiatedStartCommandEchoesTheCallersOwnRuntime pins the builder every
// printed continuation goes through. The negotiated hint sites pass the
// caller's declared identity. Every runtime with immutable transport must be
// echoed exactly; a fixed identity would lie for the rest of the supported
// set, which is why this property has its own test.
func TestNegotiatedStartCommandEchoesTheCallersOwnRuntime(t *testing.T) {
	t.Parallel()

	snapshot := reviewtransaction.Snapshot{
		Identity:   "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		Projection: reviewtransaction.ProjectionWorkspace,
	}
	for _, agent := range []model.AgentID{model.AgentClaudeCode, model.AgentOpenCode, model.AgentCodex} {
		t.Run(string(agent), func(t *testing.T) {
			command := reviewNegotiatedStartCommand(snapshot, string(agent))
			if strings.Count(command, "--agent ") != 1 || !strings.Contains(command, "--agent "+string(agent)+" ") {
				t.Errorf("negotiated start command does not contain exactly one exact caller identity %q: %s", agent, command)
			}
		})
	}

	if unbound := reviewNegotiatedStartCommand(snapshot, "   "); strings.Contains(unbound, "--agent") || strings.Contains(unbound, reviewUndeclaredRuntimeIdentitySlot) {
		t.Errorf("a blank runtime identity emitted an agent segment: %s", unbound)
	}
}

// TestTierCRecoveryStatementsCarryOnlyTheIdentitySlot keeps the issue #2440
// property on the registered Tier C statements themselves: they never bake a
// compiled runtime identity into a printed command, only the neutral
// placeholder slot. The old runtime-binding renderer was removed along with
// all Tier C stderr emission (successful negotiated operations are byte-silent
// on stderr), so the registry data is the surface that remains to guard.
func TestTierCRecoveryStatementsCarryOnlyTheIdentitySlot(t *testing.T) {
	t.Parallel()

	for _, reason := range []string{"corrected_candidate_unavailable"} {
		emission, ok := reviewNarrationRegistry["stop:"+reason]
		if !ok {
			t.Fatalf("Tier C reason %q has no narration", reason)
		}
		if !strings.Contains(emission.Statement, "--agent "+reviewUndeclaredRuntimeIdentitySlot) {
			t.Fatalf("Tier C statement lost the neutral runtime-identity slot: %s", emission.Statement)
		}
		for _, identity := range []model.AgentID{model.AgentClaudeCode, model.AgentOpenCode, model.AgentCodex} {
			if strings.Contains(emission.Statement, "--agent "+string(identity)) {
				t.Fatalf("Tier C statement baked in compiled identity %q: %s", identity, emission.Statement)
			}
		}
	}
}

// TestNoGoSourceBindsALiteralRuntimeIdentityIntoPrintedCommands is the
// source-level half of the issue #2440 guard, and it is the half an
// assets-only check misses: reviewNegotiatedStartCommand's identity was a Go
// constant compiled into the binary, never an asset, so no amount of scanning
// rendered instructions would have found it.
//
// It walks every production Go file in the repository and refuses two shapes:
// a string literal that spells out `--agent <compiled-identity>`, and a
// `--agent %…` format slot filled from a compiled runtime constant in the same
// call. Both produce user-visible guidance that asserts an identity the code
// did not learn from the caller.
func TestNoGoSourceBindsALiteralRuntimeIdentityIntoPrintedCommands(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	identities := compiledRuntimeIdentities()

	scanned := 0
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "testdata", "node_modules", "openspec", "docs":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		scanned++

		violations, analysisErr := printedRuntimeIdentityViolations(relative, string(source), identities)
		if analysisErr != nil {
			return nil // Generated or build-tagged sources this guard cannot read are out of scope.
		}
		for _, violation := range violations {
			t.Error(violation)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}
	if scanned == 0 {
		t.Fatal("walked the repository and parsed no production Go files; this guard would pass vacuously")
	}
}

// printedRuntimeIdentityViolations analyses one Go source file and returns one
// message per site that binds a compiled runtime identity into user-visible
// guidance. Extracted from the walk so the exact pre-fix shape can be pinned as
// a fixture, proving this guard would have caught the shipped defect rather
// than merely agreeing with the code as it stands today.
func printedRuntimeIdentityViolations(fileLabel, source string, identities map[string]bool) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, fileLabel, source, 0)
	if err != nil {
		return nil, err
	}
	var violations []string
	ast.Inspect(file, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.BasicLit:
			value, ok := goStringLiteral(typed)
			if !ok {
				return true
			}
			for _, match := range reviewPrintedIdentityRegexp.FindAllStringSubmatch(value, -1) {
				if identities[match[1]] {
					violations = append(violations, fmt.Sprintf("%s:%d spells out `--agent %s` in a printed string; only the caller can state which runtime is executing (issue #2440)",
						fileLabel, fset.Position(typed.Pos()).Line, match[1]))
				}
			}
		case *ast.CallExpr:
			if !callFormatsAnAgentFlag(typed) {
				return true
			}
			for _, argument := range typed.Args {
				if name, ok := compiledRuntimeIdentityExpression(argument, identities); ok {
					violations = append(violations, fmt.Sprintf("%s:%d fills a `--agent` format slot from the compiled constant %s; only the caller can state which runtime is executing (issue #2440)",
						fileLabel, fset.Position(argument.Pos()).Line, name))
				}
			}
		}
		return true
	})
	return violations, nil
}

// TestPrintedRuntimeIdentityGuardCatchesTheShippedDefect feeds the guard the
// two exact shapes that shipped in v2.3.0-rc.1. Without this, the guard could
// only ever prove that today's source agrees with itself.
func TestPrintedRuntimeIdentityGuardCatchesTheShippedDefect(t *testing.T) {
	t.Parallel()

	identities := compiledRuntimeIdentities()
	for name, source := range map[string]string{
		// internal/cli/review_facade.go before the fix: the identity is a
		// compiled constant filling the format slot, invisible to any check
		// that only reads rendered assets or scans string literals.
		"compiled constant in a format slot": `package cli

func reviewNegotiatedStartCommand(snapshot Snapshot) string {
	return fmt.Sprintf("gentle-ai review start --contract %s --agent %s --target %s --projection %s",
		ReviewIntegrationContractV2, model.AgentClaudeCode, snapshot.Identity, facadeProjection(snapshot.Projection))
}
`,
		// internal/cli/review_narration.go before the fix: the identity is
		// spelled out inside the printed statement itself.
		"literal identity in a printed statement": `package cli

var reviewStopReasonNarration = map[string]string{
	"corrected_candidate_unavailable": "Change the candidate content, then re-run " +
		"` + "`" + `gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --agent claude-code --next-transition` + "`" + `.",
}
`,
	} {
		t.Run(name, func(t *testing.T) {
			violations, err := printedRuntimeIdentityViolations("fixture.go", source, identities)
			if err != nil {
				t.Fatalf("parse fixture: %v", err)
			}
			if len(violations) == 0 {
				t.Fatal("the guard did not flag a shape that shipped as issue #2440")
			}
		})
	}
}

func goStringLiteral(literal *ast.BasicLit) (string, bool) {
	if literal.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(literal.Value)
	if err != nil {
		return "", false
	}
	return value, true
}

// callFormatsAnAgentFlag reports whether any argument of the call is a format
// string whose `--agent` value comes from a slot rather than from literal text.
func callFormatsAnAgentFlag(call *ast.CallExpr) bool {
	for _, argument := range call.Args {
		literal, ok := argument.(*ast.BasicLit)
		if !ok {
			continue
		}
		value, ok := goStringLiteral(literal)
		if !ok {
			continue
		}
		if strings.Contains(value, "--agent %") {
			return true
		}
	}
	return false
}

// compiledRuntimeIdentityExpression names the expression when it is a compiled
// runtime identity: either a `model.Agent…` selector or a string literal that
// equals one of the compiled identities.
func compiledRuntimeIdentityExpression(expression ast.Expr, identities map[string]bool) (string, bool) {
	switch typed := expression.(type) {
	case *ast.SelectorExpr:
		pkg, ok := typed.X.(*ast.Ident)
		if ok && pkg.Name == "model" && strings.HasPrefix(typed.Sel.Name, "Agent") {
			return pkg.Name + "." + typed.Sel.Name, true
		}
	case *ast.Ident:
		if strings.HasPrefix(typed.Name, "Agent") {
			return typed.Name, true
		}
	case *ast.BasicLit:
		value, ok := goStringLiteral(typed)
		if ok && identities[value] {
			return strconv.Quote(value), true
		}
	case *ast.CallExpr:
		if len(typed.Args) == 1 {
			return compiledRuntimeIdentityExpression(typed.Args[0], identities)
		}
	}
	return "", false
}

// withoutReplayRuntimeIdentity verifies that an unbound continuation is already
// executable while retaining every frozen selector the direct route emitted.
func withoutReplayRuntimeIdentity(t *testing.T, arguments []string) []string {
	t.Helper()

	for _, argument := range arguments {
		if argument == "--agent" || strings.Contains(argument, reviewUndeclaredRuntimeIdentitySlot) {
			t.Fatalf("unbound printed command contains a runtime identity segment: %v", arguments)
		}
	}
	return append([]string(nil), arguments...)
}
