# RDD shadow evaluation

Shadow evaluation runs the target seven-value relation model (`internal/reviewtransaction/shadow_*.go`) alongside a live review-lifecycle decision so Wave 1 can measure agreement before anything in the target model becomes normative. It **observes**; it never alters or blocks anything. Its identity, relation, graph, agreement, and divergence outputs are review-context evidence only and never control ordinary delivery or SDD archive.

## Quick path

1. Leave `GENTLE_AI_RDD_SHADOW` unset. This is the default and the only supported setting for normal use — shadow evaluation is off, and zero shadow code runs.
2. To opt in for local investigation, set `GENTLE_AI_RDD_SHADOW=1` before running a review-context hook (`post-apply`, `pre-commit`, `pre-push`, `pre-pr`, `release`) or `gentle-ai review start`/`status`.
3. Watch **stderr** (never stdout) for one `gentle-ai.rdd-shadow/v1` line per observed call.
4. If a line shows a divergence you did not expect, file it — see **Reporting a divergence** below.

## What it does, and what it never does

| | |
|---|---|
| Does | Resolves a live `CandidateIdentity`, computes a `ShadowRelation`, classifies authority graph health, and records the result — in memory, for that process only. |
| Never does | Blocks, delays, or alters a live consent prompt, review-context result, receipt, or authority mutation. It never authorizes, denies, blocks, or routes ordinary delivery or SDD archive. An internal shadow failure is always swallowed and recorded as advisory evidence; it can never surface as a live-path error. |

The disable switch is Wave 1's rollback boundary: unsetting `GENTLE_AI_RDD_SHADOW` makes every live review-lifecycle result byte-identical to a build with no shadow code at all (proven by `TestShadowObservationSwitchIsRollbackBoundaryGateByteIdentical` for the post-apply/pre-commit/release call sites and `TestNativePrePRGateShadowOnOffByteIdenticalForCompatibleBaseAdvance` for the pre-PR hook, the only one where shadow evaluation performs additional Git work). Zero shadow Git cost by default is separately proven by `TestNativePrePRGateWithShadowDisabledDerivesBaseAdvanceZeroTimes`.

## The stderr line format

When enabled, each observed call site writes exactly one line to stderr:

```
gentle-ai.rdd-shadow/v1 gate=<GateKind> live_result=<GateResult> has_relation=<bool> shadow_relation=<ShadowRelation> no_live_counterpart=<bool> authority_health=<healthy|repairable|blocked> err=<quoted string, empty when none>
```

`has_relation=false` means no frozen receipt was available at that call site (for example `start`, which has no prior receipt to compare against) — authority health is still recorded, but no relation is fabricated from absent evidence. `no_live_counterpart=true` means the shadow relation is `ambiguous` or `unknown`, which structurally has no live review-lifecycle decision to compare against. The `gate` field names the review-context hook only; it is never a delivery gate.

## Reporting an observed divergence

Shadow evaluation's own exit evidence — the differential matrix in `internal/reviewtransaction/testdata/shadow-differential-matrix.golden` — already documents every *expected* divergence class (Amendment B's no-input degradation, and the shadow-only `unrelated` value's vocabulary gap against the five-value live classifier). If a stderr line shows a divergence outside those documented classes:

1. Copy the exact `gentle-ai.rdd-shadow/v1` line (it contains no candidate content, only relation/health enum values).
2. Open an issue: <https://github.com/Gentleman-Programming/gentle-ai/issues/new/choose>. Include the line, the gate or command that produced it, and whether it repeats.
3. An unexplained divergence on `exact`, `compatible_base_advance`, or `provable_contraction` blocks Wave 2 entry by design (`docs/architecture/rdd-root-simplification-design.md`, Migration waves) — reporting it is how that planning boundary gets enforced. It does not block ordinary delivery or SDD archive.

## Next step

See `docs/architecture/rdd-root-simplification-design.md` for the full target architecture and the Migration waves table this evaluation gates.
