# OpenCode SDD Profiles

← [Back to README](../README.md)

---

You configured your SDD models once, and now every task -- cheap or expensive, experimental or battle-tested -- runs through the same orchestrator. Profiles fix that: **create named model configurations and switch between them with Tab inside OpenCode.**

Gentle AI supports **two ways** of working with OpenCode profiles:

1. **Generated multi-profile mode** -- the classic Gentle AI flow. Each named profile generates its own `sdd-orchestrator-{name}` plus 10 suffixed sub-agents in `opencode.json`, and you switch between them with **Tab**.
2. **External single-active mode** -- for community tools that keep profile files outside `opencode.json` and activate one runtime profile at a time.

That means you can stay with the built-in multi-profile overlay, or plug Gentle AI into an external profile manager without the two systems fighting each other.

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

## External Profile Managers

If you're using a community tool that stores profiles under `~/.config/opencode/profiles/*.json` and activates them at runtime, Gentle AI can now sync OpenCode in a compatibility mode.

### Auto-detection

On `gentle-ai sync`, if OpenCode profile files exist under:

```text
~/.config/opencode/profiles/*.json
```

Gentle AI automatically switches to **`external-single-active`** strategy for OpenCode sync.

### Manual override

You can also force the strategy explicitly:

```bash
gentle-ai sync --agent opencode --sdd-profile-strategy external-single-active
```

Or force the classic generated overlay behavior:

```bash
gentle-ai sync --agent opencode --sdd-profile-strategy generated-multi
```

### What compatibility mode does

In `external-single-active` mode, Gentle AI:

- keeps writing the base OpenCode SDD assets and shared prompt files
- **does not** auto-regenerate suffixed named profiles from `opencode.json`
- **preserves the current `sdd-orchestrator` prompt** during sync so external tools can keep their runtime policy / fallback blocks intact

This is the important bit: Gentle AI still maintains the SDD foundation, but it stops acting like `opencode.json` is the source of truth for every profile.

## Using Profiles in OpenCode

After creating profiles in generated multi-profile mode, each one appears as a selectable orchestrator in OpenCode:

| What you see in Tab | What it runs |
|---|---|
| `sdd-orchestrator` | Default profile (your original config) |
| `sdd-orchestrator-cheap` | "cheap" profile -- Haiku everywhere |
| `sdd-orchestrator-premium` | "premium" profile -- Opus everywhere |

Press **Tab** to cycle between orchestrators. All SDD slash commands (`/sdd-new`, `/sdd-ff`, `/sdd-explore`, etc.) run against whichever orchestrator is currently selected. The orchestrator delegates to its own suffixed sub-agents (e.g., `sdd-apply-cheap`), so profiles never interfere with each other.

If you're using an external single-active manager instead, you typically keep working with the base `sdd-orchestrator` while the external tool swaps its active model assignments at runtime.

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

In generated multi-profile mode, each profile generates 11 agent entries in `opencode.json`: one orchestrator (`sdd-orchestrator-{name}`, mode `primary`) and 10 sub-agents (`sdd-{phase}-{name}`, mode `subagent`, hidden). The orchestrator's permissions are scoped so it can only delegate to its own suffixed sub-agents.

Sub-agent prompts are shared across all profiles as files under `~/.config/opencode/prompts/sdd/` (e.g., `sdd-apply.md`). Each agent entry references the shared file via `{file:~/.config/opencode/prompts/sdd/sdd-apply.md}` -- only the `model` field differs between profiles. Orchestrator prompts are inlined per-profile because they contain profile-specific model assignment tables and sub-agent references.

During sync or update, Gentle AI now uses one of two strategies:

- **`generated-multi`** -- scan `opencode.json` for `sdd-orchestrator-*`, update shared prompts, regenerate profile orchestrators, preserve model assignments
- **`external-single-active`** -- detect external profile files, keep the shared SDD assets current, and preserve the existing base orchestrator prompt instead of overwriting external runtime extensions

</details>

---

← [Back to README](../README.md)
