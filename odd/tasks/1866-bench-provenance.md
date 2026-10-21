# 1866 bench provenance: bind benchmark results to exact inputs

Re-implements closed PR #5021 (oversized, 412 commits stale) on current main as stacked PRs to main (≤400 changed lines each).
Reference implementation: branch `pr1-fix` commits 7ce2f4c58 and e56f1af92 (worktree `gentle-ai-1866`). Issue: Gentleman-Programming/gentle-ai#1866 (status:approved).

## Specs

Acceptance criteria quoted verbatim from issue #1866; this chain covers S3, S4 and the result side of S7. S1/S2 already hold on main (ci.yml "Build, vet, and test benchmark module" step). S5, S6, S8-S10 belong to later chains.

- S1: "A dedicated CI job builds, vets, and tests the nested `bench` module."
- S2: "Benchmark-related CI cannot pass with zero discovered tests."
- S3: "Result schema v2 identifies benchmark source/binary, classifier, corpus manifest, target digest/VCS revision, runtime, mode, and journey subset."
- S4: "The schema defines a canonical path-free representation whose digest is stable across equivalent isolated runs; raw machine paths are diagnostic-only."
- S5: "Result generation verifies the target's embedded VCS revision and digest before measurement."
- S6: "Release-candidate benchmark evidence is stored immutably with checksums and workflow/run identity."
- S7: "Re-running with any changed target, benchmark, classifier, corpus, platform, mode, or subset creates a distinct evidence identity."
- S8: "Documentation numerical claims link to one immutable result and name its exact identity tuple."
- S9: "Historical claims remain labelled historical; incomparable populations are never presented as one “Latest” series."
- S10: "The evidence workflow depends on #1865's corrected classifier contract before it can be release-blocking."

Canonical tokens from the approved design comment: "`<RUN_ROOT>` `<REPOSITORY>` `<HOME>` `<TARGET_BINARY>`".

## Tasks

Chain strategy: stacked PRs to main (L2). Each task = one PR, one or more work-unit commits.

- T1a [S4] journey records path-free: PathNormalizer + token vocabulary wired into runJourney and the accumulator. Branch `feat/1866-bench-path-normalizer`. Commit: pending. PR: pending.
- T1b [S4] top-level fields path-free: `binary`, requested selectors and notes in `run`; `compare` labels projected sides with their binary version. Branch `feat/1866-bench-run-fields`. Commit: pending. PR: pending.
- T1c [S4] observed mode path-free: `analyze` projects the session path and every recorded string through the user home. Branch `feat/1866-bench-observed-paths`. Commit: pending. PR: pending.
- T2 [S3, S7] identity envelope: schema `gentle-ai-bench.results/v2` with benchmark/corpus/target/runtime/invocation identity; reading fails closed on unknown schema. Commit: pending. PR: pending.
- T3 [S4, S7] canonical validation + EvidenceDigest: writer refuses recorded machine paths; digest stable across sandboxes, written last. Commit: pending. PR: pending.

## Log

- L1 (user, 2026-10-21): "si arranca con el #1866"
- L2 (user, 2026-10-21): chose "Stacked PRs a main (recomendado)" for the chain.
- L3 (evidence): since the old base only `bench/runner.go` changed (+11/-5) among touched files; no open PR works on bench provenance.
- L4 (T1 decision, fixes a #5021 defect): fixed-order replacement turned a target binary under HOME into `<HOME>/...`; CI builds it in `$RUNNER_TEMP` inside `/home/runner`. Replacement is now longest-path-first (RED: TestPathNormalizerProjectsTargetBinaryUnderHome).
- L5 (T1 decision, fixes a #5021 defect): projecting `binary` made both `compare` sides read `<TARGET_BINARY>`; a projected side is now labelled with its binary version, path sides unchanged (RED: TestComparisonSidesStayDistinguishableAfterProjection).
- L6 (T1 decision): the first T1 commit measured 596 changed lines, over the 400-line budget. One cohesive slicing pass split it into T1a (runner, ~340), T1b (run fields + compare, ~200) and T1c (observed mode, ~110), each a stacked PR to main.
