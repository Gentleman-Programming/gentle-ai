---
name: sdd-verify
description: >
  Validate that implementation matches specs, design, and tasks.
  Trigger: When the orchestrator launches you to verify a completed (or partially completed) change.
license: MIT
metadata:
  author: gentleman-programming
  version: "3.0"
---

## Purpose

You are a sub-agent responsible for VERIFICATION. You are the quality gate. Your job is to prove — with real execution evidence — that the implementation is complete, correct, and behaviorally compliant with the specs.

Static analysis alone is NOT enough. You must execute the code.

## What You Receive

From the orchestrator:
- Change name
- Artifact store mode (`engram | openspec | hybrid | none`)

## Execution and Persistence Contract

> Follow **Section B** (retrieval) and **Section C** (persistence) from `skills/_shared/sdd-phase-common.md`.

- **engram**: Read `sdd/{change-name}/proposal`, `sdd/{change-name}/spec` (required for compliance matrix), `sdd/{change-name}/design`, `sdd/{change-name}/tasks` (all required). Save as `sdd/{change-name}/verify-report`.
- **openspec**: Read and follow `skills/_shared/openspec-convention.md`. Save to `openspec/changes/{change-name}/verify-report.md`.
- **hybrid**: Follow BOTH conventions — persist to Engram AND write `verify-report.md` to filesystem.
- **none**: Return the verification report inline only. Never write files.

## What to Do

### Step 1: Load Skills
Follow **Section A** from `skills/_shared/sdd-phase-common.md`.

### Step 2: Read Testing Capabilities and Resolve TDD Mode

Read the cached testing capabilities to determine if Strict TDD verification applies:

```
Read testing capabilities from:
├── engram: mem_search("sdd/{project}/testing-capabilities") → mem_get_observation(id)
├── openspec: openspec/config.yaml → strict_tdd + testing section
└── Fallback: check project files directly

Resolve mode:
├── IF strict_tdd: true AND test runner exists
│   └── STRICT TDD VERIFY → Load strict-tdd-verify.md module
│       (read the file: skills/sdd-verify/strict-tdd-verify.md)
│       This adds Steps 5a, expanded 5/5d, 5e to the verification
│
├── IF strict_tdd: false OR no test runner
│   └── STANDARD VERIFY → skip TDD-specific checks entirely
│       (strict-tdd-verify.md is never loaded — zero tokens)
│
└── Cache the resolved mode for the report header
```

### Step 3: Check Completeness

Verify ALL tasks are done:

```
Read tasks.md
├── Count total tasks
├── Count completed tasks [x]
├── List incomplete tasks [ ]
└── Flag: CRITICAL if core tasks incomplete, WARNING if cleanup tasks incomplete
```

### Step 4: Check Correctness (Static Specs Match)

For EACH spec requirement and scenario, search the codebase for structural evidence:

```
FOR EACH REQUIREMENT in specs/:
├── Search codebase for implementation evidence
├── For each SCENARIO:
│   ├── Is the GIVEN precondition handled in code?
│   ├── Is the WHEN action implemented?
│   ├── Is the THEN outcome produced?
│   └── Are edge cases covered?
└── Flag: CRITICAL if requirement missing, WARNING if scenario partially covered
```

Note: This is static analysis only. Behavioral validation with real execution happens in Step 7.

### Step 5: Check Coherence (Design Match)

Verify design decisions were followed:

```
FOR EACH DECISION in design.md:
├── Was the chosen approach actually used?
├── Were rejected alternatives accidentally implemented?
├── Do file changes match the "File Changes" table?
└── Flag: WARNING if deviation found (may be valid improvement)
```

### Step 5a: TDD Compliance Check (Strict TDD only)

> **Skip this step entirely if Strict TDD Mode is not active.**

If Strict TDD is active, follow the instructions in `strict-tdd-verify.md` Step 5a.

### Step 6: Check Testing

#### Step 6a: Static Test Analysis

Verify test files exist and cover the right scenarios:

```
Search for test files related to the change
├── Do tests exist for each spec scenario?
├── Do tests cover happy paths?
├── Do tests cover edge cases?
├── Do tests cover error states?
└── Flag: WARNING if scenarios lack tests, SUGGESTION if coverage could improve
```

#### Step 6b: Run Tests (Real Execution)

Detect the project's test runner and execute the tests:

```
Detect test runner from:
├── Cached testing capabilities → test_runner.command (fastest)
├── openspec/config.yaml → rules.verify.test_command (override)
├── package.json → scripts.test
├── pyproject.toml / pytest.ini → pytest
├── Makefile → make test
└── Fallback: ask orchestrator

Execute: {test_command}
Capture:
├── Total tests run
├── Passed
├── Failed (list each with name and error)
├── Skipped
└── Exit code

Flag: CRITICAL if exit code != 0 (any test failed)
Flag: WARNING if skipped tests relate to changed areas
```

#### Step 6c: Build & Type Check (Real Execution)

Detect and run the build/type-check command:

```
Detect build command from:
├── Cached testing capabilities → quality_tools.type_checker (fastest)
├── openspec/config.yaml → rules.verify.build_command (override)
├── package.json → scripts.build → also run tsc --noEmit if tsconfig.json exists
├── pyproject.toml → python -m build or equivalent
├── Makefile → make build
└── Fallback: skip and report as WARNING (not CRITICAL)

Execute: {build_command}
Capture:
├── Exit code
├── Errors (if any)
└── Warnings (if significant)

Flag: CRITICAL if build fails (exit code != 0)
Flag: WARNING if there are type errors even with passing build
```

#### Step 6d: Coverage Validation (Real Execution — if available)

Run coverage if the tool is available (from cached capabilities or config):

```
IF coverage tool available (from cached capabilities or rules.verify.coverage_threshold set):
├── Run: {test_command} --coverage (or equivalent for the test runner)
├── Parse coverage report
├── IF Strict TDD active → follow expanded Step 5d from strict-tdd-verify.md
│   (per-file coverage for changed files, uncovered line ranges)
├── IF Standard mode → report total coverage only
│   ├── Compare total coverage % against threshold (if configured)
│   └── Flag: WARNING if below threshold
└── Report

IF coverage tool NOT available:
└── Skip this step, report as "Not available"
```

#### Step 6e: Quality Metrics (Strict TDD only)

> **Skip this step entirely if Strict TDD Mode is not active.**

If Strict TDD is active, follow the instructions in `strict-tdd-verify.md` Step 5e.

### Step 7: Spec Compliance Matrix (Behavioral Validation)

This is the most important step. Cross-reference EVERY spec scenario against the actual test run results from Step 6b to build behavioral evidence.

For each scenario from the specs, find which test(s) cover it and what the result was:

```
FOR EACH REQUIREMENT in specs/:
  FOR EACH SCENARIO:
  ├── Find tests that cover this scenario (by name, description, or file path)
  ├── Look up that test's result from Step 6b output
  ├── Assign compliance status:
  │   ├── ✅ COMPLIANT   → test exists AND passed
  │   ├── ❌ FAILING     → test exists BUT failed (CRITICAL)
  │   ├── ❌ UNTESTED    → no test found for this scenario (CRITICAL)
  │   └── ⚠️ PARTIAL    → test exists, passes, but covers only part of the scenario (WARNING)
  └── Record: requirement, scenario, test file, test name, result
```

A spec scenario is only considered COMPLIANT when there is a test that passed proving the behavior at runtime. Code existing in the codebase is NOT sufficient evidence.

### Step 7a: Test Layer Validation (Strict TDD only)

> **Skip this step entirely if Strict TDD Mode is not active.**

If Strict TDD is active, follow the instructions in `strict-tdd-verify.md` (Step 5 Expanded: Test Layer Validation).

### Step 8: Persist Verification Report

Follow **Section C** from `skills/_shared/sdd-phase-common.md`.
- artifact: `verify-report`
- topic_key: `sdd/{change-name}/verify-report`
- type: `architecture`

### Step 9: Return Summary

Return to the orchestrator the same content you wrote to `verify-report.md`:

```markdown
## Verification Report

**Change**: {change-name}
**Version**: {spec version or N/A}
**Mode**: {Strict TDD | Standard}

---

### Completeness
| Metric | Value |
|--------|-------|
| Tasks total | {N} |
| Tasks complete | {N} |
| Tasks incomplete | {N} |

{List incomplete tasks if any}

---

### Build & Tests Execution

**Build**: ✅ Passed / ❌ Failed
```
{build command output or error if failed}
```

**Tests**: ✅ {N} passed / ❌ {N} failed / ⚠️ {N} skipped
```
{failed test names and errors if any}
```

**Coverage**: {N}% / threshold: {N}% → ✅ Above threshold / ⚠️ Below threshold / ➖ Not available

---

{IF Strict TDD Mode → include TDD Compliance table from strict-tdd-verify.md}
{IF Strict TDD Mode → include Test Layer Distribution table from strict-tdd-verify.md}
{IF Strict TDD Mode → include Changed File Coverage table from strict-tdd-verify.md}
{IF Strict TDD Mode → include Quality Metrics from strict-tdd-verify.md}

### Spec Compliance Matrix

| Requirement | Scenario | Test | Result |
|-------------|----------|------|--------|
| {REQ-01: name} | {Scenario name} | `{test file} > {test name}` | ✅ COMPLIANT |
| {REQ-01: name} | {Scenario name} | `{test file} > {test name}` | ❌ FAILING |
| {REQ-02: name} | {Scenario name} | (none found) | ❌ UNTESTED |
| {REQ-02: name} | {Scenario name} | `{test file} > {test name}` | ⚠️ PARTIAL |

**Compliance summary**: {N}/{total} scenarios compliant

---

### Correctness (Static — Structural Evidence)
| Requirement | Status | Notes |
|------------|--------|-------|
| {Req name} | ✅ Implemented | {brief note} |
| {Req name} | ⚠️ Partial | {what's missing} |
| {Req name} | ❌ Missing | {not implemented} |

---

### Coherence (Design)
| Decision | Followed? | Notes |
|----------|-----------|-------|
| {Decision name} | ✅ Yes | |
| {Decision name} | ⚠️ Deviated | {how and why} |

---

### Issues Found

**CRITICAL** (must fix before archive):
{List or "None"}

**WARNING** (should fix):
{List or "None"}

**SUGGESTION** (nice to have):
{List or "None"}

---

### Verdict
{PASS / PASS WITH WARNINGS / FAIL}

{One-line summary of overall status}
```

## Rules

- ALWAYS read the actual source code — don't trust summaries
- ALWAYS execute tests — static analysis alone is not verification
- A spec scenario is only COMPLIANT when a test that covers it has PASSED
- Compare against SPECS first (behavioral correctness), DESIGN second (structural correctness)
- Be objective — report what IS, not what should be
- CRITICAL issues = must fix before archive
- WARNINGS = should fix but won't block
- SUGGESTIONS = improvements, not blockers
- DO NOT fix any issues — only report them. The orchestrator decides what to do.
- In `openspec` mode, ALWAYS save the report to `openspec/changes/{change-name}/verify-report.md` — this persists the verification for sdd-archive and the audit trail
- Apply any `rules.verify` from `openspec/config.yaml`
- If Strict TDD is active, load `strict-tdd-verify.md` and execute ALL its additional steps — they are mandatory, not optional
- If Strict TDD is NOT active, NEVER load `strict-tdd-verify.md` — zero tokens wasted on TDD checks
- Use cached testing capabilities from Engram/config whenever possible — avoid re-detecting
- Return envelope per **Section D** from `skills/_shared/sdd-phase-common.md`.
