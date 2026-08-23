package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	approvedBaseDiffLocalMergeJourneyID = "j86-approved-base-diff-local-parent-merge-preserves-approved-receipt"
	approvedBaseDiffLocalMergeLineage   = "approved-base-diff-local-merge"
	approvedBaseDiffLocalMergePath      = "internal/auth/session.go"
	approvedBaseDiffLocalMergeAdvance   = "docs/parent-advance.md"
)

// localGateBaseAdvanceJourneys covers #2388's only approved shape: a reviewed
// high-risk base diff can absorb a disjoint parent advance without minting a
// successor receipt. The same base-relative proof governs staged pre-commit
// and the committed pre-push STATUS transition.
func localGateBaseAdvanceJourneys() []Journey {
	return []Journey{
		{
			ID:     approvedBaseDiffLocalMergeJourneyID,
			Review: reviewOptedIn,
			Title:  "An approved base-diff receipt survives a disjoint local parent merge",
			Source: "issue #2388: explicit base-ref preserves the approved B0-to-C0 receipt when a disjoint B0-to-B1 parent advance is merged into C0",
			Steps: []Step{
				{Name: "fixture: local bare origin, B0, and committed high-risk child C0 on P", Fixture: approvedBaseDiffLocalMergeFixture},
				{Name: "negotiate and execute committed base-diff START against B0", Requires: statusCapability, Composite: startApprovedBaseDiffLocalMerge},
				{Name: "capture every selected high-risk lens", Requires: captureResultCapability, Composite: captureApprovedBaseDiffLocalMergeLenses},
				{Name: "finalize captured lens results", Requires: finalizeResultsCapability, Args: productArgs("review", "finalize", "--lineage", approvedBaseDiffLocalMergeLineage, "--captured-results=true")},
				{Name: "finalize requests evidence", Requires: finalizeCapability, Args: productArgs("review", "finalize", "--lineage", approvedBaseDiffLocalMergeLineage)},
				{Name: "capture final evidence", Requires: captureEvidenceCapability, Composite: captureApprovedBaseDiffLocalMergeEvidence},
				{Name: "finalize the approved receipt", Requires: finalizeEvidenceCapability, Args: productArgs("review", "finalize", "--lineage", approvedBaseDiffLocalMergeLineage, "--captured-evidence=true")},
				{Name: "prove the approved receipt is bound to B0, C0, and P", Requires: statusCapability, Composite: proveApprovedBaseDiffLocalMergeReceipt},
				{Name: "fixture: sibling advances origin/main to B1 and local no-commit merge proves B1-to-M", Fixture: advanceParentDisjointlyAndStageMerge},
				{Name: "pre-commit allows the explicit B1 staged target", Requires: validateCapability, Args: approvedBaseDiffLocalMergeGateArgs("pre-commit"), After: requireReviewedSupersetPrePushAllow},
				{Name: "fixture: commit the proven merge tree M", Fixture: commitStaged("merge disjoint parent advance")},
				{Name: "STATUS keeps the approved receipt and prints pre-push validation", Requires: statusCapability, Composite: executeApprovedBaseDiffLocalMergePrePush},
			},
		},
	}
}

func approvedBaseDiffLocalMergeFixture(sandbox *Sandbox) error {
	if err := baseRepoWithRemote(sandbox); err != nil {
		return err
	}
	for key, ref := range map[string]string{"b0": "HEAD", "b0-tree": "HEAD^{tree}"} {
		value, err := gitOut(sandbox, sandbox.Repo, "rev-parse", ref)
		if err != nil {
			return err
		}
		sandbox.Scratch["approved-base-diff-local-merge-"+key] = value
	}
	if err := stageAuthCode(sandbox); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "commit", "-qm", "reviewed high-risk delivery"); err != nil {
		return err
	}
	for key, ref := range map[string]string{"c0": "HEAD", "c0-tree": "HEAD^{tree}"} {
		value, err := gitOut(sandbox, sandbox.Repo, "rev-parse", ref)
		if err != nil {
			return err
		}
		sandbox.Scratch["approved-base-diff-local-merge-"+key] = value
	}
	return nil
}

func approvedBaseDiffLocalMergeSelectors(run *journeyRun, extra ...string) []string {
	selectors := []string{"--lineage", approvedBaseDiffLocalMergeLineage, "--base-ref", run.sandbox.Scratch["approved-base-diff-local-merge-b0"]}
	return append(selectors, extra...)
}

func startApprovedBaseDiffLocalMerge(run *journeyRun) error {
	envelope, err := readStatusFor(run, approvedBaseDiffLocalMergeSelectors(run)...)
	if err != nil {
		return err
	}
	if envelope.NextTransition.Kind != "execute" || envelope.NextTransition.Execute.Operation != "review.start" {
		return fmt.Errorf("committed base-diff START transition = %+v", envelope.NextTransition)
	}
	observation, err := runPrintedTransition(run, envelope)
	if err != nil {
		return err
	}
	if observation.ExitCode != 0 {
		return fmt.Errorf("committed base-diff START exited %d: %s", observation.ExitCode, firstLine(observation.Stderr))
	}
	if err := rememberLineage(run.sandbox, observation); err != nil {
		return err
	}
	if run.sandbox.Lineage != approvedBaseDiffLocalMergeLineage {
		return fmt.Errorf("START lineage = %q, want %q", run.sandbox.Lineage, approvedBaseDiffLocalMergeLineage)
	}
	return nil
}

func captureApprovedBaseDiffLocalMergeLenses(run *journeyRun) error {
	return captureAllLensesFor(run, approvedBaseDiffLocalMergeSelectors(run)...)
}

func captureApprovedBaseDiffLocalMergeEvidence(run *journeyRun) error {
	return captureFinalEvidenceFor(run, approvedBaseDiffLocalMergeSelectors(run)...)
}

func proveApprovedBaseDiffLocalMergeReceipt(run *journeyRun) error {
	args := append([]string{"review", "status", "--contract", reviewContract, "--next-transition"}, approvedBaseDiffLocalMergeSelectors(run)...)
	observation := run.run(productArgsFor(run, args...), false)
	var status reviewedSupersetStatus
	if observation.ExitCode != 0 || json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &status) != nil ||
		status.Authority == nil || status.Authority.LineageID != approvedBaseDiffLocalMergeLineage || status.Authority.State != "approved" ||
		status.Receipt.Status != "present" || status.Receipt.Identity == "" ||
		status.Projection.BaseTree != run.sandbox.Scratch["approved-base-diff-local-merge-b0-tree"] ||
		status.Projection.CurrentCandidateTree != run.sandbox.Scratch["approved-base-diff-local-merge-c0-tree"] ||
		strings.Join(status.Projection.Paths, "\x00") != approvedBaseDiffLocalMergePath {
		return fmt.Errorf("approved B0/C0/P receipt was not proven: exit=%d status=%+v stderr=%s", observation.ExitCode, status, firstLine(observation.Stderr))
	}
	return nil
}

// advanceParentDisjointlyAndStageMerge creates B1 in a sibling clone then
// leaves the conflict-free B1/C0 merge staged as M, exactly as #2388 reports.
func advanceParentDisjointlyAndStageMerge(sandbox *Sandbox) error {
	sibling := filepath.Join(sandbox.Root, "parent-advance")
	if err := sandbox.git(sandbox.Root, "clone", "-q", sandbox.Remote, sibling); err != nil {
		return err
	}
	if err := sandbox.git(sibling, "checkout", "-q", "-B", "main", "origin/main"); err != nil {
		return err
	}
	for _, setting := range [][2]string{{"user.email", "bench@example.invalid"}, {"user.name", "Bench"}} {
		if err := sandbox.git(sibling, "config", setting[0], setting[1]); err != nil {
			return err
		}
	}
	if err := sandbox.write(filepath.Join(sibling, approvedBaseDiffLocalMergeAdvance), "# parent advance\n"); err != nil {
		return err
	}
	if err := sandbox.git(sibling, "add", "--", approvedBaseDiffLocalMergeAdvance); err != nil {
		return err
	}
	if err := sandbox.git(sibling, "commit", "-qm", "advance parent"); err != nil {
		return err
	}
	b1, err := gitOut(sandbox, sibling, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if err := sandbox.git(sibling, "push", "-q", "origin", "HEAD:main"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "fetch", "-q", "origin", "main"); err != nil {
		return err
	}
	c0 := sandbox.Scratch["approved-base-diff-local-merge-c0"]
	if head, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "HEAD"); err != nil || head != c0 {
		return fmt.Errorf("HEAD before merge = %q, want C0 %q", head, c0)
	}
	if err := sandbox.git(sandbox.Repo, "merge", "-q", "--no-commit", "--no-ff", "origin/main"); err != nil {
		return err
	}
	mergeHead, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "MERGE_HEAD")
	if err != nil || mergeHead != b1 {
		return fmt.Errorf("MERGE_HEAD = %q, want B1 %q", mergeHead, b1)
	}
	m, err := gitOut(sandbox, sandbox.Repo, "write-tree")
	if err != nil {
		return err
	}
	merged, err := gitOut(sandbox, sandbox.Repo, "merge-tree", "--write-tree", b1, c0)
	if err != nil || m != merged {
		return fmt.Errorf("staged merge tree M = %q, want conflict-free merge-tree %q", m, merged)
	}
	b0 := sandbox.Scratch["approved-base-diff-local-merge-b0"]
	for _, pair := range [][2]string{{b0, c0}, {b1, m}} {
		paths, err := gitOut(sandbox, sandbox.Repo, "diff", "--name-only", pair[0], pair[1])
		if err != nil || paths != approvedBaseDiffLocalMergePath {
			return fmt.Errorf("reviewed path identity %s..%s = %q", pair[0], pair[1], paths)
		}
		patch, err := gitOut(sandbox, sandbox.Repo, "diff", "--binary", "--full-index", "--no-ext-diff", pair[0], pair[1], "--")
		if err != nil {
			return err
		}
		if pair[0] == b0 {
			sandbox.Scratch["approved-base-diff-local-merge-patch"] = patch
		} else if patch != sandbox.Scratch["approved-base-diff-local-merge-patch"] {
			return fmt.Errorf("B0-to-C0 patch does not equal B1-to-M")
		}
	}
	paths, err := gitOut(sandbox, sandbox.Repo, "diff", "--name-only", b0, b1)
	if err != nil || paths != approvedBaseDiffLocalMergeAdvance {
		return fmt.Errorf("parent advance paths = %q, want %q", paths, approvedBaseDiffLocalMergeAdvance)
	}
	sandbox.Scratch["approved-base-diff-local-merge-b1"] = b1
	sandbox.Scratch["approved-base-diff-local-merge-m"] = m
	return nil
}

func approvedBaseDiffLocalMergeGateArgs(gate string) func(*Sandbox) ([]string, error) {
	return func(sandbox *Sandbox) ([]string, error) {
		return []string{"review", "validate", "--gate", gate, "--lineage", approvedBaseDiffLocalMergeLineage, "--base-ref", sandbox.Scratch["approved-base-diff-local-merge-b1"], "--cwd", sandbox.Repo}, nil
	}
}

func executeApprovedBaseDiffLocalMergePrePush(run *journeyRun) error {
	envelope, err := readStatusFor(run, "--lineage", approvedBaseDiffLocalMergeLineage, "--gate", "pre-push", "--base-ref", run.sandbox.Scratch["approved-base-diff-local-merge-b1"])
	if err != nil {
		return err
	}
	if envelope.NextTransition.Kind != "execute" || envelope.NextTransition.Execute.Operation != "review.validate" || envelope.executeArgument("gate") != "pre-push" {
		return fmt.Errorf("post-merge STATUS transition = %+v, want approved pre-push validation", envelope.NextTransition)
	}
	observation, err := runPrintedTransition(run, envelope)
	if err != nil {
		return err
	}
	return requireReviewedSupersetPrePushAllow(run.sandbox, observation)
}
