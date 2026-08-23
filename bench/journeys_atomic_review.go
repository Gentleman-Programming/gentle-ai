package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// retiredAtomicJourneyReplacements records every ordinary-path journey that
// asserted the pre-#3417 durable-receipt or deciding-gate model. They are not
// weakened: the registered corpus replaces that retired surface with j59, j60,
// and j111's worktree-bound, explicit-active, and terminal-burn journeys.
var retiredAtomicJourneyReplacements = map[string]string{
	"j01-docs-happy-path":                                                  "j111-approved-transaction-burns-and-shipped-gates-are-unmanaged",
	"j06-pre-push-after-publication":                                       "j111-approved-transaction-burns-and-shipped-gates-are-unmanaged",
	"j07-disabled-with-stale-receipts":                                     "j111-approved-transaction-burns-and-shipped-gates-are-unmanaged",
	"j15-linked-worktree":                                                  "j59-current-status-and-start-ignore-sibling-worktree-transaction",
	"j32-recovery-of-a-recovery":                                           "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j33-escalate-then-recover":                                            "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j37-sdd-bound-passing-attempt-closes-over-a-corrected-candidate":      "j111-approved-transaction-burns-and-shipped-gates-are-unmanaged",
	"j43-recovery-guard-rails-as-an-operator-meets-them":                   "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j44-corrected-current-changes-delivery":                               "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j45-completed-final-verification-retry":                               "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j46-correction-required-staged-recovery":                              "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j47-disabled-mode-archives-discovered-scope-changed-authority":        "j111-approved-transaction-burns-and-shipped-gates-are-unmanaged",
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

var atomicBurnFinalizeCapability = &Capability{Verb: []string{"review", "finalize"}, Flags: []string{
	"--cwd", "--lineage", "--captured-results", "--captured-evidence",
}}

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

func finalizeAtomicBurnReviewerSlots(r *journeyRun) error {
	lineage, err := atomicBurnLineageFor(r)
	if err != nil {
		return err
	}
	observation := r.run(productArgsFor(r, "review", "finalize", "--lineage", lineage, "--captured-results=true"), false)
	if observation.ExitCode != 0 {
		return fmt.Errorf("finalize captured atomic results: %s", firstLine(observation.Stderr))
	}
	return nil
}

func captureAtomicFinalEvidenceDescriptorFor(r *journeyRun, lineage, evidenceName string) error {
	after, err := executeV5CaptureEvidenceDescriptor(r, lineage, evidenceName)
	if err != nil {
		return err
	}
	if after.NextTransition == nil || after.NextTransition.Kind != "execute" || after.NextTransition.Execute == nil ||
		after.NextTransition.Execute.Operation != "review.finalize" {
		return fmt.Errorf("atomic capture evidence did not advance to a runnable review.finalize: %+v", after.NextTransition)
	}
	return nil
}

func captureAtomicBurnEvidence(r *journeyRun) error {
	lineage, err := atomicBurnLineageFor(r)
	if err != nil {
		return err
	}
	return captureAtomicFinalEvidenceDescriptorFor(r, lineage, "atomic-burn-evidence.txt")
}

func captureJ02FinalEvidence(r *journeyRun) error {
	if r.sandbox.Lineage == "" {
		return fmt.Errorf("j02 has no lineage for its v2 capture-evidence descriptor")
	}
	return captureAtomicFinalEvidenceDescriptorFor(r, r.sandbox.Lineage, "j02-final-evidence.txt")
}

func finalizeAtomicBurnEvidence(r *journeyRun) error {
	lineage, err := atomicBurnLineageFor(r)
	if err != nil {
		return err
	}
	observation := r.run(productArgsFor(r, "review", "finalize", "--lineage", lineage, "--captured-evidence=true"), false)
	if observation.ExitCode != 0 {
		return fmt.Errorf("finalize atomic evidence: %s", firstLine(observation.Stderr))
	}
	return requireBurnedApproval(lineage)(r.sandbox, observation)
}

func requireBurnedApproval(lineage string) func(*Sandbox, Observation) error {
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
		Title:  "#3417: selectorless STATUS renders a printed START, approval burns it, and repeat START creates a new transaction",
		Source: "#3417: selectorless STATUS owns compact binding; no approval receipt or evidence survives the terminal transaction, and delivery gates remain informational",
		Steps: []Step{
			{Name: "fixture: repository", Fixture: baseRepo},
			{Name: "fixture: high-risk candidate", Fixture: stageAtomicHighRiskCorrectionCandidate},
			{Name: "selectorless STATUS renders and executes the initial printed START", Requires: atomicReviewStatusCapability, Composite: startAtomicBurnFromSelectorlessStatus},
			{Name: "capture every exact four-lens result", Requires: captureResultCapability, Composite: captureAtomicBurnReviewerSlots},
			{Name: "finalize captured lens results", Requires: atomicBurnFinalizeCapability, Composite: finalizeAtomicBurnReviewerSlots},
			{Name: "capture final verification evidence", Requires: captureEvidenceCapability, Composite: captureAtomicBurnEvidence},
			{Name: "approval burns the exact transaction with no reusable receipt or evidence", Requires: atomicBurnFinalizeCapability, Composite: finalizeAtomicBurnEvidence},
			{Name: "all shipped gates are informational, non-deciding, and unmanaged", Requires: validateCapability, Composite: requireAllUnmanagedShippedGates},
			{Name: "repeat the selectorless STATUS request and execute its printed START as a new transaction", Requires: atomicReviewStatusCapability, Composite: requireAtomicBurnStartsNewTransaction},
		},
	}}
}
