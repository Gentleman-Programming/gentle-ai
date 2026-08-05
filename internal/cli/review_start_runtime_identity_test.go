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
// for the compiled half of issue #2440, reproduced against the real CLI.
//
// The direct route refuses `--agent` outright ("review start --agent requires
// a negotiated --contract", verified by execution below), so on this path no
// runtime has declared itself and the CLI does not know who is calling. It
// nonetheless printed an exact runnable continuation carrying a constant
// `--agent claude-code`, at the exact moment a reader is most likely to copy
// the command verbatim. A Codex or OpenCode caller who did so declared a false
// identity and passed the transport admission check in
// review_transport_capability.go under Claude Code's capability profile.
func TestDirectReviewStartRefusalInventsNoRuntimeIdentity(t *testing.T) {
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

	identities := compiledRuntimeIdentities()
	named := reviewPrintedIdentityRegexp.FindAllStringSubmatch(message, -1)
	if len(named) == 0 {
		t.Fatalf("printed continuation names no --agent binding at all: %s", message)
	}
	for _, match := range named {
		if identities[match[1]] {
			t.Errorf("printed continuation tells an undeclared caller to declare %q; copying this command verbatim passes a false identity to the review transport gate: %s", match[1], message)
		}
	}
	if !strings.Contains(message, "--agent "+reviewUndeclaredRuntimeIdentitySlot) {
		t.Errorf("printed continuation does not leave the runtime identity to be filled in: %s", message)
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
// caller's declared identity, and today only claude-code clears the transport
// gate, so a constant and an echo are observationally identical on this build.
// That is exactly why the property needs its own test: the moment a second
// runtime becomes eligible, a constant would start lying again.
func TestNegotiatedStartCommandEchoesTheCallersOwnRuntime(t *testing.T) {
	t.Parallel()

	snapshot := reviewtransaction.Snapshot{
		Identity:   "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		Projection: reviewtransaction.ProjectionWorkspace,
	}
	for _, agent := range []model.AgentID{model.AgentClaudeCode, model.AgentCodex, model.AgentOpenCode, model.AgentKilocode} {
		t.Run(string(agent), func(t *testing.T) {
			command := reviewNegotiatedStartCommand(snapshot, string(agent))
			if !strings.Contains(command, "--agent "+string(agent)+" ") {
				t.Errorf("negotiated start command does not name the caller %q: %s", agent, command)
			}
			for other := range compiledRuntimeIdentities() {
				if other == string(agent) {
					continue
				}
				if strings.Contains(command, "--agent "+other+" ") {
					t.Errorf("negotiated start command for %q names %q instead: %s", agent, other, command)
				}
			}
		})
	}

	if slot := reviewNegotiatedStartCommand(snapshot, "   "); !strings.Contains(slot, "--agent "+reviewUndeclaredRuntimeIdentitySlot+" ") {
		t.Errorf("a blank runtime identity did not fall back to the fill-in slot: %s", slot)
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

// substituteReplayRuntimeIdentity prepares a printed continuation for verbatim
// replay by supplying the one token the CLI could not know: the caller's own
// runtime identity.
//
// This is the honest resolution of a real tension between two contracts. Issue
// #2447 requires every refusal to name a command that actually runs. Issue
// #2440 requires that no printed command assert an identity the CLI never
// learned from the caller, because the direct route refuses `--agent` and a
// reader who copies a constant declares a false runtime to the transport gate.
// Both cannot hold in one fixed string, so the printed command leaves exactly
// one slot and this helper fills it the way a human would. The replay still
// proves everything #2447 cares about: the helper fails if ANY other token is
// a fill-in slot, so a hint that quietly stopped resolving the frozen selector
// would be caught here, not excused.
func substituteReplayRuntimeIdentity(t *testing.T, arguments []string, caller model.AgentID) []string {
	t.Helper()

	substituted := make([]string, len(arguments))
	filled := false
	for index, argument := range arguments {
		if argument == reviewUndeclaredRuntimeIdentitySlot {
			substituted[index] = string(caller)
			filled = true
			continue
		}
		if strings.HasPrefix(argument, "<") && strings.HasSuffix(argument, ">") {
			t.Fatalf("printed command leaves %q unresolved; only the caller's own runtime identity may need filling in", argument)
		}
		substituted[index] = argument
	}
	if !filled {
		t.Fatalf("printed command carried no runtime-identity slot to fill: %v", arguments)
	}
	return substituted
}
