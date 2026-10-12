# Task Specification: Safe Windows Transport Plans & Environment Sanitization for Pi (#3571)

## Context & Problem Statement

Issue #3571 addresses safe Windows command execution transport plans and environment variable sanitization for the Pi agent and its related commands in `gentle-ai`.

### Technical Background & Hazards on Windows
1. **Batch Shim Execution & Space-in-Path Splitting (`cmd.exe`)**:
   - On Windows, Node.js global binaries (e.g. `pi`, `npm`, `codegraph`) are installed as `.cmd` or `.bat` batch files.
   - When executed via `exec.Command` with a binary path containing spaces (e.g., `C:\Program Files\nodejs\pi.cmd`), the Windows kernel delegates to `cmd.exe /c "<commandLine>"`.
   - `cmd.exe` strips the outer double quotes and splits arguments on unquoted spaces. Without proper Windows batch command line escaping (`""C:\Program Files\nodejs\pi.cmd" arg1 arg2"`), execution fails with `"C:\Program" is not recognized as an internal or external command`.
2. **Stripped Environment Hazards**:
   - Stripping environment variables down to `HOME` and `PATH` on Windows causes Node.js and Win32 runtimes to crash immediately because essential system variables (`SystemRoot`, `ComSpec`, `PATHEXT`, `TEMP`, `USERPROFILE`, `APPDATA`) are missing.
   - Windows environment variables are case-insensitive (`Path` vs `PATH`), requiring case-insensitive deduplication and normalization to avoid duplicate or conflicting keys in the environment block.
   - Cross-platform tools often expect `HOME`, while Windows native tools expect `USERPROFILE` or `APPDATA`.
   - Arbitrary node options (`NODE_OPTIONS`, `NODE_PATH`) or dangerous git hooks/environment flags can lead to injection or unexpected behavior if not sanitized.
3. **No Windows Job Objects in this Phase**:
   - As explicitly constrained in the task directive, Windows Job Objects (`CreateJobObject`, `AssignProcessToJobObject`) are **strictly excluded** from this phase to focus on safe command execution planning, batch command line formatting, process creation flags, and environment sanitization.

---

## Technical Specification & Design

### 1. `internal/system` Process Configuration & Batch Command Line
Promote and generalize batch command line quoting into `internal/system` so that all agent and tool execution paths can construct safe Windows processes:
- `IsWindowsBatchFile(path string) bool`
- `WindowsBatchCommandLine(binary string, args []string) string`
- `QuoteWindowsArgument(arg string) string`
- `ConfigureCommandProcess(cmd *exec.Cmd, binary string, args []string)`:
  - Ensures valid working directory via `EnsureCommandDir(cmd)`.
  - On Windows: Sets `SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW` (prevents console flashing).
  - On Windows: If `binary` is a `.bat` or `.cmd` file, populates `SysProcAttr.CmdLine` with `WindowsBatchCommandLine(binary, args)`.
  - On non-Windows: Safe no-op process configuration that ensures command directory.

### 2. `internal/system` Environment Sanitization
Implement `SanitizeCommandEnvironment(base []string, passThroughKeys []string, overrides map[string]string) []string`:
- Performs case-insensitive (`strings.ToUpper`) key normalization and deduplication.
- Retains Windows essential variables automatically on Windows:
  `COMSPEC`, `PATH`, `PATHEXT`, `SYSTEMDRIVE`, `SYSTEMROOT`, `TEMP`, `TMP`, `TMPDIR`, `WINDIR`, `USERPROFILE`, `APPDATA`, `LOCALAPPDATA`, `ALLUSERSPROFILE`, `PROGRAMDATA`, `PROGRAMFILES`, `PROGRAMFILES(X86)`.
- Strips hazardous variables:
  `NODE_OPTIONS`, `NODE_PATH`, `GIT_DIR`, `GIT_WORK_TREE`, `GIT_INDEX_FILE`.
- Synchronizes `HOME` and `USERPROFILE` bidirectionally when running on Windows (ensuring both are present if either is available).
- Merges caller-requested `passThroughKeys` and `overrides`.

### 3. Integration Points
- `internal/cli/run.go`: Update `executeCommand` to configure commands using `system.ConfigureCommandProcess` and sanitized environments.
- `internal/components/communitytool/pi_codegraph.go`: Use `system.ConfigureCommandProcess` when probing or launching `codegraph`.
- `internal/reviewerprovider/windows_batch_cmdline.go`: Leverage `internal/system` or maintain backward compatibility with reviewer provider.
- `scripts/crosslane/hostpi.go`: Ensure `piReviewEnvironment` preserves Windows essential variables when running on Windows.

---

## Tasks & Plan

- [ ] Task 1: Add unit tests for Windows batch command line escaping, detection, and process configuration in `internal/system`.
- [ ] Task 2: Implement `cmdline_windows.go`, `cmdline_posix.go`, and batch quoting in `internal/system`.
- [ ] Task 3: Add unit tests for case-insensitive environment sanitization and Windows essential variable retention in `internal/system`.
- [ ] Task 4: Implement `SanitizeCommandEnvironment` in `internal/system/env.go`.
- [ ] Task 5: Integrate `ConfigureCommandProcess` and `SanitizeCommandEnvironment` into Pi execution sites (`internal/cli/run.go`, `internal/components/communitytool/pi_codegraph.go`, `scripts/crosslane/hostpi.go`).
- [ ] Task 6: Verify full test suite, linting, and commit atomic work units.
