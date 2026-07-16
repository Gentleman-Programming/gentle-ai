## Exploration: issue-445 profile-only uninstall Engram safety

### Current State

On current `main` (`4079d12`), partial/full uninstall can route through **Uninstall Scope Selection** (`ScreenUninstallProfiles`), which bundles:

1. OpenCode SDD **profile checkboxes** (shown when OpenCode agent + SDD component + discovered profiles)
2. **Engram cleanup scope** checkboxes (shown when Engram component is selected **and** workspace `.engram/` exists)

There is **no** `EngramUninstallScopeNone`. The model only supports:

- `EngramUninstallScopeGlobal` (`"global"`)
- `EngramUninstallScopeProject` (`"project"`)

Defaults always force a destructive Engram scope:

| Path | Default |
|------|---------|
| `NewModel` | `Global` |
| `withResetUninstallState` | `Global` |
| `refreshUninstallProfiles` | `Global` (also on every components→profiles entry) |
| Enter `ScreenUninstallMode` | `Global` |
| Service `NewService` / `PartialUninstall` | `Global` |
| `SetEngramUninstallScope` | any non-`project` value becomes **`global`** (fail-open to destructive) |

Additional coupling that amplifies the bug:

- `defaultUninstallComponents()` pre-selects **all** MVP components, including Engram.
- Full modes populate all agents + all components, then often land on profile/scope selection with **all profiles pre-selected**.
- When Engram is selected but project `.engram/` is absent, the scope UI is **hidden**, but execution still uses **Global** silently.
- Confirm only shows Engram scope when Engram is among selected components; result copy reports Engram scope only when Engram artifacts appear in the result.
- Execution always goes through managed uninstall (`UninstallWithProfilesFn` / `PartialUninstallWithProfiles`) when profile selection was used — never a profile-only remover.

A **separate** Welcome → Profiles → Delete flow already removes a single profile via `sdd.RemoveProfileAgents` with **no** Engram cleanup. That path is the true “delete only a profile” action today, but the uninstall scope screen presents a competing, more dangerous UX.

### Affected Areas

- `internal/model/types.go` — `EngramUninstallScope` constants (no `none`)
- `internal/tui/model.go` — defaults, `refreshUninstallProfiles`, `toggleCurrentUninstallEngramScope`, `optionCount`, confirm/`startUninstall` dispatch, `shouldShowUninstallEngramScopeSelection`
- `internal/tui/screens/uninstall.go` — `uninstallEngramScopeOptions`, `RenderUninstallProfiles`, confirm/result Engram copy
- `internal/components/uninstall/service.go` — `SetEngramUninstallScope`, Engram component planning (`ComponentEngram` branch)
- `internal/cli/uninstall.go` / `internal/app/app.go` — profile-aware uninstall wiring
- `internal/components/sdd/profiles.go` — `RemoveProfileAgents` (existing pure profile remover)
- Tests: `internal/tui/model_test.go`, `internal/tui/screens/uninstall_result_test.go`, `internal/components/uninstall/service_test.go`

### Approaches

1. **A — Profile-only isolation** — When partial mode has selected profiles, hide Engram UI and route through `RemoveProfileAgents` (or equivalent) instead of managed uninstall.
   - Pros: Matches issue wording “only remove that profile from opencode.json”; closed PR #1299 already prototyped isolation + tests; reuses existing profile remover.
   - Cons: Naive trigger (“any selected profiles”) **breaks** legitimate “uninstall SDD/components **and** remove these profiles”. Profile selection today is an **additive** SDD cleanup scope inside managed uninstall, with all profiles pre-selected by default. Isolation needs a precise trigger or it will silently skip component uninstall.
   - Effort: Medium (routing + UX conditions + regression risk on combined uninstall)

2. **B — `EngramUninstallScopeNone` default + opt-in cleanup (bundled UI kept)** — Add explicit `none` scope; default profile-aware / scope-selection entry to `none`; project/global remain explicit opt-ins; service emits **zero** Engram ops for `none` and rejects unknown scopes fail-closed.
   - Pros: Directly fixes the data-loss bug; preserves combined uninstall (agents/components + selective profiles); matches expected “opt-in (unchecked by default)”; smaller blast radius than full UX split.
   - Cons: Does **not** alone make the screen “profile-only uninstall” — other selected components still uninstall. Prior SDD attempt hit gaps on reset/re-entry, profile-discovery-error paths, and UI cursor/option cardinality. Service currently maps non-project → global, so `none` must be a first-class contract.
   - Effort: Medium (types + service + TUI defaults/render/confirm/result + strict TDD)

3. **C — Fully separate profile management from uninstall** — Remove profile selection from uninstall; profiles only via Welcome → Profiles; uninstall never touches named profiles except full/legacy “all profiles” SDD cleanup.
   - Pros: Clearest mental model; Profiles menu already exists; uninstall no longer pretends to be a profile manager.
   - Cons: Largest UX/product change; full/partial SDD uninstall loses selective profile subsetting unless reintroduced elsewhere; more copy/navigation work; higher review size.
   - Effort: High

### Recommendation

**Prefer Approach B** (`EngramUninstallScopeNone` default + opt-in), implemented as a **service-first contract** with TUI parity.

Why not pure A: on current code, profile selection is not a standalone “delete profile” mode — it is reached **after** agents/components are chosen, with components defaulting to **all**. Routing “profiles selected → profile remover only” would break partial SDD uninstall for OpenCode. Why not C now: higher cost, and Profiles already covers pure profile delete; the approved bug is specifically forced Engram cleanup.

**B must close prior gaps explicitly:**

1. **Service contract**
   - Add `EngramUninstallScopeNone = "none"`.
   - `SetEngramUninstallScope`: accept `none` / `project` / `global`; **unknown → error or no Engram ops** (never coerce to global).
   - `ComponentEngram` planning: `none` → zero targets/ops.
2. **TUI defaults (profile-aware / scope screen)**
   - Scope-selection entry and refresh for that screen default to `none` when Engram scope UI is relevant.
   - Full uninstall modes may keep global Engram intent when Engram is selected (document in design) — do not silently change full wipe semantics without product confirmation; issue is about profile-adjacent partial flow.
3. **UI**
   - Always offer a visible **No cleanup** option when Engram scope selection is shown (even if only one destructive option remains).
   - Confirm + result copy must be truthful for `none` (no “Global” implication).
   - `optionCount` / cursor navigation must match rendered rows (prior gap).
4. **Lifecycle**
   - Reset, back-navigation, re-entry, and profile-discovery failure must not re-arm Global cleanup unexpectedly.
   - `refreshUninstallProfiles` must not unconditionally stomp a confirmed user choice mid-flow without a defined rule (prefer: reset only on mode entry / explicit refresh boundaries).
5. **Optional product clarity (small, same PR if budget allows)**
   - Help text on scope screen: profiles remove OpenCode agent keys; Engram cleanup is optional and separate.
   - Do **not** claim profile checkbox alone uninstalls nothing else while components remain selected.

**Out of scope for this change (unless product expands):** rewriting uninstall into pure profile management (A/C). Document that Welcome → Profiles is the pure profile-delete path.

### Risks

- **Fail-open service mapping**: current `SetEngramUninstallScope` treats non-project as global — any incomplete `none` wiring still deletes Engram.
- **Silent Global when UI hidden**: Engram selected + no `.engram/` directory still cleans global MCP/prompt integration; `none` default and/or UI visibility rules must cover this path.
- **Default-all components**: even with Engram `none`, users may still remove SDD/skills/etc. if they continue with defaults — UX copy should not oversell “profile-only”.
- **Full vs partial semantics**: changing full-uninstall Engram default to `none` could leave Engram behind after a “full” wipe — design must pin mode-specific defaults.
- **Cursor/option drift**: adding a third scope option without updating `optionCount`, space-toggle, and render loops reintroduces navigation bugs (prior verification gaps).
- **Test gaps**: `shouldShowUninstallEngramScopeSelection` / `refreshUninstallProfiles` currently lack dedicated coverage; regressions will slip without new tests.
- **Delivery size**: focused B + tests should fit a single PR under the 400-line review budget; expanding into A/C or broad copy rewrites risks exceeding it.

### Test strategy (strict_tdd: true)

Runner: `go test ./...` (plus `go vet ./...` at verify).

RED-first units:

1. **Service** — `none` plans zero Engram targets/ops; `project` only workspace `.engram/`; `global` removes integration; unknown scope does not delete Engram.
2. **TUI model** — entering scope selection with Engram+project data defaults to `none`; space toggles among none/project/global; `optionCount` matches rows; back/re-entry does not re-arm global; profile-discovery error does not force destructive Engram cleanup.
3. **Screens** — scope render shows “No cleanup” unchecked-by-default semantics; confirm/result text for `none` is truthful; full-mode regressions keep intentional Engram behavior.
4. **Dispatch** — `startUninstall` passes selected scope through `UninstallWithProfilesFn` unchanged.

Prefer Bubbletea direct `model.Update(tea.KeyMsg…)` patterns already used in `internal/tui/model_test.go` (no teatest).

### Complexity / review budget

| Item | Estimate |
|------|----------|
| Complexity | **Medium** |
| Authored churn (impl + tests) | ~180–320 lines if scoped to B only |
| 400-line budget risk | **Low–Medium** (Low if service+TUI focused; Medium if full-mode redesign + A isolation added) |
| Chained PRs recommended | **No** for pure B; **Yes** if product expands to A isolation or C separation |

### Ready for Proposal

**Yes.** Proceed to `sdd-propose` with Approach B: explicit `none` Engram scope default for profile-aware/scope-selection flows, opt-in project/global cleanup, fail-closed service contract, truthful confirm/result copy, and lifecycle tests covering reset/re-entry/discovery-error paths. Call out Profiles menu as the existing pure profile-delete path; do not implement full isolation (A) or full separation (C) unless product re-scopes the issue.
