# Work continuity

An authorized outcome keeps moving. A pending child, a missing check, or a
reported blocker never converts authorized work into a question for the human:
the agent continues the runnable in-scope work and reports what is actually
outstanding.

This page describes the rule the ODD Close of every runtime states. Close only
when no runnable authorized step remains, then report the outcome. Each capable
orchestrator states the continue rule under Authorized work continuity; its
Native Checking Contract points at the Close instead of
restating it, because the close is installed beside it. It is shipped
policy text, not a claim about model behavior; the rule is not enforced by a
runtime watchdog.

## The rule

> Run authorized in-scope steps. Work through failures with a permitted
> route. A pending child is a wait: completion resumes the turn without
> asking. Only a real human decision, explicit user stop, denial,
> missing consent, exhausted limit, or failure without a permitted route
> stops only affected work.
>
> A tracked task ends the turn pending only for launched work still running,
> a real human decision, denial, missing consent, explicit stop, or exhausted
> limit. Report failures without a permitted route as incomplete. Continue
> independent authorized work; never abandon runnable work or present an
> unverified outcome as final.
>
> Missing, partial, unavailable, or declined proof bars claiming completion
> of affected work, not independent authorized work or required authorized
> checks in budget. Stop checks only at an exhausted or denied boundary or
> with no permitted route. Report the exact blocker and any genuine human
> decision; never invent a pass. Never turn a report into a blind
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
5. Diagnose, fix and rerun a failed check within its limits. Stop checking
   only at an exhausted or denied boundary or when no permitted route remains.
   Report the exact failed, skipped, or unavailable check and any concrete
   decision the human still owns.

## What stays a stop

| Situation | Why work stops there |
|-----------|----------------------|
| Real product or scope decision | Only the human can choose the outcome. |
| Explicit user stop, including a quoted "stop", tracked or not | The human owns the objective. |
| Denied action or missing consent | The prohibition binds the work it applies to. |
| Exhausted correction, native, or user limit | The budget for that work is spent. |
| A failure with no permitted route left | Only the affected work ends incomplete; independent authorized work continues. |
| Destructive action, publishing, another repository | Separate authorization is still required. |

A tracked task may end the turn pending while launched work is still running,
for a real human decision, or for the affected work stopped by denial, missing
consent, explicit user stop, or an exhausted limit. A failure without a
permitted route is reported as incomplete, not pending. These boundaries never
hold back independent authorized work. Waiting on launched work never licenses
abandoning runnable work or presenting an unverified outcome as final.

## Where the rule ships

| Surface | Carrier |
|---------|---------|
| ODD close, Pi | `internal/components/agentguidance/routing.go` (rendered as the provider contract bundle's `routing/pi.md`) |
| ODD close, every other runtime | The shared close gate in the same file, rendered into each runtime's routing block |
| Authorized work continuity (first sentence of the rule) | `internal/assets/skills/_shared/odd-orchestrator-sections.md`, `Orchestrator Identity and Role` |
| Native Checking Contract pointer to the close, runtimes with receipt-driven development | The inline section of each `internal/assets/<runtime>/orchestrator.md` |
| Native Checking Contract pointer to the close, every other runtime | `internal/assets/skills/_shared/odd-orchestrator-sections.md`, `Native Checking Contract (ODD only)` |
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
  gate, from Native Checking, and from the Delegated Verification Gate's
  correction bounds: blockers still open after the bounded correction leave the
  unit `status: partial`, naming any genuine human decision, while independent
  authorized work continues.
- Worker, setup, recheck, native, and user budgets stay separate; changed state
  earns a fresh check, not a budget reset.
- A worker keeps correcting a failing check while each attempt names a cause
  from the output and makes a change to address it. It stops with `status: partial`
  when the same cause fails again after a real fix and there is no new hypothesis,
  when passing would require weakening, skipping, or rewriting the check, or
  when the fix crosses an edit surface or an explicit limit. It returns the
  failing command and output, remaining work, missing proof, and diagnosed cause.
- That worker-local stop ends the cycle, not the unit. The orchestrator owns
  the next step: fix inline only when the fix is small and understood;
  otherwise give a new bounded writer the diagnosis and a new hypothesis.
  Never repeat the attempt or reset the cycle in a new session. If no route
  remains, the unit is blocked while independent work continues. Ask the user
  only for a genuine human decision.
- RED construction is separate from fixing. A harness or setup error gets one
  self-fix; if the test still cannot reproduce the failure, return `status: partial`
  with the reproduction blocker and its evidence.

## Checklist

- [ ] The authorized next step ran instead of being announced.
- [ ] A pending child yielded the turn and resumed on completion, with no human input.
- [ ] An explicit stop ended the applicable work, including on small or untracked tasks.
- [ ] Any stop names the boundary it applies to and the decision the human owns.
- [ ] A decided denial or an exhausted limit kept the affected tracking pending without resuming it.
- [ ] A missing or unavailable check blocked the completion claim only.
- [ ] A required authorized check still ran inside its existing budget.
- [ ] No pass was invented and no blind retry loop started.
