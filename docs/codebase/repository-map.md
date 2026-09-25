# Repository Map

[Back to Codebase Guide](../CODEBASE-GUIDE.md)

Use this page when you know what you need to change but not where it belongs.

## Package ownership

| Path | Owns | Do not put here |
|---|---|---|
| `cmd/gentle-ai/` | Binary entrypoint and version handoff. | Business rules or file mutation logic. |
| `internal/app/` | Top-level command dispatch, help, version routing. | Component-specific install behavior. |
| `internal/cli/` | Non-interactive install, sync, uninstall, restore, and flag normalization. | Agent path constants. |
| `internal/tui/` | Bubbletea model, screen routing, async messages, interactive flows. | CLI-only flag parsing or component internals. |
| `internal/model/` | Shared IDs, enums, and cross-package domain structs. | Platform-specific behavior. |
| `internal/catalog/` | Supported agents and component definitions. | Detection, installation, or mutation code. |
| `internal/system/` | OS detection, dependency checks, path guards. | Agent config injection. |
| `internal/planner/` | Dependency graph resolution and component ordering. | UI rendering or file writes. |
| `internal/pipeline/` | Staged execution, progress, rollback policy. | Component decision logic. |
| `internal/components/` | Reusable component injection and verification helpers, including Engram, ODD guidance, review assets, MCP, persona, skills, GGA, community tools, OpenCode plugins, uninstall, and file merge helpers. | Per-agent strategy definitions. |
| `internal/components/communitytool/` | Community tool installation orchestration plus managed guidance/config/MCP reconciliation, currently for CodeGraph. | OpenCode plugin registration or external tool runtime implementation. |
| `internal/components/uninstall/` | Managed cleanup services for installed component artifacts. | Interactive TUI state or backup storage. |
| `internal/agents/` | Adapter strategy, config paths, capability flags per agent. | Shared component behavior. |
| `internal/assets/` | Embedded prompts, skills, personas, commands, and templates. | Runtime-generated user state. |
| `internal/skillregistry/` | Skill registry scanning, cache behavior, and markdown generation. | Skill authoring rules or component injection. |
| `internal/state/` | `~/.gentle-ai/state.json` persisted install selections. | Engram memory state. |
| `internal/update/` | Update checks, self-update, and upgrade support. | Config sync semantics. |
| `internal/update/upgrade/` | External tool upgrade execution and upgrade report rendering. | Install planning or component selection. |
| `internal/verify/` | Readiness checks and rendered reports. | Mutation side effects. |
| `docs/` | User and maintainer documentation. | Source-of-truth behavior not backed by code. |
| `e2e/` | Docker-based end-to-end test harness. | Unit test fixtures. |
| `testdata/` | Golden files and fixtures. | Generated local machine state. |

## If you need X, read Y

| If you need to... | Start with... | Then check... |
|---|---|---|
| Add a supported agent | `internal/model/types.go`, `internal/catalog/agents.go` | `internal/agents/<agent>/`, `docs/agents.md` |
| Change Engram setup | `internal/components/engram/` | `docs/engram.md`, `internal/assets/*/engram-*` |
| Change OpenCode background-subagent guidance | `internal/components/agentguidance/` | `docs/opencode-profiles.md`, `internal/cli/opencode_background_test.go` |
| Change CLI flags | `internal/cli/` | `docs/usage.md`, app dispatch tests |
| Change TUI flow | `internal/tui/model.go`, `internal/tui/router.go` | `internal/tui/screens/` |
| Change install ordering | `internal/planner/`, `internal/pipeline/` | component tests and dry-run output |
| Change backups or restore | `internal/backup/`, `internal/cli/restore.go` | `docs/rollback.md` |
| Change updates or upgrades | `internal/update/`, `internal/update/upgrade/` | `internal/tui/screens/upgrade_sync.go` |
| Change OpenCode TUI plugins | `internal/components/opencodeplugin/` | README community integrations |
| Change community tool install or guidance | `internal/components/communitytool/` | `docs/components.md`, `docs/usage.md` |
| Change skill registry behavior | `internal/skillregistry/`, `internal/app/` skill-registry dispatch | `docs/skill-registry.md`, `.atl/skill-registry.md` |
| Update contributor-facing docs | `docs/CODEBASE-GUIDE.md` | `README.md` docs table |

## Golden placement rule

Place code at the narrowest stable owner:

1. **One agent only** -> adapter package.
2. **One component across agents** -> component package.
3. **Ordering or dependencies** -> planner.
4. **Execution lifecycle** -> pipeline.
5. **Interactive state only** -> TUI.
6. **User command surface** -> CLI plus docs.

## Contributor checklist

- [ ] Confirm the change has one clear owner.
- [ ] Avoid copying path rules between adapters.
- [ ] Keep generated/user state out of `internal/assets/`.
- [ ] Update docs at the entry point users will actually read.

## Navigation

Previous: [Mental model](mental-model.md) | Next: [Memory core](memory-core.md)
