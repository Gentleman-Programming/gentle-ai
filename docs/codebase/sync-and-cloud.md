# Sync and Cloud

[Back to Codebase Guide](../CODEBASE-GUIDE.md)

Gentle-AI sync refreshes managed agent configuration. Engram sync exports/imports memory. Cloud sync is not implemented in this source tree.

## Sync boundaries

| Flow | Command surface | Owner | What changes |
|---|---|---|---|
| Gentle-AI config sync | `gentle-ai sync` | `internal/cli/sync.go`, components, adapters | Agent prompts, skills, MCP configs, SDD profiles, GGA assets. |
| Engram git-friendly sync | `engram sync`, `engram sync --import` | External Engram runtime | `.engram/` memory export/import for team sharing. |
| Cloud sync | Not present in Gentle-AI source | External or future Engram capability | Do not document implementation here without source. |
| Autosync | Not present in Gentle-AI source | External or future Engram capability | Do not imply background sync exists in this repo. |

## Gentle-AI sync path

```text
gentle-ai sync
  -> parse sync flags
  -> discover installed agents from ~/.gentle-ai/state.json or explicit flags
  -> build managed selection
  -> run component injectors
  -> verify readiness
  -> report files changed or no-op
```

Important behavior from `internal/cli/sync.go`:

- Default sync scope includes SDD, Engram, Context7, GGA, and skills.
- Persona, permissions, and theme are user-adjacent and not included by default.
- OpenCode SDD profile flags preserve and update profile model assignments.
- Idempotency matters: `FilesChanged == 0` means managed assets were already current.

## Git-friendly memory sync

Engram team sharing is documented in [Engram Commands](../engram.md). The important maintainer distinction: `engram sync` exports memory to `.engram/`; `gentle-ai sync` refreshes agent configuration.

## Remote transport boundary

No remote transport implementation is present in this repository beyond update/download logic for external binaries and releases. Do not describe an Engram cloud transport, cloud server, or cloud store split as Gentle-AI code unless that code is added here.

## Cloud server/cloud store split

This repository does not contain cloud server or cloud store packages. If future Engram cloud docs are added, document them as an external Engram responsibility and keep this page focused on how Gentle-AI discovers, installs, or configures that capability.

## Contributor checklist

- [ ] Use `gentle-ai sync` for managed config, not memory export/import.
- [ ] Use `engram sync` docs for memory sharing behavior.
- [ ] Keep sync changes idempotent and test `FilesChanged` expectations.
- [ ] Do not touch untracked local `.engram/cloud.json` or `.engram/engram.db` during docs or sync work.

## Navigation

Previous: [Interfaces](interfaces.md) | Next: [Dashboard](dashboard.md)
