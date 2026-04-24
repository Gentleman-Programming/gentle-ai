# Pre-existing Test Failures on Windows

> **Context:** These failures were discovered while running the full test suite on Windows 11 + PowerShell during development of the Engram Data Directory feature. They are **not caused by any Engram-related changes** — they reproduce on the base branch (`gentle-ai-storage` before the Engram commits).
>
> **Environment:** Windows 11, PowerShell 7, Go 1.24.2, no WSL, no `bun` in PATH.

---

## 1. `internal/app` — `ListBackups*` tests scan real home directory

### Affected tests
- `TestListBackupsNewestFirst`
- `TestListBackupsWithSourceMetadata`
- `TestListBackupsFallsBackGracefullyForOldManifests`
- `TestRunArgsRestoreByIDWithYes`
- `TestRunArgsRestoreListIsDispatched`

### Error log
```
=== RUN   TestListBackupsNewestFirst
    app_test.go:57: ListBackups() returned 5 manifests, want 2
--- FAIL: TestListBackupsNewestFirst (0.01s)

=== RUN   TestListBackupsWithSourceMetadata
    app_test.go:99: ListBackups() returned 5 manifests, want 1
--- FAIL: TestListBackupsWithSourceMetadata (0.01s)

=== RUN   TestRunArgsRestoreByIDWithYes
    app_test.go:178: RunArgs(restore test-backup-001 --yes) error = backup "test-backup-001" not found
```

### Root cause
`os.UserHomeDir()` on Windows checks `USERPROFILE` **first**, then `HOMEDRIVE\HOMEPATH`, and only then `HOME`. The tests only set `HOME`, so `ListBackups()` reads from the real `C:\Users\<username>\` instead of the temp dir created by the test.

### How to reproduce
```powershell
cd G:\Antonio Bonet\gentle-ai
go test ./internal/app/... -run "TestListBackups" -v
```

### Suggested fix
Option A (minimal): set both `HOME` and `USERPROFILE` in test setup.
Option B (architectural): make `ListBackups()` use an overridable `userHomeDirFn` instead of directly calling `os.UserHomeDir()`.

---

## 2. `internal/app` — `TestRunArgsNoCommandLaunchesTUI` hangs indefinitely

### Error log
Test times out (180s+). No explicit failure message — `go test` kills it. Console shows TUI escape sequences rendered into stdout.

### Root cause
The test assumes Bubble Tea returns a `"TTY"` error when there's no terminal. On Windows, Bubble Tea does **not** return an error — it successfully initializes and blocks waiting for stdin input forever.

### How to reproduce
```powershell
go test ./internal/app/... -run "TestRunArgsNoCommandLaunchesTUI" -v
```

### Suggested fix
Skip the test when `!isatty.IsTerminal(os.Stdin.Fd())`. This is actually **more correct** for all platforms (Linux/macOS CI included) because the old string-matching on `"TTY"` was flaky.

---

## 3. `internal/app` — `TestSelfUpdate_UpdateAvailable_CallsUpgradeAndReExec` fails on Windows

### Error log
```
=== RUN   TestSelfUpdate_UpdateAvailable_CallsUpgradeAndReExec
    selfupdate_test.go:190: reExecCalled = 0, want 1
    selfupdate_test.go:202: re-exec env missing GENTLE_AI_SELF_UPDATE_DONE=1
--- FAIL: TestSelfUpdate_UpdateAvailable_CallsUpgradeAndReExec (0.00s)
```

### Root cause
`selfUpdate()` explicitly skips `reExec` on Windows and prints `"Updated to vX.Y.Z — please restart."` instead. The test expects `reExecCalled == 1` regardless of host OS.

### How to reproduce
```powershell
go test ./internal/app/... -run "TestSelfUpdate_UpdateAvailable_CallsUpgradeAndReExec" -v
```

### Suggested fix
Set `goOS = func() string { return "linux" }` in the test so the re-exec path is exercised regardless of the host OS. `swapSelfUpdateDeps` already saves/restores `goOS`.

---

## 4. `internal/app` — `TestRunArgs_UpgradeNoArgs` / `TestRunArgs_UpgradeToolFilter` timeout

### Error log
```
panic: test timed out after 1m0s
    running tests:
        TestRunArgs_UpgradeNoArgs (57s)
```

Stack trace shows `archive/tar.(*Writer).Write` → `compress/gzip.(*Writer).Write` → `backup.Snapshotter.Create` compressing the entire user home directory.

### Root cause
`RunArgs(["upgrade"])` executes a real upgrade which creates a **backup snapshot of the entire home directory** before upgrading. On a dev Windows machine with many GBs of data (`.cursor/extensions`, `.gemini`, `.codex`, etc.), this takes minutes and times out.

### How to reproduce
```powershell
go test ./internal/app/... -run "TestRunArgs_Upgrade" -v -timeout 60s
```

### Suggested fix
Option A (pragmatic): skip on Windows where home dirs are typically large.
Option B (proper): mock `os.UserHomeDir()` so the backup snapshotter targets a small temp dir instead of the real home.

---

## 5. `internal/cli` — `unique-names-generator` install failures in SDD inject tests

### Affected tests (~15+)
- `TestInjectOpenCodeWritesCommandFiles`
- `TestInjectOpenCodeIsIdempotent`
- `TestInjectOpenCodeMigratesLegacyAgentsKey`
- `TestInjectOpenCodeMultiMode`
- `TestInjectOpenCodeMultiModeIdempotent`
- `TestInjectOpenCodeEmptySDDModeDefaultsSingle`
- `TestInjectOpenCodeSingleToMultiSwitch`
- `TestInjectWritesAllFourSharedFilesToDisk`
- `TestInjectSharedDirCreatedWithAllFiles`
- `TestInjectCopiesAllFilesFromSkillDirectory`
- `TestInjectCopiesAllFilesReportedInResult`
- `TestInjectOpenCodeMultiWritesPlugin`
- `TestRunInstallDryRunMatchesActualInstallOpenCodeSDDMulti`
- `TestOpenCodePersonaBeforeSDDPreservesAllSections`
- `TestComponentSyncStepRunsSDDInject`
- `TestRunSyncAppliesManagedFilesystemChanges`
- `TestRunSyncDoesNotInvokeEngramSetup`
- `TestRunSyncDoesNotInstallBinaries`
- `TestRunSyncPreservesUnmanagedAdjacentFiles`

### Error log
```
inject_test.go:233: Inject() error = post-install check: "unique-names-generator" was not found after install in
"C:\Users\tonyb\AppData\Local\Temp\...\.config\opencode\node_modules\unique-names-generator" —
the background-agents plugin will fail to load.
Fix: run `cd ... && bun add unique-names-generator` (or npm install unique-names-generator) manually
```

### Root cause
These tests call `Inject()` which runs a real `bun add unique-names-generator` (or npm) into a temp `.config/opencode` directory. The test machine does not have `bun` in `PATH`. `npm` may also fail silently in temp paths. The post-install check then fails because the package was never installed.

### How to reproduce
```powershell
go test ./internal/components/sdd/... -run "Inject" -v
```

### Suggested fix
Mock the package manager install step in tests, or skip these tests when `bun`/`npm` are not available. Alternatively, pre-seed `node_modules/unique-names-generator` in the temp dir before calling `Inject()`.

---

## 6. `internal/cli` — `TestRunInstallKimiMissingUVFailsBeforeExecutingInstallCommands` false negative

### Error log
```
=== RUN   TestRunInstallKimiMissingUVFailsBeforeExecutingInstallCommands
    run_integration_test.go:1969: RunInstall() expected error when Kimi uv preflight fails
--- FAIL: TestRunInstallKimiMissingUVFailsBeforeExecutingInstallCommands (0.26s)
```

### Root cause
`checkDependenciesStep` first checks if Kimi is **already installed**. On this machine it is (`C:\Users\tonyb\.local\bin\kimi.exe`). When Kimi is already present, the step **skips the UV preflight entirely**. The test never reaches the "missing UV" error because it short-circuits at "already installed."

### How to reproduce
Run the test on any machine where Kimi is already in PATH.

### Suggested fix
The test needs to mock `kimi.LookPathOverride` to force `installed=false` so the UV preflight actually runs.

---

## 7. `internal/cli` — `TestDiscoverAgents*` returns real system agents

### Affected tests
- `TestDiscoverAgentsReturnsEmptyWhenNoConfigDirsPresent`
- `TestDiscoverAgentsDelegatesCanonicalDiscovery`

### Error log
```
=== RUN   TestDiscoverAgentsReturnsEmptyWhenNoConfigDirsPresent
    sync_test.go:329: DiscoverAgents() expected empty, got [kiro-ide]
--- FAIL: TestDiscoverAgentsReturnsEmptyWhenNoConfigDirsPresent (0.00s)

=== RUN   TestDiscoverAgentsDelegatesCanonicalDiscovery
    sync_test.go:416: DiscoverAgents() returned unexpected agent "kiro-ide" — no other config dirs were created
--- FAIL: TestDiscoverAgentsDelegatesCanonicalDiscovery (0.00s)
```

### Root cause
`DiscoverAgents()` scans the real filesystem (e.g. `~/.config/kiro`) and finds agents that are actually installed on the developer machine (`kiro-ide`). The tests expect an isolated environment but don't mock the filesystem boundary.

### How to reproduce
Run the test on any machine where Kiro (or other agents) are installed.

### Suggested fix
Mock `os.UserHomeDir()` or the config-dir discovery logic so tests run against a temp dir.

---

## 8. `internal/cli` — Engram path guidance tests fail on Windows paths

### Affected tests
- `TestWithPostInstallNotesDoesNotChangeNonGGA`
- `TestEngramPathGuidanceDefault`
- `TestWithGoInstallPathNoteAddsNoteWhenNotInPATH`

### Error log
```
=== RUN   TestWithPostInstallNotesDoesNotChangeNonGGA
    run_notes_test.go:35: FinalNote changed unexpectedly: "...Add /usr/local/bin to your shell PATH..."

=== RUN   TestEngramPathGuidanceDefault
    run_path_guidance_test.go:31: engramPathGuidance(default) missing "go/bin":
        Add C:\Users\tonyb\go\bin to your shell PATH and restart the terminal.

=== RUN   TestWithGoInstallPathNoteAddsNoteWhenNotInPATH
    run_path_guidance_test.go:92: FinalNote should reference go/bin dir, got:
        "...Add C:\Users\tonyb\go\bin to your shell PATH and restart the terminal."
```

### Root cause
These tests hardcode Unix-style paths (`/usr/local/bin`, `go/bin`) or string-match expectations that don't account for Windows backslash paths (`C:\Users\...\go\bin`). The `FinalNote` content changes based on the host OS and `GOPATH`.

### How to reproduce
```powershell
go test ./internal/cli/... -run "TestWithPostInstallNotes|TestEngramPathGuidance|TestWithGoInstallPathNote" -v
```

### Suggested fix
Use `filepath.Join()` and `runtime.GOOS` checks in test assertions, or mock `os.Getenv("GOPATH")` to a known temp path.

---

## 9. `internal/cli` — `TestExecuteCommandQuietModeIncludesCapturedOutputOnFailure` WSL error

### Error log
```
=== RUN   TestExecuteCommandQuietModeIncludesCapturedOutputOnFailure
    command_output_test.go:20: executeCommand() error = "exit status 1\noutput:\nS\x00u\x00b\x00s\x00i\x00s\x00t\x00e\x00m\x00a\x00..."
```

The captured output is a UTF-16LE encoded WSL error message in Spanish:
> "Subsistema de Windows para Linux no tiene distribuciones instaladas..."

### Root cause
The test runs a shell command that invokes WSL (`bash` or similar), but WSL is not installed. The error message is UTF-16LE encoded (Windows console default), which produces the `\x00` null bytes in the string.

### How to reproduce
Run on Windows without WSL installed.

### Suggested fix
Skip the test on Windows when WSL is not available, or mock `executeCommand` to avoid real shell invocation.

---

## 10. `internal/tui` — `TestStartUninstall_FullRemoveHomebrewManagedBinaryAddsManualAction`

### Error log
```
=== RUN   TestStartUninstall_FullRemoveHomebrewManagedBinaryAddsManualAction
    model_test.go:1409: os.Remove should not be called for Homebrew-managed install path
--- FAIL: TestStartUninstall_FullRemoveHomebrewManagedBinaryAddsManualAction (0.00s)
```

### Root cause
The test expects Homebrew-managed binaries to be handled via a "manual action" (telling the user to run `brew uninstall`) rather than calling `os.Remove` directly. But the uninstall flow calls `os.Remove` on the Homebrew path anyway — the Homebrew guard is either missing or incorrectly bypassed.

### How to reproduce
```powershell
go test ./internal/tui/... -run "TestStartUninstall_FullRemoveHomebrewManagedBinaryAddsManualAction" -v
```

### Suggested fix
Review `internal/components/uninstall/` logic to ensure Homebrew paths are detected early and routed to the manual-action path instead of file deletion.

---

## 11. `internal/components/engram` — Path separator mismatch in inject tests

### Affected tests
- `TestInjectVSCodeMergesEngramToMCPConfigFile`
- `TestInjectCodexInjectsTOMLKeys`

### Error log
```
=== RUN   TestInjectCodexInjectsTOMLKeys
    inject_test.go:579: config.toml model_instructions_file does not reference
    "C:\Users\tonyb\...\.codex\engram-instructions.md"; got:
    model_instructions_file = "C:\Users\tonyb\...\.codex/engram-instructions.md"
```

### Root cause
The test expects backslash-separated Windows paths, but the generated TOML contains forward slashes (`/.codex/engram-instructions.md`). This is a `filepath.Join` vs string-concatenation issue in the inject code.

### How to reproduce
```powershell
go test ./internal/components/engram/... -run "TestInjectVSCode|TestInjectCodex" -v
```

### Suggested fix
Ensure `filepath.Join` is used consistently when writing paths to config files, or normalize paths with `filepath.Clean` before comparison in tests.

---

## Summary Table

| # | Package | Test(s) | Category | Severity |
|---|---------|---------|----------|----------|
| 1 | `internal/app` | `TestListBackups*`, `TestRunArgsRestore*` | Env var portability | Medium |
| 2 | `internal/app` | `TestRunArgsNoCommandLaunchesTUI` | Platform behavior | Medium |
| 3 | `internal/app` | `TestSelfUpdate_UpdateAvailable_*` | OS-specific logic | Low |
| 4 | `internal/app` | `TestRunArgs_Upgrade*` | Non-hermetic test | High |
| 5 | `internal/cli`, `internal/components/sdd` | `TestInject*`, `TestRunInstall*`, `TestRunSync*` | Missing dependency (`bun`/`npm`) | High |
| 6 | `internal/cli` | `TestRunInstallKimiMissingUV*` | False negative (already installed) | Medium |
| 7 | `internal/cli` | `TestDiscoverAgents*` | Non-hermetic test | Medium |
| 8 | `internal/cli` | `Test*PathGuidance*`, `Test*PostInstallNotes*` | Hardcoded Unix paths | Low |
| 9 | `internal/cli` | `TestExecuteCommandQuietMode*` | WSL dependency | Low |
| 10 | `internal/tui` | `TestStartUninstall_FullRemoveHomebrew*` | Logic bug | Medium |
| 11 | `internal/components/engram` | `TestInjectVSCode*`, `TestInjectCodex*` | Path separator | Low |
