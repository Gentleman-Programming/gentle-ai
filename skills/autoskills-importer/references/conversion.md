# Conversion Reference — Compact Rules Extraction

## Goal

The critical output of conversion is a `trigger` line and 3–5 compact rules
that tell the registry *when* to load this skill — not what it does. A compact
rule that describes content is useless. A compact rule that changes agent
behavior is the target.

---

## Stage 1 — Decompose

Before writing any output, extract these fields from the source skill:

```
problem:          [one sentence — what specific problem does this skill solve?]
references:       [file extensions, frameworks, tools, config files it mentions]
rules:            [core behavioral instructions — what should an agent DO?]
negative:         [when should this skill NOT load? be specific]
example_prompts:
  - [user prompt that should trigger this skill]
  - [user prompt that should trigger this skill]
  - [user prompt that should trigger this skill]
```

Do not skip this stage. Stage 2 quality depends entirely on it.

---

## Stage 2 — Synthesize

Using Stage 1 output, produce:

**`trigger:`**
Derive from `references` + `example_prompts`. Capture *when* to load, not
what the skill is about. Narrow enough to avoid false positives.

**Compact rules (3–5 lines)**
Each rule must be:
- Actionable: "Do X when Y" — not "This skill covers X"
- Scoped: tied to a file type, intent, or condition from `references`
- Behavioral: what changes about the agent's output when this skill is active?

**`description:`**
One sentence from `problem`, trigger words first.

---

## Good vs Bad — React 19 example

Source skill: a React 19 skill covering hooks, concurrent features, and RSC.

❌ Bad:
```
- This skill handles React 19 components and hooks
- Use for frontend React development
- Covers concurrent features and Server Components
```
Why bad: describes content. "Use for frontend React development" fires on every
frontend question — meaningless as a registry trigger.

✅ Good:
```
- Prefer useTransition over useState for non-urgent state updates in React 19
- Server Components cannot use hooks or browser APIs — move those to Client Components
- use() replaces useContext and async data fetching patterns
- Use `hydrateRoot` for SSR hydration; use `createRoot` for client-only rendering
```
Why good: each rule changes agent behavior. Scoped, non-overlapping, actionable.

---

## Validation Self-Check

Before finalizing, answer:

1. Does the trigger fire for all 3 example prompts from Stage 1? If not, broaden it.
2. Does the trigger fire for prompts clearly outside this skill's scope? If yes, narrow it.
3. Do any compact rules duplicate content that belongs in `references/`? Move them.
4. Could any rule be read as always-applicable? Add a condition ("when X", "for Y files").

---

## Output Schema

```yaml
name: [kebab-case]
description: "Trigger: [condition]. [one sentence from problem]"
trigger: [condition]
compact_rules:
  - rule 1
  - rule 2
  - rule 3
```

> `problem`, `references`, `rules`, `negative`, and `example_prompts` are Stage 1 intermediate working fields. They are used to drive Stage 2 synthesis and the self-check, but they are NOT part of this output schema and are not written to the generated skill files.

Full source content goes in `references/[skill-name].md`.
Add a pointer in compact rules only if the agent needs to know where to find
extended detail.
