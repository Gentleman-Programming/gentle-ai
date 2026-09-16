# Components, Skills & Presets

← [Back to README](../README.md)

---

## Components

| Component | ID | Description |
|-----------|-----|-------------|
| Engram™ | `engram` | Persistent cross-session memory via MCP — auto-detection of project name, full-text search, git sync, project consolidation. See [engram repo](https://github.com/Gentleman-Programming/engram) |
| SDD | `sdd` | Optional Spec-Driven Development workflow, including `sdd-onboard`; selected explicitly when its formal phase artifacts are wanted |
| Skills | `skills` | Curated coding skill library |
| Context7 | `context7` | MCP server for live framework/library documentation |
| Persona | `persona` | Managed Gentleman/neutral persona injection, or unmanaged custom persona mode |
| Permissions | `permissions` | Security-first defaults and guardrails. Applied to Claude Code and OpenCode (the two adapters with permissions overlay support). Default sensitive-paths deny list: `~/.ssh/*`, `~/.ssh/**/*`, `**/*.pem`, `**/*.key`, `**/.env*`, `~/.credentials/*`, `~/.aws/credentials`, `~/.config/gh/hosts.yml`, `~/Library/Keychains/*`, `**/secrets/*`, `**/*.p12`, `**/*.pfx` |
| GGA | `gga` | Gentleman Guardian Angel — AI provider switcher |
| Theme | `theme` | Gentleman Kanagawa theme overlay |

ODD (Organic Driven Development) is shared routing guidance, not a separate component to install. Use it for everyday direct/delegated work; installing the SDD component does not select SDD for a task. See [ODD and recovery](usage.md#organic-driven-development-odd).

## Primary remote-authorization guidance

Always-installed agent guidance includes a canonical remote-operation boundary, independent of the optional persona, SDD, and permissions components. Local-development access does not authorize remote execution, transfer, or discovery/reuse of ambient SSH agents, ControlMaster sockets, credentials, or sessions. Ask for explicit destination, operation, and credential/session authorization; authorized work remains allowed within stricter user/runtime restrictions.

This first delivery covers the 15 non-Pi primary instruction carriers, not every executor role or named profile. Pi remains owned by `gentle-pi`. The behavioral section provides no sandbox or fresh-human-per-execution guarantee. Separately, OpenCode/Kilocode permission defaults ask for direct ssh/scp/sftp/rsync commands, without silently replacing personal allows or restrictions; existing installs must opt into permission sync. See [sync update instructions and limitations](usage.md#sync). Issue #4324 remains open for the remaining projections and native approvals.

## GGA Behavior

`gentle-ai install --component gga` installs/provisions the `gga` binary globally on your machine.

It does **not** run project-level hook setup automatically (`gga init` / `gga install`) because that should be an explicit decision per repository.

After global install, enable GGA per project with:

```bash
gga init
gga install
```

---

## Optional Community Tools

Community Tools are opt-in and are not included by presets or automatic detection. Select them from the installer’s **Community Tools/Plugins** screen.

| Tool | Behavior | Removal |
|---|---|---|
| CodeGraph | Installs its CLI and configures supported agent MCP/guidance integration. | Use CodeGraph’s upstream lifecycle commands. |
| RTK | On supported macOS/Linux architectures, downloads pinned `v0.49.0` to `~/.local/bin/rtk` and runs RTK’s global setup only for detected selected Claude Code, OpenCode, Codex CLI, and Pi installations. Telemetry is disabled for every RTK child process. | Follow RTK’s upstream manual removal guidance; Gentle AI does not automate RTK uninstall. |

RTK remains unavailable on Windows. Gentle AI does not edit shell profiles or `PATH`; if `~/.local/bin` is not already effective, add it manually:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

---

## Skills

### Included Skills (installed by gentle-ai)

22 skill files organized by category, embedded in the binary and injected into your agent's configuration:

#### SDD (Spec-Driven Development)

| Skill | ID | Description |
|-------|-----|-------------|
| SDD Init | `sdd-init` | Bootstrap SDD context in a project |
| SDD Explore | `sdd-explore` | Investigate codebase before committing to a change |
| SDD Propose | `sdd-propose` | Create change proposal with intent, scope, approach |
| SDD Spec | `sdd-spec` | Write specifications with requirements and scenarios |
| SDD Design | `sdd-design` | Technical design with architecture decisions |
| SDD Tasks | `sdd-tasks` | Break down a change into implementation tasks |
| SDD Apply | `sdd-apply` | Implement tasks following specs and design |
| SDD Verify | `sdd-verify` | Validate implementation matches specs |
| SDD Archive | `sdd-archive` | Sync delta specs to main specs and archive |
| SDD Onboard | `sdd-onboard` | Guided end-to-end SDD walkthrough on the real codebase |
| Judgment Day | `judgment-day` | Parallel adversarial review — two independent judges review the same target |

#### Foundation

| Skill | ID | Description |
|-------|-----|-------------|
| Go Testing | `go-testing` | Go testing patterns including Bubbletea TUI testing |
| Skill Creator | `skill-creator` | Create new AI agent skills following the Agent Skills spec |
| Skill Improver | `skill-improver` | Audit and improve existing skills against the repository style guide |
| Branch & PR | `branch-pr` | PR creation workflow with conventional commits, branch naming, and issue-first enforcement |
| Issue Creation | `issue-creation` | Issue filing workflow with bug report and feature request templates |
| Skill Registry | `skill-registry` | Build an index of installed skills with triggers, scopes, and exact `SKILL.md` paths |
| Chained PR | `chained-pr` | Plan and create reviewable stacked/chained pull requests |
| Cognitive Doc Design | `cognitive-doc-design` | Write docs that reduce review and onboarding cognitive load |
| Comment Writer | `comment-writer` | Draft warm, direct collaboration comments and review replies |
| Work Unit Commits | `work-unit-commits` | Split implementation into reviewable work units |
| RDD Defect Workflow | `rdd-defect-workflow` | Guide receipt-driven defect work with truthful evidence and authority boundaries |

Of these, `go-testing`, `skill-creator`, `skill-improver`, `skill-registry`, `chained-pr`, `cognitive-doc-design` and `work-unit-commits` are installed by default with both the `full-gentleman` (Dev Stack + Polish) and `ecosystem-only` (Dev Stack) presets. `branch-pr`, `issue-creation`, `comment-writer` and `rdd-defect-workflow` are repository-contributor workflow skills: they remain selectable with an explicit `--skills <id>`, but no default preset installs them.

### Coding Skills (separate repository)

For framework-specific skills (React 19, Angular, TypeScript, Tailwind 4, Zod 4, Playwright, etc.), see [Gentleman-Programming/Gentleman-Skills](https://github.com/Gentleman-Programming/Gentleman-Skills). These are maintained by the community and installed separately by cloning the repo and copying skills to your agent's skills directory.

---

## Presets

| Preset | ID | What's Included |
|--------|-----|-----------------|
| Dev Stack + Polish | `full-gentleman` | All components (Engram + SDD + Skills + Context7 + GGA + Permissions + Theme) + all skills |
| Dev Stack | `ecosystem-only` | Core components (Engram + SDD + Skills + Context7 + GGA) + all skills |
| Memory Only | `minimal` | Engram + SDD skills only |
| Custom | `custom` | You choose components and skills manually while keeping any existing persona/settings unmanaged |

Persona is selected separately on the Persona screen and applied independently of the preset.
