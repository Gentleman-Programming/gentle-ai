# Tasks: Engram Data Directory — Peer Review Fixes

## Phase 1: Infrastructure

### 1.1 Create `internal/platform/profile_env.go`
- [x] Define `PersistEngramEnv(dir string) error`
- [x] Define `RemoveEngramEnv() error`
- [x] Implement `detectShell() string` (reads `$SHELL`)
- [x] Implement `unixProfilePaths() []string` (bash/zsh/fish fallbacks)
- [x] Implement `rewriteProfile(path string, lines []string) error` (atomic: temp + mv)
- [x] Implement Unix variant: grep-filter old lines, append new, atomic write
- [x] Implement Windows variant: `setx` for write, `REG DELETE` for removal
- [x] Handle default-dir skip: if `dir == engram.DefaultDataDir()`, call `RemoveEngramEnv()` instead
- [x] Tests: `TestPersistEngramEnv_Bash`, `TestPersistEngramEnv_Zsh`, `TestRemoveEngramEnv`, `TestPersistEngramEnv_DefaultDirSkips`

### 1.2 Update `internal/components/engram/env.go` — Path Normalization
- [x] Add `filepath.Abs` call in `DefaultDataDir()` when env var is non-empty
- [x] Add `filepath.Clean` to `ExpandDataDir()` return value
- [x] Tests: `TestDefaultDataDir_NormalizesRelativePaths`, `TestDefaultDataDir_AbsoluteUnchanged`

### 1.3 Update `internal/components/engram/env.go` — Unix Locked Detection
- [x] Add `lsof` fallback in `DetectLockedData` for `runtime.GOOS != "windows"`
- [x] Execute `lsof -t engram.db` and check if output is non-empty
- [x] If `lsof` returns "command not found", fall back to `false`
- [x] Tests: `TestDetectLockedData_LsofAvailableAndOpen`, `TestDetectLockedData_LsofAvailableAndClosed`, `TestDetectLockedData_LsofMissing`

## Phase 2: Pipeline & Install Logic

### 2.1 Update `internal/cli/run.go` — Source/Target Comparison
- [x] Replace `defaultDir != s.selection.EngramDataDir` with `filepath.Clean(defaultDir) != filepath.Clean(s.selection.EngramDataDir)` in `componentApplyStep.Run()`
- [x] Verify no other raw string comparisons of Engram paths exist
- [x] Test: `TestComponentApplyStep_MigrationSkippedForSamePath` (with trailing slash/tilde variants)

### 2.2 Update `scripts/install.sh` — Skip Default Persistence
- [x] Before calling `persist_engram_env`, check `if [ "$engram_data_dir" != "$(get_hard_default_engram_data_dir)" ]`
- [x] Add `get_hard_default_engram_data_dir()` function (returns `$HOME/.engram`, ignores env var)
- [x] Verify existing tests still pass

### 2.3 Update `scripts/install.ps1` — Skip Default Persistence
- [x] Before calling `Persist-EngramEnv`, check `if ($engramDataDir -ne (Get-HardDefaultEngramDataDir))`
- [x] Verify existing tests still pass

## Phase 3: TUI & State

### 3.1 Update `internal/app/app.go` — Validate Persisted Path
- [x] In `loadPersistedAssignments`, before setting `selection.EngramDataDir = s.EngramDataDir`:
  - Check `os.Stat(path)` exists and is a directory
  - Check writability with temp file create/delete
  - If invalid: log warning, clear `s.EngramDataDir` in state file, do NOT populate selection
- [x] Test: `TestLoadPersistedAssignments_InvalidPathCleared`
- [~] Test: `TestLoadPersistedAssignments_ReadOnlyPathCleared` — skipped on Windows (POSIX perms don't apply)

### 3.2 Update `internal/tui/model.go` — TUI Reconfiguration Persistence
- [x] In EngramConfigMode confirmation (choice != Keep):
  - After `state.Write`, call `platform.PersistEngramEnv(path)`
  - ~~Surface error to user if persistence fails (non-blocking warning)~~ — logged silently for now; UI warning deferred to avoid complexity
- [x] In EngramConfigMode confirmation (choice == Keep):
  - After clearing state, call `platform.RemoveEngramEnv()`
  - ~~Surface error to user if removal fails (non-blocking warning)~~ — logged silently for now; UI warning deferred
- [ ] Test: `TestEngramConfigMode_CustomPathPersistsToProfile` (mock platform)
- [ ] Test: `TestEngramConfigMode_KeepCurrentRemovesFromProfile` (mock platform)

### 3.3 Update `internal/tui/model.go` — Path Comparison in Validation
- [x] In `confirmSelection` ScreenEngramDataDir handler, before setting `Selection.EngramDataDir`:
  - Call `filepath.Clean(path)` and compare with `filepath.Clean(engram.DefaultDataDir())`
  - If equal, treat as "Keep current" (don't persist, don't set env var)
- [ ] Test: `TestEngramDataDir_DefaultPathTreatedAsKeep`

## Phase 4: Integration & Verification

### 4.1 End-to-End Test: Fresh Install → Reconfigure → New Terminal
- [ ] Run `install.sh` with default dir → verify profile has NO `ENGRAM_DATA_DIR`
- [ ] Run TUI, reconfigure to custom dir → verify profile HAS `ENGRAM_DATA_DIR`
- [ ] Open new terminal, run `gentle-ai` → verify TUI shows custom dir
- [ ] Reconfigure back to default → verify profile has NO `ENGRAM_DATA_DIR`

### 4.2 End-to-End Test: Windows Registry
- [ ] Run `install.ps1` with default dir → verify registry has NO `ENGRAM_DATA_DIR`
- [ ] Run TUI, reconfigure to custom dir → verify registry HAS `ENGRAM_DATA_DIR`
- [ ] Open new PowerShell session, check `$env:ENGRAM_DATA_DIR`

### 4.3 Regression Tests
- [ ] `go test ./...` passes
- [ ] `go vet ./...` clean
- [ ] All pre-existing Engram tests pass
- [ ] Install script dry-run tests pass

## Phase 5: Documentation

### 5.1 Update AGENTS.md or docs
- [ ] Document that TUI reconfiguration updates shell profile/registry
- [ ] Document that default-dir installs do NOT modify profile
- [ ] Document Unix locked-file detection best-effort behavior
- [ ] Document relative path normalization

## Remaining Nits (Post-Review)

- [x] **NI-1**: `withEngramEnv` nested call test — added `TestWithEngramEnv_NestedCallsRestoreCorrectly`
- [x] **NI-2**: `.lockcheck` temp file cleanup on startup — `DetectLockedData` now restores `.lockcheck` files before probe
- [x] **NI-3**: `LabelStyle` contrast in renderer — already uses `ColorSubtext` (#908caa), which is the lighter/more readable Rose Pine variant
- [x] **MN-2**: `DimStyle` color choice — `ColorOverlay` (#6e6a86) is intentionally dim for unfocused inputs
- [x] **MN-3**: `router.go` stale `ScreenReview` backward route comment — added note that `goBack()` overrides Review backward routing
- [x] **MN-4**: "Abandoned data" warning when choosing Start Fresh in config mode — `RenderEngramDataDir` now shows warning when `HasExistingData && Choice == StartFresh`
- [x] **MN-5**: `install.sh` `mkdir -p` unchecked return code — all four `mkdir -p` calls now use `|| fatal`

## Task Dependencies

```
1.1 (platform) ──► 3.2 (TUI persistence)
      │
      └──► 4.1 (E2E)

1.2 (env.go normalize) ──► 2.1 (run.go comparison)
      │
      └──► 3.3 (TUI comparison)

1.3 (env.go locked) ──► 4.3 (regression)

2.2 (install.sh) ──► 4.1 (E2E)
2.3 (install.ps1) ──► 4.2 (E2E)

3.1 (app.go validate) ──► 4.3 (regression)
```
