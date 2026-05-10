# Reference Map

[Back to Codebase Guide](../CODEBASE-GUIDE.md)

This appendix maps main docs and source files to responsibilities. Use it to make claims traceable during review.

## Documentation references

| Doc | Responsibility |
|---|---|
| `README.md` | Product landing page, install paths, main docs table. |
| `docs/CODEBASE-GUIDE.md` | Maintainer codebase guide index. |
| `docs/architecture.md` | Short architecture and development reference. |
| `docs/usage.md` | CLI, TUI, flags, and typical workflows. |
| `docs/engram.md` | Engram command and MCP tool reference. |
| `docs/agents.md` | Supported agents, delegation model, and per-agent notes. |
| `docs/components.md` | Components, skills, and presets. |
| `docs/opencode-profiles.md` | OpenCode SDD profile behavior. |
| `docs/rollback.md` | Backup and restore behavior. |
| `docs/platforms.md` | Platform support and path notes. |

## Source references

| Source path | Responsibility |
|---|---|
| `cmd/gentle-ai/main.go` | Binary entrypoint and version handoff. |
| `internal/app/` | Command dispatch, help, app-level version/update routing. |
| `internal/cli/run.go` | Install flow orchestration. |
| `internal/cli/sync.go` | Managed config sync flow and SDD profile flags. |
| `internal/cli/uninstall.go` | Non-interactive uninstall flow. |
| `internal/tui/model.go` | Interactive state machine and async messages. |
| `internal/tui/router.go` | TUI route relationships. |
| `internal/model/types.go` | Shared IDs and strategy enums. |
| `internal/catalog/agents.go` | Supported agent list. |
| `internal/catalog/components.go` | Component catalog. |
| `internal/planner/resolver.go` | Dependency expansion and ordering. |
| `internal/pipeline/` | Staged execution and rollback. |
| `internal/components/engram/` | Engram install, setup, MCP injection, and verification wiring. |
| `internal/components/sdd/` | SDD prompt/profile generation and injection. |
| `internal/components/opencodeplugin/` | Optional OpenCode plugin registration. |
| `internal/agents/` | Per-agent adapter strategies and paths. |
| `internal/state/state.go` | Persisted install state in `~/.gentle-ai/state.json`. |
| `internal/update/` | Update checks and upgrade execution. |
| `internal/verify/` | Post-apply readiness reporting. |
| `e2e/` | Docker E2E harness. |
| `testdata/` | Golden fixtures for generated outputs. |

## Missing-source references

| Requested topic | Traceable conclusion |
|---|---|
| Dashboard | No dashboard, HTMX, or HTTP server package was found in this repository. |
| Cloud sync | No cloud server/cloud store implementation was found in this repository. |
| Engram store schema | Sessions, observations, prompts, relations, and sync mutations are external Engram runtime concepts, not Gentle-AI source files. |
| Full API reference | No `DOCS.md` exists in this repository; use existing focused docs and external Engram docs when needed. |

## Review checklist

- [ ] Every new claim points to a source file or an existing doc.
- [ ] Missing features are labeled as missing, not implied.
- [ ] README links to the guide landing page, not deep subpages.

## Navigation

Previous: [Maintainer playbook](maintainer-playbook.md) | Back: [Codebase Guide](../CODEBASE-GUIDE.md)
