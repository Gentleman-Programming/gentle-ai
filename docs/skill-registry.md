# Skill Registry

← [Back to README](../README.md)

The skill registry is a project-local index that lets every supported agent find the same skills without rewriting them. It stores skill names, full descriptions, scopes, and exact `SKILL.md` paths.

## When To Use It

Use `gentle-ai skill-registry refresh` after you add, remove, rename, or move skills. Normal installs wire this refresh into startup hooks where the agent supports them, including Codex, Claude Code, OpenCode, and Pi through `gentle-pi`.

## Runtime Flow

```text
User task
   │
   ▼
Orchestrator reads .atl/skill-registry.md
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
   ├─ Deduplicate by skill name
   │     project skill wins over global skill
   │
   ├─ Parse frontmatter
   │     name + full description + path + scope
   │
   └─ Write .atl/skill-registry.md + cache
```

## Registry Contract

The registry is an **index**, not a generated summary.

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

## Quick Check

```bash
gentle-ai skill-registry refresh --force
```

Open `.atl/skill-registry.md` and verify each row has a useful description and a real `SKILL.md` path.

← [Back to README](../README.md)
