# Design: Kilo Native Orchestration

## Technical Approach

Replace Kilo Code's OpenCode overlay adapter with native `.kilo/agents/*.md` sub-agent file generation, mirroring the Kiro adapter pattern exactly. The Kiro adapter already implements `SupportsSubAgents() = true`, `EmbeddedSubAgentsDir()`, and `kiroModelResolver`. We replicate this for Kilo, adding a `kiloModelResolver` interface and `KiloModelAlias` type for Kilo Gateway model routing. Additionally, generate `kilo.jsonc` for provider/permission config alongside the existing `opencode.json` merge.

## Architecture Decisions

### Decision: Native agents vs OpenCode overlay

**Choice**: Native `.kilo/agents/*.md` files with YAML frontmatter
**Alternatives considered**: Continue using OpenCode overlay (`opencode.json` with `"mode": "primary"`)
**Rationale**: Kilo Code v7 rejects `"mode": "primary"` agents as subagents (issue #729). The overlay approach produces agents that cannot be invoked by other agents. Native `.kilo/agents/*.md` files are recognized by Kilo's agent runtime and support delegation.

### Decision: Model resolver interface pattern

**Choice**: `kiloModelResolver` optional interface on the adapter (same shape as `kiroModelResolver`)
**Alternatives considered**: Hardcode model IDs in templates; use a separate resolver package
**Rationale**: Follows the established `kiroModelResolver` / `claudeModelResolver` / `codexModelResolver` pattern in `inject.go`. Optional interfaces keep the base `Adapter` contract clean — only adapters that need model resolution implement it.

### Decision: Kilo Gateway model aliases

**Choice**: Separate `KiloModelAlias` type with Gateway-specific model IDs
**Alternatives considered**: Reuse `KiroModelAlias`; use raw model ID strings
**Rationale**: Kilo Gateway routes to different model providers than Kiro. Kilo's model IDs include provider prefixes (e.g. `anthropic/claude-sonnet-4-20250514`) that differ from Kiro's bare IDs. A separate type prevents accidental cross-adapter model stamping.

### Decision: Keep OpenCode overlay as fallback

**Choice**: Keep `opencode.json` merge for Kilo's system prompt and MCP, but bypass it for sub-agents
**Alternatives considered**: Remove OpenCode overlay entirely
**Rationale**: Kilo Code still reads `opencode.json` for system prompt and MCP configuration. The overlay merge is needed for those concerns. Only sub-agent generation switches to native files. This is a partial migration, not a full replacement.

### Decision: Profile detection path

**Choice**: Detect `~/.config/kilo/profiles/` as a read-only signal; do not write to it
**Alternatives considered**: Create profile directories; integrate with Kilo's profile system
**Rationale**: The profile path is undocumented. Detecting it lets us skip redundant injection if Kilo already has profiles. Writing to an undocumented path risks breaking Kilo's internal state.

## Data Flow

### `gentle-ai install --agent kilocode`

```
install command
  → adapter.Detect() checks `kilo` binary + ~/.config/kilo/
  → adapter.InstallCommand() returns npm install -g @kilocode/cli
  → sdd.Inject() runs with AgentKilocode:
      1. Skip StrategyMarkdownSections (Kilocode excluded from prompt injection at line 229)
      2. Merge SDD overlay into opencode.json (line 354)
         → Write orchestrator prompt to opencode.json
         → Write sub-agent definitions to opencode.json (overlay)
      3. Write skills to ~/.config/kilo/skills/
      4. Write slash commands to ~/.config/kilo/commands/
      5. SupportsSubAgents() currently returns false → NO native agent files
```

### After this change: `gentle-ai sync --agent kilocode`

```
sync command
  → sdd.Inject() runs with AgentKilocode:
      1. Skip StrategyMarkdownSections (unchanged)
      2. Merge SDD overlay into opencode.json (unchanged — system prompt + MCP)
      3. Write skills to ~/.config/kilo/skills/ (unchanged)
      4. Write slash commands to ~/.config/kilo/commands/ (unchanged)
      5. SupportsSubAgents() now returns true:
         → os.MkdirAll(~/.kilo/agents/)
         → Read embedded kilocode/agents/*.md templates
         → For each template:
             → Resolve {{KILO_MODEL}} via kiloModelResolver
             → Write to ~/.kilo/agents/<phase>.md
         → Post-check: verify sdd-apply.md and sdd-verify.md exist
      6. Generate kilo.jsonc with provider config (new)
```

### SDD phase delegation in Kilo Code

```
User: /sdd-apply my-feature
  → Kilo reads ~/.kilo/agents/sdd-apply.md
  → Frontmatter: name, description, tools, model
  → Body: instructions to load ~/.kilo/skills/sdd-apply/SKILL.md
  → Agent executes SDD apply phase in its own context window
  → Returns result to orchestrator
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/model/kilo_model.go` | Create | `KiloModelAlias` type, `KiloModelID()` resolver, preset functions |
| `internal/model/kilo_model_test.go` | Create | Tests for model alias validation and ID resolution |
| `internal/agents/kilocode/adapter.go` | Modify | Enable `SupportsSubAgents()`, add `SubAgentsDir()`, `EmbeddedSubAgentsDir()`, `KiloModelID()` method |
| `internal/agents/kilocode/adapter_test.go` | Modify | Add sub-agent capability tests, model resolver tests, update capability test count |
| `internal/assets/kilocode/agents/sdd-apply.md` | Create | Agent template with YAML frontmatter + instructions (mirrors kiro/agents/sdd-apply.md) |
| `internal/assets/kilocode/agents/sdd-verify.md` | Create | Agent template for verify phase |
| `internal/assets/kilocode/agents/sdd-design.md` | Create | Agent template for design phase |
| `internal/assets/kilocode/agents/sdd-spec.md` | Create | Agent template for spec phase |
| `internal/assets/kilocode/agents/sdd-tasks.md` | Create | Agent template for tasks phase |
| `internal/assets/kilocode/agents/sdd-explore.md` | Create | Agent template for explore phase |
| `internal/assets/kilocode/agents/sdd-propose.md` | Create | Agent template for propose phase |
| `internal/assets/kilocode/agents/sdd-archive.md` | Create | Agent template for archive phase |
| `internal/assets/kilocode/agents/sdd-init.md` | Create | Agent template for init phase |
| `internal/assets/kilocode/agents/sdd-onboard.md` | Create | Agent template for onboard phase |
| `internal/assets/assets.go` | Modify | Add `all:kilocode` to embed directive |
| `internal/components/sdd/inject.go` | Modify | Add `kiloModelResolver` interface, resolve `{{KILO_MODEL}}` in sub-agent copy loop, add Kilo-specific post-injection verification |
| `internal/components/sdd/inject_test.go` | Modify | Add Kilo sub-agent injection test cases |

## Interfaces / Contracts

### `kiloModelResolver` interface (in `inject.go`)

```go
type kiloModelResolver interface {
    KiloModelID(alias model.KiloModelAlias) string
}
```

Optional interface on the adapter. When implemented, the sub-agent copy loop resolves `KiloModelAlias` values to native model IDs and stamps them into agent frontmatter via `{{KILO_MODEL}}` sentinel.

### `KiloModelAlias` type (in `model/kilo_model.go`)

```go
type KiloModelAlias string

const (
    KiloModelAuto     KiloModelAlias = "auto"
    KiloModelSonnet   KiloModelAlias = "sonnet"
    KiloModelOpus     KiloModelAlias = "opus"
    KiloModelHaiku    KiloModelAlias = "haiku"
    KiloModelGateway  KiloModelAlias = "gateway"  // Kilo Gateway free routing
)

func KiloModelID(alias KiloModelAlias) string {
    switch alias {
    case KiloModelAuto:    return "auto"
    case KiloModelSonnet:  return "anthropic/claude-sonnet-4-20250514"
    case KiloModelOpus:    return "anthropic/claude-opus-4-20250514"
    case KiloModelHaiku:   return "anthropic/claude-haiku-4-20250514"
    case KiloModelGateway: return "gateway/auto"
    default:               return "anthropic/claude-sonnet-4-20250514"
    }
}
```

### Agent file template structure

```markdown
---
name: sdd-apply
description: >
  Implement code changes from task definitions.
tools: ["@builtin", "@engram"]
model: {{KILO_MODEL}}
includeMcpJson: true
---

You are the SDD **apply** executor. Do this phase's work yourself.
Do NOT delegate further.

## Instructions

Read the skill file from the user's Kilo home skills directory:
- macOS/Linux: `~/.config/kilo/skills/sdd-apply/SKILL.md`

Also read shared conventions:
- macOS/Linux: `~/.config/kilo/skills/_shared/sdd-phase-common.md`

Execute all steps from the skill directly in this context window.
[... phase-specific instructions ...]
```

### `kilo.jsonc` schema (new generation target)

```jsonc
{
  // Provider configuration for Kilo Gateway
  "providers": {
    "anthropic": {
      "apiKey": "${ANTHROPIC_API_KEY}"
    }
  },
  // Model routing (optional, Gateway handles default routing)
  "models": {
    "default": "gateway/auto"
  }
}
```

Generated at `~/.config/kilo/kilo.jsonc` alongside existing `opencode.json`.

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | `KiloModelAlias` validation, `KiloModelID` resolution | Table-driven tests in `kilo_model_test.go` |
| Unit | Adapter capabilities (`SupportsSubAgents`, paths) | Extend existing `TestCapabilities` in `adapter_test.go` |
| Unit | `kiloModelResolver` interface satisfaction | Compile-time check + test in adapter_test.go |
| Integration | Sub-agent file injection via `sdd.Inject` | Add Kilo cases to `inject_test.go` — verify `.kilo/agents/sdd-apply.md` written with correct model |
| Integration | Post-injection verification | Test that missing `sdd-apply.md` triggers error |
| E2E | `gentle-ai sync --agent kilocode --dry-run` | Verify native agent plan shows in output |

## Migration / Rollout

No data migration required. The change is additive:

1. New `SupportsSubAgents() = true` means native agent files are written alongside existing `opencode.json` overlay
2. Existing `opencode.json` merge continues for system prompt and MCP — no breakage
3. Users who already have `~/.config/kilo/opencode.json` get native agents on next `gentle-ai sync`
4. Rollback: revert `SupportsSubAgents()` to `false`, remove `kilocode/agents/` assets

## Open Questions

- [ ] Exact `kilo.jsonc` format — needs validation against Kilo Code v7 running instance. The provider config shape is based on Kiro's `settings.json` pattern; Kilo may differ.
- [ ] Kilo Gateway model ID format — `gateway/auto` is a placeholder. Actual IDs depend on Kilo Gateway's undocumented API. Isolate in `KiloModelID()` so format changes are localized.
- [ ] Profile path `~/.config/kilo/profiles/` — detection-only for now. If Kilo v7 exposes profile APIs, this can be extended.
