# Tasks: Recover SDD Verification from Compact Authority

## Review Workload

- Delivery: single PR with `size:exception`
- Authored-line ceiling: 800
- Runtime boundary: temporary Git repositories with compact stores and multiple lineages

## Implementation

- [x] 1.1 Add RED cases for absent, exact, invalid, denied, foreign, traversal, and ambiguous authority.
- [x] 1.2 Add multi-lineage coverage for an observed stale predecessor and a newer live authority.
- [x] 2.1 Implement canonical active-change path binding and stable compact authority discovery.
- [x] 2.2 Route exactly one valid authority to verify only without synthesizing mirrors.
- [x] 3.1 Parse strict authority-only failed-report evidence and require changed authority.
- [x] 3.2 Keep substantive, command-failed, malformed, unchanged, and unrelated denied lineages blocked.
- [x] 4.1 Update embedded `sdd-verify` instructions, report format, and asset contract tests.
- [x] 5.1 Run focused tests, full tests, vet, formatting, and diff checks.
