# PI Compatibility (Gap-Closure Scope)

This document tracks the current PI contract in gentle-ai after `pi-support-gap-closure`.

## Contract Summary

- PI root resolution uses `PI_CODING_AGENT_DIR` first, then defaults to `~/.pi/agent`.
- Global PI root stores settings/prompt/skills (`<resolved-root>`), while PI generated multi-agent SDD artifacts are written to project-root `.pi/agents/*`.
- Legacy `~/.config/pi-coding-agent` remains detection-only compatibility.
- PI uses the existing **PI→Engram extension** contract as the integration baseline.
- Contract can come from explicit `engram.extension.path` + `engram.extension.enabled=true`, or from `packages[]` entries that identify PI Engram (`../../.../pi-engram`, `npm:pi-engram`, scoped variants).
- MCP and Context7 are out of scope for this iteration and are NOT blockers for PI support.
- Generated `.pi/agents/sdd.chain.md` uses native `pi-subagents` chain syntax: frontmatter + `## <agent-name>` step headings (`sdd-init` → `sdd-archive`).
- `sdd-onboard` remains an optional standalone agent (not part of the default linear chain).

PI multi-model SDD remains enabled only when `pi-subagents` is detected.

Prerequisite/remediation command:

```bash
pi install npm:pi-subagents
```

- Canonical preflight message when unavailable:

> PI multi-model requires installing the `pi-subagents` extension.

## Capability Matrix

| PI Runtime State | `profiles` | `modelPicker` | `generatedMulti` | Behavior |
|---|:---:|:---:|:---:|---|
| PI without `pi-subagents` | false | false | false | Base single-mode only; multi-mode requests fail with canonical message |
| PI with `pi-subagents` | true | true | true | Multi-model SDD enabled; `.pi/agents/*.md` + `sdd.chain.md` generated |

## Guardrails

- PI multi-model assignments must be explicit `provider/model` values.
- PI phases must not fabricate `openai/gpt-5` when no explicit assignment exists.
- OpenCode and Claude/Kiro behavior must remain unchanged by PI-specific changes.
- MCP and Context7 are out of scope for this iteration.

## Regression Evidence (OpenCode Safety)

Run these focused suites when touching PI capability gating, sync/preflight, or SDD injection:

```bash
go test ./internal/tui -run "TestAgentsViewWithPISelectionShowsCapabilityWarning|TestRenderModelPicker_EmptyStateIncludesCapabilityGatedWarning|TestRenderModelPicker_PIEnabledOmitsLegacyOpenCodeOnlyWarning|TestRenderModelPicker_PIUnsupportedBlocksMultiModelControlsWithCanonicalWarning"
go test ./internal/cli -run "TestValidatePiMultiModelPreflightFailsWithoutPiSubagents|TestValidatePiMultiModelPreflightAllowsWhenPiSubagentsPresent|TestValidatePiMultiModelPreflightNoopForOpenCodeRegressionSafety"
go test ./internal/components/sdd -run "TestInjectOpenCode|TestInjectPI"
go test ./...
```

## Final Verify Checklist

- [ ] Resolver matrix passes (PI detected/absent/ambiguous fail-closed).
- [ ] TUI PI gating matches capabilities (single-mode fallback without extension, multi-mode when present).
- [ ] CLI/preflight emits exact canonical message + install remediation command.
- [ ] PI `.pi/agents` artifacts are generated only when capability is enabled.
- [ ] OpenCode regression suites pass unchanged.
