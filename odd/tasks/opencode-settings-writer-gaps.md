# OpenCode settings writer gaps

## Objective
Close the remaining gaps from #5025 so no OpenCode install/sync step writes a settings file OpenCode does not load, and unsafe selected input is refused before any prompt or settings file changes.

## Problem and why
#5016 (issue #5013) routed the Theme, Persona, Permission, Context7 and Engram writers through the effective settings file. Review of the merged change found remaining paths that still bypass that invariant or refuse only after a partial mutation.

## Scope and constraints
- Authorized: user instruction "arregla y merge" for issue #5025 (approved); single PR `Closes #5025`, merge when required checks are green under repository policy.
- Branch `fix/opencode-settings-writer-gaps` from `main` at `e0744c8c4`, isolated worktree.
- Preserve non-OpenCode behavior, JSONC comments, file modes, backup/rollback and workspace scope semantics (#1825: OpenCode loads only the effective home/project settings).
- Test-first RED/GREEN per item when a runnable test exists. Artifacts in English.
- Forecast: 300–600 authored lines. Delivery strategy: `single-pr`; a `size:exception` label needs a direct user instruction if the final diff exceeds 400 lines.

## Tasks
- [x] B1 [delegated]: Items 2, 3, 4 and 6 of #5025 — `stripLegacyTriggerRules` uses the effective path; Persona preflight refuses escaped touched-key spellings and Gentleman sync malformed JSONC before any prompt write; `removeJSONNestedSubKey` preserves comments outside the cleaned subtree for every parent key or refuses. Route: delegated, multi-file behavior/tests.
- [x] B2 [delegated]: Items 1 and 5 of #5025 — validate the selected OpenCode settings before `engram setup opencode` side effects (or defer setup until validation succeeds); remove the stale ComponentSDD OpenCode declared settings/ownership paths (the SDD apply step is a retired no-op, so nothing writes them); align `restoreOpenCodeModelAssignmentsFromState` with the loaded settings file. Route: delegated, multi-file behavior/tests.
- [ ] B3 [inline/delegated verification]: native review, PR, green CI, merge. Route: state inline, checks delegated.

## Acceptance and checks
Dual-file fixture (selected project JSONC plus decoy global JSON): no step changes the decoy; unsafe selected input leaves prompt and settings bytes unchanged; workspace scope never declares, reads or writes `<workspace>/.config/opencode/opencode.json`; non-OpenCode agents unchanged. Focused and full isolated `internal/cli` tests, component suites, `e2e/organicruntime` (real_agent_e2e), gofmtcheck, deadcode ratchet, CI.

## Progress
- Feature document created from #5025.
- B1 done (delegated): legacy trigger-rule cleanup uses the effective loaded settings path (Kilo unchanged); persona preflight refuses an escaped `agent` spelling and parseable-but-unsafe sync cleanup input before any prompt write (shared `JSONCTopLevelKeyIsEscaped`); unparseable JSONC stays tolerated because the tools cleanup skips it (item 4 boundary); `removeJSONNestedSubKey` preserves comments outside the cleaned subtree for any parent key or refuses. RED observed per item (install and sync for item 2 re-confirmed by parent), GREEN after. Checks: gofmt, vet, Windows vet, component suites, full isolated `internal/cli` (1393s), `e2e/organicruntime` with opencode on PATH, deadcode ratchet. 326 authored lines (~231 tests). Residual: no pipeline-level OpenCode preflight, so earlier components may write before Persona refuses.
- B2 done (delegated): `engram.ValidateOpenCodeSettings` runs the merge refusals on the loaded settings before setup mode, probe and `engram setup opencode`, so a refusal leaves no setup side effects (the Engram binary install may already have happened); stale ComponentSDD OpenCode settings/ownership declarations removed (`OwnershipPath` stays declared by routing guidance, its real writer); `restoreOpenCodeModelAssignmentsFromState` always reads the loaded settings (latent: production callers already passed ScopeGlobal). RED/GREEN per item; component suites, full isolated `internal/cli` (1299s), `e2e/organicruntime`, deadcode ratchet passed.
- Product decision (user, 2026-09-27): keep `engram setup opencode` even when a project settings file is loaded. OpenCode merges global and project configs ("Configuration files are merged together, not replaced", opencode.ai/docs/config), so setup's global `mcp.engram`, `plugins/engram.ts` and tui entries are effective, not stranded. Upstream Engram gap (not fixed here): when global settings lack `mcp.engram`, setup rewrites the file with stripped comments and sorted keys, and prefers `.jsonc` over gentle-ai's managed priority.
- Native review `review-0b22ddff6ca14842` over `e0744c8c4..371d6f3a2` approved and acknowledged. PR #5028 opened (`Closes #5025`, `type:bug`, user-authorized `size:exception`); all 19 checks passed.
- Bot follow-up (user chose to fix before merge): the persona preflight refused sync on malformed JSONC when a comment appeared in the agent value, although both sync cleanups skip unparseable settings. The refusal now applies only to install. RED on Gentleman and Neutral sync, GREEN after (inline route: one guard plus one test). Deferred to a follow-up issue: trigger-rule cleanup still normalizes JSONC via `MergeJSONObjects` (as the routing injector already did on `main`), and there is no pipeline-level OpenCode preflight before telemetry, plugins and earlier components.

