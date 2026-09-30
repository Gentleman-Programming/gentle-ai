# ga-1876: managedstate transaction slice 1 (full-file sync)

Claimed 2026-09-29 (issuecomment-5900583781); prospectus issuecomment-5900803139.
Branch prospectus/1876-migration-cert (worktree ~/gentleman/gentle-ai-1876) from
upstream/main 43af401a.

## Objective

First approved slice of #1876: internal/managedstate, a write-ahead migration
journal + state machine proving crash-recovery semantics for ONE full-file
resource (persona output-style class), subordinate to the landed
internal/state manifest model (gentle-ai.managed-assets/v1). No cross-platform
gate claim, no CLI wiring (follow-up slices), no marker extents.

## Route declaration (mandatory delegation)

Inline direct. All design context lives in this session (approved thread
design, landed manifest model, filecoord API, prospectus re-anchors);
transferring it to a worker would cost more than the implementation. Single
new package, single work unit. Verify step will re-read the finished code
against the restart decision table line by line.

## Design anchors (from approved thread + main 43af401a)

- Committed authority: internal/state.Manifest (0da1ab73). ComputeBundleDigest
  excludes observed = generation identity for CAS.
- Journal: ~/.gentle-ai/transactions/<txid>/journal.json, schema
  gentle-ai.migration-journal/v1, atomic revisions (filemerge.WriteFileAtomic),
  first restartable phase = prepared.
- Phases: discovered classified snapshotted prepared applying verified
  manifest_committed completed. Terminal: blocked_unknown_ownership
  rollback_required rolled_back; restart-only: blocked_conflict.
- Lock: filecoord.Acquire (cooperative, BusyError typed) over the state dir,
  held preflight..publication.
- Snapshot: before-images inside the tx dir (digest+mode recorded), NOT the
  user backup system (isolation per dnlrsls 2026-08-13).
- Schema gate before any write: newer -> unsupported_newer_schema read-only;
  invalid -> unknown_authority read-only.
- Completion appends one audit line to the existing state.AppendJournal and
  deletes the tx dir (never a second committed truth).

## Tasks

1. [x] RED: journal round-trip + schema-gate decode tests.
2. [x] RED: coordinator matrix tests (T1 success+no-op, T3 unknown-ownership
       zero-mutation, T4 crash-before-replace resumes, T5 crash-after-replace
       commits without rewrite, T6 foreign bytes -> rollback_required no
       overwrite, T7 verify failure -> exact restore -> rolled_back, T8
       newer/invalid schema read-only, contention BusyError, state.json
       untouched, T4b crash-after-manifest-commit -> bookkeeping only,
       stale-CAS no write).
3. [x] GREEN: journal.go + snapshot.go + coordinator.go + faults.
4. [x] Full suite + gofmt + vet + GOOS=darwin/windows build.
5. [x] Work-unit commits; push + PR = user decision (size verdict honest:
       if >400 lines, slicing rationale = the state machine and its matrix
       are one review atom; model/snapshot split already taken).

## Evidence

- Task 3+4 (GREEN): all 24 tests PASS with -race; full repo suite
  `go test ./...` exit=0 (81 pkgs ok); gofmt/vet clean; GOOS=darwin and
  GOOS=windows `go build ./...` OK.
- Independent read-only verify (gentle-ai-verify subagent) against the
  restart decision table: 13/13 rules confirmed implemented + tested, with
  2 real deviations found and FIXED before commit:
  (1) foreign-bytes guard required Before.Exists; bytes appearing on an
      absent-before target were overwritten (rule 9 gap) -> guard now
      refuses both directions (fix + TestForeignBytesOnAbsentBeforeTarget...).
  (2) aligned no-op compared digest only, ignoring mode drift (rule 13)
      -> matches() digest+mode everywhere (fix + TestAlignedSecondRunConvergesDriftedMode).
  Observations fixed: lexicographic phase compare -> phaseOrder; audit-line
  ordering documented. 7 gap tests added (verified-resume publishes once,
  bookkeeping-stale, partial-restore rollback_required, invalid journal
  schema, plan drift, absent-before foreign, mode drift).
- Task 5 (size verdict): 1870 insertions > 400-line review budget. Honest
  slicing: model (journal.go+tests, 325 lines) vs machine (coordinator.go +
  matrix, 1545) is already the structural split, but every PR-level split
  ships either unrunnable tests or an untested state machine (CI must be
  green per PR). The machine plus its 24-case matrix is the review atom ->
  size:exception request in the PR body, same precedent as #5021.
- Task 1+2 (RED): stage 0 build failure, package absent:
  `go test ./internal/managedstate/` -> "undefined: Plan ... undefined: Run
  ... FAIL [build failed]" (journal_test.go + coordinator_test.go committed
  first; 13 test functions covering matrix T1,T3,T4,T5,T6a,T6b,T7,T8a-c,
  contention, state.json, bookkeeping-only, stale-CAS).

## Post-PR review activity (2026-09-30)

- CodeRabbit (PR #5124): priority Low, effort 4, one final-review-risk finding:
  resumed plans indexed positionally could misalign journal vs plan resources.
  FIXED in-commit before any wiring: alignPlanToJournal matches by ID,
  fails closed on missing ID or changed target; 3 regression tests
  (reordered plan converges per-resource, missing ID zero-writes, target
  drift zero-writes). 27 tests -race green, full suite exit=0.
- Native review lineage review-817e2399d13b3bea (medium, review-reliability):
  reviewer slot declared unachievable, relay transport bound exceeded
  (966146ms vs 966123ms derived bound, 77038-byte prompt). Recovery
  documented: raise GENTLE_PI_REVIEW_RELAY_PI_TIMEOUT_MS (floor+perMiB
  derivation, ceiling 7200000) in the relay host env, withdraw the slot,
  re-capture. Withdraw deferred until the bound can actually change;
  provider state is durable.
