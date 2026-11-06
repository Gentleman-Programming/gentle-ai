# ODD Task — #1635 Antigravity Concurrency Correction (PR5118 A+B)

Status: implemented; independent focused/race/build checks passed.
Scope: A1+A2 guards + B lock. Full suite/native Windows/macOS tests pending.

## Plan

- **A1 (classify/restore guard)**: `classifyAntigravityOwnershipTransferFailure`
  Present branch ignored `verifyErrs` and restored stale before-images over
  newer bytes; a concurrent global retirement then deleting the plugin could
  leave zero Engram registrations. Now: Present + verifyErrs → uncertainty, no
  restore; verified → reread the global immediately before restoring
  (classification seam), any drift/error → uncertainty, no destructive restore,
  no availability claim.
- **A2 (rollback drift guard)**: `installAntigravityEngramPlugin` rollback
  restored before-images unconditionally. Now each asset records the exact
  `left` state the pass last wrote; `restoreAntigravityPluginAssets` takes a
  `{before, left}` plan and restores only assets whose current bytes still
  match `left` (existence + bytes; mode excluded by design). Drift, read, and
  restore failures are joined; restoration is refused when the match cannot be
  proven (read failure → nothing restored, uncertainty reported).
- **B (coordination lock)**: reuse `internal/filecoord` (acyclic; nonblocking
  flock/LockFileEx, `ErrBusy`) around ALL managed Antigravity mutations:
  settings ensure, plugin install/rollback, global removal and
  classify/restore. Flow: side-effect-free prevalidate → acquire → revalidate
  → mutate. Malformed global creates zero paths (not even the lock root).
  Shared protocol prompt stays outside the lock. Lock target: CLI plugin dir
  `<configHome>/.gemini/antigravity-cli/plugins/gentle-ai-engram` (always CLI,
  so desktop/CLI variants share one key); root: canonical
  `<configHome>/.gentle-ai/locks`. Canonicalization: EvalSymlinks, else
  deepest-existing ancestor + suffix (statecoord pattern); aliases share a
  key, distinct physical paths (workspace/global) stay distinct. Single
  nonblocking attempt on `context.Background`; `ErrBusy` wrapped with retry
  advice; release errors joined; one acquisition per pass, no nesting.

## Evidence (RED/GREEN, deterministic, no sleeps)

RED A (existing seams; observed failures before implementation):
- `go test ./internal/components/engram/ -run 'TestInjectAntigravityClassifyDriftBlocksDestructiveRestore|TestInjectAntigravityStaleGlobalRetirementPreventsPluginRestore|TestInjectAntigravityRollbackPreservesDriftedPluginAsset' -count=1`
  → all 3 FAIL on the defective behavior (restore over drift / no reread /
  unconditional rollback restore).
RED B (new lock tests): `go vet ./internal/components/engram/` → compile
failure `undefined: acquireAntigravityCoordinationLock` (implementation absent).

GREEN:
- Same focused command → `ok` after implementation.
- `go test ./internal/components/engram/ ./internal/filecoord/ ./internal/components/filemerge/ -count=1` → all ok.
- Race-proportionate: focused antigravity/coordination suites with `-race` → ok.
- `GOOS=windows go build ./internal/components/engram/` and `GOOS=darwin ...` → ok.
- `go test ./internal/components/... ./internal/cli/ -run 'Antigravity|antigravity' -count=1` → ok.
- `gofmt -l` (authorized) → clean; `git diff --check` (diffcheck) → clean.

Contract updates to pre-existing tests (justified by A2):
- `TestInjectAntigravityRecoveryFailureIsExplicit/recovery read failure`: a
  faulted read cannot prove the disk matches what the pass wrote, so recovery
  now refuses to restore anything (landed bytes kept) instead of restoring
  under uncertainty; error identities and accounting assertions unchanged.

## Mirror

- `internal/components/engram/antigravity_transfer_lock.go` (new): lock target/
  root derivation, canonicalization, single-attempt acquisition.
- `internal/components/engram/inject.go`: `injectAntigravityOwnership` /
  `injectAntigravityMutationsUnderLock`; guarded rollback plan; classify guard.
- `internal/components/engram/antigravity_transfer_lock_test.go` (new): held
  lease, alias contention, distinct workspace/global keys, zero-paths
  malformed-global, release-on-failure, missing-leaf canonicalization.
- `internal/components/engram/inject_test.go`: A1/A2 drift tests; contract
  update above; removal of pre-existing dead helpers flagged by the static
  gate (`engramServerJSON`, `ensureAntigravitySettings`+
  `settingsBootstrapResult`, `isAbsoluteEngramPath`, `assertNestedBool`,
  unused `agentID` param blanked). Post-merge correction (a5c209d1):
  `assertNestedMissing` is retained because the upstream
  `TestInjectPiProvisioningRetiresMCPAdapterAndPreservesUnrelatedContent`
  migration test calls it; `assertNestedStringsUnordered` is dropped since
  upstream rewrote its only call sites (pi-mcp-adapter retirement), and
  `assertNestedBool` stays removed (still unused on main).
- `.deadcode-baseline.txt`: +2 entries (`internal/filecoord/lock.go`
  `UnsupportedError.Error` / `.Unwrap`). These methods are pre-existing and
  unreachable from main; importing `internal/filecoord` for the B lock makes
  them newly *analyzer-visible* (deadcode now scans that package), not newly
  runtime-reachable — pristine HEAD passes only because the package was never
  scanned. Public error API is intentionally preserved rather than deleted,
  so they are baselined: no new dead implementation was added by this task.

## Size / budget

- Direct base `a5c209d1` (latest main) → resolved worktree, measured after the
  merge resolution: 6 files, +1222/−176 changed lines (source + tests +
  baseline deltas + this document) = 1398 changed lines.
- 1398 ≤ 1400 budget (at ceiling, 2 lines of headroom).

## Limitations

- Guards are read-then-act: residual TOCTOU between guard reads and restores
  remains; cooperative locks serialize gentle-ai writers only, never arbitrary
  external editors; no multi-file atomicity.
- A dangling-symlink intermediate component under a not-yet-existing config
  home is canonicalized only through the deepest existing ancestor.
- `filepath.EvalSymlinks` alias tests skip on Windows (symlink privileges).
