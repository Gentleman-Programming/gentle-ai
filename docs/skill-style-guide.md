# LLM-first Skill Style Guide

Use this guide when creating or refactoring skills in this repo. A skill is a **runtime instruction contract for an LLM**, not human-facing documentation: it tells the model when to activate, what rules are non-negotiable, how to decide, what to do, and what to return.

## Required Structure

Every `SKILL.md` MUST use this order unless a section is truly irrelevant:

1. **Frontmatter** — complete metadata for skill discovery.
2. **Activation Contract** — exact situations that load the skill.
3. **Hard Rules** — constraints the LLM MUST NOT violate.
4. **Decision Gates** — short tables or bullets for branching choices.
5. **Execution Steps** — ordered operational workflow.
6. **Output Contract** — required final format or artifacts.
7. **References** — local files only; supporting detail lives outside the skill.

## Frontmatter Rules

- `description` MUST be one physical line, YAML-safe, and quoted.
- Put trigger words first: `"Trigger: ... . {What the skill does}."`
- `description` SHOULD be <=160 chars and MUST be <=250 chars.
- Include complete `name`, `description`, `license`, `metadata.author`, and `metadata.version`.
- Do NOT add a `Keywords` section; discovery uses frontmatter.

## Body Budget

- Target **180–450 tokens** for the skill body.
- Recommended maximum: **700 tokens**.
- Hard maximum: **1000 tokens**. Move examples, schemas, and background into `assets/` or `references/`.

## Writing Rules

### DO

- Write imperative runtime instructions: “Load X”, “Check Y”, “Return Z”.
- Lead with the activation trigger and hard constraints.
- Use compact tables for decision gates.
- Keep examples minimal and executable.
- Link to local supporting files for details.

### DON'T

- Explain history, motivation, or tutorial background.
- Duplicate long docs inside the skill.
- Add generic advice the LLM cannot execute.
- Use external URLs as primary references.
- Hide critical rules below examples.

## Supporting Files

- Use `assets/` for templates, schemas, fixtures, or generated examples.
- Use `references/` for local docs that explain concepts or edge cases.
- Keep references stable and relative to the skill directory when possible.

## Quality Gates

- Frontmatter is complete, quoted, single-line, and trigger-preserving.
- Required sections exist in the expected order.
- Hard rules are testable or observable.
- Decision gates cover meaningful forks only.
- Output contract tells the LLM exactly what to return.
- References point to local files.

## Refactor Checklist

- [ ] Move explanatory prose to local references.
- [ ] Collapse repeated rules into one hard rule.
- [ ] Replace prose branches with a decision table.
- [ ] Trim examples to the smallest useful case.
- [ ] Recheck description length and trigger words.
