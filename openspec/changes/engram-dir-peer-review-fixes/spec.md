# Engram Data Directory — Peer Review Fixes

## Purpose

Fix persistence, path handling, and safety gaps identified during peer review of the "First-class Engram Data Directory Selection & Migration" feature so that user-configured directories survive terminal restarts, updates, and reconfigurations.

---

## 1. profile-persistence

### Requirement: Cross-Platform Profile Persistence

The system MUST provide `PersistEngramEnv(dir string) error` and `RemoveEngramEnv() error` that update the user's persistent shell environment to reflect the chosen Engram data directory.

#### Scenario: Unix bash profile updated

- GIVEN the user is on a Unix system with `$SHELL` set to `/bin/bash`
- WHEN `PersistEngramEnv("/custom/engram")` is called
- THEN `~/.bashrc` (or `~/.bash_profile` if it exists) is updated to contain `export ENGRAM_DATA_DIR="/custom/engram"`
- AND any previous `export ENGRAM_DATA_DIR=` line is removed first

#### Scenario: Unix zsh profile updated

- GIVEN the user is on a Unix system with `$SHELL` set to `/bin/zsh`
- WHEN `PersistEngramEnv("/custom/engram")` is called
- THEN `~/.zshrc` is updated to contain `export ENGRAM_DATA_DIR="/custom/engram"`
- AND any previous `export ENGRAM_DATA_DIR=` line is removed first

#### Scenario: Unix fish profile updated

- GIVEN the user is on a Unix system with `$SHELL` set to `/usr/bin/fish`
- WHEN `PersistEngramEnv("/custom/engram")` is called
- THEN `~/.config/fish/config.fish` is updated to contain `set -gx ENGRAM_DATA_DIR "/custom/engram"`
- AND any previous `set -gx ENGRAM_DATA_DIR` line is removed first

#### Scenario: Windows registry updated

- GIVEN the user is on Windows
- WHEN `PersistEngramEnv("C:\\Users\\tony\\Engram")` is called
- THEN the `ENGRAM_DATA_DIR` user environment variable is set in the Windows registry
- AND any previous `ENGRAM_DATA_DIR` user variable is overwritten

#### Scenario: Removal clears profile entry

- GIVEN `~/.bashrc` contains `export ENGRAM_DATA_DIR="/old/engram"`
- WHEN `RemoveEngramEnv()` is called
- THEN the `export ENGRAM_DATA_DIR=` line is removed from `~/.bashrc`
- AND no empty or broken lines are left behind

#### Scenario: Removal on Windows clears registry

- GIVEN the Windows registry contains `ENGRAM_DATA_DIR="C:\\Old"`
- WHEN `RemoveEngramEnv()` is called
- THEN the `ENGRAM_DATA_DIR` user environment variable is removed from the registry

#### Scenario: Atomic write prevents corruption

- GIVEN the profile file is large
- WHEN `PersistEngramEnv` rewrites it
- THEN the original file is never in a partially-written state
- AND a backup file is created before mutation

---

## 2. tui-reconfiguration-persistence

### Requirement: TUI Reconfiguration Updates Persistent Environment

When the user reconfigures the Engram data directory via the TUI Welcome menu, the system MUST persist the choice to both `state.json` AND the shell profile / Windows registry.

#### Scenario: Custom dir reconfiguration persists to profile

- GIVEN the user opens "Configure Engram directory" from the Welcome menu
- AND the user enters `/mnt/external/engram` and presses Continue
- WHEN the configuration is saved
- THEN `state.json` contains `"engram_data_dir": "/mnt/external/engram"`
- AND the shell profile contains `export ENGRAM_DATA_DIR="/mnt/external/engram"`

#### Scenario: Revert to default removes profile entry

- GIVEN the user previously configured `/mnt/external/engram`
- AND the shell profile contains `export ENGRAM_DATA_DIR="/mnt/external/engram"`
- WHEN the user selects "Keep current location" and presses Continue
- THEN `state.json` contains `"engram_data_dir": ""`
- AND the shell profile NO LONGER contains any `ENGRAM_DATA_DIR` line

#### Scenario: No persistence for default dir

- GIVEN the user configures the default directory (`~/.engram`)
- WHEN the configuration is saved
- THEN `state.json` may contain `"engram_data_dir": "/home/user/.engram"`
- BUT the shell profile MUST NOT be modified

---

## 3. install-script-persistence-cleanup

### Requirement: Install Scripts Only Persist Non-Default Directories

The install scripts (`install.sh`, `install.ps1`) MUST NOT write `ENGRAM_DATA_DIR` to the shell profile or Windows registry when the chosen directory equals the default (`~/.engram` or `%USERPROFILE%\.engram`).

#### Scenario: Default install skips profile

- GIVEN the user runs `install.sh` without `--engram-data-dir`
- AND the default directory is `~/.engram`
- WHEN the script completes
- THEN the shell profile does NOT contain any `ENGRAM_DATA_DIR` line

#### Scenario: Custom install writes to profile

- GIVEN the user runs `install.sh --engram-data-dir /mnt/external/engram`
- WHEN the script completes
- THEN the shell profile contains `export ENGRAM_DATA_DIR="/mnt/external/engram"`

---

## 4. path-normalization

### Requirement: Relative Paths Are Normalized

`DefaultDataDir()` MUST return an absolute path. If `ENGRAM_DATA_DIR` contains a relative path, it MUST be resolved relative to the current working directory at the time of the call.

#### Scenario: Relative env var resolved

- GIVEN `ENGRAM_DATA_DIR=./engram`
- AND the current working directory is `/home/user/projects`
- WHEN `DefaultDataDir()` is called
- THEN it returns `/home/user/projects/engram`

#### Scenario: Absolute env var unchanged

- GIVEN `ENGRAM_DATA_DIR=/absolute/path`
- WHEN `DefaultDataDir()` is called
- THEN it returns `/absolute/path`

#### Scenario: Tilde expansion preserved

- GIVEN `ENGRAM_DATA_DIR=~/engram`
- WHEN `DefaultDataDir()` is called
- THEN it returns `~/engram` (raw) because `ExpandDataDir` handles tilde separately
- NOTE: This is acceptable because `ExpandDataDir` is called on user input, not env var

---

## 5. source-target-comparison

### Requirement: Path Comparison Uses Normalized Forms

Before comparing source and target directories for migration, the system MUST normalize both paths using `filepath.Clean` to prevent false mismatches due to trailing slashes, double slashes, or tilde differences.

#### Scenario: Same path with different string representation

- GIVEN `defaultDir` resolves to `/home/user/.engram`
- AND `selection.EngramDataDir` is `/home/user//.engram/`
- WHEN the pipeline checks `defaultDir != selection.EngramDataDir`
- THEN the comparison returns `false` (paths are equal after `filepath.Clean`)
- AND migration is skipped

#### Scenario: Actually different paths

- GIVEN `defaultDir` is `/home/user/.engram`
- AND `selection.EngramDataDir` is `/mnt/external/engram`
- WHEN the pipeline checks after `filepath.Clean`
- THEN the comparison returns `true`
- AND migration proceeds

---

## 6. persisted-state-validation

### Requirement: Invalid Persisted Paths Are Rejected

When loading persisted state at TUI startup, the system MUST validate `EngramDataDir` before using it. If the path does not exist, is not a directory, or is not writable, the system MUST log a warning and clear the value.

#### Scenario: Valid persisted path loaded

- GIVEN `state.json` contains `"engram_data_dir": "/valid/engram"`
- AND `/valid/engram` exists and is writable
- WHEN `loadPersistedAssignments` runs
- THEN `selection.EngramDataDir` is set to `/valid/engram`

#### Scenario: Non-existent path cleared

- GIVEN `state.json` contains `"engram_data_dir": "/does/not/exist"`
- WHEN `loadPersistedAssignments` runs
- THEN a warning is logged
- AND `selection.EngramDataDir` remains empty
- AND `state.json` is updated to clear the invalid value

#### Scenario: Read-only directory cleared

- GIVEN `state.json` contains `"engram_data_dir": "/read/only/engram"`
- AND the directory exists but is not writable
- WHEN `loadPersistedAssignments` runs
- THEN a warning is logged
- AND `selection.EngramDataDir` remains empty

---

## 7. unix-locked-detection

### Requirement: Unix Locked-File Detection Fallback

`DetectLockedData` on Unix systems MUST attempt to detect whether `engram.db` is open by another process. If `lsof` is available and reports the file is open, the function MUST return `true`.

#### Scenario: File open with lsof available

- GIVEN the system is Unix
- AND `lsof` is installed
- AND another process has `engram.db` open
- WHEN `DetectLockedData` is called
- THEN it returns `true`

#### Scenario: File not open with lsof available

- GIVEN the system is Unix
- AND `lsof` is installed
- AND no process has `engram.db` open
- WHEN `DetectLockedData` is called
- THEN it returns `false`

#### Scenario: lsof not available

- GIVEN the system is Unix
- AND `lsof` is not installed
- WHEN `DetectLockedData` is called
- THEN it returns `false` (best-effort, same as current behavior)

#### Scenario: Windows unchanged

- GIVEN the system is Windows
- WHEN `DetectLockedData` is called
- THEN it uses the existing rename probe mechanism
- AND the Unix `lsof` fallback is not invoked
