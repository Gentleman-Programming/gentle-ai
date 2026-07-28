package sdd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

var boundedReviewRequiredClauses = []string{
	"Parent orchestrator and native CLI only",
	"gentle-ai review start",
	"selected lens once in the foreground",
	"assembled from this exact START response as one JSON object",
	"`lineage` from `lineage_id`, `target` from `target_identity`, `lens`/`order` from `lens_bindings`",
	"gentle-ai review capture-result",
	"repeated `--result-artifact-file <path>` arguments",
	"BOM-less UTF-8 on Windows PowerShell 5.1",
	"POSIX inline `--result-artifact '<manifest-json>'` form remains compatible",
	"Native Go validates, canonicalizes, persists, hashes, reopens, and binds results",
	"Only `introduced`, `behavior-activated`, or `worsened`",
	"Route `pre-existing` and `base-only` to follow-ups; `unknown` escalates",
	"canonical four-lens selection is long work",
	"one cost/side-effect forecast",
	"four reviewer model runs",
	"--consent relay",
	"Lossless Blocking Prompt",
	"not the kill switch",
	"one correction transaction",
	"positive `--correction-lines` forecast before editing",
	"one read-only scoped fix validator",
	"one independent requirements/runtime verification",
	"### Authority-First Terminal Procedure",
	"rerun the same facade operation",
	"Repository Git common-dir CAS remains authoritative",
	"Existing transaction, policy, ledger, receipt, bundle, and gate-context schemas",
	"gentle-ai review validate --gate <gate>",
	"Model/provider/profile selection remains user-owned",
}

func TestBoundedReviewContractRendersForEverySupportedAgent(t *testing.T) {
	agents := catalog.AllAgents()
	if len(agents) != 16 {
		t.Fatalf("catalog.AllAgents() = %d, want 16", len(agents))
	}
	for _, agent := range agents {
		t.Run(string(agent.ID), func(t *testing.T) {
			content := renderSDDOrchestratorAsset(agent.ID)
			assertTextContainsClauses(t, string(agent.ID), content, boundedReviewRequiredClauses)
			// The retired WorkRun commands are gone from the assets, so nothing
			// here may require them. internal/assets/assets_test.go owns the
			// inverse assertion that they never come back.
			if strings.Contains(content, runtimeAgentIDPlaceholder) {
				t.Errorf("rendered %s retains runtime agent placeholder", agent.ID)
			}
			for _, forbidden := range []string{"review-start", "review-step", "review-resume", "review-validate", "review-bundle-export", "review-bundle-import"} {
				if strings.Contains(content, forbidden) {
					t.Errorf("rendered %s exposes lower-level compatibility command %q", agent.ID, forbidden)
				}
			}
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
	for _, forbidden := range []string{"review-start", "review-step", "review-resume", "review-validate", "review-bundle-export", "review-bundle-import"} {
		if strings.Contains(boundedReviewContract(), forbidden) {
			t.Errorf("orchestrator contract exposes lower-level compatibility command %q", forbidden)
		}
	}
	if got := sddOrchestratorAsset(model.AgentPi); got != "generic/sdd-orchestrator.md" {
		t.Fatalf("Pi orchestrator asset = %q, want generic adapter", got)
	}
}

func TestPreservedSharedOrchestratorSubstitutesRuntimeAgentIdentity(t *testing.T) {
	t.Parallel()

	preserved := strings.Join([]string{
		"Bind this to the dedicated `sdd-orchestrator` agent only.",
		runtimeAgentIDPlaceholder,
	}, "\n")
	rendered := renderPreservedOpenCodeOrchestratorPrompt(
		preserved,
		model.AgentKilocode,
	)
	if !strings.Contains(rendered, "Bind this to the dedicated `gentle-orchestrator` agent only.") {
		t.Fatalf("preserved prompt lost its migration:\n%s", rendered)
	}
	if strings.Contains(rendered, runtimeAgentIDPlaceholder) {
		t.Fatal("preserved prompt retained runtime agent placeholder")
	}
	if !strings.Contains(rendered, string(model.AgentKilocode)) {
		t.Fatalf("preserved prompt missing runtime agent identity:\n%s", rendered)
	}
}

// The retired WorkRun commands no longer exist, so a preserved prompt that
// still mentions them must pass through migration untouched instead of being
// rewritten into a better-formed invocation of a deleted command.
func TestPreservedSharedOrchestratorLeavesRetiredWorkCommandsUntouched(t *testing.T) {
	t.Parallel()

	retired := []string{
		"gentle-ai work-capabilities --cwd <repo> --contract gentle-ai.work-capabilities/v2 --json",
		"gentle-ai work-start --cwd <repo> --contract gentle-ai.work-start/v1 --json",
	}
	rendered := renderPreservedOpenCodeOrchestratorPrompt(
		strings.Join(retired, "\n"),
		model.AgentKilocode,
	)
	for _, command := range retired {
		if !strings.Contains(rendered, command) {
			t.Fatalf("preserved prompt rewrote retired command %q:\n%s", command, rendered)
		}
	}
	if strings.Contains(rendered, "--agent "+string(model.AgentKilocode)) {
		t.Fatalf("preserved prompt injected an adapter identity into a retired command:\n%s", rendered)
	}
}

func TestRenderedReviewersAreReadOnlyAndSingleResult(t *testing.T) {
	for _, family := range []string{"claude", "cursor", "kimi", "kiro"} {
		for _, lens := range []string{"risk", "readability", "reliability", "resilience"} {
			path := family + "/agents/review-" + lens + ".md"
			t.Run(family+"/"+lens, func(t *testing.T) {
				content := renderBoundedReviewAsset(path)
				for _, want := range []string{"read-only reviewer", "immutable candidate diff once", "## Candidate-Causal Admission", "Return one JSON object and no prose", `"subject_hash":"<artifact_subject.subject_hash>"`, `"inspection":{"status":"completed","paths":["<every changed_path_manifest.path in exact order>"]}`, "Never emit summary, skill_resolution, or any other unknown field", "evidence contains only genuine inspection evidence"} {
					if !strings.Contains(content, want) {
						t.Errorf("%s missing %q", path, want)
					}
				}
			})
		}
	}
}

func TestJudgmentDayReviewersUseNativeResultSchema(t *testing.T) {
	for name, content := range map[string]string{
		"rendered contract": judgmentDayReviewerContract(),
		"skill reference":   assets.MustRead("skills/judgment-day/references/prompts-and-formats.md"),
	} {
		for _, want := range []string{nativeReviewerResultSchema, "Never emit", "skill_resolution", "unknown field", "orchestration metadata outside the native result JSON", `{"findings":[],"evidence":["what was inspected"]}`} {
			if !strings.Contains(content, want) {
				t.Errorf("%s missing %q", name, want)
			}
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

func TestBoundedReviewContractListsOnlySupportedLifecycleGates(t *testing.T) {
	content := boundedReviewContract()
	for _, gate := range []string{"post-apply", "pre-commit", "pre-push", "pre-pr", "release"} {
		if !strings.Contains(content, gate) {
			t.Errorf("contract missing supported gate %q", gate)
		}
	}
	if strings.Contains(content, "archive, incident") {
		t.Error("contract promises archive as a lifecycle CLI gate")
	}
	for _, clause := range []string{"structured status", "reviewGate.result: allow", "approved receipt"} {
		if !strings.Contains(content, clause) {
			t.Errorf("contract missing archive readiness check %q", clause)
		}
	}
}

func TestAuthorityFirstTerminalProcedureIsStructuredAndMirrorEligibilityIsClosed(t *testing.T) {
	rows := parseAuthorityFirstRows(t, authorityFirstTerminalProcedure())
	wantOperations := []string{
		"gentle-ai review start", "gentle-ai review finalize",
		"gentle-ai review validate --gate <gate> --cwd <repo>", "reconcile-terminal-mirrors",
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
