# PRD: plugin-sdd — OpenCode Visual Plugin for SDD Profile Management & Engram Viewer

> **Manage SDD model profiles and inspect Engram project memories without leaving OpenCode.**

**Version**: 1.1.0
**Author**: Gentleman Programming
**Date**: 2026-04-09
**Status**: Implemented & manually verified — pending PR merge
**Related issue**: [#270](https://github.com/Gentleman-Programming/gentle-ai/issues/270)
**Related PRD**: [`prd-opencode-profiles.md`](./prd-opencode-profiles.md)

---

## 1. Context & Distinction

This PRD covers the **OpenCode visual plugin** (`plugin-sdd-opencode`) — an in-session TUI plugin that runs **inside OpenCode** and is activated via `alt+k` or `/sdd-model`.

It is **not** the same as the profile management feature described in `prd-opencode-profiles.md`, which covers a dedicated screen inside the `gga` CLI TUI. Both solve complementary problems:

| | `prd-opencode-profiles.md` | **This PRD** |
|---|---|---|
| **Where it runs** | `gga` terminal UI | Inside OpenCode (active session) |
| **How to access** | `gentle-ai` CLI command | `alt+k` or `/sdd-model` |
| **Restart required** | Yes — edits `opencode.json` | **No** — live runtime patching |
| **Engram viewer** | No | Yes |
| **Status badge** | No | Yes |

---

## 2. Problem Statement

OpenCode users working with the SDD workflow face two recurring pain points in every session:

**Profile switching requires manual config editing and a session restart.**
Switching between SDD model configurations — for example, from "premium" (Opus orchestrator + Sonnet sub-agents) to "cheap" (Haiku everywhere) — requires manually editing `~/.config/opencode/opencode.json` and restarting OpenCode. There is no in-session mechanism to switch profiles.

**Engram memory is invisible from within OpenCode.**
Developers using Engram for persistent memory across sessions have no way to view, audit, or clean up memories for a project without leaving OpenCode and using a separate DB browser or CLI tool. Stale or incorrect memories silently affect agent behavior without the developer noticing.

---

## 3. Solution

`plugin-sdd-opencode` is an OpenCode TUI plugin that exposes profile management and Engram memory operations directly from within an active OpenCode session.

It is installed by `gga` via the **"Install OpenCode Plugin"** option in the welcome menu. The installer:

1. Copies the plugin to `~/.config/opencode/plugins/plugin-sdd-opencode/`
2. Registers it in `~/.config/opencode/tui.json`
3. Ensures `~/.config/opencode/package.json` has the required runtime dependencies

Once installed, the plugin registers:
- A keybind (`alt+k`) and slash command (`/sdd-model`) to open the plugin menu
- UI slots in the status bar and sidebar showing the currently active model profile

### Entry point

Activated via `alt+k` or `/sdd-model` from anywhere in OpenCode.

```
┌─────────────────────────────────────────┐
│  SDD Profile Management                  │
│                                          │
│  ▸  Create New SDD Profile               │
│     Manage SDD Profiles                  │
│     View Project Memories                │
│     ✕ Close                              │
└─────────────────────────────────────────┘
```

---

## 4. Features

### 4.1 Profile Creation

The user selects **"Create New SDD Profile"** and enters a name. The plugin reads the current SDD agent model assignments from `opencode.json` (agent entries whose key starts with `sdd-`) and saves them as a named JSON file under `~/.config/opencode/profiles/<name>.json`.

**Validation:**
- Name is trimmed and stripped of the `.json` extension if provided
- If the profile already exists, the user receives an error toast — no silent overwrites
- If no SDD agents with model assignments are found in the current config, the operation is aborted with an informative error

**Profile file format:**
```json
{
  "sdd-orchestrator": "anthropic/claude-opus-4-20250514",
  "sdd-init": "anthropic/claude-sonnet-4-20250514",
  "sdd-explore": "anthropic/claude-sonnet-4-20250514",
  "sdd-apply": "anthropic/claude-sonnet-4-20250514"
}
```

A backup of the current `opencode.json` is written to `opencode.json.bak` before any activation operation.

---

### 4.2 Profile List & Detection

**"Manage SDD Profiles"** shows all `.json` files under `~/.config/opencode/profiles/`. The active profile is automatically detected by comparing the model assignments in each profile file against the current live config (`api.state.config`). The matching profile is highlighted with a `✓` prefix.

```
┌─────────────────────────────────────────┐
│  Select SDD Profile                      │
│                                          │
│  ✓ premium                               │
│    cheap                                 │
│    gemini-experimental                   │
│  ← Back                                  │
└─────────────────────────────────────────┘
```

---

### 4.3 Profile Detail & Per-Agent Editing

Selecting a profile opens its detail view, which lists every SDD agent in that profile alongside its current model assignment. From this view the user can:

- **Activate** the profile
- **Rename** the profile file
- **Delete** the profile file (with confirmation)
- **Reassign the model** for any individual agent — a provider picker and then a model picker (populated from `api.state.provider`) let the user drill down to any available model

```
┌─────────────────────────────────────────┐
│  Profile: cheap                          │
│                                          │
│  Profile                                 │
│  ✏ Name: cheap                           │
│                                          │
│  Agents (Click to edit model)            │
│  sdd-orchestrator   anthropic/claude-haiku-3.5
│  sdd-init           anthropic/claude-haiku-3.5
│  sdd-apply          anthropic/claude-haiku-3.5
│  ...                                     │
│                                          │
│  ──────────────                          │
│  ✓ Activate Profile                      │
│  ✕ Delete Profile                        │
│  ← Back                                  │
└─────────────────────────────────────────┘
```

---

### 4.4 Profile Activation — No Restart Required

Activating a profile patches the global config at runtime using `api.client.global.config.update`. This means model assignments take effect in the **current session** without restarting OpenCode.

The flow:
1. Read the profile's model assignments
2. Fetch the current global config via `api.client.global.config.get`
3. Merge the profile models into the config (only the `agent[name].model` fields are touched — all other agent config is preserved)
4. Push the updated config via `api.client.global.config.update`
5. Persist the result to `opencode.json` on disk
6. Update the in-plugin state so the status badge reflects the new active model immediately

If the API call fails, the user receives an error toast and the disk file is not modified.

---

### 4.5 Engram Memory Viewer

**"View Project Memories"** opens a list of Engram observations scoped to the current project. The project name is resolved in this order:

1. `git remote get-url origin` → repo name (last path segment, lowercase, `.git` stripped)
2. `git rev-parse --show-toplevel` → root directory name
3. `basename` of the current working directory

The plugin reads directly from `~/.engram/engram.db` via the `sqlite3` CLI (timeout: 5 seconds). Only rows where `project = <resolved name>` and `deleted_at IS NULL` are shown, ordered by `updated_at DESC`.

```
┌─────────────────────────────────────────┐
│  Memories: gentle-ai                     │
│                                          │
│  [42] JWT auth middleware                │
│       [DECISION] 09/04/2026 · project   │
│  [38] Fixed N+1 in user list            │
│       [BUGFIX] 08/04/2026 · project     │
│  ← Back                                  │
└─────────────────────────────────────────┘
```

---

### 4.6 Memory Detail & Soft Delete

Selecting a memory opens a detail view that displays:
- Title and topic key
- Type, scope, and date (formatted with `toLocaleString`)
- Full content, word-wrapped at 52 characters per line, with markdown formatting stripped (`**bold**`, `` `code` ``, smart quotes, arrows)

From the detail view the user can **delete** the memory. Deletion is a soft delete: `deleted_at` and `updated_at` are set to `datetime('now')` via a direct SQLite `UPDATE`. The memory ID is validated as a positive integer before execution.

```
┌─────────────────────────────────────────┐
│  Delete Memory                           │
│                                          │
│  Eliminar 'JWT auth middleware'?         │
│                                          │
│  ▸ Confirm                               │
│    Cancel                                │
└─────────────────────────────────────────┘
```

---

### 4.7 Status Badge

The plugin registers UI slots in `home_bottom` and `sidebar_content` that display the active model at all times:

- **Profile active**: `󰚩 claude-sonnet-4 · 200k ctx` (in the theme's primary color, bold)
- **No profile**: `󱚧 No SDD model active` (in muted color)

The badge updates immediately when a profile is activated.

---

## 5. Technical Design

### 5.1 Plugin File Structure

```
plugin-sdd-opencode/
├── index.tsx            # Plugin entry — registers command, slots, reads active profile
├── components.tsx       # ActiveModelBadge UI component
├── package.json         # Plugin manifest — entry point: index.tsx
└── src/
    ├── index.ts         # Barrel exports — re-exports all src modules
    ├── config.ts        # Path resolution (~/.config/opencode/profiles/, opencode.json)
    ├── dialogs.tsx      # All TUI dialog flows
    ├── memories.ts      # SQLite reads and soft-delete via sqlite3 CLI
    ├── profiles.ts      # Profile CRUD — read, write, activate, detect, rename, delete
    ├── state.ts         # Global reactive state (activeProfile)
    ├── types.ts         # Shared types: ActiveProfileState, ProfileModels, EngramObservation
    └── utils.ts         # Formatting helpers, isManagedSddAgent, resolveModelInfo
```

> **Note on `src/index.ts`:** This is a barrel file that re-exports all modules in `src/` so that `index.tsx` (the plugin entry point) can import from a single location. It is required for clean module resolution and is not the plugin entry point itself — that role belongs to the root `index.tsx` declared in `package.json`.

### 5.2 gga Installer File Structure

The installer lives entirely in new files — no existing file was rewritten:

```
internal/
├── components/sdd/
│   └── plugin_sdd_install.go   # InstallSddPlugin — copy, tui.json, package.json
├── tui/
│   ├── plugin_install_test.go  # Navigation test for the new menu item
│   └── screens/
│       └── plugin_sdd_screen.go # RenderPluginInstall — result screen
```

Minimal additions to existing files:

| File | What changed |
|------|--------------|
| `internal/tui/screens/welcome.go` | +1 option `"Install OpenCode Plugin"` at index 6 |
| `internal/tui/model.go` | +`ScreenPluginInstall`, `PluginInstallDoneMsg`, `startPluginInstall`, handlers in `Update`/`View`/`confirmSelection` |
| `internal/tui/router.go` | +1 route `ScreenPluginInstall → ScreenWelcome` |
| `internal/assets/assets.go` | No change needed — `//go:embed all:opencode` already embeds the plugin directory |

### 5.3 Installation Flow (`InstallSddPlugin`)

```
InstallSddPlugin(homeDir)
  │
  ├── 1. copyEmbeddedPlugin
  │       └── os.RemoveAll(destDir)           — clean stale files from previous versions
  │       └── fs.WalkDir(assets.FS, ...)      — copy from embed.FS recursively
  │       └── filemerge.WriteFileAtomic(...)  — atomic write per file
  │
  ├── 2. writeTUIJSON
  │       └── os.ReadFile(tui.json)           — read existing if present
  │       └── json.Unmarshal → map[string]json.RawMessage  — preserve all top-level keys
  │       └── pluginEntryExists(plugins, key) — check string AND tuple-style entries
  │       └── append(plugins, registryKey)    — add only if missing
  │       └── filemerge.WriteFileAtomic(...)  — atomic write
  │
  └── 3. ensureOpencodePackageJSON
          └── os.ReadFile(package.json)       — read existing or start fresh
          └── json.Unmarshal → map[string]json.RawMessage  — preserve all top-level keys
          └── for each required dep:
          │       if missing        → add it
          │       if @opencode-ai/plugin < 1.4.2 → upgrade to minimum
          │       if existing ≥ minimum → leave untouched
          └── filemerge.WriteFileAtomic(...)  — atomic write
```

### 5.4 Key Design Decisions

#### tui.json — preserve everything, add only what's missing

**Why:** OpenCode's `tui.json` supports heterogeneous plugin entries:
- Simple strings: `"./plugins/my-plugin/"`
- Tuple-style with options: `["./plugins/my-plugin/", { "theme": "dark", ... }]`

An early version treated `plugin` as `[]string` and destroyed tuple entries on rewrite.

**Fix:** The file is parsed as `map[string]json.RawMessage` to preserve all top-level keys (`keybinds`, `theme`, etc.). The `plugin` array is decoded as `[]any`, and duplicates are checked against both string entries and the first element of tuple entries.

#### package.json — preserve everything, enforce minimum semver

**Why:** The `~/.config/opencode/package.json` file is owned by the user and may contain `scripts`, `devDependencies`, `name`, and other fields unrelated to the plugin.

**Rules:**
- All top-level fields are preserved via `map[string]json.RawMessage` round-trip
- `devDependencies` is never touched
- `dependencies`:
  - missing → added with the required version
  - existing and `@opencode-ai/plugin < 1.4.2` → upgraded to `1.4.2`
  - existing and `≥ 1.4.2` or non-semver (e.g. `latest`) → left untouched
- `unique-names-generator`: added if missing, never upgraded (patch-range versions are user-managed)

**Semver comparison:** Implemented via `parseSemverPrefix` — strips `^`, `~`, `v`, `>`, `<`, `=` prefixes and compares major.minor.patch numerically. Non-semver values (e.g. `latest`, workspace aliases) return `false` for `isVersionBelowMinimum` and are preserved as-is.

#### Plugin directory — always reinstall clean

**Why:** If the user runs "Install OpenCode Plugin" again after a `gga` update, the plugin may have new or renamed files. Leaving the old directory would result in stale files that could conflict.

**Fix:** `os.RemoveAll(destDir)` is called before copying, guaranteeing a clean state on every install.

#### opencode.json — not touched

The installer does **not** modify `~/.config/opencode/opencode.json`. That file is managed by the existing SDD inject flow and the plugin's own runtime API (`api.client.global.config.update`). Modifying it from the installer would risk clobbering user model assignments.

### 5.5 Key Invariants

| Invariant | Implementation |
|-----------|----------------|
| Only `sdd-*` agents managed by plugin | `isManagedSddAgent(name)` checks `name.startsWith("sdd-")` |
| Profile files isolated | Profiles live in `~/.config/opencode/profiles/`, separate from `opencode.json` |
| Activation never silently fails | API errors surface as error toasts; disk write only after successful API call |
| Memory operations are project-scoped | SQLite query always filters by resolved project name |
| Soft delete is safe | Memory ID validated as positive integer; `UPDATE` only touches `deleted_at` and `updated_at` |
| Backup before any write | `opencode.json.bak` written before profile activation |
| tui.json entries never duplicated | `pluginEntryExists` checks string and tuple-style entries |
| tui.json unrelated fields preserved | Parsed as `map[string]json.RawMessage`, only `plugin` and `$schema` are updated |
| package.json unrelated fields preserved | Parsed as `map[string]json.RawMessage`, only `dependencies` is touched |
| Plugin API version enforced | `@opencode-ai/plugin < 1.4.2` is upgraded; newer versions preserved |
| Plugin dir always clean on reinstall | `os.RemoveAll(destDir)` before copy |
| opencode.json never modified | Installer does not read or write opencode.json |
| Cross-OS compatibility | `os.UserHomeDir()` + `filepath.Join()` — no OS-specific paths |

### 5.6 Plugin Dependencies

| Dependency | Role |
|------------|------|
| `@opencode-ai/plugin` | Plugin API (command, slots, dialog, toast, state, client) |
| `@opentui/solid` | JSX renderer for TUI components |
| `solid-js` | Reactive primitives |
| `sqlite3` CLI | Engram DB access (must be installed on the system) |
| `node:fs`, `node:path`, `node:os` | Profile file I/O |
| `node:child_process` | `sqlite3` and `git` subprocess calls |

**Minimum OpenCode version**: `>=1.3.13`

### 5.7 Paths

| Path | Purpose |
|------|---------|
| `~/.config/opencode/plugins/plugin-sdd-opencode/` | Installed plugin directory |
| `~/.config/opencode/tui.json` | Plugin registry (OpenCode reads this on startup) |
| `~/.config/opencode/package.json` | Runtime Node dependencies for all plugins |
| `~/.config/opencode/opencode.json` | Global config — **not touched by installer** |
| `~/.config/opencode/opencode.json.bak` | Pre-activation backup (written by plugin at runtime) |
| `~/.config/opencode/profiles/*.json` | Named profile files |
| `~/.engram/engram.db` | Engram SQLite database |

XDG: `configRoot` respects `$XDG_CONFIG_HOME` if set.

---

## 6. Integration into gga

The integration is fully implemented. The installer exposes a new **"Install OpenCode Plugin"** option in the `gga` welcome menu (index 6, between "Create your own Agent" and "OpenCode SDD Profiles").

### What the installer does

1. **Copies the plugin** — `internal/assets/opencode/plugins/plugin-sdd-opencode/` (embedded via `//go:embed all:opencode`) is extracted to `~/.config/opencode/plugins/plugin-sdd-opencode/`. If the directory already exists it is removed first (clean reinstall).

2. **Registers in `tui.json`** — adds `"./plugins/plugin-sdd-opencode/"` to the `plugin` array. Existing entries (including complex tuple-style entries) are preserved. The entry is not duplicated if already present.

3. **Ensures `package.json` deps** — adds `@opencode-ai/plugin` and `unique-names-generator` if missing. Upgrades `@opencode-ai/plugin` to `1.4.2` if the installed version is older. All other fields and dependencies are left untouched.

4. **Shows result screen** — success, already-installed, or error, with a reminder to run `bun install` if `package.json` was modified.

### Prerequisites

- `sqlite3` must be available on the system (present by default on macOS; on Linux it may need to be installed)
- `bun` or `npm` must be available to install Node dependencies after the plugin is copied

---

## 7. Scope

### In Scope

- Plugin installation from `gga` welcome menu
- Plugin directory clean reinstall (removes stale files)
- `tui.json` registration with full preservation of existing entries
- `package.json` dependency enforcement with semver minimum check
- Profile creation from current config
- Profile listing with active detection
- Profile activation at runtime (no restart)
- Per-agent model reassignment within a profile
- Profile rename and delete
- Engram memory listing scoped to the current project
- Engram memory detail view
- Engram memory soft delete
- Status badge in OpenCode UI slots (`home_bottom`, `sidebar_content`)

### Out of Scope

- **Other agents** (Claude Code, Cursor, Codex, Windsurf) — the plugin relies on OpenCode's plugin API exclusively
- **Engram memory creation or editing** — read and soft-delete only
- **Cross-machine profile sync** — profiles are local files
- **Full gga TUI screens** for profile management — covered by [`prd-opencode-profiles.md`](./prd-opencode-profiles.md)
- **`opencode.json` modification** — the installer does not touch the global config

---

## 8. Success Criteria

| Criterion | Target | Status |
|-----------|--------|--------|
| Plugin installation from gga menu | ✅ Always | ✅ Verified |
| Clean reinstall removes stale files | ✅ Always | ✅ Verified |
| tui.json preserves existing entries | ✅ String and tuple-style | ✅ Verified |
| tui.json no duplicate plugin entries | ✅ Always | ✅ Verified |
| package.json preserves unrelated fields | ✅ Always | ✅ Verified |
| package.json enforces minimum plugin version | ✅ `>= 1.4.2` | ✅ Verified |
| package.json preserves newer versions | ✅ Always | ✅ Verified |
| opencode.json not modified | ✅ Always | ✅ Verified |
| Profile activation applies without restart | ✅ Always | ✅ Verified |
| Active profile detected correctly on plugin open | ✅ Always | ✅ Verified |
| Memory list scoped to current project only | ✅ No cross-project leakage | ✅ Verified |
| Soft delete does not affect other memories | ✅ ID-validated UPDATE | ✅ Verified |
| No crash if Engram DB does not exist | ✅ `execFileSync` caught, returns `[]` | ✅ Verified |
| No crash if profiles directory is empty | ✅ Warning toast, returns to menu | ✅ Verified |
| Status badge updates after activation | ✅ State updated synchronously | ✅ Verified |
| Cross-OS compatibility | ✅ Linux, macOS, Windows | ✅ Verified |
