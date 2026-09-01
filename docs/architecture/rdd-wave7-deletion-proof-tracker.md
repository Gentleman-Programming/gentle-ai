# Wave 7 Deletion Proof Tracker — `superseded-by-design` Backlog Rows

**Written at:** WU3 (S9a), the add-only bracket, before any deletion slice
(WU4-WU19) has landed.

`docs/architecture/rdd-backlog-disposition.md` already classifies five
items `superseded-by-design` against the design's "Retire legacy active
paths" coverage row (Wave 7 REMOVE disposition):

| # | Kind | Title |
|---|---|---|
| [gentle-ai#1455](https://github.com/Gentleman-Programming/gentle-ai/issues/1455) | Issue | fix(review): reject completed tasks with empty reviewer results |
| [gentle-ai#1462](https://github.com/Gentleman-Programming/gentle-ai/issues/1462) | Issue | fix(review): quarantine seven invalid legacy-v1 authority records |
| [gentle-ai#1570](https://github.com/Gentleman-Programming/gentle-ai/issues/1570) | Issue | fix(review): expose legacy HEAD required by repair-legacy-alias |
| [gentle-ai#1549](https://github.com/Gentleman-Programming/gentle-ai/pull/1549) | PR | fix(review): reject completed tasks with empty reviewer results, distinguish from nested-envelope |
| [gentle-ai#1550](https://github.com/Gentleman-Programming/gentle-ai/pull/1550) | PR | fix(review): reject completed tasks with empty reviewer results |

That document's own "Closure audit protocol" step 4 already names the exact
proof obligation for these rows: *"for #1455/#1462/#1570: proof that the
legacy mutation lifecycle no longer exists and historical records parse
read-only."* This tracker records WHERE that proof lands inside Wave 7's
own work-unit sequence, so step 4 is a lookup at close-out (WU20), not a
re-investigation.

## Proof mapping (this wave's own inventory rows, per design.md)

| Backlog row | Retired surface it names | Wave 7 inventory row(s) | Deletion slice | Proof instrument |
|---|---|---|---|---|
| #1455 / #1549 / #1550 | Legacy reviewer-result completion/nested-envelope handling on the legacy mutation path | Rows 6-19 (reconcile/quarantine/repair verb clusters) | S3-S5 (WU7-WU17) | `.refusal-ratchet-baseline.txt` rows 181-186, 222-227, 664-717, 955-1009 shrink to zero for these verbs; `legacy_readonly_guard_test.go` (RG.1b) lists zero remaining reachable verbs after WU19 |
| #1462 | Quarantine of invalid legacy-v1 authority records (`quarantine-legacy`) | Row 14 (`RunReviewLegacyQuarantine`), row 17 (`legacy_quarantine.go`) | S5 (WU14, WU16) | Deletion proof entries in WU14/WU16's own exit evidence naming what `legacy_quarantine.go`/its tests covered and that quarantine-residue reading (D5 retained) still parses |
| #1570 | `repair-legacy-alias`'s legacy-HEAD exposure | Row 16 (`RunReviewLegacyAliasRepair`), row 17 (`legacy_alias_repair.go`) | S5 (WU14, WU16) | Deletion proof entries in WU14/WU16's own exit evidence |

## Forensic-read half (D5), proven now, not deferred

The other half of the closure-audit step-4 obligation — "historical records
parse read-only" — is ALREADY proven at WU1 time by
`TestLegacyReadOnlyGuardRetainedSymbolsDeclared` (RG.1a,
`internal/reviewtransaction/legacy_readonly_guard_test.go`), which is GREEN
from WU1 forward and stays GREEN through every subsequent deletion slice:
`parseLegacyBinding`, `parseBinding`, `bindingBytes`/`bindingDigest`/`bindingPath`,
`AuthoritativeStore`/`LoadChain`, `NewLegacyReadOnlyError`, and
`candidate_decline.go`'s parser all remain declared and reachable. This is
the read-half of the #1462/#1570 proof obligation; only the mutation-half
(the verb dispatch reachability, RG.1b) is still pending.

## Status at WU3 time (this commit)

- Mutation-lifecycle-gone proof: **PENDING** — RG.1b
  (`TestLegacyReadOnlyGuardMutationVerbsUnreachable`) is intentionally RED
  until WU19; it lists all 11 still-reachable legacy verbs today.
- Forensic-read-still-works proof: **SATISFIED** — RG.1a is GREEN as of
  WU1 and covers every D5 retained symbol.
- Closure audit protocol step 5 (closing the actual GitHub issues/PRs with
  a linking comment): explicitly **NOT** run by this wave's apply phase —
  it is a maintainer action gated on the whole wave's exit evidence, per
  `rdd-backlog-disposition.md`'s own framing ("advisory and closes
  nothing"). This tracker records readiness; it does not perform closure.

## What WU20 does with this tracker

At S9b close-out, WU20 re-reads this table, confirms RG.1b is fully GREEN
(zero legacy verbs reachable) and every WU14-WU17 exit-evidence deletion
proof exists, then updates `docs/architecture/rdd-backlog-disposition.md`'s
own closure audit protocol row for step 4 as satisfied for these five
items — still without performing step 5 (the actual GitHub closure),
which stays a separate maintainer action.

## Reconciliation at WU20 close-out (verify W4 — corrected, was not done as originally worded)

WU20 did not actually perform the paragraph above as written, and the
paragraph's own condition — "RG.1b is fully GREEN (zero legacy verbs
reachable)" — no longer holds in the blanket form this tracker assumed at
WU3 time. Both need an honest correction, not a silent checkmark:

- **"Zero legacy verbs reachable" was imprecise.** At WU3 time this tracker
  (and RG.1b) treated all 11 candidate verbs named in the WU3-era
  reachability list as one undifferentiated "legacy" bucket, on the
  design's original assumption that WU18 (switch removal) would land and
  retire the whole bucket together. WU18 was deferred (see the
  `rdd-single-lifecycle` spec amendment), and WU19's honest D4
  classification — done against the actual, not the assumed, tree — found
  only 5 of those 11 verbs are truly legacy-only and were in fact retired
  this wave (`reconcile-authority`, `reconcile-authority-batch`,
  `quarantine-legacy`, `quarantine-legacy-fix-scope`, `repair-legacy-alias`
  — landed WU14/WU16). The remaining 6 (`invalidate`, `abandon`, `recover`,
  `reclaim`, `dispose-result`, `reopen-results`) turned out to be live,
  active compact-v2 mutation surface, not legacy code awaiting deletion —
  they stay reachable by design, gated on the still-present switch, and
  RG.1b now asserts that reachability positively
  (`TestLegacyReadOnlyGuardLiveCompactV2VerbsRemainReachable`) rather than
  demanding it go to zero.
- **Scoped to what #1455/#1462/#1570 actually claim, the proof still
  holds.** None of the 6 live D4 verbs are the retired surface those three
  issues name. #1462 names `quarantine-legacy`; #1570 names
  `repair-legacy-alias`; #1455/#1549/#1550 name the reconcile/quarantine/
  repair verb clusters (rows 6-19) — all five of those specific verbs are
  the ones confirmed unreachable by the narrowed
  `TestLegacyReadOnlyGuardMutationVerbsUnreachable` (RG.1b). So the
  closure-audit step-4 proof obligation for these three specific backlog
  rows IS satisfied; it was this tracker's broader "zero legacy verbs
  reachable" framing — not the underlying proof for #1455/#1462/#1570 —
  that was wrong.
- **`rdd-backlog-disposition.md`'s closure audit protocol was not edited.**
  It has no per-item "satisfied" checkbox to flip (step 4 is prose applying
  uniformly to all five `superseded-by-design` rows, not a row of its own);
  a maintainer applying step 4 today should read it as satisfied for
  #1455/#1462/#1570/#1549/#1550 specifically, on the corrected evidence
  above, not on this tracker's original unscoped claim.
