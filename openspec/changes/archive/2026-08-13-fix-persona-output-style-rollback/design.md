# Design: Rollback-safe persona output-style transitions

## Context

Issue #3163: `removeFileAtomic` (inject.go:696-708) calls `os.Remove` after the new style + settings are written; a removal error (e.g. Windows file lock) leaves a mixed state and exits 1. #3161 (merged ea95254a) already provides the snapshot/rollback boundary; this change closes the removal-failure gap: injectable removal seam (REQ-REMOVE-HOOK), propagation into pipeline rollback (REQ-ROLLBACK-PROPAGATION), exit 0 + warning (REQ-EXIT-WARNING), no retry (REQ-NO-RETRY), parity (REQ-PARITY).

## Technical Approach

Route `removeFileAtomic` through an exported removal seam; on failure return a **typed error** from `injectInternal` step 3. The pipeline already treats step errors as rollback triggers, so the existing boundary restores state; the CLI then classifies the rolled-back outcome post-execution and converts it to warning + exit 0. No retry loop, no in-function restore.

**Question 3 answer (verified in code): the existing path ALREADY rolls back on removal failure.** inject.go:424-433 returns the error → wrapped by the persona step (run.go:1643-1645 / sync.go:1194-1197) → `Runner` (StopOnError default, stages.go:25-29) stops → `ExecuteRollback` reverse-order (orchestrator.go:56-61) → `rollbackRestoreStep.Rollback()` (run.go:1172-1179, FIRST apply step at run.go:738/sync.go:543 so it always succeeded) → `RestoreService.Restore` (restore.go:161-184) rewrites pre-existing entries and removes created ones. The only behavioral gap is exit code + message: today exit 1, spec wants exit 0 + warning.

## Architecture Decisions

### D1: Snapshot scope (Q1 — PINNED)

| Option | Evidence | Decision |
|---|---|---|
| settings.json not covered | backupTargets run.go:1975-1985 / syncBackupTargets sync.go:643-653 only add `OutputStylePaths.Backup` (both style files) | Snapshot set is the UNION with the component path contract: settings.json IS covered |
| settings.json covered | componentPathsWithWorkspaceScoped run.go:2284-2286 and syncPersonaPathsWithWorkspace sync.go:818-820 add `adapter.SettingsPath` whenever `stylePaths.Write != ""` (true for Gentleman/Neutral) | **Per-adapter snapshot = {systemPrompt, settings.json, gentleman.md, neutral.md}**; restore rewrites pre-existing + removes created (neutral.md) — REQ-ROLLBACK-PROPAGATION provable, no target-set change needed |

### D2: Warning plumbing (Q2 + Q4)

| Option | Tradeoff | Decision |
|---|---|---|
| Warn inside pipeline (rollbackRestoreStep.Rollback) | Rollback() has no knowledge of WHY rollback ran; warns on unrelated failures | **Rejected** |
| Warn inside persona package | Package would need os.Stderr output; breaks layering | **Rejected** |
| Typed error + CLI classification | Pipeline keeps failure semantics; CLI converts after execution | **Chosen**: `persona.OutputStyleRemovalError` returned by injectInternal; `errors.As(Execution.Err)` + `Execution.Rollback.Success` at the two exit points |

On rollback success the orchestrator keeps `Err = applyResult.Err` (orchestrator.go:55; overwritten only when rollback FAILS, lines 58-60 — where the removal error is lost and classification correctly falls through to hard failure). Both run.go:223-225 and sync.go:1599-1601 call one shared helper `handleRolledBackPersonaTransition(exec)`; when it returns true: print `WARNING: <MessageRolledBackOutputStyle>` to stderr (pattern: run.go:188/209/1711) and return nil → exit 0. Returning BEFORE post-apply verification is required: rolled-back state is pre-transition (Neutral absent), so verification would fail and trigger a second pointless rollback.

### D3: Removal seam (REQ-REMOVE-HOOK)

| Option | Tradeoff | Decision |
|---|---|---|
| `var osRemove = os.Remove` (unexported, per proposal) | Mirrors osReadFile inject.go:590 + piCodeGraphRemove pi_codegraph.go:34, but package-`cli` e2e tests cannot reach it | **Rejected for CLI tests** |
| Exported `persona.RemoveFileFn` | Mirrors `backup.UserHomeDirFn` (restore.go:14) — the established exported-test-seam precedent, already overridden cross-package (sync_review_retirement_test.go:55) | **Chosen**: one seam serves white-box unit tests AND CLI e2e |

### D4: Failure injection (Windows-safe)

| Option | Tradeoff | Decision |
|---|---|---|
| chmod 000 on retired file | Unix-only; unreliable under root | **Rejected** |
| Override `persona.RemoveFileFn` with failing func | Platform-independent, deterministic, works on Windows CI | **Chosen**; count invocations to assert exactly-once (REQ-NO-RETRY) |

## Data Flow

```
injectInternal step 3 (inject.go:383-433)
  write neutral.md ──> merge outputStyle=Neutral into settings.json ──> remove gentleman.md (via RemoveFileFn)
        failure: &OutputStyleRemovalError ──> component step error (run.go:1644 / sync.go:1196)
          ──> Runner StopOnError ──> ExecuteRollback (reverse) ──> rollbackRestoreStep.Rollback()
          ──> RestoreService.Restore: restore settings.json+gentleman.md, remove neutral.md
          ──> RunInstall/RunSync: errors.As + Rollback.Success ──> WARNING + exit 0
```

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/components/persona/inject.go` | Modify | Add `RemoveFileFn` seam (inject.go:590 area); `removeFileAtomic` (696-708) routes through it; removal loop (424-433) returns `&OutputStyleRemovalError`; add exported error type + `MessageRolledBackOutputStyle` constant |
| `internal/cli/persona_rollback.go` | Create | Shared `handleRolledBackPersonaTransition(exec pipeline.ExecutionResult) bool` |
| `internal/cli/run.go` | Modify | run.go:223-225: call helper before wrapping error; return nil on handled rollback |
| `internal/cli/sync.go` | Modify | sync.go:1599-1601: same |
| `internal/components/persona/inject_test.go` | Modify | Unit failure-injection tests |
| `internal/cli/persona_transition_test.go` | Create | CLI e2e: install + sync parity, restore + warning + exit 0 |

## Interfaces / Contracts

```go
// persona package
var RemoveFileFn = os.Remove // removal seam; tests override (backup.UserHomeDirFn pattern)

// OutputStyleRemovalError marks a retired-style removal failure that the
// pipeline may roll back; CLI converts to exit-0 warning when rollback succeeded.
type OutputStyleRemovalError struct{ Path string; Err error }
func (e *OutputStyleRemovalError) Error() string // "remove retired output style %q: %v"
func (e *OutputStyleRemovalError) Unwrap() error

const MessageRolledBackOutputStyle = "The retired persona output style could not be removed, so the " +
    "previous style file and settings were restored. Nothing was half-applied. " +
    "Close any program that may have the file open and run again."

// internal/cli
func handleRolledBackPersonaTransition(exec pipeline.ExecutionResult) bool {
    var removalErr *persona.OutputStyleRemovalError
    if !errors.As(exec.Err, &removalErr) || !exec.Rollback.Success { return false }
    fmt.Fprintf(os.Stderr, "WARNING: %s\n", persona.MessageRolledBackOutputStyle)
    return true
}
```

## Testing Strategy

| Layer | What | Approach |
|---|---|---|
| Unit (inject_test.go) | Removal failure surfaces as `*OutputStyleRemovalError`; error wraps path+err; noop removal stays silent | Override `RemoveFileFn` (pattern inject_test.go:2811-2813, pi_codegraph_test.go:631-638, `t.Cleanup`); assert `errors.As`; SEN-INJECTED-REMOVAL-FAILURE |
| CLI e2e (persona_transition_test.go) | Gentleman→Neutral, `RemoveFileFn` fails → rollback restores gentleman.md + settings byte-for-byte, neutral.md absent, user file untouched, err==nil, stderr has constant, hook called once | Mirror TestPersonaSyncOutputStyleSwitchIsIdempotent (sync_test.go:3988) + osUserHomeDir/backup.UserHomeDirFn overrides (sync_review_retirement_test.go:52-55); install side via `RunInstall(args, system.DetectionResult{})` (run_integration_test.go:74) |
| Parity (REQ-PARITY) | Same injection for install AND sync → identical warning + restore + exit 0 | Loop both pipelines in one test (SEN-INSTALL-SYNC-PARITY) |
| Helper unit | Classification: typed+rollback-ok→true; rollback-failed→false; generic error→false | Construct `ExecutionResult` directly |
| E2E | Not needed | CLI pipeline tests are the top layer (sync_review_retirement_test precedent) |

## Failure Modes

| Mode | Behavior |
|---|---|
| Removal fails after write+settings merge (primary) | Typed error → rollback → warn + exit 0 |
| Failure before write | Unreachable: writes precede removal (inject.go:391-407 before 424-433) |
| Multiple retired paths, later one fails | Earlier removals restored from snapshot |
| Second run after successful transition | `os.IsNotExist` → silent noop, exit 0 (REQ-NOOP, idempotency test unaffected) |
| Windows locked file | `os.Remove` fails → rollback, warn, exit 0, attempted exactly once (REQ-NO-RETRY) |
| User files / unrelated settings | Restored byte-for-byte from snapshot on rollback; overlay merge touches only `outputStyle` on success |
| Rollback itself fails | Hard failure with joined errors (orchestrator.go:58-60), no warning |

## Threat Matrix

N/A — no routing, shell command, subprocess, VCS/PR automation, executable-file classification, or process-integration boundary is introduced or changed; the change adds a file-removal seam and exit-code classification inside the existing install/sync pipeline.

## Migration / Rollout

No migration required. Hook defaults to `os.Remove`; behavior identical on success paths.

## Open Questions

None blocking. Note: removing the old error string `"remove retired output style: %w"` may affect an existing test asserting that text — sdd-apply should grep before changing.
