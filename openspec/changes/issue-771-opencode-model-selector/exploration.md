## Exploration: issue-771-opencode-model-selector

### Current State
- Working tree is on `feat/opencode-model-selector-771`, currently at the same commit as `main`, `origin/main`, and `upstream/main` (`89835570 fix(windows): bring the full suite to parity (#3891)`). The old local branch `fix/custom-opencode-model-selector` was not read or used as source of truth.
- Issue #771 is open and approved with labels `status:approved` and `type:bug`: <https://github.com/Gentleman-Programming/gentle-ai/issues/771>. The bug report says OpenCode custom providers/models declared in `opencode.jsonc` are not detected in Configure Models and Gentle AI writes/reads `opencode.json` while the user uses `opencode.jsonc`.
- PR #1280 is closed/stale and must not be resurrected as-is: <https://github.com/Gentleman-Programming/gentle-ai/pull/1280>. Its useful evidence is the intended #771 solution shape, not its branch bytes.
- Maintainer constraint from <https://github.com/Gentleman-Programming/gentle-ai/pull/1280#issuecomment-5222263352>: `internal/assets/opencode/plugins/review-result-artifacts.ts` and `internal/assets/review_plugin_recovery_test.go` must be dropped from this work because they are not part of #771, current `main` rewrote that plugin/recovery area, and carrying them would conflict or reintroduce removed symbols (`verifiedLensContext`, `parseBinding`, admission-recovery machinery). The same comment says to merge/rebase current `main`, and explicitly preserves the feature direction: carrier consolidation is right, and the presence-aware read that prevents stale state from overwriting a cleared OpenCode assignment is correct.
- Maintainer constraint from <https://github.com/Gentleman-Programming/gentle-ai/pull/1280#issuecomment-5384206922>: stale carriers were closed; #771 remains open for a clean implementation against current `main`; the accepted substance to carry forward is consolidating #771, preserving current model assignments, and carrying LM Studio URL behavior through the implementation.
- Current `internal/opencode/catalog.go` discovers OpenCode's effective provider catalog by running `opencode models --verbose` in a project directory and parsing providers/models. This already aligns with effective runtime catalog discovery and should remain the preferred model-selector data path.
- Current `internal/opencode/models.go` still has `DefaultSettingsPath()` hard-coded to `$HOME/.config/opencode/opencode.json` and `Provider` lacks URL/options metadata. This is the likely root mismatch for file-backed config handling and LM Studio configured URL preservation.
- Current `internal/tui/model.go` wires `modelPickerSettingsPath = opencode.DefaultSettingsPath`, `modelPickerCatalogDiscoverer = opencode.DiscoverCatalog`, and `readCurrentAssignmentsFn = sdd.ReadCurrentModelAssignments`. Model configuration therefore depends on both runtime catalog discovery and file-backed assignment reads.
- Current `internal/tui/screens/model_picker.go` can consume a runtime-discovered provider catalog through `RuntimeCatalogDiscoverer`, refresh runtime models, render provider/model selection, clear assignments, and preserve matching effort when assigning models. The selector plumbing exists; the risk is which providers/models reach it.
- Current `internal/components/sdd/read_assignments.go` reads assignments from one `settingsPath`, silently ignores missing/malformed files, extracts `agent.*.model`, maps legacy orchestrator naming, and supports configurable plus custom agents. It does not appear to read an effective merged OpenCode config or distinguish present-empty assignments from absent/stale state.
- Current `internal/cli/sync.go` is an affected integration point because sync restores/preserves persisted model assignments and may overwrite current OpenCode state if presence is not represented correctly.
- Related authority state: #1279 is a closed duplicate; #946 and #1659 are closed unmerged prior carriers; #1917 is merged LM Studio discovery behavior and must be preserved; #934/#2288 and #1015 are separate scopes and must not be absorbed.

### Affected Areas
- `internal/opencode/models.go` — owns default OpenCode config path and provider/model structs; likely needs config-file discovery support beyond `opencode.json` and provider URL/options metadata if file-backed providers are merged.
- `internal/opencode/catalog.go` — current runtime catalog discovery via `opencode models --verbose`; should be preserved as the selector's runtime source, especially for LM Studio dynamic discovery.
- `internal/tui/screens/model_picker.go` — model selector uses provider catalogs and assignment logic; should receive custom providers/models without regressing runtime catalog behavior.
- `internal/tui/model.go` — wires settings path, runtime catalog discoverer, and current assignment reader into the Configure Models flow.
- `internal/components/sdd/read_assignments.go` — reads current OpenCode assignments; needs presence-aware effective-config behavior so stale Gentle AI state cannot overwrite explicitly cleared/current assignments.
- `internal/components/sdd/inject.go` — OpenCode installation/injection path likely decides which OpenCode config file is written; #771 explicitly expects `opencode.jsonc` to be honored when it exists.
- `internal/cli/sync.go` — sync preservation must prefer current OpenCode assignments over stale persisted Gentle AI state and must preserve explicitly cleared assignments.
- `internal/state/state.go` and `internal/model/model_assignment.go` — state representation for model assignments and reasoning effort may need no schema change, but they are adjacent to stale-state preservation.
- Tests likely affected: `internal/opencode/*_test.go`, `internal/tui/screens/model_picker_test.go`, `internal/components/sdd/read_assignments_test.go`, `internal/components/sdd/inject_test.go`, `internal/cli/sync_test.go`, and targeted TUI model tests.
- Explicitly out of scope: `internal/assets/opencode/plugins/review-result-artifacts.ts`, `internal/assets/review_plugin_recovery_test.go`, #934/#2288 tool-call warning UX, and #1015 SQLite/runtime discovery.

### Approaches
1. **Effective OpenCode config boundary plus runtime catalog merge** — Add a small OpenCode config discovery/parse boundary that finds the effective supported config file(s), tolerates JSONC, reads providers/models/agent assignments with presence information, and merges that data with the existing `opencode models --verbose` runtime catalog.
   - Pros: Directly matches #771 and maintainer comments; preserves #1917 runtime LM Studio discovery; creates one boundary for JSON/JSONC and config precedence; supports assignment preservation cleanly.
   - Cons: Requires careful tests around precedence, missing files, malformed JSONC, direct `url` vs `options.baseURL`, and present-empty assignments.
   - Effort: Medium.

2. **Selector-only custom provider injection** — Teach only the model picker to read `opencode.jsonc` and append custom providers/models to the selector list.
   - Pros: Smallest apparent selector diff.
   - Cons: Does not solve install/sync assignment preservation, risks duplicate config parsing, and fails the maintainer-approved stale-cleared assignment invariant.
   - Effort: Low initially, high risk later.

3. **Runtime-only OpenCode catalog reliance** — Rely exclusively on `opencode models --verbose` and avoid direct config parsing except for assignments.
   - Pros: Minimal local model parsing and good alignment with OpenCode's effective runtime behavior.
   - Cons: Does not address the reported `opencode.jsonc` install/write path, may not represent current assignment presence, and may not expose configured provider URL metadata needed for LM Studio fallback behavior.
   - Effort: Low/Medium but incomplete for #771.

### Recommendation
Use Approach 1. Keep the implementation against current `main`; do not resurrect PR #1280 branch bytes. The proposal/design should define an effective OpenCode config boundary with JSONC support, deterministic precedence, custom provider/model availability in Configure Models, presence-aware assignment preservation, stale-cleared assignment behavior, and explicit preservation of merged LM Studio URL behavior from #1917 (`url` takes precedence, `options.baseURL` is fallback when direct `url` is absent).

### RDD Notes
- `rdd_mode`: enabled/requested.
- `issue_pr`: Issue #771 is the approved authority; PR #1280 is closed stale evidence only, not an implementation source.
- `causal_invariant`: Gentle AI must honor the same effective OpenCode configuration surface for model selection, installation/sync, and assignment preservation, without letting stale Gentle AI state overwrite current or explicitly cleared OpenCode assignments.
- `operator_flows`: Configure Models with `opencode.jsonc`; install/inject when `opencode.jsonc` exists; sync with current assignments present; sync after assignment was explicitly cleared; LM Studio discovery with configured URL and fallback behavior.
- `journey_runtime_evidence`: Exploration only; implementation should add behavior-first Go tests and, if bench can represent Configure Models, one black-box journey for the selector flow.
- `changed_line_budget`: delivery strategy is `auto-chain`, review budget is 400 changed lines; current exploration predicts medium line-budget risk because this spans config parsing, TUI selector, sync, and tests.
- `rollback`: revert the focused #771 changes in OpenCode config/model-selector/sync paths; no plugin/recovery files should be touched.
- `unresolved_authority_decisions`: none for exploration; maintainer comments already narrow scope.

### Risks
- Accidentally importing stale PR #1280 branch code would reintroduce conflict/noise; use GitHub/memory/current-main evidence only.
- Treating `opencode.jsonc` as just another file path without defining effective precedence can create inconsistent selector/install/sync behavior.
- Preserving assignments without presence semantics can overwrite an explicit user clear with stale Gentle AI state.
- LM Studio behavior from #1917 can regress if configured provider URLs are dropped or if `options.baseURL` is ignored.
- Touching review plugin/recovery files would violate maintainer authority and expand the rollback boundary.

### Ready for Proposal
Yes. The next phase should create a proposal scoped to #771 only, with a clean-current-main implementation plan and explicit exclusions for stale PR #1280 plugin/recovery files plus unrelated issues #934/#2288 and #1015.
