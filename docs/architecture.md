# Architecture & Development

> [!NOTE]
> These docs track `main`, which may include unreleased changes. For the latest release, see the [v4.0.0 docs](https://github.com/Gentleman-Programming/gentle-ai/tree/v4.0.0/docs).

← [Back to README](../README.md)

---

## Architecture

```
cmd/gentle-ai/             CLI entrypoint
cmd/gentle-telemetry/      Self-hosted telemetry collector (see telemetry-collector.md)
internal/
  app/                     Command dispatch + runtime wiring
  model/                   Domain types (agents, components, skills, presets, personas)
  catalog/                 Registry definitions (agents, skills, components)
  system/                  OS/distro detection, dependency checks, platform guards
  cli/                     Install flags, validation, orchestration, dry-run
  planner/                 Dependency graph, resolution, ordering, review payloads
  installcmd/              Profile-aware command resolver (brew/apt/pacman/dnf/winget/go install)
  pipeline/                Staged execution + rollback orchestration
  backup/                  Config snapshot + restore
  assets/                  Embedded skill files + persona templates
  components/              Per-component install/inject logic
    engram/  skills/  mcp/  persona/  theme/  permissions/  gga/
    agentguidance/         Always-installed routing guidance projected into every agent
    agenthooks/            Runtime hook installation independent of optional components
    telemetryruntime/      Native runtime telemetry event adapters and managed hooks
    reviewassets/          Runtime review contract rendering for each agent
    communitytool/         Community tool install/guidance/config orchestration
    opencodeagents/        OpenCode-compatible agents owned by gentle-ai
    opencodedefault/       OpenCode default_agent ownership and custom agent discovery
    opencodeplugin/        OpenCode TUI plugin registration/local plugin helpers
    opencoderuntimeplugins/  Names and eligibility of managed OpenCode runtime plugins
    uninstall/             Managed uninstall cleanup service
    legacyassets/          Retired managed paths for snapshots and rollback
    mutationjournal/       Before-images of mutated files for atomic rollback
    filemerge/             Marker-based file merging (inject without clobbering)
  skillregistry/           .atl skill registry refresh/list support
  agents/                  Agent adapters (config strategy per agent)
    <agent>/               One adapter per supported agent; see agents.md
  opencode/                OpenCode model/config parsing utilities
  state/                   Installation state tracking
  update/                  Self-update + upgrade logic
    upgrade/               Tool upgrade execution and reporting
  verify/                  Post-apply health checks + reporting
  tui/                     Bubbletea TUI (Rose Pine theme)
    styles/  screens/
scripts/                   Installer, release, and maintainer scripts (install.sh, release preflights, cross-lane battery)
e2e/                       Docker-based E2E tests (Ubuntu + Arch)
testdata/                  Golden test fixtures
```

---

## Testing

```bash
# Unit tests
go test ./...

# Docker E2E (Ubuntu + Arch, requires Docker)
RUN_FULL_E2E=1 RUN_BACKUP_TESTS=1 ./e2e/docker-test.sh

# Dry-run smoke test (macOS/Linux)
gentle-ai install --dry-run --agent claude-code --preset minimal

# Dry-run smoke test (Windows PowerShell)
gentle-ai.exe install --dry-run --agent claude-code --preset minimal
```

Test coverage is broad and changes frequently. Keep this section qualitative unless counts are generated automatically:

- Unit tests cover agent adapters, components, system detection, app dispatch, update/upgrade behavior, and TUI flows.
- Docker E2E tests exercise Ubuntu and Arch paths when `RUN_FULL_E2E=1` is enabled.
- Golden fixtures snapshot generated component output under `testdata/`.
- Full pipeline paths are tested: detection, planning, execution, backup, restore, and verification.
- Agent adapter tests include cross-platform path validation.

---

## Relationship to Gentleman.Dots

| | Gentleman.Dots | AI Gentle Stack |
|--|---------------|-----------------|
| **Purpose** | Dev environment (editors, shells, terminals) | AI development layer (agents, memory, skills) |
| **Installs** | Neovim, Fish/Zsh, Tmux/Zellij, Ghostty | Configures the [supported AI agents](agents.md) |
| **Overlap** | None — complementary | None — different layer |

Install Gentleman.Dots first for your dev environment, then AI Gentle Stack for the AI layer on top.

---

## License

MIT
