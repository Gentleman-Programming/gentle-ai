# SDD Verify Report Format

## Compliance Statuses

- ✅ `COMPLIANT`: covering test exists and passed.
- ❌ `FAILING`: covering test exists but failed.
- ❌ `UNTESTED`: no covering test found.
- ⚠️ `PARTIAL`: test passes but covers only part of the scenario.

## Report Template

~~~markdown
```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:{current-evidence-digest}
verdict: pass
blockers: 0
critical_findings: 0
requirements: {complete}/{actual-total}
scenarios: {complete}/{actual-total}
test_command: {exact command}
test_exit_code: 0
test_output_hash: sha256:{exact-output-digest}
build_command: {exact command}
build_exit_code: 0
build_output_hash: sha256:{exact-output-digest}
```

## Verification Report

**Change**: {change-name}
**Version**: {spec version or N/A}
**Mode**: {Strict TDD | Standard}

### Completeness
| Metric | Value |
|--------|-------|
| Tasks total | {N} |
| Tasks complete | {N} |
| Tasks incomplete | {N} |

### Build & Tests Execution
**Build**: ✅ Passed / ❌ Failed
```text
{build command and relevant output}
```

**Tests**: ✅ {N} passed / ❌ {N} failed / ⚠️ {N} skipped
```text
{test command and failure details}
```

**Coverage**: {N}% / threshold: {N}% → ✅ Above / ⚠️ Below / ➖ Not available

### Spec Compliance Matrix
| Requirement | Scenario | Test | Result |
|-------------|----------|------|--------|
| {REQ-01} | {Scenario} | `{file} > {test}` | ✅ COMPLIANT |
| {REQ-02} | {Scenario} | (none found) | ❌ UNTESTED |

**Compliance summary**: {N}/{total} scenarios compliant

### Correctness (Static Evidence)
| Requirement | Status | Notes |
|------------|--------|-------|
| {Req name} | ✅ Implemented | {brief note} |

### Coherence (Design)
| Decision | Followed? | Notes |
|----------|-----------|-------|
| {Decision} | ✅ Yes | |

### Issues Found
**CRITICAL**: {list or None}
**WARNING**: {list or None}
**SUGGESTION**: {list or None}

### Verdict
{PASS / PASS WITH WARNINGS / FAIL}
{one-line reason}
~~~

The YAML envelope MUST be the first non-empty content and contains every field exactly once. Counts come from the actual retrieved specs. Admission rejects malformed, unknown, missing, contradictory, or count-mismatched evidence. A canonical failure with blocker, critical, command-exit, or incomplete evidence is valid and persistable but not archive-ready. Human prose after the envelope never controls routing. Model/provider/profile/effort selection remains user-owned.

Before persistence, hold the complete report as exact candidate bytes and run `gentle-ai sdd-verify-validate --input <path|-> --requirements <n> --scenarios <n>` before any OpenSpec or Engram write. If the validator is unavailable or denies admission, make zero writes and preserve the prior report; otherwise persist the same bytes, including a valid `fail`.

## Review Context and Verification Availability

Review state is informational and never a verification prerequisite. A missing, pending, invalid, or non-allow review state never suppresses tests or builds. Exit `125` is reserved for an actual verification prerequisite or unavailable verification tooling, never missing review authority.

When a genuine prerequisite for the declared verification command is unavailable, record that condition using the established verification-result schema and preserve the command/output evidence it permits. Do not invent review-authority fields or treat review state as a command preflight. Substantive verification failures and executed command failures remain ordinary `fail` evidence.

When Strict TDD is active, insert the TDD compliance, test layer distribution, changed-file coverage, and quality metrics sections from `strict-tdd-verify.md`.
