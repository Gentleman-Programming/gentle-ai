```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:254edc0b99d18508f0b40e59215cac96149534b98ac3bc8e46e69eafb4b6a114
verdict: pass_with_warnings
blockers: 0
critical_findings: 0
requirements: 17/17
scenarios: 26/26
test_command: go test ./... -count=1
test_exit_code: 0
test_output_hash: sha256:8b5acb4c546aad16b0b9bca8242ec8a5a6ede07958a984a72826c5c6353dff0b
build_command: go vet ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report — Cycle 3 (final closing pass)

**Change**: `rdd-root-simplification-wave5` (Gate Cutover) | **Mode**: full spec-driven verification (proposal + design + tasks + 5 delta specs, one amended) | **Strict TDD**: active | **Store**: hybrid
**Candidate**: chain tip `63c0583a` on `feat/rdd-wave5-f3-findings-admission`
**Fix cycle under review**: `8e5f287a..63c0583a` — `ff3b2a72` (C-E), `a793e02c` (W-8), `63c0583a` (W-5/W-6/W-2 docs)
**Attempt authority (echoed, not settled)**: `sha256:6bd32ee5303a2dc567c88f1d247afa4b068acee37ce671a52c7c17dc1588f25c`

**Verdict**: **PASS WITH WARNINGS** — 0 CRITICAL, 3 WARNING (new), 3 SUGGESTION.

C-E is genuinely closed and mutation-proven by my own strip. W-8 is closed, and I executed its named continuation to completion. The W-2 amendment is internally consistent and every test it cites exists and passes. W-4 closed as a side effect. Every requirement is now covered by a passing runtime test or explicitly amendment-accepted.

The three new warnings are all v2-vs-v3 admission-strictness divergences on the *new, activation-gated* lineage. None is a regression introduced by this cycle's own logic — each traces to `AdmitCandidateCausalFindings`' documented, spec-cited routing that the coordinator's design decision explicitly directed the fix to reuse — but two of them are fail-open in effect and I am naming them rather than letting them pass silently.

## Command Evidence (verbatim, cycle 3)

| Command | Exit | Result |
|---|---|---|
| `go build -trimpath -o <scratch>/gentle-ai-c3 ./cmd/gentle-ai` (at `63c0583a`) | 0 | `BUILD_OK` |
| `go test ./... -count=1` (root module) | 0 | 63 `ok`, zero `FAIL`; hash `sha256:8b5acb4c546aad16b0b9bca8242ec8a5a6ede07958a984a72826c5c6353dff0b` |
| `go test ./... -count=1` (bench module) | 0 | `ok github.com/gentleman-programming/gentle-ai/bench 0.183s` |
| `go run . run --binary <fresh> --out <scratch>/c3-bench-core.json` | 0 | `journeys: 59 completed, 0 unsupported, 0 failed` |
| `go run . run --binary <fresh> --axis all --out <scratch>/c3-bench-axis.json` | 0 | `journeys: 79 completed, 1 unsupported, 0 failed` (`j57`, declared source-coupled) |
| `gofmt -l .` | 0 | empty |
| `go vet ./...` | 0 | empty |
| `bash scripts/deadcode-ratchet.sh` | 0 | `no new unreachable functions` |
| `go test ./internal/cli/ -run TestEveryProductionRefusalNamesResolutionOrDeclaresByDesign -count=1` | 0 | `--- PASS` |
| `go test ./internal/reviewtransaction/ -run TestGateBoundaryMatrix_35Cells -count=1` (no `-update`) | 0 | `--- PASS (10.35s)`, golden unchanged (9/35 wired, 26 skips), `git status` clean |

Worktree clean throughout; every mutation ran on a `git archive` copy in scratch.

## 1. C-E — CLOSED (A/B repro re-run on a fresh `63c0583a` binary)

Identical candidate (`func boom() int { return 1/0 }`), identical finding (`severity: BLOCKER`, `evidence_class: deterministic`, `causal_disposition: introduced`, id `R3-boom-div-zero`):

| lineage | cycle 2 | cycle 3 |
|---|---|---|
| v2 / compact | `state: "correction_required"` | `state: "correction_required"` (unchanged) |
| v3 / new lineage | `state: "approved"` — fail-open | **`state: "escalated"`, `admitted_finding_ids: ["R3-boom-div-zero"]`** |

Severity/causality behaviour on v3:

| finding | v3 result |
|---|---|
| `WARNING` / `pre-existing` | `approved` — stays info |
| `SUGGESTION` / `base-only` | `approved` — stays info |
| `BLOCKER` / `introduced` | `escalated`, admitted |

`--admission-findings` override still works and still wins: with a captured BLOCKER present, `finalize --captured-results --admission-findings <empty-list>` yields `approved` and the gate then returns `allow` / `exact` — the explicit override semantics `review_facade.go:2195-2201` documents (`--admission-findings` takes precedence, captured findings are the fallback) are preserved.

**Mutation proof, re-run by me** (strip of the admission-wiring half — removed the `else if *capturedResults { findings = newRecord.Authority.CapturedFindingEvidence() }` branch, rebuilt the binary):

```
########## MUTANT: v3 + BLOCKER ##########
"state": "approved"          <- the exact original fail-open returns
########## MUTANT: named test ##########
--- FAIL: TestReviewFacadeCaptureResultNewLineage_CandidateCausalBlockerEscalates (0.08s)
```

**W-4 closed as a side effect**: the one-shot conflict is now CLI-reachable, because findings participate in the idempotency comparison. Capturing the same lens with different findings is refused (`lens already captured with a different binding; capture is one-shot per lens`) while an identical resubmission stays an idempotent no-op. A reviewer cannot silently overwrite a blocker with a clean result.

## 2. W-8 — CLOSED (continuation executed to completion)

Zero captures on a 4-lens high-tier v3 authority:

```
Error: new-lineage finalize requires captured results for every frozen selected lens before approving:
missing review-risk, review-resilience, review-readability, review-reliability; capture each with
`gentle-ai review capture-result --cwd <repo> --lineage w8 --target <target> --lens <lens> --order <order>
--input <result.json>` (run with --preflight first to discover the exact subject hash to echo back), then
retry `gentle-ai review finalize --cwd <repo> --lineage w8 --captured-results=true`
```

Genuine partial (2 of 4 captured) names only what is actually missing: `missing review-readability, review-reliability`. I then executed the continuation — captured lenses 2 and 3 — and finalize returned `state: "approved"`.

`.git/gentle-ai/defect-reports/` **does not exist** in that repository at any point. In cycle 2 this same state wrote a report titled *"Gentle AI reached a tool-internal fault state (`operation_outcome_unknown`) that should never happen"*.

## 3. W-2 amendment — internally consistent, cited tests real and passing

The amendment block sits under the "Outcome equivalence proven by matrix, not byte-diff" scenario. It states the letter/intent split honestly (9/35 wired, all compact/v2), names the substitute proof, attributes the acceptance to the coordinator's own fix-cycle-3 instruction, and keeps the matrix as the long-term vehicle with its count tracked in `tasks.md`.

All three cited tests exist in `internal/reviewtransaction/legacy_projection_test.go` and pass:

```
--- PASS: TestEvaluateLegacyGateAllowsExactAtAllFiveGates (0.07s)
--- PASS: TestEvaluateLegacyGateAllowsExactAndDeniesChanged (0.06s)
--- PASS: TestEvaluateLegacyGateValidatesReceiptFromAnInFlightCorrection (0.07s)
```

I read the load-bearing one rather than trusting its name: it iterates `{GatePostApply, GatePreCommit, GatePrePush, GatePrePR, releaseInput}` — release carrying all five real release artifacts — and fails unless every one returns `GateAllow`. The claim matches the code.

One limit of my verification, stated plainly: the amendment cites a coordinator instruction I cannot read. I verified it is internally consistent, factually accurate about the matrix state and the tests, and consistent with the coordinator message I did receive ("W-2 spec amendment citing the coordinator decision"). I did not independently confirm the quoted instruction text.

The related W-5/W-6 doc cleanup is also honest: the phantom `..._Allow_ExactReceiptGovernsDelivery` row is replaced with tests that exist (I re-checked `TestReviewFacadeCaptureResultNewLineage_MediumTierFinalizeAllowsAllFiveGates` really covers release with artifacts — it does), and task 1.4 stays unchecked with a verified explanation (Wave 4 *is* archived on `main` at `da3216d9`, by a commit this branch's history does not contain — so the box correctly is not ticked for an action this branch never performed).

## 4. Adversarial pass over `8e5f287a..63c0583a` — 0 new CRITICAL, 3 new WARNING

The diff is small and disciplined: `Findings []FindingEvidence` on `NewLineageCapturedResult`, `CapturedFindingEvidence()`, `MissingCapturedLensNames()`, a wire-shape converter, one `errors.Is` classification branch, and `Validate()` extensions. `reflect.DeepEqual` on findings is what makes the one-shot conflict real. No new writer, no new lock, no new gate branch.

Probing the admission edge of the new channel found three v2/v3 divergences:

**W-9 (CONFIRMED, fail-open): `causal_disposition: unknown` on a severe finding approves on v3, escalates on v2.**

```
v3: BLOCKER + evidence_class inferential + causal_disposition "unknown"  ->  state "approved"
v2: identical payload                                                    ->  state "escalated"
```

Root cause is `AdmitCandidateCausalFindings` (`candidate_causal_admission.go:36`), whose own doc comment routes `CausalUnknown` and the zero value to follow-up deliberately, citing spec `rdd-review-core-transitions` "Candidate-Causal Admission Only". Fix cycle 3 reused it verbatim, exactly as the coordinator's design decision directed and exactly as the pre-existing `--admission-findings` channel does. v2's stricter outcome comes from its separate ArtifactSubject admission pipeline, which the coordinator explicitly scoped out. So this is not a defect this cycle introduced — but it does contradict the root orchestration contract's "unknown escalates", and on v3 an honest "BLOCKER, causality unproven" review yields an approved receipt every gate then honours.

**W-10 (CONFIRMED, fail-open, the cheapest to fix): the v3 capture path accepts a severe finding that its own published schema forbids.**

```
v3: BLOCKER with NO evidence_class and NO causal_disposition -> capture ACCEPTED -> finalize "approved"
v2: identical payload -> capture REFUSED:
    "reviewer artifact admission incomplete: severe reviewer finding requires supported
     evidence_class and causal_disposition"
```

`gentle-ai review schema reviewer` requires both fields when `severity` is `BLOCKER`/`CRITICAL` (the `allOf`/`if`/`then` clause). The v3 capture path does not enforce that conditional, and the missing `causal_disposition` then routes to follow-up via W-9's rule. This one lives squarely in code fix cycle 3 touched (`newLineageCapturedFindings` and the v3 capture validator) and is closed by mirroring v2's existing refusal at capture time.

**W-11 (CONFIRMED, fail-closed): a `WARNING`-severity finding with a candidate-causal disposition escalates on v3 but not on v2.**

```
v3: WARNING + causal_disposition "introduced" -> state "escalated"
v2: identical payload                          -> state "validating" (non-blocking)
```

`AdmitCandidateCausalFindings` partitions by causality only and never consults severity, so v3 over-blocks where the root contract says "WARNING/SUGGESTION remain info". Fail-closed, so not a fail-open risk, but it misses the coordinator's stated criterion that WARNING/SUGGESTION stay info on both paths.

**Why none of these is CRITICAL.** Each traces to a documented, spec-cited admission function the coordinator's own decision directed the fix to reuse rather than replace; each behaves identically on the long-standing `--admission-findings` channel; none is a regression relative to cycle 2 (where *all* v3 findings were dropped); the five delta specs under verification do not govern finding admission; and v3 remains behind `GENTLE_AI_RDD_NEW_LINEAGE` while the shipped default v2 path is unaffected and correct on all three. C-E — a channel that validated findings and then lied about carrying them — was categorically different, and it is fixed.

**Nothing else found.** Checked and clean: no new authority write, lock, or receipt removal; `Validate()` rejects malformed captured findings; the wire→admission conversion is total and loses only `proof_refs` list structure (joined with `"; "` into the single free-text `Proof` field `FindingEvidence` has always had); the W-8 classification branch is scoped to `errors.Is(ErrFinalizeRequiresLensResults)` and cannot swallow another error; kill-switch, write-guard, composition-deletion and decline-deletion guards all re-run green.

## 5. Spec compliance matrix (recomputed against the amended specs)

| Spec | Requirement | Scenarios | Status |
|---|---|---|---|
| receipt-only-gates | One Read-Only Path For All Gates And Lineages | 2/2 | PASS |
| receipt-only-gates | Kill Switch Consulted Before Governing Authority Is Read | 2/2 | PASS |
| receipt-only-gates | Six Prohibited Gate Actions | 2/2 | PASS |
| receipt-only-gates | Every Denial Carries An Executable Next Step | 2/2 | PASS — W-7's two codes and W-8's continuation all executed as printed |
| receipt-only-gates | Receipt File Persists; `invalidated` Fully Derived | 2/2 | PASS |
| receipt-only-gates | Legacy Receipts Validate Without Rewrite | 1/1 | PASS |
| receipt-only-gates | Verdict Is A Total Function Of Relation x Gate | 1/1 | PASS — exact-value pinning, mutation-proven |
| relation-algebra | Gate Boundary Descriptor Is A First-Class Algebra Input | 1/1 | PASS |
| relation-algebra | Verdict Is A Total Function Of Relation x Gate | 1/1 | PASS |
| delivery-exception-removal | Pre-PR Chain Composition Removed | 1/1 | PASS |
| delivery-exception-removal | Candidate-Decline Delivery Authorization Removed | 1/1 | PASS |
| delivery-exception-removal | Characterization Tests Precede Removal | 1/1 | PASS |
| delivery-exception-removal | No Composed Or Decline-Sourced Authority Remains Reachable | 1/1 | PASS |
| new-lineage-activation | Cutover Replaces The Additive Gate Branch | 2/2 | PASS — scenario 2 **amendment-accepted** (letter: matrix, 9/35 compact-only; intent: `TestEvaluateLegacyGateAllowsExactAtAllFiveGates`, verified real and passing) |
| new-lineage-activation | Unconditional Receipt Precedence | 2/2 | PASS |
| new-lineage-activation | Rollback Restores The Additive Branch, Never Invalidation Writes | 2/2 | PASS |
| review-core-transitions | `validate` Is The Single Governing Path For Legacy Lineages | 2/2 | PASS |

**Totals: requirements 17/17, scenarios 26/26.** (cycle 1: 12/17, 21/26 · cycle 2: 16/17, 25/26.)

## 6. Findings (cycle 3)

| ID | Severity | Status | Finding |
|---|---|---|---|
| W-9 | WARNING | CONFIRMED, fail-open | `causal_disposition: unknown` on a severe finding approves on v3, escalates on v2. Root cause is `AdmitCandidateCausalFindings`' spec-cited routing, reused per the coordinator's design decision; contradicts the root contract's "unknown escalates". |
| W-10 | WARNING | CONFIRMED, fail-open | v3 capture accepts a `BLOCKER` lacking `evidence_class`/`causal_disposition` — which `review schema reviewer` forbids and v2 refuses at capture — and it then approves. |
| W-11 | WARNING | CONFIRMED, fail-closed | `WARNING` + candidate-causal disposition escalates on v3, stays non-blocking on v2; `AdmitCandidateCausalFindings` never consults severity. |
| S-7 | SUGGESTION | — | Mirror v2's exact capture-time refusal for severe findings missing `evidence_class`/`causal_disposition` (closes W-10 at the boundary that already validates the payload). |
| S-8 | SUGGESTION | — | Decide the `unknown` disposition once, in `AdmitCandidateCausalFindings`, so the root contract, v2 and v3 agree (W-9); today three sources disagree. |
| S-9 | SUGGESTION | — | `--admission-findings` with an empty list silently discards every captured finding and approves. Matching the pre-existing channel is defensible, but the override leaves no record of what it overrode. |

**Closed this cycle**: C-E, W-8, W-5, W-6, W-4; W-2 amendment-accepted.
**All-wave tally**: C-A, C-B, C-C, C-D, C-E closed · W-1, W-3, W-4, W-5, W-6, W-7, W-8 closed · W-2 amendment-accepted · W-9, W-10, W-11 newly raised as warnings.

## 7. Verdict

**PASS WITH WARNINGS.** Zero CONFIRMED criticals. Every requirement is covered by a passing runtime test or explicitly amendment-accepted, and I re-ran each of my own repros against a freshly built binary rather than trusting the fix claims. The wave's gate cutover is real: one read-only evaluation path, kill switch consulted exactly once before any authority read (mutation-proven both directions), no gate write path, no composed or decline-sourced authority, `invalidated` fully derived, legacy allowed at all five gates, v3 finalizable at every tier with reviewer findings that actually bind, and default-deny pinned by value across all 35 relation × gate cells.

Archivable. The three warnings are v3-only admission-strictness divergences on an activation-gated lineage, each traceable to a documented function the coordinator scoped this fix to reuse; W-10 is the sharpest and the cheapest to close, and I would take it before v3 activation ships, not before this wave archives.

---

# Appendix — Cycle 2 report (historical, superseded)

## Verification Report — Cycle 2 (re-verification after fix cycles 1 and 2)

**Change**: `rdd-root-simplification-wave5` (Gate Cutover) | **Mode**: full spec-driven verification (proposal + design + tasks + 5 delta specs) | **Strict TDD**: active | **Store**: hybrid
**Candidate**: chain tip `8e5f287a` on `feat/rdd-wave5-f2-v3-capture`, worktree `/home/gentleman/work/gentle-ai-worktrees/rdd-wave5`
**Fix cycles under review**: `1f875015..8e5f287a` — cycle 1 (`e2423787`, `92682ed1`, `e0a51de3`) and cycle 2 (`d43870ef`, `f4b47266`, `59df5f92`)
**Attempt authority (echoed, not settled)**: `sha256:1e1f7fe3390bea7360a445b8ea642135553fe8cedd0aafdd67567e84224762d5`

**Verdict**: **FAIL** — 1 CRITICAL (CONFIRMED, NEW to fix cycle 2), 5 WARNING, 3 SUGGESTION.

**All four cycle-1 CRITICALs are genuinely closed**, each re-proven by my own original repro run against a freshly built `8e5f287a` binary — not by trusting the fix claims. W-1, W-3 and W-7 are closed; W-2 is partial and honestly disclosed. The single remaining blocker is a new fail-open that fix cycle 2's own capture primitive introduced: the v3 reviewer channel accepts a reviewer result carrying a candidate-causal `BLOCKER` and silently discards it, so the receipt is issued `approved` where the v2 path on the identical candidate returns `correction_required`.

## Command Evidence (verbatim, cycle 2)

| Command | Exit | Result |
|---|---|---|
| `go build -trimpath -o <scratch>/gentle-ai-c2 ./cmd/gentle-ai` (at `8e5f287a`) | 0 | `BUILD_OK` |
| `go test ./... -count=1` (root module) | 0 | 63 `ok`, zero `FAIL`; hash `sha256:7dde2e49855271aa5f5541185ed47ed6e6e4c3ece4027e3fad726650c4a4a26d` |
| `go test ./... -count=1` (bench module) | 0 | `ok github.com/gentleman-programming/gentle-ai/bench 0.180s` |
| `go run . run --binary <fresh> --out <scratch>/c2-bench-core.json` | 0 | `journeys: 59 completed, 0 unsupported, 0 failed` |
| `go run . run --binary <fresh> --axis all --out <scratch>/c2-bench-axis.json` | 0 | `journeys: 79 completed, 1 unsupported, 0 failed` (`j57`, declared source-coupled) |
| `gofmt -l .` | 0 | empty |
| `go vet ./...` | 0 | empty |
| `bash scripts/deadcode-ratchet.sh` | 0 | `no new unreachable functions` |
| `go test ./internal/cli/ -run TestEveryProductionRefusalNamesResolutionOrDeclaresByDesign -count=1` | 0 | `--- PASS` |
| `go test ./internal/reviewtransaction/ -run TestGateBoundaryMatrix_35Cells -count=1` (no `-update`) | 0 | `--- PASS (10.34s)`, golden unchanged, `git status` clean |

Worktree stayed clean throughout; every mutation experiment ran on `git archive` copies in scratch.

## 1. The four cycle-1 CRITICALs, re-run exactly

### C-A — CLOSED (re-proven end-to-end on the real binary)

```
$ GENTLE_AI_RDD_NEW_LINEAGE=1 gentle-ai review start --cwd <repo> --lineage med2
  risk_level: medium, selected_lenses: ["review-reliability"]
$ ... review capture-result --lineage med2 --target <t> --lens review-reliability --order 0 --preflight
  subject_hash: sha256:da3c08d8…
$ ... review capture-result --lineage med2 ... --input result.json
  {"operation":"review/capture-result", ... "subject_hash":"sha256:da3c08d8…"}
$ ... review finalize --lineage med2 --captured-results
  {"operation":"review/finalize","lineage_id":"med2","state":"approved","receipt":{…}}
```

Gates on that approved medium-tier v3 receipt:

| gate | result | reason |
|---|---|---|
| post-apply | `allow` | `exact` |
| pre-commit | `allow` | `exact` (after staging) |
| pre-push | `allow` | `exact` |
| pre-pr | `allow` | `exact`, `base_relationship_valid: true`, `relation: "exact"`, `next: {reason_code: "allow"}` |
| release | `invalidated` | `release boundary cannot be derived: release configuration artifact is required` (correct — no release evidence supplied) |

High tier re-proven too: 4 lenses selected, 4 captured, `state: approved`. **Tier-low unaffected**: `selected_lenses: []`, plain `review finalize` (no flag) → `approved`, post-apply `allow`.

The dead-end is gone: `review capture-result` now routes by lineage kind and binds `review-transactions/v3/<lineage>`, where in cycle 1 it failed with `open …/review-transactions/v2/med1/review-state.json: no such file or directory`.

### C-B — CLOSED (re-ran my own five-gate probe)

Driving the shipped `legacyApprovedChainFixture` through `EvaluateLegacyGate` at all five gates on a sandbox copy of `8e5f287a`:

```
GATE=post-apply result=allow  relation=exact reason_code="allow"
GATE=pre-commit result=allow  relation=exact reason_code="allow"
GATE=pre-push   result=allow  relation=exact reason_code="allow"
GATE=pre-pr     result=allow  relation=exact reason_code="allow"
GATE=release    result=allow  relation=exact reason_code="allow"
```

(cycle 1: pre-pr and release were `invalidated / base_relationship_invalid`.)

Fail-closed controls prove the preconditions are genuinely derived, not hardcoded true:

```
RELEASE-NO-EVIDENCE   result=invalidated reason="release boundary cannot be derived: release configuration artifact is required"
PRE-PR-DRIFTED-BASE   result=invalidated relation=changed reason_code="base_relationship_invalid"
```

### C-C — CLOSED (absorbed N2 really closed this time)

`newLineageGateEvaluation` is gone; `reviewtransaction.EvaluateNewLineageGate` (`new_lineage_gate.go`) now builds a real `GateContext`, derives `BaseRelationshipValid` from `live.BaseTree == authority.CandidateIdentity.BaseTree`, derives release evidence for `GateRelease`, and calls `gateVerdict`. Product-surface proof, the exact cycle-1 repro:

- cycle 1: `--gate release` on an approved v3 receipt → **`allow`**, `base_relationship_valid: false`, no release evidence anywhere.
- cycle 2: same shape → **`invalidated`**, `release boundary cannot be derived: release configuration artifact is required`.
- pre-pr now reports `base_relationship_valid: true` for a genuinely exact candidate and emits `relation` / `next` on the wire, which the v3 path never did before.

### C-D — CLOSED (flip experiment re-run, each relation individually)

| mutation | result |
|---|---|
| `ShadowRelationProvableContraction → GateAllow` | `--- FAIL: TestGateVerdict_TotalFunction_35Cells` |
| `ShadowRelationUnrelated → GateAllow` | `--- FAIL: TestGateVerdict_TotalFunction_35Cells` |
| `ShadowRelationAmbiguous → GateAllow` | `--- FAIL: TestGateVerdict_TotalFunction_35Cells` |
| `ShadowRelationUnknown → GateAllow` | `--- FAIL: TestGateVerdict_TotalFunction_35Cells` |

All four were fully green in cycle 1. Default-deny is now pinned by exact `(GateResult, ReasonCode)` value.

## 2. C-A hardening

**One-shot / no-reopen.** `CaptureLensResult`'s documented semantics hold: an identical resubmission is an idempotent no-op (verified: capture #1 and #2 of the same lens both succeed, authority unchanged); a wrong subject hash is refused with a runnable continuation:

```
Error: captured reviewer result does not bind the provider-owned subject hash; refresh the binding with
`gentle-ai review capture-result --cwd <repo> --lineage hi1 --target <target> --lens review-risk --order 0 --preflight`

Error: capture binding does not match the frozen selected-lens order for lineage "hi1"; discover the exact
lens/order pairs with `gentle-ai review capture-result … --preflight`

Error: new-lineage capture-result requires a reviewing or validating authority: lineage "med2" is "approved"
```

Note: `ErrNewLineageCaptureConflict` (a *different* subject hash for an already-captured lens) is unreachable through the CLI — the CLI compares against the same deterministic `wantSubject` first, so any differing hash is rejected earlier as a subject mismatch. The one-shot guard is genuine defence-in-depth at the Go API, but it is not a product-reachable path (W-4).

**Subject-hash stability.** The disclosed mid-implementation fix holds. On a 4-lens high-tier authority, the preflight subject hash for lens order 3 was recorded, then three unrelated captures (orders 0, 1, 2) each bumped the authority revision, then order 3 was re-preflighted:

```
lens3 subject BEFORE = sha256:b8054e5db86ff84d67b21fbcff524c6aa7a90d890aeacbebe5c286110c6030f1
lens3 subject AFTER  = sha256:b8054e5db86ff84d67b21fbcff524c6aa7a90d890aeacbebe5c286110c6030f1
SUBJECT_HASH_STABLE: YES
```

## 3. W-7 — CLOSED, both continuations executed as printed

| case | denial code | printed continuation | executed result |
|---|---|---|---|
| receipt absent | `approved_without_receipt` | `gentle-ai review finalize --lineage w7` | exit 0, `state: approved`, receipt re-issued; the gate then returns `allow` / `exact` |
| receipt present but tampered | `approved_receipt_corrupt` | `gentle-ai review start --cwd <repo> --lineage <new-lineage-id>` | exit 0, `lineage_id: w7-successor`, `state: reviewing` |

Both were run for real, not read. The two codes are genuinely distinguished by the tamper fixture.

## 4. Matrix

9/35 wired (up from 8), 26 explained skips; `TestGateBoundaryMatrix_35Cells` PASS with no `-update`, golden unchanged.

Wired: `exact` and `changed` at post-apply / pre-commit / pre-push / pre-pr, plus the new `release`/`exact`:

```
driven via the real gentle-ai binary: review start -> finalize -> validate --gate release
with the five release-boundary artifacts on the identical, unchanged candidate
```

W-1 closed — the generic skip reason is now honest (spot-checked 3 distinct reasons):

```
gateVerdict, NativeGateEvaluation.Relation, EvaluateLegacyGate, and EvaluateNewLineageGate all exist and are
wired production code as of Wave 5 fix cycle 1 … this cell's own binary-driven fixture … has not been built
yet, not because the underlying mechanism is missing.
```

Residual, disclosed: all 9 wired cells drive the **compact/v2** lineage. Zero cells drive legacy v1 or new-lineage v3, so the matrix still does not carry the legacy outcome-equivalence proof `rdd-new-lineage-activation` names explicitly (W-2, and the one remaining uncovered spec scenario).

## 5. Adversarial pass over `1f875015..8e5f287a` — 1 NEW CRITICAL

**CRITICAL-E (CONFIRMED, NEW): the v3 capture channel accepts and silently discards candidate-causal blocking findings.**

`review capture-result` on a v3 lineage parses a full reviewer result, requires non-nil `findings` and `evidence`, validates them against `review schema reviewer` — and then uses only `result.SubjectHash`. `CaptureLensResult` persists `{Lens, Order, SubjectHash}` and nothing else (`NewLineageCapturedResult`, `authority_store.go`). The findings never reach `AdmitCandidateCausalFindings`.

A/B on the identical candidate (`func boom() int { return 1/0 }`) with the identical BLOCKER finding (`severity: BLOCKER`, `evidence_class: deterministic`, `causal_disposition: introduced`):

| lineage kind | capture | finalize `--captured-results` |
|---|---|---|
| v2 / compact (default) | admission pipeline runs (`reviewer artifact admission …` binding checks) | **`state: "correction_required"`** — the blocker blocks |
| v3 / new lineage | accepted, findings dropped | **`state: "approved"`** — terminal receipt issued |

The blocking machinery exists but is on a different, optional channel: `review finalize --admission-findings <file>` (a distinct `FindingEvidence` schema) does produce `state: "escalated"` with `admitted_finding_ids: ["R3-boom-div-zero"]`. So the defect is precisely that the reviewer channel *looks like* it carries findings and does not, while an operator-supplied side file is what actually gates. A reviewer reporting a deterministic, candidate-introduced BLOCKER yields an `approved` receipt that every gate then honours.

Newly reachable: before fix cycle 2 no medium/high v3 approval was possible at all, so no blocker could be dropped. The v2/compact default path is unaffected. Scoped behind `GENTLE_AI_RDD_NEW_LINEAGE`.

The minimal-primitive design decision was disclosed (`new_lineage_capture.go` header, `NewLineageCapturedResult` doc comment, tasks.md Fix Cycle 2); the *consequence* — approval over an unadmitted candidate-causal blocker — was not, and it is a fail-open in an authorization system. Two proportionate remediations, neither requiring the v2 admission pipeline: refuse a non-empty `findings` array on the v3 capture path (making the gap explicit rather than silent), or feed captured findings into `AdmitCandidateCausalFindings` at finalize.

**No other fail-open found in the fix cycles.** Specifically checked and clean:
- `gateVerdict`'s W-3 narrowing is exactly `compatibleBaseAdvanceAtPrePR := gate == GatePrePR && relation == ShadowRelationCompatibleBaseAdvance`, matching `validateDerivedGate`'s `context.Gate == GatePrePR && …` (`receipt.go:289`). Release is no longer exempt.
- `EvaluateNewLineageGate` reconstructs the relation from `transition.ReasonCode`; an out-of-vocabulary value lands in `gateVerdict`'s `default` and fails closed to `GateInvalidated`.
- `deriveGateReleaseEvidenceFromInput` hardcodes `PublicationStateSealed` / `EvidenceFreshnessCurrent`, matching the pre-existing `compact_gate.go:840` and `native_request.go:105-106` producers — consistent, not a new weakening.
- `NewLineageAuthority.Validate()` gained real checks on captured results (non-empty lens, non-negative order, canonical subject hash, each lens at most once).
- Kill-switch, write-guard, composition-deletion and decline-deletion guards all still green (re-run, not assumed).

## 6. Spec compliance matrix (recomputed, cycle 2)

| Spec | Requirement | Scenarios | Status |
|---|---|---|---|
| receipt-only-gates | One Read-Only Path For All Gates And Lineages | 2/2 | PASS — legacy and v3 both reach `gateVerdict` now |
| receipt-only-gates | Kill Switch Consulted Before Governing Authority Is Read | 2/2 | PASS |
| receipt-only-gates | Six Prohibited Gate Actions | 2/2 | PASS |
| receipt-only-gates | Every Denial Carries An Executable Next Step | 2/2 | PASS — W-7's two codes executed as printed |
| receipt-only-gates | Receipt File Persists; `invalidated` Fully Derived | 2/2 | PASS |
| receipt-only-gates | Legacy Receipts Validate Without Rewrite | 1/1 | PASS — and the outcome is now correct at all five gates |
| receipt-only-gates | Verdict Is A Total Function Of Relation x Gate | 1/1 | PASS — exact-value pinning, mutation-proven on all 4 formerly-blind relations |
| relation-algebra | Gate Boundary Descriptor Is A First-Class Algebra Input | 1/1 | PASS |
| relation-algebra | Verdict Is A Total Function Of Relation x Gate | 1/1 | PASS |
| delivery-exception-removal | Pre-PR Chain Composition Removed | 1/1 | PASS |
| delivery-exception-removal | Candidate-Decline Delivery Authorization Removed | 1/1 | PASS |
| delivery-exception-removal | Characterization Tests Precede Removal | 1/1 | PASS |
| delivery-exception-removal | No Composed Or Decline-Sourced Authority Remains Reachable | 1/1 | PASS |
| new-lineage-activation | Cutover Replaces The Additive Gate Branch | 1/2 | **FAIL** — "Outcome equivalence proven by matrix, not byte-diff": all 9 wired cells drive compact/v2; zero legacy or v3 cells exist, so the matrix still carries no legacy equivalence proof. Equivalence IS proven, but by `TestEvaluateLegacyGateAllowsExactAtAllFiveGates`, which is neither the matrix the requirement names nor a byte-diff. |
| new-lineage-activation | Unconditional Receipt Precedence | 2/2 | PASS |
| new-lineage-activation | Rollback Restores The Additive Branch, Never Invalidation Writes | 2/2 | PASS |
| review-core-transitions | `validate` Is The Single Governing Path For Legacy Lineages | 2/2 | PASS |

**Totals: requirements 16/17, scenarios 25/26.** (cycle 1: 12/17 and 21/26.)

## 7. Findings (cycle 2)

| ID | Severity | Status | Finding |
|---|---|---|---|
| C-E | CRITICAL | CONFIRMED, NEW | v3 `capture-result` accepts a reviewer result carrying a candidate-causal BLOCKER, validates it, then discards the findings; `finalize --captured-results` issues `approved`. v2 on the identical input returns `correction_required`. |
| W-2 | WARNING | CONFIRMED, disclosed | Matrix is 9/35 wired and every wired cell drives compact/v2; `release/changed`, the other 5 release relations, and all legacy/v3 cells remain skips. This is the one uncovered spec scenario. |
| W-8 | WARNING | CONFIRMED, NEW | An ordinary partial-capture state (3 of 4 lenses captured) surfaces as an *unexpected fault*: exit 1 plus a written defect report titled "Gentle AI reached a tool-internal fault state (`operation_outcome_unknown`) that should never happen" and a GitHub issue URL. The refusal also never names `review capture-result` or `--captured-results` as the continuation. Cycle 2 turned this from a permanent dead-end into a routine transient state, which makes the misclassification far more visible. |
| W-4 | WARNING | CONFIRMED | `ErrNewLineageCaptureConflict` is unreachable through the CLI (the deterministic `wantSubject` check fires first), so the one-shot guard is Go-API-only defence in depth. |
| W-5 | WARNING | CONFIRMED, carried | The Gate Regression Test Index still names 5 `..._Allow_ExactReceiptGovernsDelivery` tests that were never implemented (disclosed in task 8.9, never corrected in the index). |
| W-6 | WARNING | CONFIRMED, carried | Task 1.4 (archive Wave 4) is still unchecked, and cycle 1's deferral entries for C-A and W-7 remain unchecked even though cycle 2's own section closes them — stale bookkeeping in `tasks.md`. |
| S-4 | SUGGESTION | — | Refuse a non-empty `findings` array on the v3 capture path, or route it into `AdmitCandidateCausalFindings` at finalize; either closes C-E without rebuilding the v2 admission pipeline. |
| S-5 | SUGGESTION | — | Classify `ErrFinalizeRequiresLensResults` as an ordinary typed refusal that names `review capture-result` / `--captured-results`, so it stops emitting defect reports for a normal state (W-8). |
| S-6 | SUGGESTION | — | The pre-pr `compatible_base_advance` skip reason still says "the other 26 remaining skip cells"; there are now 25 besides itself. |

**Closed since cycle 1**: C-A, C-B, C-C, C-D, W-1, W-3, W-7. **Carried**: W-2 (was W-2), W-5, W-6 (was W-4/W-5). **Retired**: cycle-1 W-7 (v3 tamper continuation) — now correct and executed.

## 8. Verdict

**FAIL**, narrowly and for one reason: CRITICAL-E is a CONFIRMED, reproduced fail-open that fix cycle 2 introduced, in exactly the authorization surface this wave exists to make trustworthy — a receipt issued `approved` over a reviewer-reported, deterministic, candidate-introduced BLOCKER, which every gate then honours. The one remaining uncovered spec scenario (matrix-proven legacy outcome equivalence) is a genuine but secondary gap.

Everything else the coordinator asked me to re-verify is genuinely fixed, and I re-ran each original repro rather than trusting the claims. If C-E is judged out of Wave 5's scope by the maintainer and deferred explicitly (as 8.5 was to Wave 7), the remaining state is one disclosed partial (W-2) and a clean bill everywhere else.

Not archivable as-is. Return to `sdd-apply` for C-E.

---

# Appendix — Cycle 1 report (historical, superseded)

Cycle-1 envelope (superseded by the cycle-2 envelope at the top of this file):

```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:0ceb6847fdab6d345a78c971529eaee3ed441f0adb5e4e70857576bc255a128c
verdict: fail
blockers: 4
critical_findings: 4
requirements: 12/17
scenarios: 21/26
test_command: go test ./... -count=1
test_exit_code: 0
test_output_hash: sha256:ee707fd2bf359c2451ede4a4350dab2427cf59b1b9a69317278e11eed7fc9f00
build_command: go vet ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```


## Verification Report

**Change**: `rdd-root-simplification-wave5` (Gate Cutover) | **Mode**: full spec-driven verification (proposal + design + tasks + 5 delta specs) | **Strict TDD**: active | **Store**: hybrid
**Candidate**: chain tip `1f875015` on `feat/rdd-wave5-p9-frozen-anchor-archive-gate`, worktree `/home/gentleman/work/gentle-ai-worktrees/rdd-wave5`
**Behavior-delta base**: `7598eda4` (Wave 4 verified tip), rebuilt independently for A/B probes
**Attempt authority (echoed, not settled)**: `sha256:3f153af34bd8db01a48b4243fe4008251eda3bb491046e81978dbfa0ad86c745`

**Verdict**: **FAIL** — 4 CRITICAL (all CONFIRMED by reproduction), 7 WARNING, 3 SUGGESTION.

Everything the apply phases claimed to have *built* is real: the deletions are complete and call-absence-proven, the kill-switch collapse is genuine and mutation-proven, Phase 9's frozen anchor is correct and its named continuation is genuinely runnable, and all 63 root packages plus the bench module and the 59-journey corpus are green against a freshly built binary. The failure is that the cutover's two headline promises — *a medium/high candidate can still be delivered* and *default-deny holds at every gate for every lineage* — do not hold at the product surface, and the wave's own evidence does not detect that.

## Command Evidence (verbatim)

| Command | Exit | Result |
|---|---|---|
| `go build -trimpath -o <scratch>/gentle-ai-w5 ./cmd/gentle-ai` (at `1f875015`) | 0 | `BUILD_OK`, `gentle-ai dev` |
| `go test ./... -count=1` (root module) | 0 | 63 `ok` lines, zero `FAIL`; hash `sha256:ee707fd2bf359c2451ede4a4350dab2427cf59b1b9a69317278e11eed7fc9f00` |
| `go test ./... -count=1` (bench module) | 0 | `ok github.com/gentleman-programming/gentle-ai/bench 0.175s` |
| `go run . run --binary <fresh> --out <scratch>/bench-core.json` | 0 | `journeys: 59 completed, 0 unsupported, 0 failed` |
| `go run . run --binary <fresh> --axis all --out <scratch>/bench-axis.json` | 0 | `journeys: 79 completed, 1 unsupported, 0 failed` (`j57` unsupported = declared source-coupled fixture) |
| `gofmt -l .` | 0 | empty |
| `go vet ./...` | 0 | empty |
| `bash scripts/deadcode-ratchet.sh` | 0 | `no new unreachable functions` |
| `go test ./internal/cli/ -run TestEveryProductionRefusalNamesResolutionOrDeclaresByDesign -count=1` | 0 | `--- PASS` |
| `go test ./internal/reviewtransaction/ -run TestGateBoundaryMatrix_35Cells -count=1` (no `-update`) | 0 | `--- PASS (10.05s)`, golden unchanged, `git status` clean |

Worktree remained clean throughout (`git status --short` empty after every run); all mutation work was done on `git archive` copies in scratch, never in the worktree.

## Item-by-item evidence

### 1. Default-deny cutover — PARTIAL

The 15 per-gate named tests + 5 double-eval byte-equivalence tests ran green with `-count=1`:

```
--- PASS: TestPostApplyGate_Disabled_ReportsUnmanagedBeforeAuthorityRead (0.03s)
--- PASS: TestPreCommitGate_Disabled_ReportsUnmanagedBeforeAuthorityRead (0.03s)
--- PASS: TestPrePushGate_Disabled_ReportsUnmanagedBeforeAuthorityRead (0.03s)
--- PASS: TestPrePRGate_Disabled_ReportsUnmanagedBeforeAuthorityRead (0.03s)
--- PASS: TestReleaseGate_Disabled_ReportsUnmanagedBeforeAuthorityRead (0.03s)
--- PASS: TestDisabledGateOutput_DoubleEval_ByteEquivalent_PostApply (0.03s)
--- PASS: TestDisabledGateOutput_DoubleEval_ByteEquivalent_PreCommit (0.03s)
--- PASS: TestDisabledGateOutput_DoubleEval_ByteEquivalent_PrePush (0.04s)
--- PASS: TestDisabledGateOutput_DoubleEval_ByteEquivalent_PrePR (0.03s)
--- PASS: TestDisabledGateOutput_DoubleEval_ByteEquivalent_Release (0.03s)
--- PASS: TestPostApplyGate_Deny_ChangedRelationCarriesNextStep (0.09s)
--- PASS: TestPreCommitGate_Deny_ChangedRelationCarriesNextStep (0.09s)
--- PASS: TestPrePushGate_Deny_ChangedRelationCarriesNextStep (0.15s)
--- PASS: TestReleaseGate_Deny_ChangedRelationCarriesNextStep (0.08s)
--- PASS: TestPrePRGate_Deny_BaseMismatchDeniesWithoutComposition (0.16s)
```

The "Allow (S4)" row of the index (`Test{PostApply,PreCommit,PrePush,PrePR,Release}Gate_Allow_ExactReceiptGovernsDelivery`) does not exist — disclosed honestly in task 8.9, but the index in `tasks.md` still names them (W-5).

Adversarial default-deny probe against the REAL binary, using a genuinely approved v3 receipt (low tier, `review start` -> `review finalize` -> `state: approved`), then each gate:

| gate | result | relation | notes |
|---|---|---|---|
| post-apply | `allow` | `exact` | correct |
| pre-commit | `escalated` | `unknown` | staged projection, nothing staged — correct |
| pre-push | `allow` | `exact` | correct |
| pre-pr | **`allow`** | `exact` | `"base_relationship_valid": false` in the emitted context — allowed anyway |
| release | **`allow`** | `exact` | `"base_relationship_valid": false`, **no release evidence supplied or checked** |

Default-deny therefore does NOT hold everywhere the spec says (see CRITICAL-C). Matrix golden re-verified end-to-end: 8 wired cells (`exact`/`changed` at post-apply, pre-commit, pre-push, pre-pr), 27 skips. Spot-check of 5 skip reasons: **26 of the 27 carry a reason that is now factually false** (W-1); none claims unreachability, so no unreachable-relation claim needed falsifying.

### 2. Kill-switch-first — PASS (mutation-proven both directions)

`TestKillSwitchOrdering_SingleCallBeforeAuthorityRead` and the 5-gate decoy-store byte-identity proof re-ran green. RED-capability proven on `git archive` sandbox copies:

- Strip the single consultation (`if reviewDeliveryDisposition(...) ==` -> `if false`):
  `--- FAIL: TestKillSwitchOrdering_SingleCallBeforeAuthorityRead`, `--- FAIL: TestPostApplyGate_Disabled_...`, `--- FAIL: TestReleaseGate_Disabled_...`, `--- FAIL: TestDisabledOutputIsByteIdenticalRegardlessOfAuthorityStoreContent`.
- Add a second consultation:
  `review_kill_switch_single_call_test.go:81: runReviewFacadeValidate calls reviewDeliveryDisposition 2 times, want exactly 1`.

The guard is genuinely RED-capable in both directions. This is the strongest evidence in the wave.

### 3. N1 — CLOSED WITH A NEW DEAD-END (CRITICAL-A, CONFIRMED)

Driven end-to-end through the REAL binary with the switch ON:

```
$ GENTLE_AI_RDD_NEW_LINEAGE=1 gentle-ai review start --cwd <repo> --lineage med1
{ "operation": "review/start", "lineage_id": "med1", "state": "reviewing",
  "risk_level": "medium", "selected_lenses": ["review-reliability"], "correction_budget": 3, ... }

$ GENTLE_AI_RDD_NEW_LINEAGE=1 gentle-ai review finalize --cwd <repo> --lineage med1
Error: review core finalize: new-lineage finalize requires captured results for every frozen
selected lens before approving: lineage "med1"   [exit 1]
```

There is NO ingestion path for the new lineage:

- `FinalizeAdvanceRequest.CapturedLensResults` has exactly one production construction site, `internal/cli/review_facade_finalize_new_lineage.go:79`, and it omits the field: `AdvanceRequest: &reviewtransaction.FinalizeAdvanceRequest{Failed: failed, AdmittedFindingIDs: admitted}`. Every other reference to the field is in `review_core_finalize_lens_results_test.go`.
- `review capture-result` binds the **v2 compact** store: preflighting the v3 lineage fails with `open <repo>/.git/gentle-ai/review-transactions/v2/med1/review-state.json: no such file or directory`, while the lineage actually lives at `.git/gentle-ai/review-transactions/v3/med1/`.
- The refusal's own doc comment concedes it: *"the v3 reviewer-result ingestion pipeline, not yet wired"* (`review_core.go:60`).

Consequence, reproduced at all five gates:

```
$ ... review validate --gate {post-apply|pre-push|release} --lineage med1
"result": "invalidated",
"reason": "facade review receipt is not available; run gentle-ai review finalize --lineage med1 to produce one"
```

**The named next step is the exact command that refuses.** The only escape is `--failed`, which yields `state: escalated` — a terminal non-approval that still denies at every gate. A medium/high v3 candidate is permanently undeliverable.

Delta vs base `7598eda4` (independently rebuilt): the identical medium fixture finalizes to `"state": "approved"`. So S4/task 5.10 converted the N1 self-approval bug into a hard dead-end without shipping the ingestion path it depends on — exactly the branch Engram observation #10179 said must not resolve silently.

**Tier-low still finalizes with zero results**: confirmed — `selected_lenses: []`, `review finalize` -> `"state": "approved"` with a receipt ref.

Scope note (context, not mitigation): v3 lineages are only created when `GENTLE_AI_RDD_NEW_LINEAGE` is set, so the default shipped configuration does not reach the dead-end today. The wave nevertheless ships a gate cutover whose new lineage has no reachable approval path above tier-low.

### 4. N2 / N3 — N3 CLOSED, N2 NOT CLOSED AT THE PRODUCT SURFACE (CRITICAL-C)

**N3 — CLOSED.** `review_governing_authority.go:106` now adds `receipt.CandidateIdentity != record.Authority.CandidateIdentity` to the approved-state cross-check. Tamper test against the real binary: editing `CandidateIdentity.BaseTree` in `review-receipt.json` flips post-apply from `allow` to `invalidated` with `denial.code: "approved_without_receipt"`; restoring the bytes restores `allow`.

**N2 — NOT CLOSED.** `gateVerdict` (`gate.go:1744-1774`) does implement the preconditions and matches `validateDerivedGate`'s shape (`TestGateVerdict_PerGatePreconditions_MatchLegacyValidateDerivedGate` PASS, including the `compatible_base_advance` exemption). But the v3 gate path never calls it: `resolveGoverningAuthority` -> `ReviewCore.Next(validate)` -> `newLineageGateEvaluation` (`review_governing_authority.go:241-263`), still mapping `CoreTransitionContinue -> GateAllow` uniformly for all five gates — the exact eleven lines N2 cites, and the chain diff touches that file only for the N3 one-liner. `attachGateVerdictRelation` (`compact_gate.go:154`) explicitly documents that it "NEVER changes Result", so the compact path also discards `gateVerdict`'s verdict.

Reproduced above (item 1): `--gate release` with an approved v3 receipt returns `allow` with `base_relationship_valid: false` and no release evidence anywhere in the pipeline. `--gate pre-pr` likewise. The base binary reproduces both identically, so this is **not a new fail-open regression** — but task 4.7's `[x] ABSORBED N2` and the PR0 disposition ("Closure: `gateVerdict(gate, relation)` must reproduce this per-gate precondition shape, not a uniform continue->allow") are not true of the shipped path. The N2 debt is still open.

### 5. Legacy projection — REGRESSION AT pre-pr AND release (CRITICAL-B, CONFIRMED)

Legacy lineages do evaluate through the shared algebra (`runFacadeLegacyValidateNegotiated` deleted; only comments reference it), and the S1 characterization corpus passes unchanged end-to-end (`TestLegacyFunnelCharacterization_RunFacadeLegacyValidateNegotiated` PASS). Byte-identity and the in-flight-correction regression both hold (`TestProjectLegacyAuthority*`, `TestEvaluateLegacyGateValidatesReceiptFromAnInFlightCorrection` PASS). The v3-over-legacy precedence rule holds as amended (`TestRunReviewFacadeValidateNewLineageGovernsOverAnAllowingLegacyReceipt`).

But **every existing legacy test drives `GatePreCommit` only**. Driving the shipped `legacyApprovedChainFixture` through `EvaluateLegacyGate` at all five gates on a sandbox copy:

```
GATE=post-apply result=allow        relation=exact  transition="" reason_code="allow"
GATE=pre-commit result=allow        relation=exact  transition="" reason_code="allow"
GATE=pre-push   result=allow        relation=exact  transition="" reason_code="allow"
GATE=pre-pr     result=invalidated  relation=exact  transition="" reason_code="base_relationship_invalid"
GATE=release    result=invalidated  relation=exact  transition="" reason_code="base_relationship_invalid"
```

Cause: `legacy_projection.go:110-113` builds its `GateContext` with `Gate`, `LineageID`, `StoreRevision`, `BaseTree`, `CandidateTree`, `PolicyHash` — and never `BaseRelationshipValid` or `Release`. `gateVerdict` gates pre-pr/release on both. Pre-cutover, `EvaluateNativeGate` derived `BaseRelationshipValid: snapshot.BaseTree == receipt.BaseTree` (`gate.go:326`) and carried release evidence, so the identical receipt could pass. A byte-identical, unchanged legacy candidate is now permanently invalidated at pre-pr and release, with a `reason_code` naming a field no operator command can make true.

This is fail-CLOSED (no authorization is granted that should not be), but it is a real outcome-equivalence break and it is unproven by the matrix, whose legacy cell count is zero.

### 6. Deletions — PASS

Non-test call sites for `EvaluateCompactPrePRChain`, `compactPrePRChainProof`, `ResolveCandidateDeclineForGate`, `RecordCandidateDecline`, `emitCandidateDeclinedUnmanagedDelivery`, `InvalidateApprovedCompactAuthority`, `CompactApprovedInvalidationRequest`, `invalidateApproved`, `runFacadeLegacyValidateNegotiated`: **zero** — every surviving occurrence is a comment. `compact_chain.go` and `compact_approved_invalidation.go` are gone from disk. AST guards green: `TestPrePRComposition_ZeroCallers`, `TestCandidateDecline_ZeroCallers`, `TestNoGateWritesAuthority_CallAbsenceGuard`, `TestPrePRGate_KillSwitchBeforeComposition_TriviallyPreserved`.

Read-only parsers retained and forensically functional: `TestParseCandidateDeclineAuthorizationRoundTripsCanonicalBytes`, `TestParseCandidateDeclineAuthorizationRejectsNonCanonicalBytes`, `TestPreCutoverInvalidatedRecordsStayReadable` all PASS.

S1 pin migration spot-read (3): `TestInvalidationVerbDowngrade_RefusesInsteadOfDeriving` (asserts persisted state unmutated AND receipt bytes unchanged after the refusal — strictly stronger than the pin it replaced), `TestPrePRChainCompositionDeletionSupersedesRemovalDelta` (pins the permanent half of the delta), `TestCandidateDeclineDowngrade_DeniesLikeAnyNeverReviewedCandidate` (pins the new generic denial). None weakened.

### 7. Phase 9 anti-livelock — PASS

`blockArchiveForUnsatisfiedReVerify` anchors on `CompactCorrectionAttempt.FixDeltaHash`/`Snapshot.CandidateTree`, both written once at `CompleteCorrection` time into an append-only slice; satisfaction is `attempt.Outcome == AttemptPassed && attempt.FinishCandidateTree == evidence.candidateTree`. Neither side is re-derived per `Resolve()`, so satisfied is monotone — the W4 relabelling livelock is structurally impossible. All 5 named tests + subtests PASS, including `_FrozenAnchorDoesNotRelabel`, `_StructuralAbsence`, and `_MutatesOnlyArchive`.

The named continuation is genuinely runnable as printed — verified against the real binary rather than by reading:

```
$ gentle-ai sdd-attempt finish --cwd <repo> --change demo --expected-revision sha256:000... \
    --request-id rq-1 --outcome passed --evidence-revision sha256:111... --diagnosis d \
    --harness-disposition reused --cleanup-evidence c --process-evidence p
Error: sdd-attempt finish: SDD runtime ledger revision conflict: expected "sha256:000...", current "";
retry with --expected-revision ""
```

The error is semantic (revision conflict), NOT `sdd-attempt finish requires ...` — so `validateSDDAttemptOperationFlags` accepts the printed shape, and the 0-or-3 remediation-trio rule is satisfied by naming zero of the three. This closes the W4 CRITICAL-A "unrunnable as printed" defect. A full on-disk correction round trip was not driven (the apply phase's own documented cost decision); the anti-relabel property is proven structurally plus by the frozen-anchor unit tests.

### 8. Suites, corpus, ratchets, goldens — PASS

All listed in the Command Evidence table. Goldens ran without `-update` and the worktree stayed clean.

### 9. Adversarial pass over `7598eda4..1f875015` — 2 NEW CONFIRMED criticals, 1 CONFIRMED evidence gap

Hunting fail-open specifically (relation misclassification that allows, precondition bypass, projection granting more than v3 would):

- **No fail-open was introduced by this chain.** The release/pre-pr precondition bypass (CRITICAL-C) reproduces identically on the base binary; the legacy projection grants strictly LESS than pre-cutover, not more.
- **`gateVerdict`'s default-deny is not pinned for 4 of the 7 relations** (CRITICAL-D, CONFIRMED). On a sandbox copy, flipping `ShadowRelationProvableContraction`, `ShadowRelationUnrelated`, `ShadowRelationAmbiguous`, and `ShadowRelationUnknown` from `GateInvalidated` to **`GateAllow`** leaves everything green:
  ```
  ok  .../internal/reviewtransaction   120.970s
  ok  .../internal/cli                 170.505s
  ok  .../internal/sddstatus            23.076s
  ok  .../e2e/organicruntime            10.741s
  ```
  `TestGateVerdict_TotalFunction_35Cells` asserts only that the result is one of four known `GateResult` values and that non-allow carries a next step (`gate_verdict_test.go:42-50`) — never which relations must deny. The 35-cell matrix golden proves nothing here because 27 of its cells are skips. Control: `ShadowRelationChanged -> GateAllow` IS caught, by `TestEvaluateLegacyGateAllowsExactAndDeniesChanged` and the legacy characterization pin — so the harness is RED-capable, just blind on those four.
- Latent (currently unreachable) fail-open: `gateVerdict` exempts `compatible_base_advance` from the `BaseRelationshipValid` precondition at **both** pre-pr and release, whereas `validateDerivedGate` computes `compatibleAdvance` as pre-PR-only (`receipt.go:289`). Unreachable today because release always hits `Release == nil` first on the legacy path and the compact path discards `gateVerdict`'s verdict — but it becomes a real hole the moment release is wired to `gateVerdict` (W-3).

## Spec compliance matrix

| Spec | Requirement | Scenarios met | Status | Evidence |
|---|---|---|---|---|
| receipt-only-gates | One Read-Only Path For All Gates And Lineages | 1/2 | **FAIL** | pre-PR parity proven; three distinct verdict functions still exist by lineage kind (`newLineageGateEvaluation` v3, `evaluateCompactGate` v2, `gateVerdict` legacy) |
| receipt-only-gates | Kill Switch Consulted Before Governing Authority Is Read | 2/2 | PASS | 5 named + 5 double-eval + AST guard + decoy-store, mutation-proven |
| receipt-only-gates | Six Prohibited Gate Actions | 2/2 | PASS | `TestNoGateWritesAuthority_CallAbsenceGuard`, `TestPrePRGate_Deny_BaseMismatchDeniesWithoutComposition` |
| receipt-only-gates | Every Denial Carries An Executable Next Step | 1/2 | **FAIL** | base-mismatch names a step; `unknown` has no covering test and the v3 path emits no `next` at all; CRITICAL-A/B make named steps unrunnable |
| receipt-only-gates | Receipt File Persists; `invalidated` Fully Derived | 2/2 | PASS | `TestReceiptFilePersistsAfterDerivedInvalidation_AllFiveGates`, `TestPreCutoverInvalidatedRecordsStayReadable`, task 8.8 audit |
| receipt-only-gates | Legacy Receipts Validate Without Rewrite | 1/1 | PASS | `TestProjectLegacyAuthority*` byte-identity (validation OUTCOME regressed — see CRITICAL-B) |
| receipt-only-gates | Verdict Is A Total Function Of Relation x Gate | 0/1 | **FAIL (UNTESTED)** | 27/35 cells skipped with stale reasons; 4/7 relations mutation-unpinned |
| relation-algebra | Gate Boundary Descriptor Is First-Class Input | 1/1 | PASS | `gateVerdict(gate, relation, GateContext)` 3-arg amendment, disclosed and justified |
| relation-algebra | Verdict Is A Total Function Of Relation x Gate | 0/1 | **FAIL (UNTESTED)** | same as above |
| delivery-exception-removal | Pre-PR Chain Composition Removed | 1/1 | PASS | `TestPrePRComposition_ZeroCallers`, file deleted |
| delivery-exception-removal | Candidate-Decline Authorization Removed | 1/1 | PASS | `TestCandidateDecline_ZeroCallers`, `..._DeniesLikeAnyNeverReviewedCandidate` |
| delivery-exception-removal | Characterization Tests Precede Removal | 1/1 | PASS | S1 (`362c7fa4`..`54d70b6a`) lands before S5/S6/S7 commits |
| delivery-exception-removal | No Composed/Decline-Sourced Authority Reachable | 1/1 | PASS | AST call-absence guards, zero non-comment references |
| new-lineage-activation | Cutover Replaces The Additive Gate Branch | 1/2 | **FAIL** | legacy uses the algebra, but outcome equivalence is broken at pre-pr/release and the matrix has zero legacy cells |
| new-lineage-activation | Unconditional Receipt Precedence | 2/2 | PASS | `TestRunReviewFacadeValidateNewLineageGovernsOverAnAllowingLegacyReceipt` (disclosed reframing accepted on the merits) |
| new-lineage-activation | Rollback Restores Additive Branch, Never Invalidation Writes | 2/2 | PASS | write-guard + `TestEvaluateLegacyGateValidatesReceiptFromAnInFlightCorrection` |
| review-core-transitions | `validate` Is The Single Governing Path For Legacy | 2/2 | PASS | `runFacadeLegacyValidateNegotiated` deleted; no per-lineage discovery fork survives |

**Totals: requirements 12/17 complete, scenarios 21/26 covered by a passing runtime test.**

## Findings

| ID | Severity | Status | Finding |
|---|---|---|---|
| C-A | CRITICAL | CONFIRMED, NEW | Medium/high-tier v3 candidates can never obtain an approved receipt: `finalize` refuses (`ErrFinalizeRequiresLensResults`), no production code sets `CapturedLensResults`, and `review capture-result` binds the v2 store. Gates then deny at all five while naming `review finalize` — the command that refuses. Base `7598eda4` finalizes the same fixture to `approved`. Blocking-budget violation at cutover. |
| C-B | CRITICAL | CONFIRMED, NEW | `EvaluateLegacyGate` never populates `GateContext.BaseRelationshipValid` or `Release`, so every legacy lineage — even byte-identical/`exact` — is permanently `invalidated` at `pre-pr` and `release` with an unactionable `base_relationship_invalid`. Pre-cutover `EvaluateNativeGate` derived both. No test covers legacy at those two gates. |
| C-C | CRITICAL | CONFIRMED, pre-existing behavior / closure claim unmet | Absorbed N2 is marked `[x]` but is not closed at the product surface: the v3 gate path routes through `newLineageGateEvaluation`'s uniform `continue -> allow`, never `gateVerdict`. Real-binary probe: approved v3 receipt allows at `--gate release` with `base_relationship_valid: false` and zero release evidence. |
| C-D | CRITICAL | CONFIRMED | `gateVerdict`'s default-deny is unpinned for `provable_contraction`, `unrelated`, `ambiguous`, `unknown`: flipping all four to `GateAllow` leaves reviewtransaction, cli, sddstatus and e2e fully green. The totality test asserts shape, not values; 27/35 matrix cells are skips. |
| W-1 | WARNING | CONFIRMED | 26 of 27 matrix skip reasons state that `gateVerdict` does not exist, `NativeGateEvaluation` has no `Relation` field, and legacy projection has not landed — all three untrue at the chain tip (`gate_boundary_matrix_test.go:51`). Design decision 7's "zero unexplained divergences" bar is met only nominally. |
| W-2 | WARNING | CONFIRMED | All 7 release-gate matrix cells are skips. Release is the highest-consequence gate and has zero driven matrix coverage. |
| W-3 | WARNING | CONFIRMED | `gateVerdict` exempts `compatible_base_advance` from the base precondition at pre-pr AND release; `validateDerivedGate` scopes that exemption to pre-PR only. Latent fail-open once release is wired to `gateVerdict`. |
| W-4 | WARNING | CONFIRMED | Task 1.4 (archive Wave 4) is unchecked. Correctly deferred at apply time; the tasks artifact is not fully complete. |
| W-5 | WARNING | CONFIRMED | The Gate Regression Test Index still names 5 `..._Allow_ExactReceiptGovernsDelivery` tests that were never implemented (disclosed in 8.9 but not corrected in the index itself). |
| W-6 | WARNING | CONFIRMED | The "changed relation carries a typed transition" property is proven at only 3 of 5 gates: pre-PR and release deny on `base_relationship_invalid` instead (disclosed in 4.2). |
| W-7 | WARNING | CONFIRMED | The v3 `approved_without_receipt` denial (tamper case) names `review finalize --lineage <id>`, which for an already-approved lineage re-issues the same receipt rather than repairing the tamper. |
| S-1 | SUGGESTION | — | Retire `gateBoundaryMatrixNotWiredReason` for the current truth, or convert the affected cells to driven rows now that their stated blockers have landed. |
| S-2 | SUGGESTION | — | Give `TestGateVerdict_TotalFunction_35Cells` a per-cell expected-verdict column; that alone would have caught the four-relation mutation. |
| S-3 | SUGGESTION | — | Populate `BaseRelationshipValid`/`Release` in `EvaluateLegacyGate`'s `GateContext` from the same derivation `EvaluateNativeGate` uses (`gate.go:326`), which closes C-B directly. |

## Disclosed deviations judged on the merits

| Deviation | Verdict |
|---|---|
| `gateVerdict` 3-arg signature (design amendment) | **Accepted.** The 2-arg sketch cannot express a per-gate precondition; the amendment is documented in both `design.md` and `gate.go`. |
| `ProjectLegacyAuthority(ctx, root, chain, receiptPath)` instead of `(chain, artifacts)` | **Accepted.** `facadeArtifacts` is an unexported `cli` type; `reviewtransaction` cannot import `cli`. |
| Three-consultation-point collapse (design amendment) | **Accepted.** Substance unchanged; only the stale `d591f4cf` line numbers were corrected. |
| Task 5.3 reframing ("legacy-only authority" is not a reachable state) | **Accepted.** The reasoning against `relateCandidates` is sound and the substitute test proves the stronger precedence rule. |
| Task 6.3 `changed` promoted from skip to driven | **Accepted.** Genuinely made reachable by S5's deletion. |
| Tasks 7.2 / 8.2 recorded as already-true regression guards, not new REDs | **Accepted.** Honest, and the guards are still valuable. |
| Task 9.2 naming the 8-base-flag `finish` instead of the 3-flag trio | **Accepted.** Verified runnable against the real binary; the trio would have re-created the W4 unrunnable-continuation defect at one remove. |
| Task 5.4 byte-identity folded into 5.1/5.2 rather than a 5-gate hash test | **Not accepted as sufficient.** A 5-gate before/after sweep would have surfaced C-B. |
| Task 5.10 N1 closure without an ingestion path | **Not accepted.** See C-A. |
| Task 4.7 N2 closure via `gateVerdict` alone | **Not accepted.** See C-C. |

## Verdict

**FAIL.** Two CONFIRMED new criticals (C-A, C-B) make delivery unreachable for whole classes of candidate — medium/high v3 candidates at every gate, and every legacy candidate at pre-pr and release — with denials whose named next steps cannot succeed. One CONFIRMED closure claim (C-C, absorbed N2) is untrue of the shipped path. One CONFIRMED evidence gap (C-D) means a regression to fail-open across four of the seven relations would ship silently. Three of these are genuine uncovered spec-MUSTs (`Every Denial Carries An Executable Next Step`, `Verdict Is A Total Function Of Relation x Gate`, `Outcome equivalence proven by matrix`).

Not archivable. Return to `sdd-apply`.
