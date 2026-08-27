package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// This file extends the corpus with SDD lifecycle coverage, including the kill
// switch pointed at SDD and the recovery guard rails an operator meets.
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
//   - State-transition steps use Capability.Probe to test their continuation
//     shape without charging instrumentation to the measured journey.

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
	sddWrongEvidence     = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
)

const (
	sddStaleAuthorityLineage   = "review-sdd-stale"
	sddNewerAuthorityLineage   = "review-sdd-newer"
	sddAmbiguousFirstLineage   = "review-sdd-ambiguous-first"
	sddAmbiguousLastLineage    = "review-sdd-ambiguous-last"
	sddForeignAuthorityLineage = "review-sdd-foreign"
)

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
		Ordinal                    int    `json:"ordinal"`
		ObjectiveID                string `json:"objective_id"`
		ObjectiveGeneration        int    `json:"objective_generation"`
		BeginCandidateTree         string `json:"begin_candidate_tree"`
		FinishCandidateTree        string `json:"finish_candidate_tree"`
		AttestedVerifyReportDigest string `json:"attested_verify_report_digest"`
		Outcome                    string `json:"outcome"`
		EvidenceRevision           string `json:"evidence_revision"`
		RemediatesEvidenceRevision string `json:"remediates_evidence_revision"`
	} `json:"attempts"`
	EvidenceRevision string `json:"evidence_revision"`
	BindingRevision  string `json:"binding_revision"`
	Binding          *struct {
		Change  string `json:"change"`
		Lineage string `json:"lineage"`
	} `json:"binding"`
	LastRescope *struct {
		PreviousObjectiveID  string `json:"previous_objective_id"`
		PreviousGeneration   int    `json:"previous_generation"`
		RescopeCandidateTree string `json:"rescope_candidate_tree"`
		Reason               string `json:"reason"`
		Actor                string `json:"actor"`
	} `json:"last_rescope"`
	NextAction string `json:"next_action"`
	Complete   bool   `json:"complete"`
}

type sddCompactAttemptResult struct {
	State  string `json:"state"`
	Reason string `json:"reason"`
	Token  string `json:"token"`
}

// sddStatusV2 is the subset of `sdd-status --json` the SDD journeys read.
type sddStatusV2 struct {
	NextRecommended string `json:"nextRecommended"`
	Dependencies    struct {
		Apply   string `json:"apply"`
		Verify  string `json:"verify"`
		Archive string `json:"archive"`
	} `json:"dependencies"`
	ReviewOffer *struct {
		Available  bool   `json:"available"`
		Invocation string `json:"invocation"`
	} `json:"reviewOffer"`
	BlockedReasons    []string `json:"blockedReasons"`
	PhaseInstructions struct {
		Verify    []string `json:"verify"`
		Remediate []string `json:"remediate"`
	} `json:"phaseInstructions"`
	TaskProgress struct {
		Total       int  `json:"total"`
		Completed   int  `json:"completed"`
		AllComplete bool `json:"allComplete"`
	} `json:"taskProgress"`
	RemediationState struct {
		Required bool `json:"required"`
	} `json:"remediationState"`
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
// The OpenSpec change is COMMITTED on purpose. An uncommitted change directory
// would join the candidate and make every later candidate comparison a
// comparison of the fixture rather than of the work.
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
		var status sddStatusV2
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

// sddStaleAuthorityFixture recreates the #1893 shape through the public CLI:
// an older reviewing lineage and a newer approved lineage bind the same
// OpenSpec paths but freeze different candidate trees.
func sddStaleAuthorityFixture(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	if err := sddStageAuthorityChange(sandbox, false); err != nil {
		return err
	}
	paths, err := gitOut(sandbox, sandbox.Repo, "diff", "--cached", "--name-only")
	if err != nil {
		return err
	}
	if err := sddFixtureStart(sandbox, sddStaleAuthorityLineage); err != nil {
		return fmt.Errorf("start stale authority: %w", err)
	}

	if err := sandbox.write(filepath.Join(sddChangeRoot(sandbox), "tasks.md"), "# tasks\n\n- [x] 1.1 write the newer prose\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "openspec"); err != nil {
		return err
	}
	newerPaths, err := gitOut(sandbox, sandbox.Repo, "diff", "--cached", "--name-only")
	if err != nil {
		return err
	}
	if paths != newerPaths {
		return fmt.Errorf("fixture changed the bound path set from %q to %q", paths, newerPaths)
	}
	if err := sddFixtureStart(sandbox, sddNewerAuthorityLineage); err != nil {
		return fmt.Errorf("start newer authority: %w", err)
	}

	head, err := proveAuthorities(sandbox)
	if err != nil {
		return err
	}
	states := map[string]int{}
	for _, entry := range head.Entries {
		states[entry.State]++
	}
	if states["reviewing"] != 2 {
		return fmt.Errorf("fixture needs stale and newer reviewing lineages before approval, got %+v", head.Entries)
	}
	if sandbox.Lineage != sddNewerAuthorityLineage {
		return fmt.Errorf("fixture selected lineage %q, want newer lineage %q", sandbox.Lineage, sddNewerAuthorityLineage)
	}
	return nil
}

// sddStageAuthorityChange stages the complete OpenSpec scope an SDD authority
// must bind. foreign adds another change to construct the mixed-path rejection.
func sddStageAuthorityChange(sandbox *Sandbox, foreign bool) error {
	root := sddChangeRoot(sandbox)
	for path, content := range map[string]string{
		filepath.Join(root, "proposal.md"):               "# " + sddChange + "\n\n## Why\n\nexercise stale authority selection.\n",
		filepath.Join(root, "design.md"):                 "# design\n\n## Approach\n\nplain prose.\n",
		filepath.Join(root, "tasks.md"):                  "# tasks\n\n- [x] 1.1 write the prose\n",
		filepath.Join(root, "specs", "prose", "spec.md"): "### Requirement: prose exists\n#### Scenario: prose is present\n\n- **WHEN** the reader opens the change\n- **THEN** the prose is there\n",
	} {
		if err := sandbox.write(path, content); err != nil {
			return err
		}
	}
	if foreign {
		if err := sandbox.write(filepath.Join(sandbox.Repo, "openspec", "changes", "foreign-change", "tasks.md"), "# tasks\n\n- [x] 1.1 foreign work\n"); err != nil {
			return err
		}
	}
	if err := sandbox.git(sandbox.Repo, "add", "openspec"); err != nil {
		return err
	}
	return nil
}

func sddSingleAuthorityFixture(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	if err := sddStageAuthorityChange(sandbox, false); err != nil {
		return err
	}
	return sddFixtureStart(sandbox, sddNewerAuthorityLineage)
}

func sddAmbiguousAuthorityFixture(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	if err := sddStageAuthorityChange(sandbox, false); err != nil {
		return err
	}
	return sddFixtureStart(sandbox, sddAmbiguousFirstLineage)
}

func sddForeignAuthorityFixture(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	if err := sddStageAuthorityChange(sandbox, true); err != nil {
		return err
	}
	return sddFixtureStart(sandbox, sddForeignAuthorityLineage)
}

func sddFixtureStart(sandbox *Sandbox, lineage string) error {
	observation := sandbox.readBack("review", "start", "--cwd", sandbox.Repo, "--lineage", lineage)
	if observation.ExitCode != 0 {
		return fmt.Errorf("review start for %q exited %d: %s", lineage, observation.ExitCode, firstLine(observation.Stderr, observation.Stdout))
	}
	if err := rememberLineage(sandbox, observation); err != nil {
		return err
	}
	if sandbox.Lineage != lineage {
		return fmt.Errorf("review start returned lineage %q, want %q", sandbox.Lineage, lineage)
	}
	return nil
}

// sddProveSelectedApproval verifies the terminal state after the separately
// capability-guarded approval steps have run.
func sddProveSelectedApproval(r *journeyRun) error {
	head, err := proveAuthorities(r.sandbox)
	if err != nil {
		return err
	}
	for _, entry := range head.Entries {
		if entry.LineageID == r.sandbox.Lineage && entry.State == "approved" {
			return nil
		}
	}
	return fmt.Errorf("fixture claims approved lineage %q but review status reports %+v", r.sandbox.Lineage, head.Entries)
}

// The authority controls intentionally drive the lineage their fixtures chose.
// Generic journeys must instead follow the product's active authority.
func sddCaptureSelectedAuthorityLenses(r *journeyRun) error {
	if r.sandbox.Lineage == "" {
		return errors.New("no selected authority lineage")
	}
	return captureAllLensesFor(r, "--lineage", r.sandbox.Lineage)
}

func sddCaptureSelectedAuthorityEvidence(r *journeyRun) error {
	if r.sandbox.Lineage == "" {
		return errors.New("no selected authority lineage")
	}
	return captureFinalEvidenceFor(r, "--lineage", r.sandbox.Lineage)
}

func sddSelectLineage(lineage string) func(*Sandbox) error {
	return func(sandbox *Sandbox) error {
		head, err := proveAuthorities(sandbox)
		if err != nil {
			return err
		}
		for _, entry := range head.Entries {
			if entry.LineageID == lineage {
				sandbox.Lineage = lineage
				return nil
			}
		}
		return fmt.Errorf("fixture cannot select lineage %q from %+v", lineage, head.Entries)
	}
}

func sddReceiptPath(sandbox *Sandbox) (string, error) {
	directory, err := storeLineageDir(sandbox, sandbox.Lineage)
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "review-receipt.json"), nil
}

// sddDuplicateSelectedAuthority models a historical double-publication. A
// second `review start` correctly resumes an equivalent live authority, so this
// persisted fixture builds two separately terminal receipts governing identical
// bytes.
func sddDuplicateSelectedAuthority(sandbox *Sandbox) error {
	fromState, err := storeStatePath(sandbox, sandbox.Lineage)
	if err != nil {
		return err
	}
	fromReceipt, err := sddReceiptPath(sandbox)
	if err != nil {
		return err
	}
	statePayload, err := os.ReadFile(fromState)
	if err != nil {
		return err
	}
	receiptPayload, err := os.ReadFile(fromReceipt)
	if err != nil {
		return err
	}
	directory, err := storeLineageDir(sandbox, sddAmbiguousLastLineage)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	statePath := filepath.Join(directory, "review-state.json")
	if err := os.WriteFile(statePath, statePayload, 0o644); err != nil {
		return err
	}
	record, err := loadStoreRecord(statePath)
	if err != nil {
		return err
	}
	if !setOrderedMember(record.state, "lineage_id", sddAmbiguousLastLineage) {
		return errors.New("compact state carries no lineage_id")
	}
	if _, err := record.save(); err != nil {
		return err
	}
	receipt, err := decodeOrderedJSON(receiptPayload)
	if err != nil {
		return err
	}
	if lineage, ok := orderedString(receipt, "lineage_id"); !ok || lineage == "" {
		return errors.New("compact receipt carries no lineage_id")
	}
	if !setOrderedMember(receipt, "lineage_id", sddAmbiguousLastLineage) {
		return errors.New("compact receipt carries no lineage_id")
	}
	var compact bytes.Buffer
	if err := encodeOrderedJSON(receipt, &compact); err != nil {
		return err
	}
	var indented bytes.Buffer
	if err := json.Indent(&indented, compact.Bytes(), "", "  "); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(directory, "review-receipt.json"), append(indented.Bytes(), '\n'), 0o644); err != nil {
		return err
	}
	head, err := proveAuthorities(sandbox)
	if err != nil {
		return err
	}
	approved := map[string]bool{}
	for _, entry := range head.Entries {
		if entry.State == "approved" {
			approved[entry.LineageID] = true
		}
	}
	if !approved[sandbox.Lineage] || !approved[sddAmbiguousLastLineage] {
		return fmt.Errorf("fixture claims two approved authorities but review status reports %+v", head.Entries)
	}
	return nil
}

func sddRemoveReceipt(sandbox *Sandbox) error {
	path, err := sddReceiptPath(sandbox)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("fixture expected an approved receipt at %q: %w", path, err)
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return fmt.Errorf("fixture did not remove receipt %q: %v", path, err)
	}
	return nil
}

func sddMismatchReceipt(sandbox *Sandbox) error {
	path, err := sddReceiptPath(sandbox)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		return err
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if string(payload) != "{}\n" {
		return fmt.Errorf("fixture wrote receipt mismatch %q, read %q", path, payload)
	}
	return nil
}

func sddDenyPostApply(sandbox *Sandbox) error {
	path := filepath.Join(sddChangeRoot(sandbox), "tasks.md")
	if err := sandbox.write(path, "# tasks\n\n- [x] 1.1 changed after approval\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "openspec"); err != nil {
		return err
	}
	if provePostApplyAllows(sandbox) {
		return errors.New("fixture claims a non-allow post-apply gate but it still allows")
	}
	return nil
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

const sddFailedVerifyReport = "```yaml\n" +
	"schema: gentle-ai.verify-result/v1\n" +
	"evidence_revision: " + sddFailedEvidence + "\n" +
	"verdict: fail\n" +
	"blockers: 1\n" +
	"critical_findings: 0\n" +
	"requirements: 1/1\n" +
	"scenarios: 1/1\n" +
	"test_command: go test ./internal/example\n" +
	"test_exit_code: 1\n" +
	"test_output_hash: sha256:2222222222222222222222222222222222222222222222222222222222222222\n" +
	"build_command: go test ./cmd/gentle-ai\n" +
	"build_exit_code: 0\n" +
	"build_output_hash: sha256:3333333333333333333333333333333333333333333333333333333333333333\n" +
	"```\n"

const sddHistoricalStalePassReport = "```yaml\n" +
	"schema: gentle-ai.verify-result/v1\n" +
	"evidence_revision: sha256:1111111111111111111111111111111111111111111111111111111111111111\n" +
	"verdict: pass\n" +
	"blockers: 0\n" +
	"critical_findings: 0\n" +
	"requirements: 0/0\n" +
	"scenarios: 1/1\n" +
	"test_command: go test ./internal/example\n" +
	"test_exit_code: 0\n" +
	"test_output_hash: sha256:2222222222222222222222222222222222222222222222222222222222222222\n" +
	"build_command: go test ./cmd/gentle-ai\n" +
	"build_exit_code: 0\n" +
	"build_output_hash: sha256:3333333333333333333333333333333333333333333333333333333333333333\n" +
	"```\n"

// sddHistoricalStalePass creates a first-time component whose change-local
// delta spec uses the valid historical requirement heading. The all-green report
// predates that count and must re-enter fresh review routing, never remediation.
func sddHistoricalStalePass(sandbox *Sandbox) error {
	if err := sddPlanningArtifacts("")(sandbox); err != nil {
		return err
	}
	root := sddChangeRoot(sandbox)
	if err := sandbox.write(filepath.Join(root, "specs", "prose", "spec.md"),
		"### REQ-1: prose exists\n#### Scenario: prose is present\n"); err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(root, "verify-report.md"), sddHistoricalStalePassReport); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "openspec"); err != nil {
		return err
	}
	return sandbox.git(sandbox.Repo, "commit", "-qm", "historical SDD verification evidence")
}

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

// selectedReviewArgs keeps a multi-lineage journey on the authority its fixture
// selected. Without the selector, lifecycle discovery may choose stale history.
func selectedReviewArgs(parts ...string) func(*Sandbox) ([]string, error) {
	return func(sandbox *Sandbox) ([]string, error) {
		if sandbox.Lineage == "" {
			return nil, errors.New("no selected review lineage")
		}
		args := append([]string{}, parts...)
		return append(args, "--cwd", sandbox.Repo, "--lineage", sandbox.Lineage), nil
	}
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

var sddUnmanagedObjective = []string{
	"--work-unit", "bench unmanaged correction",
	"--evidence-goal", "repair admitted verification failure",
	"--max-attempts", "2", "--max-changed-lines", "20",
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

func sddBeginFailedUnmanagedVerification(r *journeyRun) error {
	status, err := readRuntimeStatus(r)
	if err != nil {
		return err
	}
	r.run(sddAttemptArgs(r, "begin", status.Revision, "bench-unmanaged-begin-verification", sddUnmanagedObjective...), false)
	if status, err = readRuntimeStatus(r); err != nil {
		return err
	}
	r.run(sddAttemptArgs(r, "finish", status.Revision, "bench-unmanaged-finish-verification",
		append([]string{"--outcome", "failed", "--evidence-revision", sddFailedEvidence}, sddTerminalEvidence...)...), false)
	status, err = proveRuntime(r.sandbox)
	if err != nil {
		return err
	}
	if status.ActiveAttempt != nil || len(status.Attempts) != 1 || status.Attempts[0].Outcome != "failed" || status.NextAction != "begin" {
		return fmt.Errorf("failed verification did not leave exactly one bounded successor: %#v", status)
	}
	return nil
}

func sddUnmanagedAcquireCorrection(r *journeyRun) error {
	observation := r.run(append([]string{
		"sdd-attempt", "acquire", "--cwd", r.sandbox.Repo, "--change", sddChange,
		"--request-id", "bench-unmanaged-acquire", "--remediates-evidence-revision", sddFailedEvidence,
	}, sddUnmanagedObjective...), false)
	var result sddCompactAttemptResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &result); err != nil {
		return fmt.Errorf("parse unmanaged correction acquire: %w (stderr: %s)", err, firstLine(observation.Stderr))
	}
	if observation.ExitCode != 0 || result.State != "proceed" || result.Token == "" {
		return fmt.Errorf("unmanaged correction acquire = %#v exit=%d", result, observation.ExitCode)
	}
	r.sandbox.Scratch["unmanaged-token"] = result.Token
	return nil
}

func sddUnmanagedSettle(r *journeyRun, requestID, failedEvidence string, wantSuccess bool) error {
	token := r.sandbox.Scratch["unmanaged-token"]
	observation := r.run(append([]string{
		"sdd-attempt", "settle", "--cwd", r.sandbox.Repo, "--change", sddChange, "--token", token,
		"--request-id", requestID, "--outcome", "passed", "--evidence-revision", sddCorrectedEvidence,
		"--remediates-evidence-revision", failedEvidence,
	}, sddTerminalEvidence...), false)
	var result sddCompactAttemptResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &result); err != nil {
		return fmt.Errorf("parse unmanaged correction settle: %w (stderr: %s)", err, firstLine(observation.Stderr))
	}
	if wantSuccess && (observation.ExitCode != 0 || result.State != "complete") {
		if result.State == "blocked" && result.Reason == "invalid_continuation" {
			if err := proveActiveAttempt(r.sandbox, 2, sddFailedEvidence); err != nil {
				return fmt.Errorf("invalid continuation did not leave the remediation attempt running: %w", err)
			}
			return fmt.Errorf("bounded unmanaged correction was blocked as invalid_continuation and left its remediation attempt running")
		}
		return fmt.Errorf("bounded unmanaged correction did not settle: %#v exit=%d", result, observation.ExitCode)
	}
	if !wantSuccess && result.State != "blocked" {
		return fmt.Errorf("invalid unmanaged correction = %#v, want blocked", result)
	}
	return nil
}

func sddUnmanagedCorrectionRemainsBounded(r *journeyRun) error {
	if err := sddUnmanagedSettle(r, "bench-unmanaged-unchanged", sddFailedEvidence, false); err != nil {
		return err
	}
	return proveActiveAttempt(r.sandbox, 2, sddFailedEvidence)
}

func sddUnmanagedWrongEvidenceIsRejected(r *journeyRun) error {
	if err := sddUnmanagedSettle(r, "bench-unmanaged-wrong-evidence", sddWrongEvidence, false); err != nil {
		return err
	}
	return proveActiveAttempt(r.sandbox, 2, sddFailedEvidence)
}

func sddUnmanagedCorrectionCompletes(r *journeyRun) error {
	if err := sddUnmanagedSettle(r, "bench-unmanaged-correct", sddFailedEvidence, true); err != nil {
		return err
	}
	status, err := proveRuntime(r.sandbox)
	if err != nil {
		return err
	}
	if status.Binding != nil || status.BindingRevision != "" || len(status.Attempts) != 2 ||
		status.Attempts[1].RemediatesEvidenceRevision != sddFailedEvidence {
		return fmt.Errorf("unmanaged correction invented review authority or lost failed evidence: %#v", status)
	}
	return nil
}

func sddUnmanagedReplayIsComplete(r *journeyRun) error {
	observation := r.run(append([]string{"sdd-attempt", "acquire", "--cwd", r.sandbox.Repo, "--change", sddChange, "--request-id", "bench-unmanaged-replay"}, sddUnmanagedObjective...), false)
	var result sddCompactAttemptResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &result); err != nil {
		return fmt.Errorf("parse unmanaged correction replay: %w", err)
	}
	if observation.ExitCode != 0 || result.State != "complete" {
		return fmt.Errorf("unmanaged correction replay = %#v exit=%d", result, observation.ExitCode)
	}
	return nil
}

func sddReplaceFailedVerifyReport(sandbox *Sandbox) error {
	return sandbox.write(filepath.Join(sddChangeRoot(sandbox), "verify-report.md"), sddVerifyReport)
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
		append([]string{"--outcome", "interrupted"}, sddTerminalEvidence...)...), false)

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
func sddStatusAssertion(name string, check func(sddStatusV2) error) func(*Sandbox, Observation) error {
	return func(_ *Sandbox, observation Observation) error {
		var status sddStatusV2
		if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &status); err != nil {
			return fmt.Errorf("parse sdd-status: %w (stderr: %s)", err, firstLine(observation.Stderr))
		}
		if err := check(status); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		return nil
	}
}

// sddStatusIgnoresCorruptCompactAuthorityPreVerify (formerly
// sddStatusFailsClosed) pinned `applyPreVerifyCompactBridgeRouting`, deleted
// by Wave 4 commit 21dfc0fe ("remove pre-verify review supervision, add
// offer absence guard", S3) along with `applyPreVerifyReviewRouting` (the
// mechanism j41 pinned). `discoverCompactPreVerifyAuthority`
// (internal/sddstatus/review_gate.go) still computes the exact `Relevant`/
// `Reason` values these fixtures are named for (ambiguous authorities,
// missing/mismatched receipt, non-allow gate, foreign OpenSpec path), but
// nothing in production reads them anymore -- only the still-alive
// `Eligible` field feeds the unrelated stale-report-recovery bridge. This is
// spec-ratified, not an oversight: `rdd-post-verify-review-offer`'s "Offer
// Occurs Strictly Post-Verify, Pre-Archive" requirement is unconditional
// ("SDD MUST NOT consult, block on, or offer RDD review before or during
// apply"), and these fixtures never reach a passing SDD verify-report, so
// under Wave 4 the corrupted compact authority is correctly never even
// looked at. Corrective verify cycle 3 (CRITICAL-C) rewrites the assertion
// to pin that absence directly, the same "ready/verify, zero blocked
// reasons, regardless of what garbage exists in the review store" shape
// j41 already established for the sibling pre-verify-routing case.
func sddStatusIgnoresCorruptCompactAuthorityPreVerify(_ string) func(*Sandbox, Observation) error {
	return sddStatusAssertion("corrupt compact authority is not consulted pre-verify", func(status sddStatusV2) error {
		if status.Dependencies.Verify != "ready" || status.NextRecommended != "verify" {
			return fmt.Errorf("verify=%q nextRecommended=%q, want ready/verify (pre-verify review consultation was removed in Wave 4); blocked reasons=%v",
				status.Dependencies.Verify, status.NextRecommended, status.BlockedReasons)
		}
		if len(status.BlockedReasons) != 0 {
			return fmt.Errorf("pre-verify status reports blocked reasons despite no review consultation: %v", status.BlockedReasons)
		}
		return nil
	})
}

func sddApprovedAuthoritySteps(fixture func(*Sandbox) error) []Step {
	return []Step{
		{Name: "fixture: valid compact authority", Fixture: fixture},
		{Name: "capture every lens for the selected authority", Requires: captureResultCapability, Composite: sddCaptureSelectedAuthorityLenses},
		{Name: "finalize selected authority results", Requires: selectedFinalizeResultsCapability,
			Args: selectedReviewArgs("review", "finalize", "--captured-results=true")},
		{Name: "capture final evidence for the selected authority", Requires: captureEvidenceCapability, Composite: sddCaptureSelectedAuthorityEvidence},
		{Name: "approve the selected authority", Requires: selectedFinalizeEvidenceCapability,
			Args: selectedReviewArgs("review", "finalize", "--captured-evidence=true")},
		{Name: "prove selected authority approval", Composite: sddProveSelectedApproval},
	}
}

// sddBurnedAuthoritySteps is the #3417 counterpart for the three SDD journeys
// that used to depend on a durable approved lineage. It keeps their real review
// exercise but proves the terminal transaction is gone before SDD continues under
// ordinary policy.
func sddBurnedAuthoritySteps(fixture func(*Sandbox) error) []Step {
	return []Step{
		{Name: "fixture: exact active-lineage compact transaction", Fixture: fixture},
		{Name: "capture every selected lens for the exact active lineage", Requires: captureResultCapability, Composite: sddCaptureSelectedAuthorityLenses},
		{Name: "finalize exact active-lineage reviewer results", Requires: selectedFinalizeResultsCapability,
			Args: selectedReviewArgs("review", "finalize", "--captured-results=true")},
		{Name: "capture final evidence for the exact active lineage", Requires: captureEvidenceCapability, Composite: sddCaptureSelectedAuthorityEvidence},
		{Name: "#3417 final evidence burns the exact active-lineage transaction", Requires: selectedFinalizeEvidenceCapability,
			Args: selectedReviewArgs("review", "finalize", "--captured-evidence=true"), After: func(sandbox *Sandbox, observation Observation) error {
				return requireBurnedApproval(sandbox.Lineage)(sandbox, observation)
			}},
		{Name: "prove the terminal burn leaves no durable authority or receipt", Composite: sddProveSelectedBurned},
	}
}

func sddProveSelectedBurned(r *journeyRun) error {
	if r.sandbox.Lineage == "" {
		return errors.New("#3417 SDD fixture has no exact lineage to prove burned")
	}
	return requireAtomicLineageBurned(r, r.sandbox.Lineage)
}

// ---------------------------------------------------------------------------
// Capabilities
// ---------------------------------------------------------------------------

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
	Probe: []string{"sdd-attempt", "finish", "--remediates-evidence-revision=probe"},
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

var selectedFinalizeResultsCapability = &Capability{
	Verb:  []string{"review", "finalize"},
	Flags: []string{"--cwd", "--lineage", "--captured-results"},
}

var selectedFinalizeEvidenceCapability = &Capability{
	Verb:  []string{"review", "finalize"},
	Flags: []string{"--cwd", "--lineage", "--captured-evidence"},
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
// reached, and these exercise active SDD lifecycle surfaces.
func sddJourneys() []Journey {
	return []Journey{
		{
			ID:     "j40-sdd-attempt-reset-after-drift",
			Review: reviewOptedIn,
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
			Review: reviewOptedIn,
			Title:  "Pre-verify: RDD supervises nothing, on or off, before verify runs",
			Source: "shape 5 (the kill switch and the pre-verify router) + Wave 4's own removal of pre-verify review supervision",
			// Corrective verify cycle 3 (CRITICAL-C): this journey pinned a
			// PRE-Wave-4 shape -- a pre-verify review gate that blocked
			// Dependencies.Verify and routed nextRecommended to "review" while
			// reviews were on, stepping aside only when the switch was off. Wave
			// 4 commit 21dfc0fe ("remove pre-verify review supervision, add offer
			// absence guard", S3) deliberately removed that gate entirely --
			// ratified by rdd-post-verify-review-offer's "Offer Occurs Strictly
			// Post-Verify, Pre-Archive" requirement: SDD MUST NOT consult, block
			// on, or offer RDD review before or during apply, full stop, on
			// either side of the switch. This journey was never updated when
			// that landed, so it pinned dead behavior and could never again
			// observe a real regression in the code path it named.
			//
			// Rewritten to pin the CURRENT, ratified shape: with planning
			// complete and no verify report yet, nextRecommended is "verify" and
			// Dependencies.Verify is "ready" -- identically, byte-for-byte in
			// the fields that matter -- whether the switch is on or off, because
			// there is no pre-verify review supervision left to differ. The
			// switch toggling is kept in the journey (rather than deleted
			// outright) specifically to prove that absence: on, off, and back on
			// all produce the same pre-verify routing.
			Steps: []Step{
				{Name: "fixture: change with planning complete and no verification yet", Fixture: sddPlanningArtifacts("")},
				{Name: "sdd-status with reviews on", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json"),
					After: sddStatusAssertion("pre-verify routing with reviews on", func(status sddStatusV2) error {
						if status.NextRecommended != "verify" {
							return fmt.Errorf("nextRecommended = %q, want verify: pre-verify review supervision was removed in Wave 4", status.NextRecommended)
						}
						if status.Dependencies.Verify != "ready" {
							return fmt.Errorf("dependencies.verify = %q, want ready; blocked reasons = %v",
								status.Dependencies.Verify, status.BlockedReasons)
						}
						if len(status.BlockedReasons) != 0 {
							return fmt.Errorf("enabled pre-verify routing reports blocked reasons: %v", status.BlockedReasons)
						}
						return nil
					})},
				{Name: "mode disable", Requires: modeCapability, Args: productArgs("review", "mode", "disable", "--json")},
				{Name: "sdd-status with reviews off", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json"),
					After: sddStatusAssertion("pre-verify routing with reviews off", func(status sddStatusV2) error {
						if status.NextRecommended != "verify" {
							return fmt.Errorf("nextRecommended = %q, want verify", status.NextRecommended)
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
					After: sddStatusAssertion("pre-verify routing is unchanged once the switch returns", func(status sddStatusV2) error {
						if status.NextRecommended != "verify" {
							return fmt.Errorf("nextRecommended = %q, want verify: re-enabling must not resurrect pre-verify supervision", status.NextRecommended)
						}
						if status.Dependencies.Verify != "ready" {
							return fmt.Errorf("dependencies.verify = %q, want ready", status.Dependencies.Verify)
						}
						return nil
					})},
			},
		},
		{
			ID:     "j42-kill-switch-versus-sdd-archive",
			Review: reviewOptedIn,
			Title:  "The offer is an invitation, never a gate: archive proceeds with reviews on or off",
			Source: "shape 5 (a shipped agent contract and the product disagreeing about the same fact) + corrective verify cycle 4 BLOCKER-1",
			// Corrective verify cycle 4, BLOCKER-1 (rdd-post-verify-review-offer's
			// "Decline Proceeds to Unmanaged Ordinary Archive"): a genuinely
			// missing receipt is decline-by-absence-of-action, not a blocker, on
			// EITHER side of the switch. Superseded expectation (documented, not
			// silently dropped): this journey previously required
			// dependencies.archive = "blocked" with reviews on, treating an
			// unacted-on offer as a hard gate.
			//
			// Rewritten to pin the ratified "invitation, never a gate" shape: with
			// reviews on, verify passed, and no receipt, archive is READY and
			// reviewOffer is present (the invitation the user may act on or not --
			// declining simply means archiving). With reviews off, archive is
			// READY and reviewOffer is structurally ABSENT (corrective verify
			// cycle CRITICAL-1/CRITICAL-3, rdd-post-verify-review-offer's
			// "Kill-Switch-Off Is Structural Absence" requirement — no offer and
			// no status review authority). The one distinction is exactly the
			// offer itself, never whether archive proceeds.
			//
			// The shipped sdd-archive skill states the same non-gating rule.
			Steps: []Step{
				{Name: "fixture: change complete with an independent verification", Fixture: sddPlanningArtifacts(sddVerifyReport)},
				{Name: "sdd-status with reviews on", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json"),
					After: sddStatusAssertion("archive routing with reviews on", func(status sddStatusV2) error {
						if status.Dependencies.Archive != "ready" || status.NextRecommended != "archive" {
							return fmt.Errorf("dependencies.archive = %q next = %q, want ready/archive", status.Dependencies.Archive, status.NextRecommended)
						}
						if status.ReviewOffer == nil || !status.ReviewOffer.Available {
							return fmt.Errorf("reviewOffer = %+v, want an available invitation", status.ReviewOffer)
						}
						return nil
					})},
				{Name: "mode disable", Requires: modeCapability, Args: productArgs("review", "mode", "disable", "--json")},
				{Name: "sdd-status with reviews off", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json"),
					After: sddStatusAssertion("archive routing with reviews off", func(status sddStatusV2) error {
						if status.Dependencies.Archive == "blocked" {
							return fmt.Errorf("dependencies.archive = %q, want unblocked; blocked reasons = %v",
								status.Dependencies.Archive, status.BlockedReasons)
						}
						if status.ReviewOffer != nil {
							return fmt.Errorf("reviewOffer = %+v, want structural absence while the kill switch is off", status.ReviewOffer)
						}
						return nil
					})},
			},
		},
		{
			ID:     "j63-disabled-failed-verification-unmanaged-remediation",
			Review: reviewOptedIn,
			Title:  "Failed verification gets one evidence-bound correction; re-enabled review context remains informational",
			Source: "#3417: failed, unknown, and pending review evidence remains visible but never gates completed SDD archive routing",
			Steps: []Step{
				{Name: "fixture: completed change with admitted failed verification", Fixture: sddPlanningArtifacts(sddFailedVerifyReport)},
				{Name: "enabled failed verification records missing remediation authority", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json"), After: sddStatusAssertion("enabled remediation", func(status sddStatusV2) error {
						if !strings.Contains(strings.Join(status.BlockedReasons, "\n"), "verify evidence requires independent SDD remediation") {
							return fmt.Errorf("enabled remediation omitted its independent remediation context: %v", status.BlockedReasons)
						}
						return nil
					})},
				{Name: "mode disable", Requires: modeCapability, Args: productArgs("review", "mode", "disable", "--json")},
				{Name: "failed verification enters unmanaged remediation", Requires: sddAttemptBeginCapability, Composite: sddBeginFailedUnmanagedVerification},
				{Name: "disabled status names remediation without review authority", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json", "--instructions"), After: sddStatusAssertion("disabled remediation", func(status sddStatusV2) error {
						if status.NextRecommended != "remediate" {
							return fmt.Errorf("disabled failed verification = next %q, want remediate", status.NextRecommended)
						}
						instructions := strings.Join(status.PhaseInstructions.Remediate, "\n")
						if !strings.Contains(instructions, "gentle-ai sdd-attempt acquire") ||
							!strings.Contains(instructions, "--remediates-evidence-revision "+sddFailedEvidence) {
							return fmt.Errorf("disabled remediation emitted no executable evidence-bound continuation: %s", instructions)
						}
						return nil
					})},
				{Name: "acquire the one bounded correction", Requires: sddAttemptRemediationCapability, Composite: sddUnmanagedAcquireCorrection},
				{Name: "unchanged candidate cannot satisfy correction", Requires: sddAttemptRemediationCapability, Composite: sddUnmanagedCorrectionRemainsBounded},
				{Name: "fixture: correction changes the candidate", Fixture: sddBoundedCorrection},
				{Name: "wrong failed evidence cannot satisfy correction", Requires: sddAttemptRemediationCapability, Composite: sddUnmanagedWrongEvidenceIsRejected},
				{Name: "settle the evidence-bound correction", Requires: sddAttemptRemediationCapability, Composite: sddUnmanagedCorrectionCompletes},
				{Name: "replay cannot acquire another correction", Requires: sddAttemptRemediationCapability, Composite: sddUnmanagedReplayIsComplete},
				{Name: "fresh verification is required before archive", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json"), After: sddStatusAssertion("fresh verification", func(status sddStatusV2) error {
						if status.Dependencies.Verify != "ready" || status.Dependencies.Archive != "blocked" || status.NextRecommended != "verify" {
							return fmt.Errorf("post-correction status = verify %q archive %q next %q", status.Dependencies.Verify, status.Dependencies.Archive, status.NextRecommended)
						}
						return nil
					})},
				{Name: "fixture: fresh independent verification passes", Fixture: sddReplaceFailedVerifyReport},
				{Name: "archive is ready without review authority", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json"), After: sddStatusAssertion("disabled archive", func(status sddStatusV2) error {
						if status.Dependencies.Archive != "ready" || status.NextRecommended != "archive" || status.ReviewOffer != nil {
							return fmt.Errorf("disabled archive = archive %q next %q offer=%+v", status.Dependencies.Archive, status.NextRecommended, status.ReviewOffer)
						}
						return nil
					})},
				{Name: "mode enable after unmanaged correction", Requires: modeCapability, Args: productArgs("review", "mode", "enable", "--json")},
				{Name: "re-enabled ordinary delivery remains archive-ready", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json"), After: sddStatusAssertion("re-enabled unmanaged correction", func(status sddStatusV2) error {
						if status.Dependencies.Verify != "all_done" || status.Dependencies.Archive != "ready" || status.NextRecommended != "archive" {
							return fmt.Errorf("re-enabled archive = verify %q archive %q next %q; want all_done/ready/archive", status.Dependencies.Verify, status.Dependencies.Archive, status.NextRecommended)
						}
						if status.ReviewOffer == nil || !status.ReviewOffer.Available || !strings.Contains(status.ReviewOffer.Invocation, "review start") {
							return fmt.Errorf("re-enabled archive omitted its optional fresh-review offer: %+v", status.ReviewOffer)
						}
						return nil
					})},
			},
		},
		{
			ID:     "j52-sdd-stale-authority-does-not-shadow-approved-candidate",
			Review: reviewOptedIn,
			Title:  "Stale same-path review authority: SDD selects the newer approved candidate",
			Source: "issue #1893: stale compact authority must not shadow an exact approved candidate",
			Steps: []Step{
				{Name: "fixture: stale and newer same-path review lineages", Fixture: sddStaleAuthorityFixture},
				{Name: "capture every lens for the newer candidate", Requires: captureResultCapability, Composite: sddCaptureSelectedAuthorityLenses},
				{Name: "finalize the newer candidate results", Requires: selectedFinalizeResultsCapability, Args: selectedReviewArgs("review", "finalize", "--captured-results=true"), After: rememberLineage},
				{Name: "capture final evidence for the newer candidate", Requires: captureEvidenceCapability, Composite: sddCaptureSelectedAuthorityEvidence},
				{Name: "approve the newer candidate", Requires: selectedFinalizeEvidenceCapability, Args: selectedReviewArgs("review", "finalize", "--captured-evidence=true"), After: rememberLineage},
				{Name: "sdd-status selects the approved candidate", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json"),
					After: sddStatusAssertion("stale authority does not shadow the approved candidate", func(status sddStatusV2) error {
						if status.NextRecommended != "verify" {
							return fmt.Errorf("nextRecommended = %q, want verify; blocked reasons = %v", status.NextRecommended, status.BlockedReasons)
						}
						if status.Dependencies.Verify != "ready" {
							return fmt.Errorf("dependencies.verify = %q, want ready; blocked reasons = %v", status.Dependencies.Verify, status.BlockedReasons)
						}
						if len(status.BlockedReasons) != 0 {
							return fmt.Errorf("approved candidate was shadowed by stale authority: %v", status.BlockedReasons)
						}
						return nil
					})},
			},
		},
		{
			ID:     "j53-sdd-ambiguous-authorities-fail-closed",
			Review: reviewOptedIn,
			Title:  "Two approved candidates for the same OpenSpec change: not consulted before verify",
			Source: "compact authority discovery contract (superseded pre-verify half, corrective verify cycle 3 CRITICAL-C) + Wave 4's post-verify-only review consultation",
			Steps: append(sddApprovedAuthoritySteps(sddAmbiguousAuthorityFixture),
				Step{Name: "fixture: duplicate its terminal authority", Fixture: sddDuplicateSelectedAuthority},
				Step{Name: "sdd-status ignores the ambiguous authorities pre-verify", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json"), After: sddStatusIgnoresCorruptCompactAuthorityPreVerify("multiple eligible path-bound compact authorities found")},
			),
		},
		{
			ID:     "j54-sdd-missing-authority-receipt-fails-closed",
			Review: reviewOptedIn,
			Title:  "Approved compact authority without its receipt: not consulted before verify",
			Source: "compact authority discovery contract (superseded pre-verify half, corrective verify cycle 3 CRITICAL-C) + Wave 4's post-verify-only review consultation",
			Steps: append(sddApprovedAuthoritySteps(sddSingleAuthorityFixture),
				Step{Name: "fixture: remove the published authority receipt", Fixture: sddRemoveReceipt},
				Step{Name: "sdd-status ignores the missing receipt pre-verify", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json"), After: sddStatusIgnoresCorruptCompactAuthorityPreVerify("path-bound compact authority receipt is missing")},
			),
		},
		{
			ID:     "j55-sdd-mismatched-authority-receipt-fails-closed",
			Review: reviewOptedIn,
			Title:  "Approved compact authority with a mismatched receipt: not consulted before verify",
			Source: "compact authority discovery contract (superseded pre-verify half, corrective verify cycle 3 CRITICAL-C) + Wave 4's post-verify-only review consultation",
			Steps: append(sddApprovedAuthoritySteps(sddSingleAuthorityFixture),
				Step{Name: "fixture: replace the published authority receipt", Fixture: sddMismatchReceipt},
				Step{Name: "sdd-status ignores the mismatched receipt pre-verify", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json"), After: sddStatusIgnoresCorruptCompactAuthorityPreVerify("path-bound compact authority receipt does not equal approved state")},
			),
		},
		{
			ID:     "j56-sdd-non-allow-post-apply-gate-fails-closed",
			Review: reviewOptedIn,
			Title:  "Valid approved authority over changed bytes: not consulted before verify",
			Source: "compact authority discovery contract (superseded pre-verify half, corrective verify cycle 3 CRITICAL-C) + Wave 4's post-verify-only review consultation",
			Steps: append(sddApprovedAuthoritySteps(sddSingleAuthorityFixture),
				Step{Name: "fixture: change the candidate after approval", Fixture: sddDenyPostApply},
				Step{Name: "sdd-status ignores the non-allow gate pre-verify", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json"), After: sddStatusIgnoresCorruptCompactAuthorityPreVerify("path-bound compact authority post-apply gate is not allow")},
			),
		},
		{
			ID:     "j58-sdd-foreign-openspec-path-fails-closed",
			Review: reviewOptedIn,
			Title:  "Mixed OpenSpec authority path set: not consulted before verify",
			Source: "compact authority discovery contract (superseded pre-verify half, corrective verify cycle 3 CRITICAL-C) + Wave 4's post-verify-only review consultation",
			Steps: append(sddApprovedAuthoritySteps(sddForeignAuthorityFixture),
				Step{Name: "sdd-status ignores the foreign OpenSpec path pre-verify", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json"), After: sddStatusIgnoresCorruptCompactAuthorityPreVerify("path-bound compact authority contains a foreign OpenSpec path")},
			),
		},

		// ------------------------------------------------ recovery guard rails
		{
			ID:     "j43-recovery-guard-rails-as-an-operator-meets-them",
			Review: reviewOptedIn,
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
				// This step used to carry a by-design world-action declaration:
				// the approval covers exactly the candidate in the working tree,
				// so nothing the operator runs makes it stale, and the product
				// was said to be unable to print a complete command because the
				// successor's name is the operator's to choose.
				//
				// The product now prints one, so the declaration was failing as
				// a stale claim. It is removed rather than restated, because the
				// runner already counts this in_band from mechanical evidence and
				// a declaration that disagrees with what the product printed is
				// worth less than the printed words themselves.
				{Name: "invalidate the healthy approved authority", Requires: invalidateCapability,
					Args: sddInvalidateHealthyApproved},
				{Name: "the refused invalidation changed nothing", Composite: sddProveApprovalSurvived},
				{Name: "fixture: change the candidate, which is what all three asked for", Fixture: stageProse("", "changed")},
				{Name: "recover, following exactly what the gate then names",
					Requires: recoverCapability, Composite: recoverScopeChangeRoundTrip("review-guardrail-successor")},
			},
		},
		{
			ID:     "j44-sdd-historical-requirement-stale-pass",
			Review: reviewOptedIn,
			Title:  "Historical change-local requirement heading: stale PASS restarts verification instead of failed remediation",
			Source: "issue #2137 (historical OpenSpec requirement compatibility and stale verification routing)",
			Steps: []Step{
				{Name: "fixture: first-time component with historical requirement evidence", Fixture: sddHistoricalStalePass},
				// Wave 4 S3 removed pre-verify review supervision: this fixture
				// carries no review artifacts anywhere, so absent authority is
				// decline-by-absence and the stale PASS re-enters verification
				// directly (never review, never remediation). Archive stays
				// blocked until that fresh verification lands. Mirrors
				// TestEnabledStaleEvidenceWithNoReceiptRestartsVerification.
				{Name: "sdd-status routes stale PASS to fresh verification", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json"), After: sddStatusAssertion("historical stale PASS routing", func(status sddStatusV2) error {
						if status.NextRecommended != "verify" || status.Dependencies.Verify != "ready" || status.Dependencies.Archive != "blocked" {
							return fmt.Errorf("nextRecommended=%q verify=%q archive=%q, want fresh verification before archive", status.NextRecommended, status.Dependencies.Verify, status.Dependencies.Archive)
						}
						if status.RemediationState.Required {
							return errors.New("stale PASS entered failed-verification remediation")
						}
						for _, reason := range status.BlockedReasons {
							if strings.Contains(reason, "bounded review transaction is missing") || strings.Contains(reason, "remediation") {
								return fmt.Errorf("stale PASS exposed failed-evidence routing: %q", reason)
							}
						}
						return nil
					})},
			},
		},
	}
}
