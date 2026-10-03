# Feature: Preserve Codex Capabilities and OpenCode Fast Variants (#2218)

## 1. Diagnosis & Technical Proposal

### 1.1 Root-Cause Diagnosis
Gentle AI's model assignment flow discards or truncates model capabilities across two subsystems:
1. **Codex (`internal/model/codex_model.go` & TUI)**:
   - `CodexEffort` is limited to `low`, `medium`, `high`, `xhigh`. Modern models (`gpt-5.6-sol`, `gpt-5.6-terra`, `gpt-5.6-luna`) advertise `max` and `ultra` reasoning efforts.
   - `codexEffortOptions` in `internal/tui/screens/codex_model_picker.go` hardcodes only the legacy 4 levels, preventing users from selecting `max` or `ultra`.
2. **OpenCode (`internal/opencode/catalog.go`, `config_v2.go` & `internal/tui/model.go`)**:
   - `sortVariants` only recognizes `low: 0, medium: 1, high: 2`. Adding `xhigh`, `max`, `ultra`, or `fast` breaks semantic sorting and falls back to naive lexical sort.
   - `MergeConfiguredCatalog` skips merging configured model variants when the model already exists in the runtime catalog (`!exists` check), discarding custom or speed variants (such as `fast`).
   - `sanitizeKnownModelEfforts` in `internal/tui/model.go` strips assignment effort to `""` if `available.Reasoning` is false or if the variant wasn't preserved in `available.EffortLevels()`.

### 1.2 Technical Proposal
- **Codex Effort Expansion**:
  - Add `CodexEffortMax = "max"` and `CodexEffortUltra = "ultra"` to `internal/model/codex_model.go` and update `Valid()`.
  - Update `codexEffortOptions` in `internal/tui/screens/codex_model_picker.go` to include `max` and `ultra`.
- **OpenCode Variant & Fast Preservation**:
  - Extend `effortRank` in `internal/opencode/catalog.go` to support `low`, `medium`, `high`, `xhigh`, `max`, `ultra` and prioritize/categorize `fast`.
  - Update `MergeConfiguredCatalog` in `internal/opencode/catalog.go` to union/merge `Variants` when a model exists in both runtime and configured catalogs.
  - Update `sanitizeKnownModelEffort` in `internal/tui/model.go` to preserve recognized variants (including `fast`) rather than stripping them blindly.
- **Review Workload Boundary**:
  - All changes stay within ~150-200 lines, well below the 400-line PR gate.

---

## 2. Technical Specification & Contracts

### 2.1 Codex Model Contracts
- In `internal/model/codex_model.go`:
  ```go
  const (
      CodexEffortLow    CodexEffort = "low"
      CodexEffortMedium CodexEffort = "medium"
      CodexEffortHigh   CodexEffort = "high"
      CodexEffortXHigh  CodexEffort = "xhigh"
      CodexEffortMax    CodexEffort = "max"
      CodexEffortUltra  CodexEffort = "ultra"
  )
  ```
  `Valid()` returns `true` for all 6 effort tiers.
- In `internal/tui/screens/codex_model_picker.go`:
  `codexEffortOptions` contains `[low, medium, high, xhigh, max, ultra]`.

### 2.2 OpenCode Catalog & Variant Sorting Contracts
- In `internal/opencode/catalog.go`:
  `effortRank` maps:
  `low: 0, medium: 1, high: 2, xhigh: 3, max: 4, ultra: 5`.
  `sortVariants` sorts effort tiers by rank first, followed by non-effort/speed variants such as `fast` in a stable order.
- In `MergeConfiguredCatalog`:
  If model exists in both `runtimeProvider.Models[modelID]` and `configuredProvider.Models[modelID]`, merge any non-duplicate variants from configured into runtime and re-sort.
- In `internal/tui/model.go`:
  `sanitizeKnownModelEffort` preserves non-empty effort if it is an existing variant in `available.EffortLevels()` or if it matches `fast`.

---

## 3. Tasks & Evidence

- [x] Task 1: Expand Codex effort enum and picker options with unit tests
  - Edit: `internal/model/codex_model.go`, `internal/tui/screens/codex_model_picker.go`
  - Tests: `internal/model/codex_model_test.go`, `internal/tui/screens/codex_model_picker_test.go`
  - Evidence: `TestCodexEffortValid`, `TestCodexModelPickerOptionCount_EffortMode`, `TestCodexCustomEffortSelect_RendersOptions` PASS.

- [x] Task 2: Enhance OpenCode variant sorting, merging, and sanitization for fast/extended variants
  - Edit: `internal/opencode/catalog.go`, `internal/tui/model.go`
  - Tests: `internal/opencode/catalog_test.go`, `internal/tui/model_test.go`
  - Evidence: `TestSortVariants_ExtendedEffortAndSpeedVariants`, `TestMergeConfiguredCatalogMergesAndSortsVariants`, `TestSanitizeKnownModelEfforts_PreservesFastAndAvailableEffortLevels` PASS.

- [x] Task 3: Full package verification and regression check
  - Run: `go test ./internal/model/... ./internal/opencode/... ./internal/tui/...`
  - Verify line count budget (`git diff --stat`: 8 files changed, 147 insertions(+), 17 deletions(-)).
  - Evidence: Clean build, `go vet`, `git diff --check`, and test suite execution PASS.
  - Work-unit commit: `bb0c03ec` (`fix(tui): preserve Codex capabilities and OpenCode fast variants (#2218)`).

---

## 4. Acceptance Criteria & Verification

- [x] `CodexEffort` accepts `max` and `ultra`.
- [x] Codex custom model picker displays `max` and `ultra` as selectable effort options.
- [x] OpenCode `sortVariants` sorts `low`, `medium`, `high`, `xhigh`, `max`, `ultra` in ascending order without reverting to lexical sort on extended levels.
- [x] `MergeConfiguredCatalog` preserves configured variants (like `fast`) even when the base model is present in the runtime catalog.
- [x] `sanitizeKnownModelEfforts` retains `fast` and recognized model variants.
- [x] All existing and new tests pass without regressions.
