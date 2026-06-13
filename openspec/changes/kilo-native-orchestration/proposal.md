# Proposal: Kilo Native Orchestration

## Intent

Kilo Code v7 rejects the OpenCode overlay approach (issue #729) because `opencode.json` agents with `"mode": "primary"` cannot be used as subagents. The current adapter also returns `SupportsSubAgents() = false`, so SDD multi-mode doesn't work at all. Additionally, Kilo Gateway provides free model routing that we're not leveraging. This change fixes the integration by switching to Kilo's native `.kilo/agents/*.md` format (matching Kiro's pattern) and adds `kilo.jsonc` generation for provider config.

## Scope

### In Scope
- Fix issue #729: replace OpenCode overlay with native `.kilo/agents/*.md` sub-agent files
- Enable `SupportsSubAgents()` returning `true` with `EmbeddedSubAgentsDir() = "kilocode/agents"`
- Add `kilocode/agents/*.md` asset templates (mirroring `kiro/agents/`)
- Add `kilo.jsonc` generation for provider/permission config
- Add `KiloModelAlias` support and `kiloModelResolver` interface
- Add profile detection for `~/.config/kilo/profiles/`
- Kilo-specific injection path in `sdd/inject.go`
- Post-injection verification for Kilo Code
- Update adapter tests

### Out of Scope
- Modifying the shared OpenCode overlay format
- Kilo Cloud agent features
- Kilo Code upstream changes

## Capabilities

### New Capabilities
- `kilo-native-agents`: Native `.kilo/agents/*.md` sub-agent file generation and injection
- `kilo-provider-config`: `kilo.jsonc` generation for provider and permission configuration
- `kilo-gateway-routing`: Per-phase model routing via Kilo Gateway free models

### Modified Capabilities
- `sdd-orchestrator-assets`: Kilo adapter gains sub-agent support and model resolver interface

## Approach

Mirror the Kiro adapter pattern exactly. The Kiro adapter already implements `.kiro/agents/*.md` with `SupportsSubAgents() = true`, `EmbeddedSubAgentsDir()`, and `kiroModelResolver`. We replicate this for Kilo:

1. Enable `SupportsSubAgents()` → `true`, set `SubAgentsDir()` → `~/.kilo/agents/`
2. Add `internal/assets/kilocode/agents/` with SDD phase templates (frontmatter + instructions)
3. Implement `kiloModelResolver` interface for model alias resolution
4. Add `kilo.jsonc` generation alongside existing `opencode.json` merge
5. Add profile detection for `~/.config/kilo/profiles/`
6. Update `sdd/inject.go` to handle Kilo's native agent path (bypass overlay merge for sub-agents)
7. Add post-injection verification for agent file completeness

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/agents/kilocode/adapter.go` | Modified | Enable sub-agents, add model resolver |
| `internal/assets/kilocode/agents/` | New | SDD phase agent templates |
| `internal/components/sdd/inject.go` | Modified | Kilo-specific injection path |
| `internal/model/types.go` | Modified | Add `KiloModelAlias` type |
| `internal/agents/kilocode/adapter_test.go` | Modified | Update test counts and cases |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Kilo Gateway API changes break model routing | Low | Pin to documented v7 API; isolate in adapter |
| `kilo.jsonc` format diverges from `opencode.json` | Med | Test against actual Kilo v7 config; document known format |
| Profile path `~/.config/kilo/profiles/` undocumented | Med | Detect-only; graceful fallback if absent |

## Rollback Plan

Revert `SupportsSubAgents()` to `false`, remove `kilocode/agents/` assets, remove `kilo.jsonc` generation. The OpenCode overlay path remains functional as fallback.

## Dependencies

- Kilo Code v7 installed for testing
- Existing Kiro adapter as reference implementation

## Success Criteria

- [ ] `go build ./...` and `go vet ./...` pass clean
- [ ] `go test ./internal/agents/kilocode/...` — sub-agent support, model resolver, config paths pass
- [ ] `go test ./internal/components/sdd/...` — Kilo injection cases pass
- [ ] `.kilo/agents/sdd-apply.md` and `.kilo/agents/sdd-verify.md` written correctly
- [ ] `kilo.jsonc` generated with provider config
- [ ] `gentle-ai install --agent kilocode --dry-run` shows native agent plan
