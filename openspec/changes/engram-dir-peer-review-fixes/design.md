# Design: Engram Data Directory — Peer Review Fixes

## Architecture Decisions

### ADR-1: Extract Profile Persistence into Go (not shell-only)

**Decision:** Move shell profile manipulation from `install.sh` into `internal/platform/profile_env.go` so the TUI can call it directly.

**Rationale:** The TUI reconfiguration flow needs to update the shell profile, but spawning a shell script from Bubble Tea is fragile (cross-platform path issues, shell detection, error handling). A Go package with platform-specific implementations is testable and composable.

**Trade-off:** Duplicates some logic that already exists in `install.sh`. We mitigate by keeping `install.sh`'s implementation as the reference and matching its behavior exactly.

### ADR-2: Only Persist Non-Default Directories

**Decision:** Both install scripts and TUI only write to the profile/registry when the chosen directory differs from the hard default (`~/.engram` / `%USERPROFILE%\.engram`).

**Rationale:** Writing the default to the profile creates unnecessary clutter and sets up the stale-profile bug. If the profile is clean, `DefaultDataDir()` falls back to `~/.engram` naturally.

**Trade-off:** Users who manually inspect their profile won't see an explicit `ENGRAM_DATA_DIR` after default install. This is acceptable — the absence of the variable is equivalent to the default.

### ADR-3: Normalize Paths at Resolution Time

**Decision:** `DefaultDataDir()` calls `filepath.Abs` on env var values. Equality checks in migration logic call `filepath.Clean` before comparison.

**Rationale:** Prevents CWD-dependent behavior for relative paths and false mismatches from trailing slashes.

**Trade-off:** `filepath.Abs` uses the process CWD. If gentle-ai is launched from an unusual directory, a relative env var resolves unexpectedly. This is acceptable because relative env vars are user error — we normalize defensively.

### ADR-4: Validate on Load, Not on Save

**Decision:** Invalid persisted paths are detected and cleared at TUI startup (`loadPersistedAssignments`), not at save time.

**Rationale:** The path might become invalid between save and load (e.g., external drive disconnected). Validating at load catches this. Validating at save would miss post-save invalidation.

**Trade-off:** The user sees a warning at startup rather than at the moment of configuration. This is acceptable because the root cause is external to gentle-ai.

### ADR-5: Best-Effort Unix Locked-File Detection

**Decision:** On Unix, try `lsof`. If unavailable, return `false`.

**Rationale:** True cross-process file locking on Unix is complex (varies by filesystem, no standard API). `lsof` is widely available but not universal. A best-effort check is better than nothing.

**Trade-off:** Migration might still corrupt an actively-written database on systems without `lsof`. We document this limitation and recommend closing agents before migration.

---

## Sequence Diagram: TUI Reconfiguration with Profile Persistence

```
User
  │
  │ Select "Configure Engram directory"
  ▼
TUI ──► setScreen(ScreenEngramDataDir)
  │
  │ Enter "/mnt/external/engram", press Continue
  ▼
TUI ──► Validate path (exists, writable)
  │
  │ Valid
  ▼
TUI ──► state.Write(homeDir, {EngramDataDir: "/mnt/external/engram"})
  │
  │ Success
  ▼
TUI ──► platform.PersistEngramEnv("/mnt/external/engram")
  │
  │ Unix: detect $SHELL → find profile → grep -v old → append new → atomic mv
  │ Win:  setx ENGRAM_DATA_DIR "C:\..."
  ▼
TUI ──► os.Setenv("ENGRAM_DATA_DIR", "/mnt/external/engram")
  │
  ▼
User sees Welcome screen

[Later: new terminal session]
Shell ──► source ~/.bashrc → ENGRAM_DATA_DIR="/mnt/external/engram"
  │
  ▼
gentle-ai ──► os.Getenv → "/mnt/external/engram" (already set)
  │
  ▼
Engram binary ──► reads ENGRAM_DATA_DIR → uses /mnt/external/engram
```

---

## Component Diagram

```
┌─────────────────────────────────────────────┐
│          internal/tui/model.go              │
│  EngramConfigMode confirmation handler      │
│    ├── state.Write(EngramDataDir)           │
│    ├── platform.PersistEngramEnv(dir)       │
│    └── os.Setenv(ENGRAM_DATA_DIR, dir)      │
└─────────────────────────────────────────────┘
                      │
          ┌───────────┴───────────┐
          ▼                       ▼
┌─────────────────────┐   ┌─────────────────────┐
│ internal/state      │   │ internal/platform   │
│ state.json I/O      │   │ profile_env.go      │
└─────────────────────┘   │                     │
                          │  Unix:              │
                          │    detectShell()    │
                          │    rewriteProfile() │
                          │  Windows:           │
                          │    setx / registry  │
                          └─────────────────────┘
```

---

## Interface Contracts

### ProfileEnv Interface

```go
package platform

// PersistEngramEnv writes ENGRAM_DATA_DIR to the user's persistent shell
// environment (Unix profile or Windows registry). If the directory equals
// the default (~/.engram), the function removes any existing entry instead.
func PersistEngramEnv(dir string) error

// RemoveEngramEnv removes ENGRAM_DATA_DIR from the user's persistent shell
// environment.
func RemoveEngramEnv() error
```

### Modified Functions

```go
// internal/components/engram/env.go
func DefaultDataDir() string
// NOW: calls filepath.Abs on env var before returning

func DetectLockedData(dir string) (bool, error)
// NOW: on Unix, tries lsof before returning false

// internal/cli/run.go
// BEFORE: if defaultDir != s.selection.EngramDataDir
// AFTER:  if filepath.Clean(defaultDir) != filepath.Clean(s.selection.EngramDataDir)

// internal/app/app.go
func loadPersistedAssignments(homeDir string, selection *model.Selection)
// NOW: validates EngramDataDir path before populating selection
```

---

## Data Flow: Profile Update (Unix)

```
1. Detect shell from $SHELL env var
   ├── bash → ~/.bashrc (or ~/.bash_profile if exists)
   ├── zsh  → ~/.zshrc
   ├── fish → ~/.config/fish/config.fish
   └── default → ~/.bashrc

2. If profile does not exist, create it

3. Read profile contents

4. Filter out lines matching:
   - ^export ENGRAM_DATA_DIR=
   - ^set -gx ENGRAM_DATA_DIR

5. If dir != default (~/.engram):
   - Append comment + export line

6. Write to temp file (mktemp)

7. Atomic rename: mv temp → profile

8. Return error if any step fails
```

## Data Flow: Profile Update (Windows)

```
1. Call os.Setenv("ENGRAM_DATA_DIR", dir) for current process

2. Execute: cmd /c setx ENGRAM_DATA_DIR "dir"
   - setx updates the user registry hive
   - New processes inherit the value automatically

3. If dir == default (%USERPROFILE%\.engram):
   - Execute: cmd /c REG DELETE HKCU\Environment /V ENGRAM_DATA_DIR /F
   - Removes the registry entry entirely

4. Return error if command fails
```
