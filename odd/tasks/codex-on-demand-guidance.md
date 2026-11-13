# Codex on-demand guidance and current multi-agent configuration
Objective: extend existing on-demand delivery to Codex without losing essential policies or user configuration. Branch: `fix/codex-on-demand-guidance`; base: `a9c446bdcf4378e647d26c3bd644a814a61042a0`.
Delivery: single-pr, exception-ok (user selected "Un solo pr size exception"; forecast 500–700 authored changed lines, excluding generated goldens). Runner: Go tests with isolated temporary homes.

## Specs

- S1 — Original report: "cosas a arreglar de gentle ai para codex: `[features].multi_agent` is deprecated. It still controls the V1 fallback. `[agents].enabled = false` disables subagent tools even when the model selects a subagent version, unless `[features].multi_agent_v2` is enabled Combined user instructions use 122188 bytes, above the 32768-byte `project_doc_max_bytes` setting. The total includes global and thread instructions. The limit applies only to project files." Stop generating the deprecated configuration and align shipped capability guidance with the current interface. Preserve explicit user disablement and unrelated configuration. Verify fresh injection, legacy migration, explicit disablement, and idempotence.
- S2 — User asked "pero gentle-ai no tiene justo carga por demanda? como gentle-shell?" and authorized "Hagámoslo" after the proposal to extend existing modular delivery to Codex with essential rules always loaded. Reuse the existing core/module and ownership machinery; install reachable on-demand files while retaining safety, consent, and authority rules in the core. Preserve module bytes and verify reconstruction and reachable trigger pointers. Do not simply raise `project_doc_max_bytes` or summarize away rules.
- S3 — Necessary lifecycle integrity for S2: install, sync, upgrade backup/rollback, and full uninstall must account for managed module files and ownership. Preserve user-edited modules and existing Claude delivery. Verification uses isolated state, never the real Codex home. No real-home installation, remote operation, push, PR, or merge is authorized by this implementation.

## Tasks

- T1 [S2, S3] — Codex core/module delivery and lifecycle integration, including regression tests and docs. Route: delegated; two independent behavior units with disjoint module/config edit surfaces launched together. Status: implemented and functionally verified; local delivery authorized by the user despite unresolved advisory RDD. Commit: `75bfa6b2f`. Risk: items 1, 3, 5 (managed-file lifecycle, shipped instruction contracts, installer behavior).
- T2 [S1] — Modern multi-agent migration and capability guidance, including tests and docs. Route: delegated; parallel with T1, owning config injection and Codex capability asset only. Status: implemented and functionally verified; native severe findings remain unresolved. Commit: `0dbb87fce`. Risk: items 3, 5 (config compatibility and installer behavior).
- T3 [S1–S3] — Cross-runtime regression checks, isolated independent verifier, and native review assessment. Route: delegated verification due to named high-risk items; native review only through provider transitions and candidate consent. Status: functional and independent verification passed; native correction plan unavailable. Commit: verification accompanies each behavior unit.

## Log

### L1 — Original request

cosas a arreglar de gentle ai para codex: `[features].multi_agent` is deprecated. It still controls the V1 fallback. `[agents].enabled = false` disables subagent tools even when the model selects a subagent version, unless `[features].multi_agent_v2` is enabled Combined user instructions use 122188 bytes, above the 32768-byte `project_doc_max_bytes` setting. The total includes global and thread instructions. The limit applies only to project files.

### L2 — User clarifications and authorization, 2026-11-13

"pero de donde viene? es laog que instalo gentle-ai ? work/gentle-ai"

"pero gentle-ai no tiene justo carga por demanda? como gentle-shell?"

"Hagámoslo"

### L3 — Verified baseline and scope

- Local Codex CLI 0.169.0 reports `multi_agent` deprecated and `multi_agent_v2` stable. Gentle AI injects the former unconditionally in `internal/components/engram/inject.go:931-938`.
- Local global `~/.codex/AGENTS.md` measured 110,031 bytes; repository `AGENTS.md` measured 2,678 bytes. The reported combined total is not proof of project-file truncation. Do not claim a 32 KiB total budget or warning elimination without measurement.
- Existing `buildOrchestratorModules` accepts only Claude Code; Codex receives monolithic prompt sections. Pi orchestration belongs to Gentle Shell.
- Existing split moves only approximately 6.2 KB into delegation, writer, and skills modules. Reusing it alone will not bring the complete global carrier below 32 KiB. Measure the delivered result; no unrequested deletion of essential policy.
- Lifecycle map: `agentguidance/orchestrator_module_{install,transaction,uninstall}.go`; CLI install/sync snapshot plumbing; upgrade backup; full uninstall. Reuse ownership, compare-and-swap, and rollback helpers rather than introducing a second representation.
- Existing untracked `.worktrees/` and `odd/tasks/5930-antigravity-parity.md` are unrelated and must remain untouched.
- RDD is globally on. No review has started and no candidate consent has been granted.
- Source writes and implementation tests are pending. Pre-investigation focused multi-agent and orchestrator baseline tests passed; these are baseline evidence, not proof of a fix.

### L4 — Delivery boundary

- User selected "Un solo pr size exception". One coherent PR is planned because the warning fix spans config migration, shared module delivery, and backup/rollback/uninstall integrity. The exception is a local delivery choice, not a remote label mutation or publication authorization.
- Engram mirror: observation 27377; initial full-document readback verified.

### L5 — Implementation decisions, 2026-11-13

- Reuse the existing module mechanism for Codex global install/sync by default; do not add a parallel opt-in flag. Workspace delivery remains unchanged and must not acquire external home dependencies.
- Preserve the existing fragment policy, keeping safety/consent/authority in the core. Measure resulting carrier bytes and disclose residual size; broad policy rewrites and a guaranteed 32 KiB total are not authorized requirements.
- T1 owns module machinery/lifecycle/docs. T2 owns Engram Codex config injection/tests and the Codex capability asset. Parent owns integration/goldens/logbook/git. No writer commits or modifies real HOME.

### L6 — Writer and parent checks

- T1 implementation returned with observed RED tests for unsupported Codex modules, absent global default, missing upgrade snapshots, and missing uninstall snapshots. Required focused agentguidance, CLI module, uninstall module, and upgrade tests passed. Existing Claude global and workspace Codex outputs matched baseline in `<external-module-differential-scratch>`.
- T2 implementation returned with observed migration RED and GREEN; focused migration tests and the entire Engram component suite passed. OpenCode baseline/current output hashes matched in `<external-config-differential-scratch>`.
- Parent reran both migration and module/budget checks successfully; `git diff --check` passed. Baseline and component golden update commands completed without tracked golden changes.
- Isolated production install measured AGENTS.md at 86,529 bytes and three on-demand files at 1,171, 1,977, and 1,669 bytes. This does not eliminate the aggregate-size warning; only the existing safe fragment split is being extended.
- Native assessment was unassessable because intended and unrelated untracked files need explicit inventory declaration. It returned high risk and review_due=true; no risk downgrade, review START, or consent occurred. Reassess after intended files are tracked, with unrelated files excluded by the native declaration.
- Independent verifier is running the six affected package suites and isolated baseline-differential probes. Implementation remains uncommitted and not delivery-complete pending that proof and native review consent.

### L7 — Independent verification and bounded correction

- Initial verifier: S1 blocked by valid tab-separated/quoted `agents.enabled` keys becoming duplicate invalid TOML; baseline preserved those settings. S2/S3 probes passed, but the existing cross-runtime CLI assertion incorrectly still required a literal Codex monolith. Five package suites passed; CLI failed that assertion and later reached its default ten-minute timeout. The active timeout test passed independently on both baseline and candidate, so no extra defect was inferred.
- One correction batch: parser-first modern key detection reuses the existing BurntSushi TOML dependency; committed-regression candidate `TestCodexMultiAgentPreservesExplicitDisableVariants` observed RED then GREEN. CLI invariant now reconstructs exact guidance from reachable module paths/sections, rejects duplicate/missing/unreferenced content, and keeps other-runtime assertions unchanged. Customized-module retirement probe was added as a regression test.
- Corrected focused and complete Engram suite passed. Corrected CLI invariant/module tests passed. Parent reran both original blocker regressions successfully.
- Targeted independent recheck is pending; the failed CLI package is being rerun with a thirty-minute native timeout, not a new all-package review sweep. Verifier scratch: `<external-verifier-scratch>`.
- Pre-existing advisory: legacy feature assignments using unsupported alternate TOML syntax were already mishandled by the old injector. This migration targets the syntax Gentle AI emitted; no broad shared TOML writer redesign is included.


### L8 — Final functional proof and retained review blocker, 2026-11-13

- Independent correction recheck returned PASS for both original blockers. All six affected package suites passed collectively. The corrected full CLI retry, `go test -json ./internal/cli -count=1 -timeout=30m`, exited 0 after 1436 seconds with 4184 pass events and zero fail events. Parent read the saved exit/timing metadata and reran the blocker regressions. No new source changes or repeat full-suite run were needed.
- T2 was committed as `0dbb87fce` (`fix(codex): migrate deprecated multi-agent configuration`). T1 remains in the worktree with its tests and docs. The delivery remains one PR with the user's size exception; no push, PR creation, or actual-home sync occurred.
- User granted the first config candidate's native review. Its final capture entered `correction_required`, with severe findings, but the exact returned bound STATUS requested intended-untracked selection rather than the correction plan. Its exact selection submission omitted lineage and returned a fresh START, which was not executed.
- User selected "2 pero fíjate por qué y si está resuelto en main". The original exact candidate decline ran once and returned the expected target and `declined_this_candidate`; the subsequent exact bound STATUS still retained the unresolved correction. No approval, review toggle, substitute START, or source-built runtime was fabricated. Preserve the frozen binding rather than changing HEAD to bypass this blocker.
- Required remaining proof: the native correction findings/plan and their bounded resolution are unavailable through the returned continuation. Functional PASS is not native approval. T1/T2 must not be reported delivery-complete.

### L9 — Separate defect handoff, 2026-11-13

- User: "crea issue para eso y lo ataco luego yo y sigue con lo tuo". User then answered "si" to authorization for the current authenticated gh session on github.com, searching duplicates and reporting to Gentleman-Programming/gentle-ai.
- Open-and-closed duplicate search identified existing open #6058 for the same bound correction/intended-untracked failure. No duplicate issue was created. One in-place report-repair request with the Codex reproduction was posted and exact host readback confirmed: https://github.com/Gentleman-Programming/gentle-ai/issues/6058#issuecomment-6864103196. No labels, issue body, or state were changed.
- Archived local main and cached origin/main both reproduced the explicit pinned-base plus unrelated-untracked failure. Current remote main `5e4227bf02cdf13de874fce17049b4a910300d33` still has the `selectedBaseRef == ""` guard; this newest observation is source inspection, not an end-to-end reproduction on that commit.
- The separate RDD defect is left for the user. Original source work and prior proof are preserved; only this logbook was updated after the handoff. No review-authority mutation or further source implementation occurred.


### L10 — Explicit delivery instruction, 2026-11-13

- User: "nada exitos, si en la verificacion paso no pasa nada, pr y merge"; then "perdon continua" after interrupting a read-only command.
- This explicitly authorizes local delivery, push, PR creation and merge subject to ordinary repository requirements. The user accepts proceeding on functional verification without resolving this candidate's advisory RDD. RDD remains globally unchanged and no approval receipt is claimed.
- Preserve the existing review state as historical unresolved evidence; do not start another review for this delivery. Earlier L8/L9 delivery holds are superseded by this explicit instruction, not by a fabricated PASS.
- Base-repository issue #5256 is approved and already closed; it covers Codex core/module lifecycle. Human selection of closing/non-closing PR reference remains pending under repository policy.


### L11 — Linux-only baseline correction after CI, 2026-11-13

- PR #6121 links `Refs #5256` by the user's explicit selection. The user explicitly authorized `type:feature` and `size:exception` on #6121; both were applied and read back on the exact target. Native GitHub squash auto-merge was enabled without bypassing required checks.
- CI Unit Tests failed `TestOrchestratorModuleBaseline`. All three required Linux E2E jobs and policy checks passed. The full CLI package passed in CI. The baseline test skips on non-Linux; prior successful macOS package and `-update` commands did not execute this snapshot check. Earlier claims of baseline coverage were incomplete.
- Reproduced RED on Linux arm64 using a locally cross-compiled package test binary in an isolated Alpine container with no network, read-only root filesystem, temporary HOME, and no personal configuration mounts.
- Used the existing `-update` path on the complete baseline matrix. Focused rerun without `-update` and the full `agentguidance` package suite passed in Linux. Only the Codex snapshot row changed; the other 22 variants and the embedded review-contract hash/length remain unchanged. The drift comes from the intended Codex capability asset change, not a new runtime change.
- Copied only the generated baseline file back. `git diff --check` passed. No production code changed in this correction. Risk: none for this fixture-only correction; the feature's existing items 1, 3 and 5 remain as documented.
- Updated PR CI must pass before automatic merge. No merged outcome is claimed until target-host readback confirms it.


### L12 — Main integration for generated baseline, 2026-11-13

- Fixture correction `49cf16996` exposed a conflict with the baseline already updated on current main `5e4227bf02cdf13de874fce17049b4a910300d33`. Integrated that main without rewriting the feature branch. The only conflict was the generated baseline JSON; other files merged automatically.
- Regenerated from the integrated working tree, starting with main's baseline. The complete Linux matrix reproduced RED, then the official `-update` path, focused rerun and full Linux `agentguidance` suite all passed. Exactly the Codex row differs from current main; the other 22 variants and embedded review-contract measurements are unchanged.
- The PR remains pending CI and automatic merge. No production behavior change was added by the snapshot correction.


### L13 — Dead-code check correction during monitoring, 2026-11-13

- User: "monitorea". The next CI execution passed unit tests, benchmark build/vet/tests and evidence, all required E2E jobs, and policy checks. The Unit Tests job failed its later dead-code ratchet, not its unit-test step.
- The five new unreachable symbols were obsolete private Claude-only wrappers: `commitOrchestratorCore`, `commitObservedOrchestratorCore`, `commitPreparedOrchestratorCore`, `orchestratorCorePaths`, and `claudeGlobalModuleLedger`. CodeGraph identified only four test call sites for the first two; production already uses the generalized helpers.
- Removed the five wrappers and updated the four test calls to pass the existing fixture core path to the unchanged generalized helpers. Preserved transaction/rollback behavior and comments. No exception was added to `.deadcode-baseline.txt`.
- Observed the exact five-symbol RED using the CI-pinned `deadcode@v0.30.0` analyzer targeting Linux/amd64. After cleanup, the same normalized comparison reports zero new unreachable functions. Native macOS analysis also reports unrelated platform-only entries, so it is not claimed as a Linux-equivalent pass.
- Focused transaction tests, the complete Linux `agentguidance` suite, Go format and diff checks passed. Risk: none for removal of unreachable private wrappers and equivalent test call rewiring; live helper implementations and public contracts are unchanged.
- Push the correction and continue monitoring the already-authorized automatic merge. Do not bypass failed required checks.
