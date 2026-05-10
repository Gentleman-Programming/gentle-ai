---
name: judgment-day
description: "Trigger: judgment day, dual review, adversarial review, juzgar. Run blind dual review, fix confirmed issues, then re-judge."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.4"
---

## Activation Contract

Load this skill only when the user explicitly asks for Judgment Day, dual/adversarial review, or equivalent Spanish trigger (`juzgar`, `que lo juzguen`). Review a specific target: files, feature, PR, or architecture slice.

## Hard Rules

- Resolve project skills before launching agents: read skill registry, match compact rules by target files/task, and inject the same `Project Standards` block into both judge prompts and fix prompts.
- Launch **two blind judges in parallel** with identical target and criteria; never review the code yourself.
- Wait for both judges before synthesis; never accept a partial verdict.
- Classify warnings as `WARNING (real)` only if normal intended use can trigger them; otherwise downgrade to INFO as `WARNING (theoretical)`.
- Ask before fixing Round 1 confirmed issues.
- After any fix agent runs, immediately re-launch both judges in parallel before commit/push/done/session summary.
- Terminal states are only `JUDGMENT: APPROVED` or `JUDGMENT: ESCALATED`.
- After 2 fix iterations with remaining issues, ask the user whether to continue.

## Decision Gates

| Condition | Action |
|---|---|
| Target unclear | Ask for scope; do not launch judges. |
| No skill registry | Warn, proceed with generic criteria, and record `Skill Resolution: none`. |
| Both judges find same CRITICAL/real WARNING | Confirmed; ask/fix according to round rules. |
| One judge finds issue | Suspect; report and triage, do not auto-fix. |
| Judges contradict | Escalate for manual decision. |
| Round 2+ has only theoretical warnings/suggestions | Report as INFO; do not re-judge. |

## Execution Steps

1. Confirm target and optional custom criteria.
2. Resolve compact project standards from registry or warn if missing.
3. Start Judge A and Judge B concurrently via delegation.
4. Synthesize findings into confirmed, suspect, contradiction, and INFO buckets.
5. Ask before Round 1 fixes; delegate a separate fix agent for confirmed approved fixes only.
6. Re-judge in parallel after fixes; repeat until approved, escalated, or user asks to stop.
7. Before any terminal action, verify every active Judgment Day has a terminal state.

## Output Contract

Return `## Judgment Day — {target}` with round number, verdict table, confirmed/suspect/contradiction counts, fixes applied, re-judgment result, `Skill Resolution`, and final `JUDGMENT: APPROVED ✅` or `JUDGMENT: ESCALATED ⚠️`.

## References

- [references/prompts-and-formats.md](references/prompts-and-formats.md) — judge/fix prompts, warning rubric, verdict tables, and language snippets.
