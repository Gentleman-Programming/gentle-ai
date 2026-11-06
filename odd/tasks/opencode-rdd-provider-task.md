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
    - Commit: `b37b0e36b2ecdcc21318acb1f225c3f1cf7bc8b2` (`fix(review): emit provider-owned OpenCode lens tasks`).
    - RED: `go test ./internal/cli -run '^TestOpenCodeV1StatusEmitsProviderOwnedLensTasks$' -count=1` failed because STATUS still emitted `gentle-ai.review-integration.status/v7` instead of v8.
    - GREEN: focused provider-task, mutation-refusal, runtime-preservation, capabilities/schema, legacy relay, and OpenCode V2 refusal tests passed.
    - `go vet ./...` passed.
    - `go test ./internal/cli -count=1` and `go test ./... -count=1` reached the package's 10-minute timeout in unrelated repository/Git process tests; the full run also reproduced pre-existing environment failures under `/var` lock paths and macOS Bash 3.2 release scripts. No ORPT-1-focused test failed.
    - Rollback boundary: revert this work-unit commit to remove status/v8, capabilities/v2.6, and OpenCode V1 lens `provider_task` emission without changing legacy transport admission or the TypeScript relay.

- [x] **ORPT-2 — Consume the opaque task and prove the organic lane**
  - Update the OpenCode orchestration contract to copy `provider_task.agent` and `provider_task.prompt` exactly.
  - Remove host-side binding construction from the OpenCode cross-lane path.
  - Preserve adapter-minimality guards and legacy admission coverage.
  - Acceptance: rendered contract contains no OpenCode model-authored binding construction; cross-lane proof uses the provider-owned task.
  - Checks: focused component/assets tests, cross-lane test/harness, `go test ./...`, `go vet ./...`.
  - Evidence:
    - RED: `go test ./internal/components/sdd ./scripts/crosslane -run '^(TestOpenCodeReviewContractRelaysProviderOwnedLensTaskExactly|TestOpenCodeHookHarnessRequiresExactProviderOwnedTask)$' -count=1` failed because the rendered contract still required model-authored binding JSON and the harness still assembled `binding_pairs` with `Object.fromEntries`.
    - GREEN: the final focused contract, exact-copy, V2, concurrent-group, transport-selection, and prompt-cost tests passed in `0.429s` (`internal/components/sdd`) and `0.011s` (`scripts/crosslane`).
    - `go test ./internal/components/sdd ./internal/assets ./scripts/crosslane -count=1` passed: `119.812s`, `5.929s`, and `0.702s` respectively.
    - Adapter-minimality/legacy admission/OpenCode V2 checks passed: `go test ./internal/cli -run '^(TestOpenCodeV2TransportCapabilityUnavailable|TestOpenCodeV2TransportDeclarationCannotInheritV1|TestOpenCodeReviewTransportAdmitsContractShapedHostLensFrame|TestOpenCodeReviewTransportAdmitsHostLensFrameWithNumericOrderAndShuffledKeys|TestOpenCodeReviewTransportRefusesHostLensFrameValueTampering)$' -count=1` (`6.865s`).
    - Built `./cmd/gentle-ai` to a local binary and ran `go run ./scripts/crosslane --binary /tmp/gentle-ai-orpt2`: all ten non-model OpenCode lifecycle checks passed, including exact lens/validator task relay, correction, acknowledgement, and legacy host-echo admission. The overall battery exited 1 only because the unrelated schema lane cannot resolve the historical relative `$id` in `intended-untracked-selection.schema.json`; three real-host subscription tiers were explicitly skipped by the default non-model run. An initial root-package build attempt failed with `no Go files`; the supported `./cmd/gentle-ai` build succeeded.
    - The OpenCode golden was regenerated through its repository `-update` path, inspected, and passed without `-update`; `go test ./internal/components -count=1` passed in `6.061s`.
    - `go vet ./...` passed.
    - `go test ./... -count=1` failed after ten minutes in unrelated `internal/cli` Git process and `internal/reviewtransaction` lock waits, plus the pre-existing macOS `/var` authority-path and Bash 3.2 release-script incompatibilities. The run also found the expected OpenCode golden drift; it was regenerated and its package passed afterward.
    - Rollback boundary: revert the second ORPT-2 work-unit commit to restore the prior orchestration prose and host-assembled cross-lane harness without removing ORPT-1 status/v8 or provider-task emission.
    - Authored change: 442 lines for ORPT-2, excluding the regenerated golden; 1,081 cumulative authored lines across ORPT-1 and ORPT-2.
    - Commit: `cbb0d61d815e386faa60921f12517da66a3c7f5a` (`fix(review): relay provider-owned OpenCode tasks`).

- [x] **ORPT-3 — Preserve j75's Pi intended-untracked contract**
  - Diagnose why the OpenCode V1 provider-task change breaks the Pi intended-untracked journey.
  - Correct only the stale or over-broad contract assumption exposed by j75; do not change provider-task behavior or unrelated review flows.
  - Acceptance: the driven `j75-intended-untracked-selection-executes-printed-start` journey completes while still exercising Pi and executing the exact printed START.
  - Checks: focused driven j75 run; benchmark module tests; affected component/assets/cross-lane packages; formatting and diff checks.
  - Evidence:
    - Root cause: ORPT-1 intentionally advanced every negotiated v2 STATUS envelope from `status/v7` to `status/v8`, but the black-box bench corpus still pinned j75 and the shared capture-evidence assertion to the former current schema. Pi behavior and the intended-untracked transition were unchanged; only the corpus schema expectation was stale.
    - RED: the focused driven j75 run failed on `initial Pi STATUS` because the product emitted `gentle-ai.review-integration.status/v8` while the journey required v7.
    - GREEN: the same locally built harness/product run completed j75 with 3 commands, 0 blocks, and 0 model runs.
    - REFACTOR: the bench constant and all current-schema assertions now name v8 explicitly; no product behavior changed.
    - `go test ./... -count=1` from `bench/` passed in `5.505s`.
    - `go test ./internal/components/sdd ./internal/assets ./scripts/crosslane -count=1` passed in `116.295s`, `6.536s`, and `0.758s` respectively.
    - `go run ./internal/gofmtcheck` passed with no output.
    - Rollback boundary: revert this work unit to restore the stale v7 bench expectation; native STATUS/provider-task behavior is unaffected.

## Progress

- Exploration complete: the defect is the model-authored STATUS-row-to-JSON transformation, not the TypeScript relay or strict Go decoder.
- Delivery decision: one PR with `size:exception`, explicitly authorized by the user.
- ORPT-1 complete in commit `b37b0e36b2ecdcc21318acb1f225c3f1cf7bc8b2`.
- ORPT-2 complete in commit `cbb0d61d815e386faa60921f12517da66a3c7f5a`.
- ORPT-3 complete locally: the driven j75 assertion follows the ratified status/v8 contract without changing Pi or provider-task behavior.
- Cumulative authored change: 1,081 lines across the two work units, excluding the regenerated golden; delivery remains one PR with maintainer-approved `size:exception`.
- Next step: verify and deliver the ORPT-3 work-unit commit through ordinary PR policy; push and GitHub mutation remain outside this task's authorization.
