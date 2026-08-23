# Design: Source-Backed SDD Research

## Technical Approach

Add a prompt-routed `sdd-research` phase between exploration and proposal while leaving native status v1 untouched. A dedicated Go contract supplies each runtime's maximum evidence capability; shared assets define evidence, hybrid state, reconciliation, and proposal admission. Orchestrators apply this gate whenever status recommends `propose`.

## Architecture Decisions

| Decision | Choice | Alternative rejected | Rationale |
|---|---|---|---|
| Capability authority | Add sibling package `internal/agents/researchcapability`; do not extend `AgentCapabilityManifest` v1 | Infer from MCP/Bash or mutate generic v1 | Keeps the integration projection immutable and defaults unknown agents, versions, classes, and grants to denial. |
| Runtime grants | Claude: `documentation=[WebFetch]`, `open-web=[WebSearch,WebFetch]`; Kiro: `documentation=[@context7]`; all others: none | Inherited or generic tools | Admission requires every exact declared grant; persistence tools are never evidence grants. |
| Hybrid consistency | Write identical versioned state bytes at revision `n+1` to OpenSpec and Engram; compare both before readiness | Last-write-wins | There is no cross-store transaction, so missing, unequal, or failed writes must block without preferring either copy. |
| Ownership | Orchestrator owns state and product discovery; research owns evidence; proposer only consumes a confirmed handoff | Proposer interviews | Prevents parallel state races and implicit product consent in automatic mode. |
| Parity | Insert one canonical research-lifecycle asset into all 12 renderer sources and derive tests from `catalog.AllAgents()` | Copy untested prose | Preserves 16-AgentID parity and existing bounded-review clauses. |

## Data Flow

    intake → persist intent/revision → exact-grant admission
          ├→ sdd-explore ─┐
          └→ sdd-research ┴→ validate artifacts → reconcile stores
             → confirm product decisions → proposal gate → sdd-propose

Denial, partial/blocked research, unresolved decisions, invalid references, or store mismatch persists the current intent and stops before proposal. Automatic mode emits one lossless grouped decision prompt and blocks.

## File Changes

| Files | Action | Purpose |
|---|---|---|
| `internal/agents/researchcapability/{contract.go,contract_test.go}` | Create | Closed schema, classes, grant sets, validation, and per-AgentID declarations. |
| `internal/assets/skills/sdd-research/SKILL.md`, `internal/assets/skills/_shared/{research-lifecycle,persistence-contract,engram-convention,openspec-convention,sdd-status-contract}.md` | Create/Modify | Phase, artifact/state schemas, hybrid recovery, and status-v1 gate guidance. |
| `internal/assets/{claude,kiro,cursor,kimi}/agents/sdd-research.*`, `internal/assets/{claude,opencode}/commands/sdd-research.md`, `internal/assets/opencode/sdd-overlay-{single,multi}.json` | Create/Modify | Executors, commands, explicit grants, task permission, and default-deny assets. |
| `internal/assets/{antigravity,claude,codex,cursor,gemini,generic,hermes,kimi,kiro,opencode,qwen,windsurf}/sdd-orchestrator*.md`, `internal/components/sdd/{boundedreview.go,inject.go}` | Modify | Render the canonical optional lane and pre-proposal gate. |
| `internal/assets/skills/sdd-propose/SKILL.md`, `internal/assets/{claude,cursor,kiro,kimi}/agents/sdd-propose.*` | Modify | Remove interviews; require the confirmed handoff. |
| `internal/model/{types,claude_model,kiro_model,codex_model}.go`, `internal/catalog/skills.go`, `internal/opencode/models.go`, `internal/components/sdd/{commands,profiles,prompts}.go`, `internal/tui/screens/{model_picker,claude_model_picker,kiro_model_picker,codex_model_picker}.go`, `internal/agents/kimi/adapter.go`, `internal/components/uninstall/service.go` | Modify | Add phase/catalog/model/profile/injection/picker/entrypoint/uninstall inventory after `sdd-explore`; profile size becomes 12 agents (36 for three profiles). |
| `internal/assets/windsurf/workflows/sdd-new.md` | Replace | Use OpenSpec/Engram ordering and research/decision gates instead of `.sdd/` proposal/spec creation. |
| `internal/components/sdd/{inject,profiles,prompts,commands,bounded_review_contract}_test.go`, `internal/model/*_model_test.go`, `internal/tui/screens/*model_picker_test.go`, `internal/components/uninstall/service_test.go`, `internal/assets/assets_test.go`, `internal/components/golden_test.go` | Modify/Create | Contract, inventory, parity, ordering, and golden coverage. |

## Interfaces / Contracts

`gentle-ai.sdd-research-capability/v1` admits only `documentation` and `open-web`. `research.md` / `sdd/{change}/research` uses `gentle-ai.sdd-research/v1` with revision, outcome, questions, admission and observed grants, sources (`id,class,title,publisher,url,accessed_at,excerpt`), claims with source IDs, contradictions, uncertainty/freshness, and non-authoritative product choices. Partial/blocked artifacts exclude unvalidated claims.

`gentle-ai.sdd-preproposal/v1` stores revision; exploration outcome/ref; research requested/classes/admission/outcome/OpenSpec+Engram refs; product decisions (`pending|confirmed`); and `proposal_ready`. The gate requires byte-equal stores and evidence, selected research `done`, confirmed decisions, and valid references. No-request skips only research. Handoff carries the state revision, confirmed decisions, and optional evidence refs.

## Testing Strategy

Strict TDD starts with focused RED tests: capability/default-deny and documentation-like adversaries; asset grant parsing; evidence/state mismatch and gate cases; proposer non-interviewing; 12-template/16-AgentID parity; Windsurf order; all phase/model/profile/catalog/injection/uninstall inventories; assignment round trips and goldens. Add `internal/sddstatus/{status_test,status_v1_test}.go` regressions proving missing proposal still routes `propose` and v1 rejects research fields/tokens. Run focused package tests, then `go test ./...` and `go vet ./...`.

No bench run is planned because native binary routing is unchanged. If implementation changes it, reproduce CI's locally built `gentle-ai-bench run --binary ...`; `go test ./bench` is not execution proof.

## Threat Matrix

| Boundary | Applicability | Safe/failure behavior | Planned RED tests |
|---|---|---|---|
| Documentation-like paths | Applicable | Never infer class or execute by suffix; `requirements.txt`, `CMakeLists.txt`, executable MD/MDX, and `README.sh` as grant evidence deny with no claims. | Table case for each listed class. |
| Git repository selection | N/A — no Git command/cwd selection | — | — |
| Commit state | N/A — no commit automation | — | — |
| Push state | N/A — no push automation | — | — |
| PR commands | N/A — no PR automation | — | — |

## Migration / Rollout

Ship declarations/default denial, then assets and inventories atomically. Existing proposals continue; proposal-less changes initialize pre-proposal v1 with research unselected and decisions pending unless already confirmed. Rollback removes registrations/assets and ignores additive research state/artifacts; frozen status v1 remains operable.

## Open Questions

None.
