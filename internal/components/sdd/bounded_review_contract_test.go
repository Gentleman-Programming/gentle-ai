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
	"gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --next-transition",
	"route only from the returned `next_transition`",
	"exact operation and ordered argument tokens unchanged",
	"exact `review.capture-result` collection input once in the foreground",
	"exact literal prefix `GENTLE_AI_REVIEW_BINDING `",
	"including the trailing space and never `=`",
	"These are the prompt's first bytes",
	"one-line JSON assembled only from that input",
	"`revision` from `expected-revision`",
	"`subject_hash` from `artifact_subject.subject_hash`",
	"Capture follows the native transition",
	"via repeated `--result-artifact-file <path>`",
	"BOM-less UTF-8 on Windows PowerShell 5.1",
	"POSIX inline `--result-artifact '<manifest-json>'` and provider-owned `--captured-results` remain compatible",
	"Native Go owns validation, canonicalization, persistence, hashing, reopening, and binding",
	"Only candidate-caused severe findings block",
	"pre-existing/base-only become follow-ups, unknown escalates",
	"canonical four-lens selection is long work",
	"one cost/side-effect forecast",
	"four reviewer model runs",
	"typed `gentle-ai.review-integration.consent/v2` envelope",
	"Lossless Blocking Prompt",
	"not the kill switch",
	"one correction transaction",
	"positive forecast before editing",
	"one read-only scoped fix validator",
	// The fix validator's capability is named because leaving it unnamed cost a
	// real correction attempt: an orchestrator routed targeted validation to the
	// refuter, which has no shell by design, and its inconclusive answer was
	// submitted as a failed check that escalated the lineage irreversibly.
	"must hold read-only Git execution against the immutable trees",
	"never route it to the refuter or any other actor that cannot run Git",
	"produced no verdict",
	"surface one blocked human decision and submit nothing",
	"one independent requirements/runtime verification",
	"### Authority-First Terminal Procedure",
	"query STATUS again",
	"Repository Git common-dir CAS remains authoritative",
	"Existing transaction, policy, ledger, receipt, bundle, and gate-context schemas",
	"exact returned `review.validate`",
	"Model/provider/profile selection remains user-owned",
}

func TestBoundedReviewContractRequiresProviderOwnedReviewerContext(t *testing.T) {
	content := boundedReviewContract()
	for _, want := range []string{
		"Never hand candidate bytes through `/tmp`",
		"another external file",
		"a repository scratch file",
		"`GENTLE_AI_FROZEN_CANDIDATE_CONTEXT`",
		"OpenCode preflights the opaque binding, discards the caller-authored task body",
		"injects only the provider's `artifact_subject`, `base_tree`, `candidate_tree`, and ordered manifest",
		"broad deny precedes narrow allows",
		"Runtimes that cannot enforce a per-command shell boundary expose no shell and stop incomplete",
		"read-only native Git commands against those exact immutable trees",
		"compact `--name-status`/`--numstat` discovery",
		"replacement objects, external diff and textconv, forces `--text`",
		"literal pathspecs",
		"Never pass `--binary`",
		"Never add `candidate_diff`",
		"read live worktree/index/HEAD",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("orchestrator contract missing reviewer context rule %q", want)
		}
	}
	if strings.Contains(content, "`GENTLE_AI_REVIEW_BINDING=") {
		t.Fatal("orchestrator contract permits equals-delimited reviewer bindings")
	}
}

func TestGeneratedOpenCodeReviewControllersUseNegotiatedStatusRouting(t *testing.T) {
	controllers := map[string]string{
		"orchestrator": renderSDDOrchestratorAsset(model.AgentOpenCode),
		"post-apply":   renderBoundedReviewAsset("opencode/commands/sdd-apply.md"),
	}
	for name, content := range controllers {
		t.Run(name, func(t *testing.T) {
			for _, required := range []string{
				"gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --next-transition",
				"route only from the returned `next_transition`",
				"exact operation and ordered argument tokens unchanged",
				"`execute`", "`collect`", "`stop`",
			} {
				if !strings.Contains(content, required) {
					t.Errorf("generated OpenCode %s controller missing negotiated routing clause %q", name, required)
				}
			}
			for _, stale := range []string{
				"Call `gentle-ai review start` once.",
				"runs `gentle-ai review start --cwd <repo>`",
				"| 01 | `gentle-ai review start`",
			} {
				if strings.Contains(content, stale) {
					t.Errorf("generated OpenCode %s controller retains direct START route %q", name, stale)
				}
			}
		})
	}
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
				for _, want := range []string{"Review once", "GENTLE_AI_REVIEW_CONTEXT", "sole source of artifact_subject", "changed_path_manifest", "base_tree", "candidate_tree", "env -i", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_ATTR_NOSYSTEM=1", "--no-replace-objects", "diff --name-status --text --no-ext-diff --no-textconv --no-renames", "diff --numstat --text --no-ext-diff --no-textconv --no-renames", "diff --patch --text --full-index --no-color --no-renames --no-ext-diff --no-textconv --diff-algorithm=myers --no-indent-heuristic --unified=3", "cat-file -p '<tree>:<path>'", ":(literal)<path>", "never pass --binary", "attributes must never suppress a hunk", "incomplete inspection", "Never read the live worktree", "## Candidate-Causal Admission", "Return one JSON object and no prose", `"subject_hash":"<artifact_subject.subject_hash>"`, "GENTLE_AI_REVIEW_BINDING.subject_hash", `"inspection":{"status":"completed","paths":["<every changed_path_manifest.path in exact order>"]}`, "lens triage", "Emit no unknown fields"} {
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
		"gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --next-transition",
		"provider-returned transition", "repeat 01–02", "reconcile-terminal-mirrors",
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
			if !strings.Contains(content, "gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --next-transition") {
				t.Fatalf("%s does not begin negotiated review routing with STATUS", path)
			}
			if strings.Contains(content, "runs `gentle-ai review start --cwd <repo>`") {
				t.Fatalf("%s retains direct post-apply START routing", path)
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
