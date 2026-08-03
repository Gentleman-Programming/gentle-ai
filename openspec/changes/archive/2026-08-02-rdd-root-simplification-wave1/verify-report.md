```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:4ef296a17accd02813430fcd56277c49a17f73d43af6ac2d5902cd9896ec7b79
verdict: pass
blockers: 0
critical_findings: 0
requirements: 22/22
scenarios: 28/28
test_command: go test ./... -count=1
test_exit_code: 0
test_output_hash: sha256:7c08f62bc3dfcd99490c924e7098838555629354b8b3294d0711622677309d2c
build_command: go build ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report — Final verification (third cycle)

**Change**: rdd-root-simplification-wave1
**Version**: Wave 1 (Shadow Algebra), 4 capability specs — 22 requirements / 28 scenarios
**Mode**: Strict TDD
**Candidate**: worktree `/home/gentleman/work/gentle-ai-worktrees/rdd-wave0`, branch `feat/rdd-wave1-shadow-observer-wiring`, chain tip `3480bcd0`, working tree clean (`git status --short` empty, `git diff HEAD` empty)
**Prior report**: FAIL at tip `7fbfece3` — 1 CRITICAL (CRITICAL-3), 7 WARNING, 4 SUGGESTION (preserved verbatim below)
**Verified by**: independent re-execution. Every remediation claim in commit `3480bcd0` and in `apply-progress` was re-derived from source and runtime, including an independent RED reproduction; nothing was trusted.
**Attempt authority (echoed, not settled)**: `sha256:afd75416724368c9cf8ed95c1be7ac4e36de2cd082f838107e7d2bb3280dc2ff`

### What changed since the prior report

One commit, `3480bcd0` (`test(review): cover pi-overlay selector out-of-scope refusal`), 28 insertions / 6 deletions across 2 files: `internal/reviewtransaction/shadow_identity_test.go` (new negative test + tautology deletion) and `openspec/changes/rdd-root-simplification-wave1/tasks.md` (5.3 / 5.5 / 5.7 prose corrections). No production source file was touched, so CRITICAL-1 and CRITICAL-2 closure evidence carries forward unchanged and was re-confirmed at this tip.

### CRITICAL-3 — **CLOSED**

`TestShadowIdentityPiOverlaySelectorIsOutOfWave1Scope` (`shadow_identity_test.go:320`) — **PASS**, re-executed at `0.00s`, not skipped (`requireSnapshotGit` satisfied, `--- PASS` observed in a `-v` run).

The test genuinely covers `specs/rdd-candidate-identity/spec.md:70-75`. Full path re-derived from source rather than from the commit message:

1. `shadowCandidateIdentity` (`shadow_identity.go:127`) resolves the repository lease first, so `RepositoryID` evidence exists before any refusal.
2. `shadowSelectorAmbiguityReason` (`:176`) switches only on `staged` and `committed-range`, so an unknown kind returns `""` and does **not** divert to the ambiguity path.
3. `shadowSelectorTarget` (`:215`) has no case for the injected kind, so control reaches the `default` at `:265-266`: `return Target{}, fmt.Errorf("unsupported shadow selector kind %q", selector.Kind)`.
4. `shadowCandidateIdentity:156-161` converts that into `CandidateIdentity{}, &shadowIdentityFailure{Reason: err.Error()}` — the zero value plus a typed failure.

The three assertions map onto the scenario's operative clauses, and assert refusal rather than fabrication:

| Spec clause | Assertion | Line |
|---|---|---|
| GIVEN a gentle-pi protocol-1.1 overlay selector | `Kind: shadowSelectorKind("gentle-pi-protocol-1.1-overlay")` — a value outside the closed 4-value enum | `:325` |
| THEN the resolver does not claim to resolve it | `errors.As(err, &failure)` — typed refusal, not a success | `:328` |
| …as a *supported Wave 1 selector* | `strings.Contains(failure.Reason, "unsupported shadow selector kind")` — binds to the exact refusal at `shadow_identity.go:266`, not to any generic failure | `:331` |
| AND coverage is not silently assumed working | `identity != (CandidateIdentity{})` fails — the resolver must not fabricate a tuple | `:334` |

**RED reproduced independently.** Rather than trusting the recorded evidence, the `shadow_identity.go:266` default-case refusal was temporarily replaced with a fabricated `Target{Kind: TargetBaseDiff, BaseRef: selector.BaseRef}, nil` resolution. The test failed with exactly the recorded message:

```text
--- FAIL: TestShadowIdentityPiOverlaySelectorIsOutOfWave1Scope (0.03s)
    shadow_identity_test.go:329: error = <nil> <nil>, want *shadowIdentityFailure for an out-of-scope pi-overlay selector
```

The file was then restored with `git checkout --`, re-verified byte-exact (`git diff HEAD` empty, tip still `3480bcd0`), and the test re-run GREEN. The test is non-vacuous: it detects precisely the fabrication the scenario forbids.

Note on selector naming: the spec names no wire-level identifier for a pi overlay selector, so the test uses a representative unrecognized kind string. The load-bearing property is *not being a supported Wave 1 enum value*, which is exactly what the scenario constrains.

### Assertion-quality correction (prior WARNING-5) — **CLOSED**

The tautology formerly at `shadow_identity_test.go:302-304` (`if (CandidateIdentity{}) != (CandidateIdentity{})`) is deleted. The enclosing `TestShadowIdentityUnresolvableSelectorFailsClosed` retains all of its genuine assertions, verified by reading `:289-312`: the `errors.As` typed-failure check on `not-a-real-revision` (`:297`), the `failure.RepositoryID == ""` evidence-gathering check (`:300`), and the second `errors.As` check for an empty committed-range selector (`:309`). Nothing load-bearing was removed alongside the tautology. A fresh scan of all `shadow_*_test.go` plus `gate_test.go` finds no remaining tautology, `if true`, or self-comparison pattern.

### Build & Tests Execution (re-executed at tip `3480bcd0`)

**Build**: PASS

```text
go build ./...                 exit 0, empty output (sha256:e3b0c442…b855)
go vet ./...                   exit 0
gofmt -l internal/ docs/       empty
scripts/deadcode-ratchet.sh    exit 0 — "no new unreachable functions"
git status --short             empty; git diff HEAD empty
```

**Tests**: PASS — full module, every package

```text
go test ./... -count=1         exit 0, zero FAIL lines
  internal/cli                 ok  166.181s
  internal/reviewtransaction   ok  124.901s
  internal/sddstatus           ok   24.949s
output sha256:7c08f62bc3dfcd99490c924e7098838555629354b8b3294d0711622677309d2c
```

Targeted verbose re-execution — 5/5 PASS, zero SKIP:

| Test | Purpose | Result |
|---|---|---|
| `TestShadowIdentityPiOverlaySelectorIsOutOfWave1Scope` | new — CRITICAL-3 closure | PASS (0.00s) |
| `TestShadowIdentityUnresolvableSelectorFailsClosed` | tautology-removal regression check | PASS (0.01s) |
| `TestNativePrePRGateWithShadowDisabledDerivesBaseAdvanceZeroTimes` | spot-check — zero shadow Git cost by default | PASS (0.26s) |
| `TestNativePrePRGateShadowOnOffByteIdenticalForCompatibleBaseAdvance` | spot-check — pre-PR ON/OFF byte identity | PASS (0.41s) |
| `TestShadowMatrixRealCorpusHasZeroUnexplainedDivergences` | spot-check — zero unexplained divergences | PASS (0.00s) |

**Ratchets**: both clean. Deadcode ratchet exit 0 without `--update`. Refusal ratchet (`TestEveryProductionRefusalNamesResolutionOrDeclaresByDesign`) green inside the passing `internal/cli` package, `GENTLE_AI_REFUSAL_RATCHET_UPDATE` never set. Note the new test exercises the `shadow_identity.go:266` site that carries a `// refusal:by-design world-action:` annotation — the annotation now has runtime coverage behind it, not only static declaration.

**Golden determinism**: `TestShadowMatrixCoveringArrayGolden` ran inside the full suite without `-update`; `git status --short` empty afterwards.

**Coverage**: not measured — no coverage tool configured for this module. Not a failure.

### CRITICAL-1 / CRITICAL-2 — re-confirmed closed at this tip

Re-read rather than assumed. `gate.go:360-363` still reads:

```go
var shadowAdvance *BaseAdvanceCompatibility
if shadowObservationEnabled() && request.Gate == GatePrePR && snapshot.BaseTree != receipt.BaseTree {
	shadowAdvance = shadowDeriveBaseAdvance(ctx, repo, receipt, request, snapshot, resolvedPrePR, preimages)
}
```

That is the live derivation's own precondition at `gate.go:296` plus `shadowObservationEnabled()`, so shadow derivation remains a strict subset of live and is unreachable with the switch off. A fresh production-file sweep still finds exactly one `shadowDeriveBaseAdvance` call site (guarded) and five `ObserveShadowRelation` call sites (`gate.go:364`, `compact_gate.go:525`, `compact_recovery_binding.go:311`, `review_facade.go:836`, `review_facade.go:1567`), each protected by the callee's own `!shadowObservationEnabled() → return`. Unchanged from the prior report.

### Spec Compliance Matrix — delta only

Every other row was re-run inside the green full suite and still holds.

| Requirement | Scenario | Test | Prior | Now |
|---|---|---|---|---|
| Wave 1 Selector Scope | Pi overlay selector is explicitly out of scope | `TestShadowIdentityPiOverlaySelectorIsOutOfWave1Scope` | ❌ UNTESTED | ✅ COMPLIANT |

**Compliance summary**: 28/28 scenarios compliant (was 27/28), 0 UNTESTED, 0 FAILING, 0 PARTIAL. 22/22 requirements fully compliant (was 21/22).

### Completeness

| Metric | Value |
|---|---|
| Tasks total | 50 |
| Tasks complete | 50 |
| Tasks incomplete | 0 |
| Spec requirements | 22 (4 + 5 + 6 + 7 across the four capability specs) |
| Spec scenarios | 28 (5 + 7 + 9 + 7) |

Task/code agreement re-checked: `tasks.md` 5.3, 5.5 and 5.7 now cite the tests that actually carry the claims, and 5.7's guard text matches `gate.go:361` verbatim. The prior mirror drift is gone — `/home/gentleman/work/gentle-ai/openspec/changes/rdd-root-simplification-wave1/tasks.md` and the worktree copy are byte-identical (`diff` clean), both 50 checked / 0 unchecked.

### Machine gate

`gentle-ai sdd-verify-validate --input <candidate> --requirements 22 --scenarios 28` (v2.2.4+pr2151) — **ADMITTED**, exit 0:

```json
{
  "valid": true,
  "verdict": "pass",
  "evidence_revision": "sha256:4ef296a17accd02813430fcd56277c49a17f73d43af6ac2d5902cd9896ec7b79"
}
```

The counts passed to the validator are the authoritative ones re-counted from the four retrieved spec files (4+5+6+7 requirements, 5+7+9+7 scenarios), not carried over from the prior report. The same tool refused a `pass` at the prior report's `27/28`; it admits one here only because every scenario now has a covering test that passed at runtime.

### Issues Found

**CRITICAL**: None. All three prior CRITICAL findings are closed with independently re-executed runtime evidence.

**WARNING**

1. **Delivery slice is over the authored-lines budget.** `feat/rdd-wave1-shadow-observer-wiring` is 6 commits ahead of PR4 (`c6eb165b`), 0 behind: 1599 additions / 52 deletions = 1651 changed lines, 334 of them golden → **1317 authored**, against design Decision 7's ≤1000 authored-lines cap (was 1289 at the prior tip). Delivery-time concern for the orchestrator; no spec MUST is violated.
2. **Delivered granularity is 6 PRs, not the planned 7.** PR5, PR6 and three post-verify fix commits share one branch. Each logical slice is individually within budget; the aggregate branch is not. Delivery-time concern for the orchestrator.
3. **PR0 remains 1080 changed lines**, above the cap; pre-forecast at ~1300 and accepted as doc/rename bulk. Unchanged.
4. **TDD Cycle Evidence for batches 1-7 remains unretrievable** from the current `apply-progress` revision ("see prior revisions"). Substituted, as in both prior passes, by per-task RED/GREEN prose in `tasks.md` plus full independent re-execution of every test. Batch 10's evidence table is present and was independently reproduced.
5. **`docs/architecture/rdd-shadow-evaluation.md:19` remains slightly over-broad.** It calls pre-PR/pre-push "the only family where shadow evaluation performs additional Git work", but after the CRITICAL-1 fix the guard is `GatePrePR` only, so pre-push performs no shadow derivation at all. Documentation-only imprecision, carried over unchanged; the fix commit did not touch docs.

**SUGGESTION**

1. `TestNativePrePRGateShadowOnOffByteIdenticalForCompatibleBaseAdvance` still does not assert that the ON run incremented `shadowDeriveBaseAdvanceCallCountForTest()` (`gate_test.go` uses the hook only in the `!= 0` OFF assertion at `:39`). Both tests would still pass if the guard were hard-wired to `false`. A one-line `want ≥ 1` assertion in the ON branch would make that machine-checked. Carried over.
2. Design Decision 1's "one exported function plus two exported data types **and nothing else**" is still literally unmet — 7 exported `ShadowRelation*` constants also exist. Amend the decision or unexport.
3. The exit-bar test still injects a `synthetic/exit-bar-mechanism-proof` row that no reachable input shape can produce. Sound, but worth noting.
4. `.deadcode-baseline.txt` carries the two `shadowDeriveBaseAdvance*ForTest` helpers alongside the three pre-existing `shadowObserver*ForTest` entries. Consistent test-only-helper pattern; neutral, recorded for audit.

### TDD Compliance (delta)

| Check | Result | Details |
|---|---|---|
| TDD Evidence reported (Batch 10) | Yes | Table present in `apply-progress`; the single row was independently reproduced |
| RED confirmed | Yes — independently | Mutating `shadow_identity.go:266` to fabricate a target reproduced the exact recorded failure; file restored byte-exact |
| GREEN confirmed | Yes | New test PASS; 5/5 targeted PASS; whole module exit 0 |
| Triangulation | Adequate | The new test adds a fourth distinct failure shape to the identity suite (unknown-kind refusal), alongside unresolvable-revision, empty-selector, and ambiguity cases |
| Safety net | Yes | Full `go test ./... -count=1` re-run on the tip after the fix |
| Assertion quality | ✅ Clean | Tautology removed; zero banned patterns remain across `shadow_*_test.go` and `gate_test.go` |

### Verdict

**PASS** — the third-cycle fix closes the last blocking gap with real runtime evidence. `TestShadowIdentityPiOverlaySelectorIsOutOfWave1Scope` genuinely exercises `shadowSelectorTarget`'s default-case refusal and asserts both the typed refusal and the absence of a fabricated identity; its RED was independently reproduced and the candidate restored byte-exact. The prior tautology is gone with all genuine assertions intact, both ratchets are clean, the full module suite is exit 0, both `tasks.md` mirrors agree at 50/50, and every one of the 28 scenarios across 22 requirements now has a covering test that passed at runtime. The five remaining WARNINGs are delivery-granularity and documentation-precision items for the orchestrator; none violates a spec MUST.

---

## Superseded — re-verification report (tip `7fbfece3`, FAIL)

Preserved verbatim as history. Only its opening code-fence language is changed from `yaml` to `text`, so this file carries exactly one authoritative `gentle-ai.verify-result/v1` envelope; no other byte is altered.

```text
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:c5b74c854b7a2ff4f3c25ef0007cb3442a629f96d31b69e54ac8adceb9728ebe
verdict: fail
blockers: 1
critical_findings: 1
requirements: 21/22
scenarios: 27/28
test_command: go test ./... -count=1
test_exit_code: 0
test_output_hash: sha256:057befd1e125bd6f48c596e9eb66575ba08280c06c7d47822aeadf66eb616b6a
build_command: go build ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report — Re-verification (corrective re-run)

**Change**: rdd-root-simplification-wave1
**Version**: Wave 1 (Shadow Algebra), 4 capability specs — 22 requirements / 28 scenarios
**Mode**: Strict TDD
**Candidate**: worktree `/home/gentleman/work/gentle-ai-worktrees/rdd-wave0`, branch `feat/rdd-wave1-shadow-observer-wiring`, chain tip `7fbfece3`, working tree clean
**Prior report**: FAIL at tip `933fb329` — 2 CRITICAL, 7 WARNING, 3 SUGGESTION (preserved verbatim below)
**Verified by**: independent re-execution. Remediation claims in commit `7fbfece3` and in `apply-progress` were re-derived from source and runtime, not trusted.

### What changed since the prior report

One commit, `7fbfece3` (`fix(review): run shadow base-advance derivation only when observation is enabled`), 105 insertions / 7 deletions across 6 files: `internal/reviewtransaction/gate.go`, `internal/reviewtransaction/shadow_relation.go`, `internal/reviewtransaction/gate_test.go`, `docs/architecture/rdd-shadow-evaluation.md`, `.deadcode-baseline.txt`, `openspec/changes/rdd-root-simplification-wave1/tasks.md`.

### Prior CRITICAL findings — disposition

#### CRITICAL-1 (unguarded shadow derivation on the live path) — **CLOSED**

`gate.go:359-363` now reads:

```go
var shadowAdvance *BaseAdvanceCompatibility
if shadowObservationEnabled() && request.Gate == GatePrePR && snapshot.BaseTree != receipt.BaseTree {
	shadowAdvance = shadowDeriveBaseAdvance(ctx, repo, receipt, request, snapshot, resolvedPrePR, preimages)
}
```

Guard shape verified against the live precondition it claims to mirror. The live derivation at `gate.go:296-299` is `if request.Gate == GatePrePR && snapshot.BaseTree != receipt.BaseTree`. The shadow guard is that exact predicate **plus** `shadowObservationEnabled()`, so shadow derivation is a strict subset of where live already derives, and is unreachable with the switch off. The guard is more conservative than the prior report demanded: pre-push now performs no shadow derivation even when the switch is ON.

Exhaustive call-site sweep of non-test production files (`rg` over `internal/**/*.go` excluding `_test.go` and excluding `shadow_*.go` internals) finds exactly one `shadowDeriveBaseAdvance` invocation and five `ObserveShadowRelation` invocations:

| Site | Guard | Shadow-specific work at the call site |
|---|---|---|
| `gate.go:362` `shadowDeriveBaseAdvance` | `shadowObservationEnabled() && GatePrePR && base changed` | the only shadow Git derivation; now switch-gated |
| `gate.go:364` `ObserveShadowRelation` | inside-callee `!shadowObservationEnabled() → return` (`shadow_observer.go:148`) | none — passes already-computed values |
| `compact_gate.go:525` | same inside-callee guard | none — reuses live-derived `compatibility` |
| `compact_recovery_binding.go:311` | same | none — reuses live-derived `compatibility` |
| `review_facade.go:836` (status) | same | none — literals + already-built `liveSnapshot` |
| `review_facade.go:1567` (start) | same | none — literals + already-built `snapshot` |

No unguarded shadow call site remains. With `GENTLE_AI_RDD_SHADOW` unset the only shadow code that executes is the `ObserveShadowRelation` prologue (one mutex-protected test-hook counter increment plus one `os.Getenv`) before its early return — no resolver, no relation, no classifier, and no Git process, which is exactly what the scenario's operative `THEN` requires.

#### CRITICAL-2 ("Off by Default in Live Paths" untested and violated) — **CLOSED**

`TestNativePrePRGateWithShadowDisabledDerivesBaseAdvanceZeroTimes` (`gate_test.go:30`) — **PASS**, re-executed.

Hook authenticity checked as instructed, without mutating the tree. The counter is incremented by the first three statements of `shadowDeriveBaseAdvance` itself (`shadow_relation.go:140-142`), before the `deriveBaseAdvanceCompatibility` delegation — not by a sibling, not by a wrapper, not by the observer. `shadowObserverCallCountForTest` (the hook the prior report rejected as non-covering) is a separate counter in `shadow_observer.go:144-146` and is untouched by this test.

RED evidence without reverting the guard, established from three independent facts:

1. Before `7fbfece3` the call was unconditional (`git show 7fbfece3 -- gate.go` shows the removed line `shadowAdvance := shadowDeriveBaseAdvance(...)` with no enclosing `if`).
2. The fixture *does* reach the guard: in the ON/OFF test on the same `newCompatiblePrePRFixture`, the observer at `gate.go:364` emitted `gentle-ai.rdd-shadow/v1 gate=pre-pr live_result=allow has_relation=true shadow_relation=compatible_base_advance ...` on stderr. That line is one statement after the guard and reports a `compatible_base_advance` relation, which is only derivable from a non-nil `shadowAdvance` — so the guarded body executes when enabled, and the same fixture reaches the guard when disabled.
3. The test additionally asserts `evaluation.Context.BaseAdvance != nil && .valid()`, proving the *live* seven-condition derivation (including `merge-tree --write-tree`) ran at `gate.go:296-299` in the same run. So the observed count of `0` isolates the shadow call specifically; it is not an artifact of the fixture failing to reach base-advance territory.

Removing either guard conjunct makes this test fail with count `1`. The regression guard is real.

### Build & Tests Execution

**Build**: PASS

```text
go build ./...                 exit 0, empty output (sha256:e3b0c442…b855)
go vet ./...                   exit 0
gofmt -l internal/ docs/       empty
scripts/deadcode-ratchet.sh    exit 0 — "no new unreachable functions"
git status --short             empty (clean tree after the full run)
```

**Tests**: PASS — full module, every package

```text
go test ./... -count=1         exit 0, zero FAIL lines
  internal/reviewtransaction   ok  123.305s
  internal/cli                 ok  (pass)
  internal/sddstatus           ok  24.780s
output sha256:057befd1e125bd6f48c596e9eb66575ba08280c06c7d47822aeadf66eb616b6a
```

Targeted re-execution (`go test ./internal/reviewtransaction/ -count=1 -v -run …`) — 6/6 PASS:

| Test | Purpose | Result |
|---|---|---|
| `TestNativePrePRGateWithShadowDisabledDerivesBaseAdvanceZeroTimes` | new — zero shadow Git cost by default | PASS (0.25s) |
| `TestNativePrePRGateShadowOnOffByteIdenticalForCompatibleBaseAdvance` | new — pre-PR ON/OFF byte identity | PASS (0.41s) |
| `TestShadowObservationSwitchIsRollbackBoundaryGateByteIdentical` | spot-check — post-apply ON/OFF byte identity | PASS (2 sub-tests) |
| `TestShadowReadOnlyGuardHoldsForProductionFiles` | spot-check — AST read-only guard | PASS (4 sub-tests, all 4 `shadow_*.go`) |
| `TestShadowMatrixRealCorpusHasZeroUnexplainedDivergences` | spot-check — zero-unexplained-divergence matrix | PASS |
| `TestShadowMatrixUnexplainedDivergenceOnCoreRelationBlocksWave2` | spot-check — exit-bar mechanism | PASS |

**Golden determinism**: `TestShadowMatrixCoveringArrayGolden` ran inside the full suite without `-update`; `git status --short` empty afterwards. Golden still deterministic and byte-stable.

**Ratchets**: deadcode PASS; refusal ratchet PASS (`TestEveryProductionRefusalNamesResolutionOrDeclaresByDesign` inside the green `internal/cli` package, `GENTLE_AI_REFUSAL_RATCHET_UPDATE` never set).

**Coverage**: not measured — no coverage tool configured for this module. Not a failure.

### Spec Compliance Matrix — delta only

Unchanged rows from the prior report were re-run in the full suite and still hold. Only the five previously non-compliant rows are re-adjudicated here.

| Requirement | Scenario | Test | Prior | Now |
|---|---|---|---|---|
| Advisory-Only, Never Blocking | Shadow failure does not block the live path | `TestShadowObserverAdvisoryFailureNeverBlocksLivePath` + `TestNativePrePRGateWithShadowDisabledDerivesBaseAdvanceZeroTimes` | PARTIAL (delay) | ✅ COMPLIANT |
| Disable Switch Is the Rollback Boundary | Disabling removes all shadow execution | `TestShadowObserverDisabledIsANoOp` + the zero-times guard | ❌ FAILING | ✅ COMPLIANT |
| Zero Live-Lifecycle Behavior Change | Live outcome identical with shadow on or off | `TestShadowObservationSwitchIsRollbackBoundaryGateByteIdentical` (post-apply) + `TestNativePrePRGateShadowOnOffByteIdenticalForCompatibleBaseAdvance` (pre-PR) | PARTIAL | ✅ COMPLIANT |
| Off by Default in Live Paths | Default configuration produces no live Git cost | `TestNativePrePRGateWithShadowDisabledDerivesBaseAdvanceZeroTimes` | ❌ FAILING / UNTESTED | ✅ COMPLIANT |
| Wave 1 Selector Scope | Pi overlay selector is explicitly out of scope | (none) | PARTIAL | ❌ **UNTESTED — CRITICAL-3** |

**Compliance summary**: 27/28 scenarios compliant (was 23/28), 1 UNTESTED. 21/22 requirements fully compliant (was 17/22).

### Issues Found

**CRITICAL**

1. **CRITICAL-3 — `rdd-candidate-identity` → "Wave 1 Selector Scope" has no covering test.** Scenario *"Pi overlay selector is explicitly out of scope"* (`specs/rdd-candidate-identity/spec.md:70-75`) requires that when the resolver is invoked with a gentle-pi protocol-1.1 overlay selector it *"does not claim to resolve it as a supported Wave 1 selector"*. Exhaustive search finds no test that constructs an unsupported `shadowSelectorKind` and asserts the `unsupported shadow selector kind %q` refusal at `shadow_identity.go:266`. `shadowMatrixNoShadowDecisionCase(shadowSelectorWorkspaceOverlay, …)` covers gentle-ai's own `workspace-overlay` selector, which is a *supported* Wave 1 selector — a different thing.

   This is not a new defect: the prior report listed the same gap, but classified it `PARTIAL` and demoted it to WARNING-7. Per `references/report-format.md`, `PARTIAL` means *"test passes but covers only part of the scenario"*; with no test at all the correct status is `UNTESTED`, and the skill's decision gate makes an uncovered scenario CRITICAL ("A spec scenario is compliant only when a covering test passed at runtime"). The classification is corrected here, so this report is internally consistent with its own envelope arithmetic — which is unchanged in method from the prior report (both count PARTIAL/UNTESTED as non-compliant: prior 28−5=23, now 28−1=27).

   Corroboration from the machine gate: `gentle-ai sdd-verify-validate` (v2.2.4+pr2151) refuses a `pass` verdict at `scenarios: 27/28` with *"passing verdict contradicts failing or incomplete evidence"*. A pass is not admissible while any scenario lacks runtime coverage, independent of how the prose labels it.

   Remediation is small and well-defined: one negative test in `shadow_identity_test.go` asserting `shadowCandidateIdentity(ctx, shadowSelector{Kind: "pi-overlay-1.1", Repo: repo})` returns the `unsupported shadow selector kind` error rather than a resolved `Target`. Structural closure (the four-value closed enum) is a real design strength but is not runtime evidence.

**WARNING**

1. **Delivery slice is now further over budget.** `feat/rdd-wave1-shadow-observer-wiring` is 5 commits ahead of PR4 (`c6eb165b`), 0 behind: 1574 additions / 49 deletions = 1623 changed lines, 334 of them golden → **1289 authored**, against design Decision 7's ≤1000 authored-lines cap (was 1183 before the fix). Delivered granularity is still 6 PRs, not the planned 7. Delivery-time concern for the orchestrator, not a spec violation.
2. **PR0 remains 1080 changed lines**, above the cap; pre-forecast at ~1300 and accepted as doc/rename bulk. Unchanged.
3. **`tasks.md` 5.3, 5.5 and 5.7 prose is now stale in the opposite direction.** The remediation corrected 0.1-0.3 (verified: both the tracked copy and the main-checkout mirror now read RESOLVED/DONE with the `623ce88b` citation), but left the three task entries the prior report criticised:
   - 5.3 still names `shadowObserverCallCountForTest()` as the proof of "Off by Default in Live Paths". That claim was CRITICAL-2's substance and is still wrong; the real proof is now `TestNativePrePRGateWithShadowDisabledDerivesBaseAdvanceZeroTimes`, which 5.3 does not cite.
   - 5.5 still generalises post-apply ON/OFF coverage to all five gate kinds without citing the new pre-PR test that actually closes the gap.
   - 5.7 still describes the `shadowDeriveBaseAdvance` call as unconditional ("additionally calls `shadowDeriveBaseAdvance` directly to exercise Amendment A's delegation seam from a live call site") with no mention of either guard conjunct.
   Artifact text contradicts the delivered code state. Documentation-only.
4. **The main-checkout `tasks.md` mirror is one task behind the worktree.** `/home/gentleman/work/gentle-ai/openspec/changes/rdd-root-simplification-wave1/tasks.md` has 49 checked tasks; the worktree copy has 50. The mirror carries the 0.1-0.3 fix but is missing "Injected Task 1" and the post-PR6 rewording of task 6.4. Zero unchecked tasks in either copy.
5. **Assertion-quality correction — one tautology exists and the prior report missed it.** `shadow_identity_test.go:302-304`:

   ```go
   if (CandidateIdentity{}) != (CandidateIdentity{}) {
   	t.Fatalf("sanity: zero value must compare equal to itself")
   }
   ```

   This is a literal tautology (`x != x` on a comparable struct); Go's language spec makes it unreachable. The prior report asserted "Zero tautologies" — that was inaccurate. Classified WARNING rather than CRITICAL because the strict-TDD module's CRITICAL criterion is *"test proves NOTHING"*, and the enclosing `TestShadowIdentityUnresolvableSelectorFailsClosed` carries two genuine `errors.As` assertions against real resolver failures; the tautology is inert dead weight, not the test's load-bearing claim. Recommend deleting the three lines. Flagged explicitly so the orchestrator can override this calibration if it prefers the literal reading.
6. **TDD Cycle Evidence for batches 1-7 remains unretrievable** from the current `apply-progress` revision ("unchanged, see prior revision for full table"). Substituted, as before, by per-task RED/GREEN prose in `tasks.md` plus full independent re-execution of every test.
7. **Docs sentence is slightly over-broad in the other direction.** `docs/architecture/rdd-shadow-evaluation.md:19` now cites the new test "for the pre-PR/pre-push gate kind, the only family where shadow evaluation performs additional Git work". After the fix, pre-push performs no shadow derivation at all (the guard is `GatePrePR` only), so naming pre-push there is imprecise. Quick-path step 1's "zero shadow code runs" is now materially accurate (no resolver/relation/classifier/Git work); the residual `ObserveShadowRelation` prologue is one counter increment plus one `os.Getenv`.

**SUGGESTION**

1. `TestNativePrePRGateShadowOnOffByteIdenticalForCompatibleBaseAdvance` does not assert that the ON run actually incremented `shadowDeriveBaseAdvanceCallCountForTest()`. Both new tests would still pass if the guard were hard-wired to `false`. Runtime stderr in the verbose run proved the ON path executes, but a one-line `want ≥ 1` assertion in the ON branch would make that machine-checked. (Prior SUGGESTION-3, narrowed and still open.)
2. Design Decision 1's "one exported function plus two exported data types **and nothing else**" is still literally unmet — 7 exported `ShadowRelation*` constants also exist. Unchanged; amend the decision or unexport.
3. The exit-bar test still injects a `synthetic/exit-bar-mechanism-proof` row that no reachable input shape can produce. Unchanged; sound but worth noting.
4. `.deadcode-baseline.txt` gained two entries (`shadowDeriveBaseAdvanceCallCountForTest`, `shadowResetDeriveBaseAdvanceCallCountForTest`). Consistent with the three pre-existing `shadowObserver*ForTest` baseline entries — test-only helpers the deadcode analyser cannot see from `_test.go`. Neutral, recorded for audit.

### TDD Compliance (delta)

| Check | Result | Details |
|---|---|---|
| RED evidence for the two new tests | Established | Guard-removal would flip the zero-times assertion to `1`; hook instruments `shadowDeriveBaseAdvance` itself, verified at `shadow_relation.go:140-142` |
| GREEN confirmed | Yes | 2/2 new tests PASS; 6/6 targeted; whole module exit 0 |
| Safety net | Yes | Full `go test ./... -count=1` re-run on the tip after the fix |
| Assertion quality | 1 WARNING | One inert tautology, see WARNING-5 |

### Verdict

**FAIL** — both prior CRITICAL findings are genuinely closed with independently re-executed runtime evidence: `gate.go` no longer performs any shadow Git derivation with the switch off, the zero-cost guard is machine-checked by a hook that instruments the exact function, and the pre-PR gate family now has real ON/OFF byte-identity coverage. One scenario, `rdd-candidate-identity` → "Pi overlay selector is explicitly out of scope", still has no covering test at runtime; that keeps the compliance count at 27/28, which `gentle-ai sdd-verify-validate` will not admit as a pass. Everything blocking archive is one small negative test away.

---

## Superseded — original FAIL verification report (tip `933fb329`)

Preserved verbatim as history. Only its opening code-fence language is changed from `yaml` to `text`, so this file carries exactly one authoritative `gentle-ai.verify-result/v1` envelope; no other byte is altered.

```text
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:7ebf4f11bef26f0ee4a33cf610e9197ce90fa3d68bedf34c306fc6d7329b3ed8
verdict: fail
blockers: 2
critical_findings: 2
requirements: 17/22
scenarios: 23/28
test_command: go test ./... -count=1
test_exit_code: 0
test_output_hash: sha256:a925440f213035135485df3bc07bee92547b3f690282fb4d1618f2c3d4f8cc51
build_command: go build ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: rdd-root-simplification-wave1
**Version**: Wave 1 (Shadow Algebra), 4 capability specs
**Mode**: Strict TDD
**Candidate**: worktree `/home/gentleman/work/gentle-ai-worktrees/rdd-wave0`, chain tip `933fb329`, working tree clean
**Verified by**: independent re-execution (apply-progress claims were not trusted)

### Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 50 (48 phase tasks + 2 injected) |
| Tasks complete | 50 |
| Tasks incomplete | 0 |
| Spec requirements | 22 |
| Spec scenarios | 28 |

### Chain Integrity

Each branch is exactly N commits ahead of its parent and 0 behind. Linear, no divergence.

| Slice | Branch | Tip | Ahead/Behind | Changed lines | Golden | Authored | vs 1000 cap |
|---|---|---|---|---|---|---|---|
| tracker | `feature/rdd-root-simplification` | `2674e9fe` | — | — | — | — | — |
| PR0 | `docs/rdd-wave1-sdd-artifacts` | `623ce88b` | 2 / 0 | 1080 | 0 | 1080 | over (docs/renames, pre-forecast ~1300) |
| PR1 | `test/rdd-wave1-prepr-base-advance-characterization` | `75933ae4` | 1 / 0 | 337 | 0 | 337 | within |
| PR2 | `feat/rdd-wave1-shadow-candidate-identity` | `baebc0b3` | 1 / 0 | 984 | 0 | 984 | within |
| PR3 | `feat/rdd-wave1-shadow-relation-algebra` | `01819875` | 1 / 0 | 688 | 0 | 688 | within |
| PR4 | `feat/rdd-wave1-shadow-authority-health` | `c6eb165b` | 1 / 0 | 306 | 0 | 306 | within |
| PR5+PR6+fix | `feat/rdd-wave1-shadow-observer-wiring` | `933fb329` | 4 / 0 | 1517 | 334 | 1183 | **over** (see WARNING-1) |

Per-commit line counts match apply-progress exactly (`e923545d` 648, `623ce88b` 435, `75933ae4` 333/4, `baebc0b3` 972/12, `01819875` 679/9, `c6eb165b` 300/6, `5704db8b` 432/26, `daf65dbc` 7/7, `92aed670` 973/10, `933fb329` 96/38).

### Build & Tests Execution

**Build**: PASS

```text
go build ./...                 exit 0, empty output
go vet ./...                   exit 0
gofmt -l internal/ docs/       empty
scripts/deadcode-ratchet.sh    exit 0 — "no new unreachable functions"
go test ./internal/cli/... -run TestEveryProductionRefusalNamesResolutionOrDeclaresByDesign  ok (refusal ratchet clean)
```

**Tests**: PASS — full module, every package

```text
go test ./... -count=1         exit 0, zero FAIL lines
  internal/reviewtransaction   ok  122.587s
  internal/cli                 ok  164.338s
```

All 32 Wave 1 tests re-executed individually and PASS: 2 `TestDeriveBaseAdvanceCompatibility*`, 7 `TestShadowIdentity*`, 11 `TestShadowRelate*`/`TestShadowRelation*`, 4 `TestShadowAuthorityHealth*`, 4 `TestShadowObserver*`/`TestShadowObservation*`, 2 `TestShadowReadOnlyGuard*`, 3 `TestShadowMatrix*`.

**Golden determinism**: `TestShadowMatrixCoveringArrayGolden` run twice without `-update`; PASS both times, `git status` clean after each — the golden is deterministic and byte-stable.

**Coverage**: not measured — no coverage tool configured for this module. Not a failure.

### Spec Compliance Matrix

#### rdd-candidate-identity (5 requirements / 7 scenarios)

| Requirement | Scenario | Test | Result |
|---|---|---|---|
| Canonical Identity Structure | Structure completeness | `TestShadowIdentityStructureIsComplete` | COMPLIANT |
| Selector Normalization | Staged and workspace converge | `TestShadowIdentitySelectorsConverge` | COMPLIANT |
| Selector Normalization | Committed-range resolves canonically | `TestShadowIdentitySelectorsConverge/committed-range...` | COMPLIANT |
| Read-Only Resolution | Resolution has no side effect | `TestShadowIdentityReadOnlyResolution` + `TestShadowReadOnlyGuardHoldsForProductionFiles` | COMPLIANT |
| Deterministic Ambiguity/Failure | Ambiguous selector returns full set | `TestShadowIdentityAmbiguousSelectorReturnsFullSet` | COMPLIANT |
| Deterministic Ambiguity/Failure | Unresolvable selector fails closed | `TestShadowIdentityUnresolvableSelectorFailsClosed` | COMPLIANT |
| Wave 1 Selector Scope | Pi overlay selector out of scope | (no named test) — closed 4-value `shadowSelectorKind` enum + `unsupported shadow selector kind` refusal, `shadow_identity.go:52-55,266` | PARTIAL |

#### rdd-candidate-relation-algebra (6 requirements / 9 scenarios)

| Requirement | Scenario | Test | Result |
|---|---|---|---|
| Seven-Value Relation Output | Identical candidate/policy → exact | `TestShadowRelateExactAndUnrelatedScenarios`, `TestShadowRelationSevenValuesNoEighth` | COMPLIANT |
| Seven-Value Relation Output | No governing lineage → unrelated | `TestShadowRelateExactAndUnrelatedScenarios` | COMPLIANT |
| `compatible_base_advance` Delegation (Amendment A) | All seven conditions hold | `TestShadowRelateDelegatesCompatibleBaseAdvanceToDeriveBaseAdvanceCompatibility` (real git fixture) | COMPLIANT |
| `compatible_base_advance` Delegation (Amendment A) | Any condition fails | `TestShadowRelateAmendmentANeverOverridesAFailedDelegatedCondition` | COMPLIANT |
| `provable_contraction` Degradation (Amendment B) | Contraction with no excluded-path findings | `TestShadowRelateAmendmentBDegradesContractionOnExcludedFinding` | COMPLIANT |
| `provable_contraction` Degradation (Amendment B) | Contraction with excluded-path finding degrades | `TestShadowRelateAmendmentBDegradesContractionOnExcludedFinding` | COMPLIANT |
| Characterization Tests Precede Delegation-Seam Changes | Coverage exists before delegation exercised | `prepr_base_advance_characterization_test.go` landed in `75933ae4` (PR1), two commits before the delegation seam in `01819875` (PR3); both tests PASS | COMPLIANT |
| Read-Only, Zero Live-Lifecycle Change | Shadow evaluation changes nothing observable | `TestShadowObservationSwitchIsRollbackBoundaryGateByteIdentical` | COMPLIANT |
| `ambiguous`/`unknown` No Fabricated Live Counterpart | Ambiguous fixture row marked, not fabricated | `TestShadowRelationHasNoLiveCounterpartOnlyForAmbiguousAndUnknown` + 8 `no-live-decision` golden rows | COMPLIANT |

Amendment A verified statically as well: `shadowDeriveBaseAdvance` (`shadow_relation.go:128-142`) calls `deriveBaseAdvanceCompatibility` (`prepr.go:73`) and returns nil on error. No shadow-side reimplementation of any of the seven conditions exists; `shadowBaseAdvanceApplies` only checks `proof.valid()` plus identity binding.

#### rdd-authority-graph-classification (4 requirements / 5 scenarios)

| Requirement | Scenario | Test | Result |
|---|---|---|---|
| Three-Value Health Classification | Consistent graph → healthy | `TestShadowAuthorityHealthThreeValueClassification` | COMPLIANT |
| Three-Value Health Classification | Classified leaf anomaly → repairable | `TestShadowAuthorityHealthThreeValueClassification` | COMPLIANT |
| No Mutation or Execution | Classification has no side effect | `TestShadowAuthorityHealthNoMutationOrExecution` | COMPLIANT |
| Fail-Closed on Unknown Shape | Unclassifiable shape blocks | `TestShadowAuthorityHealthFailClosedOnUnknownShape` (6 sub-tests) | COMPLIANT |
| Deterministic, Evidence-Backed | Same graph classifies identically | `TestShadowAuthorityHealthDeterministicEvidenceBacked` | COMPLIANT |

#### rdd-shadow-evaluation (7 requirements / 7 scenarios)

| Requirement | Scenario | Test | Result |
|---|---|---|---|
| Advisory-Only, Never Blocking | Shadow failure does not block the live path | `TestShadowObserverAdvisoryFailureNeverBlocksLivePath` | PARTIAL — see CRITICAL-1 (the requirement also forbids *delay*) |
| Disable Switch Is the Rollback Boundary | Disabling removes all shadow execution | `TestShadowObserverDisabledIsANoOp` covers `ObserveShadowRelation` only | **FAILING** — CRITICAL-1 |
| Zero Live-Lifecycle Behavior Change | Live outcome identical with shadow on or off | `TestShadowObservationSwitchIsRollbackBoundaryGateByteIdentical` (post-apply fixture only) | PARTIAL — see WARNING-3 |
| No Persisted Divergence Artifact | Divergence recorded outside production authority | `TestShadowObserverEnabledWritesStderrOnlyNeverStdout` + AST guard + chain diff touches no receipt/state schema | COMPLIANT |
| Off by Default in Live Paths | Default configuration produces no live Git cost | (no covering test) | **FAILING / UNTESTED** — CRITICAL-2 |
| Differential Matrix Exit Evidence | Matrix covers the full selector × relation space | `TestShadowMatrixCoveringArrayGolden` — golden re-tallied: 40 rows, 4 selectors × 10, all 7 relations, 16 agreement / 12 divergence (12 explained) / 8 no-live-decision / 4 no-shadow-decision, 0 unexplained | COMPLIANT |
| Unexplained Divergence Blocks Wave 2 | Unexplained core-relation divergence stops the boundary | `TestShadowMatrixUnexplainedDivergenceOnCoreRelationBlocksWave2` + `TestShadowMatrixRealCorpusHasZeroUnexplainedDivergences` | COMPLIANT |

**Compliance summary**: 23/28 scenarios COMPLIANT, 3 PARTIAL, 2 FAILING. 17/22 requirements fully compliant.

### Correctness (Static Evidence)

| Check | Status | Notes |
|---|---|---|
| Exactly 3 exported shadow symbols | Implemented (with deviation) | `CandidateIdentity`, `ShadowRelation`, `ObserveShadowRelation` are the only exported top-level declarations; 7 exported `ShadowRelation*` constants also exist (SUGGESTION-1) |
| AST readonly guard passes | Implemented | `TestShadowReadOnlyGuardHoldsForProductionFiles` PASS over all 4 production `shadow_*.go`; scanner self-tested against 6 synthetic sources |
| Switch defaults OFF | Implemented | `shadowObservationEnabled()` = `strings.TrimSpace(os.Getenv("GENTLE_AI_RDD_SHADOW")) != ""` — unset/empty/whitespace all OFF |
| No persisted divergence artifact | Implemented | Sink is an unexported in-memory slice; only writer is `fmt.Fprintf(shadowObserverStderr, ...)`; chain diff adds no file under `.git`, `review-state.json`, or `review-receipt.json` |
| CandidateTree binding fix present | Implemented | `shadow_relation.go:117` `input.Frozen.CandidateTree == input.Live.CandidateTree`; `TestShadowRelateBaseAdvanceProofBindsToCandidateTree` PASS |
| Observer swallows all errors | Implemented | `ObserveShadowRelation` returns nothing, `defer func(){ _ = recover() }()`, every error becomes `row.Err` |
| Zero live-lifecycle diff | Implemented (with deviation) | Non-shadow, non-test, non-docs production files changed vs tracker: `gate.go` +9/-0, `compact_gate.go` +7/-0, `compact_recovery_binding.go` +8/-0, `internal/cli/review_facade.go` +14/-0, `.deadcode-baseline.txt` +3/-4. Zero deletions in all four call-site files — no live logic was removed or rewritten. But see CRITICAL-1/2 for what one of those added lines does. |
| Wave 0 archive move resolved | Implemented | `623ce88b` moved all 8 files as pure renames (0/0) plus a new 181-line `archive-report.md`; archived `verify-report.md` is the full 263-line report; source `openspec/changes/rdd-root-simplification-wave0/` removed |

### Coherence (Design)

| Decision | Followed? | Notes |
|---|---|---|
| 1 — shadow inside `package reviewtransaction`, one exported function + two types | Mostly | 3 top-level exports as designed; 7 exported enum constants beyond the literal "and nothing else" (SUGGESTION-1) |
| 2 — opt-in `GENTLE_AI_RDD_SHADOW`, no shadow Git work on the human's blocking path | **No** | `gate.go:350` calls `shadowDeriveBaseAdvance` unconditionally — CRITICAL-1/2 |
| 3 — in-memory sink + stderr line + golden under `testdata/` | Yes | `gentle-ai.rdd-shadow/v1` stderr line; no sidecar journal |
| 4 — `CandidateIdentity` five-field tuple, `policy_hash` never fabricated | Yes | `TestShadowIdentityPolicyHashNeverFabricated` PASS |
| 5 — ordered fail-closed relation + Amendment B no-input degradation | Yes | `shadowRelate` order verified by reading and by `TestShadowRelateOrderedFailClosedPrecedence` |
| 6 — covering array ~40-60 rows, 4 verdict classes | Yes | 40 rows, 4 distinct verdicts, `no-shadow-decision` kept separate |
| 7 — six slices, feature-branch chain, ≤1000 authored lines each | Mostly | Chain correct; PR5+PR6+fix share one branch at 1183 authored lines (WARNING-1) |

### Documentation Deliverables

| Deliverable | Status | Evidence |
|---|---|---|
| `docs/architecture/rdd-shadow-evaluation.md` | Present (41 lines) | Documents `GENTLE_AI_RDD_SHADOW` default-off, never-blocks contract, exact `gentle-ai.rdd-shadow/v1` stderr line format, and the divergence-reporting path (`issues/new/choose`) |
| Design doc Wave 1 exit-evidence pointer | Present | `docs/architecture/rdd-root-simplification-design.md:445` — golden path, 40-row breakdown (16/12/8/4/0), operator-doc pointer |
| Amendment A paraphrase fix | Present | `openspec/specs/rdd-simplification-design/spec.md:55` — trust root inside condition 6, condition 7 (base/HEAD non-advance revalidation) restored |

### TDD Compliance

| Check | Result | Details |
|---|---|---|
| TDD Evidence reported | Partial | Full TDD Cycle Evidence table present for Batch 8 only; batches 1-7 tables elided as "unchanged, see prior revision" and are not retrievable from the current Engram artifact (WARNING-4) |
| All tasks have tests | Yes | Every phase task 1.1-6.8 names its RED test and GREEN production file in `tasks.md` |
| RED confirmed (tests exist) | Yes | All named test files exist and were re-executed |
| GREEN confirmed (tests pass) | Yes | 32/32 Wave 1 tests PASS on independent re-execution |
| Triangulation adequate | Yes | Table-driven throughout: 8 characterization sub-tests, 6 precedence sub-tests, 6 fail-closed sub-tests, 3 Amendment B sub-cases, 40-row corpus |
| Safety net for modified files | Yes | Full module suite re-run after every batch; re-verified here |

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|---|---|---|---|
| Unit (pure/table-driven) | 12 | 4 | `go test` |
| Integration (real `git` + real repo fixtures) | 17 | 5 | `go test` + system `git` |
| Golden | 3 | 1 | `go test` + `testdata/` |
| **Total** | **32** | **7** | |

E2E/browser layers: not applicable to this change.

### Assertion Quality

No banned patterns found. Zero tautologies, zero assertions that never call production code. Ghost-loop protection is explicit (`shadow_readonly_guard_test.go:161` fails when no production file is discovered). Empty-collection assertions all have non-empty companions: `shadowMatrixExitBarBlockers` is asserted `== 1` in the synthetic exit-bar test and `== 0` in the real-corpus test; `shadowObserverRowsForTest()` is asserted empty when OFF and non-empty when ON.

**Assertion quality**: all assertions verify real behavior.

### Quality Metrics

**Linter/vet**: no errors (`go vet ./...` exit 0)
**Formatter**: clean (`gofmt -l` empty)
**Deadcode ratchet**: clean
**Refusal ratchet**: clean (7 `shadow_identity.go` sites annotated `// refusal:by-design world-action:`, `GENTLE_AI_REFUSAL_RATCHET_UPDATE` never set)

### Issues Found

**CRITICAL**

1. **Shadow base-advance derivation runs on the live path with the switch OFF** — `internal/reviewtransaction/gate.go:350`

   ```go
   shadowAdvance := shadowDeriveBaseAdvance(ctx, repo, receipt, request, snapshot, resolvedPrePR, preimages)
   ```

   This call has no `shadowObservationEnabled()` guard and no gate-kind guard. It is on the straight-line path to `EvaluateNativeGate`'s final return, so it executes on every native gate evaluation regardless of `GENTLE_AI_RDD_SHADOW`.

   `shadowDeriveBaseAdvance` calls `deriveBaseAdvanceCompatibility` (`prepr.go:73`), which returns without Git only when `refs == nil`, `ExternalEvidence != None`, or no CI attestation artifact is present. `resolvedPrePR` is non-nil for `GatePrePush` and `GatePrePR` (`gate.go:367-408`). So for an attested pre-PR gate the full derivation runs — `merge-base`, `resolveTree`, up to three `changedPaths`, **two `patchIdentity` runs, and one `merge-tree --write-tree`** — with the switch unset.

   The live path guards the same derivation at `gate.go:296-299` with `request.Gate == GatePrePR && snapshot.BaseTree != receipt.BaseTree`. Two consequences:
   - base unchanged: live derives nothing, shadow adds a complete derivation;
   - base advanced: live derives once, shadow derives a **second** identical time.

   Runtime anchor: `TestNativePrePRGateAllowsOnlyCryptographicallyAttestedCompatibleBaseAdvance` (`gate_test.go:17`) passes with `GENTLE_AI_RDD_SHADOW` explicitly unset and asserts `evaluation.Context.BaseAdvance` is non-nil and `valid()` — proving the seven-condition derivation (including `merge-tree --write-tree`) completes at line 297 in that fixture. Line 350 is unconditional and receives identical arguments, so it repeats that exact sequence. External `GIT_TRACE2_EVENT` instrumentation cannot confirm invocation counts because `runGitCapturedRange` (`snapshot.go:1510`) replaces the child environment via `sanitizedGitEnvironmentForRun`.

   Violates:
   - `rdd-shadow-evaluation` → "Disable Switch Is the Rollback Boundary": *"When disabled, zero shadow code path executes"* and *"no resolver, relation, or classifier code runs"*. `shadowDeriveBaseAdvance` lives in `shadow_relation.go` and is the Amendment A delegation seam of the relation algebra.
   - `rdd-shadow-evaluation` → "Advisory-Only, Never Blocking": *"MUST NOT block, **delay**, or alter any ... live gate outcome"*.
   - design.md Decision 2, which rejected inline evaluation precisely because *"merge-tree --write-tree plus two patchIdentity runs per gate would put shadow Git work on the human's blocking path, breaching the freeze policy's Blocking budget"*, and its Threat Matrix note that shadow Git calls *"run only behind the opt-in switch or inside tests"*.

   Note on blast radius: the live **decision** is still outcome-neutral — `shadowAdvance` is only passed to the void `ObserveShadowRelation`, and derivation errors are swallowed. The breach is latency, repository object-store writes (`merge-tree --write-tree` creates a loose object), and the rollback-boundary contract, not correctness of the gate verdict.

   `tasks.md` 5.7 records this as deliberate ("additionally calls `shadowDeriveBaseAdvance` directly to exercise Amendment A's delegation seam from a live call site"), but no spec or design decision authorizes running it with the switch off.

2. **"Off by Default in Live Paths" has no covering test and is violated** — `rdd-shadow-evaluation` scenario *"Default configuration produces no live Git cost"* requires that with default configuration *"no additional `merge-tree` or patch-identity Git invocation occurs due to the shadow harness"*. CRITICAL-1 is a direct violation of that scenario, and no test asserts it. `tasks.md` 5.3 claims the scenario is proven by `shadowObserverCallCountForTest()` in `TestShadowObserverDisabledIsANoOp`, but that hook only counts `ObserveShadowRelation` entries; it cannot observe the sibling `shadowDeriveBaseAdvance` call, which is where the Git cost originates. The claimed proof does not cover the claimed requirement.

**WARNING**

1. **PR5 + PR6 + the pre-verify fix share one branch**, so the delivered slice is a single 1517-changed-line diff (1183 authored after excluding the 334-line golden) against PR4 — over design Decision 7's ≤1000 authored-lines-per-slice cap. Taken as separate logical slices each is within budget (PR5 458, PR6 997 / 663 authored, fix 134). Delivered granularity is 6 PRs, not the planned 7.
2. **PR0 is 1080 changed lines**, above the 1000 cap. Pre-forecast in `tasks.md` at ~1300 and accepted as doc/rename bulk; recorded for completeness.
3. **ON/OFF byte-identity coverage is post-apply-only.** `TestShadowObservationSwitchIsRollbackBoundaryGateByteIdentical` uses `nativeGateFixture` (post-apply), where `resolvedPrePR` is nil. `tasks.md` 5.5 generalizes from it to all five gate kinds on the grounds that they share one `ObserveShadowRelation` call site. That reasoning holds for the observer, but not for the sibling `shadowDeriveBaseAdvance` call, whose behaviour differs materially between nil and non-nil `resolvedPrePR`. The pre-PR and pre-push gate kinds — the only ones where shadow Git work actually occurs — are not covered by any ON/OFF test.
4. **TDD Cycle Evidence for batches 1-7 is not retrievable.** The current `sdd/rdd-root-simplification-wave1/apply-progress` revision replaces those tables with "unchanged, see prior revision for full table". Substituted here by per-task RED/GREEN prose in `tasks.md` and by full independent re-execution of every test.
5. **`tasks.md` 0.1-0.3 prose is stale.** All three are `[x]` while their text still says BLOCKED / "not performed this batch". The follow-up commit `623ce88b` actually completed the archive move (verified: 8 pure renames, full 263-line `verify-report.md` archived, source directory removed). Artifact text contradicts the delivered code state.
6. **`docs/architecture/rdd-shadow-evaluation.md` overstates the off state.** Quick-path step 1 says *"shadow evaluation is off, and zero shadow code runs"*. Given CRITICAL-1 that is inaccurate for pre-PR and pre-push gates.
7. **Wave 1 Selector Scope has no named negative test.** The pi-overlay-out-of-scope scenario is satisfied structurally by the closed four-value `shadowSelectorKind` enum and the `unsupported shadow selector kind` refusal, but nothing names the scenario.

**SUGGESTION**

1. Design Decision 1 says "one exported function plus two exported data types **and nothing else**". Seven exported `ShadowRelation*` constants also exist. They are the vocabulary of an exported type and are arguably in-contract, but the decision's literal wording is not met — either amend the decision or unexport the constants.
2. The exit-bar test now injects a `synthetic/exit-bar-mechanism-proof` row that the test's own comment says "a real caller could never produce". The mechanism is proven and the real-corpus zero-blocker counterpart exists, so this is sound — but the exit bar is no longer exercised by any reachable input shape.
3. Consider adding a Git-invocation-count assertion (via a counting hook around `runGit`) so the "no live Git cost by default" requirement becomes machine-checked rather than reasoned.

### Deviations Log (documented elaborations from batches 4-8)

| # | Deviation | Origin | Assessment |
|---|---|---|---|
| D1 | `shadowRelationInput.LiveUnresolvable` added beyond design.md's field sketch | Batch 4 (PR3) | Accepted — documented in `shadow_relation.go:32-42`; needed so "unknown" stays a relation-function outcome |
| D2 | Threat-matrix commit-state narrowed to unborn HEAD / empty index rather than all four projections | Batch 4, task 3.5 | Accepted — matches design.md's own Threat Matrix response |
| D3 | `unrelated` recorded as its own explained `divergence` row instead of `no-live-decision` | Batch 7, task 6.3, maintainer-confirmed | Accepted — stricter than the spec, which only mandates the marking for `ambiguous`/`unknown` |
| D4 | Matrix corpus built at the pure-function level, not through real Git resolution | Batch 7, task 6.1 | Accepted — `shadowRelate` and `classifyCompactTargetRelation` are pure; selector normalization is proven separately by PR2 |
| D5 | Injected Task 0 — 7 refusal-ratchet violations fixed by `// refusal:by-design` annotation | Batch 7 | Accepted — re-verified: ratchet PASS, `GENTLE_AI_REFUSAL_RATCHET_UPDATE` never set |
| D6 | Injected Task 1 — `CandidateTree` binding gap fixed pre-verify on the same branch | Batch 8 | Accepted — real algebra gap, RED/GREEN evidence re-verified, golden unchanged |
| D7 | PR6 continued on PR5's branch instead of its own | Batch 7 | See WARNING-1 |
| D8 | `shadowDeriveBaseAdvance` called unguarded from `gate.go` | Batch 6, task 5.7 | **Rejected** — see CRITICAL-1/2 |

### Verdict

**FAIL** — the implementation is functionally correct, fully tested at the algebra layer, and outcome-neutral for every live gate verdict, but `gate.go:350` runs the shadow base-advance delegation seam on the live pre-PR/pre-push path with the disable switch off, breaking the rollback-boundary and off-by-default requirements that Wave 1's own spec names as its blocking-budget compliance contract.
