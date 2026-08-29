package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// retiredAtomicJourneyReplacements records every ordinary-path journey that
// asserted the pre-#3417 durable-receipt or deciding-gate model. #3564
// reactivates j47 for disabled-mode V2 structural-absence/archive routing: it
// no longer pins durable-receipt or deciding-gate behavior. The remaining
// entries are not weakened: the registered corpus replaces that retired surface
// with j59, j60, and j111's worktree-bound, explicit-active, and terminal-burn
// journeys.
var retiredAtomicJourneyReplacements = map[string]string{
	"j01-docs-happy-path":                                                  "j111-approved-transaction-burns-and-shipped-gates-are-unmanaged",
	"j06-pre-push-after-publication":                                       "j111-approved-transaction-burns-and-shipped-gates-are-unmanaged",
	"j07-disabled-with-stale-receipts":                                     "j111-approved-transaction-burns-and-shipped-gates-are-unmanaged",
	"j15-linked-worktree":                                                  "j59-current-status-and-start-ignore-sibling-worktree-transaction",
	"j32-recovery-of-a-recovery":                                           "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j33-escalate-then-recover":                                            "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j43-recovery-guard-rails-as-an-operator-meets-them":                   "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j44-corrected-current-changes-delivery":                               "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j45-completed-final-verification-retry":                               "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j46-correction-required-staged-recovery":                              "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j48-recovered-workspace-preserves-full-candidate-scope":               "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j50-candidate-decline-denies-generically-then-disabled":               "j111-approved-transaction-burns-and-shipped-gates-are-unmanaged",
	"j52-sdd-stale-authority-does-not-shadow-approved-candidate":           "j59-current-status-and-start-ignore-sibling-worktree-transaction",
	"j53-sdd-ambiguous-authorities-fail-closed":                            "j59-current-status-and-start-ignore-sibling-worktree-transaction",
	"j54-sdd-missing-authority-receipt-fails-closed":                       "j111-approved-transaction-burns-and-shipped-gates-are-unmanaged",
	"j55-sdd-mismatched-authority-receipt-fails-closed":                    "j111-approved-transaction-burns-and-shipped-gates-are-unmanaged",
	"j56-sdd-non-allow-post-apply-gate-fails-closed":                       "j111-approved-transaction-burns-and-shipped-gates-are-unmanaged",
	"j58-sdd-foreign-openspec-path-fails-closed":                           "j59-current-status-and-start-ignore-sibling-worktree-transaction",
	"j61-pre-pr-multi-segment-delivery-denies-without-composition":         "j111-approved-transaction-burns-and-shipped-gates-are-unmanaged",
	"j65-selectorless-committed-correction-continuation":                   "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j76-scope-changed-four-lens-successor":                                "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j82-reviewed-superset-pre-push-allows-unpublished-subset":             "j111-approved-transaction-burns-and-shipped-gates-are-unmanaged",
	"j83-pre-pr-moving-advertised-base-binds-merge-base":                   "j111-approved-transaction-burns-and-shipped-gates-are-unmanaged",
	"j86-approved-base-diff-local-parent-merge-preserves-approved-receipt": "j111-approved-transaction-burns-and-shipped-gates-are-unmanaged",
	"j90-explicit-frozen-reviewing-lineage-resumes-after-drift":            "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j94-escalated-changed-scope-negotiates-recovery":                      "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j97-pre-push-preserves-ls-remote-failure":                             "j111-approved-transaction-burns-and-shipped-gates-are-unmanaged",
	"j100-pre-push-unqualified-selector-ignores-unreachable-remote":        "j111-approved-transaction-burns-and-shipped-gates-are-unmanaged",
	// #3587 removes the public FINALIZE/evidence/retry path. These scenarios'
	// sole subjects were that retired surface; the replacements below keep the
	// corresponding clean close, correction, and unmanaged-delivery evidence.
	"j03-kill-switch":                                                    "j111-approved-transaction-burns-and-shipped-gates-are-unmanaged",
	"j04-size-does-not-escalate":                                         "j111-approved-transaction-burns-and-shipped-gates-are-unmanaged",
	"j08-finalize-without-reviewer-results":                              "j114-last-reviewer-capture-closes-and-burns",
	"j09-finalize-without-evidence":                                      "j114-last-reviewer-capture-closes-and-burns",
	"j11-unborn-head":                                                    "j110-untracked-terminal-burn-and-unmanaged-staged-validation",
	"j16-detached-head":                                                  "j114-last-reviewer-capture-closes-and-burns",
	"j18-space-and-non-ascii-path":                                       "j114-last-reviewer-capture-closes-and-burns",
	"j19-submodule-gitlink":                                              "j114-last-reviewer-capture-closes-and-burns",
	"j20-symlink-candidate":                                              "j114-last-reviewer-capture-closes-and-burns",
	"j21-mode-only-change":                                               "j114-last-reviewer-capture-closes-and-burns",
	"j22-pure-rename":                                                    "j114-last-reviewer-capture-closes-and-burns",
	"j23-deletion-only":                                                  "j114-last-reviewer-capture-closes-and-burns",
	"j24-empty-file":                                                     "j114-last-reviewer-capture-closes-and-burns",
	"j25-no-trailing-newline":                                            "j114-last-reviewer-capture-closes-and-burns",
	"j26-crlf-content":                                                   "j114-last-reviewer-capture-closes-and-burns",
	"j27-merge-in-progress":                                              "j114-last-reviewer-capture-closes-and-burns",
	"j28-rebase-in-progress":                                             "j114-last-reviewer-capture-closes-and-burns",
	"j29-cherry-pick-in-progress":                                        "j114-last-reviewer-capture-closes-and-burns",
	"j30-kill-switch-flipped-mid-review":                                 "j111-approved-transaction-burns-and-shipped-gates-are-unmanaged",
	"j35-correction-budget-exactly-zero":                                 "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j66-v5-capture-evidence-descriptors-execute":                        "j114-last-reviewer-capture-closes-and-burns",
	"j67-v5-capture-evidence-correction-descriptor-executes":             "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j85-review-parse-refusals-are-preflight":                            "j114-last-reviewer-capture-closes-and-burns",
	"j91-audited-abandon-preplan-over-budget-correction":                 "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j95-targeted-validator-inspects-provider-bound-corrected-tree":      "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j99-issue-2906-finalize-missing-contract":                           "j114-last-reviewer-capture-closes-and-burns",
	"j107-sdd-approved-active-change-allows-shared-openspec-scaffolding": "j111-approved-transaction-burns-and-shipped-gates-are-unmanaged",
	"j108-sdd-post-review-verify-report-is-natively-bound":               "j111-approved-transaction-burns-and-shipped-gates-are-unmanaged",
	"j109-sdd-legacy-post-review-report-requires-current-attestation":    "j111-approved-transaction-burns-and-shipped-gates-are-unmanaged",
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

// j111 proves #3417's terminal boundary at the built-binary surface. Approval is
// the end of the exact transaction, not a receipt that later gates can reuse.
func atomicBurnLineageFor(r *journeyRun) (string, error) {
	if r.sandbox.Lineage == "" {
		return "", fmt.Errorf("atomic burn journey has no selectorless START lineage")
	}
	return r.sandbox.Lineage, nil
}

func startAtomicBurnFromSelectorlessStatus(r *journeyRun) error {
	lineage, err := startAtomicTransactionFromSelectorlessStatus(r)
	if err != nil {
		return fmt.Errorf("initial selectorless START: %w", err)
	}
	r.sandbox.Lineage = lineage
	r.sandbox.Scratch[atomicBurnInitialKey] = lineage
	return requireExplicitAtomicFourLensStatusFor(r, lineage)
}

func captureAtomicBurnReviewerSlots(r *journeyRun) error {
	lineage, err := atomicBurnLineageFor(r)
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
			return fmt.Errorf("parse burned approval: %w", err)
		}
		if finalized.LineageID != lineage || finalized.State != "approved" ||
			!strings.Contains(strings.ToLower(finalized.Action), "burn") || finalized.ReceiptPath != "" {
			return fmt.Errorf("burned approval = %+v, want approved #3417 terminal action without a receipt path", finalized)
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

func requireAllUnmanagedShippedGates(r *journeyRun) error {
	for _, gate := range []string{"post-apply", "pre-commit", "pre-push", "pre-pr", "release"} {
		observation := r.run([]string{"review", "validate", "--gate", gate, "--cwd", r.sandbox.Repo}, false)
		if err := requireUnmanagedShippedGate(observation, gate); err != nil {
			return err
		}
	}
	return nil
}

func requireAtomicBurnStartsNewTransaction(r *journeyRun) error {
	burnedLineage := r.sandbox.Scratch[atomicBurnInitialKey]
	if burnedLineage == "" {
		return fmt.Errorf("atomic burn journey did not record the burned selectorless binding")
	}
	// The selectorless lineage is target-derived and may therefore repeat. The
	// preceding selectorless STATUS must have no active authority, and the exact
	// rendered START must answer created/reviewing, which proves this is a new
	// compact transaction rather than reusable burned authority or inventory.
	newLineage, err := startAtomicTransactionFromSelectorlessStatus(r)
	if err != nil {
		return fmt.Errorf("selectorless START after burn did not create a new transaction: %w", err)
	}
	r.sandbox.Lineage = newLineage
	return requireExplicitAtomicFourLensStatusFor(r, newLineage)
}

func requireExplicitAtomicFourLensStatusFor(r *journeyRun, lineage string) error {
	status, err := readAtomicReviewStatus(r, lineage)
	if err != nil {
		return err
	}
	if status.Authority.LineageID != lineage || status.Authority.State != "reviewing" ||
		status.NextTransition.Kind != "collect" || status.NextTransition.ReasonCode != "reviewer_results_required" ||
		len(status.NextTransition.Collect.Inputs) != 4 {
		return fmt.Errorf("post-burn STATUS = authority=%+v transition=%+v, want a new active four-lens transaction", status.Authority, status.NextTransition)
	}
	return nil
}

func atomicReviewJourneys() []Journey {
	return []Journey{{
		ID:     "j111-approved-transaction-burns-and-shipped-gates-are-unmanaged",
		Title:  "#3797: selectorless STATUS renders a printed START, the last lens emits acknowledgement, and repeat START follows the exact burn",
		Source: "#3797: selectorless STATUS owns compact binding; terminal approval awaits exact acknowledgement, leaves no receipt or sidecar, and delivery gates remain informational",
		Steps: []Step{
			{Name: "fixture: repository", Fixture: baseRepo},
			{Name: "fixture: high-risk candidate", Fixture: stageAtomicHighRiskCorrectionCandidate},
			{Name: "selectorless STATUS renders and executes the initial printed START", Requires: atomicReviewStatusCapability, Composite: startAtomicBurnFromSelectorlessStatus},
			{Name: "capture every exact four-lens result; the last capture emits acknowledgement before the exact burn", Requires: captureResultCapability, Composite: captureAtomicBurnReviewerSlots},
			{Name: "the exact acknowledgement leaves no reusable authority, receipt, or evidence", Requires: statusCapability, Composite: func(r *journeyRun) error {
				return requireAtomicLineageAcknowledged(r, r.sandbox.Lineage)
			}},
			{Name: "all shipped gates are informational, non-deciding, and unmanaged", Requires: validateCapability, Composite: requireAllUnmanagedShippedGates},
			{Name: "repeat the selectorless STATUS request and execute its printed START as a new transaction", Requires: atomicReviewStatusCapability, Composite: requireAtomicBurnStartsNewTransaction},
		},
	}}
}
