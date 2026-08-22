package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCorpusJourneysDeclareReviewMode(t *testing.T) {
	for _, journey := range allDeclaredJourneys() {
		if journey.Review != reviewOptedIn && journey.Review != reviewUntouched {
			t.Errorf("journey %q declares review mode %q; every runnable journey must explicitly opt in or remain untouched", journey.ID, journey.Review)
		}
	}
}

func TestCorpusRejectsRetiredOrdinaryPathReviewJourneys(t *testing.T) {
	replacedByAtomicJourneys := map[string]string{
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

	for _, journey := range allDeclaredJourneys() {
		if replacement, stale := replacedByAtomicJourneys[journey.ID]; stale {
			t.Errorf("retired ordinary-path journey %q is still registered; #3417 replaces it with %q", journey.ID, replacement)
		}
	}
}

func TestCorpusDoesNotReintroduceRetiredReceiptAndGatePins(t *testing.T) {
	retiredPins := []string{
		"receipt survives",
		"receipt preserves",
		"receipt allows",
		"pre-push allows",
		"pre-pr binds",
		"gate allows",
		"gate denies",
		"gate blocks",
		"delivery is discovered selector-free",
		"staged delivery candidate can validate",
	}
	for _, journey := range allDeclaredJourneys() {
		if journey.ID == "j44-sdd-historical-requirement-stale-pass" || journey.ID == "j85-review-parse-refusals-are-preflight" {
			// TestHistoricalCompatibilityJourneysAreNarrowlyNamed constrains the
			// only retained historical parser/refusal compatibility fixtures.
			continue
		}
		declaration := strings.ToLower(journey.Title)
		for _, step := range journey.Steps {
			declaration += " " + strings.ToLower(step.Name)
		}
		for _, pin := range retiredPins {
			if strings.Contains(declaration, pin) {
				t.Errorf("journey %q reintroduced retired #3417 receipt/gate pin %q: %s", journey.ID, pin, declaration)
			}
		}
	}
}

func TestHistoricalCompatibilityJourneysAreNarrowlyNamed(t *testing.T) {
	for _, journey := range allDeclaredJourneys() {
		if journey.ID != "j44-sdd-historical-requirement-stale-pass" && journey.ID != "j85-review-parse-refusals-are-preflight" {
			continue
		}
		declaration := strings.ToLower(journey.ID + " " + journey.Title + " " + journey.Source)
		if !strings.Contains(declaration, "historical") || !strings.Contains(declaration, "compatibility") {
			t.Errorf("historical compatibility journey %q must say historical and compatibility: %s", journey.ID, declaration)
		}
	}
}

func TestBenchRunnerDoesNotRestoreNewLineageEnvironmentActivation(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("runner.go"))
	if err != nil {
		t.Fatalf("read runner.go: %v", err)
	}
	if strings.Contains(string(content), "GENTLE_AI_RDD_NEW_LINEAGE") {
		t.Fatal("bench runner restored retired GENTLE_AI_RDD_NEW_LINEAGE ambient-authority activation")
	}
}

func TestFinalAtomicMigrationJourneysRatifyIssue3417(t *testing.T) {
	journeys := map[string]Journey{}
	for _, journey := range Journeys() {
		journeys[journey.ID] = journey
	}

	for id, required := range map[string][]string{
		"j104-repository-context-survives-fresh-process":                     {"#3417", "target", "repository", "worktree", "subject"},
		"j106-captured-provider-validator-slot-finalizes-generically":        {"#3417", "exact active lineage", "provider validator"},
		"j51-negotiated-status-correction-continuation":                      {"#3417", "selectorless", "exact active lineage"},
		"j67-v5-capture-evidence-correction-descriptor-executes":             {"#3417", "exact active lineage", "correction"},
		"j91-audited-abandon-preplan-over-budget-correction":                 {"#3417", "exact active lineage", "abandon"},
		"j95-targeted-validator-inspects-provider-bound-corrected-tree":      {"#3417", "exact active lineage", "targeted validator"},
		"j12-rejected-capture-then-recapture":                                {"#3417", "exact active lineage", "recapture"},
		"j77-capture-result-input-preflight-is-read-only":                    {"#3417", "exact active lineage", "preflight"},
		"j78-lens-finding-id-prefix-discovery":                               {"#3417", "exact active lineage", "finding"},
		"j66-v5-capture-evidence-descriptors-execute":                        {"#3417", "exact active lineage", "full selected lens set"},
		"j107-sdd-approved-active-change-allows-shared-openspec-scaffolding": {"#3417", "unmanaged", "ordinary policy"},
		"j108-sdd-post-review-verify-report-is-natively-bound":               {"#3417", "unmanaged", "ordinary policy"},
		"j109-sdd-legacy-post-review-report-requires-current-attestation":    {"#3417", "unmanaged", "ordinary policy"},
	} {
		journey, ok := journeys[id]
		if !ok {
			t.Errorf("missing final #3417 migration journey %q", id)
			continue
		}
		declaration := strings.ToLower(journey.Source + " " + journey.Title)
		for _, step := range journey.Steps {
			declaration += " " + strings.ToLower(step.Name)
		}
		for _, phrase := range required {
			if !strings.Contains(declaration, strings.ToLower(phrase)) {
				t.Errorf("final #3417 migration journey %q declaration does not name %q: %s", id, phrase, declaration)
			}
		}
	}
}

func TestAtomicReviewJourneysRatifyIssue3417(t *testing.T) {
	journeys := map[string]Journey{}
	for _, journey := range Journeys() {
		journeys[journey.ID] = journey
	}

	for id, required := range map[string][]string{
		"j59-current-status-and-start-ignore-sibling-worktree-transaction": {
			"#3417", "selectorless STATUS", "START", "sibling worktree",
		},
		"j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow": {
			"#3417", "four lenses", "correction", "validator",
		},
		"j111-approved-transaction-burns-and-shipped-gates-are-unmanaged": {
			"#3417", "selectorless STATUS", "printed START", "new transaction",
		},
		"j02-high-risk-four-lens": {
			"#3417", "actual four-lens capture", "burns", "terminal transaction",
		},
		"j04-size-does-not-escalate": {
			"#3417", "low risk", "burns", "terminal transaction",
		},
		"j05-gate-without-any-review": {
			"#3417", "informational", "unmanaged",
		},
		"j89-staged-validation-is-informational-and-unmanaged": {
			"#3417", "staged", "informational", "unmanaged",
		},
		"j110-untracked-terminal-burn-and-unmanaged-staged-validation": {
			"#3417", "untracked", "burn", "unmanaged",
		},
	} {
		journey, ok := journeys[id]
		if !ok {
			t.Errorf("missing #3417 journey %q", id)
			continue
		}
		if journey.Review != reviewOptedIn {
			t.Errorf("#3417 journey %q review mode = %q, want %q", id, journey.Review, reviewOptedIn)
		}
		declaration := journey.Source + " " + journey.Title
		for _, step := range journey.Steps {
			declaration += " " + step.Name
		}
		for _, phrase := range required {
			if !strings.Contains(strings.ToLower(declaration), strings.ToLower(phrase)) {
				t.Errorf("#3417 journey %q declaration does not name %q: %s", id, phrase, declaration)
			}
		}
	}
}
