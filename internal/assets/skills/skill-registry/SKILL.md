---
name: skill-registry
description: "Trigger: update skills, skill registry, actualizar skills, after skill changes. Index available skills by trigger and path."
license: MIT
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Use this skill after installing, removing, creating, moving, or renaming skills, or when a delegator needs a fresh skill index.

## Hard Rules

- The registry is an index, not a compiler or summary. `SKILL.md` remains the source of truth.
- Do not generate or inject compact rules by default; preserve author intent by passing exact skill paths to subagents.
- Refresh the shared disk index with `gentle-ai skill-registry refresh --cwd <project>`; it owns discovery, ordering, deduplication, and publication. Scan by hand only when that command is unavailable; use results for inspection, never publish a hand-rendered index.
- The shared file excludes Pi session-only runtime skills. Pi delegators select from their current effective session view; other delegators read the current disk index.
- Never substitute a stored Engram or session copy for the current disk index, or persist Pi's session-local effective view.
- Add `.atl/` to `.gitignore` when possible unless explicitly disabled.

## Decision Gates

| Situation | Action |
| --- | --- |
| Same skill exists globally and in project | Keep the project-level skill |
| Same skill exists in multiple global locations | Keep the first source in scan order |
| No skills found | Let refresh produce an empty disk index |
| Agent will delegate work | Select matching rows from the current effective view and pass exact `SKILL.md` paths |

## Execution Steps

1. Run `gentle-ai skill-registry refresh --cwd <project>`; every refresh rescans, and `--force` also leaves identical bytes untouched.
2. If skipped (not a project), unavailable, or failed, report that outcome. A read-only hand scan is optional only if the command is unavailable; do not write its results as the shared index.
3. For delegation, use the current disk index or Pi's effective session view and pass matching exact paths.
4. Return the registry path, skill count when reported, and whether the index changed.

## Output Contract

Return:
- Project name and `.atl/skill-registry.md` path.
- Number of indexed skills.
- Whether the index changed or was already current.
- Any skipped or duplicate skills when relevant.

## References

- `docs/skill-style-guide.md` — how skills should be authored before indexing.
- `skills/_shared/skill-resolver.md` — how delegators use the index.
