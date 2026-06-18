# Proposal: OpenCode SDD Profiles

## Intent

Users cannot switch between model configurations (premium/cheap/experimental) without manually editing `opencode.json`. This change adds profile CRUD from the TUI — each profile generates its own orchestrator + suffixed sub-agents, selectable with Tab in OpenCode.

## Scope

### In Scope
- Profile list screen (view, navigate, edit, delete actions)
- Profile creation flow (name -> orchestrator model -> sub-agent models -> confirm + sync)
- Profile edit flow (modify models of existing profile + sync)
- Profile delete flow (confirmation -> remove agents from JSON -> sync)
- Shared prompt files: extract sub-agent prompts to `~/.config/opencode/prompts/sdd/*.md` with `{file:...}` refs
- Profile agent generation: 1 orchestrator + 10 sub-agents per profile with suffix naming
- Sync integration: detect all profiles, update shared prompts, preserve model assignments
- CLI `--profile` flag for headless profile creation during sync
- Welcome screen: new "OpenCode SDD Profiles" option with count badge

### Out of Scope
- Claude Code profiles (permanently — depends on opencode.json agent system)
- Export/import profiles between machines (future)
- Profile-specific skill files (prompts are shared; only models differ)

## Capabilities

### New Capabilities
- `sdd-profiles`: Profile CRUD (create, list, edit, delete) from TUI + CLI, agent generation with suffix naming, shared prompt file management
- `sdd-profile-sync`: Sync-time profile detection, prompt file maintenance, per-profile orchestrator regeneration

### Modified Capabilities
- `gga`: Welcome screen gains "OpenCode SDD Profiles" option; sync flow gains `--profile` flag and multi-profile awareness

## Approach

Six-phase implementation following the PRD:

1. **Shared prompt refactor** — Extract sub-agent prompts from inline overlay to `~/.config/opencode/prompts/sdd/*.md`. Update `inject.go` to write files + use `{file:...}` refs. Zero behavioral change.
2. **Profile data model + generation** — Add `Profile` struct to `internal/model/types.go`. New `internal/components/sdd/profiles.go` for CRUD: generate/detect/delete agents in opencode.json.
3. **TUI: profile list + create** — New screens (`profiles.go`, `profile_create.go`). Reuse existing ModelPicker. Wire into Welcome screen + router.
4. **TUI: edit + delete** — `profile_delete.go` confirm screen. Edit reuses creation flow with pre-populated values. Default profile: editable, not deletable.
5. **Sync integration** — Detect `sdd-orchestrator-*` pattern. Update prompts, regenerate orchestrator per-profile. Add `--profile` CLI flag. Backup coverage for prompt files.
6. **Polish + testing** — E2E tests, edge cases (missing cache, reserved names, idempotent sync).

Key architecture decisions:
- Orchestrator prompts are **inlined per-profile** (profile-specific model table + sub-agent refs)
- Sub-agent prompts are **shared files** (identical across profiles, only model field changes)
- `opencode.json` is the **single source of truth** — no separate profile state file

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/model/types.go` | Modified | Add `Profile` struct |
| `internal/model/selection.go` | Modified | Add `Profiles []Profile` to `Selection` and `SyncOverrides` |
| `internal/tui/model.go` | Modified | Add profile screen constants + state fields |
| `internal/tui/router.go` | Modified | Add routes for profile screens |
| `internal/tui/screens/welcome.go` | Modified | Add "OpenCode SDD Profiles" option |
| `internal/tui/screens/profiles.go` | New | Profile list screen |
| `internal/tui/screens/profile_create.go` | New | Profile creation/edit flow |
| `internal/tui/screens/profile_delete.go` | New | Delete confirmation screen |
| `internal/components/sdd/inject.go` | Modified | Extract prompts to files, handle profile generation |
| `internal/components/sdd/profiles.go` | New | Profile CRUD: generate, detect, delete agents |
| `internal/components/sdd/prompts.go` | New | Shared prompt file management |
| `internal/components/sdd/read_assignments.go` | Modified | Profile detection from opencode.json |
| `internal/cli/sync.go` | Modified | `--profile` flag, multi-profile sync |
| `internal/assets/opencode/sdd-overlay-multi.json` | Modified | Refactor to `{file:...}` references |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| OpenCode `{file:...}` doesn't support `~` expansion | Med | Expand to absolute path during generation; validate early in Phase 1 |
| Large profile count degrades sync performance | Low | Test with 10 profiles; JSON merge is O(agents), already fast |
| Breaking change to overlay format during prompt extraction | Med | Phase 1 is zero-behavioral-change refactor; E2E tests validate before/after |
| Name collisions with user-defined agents in opencode.json | Low | Prefix all profile agents with `sdd-`; deep merge preserves non-SDD keys |

## Rollback Plan

1. **Phase 1 (prompt extraction)**: Revert to inline prompts by restoring `sdd-overlay-multi.json` from backup. Prompt files in `~/.config/opencode/prompts/sdd/` are harmless orphans.
2. **Phase 2-6 (profiles)**: Remove profile agents from `opencode.json` by running sync without profile support (overlay merge only writes default agents). Delete prompt files manually or via next sync.
3. **Full revert**: `git revert` the feature branch. User runs `gentle-ai sync` to restore clean state.

## Dependencies

- OpenCode `{file:path}` syntax for prompt references (validate in Phase 1)
- OpenCode model cache at `~/.cache/opencode/models.json` (existing dependency, already handled)

## Success Criteria

- [ ] Profile creation from TUI completes in < 60 seconds
- [ ] Sync with 3 profiles adds < 5 seconds overhead
- [ ] Users without profiles see zero behavioral change (backward compatible)
- [ ] All profile orchestrators appear as selectable with Tab in OpenCode
- [ ] Sync is idempotent: re-sync with no changes produces `filesChanged = 0`
- [ ] Default profile cannot be deleted; can be edited
- [ ] E2E tests cover create, edit, delete, and sync flows
