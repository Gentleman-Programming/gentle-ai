package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var sddBornDuringUntrackedCapability = &Capability{
	Verb:  []string{"sdd-attempt", "settle"},
	Flags: []string{"--untracked-scope", "--expected-untracked-inventory", "--intended-untracked"},
}

var sddSelectedUntrackedCapability = &Capability{
	Verb:  []string{"sdd-attempt", "acquire"},
	Flags: []string{"--untracked-scope", "--expected-untracked-inventory", "--intended-untracked"},
}

func sddSelectedUntrackedCandidate(sandbox *Sandbox) error {
	return sandbox.write(filepath.Join(sandbox.Repo, "docs", "selected.md"), "initial selected candidate\n")
}

func driveSelectedUntrackedSDDAttempt(r *journeyRun) error {
	selection, err := readStatusForContract(r, reviewContractV2)
	if err != nil {
		return err
	}
	digest := selection.argument("expected_untracked_inventory")
	if digest == "" {
		return fmt.Errorf("review status did not publish the canonical untracked inventory: %+v", selection.NextTransition)
	}
	acquire := r.run([]string{
		"sdd-attempt", "acquire", "--cwd", r.sandbox.Repo, "--change", sddChange, "--request-id", "bench-selected-acquire",
		"--work-unit", "selected untracked lifecycle", "--evidence-goal", "account only declared untracked bytes",
		"--max-attempts", "2", "--max-changed-lines", "20", "--untracked-scope", "select",
		"--expected-untracked-inventory", digest, "--intended-untracked", "docs/selected.md",
	}, false)
	var claimed sddCompactAttemptResult
	if err := json.Unmarshal([]byte(acquire.Stdout), &claimed); err != nil || acquire.ExitCode != 0 || claimed.State != "proceed" || claimed.Token == "" {
		return fmt.Errorf("selected acquire = %#v parse=%v exit=%d", claimed, err, acquire.ExitCode)
	}
	if err := r.sandbox.write(filepath.Join(r.sandbox.Repo, "docs", "attempt.md"), "tracked change\n"); err != nil {
		return err
	}
	if err := r.sandbox.write(filepath.Join(r.sandbox.Repo, "docs", "selected.md"), "corrected selected candidate\n"); err != nil {
		return err
	}
	settled := r.run(append([]string{
		"sdd-attempt", "settle", "--cwd", r.sandbox.Repo, "--change", sddChange, "--token", claimed.Token,
		"--request-id", "bench-selected-settle", "--outcome", "failed", "--evidence-revision", sddFailedEvidence,
	}, sddTerminalEvidence...), false)
	var result sddCompactAttemptResult
	if err := json.Unmarshal([]byte(settled.Stdout), &result); err != nil || settled.ExitCode != 0 || result.State != "proceed" {
		return fmt.Errorf("selected settle = %#v parse=%v exit=%d", result, err, settled.ExitCode)
	}
	var status struct {
		Revision string `json:"revision"`
		Attempts []struct {
			ChangedLines      int      `json:"changed_lines"`
			IntendedUntracked []string `json:"intended_untracked"`
		} `json:"attempts"`
	}
	if err := proveJSON(r.sandbox, &status, "sdd-attempt", "status", "--cwd", r.sandbox.Repo, "--change", sddChange); err != nil {
		return err
	}
	if len(status.Attempts) != 1 || status.Attempts[0].ChangedLines != 6 || len(status.Attempts[0].IntendedUntracked) != 1 || status.Attempts[0].IntendedUntracked[0] != "docs/selected.md" {
		return fmt.Errorf("selected SDD lifecycle status = %#v", status)
	}
	rescoped := r.run([]string{
		"sdd-attempt", "rescope", "--cwd", r.sandbox.Repo, "--change", sddChange, "--expected-revision", status.Revision,
		"--request-id", "bench-selected-rescope", "--work-unit", "selected untracked continuation",
		"--evidence-goal", "prove the rescope successor preserves selected bytes", "--max-attempts", "2", "--max-changed-lines", "20",
		"--reason", "maintainer narrowed the failed selected-untracked objective", "--actor", "bench",
	}, false)
	if rescoped.ExitCode != 0 {
		return fmt.Errorf("selected rescope = exit=%d stderr=%s", rescoped.ExitCode, firstLine(rescoped.Stderr))
	}
	admission := r.run([]string{
		"sdd-attempt", "status", "--cwd", r.sandbox.Repo, "--change", sddChange,
		"--work-unit", "selected untracked continuation", "--evidence-goal", "prove the rescope successor preserves selected bytes",
		"--max-attempts", "2", "--max-changed-lines", "20",
	}, false)
	var continuationStatus struct {
		BlockedReason string `json:"blocked_reason"`
		BlockedExit   string `json:"blocked_exit"`
	}
	if err := json.Unmarshal([]byte(admission.Stdout), &continuationStatus); err != nil || admission.ExitCode != 0 || continuationStatus.BlockedReason != "" || continuationStatus.BlockedExit != "" {
		return fmt.Errorf("declaration-free selected rescope status = %#v parse=%v exit=%d", continuationStatus, err, admission.ExitCode)
	}
	continued := r.run([]string{
		"sdd-attempt", "acquire", "--cwd", r.sandbox.Repo, "--change", sddChange, "--request-id", "bench-selected-acquire-successor",
		"--work-unit", "selected untracked continuation", "--evidence-goal", "prove the rescope successor preserves selected bytes",
		"--max-attempts", "2", "--max-changed-lines", "20",
	}, false)
	var successor sddCompactAttemptResult
	if err := json.Unmarshal([]byte(continued.Stdout), &successor); err != nil || continued.ExitCode != 0 || successor.State != "proceed" || successor.Token == "" {
		return fmt.Errorf("declaration-free selected rescope acquire = %#v parse=%v exit=%d", successor, err, continued.ExitCode)
	}
	var continuedStatus struct {
		ActiveAttempt *struct {
			IntendedUntracked []string `json:"intended_untracked"`
		} `json:"active_attempt"`
	}
	if err := proveJSON(r.sandbox, &continuedStatus, "sdd-attempt", "status", "--cwd", r.sandbox.Repo, "--change", sddChange); err != nil {
		return err
	}
	if continuedStatus.ActiveAttempt == nil || len(continuedStatus.ActiveAttempt.IntendedUntracked) != 1 || continuedStatus.ActiveAttempt.IntendedUntracked[0] != "docs/selected.md" {
		return fmt.Errorf("selected rescope successor swept untracked paths: %#v", continuedStatus)
	}
	return nil
}

// driveBornDuringUntrackedSDDAttempt drives #4090 end to end. It acquires two
// canonicalized initial selections, creates an additional candidate while the
// attempt runs, and proves a stale compact settlement returns the retained floor
// and current inventory needed to retry without touching Review.
const bornDuringStaleInventory = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

func driveBornDuringUntrackedSDDAttempt(r *journeyRun) error {
	const (
		bornPath      = "docs/born.md"
		bornContents  = "born during the attempt\n"
		retainedAPath = "docs/retained-a.md"
		retainedZPath = "docs/retained-z.md"
	)
	retainedFloor := []string{retainedAPath, retainedZPath}
	for path, contents := range map[string]string{
		retainedAPath: "retained initial selection a\n",
		retainedZPath: "retained initial selection z\n",
	} {
		if err := r.sandbox.write(filepath.Join(r.sandbox.Repo, path), contents); err != nil {
			return err
		}
	}
	initialInventory, err := bornDuringUntrackedInventory(r.sandbox)
	if err != nil {
		return err
	}
	modeBefore, err := bornDuringReviewMode(r)
	if err != nil {
		return err
	}
	acquire := r.run([]string{
		"sdd-attempt", "acquire", "--cwd", r.sandbox.Repo, "--change", sddChange, "--request-id", "bench-born-acquire",
		"--work-unit", "born during lifecycle", "--evidence-goal", "recover a stale compact settlement without review authority",
		"--max-attempts", "2", "--max-changed-lines", "20", "--untracked-scope", "select",
		"--expected-untracked-inventory", initialInventory, "--intended-untracked", retainedZPath, "--intended-untracked", retainedAPath,
	}, false)
	var claimed sddCompactAttemptResult
	if err := json.Unmarshal([]byte(acquire.Stdout), &claimed); err != nil || acquire.ExitCode != 0 || claimed.State != "proceed" || claimed.Token == "" {
		return fmt.Errorf("initial selected acquire = %#v parse=%v exit=%d", claimed, err, acquire.ExitCode)
	}
	if err := r.sandbox.write(filepath.Join(r.sandbox.Repo, bornPath), bornContents); err != nil {
		return err
	}
	currentInventory, err := bornDuringUntrackedInventory(r.sandbox)
	if err != nil {
		return err
	}
	settle := append([]string{
		"sdd-attempt", "settle", "--cwd", r.sandbox.Repo, "--change", sddChange, "--token", claimed.Token,
		"--request-id", "bench-born-settle", "--outcome", "passed", "--evidence-revision", sddCorrectedEvidence,
	}, sddTerminalEvidence...)
	stale := r.run(append(append([]string{}, settle...),
		"--untracked-scope", "select", "--expected-untracked-inventory", bornDuringStaleInventory, "--intended-untracked", bornPath), false)
	var raw map[string]json.RawMessage
	var refusal struct {
		State    string `json:"state"`
		Reason   string `json:"reason"`
		Recovery *struct {
			ExpectedUntrackedInventory string   `json:"expected_untracked_inventory"`
			RetainedIntendedUntracked  []string `json:"retained_intended_untracked"`
		} `json:"recovery"`
	}
	if err := json.Unmarshal([]byte(stale.Stdout), &raw); err != nil {
		return fmt.Errorf("parse stale born-during compact settlement: %w", err)
	}
	recoveryRaw, recoveryPresent := raw["recovery"]
	var recoveryFields map[string]json.RawMessage
	if recoveryPresent {
		if err := json.Unmarshal(recoveryRaw, &recoveryFields); err != nil {
			return fmt.Errorf("parse stale born-during compact recovery: %w", err)
		}
	}
	_, retainedPresent := recoveryFields["retained_intended_untracked"]
	if err := json.Unmarshal([]byte(stale.Stdout), &refusal); err != nil || stale.ExitCode != 0 ||
		refusal.State != "blocked" || refusal.Reason != "undeclared_untracked" || refusal.Recovery == nil ||
		!recoveryPresent || !retainedPresent || refusal.Recovery.ExpectedUntrackedInventory != currentInventory ||
		len(refusal.Recovery.RetainedIntendedUntracked) != len(retainedFloor) {
		return fmt.Errorf("stale born-during compact settlement = %#v recovery-present=%t retained-present=%t parse=%v exit=%d", refusal, recoveryPresent, retainedPresent, err, stale.ExitCode)
	}
	for index, path := range retainedFloor {
		if refusal.Recovery.RetainedIntendedUntracked[index] != path {
			return fmt.Errorf("stale born-during retained floor = %#v, want %#v", refusal.Recovery.RetainedIntendedUntracked, retainedFloor)
		}
	}

	// The compact refusal owns the canonical retained floor and current digest.
	// The retry preserves that floor, then explicitly adds its born-during path.
	intended := append([]string{}, refusal.Recovery.RetainedIntendedUntracked...)
	intended = append(intended, bornPath)
	settledIntended := []string{bornPath, retainedAPath, retainedZPath}
	retry := append(append([]string{}, settle...), "--untracked-scope", "select",
		"--expected-untracked-inventory", refusal.Recovery.ExpectedUntrackedInventory)
	for _, path := range intended {
		retry = append(retry, "--intended-untracked", path)
	}
	settled := r.run(retry, false)
	var result sddCompactAttemptResult
	if err := json.Unmarshal([]byte(settled.Stdout), &result); err != nil || settled.ExitCode != 0 || result.State != "complete" {
		return fmt.Errorf("same-ID native recovery settlement = %#v parse=%v exit=%d", result, err, settled.ExitCode)
	}

	var status struct {
		ActiveAttempt any `json:"active_attempt"`
		Attempts      []struct {
			ChangedLines        int      `json:"changed_lines"`
			FinishCandidateTree string   `json:"finish_candidate_tree"`
			IntendedUntracked   []string `json:"intended_untracked"`
		} `json:"attempts"`
	}
	if err := proveJSON(r.sandbox, &status, "sdd-attempt", "status", "--cwd", r.sandbox.Repo, "--change", sddChange); err != nil {
		return err
	}
	if status.ActiveAttempt != nil || len(status.Attempts) != 1 || status.Attempts[0].ChangedLines != 1 ||
		status.Attempts[0].FinishCandidateTree == "" || len(status.Attempts[0].IntendedUntracked) != len(settledIntended) {
		return fmt.Errorf("born-during native recovery accounting = %#v", status)
	}
	for index, path := range settledIntended {
		if status.Attempts[0].IntendedUntracked[index] != path {
			return fmt.Errorf("born-during native recovery intended paths = %#v, want %#v", status.Attempts[0].IntendedUntracked, settledIntended)
		}
	}
	contents, err := os.ReadFile(filepath.Join(r.sandbox.Repo, bornPath))
	if err != nil || string(contents) != bornContents {
		return fmt.Errorf("born-during candidate was not preserved: contents=%q err=%v", contents, err)
	}
	modeAfter, err := bornDuringReviewMode(r)
	if err != nil || modeAfter != modeBefore {
		return fmt.Errorf("born-during SDD recovery changed review mode from %q to %q: %v", modeBefore, modeAfter, err)
	}
	return nil
}

func bornDuringUntrackedInventory(sandbox *Sandbox) (string, error) {
	output, err := gitOut(sandbox, sandbox.Repo, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return "", err
	}
	paths := []string{}
	if output != "" {
		paths = strings.Split(output, "\n")
		sort.Strings(paths)
	}
	hash := sha256.New()
	for _, value := range append([]string{"gentle-ai.intended-untracked-inventory/v1"}, paths...) {
		_, _ = fmt.Fprintf(hash, "%d\x00%s\x00", len(value), value)
	}
	return "sha256:" + fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func bornDuringReviewMode(r *journeyRun) (string, error) {
	observation := r.run([]string{"review", "mode", "status", "--cwd", r.sandbox.Repo, "--json"}, false)
	var mode struct {
		Status struct {
			Effective string `json:"effective"`
		} `json:"status"`
	}
	if err := json.Unmarshal([]byte(observation.Stdout), &mode); err != nil || observation.ExitCode != 0 || mode.Status.Effective == "" {
		return "", fmt.Errorf("review mode observation = %#v parse=%v", observation, err)
	}
	return mode.Status.Effective, nil
}

func selectedUntrackedSDDJourneys() []Journey {
	return []Journey{{
		ID:     "j84-sdd-attempt-selected-untracked-lifecycle",
		Title:  "SDD attempt: selected untracked scope survives a zero-drift rescope continuation",
		Source: "issues #2716 and #3801: explicit selected scope remains provenance and a fresh rescope successor must continue it without sweeping workspace files",
		Steps: []Step{
			{Name: "fixture: runtime repository", Fixture: sddRuntimeRepo},
			{Name: "fixture: selected untracked candidate", Fixture: sddSelectedUntrackedCandidate},
			{Name: "acquire, settle, and prove selected-path accounting", Requires: sddSelectedUntrackedCapability, Composite: driveSelectedUntrackedSDDAttempt},
		},
	}, {
		ID:     "j99-sdd-attempt-born-during-untracked-lifecycle",
		Review: reviewUntouched,
		Title:  "SDD attempt: native compact recovery settles files born during an attempt",
		Source: "#4090 Slice B: native compact recovery, not Review STATUS, returns the stale-settlement digest and retained selection floor",
		Steps: []Step{
			{Name: "fixture: runtime repository", Fixture: sddRuntimeRepo},
			{Name: "acquire clean, create a born-during candidate, recover stale compact settlement, and preserve review mode", Requires: sddBornDuringUntrackedCapability, Composite: driveBornDuringUntrackedSDDAttempt},
		},
	}}
}
