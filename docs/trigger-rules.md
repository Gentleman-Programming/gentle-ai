# Agent Trigger Rules

<- [Back to README](../README.md)

---

gentle-ai injects a **trigger-rules** section into every supported agent's system prompt or orchestrator configuration. This section is a deterministic triage router that tells the AI orchestrator which review and verification agents to run at each moment of the development workflow.

## What Are Trigger Rules?

Trigger rules are a **deterministic triage router, not advice**. gentle-ai renders the rules as plain instruction text and injects them into the agent's prompt; the AI orchestrator applies them as a decision procedure at each event. gentle-ai never fires, blocks, or executes any rule itself — it renders text only, and nothing in the router pauses or gates the user's workflow.

The injected section looks like this in your agent's system-prompt file:

```markdown
<!-- gentle-ai:trigger-rules -->
## Agent Trigger Rules

Deterministic triage router. gentle-ai renders this text; the AI orchestrator
applies it as a decision procedure, not advice. Triage every diff into exactly
one tier before acting:

1. **Trivial diff** (ONLY documentation, comments, formatting, or typo fixes
   in strings — zero executable code and zero configuration changes): run no
   review lens. Any diff touching executable code or configuration is at
   least standard tier.
2. **Standard diff**: run exactly ONE lens — the risk-table row matching the
   dominant risk; do not add lenses.
3. **Hot path or large operational diff**: run the full 4R fan-out; never at
   pre-commit or pre-push. Pure human documentation above 400 authored lines
   runs only `review-readability`.

- At **pre-commit**, always: trivial diff → no lens; otherwise run exactly ONE
  lens selected by the risk table (default `review-readability`); never the
  full 4R fan-out here.
- At **pre-pr**: validate the existing content-bound receipt; reviewer lenses
  run only inside explicit `review/start(target)`. At START, hot paths and
  diffs above 400 authored lines outside pure human documentation run all four
  4R lenses; large pure human documentation runs only `review-readability`.
- ...
<!-- /gentle-ai:trigger-rules -->
```

## Where the Section Is Injected

| Agent | Location |
|-------|----------|
| Claude Code | `~/.claude/CLAUDE.md` (marker section) |
| Gemini CLI | `~/.gemini/GEMINI.md` (marker section) |
| Cursor | `~/.cursor/rules/gentle-ai.mdc` (marker section) |
| VS Code Copilot | `.instructions.md` (marker section) |
| Codex | `~/.codex/AGENTS.md` (marker section) |
| Antigravity | `~/.gemini/GEMINI.md` (marker section) |
| Windsurf | `~/.codeium/windsurf/memories/global_rules.md` (marker section) |
| Kiro | `~/.kiro/steering/gentle-ai.md` (marker section) |
| Hermes | `~/.hermes/SOUL.md` (marker section) |
| OpenCode | `opencode.json` → `agent.gentle-orchestrator.prompt` (inline) |
| Kilocode | `opencode.json` → `agent.gentle-orchestrator.prompt` (inline) |
| Kimi | `~/.kimi/trigger-rules.md` (Jinja module, included via `KIMI.md`) |

## Triage Tiers

The orchestrator triages every diff into exactly one tier before acting:

**Tier 1 — Trivial diff → no lens**

- A diff is trivial ONLY if it changes documentation, comments, formatting, or fixes typos in strings — zero executable code and zero configuration changes. Run no review lens at all.
  Cost: 0x. Trivial work never pays a review cycle. Any diff touching executable code or configuration is at least standard tier.

**Tier 2 — Standard diff → exactly ONE lens**

- Every non-trivial diff runs exactly one lens: the risk-table row matching the dominant risk (clear naming, structure, maintainability, or small refactors → `review-readability`; behavior, state, tests, determinism, or regressions → `review-reliability`; shell/process integration, partial failures, recovery, or degraded dependencies → `review-resilience`; security, permissions, data exposure/loss, architecture, or dependencies → `review-risk`). If multiple rows match, the orchestrator picks the single highest-impact row; it never adds lenses.
  Cost: ~1x. This is the everyday tier for `pre-commit`, `pre-push`, and off-hot-path `pre-pr` diffs.

**Tier 3 — Hot path or large operational diff → full 4R fan-out**

- Inside explicit `review/start(target)`, auth/update/security/payments paths or
  more than 400 authored lines in code, configuration, prompts, agent rules,
  workflows, runtime instruction docs, mixed content, or active content run all
  four 4R lenses (`review-risk`, `review-resilience`, `review-readability`,
  `review-reliability`). Pure human documentation above 400 lines runs only
  `review-readability`.
  Cost: ~4x for full 4R and ~1x for large pure documentation. Lifecycle gates
  validate the receipt and never launch reviewers.

For ad-hoc 4R outside a native ordinary transaction, after a fix rerun only the
originating lens or lenses that produced open verified BLOCKER/CRITICAL
findings. Do not rerun clean lenses or lenses with only WARNING/SUGGESTION
findings. Native ordinary review keeps its targeted validator and never reruns
initial lenses.

**High-stakes SDD phases**

- `post-sdd-phase` after the `design` or `apply` phase: run `judgment-day` adversarial verification.
  Cost: two blind judges per judgment round, with no refuter fan-out. Reserved for the SDD phases most likely to introduce architectural debt.

**No built-in binding for `on-ci` and `on-schedule`** — the appropriate agent and cadence for CI and scheduled runs are installation-specific. Both events are part of the supported event vocabulary and can be used in a future override mechanism.

## Refreshing the Injected Section

Re-run install or sync after an update to refresh the injected section:

```bash
gentle-ai install   # full install
gentle-ai sync      # re-sync only (faster)
```

The injection is idempotent — running it twice replaces the existing section without duplication.
