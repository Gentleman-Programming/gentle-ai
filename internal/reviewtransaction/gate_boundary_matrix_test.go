package reviewtransaction

// Wave 5 (Gate Cutover), Slice 1, task 2.6: the 35-cell gate-boundary-matrix
// harness (design decision 7). 5 gates x 7 algebra relations
// (exact, compatible_base_advance, provable_contraction, changed, ambiguous,
// unknown, unrelated). Per tasks.md 2.6, this slice builds the HARNESS ONLY:
// gateVerdict(gate, relation) is a Wave 5 Slice 3 deliverable that does not
// exist yet at this commit, and NativeGateEvaluation carries no Relation
// field until Slice 3 lands it. A cell can therefore be wired here ONLY when
// a REAL, ALREADY-SHIPPED production code path can be driven to prove it —
// this harness must never reimplement the algebra by hand to fabricate a
// verdict. Every cell this slice cannot yet drive through real production
// code is an explicit, reasoned SKIP (Explained: true), never a fabricated
// pass. Wiring lands incrementally in S2-S7 (design's PR Slicing Preview);
// the full 35-cell run with zero unexplained divergences is S6/S7's exit
// bar (tasks.md 6.6, 8.7).
//
// The wired cells drive the REAL compiled gentle-ai binary as a subprocess
// (not an in-process Go call, not a reimplementation) -- the same technique
// e2e/organicruntime uses (buildOrganicBinary): `go build ./cmd/gentle-ai`
// once per test run, then every wired cell execs that one binary through
// `review start` / `review finalize` / `review validate --gate <gate>`.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

var gateBoundaryMatrixGates = []string{
	string(GatePostApply), string(GatePreCommit), string(GatePrePush), string(GatePrePR), string(GateRelease),
}

// gateBoundaryMatrixRelations mirrors the 7-value CandidateRelation
// vocabulary (rdd-candidate-relation-algebra, Wave 1) as plain strings: this
// harness is gate-and-verdict-shaped scaffolding, not an algebra consumer,
// so it names the relations without importing the (not-yet-gate-facing)
// CandidateRelation type.
var gateBoundaryMatrixRelations = []string{
	"exact", "compatible_base_advance", "provable_contraction", "changed", "ambiguous", "unknown", "unrelated",
}

// gateBoundaryMatrixNotWiredReason (W-1, Wave 5 fix cycle 1, verify-report
// #10186): the ORIGINAL text below claimed gateVerdict, NativeGateEvaluation's
// Relation field, and legacy-through-algebra projection did not exist yet --
// all three landed in S3/S4 and every one of gateVerdict/EvaluateLegacyGate/
// EvaluateNewLineageGate is now real, wired production code (this fix
// cycle's C-B/C-C). The honest remaining reason for every cell still marked
// here is narrower and true today: this harness only counts a cell as
// wired once it drives that real code end-to-end through the compiled
// binary with a genuinely constructed fixture (this file's own top-level
// doc comment, "a REAL, ALREADY-SHIPPED production code path can be
// driven"), and building that fixture -- particularly for release, whose
// boundary needs five additional artifact files and (for a v3 lineage)
// GENTLE_AI_RDD_NEW_LINEAGE plus a tier that can actually reach approved
// (C-A) -- has not been done for every remaining cell yet. This is a
// disclosed scope/time gap, not a missing mechanism.
const gateBoundaryMatrixNotWiredReason = "gateVerdict, NativeGateEvaluation.Relation, EvaluateLegacyGate, and " +
	"EvaluateNewLineageGate all exist and are wired production code as of Wave 5 fix cycle 1 (verify-report " +
	"#10186's C-B/C-C) -- this cell's own binary-driven fixture (start/finalize/validate through the compiled " +
	"gentle-ai binary) has not been built yet, not because the underlying mechanism is missing. This harness must " +
	"drive real code, not reimplement the algebra by hand, so an unbuilt fixture is an explicit SKIP, not a " +
	"fabricated pass."

// gateBoundaryMatrixPrePRCompatibleBaseAdvanceReason is task 6.3's named,
// specific explained-divergence reason for pre-pr's compatible_base_advance
// cell (design decision 7): unlike the other 26 remaining unwired cells,
// this one is not "just not gotten to it yet" -- compact_gate.go:91-102
// deliberately forces baseMatches = true and admits a current-changes
// boundary proof for pre-PR specifically, so this cell's own verdict
// legitimately differs from every other gate's compatible_base_advance
// column by design, not by omission. A real binary-driven recipe for it is
// still not built this slice (unlike its "changed" sibling above, which
// Slice 5's composition deletion made reachable); when one is built, this
// reason retires in favor of a driven row exactly as "changed" just did.
const gateBoundaryMatrixPrePRCompatibleBaseAdvanceReason = "pre-pr's compatible_base_advance cell legitimately " +
	"diverges from the other four gates by design (decision 7): compact_gate.go:91-102 forces baseMatches = true " +
	"and admits a current-changes boundary proof for pre-PR specifically, so a pre-PR candidate can reach " +
	"compatible_base_advance under conditions the other gates' identical relation never admits. This is a named, " +
	"explained architectural divergence, not an un-investigated gap -- unlike the generic 'not wired yet' reason, " +
	"which the other 26 remaining skip cells still carry."

type gateBoundaryMatrixRow struct {
	Gate      string `json:"gate"`
	Relation  string `json:"relation"`
	Verdict   string `json:"verdict"`
	NextStep  string `json:"next_step,omitempty"`
	Explained bool   `json:"explained"`
	Reason    string `json:"reason"`
}

// updateGateBoundaryMatrixGolden used to reuse shadow_matrix_test.go's own
// "-update" flag (same package, same golden-update convention) rather than
// registering a second "-update" flag, which would panic at test-binary
// init. shadow_matrix_test.go retired in Wave 7 S2b/S2c (its generating
// test deleted; shadow-differential-matrix.golden itself is retained
// byte-unchanged as historical evidence) -- this file now owns the "-update"
// flag registration directly.
var updateGateBoundaryMatrixGolden = flag.Bool("update", false, "update the gate boundary matrix golden file")

var (
	gateBoundaryMatrixBinaryOnce sync.Once
	gateBoundaryMatrixBinaryPath string
	gateBoundaryMatrixBinaryErr  error
)

// gateBoundaryMatrixBinary compiles the real gentle-ai binary once for the
// whole test run (mirrors e2e/organicruntime's buildOrganicBinary) so every
// wired cell drives the identical, actually-shipped CLI, never a
// reimplementation of gate-evaluation logic.
func gateBoundaryMatrixBinary(t *testing.T) string {
	t.Helper()
	gateBoundaryMatrixBinaryOnce.Do(func() {
		_, source, _, ok := runtime.Caller(0)
		if !ok {
			gateBoundaryMatrixBinaryErr = errors.New("resolve gate boundary matrix test source")
			return
		}
		moduleRoot := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
		name := "gentle-ai-gate-boundary-matrix"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		dir, err := os.MkdirTemp("", "gate-boundary-matrix-binary-*")
		if err != nil {
			gateBoundaryMatrixBinaryErr = err
			return
		}
		path := filepath.Join(dir, name)
		cmd := exec.CommandContext(context.Background(), "go", "build", "-trimpath", "-o", path, "./cmd/gentle-ai")
		cmd.Dir = moduleRoot
		cmd.Env = os.Environ()
		if output, err := cmd.CombinedOutput(); err != nil {
			gateBoundaryMatrixBinaryErr = fmt.Errorf("build gentle-ai test binary: %w\n%s", err, output)
			return
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			gateBoundaryMatrixBinaryErr = fmt.Errorf("built gentle-ai binary %q is unusable: %v", path, err)
			return
		}
		gateBoundaryMatrixBinaryPath = path
	})
	if gateBoundaryMatrixBinaryErr != nil {
		t.Fatalf("gate boundary matrix binary: %v", gateBoundaryMatrixBinaryErr)
	}
	return gateBoundaryMatrixBinaryPath
}

// runGateBoundaryMatrixReview execs the real binary's `review <verb>` for
// one repository, appending `--cwd repo` so callers only name the verb and
// its verb-specific flags. stdout and stderr are captured separately: a
// denial writes its full JSON result to stdout AND exits non-zero with an
// "Error: ..." line on stderr, and CombinedOutput's interleaving of the two
// would corrupt the JSON this function's callers decode. On failure, both
// streams are folded into the returned error text so callers still see the
// full diagnostic.
func runGateBoundaryMatrixReview(binary, repo string, args ...string) (string, error) {
	full := append([]string{"review"}, args...)
	full = append(full, "--cwd", repo)
	cmd := exec.CommandContext(context.Background(), binary, full...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return stdout.String(), fmt.Errorf("%w: %s", err, stderr.String())
	}
	return stdout.String(), nil
}

type gateBoundaryMatrixCLIResult struct {
	Result   string `json:"result"`
	Relation string `json:"relation,omitempty"`
	Next     *struct {
		Transition string `json:"transition,omitempty"`
		ReasonCode string `json:"reason_code"`
	} `json:"next,omitempty"`
}

func decodeGateBoundaryMatrixResult(t *testing.T, payload string) gateBoundaryMatrixCLIResult {
	t.Helper()
	var result gateBoundaryMatrixCLIResult
	if err := json.Unmarshal([]byte(payload), &result); err != nil {
		t.Fatalf("decode gate boundary matrix CLI result: %v\n%s", err, payload)
	}
	return result
}

// TestGateBoundaryMatrix_35Cells builds and checks
// testdata/gate-boundary-matrix.golden: 35 rows (5 gates x 7 relations).
//
// Wired cells: 8 as of Slice 5 (up from 7 after Slice 3, up from 4 after
// Slice 1) -- the "exact" relation at post-apply/pre-commit/pre-push/pre-pr
// (Slice 1), the "changed" relation at post-apply/pre-commit/pre-push
// (Slice 3, new: gateVerdict + attachGateVerdictRelation now populate
// Relation/Next on a real denial, observable through the CLI's
// ReviewValidateResult.Relation/Next wire fields Slice 3 also adds), and now
// pre-pr's own "changed" cell (Slice 5: deleting EvaluateCompactPrePRChain
// made a real, composition-free CLI recipe honestly reachable for the first
// time -- see this cell's own wiring comment below for the full history).
// Every other cell -- including release's "exact" cell -- is an explicit,
// reasoned SKIP;
// see each SKIP's own comment above for why it is not wired yet.
func TestGateBoundaryMatrix_35Cells(t *testing.T) {
	binary := gateBoundaryMatrixBinary(t)
	wired := map[[2]string]gateBoundaryMatrixRow{}

	// post-apply / exact: workspace candidate matches the approved receipt
	// with zero further changes.
	{
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "docs/notes.md", "release notes\n")
		gitSnapshot(t, repo, "add", "--", "docs/notes.md")
		lineage := "matrix-post-apply-exact"
		if out, err := runGateBoundaryMatrixReview(binary, repo, "start", "--lineage", lineage); err != nil {
			t.Fatalf("post-apply/exact review start: %v\n%s", err, out)
		}
		if out, err := runGateBoundaryMatrixReview(binary, repo, "finalize", "--lineage", lineage); err != nil {
			t.Fatalf("post-apply/exact review finalize: %v\n%s", err, out)
		}
		out, err := runGateBoundaryMatrixReview(binary, repo, "validate", "--lineage", lineage, "--gate", string(GatePostApply))
		if err != nil {
			t.Fatalf("post-apply/exact review validate: %v\n%s", err, out)
		}
		result := decodeGateBoundaryMatrixResult(t, out)
		if result.Result != string(GateAllow) {
			t.Fatalf("post-apply/exact result = %#v", result)
		}
		wired[[2]string{string(GatePostApply), "exact"}] = gateBoundaryMatrixRow{
			Gate: string(GatePostApply), Relation: "exact", Verdict: string(GateAllow), Explained: false,
			Reason: "driven via the real gentle-ai binary: review start -> finalize -> validate --gate post-apply on the identical workspace candidate",
		}
	}

	// pre-commit / exact: same workspace candidate, staged (not committed).
	{
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "docs/notes.md", "release notes\n")
		gitSnapshot(t, repo, "add", "--", "docs/notes.md")
		lineage := "matrix-pre-commit-exact"
		if out, err := runGateBoundaryMatrixReview(binary, repo, "start", "--lineage", lineage); err != nil {
			t.Fatalf("pre-commit/exact review start: %v\n%s", err, out)
		}
		if out, err := runGateBoundaryMatrixReview(binary, repo, "finalize", "--lineage", lineage); err != nil {
			t.Fatalf("pre-commit/exact review finalize: %v\n%s", err, out)
		}
		gitSnapshot(t, repo, "add", "docs/notes.md")
		out, err := runGateBoundaryMatrixReview(binary, repo, "validate", "--lineage", lineage, "--gate", string(GatePreCommit))
		if err != nil {
			t.Fatalf("pre-commit/exact review validate: %v\n%s", err, out)
		}
		result := decodeGateBoundaryMatrixResult(t, out)
		if result.Result != string(GateAllow) {
			t.Fatalf("pre-commit/exact result = %#v", result)
		}
		wired[[2]string{string(GatePreCommit), "exact"}] = gateBoundaryMatrixRow{
			Gate: string(GatePreCommit), Relation: "exact", Verdict: string(GateAllow), Explained: false,
			Reason: "driven via the real gentle-ai binary: review start -> finalize -> git add -> validate --gate pre-commit on the identical staged candidate",
		}
	}

	// pre-push and pre-pr / exact: the same committed candidate, one commit
	// ahead of an unchanged origin remote, satisfies both gates' boundary.
	{
		repo := initSnapshotRepo(t)
		branch := currentBranch(context.Background(), repo)
		remote := configurePublicationRemote(t, repo, branch)
		writeSnapshotFile(t, repo, "docs/notes.md", "release notes\n")
		gitSnapshot(t, repo, "add", "--", "docs/notes.md")
		lineage := "matrix-push-pr-exact"
		if out, err := runGateBoundaryMatrixReview(binary, repo, "start", "--lineage", lineage); err != nil {
			t.Fatalf("pre-push/pre-pr exact review start: %v\n%s", err, out)
		}
		if out, err := runGateBoundaryMatrixReview(binary, repo, "finalize", "--lineage", lineage); err != nil {
			t.Fatalf("pre-push/pre-pr exact review finalize: %v\n%s", err, out)
		}
		gitSnapshot(t, repo, "add", "docs/notes.md")
		gitSnapshot(t, repo, "commit", "-m", "reviewed candidate")
		_ = remote

		pushOut, err := runGateBoundaryMatrixReview(binary, repo, "validate", "--lineage", lineage,
			"--gate", string(GatePrePush), "--base-ref", "origin/"+branch)
		if err != nil {
			t.Fatalf("pre-push/exact review validate: %v\n%s", err, pushOut)
		}
		pushResult := decodeGateBoundaryMatrixResult(t, pushOut)
		if pushResult.Result != string(GateAllow) {
			t.Fatalf("pre-push/exact result = %#v", pushResult)
		}
		wired[[2]string{string(GatePrePush), "exact"}] = gateBoundaryMatrixRow{
			Gate: string(GatePrePush), Relation: "exact", Verdict: string(GateAllow), Explained: false,
			Reason: "driven via the real gentle-ai binary: review start -> finalize -> git add/commit -> validate --gate pre-push against an unchanged origin remote",
		}

		prOut, err := runGateBoundaryMatrixReview(binary, repo, "validate", "--lineage", lineage,
			"--gate", string(GatePrePR), "--base-ref", "origin/"+branch)
		if err != nil {
			t.Fatalf("pre-pr/exact review validate: %v\n%s", err, prOut)
		}
		prResult := decodeGateBoundaryMatrixResult(t, prOut)
		if prResult.Result != string(GateAllow) {
			t.Fatalf("pre-pr/exact result = %#v", prResult)
		}
		wired[[2]string{string(GatePrePR), "exact"}] = gateBoundaryMatrixRow{
			Gate: string(GatePrePR), Relation: "exact", Verdict: string(GateAllow), Explained: false,
			Reason: "driven via the real gentle-ai binary: review start -> finalize -> git add/commit -> validate --gate pre-pr against an unchanged origin remote base",
		}
	}

	// Wave 5 Slice 3: the "changed" relation is now real for all 5 gates
	// (attachGateVerdictRelation, compact_gate.go), driven here through the
	// identical real binary -- each fixture reaches its approved receipt
	// exactly as the "exact" cells above, then drifts the candidate so the
	// SAME EvaluateCompactGate call denies scope-changed/base-mismatch and
	// carries Relation="changed" + Next in its real JSON output (the
	// ReviewValidateResult.Relation/Next wire fields Slice 3 adds).
	assertChanged := func(t *testing.T, gate string, out string, err error, wantNext string, wantReasonCode string) gateBoundaryMatrixRow {
		t.Helper()
		if err == nil {
			t.Fatalf("%s/changed review validate unexpectedly allowed:\n%s", gate, out)
		}
		result := decodeGateBoundaryMatrixResult(t, out)
		if result.Relation != "changed" {
			t.Fatalf("%s/changed relation = %q, want %q:\n%s", gate, result.Relation, "changed", out)
		}
		if result.Next == nil || result.Next.Transition != wantNext || result.Next.ReasonCode != wantReasonCode {
			t.Fatalf("%s/changed next = %#v, want transition %q reason_code %q", gate, result.Next, wantNext, wantReasonCode)
		}
		return gateBoundaryMatrixRow{
			Gate: gate, Relation: "changed", Verdict: result.Result, NextStep: wantNext, Explained: false,
			Reason: "driven via the real gentle-ai binary: approved candidate drifted, then validate --gate " + gate + " denies with Relation/Next populated by gateVerdict (Slice 3)",
		}
	}

	// post-apply / changed: drift the reviewed file's own content. Since
	// #2394 an undeclared ADDITION is not review scope at all, so it changes
	// nothing here; drifting a declared path is what the gate must catch.
	{
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "docs/notes.md", "release notes\n")
		gitSnapshot(t, repo, "add", "--", "docs/notes.md")
		lineage := "matrix-post-apply-changed"
		if out, err := runGateBoundaryMatrixReview(binary, repo, "start", "--lineage", lineage); err != nil {
			t.Fatalf("post-apply/changed review start: %v\n%s", err, out)
		}
		if out, err := runGateBoundaryMatrixReview(binary, repo, "finalize", "--lineage", lineage); err != nil {
			t.Fatalf("post-apply/changed review finalize: %v\n%s", err, out)
		}
		writeSnapshotFile(t, repo, "docs/notes.md", "drifted after approval\n")
		out, err := runGateBoundaryMatrixReview(binary, repo, "validate", "--lineage", lineage, "--gate", string(GatePostApply))
		wired[[2]string{string(GatePostApply), "changed"}] = assertChanged(t, string(GatePostApply), out, err, "review start", "candidate_changed")
	}

	// pre-commit / changed: same drift, staged.
	{
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "docs/notes.md", "release notes\n")
		gitSnapshot(t, repo, "add", "--", "docs/notes.md")
		lineage := "matrix-pre-commit-changed"
		if out, err := runGateBoundaryMatrixReview(binary, repo, "start", "--lineage", lineage); err != nil {
			t.Fatalf("pre-commit/changed review start: %v\n%s", err, out)
		}
		if out, err := runGateBoundaryMatrixReview(binary, repo, "finalize", "--lineage", lineage); err != nil {
			t.Fatalf("pre-commit/changed review finalize: %v\n%s", err, out)
		}
		writeSnapshotFile(t, repo, "docs/notes.md", "drifted after approval\n")
		gitSnapshot(t, repo, "add", "docs/notes.md")
		out, err := runGateBoundaryMatrixReview(binary, repo, "validate", "--lineage", lineage, "--gate", string(GatePreCommit))
		wired[[2]string{string(GatePreCommit), "changed"}] = assertChanged(t, string(GatePreCommit), out, err, "review start", "candidate_changed")
	}

	// pre-push / changed: committed delivery, then AMENDED (not a further
	// commit) so the delivery stays exactly one commit ahead of its
	// reviewed base -- pre-push's current-changes receipt requires exactly
	// one delivery commit, so an additional drift commit trips that
	// "not exactly one commit" check before ever reaching the
	// candidate-or-paths-mismatch comparison this cell targets.
	{
		repo := initSnapshotRepo(t)
		branch := currentBranch(context.Background(), repo)
		configurePublicationRemote(t, repo, branch)
		writeSnapshotFile(t, repo, "docs/notes.md", "release notes\n")
		gitSnapshot(t, repo, "add", "--", "docs/notes.md")
		lineage := "matrix-pre-push-changed"
		if out, err := runGateBoundaryMatrixReview(binary, repo, "start", "--lineage", lineage); err != nil {
			t.Fatalf("pre-push/changed review start: %v\n%s", err, out)
		}
		if out, err := runGateBoundaryMatrixReview(binary, repo, "finalize", "--lineage", lineage); err != nil {
			t.Fatalf("pre-push/changed review finalize: %v\n%s", err, out)
		}
		gitSnapshot(t, repo, "add", "docs/notes.md")
		gitSnapshot(t, repo, "commit", "-m", "reviewed candidate")
		writeSnapshotFile(t, repo, "docs/notes.md", "drifted after approval\n")
		gitSnapshot(t, repo, "add", "docs/notes.md")
		gitSnapshot(t, repo, "commit", "--amend", "--no-edit")
		out, err := runGateBoundaryMatrixReview(binary, repo, "validate", "--lineage", lineage, "--gate", string(GatePrePush), "--base-ref", "origin/"+branch)
		wired[[2]string{string(GatePrePush), "changed"}] = assertChanged(t, string(GatePrePush), out, err, "review start", "candidate_changed")
	}

	// pre-pr / changed: Wave 5 Slice 5 wires this cell for the first time,
	// via the composition-free recipe its own deletion made honestly
	// reachable. Before this slice, EvaluateCompactPrePRChain composed
	// sequential same-path receipts into an allow, so a plain `review
	// start`-driven CLI recipe could never reliably reach the denial (the
	// prior blocker: verifying review start's TargetCurrentChanges shape
	// byte-for-byte matched the Go-level TargetExactRevision fixture that
	// DID prove it, gate_verdict_deny_golden_test.go's
	// TestPrePRGate_Deny_BaseMismatchDeniesWithoutComposition). Three
	// lineages review the SAME path in sequence, each individually approved
	// and delivered as its own commit: lineage-free discovery resolves the
	// LAST lineage uniquely (the shared path narrows what would otherwise be
	// ambiguous -- TestPrePRChainCompositionDeletionSupersedesRemovalDelta
	// above proves the 3-DISTINCT-path sibling denies via ambiguity
	// instead), but that lineage's own receipt only covers its own segment,
	// so EvaluateCompactGate denies it base-mismatch against the live
	// candidate three commits ahead of the reviewed base.
	// TestUnqualifiedPrePRDiscoveryDeniesSequentialReceiptsForSamePathWithoutComposition
	// (internal/cli) is this cell's funnel-level sibling proof, through the
	// negotiated envelope rather than this raw-CLI-JSON shape.
	//
	// Unlike the other 3 "changed" cells (S3), this denial's precondition
	// fires BEFORE the relation table (absorbed N2, gate.go:1745,
	// "BaseRelationshipValid is gated to pre-pr/release only") with NO
	// Next.Transition: a base-relationship-invalid denial has no
	// review-start-shaped fix by design -- restarting review of the same
	// candidate cannot repair a stale reviewed base. This is a genuine,
	// deliberate gap in gateVerdict's next-step vocabulary (task 6.7's own
	// "every denial names a runnable next step" goal is not yet met here),
	// not a Slice 5 regression; recorded, not silently accepted.
	{
		repo := initSnapshotRepo(t)
		branch := currentBranch(context.Background(), repo)
		configurePublicationRemote(t, repo, branch)
		lineages := []string{"matrix-pre-pr-changed-first", "matrix-pre-pr-changed-second", "matrix-pre-pr-changed-third"}
		for index, lineage := range lineages {
			writeSnapshotFile(t, repo, "docs/shared.md", "reviewed segment "+string(rune('a'+index))+"\n")
			gitSnapshot(t, repo, "add", "--", "docs/shared.md")
			if out, err := runGateBoundaryMatrixReview(binary, repo, "start", "--lineage", lineage); err != nil {
				t.Fatalf("pre-pr/changed review start (%s): %v\n%s", lineage, err, out)
			}
			if out, err := runGateBoundaryMatrixReview(binary, repo, "finalize", "--lineage", lineage); err != nil {
				t.Fatalf("pre-pr/changed review finalize (%s): %v\n%s", lineage, err, out)
			}
			gitSnapshot(t, repo, "add", "-A")
			gitSnapshot(t, repo, "commit", "-m", "deliver "+lineage)
		}
		out, err := runGateBoundaryMatrixReview(binary, repo, "validate", "--gate", string(GatePrePR), "--base-ref", "origin/"+branch)
		if err == nil {
			t.Fatalf("pre-pr/changed same-path sequential delivery unexpectedly allowed:\n%s", out)
		}
		result := decodeGateBoundaryMatrixResult(t, out)
		if result.Relation != "changed" {
			t.Fatalf("pre-pr/changed relation = %q, want %q:\n%s", result.Relation, "changed", out)
		}
		if result.Next == nil || result.Next.Transition != "" || result.Next.ReasonCode != "base_relationship_invalid" {
			t.Fatalf("pre-pr/changed next = %#v, want empty transition, reason_code base_relationship_invalid", result.Next)
		}
		wired[[2]string{string(GatePrePR), "changed"}] = gateBoundaryMatrixRow{
			Gate: string(GatePrePR), Relation: "changed", Verdict: result.Result, Explained: false,
			Reason: "driven via the real gentle-ai binary: three lineages reviewing the same path in sequence, each individually approved and committed; lineage-free validate --gate pre-pr denies base-mismatch -- composition-free for the first time since Wave 5 Slice 5's deletion",
		}
	}

	// W-2 (Wave 5 fix cycle 2, verify-report #10186): release had zero driven
	// cells (W-2's own finding). release/exact and release/changed are wired
	// here, mirroring the identical exact/changed recipes above, extended
	// with the five release-boundary artifact flags gateVerdict's
	// gate==GateRelease precondition requires (C-B/C-C wired the receiving
	// end of that precondition for legacy and v3; this cell exercises it
	// through the compact/v2 path, the one lineage kind a plain binary-driven
	// `review start` can freshly create -- confirmed empirically: a v1 legacy
	// lineage has no CLI-reachable creation path at all, and a v3 lineage
	// needs GENTLE_AI_RDD_NEW_LINEAGE threaded into the subprocess, both
	// deferred rather than rushed into this budget).
	releaseArtifactArgs := func(t *testing.T) []string {
		t.Helper()
		dir := t.TempDir()
		artifact := func(name, content string) string {
			path := filepath.Join(dir, name)
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			return path
		}
		return []string{
			"--release-configuration", artifact("configuration.txt", "configuration\n"),
			"--release-generated", artifact("generated.txt", "generated\n"),
			"--release-provenance", artifact("provenance.txt", "provenance\n"),
			"--release-publication-boundary", artifact("publication-boundary.txt", "publication boundary\n"),
			"--release-evidence-freshness", artifact("evidence-freshness.txt", "evidence freshness\n"),
		}
	}

	// release / exact: release's own target resolution is TargetExactRevision
	// at the current committed HEAD (lifecycleTargetForGate's GateRelease
	// case), not TargetCurrentChanges -- so, unlike post-apply/pre-commit,
	// the reviewed candidate must actually be committed before release can
	// see it at all (confirmed empirically: an uncommitted candidate denies
	// scope-changed, HEAD's tree never contained the reviewed path).
	{
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "docs/notes.md", "release notes\n")
		gitSnapshot(t, repo, "add", "--", "docs/notes.md")
		lineage := "matrix-release-exact"
		if out, err := runGateBoundaryMatrixReview(binary, repo, "start", "--lineage", lineage); err != nil {
			t.Fatalf("release/exact review start: %v\n%s", err, out)
		}
		if out, err := runGateBoundaryMatrixReview(binary, repo, "finalize", "--lineage", lineage); err != nil {
			t.Fatalf("release/exact review finalize: %v\n%s", err, out)
		}
		gitSnapshot(t, repo, "add", "docs/notes.md")
		gitSnapshot(t, repo, "commit", "-m", "deliver reviewed candidate")
		args := append([]string{"validate", "--lineage", lineage, "--gate", string(GateRelease)}, releaseArtifactArgs(t)...)
		out, err := runGateBoundaryMatrixReview(binary, repo, args...)
		if err != nil {
			t.Fatalf("release/exact review validate: %v\n%s", err, out)
		}
		result := decodeGateBoundaryMatrixResult(t, out)
		if result.Result != string(GateAllow) {
			t.Fatalf("release/exact result = %#v", result)
		}
		wired[[2]string{string(GateRelease), "exact"}] = gateBoundaryMatrixRow{
			Gate: string(GateRelease), Relation: "exact", Verdict: string(GateAllow), Explained: false,
			Reason: "driven via the real gentle-ai binary: review start -> finalize -> validate --gate release with the five release-boundary artifacts on the identical, unchanged candidate",
		}
	}

	// release / changed: investigated, not pursued this budget. Unlike the
	// other four gates, release's own target resolves TargetExactRevision at
	// the current committed HEAD (lifecycleTargetForGate/buildCompactGateRequestWithPushBase's
	// GateRelease case), not TargetCurrentChanges -- an uncommitted workspace
	// drift (the recipe the other four "changed" cells use) never reaches
	// release's own candidate comparison at all, and the fixture instead
	// needs a genuinely delivered (committed) drift, mirroring pre-push's own
	// "amend the delivery commit" recipe rather than the plain post-apply/
	// pre-commit/pre-pr shape. Confirmed empirically: the plain-drift recipe
	// produces an unrelated scope-change diagnostic, not a clean "changed"
	// denial. Deferred rather than rushed; release/exact above is this
	// budget's genuine, verified increment.

	rows := make([]gateBoundaryMatrixRow, 0, len(gateBoundaryMatrixGates)*len(gateBoundaryMatrixRelations))
	for _, gate := range gateBoundaryMatrixGates {
		for _, relation := range gateBoundaryMatrixRelations {
			if row, ok := wired[[2]string{gate, relation}]; ok {
				rows = append(rows, row)
				continue
			}
			reason := gateBoundaryMatrixNotWiredReason
			if gate == string(GatePrePR) && relation == "compatible_base_advance" {
				reason = gateBoundaryMatrixPrePRCompatibleBaseAdvanceReason
			}
			rows = append(rows, gateBoundaryMatrixRow{
				Gate: gate, Relation: relation, Explained: true, Reason: reason,
			})
		}
	}
	if len(rows) != 35 {
		t.Fatalf("gate boundary matrix has %d rows, want 35 (5 gates x 7 relations)", len(rows))
	}
	if len(wired) != 9 {
		t.Fatalf("gate boundary matrix wired %d cells, want exactly 9 (4 from S1: post-apply/pre-commit/pre-push/pre-pr exact; 3 from S3: post-apply/pre-commit/pre-push changed; 1 from S5: pre-pr changed; 1 new from Wave 5 fix cycle 2 W-2: release exact)", len(wired))
	}

	actual, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	actual = append(actual, '\n')

	goldenPath := filepath.Join("testdata", "gate-boundary-matrix.golden")
	if *updateGateBoundaryMatrixGolden {
		if err := os.WriteFile(goldenPath, actual, 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", goldenPath, err)
		}
		t.Logf("updated golden file: %s (%d rows, %d wired)", goldenPath, len(rows), len(wired))
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("missing golden %s -- run: go test ./internal/reviewtransaction/... -run TestGateBoundaryMatrix_35Cells -update (%v)", goldenPath, err)
	}
	if string(want) != string(actual) {
		t.Fatalf("golden mismatch for %s -- rerun with -update after inspecting the diff\nwant:\n%s\ngot:\n%s", goldenPath, want, actual)
	}
}

// gateBoundaryMatrixGoldenRow re-reads the committed golden and returns the
// exact row for (gate, relation) -- task 6.3's two named tests assert
// against the persisted golden directly (not a freshly-driven binary run),
// so they fail loudly on ANY accidental regression to the generic
// "not wired yet" reason or an unexplained divergence, independent of
// whether TestGateBoundaryMatrix_35Cells itself is also run.
func gateBoundaryMatrixGoldenRow(t *testing.T, gate, relation string) gateBoundaryMatrixRow {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join("testdata", "gate-boundary-matrix.golden"))
	if err != nil {
		t.Fatal(err)
	}
	var rows []gateBoundaryMatrixRow
	if err := json.Unmarshal(payload, &rows); err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Gate == gate && row.Relation == relation {
			return row
		}
	}
	t.Fatalf("gate boundary matrix golden has no row for gate=%q relation=%q", gate, relation)
	return gateBoundaryMatrixRow{}
}

// TestPrePRDivergence_CompatibleBaseAdvanceExplained is task 6.3: pre-PR's
// compatible_base_advance cell must carry explained: true with the specific
// boundary-proof reason (design decision 7) -- never the generic
// "not wired yet" reason every un-investigated skip cell carries, and never
// a silent, unexplained divergence.
func TestPrePRDivergence_CompatibleBaseAdvanceExplained(t *testing.T) {
	row := gateBoundaryMatrixGoldenRow(t, string(GatePrePR), "compatible_base_advance")
	if !row.Explained {
		t.Fatalf("pre-pr/compatible_base_advance = %#v, want explained: true", row)
	}
	if row.Reason != gateBoundaryMatrixPrePRCompatibleBaseAdvanceReason {
		t.Fatalf("pre-pr/compatible_base_advance reason = %q, want the specific boundary-proof reason, got the generic one: %v",
			row.Reason, row.Reason == gateBoundaryMatrixNotWiredReason)
	}
}

// TestPrePRDivergence_ChangedExplained is task 6.3's sibling: pre-PR's
// changed cell is no longer a skip at all (Slice 5 made it honestly
// reachable through a real, composition-free binary-driven recipe) -- its
// divergence from the other four gates' own changed cells (no
// Next.Transition, a precondition-stage denial rather than a relation-table
// one) is proven, not merely explained in prose.
func TestPrePRDivergence_ChangedExplained(t *testing.T) {
	row := gateBoundaryMatrixGoldenRow(t, string(GatePrePR), "changed")
	if row.Explained {
		t.Fatalf("pre-pr/changed = %#v, want explained: false -- it is a driven, proven cell as of Slice 5, not a skip", row)
	}
	if row.Verdict != string(GateInvalidated) {
		t.Fatalf("pre-pr/changed verdict = %q, want %q", row.Verdict, GateInvalidated)
	}
	if row.NextStep != "" {
		t.Fatalf("pre-pr/changed next step = %q, want empty -- a base-relationship-invalid denial names no review-start-shaped fix by design", row.NextStep)
	}
}
