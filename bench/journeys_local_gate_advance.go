package main

import "path/filepath"

// journeys_local_gate_advance.go covers issue #2388 / D2 (#2471 option a) at
// the product boundary: an approved, committed base-diff receipt followed by
// a disjoint parent advance that the child locally merges in (staged, not yet
// committed -- the literal #2388 reproduction). D2 decided to extend
// compatible base advance to pre-commit/pre-push by reusing pre-PR's existing
// content checks without its CI attestation. internal/reviewtransaction's
// deriveBaseAdvanceCompatibility now supports that (requireAttestation
// parameter, see prepr.go), and receipt.go's baseAdvanceStatusAllowedForGate
// generalizes the per-gate policy -- but wiring it into GatePreCommit's own
// target derivation is a STOPPED sub-path (see
// internal/reviewtransaction/compact_gate_local_advance_test.go): a staged
// merge always changes CandidateTree, never just BaseTree, so the shared,
// unchanged content checks can never certify it compatible. This journey
// pins the honest, still-refused CLI-visible outcome so a future change that
// resolves the stop (or accidentally regresses today's refusal into a false
// allow) has a driven signal either way.
func localGateBaseAdvanceJourneys() []Journey {
	return []Journey{
		{
			ID:     "j73-pre-commit-still-refuses-staged-merge-of-an-advanced-parent",
			Title:  "An approved base-diff receipt's pre-commit gate still refuses a staged merge of a disjointly-advanced parent",
			Source: "issue #2388 / D2 (#2471 option a): compatible base advance without CI attestation is scoped to content checks pre-commit's own target derivation cannot satisfy (base and candidate cannot move independently for a staged merge), so this remains a refusal by design, not a regression",
			Steps: []Step{
				{Name: "fixture: repo with remote", Fixture: baseRepoWithRemote},
				{Name: "fixture: stage reviewed docs", Fixture: stageDocs("reviewed")},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: rememberLineage},
				{Name: "fixture: commit the approved delivery", Fixture: commitStaged("reviewed delivery")},
				{Name: "fixture: advance the parent on a disjoint path, then stage a merge of it (no commit)", Fixture: advanceParentDisjointlyAndStageMerge},
				{Name: "gate pre-commit still refuses the staged merge", Requires: validateCapability,
					Args: func(sandbox *Sandbox) ([]string, error) {
						return []string{"review", "validate", "--gate", "pre-commit", "--lineage", sandbox.Lineage, "--cwd", sandbox.Repo}, nil
					},
					After: assertStderrContains("scope-changed"), AbortOnBlock: true},
			},
		},
	}
}

// advanceParentDisjointlyAndStageMerge reproduces #2388's exact repro shape:
// branch off the pre-delivery parent commit, add content on a path disjoint
// from the reviewed delivery, then stage (never commit) a merge of that
// advance back into the branch carrying the approved delivery. HEAD stays at
// the approved candidate; only the index/working tree changes.
func advanceParentDisjointlyAndStageMerge(sandbox *Sandbox) error {
	parent, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "HEAD~1")
	if err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "branch", "advance-source", parent); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "checkout", "-q", "advance-source"); err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(sandbox.Repo, "docs", "unrelated.md"), "# unrelated\n\nparent advance, disjoint from the reviewed delivery.\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "-A"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "commit", "-qm", "advance the parent"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "checkout", "-q", "main"); err != nil {
		return err
	}
	// The merge is expected to succeed cleanly (disjoint paths); --no-commit
	// leaves it staged, matching #2388's reproduction exactly.
	_ = sandbox.git(sandbox.Repo, "merge", "-q", "--no-commit", "--no-ff", "advance-source")
	return nil
}
