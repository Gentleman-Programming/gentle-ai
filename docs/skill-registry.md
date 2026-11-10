# Skill Registry

> [!NOTE]
> These docs track `main`, which may include unreleased changes. For the latest release, see the [v4.0.0 docs](https://github.com/Gentleman-Programming/gentle-ai/tree/v4.0.0/docs).

← [Back to README](../README.md)

The shared skill registry is a project-local **disk** index of skill names, descriptions, scopes, and exact `SKILL.md` paths. Pi also offers its current session's runtime-resolved skills in an effective view; session-only entries are intentionally absent from the shared file.

## When To Use It

Use `gentle-ai skill-registry refresh` after you add, remove, rename, or move skills. Normal installs wire this refresh into startup hooks where the agent supports them, including Codex, Claude Code, OpenCode, and Pi through `gentle-pi`.

## Runtime Flow

```text
User task
   │
   ▼
Orchestrator uses current effective view
(Pi: session view; others: disk index)
   │
   ▼
Matches task + file context against full skill descriptions
   │
   ▼
Passes exact SKILL.md paths to subagent
   │
   ▼
Subagent reads full skills before work
   │
   ▼
Subagent executes with original skill intent preserved
```

## Refresh Flow

```text
gentle-ai skill-registry refresh
   │
   ├─ Scan project skill roots first
   │     skills/, .opencode/skills/, .claude/skills/, ...
   │
   ├─ Scan global agent skill roots second
   │     ~/.config/opencode/skills/, ~/.claude/skills/, ...
   │
   ├─ Parse direct skills, then one category level per root
   │     project wins; within scope the first candidate wins
   │
   └─ Compare rendered index bytes, then repair the cache hint
```

## Registry Contract

The registry is an **index**, not a generated summary. Direct readers and `list` see disk skills only; Pi delegators select from the current effective session view instead. A selected subagent still reads the exact skill file.

| Field | Meaning |
| --- | --- |
| `Skill` | Skill `name` from frontmatter, or directory name fallback |
| `Trigger / description` | Full `description`, including YAML folded multiline descriptions |
| `Scope` | `project` or `user` |
| `Path` | Exact `SKILL.md` file to load |

## Skill Loading Contract

Delegators pass paths, not digested rules:

```markdown
## Skills to load before work

Read these exact files before reading, writing, reviewing, testing, or creating artifacts:

- /path/to/skills/go-testing/SKILL.md
- /path/to/skills/docs-writer/SKILL.md
```

The subagent then reads those files. This keeps the original `SKILL.md` as the source of truth and avoids breaking author intent through automatic summarization.

OpenCode delegators list installed skills by the identifier the native `skill` tool takes instead of a path. Installed skills live in the OpenCode config directory, outside the workspace, where a delegated agent may be denied reads; the `skill` tool loads them by `<name>` on OpenCode 1.x and by the skill's `<id>` as listed (its directory name for `<dir>/SKILL.md`) on OpenCode 2.x, and `<available_skills>` lists the identifier each major expects. Skill files inside the workspace are still listed by their workspace-relative `SKILL.md` path.

## Skill Authoring Flow

```text
New reusable pattern
   │
   ▼
skill-creator creates SKILL.md
   │
   ▼
skill-registry indexes SKILL.md path and full description
   │
   ▼
orchestrator passes matching paths to agents
```

## Skill Improvement Flow

```text
Existing skills
   │
   ▼
skill-improver reads .atl/skill-registry.md
   │
   ▼
Audits each indexed SKILL.md against docs/skill-style-guide.md
   │
   ├─ Audit mode: report issues only
   │
   └─ Apply mode: safely refactor skills and preserve intent
   │
   ▼
Run gentle-ai skill-registry refresh again
```

## Why Not Compact Rules?

Compact rules were cheaper per delegation but could distort skills. The index-first design spends tokens only when a subagent actually needs a skill, and it preserves the complete runtime contract.

| Design | Benefit | Tradeoff |
| --- | --- | --- |
| Compact summaries | Small prompt injection | Can lose nuance and break custom skills |
| Index + paths | Preserves full skill intent | Subagents read selected full skills |

## Excluded Skills

The registry never indexes `_shared`, `skill-registry`, or any `sdd-*` skill or category.
The first two are internal plumbing; the `sdd-*` prefix stays reserved
because it belonged to the retired SDD workflow. This exclusion is intentional and
silent, so a user skill whose name collides with these prefixes is dropped
without a warning.

## Inspecting Without Writing

`skill-registry list` resolves the same deduplicated skill set as `refresh`, but
prints it instead of writing `.atl/skill-registry.md`, the cache, or
`.gitignore`. It does not include Pi session-only skills.

```bash
gentle-ai skill-registry list          # name<TAB>scope<TAB>path
gentle-ai skill-registry list --json   # machine-readable, includes descriptions
```

## Refresh and compatibility

Every refresh rescans disk and compares the exact rendered bytes, including with `--force`. An unchanged index is not rewritten, including after edits to unindexed skill body text. The adjacent schema-1 SHA-256 cache is a non-authoritative hint: missing, stale, or legacy cache bytes are repaired independently, and a matching cache never validates a stale registry. Registry and cache are individually atomic, not an atomic pair; a later refresh repairs an interrupted or interleaved publication.

Upgraded Pi and Gentle AI converge for the same normalized project, home, `CODEX_HOME`, filesystem snapshot, and OS. Different homes/overrides and mixed old/new binaries are outside that guarantee. Upgrade both writers before relying on convergence. The CLI keeps its project-root guard and its `--quiet`, `--no-gitignore`, `--force`, and `--cwd` options.

## Quick Check

```bash
gentle-ai skill-registry refresh --force
```

Open `.atl/skill-registry.md` and verify each row has a useful description and a real `SKILL.md` path.

← [Back to README](../README.md)
