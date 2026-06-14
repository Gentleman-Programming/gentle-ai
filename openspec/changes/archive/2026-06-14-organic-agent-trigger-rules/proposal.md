# Proposal: Organic Agent Trigger Rules

Give gentle-ai a declarative way to express WHEN each supported agent (the 4R review lenses, judgment-day judges, sdd-* phases, and any future agent) should run during the everyday dev lifecycle, then INJECT those rules into the assets gentle-ai already installs so every AI tool's orchestrator picks them up and follows them organically. Today there is no declarative trigger layer at all: users must remember to invoke agents by hand, so the agents gentle-ai installs sit idle until someone thinks to call them. This change makes them integrate into the daily workflow as a natural, recommended part of the flow — without turning gentle-ai into a runtime that executes anything.

## Why

- **No declarative "when" exists.** gentle-ai installs powerful agents (review-risk, review-readability, review-reliability, review-resilience, judgment-day judges, sdd phases) but ships zero guidance on the lifecycle moments at which each should run. The value of an installed agent that nobody remembers to invoke is near zero.
- **Manual invocation does not scale across a team or a workflow.** A reviewer agent is only useful if it fires at the right moment (before a commit, before a PR). Relying on human memory means the most valuable lenses run least often — exactly when fatigue is highest and stakes are highest.
- **Token cost is unmanaged.** The 4R as a fan-out costs ~4x fresh context versus ~1x for a classic single-context reviewer; adversarial verification scales as roughly `4 + 3 * findings`. With no declarative filter, the only two states are "never run them" or "run everything everywhere and burn tokens until users disable the feature." A `when` condition is the missing token-budget controller.
- **gentle-ai is an installer, and should stay one.** The product already injects system-prompt sections, AGENTS.md sections, and per-agent `.md` files. Trigger rules belong in that same injected layer — declarative text the orchestrator reads — not in a new execution engine.

## What changes

Introduce a **declarative trigger-rules system** that gentle-ai installs (not executes) into every supported agent. The system has three conceptual pieces and one hard constraint.

### Conceptual model

| Piece | Role | Plain meaning |
|-------|------|---------------|
| **Events** | the "when" | Lifecycle moments the AI orchestrator recognizes (pre-commit, pre-push, pre-pr, post-sdd-phase, on-ci, on-schedule). Semantic moments the orchestrator honors, NOT OS-level hooks. |
| **Bindings** | the "what with what" | A mapping `event -> agent(s)` carrying a `when` condition (path globs, diff size, change type) and a `mode` (advisory vs strong recommendation). |
| **Execution policy** | the "how" | Advisory vs strong recommendation, parallel lenses, which severity matters. No real blocking — organic. |

### Declarative schema concept (illustrative, not final)

```yaml
trigger_rules:
  - on: pre-pr
    when: diff.touches("**/auth/**", "**/update/**") or diff.lines > 400
    run: [review-risk, review-resilience]
    mode: strong
  - on: pre-commit
    when: always
    run: [review-readability]
    mode: advisory
  - on: post-sdd-phase
    when: phase in ["design", "apply"]
    run: [judgment-day]
    mode: strong
```

The schema is authored once (built-in defaults plus user overrides) and rendered into a human-readable directive block that the orchestrator reads as part of its normal flow. The exact serialization (YAML-like vs a rendered markdown table) is a design-phase decision; note the repo has NO YAML/TOML parse library (custom helpers in `internal/filemerge/`, JSON via `encoding/json`), so the authoring format and the injected format may differ.

### Token budget via `when` (core requirement, not optional)

`when` is the cost controller and MUST be treated as a first-class requirement, not a convenience filter. Bindings MUST scale by blast radius:

- **Cheap advisory lenses on everyday events** — e.g. review-readability (R2) on `pre-commit`, `when: always`.
- **The full 4R fan-out only when `when` matches a hot path or a large diff** — e.g. `pre-pr` + `diff.touches(hot paths) or diff.lines > 400`.
- **Adversarial verification (judgment-day) reserved for high-stakes moments** — e.g. `post-sdd-phase` on design/apply, never on every commit.

The default rule set MUST be tuned so a normal day costs a small, predictable token amount and the expensive lenses only fire when the change actually warrants them.

### Organic injection across all supported agents

Rules are injected through the existing installer path — the same mechanism that already writes `<!-- gentle-ai:... -->` marker sections into system prompts and SDD orchestrator assets (`internal/components/sdd/inject.go`). Natural injection points, in priority order:

1. **Per-agent system-prompt / orchestrator section** (primary) — guaranteed loaded every session.
2. **AGENTS.md section** — alongside the existing skill index + trigger table.
3. **Per-agent `.md` files** — for agent-specific phrasing where needed.

Rules MUST be rendered for ALL supported agents: claude, opencode, cursor, codex, gemini, vscode, windsurf, antigravity. The injected content is plain instructional text ("at pre-pr, if the diff touches auth or exceeds 400 lines, strongly recommend running review-risk and review-resilience in parallel"), so it works regardless of an agent's native config format.

## Scope

### In Scope

1. A **declarative trigger-rules schema** (events, bindings with `when` + `mode`, execution policy) — defined and documented.
2. A **supported events catalog** — the closed set of lifecycle moments the orchestrator is told to recognize.
3. A **built-in / default rule set** — sensible, token-aware defaults for the 4R, judgment-day, and sdd phases.
4. **Rendering + injection** of rules into the installed assets for ALL supported agents, through the existing installer/injection path.
5. The **token-budget-via-`when`** requirement, baked into both the schema and the default rule set.
6. User-facing **documentation** of how rules are authored, defaulted, and (later) overridden.

### Out of Scope (Non-goals)

| Non-goal | Reason |
|----------|--------|
| **Executing agents** | gentle-ai stays an installer/injector. It renders rules; the AI tool's orchestrator runs them. |
| **Generating git hooks** | Events are semantic moments honored by the orchestrator, not OS-level hooks. |
| **Event bus / runtime dispatch** | No runtime layer is added; no daemon, no listener, no scheduler. |
| **Deterministic / hard gates** | Organic-only by decision. No blocking. `mode: strong` is the strongest level — a strong recommendation, not a gate. |
| **Deterministic or hybrid execution model** | Explicitly deferred. May come LATER as a separate change; the organic cut is the deliberate first slice. |
| **A `when` expression engine that gentle-ai evaluates** | gentle-ai does not evaluate `when`; it renders the condition as instruction text for the orchestrator to interpret. |

## How it integrates (high-level approach)

1. **Define the schema and events catalog** as data structures (`encoding/json`-friendly, no new parse dependency) plus a documented authoring format.
2. **Ship a built-in default rule set** in the catalog layer alongside the existing skills catalog (`internal/catalog/`), token-tuned per the budget requirement.
3. **Render rules to instructional text** — a renderer turns the rule set into per-agent directive blocks (human-readable, marker-wrapped).
4. **Inject via the existing path** — extend the current SDD/AGENTS.md section injection (`internal/components/sdd/inject.go`, `internal/filemerge/section.go`) to write a `<!-- gentle-ai:trigger-rules -->` section into each agent's system prompt / AGENTS.md, idempotently, for every supported adapter.
5. **No execution code anywhere** — the entire change is schema + defaults + renderer + injection wiring + tests + docs.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/model/types.go` | Modified | Trigger-rule, event, binding, and mode types |
| `internal/catalog/` (new file, e.g. `triggers.go`) | New | Built-in default rule set + supported events catalog |
| `internal/components/sdd/inject.go` | Modified | Render + inject the trigger-rules section per agent |
| `internal/filemerge/section.go` | Possibly modified | Marker section for `gentle-ai:trigger-rules` if a new marker is needed |
| `internal/assets/{agent}/...` | Modified | Per-agent rendered directive blocks where agent-specific phrasing is required |
| AGENTS.md (rendered output) | Modified | Trigger-rules section alongside the skill index/trigger table |
| Tests across `internal/{catalog,components,model}` | New/Modified | Schema, default-set, renderer, and per-agent injection tests (strict TDD active) |

## Risks & tradeoffs

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| **Organic = not guaranteed to fire.** The orchestrator MAY ignore an injected recommendation. | High (inherent) | Accepted tradeoff of the organic-only decision. `mode: strong` uses firm directive language; deterministic gating is an explicit future change, not this one. |
| **LLM ignores or misreads the rules section** | Med | Keep the rendered block short, scannable, and unambiguous (cognitive-doc-design principles); place it in the always-loaded system prompt, not a deep file. |
| **Token blow-up if defaults are mis-tuned** | Med | `when` is a first-class requirement; default set fans out the 4R only on hot paths / large diffs; everyday events get a single cheap lens. |
| **`when` semantics drift per agent** (one orchestrator reads `diff.lines` differently than another) | Med | Render `when` as plain, self-explanatory instruction text; keep the vocabulary small and documented in the events catalog. |
| **Per-agent rendering divergence** across 8 agents | Med | Single renderer, single default set; per-agent differences limited to phrasing, covered by per-adapter injection tests. |
| **No YAML/TOML parser in the module** | Low | Author rules as Go data + `encoding/json`; render to text. No new parse dependency, consistent with the existing `internal/filemerge/` approach. |

## Rollback Plan

The change is additive and injection-based. Rollback is removing the `<!-- gentle-ai:trigger-rules -->` section from the installed assets (the next sync/install with the feature reverted strips it via the marker), plus reverting the catalog/types/renderer additions. No runtime state, no migrations, no executed side effects to unwind.

## Delivery context

- Single PR, `size:exception` (line count is not a constraint for this change).
- Work happens in worktree `/Users/alanbuscaglia/work/gentle-ai-wt-organic-triggers` on branch `feat/organic-agent-trigger-rules`; closes via issue -> PR -> main.
- Strict TDD is active (`go test ./...`); every new schema/renderer/injection unit lands test-first.

## Success Criteria

- [x] A declarative trigger-rules schema (events, bindings with `when` + `mode`, execution policy) is defined and documented.
- [x] A closed supported-events catalog exists (pre-commit, pre-push, pre-pr, post-sdd-phase, on-ci, on-schedule, plus any agreed additions).
- [x] A token-aware built-in default rule set ships covering the 4R, judgment-day, and sdd phases.
- [x] Default bindings demonstrably scale by blast radius: cheap advisory lens on everyday events; full 4R only on hot paths / large diffs; judgment-day only at high-stakes moments.
- [x] Rules are rendered and injected into the installed assets for ALL supported agents (claude, opencode, cursor, codex, gemini, vscode, windsurf, antigravity), idempotently.
- [x] gentle-ai adds NO execution, NO git hooks, NO event bus, NO deterministic gate — verified by the absence of any runtime dispatch code.
- [x] `go build ./...`, `go vet ./...`, and `go test ./...` pass clean.
