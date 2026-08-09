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

The YAML envelope MUST be the first non-empty content and contains every field exactly once. Omit both `scope` and `slice_id` for the default whole-change report. Whole-change totals come from retrieved specifications and must exactly match the authoritative `--requirements` and `--scenarios` caller totals. A slice report MUST set `scope: slice` and the exact provider-owned runtime objective ID in `slice_id`; its requirement and scenario totals derive only from that objective's immutable assignments. Admission rejects malformed, unknown, partial, mismatched, duplicate, overlapping, contradictory, or count-mismatched evidence. A canonical failure with blocker, critical, command-exit, or incomplete evidence is valid and persistable but not archive-ready. Human prose after the envelope never controls routing. Model/provider/profile/effort selection remains user-owned.

For a scoped slice, add these fields inside the same YAML envelope:

```yaml
scope: slice
slice_id: sha256:{provider-owned-runtime-objective-id}
```

Hold the complete report as exact candidate bytes before any OpenSpec or Engram write. For a whole change, run `gentle-ai sdd-verify-validate --input <path|-> --requirements <n> --scenarios <n>` with required authoritative nonnegative caller totals. For a slice, run `gentle-ai sdd-verify-validate --input <path|-> --cwd <repo> --change <name> --scope slice --slice-id <provider-owned-runtime-objective-id>` and never provide caller totals. If the validator is unavailable or denies admission, make zero writes and preserve the prior report; otherwise persist the same bytes, including a valid `fail`.

## Authority-Only Preflight Denial

When authoritative preflight alone denies entry because review authority is missing, emit the normal failed envelope plus exactly these five recovery fields:

```yaml
authority_only_failure: true
missing_review_authority: true
substantive_failure: false
command_failed: false
observed_authority_revision: sha256:{observed-authority-revision}
```

The observed revision is the authority revision read during the denied preflight. The declared test and build commands must not be executed; use `test_exit_code: 125` and `build_exit_code: 125`, with both output hashes set to the SHA-256 digest of exact empty output. Exit `125` describes preflight denial, so `command_failed` remains `false`. Never emit this recovery shape for substantive verification failures, executed command failures, malformed authority, or unknown authority.

When Strict TDD is active, insert the TDD compliance, test layer distribution, changed-file coverage, and quality metrics sections from `strict-tdd-verify.md`.
