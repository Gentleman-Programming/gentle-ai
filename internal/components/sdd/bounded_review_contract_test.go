package sdd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/assets"
	"github.com/gentleman-programming/gentle-ai/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

var boundedReviewRequiredClauses = []string{
	"Review is explicit `review/start(target)`",
	"detached, read-only, and terminal after one result",
	"Findings freeze after the initial selected-lens review",
	"canonical empty ledger bytes are exactly `{\"schema\":\"gentle-ai.review-ledger/v1\",\"findings\":[]}`",
	"### Authority-First Terminal Procedure",
	"Hashes cannot reconstruct policy, ledger, or verification-evidence bytes",
	"No mirror mutation may precede its confirmed authority boundary",
	"recover with `review-resume`, never `review-start` or a new budget",
	"existing malformed lineage remains invalid and requires an explicit replacement lineage",
	"in-memory orchestration claim with neutral evidence",
	"MUST NOT serialize `evidence_class` or `status` into the strict native ledger",
	"Deterministic severe findings become `corroborated` with proof and never invoke a refuter",
	"exactly ONE detached refuter operation for the transaction",
	"Insufficient findings become `inconclusive` and are never auto-fixed",
	"one correction transaction composed of atomic work units",
	"immutable genesis path set",
	"maps exactly to frozen accepted/blocking IDs",
	"exactly one scoped fix-delta validator",
	"original acceptance criteria/tests and correction regression evidence",
	"Later observations are non-blocking follow-ups",
	"can return only `approve` or `escalate`",
	"Final verification is independent requirements/runtime verification",
	"Judgment Day replaces ordinary 4R",
	"Only the parent orchestrator may launch a correction actor or scoped validator",
	"openspec/changes/{change-name}/reviews/transaction.json",
	"sdd/{change-name}/review/transaction",
	"gentle-ai review-validate --cwd <repo> --lineage <id> --gate",
	"never hand-author request JSON",
	"base_advanced_compatible",
	"Ed25519",
	"Model, provider, profile, and effort selection remain optional user choices",
}

func TestBoundedReviewContractRendersForEverySupportedAgent(t *testing.T) {
	agents := catalog.AllAgents()
	if len(agents) != 17 {
		t.Fatalf("catalog.AllAgents() = %d, want 17", len(agents))
	}
	for _, agent := range agents {
		t.Run(string(agent.ID), func(t *testing.T) {
			content := renderSDDOrchestratorAsset(agent.ID)
			assertTextContainsClauses(t, string(agent.ID), content, boundedReviewRequiredClauses)
			for _, forbidden := range []string{
				"exactly THREE refuters total",
				"3 total for full-4R",
				"run at most 2 sweeps per lens",
				"standard review or three lens passes sequentially",
				"verifies fix-touched lines",
				"may append fix-caused defects",
			} {
				if strings.Contains(content, forbidden) {
					t.Errorf("rendered %s retains obsolete review clause %q", agent.ID, forbidden)
				}
			}
		})
	}
	if got := sddOrchestratorAsset(model.AgentPi); got != "generic/sdd-orchestrator.md" {
		t.Fatalf("Pi orchestrator asset = %q, want generic adapter", got)
	}
}

func TestRenderedReviewersAreReadOnlyAndSingleResult(t *testing.T) {
	for _, family := range []string{"claude", "cursor", "kimi", "kiro"} {
		for _, lens := range []string{"risk", "readability", "reliability", "resilience"} {
			path := family + "/agents/review-" + lens + ".md"
			t.Run(family+"/"+lens, func(t *testing.T) {
				content := renderBoundedReviewAsset(path)
				for _, want := range []string{"read-only reviewer", "Return one result and terminate", "in-memory orchestration claim with neutral evidence"} {
					if !strings.Contains(content, want) {
						t.Errorf("%s missing %q", path, want)
					}
				}
			})
		}
	}
}

func TestBoundedReviewContractDoesNotEnforceModelPolicy(t *testing.T) {
	content := boundedReviewContract()
	for _, forbidden := range []string{"MUST use model", "required provider", "enforced effort", "mandatory profile"} {
		if strings.Contains(content, forbidden) {
			t.Errorf("bounded review contract enforces model policy with %q", forbidden)
		}
	}
}

func TestAuthorityFirstTerminalProcedureIsStructuredAndMirrorEligibilityIsClosed(t *testing.T) {
	rows := parseAuthorityFirstRows(t, authorityFirstTerminalProcedure())
	wantOperations := []string{
		"preserve-preimages", "review-start", "record-lens-result", "freeze-findings", "classify-evidence",
		"review-resume-preterminal", "begin-final-verification", "independent-final-verification", "complete-final-verification",
		"review-resume-terminal", "review-bundle-export", "extract-terminal-receipt", "construct-gate-request",
		"review-validate", "reconcile-terminal-mirrors",
	}
	if len(rows) != len(wantOperations) {
		t.Fatalf("authority-first rows = %d, want %d", len(rows), len(wantOperations))
	}
	for index, want := range wantOperations {
		row := rows[index]
		if row.order != index+1 || row.operation != want {
			t.Fatalf("authority-first row[%d] = %#v, want operation %q", index, row, want)
		}
		wantEligibility := "blocked"
		if index == len(wantOperations)-1 {
			wantEligibility = "allowed"
		}
		if row.mirrorEligibility != wantEligibility {
			t.Fatalf("authority-first row[%d] mirror eligibility = %q, want %q", index, row.mirrorEligibility, wantEligibility)
		}
	}
}

func TestAuthorityFirstLifecycleRendersIdenticallyForEverySupportedAgent(t *testing.T) {
	procedure := authorityFirstTerminalProcedure()
	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			content := renderSDDOrchestratorAsset(agent.ID)
			if strings.Count(content, procedure) != 1 {
				t.Fatal("rendered orchestrator does not contain exactly one canonical terminal procedure")
			}
		})
	}
}

func TestOpenCodeAndClaudeApplyCommandsRequireAuthorityBeforeMirrors(t *testing.T) {
	for _, path := range []string{"opencode/commands/sdd-apply.md", "claude/commands/sdd-apply.md"} {
		t.Run(path, func(t *testing.T) {
			raw := assets.MustRead(path)
			if strings.Count(raw, authorityFirstProcedurePlaceholder) != 1 {
				t.Fatalf("%s must reference the centralized terminal procedure exactly once", path)
			}
			content := renderBoundedReviewAsset(path)
			if strings.Contains(content, authorityFirstProcedurePlaceholder) || strings.Count(content, authorityFirstTerminalProcedure()) != 1 {
				t.Fatalf("%s did not render the centralized terminal procedure", path)
			}
		})
	}
}

type authorityFirstRow struct {
	order             int
	operation         string
	mirrorEligibility string
}

func parseAuthorityFirstRows(t *testing.T, content string) []authorityFirstRow {
	t.Helper()
	rows := make([]authorityFirstRow, 0, 15)
	for _, line := range strings.Split(content, "\n") {
		if len(line) < 4 || line[0] != '|' || line[2] < '0' || line[2] > '9' {
			continue
		}
		fields := strings.Split(line, "|")
		if len(fields) != 6 {
			t.Fatalf("malformed authority-first table row %q", line)
		}
		var order int
		if _, err := fmt.Sscanf(strings.TrimSpace(fields[1]), "%d", &order); err != nil {
			t.Fatalf("parse authority-first order: %v", err)
		}
		rows = append(rows, authorityFirstRow{
			order: order, operation: strings.Trim(strings.TrimSpace(fields[2]), "`"),
			mirrorEligibility: strings.TrimSpace(fields[4]),
		})
	}
	return rows
}
