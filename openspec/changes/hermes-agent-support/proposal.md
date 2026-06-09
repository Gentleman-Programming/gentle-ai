# Proposal: Hermes Agent Support

Add Hermes (Nous Research) as a supported coding agent in the Gentle AI ecosystem so users who run Hermes get the same first-class treatment as Codex, Gemini, Qwen, and the detect-only agents: MCP wiring (context7 + engram), SDD orchestrator instructions, and strict-TDD/persona protocol — all injected into Hermes's native config and `SOUL.md`. Hermes is a detect-only agent (no auto-install) and uses a YAML config, so this change introduces a new `StrategyMergeIntoYAML` MCP strategy backed by hand-rolled, comment-preserving YAML helpers.

## Why

- **Coverage gap.** Gentle AI configures every other agent in its catalog, but a Hermes user gets nothing today. They must hand-wire context7/engram MCP servers and copy the SDD protocol manually — exactly the toil Gentle AI exists to remove.
- **New config shape, no tooling yet.** Hermes stores MCP servers in `~/.hermes/config.yaml` under a top-level `mcp_servers` key. The repo has no YAML strategy and intentionally no YAML library (see Risks). Adding Hermes is the trigger to define a clean, dependency-free YAML merge path that future YAML agents can reuse.
- **Parity expectation.** Hermes ships its own native persistent memory and skill-learning loop, but the product decision (locked) is to inject engram anyway for ecosystem parity. The value is a consistent cross-agent memory protocol; the overlap is handled by documentation, not by skipping the feature.

## Goals

- Register Hermes as a `TierFull` agent that Gentle AI can detect, validate, select in the TUI, and configure.
- Inject context7 + engram MCP servers into `~/.hermes/config.yaml` without destroying user content or comments, idempotently.
- Inject engram-protocol, SDD-orchestrator, and strict-TDD instructions into `~/.hermes/SOUL.md` using marker-based markdown sections.
- Document the complementary relationship between engram and Hermes's native memory inside `SOUL.md` so users understand why both exist.
- Keep all changes additive — zero behavior change for existing agents.

## Non-Goals

| Non-goal | Reason |
|----------|--------|
| Auto-install Hermes | Locked decision: detect-only. Hermes only ships a `curl \| bash` installer (Python project, no pinnable npm package). The repo only auto-installs via pinned npm packages with `--ignore-scripts` or controlled package managers. A `curl \| bash` install conflicts with that supply-chain posture, so Hermes joins the detect-only group (openclaw/cursor/kiro/antigravity/trae/vscode/windsurf). |
| Profile-based config (`~/.hermes/profiles/<name>/`) | This change targets the global `~/.hermes/config.yaml` only. Per-profile injection is a documented limitation, not a target. |
| Permissions overlay | Hermes's permission model/format is not yet documented. Permissions injection is skipped initially (return nil) until the format is known. |
| Native `engram setup` slug | No native `engram setup` target for Hermes initially; direct YAML injection covers MCP wiring. `SetupAgentSlug` returns `"", false`. |
| Full YAML parser support (anchors, multi-doc, deep nesting) | The hand-rolled helpers cover only the flat key-value + single nested `mcp_servers` table that Hermes actually uses. |

## Approach

The change is a hybrid of two existing patterns: the **OpenClaw** detect-only adapter and the **Codex** string-based non-JSON config merge.

1. **Adapter (mirror OpenClaw, minus workspace logic).** New `internal/agents/hermes` package with an `Adapter` implementing the `agents.Adapter` interface. `SupportsAutoInstall()` returns `false`; `InstallCommand` returns the not-installable error (same as OpenClaw). Detection is `lookPath("hermes")`. Unlike OpenClaw, Hermes is **global-only** — no `resolveWorkspaceDir` / `validateOpenClawWorkspacePath` logic is needed.

2. **New MCP strategy: `StrategyMergeIntoYAML`.** Add `StrategyMergeIntoYAML MCPStrategy = 4` to the existing `MCPStrategy` enum in `internal/model/types.go` (joins `StrategySeparateMCPFiles`, `StrategyMergeIntoSettings`, `StrategyMCPConfigFile`, `StrategyTOMLFile`).

3. **Hand-rolled YAML merge (mirror Codex `toml.go`).** New `internal/components/filemerge/yaml.go` with string-based helpers that upsert MCP server blocks under the `mcp_servers:` key — strip-then-re-append, idempotent, comment-preserving for content outside managed blocks. This mirrors `UpsertCodexEngramBlock` / `UpsertCodexMCPServerBlock` exactly. **No `gopkg.in/yaml.v3` dependency** is added (round-trip Marshal destroys user comments and inflates the binary). The MCP and engram injection switches gain a `StrategyMergeIntoYAML` case routing to a new `injectYAMLFile` helper.

4. **SOUL.md instruction injection (`StrategyMarkdownSections`).** `~/.hermes/SOUL.md` is the system prompt ("slot #1") and is guaranteed loaded every session. The standard markdown-sections flow writes to `SystemPromptFile(homeDir)`, which resolves directly to `~/.hermes/SOUL.md`. **No persona special-casing is needed** — unlike OpenClaw (which writes SOUL.md into the workspace dir), Hermes is global, so the standard flow handles it. Engram-protocol, SDD-orchestrator, and strict-TDD instructions are injected via `<!-- gentle-ai:... -->` markers.

5. **Engram/Hermes memory documentation.** The engram protocol section injected into `SOUL.md` must explicitly explain the complementary relationship between engram (cross-agent, cross-session memory protocol) and Hermes's native memory/skill-learning loop, so users do not perceive them as conflicting.

6. **Catalog, factory, scan, validate, TUI, assets.** Standard additive registration so Hermes shows up everywhere other agents do.

## Scope summary

High-level touch surface (~25 files). Full task breakdown belongs in `tasks.md`.

### Net-new

| File | Mirrors |
|------|---------|
| `internal/agents/hermes/adapter.go` | OpenClaw adapter (detect-only) + YAML strategy |
| `internal/agents/hermes/adapter_test.go` | qwen `adapter_test.go` |
| `internal/assets/hermes/sdd-orchestrator.md` | qwen `sdd-orchestrator.md` (with `~/.hermes/skills/` paths) |
| `internal/components/filemerge/yaml.go` | codex `toml.go` (no YAML equivalent exists yet) |
| `internal/components/filemerge/yaml_test.go` | codex `toml_test.go` — golden/table-driven |

### Modified (additive, mirror existing agent registration)

| File | Change |
|------|--------|
| `internal/model/types.go` | add `AgentHermes` + `StrategyMergeIntoYAML = 4` |
| `internal/agents/factory.go` | register adapter + default agent IDs |
| `internal/catalog/agents.go` | catalog entry (`TierFull`, `~/.hermes`) |
| `internal/assets/assets.go` | `all:hermes` embed directive |
| `internal/components/sdd/inject.go` | `sddOrchestratorAsset()` case + YAML MCP path |
| `internal/components/mcp/inject.go` | `StrategyMergeIntoYAML` case + `injectYAMLFile()` |
| `internal/components/engram/inject.go` | YAML overlay + `isStandardAgent()` |
| `internal/components/engram/setup.go` | slug returns `"", false` |
| `internal/components/permissions/inject.go` | skip (return nil) until format known |
| `internal/system/config_scan.go` | `knownAgentConfigDirs()` entry |
| `internal/cli/validate.go` | agent validation case |
| `internal/tui/model.go` | TUI selection case |
| Tests: `inject_test.go` (sdd/mcp/engram), `setup_test.go`, `registry_test.go`, `config_scan_test.go` | add Hermes cases / counts |

Note: `internal/components/persona/inject.go` is intentionally **not** modified — Hermes needs no special-case (standard global SOUL.md flow handles it).

## Risks & mitigations

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| YAML indentation sensitivity — wrong indentation silently produces invalid/misread config | Med | Helpers always emit consistent 2-space indentation; rigorous golden-file tests in `yaml_test.go` covering insert/upsert/absent-key cases |
| YAML comment loss inside managed blocks | Low | Strip+re-append preserves everything outside the managed `mcp_servers` block; managed blocks are gentle-ai-owned by contract |
| Idempotency when `mcp_servers:` key is absent on first run | Med | Helper creates the key on first run and detects the created key on subsequent runs to avoid duplication; covered by tests |
| Hermes schema immaturity — Nous may change `mcp_servers` structure | Med | New/emerging agent; isolate schema knowledge in `yaml.go` so a schema change is a localized fix; pin behavior with tests |
| Engram / Hermes native-memory overlap confuses users | Med | SOUL.md engram section explicitly documents the complementary relationship (locked decision) |
| Native skill-format conflict — Hermes skills may use a different format than gentle-ai's `SKILL.md` | Med | Validate Hermes skill format before writing to `~/.hermes/skills/`; SDD orchestrator lives in SOUL.md regardless, so skills are reference-only |
| Profiles limitation — users on `~/.hermes/profiles/<name>/` won't get MCP servers | Low | Documented non-goal; global config is the target |

## Open questions

- **Permissions format.** Hermes's permission model is undocumented. Confirmed as a non-goal for this slice (skip injection). To revisit once the format is published.
- **Skill format.** Whether Hermes skills accept gentle-ai's `SKILL.md` frontmatter format needs validation during spec/design. Does not block the SOUL.md-based orchestrator injection.

## Success criteria

- [ ] `go build ./...` and `go vet ./...` pass clean
- [ ] `go test ./internal/components/filemerge/...` — YAML upsert insert/upsert/absent-key golden tests pass
- [ ] `go test ./internal/agents/hermes/...` — detection, not-installable error, config paths, capabilities pass
- [ ] `go test ./internal/components/{sdd,mcp,engram}/...` — Hermes injection cases pass
- [ ] `go test ./internal/{agents,system}/...` — registry + config-scan counts updated
- [ ] Hermes appears in the TUI agent selection screen
- [ ] `gentle-ai install --agent hermes --dry-run` shows context7 + engram + SOUL.md plan, no auto-install step
