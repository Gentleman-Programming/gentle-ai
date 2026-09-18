# OpenCode RDD Provider-Owned Lens Tasks

## Objective

Remove model-authored `GENTLE_AI_REVIEW_BINDING` JSON assembly from the OpenCode V1 RDD reviewer path. Native Go must issue the exact opaque lens task that OpenCode executes.

## Problem and why

`review.capture-result` currently exposes structured CLI argument rows, while the OpenCode orchestration contract asks the model to rename, flatten, and serialize those rows into strict JSON. A real `v2.7.0` occurrence failed all four reviewer slots before execution with `Task prompt binding is not provider-issued JSON`. Correctly assembled bindings pass existing tests, so the remaining defect is the model-authored transformation boundary.

## Authorized scope

The user explicitly requested the fix. Work is isolated on branch `fix/opencode-rdd-provider-task` in this owned worktree. Local implementation, tests, task tracking, and work-unit commits are authorized. Push, PR creation, merge, release, GitHub issue/comment mutation, and remote operations are not authorized.

## Scope

- Emit a Go-authored OpenCode V1 lens `provider_task` from native STATUS.
- Version the published status and capabilities schemas without rewriting historical schemas.
- Update rendered OpenCode orchestration and cross-lane proof to consume the opaque task.
- Preserve legacy binding admission for active lineages.
- Keep OpenCode V2 native review unavailable and fail-closed.

## Constraints

- Go remains the sole owner of bindings, prompts, schemas, budgets, admission, capture, and closure.
- The TypeScript adapter remains an opaque relay and must not parse or assemble bindings.
- Do not relax strict JSON decoding or authority validation.
- Do not change RDD lens selection, findings semantics, correction budget, receipts, or delivery policy.
- Historical status/capabilities schemas remain readable and unchanged.

## TDD and verification

- Mode: strict TDD.
- Source: `openspec/config.yaml` (`strict_tdd: true`).
- Runner: `go test ./...`.
- Required cycle per task: observed RED, GREEN, then REFACTOR.

## Delivery forecast

- Estimated authored change: 450-550 lines across two work units.
- Delivery strategy: `single-pr` with maintainer-approved `size:exception`.
- Chain strategy: not applicable; the user explicitly selected one PR.
- Review boundary: each completed work unit is an independently testable commit; native RDD handling follows the repository switch and risk assessment.

## Tasks

- [x] **ORPT-1 — Emit provider-owned OpenCode lens tasks**
  - Add a Go-issued lens provider task to OpenCode V1 `review.capture-result` inputs.
  - Add new status/capabilities schema versions and validate the exact task against native arguments and artifact subject.
  - Preserve other runtime inputs and OpenCode V2 refusal behavior.
  - Acceptance: focused tests prove STATUS emits byte-exact provider-owned lens tasks and rejects mutated task fields.
  - Checks: focused `internal/cli` tests; relevant schema/capabilities tests.
  - Evidence:
    - RED: `go test ./internal/cli -run '^TestOpenCodeV1StatusEmitsProviderOwnedLensTasks$' -count=1` failed because STATUS still emitted `gentle-ai.review-integration.status/v7` instead of v8.
    - GREEN: focused provider-task, mutation-refusal, runtime-preservation, capabilities/schema, legacy relay, and OpenCode V2 refusal tests passed.
    - `go vet ./...` passed.
    - `go test ./internal/cli -count=1` and `go test ./... -count=1` reached the package's 10-minute timeout in unrelated repository/Git process tests; the full run also reproduced pre-existing environment failures under `/var` lock paths and macOS Bash 3.2 release scripts. No ORPT-1-focused test failed.
    - Rollback boundary: revert this work-unit commit to remove status/v8, capabilities/v2.6, and OpenCode V1 lens `provider_task` emission without changing legacy transport admission or the TypeScript relay.

- [ ] **ORPT-2 — Consume the opaque task and prove the organic lane**
  - Update the OpenCode orchestration contract to copy `provider_task.agent` and `provider_task.prompt` exactly.
  - Remove host-side binding construction from the OpenCode cross-lane path.
  - Preserve adapter-minimality guards and legacy admission coverage.
  - Acceptance: rendered contract contains no OpenCode model-authored binding construction; cross-lane proof uses the provider-owned task.
  - Checks: focused component/assets tests, cross-lane test/harness, `go test ./...`, `go vet ./...`.
  - Evidence: pending.

## Progress

- Exploration complete: the defect is the model-authored STATUS-row-to-JSON transformation, not the TypeScript relay or strict Go decoder.
- Delivery decision: one PR with `size:exception`, explicitly authorized by the user.
- ORPT-1 complete in one work-unit commit; the exact commit hash is reported in the handoff.
- Next step: execute ORPT-2 without reopening ORPT-1 or changing its transport/schema boundary.
