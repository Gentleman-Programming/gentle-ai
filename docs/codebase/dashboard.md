# Dashboard

[Back to Codebase Guide](../CODEBASE-GUIDE.md)

Gentle-AI does not currently contain dashboard source code. This page exists to prevent accidental invention of dashboard, HTMX, auth, or admin behavior while documenting the codebase.

## Source validation result

| Expected dashboard area | Status in this repository |
|---|---|
| Dashboard package | Not found. |
| HTMX templates or handlers | Not found. |
| Local HTTP server routes | Not found. |
| Auth/session/admin boundary | Not implemented here. |
| Dashboard tests | Not found. |

## What exists instead

| User surface | Source owner |
|---|---|
| Interactive terminal UI | `internal/tui/` |
| CLI commands | `internal/app/`, `internal/cli/` |
| Engram memory browser command mention | `docs/engram.md` documents `engram tui`; implementation is external. |
| OpenCode community plugin registration | `internal/components/opencodeplugin/` updates `~/.config/opencode/tui.json`. |

## HTMX/server-rendered flow boundary

No HTMX or server-rendered dashboard flow is present. If a future dashboard is added, document the actual request flow here instead of guessing. A minimal future shape should answer:

- [ ] Which package owns HTTP routing?
- [ ] Which templates are server-rendered?
- [ ] Which state is read-only vs mutable?
- [ ] Which auth/session layer protects admin actions?
- [ ] Which tests prove route and auth behavior?

## Dashboard invariants for future work

- **No unauthenticated admin actions**: dashboard writes must have an explicit auth/session boundary.
- **No full API duplication**: link to the endpoint/schema source of truth when one exists.
- **No direct local DB assumptions**: do not read `.engram/engram.db` from Gentle-AI dashboard code unless the architecture explicitly chooses that boundary.
- **No silent cloud coupling**: remote state must be isolated behind a transport/client interface.

## Contributor checklist

- [ ] Search the source tree before documenting dashboard behavior.
- [ ] Mark absent functionality as absent instead of filling gaps with assumptions.
- [ ] Keep terminal TUI docs separate from web dashboard docs.

## Navigation

Previous: [Sync and cloud](sync-and-cloud.md) | Next: [Integrations](integrations.md)
