# Work continuity

An authorized outcome keeps moving. A pending child, a missing check, or a
reported blocker never converts authorized work into a question for the human:
the agent continues the runnable in-scope work and reports what is actually
outstanding.

This page describes the rule the ODD close of every runtime states, and the
Native Checking Contract each installed orchestrator carries. It is shipped
policy text, not a claim about model behavior; the rule is not enforced by a
runtime watchdog.

## The rule

> Authorized work continues: runnable in-scope steps run, a pending child
> is a wait event whose completion resumes the turn without asking the
> human, and only a real human decision, the user's explicit stop, a
> denial, missing consent, or an exhausted limit stops the work it applies
> to, never independent authorized work. A tracked task ends the turn
> pending only while it waits on already-launched work, for the affected
> work a denial, missing consent, the user's explicit stop, or an
> exhausted limit applies to, or for a genuine human decision still
> outstanding; independent authorized work continues, and it never
> abandons runnable work or presents an unverified outcome as final.
> Missing, partial, unavailable, or declined proof forbids claiming
> completion of the affected work, never an independently authorized next
> step, while a required authorized check the existing budget still allows
> keeps running; stop checking only inside an exhausted or denied
> boundary, report that exact blocker and the decision that is genuinely
> the human's, and never invent a pass. Never turn a report into a blind
> self-reprompt loop.

## Quick path

1. The request already authorizes the work, so run the next in-scope step
   instead of asking to continue.
2. A launched background or delegated unit is a wait event: end the turn while
   it runs, then resume it on its completion event without asking the human.
3. The user's explicit stop stops the work it applies to even when the task is
   small or untracked. A denial, missing consent, or an exhausted limit stops
   only its own work, and none of them ever resumes a prohibited path.
4. Missing proof blocks the completion claim for the affected work; it never
   blocks an independently authorized next step, and it never cancels a required
   authorized check the existing budget still allows.
5. Stop checking only inside an exhausted or denied boundary. Report the exact
   failed, skipped, or unavailable check with the concrete decision the human
   still owns; do not repeat the whole check.

## What stays a stop

| Situation | Why work stops there |
|-----------|----------------------|
| Real product or scope decision | Only the human can choose the outcome. |
| Explicit user stop, including a quoted "stop", tracked or not | The human owns the objective. |
| Denied action or missing consent | The prohibition binds the work it applies to. |
| Exhausted correction, native, or user limit | The budget for that work is spent. |
| Destructive action, publishing, another repository | Separate authorization is still required. |

A tracked task may end the turn pending for exactly these boundaries, including
a decided denial or an exhausted limit that already covers the affected work; a
decided boundary is never reopened by the continuity rule, and it never holds
back independent authorized work. Everything else inside the current
authorization continues. A pending child, an inapplicable check, a partial
result, or a check the existing budget still allows are states to run or report,
not decisions to delegate back. Waiting on launched work never licenses
abandoning runnable work or presenting an unverified outcome as final.

## Where the rule ships

| Surface | Carrier |
|---------|---------|
| ODD close, Pi | `internal/components/agentguidance/routing.go` (rendered as the provider contract bundle's `routing/pi.md`) |
| ODD close, every other runtime | The shared close gate in the same file, rendered into each runtime's routing block |
| Native Checking Contract, runtimes with receipt-driven development | The inline section of each `internal/assets/<runtime>/orchestrator.md` |
| Native Checking Contract, every other runtime | `internal/assets/skills/_shared/odd-orchestrator-sections.md`, `Native Checking Contract (ODD only)` |
| Installed orchestrator carriers, compact variants | Generated from the same sources; never hand-edited copies |

## Boundaries

- No blind final-text detection and no self-reprompt loop: a report does not
  restart the same pass hoping for a different wording.
- Missing proof is never a pass, and it is not a ban on verification: a required
  authorized check the existing budget still allows keeps running, with no new
  budget and no reset. An unavailable verifier is reported as unavailable, never
  retried indefinitely.
- Measured routing stays as it is: no new thresholds, writer reasons, budgets,
  or defaults are introduced by this rule.
- Genuinely exhausted limits, denied actions, and consent boundaries keep their
  existing authority; the continuity rule never reopens them.
- The generic "Needs your decision" terminal phrase is retired from the close
  gate and from Native Checking: a concrete blocker with the exact human
  decision is reported instead.

## Checklist

- [ ] The authorized next step ran instead of being announced.
- [ ] A pending child yielded the turn and resumed on completion, with no human input.
- [ ] An explicit stop ended the applicable work, including on small or untracked tasks.
- [ ] Any stop names the boundary it applies to and the decision the human owns.
- [ ] A decided denial or an exhausted limit kept the affected tracking pending without resuming it.
- [ ] A missing or unavailable check blocked the completion claim only.
- [ ] A required authorized check still ran inside its existing budget.
- [ ] No pass was invented and no blind retry loop started.
