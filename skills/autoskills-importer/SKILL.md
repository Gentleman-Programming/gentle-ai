---
name: autoskills-importer
description: "Trigger: /autoskills-import, import skills, skills.sh. Bridge skills.sh skills into Gentleman format with lazy-loading, compact rules, and registry integration."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Load when the user runs `/autoskills-import` or asks to import, fetch, or install skills from skills.sh into the current project.

## Hard Rules

- Never fetch a skill whose key was not confirmed in the live skills.sh index.
- Never overwrite a manually authored skill already in `.atl/skill-registry.md` — replace only entries previously imported by this skill.
- Always run the two-stage conversion from `references/conversion.md`; never one-shot summarize.
- Never silently drop an unresolvable technology — collect gaps and report them.
- Save the import log to Engram before reporting to the user.

## Decision Gates

| Condition | Action |
|---|---|
| `pending_user_confirmation: true` in detection output | Ask user before including low-confidence techs |
| skills.sh key missing for a detected tech | Log as gap, skip fetch, report at end |
| Skill already registered (imported previously) | Replace entry; note replacement in Engram save |
| Validation self-check fails after Stage 2 | Revise compact rules before writing files |

## Execution Steps

1. Load `references/detection.md`. Read project files, produce detection output schema. If `pending_user_confirmation: true`, wait for user.
2. Fetch the live skills.sh index. For each detected technology, validate its mapped key exists. Keys not in the index go to gaps.
3. For each validated key: fetch raw content → Stage 1 decompose → Stage 2 synthesize → self-check → write `skills/[name]/SKILL.md` + `skills/[name]/references/[name].md`. Use `assets/skill-template.md` as base.
4. Update `.atl/skill-registry.md` per `references/registry.md`.
5. Save to Engram: imported skills, versions, gaps, replacements.
6. Report imported, gaps, and skipped items.

## Output Contract

```
✓ Imported:  [skill@version, ...]
⚠ Gaps:      [techs with no skills.sh key]
⚠ Skipped:   [low-confidence techs the user declined]
```

## References

- [references/detection.md](references/detection.md) — stack detection logic, file signals, confidence rules, output schema.
- [references/conversion.md](references/conversion.md) — two-stage compact rules extraction and validation self-check.
- [references/registry.md](references/registry.md) — how to read, update, and validate `.atl/skill-registry.md`.
- [assets/skill-template.md](assets/skill-template.md) — base template for generated skills.
