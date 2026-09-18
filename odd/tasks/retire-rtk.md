# Retire RTK

## Objective

Remove RTK completely from the active repository product surface while preserving generic Community Tools behavior and CodeGraph.

## Problem

RTK was added as an optional Community Tool and is now deprecated. Leaving dormant model identities, UI choices, installation or synchronization branches, runtime acquisition code, tests, or documentation would preserve an unsupported product path.

## Why

A complete retirement is safer and easier to understand than a partially disabled integration. The user authorized one atomic repository-only removal because the implementation is primarily deletion and must remain coherent across the CLI, TUI, component, model, tests, and documentation.

## Scope

- Remove RTK source, acquisition, runtime, and their tests.
- Remove RTK model identity and Community Tool definition.
- Remove RTK install, backup, synchronization, persisted-state restoration, status, and TUI branches.
- Remove RTK-specific tests, fixtures, and active documentation.
- Preserve generic Community Tools infrastructure and CodeGraph behavior.

## Constraints

- Repository-only scope: do not alter personal RTK binaries or configuration.
- Do not delete unrelated RTK branches or worktrees.
- Preserve generic Community Tools and CodeGraph.
- Keep the retirement atomic; current forecast is approximately 1,691 authored changed lines, mostly deletions.
- Delivery strategy: `exception-ok`, inherited from the user's accepted atomic removal decision. Any future single PR requires an explicit review-size exception.
- Commit, push, pull request creation, labels, and merge are not authorized by this implementation request.
- TDD mode is not established for this resumed organic change. The current reconciliation step is read-only; resolve the mode only if corrective implementation beyond the existing deletion candidate is required. Ordinary regression verification remains mandatory.

## Tasks

- [x] **T-01 — Reconcile the existing retirement candidate**
  - Route: delegated read-only exploration.
  - Trigger: the candidate spans 21 source, test, and documentation paths, exceeding the four-file mapping threshold.
  - Outcome: inventory remaining active RTK references, accidental removals, stale fixtures, compile hazards, and exact correction surfaces.
  - Checks: repository-wide tracked/untracked reference scan; candidate diff review against the authorized scope.
  - Evidence: delegated mapper returned `COMPLETE` structurally. No active RTK references or RTK-named product paths remain outside this progress artifact; no stale RTK fixtures, docs, source, acquisition paths, or obvious static compile hazards were found. Generic Community Tools and CodeGraph remain present.

- [x] **T-02 — Apply bounded retirement corrections**
  - Route: delegated writer if T-01 identifies any correction; otherwise close as not needed with evidence.
  - Trigger: any correction is expected to span multiple non-trivial product surfaces.
  - Outcome: active RTK UI, installation, sync, status, source/acquisition, tests, and docs are absent while generic Community Tools and CodeGraph remain intact.
  - Checks: writer-owned focused tests and formatting for exact corrected surfaces.
  - Evidence: no correction was needed; the mapper recommended no edit surfaces.

- [x] **T-03A — Diagnose verification blockers**
  - Route: delegated read-only command diagnosis.
  - Trigger: the first verification hit a scan-glob defect and two 120-second harness timeouts, requiring a separate incident diagnosis before verification resumes.
  - Outcome: exclude the worktree `.git` pointer correctly and identify which affected Go packages complete or time out under bounded individual runs.
  - Checks: corrected RTK scan; one bounded, non-cached test command per previously unverified affected package; focused failing-test isolation; clean base snapshot reproduction if needed for attribution.
  - Evidence: corrected RTK scan passed. Community Tool (17.510s), uninstall (1.372s), and TUI (2.883s) packages passed. The two implicated candidate review tests passed independently in 0.768s and 6.272s. A temporary clean snapshot of base `9ec0cf443622f20fe511815ff5a83088c7467bff` reproduced a material `internal/cli` review-assessment timeout at 90.136s. The exact timed-out test varied, proving pre-existing package-level timeout behavior rather than a candidate-specific RTK retirement failure. Temporary cleanup succeeded.

- [x] **T-03B — Bound verification to candidate-causal surfaces**
  - Route: delegated read-only mapping followed by delegated focused verification.
  - Trigger: exhaustive repository shards exposed numerous unrelated-path timeouts and environment-sensitive failures, so candidate-causal checks must be mapped explicitly rather than treating the red baseline as an RTK defect.
  - Outcome: map every surviving changed function/behavior to exact focused tests and classify whether any broad-shard failure intersects the RTK retirement surfaces.
  - Checks: changed-function-to-test map for CLI, Community Tool, model/state, TUI, and docs; exact bounded test regexes; no source edits.
  - Evidence: mapping found complete surviving coverage for generic Community Tools, CodeGraph install/status/sync/backup/persisted restoration, model/state round trips, and TUI selection/install/rendering. No source or test correction is required. Broad failures in review/refuter/maintenance, SDD status, update, OpenCode, app, and review-transaction code do not intersect changed RTK retirement surfaces; only the review timeout class has clean-base attribution, while the remaining broad failures stay outside changed surfaces and unattributed.

- [x] **T-03 — Verify the atomic removal**
  - Route: delegated verification.
  - Trigger: command-running verification must use the verification worker.
  - Outcome: focused checks demonstrate that the candidate compiles, no active RTK references remain, and generic Community Tools/CodeGraph behavior is preserved; unavailable broad gates are recorded with causal evidence.
  - Checks: status/diff inventory, untracked inventory, corrected repository-wide RTK scan, formatting, candidate-causal affected tests, `go vet ./...`, and explicit broad-suite limitations.
  - Evidence: final verdict `PASS-WITH-LIMITATION`. All 13 candidate-causal commands passed: diff check, 21-path inventory, corrected RTK scan, formatting, focused vet, Community Tool tests (17.686s), CLI tests (7.641s), model tests (0.267s), state tests (0.280s), TUI tests (0.205s), TUI screen tests (0.467s), documentation scan, and final status. No active RTK references remain; generic Community Tools and CodeGraph remain tested and documented. The monolithic CLI timeout class reproduces on clean base; other broad-suite failures remain outside changed surfaces and unattributed. The parent spot-check reran `git diff --check` successfully and confirmed the expected candidate status.

- [ ] **T-04 — Close the work unit and record review/delivery evidence**
  - Route: parent orchestration plus native assessment/review when an authorized commit or PR-slice candidate exists.
  - Outcome: verification evidence, authored line count, rollback boundary, review outcome, and commit identity are recorded.
  - Checks: work-unit checklist; no commit, push, PR, label, or merge without separate authorization.
  - Evidence: T-03 passed with documented limitations. Awaiting explicit user authorization for the atomic work-unit commit; native candidate assessment/review and any push or PR remain pending and separately gated.

## Acceptance Criteria

- No active tracked or untracked repository code, test, fixture, or documentation references RTK.
- No RTK Community Tool identity, definition, UI option, installation path, synchronization path, status path, source/acquisition code, or runtime code remains.
- Generic Community Tools infrastructure and CodeGraph behavior remain present and verified.
- All selected focused checks pass; every skipped or unavailable check is recorded explicitly.
- The final diff contains only the atomic RTK retirement and its required progress evidence.

## Progress

- The feature branch `dnlrsls/retire-rtk` is based on `9ec0cf443622f20fe511815ff5a83088c7467bff`.
- The resumed worktree already contains a 21-path candidate with 23 additions and 1,668 deletions.
- Delegated structural reconciliation found the candidate complete, with no correction surfaces required.
- T-01 and T-02 are complete.
- The first T-03 pass preserved the candidate but exposed a faulty `.git` exclusion and two verifier harness timeouts.
- T-03A completed: the corrected scan passed, implicated candidate tests passed independently, and a clean base snapshot reproduced the `internal/cli` review-assessment timeout class.
- Exhaustive sharding covered all 1,571 CLI tests and 81 non-CLI packages but exposed a broadly red, environment-sensitive baseline rather than a bounded RTK signal.
- T-03B completed with no coverage gap or correction: every surviving changed behavior maps to existing generic Community Tool, CodeGraph, model/state, or TUI tests.
- T-03 completed with `PASS-WITH-LIMITATION`; every candidate-causal command passed and broad baseline failures are preserved separately.
- T-04 is awaiting explicit commit authorization.
- No source writes, commits, pushes, PRs, labels, or merges have been performed in this resumed session.

## Verification Evidence

- `git diff --check`: passed in the resumed worktree before further edits.
- Preliminary `git grep` scan excluding Git metadata: no active RTK references found.
- Delegated mapping: structurally complete; generic Community Tools and CodeGraph preserved; no corrections recommended.
- First delegated verification: `FAIL` due to the scan matching only `.git` worktree metadata and 120-second harness timeouts for combined focused/full tests. Diff check, formatting, and `go vet ./...` passed; model/state packages passed; no repository files changed.
- Separate bounded diagnosis: corrected scan passed; Community Tool, uninstall, and TUI packages passed; `internal/cli` timed out in an unrelated review-assessment test, with causality still unattributed.
- Focused candidate isolation: both implicated review tests passed independently.
- Clean-base attribution: a temporary base snapshot reproduced the material `internal/cli` review-assessment timeout class; cleanup succeeded.
- Exhaustive sharded verification: `FAIL`; all eight CLI shards reproduced review timeout behavior, while unrelated SDD status, update, OpenCode, app, and review-transaction packages reported assertions, Git ownership/configuration failures, or timeouts. The candidate remained unchanged.
- Candidate-causal changed-function/test mapping: complete with no coverage gap; broad failures do not intersect the changed RTK retirement surfaces.
- Final focused verification: `PASS-WITH-LIMITATION`; all 13 commands passed, no active RTK references remain, and CodeGraph/generic Community Tools remain covered.
- Parent spot check: `git diff --check` passed; status still contains exactly the 21 tracked candidate paths plus `odd/`.

## Rollback Boundary

Revert the atomic candidate paths listed by `git diff --name-status` plus this feature progress document. Do not alter unrelated Community Tools or CodeGraph changes.

## Next Step

Obtain the user's explicit decision on creating the atomic work-unit commit. If authorized, commit only the verified candidate and progress artifact, then assess/review that exact committed range before any separately authorized delivery.
