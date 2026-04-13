# Design: OpenCode SDD Profiles

## Technical Approach

Profiles are a layer ON TOP of the existing multi-mode SDD injection. Each profile generates 1 orchestrator + 10 sub-agents with suffixed keys (e.g. `sdd-apply-cheap`). Sub-agent prompts are extracted to shared `{file:...}` references; orchestrator prompts remain inlined per-profile (profile-specific model table + sub-agent references). The `opencode.json` agent map is the single source of truth — no separate profile state file.

## Architecture Decisions

| Decision | Choice | Alternatives | Rationale |
|----------|--------|-------------|-----------|
| Profile source of truth | `opencode.json` agent keys (detect via `sdd-orchestrator-*` pattern) | Separate `profiles.json` state file | PRD R-PROF-40: JSON IS the truth. Avoids sync drift between two files. Detection is cheap (scan keys). |
| Orchestrator prompt storage | Inlined per-profile in JSON | Shared file with placeholder replacement | Each profile needs unique model assignments table + suffixed sub-agent refs. A shared template would need runtime rendering that `{file:...}` doesn't support. |
| Sub-agent prompt storage | Shared files at `~/.config/opencode/prompts/sdd/*.md` via `{file:...}` | Keep inlining in JSON | PRD R-PROF-20: prompts identical across profiles (only model differs). Shared files eliminate N×10 duplicated prompt strings. |
| Profile struct location | `model.Profile` in `internal/model/types.go` | Nested inside SDD component package | Profile is a domain concept passed through TUI → sync → inject. Following existing pattern (ModelAssignment, Selection live in `model`). |
| Prompt file path format | Absolute expanded path (not `~`) | `~` tilde syntax | OpenCode `{file:...}` tilde support is unverified (PRD open question #4). Expanding to absolute at generation time is safe. |
| Profile CRUD ownership | New `internal/components/sdd/profiles.go` | TUI screens do JSON manipulation directly | Separation of concerns: TUI screens manage navigation/state, `profiles.go` handles agent generation/detection/deletion as pure functions. |

## Data Flow

### Profile Creation
```
TUI ScreenProfileCreate
  → name input + model picker (reuse existing)
  → Profile struct built in TUI state
  → confirmSelection triggers sync
     ↓
SyncOverrides.Profiles = []Profile{newProfile}
  → componentSyncStep(ComponentSDD)
     → sdd.Inject()
        → writeSharedPromptFiles()   (idempotent)
        → generateProfileAgents()    (build overlay for this profile)
        → mergeJSONFile()            (deep merge into opencode.json)
```

### Profile Detection (sync-time)
```
opencode.json
  → readExistingAgentModels() returns all keys
  → DetectProfiles() scans for sdd-orchestrator-{name} pattern
  → returns []Profile with name + model assignments extracted
  → sync loops: for each profile → regenerate orchestrator prompt → preserve models
```

### Profile Deletion
```
TUI ScreenProfileDelete → confirm
  → RemoveProfileAgents(name) → reads JSON, deletes 11 keys, atomic write
  → sync to ensure consistency
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/model/types.go` | Modify | Add `Profile` struct |
| `internal/model/selection.go` | Modify | Add `Profiles []Profile` to `SyncOverrides` |
| `internal/components/sdd/profiles.go` | Create | `DetectProfiles`, `GenerateProfileOverlay`, `RemoveProfileAgents`, `ValidateProfileName` |
| `internal/components/sdd/prompts.go` | Create | `WriteSharedPromptFiles`, `SharedPromptDir` — extract embedded prompts to `~/.config/opencode/prompts/sdd/` |
| `internal/components/sdd/inject.go` | Modify | Call `writeSharedPromptFiles`; replace inline sub-agent prompts with `{file:...}` refs; iterate profiles during inject |
| `internal/components/sdd/read_assignments.go` | Modify | Add `DetectProfiles` that wraps profile detection from agent keys |
| `internal/tui/screens/profiles.go` | Create | Profile list screen: render, optionCount, key handling (enter→edit, d→delete, n→new) |
| `internal/tui/screens/profile_create.go` | Create | Multi-step creation: name input → orchestrator picker → sub-agent picker → confirm. Reuses `ModelPickerState` |
| `internal/tui/screens/profile_delete.go` | Create | Delete confirmation screen with agent list |
| `internal/tui/model.go` | Modify | Add `ScreenProfiles`, `ScreenProfileCreate`, `ScreenProfileDelete` + state fields + key handling + optionCount |
| `internal/tui/router.go` | Modify | Add routes for 3 new profile screens |
| `internal/tui/screens/welcome.go` | Modify | Add "OpenCode SDD Profiles" option between "Configure models" and "Manage backups"; show count badge |
| `internal/cli/sync.go` | Modify | Add `--profile` flag to `SyncFlags`; pass profiles through to SDD inject; update `syncBackupTargets` to include prompt dir |
| `internal/assets/opencode/sdd-overlay-multi.json` | Modify | Sub-agent prompts → `{file:...}` references (Phase 1 zero-change refactor) |

## Interfaces / Contracts

```go
// internal/model/types.go
type Profile struct {
    Name              string
    OrchestratorModel ModelAssignment
    PhaseAssignments  map[string]ModelAssignment // key = phase name (e.g. "sdd-apply")
}

// internal/components/sdd/profiles.go
func DetectProfiles(settingsPath string) ([]Profile, error)
func GenerateProfileOverlay(profile Profile, homeDir string) ([]byte, error) // returns JSON overlay for one profile
func RemoveProfileAgents(settingsPath string, profileName string) error
func ValidateProfileName(name string) error
func ProfileAgentKeys(name string) []string // returns the 11 keys for a profile

// internal/components/sdd/prompts.go
func WriteSharedPromptFiles(homeDir string) (changed bool, err error)
func SharedPromptDir(homeDir string) string
```

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | `ValidateProfileName` — reserved names, slug rules, edge cases | Table-driven Go tests |
| Unit | `DetectProfiles` — parse agent keys, extract models, handle malformed JSON | Mock opencode.json content |
| Unit | `GenerateProfileOverlay` — correct keys, suffixed names, permission scoping, `{file:...}` refs | Golden-file comparison |
| Unit | `RemoveProfileAgents` — removes exactly 11 keys, preserves others, atomic write | Temp file + verify JSON |
| Unit | `WriteSharedPromptFiles` — idempotent, correct content, creates dir | Temp dir + hash comparison |
| Integration | Sync with 3 profiles — all agents present, prompts current, models preserved | `RunSyncWithSelection` with Profile overrides |
| TUI | Profile list navigation, creation flow, delete confirmation | teatest with `WaitForValue` |
| TUI | Welcome screen option count with/without profiles | teatest snapshot |

## Migration / Rollout

**Phase 1 (shared prompts)** is a zero-behavioral-change refactor. On first sync after update:
1. Creates `~/.config/opencode/prompts/sdd/` directory
2. Writes 10 prompt `.md` files (extracted from current inline content)
3. Updates sub-agent entries in overlay to use `{file:<absolute-path>/sdd-init.md}` references
4. Deep merge preserves ALL existing model assignments and user customizations

**Backward compatibility**: Users without profiles see no changes. The `sdd-overlay-multi.json` still produces the same agent keys (`sdd-orchestrator`, `sdd-init`, ..., `sdd-archive`). Only the prompt delivery mechanism changes (inline → file reference).

**Users with existing multi-mode**: Their model assignments in `opencode.json` are preserved by the deep merge. Prompts silently migrate from inline to `{file:...}` — no disruption.

## Open Questions

- [ ] Validate OpenCode `{file:...}` tilde expansion — if unsupported, use absolute paths (mitigation already in design)
- [ ] Confirm `{file:...}` is resolved relative to `opencode.json` location or absolute — determines path format
