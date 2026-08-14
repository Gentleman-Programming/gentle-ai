# Proposal: Rollback-safe persona output-style transitions

## Intent

Fix #3163 (`status:approved`, `type:bug`); a PR may open when implementation completes. `removeFileAtomic` (inject.go:699) calls `os.Remove` after the new style and settings are written; a removal error leaves a mixed state instead of restoring pre-transition files/settings. #3161 (ea95254a) provides the resource plan and CLI rollback boundary; this change closes the removal-failure gap.

## Outcome

- **Success**: `neutral.md` exists, `gentleman.md` absent, settings select Neutral.
- **Failure**: previous style + settings restored, no partial Neutral; exit 0 + warning.
- **No retry loop** on removal failure (Windows locks) — direct rollback.
- Second successful install/sync is a no-op; user-owned files and unrelated settings unchanged.

## Scope

### In scope

- Reuse `ResourcePlanFor`/`OutputStylePaths` (Write/Backup/Remove) from #3161.
- Integrate removal with existing snapshot (`backupTargets`, `syncBackupTargets`) and pipeline `rollbackRestoreStep`.
- Add `var osRemove = os.Remove` hook (mirrors `osReadFile` inject.go:590, `piCodeGraphRemove` pi_codegraph.go:34) for failure injection.
- Failure-injection tests: unit + CLI end-to-end (per `TestSyncRollbackRestoresRemovedRetiredReviewPlugins`).
- Install and sync stay equivalent (both callers use `DefaultRollbackPolicy`).

### Out of scope

- No ledger, journal, or global state machine.
- No Claude `@import` work; no ownership reconstruction for legacy/user-owned files.
- No unrelated lifecycle refactor; no retry loop; no in-function restore.

## Capabilities

- **New** — `persona-output-style-transitions`: target-atomic transitions with rollback on removal failure, exit-0 warning, no-retry, install/sync parity.
- **Modified** — None (`persona-behavior-contract` content requirements unchanged).

## Approach

1. Route `removeFileAtomic` through `osRemove`.
2. Propagate removal error through `injectInternal` so `rollbackRestoreStep` restores snapshots (run.go:1643 / sync.go:1194).
3. Exit 0 + warning after restore.
4. Failure-injection tests at both layers assert observable files/settings.

## Alternatives considered

- In-function restore vs pipeline-boundary rollback → pipeline boundary (exploration).
- Retry-on-lock vs direct rollback → direct (user decision).
- Hard failure vs warning + exit 0 → warning (user decision).

## Affected areas

- `internal/components/persona/inject.go` — Modified: `osRemove` hook; `removeFileAtomic` routed through it.
- `internal/components/persona/inject_test.go` — Modified: unit failure-injection tests.
- `internal/cli/run.go`, `sync.go` — Modified: removal error joins rollback boundary; warning surfacing.
- `internal/cli/persona_transition_test.go` — New: end-to-end transition rollback coverage.

## Risks

- `var osRemove` hook rejected by maintainers — Low; mirrors existing patterns, confirm in review.
- Rollback restores files but not settings — Low; snapshot covers both style files + settings.json.
- Warning path hides real failures — Low; explicit warning, tests assert exit 0 + message.

## Rollback plan

Revert the PR commit(s); hook and tests are self-contained, no migration. Re-running install/sync after revert restores intermediate state.

## Dependencies

- #3161 resource plan (merged ea95254a) — required foundation, already on `main`.

## Success criteria

- [ ] Failed Gentleman→Neutral removal restores previous style file and settings (AC1)
- [ ] No partially published Neutral style after rollback (AC2)
- [ ] Successful transition removes only the retired Gentle-AI-owned style (AC3)
- [ ] Install and sync use the same transition contract (AC4)
- [ ] Second successful run is a semantic no-op (AC5)
- [ ] Failure-path tests assert externally observable files and settings (AC6)
- [ ] Removal failure → exit 0 + warning explaining rollback (PD1–PD2)
- [ ] No retry loop on removal failure (PD3)
