---
name: sdd-verify
description: "Trigger: explicitly requested SDD verification. Run optional practical diagnostics against available implementation and artifacts."
disable-model-invocation: true
user-invocable: false
license: MIT
metadata:
  author: gentleman-programming
  version: "4.0"
  delegate_only: true
---

## Execution Role

If you are the dedicated `sdd-verify` executor, perform the diagnostics below; do not delegate. If you are the orchestrator loading this skill, delegate to that executor.

## Activation Contract

Run when the orchestrator explicitly requests verification. Verification is optional, not a prerequisite for archive.

## Language Domain Contract

Generated technical artifacts default to English. Do not inherit the user's conversational language or the active persona's regional voice for SDD artifacts unless the user explicitly requests that artifact language or the project convention requires it.

If technical artifacts are explicitly requested in another language, use a neutral/professional register unless the user explicitly requests a different tone or regional variant.

Public/contextual comments follow the target context language by default. Explicit user language or tone overrides win; otherwise use a neutral/professional register unless the target context clearly calls for another tone or regional variant.

## Hard Rules

- Use the supplied structured status, artifact store, change identity, and edit permissions. Verification grants no mutation authority; do not fix code or tasks.
- Inspect available artifacts and implementation, including partial work. Missing artifacts limit conclusions, not permission to report useful diagnostics.
- Preserve user-owned `strict_tdd`, test commands, and model/provider/profile/effort selection. When Strict TDD is active, load `strict-tdd-verify.md` and assess the available apply-progress evidence honestly; never fabricate historical RED or GREEN.
- Report actual command results and limitations. Source inspection, unchecked tasks, and unexecuted tests are not runtime proof. Missing tooling means unavailable checks, not PASS.
- Do not require a report schema, validator, immutable attestation, evidence search, or settlement. Missing, stale, malformed, or failed reports do not gate archive.
- SDD never offers, launches, or consumes RDD. Findings do not start automatic review, refuter, or correction loops.
- Apply `rules.verify` from `openspec/config.yaml` to requested diagnostics without treating report format as archive authority.

## Decision Gates

| Condition | Action |
|---|---|
| Partial implementation or missing specs/design | Inspect what exists; name unfinished work and skipped dimensions. |
| Strict TDD active | Check actual TDD evidence for implemented work; disclose missing evidence. |
| Test/build fails or a requirement is unmet | Report the finding and its evidence, without editing or certifying completion. |
| Tooling or permission unavailable | Report the limitation; do not bypass authorization. |
| Workspace-planning context | Limit diagnostics to accessible planning artifacts; do not edit linked repositories. |

## Execution Steps

1. Load relevant skills and retrieve available artifacts through shared Sections A/B, using the supplied locators and active store.
2. Compare implemented behavior with available requirements and design. Record task completion as observed; do not rewrite checkboxes.
3. Run applicable tests, build/type-check, and other practical project checks within the authorized scope. Adapt depth to the change; do not force exhaustive scenario searches or a fixed evidence matrix.
4. Record commands, exit codes, useful output, findings, and unavailable or unrun checks. Distinguish verified behavior from assumptions and static observations.
5. Persist the diagnostic report through shared Section C when the selected store permits it; preserve prior historical findings and identify what changed. Do not rewrite old user reports merely to satisfy a format. Return shared Section D.

## Output Contract

Return concise findings, observed task state, executed checks and their outcomes, limitations, and recommended next work. A diagnostic report may be partial or failed; neither blocks archive. Completed implementation normally proceeds to archive; unfinished implementation normally returns to apply. Archive records the actual state, never a synthetic PASS.

## References

- [references/report-format.md](references/report-format.md) — optional report outline.
- [strict-tdd-verify.md](strict-tdd-verify.md) — only when Strict TDD is active.
- `../_shared/sdd-phase-common.md` — skill loading, retrieval, persistence, and return envelope.
