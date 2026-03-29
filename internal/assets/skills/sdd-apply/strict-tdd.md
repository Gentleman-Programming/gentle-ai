# Strict TDD Module — Apply Phase

> **This module is loaded ONLY when Strict TDD Mode is enabled AND a test runner is available.**
> If you are reading this, the orchestrator already verified both conditions. Follow every instruction.

## TDD Philosophy

TDD is not testing. TDD is **software design driven by tests**. You write a test that describes what the code SHOULD do, then write the minimum code to make it real. The tests design the API, the contracts, the behavior. Code is a side effect of tests.

### The Three Laws

1. **Do NOT write production code** until you have a failing test
2. **Do NOT write more test** than is necessary to fail
3. **Do NOT write more code** than is necessary to pass the test

## TDD Implementation Cycle

For EVERY task assigned to you, follow this cycle strictly:

```
FOR EACH TASK:
├── 0. SAFETY NET (only if modifying existing files)
│   ├── Run existing tests for files being modified
│   ├── Capture baseline: "{N} tests passing"
│   ├── If any FAIL → STOP, report as "pre-existing failure"
│   │   (do NOT fix pre-existing failures — report to orchestrator)
│   └── This baseline proves you did not break what already worked
│
├── 1. UNDERSTAND
│   ├── Read the task description
│   ├── Read relevant spec scenarios (these ARE your acceptance criteria)
│   ├── Read the design decisions (these CONSTRAIN your approach)
│   ├── Read existing code and test patterns (match the style)
│   └── Determine test layer (see "Choosing Test Layer" below)
│
├── 2. RED — Write a failing test FIRST
│   ├── Write test(s) that describe the expected behavior from the spec
│   ├── Prefer pure functions where possible (no side effects = easy to test)
│   ├── The test MUST reference production code that does NOT exist yet
│   │   (this guarantees failure — no need to execute to confirm)
│   ├── If the production code/function already exists:
│   │   └── Write a test for the NEW behavior that is NOT yet implemented
│   └── GATE: Do NOT proceed to GREEN until the test is written
│
├── 3. GREEN — Write the MINIMUM code to pass
│   ├── Implement ONLY what the failing test needs
│   ├── Fake It is VALID here (hardcoded return values are OK)
│   ├── EXECUTE tests → must PASS
│   │   ├── ✅ Passed → proceed to TRIANGULATE or REFACTOR
│   │   └── ❌ Failed → fix the implementation, NOT the test
│   └── GATE: Do NOT proceed until GREEN is confirmed by execution
│
├── 4. TRIANGULATE (when behavior has multiple cases)
│   ├── Check: does the spec define multiple scenarios for this behavior?
│   ├── If YES (multiple inputs/outputs/edge cases):
│   │   ├── Add a second test case with DIFFERENT inputs/expected outputs
│   │   ├── EXECUTE tests → if Fake It breaks (hardcoded no longer works):
│   │   │   └── Generalize to real logic (this is the whole point)
│   │   ├── Repeat until ALL spec scenarios for this task are covered
│   │   └── Each triangulation pass: write test → run → fix implementation
│   ├── If NO (single-case behavior, e.g., config setup, simple mapping):
│   │   └── Skip triangulation, proceed to REFACTOR
│   └── GATE: All spec scenarios for this task must have tests before REFACTOR
│
├── 5. REFACTOR — Improve without changing behavior
│   ├── Extract constants (eliminate magic numbers)
│   ├── Extract functions (reduce cyclomatic complexity)
│   ├── Improve naming, remove duplication
│   ├── Push toward pure functions where feasible
│   ├── Apply Boy Scout Rule: leave code cleaner than you found it
│   ├── EXECUTE tests after EACH refactoring step → must STILL PASS
│   │   ├── ✅ Still passing → refactoring is safe, continue
│   │   └── ❌ Failed → REVERT that refactoring step, try smaller
│   └── GATE: Tests green after EVERY refactoring change
│
├── 6. Mark task complete [x]
└── 7. Note any deviations or issues discovered
```

## Choosing Test Layer

Based on the testing capabilities cached in Engram (`sdd/{project}/testing-capabilities`), choose the appropriate test layer for each task:

```
Determine test layer by WHAT the task does:
├── Pure logic, utility function, calculation, data transformation
│   └── Unit test (always available if test runner exists)
│
├── Component rendering, user interaction, state changes
│   ├── IF integration tools available → Integration test
│   └── IF NOT → Unit test with mocks (degrade gracefully)
│
├── Multi-component flow, API interaction, context/provider behavior
│   ├── IF integration tools available → Integration test
│   └── IF NOT → Unit test with mocks
│
├── Critical business flow, full user journey, cross-page navigation
│   ├── IF E2E tools available → E2E test
│   ├── IF NOT but integration available → Integration test
│   └── IF neither → Unit test (degrade gracefully)
│
└── Default: Unit test (always the fallback)
```

**Key rule**: Use the HIGHEST available layer that fits the task. But NEVER skip a task because a layer is unavailable — degrade to the next available layer.

## Test Execution

Detect the test runner from the cached testing capabilities:

```
Read test command from:
├── Cached capabilities → test_runner.command (fastest — already detected)
├── openspec/config.yaml → rules.apply.test_command (override)
└── Fallback: detect from package.json/pyproject.toml/go.mod

When executing tests during TDD:
├── Run ONLY the relevant test file, not the entire suite
│   ├── JS/TS: {runner} {test-file-path} (e.g., pnpm vitest run src/utils/tax.test.ts)
│   ├── Python: pytest {test-file-path}
│   ├── Go: go test ./{package}/... -run {TestName}
│   └── Adapt to the runner's CLI
├── This keeps the cycle FAST
└── Full suite runs happen in sdd-verify, not here
```

## Pure Function Preference

When writing production code in GREEN/TRIANGULATE steps, prefer pure functions:

```
✅ PREFER (pure — easy to test):
function calculateDiscount(price: number, quantity: number): number {
  return quantity >= 5 ? price * quantity * 0.1 : 0
}

❌ AVOID (impure — hard to test):
function calculateDiscount(item: Item) {
  globalState.lastDiscount = item.price * 0.1  // side effect
  updateDOM()                                   // side effect
  return globalState.lastDiscount
}
```

**Why**: Pure functions are deterministic (same input → same output), have no side effects, and are trivially testable. TDD naturally pushes you toward pure functions — embrace it.

## Approval Testing (for refactoring existing code)

When a task involves REFACTORING existing code (not writing new code):

```
BEFORE touching production code:
├── 1. Identify existing behavior to preserve
├── 2. Write "approval tests" that capture current behavior:
│   ├── Call the function with known inputs
│   ├── Assert the CURRENT outputs (even if ugly or wrong)
│   └── These tests document what the code does NOW
├── 3. Run approval tests → must PASS (they describe current reality)
├── 4. NOW refactor the production code
├── 5. Run approval tests again → must STILL PASS
│   ├── ✅ Passing → refactoring preserved behavior
│   └── ❌ Failing → refactoring broke something, revert
└── 6. If the spec says behavior should CHANGE:
    ├── Update the approval test to reflect NEW expected behavior
    ├── Run → test FAILS (RED — new behavior not implemented yet)
    └── Implement new behavior → GREEN
```

## Return Summary Extension

When Strict TDD Mode is active, your return summary MUST include this section:

```markdown
### TDD Cycle Evidence
| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 1.1 | `path/test.ext` | Unit | ✅ 5/5 | ✅ Written | ✅ Passed | ✅ 3 cases | ✅ Clean |
| 1.2 | `path/test.ext` | Integration | N/A (new) | ✅ Written | ✅ Passed | ➖ Single | ✅ Clean |
| 1.3 | `path/test.ext` | Unit | ✅ 2/2 | ✅ Written | ✅ Passed | ✅ 2 cases | ➖ None needed |

### Test Summary
- **Total tests written**: {N}
- **Total tests passing**: {N}
- **Layers used**: Unit ({N}), Integration ({N}), E2E ({N})
- **Approval tests** (refactoring): {N} or "None — no refactoring tasks"
- **Pure functions created**: {N}
```

**Column definitions**:
- **Safety Net**: Pre-existing tests run before modifying files. "N/A (new)" for new files.
- **RED**: Test written first, referencing code that doesn't exist yet. Always "✅ Written".
- **GREEN**: Tests executed and passing after minimal implementation. Must show execution result.
- **TRIANGULATE**: Additional test cases added to force real logic. "➖ Single" if spec has only one scenario.
- **REFACTOR**: Code improved with tests still passing. "➖ None needed" if code was already clean.

## Rules (Strict TDD specific)

- NEVER write production code before writing its test — this is the ONE rule that cannot be broken
- NEVER skip the GREEN execution gate — you MUST run tests and confirm they pass
- NEVER skip triangulation when the spec defines multiple scenarios — hardcoded Fake It must be forced out
- ALWAYS run the Safety Net before modifying existing files — protect what already works
- ALWAYS report the TDD Cycle Evidence table — the verify phase will check it
- If a test runner execution fails for infrastructure reasons (not test failures), report as "Blocked" and continue to next task
- Prefer pure functions — but don't force it where it doesn't fit (e.g., React components with state)
- For refactoring tasks, ALWAYS write approval tests before touching code
- Run ONLY the relevant test file during the cycle, not the full suite
