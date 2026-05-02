# Self Peer Review: Production Readiness

> Reviewer: Self / CTO hat  
> Standard: OSS production code — no hacks, no jr-dev glue, architecture-aligned

---

## 🔴 Critical: Syscalls in Bubbletea View() Path

**Rule:** Bubbletea's `View()` runs on the render thread (up to 60×/sec). It must be pure, side-effect-free, and **never** do I/O.

**Violations:**

| Location | Syscall | Frequency |
|----------|---------|-----------|
| `model.go:768` | `backend.AvailableSpace()` | Every frame while on `ScreenEngramDataDir` |
| `model.go:773` | `engram.SuggestedLocations()` → `os.Stat`/`os.ReadDir` | Every frame while path input visible |
| `model.go:806` | `backend.EstimateMigration()` → `os.Stat` | Every frame while on `ScreenComplete` |

**Impact:**
- Frame drops on every keystroke
- If the user has a network-mounted home dir, the TUI hangs
- Battery drain from unnecessary disk polling

**Fix:** Move all I/O to `setScreen()` or `Update()` (the async/msg thread), cache results in model state.

---

## 🔴 Critical: OOM Risk in Clean Temp Backup

**Rule:** Never `os.ReadFile` a user database. SQLite files can be 10+ GB.

**Violation:** `data_directory.go:271` — `os.ReadFile(srcPath)` loads the entire DB into memory.

**Existing pattern to follow:** `filesystem_backend.go:327` `copyFileBuffered` uses a 64 KiB buffer specifically to avoid OOM on large SQLite files.

**Fix:** Replace `os.ReadFile`/`os.WriteFile` with `copyFileBuffered` or extract a shared `copyFile` helper.

---

## 🟡 Medium: Misleading UX in First-Time Flow

**Issue:** The heading "Engram will store your memories at: ~/.engram" is shown even when the user has selected **"Choose a different location"**. The heading promises one path while the user's intent is to change it.

**Fix:** Only show the heading when `Choice == EngramChoiceDefault`.

---

## 🟡 Medium: Double Syscalls in Location Suggestions

**Issue:** `locations.go:46-47` calls `backend.AvailableSpace(path)` **twice** per candidate — once for `Space`, once for `SpaceErr`.

For 8 candidates = 16 syscalls instead of 8.

**Fix:** Call once, capture both return values.

---

## 🟡 Medium: Unbounded Suggestion List

**Issue:** `platformCandidates` has no upper bound. A Linux CI server with 50+ mount points will produce an unusable 50-line UI.

**Fix:** Cap at 8 suggestions. Prioritize: home > documents > external drives > mounts.

---

## 🟢 Low: Silent Backup Failure

**Issue:** `Execute(ActionClean)` silently ignores `backupBeforeClean` errors:
```go
backupDir, _ := s.backupBeforeClean(src)
```

If the temp dir is full and backup fails, Clean proceeds with zero safety net and the user is never informed.

**Fix:** Log the error (the project uses `log.Printf` for non-fatal warnings) so support can diagnose.

---

## Action Plan

| # | Issue | File | Effort |
|---|-------|------|--------|
| 1 | Move `AvailableSpace` out of View | `model.go` | Small |
| 2 | Move `SuggestedLocations` out of View | `model.go` | Small |
| 3 | Move `EstimateMigration` out of View | `model.go` | Small |
| 4 | Buffered copy for temp backup | `data_directory.go` | Small |
| 5 | Conditional first-time heading | `engram_data_dir.go` | Tiny |
| 6 | Single syscall per suggestion | `locations.go` | Tiny |
| 7 | Cap suggestion count | `locations.go` | Tiny |
| 8 | Log backup failures | `data_directory.go` | Tiny |
