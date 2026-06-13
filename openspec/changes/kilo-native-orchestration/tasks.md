# Tasks: Kilo Native Orchestration

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~350–500 |
| 400-line budget risk | Medium |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 (model + adapter) → PR 2 (templates + embed) → PR 3 (inject + kilo.jsonc) → PR 4 (tests) |
| Delivery strategy | ask-on-risk |
| Chain strategy | stacked-to-main |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: Medium

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | Model types + adapter capability | PR 1 | Base: main. Foundation; no tests needed yet. |
| 2 | Agent templates + embed directive | PR 2 | Depends on PR 1. 10 .md template files + assets.go change. |
| 3 | Injection logic + kilo.jsonc generation | PR 3 | Depends on PR 2. Core runtime behavior. |
| 4 | Tests for model, adapter, injection | PR 4 | Depends on PR 3. Covers all prior work. |

## Phase 1: Model Types & Adapter Capability

- [ ] 1.1 Create `internal/model/kilo_model.go`: define `KiloModelAlias` type, constants (`KiloModelAuto`, `KiloModelSonnet`, `KiloModelOpus`, `KiloModelHaiku`, `KiloModelGateway`), `Valid()` method, `KiloModelID()` resolver, and `KiloModelPresetBalanced()` preset function. Mirror `kiro_model.go` structure exactly.
- [ ] 1.2 Modify `internal/agents/kilocode/adapter.go`: change `SupportsSubAgents()` to return `true`, `SubAgentsDir()` to return `filepath.Join(homeDir, ".kilo", "agents")`, `EmbeddedSubAgentsDir()` to return `"kilocode/agents"`. Add `KiloModelID(alias model.KiloModelAlias) string` method that delegates to `model.KiloModelID`.

## Phase 2: Agent Templates & Embed

- [ ] 2.1 Create directory `internal/assets/kilocode/agents/`. Create 10 agent template files (`sdd-apply.md`, `sdd-verify.md`, `sdd-design.md`, `sdd-spec.md`, `sdd-tasks.md`, `sdd-explore.md`, `sdd-propose.md`, `sdd-archive.md`, `sdd-init.md`, `sdd-onboard.md`). Each file must have YAML frontmatter with `name`, `description`, `tools`, `model: {{KILO_MODEL}}`, `includeMcpJson: true`, and body instructions pointing to `~/.config/kilo/skills/<phase>/SKILL.md`. Mirror kiro agent templates but with Kilo paths.
- [ ] 2.2 Modify `internal/assets/assets.go`: add `all:kilocode` to the `//go:embed` directive (line 5) so the new templates are embedded.

## Phase 3: Injection Logic & Config Generation

- [ ] 3.1 Modify `internal/components/sdd/inject.go`: add `kiloModelResolver` interface (same shape as `kiroModelResolver`). In the sub-agent copy loop (step 3c), add a block that checks if adapter implements `kiloModelResolver`, resolves `{{KILO_MODEL}}` sentinel using `KiloModelAlias` from `InjectOptions.KiloModelAssignments` (with fallback to default), and stamps the resolved model ID into agent frontmatter.
- [ ] 3.2 Add `KiloModelAssignments map[string]model.KiloModelAlias` field to `InjectOptions` struct in `inject.go`.
- [ ] 3.3 Add Kilo-specific post-injection verification in `inject.go`: after sub-agent files are written for a Kilo adapter, verify all expected `.kilo/agents/sdd-*.md` files exist, are non-empty, and have valid YAML frontmatter (at minimum check `name:` field exists). On failure, return error listing missing files.
- [ ] 3.4 Add `kilo.jsonc` generation: after sub-agent injection, generate `kilo.jsonc` at workspace root (`opts.WorkspaceDir` + `kilo.jsonc`) with a `providers` block containing a `kilo-gateway` entry (baseUrl placeholder, apiKey placeholder) and a `models.default` of `"gateway/auto"`. Skip if `opts.WorkspaceDir` is empty.

## Phase 4: Tests

- [ ] 4.1 Create `internal/model/kilo_model_test.go`: table-driven tests for `KiloModelAlias.Valid()`, `KiloModelID()` resolution for all aliases plus unknown fallback, and `KiloModelPresetBalanced()` coverage (all phases have non-empty aliases).
- [ ] 4.2 Modify `internal/agents/kilocode/adapter_test.go`: add `SupportsSubAgents` → `true` to `TestCapabilities`. Add tests for `SubAgentsDir("/home/user")` → `"/home/user/.kilo/agents"`, `EmbeddedSubAgentsDir()` → `"kilocode/agents"`. Add compile-time check that `*Adapter` satisfies `kiloModelResolver` interface.
- [ ] 4.3 Modify `internal/components/sdd/inject_test.go`: add test case `TestInjectKilocodeWritesNativeAgentFiles` — run `Inject` with a kilocode adapter, verify `.kilo/agents/sdd-apply.md` and `.kilo/agents/sdd-verify.md` exist with valid frontmatter and resolved model ID. Add test case `TestInjectKilocodeVerificationFailsOnMissing` — verify error when expected agent file is missing.
