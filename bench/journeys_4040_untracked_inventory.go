package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// untrackedRecoveryLoopCandidatePath is the untracked file born during the
// attempt after an acquire -- exactly the shape #4040's reporters hit: the
// attempt's own product appears as untracked bytes while it runs, and a later
// settlement must account for it.
const untrackedRecoveryLoopCandidatePath = "docs/4040-recovered.md"

// untrackedRecoveryLoopRetainedPath is the untracked file declared at acquire
// that forms the non-empty retained selection floor across settlement recovery.
const untrackedRecoveryLoopRetainedPath = "docs/4040-retained.md"

var sdd4040SettleCapability = &Capability{
	Verb:  []string{"sdd-attempt", "settle"},
	Flags: []string{"--untracked-scope", "--expected-untracked-inventory", "--intended-untracked"},
}

var untrackedRecoveryLoopDigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func untrackedInventoryDigest(paths ...string) string {
	sort.Strings(paths)
	hash := sha256.New()
	for _, s := range append([]string{"gentle-ai.intended-untracked-inventory/v1"}, paths...) {
		_, _ = fmt.Fprintf(hash, "%d\x00%s\x00", len(s), s)
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

// driveUntrackedInventoryRecoveryLoop reproduces issues #4040 and #4219:
// driving stale compact settle refusal through successful same-ID retry with
// a non-empty retained selection, without routing through Review STATUS.
func driveUntrackedInventoryRecoveryLoop(r *journeyRun) error {
	if err := r.sandbox.write(filepath.Join(r.sandbox.Repo, untrackedRecoveryLoopRetainedPath), "retained across recovery\n"); err != nil {
		return err
	}

	initialDigest := untrackedInventoryDigest(untrackedRecoveryLoopRetainedPath)
	if !untrackedRecoveryLoopDigestPattern.MatchString(initialDigest) {
		return fmt.Errorf("#4040 initial digest invalid: %q", initialDigest)
	}

	acquire := r.run([]string{
		"sdd-attempt", "acquire", "--cwd", r.sandbox.Repo, "--change", sddChange, "--request-id", "bench-4040-acquire",
		"--work-unit", "bench 4040 recovery objective", "--evidence-goal", "bench proves same-id untracked recovery",
		"--max-attempts", "2", "--max-changed-lines", "50",
		"--untracked-scope", "select", "--expected-untracked-inventory", initialDigest,
		"--intended-untracked", untrackedRecoveryLoopRetainedPath,
	}, false)
	var claimed sddCompactAttemptResult
	if err := json.Unmarshal([]byte(acquire.Stdout), &claimed); err != nil || acquire.ExitCode != 0 || claimed.State != "proceed" || claimed.Token == "" {
		return fmt.Errorf("#4040 acquire = %#v parse=%v exit=%d", claimed, err, acquire.ExitCode)
	}

	// The attempt's own product, born untracked while it runs.
	if err := r.sandbox.write(filepath.Join(r.sandbox.Repo, untrackedRecoveryLoopCandidatePath), "recovered by #4040\n"); err != nil {
		return err
	}

	const settleRequestID = "bench-4040-settle"
	settleBase := append([]string{
		"sdd-attempt", "settle", "--cwd", r.sandbox.Repo, "--change", sddChange, "--token", claimed.Token,
		"--request-id", settleRequestID, "--outcome", "failed", "--evidence-revision", sddFailedEvidence,
	}, sddTerminalEvidence...)

	// Undeclared settlement must refuse with blocked(undeclared_untracked).
	undeclared := r.run(settleBase, false)
	var undeclaredRefusal sddCompactAttemptResult
	if err := json.Unmarshal([]byte(undeclared.Stdout), &undeclaredRefusal); err != nil || undeclaredRefusal.State != "blocked" || undeclaredRefusal.Reason != "undeclared_untracked" {
		return fmt.Errorf("#4040 undeclared settle did not refuse with undeclared_untracked: %#v parse=%v exit=%d", undeclaredRefusal, err, undeclared.ExitCode)
	}

	// Stale digest computed while both files exist.
	staleDigest := untrackedInventoryDigest(untrackedRecoveryLoopRetainedPath, untrackedRecoveryLoopCandidatePath)
	if !untrackedRecoveryLoopDigestPattern.MatchString(staleDigest) || staleDigest == initialDigest {
		return fmt.Errorf("#4040 inventory with born file = %q, want distinct from %q", staleDigest, initialDigest)
	}

	// Remove the born-during file.
	if err := os.Remove(filepath.Join(r.sandbox.Repo, untrackedRecoveryLoopCandidatePath)); err != nil {
		return err
	}

	// Stale settle attempt with same request ID: must refuse, disclose fresh
	// digest, guide same-ID retry, and NOT route to Review STATUS.
	stale := r.run(append(append([]string{}, settleBase...),
		"--untracked-scope", "select", "--intended-untracked", untrackedRecoveryLoopRetainedPath,
		"--intended-untracked", untrackedRecoveryLoopCandidatePath,
		"--expected-untracked-inventory", staleDigest,
	), false)
	var staleRefusal sddCompactAttemptResult
	if err := json.Unmarshal([]byte(stale.Stdout), &staleRefusal); err != nil || staleRefusal.State != "blocked" || staleRefusal.Reason != "undeclared_untracked" {
		return fmt.Errorf("#4040 stale settle did not refuse with undeclared_untracked: %#v parse=%v exit=%d", staleRefusal, err, stale.ExitCode)
	}
	if strings.Contains(staleRefusal.Exit, "gentle-ai review status --next-transition") {
		return fmt.Errorf("#4040 stale settle refusal routed to Review STATUS: %s", staleRefusal.Exit)
	}

	// Extract the fresh digest directly from the compact refusal.
	const freshDigestFlagPrefix = "--expected-untracked-inventory="
	idx := strings.Index(staleRefusal.Exit, freshDigestFlagPrefix)
	if idx == -1 || len(staleRefusal.Exit) < idx+len(freshDigestFlagPrefix)+71 {
		return fmt.Errorf("#4040 stale settle refusal missing valid digest flag: %s", staleRefusal.Exit)
	}
	freshDigest := staleRefusal.Exit[idx+len(freshDigestFlagPrefix) : idx+len(freshDigestFlagPrefix)+71]
	if !untrackedRecoveryLoopDigestPattern.MatchString(freshDigest) || freshDigest != initialDigest {
		return fmt.Errorf("#4040 fresh digest from refusal = %q, want %q", freshDigest, initialDigest)
	}
	if !strings.Contains(staleRefusal.Exit, "retry `gentle-ai sdd-attempt settle` with the same --request-id") {
		return fmt.Errorf("#4040 stale settle refusal did not name same-ID retry: %s", staleRefusal.Exit)
	}

	// Ineligible path settle: must refuse without routing to Review STATUS.
	deleted := r.run(append(append([]string{}, settleBase...),
		"--untracked-scope", "select", "--intended-untracked", untrackedRecoveryLoopRetainedPath,
		"--intended-untracked", untrackedRecoveryLoopCandidatePath,
		"--expected-untracked-inventory", freshDigest,
	), false)
	var deletedRefusal sddCompactAttemptResult
	if err := json.Unmarshal([]byte(deleted.Stdout), &deletedRefusal); err != nil || deletedRefusal.State != "blocked" || deletedRefusal.Reason != "undeclared_untracked" {
		return fmt.Errorf("#4040 deleted path settle did not refuse: %#v parse=%v exit=%d", deletedRefusal, err, deleted.ExitCode)
	}
	if strings.Contains(deletedRefusal.Exit, "gentle-ai review status --next-transition") {
		return fmt.Errorf("#4040 deleted path settle refusal routed to Review STATUS: %s", deletedRefusal.Exit)
	}
	if !strings.Contains(deletedRefusal.Exit, untrackedRecoveryLoopCandidatePath) ||
		!strings.Contains(deletedRefusal.Exit, "only eligible paths") {
		return fmt.Errorf("#4040 deleted path refusal did not name ineligible path and eligible paths guidance: %s", deletedRefusal.Exit)
	}

	// Active attempt must remain preserved across refusals.
	status, err := readRuntimeStatus(r)
	if err != nil || status.ActiveAttempt == nil {
		return fmt.Errorf("#4040 refusals did not preserve active attempt: status=%#v err=%v", status, err)
	}

	// Successful same-ID compact settle retry with non-empty retained selection.
	recovered := r.run(append(append([]string{}, settleBase...),
		"--untracked-scope", "select", "--intended-untracked", untrackedRecoveryLoopRetainedPath,
		"--expected-untracked-inventory", freshDigest,
	), false)
	var recoveredResult sddCompactAttemptResult
	if err := json.Unmarshal([]byte(recovered.Stdout), &recoveredResult); err != nil || recovered.ExitCode != 0 || (recoveredResult.State != "proceed" && recoveredResult.State != "complete") {
		return fmt.Errorf("#4040 same-ID retry failed: %#v parse=%v exit=%d", recoveredResult, err, recovered.ExitCode)
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
	if final.ActiveAttempt != nil || len(final.Attempts) != 1 || final.Attempts[0].Outcome != "failed" ||
		len(final.Attempts[0].IntendedUntracked) != 1 || final.Attempts[0].IntendedUntracked[0] != untrackedRecoveryLoopRetainedPath {
		return fmt.Errorf("#4040 recovered settle did not account retained untracked selection: %#v", final)
	}

	// Assert that the recovery flow does not consult Review STATUS.
	for _, record := range r.accumulator.records {
		if len(record.Args) >= 2 && record.Args[0] == "review" && record.Args[1] == "status" {
			return fmt.Errorf("#4040 recovery consulted Review STATUS: %v", record.Args)
		}
	}
	return nil
}

func untrackedInventoryRecoveryLoopJourneys() []Journey {
	return []Journey{{
		ID:     "j4040-untracked-inventory-recovery-loop",
		Review: reviewUntouched,
		Title:  "#4040/#4219: deleting a born-during file recovers active settlement through same-ID retry",
		Source: "issues #4040 and #4219: a stale settlement declaration after its born-during eligible file disappears discloses the current eligible_untracked_inventory digest; retrying compact settle with the same settle request ID retains the selection floor and clears the attempt",
		Steps: []Step{
			{Name: "fixture: runtime repository", Fixture: sddRuntimeRepo},
			{Name: "acquire with retained selection, born-during file, deletion, stale refusals preserve attempt, and same-ID retry closes it", Requires: sdd4040SettleCapability, Composite: driveUntrackedInventoryRecoveryLoop},
		},
	}}
}
