# Tasks: OpenCode SDD Profiles

## Phase 1: Shared Prompt Refactor (Foundation — Zero Behavioral Change)

- [x] 1.1 **[RED]** Write failing tests for `WriteSharedPromptFiles` in `internal/components/sdd/prompts_test.go`: verify 10 prompt files created in temp dir, idempotent (second call returns changed=false), content matches embedded assets
- [x] 1.2 **[GREEN]** Create `internal/components/sdd/prompts.go` — implement `WriteSharedPromptFiles(homeDir string) (bool, error)` and `SharedPromptDir(homeDir string) string`; extract 10 sub-agent prompt strings from inline/assets into `~/.config/opencode/prompts/sdd/*.md`
- [x] 1.3 **[REFACTOR]** Update `internal/assets/opencode/sdd-overlay-multi.json` — replace inline sub-agent prompt strings with `{file:<absolute-path>/sdd-{phase}.md}` placeholders; update `inlineOpenCodeSDDPrompts` in `inject.go` to skip sub-agents (they now use file refs)
- [x] 1.4 **[GREEN]** Wire `WriteSharedPromptFiles` into `sdd.Inject()` — call before overlay merge for OpenCode multi-mode; returned `changed` propagates to `InjectionResult`
- [x] 1.5 **[TEST]** Integration: run full inject with SDDModeMulti on a temp `opencode.json`; assert sub-agent prompt fields contain `{file:...}`, orchestrator prompt remains inlined, `filesChanged=0` on second call

## Phase 2: Profile Data Model & Generation

- [x] 2.1 Add `Profile` struct to `internal/model/types.go`: `Name string`, `OrchestratorModel ModelAssignment`, `PhaseAssignments map[string]ModelAssignment`
- [x] 2.2 Add `Profiles []Profile` to `SyncOverrides` in `internal/model/selection.go`
- [x] 2.3 **[RED]** Write failing tests for `ValidateProfileName` and `ProfileAgentKeys` in `internal/components/sdd/profiles_test.go`: reserved names (`default`, `sdd-orchestrator`), slug rules, empty/space rejection, auto-lowercase, correct 11-key list
- [x] 2.4 **[RED]** Write failing tests for `DetectProfiles`: given a JSON with `sdd-orchestrator-cheap` (mode:primary) + sub-agents, returns 1 profile with name `cheap` and correct model assignments; handles missing file gracefully
- [x] 2.5 **[RED]** Write failing golden-file tests for `GenerateProfileOverlay`: given Profile{Name:"cheap", OrchestratorModel:haiku}, output JSON has `sdd-orchestrator-cheap` (mode:primary, permission scoped to `sdd-*-cheap`), 10 sub-agents `sdd-{phase}-cheap` (mode:subagent, hidden:true, `{file:...}` prompt refs), orchestrator prompt inlined with suffix-replaced sub-agent references
- [x] 2.6 **[RED]** Write failing tests for `RemoveProfileAgents`: given JSON with cheap+default profiles, removes exactly 11 cheap keys, preserves all default keys, writes atomically
- [x] 2.7 **[GREEN]** Create `internal/components/sdd/profiles.go` — implement all 5 functions: `ValidateProfileName`, `ProfileAgentKeys`, `DetectProfiles`, `GenerateProfileOverlay`, `RemoveProfileAgents`
- [x] 2.8 **[REFACTOR]** Update `inject.go` to iterate `opts.Profiles` (from `InjectOptions`): for each Profile, call `GenerateProfileOverlay` then `mergeJSONFile`; add `Profiles []model.Profile` to `InjectOptions`
- [x] 2.9 Modify `internal/components/sdd/read_assignments.go` — add `DetectProfiles` wrapper that calls `sdd.DetectProfiles(settingsPath)` and returns detected profiles for sync-time regeneration

## Phase 3: TUI Screens — Profile List & Create

- [x] 3.1 Add screen constants to `internal/tui/model.go`: `ScreenProfiles`, `ScreenProfileCreate`, `ScreenProfileDelete`; add state fields: `ProfileList []model.Profile`, `ProfileCursor int`, `ProfileCreateStep int` (0=name, 1=orch-model, 2=subagent-models, 3=confirm), `ProfileDraft model.Profile`, `ProfileDeleteTarget string`
- [x] 3.2 Add routes to `internal/tui/router.go`: `ScreenProfiles{Backward: ScreenWelcome}`, `ScreenProfileCreate{Backward: ScreenProfiles}`, `ScreenProfileDelete{Backward: ScreenProfiles}`
- [x] 3.3 **[RED]** Write teatest test for `ScreenProfiles` in `internal/tui/screens/profiles_test.go`: renders profile list with ✦ for default, navigates with j/k, `n` transitions to create, `d` on non-default shows delete, `d` on default is no-op, `esc` goes back
- [x] 3.4 **[GREEN]** Create `internal/tui/screens/profiles.go` — profile list screen: renders existing profiles with ✦ default marker, "Create new profile" action, "Back" action; handles j/k navigation, enter (→ edit), n (→ create), d (→ delete, guards default), esc (→ welcome)
- [x] 3.5 **[RED]** Write teatest test for `ScreenProfileCreate` step flow: name input validates and rejects reserved/invalid names, enter advances to orchestrator picker (reuses `ModelPickerState`), sub-agent picker step, confirm step shows summary
- [x] 3.6 **[GREEN]** Create `internal/tui/screens/profile_create.go` — 4-step creation flow: (1) name text input with validation, (2) orchestrator model picker (reuse `ModelPickerState`), (3) sub-agent models picker (10 phases + "Set all"), (4) confirm screen with agent count; triggers `SyncFn` with `SyncOverrides{Profiles: []Profile{draft}}`
- [x] 3.7 Wire profile screens into `model.go` Update/View/Init dispatch: load `ProfileList` via `sdd.DetectProfiles` on entry to `ScreenProfiles`; handle `SyncDoneMsg` to refresh list; update key-handling `Update` for all 3 new screens
- [x] 3.8 Modify `internal/tui/screens/welcome.go` — add "OpenCode SDD Profiles" option between "Configure Models" and "Manage Backups"; show `(N)` badge where N = count of non-default profiles (0 = no badge); only show option when OpenCode is installed

## Phase 4: TUI Screens — Profile Edit & Delete

- [x] 4.1 **[RED]** Write teatest test for edit flow: pressing enter on existing profile pre-populates models in picker, name shown as fixed header (not editable), save triggers sync with updated Profile
- [x] 4.2 **[GREEN]** Extend `internal/tui/screens/profile_create.go` to support edit mode: `EditMode bool`, `OriginalName string`; step 1 shows name as read-only header; steps 2–4 identical to create but pre-populated with current model assignments from `ProfileDraft`
- [x] 4.3 **[RED]** Write teatest test for `ScreenProfileDelete`: renders profile name, lists all 11 agent keys, "Delete & Sync" and "Cancel" options; cancel returns to list; confirm calls `RemoveProfileAgents` then sync
- [x] 4.4 **[GREEN]** Create `internal/tui/screens/profile_delete.go` — confirmation screen: shows profile name, agent key list (`sdd-orchestrator-{name}` + 10 sub-agents), "Delete & Sync" / "Cancel"; confirm calls `sdd.RemoveProfileAgents` then triggers `SyncFn`; success returns to `ScreenProfiles` with refreshed list
- [x] 4.5 Wire edit/delete transitions in `model.go` Update: entering `ScreenProfileCreate` with `EditMode=true` populates `ProfileDraft` from selected profile; `ScreenProfileDelete` sets `ProfileDeleteTarget`; handle post-delete `SyncDoneMsg` → navigate to `ScreenProfiles`

## Phase 5: Sync Integration & CLI

- [x] 5.1 **[RED]** Write unit test for `ParseSyncFlags` with `--profile cheap:anthropic/claude-haiku-3.5-20241022` and `--profile-phase cheap:sdd-apply:anthropic/claude-sonnet-4-20250514`: assert `SyncFlags.Profiles` populated correctly, invalid format returns error
- [x] 5.2 Add `Profiles []ProfileFlag` to `SyncFlags` in `internal/cli/sync.go`; implement `--profile` and `--profile-phase` multi-value flags; parse `name:provider/model` format; pass resulting `[]model.Profile` into `SyncOverrides`
- [x] 5.3 Update sync pipeline in `internal/cli/sync.go`: when `SyncOverrides.Profiles` is non-empty, call `sdd.DetectProfiles` to merge with existing profiles; pass all profiles to `sdd.Inject` via `InjectOptions.Profiles`
- [x] 5.4 Update `syncBackupTargets` in `internal/cli/sync.go` to include `~/.config/opencode/prompts/sdd/` directory in pre-sync backup scope (per R-PROF-32)
- [x] 5.5 **[TEST]** Integration: `RunSyncWithSelection` with 3 profiles — assert all 33 profile sub-agent keys present in resulting `opencode.json`, model assignments preserved, prompts files written, `filesChanged=0` on idempotent re-sync

## Phase 6: Polish & Edge Cases

- [x] 6.1 **[TEST]** E2E: profile creation, sync, list display, edit (model change), re-sync, delete, confirm deletion removed from JSON — verify agent keys before/after each operation
- [x] 6.2 Handle missing OpenCode model cache edge case in `ScreenProfileCreate`: if `~/.cache/opencode/models.json` does not exist, show "Run OpenCode at least once to populate the model cache" message and only offer "Back"
- [x] 6.3 Handle profile name collision in `ScreenProfileCreate`: if entered name already exists in `ProfileList`, show overwrite confirmation prompt before proceeding to model picker
- [x] 6.4 Handle sync-time missing model warning (R-PROF-31): if profile sub-agent model not found in OpenCode model cache, log warning and preserve existing assignment — do NOT error
- [x] 6.5 Verify post-injection check in `inject.go` covers profile orchestrators: extend the `strings.Contains(settingsText, ...)` post-check to verify `sdd-orchestrator-{name}` for each injected profile
- [x] 6.6 **[TEST]** TUI snapshot tests for welcome screen: no profiles (no badge), 1 profile (badge shows 1), 3 profiles (badge shows 3)
