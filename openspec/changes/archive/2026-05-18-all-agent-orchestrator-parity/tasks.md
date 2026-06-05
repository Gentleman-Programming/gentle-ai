# Tasks: All-Agent SDD Orchestrator Parity

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated changed lines | 520-820 |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | single PR with maintainer-approved `size:exception` |
| Delivery strategy | exception-ok |
| Chain strategy | size-exception |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: size-exception
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|---|---|---|---|
| 1 | Update delegate-capable non-Claude assets (codex/gemini/qwen/generic) with canonical Chain Strategy + `chain_strategy` propagation | PR 1 | Base main; includes focused static assertions for these assets |
| 2 | Update platform-native family (kimi/kiro/windsurf/antigravity) preserving inline/native semantics | PR 2 | Base PR1; include wording guard assertions to prevent OpenCode persistence claims |
| 3 | Refresh impacted injection/golden coverage and run targeted + broad verification | PR 3 | Base PR2; only direct semantic golden churn |

## Phase 1: Foundation and Guardrails

- [x] 1.1 Confirm no runtime/template refactor scope in `openspec/changes/all-agent-orchestrator-parity/{proposal.md,design.md}` and capture file-level implementation boundaries.
- [x] 1.2 Add/extend table-driven static assertion matrix in `internal/assets/assets_test.go` for canonical `stacked-to-main`, `feature-branch-chain`, and `chain_strategy` propagation expectations.
- [x] 1.3 Add forbidden-phrase assertions in `internal/assets/assets_test.go` for Windsurf/Antigravity/Kimi/Kiro inaccurate OpenCode persistence or subagent guarantees.

## Phase 2: Delegate-Capable Platform Family Assets

- [x] 2.1 Update `internal/assets/codex/sdd-orchestrator.md` with canonical `### Chain Strategy` section and explicit forwarding of `delivery_strategy` + `chain_strategy` to `sdd-tasks` and `sdd-apply`.
- [x] 2.2 Apply the same canonical strategy/propagation updates to `internal/assets/gemini/sdd-orchestrator.md` while preserving Gemini-native orchestration wording.
- [x] 2.3 Apply the same updates to `internal/assets/qwen/sdd-orchestrator.md` while preserving Qwen-native phrasing and flow.
- [x] 2.4 Update `internal/assets/generic/sdd-orchestrator.md` similarly, preserving generic host/model guidance blocks.

## Phase 3: Platform-Native / Solo-Inline Family Assets

- [x] 3.1 Update `internal/assets/kimi/sdd-orchestrator.md` to include canonical strategies and pass both strategy fields using Kimi-native `/skill:sdd-*` + `multiagent:Task` wording.
- [x] 3.2 Update `internal/assets/kiro/sdd-orchestrator.md` to include canonical strategies with Kiro phase-context/subagent-context wording and existing approval semantics.
- [x] 3.3 Update `internal/assets/windsurf/sdd-orchestrator.md` to include canonical strategies and inline phase-context propagation (no custom subagent/OpenCode persistence claims).
- [x] 3.4 Update `internal/assets/antigravity/sdd-orchestrator.md` with the same solo-inline constraints as Windsurf.

## Phase 4: Injection and Golden Verification

- [x] 4.1 Extend focused wording checks in `internal/components/sdd/inject_test.go` for Kimi/Kiro/Windsurf/Antigravity (and any directly regressed non-Claude host) to enforce platform-native semantics.
- [x] 4.2 Run static and injection targets: `go test ./internal/assets -run 'Test.*SDDOrchestrator.*'` and `go test ./internal/components/sdd -run 'TestInject(Kimi|Qwen|Gemini|OpenClaw|.*Windsurf|.*Antigravity|.*Kiro)'`; fix failures before golden updates.
- [x] 4.3 Update only directly impacted fixtures in `testdata/golden/sdd-*.golden` via targeted `go test ./internal/components -run 'TestGoldenSDD_(Codex|Gemini|Windsurf|Kiro|Antigravity|Cursor|VSCode)' -update`, inspect semantic diff, rerun without `-update`.
- [x] 4.4 Run regression safety suite: `go test ./...` then `go vet ./...`.

## Phase 5: Final Scope and Delivery Check

- [x] 5.1 Verify changed files stay within orchestrator assets/tests/goldens listed in design (`internal/assets/*/sdd-orchestrator.md`, `internal/assets/assets_test.go`, `internal/components/sdd/inject_test.go`, optional `internal/components/golden_test.go`, `testdata/golden/*`).
- [x] 5.2 If total diff remains >400 lines, document maintainer-approved `size:exception` for the single-PR delivery path.
