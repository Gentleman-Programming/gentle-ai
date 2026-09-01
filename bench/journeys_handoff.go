package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var sddAttemptHandoffCapability = &Capability{
	Verb:  []string{"sdd-attempt", "handoff"},
	Probe: []string{"sdd-attempt", "handoff", "--destination-worktree=probe"},
}

func sddHandoffRuntimeRepo(sandbox *Sandbox) error {
	if err := sddRuntimeRepo(sandbox); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "reset", "--", "docs/attempt.md"); err != nil {
		return err
	}
	return os.Remove(filepath.Join(sandbox.Repo, "docs", "attempt.md"))
}
func sddHandoffDelegatedWorktree(sandbox *Sandbox) error {
	worktree := filepath.Join(sandbox.Root, "delegated-worktree")
	if err := sandbox.git(sandbox.Repo, "worktree", "add", "-q", "-b", "bench-handoff-delegated", worktree); err != nil {
		return err
	}
	sandbox.Scratch["delegated-worktree"] = worktree
	return nil
}

func sddHandoffAndSettleDelegatedWorktree(r *journeyRun) error {
	acquired := r.run([]string{
		"sdd-attempt", "acquire", "--cwd", r.sandbox.Repo, "--change", sddChange,
		"--request-id", "bench-handoff-acquire", "--work-unit", "delegated worktree proof",
		"--evidence-goal", "prove explicit handoff preserves begin provenance", "--max-attempts", "2", "--max-changed-lines", "20",
	}, false)
	var acquire sddCompactAttemptResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(acquired.Stdout)), &acquire); err != nil {
		return fmt.Errorf("parse handoff acquire: %w", err)
	}
	if acquired.ExitCode != 0 || acquire.State != "proceed" || acquire.Token == "" {
		return fmt.Errorf("handoff acquire = %#v exit=%d", acquire, acquired.ExitCode)
	}
	worktree := r.sandbox.Scratch["delegated-worktree"]
	settle := func(requestID string) Observation {
		return r.runAt(worktree, []string{
			"sdd-attempt", "settle", "--cwd", worktree, "--change", sddChange, "--token", acquire.Token,
			"--request-id", requestID, "--outcome", "failed", "--evidence-revision", sddFailedEvidence,
			"--diagnosis", "the benchmark settles the delegated worktree once", "--harness-disposition", "reused",
			"--cleanup-evidence", "workspace clean", "--process-evidence", "no stray processes",
		}, false)
	}
	before := settle("bench-handoff-before")
	var blocked struct {
		State  string `json:"state"`
		Reason string `json:"reason"`
		Exit   string `json:"exit"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(before.Stdout)), &blocked); err != nil {
		return fmt.Errorf("parse pre-handoff settle: %w", err)
	}
	if blocked.State != "blocked" || blocked.Reason != "worktree_mismatch" || blocked.Exit == "" || blocked.Detail == "" {
		return fmt.Errorf("pre-handoff settle = %#v", blocked)
	}
	status, err := readRuntimeStatus(r)
	if err != nil {
		return err
	}
	handoff := r.run([]string{
		"sdd-attempt", "handoff", "--cwd", r.sandbox.Repo, "--change", sddChange,
		"--expected-revision", status.Revision, "--request-id", "bench-handoff-a-to-b", "--destination-worktree", worktree,
	}, false)
	var handoffResult sddCompactAttemptResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(handoff.Stdout)), &handoffResult); err != nil {
		return fmt.Errorf("parse handoff result: %w", err)
	}
	if handoff.ExitCode != 0 || handoffResult.State != "proceed" {
		return fmt.Errorf("handoff result = %#v exit=%d", handoffResult, handoff.ExitCode)
	}
	if err := r.sandbox.write(filepath.Join(worktree, "docs", "delegated.md"), "delegated handoff\n"); err != nil {
		return err
	}
	if err := r.sandbox.git(worktree, "add", "docs/delegated.md"); err != nil {
		return err
	}
	after := settle("bench-handoff-after")
	var settled sddCompactAttemptResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(after.Stdout)), &settled); err != nil {
		return fmt.Errorf("parse post-handoff settle: %w", err)
	}
	if after.ExitCode != 0 || settled.State != "proceed" {
		return fmt.Errorf("post-handoff settle = %#v exit=%d", settled, after.ExitCode)
	}
	var final struct {
		ActiveAttempt any `json:"active_attempt"`
		Attempts      []struct {
			Outcome           string `json:"outcome"`
			ChangedLines      int    `json:"changed_lines"`
			BeginWorktree     string `json:"begin_worktree"`
			EffectiveWorktree string `json:"effective_worktree"`
			Handoff           any    `json:"handoff"`
		} `json:"attempts"`
	}
	if err := proveJSON(r.sandbox, &final, "sdd-attempt", "status", "--cwd", r.sandbox.Repo, "--change", sddChange); err != nil {
		return err
	}
	if final.ActiveAttempt != nil || len(final.Attempts) != 1 || final.Attempts[0].Outcome != "failed" ||
		final.Attempts[0].ChangedLines != 1 || final.Attempts[0].BeginWorktree != r.sandbox.Repo ||
		final.Attempts[0].EffectiveWorktree != worktree || final.Attempts[0].Handoff == nil {
		return fmt.Errorf("post-handoff runtime status = %#v", final)
	}
	return nil
}
func handoffJourneys() []Journey {
	return []Journey{
		{
			ID:     "j74-sdd-attempt-explicit-linked-worktree-handoff",
			Review: reviewOptedIn,
			Title:  "Delegated linked worktree: settlement refuses until one explicit handoff binds it",
			Source: "issue #2469: preserve Begin provenance while moving only effective execution worktree",
			Steps: []Step{
				{Name: "fixture: runtime repository with no candidate drift", Fixture: sddHandoffRuntimeRepo},
				{Name: "fixture: registered delegated linked worktree", Fixture: sddHandoffDelegatedWorktree},
				{Name: "acquire in A, refuse B, hand off A to B, and settle B once", Requires: sddAttemptHandoffCapability, Composite: sddHandoffAndSettleDelegatedWorktree},
			},
		},
	}
}
