---
name: judgment-day
description: "Trigger: judgment day, dual review, adversarial review, juzgar. Run explicit blind dual review with at most two scoped fix/re-judgment rounds."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.7"
---

## Activation Contract

Load only when the user explicitly requests Judgment Day or equivalent dual/adversarial review for a concrete target. Judgment Day is a standalone developer tool: judges run whenever asked, on any runtime, and need no review transaction, runtime identity, or delivery-receipt machinery to start. It replaces ordinary 4R as the adversarial method for that target; never run both.

## Hard Rules

- Resolve matching project skills before starting and pass the same paths to both judges (`jd-judge-a`, `jd-judge-b`) and the fix actor (`jd-fix-agent`).
- Build one complete immutable target, then launch two blind read-only judges in parallel with identical scope and criteria.
- Each judge returns one neutral findings result and terminates. Wait for both; never accept a partial judgment.
- Never launch `review-refuter`; two-judge agreement is the corroboration mechanism.
- Only the parent orchestrator merges/persists findings, launches the fix actor (`jd-fix-agent`), and launches scoped re-judgment.
- Fix only severe findings confirmed by both judges. WARNING/SUGGESTION rows remain `info`.
- Permit at most two fix rounds and two scoped re-judgments. Re-judgment sees only the frozen ledger plus fix delta and may record fix-caused defects.
- The only terminal verdicts are `APPROVED | ESCALATED`; never reset or extend an exhausted round budget.
- A judgment issues no receipt and carries no delivery authority: it satisfies no commit, push, PR, or release gate. When the caller explicitly wants ordinary negotiated review for the same target, run it as its own step; neither outcome authorizes delivery, which remains under ordinary repository policy.

## Decision Gates

| Condition | Action |
|---|---|
| Target unclear | Ask one scope question and stop. |
| Both judges confirm severe finding | Ask before round-one correction; then use the bounded fix actor. |
| One judge reports it | Record suspect; do not auto-fix. |
| Judges contradict | Escalate for explicit human decision. |
| Scoped re-judgment fails before round two | Parent may launch the final bounded fix round. |
| Any issue remains after round two | Escalate and stop. |

## Execution Steps

1. Build the complete immutable target and freeze the scope both judges will inspect.
2. Launch both read-only judges in parallel (`jd-judge-a`, `jd-judge-b`) against the same immutable target.
3. Merge findings into the frozen ledger and persist it through the selected artifact store.
4. Ask before round-one correction; run the fix actor (`jd-fix-agent`) only for confirmed severe IDs.
5. Run both judges again (`jd-judge-a`, `jd-judge-b`) only over the frozen ledger plus immutable fix delta.
6. Repeat once at most, then run independent final verification and return the terminal verdict.

## Output Contract

Return target identity, round, confirmed/suspect/contradiction/INFO counts, correction work units, scoped re-judgment result, artifact references, skill resolution, and exactly one final `JUDGMENT: APPROVED ✅` or `JUDGMENT: ESCALATED ⚠️`.

## References

- [references/prompts-and-formats.md](references/prompts-and-formats.md) — compact judge/fix prompts and verdict shape.
- [../_shared/review-ledger-contract.md](../_shared/review-ledger-contract.md) — optional ordinary negotiated-review context: consult it only when the caller explicitly requests that lifecycle; never required to run judges and never delivery authorization.
