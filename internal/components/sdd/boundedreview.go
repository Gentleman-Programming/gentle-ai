package sdd

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

const boundedReviewContractAsset = "skills/_shared/review-ledger-contract.md"

const nativeReviewerResultSchema = `{"findings":[{"location":"path:line","severity":"CRITICAL","claim":"observable incorrect behavior","evidence_class":"deterministic","causal_disposition":"introduced","proof_refs":["concrete proof"]}],"evidence":["what was inspected"]}`

type reviewerRole struct {
	title string
	focus string
}

var reviewerRoles = map[string]reviewerRole{
	"review-risk": {
		title: "R1 Risk",
		focus: "Inspect security, authorization, data exposure or loss, unsafe input handling, secrets, and dependency vulnerabilities. Require backend enforcement and concrete exploit or scanner evidence; do not report hypothetical risk without a reachable impact.",
	},
	"review-resilience": {
		title: "R4 Resilience",
		focus: "Inspect failure handling, rollback or fix-forward behavior, retry safety, graceful degradation, observability, latency, and load. Require a concrete production failure mode or measured impact; do not report generic operational speculation.",
	},
	"review-readability": {
		title: "R2 Readability",
		focus: "Inspect maintainability defects that obscure behavior: misleading names, duplicated or dead logic, unexplained business constants, unsafe complexity, and missing change context. Report style only when it hides a concrete defect or makes the change unsafe to maintain.",
	},
	"review-reliability": {
		title: "R3 Reliability",
		focus: "Inspect behavior, tests, boundaries, invalid inputs, failure paths, determinism, and regressions. Require externally observable assertions at the cheapest useful test level; report missing coverage only when it leaves candidate behavior unproved.",
	},
}

const (
	authorityFirstProcedurePlaceholder = "{{GENTLE_AI_AUTHORITY_FIRST_TERMINAL_PROCEDURE}}"
	authorityFirstProcedureStart       = "<!-- authority-first-terminal-procedure:start -->"
	authorityFirstProcedureEnd         = "<!-- authority-first-terminal-procedure:end -->"
)

func boundedReviewContract() string {
	return strings.TrimSpace(assets.MustRead(boundedReviewContractAsset))
}

func renderSDDOrchestratorAsset(agent model.AgentID) string {
	return renderBoundedReviewAsset(sddOrchestratorAsset(agent))
}

func renderBoundedReviewAsset(path string) string {
	content := assets.MustRead(path)
	content = strings.ReplaceAll(content, authorityFirstProcedurePlaceholder, authorityFirstTerminalProcedure())
	if strings.HasSuffix(path, "/sdd-orchestrator.md") {
		return replaceBoundedReviewSection(content, "#### Review Execution Contract", "Cost and Context Balance")
	}
	if prompt, ok := reviewerPrompt(reviewerName(path)); ok {
		return replaceAgentBody(content, prompt)
	}
	if strings.Contains(path, "/agents/jd-judge-") {
		return replaceBoundedReviewSection(content, "## Review ledger contract", "", judgmentDayReviewerContract())
	}
	if strings.Contains(path, "/agents/jd-fix-agent.") {
		return content
	}
	if strings.Contains(content, "## Review ledger contract") {
		return replaceBoundedReviewSection(content, "## Review ledger contract", "")
	}
	return content
}

func authorityFirstTerminalProcedure() string {
	contract := boundedReviewContract()
	start := strings.Index(contract, authorityFirstProcedureStart)
	end := strings.Index(contract, authorityFirstProcedureEnd)
	if start < 0 || end < start {
		return ""
	}
	start += len(authorityFirstProcedureStart)
	return strings.TrimSpace(contract[start:end])
}

func replaceBoundedReviewSection(content, heading, nextHeading string, contracts ...string) string {
	start := strings.Index(content, heading)
	if start < 0 {
		return content
	}
	end := len(content)
	if nextHeading != "" {
		remainder := content[start+len(heading):]
		for _, candidate := range []string{"\n#### " + nextHeading, "\n### " + nextHeading, "\n## " + nextHeading} {
			if relative := strings.Index(remainder, candidate); relative >= 0 {
				end = start + len(heading) + relative + 1
				break
			}
		}
	}
	contract := boundedReviewContract()
	if len(contracts) > 0 {
		contract = contracts[0]
	}
	replacement := heading + "\n\n" + contract + "\n\n"
	return strings.TrimRight(content[:start], "\n") + "\n\n" + replacement + strings.TrimLeft(content[end:], "\n")
}

func reviewerName(path string) string {
	name := path
	if slash := strings.LastIndex(name, "/"); slash >= 0 {
		name = name[slash+1:]
	}
	return strings.TrimSuffix(name, ".md")
}

func reviewerPrompt(name string) (string, bool) {
	role, ok := reviewerRoles[name]
	if !ok {
		return "", false
	}
	prompt := fmt.Sprintf(`# %s Review

You are a read-only reviewer. Inspect the immutable candidate diff once, return one result, and stop. Do not edit, delegate, or expand beyond the candidate and the minimum base context needed for proof.

## Scope

%s

## Candidate-Causal Admission

Report a finding only for real, user-impacting incorrect behavior. Classify it as introduced, behavior-activated, worsened, pre-existing, base-only, or unknown. A BLOCKER or CRITICAL finding requires proof that a candidate hunk introduced or worsened the behavior, created a path to it, or fails a differential test that passes on base. Use pre-existing or base-only when the candidate did not activate the defect, and unknown when causality cannot be proved. Style preferences and unsupported suspicion are not findings.

## Severity

- BLOCKER: unsafe to deliver; catastrophic impact or no viable recovery.
- CRITICAL: material user, security, data, or correctness failure.
- WARNING: proven non-blocking defect or follow-up risk.
- SUGGESTION: optional improvement with concrete value.

## Evidence

Each finding needs one exact path:line location, a neutral observable claim, deterministic | inferential | insufficient evidence class, causal disposition, and concrete proof references such as a changed hunk, command/output, differential test, trace, or before/after behavior. Do not invent evidence or use placeholders.

## Output

Return one JSON object and no prose. Use exactly this native result shape:

%s

The only allowed top-level fields are findings and evidence, and the only allowed finding fields are location, severity, claim, evidence_class, causal_disposition, and proof_refs. Never emit summary, skill_resolution, or any other unknown field. Keep orchestration metadata outside the native result JSON; evidence contains only genuine inspection evidence.

Return {"findings":[],"evidence":["what was inspected"]} when clean.`, role.title, role.focus, nativeReviewerResultSchema)
	return prompt, true
}

func judgmentDayReviewerContract() string {
	return fmt.Sprintf(`You are a read-only adversarial reviewer. Inspect only the immutable target named by the task, return one independent result, and stop. Do not edit, delegate, or inspect unrelated scope.

Report only real, user-impacting defects. Every severe finding must state whether the candidate introduced, behavior-activated, or worsened the behavior and cite changed-hunk, differential-test, candidate-created-path, or before/after proof. Mark unchanged defects pre-existing or base-only; use unknown when causality cannot be proved.

Use BLOCKER | CRITICAL | WARNING | SUGGESTION. BLOCKER/CRITICAL require concrete causal proof; WARNING/SUGGESTION are non-blocking observations. Each finding includes location, neutral claim, evidence_class, causal_disposition, and concrete proof_refs.

Return one JSON object and no prose. Use exactly this native result shape:

%s

The only allowed top-level fields are findings and evidence, and the only allowed finding fields are location, severity, claim, evidence_class, causal_disposition, and proof_refs. Never emit summary, skill_resolution, or any other unknown field. Keep orchestration metadata outside the native result JSON; evidence contains only genuine inspection evidence.

Return {"findings":[],"evidence":["what was inspected"]} when clean.`, nativeReviewerResultSchema)
}

func replaceAgentBody(content, body string) string {
	frontmatterEnd := strings.Index(content, "\n---\n")
	if frontmatterEnd < 0 {
		return body
	}
	return strings.TrimRight(content[:frontmatterEnd+5], "\n") + "\n\n" + body + "\n"
}
