# Native agent ownership recovery guidance (#4994)

## Objective
Clarify why sync preserves native agents and give a safe, opt-in per-file recovery path without changing ownership policy.

## Problem and rationale
The warning conflates unverified ownership with customized bytes and suggests merging even when no changes exist. Ledger-less matching files are intentionally unmanaged. Recovery should explain how to keep user files or explicitly reinstall a selected file under management.

## Authorized scope and constraints
- Message wording in `internal/cli/run.go`.
- Focused regression coverage in `internal/cli/run_component_paths_test.go` and, if needed, `internal/cli/sync_test.go`.
- Recovery subsection in `docs/rollback.md`.
- Preserve ownership/adoption logic, matching-file refusal, customized bytes and symlink safeguards.
- No real HOME installation, sync, deletion, GitHub posting, publishing, push or PR.
- Technical artifacts in English.

## Work unit
- [x] T1: Clarify preservation messaging, document safe per-file recovery, and verify unchanged ownership protection.
  - Status: complete (checks passed and work-unit committed).
  - Route: delegated direct; message, tests and documentation are multiple non-trivial files; parent reused prior ownership exploration and one scoped message/docs map.
  - Acceptance: warning identifies ownership as unverified without asserting files were modified; explains backup, removal of only the warned file, and reinstall with explicit user choice; keeping customized files remains supported; no auto-adoption or ledger manipulation.
  - Test-first: observe focused RED for new message assertions, implement GREEN, normalize before final checks.
  - Checks: focused CLI message/report tests; ownership/no-ledger tests; full CLI and reviewassets packages; full Go suite if practical through verifier; structural docs readback and `git diff --check`; user-owned RDD on requires native review of this work unit.
  - Runtime proof: test fixtures only, never installed user agents. Recovery command syntax checked against existing CLI/docs, with isolated fixture execution where practical.
  - Rollback boundary: revert changed warning, message assertions and recovery subsection together; ownership implementation unchanged.
  - Commit: `1508e09cd0ad866e01428f7e7fa5653003f053a6` — `fix(sync): clarify preserved native agent recovery` (four files, 118 additions / 3 deletions).

## Delivery
Feature branch: `fix/4994-native-agent-recovery-guidance` (from clean `refactor/remove-opencode-community-plugins`, HEAD `d13d39bfc49395d53778dead710d826ab4db21df`).
Strategy: ask-on-risk. Forecast: 100–180 authored changed lines, one coherent work unit; delivered behavior unit: 121 authored changed lines (tracking document excluded from behavior count). No remote delivery authorized.

## Evidence and progress
- Exploration: shared warning helper `internal/cli/run.go:945–947`; existing tests `run_component_paths_test.go:227–260`, `sync_test.go:3422–3462`; existing guide `docs/rollback.md`.
- Ownership policy intentionally pinned by `TestNativeAgentNoLedgerDoesNotAdoptMatchingBytes`; no policy edits authorized.
- RDD reads on, decided by global.
- Writer observed focused RED on old warning and GREEN after wording change; gofmt complete.
- Writer focused CLI and ownership checks passed; full CLI/reviewassets packages passed after an initial 120-second timeout (retry CLI 198.203s). `git diff --check` passed, also independently spot-checked by parent.
- Disposable Kiro/global fixture exercised recovery and a subsequent sync; Claude/Kiro/Kimi global/workspace command syntax parser-validated. Other combinations are not runtime-proven.
- Diff: four source/test/doc files, +118/-3. Ownership implementation unchanged.
- Independent verifier `muqwdlvo-4-br9f`: focused CLI tests passed (0.090s), focused ownership tests passed (0.046s), `go test ./...` passed (CLI 185.465s, reviewassets 1.126s; some other packages cached), `git diff --check` passed, check-only gofmt clean; no blocking findings.
- Risk: conservative high verification after initially unassessable assessment; native review classified source candidate medium. Independent full-suite verification retained.
- Limits: no standalone binary recovery beyond fixtures; fixture does not prove workspace, nondefault model preservation, unrelated-file preservation or exact regenerated ledger hash. Full suite was nonverbose, so internal skips were not enumerated.
- Native reliability review `review-e10d772b3201a069` approved; exact acknowledgement burned authority for frozen target `sha256:d55cf72a78205e6753117cebded08731f141553bdaede044c03a98f959d495e0`. This task progress update is passive bookkeeping after that reviewed snapshot.
- Initial ASSESS was unassessable due undeclared untracked task file; exact inspect selection resolved scope, with independent verification retained.

## Authorized PR delivery
The user explicitly authorized GitHub delivery through the current gh session, adding `status:approved` to #4994, creating a `type:bug` PR, and merging after checks. The user selected `Closes #4994`; this closes the recovery issue while intentionally retaining the no-adoption policy.

- [ ] T2: Revalidate the isolated PR candidate on main (in progress).
  - Route: parent Git isolation plus independent verifier; no new behavior edits.
  - Main base: `9dfe17d837dcd5c164c904164ae3888ff4f3ff54`; PR branch: `fix/4994-native-agent-recovery-main`.
  - Source unit cherry-picked as `da452f6c`; initial tracking commit `71740216`. Original branch and OpenCode PR #5200 left untouched.
  - Checks: focused CLI/ownership regressions, full Go suite, canonical gofmtcheck, diff check, exact isolated native review.
- [ ] T3: Publish one focused PR and merge only after target-required CI (pending).
  - Route: parent authorized gh/Git delivery; do not bypass required checks or merge unrelated OpenCode changes.
  - Verified #4994 approval mutation/readback with target-host ADMIN actor.
  - Required checks from active target rules: issue approval/reference, exactly one PR type label, Unit Tests, E2E Tests (ubuntu/arch/fedora).
  - PR source slice: 168 authored changed lines before this delivery bookkeeping update; below 400, no exception or chain needed.
  - Remote CI verifies E2E; local Docker E2E not yet executed.

## Next step
Verify isolated branch, review and publish the focused PR, then await required CI before merge. Record final delivery in Engram without modifying approved source solely to record external CI. No real installed agents were touched; automatic adoption remains forbidden.
