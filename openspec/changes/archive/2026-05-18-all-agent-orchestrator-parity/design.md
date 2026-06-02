# Design: All-Agent SDD Orchestrator Parity

## Technical Approach

Patch only the non-Claude SDD orchestrator markdown assets that drifted from the Claude/OpenCode chain-strategy reference. Keep `internal/assets/opencode/sdd-orchestrator.md` and `internal/assets/claude/sdd-orchestrator.md` unchanged as semantic references. No runtime/template refactor: generated outputs change only because embedded markdown assets change.

## Architecture Decisions

| Decision | Choice | Alternatives considered | Rationale |
|---|---|---|---|
| Parity mechanism | Direct asset edits per platform | Shared template/helper | Scope is prompt parity only; a template refactor would broaden blast radius and invalidate the proposal. |
| Strategy wording | Add the canonical `### Chain Strategy` section with `stacked-to-main` and `feature-branch-chain` everywhere delivery planning exists | Use abbreviated or platform-specific aliases | Spec requires canonical names exactly; aliases would break downstream handoff. |
| Platform wording | Preserve each agent family’s execution model while adding `chain_strategy` propagation | Normalize all assets to OpenCode `delegate`/plugin wording | The change is parity in intent, not platform homogenization. |

## Data Flow

```text
embedded asset markdown
  └─ sdd.Inject(...)
       ├─ writes host-specific prompt/rule file
       └─ golden tests compare generated files

static tests ──→ validate required substrings and forbidden platform claims
```

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/assets/codex/sdd-orchestrator.md` | Modify | Add Chain Strategy section after Delivery Strategy; update Review Workload Guard to request/cache/pass `chain_strategy` to `sdd-tasks` and `sdd-apply`. Keep Codex `delegate`/`task` wording. |
| `internal/assets/gemini/sdd-orchestrator.md` | Modify | Same as Codex, preserving Gemini paths and sub-agent wording. |
| `internal/assets/qwen/sdd-orchestrator.md` | Modify | Same as Codex, preserving Qwen paths and “¿Seguimos?” phrasing. |
| `internal/assets/generic/sdd-orchestrator.md` | Modify | Add canonical Chain Strategy and propagation; preserve generic model-assignment block. |
| `internal/assets/kimi/sdd-orchestrator.md` | Modify | Add Chain Strategy using Kimi-native `/skill:sdd-*` and `multiagent:Task` language; pass both strategies to `sdd-tasks`/`sdd-apply` custom-agent prompts. |
| `internal/assets/kiro/sdd-orchestrator.md` | Modify | Add Chain Strategy but phrase propagation as Kiro phase context/native subagent context; keep approval-gate semantics. |
| `internal/assets/windsurf/sdd-orchestrator.md` | Modify | Add Chain Strategy for solo-inline execution; replace “sub-agent launch/prompts” strategy forwarding with “phase context”/inline phase wording. |
| `internal/assets/antigravity/sdd-orchestrator.md` | Modify | Same solo-inline strategy as Windsurf; do not claim SDD custom sub-agents or OpenCode persistence. |
| `internal/assets/assets_test.go` | Modify | Add table-driven non-Claude static assertions for canonical strategies, `chain_strategy`, and forbidden OpenCode persistence claims. Extend scoped asset list if needed for Windsurf/Antigravity. |
| `internal/components/sdd/inject_test.go` | Modify | Add focused generated-output checks for Kimi/Kiro/Windsurf/Antigravity wording where platform-native semantics are easiest to regress. |
| `internal/components/golden_test.go` | Modify | No structural change expected; existing golden tests will surface fixture drift. Only touch if a targeted assertion is clearer than a broad golden diff. |
| `testdata/golden/sdd-{codex,gemini,windsurf,kiro,antigravity}-*.golden` | Modify | Update direct golden fixtures affected by changed assets. Qwen has injection tests but no current golden fixture. Generic changes may affect Cursor/VS Code goldens because they consume `generic/sdd-orchestrator.md`; update only if diffs are direct and semantic. |

## Interfaces / Contracts

No Go API or data structure changes. Prompt contract change: downstream planning/apply instructions now carry both `delivery_strategy` and `chain_strategy` whenever chained PR delivery is selected or required.

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit/static | Non-Claude assets contain canonical strategy labels and no inaccurate OpenCode persistence claims | `go test ./internal/assets -run 'Test.*SDDOrchestrator.*'` |
| Injection | Platform-generated prompt files preserve native wording | `go test ./internal/components/sdd -run 'TestInject(Kimi|Qwen|Gemini|OpenClaw|.*Windsurf|.*Antigravity|.*Kiro)'` with targeted test names after implementation |
| Golden | Direct generated SDD fixtures match changed assets | `go test ./internal/components -run 'TestGoldenSDD_(Codex|Gemini|Windsurf|Kiro|Antigravity|Cursor|VSCode)' -update`, inspect diff, then rerun the same command without `-update`. |
| Broad | Regression safety | `go test ./...` then `go vet ./...`. |

## Migration / Rollout

No migration required. Existing installations pick up new wording on the next SDD injection/sync.

## Open Questions

- None.
