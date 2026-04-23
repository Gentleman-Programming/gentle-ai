# Proposal: Engram Data Directory — Peer Review Fixes

## Intent

The "First-class Engram Data Directory Selection & Migration" feature was implemented to let users choose where Engram stores its SQLite database, migrate existing data, and persist that choice across sessions. A comprehensive peer review revealed that while the core feature works, several edge cases and persistence gaps break the user experience — most critically, TUI reconfiguration does not update the shell environment, so the user's choice is silently lost after restarting their terminal.

This change fixes the identified issues to make the feature production-ready.

## Scope

### In Scope
- Cross-platform shell profile / Windows registry persistence from TUI reconfiguration
- Install scripts: only persist non-default Engram directories
- Path normalization: relative paths in `ENGRAM_DATA_DIR`, string mismatch between tilde and absolute forms
- Validation of persisted `EngramDataDir` before loading into TUI model
- Unix locked-file detection fallback (`lsof`/`flock`) before migration

### Out of Scope
- Interrupted migration resume (requires migration journal)
- Rollback restoring old Engram location (requires backup including Engram data)
- Concurrent TUI/CLI coordination (requires IPC)
- WSL path translation (requires platform detection)
- "Abandoned data" warning when choosing Start Fresh with existing data (MN-4)
- Style/color nits (NI-3, MN-2)

## Capabilities

### New Capabilities
- `profile-persistence`: Cross-platform Go API for writing/removing `ENGRAM_DATA_DIR` from shell profiles and Windows registry
- `unix-locked-detection`: `lsof`/`flock` fallback for `DetectLockedData` on Unix systems

### Modified Capabilities
- `engram-dir-selection`: TUI reconfiguration now propagates to persistent shell environment
- `install-script-persistence`: Only writes to profile/registry when dir differs from default
- `engram-env`: `DefaultDataDir()` normalizes relative paths; `ExpandDataDir()` validates before use

## Approach

1. **Profile persistence API**: Extract shell profile manipulation from `install.sh` into `internal/platform/profile_env.go`. Implement Unix (bash/zsh/fish profile detection, `grep -v` + atomic rewrite) and Windows (`setx` registry) variants. Expose `PersistEngramEnv(dir)` and `RemoveEngramEnv()`.
2. **TUI integration**: Call `PersistEngramEnv` from `model.go` EngramConfigMode confirmation when the chosen dir differs from default. Call `RemoveEngramEnv` when user selects "Keep current" and a custom dir was previously persisted.
3. **Install script cleanup**: Update `install.sh` and `install.ps1` to skip `persist_engram_env` when the target equals the hard default (`~/.engram`).
4. **Path normalization**: Add `filepath.Abs` to `DefaultDataDir()` so relative env vars resolve consistently. Add `filepath.Clean` before source/target equality checks in `run.go` and `model.go`.
5. **Validation**: In `app.go` `loadPersistedAssignments`, verify the persisted path exists and is writable before populating `selection.EngramDataDir`. If invalid, log and clear.
6. **Unix locked detection**: In `DetectLockedData`, attempt `lsof` on the `engram.db` file. If `lsof` reports the file is open, return `true`. If `lsof` is unavailable, fall back to `false` (current behavior).

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/platform/` | New | `profile_env.go` — cross-platform shell profile/registry persistence |
| `internal/components/engram/env.go` | Modified | Normalize paths, add Unix locked-file fallback |
| `internal/tui/model.go` | Modified | Call profile persistence in EngramConfigMode confirmation |
| `internal/app/app.go` | Modified | Validate persisted `EngramDataDir` before loading |
| `internal/cli/run.go` | Modified | `filepath.Clean` before source/target comparison |
| `scripts/install.sh` | Modified | Skip persistence for default dir |
| `scripts/install.ps1` | Modified | Skip persistence for default dir |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Profile parsing corrupts user's `.bashrc`/`.zshrc` | Low | Atomic write (`mktemp` + `mv`); backup original file before mutation; use `grep -v` only on known prefix patterns |
| `setx` on Windows truncates env var value | Low | `setx` has 1024-char limit; Engram paths are typically short; document limitation |
| `lsof` not available on minimal Unix systems | Med | Graceful fallback to `false` (current behavior); document that migration on busy systems is best-effort on Unix |
| Relative path normalization breaks existing user workflows | Low | Only affects users who manually set relative `ENGRAM_DATA_DIR`; this was already broken behavior (CWD-dependent) |

## Rollback Plan

1. Revert the branch merge — all changes are additive or defensive (new package, path normalization, validation)
2. If profile corruption occurred, user can restore from `.bashrc.bak` or `.zshrc.bak` (created by atomic write)
3. Windows registry change can be reverted manually via System Properties → Environment Variables

## Dependencies

- `lsof` command (optional, Unix-only) — for locked-file detection fallback
- `setx` command (Windows) — for registry persistence

## Success Criteria

- [ ] TUI reconfiguration to `/custom/engram` updates shell profile/registry; new terminal session uses `/custom/engram`
- [ ] TUI reconfiguration to "Keep current" removes custom dir from profile/registry if previously set
- [ ] Install script with default dir does NOT write to shell profile/registry
- [ ] `ENGRAM_DATA_DIR=./engram` resolves to absolute path before use
- [ ] `ENGRAM_DATA_DIR="~/.engram"` vs expanded `/home/user/.engram` does not trigger self-copy migration
- [ ] Corrupted/invalid `EngramDataDir` in `state.json` is cleared with a warning, not propagated
- [ ] `DetectLockedData` returns `true` on Unix when `lsof` reports `engram.db` is open
- [ ] `go test ./...` passes; `go vet ./...` clean
