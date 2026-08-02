package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// This file extends the corpus with the SDD remediation successor cycle and the
// two surfaces around it that nothing else in this benchmark had ever driven:
// the kill switch pointed at SDD, and the recovery guard rails as an operator
// meets them.
//
// It is the corpus's largest blind spot closed. A community tester found a hard
// deadlock on this path that no internal audit had caught; two of its blocks
// were fixed by hand and neither was pinned by anything that runs in a loop.
// Until these journeys existed the benchmark reported zero out-of-band blocks
// and zero dead ends for a surface it simply never looked at, which is the same
// way four macOS defects shipped.
//
// The five defect shapes named in journeys_edge.go still apply and every journey
// here names the ones it stresses. Two more properties are specific to this file:
//
//   - The state these journeys need cannot be built with git alone. An attempt
//     ordinal, a populated review binding and the leaf/non-leaf topology of a
//     lineage all live inside the product, so every fixture and every composite
//     PROVES them by reading them back out of the product with Sandbox.readBack
//     — uncounted, because an assertion is instrumentation — and fails the
//     journey loudly when the state is not what it claims. A journey that set its
//     own premise up wrongly and then reported a clean number would be worse than
//     no journey at all.
//   - Several verbs here cannot be capability-probed with `--help`, because the
//     `sdd-attempt` operations parse `--help` as an ordinary flag and reject it
//     with the same words a missing flag produces. Those steps declare
//     Capability.Probe instead: an argv that names the flag under test and can
//     only fail on state.

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

// sddChange is the OpenSpec change every journey in this file drives. The
// runtime authority is per-change, so one name keeps the fixtures comparable.
const sddChange = "bench-change"

// The evidence revisions are fixed strings so two consecutive runs produce
// identical bytes. What they hash is irrelevant to the runtime ledger: it stores
// the revision, it never recomputes it.
const (
	sddFailedEvidence    = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	sddCorrectedEvidence = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// sddSuccessorLineage is the name a recovery successor is minted under. The CLI
// help is explicit that the successor's name is the operator's to choose, so the
// benchmark chooses one and keeps it stable.
const sddSuccessorLineage = "review-sdd-successor"

// ---------------------------------------------------------------------------
// Envelopes
// ---------------------------------------------------------------------------

// sddRuntimeStatus is the subset of `sdd-attempt status` these journeys read.
// Unknown fields are ignored so an older or newer envelope still parses.
type sddRuntimeStatus struct {
	Change        string `json:"change"`
	Revision      string `json:"revision"`
	ActiveAttempt *struct {
		Ordinal            int    `json:"ordinal"`
		BeginCandidateTree string `json:"begin_candidate_tree"`
		Outcome            string `json:"outcome"`
	} `json:"active_attempt"`
	Attempts []struct {
		Ordinal          int    `json:"ordinal"`
		Outcome          string `json:"outcome"`
		EvidenceRevision string `json:"evidence_revision"`
	} `json:"attempts"`
	EvidenceRevision string `json:"evidence_revision"`
	BindingRevision  string `json:"binding_revision"`
	Binding          *struct {
		Change  string `json:"change"`
		Lineage string `json:"lineage"`
	} `json:"binding"`
	NextAction string `json:"next_action"`
	Complete   bool   `json:"complete"`
}

// sddStatusV1 is the subset of `sdd-status --json` the kill-switch journeys read.
type sddStatusV1 struct {
	NextRecommended string `json:"nextRecommended"`
	Dependencies    struct {
		Apply   string `json:"apply"`
		Verify  string `json:"verify"`
		Archive string `json:"archive"`
	} `json:"dependencies"`
	ReviewGate *struct {
		Result   string `json:"result"`
		Reason   string `json:"reason"`
		Delivery string `json:"delivery"`
	} `json:"reviewGate"`
	BlockedReasons []string `json:"blockedReasons"`
	TaskProgress   struct {
		Total       int  `json:"total"`
		Completed   int  `json:"completed"`
		AllComplete bool `json:"allComplete"`
	} `json:"taskProgress"`
}

// gateResult is the subset of a lifecycle gate envelope the proofs read.
type gateResult struct {
	Result  string `json:"result"`
	Allowed bool   `json:"allowed"`
}

// ---------------------------------------------------------------------------
// Reading the product back — uncounted proofs
// ---------------------------------------------------------------------------

// proveJSON runs one uncounted product invocation and decodes its stdout. A
// non-JSON answer is reported with the product's own first stderr line, because
// "the proof could not parse" and "the product refused" are different failures
// and a fixture that confuses them teaches nothing.
func proveJSON(sandbox *Sandbox, target any, args ...string) error {
	observation := sandbox.readBack(args...)
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), target); err != nil {
		return fmt.Errorf("%s: %w (stderr: %s)", strings.Join(args, " "), err, firstLine(observation.Stderr))
	}
	return nil
}

// proveRuntime reads the SDD runtime authority back out of the product.
func proveRuntime(sandbox *Sandbox) (sddRuntimeStatus, error) {
	var status sddRuntimeStatus
	err := proveJSON(sandbox, &status, "sdd-attempt", "status", "--cwd", sandbox.Repo, "--change", sddChange)
	return status, err
}

// proveAuthorities reads every review lineage back out of the product.
func proveAuthorities(sandbox *Sandbox) (authorityHead, error) {
	var head authorityHead
	err := proveJSON(sandbox, &head, "review", "status", "--cwd", sandbox.Repo)
	return head, err
}

// provePostApplyAllows reports whether the live post-apply gate currently allows.
// It is the mechanical definition of "this lineage is the compact recovery leaf
// and it governs these bytes", which is the whole difference between the two
// branches of the remediation refusal.
func provePostApplyAllows(sandbox *Sandbox) bool {
	var result gateResult
	if err := proveJSON(sandbox, &result, "review", "validate", "--cwd", sandbox.Repo, "--gate", "post-apply"); err != nil {
		return false
	}
	return result.Allowed && result.Result == "allow"
}

// proveActiveAttempt asserts the runtime really is where a journey says it is.
func proveActiveAttempt(sandbox *Sandbox, ordinal int, evidenceRevision string) error {
	status, err := proveRuntime(sandbox)
	if err != nil {
		return err
	}
	if status.Change != sddChange {
		return fmt.Errorf("runtime authority reports change %q, want %q", status.Change, sddChange)
	}
	if status.ActiveAttempt == nil {
		return fmt.Errorf("fixture claims an active attempt on ordinal %d but the runtime has none", ordinal)
	}
	if status.ActiveAttempt.Ordinal != ordinal || status.ActiveAttempt.Outcome != "running" {
		return fmt.Errorf("fixture claims a running ordinal %d but the runtime reports ordinal %d, outcome %q",
			ordinal, status.ActiveAttempt.Ordinal, status.ActiveAttempt.Outcome)
	}
	if status.EvidenceRevision != evidenceRevision {
		return fmt.Errorf("fixture claims the failed evidence %q but the runtime carries %q",
			evidenceRevision, status.EvidenceRevision)
	}
	return nil
}

// proveBinding asserts the runtime carries a populated binding for this change.
func proveBinding(sandbox *Sandbox, lineage string) error {
	status, err := proveRuntime(sandbox)
	if err != nil {
		return err
	}
	if status.BindingRevision == "" || status.Binding == nil {
		return errors.New("fixture claims a populated review binding but the runtime publishes none")
	}
	if status.Binding.Lineage != lineage {
		return fmt.Errorf("fixture claims the binding names %q but the runtime publishes %q", lineage, status.Binding.Lineage)
	}
	if status.Binding.Change != sddChange {
		return fmt.Errorf("binding names change %q, want %q", status.Binding.Change, sddChange)
	}
	return nil
}

// proveLeafTopology asserts the bound lineage IS the compact recovery leaf: it
// is the only authority and its live post-apply gate allows. That is the exact
// condition the product's read-only probe checks before it names the
// self-successor finish, so a journey claiming the leaf branch must hold it.
func proveLeafTopology(sandbox *Sandbox, lineage string) error {
	head, err := proveAuthorities(sandbox)
	if err != nil {
		return err
	}
	if len(head.Entries) != 1 || head.Entries[0].LineageID != lineage || head.Entries[0].State != "approved" {
		return fmt.Errorf("fixture claims one approved leaf %q but review status lists %d entries: %+v",
			lineage, len(head.Entries), head.Entries)
	}
	if !provePostApplyAllows(sandbox) {
		return errors.New("fixture claims the bound lineage is the leaf but its post-apply gate does not allow")
	}
	return nil
}

// proveNonLeafTopology asserts the opposite shape: a real recovery successor now
// exists, so the bound lineage is no longer the leaf and its gate stops allowing.
func proveNonLeafTopology(sandbox *Sandbox, bound, successor string) error {
	head, err := proveAuthorities(sandbox)
	if err != nil {
		return err
	}
	if _, _, ok := head.entry(bound); !ok {
		return fmt.Errorf("fixture claims the bound lineage %q survives recovery but review status does not list it", bound)
	}
	found := false
	for _, entry := range head.Entries {
		if entry.LineageID == successor {
			found = true
			if entry.State != "reviewing" {
				return fmt.Errorf("fixture claims a pristine successor but %q is %q", successor, entry.State)
			}
		}
	}
	if !found {
		return fmt.Errorf("fixture claims a minted successor %q but review status does not list it", successor)
	}
	if provePostApplyAllows(sandbox) {
		return errors.New("fixture claims a non-leaf bound lineage but its post-apply gate still allows")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

// sddChangeRoot is where the OpenSpec change lives inside the repository.
func sddChangeRoot(sandbox *Sandbox) string {
	return filepath.Join(sandbox.Repo, "openspec", "changes", sddChange)
}

// sddRuntimeRepo is the base fixture for the remediation journeys: a repository
// carrying a committed OpenSpec change, plus one staged prose file as the
// candidate under work.
//
// The OpenSpec change is COMMITTED on purpose. `review bind-sdd` refuses a
// change that does not exist, and an uncommitted change directory would join the
// candidate and make every later candidate comparison a comparison of the
// fixture rather than of the work.
func sddRuntimeRepo(sandbox *Sandbox) error {
	if err := sandbox.initRepo(sandbox.Repo); err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(sandbox.Repo, "README.md"), "# demo\n\nhello\n"); err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(sddChangeRoot(sandbox), "proposal.md"),
		"# "+sddChange+"\n\n## Why\n\nthe benchmark drives a remediation cycle.\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "-A"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "commit", "-qm", "initial"); err != nil {
		return err
	}
	if err := stageProse("", "attempt")(sandbox); err != nil {
		return err
	}

	// Proof: the change directory is committed, so nothing about it is in the
	// candidate, and the product can already resolve a runtime authority for it
	// that has no attempts yet.
	tracked, err := gitOut(sandbox, sandbox.Repo, "ls-files", "openspec/changes/"+sddChange+"/proposal.md")
	if err != nil {
		return err
	}
	if tracked == "" {
		return errors.New("fixture claims a committed OpenSpec change but git does not track it")
	}
	staged, err := gitOut(sandbox, sandbox.Repo, "diff", "--cached", "--name-only")
	if err != nil {
		return err
	}
	if staged != "docs/attempt.md" {
		return fmt.Errorf("fixture claims one staged prose file but the staged diff is %q", staged)
	}
	status, err := proveRuntime(sandbox)
	if err != nil {
		return err
	}
	if status.Change != sddChange || status.NextAction != "begin" || len(status.Attempts) != 0 {
		return fmt.Errorf("fixture claims a fresh runtime authority but the product reports change=%q next_action=%q attempts=%d",
			status.Change, status.NextAction, len(status.Attempts))
	}
	return nil
}

// sddBoundedCorrection is the edit that makes the remediation cycle necessary:
// the candidate moves AFTER the second attempt began, which is the exact
// condition a bound passing finish is not allowed to close over.
func sddBoundedCorrection(sandbox *Sandbox) error {
	before, err := gitOut(sandbox, sandbox.Repo, "rev-parse", ":docs/attempt.md")
	if err != nil {
		return err
	}
	path := filepath.Join(sandbox.Repo, "docs", "attempt.md")
	if err := sandbox.write(path,
		"# attempt\n\nplain prose, no executable content.\nthe correction the failed attempt asked for.\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "-A"); err != nil {
		return err
	}

	// Proof: the STAGED blob moved. Rewriting the working tree with identical
	// bytes, or forgetting to stage, would leave the candidate where it was and
	// the journey would then be measuring the unchanged-candidate path instead.
	after, err := gitOut(sandbox, sandbox.Repo, "rev-parse", ":docs/attempt.md")
	if err != nil {
		return err
	}
	if before == after {
		return fmt.Errorf("fixture claims a bounded correction but the staged blob is still %s", before)
	}
	return nil
}

// sddWidenScope stages a second prose file. The approved scope really changes,
// which is what `review recover` requires and what turns the bound lineage into
// a non-leaf.
func sddWidenScope(sandbox *Sandbox) error {
	if err := stageProse("", "widened")(sandbox); err != nil {
		return err
	}
	staged, err := gitOut(sandbox, sandbox.Repo, "diff", "--cached", "--name-only")
	if err != nil {
		return err
	}
	if !strings.Contains(staged, "docs/widened.md") {
		return fmt.Errorf("fixture claims a widened scope but the staged diff is %q", staged)
	}
	return nil
}

// sddStrandSuccessor takes the widened path back out. The successor's frozen
// target still names it, so that successor can never be finalized: its live
// projection will never match its own authority again.
func sddStrandSuccessor(sandbox *Sandbox) error {
	if err := sandbox.git(sandbox.Repo, "rm", "-q", "--cached", "docs/widened.md"); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(sandbox.Repo, "docs", "widened.md")); err != nil {
		return err
	}

	// Proof, from the product rather than from git: the successor is still a
	// pristine reviewing authority, and the identity it froze is no longer the
	// identity of the candidate on disk. That pair is what "stranded" means.
	head, err := proveAuthorities(sandbox)
	if err != nil {
		return err
	}
	_, frozen, ok := head.entry(sddSuccessorLineage)
	if !ok {
		return fmt.Errorf("fixture claims a stranded successor but review status does not list %q", sddSuccessorLineage)
	}
	var live statusEnvelope
	if err := proveJSON(sandbox, &live, "review", "status", "--cwd", sandbox.Repo,
		"--contract", reviewContract, "--next-transition"); err != nil {
		return err
	}
	if live.TargetIdentity == "" {
		return errors.New("fixture cannot prove a stranded successor: the product published no live target identity")
	}
	if frozen == live.TargetIdentity {
		return fmt.Errorf("fixture claims a stranded successor but its frozen target %s is still the live candidate", frozen)
	}
	return nil
}

// sddDrift moves the candidate after a terminal attempt closed, which is the
// shape that used to have no way out: begin refuses because the objective
// changed, and reset used to refuse because the last attempt was terminal but
// still under budget.
func sddDrift(sandbox *Sandbox) error {
	before, err := gitOut(sandbox, sandbox.Repo, "rev-parse", ":docs/attempt.md")
	if err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(sandbox.Repo, "docs", "attempt.md"),
		"# attempt\n\nplain prose, no executable content.\nthe candidate drifted after the attempt closed.\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "-A"); err != nil {
		return err
	}
	after, err := gitOut(sandbox, sandbox.Repo, "rev-parse", ":docs/attempt.md")
	if err != nil {
		return err
	}
	if before == after {
		return fmt.Errorf("fixture claims candidate drift but the staged blob is still %s", before)
	}
	return nil
}

// sddPlanningArtifacts writes a complete planning set for the change and commits
// it, so `sdd-status` routes on the apply/verify/archive edge rather than on
// missing planning. verifyReport empty means the change has not been verified
// yet, which is where the pre-verify path decides whether a review is required.
func sddPlanningArtifacts(verifyReport string) func(*Sandbox) error {
	return func(sandbox *Sandbox) error {
		if err := sddRuntimeRepo(sandbox); err != nil {
			return err
		}
		root := sddChangeRoot(sandbox)
		files := map[string]string{
			filepath.Join(root, "design.md"): "# design\n\n## Approach\n\nplain prose, no executable content.\n",
			filepath.Join(root, "tasks.md"):  "# tasks\n\n- [x] 1.1 write the prose\n",
			filepath.Join(root, "specs", "prose", "spec.md"): "### Requirement: prose exists\n" +
				"#### Scenario: prose is present\n\n- **WHEN** the reader opens docs/attempt.md\n- **THEN** the prose is there\n",
		}
		if verifyReport != "" {
			files[filepath.Join(root, "verify-report.md")] = verifyReport
		}
		for path, content := range files {
			if err := sandbox.write(path, content); err != nil {
				return err
			}
		}
		if err := sandbox.git(sandbox.Repo, "add", "openspec"); err != nil {
			return err
		}
		if err := sandbox.git(sandbox.Repo, "commit", "-qm", "sdd planning artifacts"); err != nil {
			return err
		}

		// Proof, read back out of the product: the planning set really is
		// complete and every task is checked, so nothing downstream is routing on
		// a half-written change.
		var status sddStatusV1
		if err := proveJSON(sandbox, &status, "sdd-status", sddChange, "--cwd", sandbox.Repo, "--json"); err != nil {
			return err
		}
		if !status.TaskProgress.AllComplete || status.TaskProgress.Total == 0 {
			return fmt.Errorf("fixture claims a complete task list but the product reports %d/%d complete=%v",
				status.TaskProgress.Completed, status.TaskProgress.Total, status.TaskProgress.AllComplete)
		}
		if status.Dependencies.Apply != "all_done" {
			return fmt.Errorf("fixture claims apply is finished but the product reports %q", status.Dependencies.Apply)
		}
		return nil
	}
}

// sddVerifyReport is the fenced envelope a completed independent verification
// writes. Its exact shape matters: a report the product cannot parse routes as
// "verification is missing", which is a different journey.
const sddVerifyReport = "```yaml\n" +
	"schema: gentle-ai.verify-result/v1\n" +
	"evidence_revision: sha256:1111111111111111111111111111111111111111111111111111111111111111\n" +
	"verdict: pass\n" +
	"blockers: 0\n" +
	"critical_findings: 0\n" +
	"requirements: 1/1\n" +
	"scenarios: 1/1\n" +
	"test_command: go test ./internal/example\n" +
	"test_exit_code: 0\n" +
	"test_output_hash: sha256:2222222222222222222222222222222222222222222222222222222222222222\n" +
	"build_command: go test ./cmd/gentle-ai\n" +
	"build_exit_code: 0\n" +
	"build_output_hash: sha256:3333333333333333333333333333333333333333333333333333333333333333\n" +
	"```\n"

// ---------------------------------------------------------------------------
// Counted operator work
// ---------------------------------------------------------------------------

// readRuntimeStatus issues one COUNTED `sdd-attempt status`. Every mutation of
// the runtime ledger needs the exact current revision, and the remediation exit
// needs three more values, all of which the product publishes only here — so an
// agent driving this flow really does have to spend the invocation.
func readRuntimeStatus(r *journeyRun) (sddRuntimeStatus, error) {
	observation := r.run([]string{"sdd-attempt", "status", "--cwd", r.sandbox.Repo, "--change", sddChange}, false)
	var status sddRuntimeStatus
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &status); err != nil {
		return status, fmt.Errorf("parse sdd-attempt status: %w (stderr: %s)", err, firstLine(observation.Stderr))
	}
	return status, nil
}

// sddAttemptArgs assembles one runtime mutation. The objective parameters must
// be byte-identical on every begin of the same objective or the ledger reports a
// changed objective, so they live in one place.
func sddAttemptArgs(r *journeyRun, operation, revision, requestID string, extra ...string) []string {
	args := []string{
		"sdd-attempt", operation, "--cwd", r.sandbox.Repo, "--change", sddChange,
		"--expected-revision", revision, "--request-id", requestID,
	}
	return append(args, extra...)
}

var sddObjective = []string{
	"--work-unit", "bench runtime objective",
	"--evidence-goal", "bench proves the corrected candidate",
	"--max-attempts", "6", "--max-changed-lines", "600",
}

// sddTerminalEvidence is the bounded evidence every finish must carry.
var sddTerminalEvidence = []string{
	"--diagnosis", "the benchmark drove this attempt to a terminal outcome",
	"--harness-disposition", "reused",
	"--cleanup-evidence", "workspace clean",
	"--process-evidence", "no stray processes",
}

// sddBeginFailBegin drives the runtime to the state every remediation journey
// starts from: a failed attempt with recorded evidence, and a second attempt
// running against the same objective.
func sddBeginFailBegin(r *journeyRun) error {
	status, err := readRuntimeStatus(r)
	if err != nil {
		return err
	}
	r.run(sddAttemptArgs(r, "begin", status.Revision, "bench-begin-one", sddObjective...), false)

	if status, err = readRuntimeStatus(r); err != nil {
		return err
	}
	r.run(sddAttemptArgs(r, "finish", status.Revision, "bench-finish-one",
		append([]string{"--outcome", "failed", "--evidence-revision", sddFailedEvidence}, sddTerminalEvidence...)...), false)

	if status, err = readRuntimeStatus(r); err != nil {
		return err
	}
	r.run(sddAttemptArgs(r, "begin", status.Revision, "bench-begin-two", sddObjective...), false)

	return proveActiveAttempt(r.sandbox, 2, sddFailedEvidence)
}

// sddBeginThenInterrupt closes the first attempt as interrupted with budget to
// spare, which is the terminal shape the reset dead end lived on.
func sddBeginThenInterrupt(r *journeyRun) error {
	status, err := readRuntimeStatus(r)
	if err != nil {
		return err
	}
	r.run(sddAttemptArgs(r, "begin", status.Revision, "bench-begin-one", sddObjective...), false)

	if status, err = readRuntimeStatus(r); err != nil {
		return err
	}
	r.run(sddAttemptArgs(r, "finish", status.Revision, "bench-finish-one",
		append([]string{"--outcome", "interrupted", "--evidence-revision", sddFailedEvidence}, sddTerminalEvidence...)...), false)

	final, err := proveRuntime(r.sandbox)
	if err != nil {
		return err
	}
	if final.ActiveAttempt != nil {
		return errors.New("fixture claims a terminal attempt but the runtime still has an active one")
	}
	if len(final.Attempts) != 1 || final.Attempts[0].Outcome != "interrupted" {
		return fmt.Errorf("fixture claims one interrupted attempt but the runtime reports %+v", final.Attempts)
	}
	if final.Complete {
		return errors.New("fixture claims budget remaining but the runtime reports the objective complete")
	}
	return nil
}

// sddBindApprovedReview binds the approved lineage to the change. The lineage is
// read out of the product, never remembered from the review that produced it.
func sddBindApprovedReview(r *journeyRun) error {
	observation := r.run([]string{"review", "status", "--cwd", r.sandbox.Repo}, false)
	var head authorityHead
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &head); err != nil {
		return fmt.Errorf("parse review status: %w (stderr: %s)", err, firstLine(observation.Stderr))
	}
	if len(head.Entries) != 1 || head.Entries[0].State != "approved" {
		return fmt.Errorf("expected exactly one approved lineage to bind, got %+v", head.Entries)
	}
	lineage := head.Entries[0].LineageID
	r.sandbox.Scratch["bound"] = lineage
	r.run([]string{
		"review", "bind-sdd", "--cwd", r.sandbox.Repo,
		"--change", sddChange, "--lineage", lineage, "--expected-binding-revision", "",
	}, false)
	return proveBinding(r.sandbox, lineage)
}

// sddPassingFinish issues the plain passing finish and returns what the product
// said. Every remediation journey turns on this one invocation.
func sddPassingFinish(r *journeyRun, requestID string) (Observation, error) {
	status, err := readRuntimeStatus(r)
	if err != nil {
		return Observation{}, err
	}
	observation := r.run(sddAttemptArgs(r, "finish", status.Revision, requestID,
		append([]string{"--outcome", "passed", "--evidence-revision", sddCorrectedEvidence}, sddTerminalEvidence...)...), false)
	if observation.ExitCode == 0 {
		return observation, errors.New(
			"a bound passing finish over a changed candidate completed without a block: " +
				"the journey's premise no longer holds, so its number would be meaningless")
	}
	return observation, nil
}

// sddNamesFinishExit reports whether a refusal named a runnable
// `gentle-ai sdd-attempt finish`. That is the leaf branch's promise.
func sddNamesFinishExit(observation Observation) bool {
	return strings.Contains(observation.Stdout+observation.Stderr, "`gentle-ai sdd-attempt finish ")
}

// sddNamesReviewRouter reports whether a refusal named the review router
// instead. That is the non-leaf branch's promise, and the two must not overlap:
// naming a finish that would be refused one layer deeper is the exact defect
// shape this branch exists to avoid.
func sddNamesReviewRouter(observation Observation) bool {
	return strings.Contains(observation.Stdout+observation.Stderr, "`gentle-ai review status ")
}

// sddBlockedLeafFinish drives the block and holds the leaf branch's contract:
// the topology really is a leaf, and the refusal named a finish command.
func sddBlockedLeafFinish(r *journeyRun) error {
	if err := proveLeafTopology(r.sandbox, r.sandbox.Scratch["bound"]); err != nil {
		return err
	}
	observation, err := sddPassingFinish(r, "bench-finish-blocked")
	if err != nil {
		return err
	}
	if !sddNamesFinishExit(observation) {
		return fmt.Errorf("the leaf refusal named no self-successor finish: %s", firstLine(observation.Stderr))
	}
	return nil
}

// sddBlockedNonLeafFinish is the same invocation against the other topology. The
// refusal must name the review router and must NOT name a finish, because on
// this shape a finish is refused one layer deeper with a message that names
// nothing at all.
func sddBlockedNonLeafFinish(r *journeyRun) error {
	if err := proveNonLeafTopology(r.sandbox, r.sandbox.Scratch["bound"], sddSuccessorLineage); err != nil {
		return err
	}
	observation, err := sddPassingFinish(r, "bench-finish-blocked")
	if err != nil {
		return err
	}
	if !sddNamesReviewRouter(observation) {
		return fmt.Errorf("the non-leaf refusal named no review router: %s", firstLine(observation.Stderr))
	}
	if sddNamesFinishExit(observation) {
		return fmt.Errorf("the non-leaf refusal named a finish command that cannot be accepted: %s", firstLine(observation.Stderr))
	}
	return nil
}

// sddTransitionCreatesALineage runs the transition the negotiated status
// publishes and requires it to have actually created something.
//
// This step exists because of a bug that is easy to reintroduce and almost
// impossible to notice. The default lineage name is a pure function of the
// target, so a superseded lineage permanently occupies the name its own target
// derives. Status was right that no leaf governed the live candidate and right
// to route to a start, but the command it printed carried no lineage, so the
// start collided with the superseded record and answered "blocked" at exit 0.
//
// The product named the next thing to run, the operator ran it exactly as
// printed, it succeeded, and nothing happened. `executeNextTransitionVerbatim`
// would not have caught it: running verbatim was never the problem.
func sddTransitionCreatesALineage(r *journeyRun) error {
	observation, err := runNextTransitionVerbatim(r)
	if err != nil {
		return err
	}
	var envelope struct {
		Action    string `json:"action"`
		LineageID string `json:"lineage_id"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &envelope); err != nil {
		return fmt.Errorf("the transition's own output is not readable: %w", err)
	}
	if envelope.LineageID == "" {
		return fmt.Errorf("the transition ran and created no lineage: action %q", envelope.Action)
	}
	return nil
}

// sddNamesAbandonExit reports whether a refusal named the abandonment. The
// backtick is part of the match for the same reason the two helpers above carry
// one: prose mentioning a verb is not a named command, and this branch exists
// precisely because prose was what the operator got.
func sddNamesAbandonExit(observation Observation) bool {
	return strings.Contains(observation.Stdout+observation.Stderr, "`gentle-ai review abandon ")
}

// sddBlockedStrandedFinish holds the contract for the third topology, and it is
// deliberately NOT the non-leaf one.
//
// A stranded successor is also non-leaf, so this journey used to assert the
// non-leaf promise: name the review router, name no finish. That promise turned
// out to be wrong here, and wrong in the most expensive way. The router's
// transition ran, exited 0 and changed nothing, and the finish it then asked
// for was refused naming nothing at all. Routing to a dead end is worse than
// routing nowhere, because the operator spends their trust on it.
//
// So the branch is split at the source now, and the contract splits with it.
// A stranded successor is pristine by construction: it froze a target the
// candidate no longer has, so it never captured a result, a finding or a
// correction. Abandoning it is not discarding the operator's work, it restores
// it, because the approved predecessor it superseded is the authority holding
// the corrected candidate and becomes the leaf again.
//
// Naming the router here would now be the defect. So would naming a finish.
func sddBlockedStrandedFinish(r *journeyRun) error {
	if err := proveNonLeafTopology(r.sandbox, r.sandbox.Scratch["bound"], sddSuccessorLineage); err != nil {
		return err
	}
	observation, err := sddPassingFinish(r, "bench-finish-blocked")
	if err != nil {
		return err
	}
	if !sddNamesAbandonExit(observation) {
		return fmt.Errorf("the stranded refusal named no abandonment: %s", firstLine(observation.Stderr))
	}
	if sddNamesReviewRouter(observation) {
		return fmt.Errorf("the stranded refusal named the review router, which runs and changes nothing here: %s", firstLine(observation.Stderr))
	}
	if sddNamesFinishExit(observation) {
		return fmt.Errorf("the stranded refusal named a finish that is refused one layer deeper: %s", firstLine(observation.Stderr))
	}
	return nil
}

// sddRemediationFinish re-runs the finish with the remediation trio, taking all
// three values from where the refusal said they are published — binding_revision,
// binding.lineage and evidence_revision on `sdd-attempt status` — and nothing
// from the journey's memory of what it did earlier.
//
// successor empty means the self-successor exit: the lineage the binding already
// names. A non-empty successor is the distinct-successor exit.
func sddRemediationFinish(successor, requestID string) func(*journeyRun) error {
	return func(r *journeyRun) error {
		status, err := readRuntimeStatus(r)
		if err != nil {
			return err
		}
		if status.Binding == nil || status.BindingRevision == "" || status.EvidenceRevision == "" {
			return fmt.Errorf("the refusal named three published values but status publishes binding=%v binding_revision=%q evidence_revision=%q",
				status.Binding, status.BindingRevision, status.EvidenceRevision)
		}
		lineage := successor
		if lineage == "" {
			lineage = status.Binding.Lineage
		}
		r.run(sddAttemptArgs(r, "finish", status.Revision, requestID,
			append([]string{
				"--outcome", "passed",
				"--evidence-revision", sddCorrectedEvidence,
				"--expected-binding-revision", status.BindingRevision,
				"--successor-lineage", lineage,
				"--remediates-evidence-revision", status.EvidenceRevision,
			}, sddTerminalEvidence...)...), false)
		return nil
	}
}

// sddProveObjectiveComplete asserts the exit really closed the objective. An
// exit that runs and leaves the attempt open would be a worse answer than a
// refusal, and it would be invisible without this.
func sddProveObjectiveComplete(r *journeyRun) error {
	status, err := proveRuntime(r.sandbox)
	if err != nil {
		return err
	}
	if status.ActiveAttempt != nil {
		return errors.New("the remediation exit ran but the attempt is still active")
	}
	if !status.Complete || status.NextAction != "complete" {
		return fmt.Errorf("the remediation exit ran but the objective reports complete=%v next_action=%q",
			status.Complete, status.NextAction)
	}
	return nil
}

// sddRecoverSuccessor follows the scope-changed gate denial exactly as it is
// written, mints the successor, and proves the topology really flipped.
func sddRecoverSuccessor(r *journeyRun) error {
	observation := r.run(productArgsFor(r, "review", "validate", "--gate", "post-apply"), false)
	var denial gateDenial
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &denial); err != nil {
		return fmt.Errorf("parse gate denial: %w (stderr: %s)", err, firstLine(observation.Stderr))
	}
	change := denial.Context.ScopeChange
	if change.PredecessorLineageID == "" || change.PredecessorRevision == "" {
		return fmt.Errorf("gate result %q named no recoverable predecessor", denial.Result)
	}
	r.run(productArgsFor(r, "review", "recover",
		"--predecessor-lineage", change.PredecessorLineageID,
		"--expected-predecessor-revision", change.PredecessorRevision,
		"--successor-lineage", sddSuccessorLineage,
		"--disposition", "scope_changed"), false)
	return proveNonLeafTopology(r.sandbox, r.sandbox.Scratch["bound"], sddSuccessorLineage)
}

// sddAbandonStrandedSuccessor is the exit that actually clears a stranded
// successor, and the product never names it. It costs a hand-assembled
// authorization, which is why this journey is also a manual_tokens exhibit.
func sddAbandonStrandedSuccessor(r *journeyRun) error {
	observation := r.run([]string{"review", "status", "--cwd", r.sandbox.Repo}, false)
	var head authorityHead
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &head); err != nil {
		return fmt.Errorf("parse review status: %w (stderr: %s)", err, firstLine(observation.Stderr))
	}
	revision, snapshot, ok := head.entry(sddSuccessorLineage)
	if !ok {
		return fmt.Errorf("review status no longer lists the stranded successor %q", sddSuccessorLineage)
	}
	const actor = "bench"
	const reason = "the stranded successor can never be finalized"
	authorization := strings.Join([]string{
		"gentle-ai.review-abandon-authorization/v1",
		"lineage=" + sddSuccessorLineage,
		"revision=" + revision,
		"snapshot_identity=" + snapshot,
		"actor=" + actor,
		"reason=" + reason,
	}, "\n")
	r.run([]string{
		"review", "abandon", "--cwd", r.sandbox.Repo,
		"--lineage", sddSuccessorLineage,
		"--expected-revision", revision,
		"--reason", reason,
		"--actor", actor,
		"--maintainer-authorization", authorization,
	}, false)

	if !provePostApplyAllows(r.sandbox) {
		return errors.New("the stranded successor was abandoned but the bound lineage's post-apply gate still does not allow")
	}
	return nil
}

// sddBeginAfterDrift issues the begin the drift refuses, then the reset, then the
// begin again. The middle command is the one that used to have no way out.
func sddBeginAfterDrift(r *journeyRun) error {
	status, err := readRuntimeStatus(r)
	if err != nil {
		return err
	}
	observation := r.run(sddAttemptArgs(r, "begin", status.Revision, "bench-begin-drifted", sddObjective...), false)
	if observation.ExitCode == 0 {
		return errors.New("begin after candidate drift was accepted: the journey's premise no longer holds")
	}
	return nil
}

// sddResetThenBegin takes the exit the refusal asks for without naming, and
// proves the objective really reopened.
func sddResetThenBegin(r *journeyRun) error {
	status, err := readRuntimeStatus(r)
	if err != nil {
		return err
	}
	r.run(sddAttemptArgs(r, "reset", status.Revision, "bench-reset-one",
		"--reason", "the candidate drifted after the interrupted attempt",
		"--actor", "bench"), false)

	if status, err = readRuntimeStatus(r); err != nil {
		return err
	}
	if status.NextAction != "begin" {
		return fmt.Errorf("reset ran but the runtime next action is %q, want begin", status.NextAction)
	}
	r.run(sddAttemptArgs(r, "begin", status.Revision, "bench-begin-three", sddObjective...), false)
	return proveActiveAttempt(r.sandbox, 2, "")
}

// sddWalkIntoRecoveryGuardRails issues the three refusals an operator reaches
// when they try to get rid of a healthy approved review, taking the selectors
// from one status read exactly as an operator would.
//
// All three refusals are CORRECT. What has never been measured is what they cost
// the person who walks into them, which is the path that produced the original
// deadlock report.
// sddInvalidateHealthyApproved is the third guard rail, split out of the
// composite above so its declaration reaches the classifier.
//
// The selectors come from the sandbox rather than from a fresh status read,
// because the finalize step already recorded them. That keeps the step to one
// invocation, which is what the declaration is attached to.
func sddInvalidateHealthyApproved(sandbox *Sandbox) ([]string, error) {
	if strings.TrimSpace(sandbox.Lineage) == "" || strings.TrimSpace(sandbox.Revision) == "" {
		return nil, fmt.Errorf("no lineage recorded to invalidate: lineage %q revision %q", sandbox.Lineage, sandbox.Revision)
	}
	return []string{
		"review", "invalidate",
		"--lineage", sandbox.Lineage,
		"--expected-revision", sandbox.Revision,
		"--gate", "post-apply",
		"--cwd", sandbox.Repo,
	}, nil
}

// sddProveApprovalSurvived re-checks that the refused invalidation changed
// nothing. A guard rail that refuses and mutates anyway would be worse than one
// that allows, because the operator would believe their approval was intact.
func sddProveApprovalSurvived(r *journeyRun) error {
	if !provePostApplyAllows(r.sandbox) {
		return errors.New("a refused invalidation left the approved authority no longer allowing")
	}
	return nil
}

func sddWalkIntoRecoveryGuardRails(r *journeyRun) error {
	observation := r.run([]string{"review", "status", "--cwd", r.sandbox.Repo}, false)
	var head authorityHead
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &head); err != nil {
		return fmt.Errorf("parse review status: %w (stderr: %s)", err, firstLine(observation.Stderr))
	}
	if len(head.Entries) != 1 || head.Entries[0].State != "approved" {
		return fmt.Errorf("expected exactly one approved lineage, got %+v", head.Entries)
	}
	lineage := head.Entries[0].LineageID
	revision := head.Entries[0].Revision
	if !provePostApplyAllows(r.sandbox) {
		return errors.New("fixture claims healthy approved authority but its post-apply gate does not allow")
	}

	// One: recover it, the way j32 and j33 recover everything else.
	r.run(productArgsFor(r, "review", "recover",
		"--predecessor-lineage", lineage,
		"--expected-predecessor-revision", revision,
		"--successor-lineage", "review-unwanted-successor",
		"--disposition", "scope_changed"), false)

	// Two: the other disposition, which is what the first refusal's wording
	// sends an operator to try next.
	r.run(productArgsFor(r, "review", "recover",
		"--predecessor-lineage", lineage,
		"--expected-predecessor-revision", revision,
		"--successor-lineage", "review-unwanted-successor",
		"--disposition", "invalidated"), false)

	// Three, invalidating it directly, is deliberately NOT here. It is the one
	// of the three whose honest exit is a world action rather than a command,
	// so it carries a by_design declaration, and a declaration on a composite
	// never reaches the classifier: the runner rejects one on a step that
	// issues no invocation of its own. It is its own step below.

	// The authority must survive both refusals untouched. A guard rail that
	// refuses and mutates anyway would be the worst outcome available here.
	if !provePostApplyAllows(r.sandbox) {
		return errors.New("two refused operations left the approved authority no longer allowing")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Assertions attached to non-blocking steps
// ---------------------------------------------------------------------------

// sddStatusAssertion turns a `sdd-status --json` expectation into an After hook.
// These journeys measure a surface where the interesting answer is that nothing
// blocks, so the pin cannot be a block count: it has to be an assertion on the
// envelope, and a regression has to fail the journey loudly rather than pass
// quietly.
func sddStatusAssertion(name string, check func(sddStatusV1) error) func(*Sandbox, Observation) error {
	return func(_ *Sandbox, observation Observation) error {
		var status sddStatusV1
		if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &status); err != nil {
			return fmt.Errorf("parse sdd-status: %w (stderr: %s)", err, firstLine(observation.Stderr))
		}
		if err := check(status); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		return nil
	}
}

// ---------------------------------------------------------------------------
// Capabilities
// ---------------------------------------------------------------------------

// The sdd-attempt operations reject `--help` as an undefined flag, which is
// byte-identical to how they reject a flag they do not have — so each of these
// probes runs an argv that carries the flag under test and can only fail on
// state. A build with the flag answers "sdd-attempt requires --cwd"; a build
// without it answers "flag provided but not defined".
var sddAttemptStatusCapability = &Capability{
	Verb:  []string{"sdd-attempt", "status"},
	Probe: []string{"sdd-attempt", "status"},
}
var sddAttemptBeginCapability = &Capability{
	Verb:  []string{"sdd-attempt", "begin"},
	Probe: []string{"sdd-attempt", "begin", "--work-unit=probe", "--evidence-goal=probe", "--max-attempts=1", "--max-changed-lines=1"},
}
var sddAttemptFinishCapability = &Capability{
	Verb:  []string{"sdd-attempt", "finish"},
	Probe: []string{"sdd-attempt", "finish", "--outcome=passed", "--evidence-revision=probe", "--harness-disposition=reused", "--cleanup-evidence=probe", "--process-evidence=probe"},
}
var sddAttemptRemediationCapability = &Capability{
	Verb:  []string{"sdd-attempt", "finish"},
	Probe: []string{"sdd-attempt", "finish", "--expected-binding-revision=probe", "--successor-lineage=probe", "--remediates-evidence-revision=probe"},
}
var sddAttemptResetCapability = &Capability{
	Verb:  []string{"sdd-attempt", "reset"},
	Probe: []string{"sdd-attempt", "reset", "--reason=probe", "--actor=probe"},
}

// sdd-status parses its own arguments too, so it gets a probe rather than a
// help read. The argv is a real read-only status call.
var sddStatusCapability = &Capability{
	Verb:  []string{"sdd-status"},
	Probe: []string{"sdd-status", "--json"},
}

// review bind-sdd has an ordinary help surface, so it is probed the ordinary way.
var bindSDDCapability = &Capability{
	Verb:  []string{"review", "bind-sdd"},
	Flags: []string{"--cwd", "--change", "--lineage", "--expected-binding-revision"},
}

var invalidateCapability = &Capability{
	Verb:  []string{"review", "invalidate"},
	Flags: []string{"--cwd", "--lineage", "--expected-revision", "--gate"},
}

// ---------------------------------------------------------------------------
// Corpus
// ---------------------------------------------------------------------------

// sddJourneys is the third part of the corpus. Journeys 1 to 14 came from the
// community testing guide, 15 to 36 are the edge cases those flows never
// reached, and these are the SDD remediation successor cycle plus the two
// surfaces that meet it — none of which any journey had driven before.
func sddJourneys() []Journey {
	return []Journey{
		// ------------------------------------------ remediation successor cycle
		{
			ID:     "j37-sdd-remediation-self-successor",
			Title:  "Bound passing attempt over a corrected candidate: the block, and the exit it names",
			Source: "shape 4 (a refusal naming something that does not work) + community deadlock report",
			// Expected: the plain passing finish is REFUSED — closing a bound
			// attempt as passed while the binding approves older bytes would
			// launder unreviewed content into a passing runtime record — and the
			// refusal names the one finish that is accepted from here, which the
			// next step then runs. The whole point of the journey is that the
			// named exit runs: a refusal that names a command refused one layer
			// deeper is the defect this branch exists to avoid.
			Steps: []Step{
				{Name: "fixture: repository with a committed OpenSpec change", Fixture: sddRuntimeRepo},
				{Name: "begin, fail, begin again", Requires: sddAttemptBeginCapability, Composite: sddBeginFailBegin},
				{Name: "fixture: the bounded correction moves the candidate", Fixture: sddBoundedCorrection},
				{Name: "review start on the corrected candidate", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: rememberLineage},
				{Name: "bind the approved review to the change", Requires: bindSDDCapability, Composite: sddBindApprovedReview},
				{Name: "plain passing finish over a changed candidate", Requires: sddAttemptFinishCapability, Composite: sddBlockedLeafFinish},
				{Name: "the self-successor finish the refusal named", Requires: sddAttemptRemediationCapability,
					Composite: sddRemediationFinish("", "bench-finish-self-successor")},
				{Name: "prove the objective closed", Composite: sddProveObjectiveComplete},
			},
		},
		{
			ID:     "j38-sdd-remediation-distinct-successor",
			Title:  "Same block with a real recovery successor in the way: the refusal routes to review, not to a finish",
			Source: "shape 3 (the same guard with a different precondition underneath) + shape 4",
			// Expected: once `review recover` mints a successor, the bound lineage
			// is no longer the compact recovery leaf and its post-apply gate stops
			// allowing, so the self-successor finish would be refused one layer
			// deeper. The refusal must therefore name the review router instead —
			// and must NOT name a finish. Following the router verbatim approves
			// the successor, and the distinct-successor finish then completes.
			Steps: []Step{
				{Name: "fixture: repository with a committed OpenSpec change", Fixture: sddRuntimeRepo},
				{Name: "begin, fail, begin again", Requires: sddAttemptBeginCapability, Composite: sddBeginFailBegin},
				{Name: "fixture: the bounded correction moves the candidate", Fixture: sddBoundedCorrection},
				{Name: "review start on the corrected candidate", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: rememberLineage},
				{Name: "bind the approved review to the change", Requires: bindSDDCapability, Composite: sddBindApprovedReview},
				{Name: "fixture: the scope widens after the binding", Fixture: sddWidenScope},
				{Name: "recover the successor the gate names", Requires: recoverCapability, Composite: sddRecoverSuccessor},
				{Name: "plain passing finish with a successor in the way", Requires: sddAttemptFinishCapability, Composite: sddBlockedNonLeafFinish},
				{Name: "run the review transition the refusal routed to", Requires: statusCapability, Composite: executeNextTransitionVerbatim},
				{Name: "the distinct-successor finish", Requires: sddAttemptRemediationCapability,
					Composite: sddRemediationFinish(sddSuccessorLineage, "bench-finish-distinct-successor")},
				{Name: "prove the objective closed", Composite: sddProveObjectiveComplete},
			},
		},
		{
			ID:     "j39-sdd-remediation-stranded-successor",
			Title:  "The successor can never be finalized: what the router names, and what actually clears it",
			Source: "shape 4 (the named continuation runs and changes nothing) + shape 2 (a clearable state read as terminal)",
			// The successor's candidate is reverted below its own frozen target,
			// so it can never be finalized. This journey was added while that
			// state routed to the review router, whose printed transition RAN,
			// exited 0 and changed nothing, after which the finish it asked for
			// was refused naming nothing at all. It reported the gap for exactly
			// one measurement before the gap was closed.
			//
			// Expected now: the refusal names the abandonment that clears it,
			// complete except for the reason and actor the operator supplies, and
			// names neither the router nor a finish, because both are dead ends
			// on this shape. Following it restores the approved predecessor as
			// the leaf, and the finish then succeeds through the self-successor
			// exit.
			//
			// The authorization still has to be assembled by hand, so the cost
			// shows up honestly in manual_tokens rather than being hidden by the
			// block having a name now.
			Steps: []Step{
				{Name: "fixture: repository with a committed OpenSpec change", Fixture: sddRuntimeRepo},
				{Name: "begin, fail, begin again", Requires: sddAttemptBeginCapability, Composite: sddBeginFailBegin},
				{Name: "fixture: the bounded correction moves the candidate", Fixture: sddBoundedCorrection},
				{Name: "review start on the corrected candidate", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: rememberLineage},
				{Name: "bind the approved review to the change", Requires: bindSDDCapability, Composite: sddBindApprovedReview},
				{Name: "fixture: the scope widens after the binding", Fixture: sddWidenScope},
				{Name: "recover the successor the gate names", Requires: recoverCapability, Composite: sddRecoverSuccessor},
				{Name: "fixture: strand the successor below its own frozen target", Fixture: sddStrandSuccessor},
				{Name: "plain passing finish with a stranded successor in the way", Requires: sddAttemptFinishCapability, Composite: sddBlockedStrandedFinish},
				{Name: "the review transition still creates a real lineage here", Requires: statusCapability, Composite: sddTransitionCreatesALineage},
				{Name: "abandon the stranded successor, as the refusal named", Requires: abandonCapability, Composite: sddAbandonStrandedSuccessor},
				{Name: "the self-successor finish, now that the strand is gone", Requires: sddAttemptRemediationCapability,
					Composite: sddRemediationFinish("", "bench-finish-after-abandon")},
				{Name: "prove the objective closed", Composite: sddProveObjectiveComplete},
			},
		},
		{
			ID:     "j40-sdd-attempt-reset-after-drift",
			Title:  "Terminal attempt, drifted candidate: begin refuses and reset is the only way on",
			Source: "shape 2 (a recoverable objective read as terminal) + shape 4",
			// Expected: begin refuses because the objective's candidate moved, and
			// reset — which used to refuse this exact shape, leaving no way on at
			// all — now admits it. What the journey measures is whether the begin
			// refusal names the reset that clears it. It does not: it says the
			// objective changed "without an explicit reset" and prints no command,
			// so this block is out_of_band and is reported as such.
			Steps: []Step{
				{Name: "fixture: repository with a committed OpenSpec change", Fixture: sddRuntimeRepo},
				{Name: "begin, then close the attempt as interrupted", Requires: sddAttemptBeginCapability, Composite: sddBeginThenInterrupt},
				{Name: "fixture: the candidate drifts after the attempt closed", Fixture: sddDrift},
				{Name: "begin against the drifted candidate", Requires: sddAttemptBeginCapability, Composite: sddBeginAfterDrift},
				{Name: "reset the objective, then begin again", Requires: sddAttemptResetCapability, Composite: sddResetThenBegin},
			},
		},

		// -------------------------------------------------- kill switch and SDD
		{
			ID:     "j41-kill-switch-versus-sdd-pre-verify",
			Title:  "Reviews off at the pre-verify decision: the router steps aside instead of naming a command the operator cannot run",
			Source: "shape 5 (the kill switch and the pre-verify router disagreeing) + a limitation this corpus measured closed",
			// This journey was written to turn a believed-open limitation into a
			// measured one: the claim was that the SDD pre-verify router still
			// demanded a bounded review while reviews were switched off, which
			// would name `review start` as the next action and then refuse it — a
			// dead end whose only exit is undoing the operator's own decision.
			//
			// The claim is FALSE as of the run that failed this journey's old
			// assertions, and the failure is how the corpus found out. The
			// assertions below now pin the behavior that replaced it, in both
			// positions, because one half alone proves nothing:
			//
			//   reviews ON  — nextRecommended `review`, verify blocked, and a
			//                 blocked reason that says why. The gate is real.
			//   reviews OFF — nextRecommended `verify`, verify ready, and NO
			//                 blocked reasons at all. The router steps aside
			//                 rather than pointing at a refusal.
			//
			// Then the switch goes back on and the gate returns, because a kill
			// switch that cannot be un-flipped would be a different defect wearing
			// this journey's pass. Nothing here blocks: the product is correct on
			// this shape, and the row's zeros are the finding.
			//
			// `review start` while off is deliberately NOT run here. With the
			// switch off the router never names it, so running it would measure an
			// off-path refusal — and j03 already owns that measurement.
			Steps: []Step{
				{Name: "fixture: change with planning complete and no verification yet", Fixture: sddPlanningArtifacts("")},
				{Name: "sdd-status with reviews on", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json"),
					After: sddStatusAssertion("pre-verify routing with reviews on", func(status sddStatusV1) error {
						if status.NextRecommended != "review" {
							return fmt.Errorf("nextRecommended = %q, want review", status.NextRecommended)
						}
						if status.Dependencies.Verify != "blocked" {
							return fmt.Errorf("dependencies.verify = %q, want blocked", status.Dependencies.Verify)
						}
						if len(status.BlockedReasons) == 0 {
							return errors.New("verify is blocked and no blocked reason says why")
						}
						return nil
					})},
				{Name: "mode disable", Requires: modeCapability, Args: productArgs("review", "mode", "disable", "--json")},
				{Name: "sdd-status with reviews off", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json"),
					After: sddStatusAssertion("pre-verify routing with reviews off", func(status sddStatusV1) error {
						if status.NextRecommended != "verify" {
							return fmt.Errorf("nextRecommended = %q, want verify: the router still steers at a review the operator is not allowed to start", status.NextRecommended)
						}
						if status.Dependencies.Verify != "ready" {
							return fmt.Errorf("dependencies.verify = %q, want ready; blocked reasons = %v",
								status.Dependencies.Verify, status.BlockedReasons)
						}
						if len(status.BlockedReasons) != 0 {
							return fmt.Errorf("reviews are off and the router still reports blocked reasons: %v", status.BlockedReasons)
						}
						return nil
					})},
				{Name: "mode enable", Requires: modeCapability, Args: productArgs("review", "mode", "enable", "--json")},
				{Name: "sdd-status with reviews back on", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json"),
					After: sddStatusAssertion("the gate returns when the switch does", func(status sddStatusV1) error {
						if status.NextRecommended != "review" {
							return fmt.Errorf("nextRecommended = %q, want review: the switch went back on and the gate did not return", status.NextRecommended)
						}
						if status.Dependencies.Verify != "blocked" {
							return fmt.Errorf("dependencies.verify = %q, want blocked", status.Dependencies.Verify)
						}
						return nil
					})},
			},
		},
		{
			ID:     "j42-kill-switch-versus-sdd-archive",
			Title:  "Reviews off at the archive decision: the product defers and never fabricates an approval",
			Source: "shape 5 (a shipped agent contract and the product disagreeing about the same fact)",
			// The second believed-open item is that `sdd-archive` still requires
			// `reviewGate.result: allow`. This journey measures the product half of
			// that pair, which is the half a black-box benchmark can see.
			//
			// Expected: with reviews ON the archive dependency is blocked and the
			// gate result is not allow. With reviews OFF the archive dependency
			// becomes ready, the gate reports the `disabled/unmanaged` delivery
			// disposition — and the result is still NOT `allow`, because declining
			// to manage is not approval.
			//
			// So the product is already correct and nothing here blocks. The
			// remaining half of the limitation is a shipped document: the
			// sdd-archive skill contract requires the literal `allow` that a
			// disabled run is right never to produce. That is a text this
			// benchmark cannot execute, so it is asserted here as the value the
			// product will never hand that contract, and reported as such.
			Steps: []Step{
				{Name: "fixture: change complete with an independent verification", Fixture: sddPlanningArtifacts(sddVerifyReport)},
				{Name: "sdd-status with reviews on", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json"),
					After: sddStatusAssertion("archive routing with reviews on", func(status sddStatusV1) error {
						if status.Dependencies.Archive != "blocked" {
							return fmt.Errorf("dependencies.archive = %q, want blocked", status.Dependencies.Archive)
						}
						if status.ReviewGate == nil || status.ReviewGate.Result == "allow" {
							return fmt.Errorf("reviewGate = %+v, want a present result that is not allow", status.ReviewGate)
						}
						return nil
					})},
				{Name: "mode disable", Requires: modeCapability, Args: productArgs("review", "mode", "disable", "--json")},
				{Name: "sdd-status with reviews off", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json"),
					After: sddStatusAssertion("archive routing with reviews off", func(status sddStatusV1) error {
						if status.Dependencies.Archive == "blocked" {
							return fmt.Errorf("dependencies.archive = %q, want unblocked; blocked reasons = %v",
								status.Dependencies.Archive, status.BlockedReasons)
						}
						if status.ReviewGate == nil {
							return errors.New("reviewGate is absent, so the disabled disposition is not on the wire")
						}
						if status.ReviewGate.Delivery != deliveryDisabledUnmanaged {
							return fmt.Errorf("reviewGate.delivery = %q, want %q", status.ReviewGate.Delivery, deliveryDisabledUnmanaged)
						}
						if status.ReviewGate.Result == "allow" {
							return errors.New("a disabled run fabricated the approval the archive contract asks for")
						}
						return nil
					})},
			},
		},

		// ------------------------------------------------ recovery guard rails
		{
			ID:     "j43-recovery-guard-rails-as-an-operator-meets-them",
			Title:  "Three correct refusals around healthy approved authority, and the one exit that works",
			Source: "shape 4 (a correct refusal that names nothing runnable) + community deadlock report",
			// These three refusals are RIGHT. `review recover` must not mint a
			// successor for a scope that has not changed, and `review invalidate`
			// must not destroy authority whose gate still allows. Unit tests pin
			// all of that.
			//
			// What has never been measured is the operator's side of them: they
			// are reached in the order a person actually reaches them, and each one
			// is classified by the same mechanical rule as everything else. None of
			// them names a runnable continuation, so all three are out_of_band.
			//
			// The exit is not a command at all, and that is why none of the three
			// could name one: to review this candidate again the candidate has to
			// change. Once it does, the gate names the recovery and the recovery
			// works — which is the last step here, so the journey ends at a real
			// `allow` rather than at an assertion.
			//
			// The authority is proven intact after all three, because a guard rail
			// that refuses and mutates anyway would be the worst outcome here and
			// is invisible from the exit codes alone.
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage docs", Fixture: stageProse("", "healthy")},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: rememberLineage},
				{Name: "walk into the recovery guard rails", Requires: recoverCapability, Composite: sddWalkIntoRecoveryGuardRails},
				// The third rail is the one whose honest exit is not a command.
				// The approval covers exactly the candidate in the working tree,
				// so nothing the operator can run makes it stale; the bytes have
				// to change first. The product cannot print a complete command
				// either, because the successor's name is the operator's to
				// choose. The declaration is what says that out loud, and it is
				// verified against the words the product actually printed.
				{Name: "invalidate the healthy approved authority", Requires: invalidateCapability,
					Args: sddInvalidateHealthyApproved,
					ByDesign: &ByDesignDeclaration{
						Shape:      ByDesignWorldAction,
						NextAction: "change the candidate first",
					}},
				{Name: "the refused invalidation changed nothing", Composite: sddProveApprovalSurvived},
				{Name: "fixture: change the candidate, which is what all three asked for", Fixture: stageProse("", "changed")},
				{Name: "recover, following exactly what the gate then names",
					Requires: recoverCapability, Composite: recoverScopeChangeRoundTrip("review-guardrail-successor")},
			},
		},
	}
}
