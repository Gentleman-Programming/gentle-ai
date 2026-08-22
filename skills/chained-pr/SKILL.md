---
name: gentle-ai-chained-pr
description: "Trigger: PRs over 400 lines, stacked PRs, review slices. Plan provider-aware chains that protect review focus and remote authority."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "2.0"
---

## Activation Contract

Load when a planned PR may exceed **400 changed lines**, SDD forecasts chain risk, or the user asks for chained/stacked PRs, review slices, or reviewer-load control.

## Hard Rules

- Detect the Git host read-only. Stop when it is unknown or ambiguous.
- Use the GitHub native route only when #3356 proves the exact host/build capability. Do not silently install it or fall back to manual GitHub choreography.
- Keep every PR at **≤400 additions + deletions**. The native GitHub route must never permit `size:exception`.
- Positively identified non-GitHub hosts use a host-specific adapter and portable chaining; a maintainer-approved `size:exception` is available only there.
- Issue approval, planning, SDD phase approval, `auto-chain`, RDD reviews/receipts, and delivery approval do not authorize remote create/submit/sync/update/merge operations. Those consume separate bounded authority.
- Compose Chain Context into generated `pr-body.md`; agents must not modify the target repository's PR template.

## Decision Gates

| Condition | Action |
| --- | --- |
| GitHub plus #3356 exact host/build proof | Use only the proven native command help surface. |
| GitHub without that proof | Fail-closed; provide the official setup and evidence guidance in the reference. |
| Positively identified non-GitHub host | Select the portable strategy explicitly. |
| Unknown or ambiguous host | Stop; request host clarification. |
| Over-budget `single-pr` on GitHub | Stop; do not use `size:exception`. |
| Over-budget `ask-on-risk` or `auto-chain` | Select or form a compliant chain; this is not remote authority. |

## Execution Steps

1. Detect the host and select GitHub native or portable routing.
2. Apply `ask-on-risk`, `auto-chain`, or `single-pr`; retain `feature-branch-chain` for portable routing.
3. Partition autonomous work units under the budget and compose `pr-body.md` with Chain Context.
4. Before every remote operation, require its separate bounded authority and use only the selected provider adapter.
5. After a sync, rebase, or base change, re-enter through review status. When RDD is disabled, follow ordinary repository policy and report `disabled/unmanaged`.

## Output Contract

Return provider, route, capability evidence, strategy, PR order, current boundary, dependency diagram, budget, verification plan, remote-authority status, and any portable `size:exception` rationale.

## References

- [references/chaining-details.md](references/chaining-details.md) — approved dependency, capability evidence, portable routes, Chain Context, and authority boundaries.
