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
│   │   ├── Must say "✅ Written"
│   │   ├── Verify: test file EXISTS in the codebase
│   │   └── Flag: CRITICAL if test file does not exist
│   │
│   ├── GREEN column:
│   │   ├── Must say "✅ Passed"
│   │   ├── Cross-reference with Step 5b test execution results:
│   │   │   └── The test file listed must PASS when you run it
│   │   └── Flag: CRITICAL if test fails now (was it really green?)
│   │
│   ├── TRIANGULATE column:
│   │   ├── If "✅ N cases" → verify N test cases exist in the test file
│   │   ├── If "➖ Single" → verify spec truly has only one scenario for this task
│   │   └── Flag: WARNING if spec has multiple scenarios but only 1 test case
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
| RED confirmed (tests exist) | ✅ / ⚠️ | {N}/{total} test files verified |
| GREEN confirmed (tests pass) | ✅ / ❌ | {N}/{total} tests pass on execution |
| Triangulation adequate | ✅ / ⚠️ / ➖ | {N} tasks triangulated / {N} single-case |
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
- Apply any `rules.verify` from `openspec/config.yaml`
