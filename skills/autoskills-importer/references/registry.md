# Registry Reference — Updating skill-registry.md

## Goal

After each skill is converted, register it in `.atl/skill-registry.md` so
the orchestrator can discover and lazy-load it. The registry is an index —
one line per skill, under ~150 characters.

---

## Entry Format

```
- [skill-name](skills/[skill-name]/SKILL.md) — trigger: [trigger line] | [one-line description]
```

Example:
```
- [react-19](skills/react-19/SKILL.md) — trigger: React 19, hooks, RSC, concurrent | React 19 patterns and Server Components
```

The trigger field comes directly from Stage 2 of `conversion.md`.
The description comes from the `description:` field generated there.

---

## Adding an Entry

1. Read `.atl/skill-registry.md`
2. Check for an existing entry with the same skill name
3. **Not found** → append under `## Imported (skills.sh)`
4. **Found and previously imported** → replace the existing line in place
5. **Found and manually authored** → do NOT replace; register the import under `[skill-name]-imported` instead, and report the conflict (see Conflict Handling below)

Create the `## Imported (skills.sh)` section if it doesn't exist. This
separates imported skills from manually authored ones.

---

## Conflict Handling

When a manually authored skill exists with the same name as an import:
- Do not overwrite it
- Add the import under a different name: `[skill-name]-imported`
- Report the conflict in the final output so the user can decide

---

## Validation

After writing, verify:
1. The entry path resolves to a real `SKILL.md` file
2. The trigger field is non-empty
3. No two entries share the same skill name
4. Line length stays under 150 characters. Truncation priority (never cut these): path, skill name. Truncation order: description first (append `…`), then trigger keywords from the end (append `…`). Example: if trigger is `React 19, hooks, RSC, concurrent, streaming` and total line exceeds 150, shorten to `React 19, hooks, RSC…`
