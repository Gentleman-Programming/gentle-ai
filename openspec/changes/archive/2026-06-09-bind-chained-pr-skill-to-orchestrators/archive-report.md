# Archive Report: Bind chained-pr Skill to SDD Orchestrator Assets

## Status

Archived successfully on 2026-06-09. Change is implemented, verified (PASS), and approved by Judgment Day.

## Specs Synced

| Domain | Action | Details |
|--------|--------|---------|
| `sdd-orchestrator-assets` | Updated | Added chained-pr skill binding requirement and scenarios: binding must reference skill by registry name (`chained-pr`, frontmatter `gentle-ai-chained-pr`); binding must be present in all 11 SDD orchestrator templates; binding must inject the skill path into sdd-tasks and sdd-apply prompts under `## Skills to load before work`; inline summary of `stacked-to-main` and `feature-branch-chain` must be retained; cursor template gains a full `### Chain Strategy` section at parity with other 10 templates; platform-accurate binding wording for solo-inline and platform-native hosts (windsurf, antigravity, kimi, kiro) must not reintroduce OpenCode persistence claims; static assertions and goldens must verify binding presence without unrelated churn. |

## Archive Contents

- proposal.md ✅
- design.md ✅
- tasks.md ✅ (10/10 tasks complete, all marked checked)
- apply-progress.md ✅ (Strict TDD, batch 1 of 1, all complete)
- verify-report.md ✅ (PASS — 0 CRITICAL, 0 WARNING, 2 SUGGESTION — informational only)
- specs/sdd-orchestrator-assets/spec.md ✅ (delta spec — all requirements and scenarios)
- archive-report.md ✅

## Source of Truth Updated

- `openspec/specs/sdd-orchestrator-assets/spec.md` — merged 8 new requirements (chained-pr skill binding, inline summary retention, cursor parity, platform-accurate wording, golden fixture coverage) plus 16 scenarios into the main spec.

## Implementation Summary

**What was built**: The chained-pr skill (registry name `chained-pr`, frontmatter `gentle-ai-chained-pr`) is now bound into all 11 SDD orchestrator templates (antigravity, claude, codex, cursor, gemini, generic, kimi, kiro, opencode, qwen, windsurf). When delivery planning yields chained PRs, the orchestrator resolves the skill by registry name through its existing Sub-Agent Launch Pattern and injects it into sdd-tasks and sdd-apply prompts under `## Skills to load before work`. The cursor template, which was missing a Chain Strategy section, now includes a full section at parity with other templates, with the canonical binding sentence placed immediately after the chain_strategy forwarding line.

**What shipped**:
- 11 modified orchestrator templates (10 existing + 1 cursor gaining the section)
- 2 test files extended with binding assertions (assets_test.go, inject_test.go)
- 13 regenerated golden fixtures (12 existing + 1 new sdd-cursor-rules.golden)
- Canonical binding sentence (byte-identical across all 11 templates): "When delivery planning yields chained PRs, treat `chained-pr` (registry skill `gentle-ai-chained-pr`) as a required skill match: resolve it by registry name through this template's existing Sub-Agent Launch Pattern and inject the resolved `SKILL.md` path into the `sdd-tasks` and `sdd-apply` phase prompts under `## Skills to load before work`, instructing those phases to read and follow it BEFORE planning or creating any PR. Do not hardcode the skill path; defer resolution to the launch pattern."
- Static assertion substring (common to all 11 + goldens): "treat `chained-pr` (registry skill `gentle-ai-chained-pr`) as a required skill match"

## Verification Results

**Verdict**: PASS (0 CRITICAL, 0 WARNING, 2 SUGGESTION — both informational).

- Full test suite: `go test ./internal/...` → all packages PASS
- Targeted assertions:
  - `TestClaudeSDDOrchestratorChainStrategy` → PASS
  - `TestNonClaudeSDDOrchestratorChainStrategyParity` (10 subtests: codex, gemini, qwen, generic, kimi, kiro, windsurf, antigravity, cursor, opencode) → PASS
  - `TestInjectKimiKiroWindsurfAntigravityPreserveNativeChainStrategyWording` (4 hosts) → PASS
  - `TestPlatformNativeSDDOrchestratorsAvoidOpenCodePersistenceClaims` (4 hosts) → PASS
  - `TestOrchestratorsRejectDelegationBypassLanguage` → PASS
  - `TestOrchestratorsRequireNonSkippableGeneralDelegationTriggers` → PASS
- All spec requirements satisfied
- All 11 templates contain binding by registry name (no hardcoded path)
- Cursor template structure matches other 10 templates
- Solo-inline forbidden-claim safety preserved (windsurf, antigravity, kimi, kiro)
- Golden churn limited to intended binding wording and cursor's new section
- No unrelated churn in any golden

## Judgment Day

- Status: APPROVED (after 2 fixes during apply phase)
- Both fixes were test-assertion refinements to match the final cursor section body wording

## Workload Summary

- Estimated changed lines: ~160–220 lines (hand-written ~100–140 + generated goldens ~60–80)
- Actual changed lines: 26 files modified, ~220 lines total (confirmed by git diff --stat)
- 400-line budget risk: Low (well within limits)
- Single PR without chaining
- Mode: Strict TDD (test-first → implementation → golden regen → full suite verification)

## Migration Notes

No data migration required. The `.claude/CLAUDE.md` will be regenerated at next build from the source templates; no hand-edit needed. All binding is by registry name, so future skill path relocations (if any) are automatic — the binding does not break.

## SDD Cycle Complete

The change has been planned (proposal), specified (spec + delta), designed, tasked, applied (Strict TDD), verified (PASS), and archived. The implementation is scoped to:
- Binding the chained-pr skill into 11 SDD orchestrator templates
- Cursor template parity (new Chain Strategy section)
- Static assertions and golden coverage for the binding
- No edits to the skill itself or any other skills, non-orchestrator assets, or behavioral changes outside the binding

All requirements are met. The spec is merged into the main specification. The change is closed.
