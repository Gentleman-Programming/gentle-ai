# Strict TDD Module — Verify Phase

> **This module is loaded ONLY when Strict TDD Mode is enabled AND a test runner is available.**
> If you are reading this, the orchestrator already verified both conditions. Follow every instruction.

## TDD Verification Philosophy

When Strict TDD Mode is active, verification goes beyond "does the code work?" to "was the code built correctly?" — meaning: was TDD actually followed? The apply phase reports TDD evidence; your job is to validate that evidence against reality.

## Step 5a: TDD Compliance Check (includes Assertion Quality Audit)

Read the `apply-progress` artifact and verify that TDD was actually followed:

```
Read apply-progress artifact:
├── Find the "TDD Cycle Evidence" table
├── FOR EACH task row:
│   ├── RED column:
│   │   ├── For qualifying behavior, must say "✅ Executed behavioral failure"
│   │   ├── Verify: command, exit status, failure class, and bounded output show an assertion/observable-behavior failure
│   │   └── Flag: CRITICAL if RED is only written, structural, or not reproducible
│   │
│   ├── GREEN column:
│   │   ├── Must say "✅ Passed"
│   │   ├── Cross-reference with Step 5b test execution results:
│   │   │   └── The test file listed must PASS when you run it
│   │   └── Flag: CRITICAL if test fails now (was it really green?)
│   │
│   ├── TRIANGULATE column:
│   │   ├── Verify semantic partitions reject the simplest special-case implementation
│   │   ├── Verify the verifier-owned anti-Fake-It fields and pass/fail decision
│   │   └── Flag: WARNING/CRITICAL when the decision or rationale is missing or failing
│   │
│   ├── SAFETY NET column:
│   │   ├── If "✅ N/N" → existing tests were run before modification (good)
│   │   ├── If "N/A (new)" → verify the file was actually NEW (not modified)
│   │   └── Flag: WARNING if file was modified but safety net shows "N/A"
│   │
│   └── REFACTOR column:
│       ├── Not strictly verifiable (subjective quality)
│       └── Skip verification, trust the report
│
├── If NO "TDD Cycle Evidence" table found:
│   └── Flag: CRITICAL — apply phase did not report TDD evidence
│       (Strict TDD was enabled but apply did not follow the protocol)
│
└── Summary: "{N}/{total} tasks have complete TDD evidence"
```

## Step 5 Expanded: Test Layer Validation

Classify ALL test files related to this change by their testing layer:

```
Scan test files created/modified by this change:
├── Classify each test file:
│   ├── Unit test: tests a single function/class in isolation
│   │   └── Indicators: no render(), no page., no HTTP calls, mocked dependencies
│   ├── Integration test: tests component interaction or user behavior
│   │   └── Indicators: render(), screen., userEvent., testing-library imports
│   ├── E2E test: tests full system through real browser/HTTP
│   │   └── Indicators: page.goto(), playwright/cypress imports, browser context
│   └── Unknown: cannot classify → report as-is
│
├── Report distribution:
│   ├── Unit: {N} tests across {N} files
│   ├── Integration: {N} tests across {N} files
│   ├── E2E: {N} tests across {N} files
│   └── Total: {N} tests
│
├── Cross-reference with capabilities:
│   ├── If integration tests exist but tools not in capabilities → how?
│   ├── If E2E tests exist but tools not in capabilities → how?
│   └── Flag: WARNING if tests use tools not detected in capabilities
│
└── For each spec scenario: note which layer covers it
    └── Flag: SUGGESTION if critical business logic only has unit tests
        (only if integration/E2E tools are available)
```

## Step 5d Expanded: Changed File Coverage

When coverage tool is available, report coverage for CHANGED files specifically:

```
IF coverage tool available (from cached capabilities):
├── Run: {test_command} --coverage (or equivalent)
├── Parse the coverage report
├── Filter to ONLY files created or modified in this change
│   (get file list from apply-progress "Files Changed" table)
├── Report per-file:
│   ├── File path
│   ├── Line coverage %
│   ├── Branch coverage % (if available)
│   ├── Uncovered line ranges (specific lines, not just %)
│   └── Flag per file:
│       ├── ≥ 95% → ✅ Excellent
│       ├── ≥ 80% → ⚠️ Acceptable
│       └── < 80% → ⚠️ Low (list uncovered lines)
├── Report aggregate:
│   ├── Average coverage of changed files
│   ├── Total uncovered lines in changed files
│   └── Compare to threshold if configured
└── Flag: WARNING if any changed file < 80% coverage

IF coverage tool NOT available:
└── Report: "Coverage analysis skipped — no coverage tool detected"
    (NOT a failure — just not available)
```

## Step 5e: Quality Metrics (if tools available)

Run quality checks ONLY on changed files, ONLY if tools are available:

```
Read quality tools from cached capabilities:

IF linter available:
├── Run linter on changed files only
├── Report: errors and warnings
└── Flag: WARNING for errors, SUGGESTION for warnings

IF type checker available:
├── Run type checker (usually whole-project, not per-file)
├── Filter output to changed files
├── Report: type errors in changed files
└── Flag: WARNING for type errors

IF neither available:
└── Report: "Quality metrics skipped — no tools detected"
```

## Report Template Extension

When Strict TDD Mode is active, your verification report MUST include these additional sections:

```markdown
### TDD Compliance
| Check | Result | Details |
|-------|--------|---------|
| TDD Evidence reported | ✅ / ❌ | {Found in apply-progress / Missing} |
| All tasks have tests | ✅ / ❌ | {N}/{total} tasks have test files |
| RED confirmed (behavior executed) | ✅ / ⚠️ | {N}/{total} qualifying behaviors have reproducible behavioral RED |
| GREEN confirmed (tests pass) | ✅ / ❌ | {N}/{total} tests pass on execution |
| Semantic partitions and anti-Fake-It adequate | ✅ / ⚠️ / ➖ | {N} decisions verified / {N} not applicable |
| Safety Net for modified files | ✅ / ⚠️ | {N}/{total} modified files had safety net |

**TDD Compliance**: {N}/{total} checks passed

---

### Test Layer Distribution
| Layer | Tests | Files | Tools |
|-------|-------|-------|-------|
| Unit | {N} | {N} | {tool} |
| Integration | {N} | {N} | {tool or "not installed"} |
| E2E | {N} | {N} | {tool or "not installed"} |
| **Total** | **{N}** | **{N}** | |

---

### Changed File Coverage
| File | Line % | Branch % | Uncovered Lines | Rating |
|------|--------|----------|-----------------|--------|
| `path/to/file.ext` | 95% | 90% | — | ✅ Excellent |
| `path/to/other.ext` | 82% | 75% | L45-48, L62 | ⚠️ Acceptable |
| `path/to/new.ext` | 100% | 100% | — | ✅ Excellent |

**Average changed file coverage**: {N}%
{or "Coverage analysis skipped — no coverage tool detected"}

---

### Assertion Quality
| File | Line | Assertion | Issue | Severity |
|------|------|-----------|-------|----------|
| ... | ... | ... | ... | ... |

**Assertion quality**: {N} CRITICAL, {N} WARNING
{or "✅ All assertions verify real behavior"}

---

### Quality Metrics
**Linter**: ✅ No errors / ⚠️ {N} warnings / ❌ {N} errors / ➖ Not available
**Type Checker**: ✅ No errors / ❌ {N} errors / ➖ Not available
```

## Step 5f: Assertion Quality Audit (MANDATORY)

Scan ALL test files created or modified by this change and check for trivial/meaningless assertions:

```
FOR EACH test file related to the change:
├── Read the file content
├── Scan for BANNED assertion patterns:
│   ├── Tautologies: expect(true).toBe(true), assert True, expect(1).toBe(1)
│   ├── Orphan empty checks: expect(result).toEqual([]) or assert len(result) == 0
│   │   └── UNLESS there is a companion test with same setup that asserts NON-EMPTY
│   ├── Type-only assertions used alone: toBeDefined(), not.toBeNull(), typeof checks
│   │   └── These are OK if COMBINED with value assertions in the same test
│   ├── Assertions that never call production code (no function call, no render, no request)
│   ├── Ghost loops: assertions inside for/forEach over queryAll/filter results
│   │   └── Check if the collection could be empty — if so, the assertions NEVER RUN
│   │       Flag: CRITICAL — a loop over an empty array is a test that ALWAYS passes
│   ├── Incomplete TDD cycle: test passes because preconditions prevent code from running
│   │   └── e.g., testing behavior of a component that is never rendered due to state
│   │       Flag: CRITICAL — test must set up conditions where the code path IS exercised
│   ├── Smoke-test-only: render() + toBeInTheDocument() without behavioral assertions
│   │   └── "Renders without crash" is NOT a valid test — it must assert WHAT was rendered
│   │       Flag: WARNING — smoke tests do not count toward TDD coverage
│   ├── Implementation detail coupling: assertions on CSS classes, internal state, mock call counts
│   │   └── expect(el.className).toContain("text-xs") or expect(mock.calls.length).toBe(3)
│   │       Flag: WARNING — tests must assert behavior, not implementation
│   └── Mock/assertion ratio: count vi.mock() calls vs expect() calls per test file
│       └── If mocks > 2× assertions → Flag: WARNING — "Mock-heavy test ({N} mocks, {N} assertions)"
│           Recommend: extract logic to pure function or move to higher test layer
│
├── For each violation found:
│   ├── Record: file, line number, the assertion, why it's trivial
│   └── Classify:
│       ├── CRITICAL: tautology (expect(true).toBe(true)) — test proves NOTHING
│       ├── CRITICAL: assertion without production code call — test exercises nothing
│       ├── CRITICAL: ghost loop — assertions inside loop over possibly-empty collection
│       ├── WARNING: empty collection without companion non-empty test
│       ├── WARNING: type-only assertion without value assertion
│       ├── WARNING: smoke-test-only — render + toBeInTheDocument without behavioral check
│       ├── WARNING: CSS class / implementation detail assertion
│       └── WARNING: mock-heavy test (mocks > 2× assertions) — wrong test layer
│
├── Check triangulation quality:
│   ├── Count distinct test cases per behavior
│   ├── If only 1 test case exists for a behavior with multiple spec scenarios:
│   │   └── Flag: WARNING — "Insufficient triangulation for {behavior}"
│   ├── If all test cases assert the SAME type of value (e.g., all check empty arrays):
│   │   └── Flag: WARNING — "No variance in test expectations — all assert empty/trivial"
│   └── A well-triangulated behavior has tests asserting DIFFERENT expected values
│
└── Summary: "{N} trivial assertions found across {N} files"
```

### Assertion Quality Report Table

Include this table in the verification report when any issues are found:

```markdown
### Assertion Quality
| File | Line | Assertion | Issue | Severity |
|------|------|-----------|-------|----------|
| `path/test.ts` | 15 | `expect(true).toBe(true)` | Tautology — proves nothing | CRITICAL |
| `path/test.ts` | 23 | `expect(result).toEqual([])` | Empty without companion non-empty test | WARNING |
| `path/test.ts` | 31 | `expect(result).toBeDefined()` | Type-only — no value asserted | WARNING |

**Assertion quality**: {N} CRITICAL, {N} WARNING
```

If zero issues found, report: "**Assertion quality**: ✅ All assertions verify real behavior"

## Rules (Strict TDD Verify specific)

- ALWAYS check the TDD Cycle Evidence table from apply-progress — it's the primary artifact
- ALWAYS cross-reference reported test files against actual execution — don't trust the report blindly
- ALWAYS run the Assertion Quality Audit (Step 5f) — trivial tests are WORSE than missing tests
- If apply-progress has no TDD evidence table, flag as CRITICAL — the protocol was not followed
- If tautology assertions are found (expect(true).toBe(true)), flag as CRITICAL — these MUST be rewritten
- Coverage and quality metrics are informational, NOT blocking — only flag as WARNING, never CRITICAL
- Test layer distribution is informational — SUGGESTION level only
- DO NOT fix issues — only report. The orchestrator decides.
- If coverage/quality tools are not available, say so cleanly and move on — never flag missing tools as failures

## Behavioral Falsification Verification Contract

This is the canonical verification contract for the apply artifact's behavioral falsification evidence. It is a prompt-and-artifact pilot: verification must disclose its limits and must not claim native historical proof or universal enforcement.

### Admission, qualification, and identity

For a qualifying behavioral work unit, verify an implementation-independent oracle, a deterministic passing focused command, Git-backed source, exact worktree reconstruction, and a bounded production-only fault surface. Documentation, formatting, generated-only, structural-only, and infrastructure-only work is `not-applicable` and keeps its existing evidence method; do not infer project-wide classification here.

Read and bind the apply artifact to these exact fields before accepting evidence:

| Field | Verification rule |
|---|---|
| `behavior_id` / boundary | Names the protected behavior and contract boundary. |
| `oracle_source` | Comes from an approved spec scenario, protocol, business rule, or invariant and is independent of the implementation. |
| `focused_command` | Exact command, working directory, timeout, exit status, and bounded output are recorded. |
| `test_identity` | Full path/mode/SHA-256 manifest covers every focused test, fixture, and test configuration file. |
| `production_identity` | A manifest and root identify affected production paths at RED, GREEN, and post-fault. |
| `base_revision` | Same base commit is used for snapshot and candidate reconstruction. |
| `pre_green_snapshot` | Reconstructible bundle contains the base commit SHA, retained binary tracked diff, retained untracked-file bytes, sorted full-worktree path/mode/SHA-256 index, and canonical bundle-index SHA-256. |

The verify artifact MUST record these falsification result fields explicitly:

| Field | Verification rule |
|---|---|
| `fault` | Verifier-selected, plausible production-only fault and patch identity. |
| `counterfactual_result` | Exactly one outcome per fault attempt, including the protected behavior and observed result. |
| `residual_risk` | Explicit remaining risk and the applicable blocking or degradation decision. |

### Executed RED, GREEN, and renewed RED

RED is valid only when the focused command was executed and failed at the assertion or observable-behavior level for the named behavior; the evidence label must be `executed behavioral RED`. A missing-symbol compile failure is `structural RED`; structural RED does not satisfy behavioral RED. Verify `expected_failure_class`, `observed_failure_class`, exit status, and bounded output. A written-only claim is a CRITICAL failure.

GREEN must run the same `focused_command` against the candidate and record the candidate `production_identity`, successful exit status, and output. After GREEN, freeze the focused command and full `test_identity` before the verifier selects a fault.

Any material test changes after RED invalidate the prior RED. If the oracle, focused setup, fixture, command, test, or test configuration changes materially after RED, reconstruct the retained `pre_green_snapshot`, overlay the changed test inputs, and execute renewed RED for the expected behavioral reason. Missing, hash-mismatched, or failed materialization is `unavailable`; the current production tree is not a substitute.

### Semantic partitions and anti-Fake-It

Verify semantic partitions, not a fixed case count. Evidence must reject the simplest constant, literal, or single-example special-case implementation. Check distinct input/output partitions and boundaries, state transitions or repeated calls, and contract-relevant success/failure partitions. A true single-point invariant requires a rationale explaining why no meaningful second partition exists.

The verifier owns and records every anti-Fake-It field:

- `anti_fake_it.applicable`;
- `anti_fake_it.simplest_rejected_implementation`;
- `anti_fake_it.discriminating_tests`;
- `anti_fake_it.decision` (`pass` or `fail`); and
- `anti_fake_it.rationale`.

When applicable, the primary fault MUST target `anti_fake_it.simplest_rejected_implementation`. A `fail` decision blocks fault execution. Raw coverage and test count are diagnostics, never proof.

### Canonical candidate and exact isolation

Before execution, verify one canonical candidate manifest whose canonical UTF-8 JSON contains the base commit SHA, SHA-256 of the retained binary tracked delta, and lexicographically sorted path/mode/SHA-256 entries for every materialized candidate path, including intended untracked files. `candidate_root` is the SHA-256 of those exact JSON bytes.

The identity predicate is exact: materialize the base, apply the indexed tracked delta, copy only the indexed untracked files, recompute the complete index, and require byte-for-byte index equality and the same `candidate_root` before every execution. Use an exact disposable detached Git worktree; the verifier-selected fault is production-only and never touches the source worktree or candidate. Remove the worktree and temporary artifacts on every exit path; report cleanup failure explicitly. Isolation or reconstruction failure is `unavailable`, not a permissive fallback.

### Four runs, no rerun loop

The complete stability budget is exactly two candidate runs plus two counterfactual runs. Independently materialize two unmodified candidates and require both to pass at the canonical `candidate_root` with the same behavior and test identity. Independently materialize two counterfactuals, apply the verifier-selected production-only fault, recompute the frozen test-source manifest and post-fault production identity/root, and run the same focused command twice.

Both counterfactual runs must fail for `protected_test_or_behavior` with the expected failure class, matching bounded `observed_output_excerpt`, and the recorded `post_fault_production_root`. Any disagreement between candidate runs or counterfactual runs, compile/setup/unrelated crash, timeout, missing protected-test result, class mismatch, output mismatch, or identity mismatch is `invalid` or `unavailable`, never `killed`. There is no rerun loop and no additional rerun loop.

### Verifier-owned fault, replacement, outcomes, and blocking

The verifier-owned fault selection is authoritative. Record one concrete plausible production-only fault; do not derive it by changing an expected value to a random alternative. Permit one primary fault and one replacement only when the primary outcome is `equivalent` or `invalid`. A valid `survived` fault is not replaceable. If both selections are equivalent or invalid, the result is `inconclusive` and the replacement budget is exhausted.

Record exactly one outcome per attempt:

| Outcome | Meaning |
|---|---|
| `killed` | Both candidate runs pass and both valid counterfactual runs fail the protected behavior at the post-fault production root. |
| `survived` | Candidate and valid counterfactual both pass. |
| `equivalent` | The plausible fault does not change observable behavior under the approved oracle. |
| `invalid` | The plausible fault is not materialized, frozen scope changes, or an unrelated compile/setup/crash/timeout/harness failure occurs. |
| `unavailable` | Exact isolation or deterministic execution cannot be established. |
| `not-applicable` | Work is outside qualifying behavioral scope. |

Every outcome record includes `protected_test_or_behavior`, `expected_failure_class`, `observed_failure_class`, `observed_output_excerpt`, and `post_fault_production_root`. Only `killed` is successful falsification; `survived`, `equivalent`, `invalid`, and `unavailable` never inflate success.

`survived` is always CRITICAL and blocks the current verify attempt. Record blocking/degradation explicitly: `inconclusive` and `unavailable` are WARNING with explicit residual risk for ordinary behavior, but CRITICAL for authorization, security, update, delivery, payment, migration/persistence, or data-loss behavior. `not-applicable` is informational and uses the existing evidence method. `killed` passes this evidence gate but leaves residual risk and does not replace requirements verification. A CRITICAL result requires a new executed RED/GREEN cycle and new test freeze; only one bounded remediation/re-verification attempt is allowed.

### Issue boundaries and pilot limits

#3727 owns behavioral falsification evidence and this first prompt/artifact pilot. #262 owns mandatory phase progression, native settlement, and guarantees that verification cannot be skipped. #986 owns project-wide risk classification and evidence-method rubrics. #1263 owns optional mutation-provider integration and Gherkin; no universal mutation-score threshold is adopted here.

This pilot does not claim native historical proof or universal enforcement. Native ledgers, runtime/CLI enforcement, project-wide classification, mutation providers, Gherkin, and broad autonomous mutation campaigns are out of scope. Benchmark the pilot on the approved corpus before proposing native ledger work.
