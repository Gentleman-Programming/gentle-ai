package sddstatus

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestSDDStatusReportsWhatAcquireReturnsForAnAcquiredAttempt reproduces #2463's
// exact sequence: acquire returns proceed and hands back a token, and status is
// queried before any executor launches. The token the caller was just handed is
// what status used to report as an active blocker, telling it to "settle only
// the external execution already associated with that token" for an execution
// that was never launched, with nextRecommended resolve-blockers and every
// dependency blocked.
//
// The ratified contract (internal/assets/skills/_shared/sdd-status-contract.md
// lines 20, 21 and 142) already makes compact acquire the launch authority and
// full runtime status a diagnostic. Status therefore has no standing to refuse
// a launch that acquire admitted; when an attempt is active it reports the
// continuation acquire itself names (--token <live token>) and leaves routing
// to the phase the artifacts select.
func TestSDDStatusReportsWhatAcquireReturnsForAnAcquiredAttempt(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	seedReadyChange(t, repo, "acquired-not-launched", "- [ ] 1.1 Work\n")
	store := mustRuntimeStore(t, repo, "acquired-not-launched")

	acquired, err := store.Acquire(context.Background(), CompactAcquireRequest{
		BeginAttemptRequest: BeginAttemptRequest{
			RequestID: "acquire-before-launch", WorkUnit: "apply-license",
			EvidenceGoal: "prove the license work unit", MaxAttempts: 1, MaxChangedLines: 800,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if acquired.State != CompactStateProceed || acquired.Token == "" {
		t.Fatalf("acquire = %#v, want proceed with a token", acquired)
	}

	status, err := Resolve(ResolveOptions{CWD: repo, ChangeName: "acquired-not-launched", IncludeInstructions: true})
	if err != nil {
		t.Fatal(err)
	}

	if status.NextRecommended == "resolve-blockers" {
		t.Fatalf("status stopped a caller that acquire admitted: next=%q blocked=%v", status.NextRecommended, status.BlockedReasons)
	}
	if status.Dependencies.Apply == DependencyBlocked {
		t.Fatalf("status blocked apply for an attempt acquire admitted: dependencies=%#v", status.Dependencies)
	}
	reasons := strings.Join(status.BlockedReasons, "\n")
	if strings.Contains(reasons, "settle only the external execution") {
		t.Fatalf("status still demands settlement of an execution that was never launched: %s", reasons)
	}
	instructions := strings.Join(status.PhaseInstructions.Apply, "\n")
	if !strings.Contains(instructions, "--token "+acquired.Token) {
		t.Fatalf("apply instructions omit the live attempt's own continuation:\n%s", instructions)
	}
}

// TestSDDStatusStillStopsForAMaintainerDecision is the converse guard: the one
// predicate must not turn a genuine maintainer-scope block into a launch. This
// case has no self-service exit, so it stays a hard stop.
func TestSDDStatusStillStopsForAMaintainerDecision(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	seedReadyChange(t, repo, "decision-required", "- [ ] 1.1 Work\n")
	store := mustRuntimeStore(t, repo, "decision-required")
	active, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "begin-decision", WorkUnit: "apply-auth",
		EvidenceGoal: "prove the auth runtime", MaxAttempts: 1, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: active.Revision, RequestID: "finish-decision", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('7'), Diagnosis: "bounded runtime reproduced the failure",
		HarnessDisposition: HarnessReused, CleanupEvidence: "runtime process group exited",
		ProcessEvidence: "post-run scan found no descendants",
	}); err != nil {
		t.Fatal(err)
	}

	status, err := Resolve(ResolveOptions{CWD: repo, ChangeName: "decision-required"})
	if err != nil {
		t.Fatal(err)
	}
	if status.NextRecommended != "resolve-blockers" || status.Dependencies.Apply != DependencyBlocked {
		t.Fatalf("maintainer decision stopped being a stop: next=%q dependencies=%#v", status.NextRecommended, status.Dependencies)
	}
	reasons := strings.Join(status.BlockedReasons, "\n")
	if !strings.Contains(reasons, "blocked(maintainer_decision)") {
		t.Fatalf("blocked reasons lost the compact reason code: %s", reasons)
	}
	// Status reports what acquire returns, so it also carries acquire's own
	// named exit rather than a hand-written paraphrase.
	if !strings.Contains(reasons, compactBlockedExitText(CompactBlockMaintainerDecision, "")) {
		t.Fatalf("blocked reasons did not carry the predicate's own exit: %s", reasons)
	}
}

// readinessTripleFields are the three RuntimeStatus fields that together answer
// "may this work proceed?". Every issue in this family (#2463, #2350, #2182)
// is one consumer reading them and reaching a different conclusion than
// another consumer reading the same bytes.
var readinessTripleFields = map[string]bool{
	"Complete":         true,
	"DecisionRequired": true,
	"ActiveAttempt":    true,
}

// readinessDecisionFiles are the two files that answer the readiness question
// for a CALLER. runtime_ledger.go is deliberately excluded: its handlers
// re-check the same fields as mutation preconditions inside the lock, which is
// a different question asked at a different layer.
var readinessDecisionFiles = []string{"runtime_compact.go", "status.go"}

// readinessTripleReaders names every function permitted to read the triple in
// those files. It is a closed set of exactly one entry on purpose: the point of
// this change is that "may this work proceed?" has one answer, derived once.
//
// Enumerating the reachable states in a test would recreate the drift one layer
// up, which is the trap this package's artifact-map collapse (#2346) avoided in
// the same way. This walks the package for rival producers instead, so a fourth
// derivation added later fails here rather than in a user's terminal.
var readinessTripleReaders = map[string]bool{
	"runtimeReadiness": true,
}

func TestOneReadinessPredicateHasNoRivalDerivations(t *testing.T) {
	fileSet := token.NewFileSet()
	for _, name := range readinessDecisionFiles {
		parsed, err := parser.ParseFile(fileSet, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil || readinessTripleReaders[function.Name.Name] {
				continue
			}
			runtimeValues := runtimeStatusIdentifiers(function)
			ast.Inspect(function.Body, func(node ast.Node) bool {
				selector, ok := node.(*ast.SelectorExpr)
				if !ok || !readinessTripleFields[selector.Sel.Name] || !isRuntimeStatusExpression(selector.X, runtimeValues) {
					return true
				}
				t.Errorf(
					"%s: %s reads the readiness field %q on a RuntimeStatus; only runtimeReadiness may decide whether work proceeds",
					fileSet.Position(selector.Pos()), function.Name.Name, selector.Sel.Name,
				)
				return true
			})
		}
	}
}

// runtimeStatusIdentifiers collects the local names bound to a RuntimeStatus in
// one function: its parameters and results, plus short variable declarations
// whose right-hand side is already a RuntimeStatus expression. That is enough
// to tell `runtimeStatus.Complete` from `remediationState.Complete` without
// dragging a type checker into a test.
func runtimeStatusIdentifiers(function *ast.FuncDecl) map[string]bool {
	bound := map[string]bool{}
	declaresRuntimeStatus := func(fieldType ast.Expr) bool {
		if star, ok := fieldType.(*ast.StarExpr); ok {
			fieldType = star.X
		}
		identifier, ok := fieldType.(*ast.Ident)
		return ok && identifier.Name == "RuntimeStatus"
	}
	for _, list := range []*ast.FieldList{function.Type.Params, function.Type.Results} {
		if list == nil {
			continue
		}
		for _, field := range list.List {
			if !declaresRuntimeStatus(field.Type) {
				continue
			}
			for _, name := range field.Names {
				bound[name.Name] = true
			}
		}
	}
	ast.Inspect(function.Body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != len(assignment.Rhs) {
			return true
		}
		for index, right := range assignment.Rhs {
			left, ok := assignment.Lhs[index].(*ast.Ident)
			if ok && isRuntimeStatusExpression(right, bound) {
				bound[left.Name] = true
			}
		}
		return true
	})
	return bound
}

// isRuntimeStatusExpression reports whether an expression denotes a
// RuntimeStatus: a bound local name, or any `X.Status` / `X.RuntimeStatus`
// selector, which is how the replay and the SDD status document both expose it.
func isRuntimeStatusExpression(expression ast.Expr, bound map[string]bool) bool {
	switch node := expression.(type) {
	case *ast.Ident:
		return bound[node.Name]
	case *ast.StarExpr:
		return isRuntimeStatusExpression(node.X, bound)
	case *ast.SelectorExpr:
		return node.Sel.Name == "Status" || node.Sel.Name == "RuntimeStatus"
	}
	return false
}

// TestMaintainerDecisionExitNamesOnlyTheAdmittedOperation is #2530. The
// maintainer-decision block told the caller to "rescope or reset", and
// runtimeObjectiveRescopeStructurallyPermitted returns false whenever
// DecisionRequired is set, which is the only way this block is reached. Half
// that advice could never run, and a wrong exit is worse than a missing one:
// a dead end says stop, while advice that cannot work sends the caller in
// circles blaming themselves.
//
// Both operations are executed against the real blocked state rather than
// matched as strings, because the defect was precisely that the text and the
// admissibility rules disagreed.
func TestMaintainerDecisionExitNamesOnlyTheAdmittedOperation(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	seedReadyChange(t, repo, "decision-exit", "- [ ] 1.1 Work\n")
	store := mustRuntimeStore(t, repo, "decision-exit")
	active, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "begin-decision-exit", WorkUnit: "apply-auth",
		EvidenceGoal: "prove the auth runtime", MaxAttempts: 1, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	blocked, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: active.Revision, RequestID: "finish-decision-exit", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('8'), Diagnosis: "bounded runtime reproduced the failure",
		HarnessDisposition: HarnessReused, CleanupEvidence: "runtime process group exited",
		ProcessEvidence: "post-run scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !blocked.DecisionRequired {
		t.Fatalf("fixture did not reach the maintainer-decision state: %#v", blocked)
	}

	exit := compactBlockedExitText(CompactBlockMaintainerDecision, "")
	if strings.Contains(exit, "rescope") {
		t.Fatalf("maintainer-decision exit still names rescope, which this state refuses: %s", exit)
	}
	if !strings.Contains(exit, "reset the objective") {
		t.Fatalf("maintainer-decision exit stopped naming reset: %s", exit)
	}

	// The advice it dropped is genuinely refused here.
	if _, err := store.Rescope(context.Background(), RescopeObjectiveRequest{
		ExpectedRevision: blocked.Revision, RequestID: "decision-exit-rescope", WorkUnit: "narrower-auth",
		EvidenceGoal: "prove a narrower auth runtime", MaxAttempts: 1, MaxChangedLines: 20,
		Reason: "attempted the exit the old message named", Actor: "maintainer",
	}); !errors.Is(err, ErrRuntimeRescopeNotAllowed) {
		t.Fatalf("rescope at decision-required error = %v, want ErrRuntimeRescopeNotAllowed", err)
	}
	// The advice it kept genuinely runs.
	if _, err := store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: blocked.Revision, RequestID: "decision-exit-reset",
		Reason: "maintainer decided the budget is spent", Actor: "maintainer",
	}); err != nil {
		t.Fatalf("the reset this exit names was refused: %v", err)
	}
}
