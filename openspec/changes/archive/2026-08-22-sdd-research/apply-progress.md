# Apply Progress: Source-Backed SDD Research

## Status

- Mode: Strict TDD
- Delivery: feature-branch-chain. Work Unit 1 (`wu1-contract-state`) is intended child PR #1 to the tracker branch; Work Unit 2 (`wu2-phase-assets-grants`) is intended child PR #2 with the immediate Work Unit 1 PR branch as its base; Work Unit 3 (`wu3-orchestration-inventory`) is intended child PR #3 with the immediate Work Unit 2 PR branch as its base; Work Unit 4 (`wu4-ordering-compatibility`) is intended child PR #4 with the immediate Work Unit 3 PR branch as its base.
- Progress: 15/15 tasks complete. Apply is complete and ready for independent `sdd-verify`.

## Completed Tasks

- [x] 1.1 Fail-closed capability adversary/admission tests
- [x] 1.2 Evidence readiness and hybrid mismatch/restart tests
- [x] 1.3 Closed v1 capability contract with exact Claude/Kiro grants
- [x] 1.4 Shared research/pre-proposal evidence and persistence contracts
- [x] 2.1 Runtime asset and OpenCode task-permission grant tests
- [x] 2.2 Research skill, executor, command, overlay, and recovery assets
- [x] 3.1 Proposal gate contract tests
- [x] 3.2 Phase inventory, profile, model, TUI, uninstall, and renderer parity tests
- [x] 3.3 Canonical research lifecycle gate across all orchestrator families
- [x] 3.4 Research phase registration and confirmed proposer handoff
- [x] 3.5 Deduplicated lifecycle rendering and preserved-prompt migration
- [x] 4.1 Windsurf pre-proposal lifecycle ordering and legacy-artifact RED tests
- [x] 4.2 Status-v1 compatibility and focused golden RED coverage
- [x] 4.3 Windsurf workflow replacement with frozen native status-v1 production behavior
- [x] 4.4 Focused, golden, uncached module, vet, diff, scope, and bench-disposition verification

## TDD Cycle Evidence

| Task | Test file | Layer | Safety net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| 1.1 | `internal/agents/researchcapability/contract_test.go` | Unit | N/A (new package) | `go test ... -run 'TestAdmit\|TestDocumentation' -count=1` failed to compile on undefined contract symbols | Capability package passed | Supported grants plus closed denial/adversary cases | `gofmt`; focused tests passed |
| 1.2 | `internal/components/sdd/research_state_contract_test.go` | Asset contract | `internal/components/sdd` passed | Focused tests failed on missing lifecycle and readiness/restart clauses | Three focused contract tests passed | Eight readiness rows and five shared assets | `gofmt`; focused tests passed |
| 1.3 | `internal/agents/researchcapability/contract_test.go` | Unit | N/A (new file) | 1.1 tests preceded production | Package passed after minimum closed declaration/admission implementation | Exact grants, 16 AgentIDs, unknowns, and extra grants | Defensive copies and grant comparison; tests passed |
| 1.4 | `internal/components/sdd/research_state_contract_test.go` | Asset contract | `internal/components/sdd` passed | 1.2 tests preceded asset edits | Contract tests passed after five shared assets were defined | Complete/partial/blocked and hybrid divergence/restart paths | Concise shared contract sections; tests passed |
| 2.1 | `internal/assets/assets_test.go`, `internal/components/sdd/commands_test.go` | Embedded asset contract | `go test ./internal/assets ./internal/components/sdd -count=1` passed | Focused tests failed on missing research assets, command, and overlay permission | Both focused packages passed after 2.2 assets | Claude/Kiro exact grants, Cursor/Kimi/OpenCode denial, persistence exclusion, and both overlays | Table-driven grant cases and JSON permission assertions; `gofmt` passed |
| 2.2 | Same Work Unit 2 contract tests | Embedded runtime asset | Existing package safety net passed | 2.1 RED preceded all production assets | Focused packages and full uncached module passed | Completion, blocked recovery, hybrid mismatch, unknown-class denial, and single/multi overlay paths | Concise style-guide skill; affected existing golden regenerated and passed without `-update` |
| 3.1 | `internal/components/sdd/{commands,profiles,prompts,bounded_review_contract}_test.go`, `internal/assets/language_contract_test.go` | Rendered contract | Eight affected packages passed before edits | Focused run failed on undefined `researchLifecycleContract`, missing gate text, and retained proposer interviews | Canonical gate and confirmed-handoff assets passed | Selected done/unselected, confirmed decisions, valid references, hybrid match, and automatic unresolved-choice blocking | Gate wording reduced to one canonical rendered fragment; focused tests passed |
| 3.2 | Model, catalog, TUI, Kimi, uninstall, and profile tests | Inventory/round trip | Eight affected packages passed before edits | Focused run failed on 10-phase inventories, 11-agent profiles, missing research model rows, TUI rows, Kimi entrypoint, and uninstall cleanup | All inventory packages passed with 11 phases and 12 agents per profile | Three profile overlays yield 36 base profile agents; 12 source families and all 16 catalog AgentIDs render parity; old fan-out remains rejected | Inventories derive from shared phase lists where existing architecture permits; focused tests passed |
| 3.3 | `bounded_review_contract_test.go`, `inject_test.go` | Renderer/injection | Existing 16-AgentID bounded-review parity passed | Canonical lifecycle count and proposal-gate clauses failed before renderer support | All orchestrator render and injection paths include exactly one gate | Preserved prompts, Claude lazy workflow, OpenCode/Kilocode, generic-family agents, and native family assets | Shared fragment extraction preserves bounded-review wording and status-v1 ownership |
| 3.4 | Model/picker/profile/catalog/uninstall/proposer tests | Inventory/assets | Existing package suite passed | New phase/model/profile/proposer expectations failed before registration | Focused package suite and full uncached module passed | Claude, Kiro, Codex, OpenCode, TUI, Kimi, profile, skill catalog, and uninstall paths covered | Proposer interview blocks removed from the shared skill and four runtime agents |
| 3.5 | `bounded_review_contract_test.go`, preserved-prompt idempotence tests | Approval/refactor | Existing renderer and migration tests passed | Exact-one canonical fragment and source-placeholder parity failed before refactor | Canonical placeholder renderer and idempotent preserved-prompt migration passed | Every source family references one placeholder; every catalog agent renders one resolved fragment | Lifecycle injection now has one extraction/substitution path instead of divergent prose |
| 4.1 | `internal/assets/windsurf_workflow_test.go` | Embedded asset contract | `go test ./internal/assets ./internal/components ./internal/sddstatus -count=1` passed before edits | Focused run failed on all five missing lifecycle headings and legacy artifact paths | `go test ./internal/assets -run 'TestWindsurfSDDNewWorkflow' -count=1` passed after workflow replacement | Ordered lifecycle assertions plus independent legacy path rejection | Workflow reduced from 218 to 69 lines; focused tests stayed green |
| 4.2 | `internal/sddstatus/{status_test,status_v1_test}.go`, `internal/components/golden_test.go` | Status projection/golden | Same three-package safety net passed | New Claude/OpenCode/Kiro golden assertions failed on five absent approved snapshots; status compatibility assertions passed first-run as coverage-gap closure | Focused status tests and approved golden update passed | Missing proposal routing, research artifact omission, research token rejection, cross-runtime skill parity, and exact Claude/Kiro grant snapshots | Table-driven projection rejection retained; golden rerun passed without `-update` |
| 4.3 | `internal/assets/windsurf/workflows/sdd-new.md` and approved goldens | Embedded workflow | 4.1 RED captured old behavior | Minimum workflow replacement made ordering and legacy-path tests pass; no native status production file changed | OpenSpec/Engram, local explore, optional admitted research, confirmed decisions, proposal, and spec order are all asserted | Concise English workflow and mechanical snapshot only |
| 4.4 | Work Unit 4 focused and full verification | Verification | Focused packages green before final verification | RED evidence is retained in 4.1/4.2; this verification task adds no production behavior | Focused packages, full uncached module, vet, and diff checks all passed | Runtime/bench disposition was inspected independently from unit declarations | `gofmt` applied; no additional refactor was needed |

## Work Unit Evidence

### Work Unit 1 — Contract/state

| Evidence | Result |
|---|---|
| Focused test | `go test ./internal/agents/researchcapability ./internal/components/sdd -count=1` — exit 0; both packages `ok` |
| Full uncached module test | `go test ./... -count=1` — exit 0; all packages passed |
| Changed-line claim | Work Unit 1 implementation paths total `+369/-0`: tracked `git diff --numstat` measured `+27/-0`, plus line counts for four untracked implementation files measured `+342/-0`. Included paths: `internal/agents/researchcapability/contract.go`, `internal/agents/researchcapability/contract_test.go`, `internal/components/sdd/research_state_contract_test.go`, and five `_shared` research contract documents: `internal/assets/skills/_shared/research-lifecycle.md`, `internal/assets/skills/_shared/engram-convention.md`, `internal/assets/skills/_shared/openspec-convention.md`, `internal/assets/skills/_shared/persistence-contract.md`, and `internal/assets/skills/_shared/sdd-status-contract.md`. Seven pre-existing untracked OpenSpec planning artifacts and unrelated pre-existing worktree changes were excluded from this implementation-path count. |
| Runtime harness | N/A — pure Go admission and embedded contract/state assets add no runtime boundary |
| Rollback boundary | Remove `internal/agents/researchcapability`, `research_state_contract_test.go`, and `research-lifecycle.md`; revert only the research sections in the other four shared assets and the Work Unit 1 progress checkboxes |

### Work Unit 2 — Phase/assets/grants

| Evidence | Result |
|---|---|
| Focused test | `go test ./internal/assets ./internal/components/sdd -count=1` — exit 0; both packages `ok` |
| Affected golden | `go test ./internal/components -run TestGoldenSDD_OpenCode_Multi -count=1` — exit 0 after mechanical `-update` regeneration |
| Full uncached module test | `go test ./... -count=1` — exit 0; all packages passed |
| Static verification | `go vet ./...` — exit 0; `git diff --check` — exit 0 |
| Changed-line claim | Work Unit 2 implementation paths total `+352/-24` = `376` changed lines. Scope: tracked `git diff --numstat` measured `+198/-24`, including the affected generated OpenCode multi golden (`+16/-0`); eight new runtime/skill/command files measured `+154/-0` with `wc -l`. Pre-existing Work Unit 1 paths, OpenSpec planning artifacts, Engram state, and unrelated worktree changes were excluded. |
| Runtime harness | N/A — asset contract tests prove exact static grants and explicit default denial; no live web/provider call belongs in this unit. |
| Child PR #2 boundary | Starts from the immediate Work Unit 1 PR branch; contains only task 2.1 tests and task 2.2 phase assets/grants plus their mechanically affected test expectations/golden. It does not include Phase 3 inventories, orchestration, proposer, model/profile/catalog/TUI/uninstall work, or status-v1 changes. |
| Rollback boundary | Remove `sdd-research` skill, Claude/Kiro/Cursor/Kimi executor files, Claude/OpenCode command files, and Kimi YAML; revert only `sdd-research` entries in both OpenCode overlays plus Work Unit 2 test/count/hash/golden updates and task/progress checkboxes. Work Unit 1 contracts and shared lifecycle assets remain intact. |

### Work Unit 3 — Orchestration/inventory

| Evidence | Result |
|---|---|
| Focused test | `go test ./internal/components/sdd ./internal/model ./internal/catalog ./internal/tui/screens ./internal/opencode ./internal/components/uninstall ./internal/agents/kimi ./internal/assets -count=1` — exit 0; all eight packages `ok` |
| Affected golden path | `go test ./internal/components ./internal/tui -update -count=1` — exit 0; only mechanically affected SDD/skills/TUI goldens changed; rerun without `-update` exited 0 |
| Full uncached module test | `go test ./... -count=1` — exit 0; all packages passed. An earlier 120-second invocation timed out as infrastructure, and the required 300-second rerun completed successfully. |
| Static verification | `go vet ./...` — exit 0; `git diff --check` — exit 0 |
| Changed-line claim | Authored Work Unit 3 paths total `+229/-217` = `446` changed lines, within the 450-line authored review budget. Reproduction from this cumulative dirty worktree: current authored path scope is `+377/-241` versus HEAD; subtract the recorded pre-WU3 overlap (`+150/-24` across `assets_test.go`, `language_contract_test.go`, `commands_test.go`, `inject_test.go`, `profiles_test.go`, and `review_ledger_contract_test.go`) to get tracked WU3 `+227/-217`, then add the shared untracked lifecycle delta from 22 to 24 lines (`+2/-0`). Per `work-unit-commits`, deterministic generated goldens are reported separately: WU3 `+25/-40` = `65`; complete snapshot identity is `+254/-257` = `511`. OpenSpec planning/progress files, Work Units 1–2, Engram state, and unrelated worktree changes are excluded. |
| Delivery exception | `size:exception: approved` — scoped only to Work Unit 3 / feature-branch-chain child PR #3. The maintainer accepted native accounting of `461` changed lines and authorized the forward-only reset at revision `sha256:5fe8520b946d887f63a1d5dd7eb2b7be7eba3b98a3c1feeff0b041485e14c37d`. Work Unit 4 remains ordinary feature-branch-chain child PR #4 with authored scope `342` and no size exception. |
| Runtime harness | N/A — the runtime boundary is static prompt rendering, profile generation, direct TUI state simulation, and uninstall planning; focused tests exercise those real in-process paths without an external provider/runtime. Native binary routing is unchanged, so no bench journey applies. |
| Child PR #3 boundary | Starts from the immediate Work Unit 2 PR branch. Contains only tasks 3.1–3.5: gate/parity tests, canonical orchestration fragment, research inventories/model/profile/TUI/uninstall wiring, proposer confirmed-handoff changes, and mechanically affected goldens. It excludes Windsurf `workflows/sdd-new.md`, status-v1 code/tests, Phase 4 compatibility assertions, and native routing. |
| Rollback boundary | Revert the Work Unit 3 phase constants/lists and picker/profile/uninstall/Kimi entries; revert the canonical placeholder renderer and the 12 family placeholders; restore the shared proposer and four proposal-agent interview blocks; revert only Work Unit 3 tests, hash, and generated golden deltas. Work Units 1–2 capability/state/runtime assets and exact grants remain intact. |

### Work Unit 4 — Ordering/compatibility

| Evidence | Result |
|---|---|
| Focused RED | `go test ./internal/assets -run 'TestWindsurfSDDNewWorkflow' -count=1` — exit 1: missing all lifecycle headings and found all three legacy artifact paths. `go test ./internal/components -run 'TestGoldenSDD_(Claude|OpenCode|Kiro)$' -count=1` — exit 1 on absent research snapshots. Status compatibility tests passed first-run as intentional coverage-gap closure for frozen behavior. |
| Focused GREEN | `go test ./internal/assets ./internal/components/sdd ./internal/components ./internal/sddstatus -count=1` — exit 0; all four packages `ok` |
| Approved golden path | `go test ./internal/components -run 'TestGoldenSDD_(Claude|OpenCode|Kiro|Windsurf)$' -update -count=1` — exit 0; inspected the five new research snapshots and changed Windsurf workflow snapshot. Non-mutating `go test ./internal/components -run '^TestGoldenSDD_' -count=1` — exit 0. |
| Full uncached module test | `go test ./... -count=1` — exit 0; all root-module packages passed |
| Static verification | `go vet ./...` — exit 0 with no output; `git diff --check` — exit 0 with no output |
| Changed-line claim | Authored Work Unit 4 scope is `+142/-200` = `342` changed lines, below the 400-line hard ceiling: workflow `+39/-188`, golden assertions `+16/-12`, status tests `+33/-0`, and new workflow tests `+54/-0`. Deterministic generated goldens are reported separately at `+151/-188` = `339`; complete snapshot identity is `+293/-388` = `681`. Reproduce tracked counts with `git diff --numstat` over the named Work Unit 4 paths and add line counts for the six new untracked files. OpenSpec progress/tasks and Engram state are excluded from the child PR review count. |
| Candidate fingerprint | Work Unit 4 implementation and generated snapshot scope hashes to `sha256:69fec4e1e526da32831780f2ac6494c8cb1be9fccc9ac314604ad8aceca4f2dc`, distinct from prior evidence revision `sha256:ca5a0f2ea7e6a40a39d32915110b9a0266f6db20d9081a9e02f38813dc8adde3`. |
| Runtime harness | N/A — Work Unit 4 changes static Windsurf workflow content, compatibility tests, and deterministic snapshots only. No native binary routing or bench journey changed. Corpus inspection found no stale `.sdd` proposal/spec creation pin; the only proposal references are current OpenSpec fixture setup. Therefore a driven bench run is not applicable, and `go test ./bench` was not misreported as execution proof. |
| Child PR #4 boundary | Starts from the immediate Work Unit 3 PR branch. Contains only tasks 4.1–4.4: Windsurf workflow ordering/tests, frozen status-v1 compatibility tests, focused research asset/grant/parity golden assertions, and their approved deterministic snapshots. It excludes native status-v1 production changes, binary routing, bench journeys, review, verification reports, and archive work. |
| Rollback boundary | Revert `internal/assets/windsurf/workflows/sdd-new.md`, remove `internal/assets/windsurf_workflow_test.go`, revert only the Work Unit 4 additions in `internal/sddstatus/{status_test,status_v1_test}.go` and `internal/components/golden_test.go`, remove the five new research goldens, restore only `testdata/golden/sdd-windsurf-workflow-sdd-new.golden`, and revert the Work Unit 4 task/progress checkboxes. Work Units 1–3 remain intact. |

## Deviations and Issues

- Deviations: None — Work Unit 4 matches the design. Native status-v1 production shape, tokens, and routing remain unchanged. Generated goldens were changed only through the approved `-update` path and then rerun without mutation.
- Issues: Work Unit 3's recorded timeout remains historical. Work Unit 4 found no new issue; all final checks pass.
- Remaining: No apply tasks remain. The change is ready for independent `sdd-verify`, not native review or archive.

## Post-Review Verification Remediation — Generated Baselines

- Work unit: `post-review-generated-baselines`
- Attempt token held by orchestrator: `sha256:16b5a2c61e017fa021d591b277c6feb1a630b9edf8ec16804eaf780d87f0b5b7`
- Failed evidence revision remediated: `sha256:0dbb6b018dc2c452c6ff5fb5edc623c134f3d63c8326ad4ac08764195d96f62c`
- Passing evidence revision: `sha256:c64b9680e4426dcf053171e2e60d9167e2e5c1580fb6f00c4323fecac384b379`
- Result: PASS. The 14 deterministic golden snapshots were refreshed through the approved `-update` path and passed without `-update`; the Kilocode settings hash now matches mechanically rendered bytes; the only permitted Go formatting change was applied; and the corrected 13-scenario spec was synchronized to Engram.
- Authority boundary: this remediation changes generated/test baselines and hybrid SDD artifacts only. It does not reopen or mutate native review authority.

### Remediation TDD Cycle Evidence

| Task | Test file | Layer | Safety net / RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|
| Refresh deterministic aftermath | `internal/components/golden_test.go`, `internal/components/sdd/review_ledger_contract_test.go`, `internal/assets/assets_test.go` | Golden/contract | Fresh non-mutating runs failed 12 SDD golden tests, 2 combined golden tests, and the Kilocode hash; `gofmt -l` listed `internal/assets/assets_test.go` | Approved golden update exited 0; non-mutating golden, Kilocode, scenario-matrix, and full uncached tests exited 0 | 14 snapshots cover the shared correction across supported renderer families; all 13 corrected scenario mappings remained green | `gofmt` changed only `internal/assets/assets_test.go`; formatter check-only passed |

### Remediation Work Unit Evidence

| Evidence | Result |
|---|---|
| Focused RED | `go test ./internal/components -run '^TestGoldenSDD_' -count=1` — exit 1 with 12 stale SDD snapshots; `go test ./internal/components -run '^(TestGoldenCombined_Claude|TestGoldenCombined_Windsurf)$' -count=1` — exit 1 with 2 stale combined snapshots; `go test ./internal/components/sdd -run '^TestKilocodeReviewSettingsMatchCurrentMainBaseline$' -count=1` — exit 1 with rendered `c834b94f...` versus stale `31219e94...`; `gofmt -l internal/assets/assets_test.go` listed the file |
| Approved golden update | `go test ./internal/components -run '^(TestGoldenSDD_.*|TestGoldenCombined_(Claude|Windsurf))$' -update -count=1` — exit 0 |
| Focused GREEN | The same golden command without `-update` — exit 0; Kilocode baseline test — exit 0; all 13 scenario mappings across `researchcapability`, `assets`, `components/sdd`, and `sddstatus` — exit 0 |
| Full uncached module test | `go test ./... -count=1` — exit 0; all root-module packages passed |
| Static verification | `go vet ./...` — exit 0; formatter check-only — exit 0; `git diff --check` — exit 0 |
| Changed-line claim | Authored remediation is `+2/-2`: one formatting-only line replacement and one mechanically derived hash replacement. Deterministic generated snapshots are `+15/-15` across 14 files. Complete code/snapshot identity is `+17/-17` across 16 files. OpenSpec/Engram progress and the Engram spec synchronization are SDD artifacts reported separately. |
| Runtime harness | N/A — the remediation changes deterministic snapshots, a test-only hash baseline, formatting, and hybrid SDD artifacts; no production behavior, native routing, provider call, or bench journey changed |
| Rollback boundary | Restore the 14 stale golden bytes, the prior Kilocode hash constant, and the pre-gofmt alignment; remove only this remediation section and restore the prior Engram spec revision. No production semantic file is part of the rollback. |
| Bench disposition | Not applicable. No `bench/`, CI invocation, native route, or product semantic changed; no `go test ./bench` execution claim is made. |

### Remediation Files

- `internal/assets/assets_test.go`
- `internal/components/sdd/review_ledger_contract_test.go`
- `testdata/golden/combined-claude-claudemd.golden`
- `testdata/golden/combined-windsurf-global-rules.golden`
- `testdata/golden/sdd-antigravity-rulesmd.golden`
- `testdata/golden/sdd-claude-claudemd.golden`
- `testdata/golden/sdd-codex-agentsmd.golden`
- `testdata/golden/sdd-codex-agentsmd-lowcost.golden`
- `testdata/golden/sdd-codex-agentsmd-powerful.golden`
- `testdata/golden/sdd-cursor-rules.golden`
- `testdata/golden/sdd-gemini-geminimd.golden`
- `testdata/golden/sdd-kiro-instructions.golden`
- `testdata/golden/sdd-opencode-multi-settings.golden`
- `testdata/golden/sdd-opencode-skill-sdd-research.golden`
- `testdata/golden/sdd-vscode-instructions.golden`
- `testdata/golden/sdd-windsurf-global-rules.golden`
- `openspec/changes/sdd-research/apply-progress.md`
- Engram topics `sdd/sdd-research/spec` and `sdd/sdd-research/apply-progress`
