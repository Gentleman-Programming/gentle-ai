package reviewtransaction

import (
	"context"
	"strings"
	"testing"
)

// coverageCorrectionFixture freezes the exact lineage shape issue #3375
// reports: a small candidate over one reviewed production file, whose only
// blocking finding is a CRITICAL saying the changed behaviour has no test
// able to execute it. The returned state sits in correction_required with a
// forecast already open, so a caller only has to write the correction and
// build its fix-diff snapshot.
func coverageCorrectionFixture(t *testing.T, repo, lineage string, forecast int) CompactState {
	t.Helper()
	writeSnapshotFile(t, repo, "internal/widget/widget.go", "package widget\n\nfunc Widget() string {\n\treturn \"posix\"\n}\n")
	gitSnapshot(t, repo, "add", "--", "internal/widget/widget.go")
	gitSnapshot(t, repo, "commit", "-m", "widget")
	// Six changed lines: the issue's small candidate, and the exact shape of
	// the observed defect (a platform-specific branch nothing can execute).
	writeSnapshotFile(t, repo, "internal/widget/widget.go",
		"package widget\n\nimport \"runtime\"\n\nfunc Widget() string {\n\tif runtime.GOOS == \"windows\" {\n\t\treturn \"windows\"\n\t}\n\treturn \"posix\"\n}\n")
	state := newCompactTestState(t, repo, lineage)
	if !equalStrings(state.GenesisPaths, []string{"internal/widget/widget.go"}) {
		t.Fatalf("fixture genesis scope = %v, want the single reviewed production file", state.GenesisPaths)
	}
	if len(state.SelectedLenses) == 0 {
		t.Fatalf("fixture risk %q selected no lens, so it cannot carry a blocking finding", state.RiskLevel)
	}
	finding := Finding{
		ID: "R3-001", Lens: state.SelectedLenses[0], Location: "internal/widget/widget.go:6", Severity: "CRITICAL",
		Claim:     "the new windows branch has no test able to execute it",
		ProofRefs: []string{"no test file in internal/widget exercises the windows branch"},
	}
	results := make([]LensResult, 0, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		result := LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"inspected the frozen candidate"}}
		if index == 0 {
			result.Findings = []Finding{finding}
		}
		results = append(results, result)
	}
	if err := state.CompleteReview(CompactReviewInput{
		LensResults: results,
		Classifications: []FindingEvidence{{
			FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced,
			Proof: "the changed hunk introduces the untested branch",
		}},
		RefuterOutcomes: []EvidenceResult{},
	}); err != nil {
		t.Fatalf("CompleteReview: %v", err)
	}
	if state.State != StateCorrectionRequired {
		t.Fatalf("fixture state = %q, want correction_required", state.State)
	}
	if err := state.BeginCorrection(forecast); err != nil {
		t.Fatalf("BeginCorrection(%d): %v", forecast, err)
	}
	return state
}

// buildCoverageFix stages the correction's files and freezes the fix-diff
// snapshot bound to the reviewed candidate.
func buildCoverageFix(t *testing.T, repo string, state CompactState, paths ...string) Snapshot {
	t.Helper()
	gitSnapshot(t, repo, append([]string{"add", "--"}, paths...)...)
	fix, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetFixDiff, BaseRef: state.CurrentSnapshot.CandidateTree,
		IntendedUntracked: state.InitialSnapshot.IntendedUntracked, LedgerIDs: state.FixFindingIDs,
	})
	if err != nil {
		t.Fatalf("build fix-diff snapshot: %v", err)
	}
	return fix
}

func passingCoverageValidation(state CompactState, fix Snapshot) ScopedValidationResult {
	fixHash := FixDeltaHashForSnapshot(fix)
	return bindTargetedValidationForTest(ScopedValidationResult{
		LedgerIDs: state.FixFindingIDs, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{},
		OriginalCriteria:     ValidationCheck{EvidenceHash: hash("2"), FixDeltaHash: fixHash, Passed: true},
		CorrectionRegression: ValidationCheck{EvidenceHash: hash("3"), FixDeltaHash: fixHash, Passed: true},
	}, fix)
}

// TestCompactCorrectionAdmitsCompanionTestPathForCoverageFinding is the
// behaviour-first reproduction for issue #3375. The single bounded correction
// the lifecycle grants must be able to satisfy the most common class of
// blocking finding: "this behaviour has no test coverage", whose only remedy
// is a new test file. The correction here stays well inside the frozen line
// budget and adds exactly one companion test file for the reviewed
// production path, so it must complete instead of forcing a scope_changed
// recovery plus a second full review.
func TestCompactCorrectionAdmitsCompanionTestPathForCoverageFinding(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	state := coverageCorrectionFixture(t, repo, "coverage-correction", 3)
	budget := state.CorrectionBudget
	writeSnapshotFile(t, repo, "internal/widget/widget_windows_test.go", "package widget\n\nimport \"testing\"\n")
	fix := buildCoverageFix(t, repo, state, "internal/widget/widget_windows_test.go")
	if equalStrings(fix.Paths, state.GenesisPaths) {
		t.Fatalf("fixture correction did not add a path: %v", fix.Paths)
	}
	actual := 3
	if actual > budget {
		t.Fatalf("fixture correction of %d lines does not fit the frozen budget of %d", actual, budget)
	}
	if err := state.CompleteCorrection(fix, actual, passingCoverageValidation(state, fix)); err != nil {
		t.Fatalf("bounded correction adding a companion test file for a coverage finding must complete: %v", err)
	}
	if state.State != StateValidating {
		t.Fatalf("state after admitted correction = %q, want validating", state.State)
	}
	if !equalStrings(state.CorrectionAddedPaths, []string{"internal/widget/widget_windows_test.go"}) {
		t.Fatalf("CorrectionAddedPaths = %v, want the one added companion test path", state.CorrectionAddedPaths)
	}
	if !equalStrings(state.GenesisPaths, []string{"internal/widget/widget.go"}) {
		t.Fatalf("genesis scope must stay frozen, got %v", state.GenesisPaths)
	}
	if state.CorrectionBudget != budget || state.CumulativeCorrectionLines != actual {
		t.Fatalf("budget accounting = %d/%d, want %d/%d", state.CumulativeCorrectionLines, state.CorrectionBudget, actual, budget)
	}
	if err := state.Validate(); err != nil {
		t.Fatalf("validate admitted correction state: %v", err)
	}
	// The single-correction rule still holds.
	if !state.CorrectionAttemptConsumed() {
		t.Fatal("an admitted path-adding correction must still consume the one correction attempt")
	}
	// The authority round-trips through its store byte-stably.
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	writeCompactFixtureRecord(t, store, state)
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !equalStrings(loaded.State.CorrectionAddedPaths, state.CorrectionAddedPaths) {
		t.Fatalf("persisted CorrectionAddedPaths = %v, want %v", loaded.State.CorrectionAddedPaths, state.CorrectionAddedPaths)
	}
}

// TestCompactCorrectionRefusesAddedProductionPath proves the widened scope is
// not a hole: a correction may not introduce a production source file that no
// lens ever inspected, even when it fits the budget.
func TestCompactCorrectionRefusesAddedProductionPath(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	state := coverageCorrectionFixture(t, repo, "coverage-production", 3)
	writeSnapshotFile(t, repo, "internal/widget/helper.go", "package widget\n\nfunc helper() {}\n")
	fix := buildCoverageFix(t, repo, state, "internal/widget/helper.go")
	before := state
	err := state.CompleteCorrection(fix, 3, passingCoverageValidation(state, fix))
	if err == nil || !strings.Contains(err.Error(), "internal/widget/helper.go") {
		t.Fatalf("CompleteCorrection(added production path) = %v, want a refusal naming the path", err)
	}
	if state.State != before.State || len(state.CorrectionAttempts) != 0 || len(state.CorrectionAddedPaths) != 0 {
		t.Fatalf("a refused correction must not mutate the authority: %q", state.State)
	}
}

// TestCompactCorrectionRefusesUnrelatedTestPath proves the addition must be a
// companion of an already-reviewed path. A test file dropped somewhere else
// in the tree is not evidence about the reviewed candidate.
func TestCompactCorrectionRefusesUnrelatedTestPath(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	state := coverageCorrectionFixture(t, repo, "coverage-unrelated", 3)
	writeSnapshotFile(t, repo, "cmd/other/main_test.go", "package main\n\nimport \"testing\"\n")
	fix := buildCoverageFix(t, repo, state, "cmd/other/main_test.go")
	if err := state.CompleteCorrection(fix, 3, passingCoverageValidation(state, fix)); err == nil ||
		!strings.Contains(err.Error(), "cmd/other/main_test.go") {
		t.Fatalf("CompleteCorrection(unrelated test path) = %v, want a refusal naming the path", err)
	}
}

// TestCompactCorrectionAddedPathStillBindsLineBudget proves an admitted
// path-adding correction must fit the frozen line budget before it mutates the
// authority; the added file's lines are correction lines like any other.
func TestCompactCorrectionAddedPathStillBindsLineBudget(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	state := coverageCorrectionFixture(t, repo, "coverage-budget", 1)
	budget, before := state.CorrectionBudget, state
	writeSnapshotFile(t, repo, "internal/widget/widget_windows_test.go", "package widget\n\nimport \"testing\"\n")
	fix := buildCoverageFix(t, repo, state, "internal/widget/widget_windows_test.go")
	err := state.CompleteCorrection(fix, budget+1, passingCoverageValidation(state, fix))
	if err == nil || !strings.Contains(err.Error(), "exceeding the frozen budget") {
		t.Fatalf("CompleteCorrection(over budget) = %v, want frozen-budget refusal", err)
	}
	if state.State != before.State || len(state.CorrectionAttempts) != len(before.CorrectionAttempts) ||
		!equalStrings(state.CorrectionScopePaths(), before.CorrectionScopePaths()) {
		t.Fatalf("over-budget path addition mutated authority: %#v", state)
	}
}

// TestCompactCorrectionAddedPathIsDisclosedInReceipt proves an added path that
// no lens ever saw is disclosed rather than hidden: the terminal receipt names
// it, and its paths digest covers the delivered scope instead of claiming the
// narrower reviewed one.
func TestCompactCorrectionAddedPathIsDisclosedInReceipt(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	state := coverageCorrectionFixture(t, repo, "coverage-receipt", 3)
	writeSnapshotFile(t, repo, "internal/widget/widget_windows_test.go", "package widget\n\nimport \"testing\"\n")
	fix := buildCoverageFix(t, repo, state, "internal/widget/widget_windows_test.go")
	if err := state.CompleteCorrection(fix, 3, passingCoverageValidation(state, fix)); err != nil {
		t.Fatalf("CompleteCorrection: %v", err)
	}
	if err := state.CompleteVerification([]byte("go test ./internal/widget/ passes\n"), true); err != nil {
		t.Fatalf("CompleteVerification: %v", err)
	}
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatalf("Receipt: %v", err)
	}
	if !equalStrings(receipt.CorrectionAddedPaths, []string{"internal/widget/widget_windows_test.go"}) {
		t.Fatalf("receipt CorrectionAddedPaths = %v, want the disclosed added path", receipt.CorrectionAddedPaths)
	}
	want := digestPaths([]string{"internal/widget/widget.go", "internal/widget/widget_windows_test.go"})
	if receipt.PathsDigest != want {
		t.Fatalf("receipt paths digest = %s, want the delivered union %s", receipt.PathsDigest, want)
	}
	if err := receipt.Validate(); err != nil {
		t.Fatalf("validate disclosing receipt: %v", err)
	}
}

// TestCompactStateRejectsForgedCorrectionAddedPaths proves the new field
// cannot be used to widen an authority's scope without an actual admitted
// correction.
func TestCompactStateRejectsForgedCorrectionAddedPaths(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "internal/widget/widget.go", "package widget\n")
	gitSnapshot(t, repo, "add", "--", "internal/widget/widget.go")
	state := newCompactTestState(t, repo, "forged-added-paths")
	if len(state.GenesisPaths) == 0 {
		t.Fatal("fixture genesis scope is empty")
	}
	for _, tt := range []struct {
		name  string
		paths []string
	}{
		{name: "no correction at all", paths: []string{"tracked_test.txt"}},
		{name: "path already inside genesis", paths: state.GenesisPaths},
		{name: "non-canonical", paths: []string{"b_test.go", "a_test.go"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			forged := state
			forged.CorrectionAddedPaths = tt.paths
			if err := forged.Validate(); err == nil {
				t.Fatalf("forged CorrectionAddedPaths %v validated", tt.paths)
			}
		})
	}
}

// TestCorrectionAddedPathAdmission pins the closed convention table that
// decides which added paths a bounded correction may carry.
func TestCorrectionAddedPathAdmission(t *testing.T) {
	genesis := []string{"internal/widget/widget.go", "src/app/service.ts", "lib/calc.py"}
	for _, tt := range []struct {
		path string
		want bool
	}{
		{path: "internal/widget/widget_windows_test.go", want: true},
		{path: "internal/widget/testdata_test.go", want: true},
		{path: "internal/widget/tests/widget_test.go", want: true},
		{path: "internal/tests/widget_test.go", want: true},
		{path: "src/app/service.spec.ts", want: true},
		{path: "src/app/service.test.ts", want: true},
		{path: "lib/test_calc.py", want: true},
		{path: "lib/calc_test.py", want: true},
		// Not a test file.
		{path: "internal/widget/helper.go", want: false},
		{path: "internal/widget/latest.go", want: false},
		{path: "internal/widget/testable.go", want: false},
		{path: "internal/widget/protest.go", want: false},
		{path: "src/app/service.ts", want: false},
		// Compilable production source the old table admitted: the pytest
		// prefix on a Go file, and any name at all inside a directory called
		// tests. Both ship inside the ordinary package.
		{path: "internal/widget/test_helper.go", want: false},
		{path: "internal/widget/tests/helper.go", want: false},
		{path: "internal/tests/helper.go", want: false},
		{path: "src/app/__tests__/service.ts", want: false},
		{path: "lib/tests/helper.py", want: false},
		// An extension whose toolchain this table cannot decide from a path.
		{path: "lib/CalcTests.cs", want: false},
		{path: "internal/widget/tests/notes.md", want: false},
		// A test file, but no reviewed path it could be evidence about.
		{path: "cmd/other/main_test.go", want: false},
		{path: "internal/widget/deep/nested/widget_test.go", want: false},
	} {
		t.Run(tt.path, func(t *testing.T) {
			if got := correctionAddedPathAdmissible(tt.path, genesis); got != tt.want {
				t.Fatalf("correctionAddedPathAdmissible(%q) = %t, want %t", tt.path, got, tt.want)
			}
		})
	}
}

// TestCorrectionTestFileNameIsLanguageAware pins the property the whole guard
// rests on: a name counts as a test only under the discovery rule of the
// toolchain that owns that exact extension. A convention borrowed from another
// ecosystem is how compilable production source got in -- test_helper.go
// carries pytest's prefix and is compiled into the ordinary Go package, and
// _test.go is the only spelling the Go toolchain itself excludes.
func TestCorrectionTestFileNameIsLanguageAware(t *testing.T) {
	for _, tt := range []struct {
		base string
		want bool
	}{
		// Go: the compiler keys on the _test.go suffix and nothing else.
		{base: "widget_test.go", want: true},
		{base: "test_helper.go", want: false},
		{base: "TestHelper.go", want: false},
		{base: "helper_spec.go", want: false},
		{base: "_test.go", want: false},
		{base: "widget_Test.go", want: false},
		// Python: pytest discovers test_*.py and *_test.py.
		{base: "test_calc.py", want: true},
		{base: "calc_test.py", want: true},
		{base: "conftest.py", want: false},
		{base: "calc.spec.py", want: false},
		// The JS/TS family: the .test/.spec infix every runner defaults to.
		{base: "service.test.ts", want: true},
		{base: "service.spec.tsx", want: true},
		{base: "service.test.mjs", want: true},
		{base: "service-test.js", want: false},
		{base: "service.ts", want: false},
		// Ruby and Elixir.
		{base: "calc_test.rb", want: true},
		{base: "calc_spec.rb", want: true},
		{base: "calc_test.exs", want: true},
		{base: "calc_test.ex", want: false},
		// Extensions this table cannot decide from a name: the suffix says
		// test, the build system ships the file anyway.
		{base: "CalcTests.cs", want: false},
		{base: "WidgetTest.java", want: false},
		{base: "WidgetTest.kt", want: false},
		{base: "widget_test.rs", want: false},
		{base: "widget_test.txt", want: false},
		{base: "widget_test", want: false},
	} {
		t.Run(tt.base, func(t *testing.T) {
			if got := correctionTestFileName(tt.base); got != tt.want {
				t.Fatalf("correctionTestFileName(%q) = %t, want %t", tt.base, got, tt.want)
			}
		})
	}
}

// TestCompactCorrectionRefusesGoSourceWithAPytestPrefix is the behaviour-first
// reproduction of R1-correction-scope-test-prefix. internal/widget/test_helper.go
// is an ordinary member of package widget: the Go toolchain compiles it into
// the shipped binary, and only the _test.go SUFFIX excludes a file from that
// build. Admitting it as a "test file" ships production source no lens
// inspected inside an approved candidate.
func TestCompactCorrectionRefusesGoSourceWithAPytestPrefix(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	state := coverageCorrectionFixture(t, repo, "coverage-pytest-prefix", 3)
	writeSnapshotFile(t, repo, "internal/widget/test_helper.go",
		"package widget\n\n// Helper ships inside the ordinary package.\nfunc Helper() string {\n\treturn \"shipped\"\n}\n")
	fix := buildCoverageFix(t, repo, state, "internal/widget/test_helper.go")
	before := state
	err := state.CompleteCorrection(fix, 3, passingCoverageValidation(state, fix))
	if err == nil || !strings.Contains(err.Error(), "internal/widget/test_helper.go") {
		t.Fatalf("CompleteCorrection(compiled Go source named test_helper.go) = %v, want a refusal naming the path", err)
	}
	if state.State != before.State || len(state.CorrectionAttempts) != 0 || len(state.CorrectionAddedPaths) != 0 {
		t.Fatalf("a refused correction must not mutate the authority: %q", state.State)
	}
}

// TestCompactCorrectionRefusesCompiledSourceInATestDirectory is the
// behaviour-first reproduction of R1-correction-scope-test-directory. A Go
// directory named tests is an ordinary compiled package, so the file's own
// name has to decide; a reserved directory segment may never answer for it.
func TestCompactCorrectionRefusesCompiledSourceInATestDirectory(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	state := coverageCorrectionFixture(t, repo, "coverage-test-directory", 3)
	writeSnapshotFile(t, repo, "internal/widget/tests/helper.go",
		"package tests\n\n// Helper is compiled and importable from production code.\nfunc Helper() string {\n\treturn \"shipped\"\n}\n")
	fix := buildCoverageFix(t, repo, state, "internal/widget/tests/helper.go")
	before := state
	err := state.CompleteCorrection(fix, 3, passingCoverageValidation(state, fix))
	if err == nil || !strings.Contains(err.Error(), "internal/widget/tests/helper.go") {
		t.Fatalf("CompleteCorrection(compiled Go source under tests/) = %v, want a refusal naming the path", err)
	}
	if state.State != before.State || len(state.CorrectionAttempts) != 0 || len(state.CorrectionAddedPaths) != 0 {
		t.Fatalf("a refused correction must not mutate the authority: %q", state.State)
	}
}

// TestCompactCorrectionEscalationRollsBackTheWidenedScope proves a failed
// targeted check records an escalation without widening delivery scope, while
// an over-budget actual is refused before either attempt or scope can mutate.
func TestCompactCorrectionEscalationRollsBackTheWidenedScope(t *testing.T) {
	requireSnapshotGit(t)
	for _, tt := range []struct {
		name    string
		overrun int
		passing bool
	}{
		{name: "over-budget", overrun: 1, passing: true},
		{name: "failed-targeted-check", overrun: 0, passing: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			state := coverageCorrectionFixture(t, repo, "coverage-escalated-"+tt.name, 1)
			writeSnapshotFile(t, repo, "internal/widget/widget_windows_test.go", "package widget\n\nimport \"testing\"\n")
			fix := buildCoverageFix(t, repo, state, "internal/widget/widget_windows_test.go")
			validation := passingCoverageValidation(state, fix)
			actual := 1
			if tt.overrun > 0 {
				actual = state.CorrectionBudget + tt.overrun
			}
			if !tt.passing {
				validation.CorrectionRegression.Passed = false
			}
			beforeState, beforeAttempts, beforeScope := state.State, len(state.CorrectionAttempts), state.CorrectionScopePaths()
			err := state.CompleteCorrection(fix, actual, validation)
			if tt.overrun > 0 {
				if err == nil || !strings.Contains(err.Error(), "exceeding the frozen budget") {
					t.Fatalf("CompleteCorrection(over budget) = %v, want frozen-budget refusal", err)
				}
				if state.State != beforeState || len(state.CorrectionAttempts) != beforeAttempts || !equalStrings(state.CorrectionScopePaths(), beforeScope) {
					t.Fatalf("over-budget correction mutated authority: %#v", state)
				}
				return
			}
			if err != nil {
				t.Fatalf("CompleteCorrection: %v", err)
			}
			if state.State != StateEscalated {
				t.Fatalf("state after a failed path-adding correction = %q, want escalated", state.State)
			}
			if len(state.CorrectionAddedPaths) != 0 {
				t.Fatalf("a correction that did not complete kept its widened scope: %v", state.CorrectionAddedPaths)
			}
			if !equalStrings(state.CorrectionScopePaths(), state.GenesisPaths) {
				t.Fatalf("delivery scope after a failed correction = %v, want the frozen reviewed manifest %v",
					state.CorrectionScopePaths(), state.GenesisPaths)
			}
			if err := state.Validate(); err != nil {
				t.Fatalf("the escalated authority must still be a valid record of its attempt: %v", err)
			}
			if len(state.CorrectionAttempts) != 1 || !state.CorrectionAttemptConsumed() {
				t.Fatalf("a failed path-adding correction must still consume the one attempt: %d attempts", len(state.CorrectionAttempts))
			}
		})
	}
}

// TestCompactAddedPathDeliversThroughThePrePRGate is the gate-level proof for
// R3-widened-scope-no-gate-proof. Admitting a companion test path into the
// bounded correction is only half the feature. Every delivery gate re-proves
// the delivered paths against the scope the receipt authorizes, so a gate
// still measuring the frozen reviewed manifest refuses the very file the
// correction was granted -- the whole point of the change -- at the last step
// before it ships. The candidate here goes all the way through: correction,
// independent verification, receipt, published delivery, pre-PR gate.
func TestCompactAddedPathDeliversThroughThePrePRGate(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	state := coverageCorrectionFixture(t, repo, "coverage-gate", 3)
	branch := currentBranch(context.Background(), repo)
	configurePublicationRemote(t, repo, branch)
	gitSnapshot(t, repo, "config", "branch."+branch+".remote", "origin")
	gitSnapshot(t, repo, "config", "branch."+branch+".merge", "refs/heads/"+branch)
	writeSnapshotFile(t, repo, "internal/widget/widget_windows_test.go", "package widget\n\nimport \"testing\"\n")
	fix := buildCoverageFix(t, repo, state, "internal/widget/widget_windows_test.go")
	if err := state.CompleteCorrection(fix, 3, passingCoverageValidation(state, fix)); err != nil {
		t.Fatalf("CompleteCorrection: %v", err)
	}
	if err := state.CompleteVerification([]byte("go test ./internal/widget/ passes\n"), true); err != nil {
		t.Fatalf("CompleteVerification: %v", err)
	}
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	writeCompactFixtureRecord(t, store, state)
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, repo, "add", "-A")
	gitSnapshot(t, repo, "commit", "-m", "deliver the corrected candidate")
	// The delivered set genuinely exceeds the frozen reviewed manifest, so the
	// allow below can only come from the widened correction scope.
	delivered := []string{"internal/widget/widget.go", "internal/widget/widget_windows_test.go"}
	if pathsAreSubset(delivered, state.GenesisPaths) == nil || pathsAreSubset(delivered, state.CorrectionScopePaths()) != nil {
		t.Fatalf("fixture does not exercise the widened scope: genesis=%v scope=%v", state.GenesisPaths, state.CorrectionScopePaths())
	}
	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{
		Gate: GatePrePR, LineageID: state.LineageID, BaseRef: "origin/" + branch,
	})
	if got.Result != GateAllow {
		t.Fatalf("pre-PR delivery of a correction that added a companion test = %#v, want allow", got)
	}
	if !equalStrings(receipt.CorrectionAddedPaths, []string{"internal/widget/widget_windows_test.go"}) {
		t.Fatalf("the authorized receipt must disclose the added path, got %v", receipt.CorrectionAddedPaths)
	}
}
