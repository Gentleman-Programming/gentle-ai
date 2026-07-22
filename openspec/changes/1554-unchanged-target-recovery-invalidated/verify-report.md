# Verify Report: Relax CLI unchanged-target recovery gate for `--disposition invalidated`

**Change**: `1554-unchanged-target-recovery-invalidated`
**Mode**: OpenSpec file artifacts (proposal.md, spec.md, design.md, tasks.md all present)
**Verdict**: PASS WITH WARNINGS (0 CRITICAL, 2 WARNING, 0 SUGGESTION) — **both WARNINGs closed post-report, see Addendum below.**

## Command Evidence

| Command | Result |
|---|---|
| `go build ./...` | exit 0, no output |
| `go vet ./...` | exit 0, no output |
| `go test ./... -count=1` (full repo, no cache) | exit 0, all packages `ok`, including `internal/cli` (176.188s) and `internal/reviewtransaction` (99.757s) |
| `go test ./internal/cli/... -run TestUnchangedTargetRecovery -v -count=1` | all 5 subtests PASS |
| `go test ./internal/reviewtransaction/... -run TestEscalatedRecoveryRequiresChangedTarget -v -count=1` | PASS |
| `git diff --stat origin/main HEAD` | 6 files changed, 500 insertions(+), 1 deletion(-) |
| `git diff --numstat -- internal/cli/review_facade.go` | 2 insertions, 1 deletion (net 3-line diff) |

## Check-by-Check

### 1. Design's exact prescribed edit — PASS

Diff at `internal/cli/review_facade.go:553-555`:

```go
if !*releaseScope && reviewtransaction.RecoveryDisposition(*disposition) != reviewtransaction.RecoveryInvalidated &&
	(baseDiff || overlay) && snapshot.Identity == predecessorRecord.State.InitialSnapshot.Identity {
	return errors.New("recovery scope has not changed")
}
```

Matches design.md's prescribed AFTER state verbatim, at the design's own corrected line number (553-555, not the proposal's stale 518-519 — design explicitly documents this correction and it holds). The base-tree guard at 550-552 and the unrelated `--committed-only`/`--base-ref` check at 518-519 are untouched (confirmed via diff — no other hunks in this file).

### 2. Every spec requirement/scenario has a passing covering test — PASS, with one caveat (see Warning 2)

| Spec requirement/scenario | Covering test | Result |
|---|---|---|
| Unchanged-target invalidated admitted | `TestUnchangedTargetRecovery_InvalidatedAdmitted` | PASS |
| Base-tree mismatch still rejected for invalidated | `TestUnchangedTargetRecovery_BaseMismatchStillBlocked` | PASS |
| scope_changed CLI gate unchanged | `TestUnchangedTargetRecovery_ScopeChangedStillBlocked` | PASS |
| escalated CLI/package gates unchanged | `TestUnchangedTargetRecovery_EscalatedStillBlocked` | PASS |
| Base-tree mismatch guard for all dispositions | Only tested with `--disposition invalidated` | Gap — see Warning 2 |
| Regression suite covers all three dispositions | All 3 dispositions present in one fixture-sharing test file | PASS |
| Changed-target invalidated still succeeds (no-regression happy path) | `TestUnchangedTargetRecovery_ChangedTargetInvalidatedStillSucceeds` | PASS |

### 3. scope_changed / escalated provably untouched, including no-persistence proof — PARTIAL (Warning 1)

- `TestUnchangedTargetRecovery_EscalatedStillBlocked` asserts: exact error string `"recovery scope has not changed"`, byte-identical predecessor state file before/after (`bytes.Equal(before, after)`), AND `os.IsNotExist` on the would-be successor authority path. This is real, load-bearing proof — not just "tests pass".
- `TestUnchangedTargetRecovery_ScopeChangedStillBlocked` asserts ONLY the exact error string. It does **not** check predecessor-state immutability or successor-path non-existence.
- This asymmetry is not an accidental gap in the implementation — it mirrors design.md's own Testing Strategy table verbatim (escalated: "Assert error non-nil AND no successor persisted"; scope_changed: "Assert exact error string unchanged" only) and tasks.md 1.3 vs 1.4. So the code/tests match the design's own (lower) bar for scope_changed. Flagging because the verification prompt specifically asked for persistence proof on "the still-blocked cases" (plural) and only one of the two has it.
- Structural mitigation: `RunReviewRecover` returns the error at line 555 before reaching any snapshot-risk-assessment or persistence code further down the function, so scope_changed rejection is provably pre-persistence by control flow even without an explicit file-system assertion. This is a real but weaker (structural, not test-asserted) guarantee.

### 4. #1419/#1429 escalated changed-target invariant unmodified — PASS

- `git diff origin/main HEAD --name-only -- internal/reviewtransaction/` is **empty** — the package is untouched, not just "test passes coincidentally."
- `TestEscalatedRecoveryRequiresChangedTarget` (in `internal/reviewtransaction/compact_store_test.go`) passes unmodified, verified with `-run` isolation and `-count=1` (no cache).

### 5. tasks.md checkboxes spot-checked against real work — PASS

- 1.1-1.6: test file `internal/cli/review_recovery_unchanged_target_test.go` exists with exactly the 5 named tests plus the shared fixture helper described.
- 1.7 (RED-first proof): confirmed via `git show 29a05a4:internal/cli/review_facade.go` — at the RED test commit, the gate condition was still the pre-fix, unconjuncted form (`if !*releaseScope && (baseDiff || overlay) && snapshot.Identity == ...`). Commit order is test-first (`29a05a4` test) → `1b7a59c` (docs) → `11737d0` (fix) → `53dfc88` (task checkboxes). This is genuine TDD ordering, not retrofitted.
- 2.1: exact conjunct confirmed at 553-555, matches the line-512 `RecoveryDisposition(*disposition)` conversion precedent.
- 2.2, 3.1, 4.1-4.3: independently re-run in this verification pass (see Command Evidence), all pass.
- 3.2 (diff scoped to conjunct + new test file only): confirmed — `git diff origin/main HEAD --name-only` touches exactly `internal/cli/review_facade.go`, `internal/cli/review_recovery_unchanged_target_test.go`, and 4 openspec markdown files. No other production file changed.
- 4.4: `git diff --stat` = 500 insertions total, well under the 400-line authored-code budget (the budget counts code, not docs; production code diff is 3 lines).
- 4.5 (no push): `git status -sb` shows `fix/1554-unchanged-target-recovery...origin/main [ahead 4]`, tracking branch not pushed; reflog shows no push events for this branch.

### 6. Full re-verification — PASS

- `go build ./...`: exit 0.
- `go vet ./...`: exit 0.
- `go test ./... -count=1` (full repo, cache disabled): exit 0, all packages `ok`, zero failures, including the touched `internal/cli` and adjacent `internal/reviewtransaction`.
- `git diff --stat origin/main HEAD`: 6 files changed, 500 insertions(+), 1 deletion(-) — matches the claimed ~500 lines total. Production code change in `review_facade.go` is exactly 2 insertions + 1 deletion (net 3 changed lines), matching the "~3 lines of actual production code" claim; the remainder is the new test file (154 lines) and 4 SDD markdown docs (proposal/spec/design/tasks, 344 lines).

## Issues

**CRITICAL**: none.

**WARNING**:
1. Persistence-immutability proof (byte-identical state file + absent successor path) exists only for the `escalated` still-blocked test, not for `scope_changed`. This matches design.md's own testing strategy table, so it is not a deviation from the design — but it is a real asymmetry a reviewer should be aware of before treating both "still-blocked" cases as equally proven. Mitigated structurally (early `return` before any persistence code runs) but not test-asserted for scope_changed.
2. The spec's "Base-tree mismatch guard MUST remain enforced for all dispositions" scenario is only empirically exercised with `--disposition invalidated` (`TestUnchangedTargetRecovery_BaseMismatchStillBlocked`). No test in the repo exercises the base-tree-mismatch guard combined with `scope_changed` or `escalated`. The guard's code itself (`review_facade.go:550-552`) is confirmed untouched by this diff, so there is no code regression risk — but the spec's "for any `--disposition` value" claim is not backed by a scope_changed/escalated-specific test.

**SUGGESTION**: none.

## Final Verdict

**PASS WITH WARNINGS.** The single-conjunct production edit exactly matches design.md's prescribed fix at the corrected line numbers, the #1419/#1429 escalated invariant package is provably untouched (empty diff) and its lock test passes unmodified, all 5 new regression tests plus the full existing suite pass with a clean cache, and every checked tasks.md box corresponds to real, independently-reproduced work. The two WARNINGs are test-coverage asymmetries already implicitly scoped by design.md itself, not implementation defects — safe to proceed to human review, but worth calling out explicitly rather than smoothing over.

## Addendum (post-report): both WARNINGs closed

`test(review): close coverage gaps flagged by sdd-verify` landed after this report was
written and closed both WARNINGs. Re-verified:

- **Warning 1 resolved**: `ScopeChangedStillBlocked` now asserts the same
  persistence-immutability proof as the escalated test (byte-identical state file
  before/after, plus `os.IsNotExist` on the would-be successor path).
- **Warning 2 resolved**: `BaseMismatchStillBlockedForScopeChanged` and
  `BaseMismatchStillBlockedForEscalated` now cover the base-tree guard for those two
  dispositions, alongside the pre-existing invalidated case.
- Suite is 7 tests, not 5, all passing (`-run TestUnchangedTargetRecovery -v`, 7/7 PASS).

Revised verdict: **PASS, no open warnings.**
