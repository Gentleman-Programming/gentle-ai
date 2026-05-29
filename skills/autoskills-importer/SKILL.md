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
- Never auto-update a previously imported skill unless `--refresh` is passed. Skills are pinned once written.
- Check `autoskills/ignore/{project}` in Engram before presenting the detection list — excluded technologies must not appear.

## Decision Gates

| Condition | Action |
|---|---|
| `pending_user_confirmation: true` in detection output | Ask user before including low-confidence techs |
| skills.sh key missing for a detected tech | Log as gap, skip fetch, report at end |
| Skill already registered, hash unchanged | Skip — already up to date; report as `✓ already installed` |
| Skill already registered, hash changed | Report `[skill] has upstream changes — run with --refresh to update`; do NOT overwrite |
| Skill already registered + `--refresh` passed | Re-run full conversion and overwrite |

> **`content_hash`** is computed over the raw fetched skills.sh source content, before any conversion. Hash the exact bytes returned by the HTTP response (UTF-8). This ensures two independent runs over the same upstream version always produce the same hash.
| Validation self-check fails after Stage 2 | Revise compact rules before writing files |

## Flags

| Flag | Behavior |
|---|---|
| `--only react,typescript` | Import only the specified technologies; ignore the rest of the detected list. Values are **detected technology names** (e.g., `react`, `nextjs`), not skills.sh keys |
| `--skip jest` | Exclude a technology from this run. Same namespace: detected technology names |
| `--refresh` | Re-import already installed skills if upstream changes are detected |

## Execution Steps

1. Load `references/detection.md`. Check `autoskills/ignore/{project}` in Engram and remove any excluded technologies before presenting results. Read project files, produce detection output schema. If `pending_user_confirmation: true`, wait for user. Apply `--only` / `--skip` flags if present.
2. Fetch the live skills.sh index. For each detected technology, validate its mapped key exists. Keys not in the index go to gaps.
3. For each validated key: check Engram for an existing import record (`source_url`, `fetched_at`, `content_hash`). Compare hash against fresh fetch. If unchanged and `--refresh` not passed, skip. Otherwise: fetch raw content → Stage 1 decompose → Stage 2 synthesize → self-check → write `skills/[name]/SKILL.md` + `skills/[name]/references/[name].md`. Use `assets/skill-template.md` as base.
4. Update `.atl/skill-registry.md` per `references/registry.md`.
5. Save to Engram per skill: `source_url`, `fetched_at`, `content_hash`, imported version, gaps, replacements.
6. Report imported, already-up-to-date, upstream-changed, gaps, and skipped items.

## Output Contract

```
✓ Imported:        [skill@version, ...]
✓ Already installed: [skill@version, ...]
⚠ Upstream changed: [skill — run with --refresh to update]
⚠ Gaps:            [techs with no skills.sh key]
⚠ Skipped:         [low-confidence techs the user declined]
```

## References

- [references/detection.md](references/detection.md) — stack detection logic, file signals, confidence rules, output schema.
- [references/conversion.md](references/conversion.md) — two-stage compact rules extraction and validation self-check.
- [references/registry.md](references/registry.md) — how to read, update, and validate `.atl/skill-registry.md`.
- [assets/skill-template.md](assets/skill-template.md) — base template for generated skills.

> **Engram** is the persistent memory layer used by Gentleman AI agents. Write to it via `mem_save` (MCP tool). Keys used by this skill: `autoskills/ignore/{project}` (exclusion list) and one record per imported skill with `source_url`, `fetched_at`, `content_hash`.
