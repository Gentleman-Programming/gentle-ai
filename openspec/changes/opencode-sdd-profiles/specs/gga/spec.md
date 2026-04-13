# Delta for GGA

## ADDED Requirements

### Requirement: Welcome Screen — OpenCode SDD Profiles Option

The Welcome screen MUST include a new menu option "OpenCode SDD Profiles"
positioned after "Configure Models". When at least one non-default profile
exists, the option MUST display the current profile count as a badge:
`"OpenCode SDD Profiles (N)"`. Selecting this option navigates to the
profile list screen.

#### Scenario: Welcome shows profile count badge

- GIVEN two non-default profiles (`cheap`, `gemini`) exist in `opencode.json`
- WHEN the Welcome screen renders
- THEN the menu option reads `"OpenCode SDD Profiles (2)"`

#### Scenario: Welcome shows option without badge when no profiles exist

- GIVEN no non-default profiles exist (only the default `sdd-orchestrator`)
- WHEN the Welcome screen renders
- THEN the menu option reads `"OpenCode SDD Profiles"` (no badge)

#### Scenario: Selecting option navigates to profile list

- GIVEN the cursor is on "OpenCode SDD Profiles"
- WHEN the user presses enter
- THEN the TUI navigates to `ScreenProfiles`

---

### Requirement: Sync `--profile` Flag

The `sync` CLI subcommand MUST accept a `--profile name:provider/model` flag.
Multiple instances of the flag on the same invocation MUST be accepted and
each creates or updates the named profile. Profile creation via `--profile`
MUST produce the same agent structure as TUI-based creation.

A companion `--profile-phase` flag MUST accept `name:phase:provider/model`
to assign an individual phase model within a named profile.

#### Scenario: `--profile` creates new profile during sync

- GIVEN `cheap` does not exist in `opencode.json`
- WHEN `gentle-ai sync --profile cheap:anthropic/claude-haiku-3.5-20241022` runs
- THEN `sdd-orchestrator-cheap` and 10 sub-agents are added to `opencode.json`
- AND the sync proceeds normally

#### Scenario: `--profile` flag with invalid format rejected

- GIVEN `gentle-ai sync --profile badformat` is run (no colon separator)
- WHEN argument parsing runs
- THEN the command exits with a usage error: "invalid --profile format: expected name:provider/model"

#### Scenario: `--profile-phase` overrides a specific sub-agent model

- GIVEN `gentle-ai sync --profile cheap:haiku --profile-phase cheap:sdd-apply:sonnet`
- WHEN sync runs
- THEN `sdd-apply-cheap.model = sonnet`
- AND all other `cheap` sub-agents use `haiku`
