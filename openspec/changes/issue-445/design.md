# Design: Profile-aware uninstall Engram safety (issue-445)

## Technical Approach

Service-first Approach B: introduce `EngramUninstallScopeNone` (`"none"`), plan **zero** Engram ops for it, and stop fail-open non-project→global mapping. TUI defaults profile-adjacent / scope-selection paths to `none`; full wipe modes keep **global** when Engram is selected. Scope UI always includes **No cleanup** when shown; confirm/result stay truthful. Lifecycle entry/reset/refresh must not re-arm Global unexpectedly.

Maps to proposal capability `uninstall-engram-scope` and exploration Approach B.

## Architecture Decisions

| Decision | Options | Tradeoff | Choice |
|----------|---------|----------|--------|
| Unknown scope | error vs coerce-global vs none | error aborts whole uninstall; global is the bug | **Fail-closed to `none`** (zero Engram ops); keep `SetEngramUninstallScope` void |
| Full-mode Engram default | always none vs keep global | none leaves Engram after “full” wipe | **Full / FullRemove / CleanInstall → `global` when Engram selected**; partial/scope-selection → `none` |
| Scope UI visibility | only if `.engram/` vs whenever Engram selected | hidden UI + old global = silent wipe | **Show when Engram selected**; Project-only row only if project data exists; **none default** covers hidden-edge paths |
| Refresh stomp | always reset scope vs preserve mid-flow | preserve needs more state | **Reset scope only on defined entry boundaries** (mode entry, components→profiles); refresh always updates project-data flag |
| Pure profile delete | reuse uninstall vs Welcome→Profiles | isolation breaks combined uninstall | **No change** — Welcome→Profiles remains pure delete |

## TUI Default Matrix

| Path | Default scope | Notes |
|------|---------------|-------|
| `NewModel` | `none` | Safe idle default |
| `withResetUninstallState` | `none` | Leaving/re-entering uninstall |
| `setScreen(ScreenUninstallMode)` + mode enter | `none` then mode may override | Entry boundary |
| Partial → components Continue → `refreshUninstallProfiles` | `none` | Profile-aware / scope path |
| Full / FullRemove / CleanInstall after populate agents+components | `global` if Engram selected else `none` | Intentional full wipe |
| Profile discovery error in refresh | keep `none` (or mode default already applied) | Must **not** force Global |
| Engram selected, no `.engram/` (UI may still show none+global) | `none` on partial | Execution uses model scope, not “hidden ⇒ global” |
| `PartialUninstall` / `CompleteUninstall` / `NewService` (no TUI scope) | unchanged global for complete; partial-without-profiles stays global only if API callers omit scope | Profile-aware API uses explicit scope arg |

Helper (conceptual): `defaultEngramUninstallScope(mode, components) EngramUninstallScope`.

## Interfaces / Contracts

```go
const EngramUninstallScopeNone EngramUninstallScope = "none"

// SetEngramUninstallScope: none|project|global only; unknown → none (never global).
// ComponentEngram: none → zero targets/ops; project → workspace .engram/ only; global → existing integration cleanup.
```

`PartialUninstallWithProfiles(..., engramScope)` continues to call `SetEngramUninstallScope` before plan; defer may reset service field to `none` (safer idle) or leave as today — prefer reset to `none` for fail-closed idle state.

## Data Flow / Sequence (partial profile-aware, none default)

```
User → UninstallMode(Partial) → Agents → Components (Engram±)
         │
         ▼
refreshUninstallProfiles
  detect .engram/ flag
  UninstallEngramScope = none          ◄── no Global stomp
         │
         ▼
ScreenUninstallProfiles
  profiles checkboxes
  Engram: [x] No cleanup | [ ] Project? | [ ] Global
  optionCount = profiles + len(scopeOptions) + 2
         │  space toggles exclusive scope
         ▼
Confirm (label "None" / no Global implication)
         │
         ▼
startUninstall → UninstallWithProfilesFn(..., none)
         │
         ▼
Service.SetEngramUninstallScope(none) → buildPlan → ComponentEngram: 0 ops
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/model/types.go` | Modify | Add `EngramUninstallScopeNone` |
| `internal/components/uninstall/service.go` | Modify | Fail-closed setter; `none` planning; safer idle default |
| `internal/tui/model.go` | Modify | Defaults matrix, refresh/reset/mode entry, toggle by options index, `optionCount`, full-mode global apply |
| `internal/tui/screens/uninstall.go` | Modify | Options include No cleanup first; always render when Engram UI shown; confirm/result for `none` |
| `internal/cli/uninstall.go`, `internal/app/app.go` | Modify | Pass-through only if needed (same scope type) |
| `*_test.go` (service, model, screens) | Modify | Strict TDD RED→GREEN |

## UI: options / cursor / optionCount

`uninstallEngramScopeOptions(projectAvailable)` order:

1. **No cleanup** (`none`) — always when Engram scope section shown  
2. **Project-only** — iff project `.engram/` available  
3. **Global cleanup** — always when section shown  

Render whenever Engram component selected (drop `len(options) > 1` hide).  
`toggleCurrentUninstallEngramScope`: `options[cursor - profileCount].Scope` (no hard-coded idx→project/global).  
`optionCount` / enter handler: `engramScopeOptionCount = len(options)` when shown, not `+2`.

Confirm: if scope `none` → “None” / “No Engram cleanup”.  
Result: if `none` or no Engram artifacts → never print Global.

## Lifecycle Guards

| Boundary | Behavior |
|----------|----------|
| Reset uninstall state | scope → `none` |
| Enter `ScreenUninstallMode` | refresh availability; scope → mode default (`none` until full selection applies global) |
| Components → Profiles | refresh; partial → `none`; full* + Engram → `global` |
| Mid-screen space toggle | preserve user choice; no refresh |
| Profile discovery failure | clear profile lists; **do not** set Global |
| Back to components then re-enter | re-apply entry default (intentional reset) |

## Testing Strategy (strict TDD)

Runner: `go test` on touched packages; verify: `go test ./...` + `go vet ./...`. Prefer Bubbletea `model.Update(tea.KeyMsg)` (no teatest).

| Layer | RED cases first |
|-------|-----------------|
| Unit service | `none` → 0 Engram ops; project → only workspace `.engram/`; global → integration; unknown → not global |
| Unit model | partial entry/refresh → `none`; full+Engram → `global`; space cycles none/project/global; `optionCount` matches rows; discovery error ≠ Global; reset ≠ Global |
| Unit screens | No cleanup visible & default-checked semantics; confirm/result truthful for `none` |
| Dispatch | `startUninstall` passes model scope unchanged to `UninstallWithProfilesFn` |

## Threat Matrix

N/A — no new routing, shell, subprocess, VCS/PR automation, executable-file classification, or process-integration boundary. Change is scope enum + planning + TUI defaults on existing uninstall path.

## Migration / Rollout

No migration. In-memory/runtime scope only. Single PR under ~400-line budget (focused B). Rollback: revert PR / branch; users reinstall Engram via normal setup if a bad build ran cleanup.

## Risks and Rollback

| Risk | Mitigation |
|------|------------|
| Incomplete `none` still maps global | Service tests first; fail-closed unknown |
| Full wipe leaves Engram | Mode-specific default matrix + tests |
| Cursor drift with 3 options | optionCount + nav tests tied to `len(options)` |
| Copy oversells “profile-only” | Confirm lists components; help: Engram optional |
| Hidden UI + Engram selected | `none` default + show scope when Engram selected |

**Rollback**: revert commit/PR. No schema/data migration.

## Open Questions

- None blocking. Product already approved opt-in Engram cleanup for profile-adjacent flows; full-mode keeps global when Engram selected.
