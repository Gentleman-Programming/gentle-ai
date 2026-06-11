# Design: Bind the chained-pr skill to all SDD orchestrator assets

## Technical Approach

Add ONE uniform, host-agnostic binding sentence to the `### Chain Strategy`
section of every SDD orchestrator template (11 total), placed immediately after
the `chain_strategy` forwarding line. The sentence references the skill by
registry name (`chained-pr`) and defers actual path resolution to each
template's existing **skill-resolution mechanism**, so it never asserts a
hardcoded path and never claims a delegation/persistence mechanism a host does
not have. Cursor gains the full `### Chain Strategy` section (currently missing)
plus the same binding. Static assertions in `assets_test.go` and
`inject_test.go` lock the binding substring across all hosts; cursor and
opencode are added to the chain-strategy parity coverage. The 13 chain-strategy
goldens (including `sdd-vscode-instructions.golden`, which derives from the
`generic` template) are regenerated.

## Architecture Decisions

### Decision: One uniform host-agnostic binding sentence (not per-host variants)

**Choice**: A single sentence, identical byte-for-byte across all 11 templates,
that names the `chained-pr` skill and delegates resolution to "your existing
skill-resolution mechanism."

**Alternatives considered**: Per-host-class variants (delegation hosts vs
solo-inline/platform-native hosts) with mechanism-specific verbs.

**Rationale**: The risk in the proposal is "wording uniformity vs platform
accuracy." Per-host variants reintroduce drift and force per-host assertion
substrings, contradicting the requirement of "one assertion substring common to
all 11." Accuracy is preserved by NOT naming a mechanism in the binding
sentence itself — it says "resolve by registry name through the existing
skill-resolution mechanism." Each template's surrounding skill-resolution
mechanism (Claude Agent/Task `### Sub-Agent Launch Pattern`, cursor named
subagents, kimi custom-agent prompt, kiro phase context, windsurf inline phase
context, antigravity dynamic subagent context) already supplies the
host-accurate verb. The binding inherits that accuracy by reference, not by
restating it.

### Decision: Bind by registry name, defer path to each host's skill-resolution mechanism

**Choice**: Reference `chained-pr` (registry name) and `gentle-ai-chained-pr`
(frontmatter) by name; do not hardcode `skills/chained-pr/SKILL.md`.

**Alternatives considered**: Inject the literal skill path.

**Rationale**: Matches the codebase pattern — the skill-resolution mechanism
resolves skills from the registry and passes the resolved `SKILL.md` path. A
hardcoded path would diverge per install location and break the "pass paths,
not summaries" contract. The spec scenarios assert "references the skill by
registry name (no hardcoded path)."

### Decision: Single common assertion substring

**Choice**: Assert one substring present in all 11 templates AND in generated
goldens: the binding sentence's stable core (see Interfaces).

**Rationale**: A single substring is the only way to keep `assets_test.go`
parity and `inject_test.go` per-host `required` lists in sync without per-host
drift, exactly as the proposal's test-breakage risk demands.

## Data Flow

    Template edit (binding sentence in ### Chain Strategy)
         │
         ▼
    inject/assets pipeline ──► generated orchestrator prompt (per host)
         │                                  │
         ▼                                  ▼
    assets_test.go (source parity)   inject_test.go (generated parity)
         │                                  │
         └──────────► goldens (-update) ◄───┘

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/assets/{antigravity,claude,codex,gemini,generic,kimi,kiro,opencode,qwen,windsurf}/sdd-orchestrator.md` | Modify | Insert the binding sentence after the `chain_strategy` forwarding line in `### Chain Strategy`. |
| `internal/assets/cursor/sdd-orchestrator.md` | Modify | Add the full `### Chain Strategy` section after `### Delivery Strategy` (line 166) and before `### Dependency Graph` (line 168), then append the binding sentence. |
| `internal/assets/assets_test.go` | Modify | Add binding substring to `TestClaudeSDDOrchestratorChainStrategy` and to the parity loop `required`; add `cursor` and `opencode` rows to `TestNonClaudeSDDOrchestratorChainStrategyParity` with their propagation scopes. |
| `internal/components/sdd/inject_test.go` | Modify | Add binding substring to each `required` list in `TestInjectKimiKiroWindsurfAntigravityPreserveNativeChainStrategyWording`. |
| `internal/components/testdata/golden/*.golden` (12 files) | Modify | Regenerate via `-update`. |

## Interfaces / Contracts

Canonical binding sentence (append after the `chain_strategy` forwarding line in
every template's `### Chain Strategy` section):

```markdown
When delivery planning yields chained PRs, treat `chained-pr` (registry skill `gentle-ai-chained-pr`) as a required skill match: resolve it by registry name through this template's existing skill-resolution mechanism (the same one it already uses to pass skills to phases) and ensure the `sdd-tasks` and `sdd-apply` phases load and follow it BEFORE planning or creating any PR. Do not hardcode the skill path; defer resolution to that mechanism.
```

New static-assertion substring (single, common to all 11 + goldens):

```
treat `chained-pr` (registry skill `gentle-ai-chained-pr`) as a required skill match
```

Cursor `### Chain Strategy` section body (mirrors the canonical 10; uses
`prompt` propagation wording like claude/codex/gemini/generic/qwen):

```markdown
### Chain Strategy

When `delivery_strategy` results in chained PRs (either by user choice via `ask-on-risk` or automatically via `auto-chain`), ask the user which chain strategy to use:

- **`stacked-to-main`**: Each PR merges to main in order. Fast iteration, fix on the go. Best for speed-first teams and independent slices.
- **`feature-branch-chain`**: The feature/tracker branch accumulates final integration; PR #1 targets the tracker branch, later child PRs target the immediate previous PR branch so review diffs stay focused. Only the tracker merges to main. Best for rollback control and coordinated releases.

Cache the chain strategy for the session. Pass it as `chain_strategy` to `sdd-tasks` and `sdd-apply` prompts alongside `delivery_strategy`. Do not ask again unless the user changes scope.

<binding sentence>
```

Parity additions to `TestNonClaudeSDDOrchestratorChainStrategyParity`:

```go
{path: "cursor/sdd-orchestrator.md",   propagationScope: "prompt"},
{path: "opencode/sdd-orchestrator.md", propagationScope: "prompt"},
```

(opencode uses "prompts" / "prompt" forwarding — confirm its `chain_strategy`
line contains the `prompt` substring; it does.)

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit (source parity) | All 11 source templates contain the binding substring; cursor + opencode added to parity loop | `assets_test.go` `required` substring assertions |
| Unit (generated parity) | Generated prompts for kimi/kiro/windsurf/antigravity contain the binding | `inject_test.go` `required` lists |
| Regression (forbidden) | Binding does not introduce forbidden persistence/delegation claims | Existing `forbidden` lists stay green (binding names no mechanism) |
| Golden | 12 chain-strategy goldens reflect the new wording + cursor section | `go test ./internal/components/ -run TestSDD -update`, then `go test ./internal/components/ ./internal/assets/...` clean |

## Migration / Rollout

No data migration. Regenerate goldens, then run the full suite:
`go test ./internal/components/ -update` to refresh, then
`go test ./internal/... ` to confirm green. Reviewers confirm the golden diff is
limited to (a) the binding sentence appearing once per chain-strategy golden and
(b) the new cursor `### Chain Strategy` block — no unrelated line churn. The
installed `~/.claude/CLAUDE.md` is regenerated, not hand-edited.

## Open Questions

- [ ] Confirm opencode's `chain_strategy` line literally contains `prompt`
  (sources read use "Pass it as `chain_strategy` ... prompts"); if it uses a
  different scope word, set its parity `propagationScope` accordingly.
- [ ] Cursor's `chain_strategy` forwarding line must contain `prompt` to satisfy
  the parity `propagationScope`; the section body above already does.
