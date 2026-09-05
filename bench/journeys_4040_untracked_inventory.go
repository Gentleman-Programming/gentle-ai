package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// untrackedRecoveryLoopCandidatePath is the untracked file born during the
// attempt after a clean, undeclared begin -- exactly the shape #4040's
// reporters hit: the attempt's own product appears as untracked bytes while
// it runs, and a later settlement must account for it.
const untrackedRecoveryLoopCandidatePath = "docs/4040-recovered.md"

var sdd4040FinishCapability = &Capability{
	Verb:  []string{"sdd-attempt", "finish"},
	Flags: []string{"--untracked-scope", "--expected-untracked-inventory", "--intended-untracked"},
}

var untrackedRecoveryLoopDigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// untrackedRecoveryLoopStatusRoute is the recovery route the refusals name.
const untrackedRecoveryLoopStatusRoute = "gentle-ai review status --next-transition"

// driveUntrackedInventoryRecoveryLoop reproduces issue #4040. Refusals from
// intendedUntrackedScopeForTarget and the runtime ledger name
// `gentle-ai review status --next-transition` as the recovery route for the
// digest they demand, but before this fix that route only ever published the
// digest inside next_transition.collect.inputs[].arguments -- a slot that
// stops firing the moment RDD is disabled (this journey deliberately leaves
// RDD at its default disabled state via Review: reviewUntouched) or the
// caller has already declared. The fix publishes eligible_untracked_inventory
// unconditionally at the STATUS top level instead (design decision 2), which
// this journey reads and feeds straight back into a successful
// `sdd-attempt finish`.
func driveUntrackedInventoryRecoveryLoop(r *journeyRun) error {
	status, err := readRuntimeStatus(r)
	if err != nil {
		return err
	}
	if begin := r.run(sddAttemptArgs(r, "begin", status.Revision, "bench-4040-begin", sddObjective...), false); begin.ExitCode != 0 {
		return fmt.Errorf("#4040 clean begin exit=%d: %s", begin.ExitCode, firstLine(begin.Stderr))
	}

	// The attempt's own product, born untracked while it runs.
	if err := r.sandbox.write(filepath.Join(r.sandbox.Repo, untrackedRecoveryLoopCandidatePath), "recovered by #4040\n"); err != nil {
		return err
	}

	// #4040's entry point, driven before the recovery route is read: a
	// settlement that declares nothing must refuse, and the refusal must be
	// the ledger's -- the one that names the unaccounted candidate and the
	// exact rerun flags. Before Fix B the finish/settle CLI preflight refused
	// first with a message naming no digest at all, hiding this one. Reading
	// STATUS without first proving this refusal would still pass if the
	// product stopped demanding a declaration.
	if status, err = readRuntimeStatus(r); err != nil {
		return err
	}
	undeclared := r.run(sddAttemptArgs(r, "finish", status.Revision, "bench-4040-undeclared-finish",
		append([]string{"--outcome", "failed", "--evidence-revision", sddFailedEvidence}, sddTerminalEvidence...)...), false)
	if undeclared.ExitCode == 0 {
		return fmt.Errorf("#4040 undeclared finish accepted an unaccounted untracked candidate; it must refuse")
	}
	if !strings.Contains(undeclared.Stderr, untrackedRecoveryLoopCandidatePath) ||
		!strings.Contains(undeclared.Stderr, "--expected-untracked-inventory=") {
		return fmt.Errorf("#4040 undeclared finish refusal named neither the candidate nor the exact rerun: %s", firstLine(undeclared.Stderr))
	}

	// Read the recovery route while the file exists, then remove it before the
	// operator can act. The previously current digest is now stale; its refusal
	// must disclose the canonical empty inventory that can still close the
	// active attempt.
	review, err := readStatusForContract(r, reviewContractV2)
	if err != nil {
		return err
	}
	if review.Schema != statusSchemaV7 {
		return fmt.Errorf("#4040 recovery STATUS schema = %q, want %q", review.Schema, statusSchemaV7)
	}
	if !untrackedRecoveryLoopDigestPattern.MatchString(review.EligibleUntrackedInventory) {
		return fmt.Errorf("#4040 recovery STATUS did not publish a top-level eligible_untracked_inventory digest: %q", review.EligibleUntrackedInventory)
	}
	if err := os.Remove(filepath.Join(r.sandbox.Repo, untrackedRecoveryLoopCandidatePath)); err != nil {
		return err
	}
	refreshed, err := readStatusForContract(r, reviewContractV2)
	if err != nil {
		return err
	}
	if !untrackedRecoveryLoopDigestPattern.MatchString(refreshed.EligibleUntrackedInventory) ||
		refreshed.EligibleUntrackedInventory == review.EligibleUntrackedInventory {
		return fmt.Errorf("#4040 deleted-file STATUS digest = %q, want a new canonical digest distinct from %q", refreshed.EligibleUntrackedInventory, review.EligibleUntrackedInventory)
	}
	if status, err = readRuntimeStatus(r); err != nil {
		return err
	}
	stale := r.run(sddAttemptArgs(r, "finish", status.Revision, "bench-4040-stale-finish",
		append([]string{
			"--outcome", "interrupted", "--untracked-scope", "select",
			"--intended-untracked", untrackedRecoveryLoopCandidatePath,
			"--expected-untracked-inventory", review.EligibleUntrackedInventory,
		}, sddTerminalEvidence...)...), false)
	if stale.ExitCode == 0 {
		return fmt.Errorf("#4040 finish accepted a stale deleted-file inventory digest; it must refuse")
	}
	if !strings.Contains(stale.Stderr, untrackedRecoveryLoopStatusRoute) ||
		!strings.Contains(stale.Stderr, refreshed.EligibleUntrackedInventory) ||
		!strings.Contains(stale.Stderr, "--expected-untracked-inventory="+refreshed.EligibleUntrackedInventory) {
		return fmt.Errorf("#4040 stale-digest refusal did not disclose recovery route and current digest: %s", firstLine(stale.Stderr))
	}
	if status, err = readRuntimeStatus(r); err != nil || status.ActiveAttempt == nil {
		return fmt.Errorf("#4040 stale-digest refusal did not preserve the active attempt: status=%#v err=%v", status, err)
	}

	deleted := r.run(sddAttemptArgs(r, "finish", status.Revision, "bench-4040-deleted-selection",
		append([]string{
			"--outcome", "interrupted", "--untracked-scope", "select",
			"--intended-untracked", untrackedRecoveryLoopCandidatePath,
			"--expected-untracked-inventory", refreshed.EligibleUntrackedInventory,
		}, sddTerminalEvidence...)...), false)
	if deleted.ExitCode == 0 || !strings.Contains(deleted.Stderr, untrackedRecoveryLoopCandidatePath) {
		return fmt.Errorf("#4040 deleted-file selection did not refuse with its ineligible path: %s", firstLine(deleted.Stderr))
	}
	if status, err = readRuntimeStatus(r); err != nil || status.ActiveAttempt == nil {
		return fmt.Errorf("#4040 deleted-file selection did not preserve the active attempt: status=%#v err=%v", status, err)
	}

	finished := r.run(sddAttemptArgs(r, "finish", status.Revision, "bench-4040-finish",
		append([]string{
			"--outcome", "interrupted", "--untracked-scope", "exclude",
			"--expected-untracked-inventory", refreshed.EligibleUntrackedInventory,
		}, sddTerminalEvidence...)...), false)
	if finished.ExitCode != 0 {
		return fmt.Errorf("#4040 refreshed interrupted finish exit=%d: %s", finished.ExitCode, firstLine(finished.Stderr))
	}

	var final struct {
		ActiveAttempt any `json:"active_attempt"`
		Attempts      []struct {
			Outcome           string   `json:"outcome"`
			IntendedUntracked []string `json:"intended_untracked"`
		} `json:"attempts"`
	}
	if err := proveJSON(r.sandbox, &final, "sdd-attempt", "status", "--cwd", r.sandbox.Repo, "--change", sddChange); err != nil {
		return err
	}
	if final.ActiveAttempt != nil || len(final.Attempts) != 1 || final.Attempts[0].Outcome != "interrupted" || len(final.Attempts[0].IntendedUntracked) != 0 {
		return fmt.Errorf("#4040 refreshed finish did not clear the active attempt without selecting the deleted file: %#v", final)
	}
	return nil
}

func untrackedInventoryRecoveryLoopJourneys() []Journey {
	return []Journey{{
		ID:     "j4040-untracked-inventory-recovery-loop",
		Review: reviewUntouched,
		Title:  "#4040: deleting a born-during file does not dead-end its active settlement",
		Source: "issue #4040: a stale settlement declaration after its born-during eligible file disappears must disclose the current top-level eligible_untracked_inventory digest; retrying with that digest can exclude the deleted path and clear the attempt even with RDD disabled",
		Steps: []Step{
			{Name: "fixture: runtime repository", Fixture: sddRuntimeRepo},
			{Name: "clean begin, born-during untracked candidate, deletion, stale and deleted-path refusals preserve the attempt, and the refreshed STATUS digest closes it", Requires: sdd4040FinishCapability, Composite: driveUntrackedInventoryRecoveryLoop},
		},
	}}
}
