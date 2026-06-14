# Organic Agent Trigger Rules Specification

**Domain**: `organic-agent-trigger-rules`  
**Type**: NEW Capability  
**Change**: organic-agent-trigger-rules  
**Status**: FINAL (Implemented and Verified)

**Purpose**: Define a declarative trigger-rules system that gentle-ai INSTALLS (not executes) into every supported agent. The system defines a closed set of lifecycle events, a binding schema with structured `when` conditions, a token-aware built-in default rule set, and mechanism for rendering rules as plain instructional text and injecting them idempotently into all supported agent assets through the existing installer/injection path.

## Key Concepts

**Events**: Lifecycle moments the AI orchestrator recognizes (pre-commit, pre-push, pre-pr, post-sdd-phase, on-ci, on-schedule). Semantic moments, NOT OS-level hooks.

**Bindings**: A mapping `event -> agent(s)` carrying a `when` condition and a `mode` (advisory vs strong recommendation).

**`when` Vocabulary**: Structured, closed conditions (Always, PathGlobs, MinDiffLines, Phases, Combine) rendered to plain instructional text.

**Mode**: Two values only: `advisory` (soft suggestion) and `strong` (firm recommendation). Neither blocks. No hard gates.

**Token Budget**: `when` is the cost controller. Everyday events get one cheap advisory lens. Full 4R fan-out only on hot paths or large diffs. Judgment-day only on high-stakes phases.

## Closed Vocabularies

### Supported Events (Exact)
- `pre-commit` — Before commit creation
- `pre-push` — Before pushing to remote
- `pre-pr` — Before PR is opened/marked ready
- `post-sdd-phase` — After an SDD phase completes
- `on-ci` — When CI pipeline run is triggered
- `on-schedule` — On a recurring schedule

### Supported Modes (Exact)
- `advisory` — Non-urgent suggestion language (e.g., "Consider running...")
- `strong` — Firm recommendation language (e.g., "Strongly recommend..."). Maximum enforcement level in organic-only model; never implies blocking.

### Supported `when` Forms (Exact)
- `Always: true` — Unconditional activation
- `MinDiffLines: N` — When cumulative changed lines exceed N (must be positive integer)
- `PathGlobs: [...]` — When changed files match any glob (must be non-empty)
- `PathGlobs + MinDiffLines (Combine: "or")` — EITHER condition activates
- `Phases: [...]` — Valid only on `post-sdd-phase`; activates when phase is in list

### Default Rule Set (Tier-Based Token Tuning)

**Tier-1: Cheap Advisory on Everyday Events**
- `pre-commit`: always, review-readability, advisory
- `pre-push`: always, review-readability, advisory

**Tier-2: Full 4R on Hot Paths or Large Diffs**
- `pre-pr`: (PathGlobs: [**/auth/**, **/update/**]) OR (MinDiffLines: 400), all four 4R agents, strong

**Tier-3: Judgment-Day on High-Stakes Phases**
- `post-sdd-phase`: phases in [design, apply], judgment-day, strong

**No Default**: on-ci, on-schedule (installation-specific; users opt in via future override)

## Implementation

**Type Model**: `internal/model/types.go` — TriggerEvent, TriggerMode, TriggerWhen, TriggerBinding, TriggerRuleSet (with JSON tags for future override loader).

**Catalog**: `internal/catalog/triggers.go` — DefaultTriggerRuleSet(), SupportedTriggerEvents(), KnownAgents(), ValidateTriggerRuleSet(). Closed agent set includes: 4R review lenses, judgment-day, all 8 SDD phase identifiers.

**Renderer**: `internal/components/sdd/triggerrules.go` — RenderTriggerRules(set) -> pure, deterministic, plain-text directive block. No markers (caller wraps via InjectMarkdownSection).

**Injection**: `internal/components/sdd/inject.go` step 1c — Injects rendered block under section ID `gentle-ai:trigger-rules` into all supported agents (claude, opencode, cursor, codex, gemini, vscode, windsurf, antigravity). Per-adapter routes:
- Jinja agents (Kimi/Qwen): write standalone module `trigger-rules.md`, add `{% include %}` to template
- System-prompt agents: InjectMarkdownSection(existing, "trigger-rules", rendered)
- OpenCode/Kilocode: append marker-wrapped block to gentle-orchestrator prompt

**Kimi Template**: `internal/assets/kimi/KIMI.md` — Add `{% include "trigger-rules.md" ignore missing %}` line.

## Non-Goals (Absence Requirements)

- No execution of agents by gentle-ai
- No git hook generation
- No event bus, daemon, listener, or scheduler
- No hard gates or blocking behavior
- No `when` evaluation engine in gentle-ai (conditions rendered as instruction text only)
- No new parse dependencies (YAML/TOML/INI)

## Testing Strategy (Strict TDD)

- **Schema validation** (model layer): closed event/mode sets, all default bindings reference known events/agents
- **Token-shape verification** (catalog): pre-commit/pre-push are exactly one advisory lens; pre-pr 4R is gated; judgment-day only on design/apply; on-ci/on-schedule have zero defaults; all bindings have non-empty reason field
- **Validator tests** (catalog): default set validates clean; unknown run/on/mode/when each return error
- **Renderer tests** (sdd component): deterministic output (golden), mode wording (consider vs strongly recommend), when phrasing matches vocabulary, organic note present, marker-free, ≤40 lines
- **Injection tests** (sdd component): per-adapter coverage, marker section present, idempotent (no duplication on re-run), Jinja module written, OpenCode path correct

## Success Criteria (All Met)

- [x] Declarative schema defined (events, bindings, when vocabulary, modes)
- [x] Closed supported-events catalog (6 events)
- [x] Token-aware default rule set (3 tiers: everyday advisory → hot-path 4R → high-stakes judgment-day)
- [x] Rules rendered and injected into all 8 supported agents, idempotently
- [x] No execution, git hooks, event bus, deterministic gates, when-eval engine, or new parse dependencies
- [x] All tests pass clean (go build/vet/test)

## Organic Nature

gentle-ai remains a pure installer/injector. It renders rules as plain instruction text for AI orchestrators to read and decide whether to follow. Orchestrators may ignore recommendations; gentle-ai does not execute, block, or verify compliance. The `when` conditions are rendered as human-readable constraints, never evaluated by the binary.
