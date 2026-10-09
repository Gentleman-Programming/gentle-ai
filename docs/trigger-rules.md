# Organic Implementation Trigger Rules

> [!NOTE]
> These docs track `main`, which may include unreleased changes. For the latest release, see the [v4.0.0 docs](https://github.com/Gentleman-Programming/gentle-ai/tree/v4.0.0/docs).

<- [Back to README](../README.md)

Ask for the outcome. Gentle AI™ keeps already-understood work inline, delegates only
the actions that benefit from fresh context, and uses [Organic Driven Development
(ODD)](usage.md#organic-driven-development-odd) for every implementation request.
Native providers own verification, review mechanics, and lifecycle authority; ordinary repository policy owns delivery.

> **Since v4.0.0:** SDD (Spec-Driven Development) is retired. ODD is
> the only implementation workflow; there is no optional alternative route.

## Quick path

1. Describe the outcome in natural language.
2. Gentle AI uses the smallest useful implementation route: direct inline or
   delegated direct.
3. The normal interaction reports only **Working**, **Checking**, **Ready**, or
   **Needs your decision**.

The user does not choose review internals, hashes, receipts, or lifecycle
transitions. A question is necessary only when the answer changes requested
scope, destructive or irreversible impact, permission or security exposure,
verification cost or external side effects, accepted residual risk, or delivery.

## Implementation routes

| Route | Use it when | What happens |
|---|---|---|
| **Direct inline** | The task is **small**: understood within the inline evidence budget, risk contained, and resumable from the request plus `git diff`, whatever the number of files. | Read, edit, and run the focused test and suite inline. |
| **Delegated direct** | Understanding exceeds the inline evidence budget; or a writer has a named reason (two or more independent units with disjoint edit surfaces launched together, or the long-session context backstop), so reading that prepares that write delegates with it; or the change is **high risk** and needs an independent verifier. Size, a large task alone, file count, or a price ratio never decides it. | Follow the 3-step sequence (exploration -> design-minimality pass -> writer brief). Delegate the narrow exploration, one bounded writer per unit with a reason, and/or the verifier needed for that action. |

File count never decides the route or the task size. High risk takes a task
out of the small path and adds an independent verifier after the change's own
checks. The feature document still comes only from a failed resume test; a
large task without a writer reason stays inline, following its logbook. When receipt-driven development is on and native assess returns a
risk tier, that tier wins; the agent may only raise it.

Delegation also applies per action. For delegated direct implementation, follow the 3-step sequence: exploration -> design-minimality pass -> writer brief. Exploration maps context and execution paths; the design-minimality pass evaluates the minimality ladder, estimates authored footprint, logs deliberate deferrals, and defines required tests and boundary invariants; and the resulting complete minimal plan and constraints are handed to the writer as its brief. Tests, builds, installs, and native review
actors may use fresh workers without changing the implementation route.

If work reveals uncertainty, use optional scoped research or clarify the affected
decision within ODD. Substantial work needs no separate planning phase or
artifact handoff.

## Native progress and authority

| Public state | Meaning |
|---|---|
| **Working** | The implementation can still change. |
| **Checking** | Gentle AI is performing the applicable functional proof and bounded review. |
| **Ready** | The exact candidate has sufficient evidence for the selected delivery route. |
| **Needs your decision** | Safe automatic convergence is impossible; Gentle AI presents the cause, impact, and concrete choices. |

The user still asks only for the outcome. Repository identity, route, policy,
candidate, delivery mechanism, and authority references remain owner-derived.
Adapters do not select review lenses, reconstruct recovery policy, or infer
success from prose.

## Review mode

Receipt-driven development is user-owned, opt-out, and independent of the
implementation route. With no source expressing an opinion it is **on**,
reported as decided by `default`:

| Command | Effect |
|---|---|
| `gentle-ai review mode status --cwd <repo>` | Report the global source, clone-local source, deciding source, and effective mode without mutation. |
| `gentle-ai review mode enable --scope global --cwd <repo>` | Explicitly enable receipt-driven development globally for future candidates. |
| `gentle-ai review mode disable --cwd <repo>` | Disable receipt-driven development globally. |
| `gentle-ai review mode disable --scope clone --cwd <repo>` | Disable it only for this clone; no other clone inherits the override. |
| `gentle-ai review mode enable --scope clone --cwd <repo>` | Clear this clone's off-only override. Inherit global mode or default ON; an explicit global OFF still wins. |

Any disabled source wins. A clone may opt out but cannot require review for the
user. Automation must never toggle the mode automatically or persist the default
as a user preference. Interactive starts ask before
reviewer work; non-interactive tier-1/tier-2 starts proceed without prompting and
report how to disable review mode. Interactive consent is asked
once per clone. Accepting records that choice; **not now** applies only to that
candidate and does not change review mode.

While review mode is disabled, continue through direct inline or delegated direct
routing without starting, retrying, or re-enabling review on the
user's behalf. Review context may remain visible when available, but it never
authorizes or blocks commit, push, PR, release, or archive. Native delivery gates
report `disabled/unmanaged` when no exact receipt applies and never fabricate approval.

## Review store reset

Review authority accumulates without bound: every candidate leaves a lineage
behind and nothing removes a delivered one, so a long-lived clone eventually
holds hundreds of lineages and hundreds of megabytes of candidate checkouts.
`review abandon` refuses terminal states and `review reclaim` refuses any entry
holding authoritative artifacts, so until now a degraded store had no exit.

| Command | Effect |
|---|---|
| `gentle-ai review store-reset --cwd <repo>` | Report, per category, what a reset would remove and what it would preserve. Removes nothing. |
| `gentle-ai review store-reset --cwd <repo> --confirm` | Remove this clone's review lineage state. Irreversible. |
| `gentle-ai review store-reset --cwd <repo> --confirm --include-in-flight` | Also remove reviews that have not reached a terminal state. |
| `gentle-ai review store-reset --cwd <repo> --confirm --include-adapter-reviews` | Also remove the adapter-written `reviews/` graph store, which the lease and the in-flight refusal do not cover. |
| `gentle-ai review store-reset --cwd <repo> --json` | The same report, machine-readable. |

Every sub-action is user-initiated only; no adapter and no automation reaches
it, because the verb carries no negotiated contract row. Preview is the default
and `--confirm` is required to remove anything: the operation is irreversible
and clone-wide, so the invocation typed from memory has to be the one that only
looks. It is clone-scoped and never touches a global or machine-wide location.

It **removes** `candidate-views/` and the `v1`, `v2`, `quarantine`,
`effect-markers`, and `incidents` subtrees of `review-transactions/`. Candidate
views are registered Git worktrees, so their administrative directories are
removed too, but only when each one's own `gitdir` file proves it belongs to
that exact view; no unrelated worktree is pruned. A registration moved aside for
a category that then could not be removed is put back; on the rare occasion the
move back also fails, the directory is kept rather than deleted, and the report
names it, names where it was left, and withdraws its usual claim that nothing
marked SKIPPED was touched.

It **preserves** the receipt-driven-development kill switch in both the
`review-mode/` location and the pre-#2882 mirror inside
`review-transactions/rar-authority/`, along with the legacy `sdd-runtime/` attempt state,
`defect-reports/`, `review-artifacts/`, `incidents/`, and
`REVIEW-MAINTENANCE.lock`. Reviews that were off stay off. The list is an
allowlist, so any path the command does not recognize -- including one a future
release adds -- is reported and left in place rather than guessed at.

It **withholds** `reviews/`, the review graph store the gentle-pi adapter
writes, and removes it only when `--include-adapter-reviews` is given. Both
safeties described below stop at the edge of that directory:
`REVIEW-MAINTENANCE.lock` does not cover any path under it, so the exclusive
lease excludes no writer there, and the in-flight classification reads only
`review-transactions/v2`, so a review living there can never be listed as open.
A default run therefore cannot tell a dead graph from a live review, and a
destructive command does not delete what it cannot vouch for. The category is
still measured and still reported, under the preserved list, carrying that
reason and the flag that overrides it.

Reviews that have not reached a terminal state (anything other than approved,
escalated, or invalidated, plus any record that cannot be parsed) are refused by
default and listed by name. The reset takes the same exclusive maintenance lease
every other maintenance operation takes *before* it reads the store, so that
classification and the removal it authorizes are one atomic step: a review that
starts while the reset is waiting for the lease is seen and refused, never
destroyed by a run that reported none open. Each category is removed all at once
or not at all -- a category that cannot be moved aside is reported with its
reason and left exactly as it was found, worktree registrations included. The
reset runs while review mode is disabled: gating cleanup on the kill switch
would rebuild the dead end it exists to remove.

The TUI exposes the same action as **Reset review store** in the main menu,
between *Manage backups* and *Managed uninstall*, showing the same survey behind
a confirmation whose cursor starts on *Cancel*. The TUI has no
`--include-in-flight` or `--include-adapter-reviews` equivalent: when open
reviews exist it refuses and prints the CLI invocation instead, so destroying
in-flight work is never one keystroke away.

## Installation and refresh

`gentle-ai install` and `gentle-ai sync` project the same canonical rules into
every supported adapter:

- Standard adapters receive the managed `agent-routing` marker in their
  adapter-owned system-prompt file.
- OpenCode and Kilocode receive it inside
  `agent.gentle-orchestrator.prompt` in their adapter-owned `opencode.json`.
- Jinja-backed adapters receive an `agent-routing.md` module included by their
  managed router template.

```bash
gentle-ai install   # full install
gentle-ai sync      # refresh managed content
```

Refresh is idempotent: the managed projection is replaced without duplication.

## Source of truth

The rendered projection comes from
[`internal/components/agentguidance/routing.go`](../internal/components/agentguidance/routing.go),
and its adapter delivery is owned by
[`internal/components/agentguidance/inject.go`](../internal/components/agentguidance/inject.go).
Canonical route facts come from
[`internal/agents/capabilitymanifest/manifest.go`](../internal/agents/capabilitymanifest/manifest.go).
Review mode is implemented in
[`internal/cli/review_mode.go`](../internal/cli/review_mode.go). The current
authority and recovery behavior is documented in the
[Organic RDD architecture](architecture/organic-rdd.md).
