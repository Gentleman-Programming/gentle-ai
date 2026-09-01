# Tasks: Source-Backed SDD Research

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated changed lines | 1,200–1,800 additions+deletions; above 800 |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | Four units; feature-branch-chain selected |
| Delivery strategy | ask-on-risk |
| Chain strategy | feature-branch-chain |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: High

### Suggested Work Units

| Unit | Goal (tests/docs) | PR | Focused test | Runtime harness | Rollback |
|---|---|---|---|---|---|
| 1 | Contract/state | 1 | `go test ./internal/agents/researchcapability ./internal/components/sdd` | N/A: pure | Remove schemas |
| 2 | Phase/assets/grants | 2 | `go test ./internal/assets ./internal/components/sdd` | N/A: no live web | Remove research assets |
| 3 | Gate/inventory/parity | 3 | `go test ./internal/components/sdd ./internal/model ./internal/catalog ./internal/tui/screens ./internal/opencode` | N/A: renderer/TUI simulation | Revert gate/inventory |
| 4 | Ordering/status/goldens | 4 | `go test ./internal/sddstatus ./internal/components/sdd && go vet ./...` | N/A: native route; bench conditional | Revert workflow/tests |

## Phase 1: Contracts and Hybrid State

- [x] 1.1 RED — `internal/agents/researchcapability/contract_test.go`: deny `requirements.txt`, `CMakeLists.txt`, executable MD/MDX, `README.sh`; emit no claims; deny unknown/missing v1/class, Bash, generic MCP, unnamed inheritance.
- [x] 1.2 RED — `internal/components/sdd/*_test.go`: test complete evidence, partial/blocked outcomes, missing/failing writes, divergent revisions/bytes, matching restart; only matching `done` is ready.
- [x] 1.3 GREEN — Create `internal/agents/researchcapability/contract.go`: closed v1, `documentation`/`open-web`, exact Claude/Kiro grants, default-deny all AgentIDs.
- [x] 1.4 GREEN/REFACTOR — Define `gentle-ai.sdd-research/v1` and `gentle-ai.sdd-preproposal/v1` in `internal/assets/skills/_shared/{research-lifecycle,persistence-contract,engram-convention,openspec-convention,sdd-status-contract}.md`: sources, mapped claims, contradictions, uncertainty/freshness, separate choices, revisioned byte-equal reconciliation, gate; `gofmt`.

## Phase 2: Research Runtime Assets

- [x] 2.1 RED — `internal/assets/assets_test.go` and `internal/components/sdd/commands_test.go`: Claude documentation=`WebFetch`, open-web=`WebSearch,WebFetch`; Kiro documentation=`@context7`; all others none; exclude persistence grants; require task permission.
- [x] 2.2 GREEN/REFACTOR — Add `internal/assets/skills/sdd-research/SKILL.md`, `internal/assets/{claude,kiro,cursor,kimi}/agents/sdd-research.*`, `{claude,opencode}/commands/sdd-research.md`, `opencode/sdd-overlay-{single,multi}.json`; document completion/recovery.

## Phase 3: Orchestration, Proposer, and Inventories

- [x] 3.1 RED — Gate tests in `internal/components/sdd/{commands,profiles,prompts,bounded_review_contract}_test.go`: selected-done/unselected handoff, confirmed decisions/refs, one lossless automatic prompt that persists and blocks.
- [x] 3.2 RED — Inventory/parity tests: phase after `sdd-explore`, 12 agents/36 profiles, all 12 templates and 16 `catalog.AllAgents()` IDs, bounded-review clauses/no old fan-out; cover `internal/model/*_model_test.go`, `internal/tui/screens/*model_picker_test.go`, `internal/components/uninstall/service_test.go`.
- [x] 3.3 GREEN — Update 12 `internal/assets/{antigravity,claude,codex,cursor,gemini,generic,hermes,kimi,kiro,opencode,qwen,windsurf}/sdd-orchestrator*.md`, `internal/components/sdd/{boundedreview,inject}.go`; gate every `propose`, keep research after exploration, and preserve runtime ownership.
- [x] 3.4 GREEN — Update `internal/model/{types,claude_model,kiro_model,codex_model}.go`, `internal/catalog/skills.go`, `internal/opencode/models.go`, `internal/components/sdd/{commands,profiles,prompts}.go`, TUI pickers, `internal/agents/kimi/adapter.go`, uninstall service; remove proposer interviews from skill/four agents.
- [x] 3.5 REFACTOR — Deduplicate lifecycle injection.

## Phase 4: Ordering, Compatibility, and Verification

- [x] 4.1 RED — Test `internal/assets/windsurf/workflows/sdd-new.md`: OpenSpec/Engram precedes research/decisions/proposal; no `.sdd/` creation.
- [x] 4.2 RED — Add `internal/sddstatus/{status_test,status_v1_test}.go` (missing proposal still recommends `propose`; v1 rejects research fields/tokens) and golden assertions in `internal/components/golden_test.go` for assets, grants, and parity.
- [x] 4.3 GREEN — Replace Windsurf ordering, leave native status-v1 unchanged, update only approved goldens.
- [x] 4.4 REFACTOR/VERIFY — Run focused tests, `go test ./...`, `go vet ./...`; if binary routing changes, reproduce CI’s `gentle-ai-bench run --binary ...`. `go test ./bench` is not execution proof.
