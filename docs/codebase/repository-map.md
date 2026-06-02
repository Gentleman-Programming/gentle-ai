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
| `internal/components/` | Reusable component injection and verification helpers. | Per-agent strategy definitions. |
| `internal/agents/` | Adapter strategy, config paths, capability flags per agent. | Shared component behavior. |
| `internal/assets/` | Embedded prompts, skills, personas, commands, and templates. | Runtime-generated user state. |
| `internal/state/` | `~/.gentle-ai/state.json` persisted install selections. | Engram memory state. |
| `internal/update/` | Update checks and self-upgrade support. | Config sync semantics. |
| `internal/verify/` | Readiness checks and rendered reports. | Mutation side effects. |
| `docs/` | User and maintainer documentation. | Source-of-truth behavior not backed by code. |
| `e2e/` | Docker-based end-to-end test harness. | Unit test fixtures. |
| `testdata/` | Golden files and fixtures. | Generated local machine state. |

## If you need X, read Y

| If you need to... | Start with... | Then check... |
|---|---|---|
| Add a supported agent | `internal/model/types.go`, `internal/catalog/agents.go` | `internal/agents/<agent>/`, `docs/agents.md` |
| Change Engram setup | `internal/components/engram/` | `docs/engram.md`, `internal/assets/*/engram-*` |
| Change SDD prompt sync | `internal/components/sdd/` | `docs/opencode-profiles.md`, `internal/assets/*/sdd-*` |
| Change CLI flags | `internal/cli/` | `docs/usage.md`, app dispatch tests |
| Change TUI flow | `internal/tui/model.go`, `internal/tui/router.go` | `internal/tui/screens/` |
| Change install ordering | `internal/planner/`, `internal/pipeline/` | component tests and dry-run output |
| Change backups or restore | `internal/backup/`, `internal/cli/restore.go` | `docs/rollback.md` |
| Change updates or upgrades | `internal/update/` | `internal/tui/screens/upgrade_sync.go` |
| Change OpenCode plugins | `internal/components/opencodeplugin/` | README community integrations |
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
