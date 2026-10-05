# SDD-retirement and review follow-ups

Locator: `odd/tasks/sdd-retirement-followups.md` · Engram mirror: `odd/sdd-retirement-followups/tasks`
Branches: one per PR slice from `origin/main` (PR A `fix/review-guidance-ownership` holds this document) (worktree `gentle-ai-worktrees/sdd-retirement-followups`, base `origin/main` 6471b688e)

## Objective

Close every follow-up left by PR #4998 and PR #5005 so the next release ships without the known rough edges.

## Why

User request (2026-09-26): "Todos" — fix all listed follow-ups.

## Scope (authorized)

- F1 — Kilo cleanup must only drop `gentle-orchestrator.permission.task.review-*` keys for review entries removed as gentle-ai-managed in the same run (or legacy `__managed_by` removals); never user-configured permissions (Copilot 4111203252 on #5005).
- F2 — Review-contract source for `agentguidance.RenderOrchestrator` must not depend on a CLI-only `init()`; pass it through injection/render options (or a dependency-neutral seam) and keep fail-closed behavior explicit.
- F3 — An orchestrator render failure must not silently block the routing block; decide and implement explicit behavior (fail the step with a clear error, or install routing and report) — evidence-based.
- F4 — Legacy v3.7.0 `~/.kimi/sdd-orchestrator.md` module: remove only with ownership proof (e.g. byte/hash match to a v3.7.0 render or known generated header); otherwise leave and document.
- F5 — Config writers that rewrite existing files with a fixed `0o644` widen private files; preserve existing permission bits on rewrite (systemic).
- F6 — Uninstall must remove gentle-ai-managed OpenCode/Kilo agent entries (managed shape only) and their orchestrator task permissions.
- F7 — First `sync` after `install` rewrites the system-prompt file in several runtimes; find root cause and make install and sync converge.
- F8 — Go tests that exercise Pi read/write the real `~/.gentle-shell` when HOME is not isolated; make them hermetic.
- F9 — `gentle-ai install` invoking `brew install` triggered a Homebrew auto-update on the user's machine; avoid unrequested auto-update side effects.

## Tasks

- [x] T0 — Root-cause map (explore muiaxmhp-e-etnp). F3: failure already fails the step and rolls back (not silent). F2: only prod caller is cli; pass source via RoutingOptions. F4: v3.7.0 wrote the rendered prompt (placeholders expanded) to ~/.kimi/sdd-orchestrator.md via StrategyJinjaModules; not reproducible → no ownership proof; main's KIMI.md no longer includes it (inert) → leave on disk, documented. F5: ~76 literal-perm callers; intentional mode setters: gga runtime scripts 0755, claude adapter OAuth 0600, backup restore, opencodeplugin restore. F6: uninstall only removes agent.gentleman for OpenCode; shapes live in internal/cli (uninstall cannot import cli). F7: install runs codegraph guidance before persona; Gentleman persona rewrites FileReplace prompt files and drops the codegraph section; sync re-adds it. F8: `PI_CODING_AGENT_DIR` (absolute, exported by Gentle Shell) overrides homeDir in Pi adapter. F9: executeCommand inherits env; no HOMEBREW_* anywhere.
- [x] T1 (PR A, `fix/review-guidance-ownership`) — F1 Kilo task-permission cleanup only for entries removed as managed in the same run or legacy-marker removals; F2 review contract via RoutingOptions (global only as test fallback, drop cli init()); F3 keep fail-closed with explicit error naming agent + remediation, test step failure + snapshot restore. Route: delegated.
- [ ] T2 (PR B) — F6 uninstall removes gentle-ai-managed OpenCode/Kilo agent entries + their orchestrator task permissions; move managed specs/shape predicates into a component package shared by cli and uninstall. Route: delegated.
- [ ] T3 (PR C) — F5 WriteFileAtomic preserves existing permission bits on replace by default; explicit forced-mode API for gga scripts, claude OAuth, backup/opencodeplugin restore. Route: delegated.
- [ ] T4 (PR D) — F7 install/sync convergence (codegraph guidance after persona; check hermes + codex config.toml). Route: delegated.
- [ ] T5 (PR E) — F8 hermetic Pi tests (unset/override PI_CODING_AGENT_DIR in package TestMains or shared helper); F9 brew commands get HOMEBREW_NO_AUTO_UPDATE=1 and HOMEBREW_NO_INSTALL_CLEANUP=1. Route: delegated.
- [x] F4 — no code change (no ownership proof; inert). Documented.

## Checks

- Isolated `go test` (`env -i` HOME) per package; full `go test ./...` vs base; gofmtcheck; vet; cross-build.
- Sandbox install/sync matrix where install behavior changes.

## Delivery

Small independent PRs preferred (stacked-to-main) — pending user delivery decision.

## Progress

- 2026-09-26: Feature doc created; worktree from 6471b688e.

- 2026-09-26: T0 done; PR slicing A–E decided.

- 2026-09-26: T1 done (writer muib6hx9-f-fv6t): Kilo task keys dropped only for review entries removed as managed or by legacy marker this run; RoutingOptions.ReviewContract + RenderOrchestratorWithSource, cli init() removed; explicit fail-closed error naming agent + remediation. Isolated tests: only known env failures. User authorized delivery A–E and approved #1913 / closed #4105. PR B writer launched (muic5yj7-g-8geo) in worktree uninstall-opencode-agents.

## Next step

PR A delivery; PR B.
