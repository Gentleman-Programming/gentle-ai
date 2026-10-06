# Components, Skills & Presets

> [!NOTE]
> These docs track `main`, which may include unreleased changes. For the latest release, see the [v4.0.0 docs](https://github.com/Gentleman-Programming/gentle-ai/tree/v4.0.0/docs).

← [Back to README](../README.md)

---

## Components

| Component | ID | Description |
|-----------|-----|-------------|
| Engram™ | `engram` | Persistent cross-session memory via MCP — auto-detection of project name, full-text search, git sync, project consolidation. See [engram repo](https://github.com/Gentleman-Programming/engram) |
| Skills | `skills` | Curated coding skill library |
| Context7 | `context7` | MCP server for live framework/library documentation |
| Persona | `persona` | Managed Gentleman/neutral persona injection, or unmanaged custom persona mode |
| Permissions | `permissions` | Security-first defaults and guardrails. Applied to Claude Code and OpenCode/Kilocode (the adapters with permissions overlay support). Default sensitive-paths deny list: `~/.ssh/*`, `~/.ssh/**/*`, `**/*.pem`, `**/*.key`, `**/.env*`, `~/.credentials/*`, `~/.aws/credentials`, `~/.config/gh/hosts.yml`, `~/Library/Keychains/*`, `**/secrets/*`, `**/*.p12`, `**/*.pfx`. OpenCode/Kilocode follow the Gentle Pi safety model: everything is allowed by design; recursive `rm` on `/`, `~`, `$HOME`, `.` or `..` (also behind `sudo`), hard reset, forced `git clean`, force push (also with `git -C <dir>`), `chmod -R 777` and recursive `chown` are denied; other recursive `rm`, `find -delete`, destructive SQL passed to `psql`/`mysql`/`mariadb`/`sqlite3`, `git push`, `git rebase`, `git branch -D` and `npm publish` ask; remote commands ask (#4324); and the sensitive paths are denied to `read` and `edit` (which covers write and patch). Like Pi, reading a secret through `bash` or the `grep` tool is not blocked. Wildcard rules cannot parse shell syntax the way Pi does, so other wrappers (`env`, `xargs`, `VAR=value`), nested shells, absolute executable paths, unusual flag permutations, and `rm -rf /*` are not guarded. |
| GGA | `gga` | Gentleman Guardian Angel — AI provider switcher |
| Theme | `theme` | Gentleman Kanagawa theme overlay |

ODD (Organic Driven Development) is shared routing guidance, not a separate component to install. It is the only implementation workflow for direct and delegated work. See [ODD and recovery](usage.md#organic-driven-development-odd).

> **Since v4.0.0:** the SDD (Spec-Driven Development) component and its `sdd-*` skills are retired in favor of ODD. A legacy `sdd` selection persisted in state is still read, but install and sync no longer write its assets. A global install or sync also removes the native `sdd-*` sub-agents earlier releases rendered for Claude Code, Kiro, Cursor, and Kimi, proven by comparing each file with every released template; edited files are preserved and reported as manual actions. A Kimi `gentleman.yaml` inherited from v3.x declares every SDD subagent by path: when its bytes match a release, sync rewrites it first so the cleanup completes; when it was edited, sync keeps every SDD agent it references and reports one manual action naming it. For OpenCode and Kilocode, install and sync remove the `sdd-*` agent entries (including profile copies and the 3.7.0 plural `agents` map) and the `prompts/sdd/` files releases wrote; see [OpenCode compatibility](opencode-compatibility.md). For every runtime except Pi, install and sync also remove the retired `sdd-*` skills, the shared `_shared/sdd-*.md` and `_shared/openspec-convention.md` references (including the shared `~/.agents/skills` root outside Windows), the `/sdd-*` and `/gentle-sdd-*` slash commands, and the Claude Code `PreToolUse` hook that runs the removed `gentle-ai sdd-preflight-hook` command, each only when it matches what a release wrote. A workspace-scoped sync handles the workspace copies, including the Windsurf `.windsurf/workflows/sdd-new.md` workflow. An edited SDD skill keeps its whole directory and the shared SDD references it reads. A skills, skill, or `_shared` directory that is a symlink is never entered, and Claude Code settings that are a symlink or not strict JSON are never rewritten; both are reported with the exact file or hook to remove. Claude Code's `_shared/sdd-orchestrator-workflow.md` was rendered per installation, so it is always kept and reported. Removed files are captured in the sync backup.

## Primary remote-authorization guidance

Always-installed agent guidance includes a canonical remote-operation boundary, independent of the optional persona and permissions components. Local-development access does not authorize remote execution, transfer, or discovery/reuse of ambient SSH agents, ControlMaster sockets, credentials, or sessions. Ask for explicit destination, operation, and credential/session authorization; authorized work remains allowed within stricter user/runtime restrictions.

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

---

## Skills

### Included Skills (installed by gentle-ai)

Skill files organized by category, embedded in the binary and injected into your agent's configuration:

#### Review

| Skill | ID | Description |
|-------|-----|-------------|
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
| Dev Stack + Polish | `full-gentleman` | All components (Engram + Skills + Context7 + GGA + Permissions + visual polish) + all skills |
| Dev Stack | `ecosystem-only` | Core components (Engram + Skills + Context7 + GGA) + all skills |
| Memory Only | `minimal` | Engram only |
| Custom | `custom` | You choose components and skills manually while keeping any existing persona/settings unmanaged |

Persona is selected separately on the Persona screen and applied independently of the preset.
