# Strict TDD Module — Verify Phase

> **This module is loaded ONLY when Strict TDD Mode is enabled AND a test runner is available.**
> If you are reading this, the orchestrator already verified both conditions. Follow every instruction.

## TDD Verification Philosophy

When Strict TDD Mode is active, verification goes beyond "does the code work?" to "was the code built correctly?" — meaning: was TDD actually followed? The apply phase reports TDD evidence; your job is to validate that evidence against reality.

## Step 5a: TDD Compliance Check

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

### Quality Metrics
**Linter**: ✅ No errors / ⚠️ {N} warnings / ❌ {N} errors / ➖ Not available
**Type Checker**: ✅ No errors / ❌ {N} errors / ➖ Not available
```

## Rules (Strict TDD Verify specific)

- ALWAYS check the TDD Cycle Evidence table from apply-progress — it's the primary artifact
- ALWAYS cross-reference reported test files against actual execution — don't trust the report blindly
- If apply-progress has no TDD evidence table, flag as CRITICAL — the protocol was not followed
- Coverage and quality metrics are informational, NOT blocking — only flag as WARNING, never CRITICAL
- Test layer distribution is informational — SUGGESTION level only
- DO NOT fix issues — only report. The orchestrator decides.
- If coverage/quality tools are not available, say so cleanly and move on — never flag missing tools as failures
