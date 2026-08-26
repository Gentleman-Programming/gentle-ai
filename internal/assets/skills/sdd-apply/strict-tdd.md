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
│   ├── For qualifying behavioral work, execute the focused test and capture
│   │   the assertion/observable-behavior failure; a written claim is invalid
│   ├── A missing-symbol or compile failure is structural RED and does not
│   │   satisfy behavioral RED; make the smallest compilable surface first
│   ├── If the production code/function already exists:
│   │   └── Write and execute a test for the NEW behavior that is not implemented
│   └── GATE: For qualifying behavioral work, do NOT proceed to GREEN until
│       executed behavioral RED is captured (assertion/observable-behavior
│       failure with command, exit status, failure class, and bounded output).
│       A written-only or structural RED is insufficient. For other work, do NOT
│       proceed to GREEN until the test is written
│
├── 3. GREEN — Write the MINIMUM code to pass
│   ├── Implement ONLY what the failing test needs
│   ├── Fake It is VALID here (hardcoded return values are OK)
│   ├── EXECUTE tests → must PASS
│   │   ├── ✅ Passed → proceed to TRIANGULATE or REFACTOR
│   │   └── ❌ Failed → fix the implementation, NOT the test
│   └── GATE: Do NOT proceed until GREEN is confirmed by execution
│
├── 4. TRIANGULATE (MANDATORY for most tasks)
│   ├── DEFAULT: triangulation is REQUIRED. You need a compelling reason to skip it.
│   ├── Add a second test case with DIFFERENT inputs/expected outputs
│   ├── EXECUTE tests → if Fake It breaks (hardcoded no longer works):
│   │   └── Generalize to real logic (this is the whole point)
│   ├── Repeat until ALL spec scenarios for this task are covered
│   ├── Each triangulation pass: write test → run → fix implementation
│   ├── For behavioral work, use semantic partitions rather than a fixed case
│   │   count. Include the cases needed to reject the simplest constant, literal,
│   │   or single-example special-case implementation (anti-Fake-It).
│   ├── WATCH OUT for GREEN that passes trivially:
│   │   ├── If your test passes because the component/element isn't rendered → NOT a real GREEN
│   │   ├── If your test passes because a loop iterates 0 times → NOT a real GREEN
│   │   ├── If your test passes because the setup doesn't trigger the code path → NOT a real GREEN
│   │   └── A real GREEN means: production code RAN and produced the expected output
│   ├── Skip triangulation ONLY when ALL of these are true:
│   │   ├── The task is purely structural (config file, constant definition, type export)
│   │   ├── There is literally ONE possible output (no branching, no logic)
│   │   └── You explicitly note "Triangulation skipped: {reason}" in the evidence table
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
- **RED**: Test written first and, for qualifying behavior, an executed behavioral failure with command, exit status, class, and bounded output.
- **GREEN**: Tests executed and passing after minimal implementation. Must show execution result and the candidate production identity.
- **TRIANGULATE**: Semantic partitions and the verifier-owned anti-Fake-It decision, not a raw case-count score.
- **REFACTOR**: Code improved with tests still passing. "➖ None needed" if code was already clean.

## Assertion Quality Rules (MANDATORY)

**Every assertion must verify REAL behavior.** A test that passes without exercising production logic is worse than no test — it gives false confidence.

### Banned Assertion Patterns (NEVER write these)

```
# TRIVIAL ASSERTIONS — test proves nothing
expect(true).toBe(true)              # ❌ Tautology
expect(false).toBe(false)            # ❌ Tautology
expect(1).toBe(1)                    # ❌ Tautology — no production code involved
assert True                          # ❌ Always passes
assert 1 == 1                        # ❌ Always passes

# EMPTY COLLECTION ASSERTIONS without setup context
expect(result).toEqual([])           # ❌ ONLY valid if you set up conditions for empty
expect(result).toHaveLength(0)       # ❌ Same — why is it empty? Did production code run?
assert len(result) == 0              # ❌ Same — prove the emptiness comes from real logic
assert result == []                  # ❌ Same

# TYPE-ONLY ASSERTIONS — proves existence, not behavior
expect(result).toBeDefined()         # ❌ Alone is useless — WHAT is the value?
expect(result).not.toBeNull()        # ❌ Alone is useless — assert the actual value
expect(typeof result).toBe('object') # ❌ Alone is useless — what does the object contain?
assert result is not None            # ❌ Alone — assert what result actually IS

# GHOST LOOP — assertion inside a loop that iterates 0 times
const items = screen.queryAllByTestId("item");  // returns []
for (const item of items) {
  expect(item).toHaveTextContent("value");       # ❌ NEVER EXECUTES — loop body is dead code
}
# FIX: assert the collection is non-empty FIRST, or set up data so it IS non-empty:
expect(items).toHaveLength(3);                   # ✅ Proves items exist
for (const item of items) { ... }                # ✅ Now the loop actually runs

# INCOMPLETE TDD CYCLE — GREEN without TRIANGULATE
# If your GREEN test passes because the setup doesn't exercise the code path,
# you are NOT done. You MUST triangulate with a setup that DOES exercise it.
# Example: testing "search doesn't update until Enter" but the component
# that receives the search is never rendered → the test proves nothing.
# FIX: add a test where the component IS rendered and verify the behavior.
```

### What Makes a REAL Assertion

Every test assertion must satisfy ALL of these:
1. **Calls production code** — the test invokes a function, method, or component from the implementation
2. **Asserts a specific output** — compares against a concrete expected value derived from the spec
3. **Would FAIL if the production code were wrong** — if you change the implementation logic, THIS test breaks

```
# ✅ REAL assertions — production code determines the result
expect(calculateDiscount(100, 10)).toBe(10)       # Real input → real output
expect(screen.getByText('Welcome, John')).toBeInTheDocument()  # Rendered from data
assert result[0].status == "FAIL"                  # Specific finding from check execution
assert response.status_code == 403                 # Real HTTP response from the endpoint
expect(result).toHaveLength(3)                     # AND you set up exactly 3 items
```

### Empty Collection Rule

`expect(result).toEqual([])` or `assert len(result) == 0` is ONLY valid when:
1. You set up a specific precondition that SHOULD produce an empty result (e.g., no matching records)
2. The production code actually ran and filtered/processed data to arrive at empty
3. A companion test with different setup produces a NON-EMPTY result (triangulation)

If you cannot explain WHY the result is empty based on setup → the assertion is trivial.

### Smoke Test Rule

A test that only renders a component without asserting any output is NOT a valid test:

```
# ❌ SMOKE TEST ONLY — proves nothing about behavior
render(<MyComponent data={mockData} />);
expect(screen.getByTestId("wrapper")).toBeInTheDocument();  # Just proves it rendered

# ✅ BEHAVIORAL TEST — proves what the component DOES with the data
render(<MyComponent data={mockData} />);
expect(screen.getByText("Expected Title")).toBeInTheDocument();  # Verifies output from data
expect(screen.getByRole("button")).toHaveTextContent("Submit");  # Verifies real content
```

"Renders without crash" is a smoke test. It is NOT a unit test, NOT an integration test, and it does NOT count toward TDD coverage. If you need a smoke test, it must be accompanied by real behavioral assertions.

### Mock Hygiene Rules

**If you need more mocks than assertions, you are testing at the WRONG level.**

```
Mock/assertion ratio guide:
├── ≤ 3 mocks for a test file → ✅ Healthy — focused test
├── 4–6 mocks → ⚠️ Consider extracting logic to a pure function
├── 7+ mocks → ❌ STOP — you are testing at the wrong layer
│   ├── Extract the logic under test to a PURE FUNCTION and test it without mocks
│   ├── OR move the test to integration/E2E layer where real dependencies exist
│   └── NEVER write 10+ mocks to verify a one-line transformation
```

**Extract-Before-Mock Rule**: If the behavior you want to test is a data transformation, mapping, filtering, or conditional logic (e.g., `MUTED → FAIL` status conversion), EXTRACT it to a pure function FIRST, then test the pure function directly. No mocks needed.

```
# ❌ BAD: 15 mocks to test a one-line status conversion
vi.mock("next/navigation", ...);
vi.mock("next/link", ...);
vi.mock("@/components/shadcn", ...);
// ... 12 more mocks ...
render(<StatusCell row={mutedRow} />);
expect(screen.getByText("FAIL")).toBeInTheDocument();

# ✅ GOOD: extract and test the logic directly
// In production code:
export function resolveDisplayStatus(status: string, isMuted: boolean): string {
  return status === "MUTED" ? "FAIL" : status;
}

// In test — ZERO mocks needed:
expect(resolveDisplayStatus("MUTED", true)).toBe("FAIL");
expect(resolveDisplayStatus("PASS", false)).toBe("PASS");
```

### Implementation Detail Coupling Rule

Tests must assert **behavior visible to the user**, not internal implementation details:

```
# ❌ COUPLED TO IMPLEMENTATION — breaks on any style refactor
expect(element.className).toContain("text-xs");
expect(element.className).toContain("-mt-2.5");
expect(element.className).toContain("border-border-error-primary");
expect(element.style.color).toBe("red");

# ❌ COUPLED TO INTERNALS — breaks when implementation changes
expect(mockService.mock.calls.length).toBe(3);  # Why 3? Brittle.
expect(component.state.isLoading).toBe(true);    # Internal state, not behavior.

# ✅ BEHAVIORAL — survives refactors, tests what users see
expect(screen.getByText("Error: Payment failed")).toBeInTheDocument();
expect(screen.getByRole("alert")).toHaveTextContent("Risk:");
expect(screen.getByRole("button")).toBeDisabled();
```

**CSS class assertions are NEVER valid test assertions.** If you need to verify visual styling:
1. Test the **semantic outcome** (e.g., element has `role="alert"`, text is visible, button is disabled)
2. OR use a visual regression tool / E2E screenshot comparison
3. NEVER assert specific Tailwind/CSS class names — they are implementation details

## Rules (Strict TDD specific)

- NEVER write production code before writing its test — this is the ONE rule that cannot be broken
- NEVER skip the GREEN execution gate — you MUST run tests and confirm they pass
- NEVER skip triangulation when the spec defines multiple scenarios — hardcoded Fake It must be forced out
- NEVER write trivial assertions (see Banned Assertion Patterns above) — they are WORSE than no test
- ALWAYS verify that every assertion CALLS production code and asserts a SPECIFIC expected value
- ALWAYS run the Safety Net before modifying existing files — protect what already works
- ALWAYS report the TDD Cycle Evidence table — the verify phase will check it
- If a test runner execution fails for infrastructure reasons (not test failures), report as "Blocked" and continue to next task
- Prefer pure functions — but don't force it where it doesn't fit (e.g., React components with state)
- For refactoring tasks, ALWAYS write approval tests before touching code
- Run ONLY the relevant test file during the cycle, not the full suite

## Behavioral Falsification Evidence Contract

This is the canonical prompt/artifact contract for a qualifying behavioral Strict TDD work unit. It supplements the ordinary TDD cycle; it does not create a native ledger or runtime enforcement.

### Qualification and artifact schema

Apply this contract only when the work has an implementation-independent oracle from an approved spec scenario, protocol, business rule, or invariant; a deterministic focused command passes on the candidate; the repository is Git-backed; and an exact disposable worktree can be reconstructed. Documentation, formatting, generated-only, structural-only, and infrastructure-only work remains on its existing evidence method.

The apply artifact MUST record these identities and fields:

| Field | Required evidence |
|---|---|
| `behavior_id` | Protected behavior and its boundary. |
| `oracle_source` | Implementation-independent source and the scenario/invariant it defines. |
| `focused_command` | Exact deterministic command, working directory, timeout, and exit status. |
| `test_identity` | Full focused test-source manifest: path, mode, and SHA-256 for every test, fixture, and test configuration file. |
| `production_identity` | A manifest and SHA-256 root for affected production paths before and after GREEN. |
| `base_revision` | Base commit SHA used for every reconstruction. |
| `pre_green_snapshot` | Content-addressed, reconstructible snapshot retained until counterfactual verification completes. |
| `verification_handoff` | `verifier_selection: pending`, `verifier_execution: pending`, and actor-described `initial_residual_risk`; verifier alone records final `fault`, `counterfactual_result`, and residual-risk decision. |
| `red` / `green` | Result class, exit status, bounded output excerpt, command, and the matching source identities. |

### Executed RED and renewal

The qualifying RED gate MUST require an executed behavioral failure before GREEN; a written-only or structural RED is insufficient.

Behavioral RED MUST be executed, not merely written or inferred. Record the exact evidence label `executed behavioral RED`; it must fail at the assertion or observable-behavior level for the named behavior. A missing-symbol compile failure is `structural RED`; structural RED does not satisfy behavioral RED. Record `expected_failure_class`, `observed_failure_class`, exit status, and a bounded output excerpt.

The `pre_green_snapshot` MUST contain the base commit SHA, retained binary diff of the full pre-GREEN tracked tree from that base, retained untracked-file bytes, a sorted full-worktree path/mode/SHA-256 index, and the SHA-256 of the canonical bundle index. Retain it until counterfactual verification completes; the current production tree is not a substitute.

Any material test changes after RED invalidate the prior RED. If the oracle, focused setup, fixture, command, test, or test configuration changes materially after RED, overlay the changed test inputs on a materialization of the retained `pre_green_snapshot`, execute the focused command again, and record renewed RED. Missing, hash-mismatched, or non-reconstructible snapshot evidence is `unavailable` and blocks renewed RED.

### GREEN, partitions, and anti-Fake-It

GREEN runs the same `focused_command` against the implementation candidate and records the candidate `production_identity`, test identity, exit status, and successful output. Freeze the focused command and full test-source manifest after GREEN and before fault selection.

Choose semantic partitions that rule out the simplest constant, literal, or single-example special-case implementation. Transformations need discriminating input/output pairs and defined boundaries; stateful behavior needs distinct transitions or repeated calls; error behavior needs each contract-relevant success/failure partition. A true single-point invariant may use one case only when the artifact explains why no meaningful second partition exists. Additional cases count only when they reject a distinct simple implementation, exercise a distinct contract partition, or protect a distinct state transition.

The verifier owns the anti-Fake-It decision and records exactly:

- `anti_fake_it.applicable`;
- `anti_fake_it.simplest_rejected_implementation`;
- `anti_fake_it.discriminating_tests`;
- `anti_fake_it.decision` (`pass` or `fail`); and
- `anti_fake_it.rationale`.

The primary fault MUST target the recorded simplest rejected implementation when `anti_fake_it.applicable` is true. If it is not applicable, the verifier records why. A failed anti-Fake-It decision blocks fault execution; raw test count and coverage are informational only.

### Canonical candidate and disposable isolation

Before any execution, the verifier retains the binary tracked delta and untracked bytes and creates one canonical candidate manifest. Its canonical UTF-8 JSON index contains the base commit SHA, the SHA-256 of the retained binary tracked delta, and lexicographically sorted entries for every candidate path, including intended untracked files, with path, mode, and SHA-256. `candidate_root` is the SHA-256 of those exact JSON bytes.

The identity predicate is: materialize the base, apply the indexed tracked delta, copy only the indexed untracked files, recompute the complete index, and require byte-for-byte index equality and the same `candidate_root`. Before every execution, a mismatch is `unavailable`. The fault is never applied to the source worktree or candidate.

Use one exact disposable detached Git worktree for each independent materialization. Record cleanup on every exit path; cleanup failure is reported explicitly and never mutates the source worktree. Unsupported submodules, sparse checkouts, ignored runtime dependencies, or any candidate that cannot be reconstructed exactly are `unavailable` unless the verifier proves an exact candidate-manifest match.

### Verifier-owned fault and fixed stability budget

After test freeze and a passing anti-Fake-It decision, the verifier-owned fault selection—not the implementation actor—chooses one plausible production-only fault from the approved oracle, changed control flow, boundary, error path, or state transition. Never derive a fault by changing an expected value to a random alternative.

The fixed stability budget is exactly two candidate runs plus two counterfactual runs: two independently materialized unmodified candidates must pass at the canonical `candidate_root`; then two independently materialized counterfactuals with the verifier-selected production-only patch must run the same focused command at the recorded post-fault production root. There is no rerun loop and no additional rerun loop. Both counterfactuals must fail for the protected behavior with the expected failure class and matching bounded output evidence. Any disagreement, compile/setup/unrelated-crash/timeout failure, missing protected-test result, class mismatch, or identity mismatch is `invalid` or `unavailable`, never `killed`.

Fault selection is verifier-owned and limited to one primary and one replacement only when the primary outcome is `equivalent` or `invalid`. A valid `survived` fault is not replaceable. If both selections are equivalent or invalid, report `inconclusive` and exhaust the replacement budget.

### Exact outcomes and degradation

Each attempt records exactly one outcome:

| Outcome | Meaning |
|---|---|
| `killed` | Both candidate runs pass and both valid counterfactual runs fail the protected behavior at the recorded post-fault production root. |
| `survived` | Candidate and valid counterfactual both pass. |
| `equivalent` | The plausible fault does not change observable behavior under the approved oracle. |
| `invalid` | The intended plausible fault is not materialized, frozen scope changes, or an unrelated compile/setup/crash/timeout/harness failure occurs. |
| `unavailable` | Exact isolation or deterministic execution cannot be established. |
| `not-applicable` | The work unit is outside qualifying behavioral scope and uses its existing evidence method. |

Every record includes `protected_test_or_behavior`, `expected_failure_class`, `observed_failure_class`, `observed_output_excerpt`, and `post_fault_production_root`. Only `killed` is successful falsification; no other outcome inflates success.

`survived` is always a CRITICAL verification finding. `inconclusive` and `unavailable` are WARNING for ordinary behavior and CRITICAL for authorization, security, update, delivery, payment, migration/persistence, or data-loss behavior. `not-applicable` is informational. `killed` passes this evidence gate but retains residual risk and does not replace requirements verification. A CRITICAL result blocks verification; remediation requires a new executed RED/GREEN cycle and test freeze, with at most one bounded remediation attempt.

### Issue boundaries and pilot disclosure

#3727 owns this behavioral falsification prompt/artifact pilot. #262 owns mandatory phase progression, native settlement, and guarantees that verification cannot be skipped. #986 owns project-wide risk classification and evidence-method rubrics. #1263 owns optional mutation-provider integration and Gherkin; no universal mutation-score threshold is adopted here.

This pilot does not claim native historical proof or universal enforcement. Native ledgers, runtime/CLI enforcement, project-wide classification, mutation providers, Gherkin, and broad mutation campaigns are out of scope. Benchmark the pilot on the approved corpus before proposing native ledger work.
