# Components, Skills & Presets

← [Back to README](../README.md)

---

## Components

| Component | ID | Description |
|-----------|-----|-------------|
| Engram | `engram` | Persistent cross-session memory — managed automatically by the agent, no manual interaction needed |
| SDD | `sdd` | Spec-Driven Development workflow (9 phases) — the agent handles SDD organically when the task warrants it, or when you ask; you don't need to learn the commands |
| Skills | `skills` | Curated coding skill library |
| Context7 | `context7` | MCP server for live framework/library documentation |
| Persona | `persona` | Gentleman, neutral, or custom behavior mode |
| Permissions | `permissions` | Security-first defaults and guardrails |
| GGA | `gga` | Gentleman Guardian Angel — AI provider switcher |
| Theme | `theme` | Gentleman Kanagawa theme overlay |

## GGA Behavior

`gentle-ai --component gga` installs/provisions the `gga` binary globally on your machine.

It does **not** run project-level hook setup automatically (`gga init` / `gga install`) because that should be an explicit decision per repository.

After global install, enable GGA per project with:

```bash
gga init
gga install
```

---

## Skills

### Included Skills (installed by gentle-ai)

14 skill files organized by category, embedded in the binary and injected into your agent's configuration:

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
| Judgment Day | `judgment-day` | Parallel adversarial review — two independent judges review the same target |

#### Foundation

| Skill | ID | Description |
|-------|-----|-------------|
| Go Testing | `go-testing` | Go testing patterns including Bubbletea TUI testing |
| Skill Creator | `skill-creator` | Create new AI agent skills following the Agent Skills spec |
| Branch & PR | `branch-pr` | PR creation workflow with conventional commits, branch naming, and issue-first enforcement |
| Issue Creation | `issue-creation` | Issue filing workflow with bug report and feature request templates |

These foundation skills are installed by default with both `full-gentleman` and `ecosystem-only` presets.

### Coding Skills (separate repository)

For framework-specific skills (React 19, Angular, TypeScript, Tailwind 4, Zod 4, Playwright, etc.), see [Gentleman-Programming/Gentleman-Skills](https://github.com/Gentleman-Programming/Gentleman-Skills). These are maintained by the community and installed separately by cloning the repo and copying skills to your agent's skills directory.

---

## Presets

| Preset | ID | What's Included |
|--------|-----|-----------------|
| Full Gentleman | `full-gentleman` | All components (Engram + SDD + Skills + Context7 + GGA + Persona + Permissions + Theme) + all skills + gentleman persona |
| Ecosystem Only | `ecosystem-only` | Core components (Engram + SDD + Skills + Context7 + GGA) + all skills + gentleman persona |
| Minimal | `minimal` | Engram + SDD skills only |
| Custom | `custom` | You pick components, skills, and persona individually |
