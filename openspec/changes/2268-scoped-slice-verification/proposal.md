# Proposal: Scoped Slice Verification (#2268)

## Intent

`sdd-verify-validate` admission binds report totals to whole-change spec counts. An honest slice PASS with later slices pending is rejected; the only remaining options are a stale failed report or fabricated completion.

Real occurrences (issue #2268 comments):

- **v2.3.0-rc.2**: bounded chained slice fully proven; whole-change evidence at 3/4 requirements, 9/10 scenarios; PASS and PASS WITH WARNINGS both rejected.
- **2.4.0-rc.1**: foundation slice independently verified (33 tests, 119 assertions) completing 0/8 requirements, 0/13 scenarios; passing report cannot persist; claiming 8/8 would fabricate completion.

## Maintainer Approval Contract (2026-08-09, dnlrsls) — VERBATIM

> The existing provider-owned `RuntimeObjective` and work-unit identity is the sole slice identity and scope owner. Do not add a free-form slice registry or parallel source of truth. Each candidate/revision must receive an immutable assignment of exact requirement and scenario IDs plus derived totals. Unknown identity, partial metadata, mismatch, overlap, duplicate coverage, or altered assignment must fail closed.
>
> A slice PASS persists only work-unit evidence. It never replaces the whole-change verify report or implies `verify all_done` or archive readiness. Final whole-change verify/archive behavior remains unchanged: all tasks and global obligations must pass. Zero-obligation work units are allowed only when provider-owned metadata explicitly records `0/0`, includes a concrete evidence goal, and has non-empty passing execution evidence; they receive no credit toward global obligations. Preserve current incomplete FAIL contradiction rules and current whole-change CLI/report behavior by default.
>
> Acceptance tests: `ValidateVerifyReportAdmission`, `RunSDDVerifyValidate`, `RuntimeObjectiveIsSoleWorkUnitScopeOwner`, `RuntimeLedgerAdvancesDistinctWorkUnitAfterPassedObjective`, `ResolveFinalVerifyWaitsForAllTasks`, and `ResolveApplyVerifyArchiveGates`, plus one chained SDD journey proving slice A can persist while B remains pending and archive opens only after final whole-change verification.

## Scope

### In Scope

1. `AssignedRequirementIDs` / `AssignedScenarioIDs` (`omitempty` JSON) on `BeginAttemptRequest` + `RuntimeObjective`; immutable once set; derived totals.
2. Propagation through begin/rescope event structs, apply functions, replay validators, normalize updates; `CompactAcquireRequest` inherits via embed.
3. New exported `ValidateSliceVerifyReportAdmission` — non-breaking; `ValidateVerifyReportAdmission` untouched.
4. Envelope optional fields: `scope`, `slice_id`, `requirement_ids`, `scenario_ids`; unknown identity, partial metadata, mismatch fail closed.
5. CLI: `sdd-verify-validate --scope/--slice-id`; assignment flags on `sdd-attempt begin/acquire/rescope`.
6. Overlap/duplicate/altered-assignment detection via caller-supplied `knownAssignments`.
7. Zero-obligation path: explicitly empty `0/0` arrays + concrete evidence goal + non-empty passing evidence; no credit toward global obligations.
8. Archive isolation: `resolveDependencies` unchanged; slice PASS persists as work-unit evidence only.
9. Current incomplete-FAIL contradiction rules and whole-change CLI/report behavior preserved by default.

### Out of Scope

- Any change to default whole-change verify/archive behavior.
- Sibling #2293; adjacent validator defects #2500, #2828.
- Free-form slice registry or parallel source of truth (rejected by maintainer).

## Capabilities

### New Capabilities

- `sdd-scoped-slice-verification`: provider-owned slice identity, immutable obligation ID assignment, slice-scoped verify admission, overlap fail-closed detection, zero-obligation units, archive isolation.

### Modified Capabilities

- None (no existing spec covers sddstatus verify admission; whole-change behavior preserved).

## Approach

Exploration Approach 1 (only design satisfying the contract): obligation ID fields on provider-owned `BeginAttemptRequest`/`RuntimeObjective` as sole scope owner; pure validator receives assignment + `knownAssignments`; status layer keeps comparing against whole-change counts, so a slice PASS can never become `verify all_done`.

## Delivery Plan (auto-chain, stacked-to-main, 400-line budget)

~530 estimated lines → TWO chained PRs, each targeting `main`, merged in order, independently reviewable, main stays green:

| PR | Content | ~Lines |
|----|---------|--------|
| PR1 | ID assignment fields on `BeginAttemptRequest`/`RuntimeObjective`, event extensions, replay validators, normalize updates, owner-test extension, ledger ID-carry test | 250 |
| PR2 | `ValidateSliceVerifyReportAdmission`, envelope fields, CLI flags, overlap/duplicate/altered detection, zero-obligation metadata, acceptance tests, chained SDD journey | 280 |

## Affected Areas

| Area | Impact |
|------|--------|
| `internal/sddstatus/runtime_ledger.go` | Modified — ID fields, events, replay validators, normalize |
| `internal/sddstatus/verification.go` | Modified — envelope fields, new validator, detection helpers |
| `internal/cli/sdd_verify_validate.go` | Modified — `--scope`, `--slice-id` flags |
| `internal/cli/sdd_attempt.go` | Modified — assignment flags on begin/acquire/rescope |
| `internal/sddstatus/status.go` | Guarded — archive isolation unchanged (regression tests) |
| `internal/sddstatus/runtime_compact.go` | Unchanged — inherits via embed |
| `bench/` journey + test files | New — acceptance tests, chained journey |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Overlap detection needs all assignments | Med | Validator stays pure; caller supplies `knownAssignments` (CLI/status aggregates) |
| Rescope must carry/reassign IDs | Med | Rescope request carries assignment; replay validator enforces |
| Zero-obligation explicit-empty vs absent arrays | Med | Distinguish `[]` from absent in JSON; fail closed on ambiguity |
| Persisted ledger backward compatibility | Low | `omitempty` → legacy records replay byte-identically |

## Rollback Plan

PR1 and PR2 revert independently (`git revert` merge commit). Fields are additive/`omitempty`; `--scope` defaults to `whole`, so reverting PR2 restores exact current CLI behavior. No data migration.

## Dependencies

- Issue #2268 `status:approved` (granted 2026-08-09). Base: upstream/main `3c6a6341`.

## Acceptance Criteria (maintainer-named tests)

1. `ValidateVerifyReportAdmission` — existing tests stay green (backward compatibility).
2. `RunSDDVerifyValidate` — green + new `--scope slice --slice-id` coverage.
3. `RuntimeObjectiveIsSoleWorkUnitScopeOwner` — green; fields only on `BeginAttemptRequest`, compact inherits via embed.
4. `RuntimeLedgerAdvancesDistinctWorkUnitAfterPassedObjective` — green + ID-assignment variant.
5. `ResolveFinalVerifyWaitsForAllTasks` — green + proves slice PASS does not unlock archive.
6. `ResolveApplyVerifyArchiveGates` — green + proves whole-change verify still required.
7. Chained SDD journey — slice A persists while B pending; archive opens only after final whole-change verification.

## Success Criteria

- [ ] All 7 acceptance tests pass; whole-change defaults byte-compatible.
- [ ] Both chained PRs merged under 400 lines each, main green after each.
