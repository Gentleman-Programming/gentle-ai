# SDD Profile Sync Specification

## Purpose

Defines the behavior of sync when profiles exist: detection of existing profiles,
maintenance of shared prompt files, per-profile orchestrator regeneration, model
preservation, and backup coverage for prompt files.

---

## Requirements

### Requirement: Profile Detection During Sync

During sync, the system MUST detect all existing profiles by scanning `opencode.json`
for agent keys matching `sdd-orchestrator-{name}` with `"mode": "primary"`.
The default profile (`sdd-orchestrator` without suffix) is always treated as present
when SDD multi-mode is active.

#### Scenario: Sync detects profiles and updates them

- GIVEN `opencode.json` contains `sdd-orchestrator-cheap` and `sdd-orchestrator-gemini`
- WHEN sync runs
- THEN both profiles are detected and updated (prompts refreshed, models preserved)

---

### Requirement: Shared Prompt File Maintenance

During sync, the system MUST write/update the shared prompt files at
`~/.config/opencode/prompts/sdd/sdd-{phase}.md` from the embedded assets.
The write MUST be atomic and idempotent (no change = `filesChanged` not incremented).

#### Scenario: Prompt files updated on sync

- GIVEN shared prompt files exist and one has stale content
- WHEN sync runs
- THEN the stale file is updated atomically
- AND `filesChanged` increments by 1

#### Scenario: Idempotent sync — no changes

- GIVEN shared prompt files exist and all content matches embedded assets
- WHEN sync runs
- THEN no prompt files are written
- AND `filesChanged` does not increment for prompt files

---

### Requirement: Per-Profile Orchestrator Regeneration

For each detected profile, sync MUST regenerate the orchestrator's inline prompt
(to inject the updated model assignments table) while preserving the `model` field.
Sub-agent prompts are auto-updated via `{file:...}` references (step 2 above).

#### Scenario: Orchestrator prompt regenerated, model preserved

- GIVEN profile `cheap` has `sdd-orchestrator-cheap.model = haiku`
- WHEN sync runs after an update to the orchestrator prompt template
- THEN `sdd-orchestrator-cheap.prompt` is updated with the new template content
- AND `sdd-orchestrator-cheap.model` remains `haiku`

---

### Requirement: Model Preservation During Sync

Sync MUST NOT modify the `model` field of any existing profile orchestrator or
sub-agent. Model changes are only allowed via explicit TUI edit or CLI `--profile` flag.

#### Scenario: Model not overwritten during sync

- GIVEN `sdd-orchestrator-gemini.model = google/gemini-2.5-pro`
- AND sync runs without a `--profile gemini:*` override
- THEN `sdd-orchestrator-gemini.model` remains `google/gemini-2.5-pro` after sync

---

### Requirement: Missing Model Warning

If a profile sub-agent references a model that no longer exists in
`~/.cache/opencode/models.json`, sync MUST emit a warning and preserve the existing
model assignment. This MUST NOT be a hard error.

#### Scenario: Stale model ID preserved with warning

- GIVEN `sdd-apply-cheap.model = anthropic/old-model-id` and the model cache
  does not contain `old-model-id`
- WHEN sync runs
- THEN a warning is emitted: "sdd-apply-cheap references unknown model old-model-id"
- AND `sdd-apply-cheap.model` is NOT changed

---

### Requirement: Backup Coverage for Prompt Files

The pre-sync backup MUST include the `~/.config/opencode/prompts/sdd/` directory
alongside `opencode.json`. If the directory does not yet exist, it is silently skipped.

#### Scenario: Prompt files backed up before sync

- GIVEN shared prompt files exist at `~/.config/opencode/prompts/sdd/`
- WHEN sync runs and creates a pre-sync backup
- THEN the backup snapshot includes the prompt directory contents
- AND the backup can be restored to return prompts to their pre-sync state

---

### Requirement: Sync Idempotency

Running sync twice with no intervening changes MUST produce `filesChanged = 0`
on the second run. This applies to both prompt files and profile orchestrator prompts.

#### Scenario: Re-sync is a no-op

- GIVEN sync has run once successfully with profiles `cheap` and `gemini`
- WHEN sync runs again immediately with no changes to assets or config
- THEN `filesChanged = 0`
- AND the sync report says "All managed assets are already up to date"

---

### Requirement: New Profile Sub-agents Added During Sync

If a profile is detected in `opencode.json` and one of its expected sub-agent keys
is missing (e.g., a new phase was added to the SDD phase list), sync MUST add the
missing sub-agent with the profile's default model.

#### Scenario: New phase added to existing profile

- GIVEN profile `cheap` exists but `sdd-onboard-cheap` is absent
- WHEN sync runs
- THEN `sdd-onboard-cheap` is added to `opencode.json` with the profile's model
