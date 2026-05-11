# PRD: VS Code Copilot SDD Profiles

> **Make VS Code Copilot a first-class multi-mode SDD agent — assign different models per SDD phase, switch between named profiles, and let the orchestrator drive the workflow.**

**Version**: 0.1.0-draft
**Author**: Gentleman Programming
**Date**: 2026-05-11
**Status**: Draft

---

## 1. Problem Statement

Today, VS Code Copilot users cannot use Gentle AI's SDD multi-mode workflow on the platform they already pay for. The VS Code adapter reports `SupportsSubAgents() == false`, which means:

- The wizard's "SDD Mode → Multi" branch is unreachable for Copilot users.
- They cannot assign different models to different SDD phases.
- The whole SDD per-phase cost-optimization story bypasses them.

Two market shifts make this gap urgent:

1. **GitHub Copilot is switching to AI Credits in June 2026** — usage-based billing replaces the current Premium Request quota. Every model invocation becomes a real cost, and per-phase model assignment turns from a nice-to-have into a meaningful cost lever (e.g. cheap model for `sdd-spec`, premium for `sdd-apply`).
2. **Most enterprises are standardized on VS Code + Copilot.** Microsoft licensing makes it the path of least resistance. Gentle AI's value proposition stops at the door for them — they'd have to switch to OpenCode, Claude Code, or Kiro to use multi-mode SDD.

Meanwhile, VS Code Copilot already supports a native multi-agent system: `.agent.md` files in `~/.copilot/agents/`, each one a self-contained sub-agent with YAML frontmatter (`name`, `description`, `model`, `tools`, `agents`, `user-invocable`, `readonly`, `background`) and a Markdown body. This is exactly the primitive Gentle AI needs.

**This feature closes the gap.**

---

## 2. Vision

**The user installs Gentle AI with VS Code Copilot selected, picks SDD multi-mode, and optionally creates named profiles ("cheap", "premium", etc.) — each profile gets its own orchestrator + 10 phase executors written to `~/.copilot/agents/` as `.agent.md` files. Inside VS Code Copilot Chat, the user invokes `@sdd-orchestrator` (or `@sdd-orchestrator-cheap`) and the orchestrator dispatches through the SDD phases in deterministic order.**

```
~/.copilot/agents/
├── sdd-orchestrator.agent.md       ← default profile orchestrator (user-invocable)
├── sdd-init.agent.md               ← default phase executors (10 total)
├── sdd-explore.agent.md
├── sdd-propose.agent.md
├── sdd-spec.agent.md
├── sdd-design.agent.md
├── sdd-tasks.agent.md
├── sdd-apply.agent.md
├── sdd-verify.agent.md
├── sdd-archive.agent.md
├── sdd-onboard.agent.md
│
├── sdd-orchestrator-cheap.agent.md ← "cheap" profile (uses Haiku for orchestrator)
├── sdd-init-cheap.agent.md
├── ... (10 suffixed executors)
│
└── sdd-orchestrator-premium.agent.md ← "premium" profile (Opus orchestrator)
    ... (10 suffixed executors)
```

In Copilot Chat the user types `@sdd-orchestrator-cheap` to drive a budget-friendly SDD run, or `@sdd-orchestrator-premium` for a high-stakes change.

---

## 3. Target Users

| User | Pain Point | How the Feature Helps |
|------|-----------|-----------------------|
| **Enterprise dev on Copilot** | Locked into VS Code by IT/licensing; cannot reach Gentle AI's SDD value | Native `.agent.md` install — no platform switch required |
| **Cost-conscious solo dev (post-June 2026)** | Every Copilot call costs AI Credits | Per-phase model assignment routes cheap phases to cheap models |
| **Power user with multiple Copilot subscriptions** | Wants to test Sonnet 4 vs Opus 4.5 vs GPT-5 without rewriting agent files | Named profiles encapsulate full model sets; switch with `@orchestrator-{name}` |
| **Team lead** | Wants to standardize SDD profiles across the team | `.agent.md` files are version-controllable in `.github/agents/` or distributed as a config |
| **Onboarding-focused contributor** | Wants the new SDD walkthrough but in VS Code | `@sdd-orchestrator` + `sdd-onboard` sub-agent runs the guided flow |

---

## 4. Scope

### In Scope (v1 — this PR)

- VS Code Copilot adapter activates sub-agent support (`SupportsSubAgents() == true`, `SubAgentsDir(homeDir) == ~/.copilot/agents/`).
- 11 embedded `.agent.md` templates under `internal/assets/vscode/agents/`: one orchestrator + 10 phase executors.
- Profile generator (`GenerateVSCodeProfileFiles`) producing 11 files per named profile, with the orchestrator's `agents:` whitelist correctly suffixed.
- Profile remover (`RemoveVSCodeProfileAgents`) cleaning all 11 suffixed files for a named profile; default profile rejected.
- Provider/model → Copilot display name mapping (`vscModelEntries`) covering the 9 most common Copilot models, with a `provider/model` fallback for unknown IDs.
- Injection pipeline (`inject.go` step 2c and 3c) writes default and named-profile files. Post-check verifies `sdd-orchestrator.agent.md` (when shipped), `sdd-apply.agent.md`, and `sdd-verify.agent.md` exist and are non-empty.
- TUI integration (companion PR): welcome menu entry `VS Code SDD Profiles (N)`, adapter-aware profile list / create / delete screens, model picker that pulls the live Copilot catalog from the OpenCode cache (`github-copilot` provider only).
- TUI warning screen surfaced when SDD multi-mode is paired with both VS Code Copilot and Claude Code — Copilot's panel scans both `.agent.md` (native) and `.md` (Claude format), so the 8 overlapping phases would otherwise look duplicated.

### Out of Scope (permanently)

- **Profile transport via `opencode.json`.** This feature is exclusive to VS Code Copilot's `.agent.md` format. Profiles for OpenCode live in `opencode.json` (see `prd-opencode-profiles.md`).
- **Custom orchestrator prompt per profile.** All profiles share the same orchestrator instructions; only the model assignment and the suffixed `agents:` whitelist vary between profiles.

### Out of Scope (v1, future consideration)

- Export / import VS Code profiles between machines.
- Cross-workspace profile sharing via `.github/agents/`.
- Background / readonly variants of phase executors (e.g. a `sdd-verify-bg` that runs while Apply continues).
- Detection of the deprecated `infer` field on existing user agents in `~/.copilot/agents/`.

---

## 5. Detailed Requirements

### 5.1 Embedded templates

**R-VSC-01**: The installer SHALL embed 11 `.agent.md` templates under `internal/assets/vscode/agents/`:

```
sdd-orchestrator.agent.md
sdd-init.agent.md
sdd-explore.agent.md
sdd-propose.agent.md
sdd-spec.agent.md
sdd-design.agent.md
sdd-tasks.agent.md
sdd-apply.agent.md
sdd-verify.agent.md
sdd-archive.agent.md
sdd-onboard.agent.md
```

**R-VSC-02**: Each template's YAML frontmatter MUST contain at minimum: `name`, `description`, `readonly`, `background`, `user-invocable`. Templates that ship a `model:` field MUST use the sentinel `{{VSC_MODEL}}` so the injector can resolve or remove it.

**R-VSC-03**: The orchestrator template MUST include `tools: ['agent']` and an `agents:` whitelist enumerating the 10 phases. Without these, VS Code Copilot cannot reliably dispatch sub-agents through it.

**R-VSC-04**: Phase executors MUST have `user-invocable: false`. Only the orchestrator is `user-invocable: true`. This keeps the agent dropdown focused — users invoke the orchestrator and let it dispatch, rather than scrolling through 11 entries.

### 5.2 Default install (3c block)

**R-VSC-10**: When the active adapter is VS Code Copilot and `SupportsSubAgents()` returns true, the injector MUST copy all embedded templates from `internal/assets/vscode/agents/` to `<homeDir>/.copilot/agents/`.

**R-VSC-11**: The injector MUST resolve `{{VSC_MODEL}}` per template by consulting `opts.OpenCodeModelAssignments` for the phase name (falling back to the `"default"` key). If `VSCodeModelID` returns `""` for the resolved assignment, the entire `model: {{VSC_MODEL}}` line MUST be removed so Copilot falls back to the user's default model.

**R-VSC-12**: The injector MUST resolve `{{VSC_PROFILE_SUFFIX}}` to the empty string in this path. The orchestrator's `agents:` whitelist therefore references the unsuffixed phase agents.

**R-VSC-13**: Writes MUST go through `filemerge.WriteFileAtomic` so the operation is idempotent: a re-run with unchanged inputs leaves files untouched and `InjectionResult.Changed` reports `false`.

### 5.3 Named profile generation (2c block)

**R-VSC-20**: When the user has defined named profiles (`profile.Name != "" && != "default"`), the injector's 2c block MUST call `vscode.GenerateVSCodeProfileFiles(profile, agentsDir)` for each one.

**R-VSC-21**: `GenerateVSCodeProfileFiles` MUST produce 11 files per profile, named:

```
sdd-orchestrator-{profile}.agent.md
sdd-init-{profile}.agent.md
sdd-explore-{profile}.agent.md
... (8 more)
sdd-onboard-{profile}.agent.md
```

**R-VSC-22**: The orchestrator file's `agents:` whitelist MUST be suffixed to match the phase files (e.g. `sdd-apply-cheap`, not `sdd-apply`). Otherwise the orchestrator would dispatch to nonexistent agents.

**R-VSC-23**: The orchestrator's body references (e.g. "delegate to `sdd-apply`") MUST also be suffixed to keep the dispatch instructions consistent with the whitelist.

**R-VSC-24**: The orchestrator's `model:` field MUST resolve from `Profile.OrchestratorModel` (not from `PhaseAssignments`). Empty assignment MUST omit the field so Copilot uses the user's default for orchestration.

**R-VSC-25**: Each phase executor's `model:` field MUST resolve from `Profile.PhaseAssignments[<phase>]`. Empty assignment MUST omit the field.

**R-VSC-26**: Default profile MUST be rejected: `GenerateVSCodeProfileFiles` MUST return an error if `profile.Name == "" || profile.Name == "default"`. The default set is owned by the 3c block.

### 5.4 Profile removal

**R-VSC-30**: `RemoveVSCodeProfileAgents(agentsDir, profileName)` MUST delete all 11 suffixed files for the named profile.

**R-VSC-31**: Default profile MUST NOT be removable: `RemoveVSCodeProfileAgents` MUST return an error for `profileName == "" || "default"`.

**R-VSC-32**: Missing files MUST be silently skipped (no error). Non-gentle-ai files in `agentsDir` MUST be left untouched — the removal only touches files matching `sdd-*-{profileName}.agent.md` and `sdd-orchestrator-{profileName}.agent.md`.

### 5.5 Model mapping

**R-VSC-40**: The injector exposes `VSCodeModelID(assignment ModelAssignment) string` which maps a provider/model pair to a Copilot display name (e.g. `"Claude Sonnet 4 (copilot)"`).

**R-VSC-41**: The mapping table (`vscModelEntries`) MUST cover the 9 most-used Copilot-exposed models: Claude Sonnet 4, Claude Opus 4.5, Claude Haiku 4.5, Gemini 2.5 Pro, Gemini 2.5 Flash, GPT 4.1, GPT 4.1 Mini, GPT 4o, GPT 4o Mini.

**R-VSC-42**: Matching uses `strings.Contains` against `ModelID`. Entries MUST be ordered from most-specific to least-specific to avoid partial matches: `gpt-4o-mini` MUST appear before `gpt-4o`; `gpt-4.1-mini` before `gpt-4.1`. The mapping comment MUST document this constraint.

**R-VSC-43**: Unknown models fall back to `ProviderID + "/" + ModelID` (e.g. `"openai/gpt-5-future"`). Empty `ModelID` returns `""` — the caller MUST omit the `model:` line entirely.

### 5.6 Post-injection verification

**R-VSC-50**: After writing the agent files, the injector MUST verify that the critical files exist and are at least 10 bytes:
- `sdd-orchestrator` (only when the adapter ships an orchestrator template — VS Code does, Claude Code does not)
- `sdd-apply`
- `sdd-verify`

**R-VSC-51**: The verification MUST tolerate the three extensions `.md`, `.yaml`, and `.agent.md` so a single check works across adapters.

**R-VSC-52**: A truncated or missing critical file MUST cause `Inject()` to return a descriptive error.

### 5.7 TUI integration (companion PR)

**R-VSC-60**: The Welcome screen MUST show a `VS Code SDD Profiles (N)` entry when VS Code Copilot is detected. `N` is the count of named profiles currently on disk under `~/.copilot/agents/`.

**R-VSC-61**: The Profiles screen (existing `ScreenProfiles`) MUST be adapter-aware via `Model.ActiveProfileAdapter` — title and subtitle adapt (`OpenCode` vs `VS Code`), and the underlying detection / write / delete backends route to the right adapter.

**R-VSC-62**: When creating a VS Code profile, the model picker MUST source its catalog from the OpenCode cache `~/.cache/opencode/models.json`, restricted to the `github-copilot` provider. This guarantees the user only assigns models that Copilot actually supports.

**R-VSC-63**: VS Code profile create / delete MUST bypass the OpenCode sync pipeline. Writes go directly to disk via `GenerateVSCodeProfileFiles` / `RemoveVSCodeProfileAgents`. After each operation, the TUI MUST refresh the profile list so the badge count stays accurate.

**R-VSC-64**: When SDD multi-mode is selected together with both VS Code Copilot and a Claude-format adapter (Claude Code in v1), the wizard MUST display a warning screen explaining that VS Code Copilot will show the 8 overlapping sub-agent phases twice. The user can `Continue anyway` or `Back to adapter selection`.

---

## 6. Technical Design

### 6.1 Data Model

The `Profile` struct (`internal/model/types.go`) is reused unchanged from the OpenCode feature:

```go
type Profile struct {
    Name              string
    OrchestratorModel ModelAssignment
    PhaseAssignments  map[string]ModelAssignment
}
```

VS Code does not need a separate type — the same per-phase mapping that drives OpenCode drives VS Code, with the model IDs interpreted through `VSCodeModelID` instead of OpenCode's provider/model format.

### 6.2 File layout on disk

```
~/.copilot/agents/
├── sdd-orchestrator.agent.md         # user-invocable orchestrator, tools: ['agent']
├── sdd-{phase}.agent.md (×10)        # phase executors, user-invocable: false
└── sdd-{phase}-{profile}.agent.md    # one full set per named profile (×N profiles)
```

Each `.agent.md` is a self-contained unit. There is no shared prompts directory (unlike OpenCode's `~/.config/opencode/prompts/sdd/`) because VS Code Copilot does not support file-reference syntax in agent definitions — the body must be inlined.

### 6.3 Orchestrator template structure

```yaml
---
name: sdd-orchestrator{{VSC_PROFILE_SUFFIX}}
description: >
  SDD workflow orchestrator — coordinates the 10 SDD phase executors in a
  strict, deterministic sequence.
model: {{VSC_MODEL}}
tools: ['agent']
agents:
  - sdd-init{{VSC_PROFILE_SUFFIX}}
  - sdd-explore{{VSC_PROFILE_SUFFIX}}
  - ... (8 more)
  - sdd-onboard{{VSC_PROFILE_SUFFIX}}
readonly: false
background: false
user-invocable: true
---

You are the SDD workflow orchestrator...
1. Delegate to `sdd-explore{{VSC_PROFILE_SUFFIX}}` — Survey the codebase...
2. Delegate to `sdd-propose{{VSC_PROFILE_SUFFIX}}` — ...
...
```

For named profiles, `{{VSC_PROFILE_SUFFIX}}` resolves to `-{profile}`. For the default set, it resolves to the empty string. The body's dispatch instructions reference suffixed phase names to match the `agents:` whitelist.

### 6.4 Phase executor template structure

```yaml
---
name: sdd-apply{{VSC_PROFILE_SUFFIX}}
description: >
  Implement code changes from task definitions
model: {{VSC_MODEL}}
readonly: false
background: false
user-invocable: false
---

You are the SDD **sdd-apply** executor. Do this phase's work yourself. Do NOT delegate further.
You are not the orchestrator. Do NOT call task/delegate. Do NOT launch sub-agents.

## Instructions

Read the skill file at `~/.copilot/skills/sdd-apply/SKILL.md` and follow it exactly.
Also read shared conventions at `~/.copilot/skills/_shared/sdd-phase-common.md`.
```

The phase body is short on purpose: detailed work instructions live in `~/.copilot/skills/sdd-{phase}/SKILL.md` (installed separately by the SDD skills component), so updates to the SDD workflow don't require regenerating every agent file.

### 6.5 Dispatch flow inside VS Code Copilot

```
User in Copilot Chat:
  @sdd-orchestrator do SDD for "Add export-to-CSV button"
  │
  ├─ VS Code routes to sdd-orchestrator.agent.md (user-invocable: true)
  │
  ├─ Orchestrator reads its body, which says:
  │   "1. Delegate to sdd-explore — Survey the codebase..."
  │
  ├─ Orchestrator uses tools: ['agent'] to invoke sdd-explore
  │   (sdd-explore is in the agents: whitelist)
  │
  ├─ sdd-explore runs as a sub-agent, returns findings
  │
  ├─ Orchestrator synthesizes, dispatches to sdd-propose
  │
  ├─ ... continues through spec → design → tasks → apply → verify → archive
  │
  └─ Each phase reads its SKILL.md and reports back
```

Why an explicit orchestrator instead of relying on Copilot Chat's default agent to discover sub-agents from descriptions:

- **Determinism.** SDD is a strict sequence. Without an orchestrator body listing the phases in order, weaker Copilot models routinely skip phases or reorder them.
- **Restriction.** The `agents:` whitelist lets the orchestrator dispatch only to the 10 SDD phases — Copilot's chat default would also surface any other custom agent the user has installed.
- **Auditability.** The orchestrator body documents the workflow contract; the user can read it to understand what the agent will do before invoking it.

### 6.6 Affected Files (implementation map)

| Area | File | Changes |
|------|------|---------|
| **Adapter** | `internal/agents/vscode/adapter.go` | `SupportsSubAgents() = true`; new `SubAgentsDir`, `EmbeddedSubAgentsDir`, `VSCModelID` delegate |
| **Templates** | `internal/assets/vscode/agents/*.agent.md` | NEW — 11 embedded templates |
| **Embed** | `internal/assets/assets.go` | Embed `vscode/` directory tree |
| **Generator** | `internal/agents/vscode/vscode_profiles.go` | NEW — `vscModelEntries`, `VSCodeModelID`, `GenerateAgentFile`, `generateOrchestratorAgent`, `GenerateVSCodeProfileFiles`, `RemoveVSCodeProfileAgents`, `DetectVSCodeProfiles`, `SDDPhases`, `OrchestratorPhase` |
| **Injector** | `internal/components/sdd/inject.go` | New step 2c (named profiles); 3c resolves `{{VSC_MODEL}}` + `{{VSC_PROFILE_SUFFIX}}`; post-check extended to `.agent.md` and conditional orchestrator check |
| **TUI (companion PR)** | `internal/tui/model.go` | `hasDetectedVSCode`, `VSCodeProfileList`, `ActiveProfileAdapter`, adapter-aware handlers, `shouldWarnAboutDuplicateAgents`, `advanceFromSDDModeSelection` |
| **TUI (companion PR)** | `internal/tui/screens/welcome.go` | `VS Code SDD Profiles (N)` menu entry |
| **TUI (companion PR)** | `internal/tui/screens/profiles.go` | `adapterLabel` parameter |
| **TUI (companion PR)** | `internal/tui/screens/profile_delete.go` | `isVSCode` flag + adapter-aware wording |
| **TUI (companion PR)** | `internal/tui/screens/vscode_model_picker.go` | NEW — `VSCodeModelPickerState`, `NewVSCodeModelPickerState`, render functions |
| **TUI (companion PR)** | `internal/tui/screens/sdd_duplicate_warning.go` | NEW — warning screen for VS Code + Claude combo |

### 6.7 Injection flow

```
Inject(homeDir, vscodeAdapter, multiMode, opts)
  │
  ├─ Step 2c: VS Code named profiles
  │   for each profile in opts.Profiles where Name != "" && Name != "default":
  │     vscode.GenerateVSCodeProfileFiles(profile, agentsDir)
  │       → 11 files: sdd-orchestrator-{name}.agent.md + 10 sdd-{phase}-{name}.agent.md
  │       → writes via filemerge.WriteFileAtomic (idempotent)
  │
  ├─ Step 3c: default profile via embedded copy loop
  │   for each entry in embedded vscode/agents/:
  │     resolve {{VSC_MODEL}} → friendly Copilot name (or remove the line)
  │     resolve {{VSC_PROFILE_SUFFIX}} → "" (empty for default)
  │     write to <agentsDir>/<entry.Name()>
  │
  └─ Post-check
      criticalPhases = ["sdd-apply", "sdd-verify"]
      if embedded dir contains sdd-orchestrator.{md,yaml,agent.md}:
          criticalPhases prepend "sdd-orchestrator"
      for each phase: verify <agentsDir>/<phase>.{md,yaml,agent.md} exists and ≥10 bytes
```

### 6.8 Idempotency

`filemerge.WriteFileAtomic` compares existing file content against the new content before writing. Identical content → no write → `WriteResult.Changed == false`. The 2c block aggregates this across the 11 files via `len(profileFiles) > 0`, and the 3c loop ORs each result into the overall `changed` flag.

Regression tests `TestInject_VSCode_DefaultProfile_IsIdempotent` and `TestInject_VSCode_NamedProfile_IsIdempotent` lock this contract: invoking `Inject()` twice with identical inputs leaves disk state and `InjectionResult.Changed` unchanged.

---

## 7. UX Flow (companion PR)

### 7.1 Welcome menu (extended)

```
┌─────────────────────────────────────────────────────────┐
│  ★  Gentleman AI Ecosystem                              │
│                                                          │
│  ▸ Start installation                                    │
│    Upgrade tools                                         │
│    Sync configs                                          │
│    Upgrade + Sync                                        │
│    Configure models                                      │
│    Create your own Agent                                 │
│    OpenCode Community Plugins                            │
│    OpenCode SDD Profiles (2)                             │
│    VS Code SDD Profiles (1)              ← NEW           │
│    Manage backups                                        │
│    Managed uninstall                                     │
│    Quit                                                  │
└─────────────────────────────────────────────────────────┘
```

The VS Code entry only appears when VS Code Copilot is detected (`hasDetectedVSCode()` returns true).

### 7.2 VS Code profile list

```
┌─────────────────────────────────────────────────────────┐
│  VS Code SDD Profiles                                    │
│                                                          │
│  Your SDD model profiles for VS Code. Each profile       │
│  creates a dedicated set of per-phase agents.            │
│                                                          │
│  • cheap                                                 │
│  ▸ premium                                               │
│                                                          │
│    Create new profile                                    │
│    Back                                                  │
│                                                          │
│  j/k: navigate • enter: edit • n: new • d: delete       │
└─────────────────────────────────────────────────────────┘
```

### 7.3 VS Code model picker (single-provider)

When the user advances to the model picker for a VS Code profile, the picker is preloaded with the `github-copilot` provider only:

```
┌─────────────────────────────────────────────────────────┐
│  Profile "cheap" — Assign Models                         │
│                                                          │
│  ▸ Set all phases ──── (not set)                         │
│    sdd-orchestrator ── claude-opus-4-5                   │
│    sdd-init ────────── claude-haiku-4-5                  │
│    sdd-explore ─────── claude-haiku-4-5                  │
│    ... (8 more)                                          │
│    Continue                                              │
│    Back                                                  │
│                                                          │
│  Provider: github-copilot                                │
└─────────────────────────────────────────────────────────┘
```

If the OpenCode model cache is missing or lacks a `github-copilot` entry, the picker renders a banner:

```
┌─────────────────────────────────────────────────────────┐
│  ⚠ github-copilot provider not found in OpenCode        │
│    models cache. Run `opencode sync` first to fetch     │
│    the Copilot model catalog.                            │
└─────────────────────────────────────────────────────────┘
```

### 7.4 Duplicate-agents warning (VS Code + Claude combo)

```
┌─────────────────────────────────────────────────────────┐
│  Heads up: VS Code will show duplicated SDD agents       │
│                                                          │
│  You're installing SDD multi-mode for both VS Code       │
│  Copilot and Claude Code. VS Code Copilot's agent panel  │
│  reads two formats in parallel:                          │
│    • Copilot native: ~/.copilot/agents/*.agent.md        │
│    • Claude format:  ~/.claude/agents/*.md               │
│                                                          │
│  These 8 sub-agents will appear twice in VS Code:        │
│    • sdd-apply                                           │
│    • sdd-archive                                         │
│    • sdd-design                                          │
│    • sdd-explore                                         │
│    • sdd-propose                                         │
│    • sdd-spec                                            │
│    • sdd-tasks                                           │
│    • sdd-verify                                          │
│                                                          │
│  Each file is correct and works in its own host — no    │
│  behavior difference. This is purely a UI quirk.        │
│                                                          │
│  ▸ Continue anyway                                       │
│    ← Back to adapter selection                           │
└─────────────────────────────────────────────────────────┘
```

---

## 8. Edge Cases & Decisions

### 8.1 OpenCode cache missing

The VS Code model picker reads from `~/.cache/opencode/models.json`, which is populated by running `opencode sync`. If the user has not run OpenCode yet, the cache is absent. The picker SHALL show a banner pointing the user to `opencode sync` and allow them to either (a) cancel and run sync, or (b) proceed without explicit model assignments (Copilot will use the user's default model for every phase).

### 8.2 VS Code without Copilot subscription

Some users have VS Code installed but have not paid for Copilot. The `.agent.md` files still get written to `~/.copilot/agents/`, but VS Code does not surface them in the chat. The install succeeds and the post-check passes — the files are there. No special handling is required from the installer side; this is purely a Copilot subscription matter.

### 8.3 Visual duplication when both VS Code Copilot and Claude Code are installed

VS Code Copilot scans both `.agent.md` (native) and `.md` (Claude format) directories. When the user installs SDD multi-mode for both adapters, the 8 phases that Claude ships as sub-agents (`sdd-apply`, `sdd-archive`, `sdd-design`, `sdd-explore`, `sdd-propose`, `sdd-spec`, `sdd-tasks`, `sdd-verify`) appear twice in VS Code's Agent customizations panel. `sdd-init` and `sdd-onboard` do not duplicate because Claude does not ship them as sub-agents.

This is not a bug — each file is correct in its own host. The installer surfaces the wizard warning in §7.4 so the user is not surprised. Two paths to resolve if the user dislikes the duplication: install only one of the two adapters, or accept the duplicates (each works correctly when invoked in its own chat).

### 8.4 Embedded template extension

Templates use the `.agent.md` extension on disk and inside the embedded asset filesystem. The injector's post-check tolerates `.md`, `.yaml`, and `.agent.md` so a single helper works across adapters with different conventions.

### 8.5 Phase ordering inside `vscModelEntries`

`strings.Contains` is used for matching, so longer substrings MUST come before shorter ones. Examples:

- `gpt-4o-mini` before `gpt-4o`
- `gpt-4.1-mini` before `gpt-4.1`
- If `claude-sonnet-4-5` is ever added, it MUST go before the existing `claude-sonnet-4` entry

The mapping table comment documents this and the test `TestVSCodeModelID_KnownProviders` covers each known model. A fallback comment captures the latent risk.

### 8.6 Default profile cannot be removed

`RemoveVSCodeProfileAgents("", "default")` is rejected with an error. The default set is owned by the 3c injection block; the only way to remove it is to uninstall the SDD component entirely.

---

## 9. Success Metrics

| Metric | Target |
|--------|--------|
| Time to create a new VS Code profile (TUI) | < 60 seconds |
| `~/.copilot/agents/` count after a default install | exactly 11 (1 orchestrator + 10 phases) |
| File churn on a re-run with identical config | 0 (idempotent) |
| Profile count supported | Tested up to 5 named profiles |
| Compatibility with `opencode sync` cache schema | 100% (read-only consumer) |
| Behavioral regression for non-VS-Code adapters | 0 (post-check orchestrator branch is conditional) |

---

## 10. Implementation Phases (history)

These were the apply-time work units. They are listed here for traceability — the feature is already implemented.

### Phase 1: VS Code adapter capability

- Adapter reports `SupportsSubAgents() == true`.
- `SubAgentsDir(homeDir)` returns `~/.copilot/agents/`.
- `EmbeddedSubAgentsDir()` returns `"vscode/agents"`.

### Phase 2: Embedded templates and asset embed

- Added 11 `.agent.md` templates under `internal/assets/vscode/agents/`.
- Updated `internal/assets/assets.go` to embed `vscode/`.
- Tests cover existence and minimum size of each template.

### Phase 3: Profile generator

- `vscode_profiles.go`: `vscModelEntries`, `VSCodeModelID`, `GenerateAgentFile`, `generateOrchestratorAgent`, `SDDPhases`, `OrchestratorPhase`.
- `GenerateVSCodeProfileFiles(profile, agentsDir)` produces 11 files per profile, suffixing names and the orchestrator whitelist.
- `RemoveVSCodeProfileAgents(agentsDir, profileName)` removes all 11 suffixed files.

### Phase 4: Injection pipeline integration

- `inject.go` step 2c writes named-profile files via `GenerateVSCodeProfileFiles`.
- 3c resolves `{{VSC_MODEL}}` per phase (via `OpenCodeModelAssignments` lookup) and `{{VSC_PROFILE_SUFFIX}}` (empty for default).
- Post-check extended to recognize `.agent.md` and to conditionally check `sdd-orchestrator`.

### Phase 5: TUI integration (companion PR)

- `Model.ActiveProfileAdapter` threads the active adapter through the shared profile screens.
- Welcome menu entry `VS Code SDD Profiles (N)` appears when VS Code Copilot is detected.
- `DetectVSCodeProfiles(agentsDir)` scans `~/.copilot/agents/sdd-*-{name}.agent.md` and dedupes by `{name}`.
- VS Code-specific model picker (`VSCodeModelPickerState`) loads the `github-copilot` provider catalog from the OpenCode cache.
- Duplicate-agents warning screen when SDD + VS Code + Claude are selected together.

### Phase 6: Idempotency regression tests

- `TestInject_VSCode_DefaultProfile_IsIdempotent` — re-run leaves 11 default files untouched.
- `TestInject_VSCode_NamedProfile_IsIdempotent` — re-run leaves 22 files (11 default + 11 cheap) untouched.

---

## 11. Open Questions

1. **Does VS Code Copilot's `agents:` whitelist support wildcards?**
   → As of May 2026, no. Each suffixed agent must be enumerated explicitly. The orchestrator template generates the full list at injection time.

2. **Should we also write a workspace-level `.github/agents/` set?**
   → No. The user-level install in `~/.copilot/agents/` is portable across all VS Code workspaces. Workspace-level installs would create the same visual-duplication issue we already warn about for Claude.

3. **Should `sdd-onboard` and `sdd-init` be `user-invocable: true`?**
   → They could be, since they are entry-point flows. For v1 we keep them dispatched-only to keep the agent dropdown minimal. The orchestrator's body explicitly tells users to ask for "init" or "onboard" — Copilot will route through the orchestrator.

4. **Can the user change the orchestrator prompt without reinstalling?**
   → Yes — they can edit `~/.copilot/agents/sdd-orchestrator.agent.md` directly. The installer will overwrite their changes on the next `gentle-ai sync` because `filemerge.WriteFileAtomic` compares against the template content. If we want to preserve user edits, a follow-up could detect a `# user-edited` marker and skip the file.

5. **What happens when Copilot deprecates a model that's hardcoded in `vscModelEntries`?**
   → The mapping returns the friendly display name regardless, but Copilot Chat will fail to find the model when it tries to dispatch. The user must edit the assignment via the TUI to pick a current model. Future work: detect mappings whose target no longer appears in the OpenCode cache and surface a warning during sync.
