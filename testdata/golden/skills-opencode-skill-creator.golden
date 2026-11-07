---
name: skill-creator
description: "Trigger: new skills, agent instructions, documenting AI usage patterns. Create LLM-first skills with valid frontmatter."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Create a skill when:
- A pattern is used repeatedly and AI needs guidance
- Project-specific conventions differ from generic best practices
- Complex workflows need step-by-step instructions
- Decision trees help AI choose the right approach

Do not create a skill when the pattern is trivial, one-off, or better served by normal documentation.

## Hard Rules

- When working in this repo, first follow `docs/skill-style-guide.md` as the normative source before creating or updating skills.
- For installed global skills, use `references/skill-style-guide.md` as the bundled local copy of that guide when `docs/skill-style-guide.md` is unavailable.
- If neither guide is available, use the compact inline rules below.
- A skill is a runtime instruction contract for an LLM, not human documentation.
- Do not add a `Keywords` section; preserve essential trigger words in `description`.
- References must point to local files.
- Keep the skill body concise: target 180–450 tokens, recommended max 700, hard max 1000.
- Run the post-creation checklist for every created skill before concluding the session.

## Decision Gates

| Need | Action |
|------|--------|
| Code templates, schemas, fixtures, generated examples | Put them in `assets/` |
| Conceptual detail, edge cases, existing docs | Put local links in `references/` |
| Long explanation in `SKILL.md` | Move it to a supporting file |
| Multiple meaningful paths | Add a compact decision table |

## Execution Steps

1. Check whether `docs/skill-style-guide.md` exists; if it does, apply it before the bundled local copy or inline fallback rules.
2. If the repo guide is unavailable, read `references/skill-style-guide.md` and apply it before the inline fallback rules.
3. Confirm the skill does not already exist and the pattern is reusable.
4. Create or update `skills/{skill-name}/SKILL.md` using this required structure:

```
skills/{skill-name}/
├── SKILL.md              # Required - main skill file
├── assets/               # Optional - templates, schemas, examples
│   ├── template.py
│   └── schema.json
└── references/           # Optional - links to local docs
    └── docs.md           # Points to docs/developer-guide/*.mdx
```
5. Use this frontmatter shape:

```markdown
---
name: {skill-name}
description: "Trigger: {essential trigger words users or agents will say}. {What this skill does}."
license: Apache-2.0
metadata:
  author: "{your-github-username}"
  version: "1.0"
---
```
6. Write sections in this order: Activation Contract, Hard Rules, Decision Gates, Execution Steps, Output Contract, References.
7. Register the skill in `AGENTS.md` when it is a project skill.
8. Execute the mandatory post-creation checklist for all skills (project and global).

## Inline Fallback Rules

- `description` MUST be one physical line, quoted, YAML-safe, and include essential trigger words first.
- `description` SHOULD be <=160 chars and MUST be <=250 chars.
- Frontmatter MUST include `name`, `description`, `license`, `metadata.author`, and `metadata.version`.
- Use imperative instructions, not tutorials or background prose.
- Put supporting material in `assets/` or `references/`, not the main skill body.

Good:

```yaml
description: "Trigger: Jira task, ticket, issue, task creation. Create Jira tasks in the team format."
```

Bad:

```yaml
description: >
  Create Jira tasks in the team format.
  Trigger: Jira task, ticket, issue, or task creation.
Keywords: jira, task
```

## Output Contract

Return:
- Files created or modified.
- Whether the repo style guide or inline fallback rules were used.
- Any AGENTS.md registration change.
- Any supporting files added under `assets/` or `references/`.
- Post-creation announcement containing skill name, trigger sentence, and registry row.

## References

- `docs/skill-style-guide.md` — normative LLM-first skill style guide for this repo.
- `references/skill-style-guide.md` — bundled local copy for installed global skills when the repo doc is unavailable.

## Post-creation checklist (mandatory)

1. Validate frontmatter (`name` matches the folder, lowercase-hyphenated, <=64 chars; single-line quoted `description` front-loaded with trigger keywords).
2. Run `gentle-ai skill-registry refresh --cwd <project>` and confirm the new row exists in `.atl/skill-registry.md`.
3. Persist a discovery pointer: `mem_save` with `topic_key: skills/<name>` (name, path, trigger, one-line purpose).
4. Announce to the user: skill name, trigger sentence, registry row.
