# OpenCode agent parity with Gentle Shell (fixes #4471, #4684, #4758)

Locator: `odd/tasks/opencode-agent-parity.md` · Engram mirror: `odd/opencode-agent-parity/tasks`
Branch: `fix/4471-opencode-agent-parity` (worktree `gentle-ai-worktrees/4471-opencode-agent-parity`, base `origin/main` 6c7f162f4)

## Objective

A fresh or upgraded `gentle-ai install/sync --agent opencode` yields OpenCode subagents functionally at parity with Gentle Shell's global agents, and no managed OpenCode agent entry carries `__managed_by`. Every runtime regains the non-SDD assets the SDD retirement dropped by accident (T4).

## Problem

- Up to v3.7.0, `internal/assets/opencode/sdd-overlay-*.json` defined 23 OpenCode agents, each with `"__managed_by": "gentle-ai/sdd"`. OpenCode forwards unknown agent fields as provider request options; strict providers (opencode-go, Fireworks, NVIDIA NIM) reject every call (#4471, dups #4684, #4758).
- Commit e219644b2 (retire SDD) deleted that overlay. It was the sole owner of ALL OpenCode agents, not only SDD ones, so JD (`jd-judge-a/b`, `jd-fix-agent`) and the four review lenses were dropped as collateral. Only `gentle-orchestrator` (permission patch), `review-refuter`, `review-validator` (run.go `installOpenCodeReviewProviderRoles`) and `gentleman` remain.
- Observed: fresh install from main in isolated HOME → `opencode.json` agents = gentle-orchestrator, gentleman, review-refuter, review-validator.
- Writes use `filemerge.MergeJSONObjects` (deep merge, never deletes) → upgraded users keep stale v3.7.0 entries including `__managed_by`; no migration, no test.
- `sync.go` model assignment, TUI model picker, and `opencode-review-transport.ts` still expect JD/lens agents.

## Why

User requirement: only SDD was meant to be retired; OpenCode agents must be functionally the same as Gentle Shell's (names may differ).

## Scope (authorized)

Parity set (canonical source: gentle-pi `assets/agents/*.md` at 89b8de3b5): `gentle-ai-explore`, `gentle-ai-verify`, `gentle-ai-worker`, `jd-judge-a`, `jd-judge-b`, `jd-fix-agent`, `review-risk`, `review-readability`, `review-reliability`, `review-resilience`. Plus retained `review-refuter`, `review-validator`, `gentle-orchestrator`, `gentleman`.

- Names: identical to Gentle Shell (decision: simplest parity; no collision with OpenCode built-in `explore`/`general`).
- Format: JSON agent entries in `opencode.json` (same owner style as `installOpenCodeReviewProviderRoles`), prompt bodies embedded under `internal/assets/opencode/agents/`. No `__managed_by` or any non-AgentConfig key.
- Routing: OpenCode `orchestrator.md` delegates to `gentle-ai-explore` / `gentle-ai-worker` / `gentle-ai-verify` like Gentle Shell.
- Migration on install/sync: `__managed_by == "gentle-ai/sdd"` is ownership proof. Owned entries for current roles are replaced cleanly; owned retired entries (`sdd-*`, v3.7.0 `general`/`explore` overrides) are removed; the field is stripped from any remaining entry. Unmarked user entries untouched. `managedConfigPriority` keeps working via shape heuristic / new ownership signal.

## Constraints

- Preserve unrelated user config; idempotent re-runs.
- Model assignment (`sync.go`) and TUI model picker cover the parity set.
- ~400 authored lines per task is advisory only; embedded prompt bodies are copied assets.

## Tasks

- [x] T1 — Install parity agents for OpenCode (embedded prompts, permissions from Gentle Shell tool sets), orchestrator routing, model assignment/picker allowlist, parity inventory test. Route: delegated (writer trigger: 2+ non-trivial files).
- [x] T2 — Upgrade migration: strip `__managed_by`, remove owned retired entries, replace owned current-role entries; regression tests from a v3.7.0 config fixture. Route: delegated (same writer, sequential).
- [x] T4 — Restore non-SDD assets dropped by the SDD retirement (authorized by user 2026-09-25): install `skills/_shared/*` still referenced by orchestrators/skills (skill-resolver, engram-convention, persistence-contract, research-lifecycle, review-ledger-contract(-pi), README) for every runtime that installed them in v3.7.0, under a non-SDD owner; restore `skill-creator`/`skill-registry` commands for OpenCode, Kilo, Qwen; restore OpenCode `default_agent: gentle-orchestrator` (with existing opencodedefault ownership file) and `share: disabled`. Tests: per-runtime install inventory asserts every `_shared/*.md` referenced by installed orchestrator/skills exists (no dangling refs); OpenCode default_agent/share asserted. Route: delegated (writer trigger).
- [x] T5 — Kilo (OpenCode fork) has the same #4471 class: v3.7.0 wrote the same overlay into `~/.config/kilo/opencode.json` (gentle-orchestrator, jd-*, 4 lenses, refuter/validator, sdd-*) with `__managed_by`; main/branch fresh Kilo install has only gentle-orchestrator + gentleman, and upgrade keeps `__managed_by` + `sdd-*`. Restore the v3.7.0 non-SDD Kilo agent set without the marker and run the same legacy migration for Kilo. Found by T3 e2e. Route: delegated.
- [x] T3 — Sandbox e2e: fresh install + upgrade from v3.7.0 config in isolated HOME (no Homebrew on PATH), idempotent re-run. Route: inline (bounded action).

## Acceptance criteria

1. Fresh install: all 10 parity agents + refuter/validator present; no agent entry contains `__managed_by`.
2. Upgrade from v3.7.0 config: no `__managed_by` anywhere; no `sdd-*`, owned `general`/`explore` removed; user-defined agents untouched.
3. Second run produces byte-identical config.
4. T4: no installed orchestrator or skill references a `_shared/*` file that is not installed; OpenCode/Kilo/Qwen skill commands present; OpenCode `default_agent` = gentle-orchestrator and `share` = disabled on fresh install; ownership preserved on uninstall.
5. `go test ./...` green (or known env failures documented); `go vet`, `gofmt` clean.

## Checks

- `go test ./internal/cli/... ./internal/opencode/... ./internal/components/... ./internal/tui/... ./internal/assets/...`
- `go test ./...`, `go vet ./...`, `gofmt -l .`
- Sandbox install per T3.

## Delivery

Forecast: ~700 authored lines + ~600 copied prompt lines. Strategy: ask-on-risk (default) — ask before PR. RDD: enabled by the user on 2026-09-25 (clone-local unset, global on) → per work-unit commit `review assess --committed-only` with base = branch point 6c7f162f4.

## Progress

- 2026-09-25: regression reproduced (fresh install, 4 agents only). Feature doc created.

- 2026-09-25: T1 delegated to gentle-ai-worker (task muhcno1f-3-f75u).
- 2026-09-25: cross-runtime sandbox audit (v3.7.0 vs main, 16 runtimes) found further SDD-retirement collateral, authorized by the user the same day as T4: `skills/_shared/*` no longer installed anywhere but still referenced by orchestrators and judgment-day/skill-registry skills; OpenCode/Kilo/Qwen lose `skill-creator`/`skill-registry` commands; OpenCode loses `default_agent: gentle-orchestrator` and `share: disabled`. Engram: `bug/sdd-retirement-collateral-regressions`.

- 2026-09-25: T1 done (writer muhcno1f-3-f75u + parent inline fixes: dropped Pi-only host-relay line from the 4 lens prompts; removed the `codegraph` CLI fallback from gentle-ai-explore because it has bash denied). Commit 72519acad (20 files, +924/-9 incl. ~600 copied prompt lines). Evidence: RED `undefined: installOpenCodeODDParityAgents`; GREEN `TestOpenCodeInstallWritesParityAgentsWithoutManagedByMarker`, `TestOpenCodeInstallParityAgentsAreIdempotent`; assets/opencode/tui/opencodedefault/telemetry packages ok; gofmt/vet clean. Known env failures on base 6c7f162f4 too (parent re-ran on clean main): `TestSyncPersonaOnlyRollbackRestoresOpenCodeSettingsAfterGentlemanCleanup`, `TestSyncRollbackRestoresLegacyOpenCodePluginAndRemovesReplacement` (+ Pi `~/.gentle-shell` leakage tests per writer). Review assess (agent pi, base 6c7f162f4): medium, review_due=true (slice_budget_reached) → START returned consent envelope, awaiting user answer. Lineage review-482425bf66d8e129.

- 2026-09-25: T1 review granted by user → lineage review-482425bf66d8e129 (1 lens: review-reliability) APPROVED and acknowledged (authority burned). Advisory non-blocking finding R3-001 (WARNING, internal/cli/run.go:1150). Reviewed boundary → 72519acad.
- 2026-09-25: user authorized finishing everything through PR + merge while away, testing that everything works first; user pre-authorized granting review consent for remaining candidates. T2 delegated (gentle-ai-worker muhfqpvl-4-q4gc).

- 2026-09-25: T2 done (writer muhfqpvl-4-q4gc reported partial only because full `internal/cli` failed under the real HOME). Parent ran `go test ./internal/cli/ -count=1` in an isolated `env -i` HOME on base 6c7f162f4 and on the branch in parallel: identical 3 failures on both (TestReviewLifecycleBootstrapsGenuinelyUnversionedWorkspace, TestReviewStartOutsideAnyRepositoryBootstrapsLocalGitAndContinuesToEmptyCandidateRefusal, TestReviewStatusSelectorFreeResolvesRepositoryRoot — no git identity in env -i); no branch-only failures. Commit b674adf58. Review assess (base 72519acad): medium, review_due=false, under_budget → pending in slice. T4 delegated (gentle-ai-worker muhh98a9-5-oocb).

- 2026-09-26: T4 first pass (writer muhh98a9-5-oocb, partial): `_shared` owned by skills component, skill commands by skills component, OpenCode default_agent/share by OpenCode routing step + opencodedefault. Parent decisions (user asleep; rule = no regression vs last release v3.7.0, main's post-retirement semantics never shipped): keep v3.7.0 default_agent overwrite+previous_default; restore Kilo share=disabled; restore `_shared` in `~/.agents/skills` compat root; exclude odd-orchestrator-sections.md; fix uninstall removeIfEmpty ordering. internal/app TempDir cleanup failure reproduced on base under env -i (environmental). Follow-up delegated (muhje48q-6-6jvn).

- 2026-09-26: T4 follow-up (muhje48q-6-6jvn) complete. Parent: GOOS=windows vet ok, CGO_ENABLED=0 linux/windows build ok, focused isolated tests ok. Commit 3d01b3762. Review assess (base 72519acad, slice T2+T4): high, due → consent granted (user pre-authorized) → lineage review-8d7176597feaf1e4, 4 lenses; readability attempt 1 refused out_of_scope (slot reoffered, rerun admitted); APPROVED + acknowledged. Advisory non-blocking: R2-001..006, R3-001/R4-001 (ownership.go:76-79 = intended v3.7.0 recapture semantics), R3-002. Boundary → 3d01b3762.
- 2026-09-26: T3 e2e first run (16 runtimes, env -i isolated HOME, fake bins): all exit 0 fresh/upgrade; no non-SDD file lost vs v3.7.0 in any runtime; upgrade leaves no `__managed_by` except Kilo (→ T5). Pre-existing on main too (not this branch): first sync after install rewrites GEMINI.md/AGENTS.md/config.toml in gemini/hermes/copilot/cursor/qwen/codex/kiro/kilo; codex second install rewrites config.toml → follow-up issue, out of scope.

- 2026-09-26: T5 done (worker muhlebys-7-24kp): Kilo gets the v3.7.0 non-SDD set (jd-*, 4 lenses, review-refuter; review-validator stays OpenCode-only as in v3.7.0 because Kilo has no review relay) + same legacy migration. Commit 57871ec75. Review assess (base 3d01b3762): medium, under_budget (not due).
- 2026-09-26: T3 e2e rerun with branch binary (16 runtimes, env -i): all fresh/upgrade/sync exit 0; no non-SDD file lost vs v3.7.0; zero `__managed_by` after upgrade in every runtime; OpenCode fresh+upgrade = 12 subagents + orchestrator + gentleman, default_agent=gentle-orchestrator, share=disabled; Kilo = 8 subagents + orchestrator + gentleman, share=disabled. Install→first-sync rewrite of the system-prompt file (GEMINI.md/AGENTS.md/steering/rules/config.toml) reproduces identically on main → pre-existing, out of scope.
- 2026-09-26: full validation: `go run ./internal/gofmtcheck` ok; `go vet ./...` ok; `go test ./... -count=1` in isolated env -i on base 6c7f162f4 and branch in parallel → identical failure sets (7 tests / 4 packages: app TempDir cleanup, cli git-identity x3, reviewtransaction, update bash 3.2), zero branch-only failures.

- 2026-09-26: PR #4998 opened (type:bug, size:exception). All required CI passed; CodeRabbit no actionable comments. Copilot raised 2 valid medium findings (legacy `_shared/SKILL.md` marker kept on Windows compat transaction and on uninstall) → fixed test-first (worker mui15vi8-8-1t23), commit 3f8df5171. Review assess (base 3d01b3762): medium, slice_budget_reached → consent granted (pre-authorized) → lineage review-09a6850f955d9903 (review-reliability) APPROVED + acknowledged; advisory R3-001..003 only.

- 2026-09-26: Copilot 2nd pass: judge `bash` kept (Gentle Shell + v3.7.0 parity; replied); settings mode widening fixed → `filemerge.ExistingFileMode` for all OpenCode-family writers added/touched (commit 209d8867e, assess medium/under_budget). Sandbox shows other pre-existing writers (~80, MCP/engram/persona/…) still force 0644 on main too → follow-up. 18/18 checks green. Copilot/CodeRabbit 3rd pass: upgrade backup must use effective OpenCode settings path (OPENCODE_CONFIG_DIR); mode 0000 must not widen → delegated (mui2ap4d-a-iuh2). Merge policy: merge after this unless a critical/blocking finding appears.

## Follow-ups (out of scope)

- ~80 pre-existing config writers force mode 0644 on rewrite (systemic).

- `internal/assets/opencode/orchestrator.md` is not read by production code (OpenCode orchestrator prompt comes from agentguidance.RenderRouting); T1 edited it for consistency only.
- Uninstall does not remove OpenCode/Kilo parity agents (pre-existing shape of the retired overlay owner).
- First sync after install rewrites the system-prompt file in several runtimes (pre-existing on main).
- Pi-related Go tests read the real `~/.gentle-shell` when HOME is not isolated (test isolation defect).

## Next step

Push fix, reply to review threads, merge PR #4998 when required checks pass.