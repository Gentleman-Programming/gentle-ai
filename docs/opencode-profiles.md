# OpenCode SDD Profiles

← [Back to README](../README.md)

---

You configured your SDD models once, and now every task -- cheap or expensive, experimental or battle-tested -- runs through the same orchestrator. Profiles fix that: **create named model configurations and switch between them with Tab inside OpenCode.**

Each profile generates its own `sdd-orchestrator` (plus all 10 sub-agents) in `opencode.json`. One profile for heavy architectural work with Opus, another for quick fixes with Haiku, a third to test Gemini -- all coexisting, switchable in a keystroke.

---

## Quick Start (TUI)

1. Launch the installer: `gentle-ai` (or `go run ./cmd/gentle-ai`).
2. Select **"OpenCode SDD Profiles"** from the welcome screen.
3. Select **"Create new profile"** (or press `n`).
4. Enter a profile name in slug format (lowercase, hyphens ok). Example: `cheap`.
5. Pick the orchestrator model (provider, then model -- reuses the existing model picker).
6. Assign sub-agent models (use "Set all phases" for a uniform config, or set each phase individually).
7. Confirm -- the installer writes the profile to `opencode.json` and runs sync.

Open OpenCode and press **Tab** -- your new orchestrator appears alongside the default.

## Quick Start (CLI)

Create a profile during sync with `--profile name:provider/model`:

```bash
gentle-ai sync --profile cheap:anthropic/claude-haiku-3.5-20241022
```

Multiple profiles in one command:

```bash
gentle-ai sync \
  --profile cheap:anthropic/claude-haiku-3.5-20241022 \
  --profile premium:anthropic/claude-opus-4-20250514
```

Override a specific phase with `--profile-phase name:phase:provider/model`:

```bash
gentle-ai sync \
  --profile cheap:anthropic/claude-haiku-3.5-20241022 \
  --profile-phase cheap:sdd-apply:anthropic/claude-sonnet-4-20250514
```

This creates a "cheap" profile where everything runs on Haiku except `sdd-apply`, which uses Sonnet.

## Using Profiles in OpenCode

After creating profiles, each one appears as a selectable orchestrator in OpenCode:

| What you see in Tab | What it runs |
|---|---|
| `sdd-orchestrator` | Default profile (your original config) |
| `sdd-orchestrator-cheap` | "cheap" profile -- Haiku everywhere |
| `sdd-orchestrator-premium` | "premium" profile -- Opus everywhere |

Press **Tab** to cycle between orchestrators. All SDD slash commands (`/sdd-new`, `/sdd-ff`, `/sdd-explore`, etc.) run against whichever orchestrator is currently selected. The orchestrator delegates to its own suffixed sub-agents (e.g., `sdd-apply-cheap`), so profiles never interfere with each other.

## Managing Profiles

From the TUI profile list screen:

| Action | Key | Notes |
|---|---|---|
| Edit a profile | `Enter` on the profile | Change models, then sync |
| Delete a profile | `d` on the profile | Removes orchestrator + all sub-agents from JSON |
| Create a new profile | `n` (or select "Create new profile") | Full creation flow |

The `default` profile (the unsuffixed `sdd-orchestrator`) can be edited but not deleted -- it always exists when SDD is configured.

### Profile name rules

| Input | Valid? | Reason |
|---|---|---|
| `cheap` | Yes | Simple slug |
| `premium-v2` | Yes | Hyphens allowed |
| `my profile` | No | Spaces not allowed |
| `default` | No | Reserved for the base orchestrator |
| `LOUD` | Becomes `loud` | Auto-lowercased |

---

<details>
<summary><strong>How It Works</strong></summary>

Each profile generates 11 agent entries in `opencode.json`: one orchestrator (`sdd-orchestrator-{name}`, mode `primary`) and 10 sub-agents (`sdd-{phase}-{name}`, mode `subagent`, hidden). The orchestrator's permissions are scoped so it can only delegate to its own suffixed sub-agents.

Sub-agent prompts are shared across all profiles as files under `~/.config/opencode/prompts/sdd/` (e.g., `sdd-apply.md`). Each agent entry references the shared file via `{file:~/.config/opencode/prompts/sdd/sdd-apply.md}` -- only the `model` field differs between profiles. Orchestrator prompts are inlined per-profile because they contain profile-specific model assignment tables and sub-agent references.

During sync or update, the installer detects existing profiles by scanning for `sdd-orchestrator-*` keys, updates shared prompt files, regenerates orchestrator prompts, and preserves your model assignments.

</details>

---

← [Back to README](../README.md)
