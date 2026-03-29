---
name: skill-registry
description: >
  Create or update the skill registry for the current project. Scans user skills and project conventions, writes .atl/skill-registry.md, and saves to engram if available.
  Trigger: When user says "update skills", "skill registry", "actualizar skills", "update registry", or after installing/removing skills.
license: MIT
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Purpose

You generate or update the **skill registry** — a catalog of all available skills with **compact rules** (pre-digested, 5-15 line summaries) that any delegator injects directly into sub-agent prompts. Sub-agents do NOT read the registry or individual SKILL.md files — they receive compact rules pre-resolved in their launch prompt.

This is the foundation of the **Skill Resolver Protocol** (see `_shared/skill-resolver.md`). The registry is built ONCE (expensive), then read cheaply at every delegation.

## When to Run

- After installing or removing skills
- After setting up a new project
- When the user explicitly asks to update the registry
- As part of `sdd-init` (it calls this same logic)

## What to Do

### Step 1: Scan User Skills

1. Glob for `*/SKILL.md` files across ALL known skill directories. Check every path below — scan ALL that exist, not just the first match:

   **User-level (global skills):**
   - `~/.claude/skills/` — Claude Code
   - `~/.config/opencode/skills/` — OpenCode
   - `~/.gemini/skills/` — Gemini CLI
   - `~/.cursor/skills/` — Cursor
   - `~/.copilot/skills/` — VS Code Copilot
   - The parent directory of this skill file (catch-all for any tool)

   **Project-level (workspace skills):**
   - `{project-root}/.claude/skills/` — Claude Code
   - `{project-root}/.gemini/skills/` — Gemini CLI
   - `{project-root}/.agent/skills/` — Antigravity (workspace)
   - `{project-root}/skills/` — Generic

2. **SKIP `sdd-*` and `_shared`** — those are SDD workflow skills, not coding/task skills
3. Also **SKIP `skill-registry`** — that's this skill
4. **Deduplicate** — if the same skill name appears in multiple locations, keep the project-level version (more specific). If both are user-level, keep the first found.
5. For each skill found, read the **full SKILL.md** (if a SKILL.md exceeds 200 lines, focus on the frontmatter and Critical Patterns / Rules sections only) to extract:
   - `name` field (from frontmatter)
   - `description` field → extract the trigger text (after "Trigger:" in the description)
   - **Compact rules** — the actionable patterns and constraints (see Step 1b)
6. Build a table of: Trigger | Skill Name | Full Path

### Step 1b: Generate Compact Rules

For each skill found in Step 1, generate a **compact rules block** (5-15 lines max) containing ONLY:
- Actionable rules and constraints ("do X", "never Y", "prefer Z over W")
- Key patterns with one-line examples where critical
- Breaking changes or gotchas that would cause bugs if missed

**DO NOT include**: purpose/motivation, when-to-use, full code examples, installation steps, or anything the sub-agent doesn't need to APPLY the skill.

Format per skill:
```markdown
### {skill-name}
- Rule 1
- Rule 2
- ...
```

**Example** — compact rules for a React 19 skill:
```markdown
### react-19
- No useMemo/useCallback — React Compiler handles memoization automatically
- use() hook for promises/context, replaces useEffect for data fetching
- Server Components by default, add 'use client' only for interactivity/hooks
- ref is a regular prop — no forwardRef needed
- Actions: use useActionState for form mutations, useOptimistic for optimistic UI
- Metadata: export metadata object from page/layout, no <Head> component
```

**The compact rules are the MOST IMPORTANT output of this skill.** They are what sub-agents actually receive. Invest time making them accurate and concise.

### Step 2: Scan Project Conventions

1. Check the project root for convention files. Look for:
   - `agents.md` or `AGENTS.md`
   - `CLAUDE.md` (only project-level, not `~/.claude/CLAUDE.md`)
   - `.cursorrules`
   - `GEMINI.md`
   - `copilot-instructions.md`
2. **If an index file is found** (e.g., `agents.md`, `AGENTS.md`): READ its contents and extract all referenced file paths. These index files typically list project conventions with paths — extract every referenced path and include it in the registry table alongside the index file itself.
3. For non-index files (`.cursorrules`, `CLAUDE.md`, etc.): record the file directly.
4. The final table should include the index file AND all paths it references — zero extra hops for sub-agents.

### Step 3: Write the Registry

Build the registry markdown:

```markdown
# Skill Registry

**Delegator use only.** Any agent that launches sub-agents reads this registry to resolve compact rules, then injects them directly into sub-agent prompts. Sub-agents do NOT read this registry or individual SKILL.md files.

See `_shared/skill-resolver.md` for the full resolution protocol.

## User Skills

| Trigger | Skill | Path |
|---------|-------|------|
| {trigger from frontmatter} | {skill name} | {full path to SKILL.md} |
| ... | ... | ... |

## Compact Rules

Pre-digested rules per skill. Delegators copy matching blocks into sub-agent prompts as `## Project Standards (auto-resolved)`.

### {skill-name-1}
- Rule 1
- Rule 2
- ...

### {skill-name-2}
- Rule 1
- Rule 2
- ...

{repeat for each skill}

## Project Conventions

| File | Path | Notes |
|------|------|-------|
| {index file} | {path} | Index — references files below |
| {referenced file} | {extracted path} | Referenced by {index file} |
| {standalone file} | {path} | |

Read the convention files listed above for project-specific patterns and rules. All referenced paths have been extracted — no need to read index files to discover more.
```

### Step 4: Persist the Registry

**This step is MANDATORY — do NOT skip it.**

#### A. Always write the file (guaranteed availability):

Create the `.atl/` directory in the project root if it doesn't exist, then write:

```
.atl/skill-registry.md
```

#### B. If engram is available, also save to engram (cross-session bonus):

```
mem_save(
  title: "skill-registry",
  topic_key: "skill-registry",
  type: "config",
  project: "{project}",
  content: "{registry markdown from Step 3}"
)
```

`topic_key` ensures upserts — running again updates the same observation.

### Step 5: Return Summary

```markdown
## Skill Registry Updated

**Project**: {project name}
**Location**: .atl/skill-registry.md
**Engram**: {saved / not available}

### User Skills Found
| Skill | Trigger |
|-------|---------|
| {name} | {trigger} |
| ... | ... |

### Project Conventions Found
| File | Path |
|------|------|
| {file} | {path} |

### Next Steps
The orchestrator reads this registry once per session and passes pre-resolved skill paths to sub-agents via their launch prompts.
To update after installing/removing skills, run this again.
```

## Rules

- ALWAYS write `.atl/skill-registry.md` regardless of any SDD persistence mode
- ALWAYS save to engram if the `mem_save` tool is available
- SKIP `sdd-*`, `_shared`, and `skill-registry` directories when scanning
- Read SKILL.md files (respecting the 200-line guard in Step 1) to generate accurate compact rules — this is a build-time cost, not a runtime cost
- Compact rules MUST be 5-15 lines per skill — concise, actionable, no fluff
- Include ALL convention index files found (not just the first)
- If no skills or conventions are found, write an empty registry (so sub-agents don't waste time searching)
- Add `.atl/` to the project's `.gitignore` if it exists and `.atl` is not already listed
