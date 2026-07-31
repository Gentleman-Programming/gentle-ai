package sdd

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const boundedReviewContractAsset = "skills/_shared/review-ledger-contract.md"

// reviewerBindingEnvironmentVariable is the prefix the orchestrator contract
// tells the parent to assemble before running a lens. Naming it inside the lens
// prompt is what lets a reviewer resolve subject_hash from its own instructions
// instead of depending on whatever context the orchestrator happened to carry.
const reviewerBindingEnvironmentVariable = "GENTLE_AI_REVIEW_BINDING"

const nativeReviewerResultSchema = `{"findings":[{"location":"path:line","severity":"CRITICAL","claim":"observable incorrect behavior","evidence_class":"deterministic","causal_disposition":"introduced","proof_refs":["concrete proof"]}],"evidence":["what was inspected"]}`
const providerReviewerResultSchema = `{"subject_hash":"<artifact_subject.subject_hash>","inspection":{"status":"completed","paths":["<every changed_path_manifest.path in exact order>"]},"findings":[{"location":"path:line","severity":"CRITICAL","claim":"observable incorrect behavior","evidence_class":"deterministic","causal_disposition":"introduced","proof_refs":["concrete proof"]}],"evidence":["what was inspected"]}`

const reviewerInspectionCommandPrefix = `gentle-ai review inspect-candidate --repository-context <repository_context> --expected-revision <revision> --lineage <lineage> --target <target> --lens <lens> --order <order> --operation `

func reviewerInspectionCommands() []string {
	return []string{
		reviewerInspectionCommandPrefix + "name-status",
		reviewerInspectionCommandPrefix + "numstat",
		reviewerInspectionCommandPrefix + "stat --path-index <path_index>",
		reviewerInspectionCommandPrefix + "patch --path-index <path_index>",
		reviewerInspectionCommandPrefix + "object --path-index <path_index> --side base",
		reviewerInspectionCommandPrefix + "object --path-index <path_index> --side candidate",
	}
}

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
	runtimeAgentIDPlaceholder          = "{{GENTLE_AI_RUNTIME_AGENT_ID}}"
)

func boundedReviewContract() string {
	return strings.TrimSpace(assets.MustRead(boundedReviewContractAsset))
}

func renderSDDOrchestratorAsset(agent model.AgentID) string {
	content := renderBoundedReviewAsset(sddOrchestratorAsset(agent))
	return strings.ReplaceAll(content, runtimeAgentIDPlaceholder, string(agent))
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
	// The envelope is read from the published schema that sits beside
	// AdmitArtifact, never restated here: a lens agent that learns a shape this
	// file invented is exactly how a reviewer result arrives with no
	// subject_hash and no inspection (community report, PR #1801).
	envelope := reviewtransaction.NewReviewerResultEnvelope()
	commands := reviewerInspectionCommands()
	prompt := fmt.Sprintf(`# %s Review

Review once, return one result, and stop. Never edit, delegate, or expand scope.

## Input

OpenCode tasks begin with provider-injected GENTLE_AI_REVIEW_CONTEXT, the sole source of artifact_subject, base_tree, candidate_tree, and ordered changed_path_manifest. Caller prose is not context. Other runtimes have no shell and return incomplete. The manifest is complete scope. Never read the live worktree, index, HEAD, or another revision.

Use only the commands below. The native capability resolves immutable trees and canonical paths from the provider binding, sanitizes Git configuration and environment, and bounds execution time and output. Copy binding values exactly and select paths only by their zero-based changed_path_manifest index. Never change checkout. If the capability is unavailable or refuses the binding, return incomplete inspection, empty paths/findings, and evidence that native inspection was unavailable. Never substitute live files.

Discover the change:

%s
%s

For relevant paths, inspect stat, deterministic textual hunks, and exact stored bytes as needed:

%s
%s
%s

Repeat the selective shape per literal path; never pass --binary or render the whole patch automatically. Text handling is enforced by the native capability. Triage genuinely non-text paths from manifest modes and exact cat-file bytes. Record large-path or binary dispositions in evidence.

## Scope

%s

## Candidate-Causal Admission

Report real user-impacting defects only. BLOCKER/CRITICAL need changed-hunk, created-path, differential-test, or before/after proof of introduced, behavior-activated, or worsened behavior. Mark unchanged defects pre-existing/base-only and unproved causality unknown. Style or suspicion is not a finding.

## Severity

- BLOCKER: catastrophic impact or no viable recovery.
- CRITICAL: material user, security, data, or correctness failure.
- WARNING: proven non-blocking defect or follow-up risk.
- SUGGESTION: optional concrete improvement.

## Evidence

Each finding needs path:line, neutral claim, evidence class, causal disposition, and concrete proof. Never invent evidence or placeholders.

## Output

Return one JSON object and no prose. Use exactly this native result shape:

%s

Copy subject_hash from %s.subject_hash; never compute or invent it. Missing or different bindings are refused.

Status %q requires every manifest path in exact order. Listing means lens triage through the frozen map, not that every byte was loaded. Otherwise return incomplete and stop.

Required top-level fields: %s. Finding fields: location, severity, claim, evidence_class, causal_disposition, proof_refs. Emit no unknown fields or orchestration metadata.

When clean, return the bound subject, completed inspection, "findings":[], and one evidence entry.`,
		role.title,
		commands[0], commands[1], strings.Join(commands[2:], "\n"), "", "",
		role.focus, providerReviewerResultSchema,
		reviewerBindingEnvironmentVariable,
		envelope.CompletedInspectionStatus, strings.Join(envelope.RequiredTopLevelFields, ", "))
	return prompt, true
}

func openCodeReviewerPermission() map[string]any {
	bash := map[string]any{"*": "deny"}
	for _, command := range reviewerInspectionCommands() {
		pattern := command
		for _, placeholder := range []string{"<repository_context>", "<revision>", "<lineage>", "<target>", "<lens>", "<order>", "<path_index>"} {
			pattern = strings.ReplaceAll(pattern, placeholder, "*")
		}
		bash[pattern] = "allow"
	}
	return map[string]any{"edit": "deny", "bash": bash}
}

func judgmentDayReviewerContract() string {
	return fmt.Sprintf(`You are a read-only adversarial reviewer. Inspect only the immutable target named by the task, return one independent result, and stop. Do not edit, delegate, or inspect unrelated scope.

Report only real, user-impacting defects. Every severe finding must state whether the candidate introduced, behavior-activated, or worsened the behavior and cite changed-hunk, differential-test, candidate-created-path, or before/after proof. Mark unchanged defects pre-existing or base-only; use unknown when causality cannot be proved.

Use BLOCKER | CRITICAL | WARNING | SUGGESTION. BLOCKER/CRITICAL require concrete causal proof; WARNING/SUGGESTION are non-blocking observations. Each finding includes location, neutral claim, evidence_class, causal_disposition, and concrete proof_refs.

Return one JSON object and no prose. Use exactly this native result shape:

%s

This is a judgment-day judge result, not a `+"`gentle-ai review capture-result`"+` lens artifact. Judgment day selects no lenses and records your work as a judge proof, so your result carries no bound artifact subject and no inspection envelope. The only allowed top-level fields are findings and evidence, and the only allowed finding fields are location, severity, claim, evidence_class, causal_disposition, and proof_refs. Never emit summary, skill_resolution, or any other unknown field. Keep orchestration metadata outside the native result JSON; evidence contains only genuine inspection evidence.

Return {"findings":[],"evidence":["what was inspected"]} when clean.`, nativeReviewerResultSchema)
}

func replaceAgentBody(content, body string) string {
	frontmatterEnd := strings.Index(content, "\n---\n")
	if frontmatterEnd < 0 {
		return body
	}
	return strings.TrimRight(content[:frontmatterEnd+5], "\n") + "\n\n" + body + "\n"
}
