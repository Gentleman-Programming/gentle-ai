# #4504 Active-Lineage Managed-Asset Freshness

## Objective

Make bound STATUS expose managed-asset skew before reoffering a capture, while preserving the active lineage and allowing the same capture slot to resume after the provider-issued sync continuation converges.

## Problem

Managed-asset freshness is checked before START and again at capture admission. For an active lineage, bound STATUS currently routes the authority transition without checking freshness. It can therefore reoffer a capture that is guaranteed to refuse at the capture guard after the candidate is already frozen.

## Why

Gentle AI issue #4504 item 5 reported a post-freeze managed-assets refusal with uncertain operation outcome. Issue #4434 fixed a separate non-converging continuation caused by invoking a different binary. Current tests prove START-time and capture-time guards, but not active-lineage STATUS → stale stop → sync → same-slot resume.

## Scope

- Detect managed-asset skew during bound active-lineage STATUS before returning a capture transition.
- Return the existing typed `managed_assets_outdated` stop and executable-anchored sync continuation without mutating authority.
- After assets converge, return the same provider-bound active transition and capture slot.
- Preserve existing START-time, no-authority, and capture-admission guards.
- Add a focused regression for the complete stale/resume sequence.

## Constraints

- Allowed edit surfaces: `internal/cli/review_next_transition.go`, `internal/cli/review_asset_provenance_test.go`, `internal/cli/review_opencode_transport_test.go` only if required, and this task document.
- Do not weaken managed-asset provenance or bypass capture authorization.
- Do not alter lineage revision, target identity, capture binding, or authority state during the stale stop.
- Technical artifacts remain in English.
- No push or pull request without separate authorization.

## TDD

- Mode: strict TDD enabled.
- Source: `openspec/config.yaml` and persisted project testing capabilities (`strict_tdd: true`).
- Focused runner: `go test ./internal/cli -count=1 -run '<focused test name>'`.
- Required cycle: RED → GREEN → REFACTOR.

## Tasks

- [x] **T1 — Prove the active-lineage timing gap.** Add a focused regression that creates an active capture transition, introduces managed-asset skew, observes bound STATUS, converges assets, and observes the same slot resume; record RED.
- [x] **T2 — Gate active STATUS without mutating authority.** Reuse the existing provenance check and typed stop before returning an active capture transition, then preserve normal routing after convergence.
- [x] **T3 — Verify the work unit.** Run focused provenance/transport tests, the complete `internal/cli` package, gofmt check, and `git diff --check`; record observed results.

## Acceptance Criteria

- Bound STATUS with stale managed assets returns only `stop/managed_assets_outdated`, not a capture transition.
- The stop carries the executable-anchored sync continuation.
- Authority state, revision, target, and capture binding remain unchanged.
- After convergence, the same bound STATUS returns the same capture slot.
- Existing START and capture guards continue passing.

## Progress

- #4504 was narrowed to this remaining active-lineage sequence.
- Source tracing found freshness checks before START and capture, but not in the active-authority STATUS branch.
- Tracking created before the first source or test write.

## Checks

- Initial `git status --short`: `?? odd/` (pre-existing task document).
- RED: `go test ./internal/cli -count=1 -run '^TestNegotiatedBoundStatusResumesSameCaptureAfterManagedAssetsConverge$'` failed as intended: stale bound STATUS still offered `collect/targeted_validation_required` instead of `stop/managed_assets_outdated` (1.849s). Production code was unchanged.
- Earlier fixture attempts exposed that reviewer materialization does not carry ProviderTask/Submission in this fixture; narrowed the regression to the existing targeted-validator provider-task fixture before recording intended RED.

- GREEN: the exact focused command above passed (2.994s) after wrapping transition resolution with the existing provenance gate for active native captures/provider tasks only. Recovery, acknowledgement, and existing stops retain their routing.
- TRIANGULATE/REFACTOR: restored reviewer coverage using its actual provider-bound ArtifactSubject rather than assuming ProviderTask/Submission; moved the displaced existing test comment back. The same focused command passed both reviewer and targeted-validator cases (4.733s). Both execute the advertised continuation in a temporary home and compare the complete resumed transition and authority.

- Focused guards: `go test ./internal/cli -count=1 -run 'TestNegotiatedReviewStartClassifiesStaleManagedAssetsBeforeAuthority|TestNegotiatedStatusReportsManagedAssetsOutdatedBeforeOfferingStart|TestManagedAssetsContinuationUsesInvokingExecutable|TestOpenCodeReviewTransportRefusesUnavailableAuthorityAtStartOrCompletion|TestNegotiatedBoundStatusResumesSameCaptureAfterManagedAssetsConverge'` passed (11.090s).
- Full package: `go test ./internal/cli -count=1` passed (552.852s).
- `go run ./internal/gofmtcheck` passed with no output.
- `git diff --check` passed with no output.
- Final `git status --short`: modified `internal/cli/review_asset_provenance_test.go` and `internal/cli/review_next_transition.go`; `?? odd/` remains. No transport-test edits were needed.
- No commit, push, issue/PR operation, native review, real-user asset sync, or other-worktree mutation was performed. Sync execution was confined to test-managed temporary homes.
- Independent verification: PASS for all six acceptance criteria; focused regression and provenance guard matrix passed, gofmt and diff checks passed, and no candidate-caused findings were reported.
- Parent spot check: the exact focused regression passed again in 4.732s; `git diff --check` passed and repository scope remained the two Go files plus this task document.
- Native risk assessment was unavailable because the native command returned empty output; policy treated the candidate as high risk and required the completed independent verification.
- Work-unit commit: `6b5eeb0d672701ac9954a8bbb04b5cc3d862890f` (`fix(review): gate stale active capture status`).
- Engram mirror: full document saved under `odd/4504-active-lineage-asset-freshness/tasks`, project `gentle-ai`. Readback is unavailable in the worker's tool set; save acknowledgements were observed.

## Next Step

Run the authorized native review over the committed work unit, then push and open the authorized PR.
