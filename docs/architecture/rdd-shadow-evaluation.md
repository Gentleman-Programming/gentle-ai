# RDD shadow evaluation (Retired)

> **Status: Retired.** The Wave 1 shadow observer surface, including the `GENTLE_AI_RDD_SHADOW` environment switch, `shadow_observer.go`, and `ObserveShadowRelation`, was retired in RDD root simplification Wave 7 S2a (commit `17e40eb0`). The underlying relation algebra was promoted to `CandidateRelation` in `internal/reviewtransaction/candidate_relation.go`, and shadow observation is no longer part of the live review lifecycle. This document is retained for historical context regarding the Wave 1 evaluation and its exit evidence.

## Retirement Notice

During Wave 1, shadow evaluation ran the seven-value relation model alongside live review-lifecycle decisions to measure agreement before the target model became normative.

In Wave 7 S2a:
- The `GENTLE_AI_RDD_SHADOW` environment switch and `shadow_observer.go` were removed.
- Setting `GENTLE_AI_RDD_SHADOW=1` has no effect on current builds.
- The `gentle-ai.rdd-shadow/v1` stderr stream is no longer emitted.
- The relation algebra (`CandidateRelation`, `relateCandidates`) is normative and lives directly in `internal/reviewtransaction/candidate_relation.go`.

## Historical Design: What it did, and what it never did

| | |
|---|---|
| Did | Resolved a live `CandidateIdentity`, computed a `ShadowRelation`, classified authority graph health, and recorded the result in memory for that process only. |
| Never did | Blocked, delayed, or altered a live consent prompt, review-context result, receipt, or authority mutation. It never authorized, denied, blocked, or routed ordinary delivery or SDD archive. An internal shadow failure was swallowed and recorded as advisory evidence; it never surfaced as a live-path error. |

The disable switch was Wave 1's rollback boundary: unsetting `GENTLE_AI_RDD_SHADOW` made every live review-lifecycle result byte-identical to a build with no shadow code at all.

## Historical stderr line format

When enabled during Wave 1, each observed call site wrote exactly one line to stderr:

```text
gentle-ai.rdd-shadow/v1 gate=<GateKind> live_result=<GateResult> has_relation=<bool> shadow_relation=<ShadowRelation> no_live_counterpart=<bool> authority_health=<healthy|repairable|blocked> err=<quoted string, empty when none>
```

`has_relation=false` meant no frozen receipt was available at that call site (for example `start`, which had no prior receipt to compare against). `no_live_counterpart=true` meant the shadow relation was `ambiguous` or `unknown`, which structurally had no live review-lifecycle decision to compare against. The `gate` field named the review-context hook only; it was never a delivery gate.

## Historical exit evidence

Shadow evaluation's exit evidence, the differential matrix in `internal/reviewtransaction/testdata/shadow-differential-matrix.golden`, documented expected divergence classes and confirmed 0 unexplained divergences on `exact`, `compatible_base_advance`, and `provable_contraction`. This provided the clean exit bar required to proceed with subsequent migration waves.

## Current architecture reference

See `docs/architecture/rdd-root-simplification-design.md` for the full target architecture and the Migration waves table.
