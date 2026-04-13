# SDD Profiles Specification

## Purpose

Defines the full behavior of the SDD profiles feature: profile CRUD operations
from the TUI and CLI, agent generation with suffix naming, shared prompt file
management, and profile detection from `opencode.json`.

---

## Requirements

### Requirement: Profile Data Model

The system MUST represent a profile as a named set of model assignments: one
orchestrator model and an optional per-phase model map. The profile name MUST
be a non-empty slug (lowercase, alphanumeric, hyphens only). The default profile
(name `""` or `"default"`) represents the existing `sdd-orchestrator` agent.

| Field | Type | Required |
|-------|------|----------|
| Name | string (slug) | Yes |
| OrchestratorModel | ModelAssignment | Yes |
| PhaseAssignments | map[string]ModelAssignment | No |

#### Scenario: Profile creation with explicit phase models

- GIVEN a user creates a profile named `cheap` with Haiku as orchestrator
- AND assigns Sonnet to `sdd-apply`
- WHEN the profile is persisted
- THEN `OrchestratorModel` = `anthropic/claude-haiku-3.5-20241022`
- AND `PhaseAssignments["sdd-apply"]` = `anthropic/claude-sonnet-4-20250514`
- AND all other phases inherit the orchestrator model

#### Scenario: Sub-agent model inheritance

- GIVEN a profile has `OrchestratorModel = haiku` and no per-phase overrides
- WHEN agent JSON is generated
- THEN every sub-agent (`sdd-init-{name}` through `sdd-archive-{name}`) receives `haiku` as its model

---

### Requirement: Profile Name Validation

The system MUST enforce slug naming: lowercase letters, digits, and hyphens only.
Input MUST be auto-lowercased. The name `default` MUST be rejected (reserved).
Empty names MUST be rejected. The name `sdd-orchestrator` MUST be rejected
(would produce an ambiguous agent key).

| Input | Valid? | Action |
|-------|--------|--------|
| `cheap` | Yes | Accept |
| `premium-v2` | Yes | Accept |
| `my profile` | No | Reject — spaces |
| `default` | No | Reject — reserved |
| `LOUD` | Normalized | Auto-lowercase to `loud` |
| `sdd-orchestrator` | No | Reject — reserved prefix |
| `` (empty) | No | Reject — must be non-empty |

#### Scenario: Spaces rejected during creation

- GIVEN the user types `my profile` as a profile name in the TUI
- WHEN they press enter to confirm
- THEN the system rejects the input with an inline error
- AND the cursor remains on the name input field

#### Scenario: Reserved name rejected

- GIVEN the user enters `default` as a profile name
- WHEN they press enter to confirm
- THEN the system shows an error: "default is reserved"
- AND does not proceed to model selection

#### Scenario: Input auto-lowercased

- GIVEN the user types `PREMIUM`
- WHEN they press any key after the first character
- THEN the text field displays `premium` (silently normalized)

---

### Requirement: Agent Generation — Naming Convention

The system MUST generate agents in `opencode.json` following these naming rules:

- Default profile: `sdd-orchestrator` (orchestrator) + `sdd-{phase}` (sub-agents, 10 total)
- Named profile `{name}`: `sdd-orchestrator-{name}` + `sdd-{phase}-{name}` (10 sub-agents)

The SDD phases are: `init`, `explore`, `propose`, `spec`, `design`, `tasks`,
`apply`, `verify`, `archive`, `onboard` (10 total).

#### Scenario: Profile `cheap` generates correct agent keys

- GIVEN a profile named `cheap` is created
- WHEN agent generation runs
- THEN `opencode.json` contains keys: `sdd-orchestrator-cheap`, `sdd-init-cheap`,
  `sdd-explore-cheap`, `sdd-propose-cheap`, `sdd-spec-cheap`, `sdd-design-cheap`,
  `sdd-tasks-cheap`, `sdd-apply-cheap`, `sdd-verify-cheap`, `sdd-archive-cheap`,
  `sdd-onboard-cheap` (11 keys total)

---

### Requirement: Agent JSON Structure

The orchestrator agent for a profile MUST have `"mode": "primary"` so it appears
as selectable with Tab in OpenCode. Sub-agents MUST have `"mode": "subagent"` and
`"hidden": true`. The orchestrator's permission block MUST scope task delegation
to its own profile's sub-agents only.

```
sdd-orchestrator-{name}:
  mode: "primary"
  model: {orchestrator_model}
  prompt: {inlined orchestrator prompt with profile-specific model table}
  permission.task.*: "deny"
  permission.task.sdd-*-{name}: "allow"
  tools: read, write, edit, bash, delegate, delegation_read, delegation_list

sdd-{phase}-{name}:
  mode: "subagent"
  hidden: true
  model: {phase_model}
  prompt: "{file:~/.config/opencode/prompts/sdd/sdd-{phase}.md}"
```

#### Scenario: Orchestrator permission scoped to profile sub-agents

- GIVEN a profile named `gemini` exists
- WHEN the orchestrator agent is generated
- THEN `permission.task["*"]` = `"deny"`
- AND `permission.task["sdd-*-gemini"]` = `"allow"`

#### Scenario: Sub-agent prompt uses shared file reference

- GIVEN a profile `cheap` is generated
- WHEN the `sdd-apply-cheap` agent definition is read from `opencode.json`
- THEN its `prompt` field is `{file:~/.config/opencode/prompts/sdd/sdd-apply.md}`
  (or the expanded absolute path if `~` is not supported)

---

### Requirement: Orchestrator Prompt — Per-Profile Inlining

The orchestrator prompt MUST be inlined (not a `{file:...}` reference) and MUST
contain a model assignments table specific to that profile. Sub-agent references
within the prompt MUST use the `sdd-{phase}-{name}` suffix form.

The system MUST perform string replacement of `sdd-{phase}` → `sdd-{phase}-{name}`
within the model assignments table and delegation rules sections only.

#### Scenario: Orchestrator prompt references correct sub-agents

- GIVEN a profile named `cheap` is created with Haiku everywhere
- WHEN the orchestrator prompt is generated
- THEN the model assignments table lists `sdd-apply-cheap`, `sdd-verify-cheap`, etc.
- AND the delegation rules reference `sdd-*-cheap` sub-agents

---

### Requirement: Shared Prompt Files

Sub-agent prompts MUST be extracted from the inline overlay to files at
`~/.config/opencode/prompts/sdd/sdd-{phase}.md`. These files are shared across
all profiles and MUST contain the same content as today's inline prompt.

The set of shared prompt files is:
`sdd-init.md`, `sdd-explore.md`, `sdd-propose.md`, `sdd-spec.md`,
`sdd-design.md`, `sdd-tasks.md`, `sdd-apply.md`, `sdd-verify.md`,
`sdd-archive.md`, `sdd-onboard.md` (10 files).

#### Scenario: Prompt files written on first sync after feature ships

- GIVEN a user has no `~/.config/opencode/prompts/sdd/` directory
- WHEN they run sync after updating to the version that ships this feature
- THEN the directory is created and 10 prompt files are written
- AND sub-agents in `opencode.json` are updated to use `{file:...}` references
- AND the effective prompt content is identical to what was previously inlined

#### Scenario: Prompt files survive profile deletion

- GIVEN profiles `cheap` and `gemini` exist, and prompt files are present
- WHEN the user deletes the `cheap` profile
- THEN `~/.config/opencode/prompts/sdd/` and all 10 prompt files remain
- AND the `gemini` profile continues to work correctly

---

### Requirement: Profile Detection from opencode.json

The system MUST detect existing profiles by reading `opencode.json` and scanning
for agent keys matching the pattern `sdd-orchestrator-{name}` with `"mode": "primary"`.
`opencode.json` is the single source of truth — no separate profile state file.

The default profile is always present when SDD multi-mode is configured
(`sdd-orchestrator` without suffix).

#### Scenario: Detect profiles on startup

- GIVEN `opencode.json` contains `sdd-orchestrator-cheap` and `sdd-orchestrator-gemini`
  with `"mode": "primary"`
- WHEN the profile list screen is opened
- THEN the screen shows `cheap` and `gemini` as detected profiles
- AND their orchestrator model is read from the `"model"` field

#### Scenario: Infer sub-agent models from JSON

- GIVEN `sdd-orchestrator-cheap` and `sdd-apply-cheap` exist in `opencode.json`
- WHEN the user enters edit mode for profile `cheap`
- THEN the sub-agent model picker pre-populates `sdd-apply` with the model from `sdd-apply-cheap`

---

### Requirement: Profile CRUD — Create

Creating a profile MUST follow this flow:
1. User enters a valid name
2. User selects orchestrator model (reuses ModelPicker)
3. User assigns sub-agent models (reuses ModelPicker rows; can set all at once)
4. User confirms → system generates agents in `opencode.json` → sync runs

#### Scenario: Successful profile creation

- GIVEN the user completes the 4-step create flow with name `cheap` and Haiku
- WHEN they select "Create & Sync"
- THEN `opencode.json` gains 11 new keys (`sdd-orchestrator-cheap` + 10 sub-agents)
- AND sync runs automatically
- AND the profile list screen reloads showing `cheap`

#### Scenario: Duplicate name — overwrite prompt

- GIVEN a profile named `cheap` already exists
- WHEN the user creates a new profile with the same name `cheap`
- THEN the system asks "Profile 'cheap' already exists. Overwrite?"
- AND if confirmed, the existing profile is overwritten with new model assignments

---

### Requirement: Profile CRUD — Edit

Editing a profile reuses the creation flow with pre-populated values. The name
MUST NOT be changeable during edit. On confirm, the profile's agents are overwritten
in `opencode.json` and sync runs.

The default profile (`sdd-orchestrator`) MUST be editable (equivalent to the
existing "Configure Models → OpenCode" flow).

#### Scenario: Edit flow pre-populates current models

- GIVEN profile `cheap` has Haiku as orchestrator
- WHEN the user presses enter on `cheap` in the profile list
- THEN the edit screen shows the profile name as a fixed header ("Edit Profile 'cheap'")
- AND the orchestrator model picker starts with Haiku pre-selected

#### Scenario: Default profile editable

- GIVEN the user navigates to the profile list and presses enter on `default`
- WHEN they change the orchestrator model and confirm
- THEN `sdd-orchestrator`'s model in `opencode.json` is updated
- AND sync runs

---

### Requirement: Profile CRUD — Delete

Pressing `d` on a non-default profile in the list MUST show a confirmation screen
listing all agents to be removed. On confirm, ALL 11 agent keys are removed from
`opencode.json` atomically and sync runs.

The default profile MUST NOT be deletable. Pressing `d` on `default` MUST be a no-op.

#### Scenario: Delete removes all profile agents

- GIVEN profile `cheap` exists with 11 agents in `opencode.json`
- WHEN the user presses `d` on `cheap` and confirms
- THEN all 11 keys (`sdd-orchestrator-cheap`, `sdd-init-cheap`, ..., `sdd-onboard-cheap`)
  are removed from `opencode.json`
- AND the write is atomic (temp file swap)
- AND sync runs
- AND the profile list reloads without `cheap`

#### Scenario: Delete blocked for default profile

- GIVEN the cursor is on the `default` profile
- WHEN the user presses `d`
- THEN nothing happens (no confirmation screen shown)

#### Scenario: Shared prompt files not deleted with profile

- GIVEN profile `cheap` is deleted
- WHEN deletion completes
- THEN `~/.config/opencode/prompts/sdd/` directory and all prompt files still exist

---

### Requirement: TUI — Profile List Screen

The profile list screen MUST display all detected profiles. Keybindings:
- `j`/`k` or arrow keys: navigate
- `enter` on a profile: edit mode
- `n` anywhere: create new profile
- `d` on a non-default profile: delete confirmation
- `esc`: back to Welcome

Each profile row MUST show: name + orchestrator model. The default profile MUST
be visually distinguished (e.g., with a `✦` marker).

#### Scenario: Profile list renders correctly

- GIVEN profiles `default` (claude-opus-4), `cheap` (haiku), `gemini` (gemini-2.5-pro)
- WHEN the profile list screen renders
- THEN three rows appear with the correct model displayed per profile
- AND `default` is marked distinctly

---

### Requirement: TUI — Profile Create Screen (Name Input)

The name input MUST validate on every keypress. Rejected characters (spaces,
uppercase) MUST be silently dropped or auto-corrected. Pressing `enter` on an
empty input MUST show an inline error.

#### Scenario: Model cache not available

- GIVEN `~/.cache/opencode/models.json` does not exist
- WHEN the user navigates to the profile create screen
- THEN a message "Run OpenCode at least once to populate the model cache" is shown
- AND only a "Back" option is available
- AND the rest of the TUI is unaffected

---

### Requirement: CLI `--profile` Flag

The `sync` command MUST accept `--profile name:provider/model` to create or update
a profile during headless sync. Multiple `--profile` flags MUST be accepted.

The `--profile-phase` flag MUST accept `name:phase:provider/model` to assign
a model to a specific sub-agent within a named profile.

#### Scenario: Headless profile creation via CLI

- GIVEN no profile named `cheap` exists
- WHEN `gentle-ai sync --profile cheap:anthropic/claude-haiku-3.5-20241022` runs
- THEN the `cheap` profile is created in `opencode.json` with Haiku for all sub-agents

#### Scenario: Multiple profiles in one sync

- GIVEN `gentle-ai sync --profile cheap:haiku --profile premium:opus` runs
- WHEN sync completes
- THEN both `cheap` and `premium` profiles are present in `opencode.json`
