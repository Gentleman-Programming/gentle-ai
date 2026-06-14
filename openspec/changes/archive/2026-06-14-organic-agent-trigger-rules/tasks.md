# Tasks: Organic Agent Trigger Rules

> STRICT TDD IS ACTIVE: every work unit follows RED → GREEN (→ REFACTOR when warranted).
> Test runner: `go test ./...`
> Delivery: single PR, `size:exception` (line-count limit does NOT apply).
> Branch: `feat/organic-agent-trigger-rules`
> Worktree: `/Users/alanbuscaglia/work/gentle-ai-wt-organic-triggers`

---

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~500–700 total |
| 400-line budget risk | Medium (within a single PR by explicit `size:exception` decision) |
| Chained PRs recommended | No — single PR with `size:exception` already decided |
| Delivery strategy | `single-pr` + `size:exception` |
| Chain strategy | N/A |

Decision needed before apply: No — already resolved.
Chained PRs recommended: No.
400-line budget risk: Medium but accepted via `size:exception`.

### Work Unit Summary

| Unit | Layer | Goal | Est. lines | Commit |
|------|-------|------|-----------|--------|
| 1 | Types | TriggerEvent, TriggerMode, TriggerWhen, TriggerBinding, TriggerRuleSet + tests | ~60 | `feat(model): add trigger-rules type model` |
| 2 | Catalog | triggers.go — DefaultTriggerRuleSet, SupportedTriggerEvents, KnownAgents, ValidateTriggerRuleSet + tests | ~200 | `feat(catalog): add trigger-rules default set and validator` |
| 3 | Renderer | triggerrules.go — RenderTriggerRules + golden/table tests | ~150 | `feat(sdd): add trigger-rules renderer` |
| 4 | Injection | inject.go step 1c + Kimi include + inject_test.go per-adapter coverage | ~150 | `feat(sdd): inject trigger-rules section into all agent assets` |
| 5 | Resolve open Qs | Confirm OpenCode/Kilocode placement, scan all Jinja adapters | ~10 | bundled into Unit 4 or standalone fixup |
| 6 | Verification | go build / vet / test clean; organic-invariant audit | 0 new lines | verification only |
| 7 | Docs | User-facing note on default rules + injected section location | ~20 | `docs(trigger-rules): document default rule set and injection` |

---

## Unit 1 — Type Model

**Spec**: A (events catalog), B (binding schema), C (when vocabulary), D (mode semantics)
**Files**: `internal/model/types.go`, `internal/model/types_test.go`

### Phase 1 — Red (failing tests)

- [x] **1.1** `internal/model/types_test.go` — table test: all six `TriggerEvent` constants exist and their string values match the spec (`pre-commit`, `pre-push`, `pre-pr`, `post-sdd-phase`, `on-ci`, `on-schedule`); iterate the const set and assert length == 6.
- [x] **1.2** `internal/model/types_test.go` — table test: all two `TriggerMode` constants exist (`advisory`, `strong`) and string values match.
- [x] **1.3** `internal/model/types_test.go` — struct field test: `TriggerBinding` has fields `On TriggerEvent`, `When TriggerWhen`, `Run []string`, `Mode TriggerMode`, `Reason string`; zero-value `Mode` is empty (not pre-filled — catalog sets the default, not the struct); `Reason` is the only optional field.
- [x] **1.4** `internal/model/types_test.go` — struct field test: `TriggerWhen` has fields `Always bool`, `PathGlobs []string`, `MinDiffLines int`, `Phases []string`, `Combine string`; all are zero-value by default.
- [x] **1.5** `internal/model/types_test.go` — struct field test: `TriggerRuleSet` has fields `Events []TriggerEvent` and `Bindings []TriggerBinding`.

### Phase 2 — Green (implementation)

- [x] **1.6** `internal/model/types.go` — add `TriggerEvent string` newtype with the six named constants (`EventPreCommit`, `EventPrePush`, `EventPrePR`, `EventPostSDDPhase`, `EventOnCI`, `EventOnSchedule`); add doc comment: "These are SEMANTIC moments honored by the AI orchestrator, not OS-level hooks. gentle-ai never fires them."
- [x] **1.7** `internal/model/types.go` — add `TriggerMode string` newtype with `ModeAdvisory = "advisory"` and `ModeStrong = "strong"`; add doc comment: "advisory: suggestion. strong: firm recommendation. Neither blocks the workflow."
- [x] **1.8** `internal/model/types.go` — add `TriggerWhen struct` with JSON tags; add doc comment: "A structured, NON-evaluated condition. gentle-ai renders it to plain instruction text; the orchestrator interprets it."
- [x] **1.9** `internal/model/types.go` — add `TriggerBinding struct` with JSON tags; `Reason` gets `json:"reason,omitempty"`; doc comment notes `Reason` is the ONLY optional field.
- [x] **1.10** `internal/model/types.go` — add `TriggerRuleSet struct` with JSON tags.

---

## Unit 2 — Catalog

**Spec**: A (closed event set), B (validator: unknown run/on/mode), C (when vocab validation), E (default set token shape), G (token-budget rationale comment)
**Files**: `internal/catalog/triggers.go` (new), `internal/catalog/triggers_test.go` (new)

### Phase 1 — Red (failing tests)

- [x] **2.1** `internal/catalog/triggers_test.go` — `TestSupportedTriggerEvents_ClosedSet`: asserts exactly 6 events are returned; enumerates them by value.
- [x] **2.2** `internal/catalog/triggers_test.go` — `TestKnownAgents_ClosedSet`: asserts the closed agent set covers the 4R lenses (`review-risk`, `review-readability`, `review-reliability`, `review-resilience`), `judgment-day`, and all 8 SDD phase identifiers (`sdd-explore`, `sdd-propose`, `sdd-spec`, `sdd-design`, `sdd-tasks`, `sdd-apply`, `sdd-verify`, `sdd-archive`).
- [x] **2.3** `internal/catalog/triggers_test.go` — `TestDefaultTriggerRuleSet_TokenShape` (table-driven): assert:
  - (a) `pre-commit` binding: exactly one, `Mode == ModeAdvisory`, `Run == ["review-readability"]`, `When.Always == true`.
  - (b) `pre-push` binding: exactly one, `Mode == ModeAdvisory`, `Run == ["review-readability"]`, `When.Always == true`. No binding for `pre-push` includes all 4R agents simultaneously.
  - (c) `pre-pr` binding: exactly one, `Mode == ModeStrong`, `Run` contains all four 4R agents, `When.MinDiffLines == 400` and `When.PathGlobs` includes at least `**/auth/**` and `**/update/**`, `When.Combine == "or"` (or equivalent rendering of the compound condition).
  - (d) `post-sdd-phase` binding: exactly one, `Mode == ModeStrong`, `Run == ["judgment-day"]`, `When.Phases` contains `"design"` and `"apply"` and no other phase names.
  - (e) `on-ci` bindings: zero (none emitted).
  - (f) `on-schedule` bindings: zero (none emitted).
  - (g) Every emitted binding has a non-empty `Reason` field.
  - (h) `judgment-day` does NOT appear in any `pre-commit` or `pre-push` binding.
- [x] **2.4** `internal/catalog/triggers_test.go` — `TestDefaultTriggerRuleSet_CopyIsolation`: mutate the returned slice; assert a second call to `DefaultTriggerRuleSet()` returns an unmodified copy (defensive copy pattern, mirrors `MVPSkills()`).
- [x] **2.5** `internal/catalog/triggers_test.go` — `TestDefaultTriggerRuleSet_ThresholdConstant`: the named constant `defaultLargeChangedLineThreshold` (or equivalent exported/unexported const) equals 400 and is referenced in the pre-pr binding's `MinDiffLines` (validated via value, not source inspection).
- [x] **2.6** `internal/catalog/triggers_test.go` — `TestValidateTriggerRuleSet` (table-driven success/failure cases):
  - Pass: `ValidateTriggerRuleSet(DefaultTriggerRuleSet())` returns `nil`.
  - Pass: a well-formed custom binding (`on: pre-pr`, `run: ["review-risk"]`, `mode: strong`, `when: PathGlobs`) returns `nil`.
  - Fail: unknown `Run` entry `"review-seo"` returns non-nil error.
  - Fail: unknown `On` value `"post-merge"` returns non-nil error.
  - Fail: unknown `Mode` value `"blocking"` returns non-nil error.
  - Fail: `When.MinDiffLines <= 0` returns non-nil error.
  - Fail: `When.PathGlobs` empty slice (non-nil but empty) returns non-nil error.
  - Fail: `When.Combine` value not in `{"", "or", "and"}` returns non-nil error.
  - Fail: `When.Phases` entry not a recognized SDD phase identifier returns non-nil error.
  - Fail: `When.Phases` used on a non-`post-sdd-phase` event returns non-nil error.
  - Fail: binding with `Run` empty slice returns non-nil error.
  - Fail: binding where all of `Always`, `PathGlobs`, `MinDiffLines`, `Phases` are zero returns non-nil error (no `when` condition set).
- [x] **2.7** `internal/catalog/triggers_test.go` — `TestDefaultTriggerRuleSet_NoExecNoHooks`: confirm that importing and calling `DefaultTriggerRuleSet()`, `SupportedTriggerEvents()`, `KnownAgents()`, and `ValidateTriggerRuleSet()` produces no file I/O, no goroutine launch, no `exec.Command`; this is structural (review + compile assertion).

### Phase 2 — Green (implementation)

- [x] **2.8** `internal/catalog/triggers.go` (create) — package-level unexported constant `defaultLargeChangedLineThreshold = 400`; add the three-tier token-budget rationale doc comment block above the constant (spec G requirement).
- [x] **2.9** `internal/catalog/triggers.go` — implement `SupportedTriggerEvents() []model.TriggerEvent`: returns a defensive copy of the 6-event closed set.
- [x] **2.10** `internal/catalog/triggers.go` — implement `KnownAgents() []string`: returns a defensive copy of the closed agent set (4R, judgment-day, 8 SDD phases).
- [x] **2.11** `internal/catalog/triggers.go` — add unexported `var defaultRuleSet = model.TriggerRuleSet{...}` with the 4 bindings per the spec E table; each binding must have a non-empty `Reason`; add the `// on-ci and on-schedule have no built-in default because the appropriate agent/cadence is installation-specific; users opt in via override.` code comment (spec E requirement).
- [x] **2.12** `internal/catalog/triggers.go` — implement `DefaultTriggerRuleSet() model.TriggerRuleSet`: returns a defensive copy of `defaultRuleSet` (same pattern as `MVPSkills()` — `make` + `copy` for the `Bindings` slice, and a copy of `Events`).
- [x] **2.13** `internal/catalog/triggers.go` — implement `ValidateTriggerRuleSet(set model.TriggerRuleSet) error`: validates each binding's `On`, `Run`, `Mode`, `When` fields against the closed vocabularies; returns descriptive error on first violation; accepts `Reason` presence or absence without error.

---

## Unit 3 — Renderer

**Spec**: C (when rendering), D (mode wording: `consider` vs `strongly recommend`), F (marker-free, ≤40 lines, plain text, organic note), G (token-budget semantics visible)
**Files**: `internal/components/sdd/triggerrules.go` (new), `internal/components/sdd/triggerrules_test.go` (new)

### Phase 1 — Red (failing tests)

- [x] **3.1** `internal/components/sdd/triggerrules_test.go` — `TestRenderTriggerRules_Deterministic`: call `RenderTriggerRules` twice on the same `TriggerRuleSet`; assert outputs are byte-identical.
- [x] **3.2** `internal/components/sdd/triggerrules_test.go` — `TestRenderTriggerRules_MarkerFree`: rendered output does NOT contain `<!-- gentle-ai:` or `<!-- /gentle-ai:` (markers are added by the caller via `InjectMarkdownSection`).
- [x] **3.3** `internal/components/sdd/triggerrules_test.go` — `TestRenderTriggerRules_OrganicNote`: rendered output contains language stating these rules are organic recommendations, not hard gates (e.g., the phrase "organic" or "not a gate" or equivalent).
- [x] **3.4** `internal/components/sdd/triggerrules_test.go` — `TestRenderTriggerRules_ModeWording` (table-driven):
  - Binding with `Mode == ModeAdvisory`: rendered text contains `"consider"` (or equivalent soft language); does NOT contain `"strongly"`, `"must"`, `"required"`, `"critical"`.
  - Binding with `Mode == ModeStrong`: rendered text contains `"strongly recommend"` (or equivalent firm language); does NOT contain `"gate"`, `"block"`, `"halt"`, `"must not proceed"`.
  - Advisory and strong renderings of identical bindings (same `On`/`When`/`Run`) are NOT equal.
- [x] **3.5** `internal/components/sdd/triggerrules_test.go` — `TestRenderTriggerRules_WhenPhrasing` (table-driven):
  - `When.Always == true` → rendered text contains `"always"` or `"every occurrence"` or `"unconditionally"`.
  - `When.PathGlobs` non-empty → rendered text contains each glob string verbatim.
  - `When.MinDiffLines == 400` → rendered text contains `"400"`.
  - `When.Phases` contains `"design"` → rendered text contains `"design"`.
  - Compound `PathGlobs OR MinDiffLines` → rendered text contains both the globs and the line count and `"OR"` or `"or"`.
- [x] **3.6** `internal/components/sdd/triggerrules_test.go` — `TestRenderTriggerRules_LineBudget`: render `DefaultTriggerRuleSet()` and assert the output has no more than 40 lines (spec F requirement).
- [x] **3.7** `internal/components/sdd/triggerrules_test.go` — `TestRenderTriggerRules_Golden` (golden file): render `DefaultTriggerRuleSet()` and assert output matches `internal/testdata/golden/trigger-rules-default.golden`; create golden with `-update` flag (same `var update = flag.Bool("update", ...)` pattern used in `internal/components/golden_test.go`).

### Phase 2 — Green (implementation)

- [x] **3.8** `internal/components/sdd/triggerrules.go` (create) — implement `RenderTriggerRules(set model.TriggerRuleSet) string`:
  - Output: fixed header, one-line organic-not-a-gate note, one bullet per binding in declaration order.
  - Each bullet: `At **{event}**, {when-phrase}, {mode-phrase} running {agents} in parallel.` (or serial if single agent).
  - `ModeAdvisory` → `"consider"` phrasing; `ModeStrong` → `"strongly recommend"` phrasing.
  - `TriggerWhen.Always` → `"always"` / `"at every occurrence"`.
  - `TriggerWhen.PathGlobs` → `` `**/auth/**` `` inline code, joined by `, ` with `or` combinator.
  - `TriggerWhen.MinDiffLines` → `"when the diff exceeds {N} changed lines"`.
  - Compound (PathGlobs + MinDiffLines with `Combine == "or"`) → `"when the diff touches {globs} OR exceeds {N} changed lines"`.
  - `TriggerWhen.Phases` → `"after the {phase} or {phase} phase completes"`.
  - Function is pure (no I/O, no globals mutated, no goroutines).
- [x] **3.9** Generate the golden file for 3.7 by running `go test ./internal/components/sdd/ -run TestRenderTriggerRules_Golden -update` and committing the result.

---

## Unit 4 — Injection

**Spec**: F (8-agent coverage, idempotency, `gentle-ai:trigger-rules` section ID, marker-section mechanism, primary placement), H (no exec, no hooks)
**Files**: `internal/components/sdd/inject.go` (modify), `internal/assets/kimi/KIMI.md` (modify), `internal/components/sdd/inject_test.go` (modify)

### Phase 1 — Red (failing tests)

- [x] **4.1** `internal/components/sdd/inject_test.go` — `TestInjectTriggerRules_SystemPromptAgent` (representative: claude adapter):
  - Call `sdd.Inject(home, claudeAdapter(), "")` in a `t.TempDir()` home.
  - Assert the resulting CLAUDE.md contains `<!-- gentle-ai:trigger-rules -->` opening marker.
  - Assert the file contains `<!-- /gentle-ai:trigger-rules -->` closing marker.
  - Assert there is at least one rendered binding line between the markers.
- [x] **4.2** `internal/components/sdd/inject_test.go` — `TestInjectTriggerRules_Idempotent` (claude adapter):
  - Call `sdd.Inject` twice on the same `t.TempDir()` home.
  - Assert the `trigger-rules` marker section appears exactly once in CLAUDE.md after the second call.
  - Assert the content between markers is identical after both calls.
- [x] **4.3** `internal/components/sdd/inject_test.go` — `TestInjectTriggerRules_JinjaModule` (kimi adapter):
  - Call `sdd.Inject(home, kimiAdapter(), "")` in a `t.TempDir()` home.
  - Assert `filepath.Join(home, ".kimi", "trigger-rules.md")` exists and contains the rendered block (no markers — the module itself is the content; KIMI.md provides the include wrapper).
- [x] **4.4** `internal/components/sdd/inject_test.go` — `TestInjectTriggerRules_OpenCodePlacement`:
  - Call `sdd.Inject(home, opencodeAdapter(), "")` in a `t.TempDir()` home.
  - Assert the gentle-orchestrator agent prompt path (resolved via the adapter or a known constant) contains `<!-- gentle-ai:trigger-rules -->`.
  - (Resolves open question (a): confirm the block lands in the orchestrator prompt, not a separate AGENTS.md section.)
- [x] **4.5** `internal/components/sdd/inject_test.go` — `TestInjectTriggerRules_KilocodePlacement`:
  - Same as 4.4 but for `kilocodeAdapter()`.
- [x] **4.6** `internal/components/sdd/inject_test.go` — `TestInjectTriggerRules_AllAdapters` (coverage guard, mirrors spec F "8-agent" requirement):
  - Iterate all adapters from the factory (or a hand-enumerated list covering all 16 adapters registered in `factory.go`).
  - For each adapter, call `sdd.Inject` in a fresh `t.TempDir()` and assert that the resulting primary system-prompt or orchestrator file contains `trigger-rules` content.
  - If a new adapter is added to the registry without injection wired, this test MUST fail (explicit, not silent).

### Phase 2 — Green (implementation)

- [x] **4.7** `internal/components/sdd/inject.go` — add unexported const `sectionTriggerRules = "trigger-rules"`.
- [x] **4.8** `internal/components/sdd/inject.go` — after the strict-tdd-mode step (inject.go:274-314), add step 1c:
  - Compute `rendered := sdd.RenderTriggerRules(catalog.DefaultTriggerRuleSet())`.
  - For `StrategyJinjaModules` adapters (currently only Kimi): write `rendered` as `trigger-rules.md` in `adapter.GlobalConfigDir(homeDir)` via `filemerge.WriteFileAtomic`; add to `files`; do NOT call `InjectMarkdownSection` (the Jinja template includes it via `{% include "trigger-rules.md" %}`).
  - For OpenCode and Kilocode: append the marker-wrapped block to the gentle-orchestrator prompt using the same injection path already used for orchestrator content (mirrors how step 1 handles `AgentOpenCode`/`AgentKilocode` — see inject.go:229 and inject.go:354).
  - For all other system-prompt agents: read `adapter.SystemPromptFile(homeDir)`, call `filemerge.InjectMarkdownSection(existing, sectionTriggerRules, rendered)`, write atomically, dedupe path in `files` (same pattern as strict-tdd-mode step, inject.go:302-312).
- [x] **4.9** `internal/assets/kimi/KIMI.md` — add `{% include "trigger-rules.md" ignore missing %}` immediately after the `{% include "strict-tdd-mode.md" ignore missing %}` line (line 8), maintaining the established include ordering.

### Resolve Open Question (a) — OpenCode/Kilocode Placement

- [x] **4.10** (Task, not just a question) Read the current OpenCode/Kilocode gentle-orchestrator prompt injection paths in `inject.go` (lines ~229 and ~354) and `inlineOpenCodeSDDPrompts`. Confirm the rendered block belongs in the gentle-orchestrator agent prompt scope (not in a top-level AGENTS.md). Document the decision as a comment in `inject.go` step 1c. (Implements design open question (a).)

### Resolve Open Question (b) — Jinja Adapter Scan

- [x] **4.11** (Task) Search all adapter implementations for `StrategyJinjaModules` (currently only `internal/agents/kimi/adapter.go` returns this strategy). If any additional Jinja adapters are found, add the `{% include "trigger-rules.md" ignore missing %}` line to their entry template (analogous to task 4.9). If none are found beyond Kimi, document the single-adapter finding as a comment in `inject.go` step 1c. The design notes the gatekeeper already confirmed only Kimi today; this task verifies and documents.

---

## Unit 5 — Organic Invariant Audit

**Spec**: H (no exec, no hooks, no event bus, no gate, no when-eval engine, no new parse dependency)
**Files**: all files modified or created by this change

### Phase 1 — Audit (no Red/Green — this is a verification pass, not a TDD unit)

- [x] **5.1** `go build ./...` — must pass clean with no new build errors.
- [x] **5.2** `go vet ./...` — must pass clean with no new warnings.
- [x] **5.3** `go test ./...` — full suite must pass; no regressions in existing golden files (the injection of the trigger-rules section changes the rendered content of all agent system-prompt files, so golden files that cover the full CLAUDE.md / GEMINI.md / etc. MUST be regenerated with `-update` and committed alongside the implementation).
- [x] **5.4** Organic invariant: grep all new and modified files for `exec.Command`, `os/exec`, goroutine launches (`go func`), channel operations (`<-`, `chan `), file-system reads of git diff output, `.git/hooks` path references. Assert none of these are attributable to the trigger-rules feature. Document the clean result as a checklist comment in the PR description.
- [x] **5.5** `go.mod` diff: confirm no new parse-library entries (YAML, TOML, INI) were added. `encoding/json` tags on structs are not a new dependency.
- [x] **5.6** Golden file regeneration: after the injection step is wired (Unit 4), run `go test ./internal/components/ -update` to regenerate all golden files that now include the `trigger-rules` section; review the diff to confirm only the expected `<!-- gentle-ai:trigger-rules -->...<!-- /gentle-ai:trigger-rules -->` block was added to each file.

---

## Unit 6 — Documentation

**Spec**: Proposal documentation success criterion; F (rendered block is self-contained)
**Files**: A new or existing doc location within the repo (e.g., a short note appended to `README.md` or a dedicated `docs/trigger-rules.md`). Check existing docs conventions first.

### Phase 2 — Green (implementation, no Red for docs)

- [x] **6.1** Write a user-facing explanation (≤ 30 lines) covering:
  - What the trigger-rules section is (organic recommendations, not gates).
  - Where the injected section appears (system-prompt or orchestrator file for each supported agent).
  - What the three default tiers are (Tier-1 advisory pre-commit/pre-push; Tier-2 4R strong on hot paths; Tier-3 judgment-day on SDD design/apply).
  - How to re-run `gentle-ai install` or `gentle-ai sync` to refresh the injected section after an update.
  - Note: `on-ci` and `on-schedule` have no built-in default; users opt in via a future override mechanism.
- [x] **6.2** Verify the doc is placed alongside existing documentation (check whether the project uses `README.md` sections, a `docs/` directory, or in-source comments as the primary user doc surface; follow the established pattern).

---

## Dependency and Sequencing Notes

```
Unit 1 (types)
    └─► Unit 2 (catalog — imports model types)
            └─► Unit 3 (renderer — imports model types and uses catalog for golden)
                    └─► Unit 4 (injection — calls renderer and catalog)
                            └─► Unit 5 (audit + golden regen — depends on all prior units)
                                    └─► Unit 6 (docs — independent, can run any time after Unit 1)
```

Units 1 → 2 → 3 → 4 are strictly sequential (each imports the previous).
Unit 5 runs after Unit 4 (golden regen requires final wired injection).
Unit 6 is independent of 3–5 and may be written after Unit 1 if desired, but MUST be committed before the PR is opened.

### Work-Unit Commit Discipline

Each unit ships as a single commit with its test + implementation + any golden files together. Commit messages use Conventional Commits. Each commit must leave `go test ./...` green so the PR diff is bisectable.

Example ordering:
1. `feat(model): add trigger-rules type model and tests` — Unit 1
2. `feat(catalog): add trigger-rules default set and validator` — Unit 2
3. `feat(sdd): add trigger-rules renderer with golden and table tests` — Unit 3
4. `feat(sdd): inject trigger-rules section into all agent assets` — Unit 4 (includes KIMI.md change)
5. `chore(sdd): regenerate golden files with trigger-rules section` — Unit 5 golden regen
6. `docs(trigger-rules): document default rule set and injection` — Unit 6

---

## Cross-Unit Notes

- **Golden file cascade (CRITICAL)**: Wiring the injection in Unit 4 causes ALL existing SDD golden files that cover full agent prompt files (CLAUDE.md, GEMINI.md, AGENTS.md, etc.) to change. Run `go test ./internal/components/ -update` ONCE after Unit 4, review the diff, and commit the regenerated goldens in the same Unit 4 commit or a dedicated Unit 5 commit. Do NOT skip this step — golden mismatches will break CI.
- **No new marker constants**: `InjectMarkdownSection(existing, "trigger-rules", rendered)` derives `<!-- gentle-ai:trigger-rules -->` automatically. `section.go` stays untouched.
- **Kimi KIMI.md static template**: the `{% include "trigger-rules.md" ignore missing %}` line is static (committed to `internal/assets/kimi/KIMI.md`). The rendered content lives in the per-install `trigger-rules.md` module file written by the injector at install/sync time.
- **Qwen Code**: check if Qwen also uses `StrategyJinjaModules` (task 4.11 covers this). Based on current codebase review, only Kimi uses `StrategyJinjaModules`; Qwen uses a different strategy.
