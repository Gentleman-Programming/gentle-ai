package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// retiredAtomicJourneyReplacements records every ordinary-path journey that
// asserted the pre-#3417 durable-receipt or deciding-gate model. #3564
// reactivates j47 for disabled-mode V2 structural-absence/archive routing: it
// no longer pins parallel-receipt behavior. The remaining entries are not
// weakened: the registered corpus replaces that retired surface with j59, j60,
// and j111's worktree-bound, explicit-active, and retained-approval journeys.
var retiredAtomicJourneyReplacements = map[string]string{
	"j01-docs-happy-path":                                                  "j111-approved-acknowledgement-preserves-staged-precommit-authority",
	"j06-pre-push-after-publication":                                       "j111-approved-acknowledgement-preserves-staged-precommit-authority",
	"j07-disabled-with-stale-receipts":                                     "j111-approved-acknowledgement-preserves-staged-precommit-authority",
	"j15-linked-worktree":                                                  "j59-current-status-and-start-ignore-sibling-worktree-transaction",
	"j32-recovery-of-a-recovery":                                           "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j33-escalate-then-recover":                                            "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j43-recovery-guard-rails-as-an-operator-meets-them":                   "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j44-corrected-current-changes-delivery":                               "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j45-completed-final-verification-retry":                               "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j46-correction-required-staged-recovery":                              "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j48-recovered-workspace-preserves-full-candidate-scope":               "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j50-candidate-decline-denies-generically-then-disabled":               "j111-approved-acknowledgement-preserves-staged-precommit-authority",
	"j52-sdd-stale-authority-does-not-shadow-approved-candidate":           "j59-current-status-and-start-ignore-sibling-worktree-transaction",
	"j53-sdd-ambiguous-authorities-fail-closed":                            "j59-current-status-and-start-ignore-sibling-worktree-transaction",
	"j54-sdd-missing-authority-receipt-fails-closed":                       "j111-approved-acknowledgement-preserves-staged-precommit-authority",
	"j55-sdd-mismatched-authority-receipt-fails-closed":                    "j111-approved-acknowledgement-preserves-staged-precommit-authority",
	"j56-sdd-non-allow-post-apply-gate-fails-closed":                       "j111-approved-acknowledgement-preserves-staged-precommit-authority",
	"j58-sdd-foreign-openspec-path-fails-closed":                           "j59-current-status-and-start-ignore-sibling-worktree-transaction",
	"j61-pre-pr-multi-segment-delivery-denies-without-composition":         "j111-approved-acknowledgement-preserves-staged-precommit-authority",
	"j65-selectorless-committed-correction-continuation":                   "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j76-scope-changed-four-lens-successor":                                "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j82-reviewed-superset-pre-push-allows-unpublished-subset":             "j111-approved-acknowledgement-preserves-staged-precommit-authority",
	"j83-pre-pr-moving-advertised-base-binds-merge-base":                   "j111-approved-acknowledgement-preserves-staged-precommit-authority",
	"j86-approved-base-diff-local-parent-merge-preserves-approved-receipt": "j111-approved-acknowledgement-preserves-staged-precommit-authority",
	"j90-explicit-frozen-reviewing-lineage-resumes-after-drift":            "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j94-escalated-changed-scope-negotiates-recovery":                      "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j97-pre-push-preserves-ls-remote-failure":                             "j111-approved-acknowledgement-preserves-staged-precommit-authority",
	"j100-pre-push-unqualified-selector-ignores-unreachable-remote":        "j111-approved-acknowledgement-preserves-staged-precommit-authority",
	// #3587 removes the public FINALIZE/evidence/retry path. These scenarios'
	// sole subjects were that retired surface; the replacements below keep the
	// corresponding clean close, correction, and delivery-boundary evidence.
	"j03-kill-switch":                                                    "j111-approved-acknowledgement-preserves-staged-precommit-authority",
	"j04-size-does-not-escalate":                                         "j111-approved-acknowledgement-preserves-staged-precommit-authority",
	"j08-finalize-without-reviewer-results":                              "j114-last-reviewer-capture-closes-and-retains-approved-authority",
	"j09-finalize-without-evidence":                                      "j114-last-reviewer-capture-closes-and-retains-approved-authority",
	"j11-unborn-head":                                                    "j110-untracked-acknowledgement-retains-authority-and-selectorless-validation-is-unmanaged",
	"j16-detached-head":                                                  "j114-last-reviewer-capture-closes-and-retains-approved-authority",
	"j18-space-and-non-ascii-path":                                       "j114-last-reviewer-capture-closes-and-retains-approved-authority",
	"j19-submodule-gitlink":                                              "j114-last-reviewer-capture-closes-and-retains-approved-authority",
	"j20-symlink-candidate":                                              "j114-last-reviewer-capture-closes-and-retains-approved-authority",
	"j21-mode-only-change":                                               "j114-last-reviewer-capture-closes-and-retains-approved-authority",
	"j22-pure-rename":                                                    "j114-last-reviewer-capture-closes-and-retains-approved-authority",
	"j23-deletion-only":                                                  "j114-last-reviewer-capture-closes-and-retains-approved-authority",
	"j24-empty-file":                                                     "j114-last-reviewer-capture-closes-and-retains-approved-authority",
	"j25-no-trailing-newline":                                            "j114-last-reviewer-capture-closes-and-retains-approved-authority",
	"j26-crlf-content":                                                   "j114-last-reviewer-capture-closes-and-retains-approved-authority",
	"j27-merge-in-progress":                                              "j114-last-reviewer-capture-closes-and-retains-approved-authority",
	"j28-rebase-in-progress":                                             "j114-last-reviewer-capture-closes-and-retains-approved-authority",
	"j29-cherry-pick-in-progress":                                        "j114-last-reviewer-capture-closes-and-retains-approved-authority",
	"j30-kill-switch-flipped-mid-review":                                 "j111-approved-acknowledgement-preserves-staged-precommit-authority",
	"j35-correction-budget-exactly-zero":                                 "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j66-v5-capture-evidence-descriptors-execute":                        "j114-last-reviewer-capture-closes-and-retains-approved-authority",
	"j67-v5-capture-evidence-correction-descriptor-executes":             "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j85-review-parse-refusals-are-preflight":                            "j114-last-reviewer-capture-closes-and-retains-approved-authority",
	"j91-audited-abandon-preplan-over-budget-correction":                 "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j95-targeted-validator-inspects-provider-bound-corrected-tree":      "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j99-issue-2906-finalize-missing-contract":                           "j114-last-reviewer-capture-closes-and-retains-approved-authority",
	"j107-sdd-approved-active-change-allows-shared-openspec-scaffolding": "j111-approved-acknowledgement-preserves-staged-precommit-authority",
	"j108-sdd-post-review-verify-report-is-natively-bound":               "j111-approved-acknowledgement-preserves-staged-precommit-authority",
	"j109-sdd-legacy-post-review-report-requires-current-attestation":    "j111-approved-acknowledgement-preserves-staged-precommit-authority",
}

func removeRetiredAtomicJourneys(journeys []Journey) []Journey {
	active := make([]Journey, 0, len(journeys))
	for _, journey := range journeys {
		if _, retired := retiredAtomicJourneyReplacements[journey.ID]; !retired {
			active = append(active, journey)
		}
	}
	return active
}

// j111 proves #3867's terminal boundary at the built-binary surface. Exact
// acknowledgement consumes the continuation while preserving the approved
// authority that governs the unchanged staged candidate.
func atomicReviewLineageFor(r *journeyRun) (string, error) {
	if r.sandbox.Lineage == "" {
		return "", fmt.Errorf("atomic review journey has no selectorless START lineage")
	}
	return r.sandbox.Lineage, nil
}

func startAtomicReviewFromSelectorlessStatus(r *journeyRun) error {
	lineage, err := startAtomicTransactionFromSelectorlessStatus(r)
	if err != nil {
		return fmt.Errorf("initial selectorless START: %w", err)
	}
	r.sandbox.Lineage = lineage
	return requireExplicitAtomicFourLensStatusFor(r, lineage)
}

func captureAtomicReviewReviewerSlots(r *journeyRun) error {
	lineage, err := atomicReviewLineageFor(r)
	if err != nil {
		return err
	}
	return captureAtomicReviewerSlots(r, lineage, false)
}

func requirePendingApproval(lineage string) func(*Sandbox, Observation) error {
	return func(_ *Sandbox, observation Observation) error {
		var finalized struct {
			Action      string `json:"action"`
			LineageID   string `json:"lineage_id"`
			State       string `json:"state"`
			ReceiptPath string `json:"receipt_path"`
		}
		if err := json.Unmarshal([]byte(observation.Stdout), &finalized); err != nil {
			return fmt.Errorf("parse pending approval: %w", err)
		}
		if finalized.LineageID != lineage || finalized.State != "approved" ||
			!strings.Contains(strings.ToLower(finalized.Action), "acknowledgement") || finalized.ReceiptPath != "" {
			return fmt.Errorf("pending approval = %+v, want approved acknowledgement without a receipt path", finalized)
		}
		return nil
	}
}

func requireUnmanagedShippedGate(observation Observation, wantGate string) error {
	var gate struct {
		Result   string         `json:"result"`
		Allowed  bool           `json:"allowed"`
		Delivery string         `json:"delivery"`
		Context  map[string]any `json:"context"`
	}
	if err := json.Unmarshal([]byte(observation.Stdout), &gate); err != nil {
		return fmt.Errorf("parse informational shipped gate: %w", err)
	}
	if observation.ExitCode != 0 || gate.Result != "invalidated" || gate.Allowed || gate.Delivery != "unmanaged" ||
		len(gate.Context) != 1 || gate.Context["gate"] != wantGate {
		return fmt.Errorf("shipped gate = exit %d payload=%+v, want informational unmanaged %s", observation.ExitCode, gate, wantGate)
	}
	for _, forbidden := range []string{"receipt", "lineage", "approval"} {
		if strings.Contains(strings.ToLower(observation.Stdout), forbidden) {
			return fmt.Errorf("shipped unmanaged gate retained deciding %q: %s", forbidden, observation.Stdout)
		}
	}
	return nil
}

func requireManagedAtomicPreCommit(observation Observation, lineage string, wantAllowed bool) error {
	var gate struct {
		Result   string `json:"result"`
		Allowed  bool   `json:"allowed"`
		Delivery string `json:"delivery"`
		Context  struct {
			Gate      string `json:"gate"`
			LineageID string `json:"lineage_id"`
		} `json:"context"`
	}
	if err := json.Unmarshal([]byte(observation.Stdout), &gate); err != nil {
		return fmt.Errorf("parse managed pre-commit gate: %w", err)
	}
	wantResult := "scope-changed"
	if wantAllowed {
		wantResult = "allow"
	}
	if observation.ExitCode != 0 || gate.Result != wantResult || gate.Allowed != wantAllowed || gate.Delivery != "" ||
		gate.Context.Gate != "pre-commit" || gate.Context.LineageID != lineage {
		return fmt.Errorf("managed pre-commit = exit %d payload=%+v, want result=%s allowed=%v lineage=%s", observation.ExitCode, gate, wantResult, wantAllowed, lineage)
	}
	return nil
}

func requireAcknowledgedAtomicPreCommit(r *journeyRun) error {
	observation := r.run([]string{
		"review", "validate", "--gate", "pre-commit", "--lineage", r.sandbox.Lineage, "--cwd", r.sandbox.Repo,
	}, false)
	return requireManagedAtomicPreCommit(observation, r.sandbox.Lineage, true)
}

func requireAtomicReviewHasNoRemote(sandbox *Sandbox) error {
	output, err := exec.Command("git", "-C", sandbox.Repo, "remote").CombinedOutput()
	if err != nil {
		return fmt.Errorf("list local repository remotes: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if remote := strings.TrimSpace(string(output)); remote != "" {
		return fmt.Errorf("local-only RDD fixture unexpectedly has remote %q", remote)
	}
	return nil
}

func mutateAtomicStagedCandidate(sandbox *Sandbox) error {
	path := filepath.Join(sandbox.Repo, "scripts", "atomic-review.sh")
	if err := sandbox.write(path, "#!/bin/sh\nprintf '%s\\n' changed-after-approval\n"); err != nil {
		return err
	}
	return sandbox.git(sandbox.Repo, "add", "--", "scripts/atomic-review.sh")
}

func requireChangedAtomicPreCommitDenied(r *journeyRun) error {
	observation := r.run([]string{
		"review", "validate", "--gate", "pre-commit", "--lineage", r.sandbox.Lineage, "--cwd", r.sandbox.Repo,
	}, false)
	return requireManagedAtomicPreCommit(observation, r.sandbox.Lineage, false)
}

func requireExplicitAtomicFourLensStatusFor(r *journeyRun, lineage string) error {
	status, err := readAtomicReviewStatus(r, lineage)
	if err != nil {
		return err
	}
	if status.Authority.LineageID != lineage || status.Authority.State != "reviewing" ||
		status.NextTransition.Kind != "collect" || status.NextTransition.ReasonCode != "reviewer_results_required" ||
		len(status.NextTransition.Collect.Inputs) != 4 {
		return fmt.Errorf("active STATUS = authority=%+v transition=%+v, want an active four-lens transaction", status.Authority, status.NextTransition)
	}
	return nil
}

func atomicReviewJourneys() []Journey {
	return []Journey{{
		ID:     "j111-approved-acknowledgement-preserves-staged-precommit-authority",
		Title:  "#3867: exact acknowledgement preserves approved authority for the unchanged staged pre-commit candidate",
		Source: "#3867: local-only staged review acknowledgement consumes its continuation, retains the approved lineage, allows the exact candidate, and denies staged drift",
		Steps: []Step{
			{Name: "fixture: local repository", Fixture: baseRepo},
			{Name: "fixture: prove RDD repository has no remote", Fixture: requireAtomicReviewHasNoRemote},
			{Name: "fixture: high-risk candidate staged before review", Fixture: stageAtomicHighRiskCorrectionCandidate},
			{Name: "selectorless STATUS renders and executes the initial printed START", Requires: atomicReviewStatusCapability, Composite: startAtomicReviewFromSelectorlessStatus},
			{Name: "capture every exact four-lens result; the last capture emits acknowledgement", Requires: captureResultCapability, Composite: captureAtomicReviewReviewerSlots},
			{Name: "exact acknowledgement retains the approved authority and consumes only its continuation", Requires: statusCapability, Composite: func(r *journeyRun) error {
				return requireAtomicLineageAcknowledged(r, r.sandbox.Lineage)
			}},
			{Name: "unchanged staged candidate is allowed by its acknowledged lineage", Requires: validateCapability, Composite: requireAcknowledgedAtomicPreCommit},
			{Name: "fixture: mutate and stage the reviewed candidate", Fixture: mutateAtomicStagedCandidate},
			{Name: "changed staged candidate is denied by the same acknowledged lineage", Requires: validateCapability, Composite: requireChangedAtomicPreCommitDenied},
		},
	}}
}
