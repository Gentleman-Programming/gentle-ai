# Proposal: Profile-aware uninstall must not force Engram cleanup

## Intent

Uninstall Scope Selection forces destructive Engram cleanup (`global` by default) even when the user is only adjusting OpenCode SDD profiles or did not opt into Engram removal. That is a data-loss bug: Engram MCP/prompt integration and project data can be wiped without an explicit choice.

**Approved expected behavior** (issue #445): profile-adjacent uninstall must not force Engram cleanup; Engram cleanup is opt-in (unchecked by default) or a separate deliberate choice.

## Assumptions (auto mode)

- Prefer exploration **Approach B** (bundled UI kept; not full profile isolation).
- Pure profile delete remains Welcome → Profiles (`sdd.RemoveProfileAgents`).
- Full uninstall may retain global Engram intent when Engram is selected; pin mode-specific defaults in design.
- Fail-closed service: never coerce unknown/non-project scope to global.

## Scope

### In Scope

- Add `EngramUninstallScopeNone` (`"none"`) as first-class scope.
- Service: `none` → zero Engram ops; accept only `none`/`project`/`global`; unknown → error or no Engram ops (never global).
- TUI: default scope-selection / profile-aware entry paths to `none` (not Global).
- UI: show **No cleanup** when Engram scope UI is shown; truthful confirm/result copy for `none`.
- Lifecycle: reset, re-entry, back-nav, profile-discovery failure must not re-arm Global unexpectedly.
- Cursor/`optionCount` parity with rendered scope rows.
- Strict TDD for service, model, screens, and dispatch.

### Out of Scope

- Approach A (route selected profiles to profile-only remover).
- Approach C (remove profiles from uninstall entirely).
- Changing non-Engram component defaults or full agent wipe semantics beyond Engram scope defaults.
- CLI flag redesign beyond wiring the same scope contract if already profile-aware.

## Capabilities

### New Capabilities

- `uninstall-engram-scope`: Engram cleanup scope contract for managed uninstall (none/project/global), fail-closed service planning, TUI defaults/opt-in selection, and truthful confirm/result reporting on profile-aware uninstall flows.

### Modified Capabilities

- None

## Approach

**Approach B — service-first `none` default + opt-in cleanup**

1. Model constant `EngramUninstallScopeNone`.
2. Uninstall service plans zero Engram targets for `none`; stop fail-open non-project→global mapping.
3. TUI defaults on scope-selection entry/refresh to `none` when Engram scope is relevant; full mode may keep global when Engram selected (document).
4. Render No cleanup option; fix option cardinality/navigation; truthful confirm/result.
5. Guard lifecycle paths that previously stomped scope back to Global.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/model/types.go` | Modified | Add `none` scope constant |
| `internal/components/uninstall/service.go` | Modified | Fail-closed scope; zero Engram ops for `none` |
| `internal/tui/model.go` | Modified | Defaults, refresh, toggle, optionCount, lifecycle |
| `internal/tui/screens/uninstall.go` | Modified | Scope options, confirm/result copy |
| `internal/cli/uninstall.go`, `internal/app/app.go` | Modified | Pass-through only if scope wiring needed |
| `internal/tui/model_test.go`, `screens/uninstall_*_test.go`, `components/uninstall/service_test.go` | Modified | RED-first coverage |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Incomplete `none` wiring still maps to global | High | Service-first tests; fail-closed unknown |
| Silent Global when Engram UI hidden (no `.engram/`) | Med | Default `none` + explicit rules for hidden UI |
| Full wipe leaves Engram if default wrongly flipped | Med | Mode-specific defaults documented in design |
| Cursor/option drift with third option | Med | optionCount + key-nav tests |
| UX oversells “profile-only” while components selected | Low | Copy: Engram optional; components still apply |

## Rollback Plan

Revert the PR / branch `fix/445-profile-only-uninstall` to restore prior Global-default behavior. No schema migration; scope is in-memory/runtime only. If a bad release shipped, users reinstall Engram via normal install/setup; no automated data restore required for this UX fix itself.

## Dependencies

- Issue #445 approved expected behavior
- Existing Profiles delete path for pure profile removal (no change required)

## Success Criteria

- [ ] Profile-aware / scope-selection entry defaults Engram scope to `none`
- [ ] With `none`, managed uninstall performs zero Engram cleanup ops
- [ ] User can opt in to `project` or `global` explicitly
- [ ] Unknown scope never becomes global
- [ ] Confirm/result never imply Global when scope is `none`
- [ ] Reset/re-entry/discovery-error paths do not force Global
- [ ] `go test` / `go vet` green for touched packages
