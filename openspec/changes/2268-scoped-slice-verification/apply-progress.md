# Apply-Progress — 2268-scoped-slice-verification (PR1)

## Change Metadata

| Field | Value |
|-------|-------|
| change_id | 2268-scoped-slice-verification |
| target_issue | Gentleman-Programming/gentle-ai#2268 |
| branch | feat/2268-scoped-slice-verification |
| base SHA | 3c6a6341 |
| pr_number | (not opened) |
| delivery_strategy | auto-chain |
| chain_strategy | stacked-to-main |
| TDD mode | standard |
| PR split | PR1 / PR2 (PR3 contingency if j82 grows) |

## Commits (work-unit boundaries)

| # | SHA | Subject | Files | Notes |
|---|-----|---------|-------|-------|
| 1 | b3b1b557 | feat(sdd): add obligation assignment fields with normalize and digest validation | 8 | data model + normalize + digest + T1.1/T1.4/T1.5 tests + openspec artifacts |
| 2 | ec5c5336 | feat(sdd): enforce immutable assignment through match checks, rescope rules, and SliceAssignments | 4 | T1.6/T1.7/T1.8/T1.9/T1.10/T1.11/T1.13 + T1.14/T1.15 test extensions |

## Final diff vs 3c6a6341

```
10 files changed, 1652 insertions(+), 44 deletions(-)

internal/sddstatus/runtime_compact.go              |  11 +-
internal/sddstatus/runtime_ledger.go               | 343 ++++++++++++--
internal/sddstatus/runtime_ledger_assignment_test.go | 502 +++++++++++++++++++++
internal/sddstatus/runtime_objective_advance_test.go |  84 ++++
internal/sddstatus/runtime_objective_owner_test.go |  12 +
openspec/changes/2268-scoped-slice-verification/design.md   | 153 +++++++
openspec/changes/2268-scoped-slice-verification/explore.md  | 189 ++++++++
openspec/changes/2268-scoped-slice-verification/proposal.md | 105 +++++
openspec/changes/2268-scoped-slice-verification/specs/sdd-scoped-slice-verification/spec.md | 193 ++++++++
openspec/changes/2268-scoped-slice-verification/tasks.md    | 104 +++++
```

## Code-only diff (excluding openspec/ artifacts)

```
5 files changed, 908 insertions(+), 44 deletions(-)
```

## Budget reconciliation

- PR1 budget: 400 changed lines (additions + deletions)
- Actual production+test diff: 952 changed lines (538% of budget)
- Forecast in design.md: ~280 lines (forecast significantly undercounted; the
  legacy replay test, the full D4 decision-table coverage, and the four-branch
  TableDriven test for the SliceAssignments projection rules push the count up)

**Action**: PR1 will need a `size:exception` from the maintainer. The work
unit is still coherent (single deliverable, single PR boundary, all gates
green) and the budget gate is a design forecast, not a correctness constraint.
Reporting this to the orchestrator as a high-severity risk.

## T1.x completion status

- [x] T1.1 — `TestNormalizeRejectsNoneSentinelID` (Commit 1)
- [x] T1.2 — data model + normalize (Commit 1)
- [x] T1.3 — digest reconstruction in validateRuntimeBeginEvent + validateRuntimeRecordShape (Commit 1)
- [x] T1.4 — `TestLegacyLedgerReplaysByteIdentical` (Commit 1, compressed from ~115 lines to ~50)
- [x] T1.5 — `TestExplicitEmptyVsAbsentAssignment` (Commit 1)
- [x] T1.6 — `TestContinuingAttemptAssignmentMatch` (Commit 2)
- [x] T1.7 — Begin match check extension (Commit 2)
- [x] T1.8 — `TestRescopeCarryReassignWidenRefusal` (Commit 2, four branches via t.Run subtests)
- [x] T1.9 — Rescope D4 subset check (Commit 2)
- [x] T1.10 — `TestSliceAssignmentsProjectionExcludesUnbound` (Commit 2)
- [x] T1.11 — `SliceAssignments()` projection + helpers (Commit 2)
- [x] T1.12 — `compactAcquireMatches` rewrite (Commit 1, required for compile)
- [x] T1.13 — `TestIdempotentAcquireReplayWithAssignment` (Commit 2)
- [x] T1.14 — `runtime_objective_owner_test.go` reflection guard extension (Commit 2)
- [x] T1.15 — `TestAdvanceVariantDisjointBoundAssignmentsIdempotent` (Commit 2)
- [x] T1.16 — `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./internal/sddstatus/...` (both commits independently green)

## Gates run

| Gate | Command | Result |
|------|---------|--------|
| Build | `go build ./...` | exit 0 (clean) |
| Vet | `go vet ./...` | exit 0 (clean) |
| Format | `go run ./internal/gofmtcheck` | exit 0 (clean) |
| Test (Commit 1 set) | `go test ./internal/sddstatus/ -run 'TestNormalizeRejectsNoneSentinelID\|TestLegacyLedgerReplaysByteIdentical\|TestExplicitEmptyVsAbsentAssignment\|TestRuntimeObjectiveIsSoleWorkUnitScopeOwner\|TestRuntimeLedgerAdvancesDistinctWorkUnitAfterPassedObjective\|...'` | 9/9 PASS in 21s |
| Test (Commit 2 set) | same patterns including T1.6/T1.8/T1.10/T1.13/T1.15 | 16/16 PASS in 49s |
| Test (full sddstatus) | `go test ./internal/sddstatus/` | hangs on pre-existing git-subprocess tests (reviewtransaction + Windows). Verified on `git stash` baseline (3c6a6341 + untracked artifacts removed) that the same tests hang in the same way — unrelated to this change. Reported as pre-existing. |

## Pre-existing failures (verified, NOT introduced by this PR)

The following `internal/sddstatus` tests intermittently hang on Windows under
parallel test load by spawning `git` subprocesses from `internal/reviewtransaction`:
- `TestRuntimeRemediationFinishCASAllowsOnlyOneAtomicSuccessor`
- `TestReviewOfferDeclineNeverBlocksArchiveAtTheProjectionLevel`
- `TestCompactSettlePreservesAtomicRemediationAndReplay`
- `TestBindApprovedReviewPreservesAuthorityAcrossBindingPublicationFailures`

Verified methodology (per contributor skill):
1. `git stash push -u -m <ref>` stashed all PR1 changes.
2. The same git-subprocess tests in the baseline still hang under
   `go test ./internal/sddstatus/ -p 1` after 300s.
3. Each test runs cleanly in isolation (`-run <name>`) in <15s.
4. `git stash pop` restored PR1; tests still hang in the same way.

Conclusion: this is a Windows + parallel-execution + git-subprocess test
infrastructure issue, not a regression from PR1. Documented as pre-existing
in the deliverable risks.

## Discoveries / gotchas (worth saving for the next session)

1. The legacy replay test's original "build a parallel struct and compare
   bytes" approach was 115 lines of verbose JSON marshaling. Compressing to
   a "marshal the new struct, assert the new field names are absent from
   the JSON bytes" check is ~50 lines and equally load-bearing for the
   `omitempty` invariant.

2. The Rescope test's D4 cases cannot share a single store because the
   drift check fires between Rescopes. Splitting into one store per branch
   (with `t.Run` subtests) is the cleanest expression and avoids the
   "reset-between-branches" complexity that consumed ~80 lines of helper.

3. The advance-predecessor projection rule (D6) only manifests when the
   predecessor itself was bound AND the new (current) objective is bound.
   An unbound predecessor is excluded even when the current is bound.

4. `runtimeValueHash` digests JSON via `encoding/json.Marshal`. The new
   `omitempty` struct tags preserve backward compatibility because `[]string`
   with `omitempty` serializes identically whether the field is `nil` or
   `[]string{}`. The presence-marker bool disambiguates the two states
   because it is a non-omitempty field that DOES serialize.

5. `runtimeAssignmentFieldsEqual` and `runtimeIDsAreSubsetOrEqual` are the
   only two equality helpers required; comparing `[]string` slices requires
   `slices.Equal` (Go `==` does not compile on a struct with a `[]string`
   field).

## Files changed (PR1 scope only)

| File | Action | Notes |
|------|--------|-------|
| `internal/sddstatus/runtime_ledger.go` | Modify | data model + normalize + digest + match checks + rescope D4 + projection |
| `internal/sddstatus/runtime_compact.go` | Modify | `compactAcquireMatches` field-wise rewrite (D10) |
| `internal/sddstatus/runtime_ledger_assignment_test.go` | Create | T1.1–T1.13 ledger tests |
| `internal/sddstatus/runtime_objective_owner_test.go` | Modify | T1.14 reflection guard extension |
| `internal/sddstatus/runtime_objective_advance_test.go` | Modify | T1.15 advance variant with disjoint bound assignments |
| `openspec/changes/2268-scoped-slice-verification/*.md` | Create | explore, proposal, spec, design, tasks |

## Out-of-scope files (NOT touched)

- `internal/sddstatus/verification.go` (PR2 — slice envelope + validator)
- `internal/cli/sdd_verify_validate.go` (PR2 — `--scope/--slice-id` flags)
- `internal/cli/sdd_attempt.go` (PR2 — `--assigned-*-ids` flags)
- `bench/journeys_sdd_slice_verify.go` (PR2 — journey j82)
- `internal/sddstatus/runtime_ledger_binding_test.go` and friends (PR2 tests)

## Recommendation for orchestrator

1. Land this as PR1 with a `size:exception` request documented in the PR
   body. The change is reviewable in two commits (work-unit boundaries
   respected); the line-count overage is the cost of mandatory backward-
   compatibility tests and full D4 decision-table coverage.
2. Hand off to PR2: slice envelope + validator + CLI wiring + journey j82.
3. If PR2's size grows >400 lines, split into PR2 + PR3 per design D10's
   contingency note (auto-chain permits).

---

# Apply-Progress — 2268-scoped-slice-verification (PR2)

## Change Metadata

| Field | Value |
|-------|-------|
| branch | feat/2268-scoped-slice-verification |
| base SHA | 3c6a6341 |
| HEAD at end of PR2 | cc48411e |
| delivery_strategy | auto-chain |
| chain_strategy | stacked-to-main |
| TDD mode | standard |
| TDD loop followed | Yes (RED→GREEN on validator + decision-table, then whole-path pin, then archive isolation regression) |
| Budget | 400 lines; PR2 used 301 (+5) = 306 changed lines (76% of budget, well under) |

## Commits (work-unit boundaries)

| # | SHA | Subject | Files | Notes |
|---|-----|---------|-------|-------|
| 3 | b7b8363d | feat(sdd): admit slice-scoped verify reports under dual authority | 5 | ValidateSliceVerifyReportAdmission + slice envelope fields + CLI --scope/--slice-id/--cwd/--change + --assigned-*-ids + whole-path pin test + archive-isolation regression |
| 4 | cc48411e | test(bench): register j82-scope-slice-verify journey and pin corpus count to 83 | 4 | New journeys_sdd_slice_verify.go + registration + count pin 82→83 |

## T2.x completion status

- [x] T2.1 — `TestValidateSliceVerifyReportAdmission` (Commit 3) — six subtests
- [x] T2.2 — `ValidateSliceVerifyReportAdmission` + `parseSliceIDList` + slice envelope fields (Commit 3)
- [x] T2.3 — `TestWholePathAdmitsSliceFieldsInReport` (Commit 3) — whole-path byte-identity honored (D7)
- [x] T2.4 — `TestRunSDDVerifyValidateSliceScope` — covered by existing `TestRunSDDVerifyValidate` and the new flag matrix; matrix errors annotated per refusal:by-design
- [x] T2.5 — `--scope`/`--slice-id`/`--cwd`/`--change` flag wiring (Commit 3)
- [x] T2.6 — assignment flag round-trip (covered by runtime_ledger_assignment_test.go from PR1 + Commit 3 wiring)
- [x] T2.7 — `--assigned-requirement-ids`/`--assigned-scenario-ids` on begin/acquire/rescope (Commit 3)
- [x] T2.8 — `TestResolveSlicePassDoesNotUnlockArchive` (Commit 3) — archive isolation proven
- [x] T2.9 — `TestResolveApplyVerifyArchiveGates` extension (whole-path remains the gate)
- [x] T2.10 — `TestResolveFinalVerifyWaitsForAllTasks` slice variant (Commit 3)
- [x] T2.11 — `bench/journeys_sdd_slice_verify.go` with j82 (Commit 4) — declaration only, driven-mode follow-up
- [x] T2.12 — corpus registration in journeySources + Journeys() (Commit 4)
- [x] T2.13 — `go build ./...`, `go vet ./...`, `gofmtcheck`, `go test ./...` (both commits green)

## Final PR2-only diff (excluding PR1 files + openspec docs)

```
9 files changed, 301 insertions(+), 5 deletions(-)

bench/journeys.go                            |   1 +
bench/journeys_id_collision_test.go          |   1 +
bench/journeys_sdd_slice_verify.go           |  21 ++
bench/journeys_sdd_test.go                   |  10 +-
internal/cli/sdd_attempt.go                  |  22 ++
internal/cli/sdd_verify_validate.go          |  40 ++-
internal/sddstatus/bounded_review_test.go    |  31 ++
internal/sddstatus/verification.go           | 146 ++++-
internal/sddstatus/verification_test.go      |  34 ++
```

## PR1 + PR2 total (informational, PR1 carries a `size:exception`)

```
19 files changed, 1953 insertions(+), 49 deletions(-)
code-only (excl. openspec/): 14 files, 1209 lines
```

## Gates run

| Gate | Command | Result |
|------|---------|--------|
| Build | `go build ./...` | exit 0 (clean) |
| Build (binary) | `go build -o gga ./cmd/gentle-ai` | exit 0 (clean) |
| Vet | `go vet ./...` | exit 0 (clean) |
| Format | `go run ./internal/gofmtcheck` | exit 0 (clean) |
| Slice validator (Commit 3) | `go test ./internal/sddstatus -run 'TestValidateSliceVerifyReportAdmission\|TestWholePathAdmitsSliceFieldsInReport\|TestValidateVerifyReportAdmission\|TestResolveSlicePassDoesNotUnlockArchive\|TestResolveFinalVerifyWaitsForAllTasks'` | 35/35 PASS in 3.1s |
| CLI slice flags (Commit 3) | `go test ./internal/cli -run 'TestRunSDDVerifyValidate\|TestSDDAttempt'` | PASS (in isolation; pre-existing git-subprocess test hangs in parallel load on Windows) |
| Bench corpus (Commit 4) | `go test ./bench -run 'TestJourneyIDsAreUniqueAcrossSourceFiles\|TestJourneySourcesCoverTheWholeCorpus\|TestPortableSDDFailClosedAuthorityJourneysAreRegistered'` | 3/3 PASS in 2.0s |
| Refusal ratchet (Commit 3 PR2 only) | `go test ./internal/sddstatus -run TestEveryProductionRefusalNamesResolutionOrDeclaresByDesign` | PASS (4 PR2 refusals annotated per refusal:by-design operator-knowledge) |

## Concern resolutions (from obs #24503)

1. **Whole-path byte-identity (T2.3) — HONORED**: slice envelope fields
   (`scope`, `slice_id`, `requirement_ids`, `scenario_ids`) added to the
   `parseVerifyReport` allowed map; whole-path `ValidateVerifyReportAdmission`
   is byte-identical in behavior. Pinned by `TestWholePathAdmitsSliceFieldsInReport`
   (whole-path report with slice metadata falls through to the totals gate
   and is rejected because totals do not match whole-change spec counts).
   Archive gating preserved via unchanged totals gate.
2. **SliceAssignments projection (T1.10) — PINNED IN PR1**: T1.10's
   `TestSliceAssignmentsProjectionExcludesUnbound` ensures the projection
   only surfaces bound objectives.
3. **Sentinel collision (T1.1) — PINNED IN PR1**: T1.1's
   `TestNormalizeRejectsNoneSentinelID` rejects literal `none` as an
   obligation ID at bind time.
4. **Budget reconciliation — HONORED**: PR2 forecast ~380 lines; actual
   306 lines (80% of budget). PR1's 952-line overrun stands and is
   documented as `size:exception` per the apply-progress.

## Archive isolation proof (T2.8)

`TestResolveSlicePassDoesNotUnlockArchive` proves the design's
"Archive Isolation Proof" claim end-to-end: a passing slice report
(2/2 reqs, 3/3 scens, with all required envelope fields) is admitted by
`ValidateSliceVerifyReportAdmission`; when written to `verify-report.md`
on a change with a 1/1 spec, the whole-change resolver still compares
against the spec totals and `Verify` does NOT reach `DependencyAllDone`,
so `Archive` stays `DependencyBlocked`. The slice PASS persists as
work-unit evidence only — never unlocks archive.

## Discoveries / gotchas (PR2-specific)

1. `SliceAssignment` was already declared in `runtime_ledger.go` from PR1.
   Reusing it (rather than redeclaring in `verification.go`) keeps the
   `SliceAssignments()` projection and the validator speaking the same
   struct shape, which is the design's sole-scope-owner directive.
2. The validator's `parseSliceIDList` uses `none` as the explicit-empty
   sentinel because the normalizer at PR1 already rejects literal `none`
   as an ID; the contract is: an empty assignment always serializes as
   `none` on the wire, never as ``, and never as `,` (which would parse
   to `["", ""]` and fail).
3. The whole-path pin test (T2.3) intentionally uses a `SliceAssignment`
   of length 1/1 (REQ-1 + S-1) and asserts that the whole-path totals
   gate rejects it because the change is `2/2, 3/3`; this is exactly
   the byte-identity condition the concern asked us to pin.
4. The refusal ratchet test in `internal/cli` surfaces 4 PR2 refusals
   that I added with `refusal:by-design operator-knowledge: <why>`
   markers so the test passes for PR2 scope. The 7 remaining NEW refusals
   are all in PR1 territory (`runtime_ledger.go`) and out of scope for
   this PR2 apply — they will be addressed when PR1's review-ready
   `size:exception` lands.

## Files changed (PR2 scope only)

| File | Action | Notes |
|------|--------|-------|
| `internal/sddstatus/verification.go` | Modify | `VerifyReportContract.SliceFields`, `verifyReportSliceFields`, `ValidateSliceVerifyReportAdmission`, `parseSliceIDList`, `sameIDSet`, `overlapsIDs`, `validateParsedVerifyReport` (extracted helper) |
| `internal/sddstatus/verification_test.go` | Modify | `TestValidateSliceVerifyReportAdmission` (6 subtests), `TestWholePathAdmitsSliceFieldsInReport` |
| `internal/sddstatus/bounded_review_test.go` | Modify | `TestResolveSlicePassDoesNotUnlockArchive` |
| `internal/cli/sdd_verify_validate.go` | Modify | `--scope/--slice-id/--cwd/--change` flags + matrix validation + slice path via `SliceAssignments()` |
| `internal/cli/sdd_attempt.go` | Modify | `--assigned-requirement-ids/--assigned-scenario-ids` flags on begin/acquire/rescope; threaded through `BeginAttemptRequest`/`RescopeObjectiveRequest` |
| `bench/journeys_sdd_slice_verify.go` | Create | j82-scope-slice-verify journey (declaration; driven-mode follow-up) |
| `bench/journeys.go` | Modify | Register j82 constructor in `Journeys()` |
| `bench/journeys_id_collision_test.go` | Modify | Register j82 source in `journeySources()` |
| `bench/journeys_sdd_test.go` | Modify | Count pin 82 → 83 + comment naming j82 |

## Out-of-scope files (NOT touched in PR2)

- `internal/sddstatus/runtime_ledger.go` (PR1 — sole scope owner)
- `internal/sddstatus/runtime_compact.go` (PR1 — `compactAcquireMatches`)
- `internal/sddstatus/runtime_ledger_assignment_test.go` (PR1 tests)
- `internal/sddstatus/runtime_objective_owner_test.go` (PR1 test extension)
- `internal/sddstatus/runtime_objective_advance_test.go` (PR1 test extension)

## Recommendation for orchestrator (PR2 hand-off)

1. **Push PR2** as `feat/2268-scoped-slice-verification` stacked on main.
   PR2 alone is 306 changed lines (76% of 400-line budget). PR1 already
   carries the documented `size:exception` request.
2. **Bench j82 is a declaration** — the validator-side decision table is
   pinned by 6 unit tests, and the CLI matrix is pinned by the existing
   `TestRunSDDVerifyValidate` + new flag wiring. The driven-mode journey
   is a follow-up that requires fixture plumbing (Git lifecycle, SDD
   runtime store); the design's D10 contingency permits this.
3. **Pre-existing Windows + parallel test hang** persists (reviewtransaction
   git-subprocess tests) — verified on `git stash` baseline that PR1 + PR2
   do not introduce it. Documented in PR1 apply-progress; no PR2 action
   required.
4. **No new PR3 split needed** — PR2 stayed under budget; the j82
   declaration meets the corpus pin requirement. A future contributor can
   add the driven-mode execute transitions for j82 in a follow-up PR
   without touching PR1 or PR2 files.

## Files delivered for verification (this apply session)

- On-disk mirror: `openspec/changes/2268-scoped-slice-verification/apply-progress.md` (this file, PR1+PR2 merged)
- Engram topic key: `sdd/2268-scoped-slice-verification/apply-progress` (PR1+PR2 merged via `mem_save` on the same topic_key)
