# Organic Implementation Trigger Rules

<- [Back to README](../README.md)

Ask for the outcome. Gentle AI keeps already-understood work inline, delegates only
the actions that benefit from fresh context, and offers SDD only when durable
planning would materially reduce uncertainty. Verification, review, delivery, and
lifecycle authority remain native provider responsibilities behind that simple
interaction.

## Quick path

1. Describe the outcome in natural language.
2. Gentle AI uses the smallest useful implementation route: direct inline,
   delegated direct, or an optional SDD proposal.
3. The normal interaction reports only **Working**, **Checking**, **Ready**, or
   **Needs your decision**.

The user does not choose review internals, hashes, receipts, or lifecycle
transitions. A question is necessary only when the answer changes requested
scope, destructive or irreversible impact, permission or security exposure,
verification cost or external side effects, accepted residual risk, or delivery.

## Implementation routes

| Route | Use it when | What happens |
|---|---|---|
| **Direct inline** | Deciding or verifying requires **1–3 files**; or the change is **one mechanical, already-understood file** with no research or unresolved design decision. | Keep the bounded action inline. |
| **Delegated direct** | Understanding requires **4+ files**; reading prepares a write; broad research is needed; or a writer must change **2+ non-trivial files**. | Delegate the narrow exploration and/or one writer needed for that action. |
| **Optional SDD** | The work has substantial ambiguity, or durable proposal, spec, design, or task artifacts would materially reduce uncertainty. | Propose SDD. Select it only after an explicit request or an accepted proposal. |

The file counts describe the context needed for the current action, not a risk
score and not an SDD threshold. Risk may strengthen native verification or
review, but it never forces SDD.

Delegation also applies per action. Tests, builds, installs, and native review
actors may use fresh workers without changing the implementation route or
creating an SDD run. Direct and delegated work create no SDD artifacts, phase
attempts, or synthetic SDD lifecycle.

If apparently simple work reveals substantial ambiguity, Gentle AI may offer SDD
at the next safe boundary. Declining it leads to a safely reduced scope, a
justified direct or delegated route, or **Needs your decision**—never silent SDD
enrollment.

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
success from prose. Existing SDD v1 runs continue through their SDD-specific
status contract; direct and delegated runs do not create or consume an SDD run.

## Review mode

Review-driven development is user-owned and independent of the implementation
route:

| Command | Effect |
|---|---|
| `gentle-ai review mode status --cwd <repo>` | Report the global source, clone-local source, deciding source, and effective mode without mutation. |
| `gentle-ai review mode disable --cwd <repo>` | Disable review-driven development globally. |
| `gentle-ai review mode disable --scope clone --cwd <repo>` | Disable it only for this clone; no other clone inherits the override. |
| `gentle-ai review mode enable --cwd <repo>` | Enable it globally for future candidates. |
| `gentle-ai review mode enable --scope clone --cwd <repo>` | Clear this clone's off-only override. |

Any disabled source wins. A clone may opt out but cannot require review for the
user. Interactive starts ask before reviewer work; non-interactive tier-1/tier-2 starts proceed without prompting and report how to disable review mode. Interactive consent is asked
once per clone. Accepting records that choice; **not now** applies only to that
candidate and does not change review mode.

While review mode is disabled, continue through direct inline, delegated direct,
or optional SDD routing without starting, retrying, or re-enabling review on the
user's behalf. Existing exact governing receipts remain authoritative; otherwise native review delivery gates report `disabled/unmanaged` and
defer to ordinary repository policy without fabricating approval.

The current unstable RDD line has two known limitations, both in SDD while
review mode is disabled. The pre-verify status path can still require review.
And the native archive gate now reports `disabled/unmanaged` and lets archive
proceed, but the `sdd-archive` skill still requires `reviewGate.result: allow`
in its own contract, so an agent following that skill blocks where the product
no longer does. See
[Organic RDD known limitations](architecture/organic-rdd.md#9-known-open).

## Installation and refresh

`gentle-ai install` and `gentle-ai sync` project the same canonical rules into
every supported adapter, independently of whether the optional SDD component is
selected:

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
