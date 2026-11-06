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

- [x] **T-04 — Close the work unit and record review/delivery evidence**
  - Route: parent orchestration plus native assessment/review when an authorized commit or PR-slice candidate exists.
  - Outcome: verification evidence, authored line count, rollback boundary, review outcome, and commit identity are recorded.
  - Checks: work-unit checklist; no commit, push, PR, label, or merge without separate authorization.
  - Evidence: the user authorized and Git created atomic work-unit commit `c6987cbcbb4e78599ea5ac163d2a851a9a134f18` (`refactor(community-tools): retire RTK integration`). Native assessment classified the committed range from `9ec0cf443622f20fe511815ff5a83088c7467bff` as high risk because `internal/cli/run.go` crosses a process boundary. The required independent post-commit verifier returned `PASS-WITH-LIMITATION`: all 14 commands passed against exact HEAD and the exact 22-path range. Native review lineage `review-a7f645c28ac89f24` ran all four high-tier lenses, approved the committed candidate, and its acknowledgement was consumed at revision `sha256:30f1b8e7be92ccbb4d2fd1fd83cd04f0586994def59732043d080d9b30b8e68d`; authority is burned. Two informational findings remain separate later work: `R2-dead-agent-scope` and `R4-orphaned-rtk-upgrade`. Push, PR, labels, and merge remain separately gated.

- [x] **T-05 — Create the RTK retirement issue**
  - Route: issue-creation workflow against `github.com/Gentleman-Programming/gentle-ai`.
  - Outcome: publish the user-confirmed Feature Request titled `refactor(community-tools)!: retire RTK integration` with the exact form body and create-time labels `enhancement` and `status:needs-review`.
  - Checks: open-and-closed duplicate search; exact form validation; explicit pre-flight affirmations; privacy scan; one create attempt; target-host readback.
  - Evidence: the user selected `Other`, affirmed both required checkboxes, and confirmed the exact draft. The first publication preflight stopped before `gh issue create` because the Python Windows alias was unavailable. The later create attempt produced issue #4763 but lost its local identity output and published an empty body because of a Windows temp-path namespace mismatch. After the user explicitly authorized repair of exact target `github.com/Gentleman-Programming/gentle-ai#4763`, one bounded body update and target-host readback returned `confirmed`: https://github.com/Gentleman-Programming/gentle-ai/issues/4763 is OPEN, body-exact, and retains `enhancement` plus `status:needs-review`.

- [x] **T-05A — Resolve the uncertain issue identity**
  - Route: human-provided target-host observation.
  - Outcome: establish either the exact created issue number/URL or authoritative confirmation that no issue exists.
  - Checks: validate any supplied issue identity against `github.com/Gentleman-Programming/gentle-ai` before continuing.
  - Evidence: the user explicitly authorized an exact-title `gh` lookup. It resolved one issue, #4763; readback showed the expected title and empty body. The authorized repair then confirmed the exact body and preserved labels/state, resolving the uncertainty.

- [x] **T-06 — Approve the RTK retirement issue**
  - Route: protected-label workflow for exact target `github.com/Gentleman-Programming/gentle-ai#4763`.
  - Outcome: replace `status:needs-review` with `status:approved` while preserving unrelated labels and state.
  - Checks: direct exact user authorization; authenticated actor identity; target-host `MAINTAIN` or `ADMIN`; existing-label discovery; exact pre-read; one atomic mutation; exact post-read.
  - Evidence: the user authorized the exact protected-label action. Authenticated actor `dnlrsls` had `MAINTAIN`. One atomic mutation returned `confirmed`; issue #4763 remains OPEN with labels `enhancement` and `status:approved`.

- [x] **T-07 — Publish the RTK retirement pull request**
  - Route: branch/PR workflow with the accepted single-PR `size:exception` strategy.
  - Outcome: commit final progress evidence, push HEAD to fork ref `refactor/retire-rtk`, and open a PR to `Gentleman-Programming/gentle-ai:main` linked with `Closes #4763` and declared `type:breaking-change`.
  - Checks: exact issue approval readback; no existing remote branch/PR; Conventional Commit; no co-author trailer; full PR template; documented 1,843-line size-exception rationale; target-host PR readback.
  - Evidence: issue #4763 is OPEN with `status:approved`. The user explicitly authorized the exact evidence commit, push, and PR creation. Commit `34372216373ab6ca9ef3e59d1af62f442a0dd809` passed independent docs-only verification; exact two-commit HEAD passed four-lens native review and acknowledgement. Fork ref `refactor/retire-rtk` was pushed and matched HEAD. PR #4764 was created and exact target-host readback returned `confirmed`: https://github.com/Gentleman-Programming/gentle-ai/pull/4764 is OPEN, targets `main`, has 22 files with 175 additions and 1,668 deletions, and its body is exact. It currently has no labels.

- [x] **T-08 — Apply required PR labels**
  - Route: exact-target delegated workflow actions on `github.com/Gentleman-Programming/gentle-ai#4764`.
  - Outcome: apply exactly one ordinary type label, `type:breaking-change`, and protected `size:exception` with the documented 1,843-line atomic-retirement rationale.
  - Checks: separate direct user authorization for each label; current target-host permission; exact pre-state and post-state; one bounded mutation per authorized action; preserve all unrelated labels/state.
  - Evidence: the user authorized both exact actions. One ordinary mutation added `type:breaking-change` and readback returned `confirmed`. A separate protected-label mutation revalidated actor `dnlrsls` with `MAINTAIN`, added `size:exception`, and readback returned `confirmed`. PR #4764 remains OPEN with exactly those two labels.

- [x] **T-09 — Rebase the conflicting PR branch**
  - Route: authorized history maintenance on fork branch `refactor/retire-rtk` only.
  - Outcome: rebase the two RTK retirement commits onto current upstream `main` and update the fork branch with `--force-with-lease`, without merging.
  - Checks: explicit user authorization for rebase and force-with-lease; fetch exact upstream main; conflict diagnosis; rerun applicable focused verification; fresh native review for the rebased candidate; remote SHA readback.
  - Evidence: the user authorized rebase and `--force-with-lease`. Rebase onto `5b82c00dce937bae079ac50c3891bf41724bd012` found four RTK-only modify/delete conflicts and one upstream stale `CommunityToolRTK` test reference. The four files were deleted, `TestOpenClawConfigDoesNotRedirectProjectToolRuntimeCwd` was corrected for CodeGraph-only selection, focused verification passed, and rebase completed as commits `110f1371…` and `c9e3bdaf…`. The runtime blocked force-with-lease twice despite explicit authorization, so the user selected the safe replacement-PR route instead.

- [x] **T-10 — Publish the rebased replacement PR**
  - Route: non-destructive replacement delivery without force-push.
  - Outcome: push rebased HEAD to a new fork ref, open a replacement PR, close superseded PR #4764, and apply required labels to the replacement.
  - Checks: exact rebased verification; fresh four-lens native review; remote SHA readback; exact PR body; direct authorization for close and labels; target-host post-readbacks.
  - Evidence: fork ref `refactor/retire-rtk-rebased` points to exact HEAD `c9e3bdafc255df7bc05cc96e9814a140a04892eb`. Replacement PR #4766 is OPEN and MERGEABLE: https://github.com/Gentleman-Programming/gentle-ai/pull/4766. It targets `main`, contains 23 files with 178 additions and 1,671 deletions, and has `type:breaking-change` plus protected `size:exception`. Superseded PR #4764 is CLOSED and was never merged. Required CI remains in progress; historical pre-label failures are followed by successful label/cognitive-load checks.

## Acceptance Criteria

- No active product code, test, fixture, or documentation references RTK; `odd/tasks/retire-rtk.md` is intentionally excluded as progress evidence.
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
- Atomic commit `c6987cbcbb4e78599ea5ac163d2a851a9a134f18` was created with the verified 22-path candidate and the configured GitHub no-reply identity.
- Native committed-range assessment is high risk; required independent post-commit verification passed with the documented broad-suite limitations.
- Native review of the exact committed range approved after all four lenses; the acknowledgement was consumed and authority burned.
- T-04 is complete. The user then authorized creation of a new retirement issue as the PR prerequisite.
- T-05 and T-05A are complete: issue #4763 exists with the confirmed form body.
- T-06 is complete: issue #4763 is OPEN with `enhancement` and protected label `status:approved`.
- T-07 is complete: fork branch `refactor/retire-rtk` matches HEAD and PR #4764 is OPEN with exact body and range.
- T-08 is complete: PR #4764 has exactly `type:breaking-change` and `size:exception`.
- T-09 is complete: rebase conflicts were resolved, focused and full candidate verification passed, and fresh native review approved the rebased range.
- T-10 is complete: replacement PR #4766 is OPEN and MERGEABLE with required labels; superseded PR #4764 is CLOSED without merge.

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
- Parent spot check: `git diff --check` passed; pre-commit status contained exactly the 21 tracked candidate paths plus `odd/`.
- Work-unit commit: `c6987cbcbb4e78599ea5ac163d2a851a9a134f18`; 22 paths, 141 additions, 1,668 deletions.
- Native assessment: high risk due to the `internal/cli/run.go` process boundary; independent post-commit verifier required.
- Independent post-commit verifier: `PASS-WITH-LIMITATION`; all 14 commands passed, HEAD and the 22-path committed range matched exactly, RTK scan was clean, and only `odd/tasks/retire-rtk.md` was modified afterward.
- Native committed-range review: approved by risk, resilience, readability, and reliability lenses; acknowledgement consumed for lineage `review-a7f645c28ac89f24`. Informational findings `R2-dead-agent-scope` and `R4-orphaned-rtk-upgrade` did not open corrections.
- Rebased candidate verification: `PASS-WITH-LIMITATION`; all 14 commands passed on HEAD `c9e3bdafc255df7bc05cc96e9814a140a04892eb`, 23 committed paths, no active RTK references, and only this task document modified locally. Parent readback counted 178 additions and 1,671 deletions.
- Rebased native review: all four lenses approved lineage `review-f50b3daf09d2f0bc`; acknowledgement consumed at revision `sha256:88adf8761dbadc4e1891abb86d5d1313084c8264fe64d98995146b9fa938a330`. Informational findings did not open corrections.
- Replacement delivery: PR #4766 exact head/base/body/range readback confirmed; `type:breaking-change` and protected `size:exception` were separately authorized and confirmed. PR #4764 closure was separately authorized and confirmed without merge.

## Rollback Boundary

Use commit `110f1371b39ae24bf8207782a9bff7acb5c6ae75` as the exact rollback boundary for the product changes, and delete `odd/tasks/retire-rtk.md` to remove the progress record and its later documentation-only updates. Do not alter unrelated Community Tools or CodeGraph changes.

## Next Step

Monitor CI and reviewer feedback on PR #4766. Do not merge. The worktree is clean, and all published progress evidence through the latest reviewer corrections is committed to the PR branch.
