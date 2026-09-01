```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:7cd9bd227f8fc2491b336690121f6c17cce29449ea8ef88f60ed9d15f873eb83
verdict: pass
blockers: 0
critical_findings: 0
requirements: 19/19
scenarios: 32/32
test_command: go test ./... -count=1
test_exit_code: 0
test_output_hash: sha256:3ff9a58e6e41d5f53aa2b09e41f927f16cd6c34641e27653f500779a535fe744
build_command: go build ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report — Cycle-3 close-out, coverage complete (FINAL)

**Change**: rdd-root-simplification-wave3
**Candidate**: worktree `/home/gentleman/work/gentle-ai-worktrees/rdd-wave0`, branch `feat/rdd-wave3-s5-taxonomy-offer-bench`, tip **`67be4867`**; `git status --porcelain` empty
**Scope of this pass**: envelope re-admission only. The C5/C6/W11/W14 remediation evidence at tip `157ab9fd` (section below) was not re-derived; `67be4867` changes no production byte, so it cannot invalidate it.
**Attempt authority (echoed, not settled)**: `sha256:0040067d0fe91ff022afd673e94d5cbd771d3af4224a0d1bf7409edf74b36297`

### FINAL VERDICT: **PASS** — 0 CRITICAL, 0 blockers, coverage complete at 19/19 requirements and 32/32 scenarios

### Candidate integrity: tests-only, independently confirmed

```
$ git log --oneline -1
67be4867 test(review): cover switch identity, switch-off zero side effects, and frozen-tier immutability

$ git diff --stat 157ab9fd..67be4867
 .../cli/review_new_lineage_frozen_tier_test.go     | 120 +++++++++++++++++++
 .../cli/review_new_lineage_kill_switch_test.go     | 131 +++++++++++++++++++++
 .../new_lineage_switch_identity_test.go            |  91 ++++++++++++++
 3 files changed, 342 insertions(+)
```

`git diff --name-only 157ab9fd..67be4867 | rg -v '_test\.go$'` returns **zero** paths: 342 insertions, 0 deletions, no production file touched. Every prior finding therefore carries forward unchanged.

### The three closed scenarios, mapped and re-run

Each spec scenario is quoted verbatim from `openspec/changes/rdd-root-simplification-wave3/specs/`, then matched to the test that now covers it.

**1. `rdd-new-lineage-activation` → "Distinct Env Switch, Default Off, Legacy Path When Disabled" → "Switch identity never overloads another switch"**
> GIVEN the activation switch, `GENTLE_AI_RDD_SHADOW`, and the RDD kill switch / WHEN any one of the three is toggled / THEN only its own scoped behavior changes; the other two are unaffected

Covered by `internal/reviewtransaction/new_lineage_switch_identity_test.go`. `TestNewLineageActivationSwitchIdentityNeverOverloadsAnotherSwitch` drives the switches' own single readers — `NewLineageActivationEnabled()` and `shadowObservationEnabled()` — across all four env combinations, asserting each reader answers for its own variable alone. `TestNewLineageActivationSwitchIndependentOfKillSwitch` proves the third pairing in **both** directions: the activation env var never influences `ResolveRDDMode`, and recording the kill switch off via `SetCloneLocalRDDMode(RDDModeOff)` never flips the activation reading on or off. UNTESTED → **COMPLIANT**.

**2. `rdd-new-lineage-activation` → "Kill-Switch-Off Is Structurally Unfailable and Creates Nothing" → "Kill switch off produces no side effect"**
> GIVEN the RDD kill switch is off / WHEN the facade is invoked at any observed call site / THEN no artifact is created, and no error path is reachable

Covered by `internal/cli/review_new_lineage_kill_switch_test.go`. With the kill switch off **and** `GENTLE_AI_RDD_NEW_LINEAGE=1`, it drives all five `review validate` gates plus `OfferReviewAfterVerify`, twice against the identical fixture, and asserts on each pass: zero error from every call, `Delivery == disabled/unmanaged`, no fabricated approval (`Allowed` false and `Result != allow`), and — the strong assertion — the **entire `.git/gentle-ai` subtree byte-identical** before and after, via a `WalkDir` snapshot comparing every relative path plus exact file content. This is what upgrades the requirement: prior coverage attested only the unwired `OfferReviewAfterVerify` guard, never the facade at its five observed gate call sites. PARTIAL → **COMPLIANT**.

**3. `rdd-review-core-transitions` → "Consent-Gated Freeze With Immutable Tier, Lenses, and Budget" → "Frozen tier is never recomputed"**
> GIVEN a frozen tier-4 lineage mid-review / WHEN a later transition re-evaluates risk inputs / THEN the persisted tier, lens set, and budget remain exactly as frozen at `start`

Covered by `internal/cli/review_new_lineage_frozen_tier_test.go`. It freezes at `RiskLow`, finalizes to approved (so the later validate genuinely reaches `ReviewCore.Next(validate)` instead of being short-circuited by C5's default-deny), then drifts the working tree by staging `internal/auth/new_handler.go` and **proves the fixture non-vacuous** by asserting `ClassifyRisk` over that drifted path returns `RiskHigh` — a fresh computation genuinely disagrees with the frozen tier. It then asserts, after the drift and again after `RunReviewFacadeValidate`, that `Tier`, `CorrectionBudgetLines`, `SelectedLenses` and the store `Revision` are all exactly as frozen. UNTESTED → **COMPLIANT**.

Verbatim run (`-count=1`):

```
--- PASS: TestNewLineageActivationSwitchIdentityNeverOverloadsAnotherSwitch (0.00s)
    --- PASS: TestNewLineageActivationSwitchIdentityNeverOverloadsAnotherSwitch/both_unset (0.00s)
    --- PASS: TestNewLineageActivationSwitchIdentityNeverOverloadsAnotherSwitch/only_shadow_set (0.00s)
    --- PASS: TestNewLineageActivationSwitchIdentityNeverOverloadsAnotherSwitch/only_activation_set (0.00s)
    --- PASS: TestNewLineageActivationSwitchIdentityNeverOverloadsAnotherSwitch/both_set (0.00s)
--- PASS: TestNewLineageActivationSwitchIndependentOfKillSwitch (0.09s)
    --- PASS: TestNewLineageActivationSwitchIndependentOfKillSwitch/kill-switch-resolution-with-activation= (0.00s)
    --- PASS: TestNewLineageActivationSwitchIndependentOfKillSwitch/kill-switch-resolution-with-activation=1 (0.00s)
PASS
ok  	github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction	0.105s
--- PASS: TestNewLineageFrozenTierIsNeverRecomputedAfterFreeze (0.16s)
--- PASS: TestNewLineageKillSwitchOffProducesZeroSideEffectsAcrossEntrySurfaces (0.20s)
PASS
ok  	github.com/gentleman-programming/gentle-ai/v2/internal/cli	0.384s
```

### Coverage

| Requirement | Scenario | Cycle 3 (`157ab9fd`) | Now (`67be4867`) |
|---|---|---|---|
| activation / Distinct Env Switch | Switch identity never overloads another switch | UNTESTED | **COMPLIANT** |
| activation / Kill-Switch-Off Structurally Unfailable | Kill switch off produces no side effect | PARTIAL | **COMPLIANT** |
| review-core / Consent-Gated Freeze | Frozen tier is never recomputed | UNTESTED | **COMPLIANT** |
| all other 29 scenarios | — | COMPLIANT | COMPLIANT (files untouched, suite re-run green) |

**Compliance summary**: **32/32 scenarios COMPLIANT**, 0 PARTIAL, 0 UNTESTED, 0 FAILING. **19/19 requirements** fully compliant (up from 16/19).

### Runtime evidence at tip `67be4867`

| Check | Command | Exit | Result |
|---|---|---|---|
| Root tests | `go test ./... -count=1` | 0 | 63 packages `ok`, 0 FAIL |
| Root build | `go build ./...` | 0 | empty output |
| Bench module tests | `cd bench && go test ./... -count=1` | 0 | `ok github.com/gentleman-programming/gentle-ai/bench 0.156s` |
| Deadcode ratchet | `bash scripts/deadcode-ratchet.sh` | 0 | `no new unreachable functions` |
| Formatter | `gofmt -l .` | 0 | 0 files |
| Vet | `go vet ./...` | 0 | clean |

### Fixture deviations recorded (not coverage deductions)

These are honest notes about how the three fixtures instantiate their scenarios. Each asserts its scenario's THEN clause verbatim; the notes concern the GIVEN.

- **N7 — SUGGESTION.** The frozen-tier fixture freezes `RiskLow` and drifts toward high, rather than the scenario's literal "frozen **tier-4** lineage". The property under test — a later transition never recomputes the persisted tier, lens set, budget or revision — is asserted directly and proved non-vacuous by the `ClassifyRisk` sanity check, and the drift-upward direction is the dangerous one. A tier-4 freeze is reachable through `review start`, so a second table row would make the fixture literal.
- **N8 — SUGGESTION.** The kill-switch fixture deliberately drives the no-`--lineage` hook shape, matching the scenario's "observed call site". Its comment asserts the explicit-`--lineage` maintainer-diagnostic shape is "proven identical whether the kill switch is on or off" — that claim is stated in the comment but not exercised by this test. Either drop the claim or add the case.
- **N9 — SUGGESTION.** The switch-identity fixture covers both activation-centric pairings in both directions, which is this requirement's subject. The `GENTLE_AI_RDD_SHADOW` ↔ kill-switch pairing is not exercised here; it belongs to Wave 1's shadow scope, not to the activation switch's distinctness.

### Carried forward, unchanged by `67be4867`

N1 and N2 (WARNING) remain open and remain Wave 4/5 entry conditions: new-lineage `review finalize` issues an approved receipt with zero captured reviewer lens results at any tier, and the new-lineage gate branch enforces no gate-specific precondition at `pre-pr`/`release`. N3–N6 (SUGGESTION) and the cycle-2 advisories W1–W10, W12, W13 and S1–S5 also remain open. None blocks archive; all block the Wave 5 cutover or are advisory.

### Verdict

**PASS.** Coverage is complete and honest at 19/19 requirements and 32/32 scenarios, with each newly closed scenario mapped to a test that asserts its THEN clause and was independently re-run. `67be4867` is tests-only, so the C5/C6/W11/W14 closure evidence below stands unchanged. Wave 3 is ready for archive.

---

## Prior report (cycle 3, tip `157ab9fd`) — preserved verbatim, envelope stripped


## Verification Report — Third-cycle close-out (FINAL re-verify)

**Change**: rdd-root-simplification-wave3
**Version**: 3 NEW + 3 MODIFIED capability specs — 19 requirements / 32 scenarios (counted from `openspec/changes/rdd-root-simplification-wave3/specs/*/spec.md`)
**Mode**: Strict TDD
**Candidate**: worktree `/home/gentleman/work/gentle-ai-worktrees/rdd-wave0`, branch `feat/rdd-wave3-s5-taxonomy-offer-bench`, tip `157ab9fd`; `git status --porcelain` empty before and after every probe
**Prior report**: FAIL at tip `413d3967` — 2 CRITICAL (C5, C6), 14 WARNING, 5 SUGGESTION (preserved verbatim below)
**Verified by**: independent re-execution. Two binaries were built from source — tip `157ab9fd` (`sha256:db909d38…`) and base `413d3967` (`sha256:9aca0e00…`, from `git archive`) — so every behavioural claim is a measured tip-vs-base delta, not an environment artifact. Nothing in `apply-progress` (#10151) was trusted.
**Attempt authority (echoed, not settled)**: `sha256:0040067d0fe91ff022afd673e94d5cbd771d3af4224a0d1bf7409edf74b36297`

### FINAL VERDICT

**Remediation verdict (this cycle's scope): PASS** — 0 CRITICAL, 2 WARNING (new), 4 SUGGESTION (new). Both cycle-2 blockers are genuinely closed, mutation- and runtime-proven. No new CRITICAL was confirmed. Every new finding below was reproduced identically against the **base** binary `413d3967`, so none is introduced by the 718-line diff under review.

**Envelope verdict: `fail`** — and this is *not* a defect verdict. `gentle-ai sdd-verify-validate` refuses admission for any non-`fail` verdict whose requirement/scenario coverage is incomplete, and coverage is genuinely incomplete: **3 of 32 spec scenarios still have no passing covering test**, unchanged from cycle 2 and untouched by `157ab9fd`, which added no test for any of them:

| Requirement | Uncovered scenario | Status |
|---|---|---|
| activation / Distinct Env Switch | Switch identity never overloads another switch | UNTESTED (no test exists) |
| activation / Kill-Switch-Off Structurally Unfailable | Kill switch off produces no side effect | PARTIAL (only the unwired `OfferReviewAfterVerify` API is attested) |
| review-core / Consent-Gated Freeze | Frozen tier is never recomputed | UNTESTED (no test exists) |

`blockers: 3` names exactly those three; `critical_findings: 0` records that no CRITICAL defect remains. Closing them requires either three covering tests or an explicit spec amendment deferring them to a later wave — a scoping decision for the orchestrator, not a re-run of this cycle's remediation.

### C5 — default-deny at all five gates: **RESOLVED (runtime-proven, tip-vs-base delta)**

Exact cycle-2 attack, isolated `HOME`/`XDG_*`/`TMPDIR`, `GENTLE_AI_RDD_NEW_LINEAGE=1`, throwaway git repo, one staged edit, `review start --lineage inflight` (exit 0, `state: reviewing`, only `v3/inflight/review-state.json` on disk — no receipt), then `review validate --gate <g> --lineage inflight` at all five gates:

| Gate | base `413d3967` | tip `157ab9fd` |
|---|---|---|
| post-apply | `"result":"allow"`, `"allowed":true`, exit 0 | `"result":"invalidated"`, `"allowed":false`, exit 1 |
| pre-commit | `"result":"allow"`, `"allowed":true`, exit 0 | `"result":"invalidated"`, `"allowed":false`, exit 1 |
| pre-push | `"result":"allow"`, `"allowed":true`, exit 0 | `"result":"invalidated"`, `"allowed":false`, exit 1 |
| pre-pr | `"result":"allow"`, `"allowed":true`, exit 0 | `"result":"invalidated"`, `"allowed":false`, exit 1 |
| release | `"result":"allow"`, `"allowed":true`, exit 0 | `"result":"invalidated"`, `"allowed":false`, exit 1 |

Tip denial message, verbatim and identical at all five gates, with the executable next step and a state-naming denial code:

```
Error: review lifecycle gate denied: invalidated: facade review receipt is not available; run gentle-ai review finalize --lineage inflight to produce one
      "denial": { "stage": "new-lineage-validate", "code": "reviewing" }
```

Source confirms the mechanism (`internal/cli/review_governing_authority.go:97-138`): the `GoverningAuthorityKindNew` arm is now an exhaustive switch over `record.Authority.State`; only `approved` **plus** a genuinely loaded, structurally valid, exactly-matching receipt falls through to the relation check that can reach `GateAllow`. `escalated` short-circuits; `default:` (reviewing/correcting/validating and any future value) denies before a relation is even computed.

### Positive control and receipt-integrity gap

Same sandbox, driven to a real terminal state (`review start` → `review finalize` → `review-receipt.json` on disk, `terminal_state: approved`):

| Case | Result |
|---|---|
| approved + valid receipt, exact candidate | `allow` / exit 0 at **all five** gates |
| receipt file **removed** | `invalidated` / exit 1, `code: approved_without_receipt` |
| receipt file **corrupted** (`{ not json`) | `invalidated` / exit 1, `code: approved_without_receipt` |
| receipt `lineage_id` **tampered** to `"other"` | `invalidated` / exit 1, `code: approved_without_receipt` |
| receipt **restored** byte-identically | `allow` / exit 0 again |

The `LoadReceipt` integrity gap the remediation named is genuinely closed: the persisted `state: approved` field alone never authorizes.

### C6 — ReviewCore-owned finalize transitions: **RESOLVED (source- and test-proven)**

- `rg -n 'next\.State\s*=' internal/cli/` returns **zero** production hits; the only 9 matches are test fixtures (`*_test.go`) constructing authorities directly. The single production `next.*` assignment left in `internal/cli` is `review_facade_new_lineage.go:75` (`next.LineageID = lineage`), which is identity, not state.
- `internal/cli` now holds exactly **two** production `store.Mutate` calls: start (`review_facade_new_lineage.go:73`) and finalize (`review_facade_finalize_new_lineage.go:85`), whose entire apply closure is `*next = *transition.Authority`.
- `ReviewCore.finalize` (`review_core.go:131-173`) is a 3-way switch with a fail-closed `default:`; `approved`/`escalated` keep idempotent re-issuance, non-terminal states advance only with a non-nil `AdvanceRequest`. The base version's `default: ErrFinalizeRequiresTerminalState` guard was preserved, not removed (verified against `git show 413d3967:internal/reviewtransaction/review_core.go`).
- `TestReviewCoreFinalizeRequiresTerminalState` **still pins the bare-finalize refusal** and passes untouched, because `AdvanceRequest` is strictly opt-in (nil default).
- End-to-end: `--failed` produces `"state":"escalated"` and `release` then denies with `"result":"escalated"` / exit 1 — the caller's classification reaches finalize's own decision, not a CLI-written state.

### W11 — AST write guard: mutation-proved this run

Baseline `TestDerivedObservationWriteGuardHoldsForProductionFiles` → `ok`. Appended a real writeAtomic-bypass mutant to `internal/reviewtransaction/authority_store.go` (`observation := DeriveObservation(...)` then `writeAtomic(path, []byte(observation.Relation), 0o644)`):

```
--- FAIL: TestDerivedObservationWriteGuardHoldsForProductionFiles/authority_store.go
    authority_store.go marshals or writes a DeriveObservation result:
    [authority_store.go:519:9: writeAtomic is called with a DeriveObservation-derived value (observation)]
FAIL	github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction
```

Reverted byte-identically: file sha256 returned to `ff0663e8a81c710d896ff02122c9ea13002b824ed8d5d57dd73d51d7f677a1ef`, `git status --porcelain` empty, `git diff` 0 lines, HEAD still `157ab9fd`. The old selector-only resolution provably could not match this shape: `writeAtomic(...)` is a bare `*ast.Ident`, and `shadowCallExprName` only resolves `*ast.SelectorExpr`.

### W14 — `correcting` finalizes: resolved at the product-surface function, still unreachable end-to-end

`TestReviewFacadeFinalizeNewLineageCorrectingStateReachesReceipt` PASS. However `rg -n NewLineageStateCorrecting --glob '!*_test.go'` shows the constant is only ever **read/compared** (`review_core.go:146,194`, `authority_store.go:68,76,403`) — no production path ever writes it. A `correcting` lineage cannot be produced by any CLI command in this wave, so the fix is real but exercised only from Go fixtures. Carried as SUGGESTION N5.

### Runtime evidence

| Check | Command | Exit | Result |
|---|---|---|---|
| Root tests | `go test ./... -count=1` | 0 | 63 packages `ok`, 0 FAIL |
| Root build | `go build ./...` | 0 | empty output |
| Bench module tests | `cd bench && go test ./... -count=1` | 0 | `ok github.com/gentleman-programming/gentle-ai/bench 0.176s` |
| Targeted C1–C6/W11/W14 suite | `go test ./internal/cli ./internal/reviewtransaction -run '<12 names>' -v` | 0 | 35 PASS, 0 FAIL |
| Deadcode ratchet | `bash scripts/deadcode-ratchet.sh` | 0 | `no new unreachable functions` |
| Refusal-resolution ratchet | `TestEveryProductionRefusalNamesResolutionOrDeclaresByDesign` | 0 | PASS |
| Formatter | `gofmt -l .` | 0 | 0 files |
| Vet | `go vet ./...` | 0 | clean |
| Bench journeys j59/j60 | `go run . run --binary <fresh tip> --only j59…,j60…` | 0 | both `completed`, 0 failed |

Root test tail (verbatim):

```
ok  	github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction	123.618s
ok  	github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus	25.088s
ok  	github.com/gentleman-programming/gentle-ai/v2/internal/skillregistry	0.120s
ok  	github.com/gentleman-programming/gentle-ai/v2/internal/state	0.102s
ok  	github.com/gentleman-programming/gentle-ai/v2/internal/storage	0.002s
ok  	github.com/gentleman-programming/gentle-ai/v2/internal/system	0.010s
ok  	github.com/gentleman-programming/gentle-ai/v2/internal/tui	2.052s
ok  	github.com/gentleman-programming/gentle-ai/v2/internal/tui/screens	0.060s
?   	github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles	[no test files]
ok  	github.com/gentleman-programming/gentle-ai/v2/internal/update	8.657s
ok  	github.com/gentleman-programming/gentle-ai/v2/internal/update/upgrade	6.931s
ok  	github.com/gentleman-programming/gentle-ai/v2/internal/verify	0.002s
ok  	github.com/gentleman-programming/gentle-ai/v2/internal/versions	0.002s
```

Named tests re-run individually, all PASS: `TestResolveGoverningAuthorityInFlightDeniesEveryGate`, `TestResolveGoverningAuthorityApprovedWithoutReceiptDenies`, `TestResolveGoverningAuthorityApprovedWithValidReceiptAllowsExactCandidate`, `TestReviewCoreFinalizeRequiresTerminalState`, `TestReviewCoreFinalizeAdvancesNonTerminalWithAdvanceRequest`, `TestReviewCoreFinalizeAdvanceEscalatesOnFailedOrAdmittedFindings`, `TestReviewCoreFinalizeRefusesNonTerminalWithoutAdvanceRequest`, `TestReviewFacadeFinalizeNewLineageCorrectingStateReachesReceipt`, plus the C1–C4 closure set (`TestNewLineageReasonTaxonomyCoversLegacyRefusalsClosedMatrix`, `TestAdmitCandidateCausalFindingsBlocksOnlyCandidateCaused`, `TestNewLineageFivePersistedStatesLifecycleNeverPersistsDerivedCategory`, `TestDerivedObservationWriteGuardCatchesMarshalShapes`, `TestDerivedObservationWriteGuardHoldsForProductionFiles`) — C1–C4 remain closed.

### Spec compliance matrix — deltas from cycle 2

| Requirement | Scenario | Test | Cycle 2 | Now |
|---|---|---|---|---|
| activation / Coexistence Precedence (Amendment C) | New-lineage receipt authorizes a new-lineage candidate | `TestResolveGoverningAuthorityInFlightDeniesEveryGate` + `…ApprovedWithValidReceiptAllowsExactCandidate` + `…ApprovedWithoutReceiptDenies` + tip-vs-base binary repro | FAILING | **COMPLIANT** |
| activation / Rollback Disables New Starts Only | In-flight new lineage still finalizes after rollback | `TestReviewFacadeFinalizeNewLineageCorrectingStateReachesReceipt` (the scenario's literal `correcting` GIVEN now reaches a receipt through `RunReviewFacadeFinalize`) | PARTIAL | **COMPLIANT** |
| review-core / Sole Transition Owner | Only ReviewCore transitions new-lineage state | Zero production `next.State` writes in `internal/cli`; `TestReviewCoreFinalize{Advances,Refuses}NonTerminal…` table over reviewing/correcting/validating | FAILING | **COMPLIANT** |
| activation / Kill-Switch-Off Structurally Unfailable | Kill switch off produces no side effect | `TestOfferReviewAfterVerifyDisabledKillSwitchReturnsUnavailableBeforeRepoRead` (unwired API only) | PARTIAL | PARTIAL |
| activation / Distinct Env Switch | Switch identity never overloads another switch | (none found) | UNTESTED | UNTESTED |
| review-core / Consent-Gated Freeze | Frozen tier is never recomputed | (none found) | UNTESTED | UNTESTED |

All other 26 rows from the cycle-2 matrix carry forward COMPLIANT (their files were untouched by `157ab9fd` and their tests re-ran green).

**Compliance summary**: 29/32 scenarios COMPLIANT, 1 PARTIAL, 2 UNTESTED, **0 FAILING**. 16/19 requirements fully compliant (up from 13/19).

### New findings this cycle

None is a new CRITICAL: every one reproduces identically on the base binary `413d3967`, so none was introduced by `157ab9fd`. Severity reflects Wave-5 cutover risk, not Wave-3 archive risk — the activation switch is off by default and this wave is explicitly additive, not a cutover.

- **N1 — WARNING (CONFIRMED, pre-existing).** New-lineage `review finalize` issues an `approved` terminal receipt with **zero captured reviewer lens results at any tier**, not only tier 0 as the function's own doc comment claims. Repro: 902-line staged candidate → `review start --lineage bigt` reports `"risk_level":"medium"`, `"selected_lenses":["review-reliability"]`, `"correction_budget":200`; `review finalize --lineage bigt` (no result flags) → `"state":"approved"` + receipt; `review validate --gate release --lineage bigt` → `"result":"allow"`, exit 0. The legacy path in the same repo refuses the equivalent: `Error: review finalize requires all 1 original reviewer result(s); capture each missing one with 'gentle-ai review capture-result'…`. Base `413d3967` behaves identically. Mitigating: `review_facade.go:2192-2196` explicitly **refuses** `--result`, `--result-artifact`, `--result-artifact-file`, `--captured-results`, `--captured-evidence`, `--validation`, `--refuter`, `--evidence` with a named next step, so the gap is declared at the product surface rather than silently ignored. Blocks the Wave 5 cutover; must be closed before the new path can ever become the default.
- **N2 — WARNING (CONFIRMED, pre-existing).** The new-lineage gate branch enforces **no gate-specific precondition**. `newLineageGateEvaluation` (`review_governing_authority.go:246-261`) maps `CoreTransitionContinue → GateAllow` identically for all five gates, while legacy `validateDerivedGate` (`receipt.go:279-321`) additionally requires release evidence at `release`, `BaseRelationshipValid` at `pre-pr`/`release`, and a matching `Generation`. Repro: an approved new lineage allows at `--gate release` with no release evidence supplied at all (exit 0); the emitted context carries `"base_relationship_valid": false`. Base identical. Same Wave-5 cutover blocker class as N1.
- **N3 — SUGGESTION (CONFIRMED).** The gate's receipt cross-check (`review_governing_authority.go:104-105`) compares `LineageID`, `AuthorityRevision` and `TerminalState`, but not `CandidateIdentity` — even though `WriteReceipt` (`authority_store.go:484`) performs exactly that check at issuance. Repro: tamper `review-receipt.json`'s `candidate_identity.CandidateTree` to all zeros, leaving lineage/revision/state intact → `--gate release` still returns `"result":"allow"`, exit 0. Inert today (no production code reads `NewLineageReceipt.CandidateIdentity` after issuance, and the relation check uses the content-verified state record), but the receipt is the artifact an external consumer would trust. One-line fix: reuse the `WriteReceipt` equality check at the gate.
- **N4 — SUGGESTION.** `CoreRequest.AdvanceRequest` converts the `ErrFinalizeRequiresTerminalState` invariant into an opt-out: any caller that sets the field advances a non-terminal authority to a terminal state. Only `runReviewFacadeFinalizeNewLineage` does today, and the nil-default pinning test guards the omission, but nothing structurally prevents a second caller. Consider a guard test asserting the single call site, mirroring the `DeriveObservation` write-guard precedent.
- **N5 — SUGGESTION.** `NewLineageStateCorrecting` has no production writer (grep-proven, see W14 above). The W14 fix cannot be exercised end-to-end through the binary in this wave; its only driver is a Go fixture.
- **N6 — SUGGESTION.** `AuthorityStore.LoadReceipt` (`authority_store.go:502-515`) uses plain `json.Unmarshal` — no `DisallowUnknownFields`, no trailing-value rejection — while `parseNewLineageRecord` (`authority_store.go:182-207`) enforces both plus a recomputed content digest. Asymmetric strictness across the store's two artifacts.

### Adversarial probes that found nothing (examined, refuted)

- **Unknown/empty state values in the exhaustive switch.** Doubly fail-closed: `NewLineageAuthority.Validate()` rejects any state outside the five at load time (`authority_store.go:76`), so a record with an out-of-vocabulary state cannot even parse; and the gate's `default:` arm denies regardless. `ReviewCore.finalize` and `newLineageGateEvaluation` each keep a fail-closed `default:`.
- **TOCTOU between `LoadReceipt` and the gate decision.** A real window exists (the record is read at `:48`, live evidence at `:59`, the receipt only at `:103`), but every skew fails closed: a concurrent finalize produces a revision the in-memory record no longer matches → deny; and `publishImmutable` makes a second receipt at a different revision impossible, so a stale-but-genuinely-approved read can never authorize more than the revision it observed. Not exploitable.
- **`AdvanceRequest` allowing a non-terminal skip.** `reviewing → approved` does skip `correcting`/`validating`, but no spec requirement mandates traversal, and admitted findings route to `escalated`, never `correcting`. Matches the documented tier-0 scope.
- **Receipt re-issuance.** A second `review finalize` on an already-approved lineage returns `"state":"approved"` idempotently, exit 0, receipt bytes unchanged — consistent with "Terminal Receipt Issuance Exactly Once".
- **Marker present, record removed.** `review validate` denies with `invalidated` / exit 1 and the corrupted-inventory reason, never falling through to legacy.
- **Guard removal in the diff.** `git show 157ab9fd` removed no invariant: the only deleted production lines are the old `finalize` body, whose `default: ErrFinalizeRequiresTerminalState` guard is preserved verbatim in the new switch.

### Carried forward, not re-adjudicated

W1–W10, W12, W13 and S1–S5 from the cycle-2 report below remain open. This cycle's scope was C5, C6, W11 and W14 only; those advisories were neither re-tested nor re-scoped here.

### Verdict

Both cycle-2 CRITICALs are closed with independent runtime, source and mutation evidence; the tip-vs-base delta proves the fix, not the environment. Zero new CRITICAL confirmed, so the remediation itself passes and no further apply cycle is warranted for C5/C6/W11/W14.

Archive remains gated on the 3 carried-over uncovered spec scenarios listed above (16/19 requirements, 29/32 scenarios), which is why the machine envelope reads `fail`. N1 and N2 must additionally be closed before the Wave 5 cutover and should be recorded as Wave 4/5 entry conditions.

---

## Prior report (cycle 2, tip `413d3967`) — preserved verbatim

```

## Verification Report — Re-verification (corrective re-run)

**Change**: rdd-root-simplification-wave3
**Version**: 3 NEW + 3 MODIFIED capability specs — 19 requirements / 32 scenarios (counted from `openspec/changes/rdd-root-simplification-wave3/specs/*/spec.md`)
**Mode**: Strict TDD
**Candidate**: worktree `/home/gentleman/work/gentle-ai-worktrees/rdd-wave0`, branch `feat/rdd-wave3-s5-taxonomy-offer-bench`, tip `413d3967`, working tree clean before and after every mutation probe (`git status --porcelain` empty)
**Prior report**: FAIL at tip `ebe26c9c` — 4 CRITICAL (C1–C4), 10 WARNING, 4 SUGGESTION (preserved verbatim below)
**Verified by**: independent re-execution against a freshly built tip binary. Every remediation claim in `413d3967` and in `apply-progress` (#10151) was re-derived from source and runtime; nothing was trusted.
**Attempt authority (echoed, not settled)**: `sha256:a6de7b19f50b9d351833c3f0e9cdc417c7aab9e5e4103030bc5111b9bb6c9f47`

### What changed since the prior report

One commit, `413d3967` (`fix(review): route finalize by lineage kind and complete new-lineage evidence`), 10 files / 1043 insertions / 4 deletions. No prior slice was rewritten, so every closure claim from the prior report that did not touch these files carries forward and was re-confirmed at this tip.

| Prior CRITICAL | Claim | Independent verdict |
|---|---|---|
| C1 — `review finalize` not routed by lineage kind | Fixed by `runReviewFacadeFinalizeNewLineage` + a discovery/routing block in `runReviewFacadeFinalize` | **RESOLVED** — CLI repro reproduced end-to-end; approved receipt issued, no defect report |
| C2 — candidate-causal admission unimplemented | Implemented as `AdmitCandidateCausalFindings`, wired via `--admission-findings` | **RESOLVED** — real run persists only the candidate-causal ID; non-causal never blocks and never admits |
| C3 — 24-cell taxonomy matrix is a tautology | Expectation side rewritten as a hardcoded literal table | **RESOLVED** — mutation-killed this run |
| C4 — five-persisted-states unattested | Lifecycle runtime test + `DeriveObservation` AST write guard | **RESOLVED** — both tests pass; guard mutation-killed against a real production file (with a documented coverage gap, W11) |
| Latent escalated-allow bug (adjudication b) | `resolveGoverningAuthority` short-circuits `escalated` | **RESOLVED** — mutation-killed; denial confirmed at all five gates through the built binary |

All four prior blockers are genuinely closed. Two new CRITICAL findings surfaced during this re-verification (C5, C6); both are consequences of the same design seam the remediation opened up, and both are stated below with the exact evidence that produced them.

### Chain Integrity

Unchanged from the prior report except for the appended remediation commit.

| Slice | Branch | Commit | Changed lines |
|---|---|---|---|
| S5 | `feat/rdd-wave3-s5-taxonomy-offer-bench` | `ebe26c9c` | 745 |
| Remediation | same branch (no new slice) | `413d3967` | 1047 (10 files) |
| Wave 3 total | — | — | 4947 |

The remediation commit alone is 1047 changed lines — 2.6x the 400-line reviewer budget on top of an already-oversized S5. Declared under `auto-chain` / High risk, so accepted by plan, but it lands as a sixth de-facto slice on an existing branch rather than its own PR (W12).

### Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 40 |
| Tasks complete | 39 |
| Tasks incomplete | 1 (1.2, Wave 2 archive move — Wave-2-timing dependent, deferred by design) |

Task state matches code state: every `[x]` phase in `tasks.md` has its named commit and its named artifacts on disk at this tip.

### Build & Tests Execution

**Build**: PASSED — `go build ./...`, exit 0, empty output (`sha256:e3b0c442…`). `gofmt -l .` clean, `go vet ./...` clean.

**Tests**: PASSED — `go test ./... -count=1`, exit 0, 63 packages ok, 0 FAIL, output `sha256:2e973a7e…`.

**Bench module** (separate Go module, own `go.mod`, NOT covered by the root suite): `go build ./...`, `gofmt -l .`, `go vet ./...`, `go test ./... -count=1` all clean.

**Ratchets**: `scripts/deadcode-ratchet.sh` → "no new unreachable functions"; baseline 236 → 233. `TestEveryProductionRefusalNamesResolutionOrDeclaresByDesign` → PASS.

**Deadcode ratchet, receipt-path entries**: `AuthorityStore.ReceiptPath`, `AuthorityStore.WriteReceipt`, and `NewLineageReceipt.Validate` are ABSENT from `.deadcode-baseline.txt` and the ratchet is clean — independent structural confirmation that C1's fix is genuinely production-reachable from `./cmd/gentle-ai`, not dead code.

**Bench journeys** re-executed against the freshly built tip binary: `j59` and `j60` both `completed`, 2/2, 0 failed. `bench/results.json` was dirtied by the run and restored (pre-existing S4 issue).

**Coverage**: skipped — no coverage tool configured for this repository.

### Independent Re-Execution (nothing trusted)

1. **C1 — the prior report's exact repro, freshly built tip binary, isolated `HOME`.**
   `GENTLE_AI_RDD_NEW_LINEAGE=1 gentle-ai review start --lineage vthree-check` → exit 0, `state: reviewing`, `v3/vthree-check/review-state.json` only.
   `gentle-ai review finalize --lineage vthree-check` (activation switch OFF — i.e. the rollback path too) → exit 0, `state: approved`, `receipt.authority_revision: sha256:8deb50c1…`, and `v3/vthree-check/review-receipt.json` published with `terminal_state: approved`.
   **Zero defect reports** anywhere under `.git/gentle-ai/defect-reports/`. The prior report's `operation-outcome-unknown` failure is gone.
2. **C1 escalated route.** `review finalize --lineage esc --failed` → `state: escalated`, receipt with `terminal_state: escalated`. Both terminal routes reach a real on-disk receipt.
3. **C2 admission in a real run.** A four-finding file (`introduced`, `pre-existing`, `base-only`, `unknown`) through `--admission-findings` → `state: escalated`, and `review-state.json` `admitted_finding_ids` = `["F-CAUSAL"]` **only**. A non-causal-only file → `state: approved`, `admitted_finding_ids` absent entirely. Non-causal findings neither block nor admit, exactly as the spec requires.
4. **C3 mutation.** Flipped `legacyReasonTaxonomyGateResult`'s `ReviewReceiptScopeChanged` arm to `GateAllow`. `TestNewLineageReasonTaxonomyCoversLegacyRefusalsClosedMatrix` **FAILED** on 2 of its 24 cells (`receipt_scope_changed/allow` and `/scope-changed`). Reverted; `cmp` byte-identical to the pre-mutation file, `git status --porcelain` empty, both taxonomy tests PASS. The tautology is genuinely gone.
5. **C4b guard mutation, two directions.**
   - Added `json.Marshal(observation)` after `review_core.go:154`'s `DeriveObservation` call → `TestDerivedObservationWriteGuardHoldsForProductionFiles/review_core.go` **FAILED** with the exact position and identifier. The guard is live against real files, not just synthetic sources.
   - Added `writeAtomic("…", []byte(observation.Relation), 0o644)` — and separately `publishImmutable(…)` — to `authority_store.go` → the guard **PASSED both times**. See W11.
   - Both mutants reverted byte-identically.
6. **Escalated-deny pinning test is non-vacuous.** Disabled the short-circuit (`if false && …`) → all five subtests FAILED with `Result:"allow", Reason:"exact"`. Reverted; PASS.
7. **Escalated denial at the product surface.** Drove the escalated `esc` lineage through `review validate` at all five gates with the tip binary: every gate returned `"result": "escalated"`, `"allowed": false`, reason `authority already escalated: escalated is a terminal non-approval`, plus a runnable `gentle-ai review recover …` continuation. State stayed `escalated`; neither artifact gained a derived-category word.
8. **C4 "no gate writes a derived category", runtime.** Ten real gate evaluations across two v3 lineages (approved + escalated), then a byte scan of `review-state.json` and `review-receipt.json` for `invalidated` / `scope_changed` / `scope-changed` / `ambiguous` / `repairable` / `corrupted`: none present, states unchanged. This is the runtime gate evidence the AST guard alone could not supply.
9. **Receipt exactly once, product surface.** A second `review finalize` on the already-approved lineage returned the identical receipt and left `review-receipt.json` byte-identical (`sha256:3cdba257…` before and after). Exactly one receipt exists.
10. **Regression spot-checks all green:** five switch-off byte-equivalence goldens (`…SwitchOffByteEquivalence{PostApply,PreCommit,PrePush,PrePR,Release}`), `TestReviewStartNewLineageSwitchOffCreatesNoV3Entries`, `TestReviewValidateDiscoveryIntegrityMarkerCorruptedDeniesNeverLegacy`, `TestDiscoverNewLineageMarkerPresentRecordRemovedDenies`, `TestReviewCoreStartFreezesOnlyAfterConsentGranted`, `TestReviewCoreConsentBlockedStartPersistsNothing`, the full `TestAuthorityStore*` suite, `TestNewLineageRollbackSafetyStaysReadableAndFinalizableWhileSwitchIsOff`, `TestResolveGoverningAuthorityFullMatrix`.
11. **NEW — non-terminal authority reaches `allow` at every gate (C5).** Started `inflight` with the switch on and never finalized (`state: reviewing`, no receipt). `review validate --gate <g> --lineage inflight` returned `"result":"allow","allowed":true` at **post-apply, pre-commit, pre-push, pre-pr, and release**. The identical situation on the legacy path denies: `review validate --gate release --lineage legflight` returns `Error: facade review receipt is not available; run gentle-ai review finalize --lineage legflight to produce one`. Without an explicit `--lineage` both paths correctly deny with `no terminal review receipt exists for gate validation`, so the divergence is confined to — and complete at — the explicit-marker gate shape the negotiated delivery lifecycle uses.

### Spec Compliance Matrix

| Requirement | Scenario | Test | Result |
|---|---|---|---|
| authority-store / Two-Artifact Model | New lineage writes exactly two artifacts | `TestAuthorityStoreNewLineageWritesExactlyTwoArtifacts` + CLI directory inspection | COMPLIANT |
| authority-store / Five Persisted States | Derived category is never a stored value | `TestNewLineageFivePersistedStatesLifecycleNeverPersistsDerivedCategory` (2 paths, byte-level) | COMPLIANT |
| authority-store / Five Persisted States | No gate writes a derived category | `TestDerivedObservationWriteGuardHoldsForProductionFiles` (mutation-killed) + 10 independent CLI gate runs with byte scan | COMPLIANT |
| authority-store / CAS Mutation | Stale revision refuses the write | `TestAuthorityStoreMutateRefusesStaleExpectedRevision` | COMPLIANT |
| authority-store / CAS Mutation | Replay identity is self-contained | `TestAuthorityStoreResolveReplayReturnsStoredTransitionWithoutMutation` | COMPLIANT |
| authority-store / Receipt Immutability | Post-issuance write attempt is rejected | `TestAuthorityStoreReceiptImmutableAfterIssuance` + byte-identical receipt across a repeat CLI finalize | COMPLIANT |
| authority-store / Reason Taxonomy Regression | Legacy refusal reason has a new-model equivalent | `…CoversLegacyRefusalsClosedMatrix` (mutation-killed this run) + `…CoversLegacyRefusals` | COMPLIANT |
| candidate-identity / Read-Only Resolution + Persisted Freeze | Resolution has no side effect | `TestCandidateReadOnlyGuardHoldsForPromotedFiles` | COMPLIANT |
| candidate-identity / Read-Only Resolution + Persisted Freeze | New-lineage start persists resolved identity as frozen authority | `TestReviewStartNewLineageSwitchOnFreezesV3Authority` + CLI inspection | COMPLIANT |
| relation-algebra / Read-Only Legacy, Deciding New | Shadow evaluation changes nothing observable | 5 switch-off goldens + prior base-vs-tip binary diff | COMPLIANT |
| relation-algebra / Read-Only Legacy, Deciding New | New-lineage call site consumes the same function | Rename proven logic-identical; `resolveGoverningAuthority` drives `DeriveObservation` | COMPLIANT |
| activation / Distinct Env Switch | Default configuration takes the legacy path | `TestReviewStartNewLineageSwitchOffCreatesNoV3Entries` + CLI legacy start | COMPLIANT |
| activation / Distinct Env Switch | Switch identity never overloads another switch | (none found) | UNTESTED |
| activation / Kill-Switch-Off Structurally Unfailable | Kill switch off produces no side effect | `TestOfferReviewAfterVerifyDisabledKillSwitchReturnsUnavailableBeforeRepoRead` (unwired API only) | PARTIAL |
| activation / Coexistence Precedence (Amendment C) | Legacy authority alone denies a new-lineage candidate | `TestResolveGoverningAuthorityFullMatrix` + `…MarkerCorruptedDeniesNeverLegacy` | COMPLIANT |
| activation / Coexistence Precedence (Amendment C) | New-lineage receipt authorizes a new-lineage candidate | Contradicted at runtime: a receipt-less `reviewing` record authorizes identically at all 5 gates | FAILING |
| activation / Additive Gate Branch | Switch-off byte-equivalence at every gate | 5 golden tests + prior independent base-vs-tip binary diff | COMPLIANT |
| activation / Additive Gate Branch | New branch never touches legacy code path | `TestResolveGoverningAuthorityAbsentWithoutMarkerCostsNoGitCall` + governs-true short circuit | COMPLIANT |
| activation / Rollback Disables New Starts Only | In-flight new lineage still finalizes after rollback | Product surface proven for `reviewing` (switch off → approved receipt); the scenario's literal `correcting` GIVEN is refused by the CLI and covered only at the Go API layer | PARTIAL |
| activation / Rollback Disables New Starts Only | Rollback blocks only new starts | `TestNewLineageRollbackSafetyStaysReadableAndFinalizableWhileSwitchIsOff` + CLI legacy start with switch off | COMPLIANT |
| review-core / Sole Transition Owner | Only ReviewCore transitions new-lineage state | Contradicted by source: `internal/cli` now advances state with two direct `AuthorityStore.Mutate` calls | FAILING |
| review-core / Consent-Gated Freeze | Tier 1 candidate freezes only after consent | `TestReviewCoreStartFreezesOnlyAfterConsentGranted` (6 cases) + `TestReviewCoreConsentBlockedStartPersistsNothing` | COMPLIANT |
| review-core / Consent-Gated Freeze | Frozen tier is never recomputed | (none found) | UNTESTED |
| review-core / Candidate-Causal Admission | Candidate-caused finding blocks | `TestAdmitCandidateCausalFindingsBlocksOnlyCandidateCaused` + `…AdmitsOnlyCandidateCausalFindings` + independent CLI run | COMPLIANT |
| review-core / Candidate-Causal Admission | Pre-existing finding becomes a follow-up | Same table + `…FollowUpFindingsDoNotBlock` + independent CLI run | COMPLIANT |
| review-core / One Bounded Correction | Second correction attempt refuses | `TestAuthorityStoreResolveReplayRefusesSecondCorrectionAttempt` | COMPLIANT |
| review-core / One Bounded Correction | Exact replay costs nothing | `TestAuthorityStoreResolveReplayReturnsStoredTransitionWithoutMutation` | COMPLIANT |
| review-core / Terminal Receipt Exactly Once | Finalize issues one receipt | `TestReviewCoreFinalizeApprovedIssuesReceiptRef` + `…ReachesReceiptNoBlocker` + repeat-finalize byte-identity | COMPLIANT |
| shadow-evaluation / Disable Switch Is Observer Boundary | Disabling removes shadow observer execution only | Wave 1 `shadow_observer_test.go` | COMPLIANT |
| shadow-evaluation / Disable Switch Is Observer Boundary | Resolver and relation stay live-callable independent of the shadow switch | `TestReviewCoreValidate*` with `GENTLE_AI_RDD_SHADOW` unset | COMPLIANT |
| shadow-evaluation / Off by Default in Live Paths | Default configuration produces no live Git cost from the observer | Wave 1 suite | COMPLIANT |
| shadow-evaluation / Off by Default in Live Paths | New-lineage live cost is not observer cost | `governingAuthorityLiveEvidence` builds its own snapshot outside the observer | COMPLIANT |

**Compliance summary**: 26/32 scenarios COMPLIANT, 2 PARTIAL, 2 UNTESTED, 2 FAILING. 13/19 requirements fully compliant (up from 21/32 and 10/19).

### Correctness (Static Evidence)

| Requirement | Status | Notes |
|---|---|---|
| Two-artifact v3 store, store-scoped lock | Implemented | Confirmed again by CLI directory inspection at tip |
| CAS on content-addressed revision | Implemented | Unchanged by the remediation |
| Five persisted states | Implemented | `NewLineageState.valid()` closed switch; now with runtime + static evidence |
| Consent before authority freeze | Implemented | Unchanged; the new finalize path calls `authorizeReviewAuthorityMutation` before any non-terminal mutation |
| Amendment C matrix + discovery-integrity denial | Implemented | Finalize now reuses the identical `DiscoverNewLineage` corruption denial |
| Candidate-causal admission | Implemented | `AdmitCandidateCausalFindings` + CLI wiring; wired at `finalize`, not `validate` (W13) |
| Finalize/correction state machine reachable in production | Partially | `reviewing → validating → approved|escalated` is reachable; `correcting` is refused by finalize and unreachable through any CLI verb (W14) |
| Gate authorization requires an approved terminal authority | NOT implemented | Only `escalated` is state-checked; `reviewing`/`correcting`/`validating` all reach `GateAllow` — C5 |
| ReviewCore is the sole writer of new-lineage state | NOT implemented | `internal/cli` advances state directly — C6 |

### Coherence (Design)

| Decision | Followed? | Notes |
|---|---|---|
| 1 — single `ReviewCore.Next` entry, six-value transition vocabulary | Deviates | `Next` is still the single entry for *decisions*, but the remediation moved *state advancement* out of it — see C6 |
| 2 — promote by rename, alias, guard split | Yes | Unchanged |
| 3 — v3 root, store-scoped lock, CAS, in-record replay identity | Yes | Re-confirmed through the real binary |
| 4 — one `resolveGoverningAuthority` shared by all five gates | Yes | Single call site; the escalated short-circuit sits inside it, so it applies uniformly at all five gates by construction and was confirmed at all five through the binary |
| 5 — start-only activation switch, finalize/validate route on discovered kind | Yes | **Closed.** Finalize now discovers via `DiscoverNewLineage` and routes on kind, mirroring start (`:1618`) and validate |
| 6 — reuse consent/tier/lenses/budget, no re-derivation | Yes | Unchanged |
| 7 — reason taxonomy mapping | Deviates | `ambiguous`/`unknown`/`unrelated` → `escalate`, not `stop` — W2 carried forward, now with a concrete consequence (see C5's interaction) |
| 8 — ship `OfferReviewAfterVerify` unwired with baselined entry | Yes | Unchanged |
| Testing Strategy — AST guard that no `DeriveObservation` result is persisted | Built, incomplete | Guard exists and is mutation-killed for marshal shapes, but misses the package's own two write primitives — W11 |
| Testing Strategy — bench tier 0/1/4 candidate→consent→review→correction→receipt→five gates | Deviates | Receipt is now reachable through the product, but no bench journey drives it; `j59`/`j60` still stop at gate evaluation — S3/S5 |
| Zero live-lifecycle change | Yes | Switch-off byte-equivalence goldens all pass; the remediation's production edits are confined to the `governs==true` / v3-discovered branches |

### TDD Compliance

| Check | Result | Details |
|---|---|---|
| TDD Evidence reported | Yes | Present in apply-progress (#10151, remediation batch) |
| All tasks have tests | Yes | Every remediation fix has a named test file; all exist |
| RED confirmed (tests exist) | Yes | All 5 new/modified test files present |
| GREEN confirmed (tests pass) | Yes | All re-executed with `-count=1` |
| RED genuineness re-proven by mutation | Yes, 3/3 this run | Taxonomy matrix, AST guard (marshal shape), escalated short-circuit — all killed their tests |
| Triangulation adequate | Yes | Admission 6 dispositions; escalated denial 5 gates; guard 4 synthetic shapes + 6 production files; lifecycle 2 full paths |
| Safety Net for modified files | Yes | The 3 deadcode-baseline removals were verified newly-reachable, not newly-dead |

**TDD Compliance**: 7/7 checks passed.

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|---|---|---|---|
| Unit (pure Go, no repo) | 16 | 5 | `go test` |
| Integration (`t.TempDir()` real Git repos, in-process CLI entry points) | 26 | 9 | `go test` |
| Black-box (built binary — bench journeys + this report's 20+ CLI probes) | 2 journeys | 1 | `bench` module |
| **Total** | **44** | **15** | |

### Assertion Quality

The prior report's single CRITICAL assertion defect (the 24-cell tautology) is fixed and mutation-verified. The five new test files were audited line by line.

| File | Line | Assertion | Issue | Severity |
|---|---|---|---|---|
| `internal/cli/review_facade_finalize_new_lineage_test.go` | 243-245 | `if strings.Contains(err.Error(), "no such file") && strings.Contains(err.Error(), "v2")` | Negative-shape assertion only; any non-`nil` error passes, so it does not pin the typed corruption denial itself | WARNING |
| `internal/cli/review_facade_finalize_new_lineage_test.go` | 312-316 | `if discoveryErr != nil { return }` inside the 5-gate loop | Early return accepts any typed discovery denial without asserting which one; the preceding allow-sanity check keeps it non-vacuous | SUGGESTION |

No tautologies, no ghost loops, no production-call-free assertions, no smoke-only tests. `…EscalatedReceiptDeniesEveryGate` carries an explicit pre-escalation allow-sanity assertion that makes the 5-gate loop provably non-vacuous — good practice, and independently confirmed by my own mutation run.

**Assertion quality**: 0 CRITICAL, 1 WARNING, 1 SUGGESTION.

### Quality Metrics

**Formatter**: `gofmt -l .` clean (root and `bench/`).
**Vet**: `go vet ./...` clean (root and `bench/`).
**Deadcode ratchet**: clean, 233 baselined entries (236 → 233; the 3 removals are the receipt-path symbols C1 made reachable).
**Refusal ratchet**: `TestEveryProductionRefusalNamesResolutionOrDeclaresByDesign` PASS.

### Issues Found

**CRITICAL**

- **C5 — a new-lineage authority that was never approved authorizes delivery at all five gates.** `resolveGoverningAuthority` (`internal/cli/review_governing_authority.go:80-108`) consults `record.Authority.State` for exactly one value, `escalated`. Every other state falls through to `ReviewCore.Next(validate)`, whose relation-based switch returns `CoreTransitionContinue` for an exact relation, which `newLineageGateEvaluation` maps to `GateAllow`. Reproduced with the tip binary: `GENTLE_AI_RDD_NEW_LINEAGE=1 review start --lineage inflight`, nothing else — no reviewer, no finalize, no `review-receipt.json` — then `review validate --gate {post-apply,pre-commit,pre-push,pre-pr,release} --lineage inflight` returns `"result":"allow","allowed":true` at **all five**, including `release`. The legacy path denies the identical situation (`facade review receipt is not available; run gentle-ai review finalize …`), and the no-`--lineage` hook shape denies on both paths with `no terminal review receipt exists for gate validation` — so this is a genuine, one-sided authorization divergence at the explicit-marker gate shape the negotiated delivery lifecycle uses. Breaks spec `rdd-new-lineage-activation` → "Coexistence Precedence Matrix (Amendment C)" scenario "New-lineage receipt authorizes a new-lineage candidate": authorization is decided by candidate relation alone and never consults the receipt or the terminal state. Corroborating evidence from the wave's own code: the remediation's `TestReviewFacadeFinalizeNewLineageEscalatedReceiptDeniesEveryGate` sanity check (`review_facade_finalize_new_lineage_test.go:292`) *asserts* that a `reviewing` authority with an exact live candidate must `GateAllow` — the bypass is currently pinned as expected behavior. The remediation's own commit message states the correct principle ("silently authorizing delivery from a lineage that was never approved") and then applies it to one state out of four. Fix: deny unless `record.Authority.State == approved` and a terminal receipt exists, and re-point that sanity assertion at an approved fixture.
- **C6 — `internal/cli` now mutates new-lineage state directly, contradicting the domain's first requirement.** `runReviewFacadeFinalizeNewLineage` (`internal/cli/review_facade_finalize_new_lineage.go:78-91`) calls `AuthorityStore.Mutate` twice, itself setting `next.State = validating` and then `next.State = approved|escalated` and appending `AdmittedFindingIDs`. `ReviewCore.Next(finalize)` is consulted only *after* the CLI has already decided and persisted the terminal state. Breaks spec `rdd-review-core-transitions` → "Sole Transition Owner for New Lineages": "`ReviewCore` MUST be the only component that performs `start`, `finalize`, or `validate` for a new-lineage candidate. No other package, adapter, or gate MAY mutate new-lineage state directly." This is not the START precedent: `runReviewFacadeStartNewLineage` (`review_facade_new_lineage.go:73-78`) persists `*next = *transition.Authority` — ReviewCore decides, the CLI stores. Finalize inverts that. `apply-progress` records the reasoning (`TestReviewCoreFinalizeRequiresTerminalState` locks `ReviewCore.finalize` to an already-terminal authority, and `validate()` never returns `approve`, so ReviewCore as shipped cannot perform this advance) — which is itself the finding: Wave 3's own `ReviewCore` API cannot satisfy Wave 3's own sole-owner requirement. Resolve by either moving the advance inside `ReviewCore` (a new request kind or a relaxed `finalize`) or recording an explicit spec amendment narrowing "sole transition owner" to decision authority, with a guard test pinning whichever is chosen. Left as-is, Wave 4's "thin consumers move to opaque transitions" rests on an invariant that production already violates.

**WARNING**

- W1 — task 1.2 (Wave 2 archive move) unchecked. Cleanup/sequencing, Wave-2-timing dependent. *(carried forward)*
- W2 — design decision 7 deviation: `ambiguous`/`unknown`/`unrelated` map to `escalate` rather than `stop`, and `escalated` is a finalize-eligible terminal state. Still needs a recorded amendment before Wave 4 persists validate outcomes. *(carried forward — delivery advisory)*
- W3 — `legacyReasonTaxonomyGateResult` is an unanchored approval snapshot of `review_facade.go:2905-2910`/`:3008-3025`. The C3 fix added a second hand-written table, so the "legacy" half is now double-unanchored: both sides can drift together if that handling changes. *(carried forward, slightly widened)*
- W4 — the five switch-off goldens remain self-captured post-change. Mitigated: the prior report proved equivalence independently against the `cb5ade42` binary at all five gates. *(carried forward — delivery advisory)*
- W5 — spec scenario "Switch identity never overloads another switch" has no covering test. *(carried forward)*
- W6 — spec scenario "Frozen tier is never recomputed" has no covering test. *(carried forward)*
- W7 — "Kill switch off produces no side effect" is covered only for the unwired `OfferReviewAfterVerify`, not for the facade at its five gate call sites. *(carried forward)*
- W8 — S5 self-certified the Wave 3 exit-evidence bar by editing the design doc in the slice being measured against it. The wave-table row itself is unchanged, so the reinterpretation does not bind. *(carried forward)*
- W9 — S2 (877), S4 (897), S5 (745) each exceed the 400-line reviewer budget by >2x. Declared under `auto-chain` / High risk. *(carried forward — delivery advisory)*
- W10 — `governingAuthorityLiveEvidence` still uses the workspace projection at `pre-push`, `pre-pr`, and `release`; only `pre-commit` uses the staged projection. Documented with a defensible reason. *(carried forward — delivery advisory)*
- **W11 — the `DeriveObservation` AST write guard misses the two write primitives that actually persist v3 state.** `derivedObservationWritePrimitives` contains only `json.Marshal`, `json.MarshalIndent`, `os.WriteFile`, `ioutil.WriteFile`, while the guard's own doc comment (lines 26-29) claims `writeAtomic` and `publishImmutable` are in scope. Those two are exactly how `authority_store.go` — one of the six scanned files — writes `review-state.json` and `review-receipt.json`. Proven by mutation: `writeAtomic(…, []byte(observation.Relation), …)` and `publishImmutable(…)` added to `authority_store.go` both left the guard green. Secondary gap: the file list is a hardcoded 6-entry allowlist over an 88-file package, so a new file marshaling a derived observation is never scanned. The requirement itself is not left unattested — the lifecycle test and my 10 gate runs supply runtime evidence — but the guard's stated coverage exceeds its actual coverage.
- **W12 — the remediation is a sixth de-facto slice (1047 changed lines) appended to the S5 branch rather than its own reviewable unit.** It is 2.6x the 400-line budget on top of an already-oversized S5, and it introduces a new production file, a new package-level function, a new CLI flag, and a behavioral change to all five gates. Reviewer load on `feat/rdd-wave3-s5-taxonomy-offer-bench` is now well past what a single PR review can absorb.
- **W13 — candidate-causal admission is wired at `finalize`, but the spec says `validate`.** Both scenarios read "WHEN `validate` runs". `apply-progress` documents the rationale (reviewer findings physically arrive at `review finalize`; `review validate` is a pure relation check with no findings input) and the behavior is correct, but the spec prose and the shipped call graph disagree and no amendment is recorded.
- **W14 — `correcting` is unreachable and unfinalizable through the product.** `runReviewFacadeFinalizeNewLineage` refuses a `correcting` lineage and directs the caller to `review validate`; but `review validate` never persists any transition, so nothing in the CLI can move a `correcting` lineage forward. This makes spec scenario "In-flight new lineage still finalizes after rollback" (whose GIVEN is literally a `correcting` lineage) unprovable at the product surface. Latent today because no CLI verb can produce a `correcting` lineage either.

**SUGGESTION**

- S1 — add a named regression asserting `governingAuthorityLiveEvidence`'s `PolicyHash` equals START's frozen value. *(carried forward)*
- S2 — the prior report's ask is satisfied: `review finalize` on a v3 lineage now succeeds. Reframed: a repeat `review finalize` on an already-terminal lineage returns an ordinary success envelope indistinguishable from first issuance; a typed `replay: true` marker would make exactly-once observable to callers.
- S3 — new-lineage black-box coverage still stops at gate evaluation; no bench journey drives `start → finalize → receipt → gates`, which is now possible.
- S4 — `bench/results.json` is tracked and overwritten in place, so any bench run dirties the worktree. Pre-existing. *(carried forward)*
- S5 — the 5-gate escalated-denial loop accepts any typed discovery denial via an early `return`; asserting the specific expected denial shape would tighten it.

### Verdict

**FAIL** — 2 CRITICAL, 14 WARNING, 5 SUGGESTION.

All four prior blockers are genuinely and independently closed: `review finalize` now routes by lineage kind and issues a real on-disk receipt for both terminal routes, candidate-causal admission is implemented and persists only causal IDs in a real run, the 24-cell taxonomy matrix now dies under mutation, and the five-persisted-states requirement has both runtime and static evidence. Every command is green — root suite (63 packages), bench module, both ratchets, formatter, vet — and switch-off byte-equivalence still holds at all five gates. Archive is nonetheless blocked by two findings this re-verification surfaced: a new-lineage authority that was never approved and has no receipt authorizes delivery at all five gates including `release`, which the legacy path denies; and the remediation moved new-lineage state advancement out of `ReviewCore` into `internal/cli`, contradicting the wave's own "Sole Transition Owner" MUST. Both are narrow, well-localized, and each admits either a small code fix or an explicit recorded spec amendment.

---

## Superseded — original FAIL verification report (tip `ebe26c9c`)

```text
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:fa137f801501031b152347dc6442cc991cc0c62c647e5926cfe4cf3122af8a35
verdict: fail
blockers: 4
critical_findings: 4
requirements: 10/19
scenarios: 21/32
test_command: go test ./... -count=1
test_exit_code: 0
test_output_hash: sha256:486cce8498d4cc5261f9e7fc4fc66d7a78eff05b6d50f4ba67bd2fae92e3525e
build_command: go build ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: rdd-root-simplification-wave3
**Version**: N/A (unarchived change)
**Mode**: Strict TDD
**Scope**: whole Wave 3 chain, worktree `/home/gentleman/work/gentle-ai-worktrees/rdd-wave0`, tip `ebe26c9c`

### Chain Integrity

Linear, one commit per slice, each parented on its predecessor. Verified with `git log --graph cb5ade42..ebe26c9c`.

| Slice | Branch | Commit | Changed lines | Forecast | Budget |
|---|---|---|---|---|---|
| W2 tip | — | `cb5ade42` | — | — | — |
| PR0 | `docs/rdd-wave3-sdd-artifacts` | `2b2453db` | 607 | docs | ok (docs only) |
| S1 | `feat/rdd-wave3-s1-promotion-rename` | `c14f5933` | 238 | ~350 | ok |
| S2 | `feat/rdd-wave3-s2-authority-store` | `2125b3bd` | 877 | ~800 | ok (<1000) |
| S3 | `feat/rdd-wave3-s3-review-core` | `48f3ac05` | 753 | ~900 | ok |
| S4 | `feat/rdd-wave3-s4-governing-authority` | `098109b5` | 897 (777 authored + 120 generated goldens) | ~700 | ok (<1000) |
| S5 | `feat/rdd-wave3-s5-taxonomy-offer-bench` | `ebe26c9c` | 745 | ~600 | ok |
| Total | — | — | 3900 (44 files) | ~3350 | — |

No slice breaches the design's 1000-authored-line hard budget. S2/S4/S5 each exceed the shared 400-line reviewer budget by >2x — declared up front in the tasks forecast under `auto-chain` / High risk, so accepted by plan, not a new deviation (see WARNING W9).

### Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 40 |
| Tasks complete | 39 |
| Tasks incomplete | 1 (1.2, Wave 2 archive move — Wave-2-timing dependent, deferred by design) |

### Build & Tests Execution

**Build**: PASSED — `go build ./...`, exit 0, empty output (`sha256:e3b0c442…`). `gofmt -l .` clean, `go vet ./...` clean.

**Tests**: PASSED — `go test ./... -count=1`, exit 0, 63 packages ok, 0 FAIL, output `sha256:486cce84…`. Slowest: `internal/cli` 162.8s, `internal/reviewtransaction` 123.9s.

**Bench module** (separate Go module `github.com/gentleman-programming/gentle-ai/bench`, own `go.mod`; NOT covered by root `go test ./...`): `go build ./...` clean, `gofmt -l .` clean, `go vet ./...` clean, `go test ./... -count=1` ok (`sha256:728e517c…`).

**Ratchets**: `scripts/deadcode-ratchet.sh` → "no new unreachable functions". `TestEveryProductionRefusalNamesResolutionOrDeclaresByDesign` → ok.

**Bench journeys, re-executed independently against a freshly built `cmd/gentle-ai` binary at the tip**: `j59-new-lineage-exact-candidate-allows-post-apply-and-pre-commit` and `j60-new-lineage-unstaged-drift-denies-post-apply-allows-pre-commit` — both `completed`, 2/2, 0 failed.

**Coverage**: skipped — no coverage tool configured for this repository.

### Independent Re-Execution (not trusting the apply report)

1. **Switch-off byte-equivalence, proven against the real pre-change binary.** Built `cmd/gentle-ai` at both `cb5ade42` (W2 tip) and `ebe26c9c` (S5 tip), ran `review validate` over identical fixture repositories at all five gates, with isolated `HOME`. Output byte-identical at `post-apply`, `pre-commit`, `pre-push`, `pre-pr`, `release`. Repeated with an explicit `--lineage` marker naming a lineage with no v3 record: byte-identical at all five gates after normalizing only the fixture path. This is stronger evidence than the shipped goldens, which are self-captured post-change (see W4).
2. **Discovery-integrity denial (task 5.2), branch-strip mutation.** Collapsed `DiscoverNewLineage`'s marker check (`if os.IsNotExist(loadErr) && !marker` → `if os.IsNotExist(loadErr)`): `TestDiscoverNewLineageMarkerPresentRecordRemovedDenies` FAILED. Separately disabled the CLI corruption branch (`if errors.As(err, &corrupted)` → `if false && …`): `TestReviewValidateDiscoveryIntegrityMarkerCorruptedDeniesNeverLegacy` FAILED, and the failure output showed the candidate falling through to a legacy gate context — exactly the "never falls through to legacy" property under test. RED claim genuine. Both mutations reverted; worktree clean at `ebe26c9c`.
3. **Pre-commit projection fix (task 6.7), branch-strip mutation.** Disabled the `GatePreCommit → ProjectionStaged` branch: `TestGoverningAuthorityLiveEvidenceUses*` FAILED. RED claim genuine.
4. **Reason-taxonomy matrix, mutation.** Flipped one arm of `legacyReasonTaxonomyGateResult` (`GateScopeChanged` → `GateAllow`). The 24-cell `…ClosedMatrix` test still PASSED; only the companion `…CoversLegacyRefusals` FAILED. See CRITICAL C3.
5. **Promoted files are a pure rename.** Filtered every non-comment changed line in `shadow_relation.go`→`candidate_relation.go` and `shadow_identity.go`→`candidate_identity.go` across `cb5ade42..ebe26c9c`: only the type/const/function renames plus `type ShadowRelation = CandidateRelation`. Zero logic delta in `candidate_identity.go`.
6. **New-lineage lifecycle driven through the shipped binary.** `GENTLE_AI_RDD_NEW_LINEAGE=1 gentle-ai review start --lineage vthree-check` → exit 0, `state: reviewing`, and `v3/vthree-check/` containing exactly `review-state.json` (`v3/LOCK` sits at the version root, not inside the lineage, so the two-artifact claim holds by directory inspection). `gentle-ai review finalize --lineage vthree-check` → see CRITICAL C1.

### Spec Compliance Matrix

| Requirement | Scenario | Test | Result |
|---|---|---|---|
| authority-store / Two-Artifact Model | New lineage writes exactly two artifacts | `authority_store_test.go > TestAuthorityStoreNewLineageWritesExactlyTwoArtifacts`; independent CLI directory inspection | COMPLIANT |
| authority-store / Five Persisted States | Derived category is never a stored value | (none found) | UNTESTED |
| authority-store / Five Persisted States | No gate writes a derived category | (none found) | UNTESTED |
| authority-store / CAS Mutation | Stale revision refuses the write | `TestAuthorityStoreMutateRefusesStaleExpectedRevision` | COMPLIANT |
| authority-store / CAS Mutation | Replay identity is self-contained | `TestAuthorityStoreResolveReplayReturnsStoredTransitionWithoutMutation` | COMPLIANT |
| authority-store / Receipt Immutability | Post-issuance write attempt is rejected | `TestAuthorityStoreReceiptImmutableAfterIssuance` | COMPLIANT |
| authority-store / Reason Taxonomy Regression | Legacy refusal reason has a new-model equivalent | `review_reason_taxonomy_test.go > TestNewLineageReasonTaxonomyCoversLegacyRefusals` (mutation-verified) | PARTIAL |
| candidate-identity / Read-Only Resolution + Persisted Freeze | Resolution has no side effect | `candidate_readonly_guard_test.go > TestCandidateReadOnlyGuardHoldsForPromotedFiles` | COMPLIANT |
| candidate-identity / Read-Only Resolution + Persisted Freeze | New-lineage start persists resolved identity as frozen authority | `TestReviewStartNewLineageSwitchOnFreezesV3Authority` | COMPLIANT |
| relation-algebra / Read-Only Legacy, Deciding New | Shadow evaluation changes nothing observable | Independent base-vs-tip binary diff at 5 gates + Wave 1 shadow suite | COMPLIANT |
| relation-algebra / Read-Only Legacy, Deciding New | New-lineage call site consumes the same function | Rename verified logic-identical; `TestReviewCoreValidate*` drive `relateCandidates` via `DeriveObservation` | COMPLIANT |
| activation / Distinct Env Switch | Default configuration takes the legacy path | `TestReviewStartNewLineageSwitchOffCreatesNoV3Entries` | COMPLIANT |
| activation / Distinct Env Switch | Switch identity never overloads another switch | (none found) | UNTESTED |
| activation / Kill-Switch-Off Structurally Unfailable | Kill switch off produces no side effect | `review_offer_test.go > TestOfferReviewAfterVerifyDisabledKillSwitchReturnsUnavailableBeforeRepoRead` | PARTIAL |
| activation / Coexistence Precedence (Amendment C) | Legacy authority alone denies a new-lineage candidate | `TestResolveGoverningAuthorityFullMatrix` (8 cells) + `TestReviewValidateDiscoveryIntegrityMarkerCorruptedDeniesNeverLegacy` (mutation-verified) | COMPLIANT |
| activation / Coexistence Precedence (Amendment C) | New-lineage receipt authorizes a new-lineage candidate | `TestResolveGoverningAuthorityFullMatrix` "related ⇒ new governs" | PARTIAL |
| activation / Additive Gate Branch | Switch-off byte-equivalence at every gate | 5 golden tests + independent base-vs-tip binary diff, with and without `--lineage` | COMPLIANT |
| activation / Additive Gate Branch | New branch never touches legacy code path | `TestResolveGoverningAuthorityAbsentWithoutMarkerCostsNoGitCall` + governs-true short circuit in `runReviewFacadeValidate` | COMPLIANT |
| activation / Rollback Disables New Starts Only | In-flight new lineage still finalizes after rollback | `TestNewLineageRollbackSafetyStaysReadableAndFinalizableWhileSwitchIsOff` (Go API only) — contradicted at the product surface | FAILING |
| activation / Rollback Disables New Starts Only | Rollback blocks only new starts | `TestNewLineageRollbackSafetyStaysReadableAndFinalizableWhileSwitchIsOff` | COMPLIANT |
| review-core / Sole Transition Owner | Only ReviewCore transitions new-lineage state | Structural only; no guard test | PARTIAL |
| review-core / Consent-Gated Freeze | Tier 1 candidate freezes only after consent | `TestReviewCoreStartFreezesOnlyAfterConsentGranted` (6 cases: tier 0/1/4 × consent) + `TestReviewCoreConsentBlockedStartPersistsNothing` | COMPLIANT |
| review-core / Consent-Gated Freeze | Frozen tier is never recomputed | (none found) | UNTESTED |
| review-core / Candidate-Causal Admission | Candidate-caused finding blocks | (none found; unimplemented) | UNTESTED |
| review-core / Candidate-Causal Admission | Pre-existing finding becomes a follow-up | (none found; unimplemented) | UNTESTED |
| review-core / One Bounded Correction | Second correction attempt refuses | `TestAuthorityStoreResolveReplayRefusesSecondCorrectionAttempt` | COMPLIANT |
| review-core / One Bounded Correction | Exact replay costs nothing | `TestAuthorityStoreResolveReplayReturnsStoredTransitionWithoutMutation` (asserts state bytes unchanged) | COMPLIANT |
| review-core / Terminal Receipt Exactly Once | Finalize issues one receipt | `TestReviewCoreFinalizeApprovedIssuesReceiptRef` + `TestAuthorityStoreReceiptImmutableAfterIssuance` | COMPLIANT |
| shadow-evaluation / Disable Switch Is Observer Boundary | Disabling removes shadow observer execution only | Wave 1 `shadow_observer_test.go`; `shadowObservationEnabled()` confirmed to gate only `ObserveShadowRelation` and the pre-existing `gate.go:361` site | COMPLIANT |
| shadow-evaluation / Disable Switch Is Observer Boundary | Resolver and relation stay live-callable independent of the shadow switch | `TestReviewCoreValidate*` execute with `GENTLE_AI_RDD_SHADOW` unset | COMPLIANT |
| shadow-evaluation / Off by Default in Live Paths | Default configuration produces no live Git cost from the observer | Wave 1 suite | COMPLIANT |
| shadow-evaluation / Off by Default in Live Paths | New-lineage live cost is not observer cost | `governingAuthorityLiveEvidence` builds its own snapshot outside the observer | COMPLIANT |

**Compliance summary**: 21/32 scenarios COMPLIANT, 4 PARTIAL, 6 UNTESTED, 1 FAILING. 10/19 requirements fully compliant.

### Correctness (Static Evidence)

| Requirement | Status | Notes |
|---|---|---|
| Two-artifact v3 store, store-scoped lock | Implemented | `authority_store.go`; `v3/LOCK` at version root, lineage dir holds only the two artifacts |
| CAS on content-addressed revision | Implemented | `Mutate` refuses on `NewLineageRevisionConflictError`; revision recomputed from content on every parse, never trusted from disk |
| Five persisted states | Implemented | `NewLineageState.valid()` closed switch; `DerivedObservation` is a separate type outside `NewLineageAuthority` |
| Consent before authority freeze | Implemented | `authorizeReviewStart` precedes the activation branch in `runReviewFacadeStart`; `ReviewCore.start` refuses tier 1/4 without `ConsentGranted`; tier 0 (`RiskLow`) carve-out; refusal returns before any store call |
| Amendment C matrix + discovery-integrity denial | Implemented | `ResolveGoverningAuthority` pure matrix; `NewLineageMarkerCorruptedError` handled before the matrix and never falls through |
| Candidate-causal admission | NOT implemented | `AdmittedFindingIDs` declared and validated but never written; `ReviewCore.validate` performs no admission |
| Finalize/correction state machine reachable in production | NOT implemented | No production path ever writes `correcting`, `validating`, `approved`, or `escalated`; only `reviewing` at start |

### Coherence (Design)

| Decision | Followed? | Notes |
|---|---|---|
| 1 — single `ReviewCore.Next` entry, six-value transition vocabulary | Yes | `review_core.go` / `review_core_transition.go` |
| 2 — promote by rename, alias, guard split | Yes | Rename proven logic-identical; both `candidate_readonly_guard_test.go` and `shadow_readonly_guard_test.go` alive and passing (guard continuity intact) |
| 3 — v3 root, store-scoped lock, CAS, in-record replay identity | Yes | Verified by directory inspection through the real binary |
| 4 — one `resolveGoverningAuthority` shared by all five gates | Yes | Single call site in `runReviewFacadeValidate` before `discoverCompactFacadeGateReview` |
| 5 — start-only activation switch, finalize/validate route on discovered kind | Partially | Start-only read confirmed (`NewLineageActivationEnabled` has exactly one caller). Validate routes on discovered kind. **Finalize does not route on kind at all** — see C1 |
| 6 — reuse consent/tier/lenses/budget, no re-derivation | Yes | All four values arrive pre-computed from `runReviewFacadeStart`'s shared preamble |
| 7 — reason taxonomy mapping | Deviates | `ambiguous`/`unknown`/`unrelated` → `escalate`, not the prose's `stop` — see W2 |
| 8 — ship `OfferReviewAfterVerify` unwired with baselined entry | Yes | 2 baseline entries, ratchet clean, kill-switch-off-before-repo-read regression passes |
| Testing Strategy — AST guard that no `DeriveObservation` result is persisted | Not built | See C4 |
| Testing Strategy — bench tier 0/1/4 candidate→consent→review→correction→receipt→five gates | Deviates | See C1 and adjudication (c) |
| Zero live-lifecycle change | Yes | Only production non-test, non-doc modifications across `cb5ade42..ebe26c9c` are the two additive `review_facade.go` branches (start `:1618`, validate `:2893`), the comment-and-rename-only `shadow_observer.go` edit, and the two promoted files. Byte-equivalence independently proven at all five gates |

### TDD Compliance

| Check | Result | Details |
|---|---|---|
| TDD Evidence reported | Yes | Present in apply-progress (S5 batch, id 10151) |
| All tasks have tests | Partially | Decision/doc/ratchet tasks (6.3, 6.4, 6.6, 6.8) correctly N/A; every code task has a test file |
| RED confirmed (tests exist) | Yes | All named files exist |
| GREEN confirmed (tests pass) | Yes | Every named test re-executed with `-count=1` and passed |
| RED genuineness re-proven by mutation | Yes, 3/3 | Discovery-integrity (2 mutants) and pre-commit projection (1 mutant) all killed their tests |
| Triangulation adequate | Yes | Consent ordering 6 cases; Amendment C 8 cells; taxonomy 6 kinds; gate selector staged + workspace pair |
| Safety Net for modified files | Yes | The S5 policy-hash fix's ripple into `review_new_lineage_rollback_safety_test.go` was caught by running existing tests, exactly the safety-net step |

**TDD Compliance**: 6/7 checks passed.

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|---|---|---|---|
| Unit (pure Go, no repo) | 12 | 4 | `go test` |
| Integration (`t.TempDir()` real Git repos, in-process CLI entry points) | 19 | 7 | `go test` |
| Black-box (built binary driven by the bench harness) | 2 journeys | 1 | `bench` module |
| **Total** | **33** | **12** | |

### Assertion Quality

| File | Line | Assertion | Issue | Severity |
|---|---|---|---|---|
| `internal/cli/review_reason_taxonomy_test.go` | 44-67 | `got := legacyReasonTaxonomyGateResult(kind) == result` compared against `reachable := result == want` where `want = legacyReasonTaxonomyGateResult(kind)` | Tautology — both sides are the same pure call with the same argument; all 24 cells always agree. Mutation-proven: flipping one arm of the snapshot leaves it green | CRITICAL |
| `internal/cli/review_reason_taxonomy_test.go` | 20-37 | `legacyReasonTaxonomyGateResult` hand-written snapshot | Approval snapshot of `review_facade.go:2905-2910`/`:3008-3025` with no anchor binding it to that code | WARNING |

**Assertion quality**: 1 CRITICAL, 1 WARNING. No tautologies, ghost loops, or production-call-free assertions found in the other 10 new test files; assertion density is high (e.g. `authority_store_test.go` 51 assertions / 357 lines).

### Quality Metrics

**Formatter**: `gofmt -l .` clean (root and `bench/`).
**Vet**: `go vet ./...` clean (root and `bench/`).
**Deadcode ratchet**: clean ("no new unreachable functions"), 236 baselined entries.
**Refusal ratchet**: `TestEveryProductionRefusalNamesResolutionOrDeclaresByDesign` passes.

### Adjudication of Flagged Deviations

**(a) Latent policy-hash fix in `governingAuthorityLiveEvidence` — ACCEPTED.**
The fix replaces `FreezeCandidateIdentity(ctx, repo, snapshot, "")` (which defaulted to the sentinel `"unknown"`) with `facadePayloadHash(facadePolicyBytes(gateInput.PolicyArtifact))`. Verified byte-for-byte identical to the computation `runReviewFacadeStartNewLineage` performs at freeze time (`facadePolicyBytes(policySource)` then `facadePayloadHash(policy)`), so an unchanged candidate now compares `exact` instead of `changed`. Correct. Coverage is adequate at two layers: the staged/workspace selector pair at the Go layer (mutation-killed) and `j59` through the built binary. Blast radius is zero for legacy — the fix lives inside the `governs==true` branch, unreachable without a v3 record, and my base-vs-tip binary diff confirms byte-equivalence at all five gates. Being beyond S5's literal task wording is not a defect here: it was a necessary precondition for task 6.7's own RED test to go green, and it was disclosed rather than folded in silently. One follow-up only, as SUGGESTION S1.

**(b) `ambiguous`/`unknown`/`unrelated` → `escalate` vs. design prose `stop` — ACCEPTED AS A DEVIATION, "strengthening" framing REJECTED.**
The shipped behavior is not permissive: `TestNewLineageGateEvaluationClosedSwitchNeverAllowsUnreachableKinds` and the 6-kind companion prove no path reaches `GateAllow`, and `GateEscalated` is a denial. To that extent the apply report is right that nothing is silently loosened. But the unqualified "strengthening" claim understates a real consequence. In this model `escalate` is not a refusal-without-mutation; it names the persisted terminal state `escalated`, and `ReviewCore.finalize` accepts exactly `approved` **or** `escalated` and issues a receipt for either. The design's `stop` meant "refused with no state mutation". Today nothing persists a validate outcome, so the divergence is latent and harmless. It stops being latent the moment Wave 4 wires a consumer that persists what validate returned: an `ambiguous` relation would then be receipt-eligible. This must be recorded as a design amendment with that implication stated, not filed as an unconditional improvement. Recorded as WARNING W2, not a blocker for this wave.

**(c) Bench journeys narrower than "tier 0/1/4 full flow" because `review finalize` is not CLI-wired — the exit-evidence bar DOES require it in this wave. CRITICAL.**
Ruling, against the design's own wording:
- The Wave 3 row's exit-evidence cell reads "End-to-end candidate, consent, 0/1/4 review, correction, receipt, and gate journeys", and the table's preamble states "A wave must prove its exit evidence before the next wave starts." The bar names `receipt` explicitly and is unqualified as to surface.
- Wave 4's scope is "Thin consumers — Move adapters and SDD to opaque transitions and `ReceiptRef`". That is the *consumption* side. Producing a receipt through the facade is Wave 3's own "New `start/finalize/validate` authority flow with two artifacts". Routing `review finalize` by lineage kind is therefore inside Wave 3's boundary, not deferred to Wave 4 by the wave table.
- Wave 3 already CLI-wired `start` and `validate` on that same surface. Leaving the third verb of its own named triple unrouted is an internal inconsistency in the delivered slice, not a clean scope line.
- This change's own `design.md` Testing Strategy names, for the Bench layer, "tier 0/1/4: candidate→consent→review→correction→receipt→five gates | Black-box journeys". Black-box means through the binary. That is an in-wave commitment, made by this change, not an inherited ambiguity.
- Decisively, the gap is not "fewer journeys". I drove it: `review start` with the switch on succeeds and hands the user lineage `vthree-check`; `review finalize --lineage vthree-check` then reads `…/v2/vthree-check/review-state.json`, fails with a raw filesystem error, and writes an `operation-outcome-unknown` defect report. The product treats its own supported lifecycle as an unanticipated incident. The `ReviewCore`-level finalize+receipt path being green in-package does not substitute, because the defect is precisely in the routing that in-package tests bypass.

Verdict: not sufficient. This is CRITICAL C1 and blocks archive.

### Issues Found

**CRITICAL**

- **C1 — `review finalize` is not routed by lineage kind; a v3 lineage created by the shipped `review start` can never be finalized through the product.** Reproduced end-to-end with the tip binary: `GENTLE_AI_RDD_NEW_LINEAGE=1 gentle-ai review start --lineage vthree-check` exits 0 and writes `v3/vthree-check/review-state.json`; `gentle-ai review finalize --lineage vthree-check --captured-results=true` then exits 1 with `load compact facade review lineage: open …/review-transactions/v2/vthree-check/review-state.json: no such file or directory` and emits `.git/gentle-ai/defect-reports/operation-outcome-unknown-*.md`. Breaks spec `rdd-new-lineage-activation` → "In-flight new lineage still finalizes after rollback" at the product surface, and inverts design decision 5's own stated rationale (finalize was left ungated specifically so an in-flight lineage is never stranded — it is stranded anyway, because the routing was never added). Fix: add the lineage-kind branch to `runReviewFacadeFinalize` mirroring the `start` (`review_facade.go:1618`) and `validate` (`:2893`) branches, plus a black-box journey reaching receipt issuance.
- **C2 — spec `rdd-review-core-transitions` → "Candidate-Causal Admission Only" is unimplemented and untested (2 scenarios).** `ReviewCore.validate` performs relation classification only; no finding admission exists. `NewLineageAuthority.AdmittedFindingIDs` is declared and structurally validated (`authority_store.go:96,135`) but is never written by any production path — the only non-test references are its own declaration and validator. `tasks.md` Phase 4 (4.1–4.7) never decomposed this requirement, so the gap originates in planning and was faithfully carried through apply. Requires either implementation + tests, or an explicit spec amendment moving the requirement to a later wave.
- **C3 — `TestNewLineageReasonTaxonomyCoversLegacyRefusalsClosedMatrix` is a 24-cell tautology.** It computes `want := legacyReasonTaxonomyGateResult(kind)` and then asserts `legacyReasonTaxonomyGateResult(kind) == result` equals `want == result` — the same pure call on both sides, so no cell can ever disagree. Proven by mutation: changing `ReviewReceiptScopeChanged` to return `GateAllow` left this test green across all 24 cells while the companion `TestNewLineageReasonTaxonomyCoversLegacyRefusals` correctly failed. Task 6.1's headline artifact ("the literal 6×4=24-cell closed matrix") proves nothing. Mitigating: the substantive spec requirement is genuinely covered by the companion test against real production functions, so this is a test-quality blocker, not a coverage hole.
- **C4 — spec `rdd-authority-store` → "Five Persisted States, Everything Else Derived" has no covering test (2 scenarios), and the design's planned guard was never built.** The design's Testing Strategy Unit row promised "AST guard proves no `DeriveObservation` result is persisted"; no such guard exists. No test asserts that `invalidated`/`scope_changed`/`ambiguous`/`repairable`/`corrupted` are rejected as persisted state, nor that no gate writes one. The structural argument is strong (`NewLineageState.valid()` is a closed switch; `DerivedObservation` lives outside `NewLineageAuthority`), but a structural argument is not runtime scenario evidence, and both scenarios are unattested.

**WARNING**

- W1 — task 1.2 (Wave 2 archive move) unchecked. Cleanup/sequencing, Wave-2-timing dependent, outside Wave 3's critical path.
- W2 — design decision 7 deviation: `ambiguous`/`unknown`/`unrelated` map to `escalate` rather than `stop`, and `escalated` is a finalize-eligible terminal state. See adjudication (b). Needs a recorded amendment before Wave 4 persists validate outcomes.
- W3 — `legacyReasonTaxonomyGateResult` is an unanchored approval snapshot of `review_facade.go:2905-2910`/`:3008-3025`; the "legacy" half of the regression can drift silently if that handling changes.
- W4 — the five switch-off goldens are self-captured post-change, not the pre-change baseline task 5.6's wording specified ("diff against pre-change golden"). They can only detect future drift, not prove equivalence to Wave 2. Mitigated: I proved equivalence independently against the `cb5ade42` binary at all five gates, with and without a `--lineage` marker.
- W5 — spec scenario "Switch identity never overloads another switch" has no covering test; separation is structural only (three distinct env vars, distinct read sites).
- W6 — spec scenario "Frozen tier is never recomputed" has no covering test.
- W7 — "Kill switch off produces no side effect" is covered only for the unwired `OfferReviewAfterVerify`, not for the facade at its five observed gate call sites.
- W8 — S5 added the "Wave 3 exit-evidence pointer" paragraph to `docs/architecture/rdd-root-simplification-design.md` in the same slice being measured against that bar, reinterpreting it as satisfiable piecewise. The bar's own text (wave-table row, line 447) is unchanged, so the reinterpretation does not bind. Self-certification of an exit bar should be a separate, reviewed decision.
- W9 — S2 (877), S4 (897) and S5 (745) each exceed the 400-line reviewer budget by more than 2x. Declared in the tasks forecast under `auto-chain` / High risk, so accepted by plan, but reviewer load on those three PRs is real.
- W10 — `governingAuthorityLiveEvidence` still uses the workspace projection at `pre-push`, `pre-pr`, and `release`; only `pre-commit` was corrected to the staged projection. Documented in the function's own comment with a defensible reason (no v3 analogue for recovery-chain rebinding), but those three gates compare against a target the gate itself does not gate on.

**SUGGESTION**

- S1 — add a named regression asserting `governingAuthorityLiveEvidence`'s `PolicyHash` equals START's frozen value; today the fix is proven only transitively through the exact-match assertion.
- S2 — `review finalize` on a v3 lineage should produce a typed refusal naming the resolution, not a raw v2 path error plus an `operation-outcome-unknown` defect report.
- S3 — new-lineage black-box coverage stops at `post-apply`/`pre-commit`; `pre-push`, `pre-pr`, and `release` have no journey.
- S4 — `bench/results.json` is a tracked file the bench runner overwrites in place, so any bench run dirties the worktree. Pre-existing, outside this wave's scope.

### Verdict

**FAIL** — 4 CRITICAL, 10 WARNING, 4 SUGGESTION. All executed commands are green (root suite, bench module, both ratchets, formatter, vet) and the wave's most load-bearing safety property — switch-off byte-equivalence at all five gates — is independently proven against the real pre-change binary; but `review finalize` never routes on lineage kind, so a lineage the shipped `review start` creates cannot reach a receipt through the product, the spec's candidate-causal admission requirement is unimplemented, the headline 24-cell taxonomy matrix is a mutation-proven tautology, and the five-persisted-states requirement has no runtime evidence.
