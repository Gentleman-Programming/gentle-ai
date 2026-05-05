<!-- ⚠️ READ BEFORE SUBMITTING
  Every PR must be linked to an issue that has the "status:approved" label.
  PRs without a linked approved issue will be automatically rejected by CI.
  See CONTRIBUTING.md for the full contribution workflow.
-->

## 🔗 Linked Issue

Closes #<!-- TODO: Link the approved issue for Engram data directory selection -->

<!-- Replace the # above with the issue number, e.g.: Closes #42 -->

---

## 🏷️ PR Type

- [x] `type:feature` — New feature (non-breaking change that adds functionality)

---

## 📝 Summary

This PR delivers **first-class Engram data directory selection, migration, and persistence** across the TUI, CLI pipeline, install scripts, and shell environment.

Users can now:
- **Choose** where Engram stores its SQLite database during install/reconfiguration
- **Migrate** existing data safely with copy-verify-delete semantics and locked-file detection
- **Persist** the choice across terminal sessions via shell profile (Unix) or Windows registry
- **Revert** to the default directory at any time, with automatic cleanup of persisted env vars

The feature includes production-grade hardening from peer review: transactional migration, path normalization, buffered I/O for multi-GB SQLite files, atomic config writes, cross-platform profile persistence, and comprehensive test coverage.

---

## 📂 Changes

### New Files

| File / Area | What Changed |
|-------------|-------------|
| `internal/components/engram/data_directory.go` | Domain service for Engram data dir actions (preview, execute, migrate, clean, start-fresh) with transactional semantics |
| `internal/components/engram/data_directory_test.go` | Comprehensive unit tests for DataDirService including migration, clean, locked-data detection, and edge cases |
| `internal/components/engram/filesystem_backend.go` | `DataBackend` implementation: file operations, migration with buffered copy, space estimation, locked-file detection |
| `internal/components/engram/filesystem_backend_test.go` | Tests for LocalDataBackend: migrate, clean, expand path, detect locked data, estimate migration |
| `internal/components/engram/env.go` | Environment helpers: `DefaultDataDir`, `ExpandDataDir`, `withEngramEnv` — with path normalization |
| `internal/components/engram/ui_presenter.go` | TUI-presentable strings: confirm titles, feedback titles, warning messages, byte formatting |
| `internal/components/engram/integration_test.go` | End-to-end litmus tests covering migrate, re-migration safety, keep-default, disk-space blocking |
| `internal/components/engram/locations.go` | Cross-platform location suggestion engine with available space (Home, Documents, drives, mounts) |
| `internal/components/engram/locations_test.go` | Tests for suggestion deduplication, platform candidates, space info |
| `internal/platform/profile_env.go` | Cross-platform shell profile / Windows registry persistence for `ENGRAM_DATA_DIR` |
| `internal/platform/profile_env_test.go` | Tests for bash/zsh/fish profile updates and Windows registry ops |
| `internal/storage/format.go` | Human-readable byte formatting (`FormatBytes`) |
| `internal/storage/space.go` + `_unix.go` + `_windows.go` | Cross-platform disk space queries |
| `internal/storage/space_test.go` | Tests for space calculation |
| `internal/tui/screens/engram_data_dir.go` | TUI renderer for Engram data directory selection, confirmation, and feedback screens |
| `internal/tui/screens/engram_data_dir_test.go` | Golden-file and unit tests for all Engram TUI screen renderers |
| `docs/pre-existing-test-failures-windows.md` | Document tracking known flaky tests on Windows |
| `openspec/changes/engram-dir-peer-review-fixes/*` | SDD artifacts: proposal, spec, design, tasks |

### Modified Files

| File / Area | What Changed |
|-------------|-------------|
| `internal/cli/run.go` | Integrates Engram component apply step; `filepath.Clean` before source/target comparison; install script skip-default logic |
| `internal/cli/run_engram_download_test.go` | Added disk-space insufficient test |
| `internal/tui/model.go` | Engram screen state machine: config mode navigation, profile persistence calls, cached disk info, DB size for Complete screen |
| `internal/tui/model_test.go` | State tests for Engram navigation, env-var detection, profile persistence integration |
| `internal/tui/screens/complete.go` | Renders `EngramDataDir` and `EngramDataSize` on install completion screen |
| `internal/tui/screens/complete_test.go` | Tests for size rendering and omission when zero |
| `internal/tui/screens/welcome.go` | Added "Configure Engram directory" menu item |
| `internal/tui/router.go` | Backward routing for Engram screens |
| `internal/tui/styles/styles.go` | Added `DimStyle`, `LabelStyle`, `ColorOverlay`, `ColorSubtext` for Engram UI |
| `internal/app/app.go` | Validates persisted `EngramDataDir` before loading into TUI model; clears invalid paths |
| `internal/app/app_test.go` | Tests for invalid/read-only path clearing |
| `internal/model/selection.go` | Added `EngramDataDir` field |
| `internal/state/state.go` | Added `EngramDataDir` persistence |
| `internal/components/engram/inject.go` | Engram MCP injection for Claude, OpenCode, Cursor, VS Code, Gemini, Antigravity, Qwen, Codex |
| `internal/components/engram/inject_test.go` | Idempotency and path tests for injections |
| `internal/components/engram/download.go` | Download latest Engram binary with platform asset selection |
| `internal/components/skills/inject.go` | Skill injection integration with Engram paths |
| `internal/components/filemerge/toml.go` + `_test.go` | TOML merging for Codex config |
| `internal/update/upgrade/strategy.go` | Atomic rename for Engram binary upgrade |
| `scripts/install.sh` | Added `--engram-data-dir` flag; skip profile persistence when dir equals default |
| `scripts/install.ps1` | Added `-EngramDataDir` parameter; skip registry persistence when dir equals default |
| `internal/backup/compression.go` | Streaming tar archive: `ArchiveEntry.Size` + `io.Copy` for OOM-safe large file backup |
| `internal/backup/compression_test.go` | Tests for streaming archive with size validation |
| `internal/backup/restore.go` | Restore enhancements for granular rollback |
| `internal/backup/snapshot.go` | Snapshot metadata integration |
| `internal/backup/validate.go` + `_test.go` | Backup path validation helpers |
| `internal/cli/backup_metadata_test.go` | Metadata test coverage for backup operations |
| `internal/components/filemerge/writer.go` + `_test.go` | File merge writer enhancements |
| `internal/components/skills/inject_test.go` | Additional skill injection tests |
| `internal/undo/recorder.go` + `_test.go` | Granular file-level undo tracker for transactional config writes |

---

## 🧪 Test Plan

**Scoped/headless tests run locally with sandboxed HOME/APPDATA/TMP/GOCACHE**
```powershell
go test -p 1 -timeout=15m -count=1 ./internal/components/engram ./internal/tui ./internal/tui/screens ./internal/backup ./internal/platform ./internal/storage ./internal/undo ./internal/components/filemerge ./internal/model
go test -p 1 -timeout=12m -count=1 ./internal/cli -run 'TestRunInstallEngram|TestWithEngramEnv|TestRunRestore'
go test -p 1 -timeout=8m -count=1 ./internal/app -run 'TestRunArgsNoCommandLaunchesTUI|TestRunArgsRestoreByIDWithYes|Test.*Engram|Test.*Persisted|Test.*Backup|Test.*Restore'
```

**Build run locally**
```powershell
go build -o .\gentle-ai.exe .\cmd\gentle-ai
.\gentle-ai.exe --version
```

**Results**
- [x] Engram domain/API tests pass
- [x] Headless TUI model and screen tests pass
- [x] Backup/restore tests pass, including rollback paths under sandbox temp roots
- [x] CLI Engram install/env/restore targeted tests pass
- [x] App restore/Engram/persisted-state targeted tests pass
- [x] Windows exe builds successfully
- [ ] Full `go test ./...` locally: attempted, but local Windows run is blocked by out-of-scope environment/test issues (real AppData agent discovery, Windows shell command assumptions, low C: temp space, and Docker/WSL dependencies). CI Linux should run the authoritative full suite.
- [ ] Docker E2E locally: not run because Docker daemon is not available (`docker info` cannot connect to Docker Desktop Linux engine). Pending CI.

**Latest local build**
```text
gentle-ai 1.23.1-0.20260505113948-962d7011e124+dirty
```

---

## 🤖 Automated Checks

| Check | Status | Description |
|-------|--------|-------------|
| Check Issue Reference | ⏳ | PR body contains `Closes/Fixes/Resolves #N` |
| Check Issue Has `status:approved` | ⏳ | Linked issue must have been approved before work began |
| Check PR Has `type:*` Label | ⏳ | Exactly one `type:*` label must be applied |
| Unit Tests | ⏳ | Scoped/headless branch tests pass locally; full `go test ./...` is authoritative in CI |
| E2E Tests | ⏳ | Pending CI; local Docker daemon unavailable |

---

## ✅ Contributor Checklist

- [ ] PR is linked to an issue with `status:approved`
- [ ] I have added the appropriate `type:*` label to this PR
- [x] Scoped/headless tests pass for branch-critical packages
- [ ] E2E tests pass in CI (local Docker daemon unavailable)
- [x] I have updated documentation (`docs/pre-existing-test-failures-windows.md`, `openspec/changes/`)
- [x] My commits follow Conventional Commits format
- [x] My commits do not include `Co-Authored-By` trailers

---

## 💬 Notes for Reviewers

### Branch Naming
> **Note:** The branch is named `gentle-ai-storage` for historical reasons. The actual changes align with `feat/engram-data-directory` semantics. If CI enforces branch naming, this may need a rename or admin override.

### Known Limitations (Out of Scope)
1. **Pipeline rollback cannot reverse Engram migration (B1):** If `ComponentEngram` succeeds but a later Apply step fails, rollback restores configs but the DB stays in the new location. This is by design (SQLite files can be multi-GB) but documented as a gap.
2. **No migration journal for crash recovery (H3):** Interrupted migration leaves partial data in the target. `DetectLockedData` and `CheckWritable` mitigate this but do not provide resume.
3. **Clean temp backup is not user-restorable via UI (B2):** The temp backup created during `ActionClean` is auto-deleted on success. A process crash preserves it, but there's no TUI path to restore.

### Architecture Decisions
- **Buffered I/O:** `copyFileBuffered` uses a 64 KiB buffer to avoid OOM on multi-GB SQLite files.
- **Cached syscalls:** Disk space and location suggestions are cached in `setScreen()`, not in `View()`, to comply with Bubbletea rendering constraints.
- **Profile persistence:** Unix uses atomic rewrite (temp + mv) with `.bak` backup; Windows uses `setx` / `REG DELETE`.
- **Transactional migration:** `MigrateData` copies → verifies → `persister.Write` → `CleanData` source. If config write fails, source is preserved.

---

## 📋 Commit History

```
bffae8f refactor(engram): peer review hardening — deduplicate TUI validation, atomic config writes, CheckWritable backend method, comprehensive tests
f1d7042 refactor(engram): production-grade OSS hardening — constants, error types, domain layering, cleanup semantics, and comprehensive tests
32d6057 fix(engram): detect and warn about partial/interrupted migration
4e1f3ca refactor(engram): transactional migration — copy → persist config → clean source
071d712 fix(engram): address review feedback — io.Copy buffered migration, cached preview, sentinel error matching, and edge-case error handling
1c8f174 refactor(engram): extract data directory into clean architecture with DataBackend interface and DataDirService
699e8eb feat(engram): add data directory selection and migration
e959b3f refactor(tui): reorder Engram data dir options with clearer labels
6ae3313 feat(engram): data directory selection, migration, and profile persistence
```
