# Installer Picker Navigation Specification

## Purpose

Defines the navigation contract for the installer TUI's conditional picker chain, ensuring that forward and backward navigation (both via Esc and "← Back" row selection) are symmetric and ordered from a single declarative source of truth. No new or modified user-observable behavior; these requirements enforce the behavior-preservation invariants that MUST hold after the internal refactor from triplicated imperative logic to a computed flow slice.

---

## Requirements

### Requirement: Declarative picker flow ordering

The installer's conditional picker chain MUST be navigated forward from a single ordered source of truth, filtered by the per-selection `shouldShow*` predicates. The forward order is:

```
Claude → Kiro → Codex → SDDMode → ModelPicker → StrictTDD → OpenCodePlugins → DependencyTree
```

`ScreenModelPicker` MUST be included when `SDDMode == Multi`. It asynchronously discovers the active project's effective OpenCode catalog after opening, so no private model-cache file gates navigation. No screen may appear out of this order, and none may be skipped unless its predicate is false. When `Preset == PresetCustom`, `ScreenDependencyTree` precedes the picker sequence (component selector first) instead of being the terminal anchor.

#### Scenario: All agents selected, full forward pass

- GIVEN all `shouldShow*` predicates return true and `SDDMode == Multi`
- WHEN the user confirms each picker screen in sequence
- THEN the screens visited in order are Claude → Kiro → Codex → SDDMode → ModelPicker → StrictTDD → OpenCodePlugins → DependencyTree

#### Scenario: Eligible-only screens are visited

- GIVEN `shouldShowKiro` is false and all other predicates are true
- WHEN the user confirms Claude and continues
- THEN the next screen is Codex (Kiro is skipped)

#### Scenario: ModelPicker is independent of private cache state

- GIVEN `SDDMode != Multi`
- WHEN the user confirms SDDMode
- THEN the next screen is StrictTDD and ModelPicker is not visited

- GIVEN `SDDMode == Multi` and no private OpenCode cache file exists
- WHEN the user confirms SDDMode
- THEN ModelPicker opens and begins runtime catalog discovery

#### Scenario: PresetCustom ordering inversion

- GIVEN `Preset == PresetCustom`
- WHEN navigation enters the picker chain
- THEN DependencyTree appears first (before Claude/Kiro/Codex), not last


### Requirement: Symmetric reverse navigation

Backward navigation MUST be the exact reverse of forward navigation and MUST be identical whether triggered by Esc or by selecting the "← Back" row. Both mechanisms read the same flow slice.

#### Scenario: SDDMode back returns to Codex when Codex is in the flow

- GIVEN Codex is in the flow (`shouldShowCodex` is true) and the user reached ScreenSDDMode
- WHEN the user presses Esc OR selects "← Back" on ScreenSDDMode
- THEN the current screen is ScreenCodex (Codex is not skipped)

#### Scenario: Codex "← Back" row is not inert

- GIVEN Kiro is in the flow (`shouldShowKiro` is true) and the user is on ScreenCodex
- WHEN the user selects "← Back" on ScreenCodex
- THEN the current screen is ScreenKiro

#### Scenario: Full round-trip symmetry for any agent subset

- GIVEN any combination of selected agents
- WHEN the user navigates forward through the whole slice and then backward through the whole slice
- THEN the backward sequence is the exact reverse of the forward sequence and the user returns to the pre-picker screen

### Requirement: Cross-cutting guards remain outside the slice

The `ModelConfigMode` exit-ramp, the `OpenCodePluginsStandalone` guard, and the GentleAI-upgrade Esc-quit MUST remain early-return guards evaluated before the flow-slice walk; they MUST NOT be folded into `pickerFlowSlice`.

#### Scenario: ModelConfigMode exit-ramp returns to config menu

- GIVEN pickers were entered from ScreenModelConfig
- WHEN the user presses Esc or "← Back" on the first picker screen
- THEN the current screen is ScreenModelConfig (not the previous picker-flow screen)

#### Scenario: OpenCodePlugins guard precedes the dependency tree without SDD

- GIVEN OpenCode is selected without the SDD component (no picker/SDDMode/StrictTDD applies)
- WHEN the user confirms the preset
- THEN the next screen is ScreenOpenCodePlugins before the dependency tree

---

## Test Enforcement

| Requirement / Scenario | Enforced by |
|------------------------|-------------|
| Forward ordering, round-trip symmetry | `internal/tui/preset_flow_test.go::TestInstallNavigationRoundTrips` (all cases) |
| SDDMode back → Codex, Codex back not inert | `TestInstallNavigationRoundTrips` regression cases + `TestPickerBackRowRegression` |
| ModelPicker gate (multi / cache) | `pickerFlowSlice` unit rows in `model_test.go` |
| PresetCustom inversion | `TestCustomPresetPostComponentFlowMatrix` |
| First-picker entry incl. Kiro/Codex-first | `TestPresetConfirmEntersFirstPickerInFlow` |
| ModelConfigMode / OpenCodePlugins guards | existing guard tests (unchanged) |

---

## Out of Scope

- `shouldShow*` predicate logic — unchanged
- View/render code — no golden output changes
- `router.go` / `linearRoutes` — untouched
- Backups, uninstall, agent-builder, profiles, upgrade/sync nav graphs
