# Peer Review: Engram Storage & Migration Flow

> Branch: `gentle-ai-storage`  
> Reviewer: CTO / Architecture  
> Date: 2026-05-02  
> Scope: Engram data directory selection, migration, backup boundaries, rollback safety, and UX

---

## 1. Executive Summary

The `gentle-ai-storage` branch improves migration safety (copy-verify-delete, locked-file detection, target validation) and adds backup metadata (`Size` tracking, rollback cleanup). **However, the Engram data migration remains outside the pipeline's transactional boundary.** If the install/sync fails after Engram succeeds, rollback restores configuration files but leaves the SQLite database in its new location — potentially orphaned.

**Verdict:** The branch is a solid incremental improvement but does not close the architectural gap between "config backup/rollback" and "data migration." Three issues are **Critical** and should block merge.

---

## 2. Intended Flow (Whiteboard)

### 2.1 User Mental Model

```
┌─────────────────────────────────────────────────────────────┐
│  User thinks: "My Engram memories are MY DATA."             │
│                                                             │
│  - Moving them should be safe and reversible.               │
│  - If install fails, I expect EVERYTHING rolled back.       │
│  - "Backup" means my data is protected.                     │
└─────────────────────────────────────────────────────────────┘
```

### 2.2 Intended Transactional Flow

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐     ┌─────────────┐
│   Prepare   │────▶│    Apply     │────▶│   Commit    │────▶│   Success   │
│             │     │              │     │             │     │             │
│ Backup ALL  │     │ Migrate DB   │     │ Delete src  │     │ Celebrate   │
│ affected    │     │ Install bin  │     │ (irrevers.) │     │             │
│ resources   │     │ Inject conf  │     │             │     │             │
└─────────────┘     └──────────────┘     └─────────────┘     └─────────────┘
                             │
                             ▼ (failure at ANY step)
                      ┌──────────────┐
                      │   Rollback   │
                      │              │
                      │ • Restore    │
                      │   configs    │
                      │ • Restore DB │
                      │   to source  │
                      │ • Clean      │
                      │   target     │
                      └──────────────┘
```

### 2.3 Intended Backup Boundary

```
What "Backup" should mean to the user:
┌──────────────────────────────────────────────┐
│  Config files  │  Engram DB  │  State.json   │
│   (included)   │  (included) │  (included)   │
└──────────────────────────────────────────────┘
        ▲                ▲
        │                └─ The actual memories
        └─ Agent configs (MCP, prompts, settings)
```

---

## 3. Actual Flow (What the Code Does Today)

### 3.1 Pipeline Reality

```
Prepare Phase                          Apply Phase
─────────────────────────────────────────────────────────────────────────────
prepareBackupStep                      rollbackRestoreStep  ←──────────────┐
  └─ snapshots CONFIG FILES ONLY         └─ restores config snapshot ──────┘
                                         agentInstallStep
                                         componentApplyStep (Engram)
                                           ├─ MigrateData(src, target)   ←──
                                           │   • copy                    │ NO
                                           │   • verify sizes            │ ROLLBACK
                                           │   • NOT backed up           │ PATH
                                           ├─ Download/install binary    │
                                           └─ engram setup / inject      │
                                         componentApplyStep (Skills)
                                           └─ undo.Recorder  ←───────────────┘
```

### 3.2 The Rollback Truth Table

| Failure Point | Configs Restored? | Engram DB Restored? | Binary Removed? | User State |
|---------------|-------------------|---------------------|-----------------|------------|
| **Prepare fails** | N/A (no snapshot) | N/A | N/A | Nothing changed |
| **Engram step fails** | ✅ Yes (global restore) | ❌ No (was never backed up) | ❌ No | DB may be partially copied to target |
| **Later step fails** | ✅ Yes | ❌ No (already migrated) | ❌ No | DB in new location, configs restored to old |
| **Success** | N/A | N/A | N/A | DB in new location, configs updated |

### 3.3 The Backup Boundary Reality

```
What gentle-ai backups ACTUALLY capture:
┌──────────────────────────────────────────────┐
│  Config files  │  Engram DB  │  State.json   │
│   ✅ included  │  ❌ MISSING │  ✅ included  │
└──────────────────────────────────────────────┘
```

`backupTargets()` in `cli/run.go` → `componentPaths()` → for `ComponentEngram` only adds:
- `adapter.MCPConfigPath(...)` (agent MCP configs)
- `adapter.SettingsPath(...)` (agent settings)
- `adapter.SystemPromptFile(...)` (system prompts)

**The SQLite database is never added to the target list.**

---

## 4. Gap Matrix

| ID | Gap | Severity | Current Behavior | Intended Behavior |
|----|-----|----------|------------------|-------------------|
| **G1** | **Engram DB is outside backup/rollback** | 🔴 Critical | Rollback restores configs but DB stays in new location | Rollback reverses migration or DB was backed up |
| **G2** | **"Clean" deletes data with no backup** | 🔴 Critical | `ActionClean` permanently deletes SQLite files | User sees what will be deleted + gets a chance to back up |
| **G3** | **Migration not reversible by pipeline** | 🔴 Critical | `componentApplyStep.Rollback()` is no-op for Engram | Migration can be undone if later steps fail |
| **G4** | **`engram stats` is broken verification** | 🟡 Medium | Complete screen says "Verify with: engram stats" but it doesn't show size | Complete screen shows DB size and path |
| **G5** | **Existing data detection uses hard default** | 🟡 Medium | `DetectExistingData(backend.HardDefaultDataDir())` ignores `ENGRAM_DATA_DIR` env var | Detects at effective data dir (`DefaultDataDir()`) |
| **G6** | **No migration journal / crash recovery** | 🟡 Medium | Power loss during migration leaves partial target + source; `ErrTargetHasData` blocks retry | Journal tracks phase; resume/rollback on restart |
| **G7** | **Size not visible outside migration flow** | 🟢 Low | User only sees DB size when choosing Migrate/StartFresh | DB size visible on Welcome / Complete screens |

---

## 5. Deep-Dive: The Critical Gaps

### G1: Engram DB Is Outside the Backup Boundary

**The user's mental model:** "gentle-ai makes a backup before changing things."

**The code's reality:** `prepareBackupStep` creates a snapshot of config files. It never sees `~/.engram/engram.db`.

**Why this matters:** If a user migrates their DB to an external drive and then the install fails on `ComponentGGA`, rollback restores their Claude `mcp_config.json` but the DB is still on the external drive. The config now points to the old location (or is ambiguous). The user's next agent session won't find its memories.

**Root cause:** `componentPaths()` for `ComponentEngram` only enumerates agent-facing config paths, not the data directory.

**Fix options:**
1. **Add DB to backup targets** — Simple but wrong. Backups become huge and slow. The backup system is designed for small text configs.
2. **Make migration reversible** — Better. Track that migration happened; on rollback, copy back and clean target.
3. **Separate data migration from component apply** — Best. Move migration to Prepare or a pre-flight step where failure halts everything before config changes.

**Recommendation:** Option 2 for this branch. Add rollback logic to `componentApplyStep` for Engram that reverses `MigrateData`.

### G2: "Clean" Is a Data Destruction Button with No Safety Net

**Flow:**
```
Welcome → Configure Engram directory → ○ Clean/reset data → Continue → Confirm
```

At Confirm, the user sees:  
> "This will permanently delete all Engram data at: /home/user/.engram"  
> "This cannot be undone. All memory will be lost."

**What's missing:**
- No file list (unlike Migrate which shows `engram.db (4.2 GB)`)
- No total size
- No backup offer
- No "Export first" suggestion

**Recommendation:** Before `CleanData`, create a temporary timestamped backup of the SQLite files in `~/.gentle-ai/backups/clean-<timestamp>/`. On success, delete the temp backup after 24h or on next run. On error, the data is preserved.

### G3: Pipeline Rollback Cannot Undo Migration

**Sequence of doom:**
```
Apply Step 1: rollbackRestoreStep     (succeeded, will be rolled back LAST)
Apply Step 2: agent:kimi-prompt-hub   (succeeded)
Apply Step 3: agent:kimi              (succeeded)
Apply Step 4: component:engram        (SUCCEEDED — migration committed)
Apply Step 5: component:context7      (succeeded)
Apply Step 6: component:skills        (FAILED)
```

Rollback runs in reverse:
1. Skills rollback → undo recorder restores skill files ✅
2. Context7 rollback → no-op
3. **Engram rollback → no-op** ❌ (DB stays in new location)
4. Kimi rollback → no-op (agent install has no rollback)
5. Prompt hub rollback → no-op
6. **Global config restore → configs restored to old state** ✅

**Result:** Configs say "use ~/.engram". DB is at `/mnt/external/engram`. Engram binary can't find its data.

**Recommendation:** Store the migration result in `componentApplyStep`. In `Rollback()`, if migration occurred, call `backend.MigrateData(target, src)` to reverse it. Then clear the persisted `EngramDataDir`.

---

## 6. UX Findings

### 6.1 The Complete Screen Lie

```
Engram data directory    /mnt/external/engram
Verify with: engram stats
```

`engram stats` output:
```
Engram Memory Stats
  Sessions:     9
  Observations: 51
  Prompts:      0
  Projects:     ...
  Database:     C:\Users\...\engram.db
```

**Problem:** `engram stats` confirms the binary works and counts rows. It does **not** confirm:
- The migration copied all files
- The DB size is what the user expected
- The DB is accessible at the new location

**Fix:** Show size on Complete screen. Add a post-migration verification step.

### 6.2 The Welcome Screen Blind Spot

A user who has been using Engram for 6 months has a 3 GB database. They open gentle-ai to sync configs. They have no idea their DB is 3 GB. They accidentally click "Clean."

**Fix:** On the Welcome screen, next to "Configure Engram directory," show a subtle indicator: `Engram: 3.2 GB`.

### 6.3 The Partial Migration Warning Is Good, But...

```
"Found data in both locations. Migration to /mnt/external/engram is blocked
until one location is cleaned or chosen explicitly."
```

**Problem:** It tells the user they're blocked but doesn't explain *why* they're in this state or offer a one-click fix.

**Fix:** Add a "Resume interrupted migration" option that verifies both copies and completes the cleanup.

---

## 7. Architectural Concerns

### 7.1 `componentApplyStep` Does Too Much

For Engram, a single step handles:
1. Data migration (cross-device file copy)
2. Binary installation (brew or HTTP download)
3. `engram setup` (external CLI invocation)
4. `engram.Inject` (config injection)

**Concern:** These have different failure modes and different rollback needs. Data migration should be reversible. Binary installation should be idempotent. Config injection is already handled by the global backup.

**Suggestion:** Split Engram into sub-steps or at least track which sub-operation succeeded so rollback can be granular.

### 7.2 `DataDirService.Execute` Is a Shadow Transaction

```go
// DataDirService.Execute (data_directory.go)
copy → verify → persist config → delete source
```

This is a hand-rolled transaction. It's solid for its scope, but it's **invisible** to the pipeline. The pipeline sees "component:engram succeeded" and moves on. It has no idea a multi-file, cross-device, irreversible operation just completed.

**Suggestion:** Either make the pipeline aware of the migration's reversibility, or move the migration to a place where failure doesn't trigger partial rollback of unrelated config changes.

### 7.3 `prepareBackupStep.Rollback()` Is Dead Code

```go
func (s prepareBackupStep) Rollback() error {
    return os.RemoveAll(s.snapshotDir)
}
```

This method exists but is **never called** by the orchestrator because `prepareBackupStep` lives in the Prepare stage, and rollback only processes Apply steps.

**Concern:** This is misleading. Future maintainers might think backups are cleaned up on rollback. They are not.

**Suggestion:** Add a comment explaining this, or remove the method if it causes confusion.

---

## 8. Recommendations (Prioritized)

### Blockers (Must Fix Before Merge)

| # | Task | Files | Effort |
|---|------|-------|--------|
| B1 | **Add Engram migration rollback** — In `componentApplyStep.Rollback()`, if Engram migration occurred, reverse it by calling `backend.MigrateData(target, src)` and clear persisted config. | `internal/cli/run.go` | Medium |
| B2 | **Protect "Clean" with a temp backup** — Before `CleanData`, copy SQLite files to `~/.gentle-ai/backups/clean-<timestamp>/`. Delete temp backup on success. | `internal/components/engram/data_directory.go` | Small |
| B3 | **Show DB size in Clean confirmation** — Reuse `PreviewFileNames` and `TotalBytes` in the Clean confirmation screen, same as Migrate. | `internal/tui/screens/engram_data_dir.go`, `internal/components/engram/ui_presenter.go` | Small |

### High Priority (Should Fix on This Branch)

| # | Task | Files | Effort |
|---|------|-------|--------|
| H1 | **Add `EngramDataSize` to `CompletePayload`** — Compute size with `backend.EstimateMigration(dataDir)` and display it on the Complete screen. | `internal/tui/screens/complete.go`, `internal/tui/model.go` | Small |
| H2 | **Fix existing data detection** — Use `backend.DefaultDataDir()` (respects env var) instead of `HardDefaultDataDir()` in `setScreen(ScreenEngramDataDir)`. | `internal/tui/model.go` | Tiny |
| H3 | **Add migration journal** — Write a JSON file `~/.gentle-ai/.engram-migration-journal` tracking `source`, `target`, `phase`. On startup, detect incomplete migrations and warn/offer cleanup. | `internal/components/engram/` | Medium |

### Medium Priority (Can Follow Up)

| # | Task | Files | Effort |
|---|------|-------|--------|
| M1 | **Show Engram size on Welcome screen** — Subtle indicator next to "Configure Engram directory." | `internal/tui/screens/welcome.go`, `internal/tui/model.go` | Small |
| M2 | **Clarify `prepareBackupStep.Rollback()`** — Add comment that this is dead code by design, or refactor to make intent explicit. | `internal/cli/run.go` | Tiny |
| M3 | **Add post-migration verification** — After `MigrateData`, verify the new binary can read the DB (`engram stats` or direct SQLite probe). | `internal/cli/run.go` | Medium |

---

## 9. Decision Log

| Date | Decision | Rationale |
|------|----------|-----------|
| 2026-05-02 | **Do NOT add Engram DB to config backups** | Config backups are optimized for small text files. Adding multi-GB SQLite DBs would break retention, deduplication, and performance. Data safety should be achieved via reversible migration, not backup bloat. |
| 2026-05-02 | **Make migration reversible instead of backing up DB** | Reversing a copy is cheaper than backing up a large DB. The migration is already copy-based; reverse copy uses the same proven logic. |
| 2026-05-02 | **Keep "Clean" inside the TUI but add temp backup** | Moving "Clean" to a separate CLI command would reduce accidental destruction but adds UI complexity. A temp backup + clear file list is the pragmatic middle ground. |
| 2026-05-02 | **Use migration journal instead of full resume logic** | Full resume (detect partial target, verify checksums, complete delete) is complex. A journal that warns the user and offers "clean up partial migration" is sufficient for v1. |

---

## 10. Sign-Off

| Role | Name | Status |
|------|------|--------|
| Architecture Review | CTO | **Changes requested** — Blockers B1-B3 must be addressed. |
| UX Review | Product | **Changes requested** — H1-H3 strongly recommended. |
| Security Review | — | **G2 (Clean without backup)** is a data-loss risk. |

**Next step:** Implement B1-B3 and H1-H3, then re-review.
